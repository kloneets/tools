package notes

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	tagHeading1     = "md-heading-1"
	tagHeading2     = "md-heading-2"
	tagHeading3     = "md-heading-3"
	tagList         = "md-list"
	tagOrdered      = "md-ordered"
	tagChecklist    = "md-checklist"
	tagQuote        = "md-quote"
	tagBold         = "md-bold"
	tagItalic       = "md-italic"
	tagCode         = "md-code"
	tagCodeBlock    = "md-code-block"
	tagCodeKeyword  = "md-code-keyword"
	tagCodeString   = "md-code-string"
	tagCodeComment  = "md-code-comment"
	tagCodeNumber   = "md-code-number"
	tagCodeType     = "md-code-type"
	tagCodeFunction = "md-code-function"
	tagCodeProperty = "md-code-property"
	tagCodeConstant = "md-code-constant"
	tagLink         = "md-link"
)

type markdownSpan struct {
	Tag   string
	Start int
	End   int
}

type markdownRender struct {
	Text  string
	Spans []markdownSpan
	Links []markdownLink
}

type markdownLink struct {
	Start int
	End   int
	URL   string
}

func markdownSpans(text string) []markdownSpan {
	return markdownRenderFromText(text).Spans
}

func markdownPreview(text string, tabSpaces int) markdownRender {
	if tabSpaces <= 0 {
		tabSpaces = 4
	}
	lines := strings.Split(text, "\n")
	rendered := make([]string, 0, len(lines))
	spans := make([]markdownSpan, 0)
	links := make([]markdownLink, 0)
	offset := 0
	inCodeBlock := false
	codeLanguage := ""
	codeBlockLines := make([]string, 0)

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				blockText, blockSpans, blockLen := renderCodeBlock(codeBlockLines, offset, codeLanguage, tabSpaces)
				rendered = append(rendered, blockText...)
				spans = append(spans, blockSpans...)
				offset += blockLen
				if len(blockText) > 0 {
					offset++
				}
				codeBlockLines = codeBlockLines[:0]
				inCodeBlock = false
				codeLanguage = ""
				continue
			}
			inCodeBlock = true
			codeLanguage = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			codeBlockLines = codeBlockLines[:0]
			continue
		}
		if inCodeBlock {
			codeBlockLines = append(codeBlockLines, line)
			continue
		}

		renderedLine, lineSpans, lineLinks, renderedLen, emitLine := renderMarkdownLine(line, offset)
		if !emitLine {
			continue
		}
		rendered = append(rendered, renderedLine)
		spans = append(spans, lineSpans...)
		links = append(links, lineLinks...)
		offset += renderedLen + 1
	}
	if inCodeBlock {
		blockText, blockSpans, _ := renderCodeBlock(codeBlockLines, offset, codeLanguage, tabSpaces)
		rendered = append(rendered, blockText...)
		spans = append(spans, blockSpans...)
	}

	return markdownRender{
		Text:  strings.Join(rendered, "\n"),
		Spans: spans,
		Links: links,
	}
}

func markdownRenderFromText(text string) markdownRender {
	lines := strings.SplitAfter(text, "\n")
	spans := make([]markdownSpan, 0)
	offset := 0
	inCodeBlock := false

	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\n")
		lineEnd := offset + runeLen(line)
		trimmed := strings.TrimLeft(line, " \t")
		indent := runeLen(line[:len(line)-len(trimmed)])

		if strings.HasPrefix(trimmed, "```") {
			spans = append(spans, markdownSpan{Tag: tagCodeBlock, Start: offset, End: lineEnd})
			inCodeBlock = !inCodeBlock
			offset += len(rawLine)
			continue
		}
		if inCodeBlock {
			spans = append(spans, markdownSpan{Tag: tagCodeBlock, Start: offset, End: lineEnd})
			offset += len(rawLine)
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "# "):
			spans = append(spans, markdownSpan{Tag: tagHeading1, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "## "):
			spans = append(spans, markdownSpan{Tag: tagHeading2, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "### "):
			spans = append(spans, markdownSpan{Tag: tagHeading3, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "- [ ] "), strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
			spans = append(spans, markdownSpan{Tag: tagChecklist, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			spans = append(spans, markdownSpan{Tag: tagList, Start: offset + indent, End: lineEnd})
		case orderedListPrefixLength(trimmed) > 0:
			spans = append(spans, markdownSpan{Tag: tagOrdered, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "> "):
			spans = append(spans, markdownSpan{Tag: tagQuote, Start: offset + indent, End: lineEnd})
		}

		spans = append(spans, inlineMarkdownSpans(line, offset)...)
		offset += runeLen(rawLine)
	}

	return markdownRender{Text: text, Spans: spans}
}

func renderMarkdownLine(line string, offset int) (string, []markdownSpan, []markdownLink, int, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	prefixText := line[:len(line)-len(trimmed)]
	lineTag := ""
	switch {
	case strings.HasPrefix(trimmed, "# "):
		line = prefixText + trimmed[2:]
		lineTag = tagHeading1
	case strings.HasPrefix(trimmed, "## "):
		line = prefixText + trimmed[3:]
		lineTag = tagHeading2
	case strings.HasPrefix(trimmed, "### "):
		line = prefixText + trimmed[4:]
		lineTag = tagHeading3
	case strings.HasPrefix(trimmed, "- [ ] "):
		line = prefixText + "☐ " + trimmed[6:]
		lineTag = tagChecklist
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		line = prefixText + "☑ " + trimmed[6:]
		lineTag = tagChecklist
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		line = prefixText + "• " + trimmed[2:]
		lineTag = tagList
	case orderedListPrefixLength(trimmed) > 0:
		lineTag = tagOrdered
	case strings.HasPrefix(trimmed, "> "):
		line = prefixText + trimmed[2:]
		lineTag = tagQuote
	}

	plain, spans, links, plainLen := renderInlineMarkdown(line, offset)
	if lineTag != "" {
		spans = append([]markdownSpan{{Tag: lineTag, Start: offset, End: offset + plainLen}}, spans...)
	}
	return plain, spans, links, plainLen, true
}

func expandTabs(text string, tabSpaces int) string {
	return strings.ReplaceAll(text, "\t", strings.Repeat(" ", tabSpaces))
}

func codeSpans(line string, offset int, language string) []markdownSpan {
	return legacyCodeSpans(line, offset, language)
}

func legacyCodeBlockSpans(text string, offset int, language string) []markdownSpan {
	lines := strings.Split(text, "\n")
	spans := make([]markdownSpan, 0, len(lines)*2)
	lineOffset := offset
	for idx, line := range lines {
		spans = append(spans, legacyCodeSpans(line, lineOffset, language)...)
		lineOffset += runeLen(line)
		if idx < len(lines)-1 {
			lineOffset++
		}
	}
	return spans
}

func legacyCodeSpans(line string, offset int, language string) []markdownSpan {
	lang := strings.ToLower(language)
	switch lang {
	case "go", "golang":
		return lexCodeLine(line, offset, "//", nil, []string{
			"func", "package", "import", "return", "if", "else", "for", "range", "switch", "case", "default",
			"struct", "interface", "type", "var", "const", "go", "defer", "map", "chan", "select", "fallthrough",
		}, []string{"string", "int", "bool", "error", "byte", "rune", "float64", "float32"})
	case "javascript", "js", "typescript", "ts":
		return lexCodeLine(line, offset, "//", nil, []string{
			"function", "return", "if", "else", "for", "while", "const", "let", "var", "class", "import", "export",
			"from", "async", "await", "new", "switch", "case", "default", "try", "catch",
		}, []string{"string", "number", "boolean", "Promise"})
	case "python", "py":
		return lexCodeLine(line, offset, "#", nil, []string{
			"def", "return", "if", "elif", "else", "for", "while", "import", "from", "class", "try", "except",
			"with", "as", "lambda", "yield", "pass", "match", "case",
		}, []string{"str", "int", "float", "bool", "list", "dict", "tuple"})
	case "json":
		return lexCodeLine(line, offset, "", nil, nil, nil)
	case "sh", "bash", "zsh", "shell":
		return lexCodeLine(line, offset, "#", nil, []string{
			"if", "then", "else", "fi", "for", "do", "done", "case", "esac", "function", "local", "export",
		}, nil)
	case "html":
		return lexMarkupLine(line, offset)
	case "css":
		return lexCodeLine(line, offset, "/*", []string{"*/"}, []string{
			"display", "position", "color", "background", "font-size", "margin", "padding", "grid", "flex",
		}, nil)
	default:
		return lexCodeLine(line, offset, "//", nil, []string{
			"if", "else", "for", "while", "return", "class", "func", "function", "const", "let", "var", "def",
		}, nil)
	}
}

func renderCodeBlock(lines []string, offset int, language string, tabSpaces int) ([]string, []markdownSpan, int) {
	if len(lines) == 0 {
		return nil, nil, 0
	}
	renderedLines := make([]string, len(lines))
	for i, line := range lines {
		renderedLines[i] = expandTabs(line, tabSpaces)
	}
	blockText := strings.Join(renderedLines, "\n")
	blockLen := runeLen(blockText)
	spans := []markdownSpan{{Tag: tagCodeBlock, Start: offset, End: offset + blockLen}}
	spans = append(spans, treeSitterSpans(blockText, offset, language)...)
	return renderedLines, spans, blockLen
}

func lexCodeLine(line string, offset int, lineComment string, blockCommentEnds []string, keywords []string, types []string) []markdownSpan {
	spans := make([]markdownSpan, 0)
	i := 0
	for i < len(line) {
		if lineComment != "" && strings.HasPrefix(line[i:], lineComment) {
			spans = append(spans, codeSpan(line, offset, i, len(line), tagCodeComment))
			return spans
		}
		if len(blockCommentEnds) > 0 && strings.HasPrefix(line[i:], lineComment) {
			end := len(line)
			for _, marker := range blockCommentEnds {
				if idx := strings.Index(line[i+len(lineComment):], marker); idx >= 0 {
					end = i + len(lineComment) + idx + len(marker)
					break
				}
			}
			spans = append(spans, codeSpan(line, offset, i, end, tagCodeComment))
			i = end
			continue
		}
		switch line[i] {
		case '"', '\'':
			end := scanQuoted(line, i)
			spans = append(spans, codeSpan(line, offset, i, end, tagCodeString))
			i = end
			continue
		}
		if isDigit(line[i]) {
			end := scanNumber(line, i)
			spans = append(spans, codeSpan(line, offset, i, end, tagCodeNumber))
			i = end
			continue
		}
		if isWordStart(line[i]) {
			end := scanWord(line, i)
			word := line[i:end]
			switch {
			case containsWord(keywords, word):
				spans = append(spans, codeSpan(line, offset, i, end, tagCodeKeyword))
			case containsWord(types, word):
				spans = append(spans, codeSpan(line, offset, i, end, tagCodeType))
			case previousNonSpaceByte(line, i) == '.':
				spans = append(spans, codeSpan(line, offset, i, end, tagCodeProperty))
			case nextNonSpaceByte(line, end) == '(':
				spans = append(spans, codeSpan(line, offset, i, end, tagCodeFunction))
			}
			i = end
			continue
		}
		i++
	}
	return spans
}

func lexMarkupLine(line string, offset int) []markdownSpan {
	spans := make([]markdownSpan, 0)
	for i := 0; i < len(line); i++ {
		if line[i] == '<' {
			end := strings.IndexByte(line[i:], '>')
			if end < 0 {
				break
			}
			end += i + 1
			spans = append(spans, codeSpan(line, offset, i, end, tagCodeKeyword))
			i = end - 1
			continue
		}
		if line[i] == '"' || line[i] == '\'' {
			end := scanQuoted(line, i)
			spans = append(spans, codeSpan(line, offset, i, end, tagCodeString))
			i = end - 1
		}
	}
	return spans
}

func codeSpan(line string, offset int, startByte int, endByte int, tag string) markdownSpan {
	return markdownSpan{
		Tag:   tag,
		Start: offset + runeLen(line[:startByte]),
		End:   offset + runeLen(line[:endByte]),
	}
}

func scanQuoted(line string, start int) int {
	quote := line[start]
	i := start + 1
	for i < len(line) {
		if line[i] == '\\' {
			i += 2
			continue
		}
		if line[i] == quote {
			return i + 1
		}
		i++
	}
	return len(line)
}

func scanNumber(line string, start int) int {
	i := start
	for i < len(line) && (isDigit(line[i]) || line[i] == '.') {
		i++
	}
	return i
}

func scanWord(line string, start int) int {
	i := start
	for i < len(line) && isWordPart(line[i]) {
		i++
	}
	return i
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isWordStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
}

func isWordPart(b byte) bool {
	return isWordStart(b) || isDigit(b)
}

func containsWord(words []string, candidate string) bool {
	for _, word := range words {
		if word == candidate {
			return true
		}
	}
	return false
}

func previousNonSpaceByte(line string, index int) byte {
	for i := index - 1; i >= 0; i-- {
		if line[i] == ' ' || line[i] == '\t' {
			continue
		}
		return line[i]
	}
	return 0
}

func nextNonSpaceByte(line string, index int) byte {
	for i := index; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			continue
		}
		return line[i]
	}
	return 0
}

func inlineMarkdownSpans(line string, offset int) []markdownSpan {
	_, spans, _, _ := renderInlineMarkdown(line, offset)
	return spans
}

func renderInlineMarkdown(line string, offset int) (string, []markdownSpan, []markdownLink, int) {
	var out strings.Builder
	out.Grow(len(line))
	spans := make([]markdownSpan, 0)
	links := make([]markdownLink, 0)
	outChars := 0

	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "**") || strings.HasPrefix(line[i:], "__") {
			delim := line[i : i+2]
			if closeIdx := strings.Index(line[i+2:], delim); closeIdx >= 0 {
				start := outChars
				content := line[i+2 : i+2+closeIdx]
				out.WriteString(content)
				outChars += runeLen(content)
				end := outChars
				spans = append(spans, markdownSpan{Tag: tagBold, Start: offset + start, End: offset + end})
				i += 2 + closeIdx + 2
				continue
			}
		}
		if line[i] == '`' {
			if closeIdx := strings.IndexByte(line[i+1:], '`'); closeIdx >= 0 {
				start := outChars
				content := line[i+1 : i+1+closeIdx]
				out.WriteString(content)
				outChars += runeLen(content)
				end := outChars
				spans = append(spans, markdownSpan{Tag: tagCode, Start: offset + start, End: offset + end})
				i += closeIdx + 2
				continue
			}
		}
		if line[i] == '[' {
			if endLabel := strings.IndexByte(line[i:], ']'); endLabel >= 0 {
				labelEnd := i + endLabel
				if labelEnd+1 < len(line) && line[labelEnd+1] == '(' {
					if endURL := strings.IndexByte(line[labelEnd+2:], ')'); endURL >= 0 {
						start := outChars
						label := line[i+1 : labelEnd]
						url := line[labelEnd+2 : labelEnd+2+endURL]
						out.WriteString(label)
						outChars += runeLen(label)
						end := outChars
						spans = append(spans, markdownSpan{Tag: tagLink, Start: offset + start, End: offset + end})
						links = append(links, markdownLink{Start: offset + start, End: offset + end, URL: url})
						i = labelEnd + 2 + endURL + 2
						continue
					}
				}
			}
		}
		if (line[i] == '*' || line[i] == '_') && !isDoubleDelimiter(line, i, line[i]) {
			delim := line[i]
			if closeIdx := singleDelimiterClose(line, i+1, delim); closeIdx >= 0 {
				start := outChars
				content := line[i+1 : closeIdx]
				out.WriteString(content)
				outChars += runeLen(content)
				end := outChars
				spans = append(spans, markdownSpan{Tag: tagItalic, Start: offset + start, End: offset + end})
				i = closeIdx + 1
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(line[i:])
		out.WriteRune(r)
		outChars++
		i += size
	}

	return out.String(), spans, links, outChars
}

func runeLen(text string) int {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return utf8.RuneCountInString(text)
		}
	}
	return len(text)
}

func singleDelimiterClose(line string, start int, delim byte) int {
	for i := start; i < len(line); i++ {
		if line[i] == delim && !isDoubleDelimiter(line, i, delim) {
			return i
		}
	}
	return -1
}

func isDoubleDelimiter(line string, index int, delim byte) bool {
	return (index > 0 && line[index-1] == delim) || (index+1 < len(line) && line[index+1] == delim)
}

func orderedListPrefixLength(line string) int {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) || line[i] != '.' || line[i+1] != ' ' {
		return 0
	}
	return i + 2
}

func applyWrap(text string, start, end int, prefix, suffix, placeholder string) (string, int, int) {
	if start != end {
		replacement := prefix + text[start:end] + suffix
		return text[:start] + replacement + text[end:], start + len(prefix), start + len(prefix) + (end - start)
	}

	replacement := prefix + placeholder + suffix
	return text[:start] + replacement + text[start:], start + len(prefix), start + len(prefix) + len(placeholder)
}

func applyLink(text string, start, end int) (string, int, int) {
	label := "link"
	if start != end {
		label = text[start:end]
	}
	replacement := fmt.Sprintf("[%s](https://example.com)", label)
	if start != end {
		return text[:start] + replacement + text[end:], start + 1, start + 1 + len(label)
	}
	return text[:start] + replacement + text[start:], start + 1, start + 1 + len(label)
}

func applyLinePrefix(text string, start, end int, prefix, placeholder string) (string, int, int) {
	if start == end {
		lineStart := lastLineStart(text, start)
		lineEnd := nextLineEnd(text, start)
		current := text[lineStart:lineEnd]
		insert := prefix
		selectStart := lineStart + len(prefix)
		selectEnd := lineEnd + len(prefix)
		if current == "" {
			insert += placeholder
			selectEnd = selectStart + len(placeholder)
		}
		return text[:lineStart] + insert + text[lineStart:], selectStart, selectEnd
	}

	blockStart := lastLineStart(text, start)
	blockEnd := nextLineEnd(text, end)
	block := text[blockStart:blockEnd]
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && blockEnd > 0 && text[blockEnd-1] == '\n' && line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	replacement := strings.Join(lines, "\n")
	return text[:blockStart] + replacement + text[blockEnd:], blockStart, blockStart + len(replacement)
}

func applyOrderedList(text string, start, end int) (string, int, int) {
	return transformSelectedLines(text, start, end, func(lines []string) []string {
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			lines[i] = fmt.Sprintf("%d. %s", i+1, stripListPrefix(line))
		}
		return lines
	}, "1. item")
}

func toggleChecklist(text string, start, end int) (string, int, int) {
	return transformSelectedLines(text, start, end, func(lines []string) []string {
		for i, line := range lines {
			trimmed := strings.TrimLeft(line, " \t")
			indent := line[:len(line)-len(trimmed)]
			switch {
			case strings.HasPrefix(trimmed, "- [ ] "):
				lines[i] = indent + "- [x] " + trimmed[6:]
			case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
				lines[i] = indent + "- [ ] " + trimmed[6:]
			default:
				content := stripListPrefix(trimmed)
				if strings.TrimSpace(content) == "" {
					content = "task"
				}
				lines[i] = indent + "- [ ] " + content
			}
		}
		return lines
	}, "- [ ] task")
}

func transformSelectedLines(text string, start, end int, transform func([]string) []string, placeholder string) (string, int, int) {
	if start == end {
		lineStart := lastLineStart(text, start)
		lineEnd := nextLineEnd(text, start)
		line := text[lineStart:lineEnd]
		lines := []string{line}
		transformed := transform(lines)
		replacement := transformed[0]
		if line == "" && replacement == "" {
			replacement = placeholder
		}
		return text[:lineStart] + replacement + text[lineEnd:], lineStart, lineStart + len(replacement)
	}

	blockStart := lastLineStart(text, start)
	blockEnd := nextLineEnd(text, end)
	block := text[blockStart:blockEnd]
	lines := strings.Split(block, "\n")
	lines = transform(lines)
	replacement := strings.Join(lines, "\n")
	return text[:blockStart] + replacement + text[blockEnd:], blockStart, blockStart + len(replacement)
}

func stripListPrefix(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	switch {
	case strings.HasPrefix(trimmed, "- [ ] "):
		return trimmed[6:]
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		return trimmed[6:]
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "> "):
		return trimmed[2:]
	case orderedListPrefixLength(trimmed) > 0:
		return trimmed[orderedListPrefixLength(trimmed):]
	default:
		return trimmed
	}
}

func lastLineStart(text string, pos int) int {
	if pos > len(text) {
		pos = len(text)
	}
	return strings.LastIndex(text[:pos], "\n") + 1
}

func nextLineEnd(text string, pos int) int {
	if pos > len(text) {
		pos = len(text)
	}
	idx := strings.Index(text[pos:], "\n")
	if idx < 0 {
		return len(text)
	}
	return pos + idx
}
