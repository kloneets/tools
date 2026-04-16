package notes

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

const managedAssetsDir = "assets"

var fileDraftRoot string

func (w *Workspace) IsFilesEditableContext() bool {
	return w != nil && (w.FileCommandMode || w.FileFilterMode)
}

func (w *Workspace) HandleFilesKey(key Key) bool {
	if w == nil {
		return false
	}
	if w.FileCommandMode {
		return w.handleFileCommandKey(key)
	}
	if w.FileFilterMode {
		return w.handleFileFilterKey(key)
	}
	switch key.Name {
	case "down", "j":
		if w.FileSelection < len(w.FileTree)-1 {
			w.FileSelection++
		}
		return true
	case "up", "k":
		if w.FileSelection > 0 {
			w.FileSelection--
		}
		return true
	case "enter", "l":
		entry := w.selectedFileEntry()
		if entry != nil && (entry.Kind == fileEntryScope || entry.Kind == fileEntryFolder) {
			w.toggleFileFolder(entry.Path)
			return true
		}
		return entry != nil
	case "a":
		w.FileScopeOnly = false
		w.FileCommandMode = true
		w.FileCommand = "import "
		return true
	case "f":
		w.FileScopeOnly = false
		w.FileCommandMode = true
		w.FileCommand = "mkdir "
		return true
	case "F":
		w.FileScopeOnly = true
		w.FileCommandMode = true
		w.FileCommand = "mkdir "
		return true
	case "r", "R":
		if entry := w.selectedFileEntry(); entry != nil && entry.Kind != fileEntryScope {
			w.FileCommandMode = true
			w.FileCommand = "rename " + entry.Label
			return true
		}
		return false
	case "m":
		if entry := w.selectedFileEntry(); entry != nil && entry.Kind != fileEntryScope {
			w.FileCommandMode = true
			w.FileCommand = "move "
			return true
		}
		return false
	case "i":
		if err := w.InsertSelectedFileReference(); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "inserted reference into note"
		return true
	case "I":
		if err := w.InsertSelectedFileReferenceAs(markdownInsertLink); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "inserted link into note"
		return true
	case "p":
		if err := w.InsertSelectedFileReferenceAs(markdownInsertImage); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "inserted image into note"
		return true
	case "o":
		if err := w.OpenSelectedFileExternally(); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "opened"
		return true
	case "d":
		if err := w.DeleteSelectedFileEntry(); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "deleted"
		return true
	case "y":
		if err := copyManagedMarkdownRef(w); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "copied markdown reference"
		return true
	case "Y":
		if err := copyManagedRelativePath(w); err != nil {
			w.FileStatus = err.Error()
			return true
		}
		w.FileStatus = "copied relative path"
		return true
	case "/":
		w.FileFilterMode = true
		return true
	case "M":
		if w.PendingMigrationCount <= 0 {
			w.FileStatus = "no loose files to migrate"
			return true
		}
		w.FileCommandMode = true
		w.FileCommand = "migrate"
		return true
	case ":":
		w.FileScopeOnly = false
		w.FileCommandMode = true
		w.FileCommand = ""
		return true
	}
	return false
}

func (w *Workspace) handleFileFilterKey(key Key) bool {
	switch key.Name {
	case "esc":
		w.FileFilterMode = false
		return true
	case "backspace":
		if len(w.FileFilter) > 0 {
			_, size := utf8.DecodeLastRuneInString(w.FileFilter)
			w.FileFilter = w.FileFilter[:len(w.FileFilter)-size]
			w.refreshFiles()
		}
		return true
	case "enter":
		w.FileFilterMode = false
		return true
	}
	if key.Rune != 0 {
		w.FileFilter += string(key.Rune)
		w.refreshFiles()
		return true
	}
	return false
}

func (w *Workspace) handleFileCommandKey(key Key) bool {
	switch key.Name {
	case "esc":
		w.FileCommandMode = false
		w.FileScopeOnly = false
		w.FileCommand = ""
		return true
	case "backspace":
		if len(w.FileCommand) > 0 {
			_, size := utf8.DecodeLastRuneInString(w.FileCommand)
			w.FileCommand = w.FileCommand[:len(w.FileCommand)-size]
		}
		return true
	case "enter":
		err := w.executeFileCommand(strings.TrimSpace(w.FileCommand))
		w.FileCommandMode = false
		w.FileScopeOnly = false
		w.FileCommand = ""
		if err != nil {
			w.FileStatus = err.Error()
		}
		return true
	}
	if key.Rune != 0 {
		w.FileCommand += string(key.Rune)
		return true
	}
	return false
}

func (w *Workspace) executeFileCommand(cmd string) error {
	switch {
	case strings.HasPrefix(cmd, "import "):
		return w.ImportManagedPaths(strings.TrimSpace(strings.TrimPrefix(cmd, "import ")))
	case strings.HasPrefix(cmd, "mkdir "):
		return w.CreateManagedFolder(strings.TrimSpace(strings.TrimPrefix(cmd, "mkdir ")))
	case strings.HasPrefix(cmd, "rename "):
		return w.RenameSelectedFileEntry(strings.TrimSpace(strings.TrimPrefix(cmd, "rename ")))
	case strings.HasPrefix(cmd, "move "):
		return w.MoveSelectedFileEntry(strings.TrimSpace(strings.TrimPrefix(cmd, "move ")))
	case cmd == "insert":
		return w.InsertSelectedFileReference()
	case cmd == "migrate":
		moved, err := migrateLooseManagedFiles()
		if err != nil {
			return err
		}
		w.PendingMigrationCount = 0
		w.refreshFiles()
		w.FilesDirty = moved > 0
		w.FileStatus = fmt.Sprintf("staged %d loose file(s) into assets/", moved)
		if moved == 0 {
			w.FileStatus = "no loose files to migrate"
		}
		return nil
	case cmd == "":
		return nil
	default:
		return fmt.Errorf("unknown file command")
	}
}

func (w *Workspace) FileRows(height int) []string {
	width := normalizeSidebarWidth(w.SidebarWidth)
	scopeLabel := w.currentFileScopeLabel()
	title := helpers.ANSI(helpers.ANSIBold, "Managed files")
	if w.FilesDirty {
		title += " " + helpers.ANSI(helpers.ANSIFgYellow, "[STAGED]")
	}
	lines := []string{
		helpers.TruncateANSI(title, width),
		helpers.TruncateANSI("Scope: "+scopeLabel, width),
	}
	if w.FileFilter != "" {
		lines = append(lines, helpers.TruncateANSI("Filter: "+w.FileFilter, width))
	}
	for i, entry := range w.FileTree {
		marker := " "
		if i == w.FileSelection {
			marker = helpers.ANSI(helpers.ANSIReverse, ">")
		}
		icon := "*"
		switch entry.Kind {
		case fileEntryScope:
			if entry.Collapsed {
				icon = "@+"
			} else {
				icon = "@-"
			}
		case fileEntryFolder:
			if entry.Collapsed {
				icon = "+"
			} else {
				icon = "-"
			}
		case fileEntryAsset:
			if entry.Image {
				icon = "img:" + managedFileType(&entry)
			} else {
				icon = "file:" + managedFileType(&entry)
			}
		}
		label := strings.Repeat("  ", entry.Depth) + icon + " " + entry.Label
		lines = append(lines, helpers.TruncateANSI(marker+" "+label, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

func (w *Workspace) FilePreviewRows(width int, height int) []string {
	entry := w.selectedFileEntry()
	scopeLabel := w.currentFileScopeLabel()
	if entry == nil {
		return padFilePreview([]string{"Current scope: " + scopeLabel, "", "No managed files"}, width, height)
	}
	lines := []string{
		helpers.TruncateANSI("Current scope: "+scopeLabel, width),
	}
	switch entry.Kind {
	case fileEntryScope:
		childFiles, childFolders := w.scopeChildCounts(entry.Scope)
		lines = append(lines,
			helpers.TruncateANSI("Scope folder: "+scopeLabel, width),
			helpers.TruncateANSI(fmt.Sprintf("Assets path: %s", filepath.ToSlash(noteAssetsRel(entry.Scope))), width),
			helpers.TruncateANSI(fmt.Sprintf("Children: %d folders, %d files", childFolders, childFiles), width),
			"",
			helpers.TruncateANSI("Actions: a import | f nested | F scope folder | o open", width),
		)
		return padFilePreview(lines, width, height)
	case fileEntryFolder:
		childFiles, childFolders := w.folderChildCounts(entry.Path)
		lines = append(lines,
			helpers.TruncateANSI("Folder: "+entry.Label, width),
			helpers.TruncateANSI(fmt.Sprintf("Scope: %s", scopeLabel), width),
			helpers.TruncateANSI(fmt.Sprintf("Children: %d folders, %d files", childFolders, childFiles), width),
			"",
			helpers.TruncateANSI("Actions: a import | f nested | F scope folder | r rename | m move | d delete | o open", width),
		)
		return padFilePreview(lines, width, height)
	default:
		lines = append(lines,
			helpers.TruncateANSI(fmt.Sprintf("Path: %s", filepath.ToSlash(entry.RelPath)), width),
			helpers.TruncateANSI(fmt.Sprintf("Name: %s", entry.Label), width),
			helpers.TruncateANSI(fmt.Sprintf("Type: %s", managedFileType(entry)), width),
			helpers.TruncateANSI(fmt.Sprintf("Size: %d bytes", entry.Size), width),
			"",
			helpers.TruncateANSI("Markdown: "+w.referenceForFile(entry, markdownInsertSmart), width),
			helpers.TruncateANSI("Relative: "+w.relativePathForEntry(entry), width),
			"",
		)
		lines = append(lines, w.filePreviewContent(entry.Path, width, max(0, height-len(lines)))...)
		return padFilePreview(lines, width, height)
	}
}

func padFilePreview(lines []string, width int, height int) []string {
	for i := range lines {
		lines[i] = helpers.TruncateANSI(lines[i], width)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		return lines[:height]
	}
	return lines
}

func (w *Workspace) filePreviewContent(path string, width int, height int) []string {
	if height <= 0 {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if isImageExt(ext) {
		return []string{
			helpers.TruncateANSI("[image preview not rendered in terminal]", width),
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{helpers.TruncateANSI("Read failed: "+err.Error(), width)}
	}
	if !utf8.Valid(data) {
		return []string{helpers.TruncateANSI("[binary file]", width)}
	}
	text := strings.ReplaceAll(string(data), "\t", strings.Repeat(" ", settings.Inst().NotesApp.TabSpaces))
	lines := strings.Split(text, "\n")
	out := make([]string, 0, min(height, len(lines)))
	for i := 0; i < len(lines) && i < height; i++ {
		out = append(out, helpers.TruncateANSI(lines[i], width))
	}
	return out
}

func (w *Workspace) FileCommandLineText(width int) string {
	scopeLabel := w.currentFileScopeLabel()
	if w.FileCommandMode {
		switch {
		case strings.HasPrefix(w.FileCommand, "import "):
			return helpers.TruncateANSI("Import into scope "+scopeLabel+": "+w.FileCommand, width)
		case strings.HasPrefix(w.FileCommand, "mkdir "):
			mode := "nested"
			if w.FileScopeOnly {
				mode = "scope"
			}
			return helpers.TruncateANSI(fmt.Sprintf("Create %s folder in scope %s: %s", mode, scopeLabel, w.FileCommand), width)
		default:
			return helpers.TruncateANSI("["+scopeLabel+"] "+w.FileCommand, width)
		}
	}
	if w.FileFilterMode {
		return helpers.TruncateANSI("Filter files: /"+w.FileFilter, width)
	}
	if w.FileStatus != "" {
		return helpers.TruncateANSI(w.FileStatus, width)
	}
	if w.FilesDirty {
		return helpers.TruncateANSI("files staged locally | ctrl+s save | D discard staged changes", width)
	}
	if w.PendingMigrationCount > 0 {
		return helpers.TruncateANSI(fmt.Sprintf("%d loose file(s) outside assets/ | M confirm migrate | / filter | y copy md | Y copy path", w.PendingMigrationCount), width)
	}
	return helpers.TruncateANSI("scope="+scopeLabel+" | / filter | a import | f nested | F scope folder | o open | r rename | m move | d delete | i insert | y md-ref | Y rel-path | : command", width)
}

func (w *Workspace) FileCursor() (int, int, bool) {
	if w == nil || w.FileCommandMode || len(w.FileTree) == 0 {
		return 0, 0, false
	}
	row := 2
	if w.FileFilter != "" {
		row++
	}
	return w.FileSelection + row, 0, true
}

func (w *Workspace) FileCommandCursor() (int, bool) {
	if w == nil {
		return 0, false
	}
	if w.FileFilterMode {
		return len([]rune("Filter files: /" + w.FileFilter)), true
	}
	if !w.FileCommandMode {
		return 0, false
	}
	scopeLabel := w.currentFileScopeLabel()
	prefix := "[" + scopeLabel + "] "
	switch {
	case strings.HasPrefix(w.FileCommand, "import "):
		prefix = "Import into scope " + scopeLabel + ": "
	case strings.HasPrefix(w.FileCommand, "mkdir "):
		mode := "nested"
		if w.FileScopeOnly {
			mode = "scope"
		}
		prefix = fmt.Sprintf("Create %s folder in scope %s: ", mode, scopeLabel)
	}
	return len([]rune(prefix + w.FileCommand)), true
}

func (w *Workspace) selectedFileEntry() *FileEntry {
	if w == nil || w.FileSelection < 0 || w.FileSelection >= len(w.FileTree) {
		return nil
	}
	return &w.FileTree[w.FileSelection]
}

func (w *Workspace) currentFileScope() string {
	if entry := w.selectedFileEntry(); entry != nil {
		return entry.Scope
	}
	if ed := w.ActiveEditor(); ed != nil {
		return noteRelPath(ed.Path)
	}
	return ""
}

func (w *Workspace) currentFileScopeLabel() string {
	if entry := w.selectedFileEntry(); entry != nil && entry.ScopeLabel != "" {
		return entry.ScopeLabel
	}
	if ed := w.ActiveEditor(); ed != nil {
		return ed.Title
	}
	return "none"
}

func (w *Workspace) currentManagedFolderRel() string {
	if w.FileScopeOnly {
		return noteAssetsRel(w.currentFileScope())
	}
	if entry := w.selectedFileEntry(); entry != nil {
		switch entry.Kind {
		case fileEntryScope:
			return noteAssetsRel(entry.Scope)
		case fileEntryFolder:
			return entry.RelPath
		case fileEntryAsset:
			return entry.Folder
		}
	}
	return noteAssetsRel(w.currentFileScope())
}

func (w *Workspace) toggleFileFolder(key string) {
	for i := range w.FileTree {
		if w.FileTree[i].Path == key && (w.FileTree[i].Kind == fileEntryScope || w.FileTree[i].Kind == fileEntryFolder) {
			w.FileTree[i].Collapsed = !w.FileTree[i].Collapsed
			break
		}
	}
	w.refreshFiles()
}

func (w *Workspace) CreateManagedFolder(name string) error {
	if err := ensureManagedFilesDraft(); err != nil {
		return err
	}
	parent := w.currentManagedFolderRel()
	name = sanitizeFolderPath(name)
	if name == "" {
		name = "New Folder"
	}
	target := filepath.Join(parent, name)
	if err := os.MkdirAll(managedNoteFolderPath(target), 0o755); err != nil {
		return err
	}
	w.refreshFiles()
	w.FilesDirty = true
	w.FileStatus = "folder staged"
	return nil
}

func (w *Workspace) ImportManagedPath(source string) error {
	return w.importManagedSinglePath(source)
}

func (w *Workspace) ImportManagedPaths(sourceSpec string) error {
	parts := splitImportPaths(sourceSpec)
	if len(parts) == 0 {
		return fmt.Errorf("import requires at least one filesystem path")
	}
	imported := 0
	for _, part := range parts {
		if err := w.importManagedSinglePath(part); err != nil {
			if imported == 0 {
				return err
			}
			w.FileStatus = fmt.Sprintf("imported %d path(s), then failed: %v", imported, err)
			return nil
		}
		imported++
	}
	if imported > 1 {
		w.FileStatus = fmt.Sprintf("imported %d paths", imported)
	}
	return nil
}

func (w *Workspace) importManagedSinglePath(source string) error {
	if err := ensureManagedFilesDraft(); err != nil {
		return err
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("import requires a filesystem path")
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	targetFolderRel := w.currentManagedFolderRel()
	targetFolderPath := managedNoteFolderPath(targetFolderRel)
	if info.IsDir() {
		targetFolderPath = filepath.Join(targetFolderPath, filepath.Base(source))
		if err := copyDirectory(source, targetFolderPath); err != nil {
			return err
		}
	} else {
		dest := uniquePathLike(filepath.Join(targetFolderPath, filepath.Base(source)), "", false)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := copyFile(source, dest); err != nil {
			return err
		}
	}
	w.refreshFiles()
	w.FilesDirty = true
	w.FileStatus = "import staged"
	return nil
}

func splitImportPaths(spec string) []string {
	parts := strings.Split(spec, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func (w *Workspace) RenameSelectedFileEntry(name string) error {
	if err := ensureManagedFilesDraft(); err != nil {
		return err
	}
	entry := w.selectedFileEntry()
	if entry == nil || entry.Kind == fileEntryScope {
		return fmt.Errorf("select a file or folder")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("rename requires a name")
	}
	var target string
	if entry.Kind == fileEntryFolder {
		target = filepath.Join(filepath.Dir(entry.Path), sanitizeNoteTitle(name))
	} else {
		base := sanitizeNoteTitle(strings.TrimSuffix(name, filepath.Ext(name)))
		ext := filepath.Ext(entry.Path)
		if strings.HasSuffix(name, ext) {
			target = filepath.Join(filepath.Dir(entry.Path), name)
		} else {
			target = filepath.Join(filepath.Dir(entry.Path), base+ext)
		}
	}
	target = uniquePathLike(target, entry.Path, entry.Kind == fileEntryFolder)
	if target == entry.Path {
		return nil
	}
	if err := os.Rename(entry.Path, target); err != nil {
		return err
	}
	w.refreshFiles()
	w.FilesDirty = true
	w.FileStatus = "rename staged"
	return nil
}

func (w *Workspace) MoveSelectedFileEntry(target string) error {
	if err := ensureManagedFilesDraft(); err != nil {
		return err
	}
	entry := w.selectedFileEntry()
	if entry == nil || entry.Kind == fileEntryScope {
		return fmt.Errorf("select a file or folder")
	}
	targetRel, err := resolveManagedMoveTarget(target)
	if err != nil {
		return err
	}
	scopeRoot := managedNoteFolderPath(noteAssetsRel(entry.Scope))
	dest := filepath.Join(scopeRoot, targetRel, filepath.Base(entry.Path))
	dest = uniquePathLike(dest, entry.Path, entry.Kind == fileEntryFolder)
	if entry.Kind == fileEntryFolder && strings.HasPrefix(dest+string(filepath.Separator), entry.Path+string(filepath.Separator)) {
		return fmt.Errorf("cannot move a folder into itself")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(entry.Path, dest); err != nil {
		return err
	}
	w.refreshFiles()
	w.FilesDirty = true
	w.FileStatus = "move staged"
	return nil
}

func (w *Workspace) DeleteSelectedFileEntry() error {
	if err := ensureManagedFilesDraft(); err != nil {
		return err
	}
	entry := w.selectedFileEntry()
	if entry == nil || entry.Kind == fileEntryScope {
		return fmt.Errorf("select a file or folder")
	}
	var err error
	if entry.Kind == fileEntryFolder {
		err = os.RemoveAll(entry.Path)
	} else {
		err = os.Remove(entry.Path)
	}
	if err != nil {
		return err
	}
	w.refreshFiles()
	w.FilesDirty = true
	w.FileStatus = "delete staged"
	return nil
}

func (w *Workspace) InsertSelectedFileReference() error {
	return w.InsertSelectedFileReferenceAs(markdownInsertSmart)
}

type markdownInsertMode int

const (
	markdownInsertSmart markdownInsertMode = iota
	markdownInsertLink
	markdownInsertImage
)

func (w *Workspace) InsertSelectedFileReferenceAs(mode markdownInsertMode) error {
	entry := w.selectedFileEntry()
	if entry == nil || entry.Kind != fileEntryAsset {
		return fmt.Errorf("select a file to insert")
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return fmt.Errorf("no active note")
	}
	insertTextAtCursor(ed, w.referenceForFile(entry, mode))
	return nil
}

func (w *Workspace) OpenSelectedFileExternally() error {
	entry := w.selectedFileEntry()
	if entry == nil {
		return fmt.Errorf("select a file or folder")
	}
	path := entry.Path
	if path == "" {
		return fmt.Errorf("selected entry has no path")
	}
	helpers.OpenURI(pathToFileURI(path))
	return nil
}

func pathToFileURI(path string) string {
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return u.String()
}

func (w *Workspace) referenceForFile(entry *FileEntry, mode markdownInsertMode) string {
	ed := w.ActiveEditor()
	target := entryRealPath(entry)
	rel := filepath.Base(entry.Path)
	if ed != nil {
		if path, err := filepath.Rel(filepath.Dir(ed.Path), target); err == nil {
			rel = filepath.ToSlash(path)
		}
	} else if entry.RelPath != "" {
		rel = filepath.ToSlash(entry.RelPath)
	}
	stem := strings.TrimSuffix(filepath.Base(entry.Path), filepath.Ext(entry.Path))
	if mode == markdownInsertImage {
		if !entry.Image {
			return fmt.Sprintf("[%s](%s)", filepath.Base(entry.Path), rel)
		}
		return fmt.Sprintf("![%s](%s)", stem, rel)
	}
	if mode == markdownInsertLink {
		return fmt.Sprintf("[%s](%s)", filepath.Base(entry.Path), rel)
	}
	if entry.Image {
		return fmt.Sprintf("![%s](%s)", stem, rel)
	}
	return fmt.Sprintf("[%s](%s)", filepath.Base(entry.Path), rel)
}

func insertTextAtCursor(ed *Editor, text string) {
	if ed == nil {
		return
	}
	rememberUndoState(ed)
	runes := []rune(ed.Text)
	idx := vimClampOffset(ed.Text, ed.Cursor)
	insert := []rune(text)
	runes = append(runes[:idx], append(insert, runes[idx:]...)...)
	ed.Text = string(runes)
	ed.Cursor = idx + len(insert)
	ed.Dirty = true
	ed.Status = "modified"
}

func listManagedFiles() ([]FileEntry, error) {
	scopes, err := managedScopes()
	if err != nil {
		return nil, err
	}
	entries := make([]FileEntry, 0, 32)
	for _, scope := range scopes {
		scopeDepth := strings.Count(filepath.ToSlash(scope.Folder), "/")
		entries = append(entries, FileEntry{
			Kind:       fileEntryScope,
			Path:       noteAssetsPath(scope.Path),
			RelPath:    noteAssetsRel(scope.RelPath),
			Scope:      scope.RelPath,
			ScopeLabel: scope.Title,
			Label:      scope.Title,
			Depth:      scopeDepth,
			Folder:     scope.Folder,
		})
		assetRoot := managedNoteAssetsPath(scope.Path)
		subEntries, err := listManagedScopeEntries(scope, assetRoot, scopeDepth+1)
		if err != nil {
			return nil, err
		}
		entries = append(entries, subEntries...)
	}
	return entries, nil
}

func managedScopes() ([]noteFile, error) {
	files, err := listNoteFiles()
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return filepath.ToSlash(files[i].RelPath) < filepath.ToSlash(files[j].RelPath)
	})
	return files, nil
}

func listManagedScopeEntries(scope noteFile, assetRoot string, baseDepth int) ([]FileEntry, error) {
	entries := make([]FileEntry, 0, 8)
	info, err := os.Stat(assetRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return entries, nil
	}
	err = filepath.WalkDir(assetRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == assetRoot {
			return nil
		}
		rel, err := filepath.Rel(assetRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		depth := baseDepth + strings.Count(rel, "/")
		if d.IsDir() {
			entries = append(entries, FileEntry{
				Kind:       fileEntryFolder,
				Path:       path,
				RelPath:    filepath.ToSlash(targetRelativeToNotes(path)),
				Scope:      scope.RelPath,
				ScopeLabel: scope.Title,
				AssetRel:   rel,
				Label:      filepath.Base(path),
				Depth:      depth,
				Folder:     filepath.ToSlash(filepath.Dir(filepath.ToSlash(targetRelativeToNotes(path)))),
			})
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, FileEntry{
			Kind:       fileEntryAsset,
			Path:       path,
			RelPath:    filepath.ToSlash(targetRelativeToNotes(path)),
			Scope:      scope.RelPath,
			ScopeLabel: scope.Title,
			AssetRel:   rel,
			Label:      filepath.Base(path),
			Depth:      depth,
			Folder:     filepath.ToSlash(filepath.Dir(filepath.ToSlash(targetRelativeToNotes(path)))),
			Size:       info.Size(),
			Image:      isImageExt(strings.ToLower(filepath.Ext(path))),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left := filepath.ToSlash(entries[i].RelPath)
		right := filepath.ToSlash(entries[j].RelPath)
		if left == right && entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == fileEntryFolder
		}
		return left < right
	})
	return entries, nil
}

func (w *Workspace) refreshFilesWithFilter(entries []FileEntry) []FileEntry {
	if strings.TrimSpace(w.FileFilter) == "" {
		return entries
	}
	query := strings.ToLower(strings.TrimSpace(w.FileFilter))
	filtered := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind == fileEntryScope {
			filtered = append(filtered, entry)
			continue
		}
		target := strings.ToLower(filepath.ToSlash(entry.RelPath) + " " + entry.Label)
		if strings.Contains(target, query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func noteAssetsRel(noteRelPath string) string {
	noteRelPath = filepath.Clean(noteRelPath)
	if noteRelPath == "." || noteRelPath == "" {
		return ""
	}
	dir := filepath.Dir(noteRelPath)
	base := strings.TrimSuffix(filepath.Base(noteRelPath), filepath.Ext(noteRelPath))
	name := base + "." + managedAssetsDir
	if dir == "." || dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}

func noteAssetsPath(notePath string) string {
	if notePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(notePath), strings.TrimSuffix(filepath.Base(notePath), filepath.Ext(notePath))+"."+managedAssetsDir)
}

func managedNoteAssetsPath(notePath string) string {
	return filepath.Join(filepath.Dir(managedNotePath(noteRelPath(notePath))), strings.TrimSuffix(filepath.Base(notePath), filepath.Ext(notePath))+"."+managedAssetsDir)
}

func isManagedAssetsDirName(name string) bool {
	return name == managedAssetsDir || strings.HasSuffix(name, "."+managedAssetsDir)
}

func noteRelPath(path string) string {
	rel, err := filepath.Rel(notesDir(), path)
	if err != nil {
		return ""
	}
	if rel == "." {
		return ""
	}
	return rel
}

func targetRelativeToNotes(path string) string {
	rel, err := filepath.Rel(managedFilesRoot(), path)
	if err != nil {
		return path
	}
	return rel
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	default:
		return false
	}
}

func isManagedNotePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".txt"
}

func managedFileType(entry *FileEntry) string {
	if entry.Image {
		return "image"
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(entry.Path)), ".")
	if ext == "" {
		return "file"
	}
	return ext
}

func copyManagedMarkdownRef(w *Workspace) error {
	entry := w.selectedFileEntry()
	if entry == nil || entry.Kind != fileEntryAsset {
		return fmt.Errorf("select a file to copy")
	}
	return helpers.CopyToClipboard(w.referenceForFile(entry, markdownInsertSmart))
}

func copyManagedRelativePath(w *Workspace) error {
	entry := w.selectedFileEntry()
	if entry == nil || entry.Kind != fileEntryAsset {
		return fmt.Errorf("select a file to copy")
	}
	return helpers.CopyToClipboard(w.relativePathForEntry(entry))
}

func (w *Workspace) relativePathForEntry(entry *FileEntry) string {
	if entry == nil {
		return ""
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return filepath.ToSlash(entry.RelPath)
	}
	if path, err := filepath.Rel(filepath.Dir(ed.Path), entryRealPath(entry)); err == nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(entry.RelPath)
}

func copyFile(source string, dest string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}

func copyDirectory(source string, dest string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func uniquePathLike(target string, current string, isDir bool) string {
	if target == current {
		return target
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return target
	}
	base := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	ext := filepath.Ext(target)
	if isDir {
		base = filepath.Base(target)
		ext = ""
	}
	dir := filepath.Dir(target)
	for i := 2; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s %d%s", base, i, ext))
		if candidate == current {
			return candidate
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func resolveManagedMoveTarget(target string) (string, error) {
	target = strings.TrimSpace(strings.ReplaceAll(target, "\\", "/"))
	if target == "" || target == "/" || target == "." {
		return "", nil
	}
	return sanitizeFolderPath(target), nil
}

func (w *Workspace) scopeChildCounts(scope string) (int, int) {
	return w.folderChildCounts(managedNoteFolderPath(noteAssetsRel(scope)))
}

func (w *Workspace) folderChildCounts(path string) (int, int) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0
	}
	files := 0
	folders := 0
	for _, entry := range entries {
		if entry.IsDir() {
			folders++
		} else {
			files++
		}
	}
	return files, folders
}

func migrateLooseManagedFiles() (int, error) {
	if err := ensureManagedFilesDraft(); err != nil {
		return 0, err
	}
	root := managedFilesRoot()
	scopes, err := managedScopes()
	if err != nil {
		return 0, err
	}
	moved := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if isManagedAssetsDirName(filepath.Base(path)) {
				return filepath.SkipDir
			}
			if settings.IsTrashRelativePath(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if isManagedNotePath(path) || settings.IsTrashRelativePath(rel) {
			return nil
		}
		targetRel, err := migratedAssetTarget(rel, scopes)
		if err != nil {
			return err
		}
		targetPath := managedNoteFolderPath(targetRel)
		targetPath = uniquePathLike(targetPath, path, false)
		if targetPath == path {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.Rename(path, targetPath); err != nil {
			return err
		}
		moved++
		return nil
	})
	if err != nil {
		return moved, err
	}
	return moved, nil
}

func countLooseManagedFiles() (int, error) {
	root := notesDir()
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if isManagedAssetsDirName(filepath.Base(path)) || settings.IsTrashRelativePath(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if isManagedNotePath(path) || settings.IsTrashRelativePath(rel) {
			return nil
		}
		count++
		return nil
	})
	return count, err
}

type pathCompletion struct {
	Start  int
	End    int
	Prefix string
}

func completeEditorPathReference(w *Workspace, ed *Editor) bool {
	if w == nil || ed == nil {
		return false
	}
	ctx, ok := markdownPathCompletionContext(ed.Text, ed.Cursor)
	if !ok {
		clearAutoComplete(ed)
		return false
	}
	matches := managedReferenceCandidates(ed.Path, ctx.Prefix)
	if len(matches) == 0 {
		clearAutoComplete(ed)
		return false
	}
	matchIndex := 0
	if ed.AutoCompletePrefix == ctx.Prefix && ed.AutoCompleteStart == ctx.Start && len(ed.AutoCompleteMatches) > 0 {
		matchIndex = (ed.AutoCompleteIndex + 1) % len(ed.AutoCompleteMatches)
		matches = ed.AutoCompleteMatches
	}
	replaceRunes(ed, ctx.Start, ctx.End, matches[matchIndex])
	ed.AutoCompletePrefix = ctx.Prefix
	ed.AutoCompleteMatches = matches
	ed.AutoCompleteIndex = matchIndex
	ed.AutoCompleteStart = ctx.Start
	ed.AutoCompleteEnd = ctx.Start + len([]rune(matches[matchIndex]))
	ed.Dirty = true
	return true
}

func completeEditorPathReferenceBackward(w *Workspace, ed *Editor) bool {
	if w == nil || ed == nil {
		return false
	}
	ctx, ok := markdownPathCompletionContext(ed.Text, ed.Cursor)
	if !ok {
		clearAutoComplete(ed)
		return false
	}
	matches := managedReferenceCandidates(ed.Path, ctx.Prefix)
	if len(matches) == 0 {
		clearAutoComplete(ed)
		return false
	}
	matchIndex := len(matches) - 1
	if ed.AutoCompletePrefix == ctx.Prefix && ed.AutoCompleteStart == ctx.Start && len(ed.AutoCompleteMatches) > 0 {
		matchIndex = ed.AutoCompleteIndex - 1
		if matchIndex < 0 {
			matchIndex = len(ed.AutoCompleteMatches) - 1
		}
		matches = ed.AutoCompleteMatches
	}
	replaceRunes(ed, ctx.Start, ctx.End, matches[matchIndex])
	ed.AutoCompletePrefix = ctx.Prefix
	ed.AutoCompleteMatches = matches
	ed.AutoCompleteIndex = matchIndex
	ed.AutoCompleteStart = ctx.Start
	ed.AutoCompleteEnd = ctx.Start + len([]rune(matches[matchIndex]))
	ed.Dirty = true
	return true
}

func replaceRunes(ed *Editor, start int, end int, replacement string) {
	rememberUndoState(ed)
	runes := []rune(ed.Text)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if end < start {
		end = start
	}
	repl := []rune(replacement)
	runes = append(runes[:start], append(repl, runes[end:]...)...)
	ed.Text = string(runes)
	ed.Cursor = start + len(repl)
}

func markdownPathCompletionContext(text string, cursor int) (pathCompletion, bool) {
	runes := []rune(text)
	if cursor < 0 || cursor > len(runes) {
		return pathCompletion{}, false
	}
	start := -1
	for i := cursor - 1; i >= 0; i-- {
		switch runes[i] {
		case '\n', ')':
			return pathCompletion{}, false
		case '(':
			if i > 0 && runes[i-1] == ']' {
				start = i + 1
			}
			i = -1
		}
	}
	if start < 0 {
		return pathCompletion{}, false
	}
	end := cursor
	for end < len(runes) && runes[end] != ')' && runes[end] != '\n' {
		end++
	}
	return pathCompletion{Start: start, End: end, Prefix: string(runes[start:cursor])}, true
}

func managedReferenceCandidates(notePath string, prefix string) []string {
	root := managedFilesRoot()
	noteDir := filepath.Dir(notePath)
	candidates := make([]string, 0, 16)
	seen := make(map[string]struct{})
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		relToRoot, relErr := filepath.Rel(root, path)
		if relErr != nil || settings.IsTrashRelativePath(filepath.ToSlash(relToRoot)) {
			if d.IsDir() && relErr == nil && settings.IsTrashRelativePath(filepath.ToSlash(relToRoot)) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			relSlash := filepath.ToSlash(relToRoot)
			if isManagedAssetsDirName(filepath.Base(path)) ||
				strings.Contains(relSlash, "/"+managedAssetsDir+"/") ||
				strings.Contains(relSlash, "."+managedAssetsDir+"/") {
				ref, err := filepath.Rel(noteDir, path)
				if err == nil {
					ref = filepath.ToSlash(ref) + "/"
					if strings.HasPrefix(strings.ToLower(ref), strings.ToLower(prefix)) {
						if _, ok := seen[ref]; !ok {
							seen[ref] = struct{}{}
							candidates = append(candidates, ref)
						}
					}
				}
			}
			return nil
		}
		if isManagedNotePath(path) {
			return nil
		}
		ref, err := filepath.Rel(noteDir, path)
		if err != nil {
			return nil
		}
		ref = filepath.ToSlash(ref)
		if strings.HasPrefix(strings.ToLower(ref), strings.ToLower(prefix)) {
			if _, ok := seen[ref]; !ok {
				seen[ref] = struct{}{}
				candidates = append(candidates, ref)
			}
		}
		return nil
	})
	sort.Strings(candidates)
	return candidates
}

func entryRealPath(entry *FileEntry) string {
	if entry == nil {
		return ""
	}
	return filepath.Join(notesDir(), filepath.FromSlash(entry.RelPath))
}

func managedFilesRoot() string {
	if fileDraftRoot != "" {
		return fileDraftRoot
	}
	return notesDir()
}

func managedNoteFolderPath(rel string) string {
	if rel == "" {
		return managedFilesRoot()
	}
	return filepath.Join(managedFilesRoot(), rel)
}

func managedNotePath(rel string) string {
	if rel == "" {
		return managedFilesRoot()
	}
	return filepath.Join(managedFilesRoot(), rel)
}

func managedDraftRootPath() string {
	return notesDir() + ".draft"
}

func ensureManagedFilesDraft() error {
	if fileDraftRoot != "" {
		return nil
	}
	root := managedDraftRootPath()
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := copyDirectory(notesDir(), root); err != nil {
		return err
	}
	fileDraftRoot = root
	return nil
}

func discardManagedFilesDraft() error {
	if fileDraftRoot == "" {
		fileDraftRoot = ""
		return os.RemoveAll(managedDraftRootPath())
	}
	root := fileDraftRoot
	fileDraftRoot = ""
	return os.RemoveAll(root)
}

func commitManagedFilesDraft() error {
	if fileDraftRoot == "" {
		return nil
	}
	if err := removeManagedFiles(notesDir()); err != nil {
		return err
	}
	if err := copyManagedFiles(fileDraftRoot, notesDir()); err != nil {
		return err
	}
	return discardManagedFilesDraft()
}

func removeManagedFiles(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			if isManagedAssetsDirName(filepath.Base(path)) {
				if err := os.RemoveAll(path); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			return nil
		}
		if !isManagedNotePath(path) {
			return os.Remove(path)
		}
		return nil
	})
}

func copyManagedFiles(source string, dest string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if isManagedNotePath(path) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func migratedAssetTarget(rel string, scopes []noteFile) (string, error) {
	rel = filepath.ToSlash(rel)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		dir = ""
	}
	best := noteFile{}
	bestDirLen := -1
	for _, scope := range scopes {
		scopeDir := filepath.ToSlash(scope.RelDir)
		if scopeDir == "." {
			scopeDir = ""
		}
		if scopeDir == "" && bestDirLen >= 0 {
			continue
		}
		if dir == scopeDir || strings.HasPrefix(dir, scopeDir+"/") || (scopeDir == "" && dir == "") {
			if len(scopeDir) > bestDirLen {
				best = scope
				bestDirLen = len(scopeDir)
			}
		}
	}
	base := filepath.Base(rel)
	if best.RelPath == "" {
		if len(scopes) == 0 {
			return "", fmt.Errorf("no note scope available")
		}
		best = scopes[0]
		bestDirLen = len(filepath.ToSlash(best.RelDir))
	}
	scopeDir := filepath.ToSlash(best.RelDir)
	remainder := strings.TrimPrefix(dir, scopeDir)
	remainder = strings.TrimPrefix(remainder, "/")
	if remainder == "" {
		return filepath.Join(noteAssetsRel(best.RelPath), base), nil
	}
	return filepath.Join(noteAssetsRel(best.RelPath), remainder, base), nil
}
