package notes

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kloneets/tools/src/helpers"
)

type sidebarSearchState struct {
	active         bool
	editing        bool
	query          string
	openEntries    []TreeEntry
	browserEntries []TreeEntry
	initialized    bool
}

func (w *Workspace) handleSidebarSearchKey(key Key) bool {
	s := &w.sidebarSearch
	if s.editing {
		switch key.Name {
		case "enter", "esc":
			s.editing = false
		case "backspace":
			r := []rune(s.query)
			if len(r) > 0 {
				s.query = string(r[:len(r)-1])
			}
			w.updateSidebarSearch()
		default:
			if !key.Ctrl && !key.Alt && !key.Meta && key.Rune >= ' ' {
				s.query += string(key.Rune)
				w.updateSidebarSearch()
			}
		}
		return true
	}
	if key.Ctrl || key.Alt || key.Meta {
		return false
	}
	switch key.Name {
	case "/":
		w.clearSidebarSearch()
		*s = sidebarSearchState{active: true, editing: true}
		w.updateSidebarSearch()
		return true
	case "esc":
		if s.active {
			w.clearSidebarSearch()
			return true
		}
	case "n", "N":
		if !s.active {
			return false
		}
		matches := w.sidebarSearchMatches()
		if len(matches) == 0 {
			return true
		}
		selection := w.activeSidebarSelection()
		target := matches[0]
		if key.Name == "n" {
			for _, index := range matches {
				if index > selection {
					target = index
					break
				}
			}
		} else {
			target = matches[len(matches)-1]
			for i := len(matches) - 1; i >= 0; i-- {
				if matches[i] < selection {
					target = matches[i]
					break
				}
			}
		}
		w.selectSidebarSearchMatch(target)
		return true
	}
	return false
}

func sidebarMatchSpans(label, query string) []markdownSpan {
	if query == "" {
		return nil
	}
	text := []rune(strings.Map(unicode.ToLower, label))
	needle := []rune(strings.Map(unicode.ToLower, query))
	spans := []markdownSpan{}
	for at := 0; at+len(needle) <= len(text); {
		if string(text[at:at+len(needle)]) == string(needle) {
			spans = append(spans, markdownSpan{Tag: tagSearch, Start: at, End: at + len(needle)})
			at += len(needle)
		} else {
			at++
		}
	}
	if len(spans) > 0 {
		return spans
	}
	// Fall back to ordered subsequence matching, retaining rune offsets for highlighting.
	at := 0
	for _, char := range needle {
		for at < len(text) && text[at] != char {
			at++
		}
		if at == len(text) {
			return nil
		}
		if len(spans) > 0 && spans[len(spans)-1].End == at {
			spans[len(spans)-1].End++
		} else {
			spans = append(spans, markdownSpan{Tag: tagSearch, Start: at, End: at + 1})
		}
		at++
	}

	return spans
}

func (w *Workspace) sidebarSearchMatches() []int {
	if !w.sidebarSearch.active || w.sidebarSearch.query == "" {
		return nil
	}
	var matches []int
	for i, entry := range w.activeSidebarEntries() {
		if (entry.Kind == treeNote || entry.Kind == treeOpenNote) && len(sidebarMatchSpans(entry.Label, w.sidebarSearch.query)) > 0 {
			matches = append(matches, i)
		}
	}
	return matches
}

func (w *Workspace) selectSidebarSearchMatch(index int) {
	if w.SidebarBrowsing {
		w.BrowserSelection = index
	} else {
		w.Selection = index
	}
}

func (w *Workspace) selectFirstSidebarMatch() {
	if matches := w.sidebarSearchMatches(); len(matches) > 0 {
		w.selectSidebarSearchMatch(matches[0])
	}
}

func (w *Workspace) sidebarSearchLabel(entry TreeEntry, width int) string {
	label := []rune(entry.Label)
	visible := min(len(label), max(0, width))
	text := string(label[:visible])
	if !w.sidebarSearch.active || (entry.Kind != treeNote && entry.Kind != treeOpenNote) {
		return text
	}
	runes := label[:visible]
	var out strings.Builder
	previous := 0
	for _, span := range sidebarMatchSpans(entry.Label, w.sidebarSearch.query) {
		if span.Start >= visible {
			break
		}
		span.End = min(span.End, visible)
		out.WriteString(string(runes[previous:span.Start]))
		out.WriteString(helpers.ANSI(helpers.ANSIRoleSearch, string(runes[span.Start:span.End])))
		previous = span.End
	}
	out.WriteString(string(runes[previous:]))
	return out.String()
}

func (w *Workspace) sidebarSearchPrompt() string {
	s := w.sidebarSearch
	prompt := "/" + s.query
	if s.query != "" {
		matches := w.sidebarSearchMatches()
		if len(matches) == 0 {
			prompt += " | No matches"
		} else {
			prompt += fmt.Sprintf(" | %d matches", len(matches))
		}
	}
	if s.editing {
		return prompt + " | enter confirm | esc finish"
	}
	return prompt + " | n next | N previous | esc clear | enter open"
}

// Search materializes a visible tree so rendering and every input path share indices.
// The unfiltered entries retain the user's folder collapse choices.
func (w *Workspace) updateSidebarSearch() {
	s := &w.sidebarSearch
	if !s.initialized {
		s.openEntries = append([]TreeEntry(nil), w.Tree...)
		s.browserEntries = append([]TreeEntry(nil), w.BrowserTree...)
		s.initialized = true
	}
	w.applySidebarSearch()
	w.selectFirstSidebarMatch()
}

func (w *Workspace) applySidebarSearch() {
	s := &w.sidebarSearch
	if !s.initialized {
		return
	}
	openPath, browserPath := "", ""
	if e := w.selectedOpenEntry(); e != nil {
		openPath = e.Path
	}
	if e := w.selectedBrowserEntry(); e != nil {
		browserPath = e.Path
	}
	w.Tree = filterSidebarEntries(s.openEntries, s.query)
	source := s.browserEntries
	if s.query != "" && w.sidebarBrowserAll != nil {
		source = w.sidebarBrowserAll
	}
	w.BrowserTree = filterSidebarEntries(source, s.query)
	w.Selection = clampSidebarSelectionByPath(w.Tree, w.Selection, openPath)
	w.BrowserSelection = clampSidebarSelectionByPath(w.BrowserTree, w.BrowserSelection, browserPath)
}

func filterSidebarEntries(entries []TreeEntry, query string) []TreeEntry {
	if query == "" {
		return append([]TreeEntry(nil), entries...)
	}
	ancestors := make(map[string]bool)
	for _, entry := range entries {
		if (entry.Kind != treeNote && entry.Kind != treeOpenNote) || len(sidebarMatchSpans(entry.Label, query)) == 0 {
			continue
		}
		for folder := entry.Folder; folder != "" && folder != "."; folder = filepath.Dir(folder) {
			ancestors[folder] = true
		}
	}
	var visible []TreeEntry
	for _, entry := range entries {
		if entry.Kind == treeFolder {
			if !ancestors[entry.Folder] {
				continue
			}
			entry.Collapsed = false
		} else if (entry.Kind != treeNote && entry.Kind != treeOpenNote) || len(sidebarMatchSpans(entry.Label, query)) == 0 {
			continue
		}
		visible = append(visible, entry)
	}
	return visible
}

func (w *Workspace) clearSidebarSearch() {
	s := w.sidebarSearch
	openPath, browserPath := "", ""
	if e := w.selectedOpenEntry(); e != nil {
		openPath = e.Path
	}
	if e := w.selectedBrowserEntry(); e != nil {
		browserPath = e.Path
	}
	if s.initialized {
		w.Tree = s.openEntries
		w.BrowserTree = s.browserEntries
	}
	w.sidebarSearch = sidebarSearchState{}
	w.Selection = clampSidebarSelectionByPath(w.Tree, w.Selection, openPath)
	w.BrowserSelection = clampSidebarSelectionByPath(w.BrowserTree, w.BrowserSelection, browserPath)
}
