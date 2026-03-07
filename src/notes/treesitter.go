package notes

import (
	"embed"
	"sort"
	"strings"
	"unicode/utf8"

	sitterlua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
	sitterbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	sittercss "github.com/tree-sitter/tree-sitter-css/bindings/go"
	sittergo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	sitterhtml "github.com/tree-sitter/tree-sitter-html/bindings/go"
	sitterjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
	sitterjavascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	sitterjson "github.com/tree-sitter/tree-sitter-json/bindings/go"
	sitterphp "github.com/tree-sitter/tree-sitter-php/bindings/go"
	sitterpython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	sitterrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	sittertypescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

//go:embed queries/*.scm
var treeSitterQueries embed.FS

type treeSitterLanguageSpec struct {
	language  func() *sitter.Language
	queryFile string
}

func treeSitterSpec(language string) (treeSitterLanguageSpec, bool) {
	switch strings.ToLower(language) {
	case "go", "golang":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sittergo.Language()) },
			queryFile: "queries/go.scm",
		}, true
	case "javascript", "js":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterjavascript.Language()) },
			queryFile: "queries/javascript.scm",
		}, true
	case "typescript", "ts":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sittertypescript.LanguageTypescript()) },
			queryFile: "queries/typescript.scm",
		}, true
	case "python", "py":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterpython.Language()) },
			queryFile: "queries/python.scm",
		}, true
	case "json":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterjson.Language()) },
			queryFile: "queries/json.scm",
		}, true
	case "sh", "bash", "zsh", "shell":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterbash.Language()) },
			queryFile: "queries/bash.scm",
		}, true
	case "zs":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterbash.Language()) },
			queryFile: "queries/bash.scm",
		}, true
	case "html":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterhtml.Language()) },
			queryFile: "queries/html.scm",
		}, true
	case "css":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sittercss.Language()) },
			queryFile: "queries/css.scm",
		}, true
	case "php":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterphp.LanguagePHP()) },
			queryFile: "queries/php.scm",
		}, true
	case "rust", "rs":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterrust.Language()) },
			queryFile: "queries/rust.scm",
		}, true
	case "java":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterjava.Language()) },
			queryFile: "queries/java.scm",
		}, true
	case "lua":
		return treeSitterLanguageSpec{
			language:  func() *sitter.Language { return sitter.NewLanguage(sitterlua.Language()) },
			queryFile: "queries/lua.scm",
		}, true
	default:
		return treeSitterLanguageSpec{}, false
	}
}

func treeSitterSpans(text string, offset int, language string) []markdownSpan {
	switch strings.ToLower(language) {
	case "md", "markdown":
		return markdownFenceSpans(text, offset)
	}

	spec, ok := treeSitterSpec(language)
	if !ok {
		return legacyCodeSpans(text, offset, language)
	}

	querySource, err := treeSitterQueries.ReadFile(spec.queryFile)
	if err != nil {
		return legacyCodeSpans(text, offset, language)
	}

	parser := sitter.NewParser()
	defer parser.Close()
	lang := spec.language()
	if err := parser.SetLanguage(lang); err != nil {
		return legacyCodeSpans(text, offset, language)
	}

	source := []byte(text)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return legacyCodeSpans(text, offset, language)
	}
	defer tree.Close()

	query, queryErr := sitter.NewQuery(lang, string(querySource))
	if queryErr != nil {
		return legacyCodeSpans(text, offset, language)
	}
	defer query.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	captures := cursor.Captures(query, tree.RootNode(), source)
	spans := make([]markdownSpan, 0)
	captureNames := query.CaptureNames()
	for match, captureIndex := captures.Next(); match != nil; match, captureIndex = captures.Next() {
		capture := match.Captures[captureIndex]
		tag, ok := treeSitterCaptureTag(captureNames[capture.Index])
		if !ok {
			continue
		}
		start := offset + byteOffsetToRuneOffset(source, int(capture.Node.StartByte()))
		end := offset + byteOffsetToRuneOffset(source, int(capture.Node.EndByte()))
		if start >= end {
			continue
		}
		spans = append(spans, markdownSpan{Tag: tag, Start: start, End: end})
	}
	if len(spans) == 0 {
		return legacyCodeSpans(text, offset, language)
	}

	return mergeMarkdownSpans(spans)
}

func markdownFenceSpans(text string, offset int) []markdownSpan {
	render := markdownRenderFromText(text)
	spans := make([]markdownSpan, 0, len(render.Spans))
	for _, span := range render.Spans {
		tag, ok := markdownCodeTag(span.Tag)
		if !ok {
			continue
		}
		spans = append(spans, markdownSpan{
			Tag:   tag,
			Start: offset + span.Start,
			End:   offset + span.End,
		})
	}
	return mergeMarkdownSpans(spans)
}

func markdownCodeTag(tag string) (string, bool) {
	switch tag {
	case tagHeading1, tagHeading2, tagHeading3, tagList, tagOrdered, tagChecklist:
		return tagCodeKeyword, true
	case tagQuote:
		return tagCodeComment, true
	case tagBold, tagItalic:
		return tagCodeConstant, true
	case tagCode:
		return tagCodeString, true
	case tagLink:
		return tagCodeProperty, true
	default:
		return "", false
	}
}

func treeSitterCaptureTag(capture string) (string, bool) {
	switch {
	case strings.HasPrefix(capture, "comment"):
		return tagCodeComment, true
	case strings.HasPrefix(capture, "string"), capture == "escape":
		return tagCodeString, true
	case strings.HasPrefix(capture, "number"), strings.Contains(capture, "number"):
		return tagCodeNumber, true
	case strings.HasPrefix(capture, "type"), strings.HasPrefix(capture, "tag"), strings.HasPrefix(capture, "attribute"), strings.HasPrefix(capture, "constructor"):
		return tagCodeType, true
	case strings.HasPrefix(capture, "function"), strings.HasPrefix(capture, "method"):
		return tagCodeFunction, true
	case strings.HasPrefix(capture, "property"), strings.HasPrefix(capture, "variable.member"), strings.HasPrefix(capture, "field"):
		return tagCodeProperty, true
	case strings.HasPrefix(capture, "constant"), capture == "boolean":
		return tagCodeConstant, true
	case strings.HasPrefix(capture, "keyword"), strings.HasPrefix(capture, "operator"), strings.HasPrefix(capture, "punctuation"):
		return tagCodeKeyword, true
	default:
		return "", false
	}
}

func byteOffsetToRuneOffset(source []byte, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(source) {
		byteOffset = len(source)
	}
	return utf8.RuneCount(source[:byteOffset])
}

func mergeMarkdownSpans(spans []markdownSpan) []markdownSpan {
	if len(spans) < 2 {
		return spans
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start == spans[j].Start {
			if spans[i].End == spans[j].End {
				return spans[i].Tag < spans[j].Tag
			}
			return spans[i].End < spans[j].End
		}
		return spans[i].Start < spans[j].Start
	})
	merged := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if span.Start >= span.End {
			continue
		}
		lastIdx := len(merged) - 1
		if lastIdx >= 0 && merged[lastIdx].Tag == span.Tag && span.Start <= merged[lastIdx].End {
			if span.End > merged[lastIdx].End {
				merged[lastIdx].End = span.End
			}
			continue
		}
		merged = append(merged, span)
	}
	return merged
}
