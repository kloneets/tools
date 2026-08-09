package notes

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type vimCommandKind string

const (
	vimCommandSave         vimCommandKind = "save"
	vimCommandSearch       vimCommandKind = "search"
	vimCommandReplace      vimCommandKind = "replace"
	vimCommandRename       vimCommandKind = "rename"
	vimCommandOpenLinks    vimCommandKind = "open-links"
	vimCommandUndo         vimCommandKind = "undo"
	vimCommandRedo         vimCommandKind = "redo"
	vimCommandPreview      vimCommandKind = "preview"
	vimCommandSidebar      vimCommandKind = "sidebar"
	vimCommandAddWord      vimCommandKind = "add-word"
	vimCommandSpell        vimCommandKind = "spell"
	vimCommandRecordKeys   vimCommandKind = "record-keys"
	vimCommandBufferDelete vimCommandKind = "buffer-delete"
	vimCommandLineMove     vimCommandKind = "line-move"
	vimCommandQuit         vimCommandKind = "quit"
	vimCommandSequence     vimCommandKind = "sequence"
)

type vimCommand struct {
	Kind        vimCommandKind
	Query       string
	Replacement string
	Global      bool
	Confirm     bool
	IgnoreCase  bool
	SearchBack  bool
	Literal     bool
	CurrentLine bool
	Range       vimCommandRange
	Name        string
	Commands    []vimCommand
	Force       bool
	LineDelta   int
}

type vimCommandRangeKind string

const (
	vimRangeDefault vimCommandRangeKind = ""
	vimRangeCurrent vimCommandRangeKind = "current"
	vimRangeAll     vimCommandRangeKind = "all"
	vimRangeLines   vimCommandRangeKind = "lines"
	vimRangeVisual  vimCommandRangeKind = "visual"
)

type vimCommandRange struct {
	Kind  vimCommandRangeKind
	Start int
	End   int
}

func parseVimCommand(raw string) (vimCommand, error) {
	cmd := strings.TrimSpace(raw)
	if cmd == "" {
		return vimCommand{}, fmt.Errorf("empty command")
	}
	if strings.HasPrefix(cmd, "'<,'>") && !strings.HasPrefix(cmd, "'<,'>s") {
		return parseVimCommand(strings.TrimSpace(strings.TrimPrefix(cmd, "'<,'>")))
	}

	if strings.HasPrefix(cmd, "/") {
		query := strings.TrimSpace(strings.TrimPrefix(cmd, "/"))
		if query == "" {
			return vimCommand{}, fmt.Errorf("search pattern is required")
		}
		return vimCommand{Kind: vimCommandSearch, Query: query}, nil
	}

	if strings.HasPrefix(cmd, "?") {
		query := strings.TrimSpace(strings.TrimPrefix(cmd, "?"))
		if query == "" {
			return vimCommand{}, fmt.Errorf("search pattern is required")
		}
		return vimCommand{Kind: vimCommandSearch, Query: query, SearchBack: true}, nil
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
	case "bd", "bdelete":
		return vimCommand{Kind: vimCommandBufferDelete}, nil
	}

	if move, ok, err := parseMoveLineCommand(cmd); ok || err != nil {
		return move, err
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

	if replacement, ok, err := parseSubstituteCommand(cmd); ok || err != nil {
		return replacement, err
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
			Range:       vimCommandRange{Kind: vimRangeAll},
			Literal:     true,
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

func parseMoveLineCommand(cmd string) (vimCommand, bool, error) {
	if !strings.HasPrefix(cmd, "ml") {
		return vimCommand{}, false, nil
	}
	if len(cmd) < 3 {
		return vimCommand{}, true, fmt.Errorf("invalid line move command: %s", cmd)
	}
	direction := cmd[len(cmd)-1]
	if direction != 'u' && direction != 'd' {
		return vimCommand{}, true, fmt.Errorf("invalid line move command: %s", cmd)
	}
	countText := cmd[2 : len(cmd)-1]
	count := 1
	if countText != "" {
		for _, r := range countText {
			if r < '0' || r > '9' {
				return vimCommand{}, true, fmt.Errorf("invalid line move count: %s", countText)
			}
		}
		parsed, err := strconv.Atoi(countText)
		if err != nil || parsed <= 0 {
			return vimCommand{}, true, fmt.Errorf("invalid line move count: %s", countText)
		}
		count = parsed
	}
	if direction == 'u' {
		count = -count
	}
	return vimCommand{Kind: vimCommandLineMove, LineDelta: count}, true, nil
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

func parseSubstituteCommand(cmd string) (vimCommand, bool, error) {
	sIndex := substituteCommandIndex(cmd)
	if sIndex < 0 {
		return vimCommand{}, false, nil
	}
	if sIndex+1 >= len(cmd) {
		return vimCommand{}, true, fmt.Errorf("invalid replace command")
	}
	rangeSpec := cmd[:sIndex]
	delimiter := rune(cmd[sIndex+1])
	if delimiter == '\\' || delimiter == 0 {
		return vimCommand{}, true, fmt.Errorf("invalid replace command")
	}
	rest := cmd[sIndex+2:]
	pattern, rest, ok := readDelimitedField(rest, delimiter)
	if !ok {
		return vimCommand{}, true, fmt.Errorf("invalid replace command")
	}
	replacement, flags, ok := readDelimitedField(rest, delimiter)
	if !ok {
		replacement = rest
		flags = ""
	}
	if pattern == "" {
		return vimCommand{}, true, fmt.Errorf("replace pattern is required")
	}
	parsedRange, err := parseSubstituteRange(rangeSpec)
	if err != nil {
		return vimCommand{}, true, err
	}
	cmdOut := vimCommand{
		Kind:        vimCommandReplace,
		Query:       unescapeDelimited(pattern, delimiter),
		Replacement: unescapeDelimited(replacement, delimiter),
		Range:       parsedRange,
		CurrentLine: parsedRange.Kind == vimRangeCurrent,
	}
	for _, flag := range flags {
		switch flag {
		case 'g':
			cmdOut.Global = true
		case 'c':
			cmdOut.Confirm = true
		case 'i':
			cmdOut.IgnoreCase = true
		case 'I':
			cmdOut.IgnoreCase = false
		default:
			return vimCommand{}, true, fmt.Errorf("unsupported replace flag: %c", flag)
		}
	}
	return cmdOut, true, nil
}

func substituteCommandIndex(cmd string) int {
	if cmd == "" {
		return -1
	}
	if cmd[0] == 's' {
		return 0
	}
	for i := 0; i < len(cmd); i++ {
		if cmd[i] != 's' {
			continue
		}
		prefix := cmd[:i]
		if prefix == "%" || prefix == "." || prefix == "$" || prefix == "'<,'>" || validLineRange(prefix) {
			return i
		}
	}
	return -1
}

func validLineRange(raw string) bool {
	if raw == "" {
		return false
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if part == "." || part == "$" {
			continue
		}
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func readDelimitedField(raw string, delimiter rune) (string, string, bool) {
	escaped := false
	for i, r := range raw {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == delimiter {
			return raw[:i], raw[i+len(string(r)):], true
		}
	}
	return "", "", false
}

func unescapeDelimited(raw string, delimiter rune) string {
	var b strings.Builder
	escaped := false
	for _, r := range raw {
		if escaped {
			if r == delimiter || r == '\\' {
				b.WriteRune(r)
			} else {
				b.WriteRune('\\')
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func parseSubstituteRange(raw string) (vimCommandRange, error) {
	switch raw {
	case "":
		return vimCommandRange{Kind: vimRangeCurrent}, nil
	case "%":
		return vimCommandRange{Kind: vimRangeAll}, nil
	case "'<,'>":
		return vimCommandRange{Kind: vimRangeVisual}, nil
	case ".":
		return vimCommandRange{Kind: vimRangeCurrent}, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 2 {
		return vimCommandRange{}, fmt.Errorf("invalid replace range: %s", raw)
	}
	start, err := parseRangeLine(parts[0])
	if err != nil {
		return vimCommandRange{}, fmt.Errorf("invalid replace range: %s", raw)
	}
	end := start
	if len(parts) == 2 {
		end, err = parseRangeLine(parts[1])
		if err != nil {
			return vimCommandRange{}, fmt.Errorf("invalid replace range: %s", raw)
		}
	}
	return vimCommandRange{Kind: vimRangeLines, Start: start, End: end}, nil
}

func parseRangeLine(raw string) (int, error) {
	switch raw {
	case ".":
		return 0, nil
	case "$":
		return -1, nil
	}
	line, err := strconv.Atoi(raw)
	if err != nil || line <= 0 {
		return 0, fmt.Errorf("invalid line")
	}
	return line, nil
}

func findNext(text string, query string, start int) int {
	if query == "" {
		return -1
	}
	re, err := compileVimRegex(query, true)
	if err == nil {
		return findNextRegex(text, re, start)
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
	re, err := compileVimRegex(query, true)
	if err == nil {
		return findPreviousRegex(text, re, start)
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

func findNextRegex(text string, re *regexp.Regexp, start int) int {
	runes := []rune(text)
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	byteStart := len(string(runes[:start]))
	loc := re.FindStringIndex(text[byteStart:])
	if loc == nil {
		return -1
	}
	return utf8.RuneCountInString(text[:byteStart+loc[0]])
}

func findPreviousRegex(text string, re *regexp.Regexp, start int) int {
	runes := []rune(text)
	if len(runes) == 0 {
		return -1
	}
	if start >= len(runes) {
		start = len(runes) - 1
	}
	if start < 0 {
		return -1
	}
	startByte := len(string(runes[:start]))
	matches := re.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return -1
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i][0] <= startByte {
			return utf8.RuneCountInString(text[:matches[i][0]])
		}
	}
	return -1
}

func replaceText(text string, oldValue string, newValue string, global bool) (string, int) {
	if oldValue == "" {
		return text, 0
	}
	re, err := compileVimRegex(oldValue, false)
	if err == nil {
		updated, count, _ := replaceRegexInRange(text, re, newValue, global, 0, len([]rune(text)))
		return updated, count
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
	re, err := compileVimRegex(oldValue, false)
	if err == nil {
		updated, count, _ := replaceRegexInRange(text, re, newValue, global, start, end)
		return updated, count
	}
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

type replaceCandidate struct {
	Start       int
	End         int
	Replacement string
}

func compileVimRegex(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	translated := translateVimRegex(pattern)
	if ignoreCase {
		translated = "(?i)" + translated
	}
	return regexp.Compile(translated)
}

func translateVimRegex(pattern string) string {
	var b strings.Builder
	escaped := false
	for _, r := range pattern {
		if escaped {
			switch r {
			case '(', ')', '+', '?', '|':
				b.WriteRune(r)
			case '<', '>':
				b.WriteString(`\b`)
			default:
				b.WriteRune('\\')
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func vimReplacementTemplate(replacement string) string {
	var b strings.Builder
	escaped := false
	for _, r := range replacement {
		if escaped {
			if r >= '0' && r <= '9' {
				b.WriteRune('$')
				b.WriteRune(r)
			} else {
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '&':
			b.WriteString("$0")
		case '$':
			b.WriteString("$$")
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func replaceRegexInRange(text string, re *regexp.Regexp, replacement string, global bool, start int, end int) (string, int, []replaceCandidate) {
	candidates := collectReplaceCandidates(text, re, replacement, global, start, end)
	if len(candidates) == 0 {
		return text, 0, nil
	}
	updated := applyReplaceCandidates(text, candidates, nil)
	return updated, len(candidates), candidates
}

func collectReplaceCandidates(text string, re *regexp.Regexp, replacement string, global bool, start int, end int) []replaceCandidate {
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
	scope := string(runes[start:end])
	template := vimReplacementTemplate(replacement)
	byteToRune := func(byteOffset int) int {
		return start + utf8.RuneCountInString(scope[:byteOffset])
	}
	buildCandidate := func(match []int) replaceCandidate {
		return replaceCandidate{
			Start:       byteToRune(match[0]),
			End:         byteToRune(match[1]),
			Replacement: expandRegexpReplacement(scope, re, template, match),
		}
	}
	if global {
		matches := re.FindAllStringSubmatchIndex(scope, -1)
		candidates := make([]replaceCandidate, 0, len(matches))
		for _, match := range matches {
			if match[0] == match[1] {
				continue
			}
			candidates = append(candidates, buildCandidate(match))
		}
		return candidates
	}
	candidates := make([]replaceCandidate, 0, 8)
	lineStartByte := 0
	for lineStartByte <= len(scope) {
		lineEndByte := strings.IndexByte(scope[lineStartByte:], '\n')
		if lineEndByte < 0 {
			lineEndByte = len(scope)
		} else {
			lineEndByte += lineStartByte
		}
		line := scope[lineStartByte:lineEndByte]
		match := re.FindStringSubmatchIndex(line)
		if match != nil && match[0] != match[1] {
			for i := range match {
				if match[i] >= 0 {
					match[i] += lineStartByte
				}
			}
			candidates = append(candidates, buildCandidate(match))
		}
		if lineEndByte == len(scope) {
			break
		}
		lineStartByte = lineEndByte + 1
	}
	return candidates
}

func expandRegexpReplacement(scope string, re *regexp.Regexp, template string, match []int) string {
	var out []byte
	out = re.ExpandString(out, template, scope, match)
	return string(out)
}

func applyReplaceCandidates(text string, candidates []replaceCandidate, accepted []bool) string {
	runes := []rune(text)
	for i := len(candidates) - 1; i >= 0; i-- {
		if accepted != nil && !accepted[i] {
			continue
		}
		candidate := candidates[i]
		replacement := []rune(candidate.Replacement)
		updated := make([]rune, 0, len(runes)-(candidate.End-candidate.Start)+len(replacement))
		updated = append(updated, runes[:candidate.Start]...)
		updated = append(updated, replacement...)
		updated = append(updated, runes[candidate.End:]...)
		runes = updated
	}
	return string(runes)
}

var bareExternalLinkPattern = regexp.MustCompile(`(?i)(?:https?|ftp|file)://[^\s<>"']+`)
var markdownExternalLinkPattern = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)

// SupportedLink describes the clickable source text for a supported URI.
type SupportedLink struct {
	Start int
	End   int
	URI   string
}

func FindSupportedLinks(text string) []SupportedLink {
	links := make([]SupportedLink, 0, 8)
	for _, match := range markdownExternalLinkPattern.FindAllStringSubmatchIndex(text, -1) {
		uri := trimTrailingExternalPunctuation(strings.TrimSpace(text[match[4]:match[5]]))
		if !isSupportedExternalURI(uri) {
			continue
		}
		links = append(links, SupportedLink{
			Start: utf8.RuneCountInString(text[:match[2]]),
			End:   utf8.RuneCountInString(text[:match[3]]),
			URI:   uri,
		})
	}
	for _, idx := range bareExternalLinkPattern.FindAllStringIndex(text, -1) {
		raw := text[idx[0]:idx[1]]
		uri := trimTrailingExternalPunctuation(strings.TrimSpace(raw))
		if !isSupportedExternalURI(uri) {
			continue
		}
		start := utf8.RuneCountInString(text[:idx[0]])
		links = append(links, SupportedLink{
			Start: start,
			End:   start + utf8.RuneCountInString(uri),
			URI:   uri,
		})
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].Start == links[j].Start {
			return links[i].End < links[j].End
		}
		return links[i].Start < links[j].Start
	})
	return links
}

func SupportedLinkAt(text string, offset int) (string, bool) {
	for _, link := range FindSupportedLinks(text) {
		if offset >= link.Start && offset < link.End {
			return link.URI, true
		}
	}
	return "", false
}

func collectSupportedLinks(text string) []string {
	seen := make(map[string]struct{})
	links := make([]string, 0, 8)
	for _, item := range FindSupportedLinks(text) {
		if _, ok := seen[item.URI]; ok {
			continue
		}
		seen[item.URI] = struct{}{}
		links = append(links, item.URI)
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
