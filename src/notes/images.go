package notes

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	gio "github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	previewImageMaxWidth  = 560
	previewImageMaxHeight = 320
)

var importableImageMIMETypes = []string{
	"image/png",
	"image/jpeg",
	"image/gif",
	"image/webp",
	"image/tiff",
}

var importableImageLocationMIMETypes = []string{
	"text/uri-list",
	"public.file-url",
	"public.url",
}

var imageWriteFile = os.WriteFile

func (n *Note) renderPreviewImages(images []markdownImage) {
	if n.previewBuffer == nil || n.preview == nil {
		return
	}

	tab := n.activeTab()
	if tab == nil {
		return
	}

	for i := len(images) - 1; i >= 0; i-- {
		image := images[i]
		texture, err := previewTextureForImage(tab.path, image.Path)
		if err != nil {
			continue
		}
		start := n.previewBuffer.IterAtOffset(image.Offset)
		end := n.previewBuffer.IterAtOffset(image.Offset + 1)
		n.previewBuffer.Delete(start, end)
		n.previewBuffer.InsertPaintable(start, gdk.BaseTexture(texture))
	}
}

func previewTextureForImage(notePath string, imagePath string) (gdk.Texturer, error) {
	resolved, err := resolveNoteImagePath(notePath, imagePath)
	if err != nil {
		return nil, err
	}
	pixbuf, err := gdkpixbuf.NewPixbufFromFileAtScale(resolved, previewImageMaxWidth, previewImageMaxHeight, true)
	if err != nil {
		return nil, err
	}
	return gdk.NewTextureForPixbuf(pixbuf), nil
}

func resolveNoteImagePath(notePath string, imagePath string) (string, error) {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return "", fmt.Errorf("image path is empty")
	}
	if filepath.IsAbs(imagePath) {
		return filepath.Clean(imagePath), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(notePath), filepath.FromSlash(imagePath))), nil
}

func (n *Note) setupImageClipboardAndDrop(tab *noteTab) {
	tab.note.ConnectPasteClipboard(func() {
		n.tryPasteImageFromClipboard()
	})

	tab.note.AddController(n.imageDropTargetForTab(tab))
	if tab.editorScroll != nil {
		tab.editorScroll.AddController(n.imageDropTargetForTab(tab))
	}
}

func (n *Note) imageDropTargetForTab(tab *noteTab) *gtk.DropTargetAsync {
	formatsBuilder := gdk.NewContentFormatsBuilder()
	formatsBuilder.AddGType(gio.GTypeFile)
	formatsBuilder.AddGType(gdk.GTypeTexture)
	for _, mimeType := range importableImageLocationMIMETypes {
		formatsBuilder.AddMIMEType(mimeType)
	}
	for _, mimeType := range importableImageMIMETypes {
		formatsBuilder.AddMIMEType(mimeType)
	}

	target := gtk.NewDropTargetAsync(formatsBuilder.ToFormats(), gdk.ActionCopy)
	target.ConnectAccept(func(drop gdk.Dropper) bool {
		return dropHasImportableImage(drop)
	})
	target.ConnectDragEnter(func(drop gdk.Dropper, _, _ float64) gdk.DragAction {
		if dropHasImportableImage(drop) {
			return gdk.ActionCopy
		}
		return 0
	})
	target.ConnectDragMotion(func(drop gdk.Dropper, _, _ float64) gdk.DragAction {
		if dropHasImportableImage(drop) {
			return gdk.ActionCopy
		}
		return 0
	})
	target.ConnectDrop(func(drop gdk.Dropper, _, _ float64) bool {
		return n.handleImageDrop(tab, drop)
	})
	return target
}

func (n *Note) tryPasteImageFromClipboard() bool {
	if n.note == nil {
		return false
	}
	clipboard := gtk.BaseWidget(n.note).Clipboard()
	if clipboard == nil {
		return false
	}
	startOffset, endOffset := n.currentInsertRange()
	if clipboardHasImportableImage(clipboard) {
		if clipboard.Formats().ContainGType(gdk.GTypeTexture) {
			clipboard.ReadTextureAsync(context.Background(), func(result gio.AsyncResulter) {
				texture, err := clipboard.ReadTextureFinish(result)
				if err != nil || texture == nil {
					return
				}
				glib.IdleAdd(func() {
					tab := n.activeTab()
					if tab == nil {
						return
					}
					if _, err := n.importTextureIntoNoteAt(tab, texture, "Pasted image", "pasted-image", startOffset, endOffset); err != nil {
						n.statusMessage("Image paste failed: " + err.Error())
					}
				})
			})
			return true
		}

		clipboard.ReadAsync(context.Background(), importableImageMIMETypes, 0, func(result gio.AsyncResulter) {
			mimeType, stream, err := clipboard.ReadFinish(result)
			if err != nil || stream == nil {
				return
			}
			data, readErr := readInputStreamAll(stream)
			if closeErr := gio.BaseInputStream(stream).Close(context.Background()); closeErr != nil && readErr == nil {
				readErr = closeErr
			}
			if readErr != nil {
				return
			}
			glib.IdleAdd(func() {
				tab := n.activeTab()
				if tab == nil {
					return
				}
				ext, ok := imageExtensionForMIMEType(mimeType)
				if !ok {
					n.statusMessage("Image paste failed: unsupported clipboard image format")
					return
				}
				markdown, err := writeImageAsset(tab.path, data, ext, "pasted-image", "Pasted image")
				if err != nil {
					n.statusMessage("Image paste failed: " + err.Error())
					return
				}
				n.insertTextIntoBufferAt(markdown, startOffset, endOffset)
			})
		})
		return true
	}
	return false
}

func (n *Note) importDroppedImageFile(tab *noteTab, sourcePath string) bool {
	if tab == nil {
		return false
	}
	markdown, err := importImageFileIntoNote(tab.path, sourcePath)
	if err != nil {
		n.statusMessage("Image import failed: " + err.Error())
		return false
	}
	n.insertTextIntoActiveBuffer(markdown)
	n.statusMessage("Image imported.")
	return true
}

func (n *Note) importDroppedTexture(tab *noteTab, texture gdk.Texturer) bool {
	if tab == nil || texture == nil {
		return false
	}
	if _, err := n.importTextureIntoNote(tab, texture, "Pasted image", "pasted-image"); err != nil {
		n.statusMessage("Image import failed: " + err.Error())
		return false
	}
	n.statusMessage("Image imported.")
	return true
}

func (n *Note) importTextureIntoNote(tab *noteTab, texture gdk.Texturer, alt string, baseName string) (string, error) {
	markdown, err := importTextureIntoNote(tab.path, texture, alt, baseName)
	if err != nil {
		return "", err
	}
	n.insertTextIntoActiveBuffer(markdown)
	return markdown, nil
}

func (n *Note) importTextureIntoNoteAt(tab *noteTab, texture gdk.Texturer, alt string, baseName string, startOffset int, endOffset int) (string, error) {
	markdown, err := importTextureIntoNote(tab.path, texture, alt, baseName)
	if err != nil {
		return "", err
	}
	n.insertTextIntoBufferAt(markdown, startOffset, endOffset)
	return markdown, nil
}

func importImageFileIntoNote(notePath string, sourcePath string) (string, error) {
	ext, ok := normalizedImageExtension(sourcePath)
	if !ok {
		return "", fmt.Errorf("unsupported image format")
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	baseName := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	return writeImageAsset(notePath, data, ext, baseName, baseName)
}

func importImageFileIntoFolderPath(targetFolder string, sourcePath string) (string, error) {
	ext, ok := normalizedImageExtension(sourcePath)
	if !ok {
		return "", fmt.Errorf("unsupported image format")
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	baseName := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	return writeImageAssetToFolder(targetFolder, data, ext, baseName)
}

func importTextureIntoNote(notePath string, texture gdk.Texturer, alt string, baseName string) (string, error) {
	targetPath, relPath, err := nextImageAssetPath(notePath, baseName, ".png")
	if err != nil {
		return "", err
	}
	if ok := gdk.BaseTexture(texture).SaveToPNG(targetPath); !ok {
		return "", fmt.Errorf("could not save clipboard image")
	}
	return imageMarkdown(alt, relPath), nil
}

func importTextureIntoFolder(targetFolder string, texture gdk.Texturer, baseName string) (string, error) {
	targetPath, err := nextFolderImageAssetPath(targetFolder, baseName, ".png")
	if err != nil {
		return "", err
	}
	if ok := gdk.BaseTexture(texture).SaveToPNG(targetPath); !ok {
		return "", fmt.Errorf("could not save dropped image")
	}
	return targetPath, nil
}

func writeImageAsset(notePath string, data []byte, ext string, baseName string, alt string) (string, error) {
	targetPath, relPath, err := nextImageAssetPath(notePath, baseName, ext)
	if err != nil {
		return "", err
	}
	if err := imageWriteFile(targetPath, data, 0o644); err != nil {
		return "", err
	}
	return imageMarkdown(alt, relPath), nil
}

func writeImageAssetToFolder(targetFolder string, data []byte, ext string, baseName string) (string, error) {
	targetPath, err := nextFolderImageAssetPath(targetFolder, baseName, ext)
	if err != nil {
		return "", err
	}
	if err := imageWriteFile(targetPath, data, 0o644); err != nil {
		return "", err
	}
	return targetPath, nil
}

func nextImageAssetPath(notePath string, baseName string, ext string) (string, string, error) {
	imageDir := noteImageDir(notePath)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return "", "", err
	}
	baseName = sanitizeImageBaseName(baseName)
	if baseName == "" {
		baseName = fmt.Sprintf("image-%s", time.Now().Format("20060102-150405"))
	}
	candidate := filepath.Join(imageDir, baseName+ext)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, imagePathForMarkdown(notePath, candidate), nil
	}
	for idx := 2; ; idx++ {
		candidate = filepath.Join(imageDir, fmt.Sprintf("%s-%d%s", baseName, idx, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, imagePathForMarkdown(notePath, candidate), nil
		}
	}
}

func nextFolderImageAssetPath(targetFolder string, baseName string, ext string) (string, error) {
	folderPath := noteFolderPath(sanitizeFolderPath(targetFolder))
	if err := os.MkdirAll(folderPath, 0o755); err != nil {
		return "", err
	}
	baseName = sanitizeImageBaseName(baseName)
	if baseName == "" {
		baseName = fmt.Sprintf("image-%s", time.Now().Format("20060102-150405"))
	}
	candidate := filepath.Join(folderPath, baseName+ext)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}
	for idx := 2; ; idx++ {
		candidate = filepath.Join(folderPath, fmt.Sprintf("%s-%d%s", baseName, idx, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
}

func noteImageDir(notePath string) string {
	return filepath.Join(filepath.Dir(notePath), "images")
}

func imagePathForMarkdown(notePath string, imagePath string) string {
	rel, err := filepath.Rel(filepath.Dir(notePath), imagePath)
	if err != nil {
		return filepath.ToSlash(filepath.Base(imagePath))
	}
	return filepath.ToSlash(rel)
}

func sanitizeImageBaseName(name string) string {
	name = sanitizeNoteTitle(name)
	name = strings.ReplaceAll(strings.ToLower(name), " ", "-")
	return strings.Trim(name, "-")
}

func normalizedImageExtension(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".tif", ".tiff":
		return strings.ToLower(filepath.Ext(path)), true
	default:
		return "", false
	}
}

func imageMarkdown(alt string, relPath string) string {
	alt = strings.TrimSpace(alt)
	if alt == "" {
		alt = "image"
	}
	return fmt.Sprintf("![%s](%s)", alt, filepath.ToSlash(relPath))
}

func clipboardHasImportableImage(clipboard *gdk.Clipboard) bool {
	if clipboard == nil {
		return false
	}
	return formatsContainImportableImage(clipboard.Formats())
}

func dropHasImportableImage(drop gdk.Dropper) bool {
	if drop == nil {
		return false
	}
	formats := gdk.BaseDrop(drop).Formats()
	if formatsContainImportableImage(formats) {
		return true
	}
	return firstImportableImageLocationMIMEType(formats) != ""
}

func formatsContainImportableImage(formats *gdk.ContentFormats) bool {
	if formats == nil {
		return false
	}
	if formats.ContainGType(gio.GTypeFile) {
		return true
	}
	if formats.ContainGType(gdk.GTypeTexture) {
		return true
	}
	for _, mimeType := range importableImageMIMETypes {
		if formats.ContainMIMEType(mimeType) {
			return true
		}
	}
	return false
}

func (n *Note) handleImageDrop(tab *noteTab, drop gdk.Dropper) bool {
	if tab == nil || drop == nil {
		return false
	}

	baseDrop := gdk.BaseDrop(drop)
	formats := baseDrop.Formats()
	if formats == nil {
		return false
	}

	if formats.ContainGType(gio.GTypeFile) {
		baseDrop.ReadValueAsync(context.Background(), gio.GTypeFile, 0, func(result gio.AsyncResulter) {
			value, err := baseDrop.ReadValueFinish(result)
			if err != nil || value == nil {
				baseDrop.Finish(0)
				return
			}
			sourcePath, ok := dropValueFilePath(value.GoValue())
			if !ok {
				baseDrop.Finish(0)
				return
			}
			glib.IdleAdd(func() {
				if n.importDroppedImageFile(tab, sourcePath) {
					baseDrop.Finish(gdk.ActionCopy)
					return
				}
				baseDrop.Finish(0)
			})
		})
		return true
	}

	if formats.ContainGType(gdk.GTypeTexture) {
		baseDrop.ReadValueAsync(context.Background(), gdk.GTypeTexture, 0, func(result gio.AsyncResulter) {
			value, err := baseDrop.ReadValueFinish(result)
			if err != nil || value == nil {
				baseDrop.Finish(0)
				return
			}
			texture, ok := value.GoValue().(gdk.Texturer)
			if !ok || texture == nil {
				baseDrop.Finish(0)
				return
			}
			glib.IdleAdd(func() {
				if n.importDroppedTexture(tab, texture) {
					baseDrop.Finish(gdk.ActionCopy)
					return
				}
				baseDrop.Finish(0)
			})
		})
		return true
	}

	if mimeType := firstImportableImageLocationMIMEType(formats); mimeType != "" {
		return n.readDroppedImageLocation(tab, baseDrop, mimeType)
	}

	mimeType := firstImportableImageMIMEType(formats)
	if mimeType == "" {
		return false
	}
	return n.readDroppedImageStream(tab, baseDrop, mimeType)
}

func (n *Note) readDroppedImageLocation(tab *noteTab, drop *gdk.Drop, mimeType string) bool {
	drop.ReadAsync(context.Background(), []string{mimeType}, 0, func(result gio.AsyncResulter) {
		_, stream, err := drop.ReadFinish(result)
		if err != nil || stream == nil {
			drop.Finish(0)
			return
		}
		data, readErr := readInputStreamAll(stream)
		if closeErr := gio.BaseInputStream(stream).Close(context.Background()); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			drop.Finish(0)
			return
		}

		paths := imagePathsFromDropPayload(string(data))
		if len(paths) == 0 {
			drop.Finish(0)
			return
		}

		glib.IdleAdd(func() {
			var imported []string
			for _, sourcePath := range paths {
				markdown, err := importImageFileIntoNote(tab.path, sourcePath)
				if err != nil {
					continue
				}
				imported = append(imported, markdown)
			}
			if len(imported) == 0 {
				drop.Finish(0)
				n.statusMessage("Image import failed.")
				return
			}
			n.insertTextIntoActiveBuffer(strings.Join(imported, "\n"))
			n.statusMessage("Image imported.")
			drop.Finish(gdk.ActionCopy)
		})
	})
	return true
}

func (n *Note) readDroppedImageStream(tab *noteTab, drop *gdk.Drop, mimeType string) bool {
	drop.ReadAsync(context.Background(), []string{mimeType}, 0, func(result gio.AsyncResulter) {
		usedMimeType, stream, err := drop.ReadFinish(result)
		if err != nil || stream == nil {
			drop.Finish(0)
			return
		}
		data, readErr := readInputStreamAll(stream)
		if closeErr := gio.BaseInputStream(stream).Close(context.Background()); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			drop.Finish(0)
			return
		}

		glib.IdleAdd(func() {
			ext, ok := imageExtensionForMIMEType(usedMimeType)
			if !ok {
				drop.Finish(0)
				n.statusMessage("Image import failed: unsupported image format")
				return
			}
			markdown, err := writeImageAsset(tab.path, data, ext, "dropped-image", "Dropped image")
			if err != nil {
				drop.Finish(0)
				n.statusMessage("Image import failed: " + err.Error())
				return
			}
			n.insertTextIntoActiveBuffer(markdown)
			n.statusMessage("Image imported.")
			drop.Finish(gdk.ActionCopy)
		})
	})
	return true
}

func firstImportableImageMIMEType(formats *gdk.ContentFormats) string {
	if formats == nil {
		return ""
	}
	for _, mimeType := range importableImageMIMETypes {
		if formats.ContainMIMEType(mimeType) {
			return mimeType
		}
	}
	return ""
}

func firstImportableImageLocationMIMEType(formats *gdk.ContentFormats) string {
	if formats == nil {
		return ""
	}
	for _, mimeType := range importableImageLocationMIMETypes {
		if formats.ContainMIMEType(mimeType) {
			return mimeType
		}
	}
	return ""
}

func imageExtensionForMIMEType(mimeType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	case "image/tiff":
		return ".tiff", true
	default:
		return "", false
	}
}

func readInputStreamAll(stream gio.InputStreamer) ([]byte, error) {
	if stream == nil {
		return nil, fmt.Errorf("stream is nil")
	}
	input := gio.BaseInputStream(stream)
	var data []byte
	for {
		chunk, err := input.ReadBytes(context.Background(), 32*1024)
		if err != nil {
			return nil, err
		}
		if chunk == nil || chunk.Size() == 0 {
			return data, nil
		}
		data = append(data, chunk.Data()...)
		if chunk.Size() < 32*1024 {
			return data, nil
		}
	}
}

func imagePathsFromDropPayload(payload string) []string {
	uris := glib.URIListExtractURIs(payload)
	if len(uris) == 0 {
		trimmed := strings.TrimSpace(payload)
		if trimmed != "" {
			uris = []string{trimmed}
		}
	}

	paths := make([]string, 0, len(uris))
	for _, rawURI := range uris {
		parsed, err := url.Parse(strings.TrimSpace(rawURI))
		if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
			continue
		}
		path := filepath.Clean(parsed.Path)
		if path == "" {
			continue
		}
		if _, ok := normalizedImageExtension(path); !ok {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func dropValueFilePath(value interface{}) (string, bool) {
	switch file := value.(type) {
	case *gio.File:
		path := file.Path()
		if _, ok := normalizedImageExtension(path); ok && path != "" {
			return path, true
		}
	case interface{ Path() string }:
		path := file.Path()
		if _, ok := normalizedImageExtension(path); ok && path != "" {
			return path, true
		}
	}
	return "", false
}

func (n *Note) insertTextIntoActiveBuffer(insert string) {
	startOffset, endOffset := n.currentInsertRange()
	n.insertTextIntoBufferAt(insert, startOffset, endOffset)
}

func (n *Note) currentInsertRange() (int, int) {
	if n.buffer == nil {
		return 0, 0
	}
	if start, end, ok := n.buffer.SelectionBounds(); ok {
		return start.Offset(), end.Offset()
	}
	cursor := n.buffer.IterAtMark(n.buffer.GetInsert())
	offset := cursor.Offset()
	return offset, offset
}

func (n *Note) insertTextIntoBufferAt(insert string, startOffset int, endOffset int) {
	if n.buffer == nil {
		return
	}
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		start = clampRuneOffset(text, startOffset)
		end = clampRuneOffset(text, endOffset)
		if end < start {
			end = start
		}
		updated := replaceRunes(text, start, end, insert)
		cursor := start + len([]rune(insert))
		return updated, cursor, cursor
	})
}

func clampRuneOffset(text string, offset int) int {
	runes := []rune(text)
	if offset < 0 {
		return 0
	}
	if offset > len(runes) {
		return len(runes)
	}
	return offset
}

func replaceRunes(text string, start int, end int, insert string) string {
	runes := []rune(text)
	return string(runes[:start]) + insert + string(runes[end:])
}
