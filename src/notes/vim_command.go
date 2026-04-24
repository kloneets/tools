package notes

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type vimCommandKind string

const (
	vimCommandSave       vimCommandKind = "save"
	vimCommandSearch     vimCommandKind = "search"
	vimCommandReplace    vimCommandKind = "replace"
	vimCommandRename     vimCommandKind = "rename"
	vimCommandOpenLinks  vimCommandKind = "open-links"
	vimCommandUndo       vimCommandKind = "undo"
	vimCommandRedo       vimCommandKind = "redo"
	vimCommandPreview    vimCommandKind = "preview"
	vimCommandSidebar    vimCommandKind = "sidebar"
	vimCommandAddWord    vimCommandKind = "add-word"
	vimCommandSpell      vimCommandKind = "spell"
	vimCommandRecordKeys vimCommandKind = "record-keys"
	vimCommandQuit       vimCommandKind = "quit"
	vimCommandSequence   vimCommandKind = "sequence"
)

type vimCommand struct {
	Kind        vimCommandKind
	Query       string
	Replacement string
	Global      bool
	CurrentLine bool
	Name        string
	Commands    []vimCommand
	Force       bool
}

func parseVimCommand(raw string) (vimCommand, error) {
	cmd := strings.TrimSpace(raw)
	if cmd == "" {
		return vimCommand{}, fmt.Errorf("empty command")
	}

	if strings.HasPrefix(cmd, "/") {
		query := strings.TrimSpace(strings.TrimPrefix(cmd, "/"))
		if query == "" {
			return vimCommand{}, fmt.Errorf("search pattern is required")
		}
		return vimCommand{Kind: vimCommandSearch, Query: query}, nil
	}

	switch cmd {
	case "w", "write", "save":
		return vimCommand{Kind: vimCommandSave}, nil
	case "q", "quit":
		return vimCommand{Kind: vimCommandQuit}, nil
	case "ol":
		return vimCommand{Kind: vimCommandOpenLinks}, nil
	case "undo":
		return vimCommand{Kind: vimCommandUndo}, nil
	case "redo":
		return vimCommand{Kind: vimCommandRedo}, nil
	case "preview":
		return vimCommand{Kind: vimCommandPreview}, nil
	case "sidebar", "sb":
		return vimCommand{Kind: vimCommandSidebar}, nil
	case "addword", "spelladd":
		return vimCommand{Kind: vimCommandAddWord}, nil
	case "spell":
		return vimCommand{Kind: vimCommandSpell}, nil
	case "recordkeys":
		return vimCommand{Kind: vimCommandRecordKeys}, nil
	}

	if chained, ok := parseOneCharCommandChain(cmd); ok {
		return chained, nil
	}

	if strings.HasPrefix(cmd, "search ") {
		query := strings.TrimSpace(strings.TrimPrefix(cmd, "search "))
		if query == "" {
			return vimCommand{}, fmt.Errorf("search pattern is required")
		}
		return vimCommand{Kind: vimCommandSearch, Query: query}, nil
	}

	if strings.HasPrefix(cmd, "%s/") || strings.HasPrefix(cmd, "s/") {
		oldValue, newValue, global, currentLine, ok := parseSubstituteCommand(cmd)
		if !ok {
			return vimCommand{}, fmt.Errorf("invalid replace command")
		}
		return vimCommand{
			Kind:        vimCommandReplace,
			Query:       oldValue,
			Replacement: newValue,
			Global:      global,
			CurrentLine: currentLine,
		}, nil
	}

	if strings.HasPrefix(cmd, "replace ") {
		fields := strings.Fields(cmd)
		if len(fields) < 3 {
			return vimCommand{}, fmt.Errorf("replace requires search and replacement text")
		}
		return vimCommand{
			Kind:        vimCommandReplace,
			Query:       fields[1],
			Replacement: strings.Join(fields[2:], " "),
			Global:      true,
		}, nil
	}

	if strings.HasPrefix(cmd, "rename ") {
		name := strings.TrimSpace(strings.TrimPrefix(cmd, "rename "))
		if name == "" {
			return vimCommand{}, fmt.Errorf("rename requires a note name")
		}
		return vimCommand{Kind: vimCommandRename, Name: name}, nil
	}

	return vimCommand{}, fmt.Errorf("unknown command: %s", cmd)
}

func parseOneCharCommandChain(cmd string) (vimCommand, bool) {
	runes := []rune(cmd)
	if len(runes) < 2 {
		return vimCommand{}, false
	}
	commands := make([]vimCommand, 0, len(runes))
	seenSave := false
	for _, r := range runes {
		switch r {
		case 'w':
			commands = append(commands, vimCommand{Kind: vimCommandSave})
			seenSave = true
		case 'q':
			commands = append(commands, vimCommand{Kind: vimCommandQuit, Force: seenSave})
		default:
			return vimCommand{}, false
		}
	}
	return vimCommand{Kind: vimCommandSequence, Commands: commands}, true
}

func parseSubstituteCommand(cmd string) (string, string, bool, bool, bool) {
	currentLine := false
	rest := cmd
	switch {
	case strings.HasPrefix(cmd, "%s/"):
		rest = strings.TrimPrefix(cmd, "%s/")
	case strings.HasPrefix(cmd, "s/"):
		rest = strings.TrimPrefix(cmd, "s/")
		currentLine = true
	default:
		return "", "", false, false, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return "", "", false, false, false
	}

	oldValue := parts[0]
	newValue := parts[1]
	flags := ""
	if len(parts) > 2 {
		flags = parts[2]
	}
	if oldValue == "" {
		return "", "", false, false, false
	}

	return oldValue, newValue, strings.Contains(flags, "g"), currentLine, true
}

func findNext(text string, query string, start int) int {
	if query == "" {
		return -1
	}
	if start < 0 {
		start = 0
	}
	normalizedText := strings.ToLower(text)
	normalizedQuery := strings.ToLower(query)
	runes := []rune(normalizedText)
	queryRunes := []rune(normalizedQuery)
	if start > len(runes) {
		start = len(runes)
	}
	for i := start; i+len(queryRunes) <= len(runes); i++ {
		if string(runes[i:i+len(queryRunes)]) == normalizedQuery {
			return i
		}
	}
	return -1
}

func findPrevious(text string, query string, start int) int {
	if query == "" {
		return -1
	}
	normalizedText := strings.ToLower(text)
	normalizedQuery := strings.ToLower(query)
	runes := []rune(normalizedText)
	queryRunes := []rune(normalizedQuery)
	if start > len(runes)-len(queryRunes) {
		start = len(runes) - len(queryRunes)
	}
	if start < 0 {
		return -1
	}
	for i := start; i >= 0; i-- {
		if i+len(queryRunes) <= len(runes) && string(runes[i:i+len(queryRunes)]) == normalizedQuery {
			return i
		}
	}
	return -1
}

func replaceText(text string, oldValue string, newValue string, global bool) (string, int) {
	if oldValue == "" {
		return text, 0
	}
	if global {
		count := strings.Count(text, oldValue)
		return strings.ReplaceAll(text, oldValue, newValue), count
	}
	updated := strings.Replace(text, oldValue, newValue, 1)
	if updated == text {
		return text, 0
	}
	return updated, 1
}

func replaceTextInRange(text string, oldValue string, newValue string, global bool, start int, end int) (string, int) {
	runes := []rune(text)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start > end {
		start, end = end, start
	}
	updatedRange, count := replaceText(string(runes[start:end]), oldValue, newValue, global)
	if count == 0 {
		return text, 0
	}
	return string(runes[:start]) + updatedRange + string(runes[end:]), count
}

var bareExternalLinkPattern = regexp.MustCompile(`(?i)(?:https?|ftp|file)://[^\s<>"']+`)

func collectSupportedLinks(text string) []string {
	type occurrence struct {
		start int
		uri   string
	}
	occurrences := make([]occurrence, 0, 8)
	render := markdownPreview(text, 4)
	for _, link := range render.Links {
		occurrences = append(occurrences, occurrence{start: link.Start, uri: link.URL})
	}
	for _, idx := range bareExternalLinkPattern.FindAllStringIndex(text, -1) {
		occurrences = append(occurrences, occurrence{
			start: utf8.RuneCountInString(text[:idx[0]]),
			uri:   text[idx[0]:idx[1]],
		})
	}
	sort.SliceStable(occurrences, func(i, j int) bool {
		return occurrences[i].start < occurrences[j].start
	})

	seen := make(map[string]struct{})
	links := make([]string, 0, 8)
	add := func(uri string) {
		uri = trimTrailingExternalPunctuation(strings.TrimSpace(uri))
		if !isSupportedExternalURI(uri) {
			return
		}
		if _, ok := seen[uri]; ok {
			return
		}
		seen[uri] = struct{}{}
		links = append(links, uri)
	}

	for _, item := range occurrences {
		add(item.uri)
	}
	return links
}

func CollectSupportedLinks(text string) []string {
	return collectSupportedLinks(text)
}

func isSupportedExternalURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ftp", "file":
		return true
	default:
		return false
	}
}

func trimTrailingExternalPunctuation(raw string) string {
	return strings.TrimRight(raw, ".,;:!?)]}\"'>")
}
