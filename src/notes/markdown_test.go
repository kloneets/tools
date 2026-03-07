package notes

import "testing"

func TestMarkdownSpansDetectsHeadingsAndInlineFormatting(t *testing.T) {
	text := "# Title\nSome **bold** and *italic* and `code`\n"

	spans := markdownSpans(text)

	assertHasSpan(t, spans, tagHeading1, 0, 7)
	assertHasSpan(t, spans, tagBold, 13, 17)
	assertHasSpan(t, spans, tagItalic, 22, 28)
	assertHasSpan(t, spans, tagCode, 33, 37)
}

func TestMarkdownSpansDetectsLinksListsAndChecklists(t *testing.T) {
	text := "1. first\n- [x] done\n[site](https://example.com)\n"
	spans := markdownSpans(text)

	assertHasSpan(t, spans, tagOrdered, 0, 8)
	assertHasSpan(t, spans, tagChecklist, 9, 19)
	assertHasSpan(t, spans, tagLink, 20, 24)
}

func TestMarkdownSpansDetectsCodeBlocks(t *testing.T) {
	text := "```\nfmt.Println(\"hi\")\n```\n"
	spans := markdownSpans(text)

	assertHasSpan(t, spans, tagCodeBlock, 0, 3)
	assertHasSpan(t, spans, tagCodeBlock, 4, 21)
	assertHasSpan(t, spans, tagCodeBlock, 22, 25)
}

func TestApplyWrapUsesSelection(t *testing.T) {
	got, start, end := applyWrap("hello world", 6, 11, "**", "**", "bold")

	if got != "hello **world**" {
		t.Fatalf("applyWrap() text = %q", got)
	}
	if start != 8 || end != 13 {
		t.Fatalf("applyWrap() selection = %d,%d want 8,13", start, end)
	}
}

func TestApplyWrapInsertsPlaceholderAtCursor(t *testing.T) {
	got, start, end := applyWrap("hello", 5, 5, "`", "`", "code")

	if got != "hello`code`" {
		t.Fatalf("applyWrap() text = %q", got)
	}
	if start != 6 || end != 10 {
		t.Fatalf("applyWrap() selection = %d,%d want 6,10", start, end)
	}
}

func TestApplyLinePrefixPrefixesCurrentLine(t *testing.T) {
	got, start, end := applyLinePrefix("first\nsecond", 8, 8, "> ", "quote")

	if got != "first\n> second" {
		t.Fatalf("applyLinePrefix() text = %q", got)
	}
	if start != 8 || end != 14 {
		t.Fatalf("applyLinePrefix() selection = %d,%d want 8,14", start, end)
	}
}

func TestApplyLinePrefixPrefixesSelectedLines(t *testing.T) {
	got, start, end := applyLinePrefix("one\ntwo\nthree", 1, 8, "- ", "item")

	if got != "- one\n- two\n- three" {
		t.Fatalf("applyLinePrefix() text = %q", got)
	}
	if start != 0 || end != len(got) {
		t.Fatalf("applyLinePrefix() selection = %d,%d want 0,%d", start, end, len(got))
	}
}

func TestApplyLinkWrapsSelection(t *testing.T) {
	got, start, end := applyLink("visit docs", 6, 10)

	if got != "visit [docs](https://example.com)" {
		t.Fatalf("applyLink() text = %q", got)
	}
	if start != 7 || end != 11 {
		t.Fatalf("applyLink() selection = %d,%d want 7,11", start, end)
	}
}

func TestMarkdownPreviewRendersCheckboxesAndLinks(t *testing.T) {
	render := markdownPreview("- [ ] task\n[site](https://example.com)", 4)

	if render.Text != "☐ task\nsite" {
		t.Fatalf("markdownPreview() text = %q", render.Text)
	}
	assertHasSpan(t, render.Spans, tagChecklist, 0, 6)
	assertHasSpan(t, render.Spans, tagLink, 7, 11)
	if len(render.Links) != 1 || render.Links[0].URL != "https://example.com" {
		t.Fatalf("markdownPreview() links = %#v", render.Links)
	}
}

func TestMarkdownPreviewAppliesHeadingToCurrentLine(t *testing.T) {
	render := markdownPreview("## Heading\nnext", 4)

	if render.Text != "Heading\nnext" {
		t.Fatalf("markdownPreview() text = %q", render.Text)
	}
	assertHasSpan(t, render.Spans, tagHeading2, 0, 7)
}

func TestMarkdownPreviewUsesCharacterOffsetsForUnicodeOutput(t *testing.T) {
	render := markdownPreview("- [ ] task\n## Heading", 4)

	assertHasSpan(t, render.Spans, tagChecklist, 0, 6)
	assertHasSpan(t, render.Spans, tagHeading2, 7, 14)
}

func TestMarkdownPreviewRendersCodeBlocksWithoutFences(t *testing.T) {
	render := markdownPreview("```\n\tfmt.Println(\"hi\")\n```\n", 2)

	if render.Text != "  fmt.Println(\"hi\")\n" {
		t.Fatalf("markdownPreview() code block text = %q", render.Text)
	}
	assertHasSpan(t, render.Spans, tagCodeBlock, 0, 19)
}

func TestMarkdownPreviewHighlightsGoCodeBlocks(t *testing.T) {
	render := markdownPreview("```go\nfunc main() {\n\tfmt.Println(\"hi\") // wave\n}\n```", 2)

	if render.Text != "func main() {\n  fmt.Println(\"hi\") // wave\n}" {
		t.Fatalf("markdownPreview() go text = %q", render.Text)
	}
	assertHasSpan(t, render.Spans, tagCodeKeyword, 0, 4)
	assertHasSpan(t, render.Spans, tagCodeFunction, 5, 9)
	assertHasSpan(t, render.Spans, tagCodeProperty, 20, 27)
	assertHasSpan(t, render.Spans, tagCodeString, 28, 32)
	assertHasSpan(t, render.Spans, tagCodeComment, 34, 41)
}

func TestMarkdownPreviewHighlightsPythonCodeBlocks(t *testing.T) {
	render := markdownPreview("```python\ndef hello(name):\n    return \"hi\"\n```", 4)

	assertHasSpan(t, render.Spans, tagCodeKeyword, 0, 3)
	assertHasSpan(t, render.Spans, tagCodeFunction, 4, 9)
	assertHasSpan(t, render.Spans, tagCodeKeyword, 21, 27)
	assertHasSpan(t, render.Spans, tagCodeString, 28, 32)
}

func TestTreeSitterCaptureTagMapsRichCategories(t *testing.T) {
	tests := map[string]string{
		"function":         tagCodeFunction,
		"function.method":  tagCodeFunction,
		"property":         tagCodeProperty,
		"constant":         tagCodeConstant,
		"constant.builtin": tagCodeConstant,
		"type":             tagCodeType,
		"keyword":          tagCodeKeyword,
		"string":           tagCodeString,
		"comment":          tagCodeComment,
	}

	for capture, want := range tests {
		got, ok := treeSitterCaptureTag(capture)
		if !ok {
			t.Fatalf("treeSitterCaptureTag(%q) returned !ok", capture)
		}
		if got != want {
			t.Fatalf("treeSitterCaptureTag(%q) = %q, want %q", capture, got, want)
		}
	}
}

func TestTreeSitterSpecSupportsAdditionalLanguages(t *testing.T) {
	for _, language := range []string{"php", "rust", "rs", "java", "lua", "zs"} {
		if _, ok := treeSitterSpec(language); !ok {
			t.Fatalf("treeSitterSpec(%q) = !ok, want ok", language)
		}
	}
}

func TestMarkdownFenceSpansMapsMarkdownSyntaxIntoCodePalette(t *testing.T) {
	spans := markdownFenceSpans("# Title\n[site](https://example.com)\n> quote", 0)

	assertHasSpan(t, spans, tagCodeKeyword, 0, 7)
	assertHasSpan(t, spans, tagCodeProperty, 8, 12)
	assertHasSpan(t, spans, tagCodeComment, 36, 43)
}

func TestApplyOrderedListRenumbersSelection(t *testing.T) {
	got, start, end := applyOrderedList("alpha\nbeta\ngamma", 0, len("alpha\nbeta"))

	if got != "1. alpha\n2. beta\ngamma" {
		t.Fatalf("applyOrderedList() text = %q", got)
	}
	if start != 0 || end != len("1. alpha\n2. beta") {
		t.Fatalf("applyOrderedList() selection = %d,%d", start, end)
	}
}

func TestToggleChecklistCyclesLineState(t *testing.T) {
	got, _, _ := toggleChecklist("- [ ] task", 0, 0)
	if got != "- [x] task" {
		t.Fatalf("toggleChecklist() checked = %q", got)
	}

	got, _, _ = toggleChecklist("- [x] task", 0, 0)
	if got != "- [ ] task" {
		t.Fatalf("toggleChecklist() unchecked = %q", got)
	}

	got, _, _ = toggleChecklist("plain item", 0, 0)
	if got != "- [ ] plain item" {
		t.Fatalf("toggleChecklist() new checklist = %q", got)
	}
}

func assertHasSpan(t *testing.T, spans []markdownSpan, tag string, start, end int) {
	t.Helper()

	for _, span := range spans {
		if span.Tag == tag && span.Start == start && span.End == end {
			return
		}
	}

	t.Fatalf("missing span tag=%s start=%d end=%d in %#v", tag, start, end, spans)
}
