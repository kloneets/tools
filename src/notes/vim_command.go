package notes

import (
	"fmt"
	"strings"
)

type vimCommandKind string

const (
	vimCommandSave    vimCommandKind = "save"
	vimCommandSearch  vimCommandKind = "search"
	vimCommandReplace vimCommandKind = "replace"
)

type vimCommand struct {
	Kind        vimCommandKind
	Query       string
	Replacement string
	Global      bool
	CurrentLine bool
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

	return vimCommand{}, fmt.Errorf("unknown command: %s", cmd)
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
