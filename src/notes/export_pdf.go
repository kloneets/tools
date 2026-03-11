package notes

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/diamondburned/gotk4/pkg/pangocairo"
	"github.com/kloneets/tools/src/settings"
)

const (
	pdfPageWidthPt   = 595.0
	pdfPageHeightPt  = 842.0
	pdfMarginLeftPt  = 48.0
	pdfMarginTopPt   = 48.0
	pdfMarginRightPt = 48.0
	pdfMarginBotPt   = 48.0

	pdfDefaultTextColor  = "#000000"
	pdfDefaultLinkColor  = "#0b63c8"
	pdfDefaultQuoteColor = "#4d4d4d"
	pdfCodeBackground    = "#ffffff"
)

type pdfTextStyle struct {
	family     string
	size       float64
	weight     string
	style      string
	foreground string
	background string
	underline  string
}

type pdfLineStyle struct {
	text       pdfTextStyle
	background string
	indentPt   float64
	topGapPt   float64
}

type pdfStyledLine struct {
	markup string
	style  pdfLineStyle
}

type pdfStyledImage struct {
	path string
}

type pdfRenderBlock struct {
	line  *pdfStyledLine
	image *pdfStyledImage
}

type pdfMarkupSegment struct {
	text  string
	style pdfTextStyle
}

func exportNotePDF(outputPath string, notePath string, noteTitle string, markdown string, appearance notesAppearance, tabSpaces int) error {
	outputPath = ensurePDFExtension(outputPath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	surface, err := cairo.CreatePDFSurface(outputPath, pdfPageWidthPt, pdfPageHeightPt)
	if err != nil {
		return err
	}
	defer surface.Close()

	cr := cairo.Create(surface)
	defer cr.Close()

	blocks := pdfRenderBlocks(notePath, noteTitle, markdown, appearance, tabSpaces)
	pageBottom := pdfPageHeightPt - pdfMarginBotPt
	y := pdfMarginTopPt

	for idx, block := range blocks {
		switch {
		case block.line != nil:
			nextY, err := renderPDFTextBlock(cr, *block.line, appearance, y, pageBottom, idx > 0)
			if err != nil {
				return err
			}
			y = nextY
		case block.image != nil:
			nextY, err := renderPDFImageBlock(cr, *block.image, y, pageBottom, idx > 0)
			if err != nil {
				return err
			}
			y = nextY
		}
	}

	return surface.Status().ToError()
}

func pdfRenderBlocks(notePath string, noteTitle string, markdown string, appearance notesAppearance, tabSpaces int) []pdfRenderBlock {
	render := markdownPreview(markdown, tabSpaces)
	if strings.TrimSpace(render.Text) == "" && len(render.Images) == 0 {
		return []pdfRenderBlock{{
			line: &pdfStyledLine{
				markup: styledMarkupSegments([]pdfMarkupSegment{{
					text:  noteTitle,
					style: defaultPDFTextStyle(appearance),
				}}),
				style: pdfLineStyle{text: defaultPDFTextStyle(appearance)},
			},
		}}
	}

	imageByOffset := make(map[int]markdownImage, len(render.Images))
	for _, image := range render.Images {
		imageByOffset[image.Offset] = image
	}

	blocks := make([]pdfRenderBlock, 0, len(strings.Split(render.Text, "\n"))+len(render.Images))
	lines := strings.Split(render.Text, "\n")
	offset := 0
	for _, line := range lines {
		lineStart := offset
		lineEnd := lineStart + len([]rune(line))
		blocks = append(blocks, pdfBlocksForLine(notePath, line, render.Spans, imageByOffset, lineStart, lineEnd, appearance)...)
		offset = lineEnd + 1
	}
	return blocks
}

func pdfBlocksForLine(notePath string, line string, spans []markdownSpan, imageByOffset map[int]markdownImage, lineStart int, lineEnd int, appearance notesAppearance) []pdfRenderBlock {
	lineRunes := []rune(line)
	blocks := make([]pdfRenderBlock, 0, 3)
	segmentStart := lineStart
	for idx, r := range lineRunes {
		if r != []rune(markdownImagePlaceholder)[0] {
			continue
		}
		offset := lineStart + idx
		image, ok := imageByOffset[offset]
		if !ok {
			continue
		}
		if segmentStart < offset {
			text := string(lineRunes[segmentStart-lineStart : offset-lineStart])
			styled := buildPDFStyledLine(text, spans, segmentStart, offset, appearance)
			blocks = append(blocks, pdfRenderBlock{line: &styled})
		}
		resolved, err := resolveNoteImagePath(notePath, image.Path)
		if err == nil {
			blocks = append(blocks, pdfRenderBlock{image: &pdfStyledImage{path: resolved}})
		}
		segmentStart = offset + 1
	}
	if segmentStart < lineEnd || len(blocks) == 0 {
		text := string(lineRunes[maxInt(segmentStart-lineStart, 0):])
		styled := buildPDFStyledLine(text, spans, segmentStart, lineEnd, appearance)
		blocks = append(blocks, pdfRenderBlock{line: &styled})
	}
	return blocks
}

func renderPDFTextBlock(cr *cairo.Context, line pdfStyledLine, appearance notesAppearance, y float64, pageBottom float64, allowPageBreak bool) (float64, error) {
	y += line.style.topGapPt
	lineWidthPt := pdfPageWidthPt - pdfMarginLeftPt - pdfMarginRightPt - line.style.indentPt
	if lineWidthPt <= 0 {
		lineWidthPt = pdfPageWidthPt - pdfMarginLeftPt - pdfMarginRightPt
	}

	layout := pangocairo.CreateLayout(cr)
	layout.SetWidth(pango.UnitsFromDouble(lineWidthPt))
	layout.SetWrap(pango.WrapWordChar)
	layout.SetLineSpacing(pdfLineSpacingFactor(appearance.lineSpacing))
	layout.SetMarkup(pdfLineMarkup(line.markup), -1)

	_, logicalRect := layout.Extents()
	layoutOffsetY, layoutHeight := pdfLayoutMetrics(logicalRect, line.style.text.size, appearance.lineSpacing)

	if allowPageBreak && y+layoutHeight > pageBottom {
		cr.ShowPage()
		y = pdfMarginTopPt + line.style.topGapPt
	}

	if line.style.background != "" {
		r, g, b := parseHexColor(line.style.background)
		cr.SetSourceRGBA(r, g, b, 1)
		cr.Rectangle(
			pdfMarginLeftPt+line.style.indentPt,
			y,
			pdfPageWidthPt-pdfMarginLeftPt-pdfMarginRightPt-line.style.indentPt,
			layoutHeight,
		)
		cr.Fill()
	}

	cr.MoveTo(pdfMarginLeftPt+line.style.indentPt, y+layoutOffsetY)
	pangocairo.ShowLayout(cr, layout)
	return y + layoutHeight, nil
}

func renderPDFImageBlock(cr *cairo.Context, image pdfStyledImage, y float64, pageBottom float64, allowPageBreak bool) (float64, error) {
	pixbuf, err := gdkpixbuf.NewPixbufFromFile(image.path)
	if err != nil {
		return y, nil
	}

	maxWidth := pdfPageWidthPt - pdfMarginLeftPt - pdfMarginRightPt
	maxHeight := 220.0
	width := float64(pixbuf.Width())
	height := float64(pixbuf.Height())
	if width <= 0 || height <= 0 {
		return y, nil
	}
	scale := minFloat(maxWidth/width, maxHeight/height)
	if scale > 1 {
		scale = 1
	}
	if scale <= 0 {
		scale = 1
	}
	drawWidth := width * scale
	drawHeight := height * scale
	topGap := 6.0
	if allowPageBreak && y+topGap+drawHeight > pageBottom {
		cr.ShowPage()
		y = pdfMarginTopPt
	}
	y += topGap
	if scale != 1 {
		scaled, scaleErr := gdkpixbuf.NewPixbufFromFileAtScale(image.path, int(drawWidth+0.5), int(drawHeight+0.5), true)
		if scaleErr == nil && scaled != nil {
			pixbuf = scaled
			drawWidth = float64(pixbuf.Width())
			drawHeight = float64(pixbuf.Height())
		}
	}
	gdk.CairoSetSourcePixbuf(cr, pixbuf, pdfMarginLeftPt, y)
	cr.Rectangle(pdfMarginLeftPt, y, drawWidth, drawHeight)
	cr.Fill()
	return y + drawHeight, nil
}

func buildPDFStyledLine(line string, spans []markdownSpan, lineStart int, lineEnd int, appearance notesAppearance) pdfStyledLine {
	lineTags := spansAtPosition(spans, lineStart, maxInt(lineEnd, lineStart), markdownTagOrder())
	lineStyle := pdfStyleForLine(lineTags, appearance)
	lineRunes := []rune(line)

	if len(lineRunes) == 0 {
		return pdfStyledLine{
			markup: styledMarkupSegments([]pdfMarkupSegment{{
				text:  " ",
				style: lineStyle.text,
			}}),
			style: lineStyle,
		}
	}

	boundaries := lineBoundaries(spans, lineStart, lineEnd)
	segments := make([]pdfMarkupSegment, 0, len(boundaries))
	for idx := 0; idx < len(boundaries)-1; idx++ {
		segmentStart := boundaries[idx]
		segmentEnd := boundaries[idx+1]
		if segmentStart >= segmentEnd {
			continue
		}
		activeTags := spansAtPosition(spans, segmentStart, segmentEnd, markdownTagOrder())
		segmentStyle := pdfStyleForSegment(activeTags, appearance, lineStyle.text)
		segments = append(segments, pdfMarkupSegment{
			text:  string(lineRunes[segmentStart-lineStart : segmentEnd-lineStart]),
			style: segmentStyle,
		})
	}

	if len(segments) == 0 {
		segments = append(segments, pdfMarkupSegment{text: line, style: lineStyle.text})
	}

	return pdfStyledLine{
		markup: styledMarkupSegments(segments),
		style:  lineStyle,
	}
}

func pdfStyleForLine(tags []string, appearance notesAppearance) pdfLineStyle {
	style := pdfLineStyle{
		text: defaultPDFTextStyle(appearance),
	}

	for _, tag := range tags {
		switch tag {
		case tagHeading1:
			style.text.weight = "bold"
			style.text.size = pdfBodyFontSize(appearance) * 1.35
			style.topGapPt = maxFloat(style.topGapPt, 8)
		case tagHeading2:
			style.text.weight = "bold"
			style.text.size = pdfBodyFontSize(appearance) * 1.2
			style.topGapPt = maxFloat(style.topGapPt, 6)
		case tagHeading3:
			style.text.weight = "600"
			style.text.size = pdfBodyFontSize(appearance) * 1.1
			style.topGapPt = maxFloat(style.topGapPt, 4)
		case tagList, tagOrdered, tagChecklist:
			style.indentPt = maxFloat(style.indentPt, 12)
		case tagQuote:
			style.indentPt = maxFloat(style.indentPt, 16)
			style.text.style = "italic"
			style.text.foreground = pdfDefaultQuoteColor
		case tagCodeBlock:
			style.text.family = pdfMonoFamily(appearance.monoFamily)
			style.text.size = pdfCodeFontSize(appearance)
			style.text.foreground = pdfDefaultTextColor
			style.background = pdfCodeBackground
			style.topGapPt = maxFloat(style.topGapPt, 6)
		}
	}

	return style
}

func pdfStyleForSegment(tags []string, appearance notesAppearance, base pdfTextStyle) pdfTextStyle {
	style := base

	for _, tag := range tags {
		switch tag {
		case tagBold:
			style.weight = "bold"
		case tagItalic:
			style.style = "italic"
		case tagCode:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = pdfDefaultTextColor
			style.background = pdfCodeBackground
		case tagCodeBlock:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = pdfDefaultTextColor
			style.background = pdfCodeBackground
		case tagCodeKeyword:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeKeyword
			style.weight = "bold"
		case tagCodeString:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeString
		case tagCodeComment:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeComment
			style.style = "italic"
		case tagCodeNumber:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeNumber
		case tagCodeType:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeType
		case tagCodeFunction:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeFunction
		case tagCodeProperty:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeProperty
		case tagCodeConstant:
			style.family = pdfMonoFamily(appearance.monoFamily)
			style.size = pdfCodeFontSize(appearance)
			style.foreground = appearance.palette.codeConstant
		case tagLink:
			style.foreground = pdfDefaultLinkColor
			style.underline = "single"
		}
	}

	return style
}

func lineBoundaries(spans []markdownSpan, lineStart int, lineEnd int) []int {
	boundaries := []int{lineStart, lineEnd}
	for _, span := range spans {
		if span.End <= lineStart || span.Start >= lineEnd {
			continue
		}
		boundaries = append(boundaries, maxInt(span.Start, lineStart), minInt(span.End, lineEnd))
	}
	slices.Sort(boundaries)
	return slices.Compact(boundaries)
}

func spansAtPosition(spans []markdownSpan, start int, end int, order []string) []string {
	active := make([]string, 0, 4)
	for _, tag := range order {
		for _, span := range spans {
			if span.Tag != tag {
				continue
			}
			if span.Start <= start && span.End >= end {
				active = append(active, tag)
				break
			}
		}
	}
	return active
}

func styledMarkupSegments(segments []pdfMarkupSegment) string {
	var markup strings.Builder
	for _, segment := range segments {
		if segment.text == "" {
			continue
		}
		markup.WriteString(`<span`)
		appendMarkupAttr(&markup, "font_family", segment.style.family)
		appendMarkupAttr(&markup, "size", fmt.Sprintf("%dpt", maxInt(int(segment.style.size+0.5), 1)))
		appendMarkupAttr(&markup, "weight", segment.style.weight)
		appendMarkupAttr(&markup, "style", segment.style.style)
		appendMarkupAttr(&markup, "foreground", segment.style.foreground)
		appendMarkupAttr(&markup, "background", segment.style.background)
		appendMarkupAttr(&markup, "underline", segment.style.underline)
		markup.WriteString(`>`)
		markup.WriteString(html.EscapeString(segment.text))
		markup.WriteString(`</span>`)
	}
	return markup.String()
}

func pdfLineMarkup(markup string) string {
	markup = strings.ReplaceAll(markup, markdownImagePlaceholder, "")
	if strings.TrimSpace(markup) == "" {
		return `<span> </span>`
	}
	return markup
}

func appendMarkupAttr(builder *strings.Builder, key string, value string) {
	if value == "" {
		return
	}
	builder.WriteString(` `)
	builder.WriteString(key)
	builder.WriteString(`="`)
	builder.WriteString(html.EscapeString(value))
	builder.WriteString(`"`)
}

func defaultPDFTextStyle(appearance notesAppearance) pdfTextStyle {
	return pdfTextStyle{
		family:     pdfSansFamily(appearance.previewFamily),
		size:       pdfBodyFontSize(appearance),
		foreground: pdfDefaultTextColor,
	}
}

func pdfLineSpacingFactor(spacing float64) float32 {
	switch {
	case spacing < 0:
		return 1
	case spacing == 0:
		return 0.01
	default:
		return float32(spacing)
	}
}

func lineHeightScale(spacing float64) float64 {
	if spacing <= 0 {
		return 1
	}
	return spacing
}

func pdfLayoutMetrics(logicalRect *pango.Rectangle, fallbackSize float64, spacing float64) (offsetY float64, height float64) {
	if logicalRect == nil {
		return pdfLayoutMetricsFromValues(0, 0, fallbackSize, spacing)
	}
	return pdfLayoutMetricsFromValues(logicalRect.Y(), logicalRect.Height(), fallbackSize, spacing)
}

func pdfLayoutMetricsFromValues(logicalY int, logicalHeight int, fallbackSize float64, spacing float64) (offsetY float64, height float64) {
	height = pango.UnitsToDouble(logicalHeight)
	if height <= 0 {
		height = float64(maxInt(int(fallbackSize*lineHeightScale(spacing)), 12))
	}

	offsetY = -pango.UnitsToDouble(logicalY)
	return offsetY, height
}

func parseHexColor(color string) (float64, float64, float64) {
	color = strings.TrimPrefix(strings.TrimSpace(color), "#")
	if len(color) != 6 {
		return 1, 1, 1
	}
	var r, g, b uint8
	_, _ = fmt.Sscanf(color, "%02x%02x%02x", &r, &g, &b)
	return float64(r) / 255, float64(g) / 255, float64(b) / 255
}

func defaultPDFExportName(notePath string) string {
	title := noteTitleFromPath(notePath)
	if title == "" {
		title = "Note"
	}
	return title + ".pdf"
}

func ensurePDFExtension(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		return path
	}
	return path + ".pdf"
}

func maxFloat(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left float64, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func pdfBodyFontSize(appearance notesAppearance) float64 {
	if appearance.pdfBodySize > 0 {
		return appearance.pdfBodySize
	}
	return settings.DefaultNotesPDFBodyFontSize
}

func pdfCodeFontSize(appearance notesAppearance) float64 {
	if appearance.pdfCodeSize > 0 {
		return appearance.pdfCodeSize
	}
	return settings.DefaultNotesPDFCodeFontSize
}

func pdfSansFamily(preferred string) string {
	if strings.TrimSpace(preferred) == "" {
		return "Sans"
	}
	return preferred + ", Sans"
}

func pdfMonoFamily(preferred string) string {
	if strings.TrimSpace(preferred) == "" {
		return "Monospace"
	}
	return preferred + ", Monospace"
}
