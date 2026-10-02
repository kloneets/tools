package app

import (
	"fmt"
	"strings"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/todo"
)

type todoSearchState struct {
	active, editing                     bool
	query, previous, selectedID, listID string
	direction                           int
	results                             []todoSearchResult
	order                               string
}
type todoSearchResult struct{ id, month string }

func (a *terminalApp) updateTodoSearch() {
	s := &a.todoSearch
	if a.todos != nil {
		id := a.todos.CurrentListID()
		if s.listID != "" && s.listID != id {
			a.todoSearch = todoSearchState{}
			a.todoScroll = 0
			return
		}
		s.listID = id
	}
	if !s.active {
		return
	}
	s.results = nil
	if s.query == "" {
		s.selectedID = ""
		return
	}
	var order strings.Builder
	add := func(items []todo.Item, month string) {
		for _, item := range items {
			order.WriteString(item.ID)
			order.WriteByte(0)
			order.WriteString(month)
			order.WriteByte(0)
			if _, ok := todo.FuzzyMatch(item.Text, s.query); ok {
				s.results = append(s.results, todoSearchResult{item.ID, month})
			}
		}
	}
	add(todo.ShortItems(a.todoStore), "")
	add(todo.LongItems(a.todoStore), "")
	groups := todo.ArchiveGroups(a.todoStore)
	for _, month := range todo.ArchiveMonths(a.todoStore) {
		order.WriteString(month)
		order.WriteByte(0)
		add(groups[month], month)
	}
	changed := s.order != order.String()
	s.order = order.String()
	for _, result := range s.results {
		if result.id == s.selectedID {
			if changed {
				a.revealTodoSearch()
			}
			return
		}
	}
	s.selectedID = ""
	if len(s.results) > 0 {
		index := 0
		if s.direction < 0 {
			index = len(s.results) - 1
		}
		s.selectedID = s.results[index].id
		a.revealTodoSearch()
	}
}
func (a *terminalApp) revealTodoSearch() {
	a.todoScrollManual = false
	for _, result := range a.todoSearch.results {
		if result.id == a.todoSearch.selectedID {
			if result.month != "" {
				if a.todoArchiveExpanded == nil {
					a.todoArchiveExpanded = map[string]bool{}
				}
				a.todoArchiveExpanded[result.month] = true
			}
			a.selectTodoByID(result.id)
			a.todoListFocus = false
			return
		}
	}
}
func (a *terminalApp) stepTodoSearch(delta int) {
	a.updateTodoSearch()
	n := len(a.todoSearch.results)
	if n == 0 {
		return
	}
	index := 0
	for i, result := range a.todoSearch.results {
		if result.id == a.todoSearch.selectedID {
			index = i
			break
		}
	}
	a.todoSearch.selectedID = a.todoSearch.results[(index+delta+n)%n].id
	a.revealTodoSearch()
}
func (a *terminalApp) handleTodoSearchKey(key notes.Key) bool {
	s := &a.todoSearch
	if s.editing {
		switch key.Name {
		case "esc":
			s.query = s.previous
			s.editing = false
			if s.query == "" {
				s.active = false
			}
		case "enter":
			s.editing = false
			a.revealTodoSearch()
		case "backspace":
			r := []rune(s.query)
			if len(r) > 0 {
				s.query = string(r[:len(r)-1])
			}
		default:
			if key.Rune != 0 && !key.Ctrl && !key.Meta && !key.Alt {
				s.query += string(key.Rune)
			}
		}
		a.updateTodoSearch()
		return true
	}
	if key.Ctrl || key.Meta || key.Alt || a.todoInputMode != "" {
		return false
	}
	switch key.Name {
	case "/", "?":
		s.active = true
		s.editing = true
		s.previous = s.query
		s.query = ""
		s.direction = 1
		if key.Name == "?" {
			s.direction = -1
		}
		a.updateTodoSearch()
		return true
	case "esc":
		if s.active {
			a.todoSearch = todoSearchState{}
			return true
		}
	case "n", "N":
		if s.active {
			delta := s.direction
			if key.Name == "N" {
				delta = -delta
			}
			a.stepTodoSearch(delta)
			return true
		}
	}
	return false
}
func (a *terminalApp) todoSearchText(text string) string {
	if !a.todoSearch.active || a.todoSearch.query == "" {
		return text
	}
	positions, ok := todo.FuzzyMatch(text, a.todoSearch.query)
	if !ok {
		return text
	}
	var out strings.Builder
	next := 0
	for i, r := range []rune(text) {
		if next < len(positions) && positions[next] == i {
			out.WriteString(helpers.ANSI(helpers.ANSIRoleSearch, string(r)))
			next++
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
func (a *terminalApp) todoSearchBar() string {
	s := &a.todoSearch
	counter := ""
	if s.query != "" {
		counter = "No matches"
		for i, result := range s.results {
			if result.id == s.selectedID {
				counter = fmt.Sprintf("%d/%d", i+1, len(s.results))
				break
			}
		}
	}
	prefix := "/"
	if s.direction < 0 {
		prefix = "?"
	}
	return prefix + s.query + "  " + counter + "  Current list · active + loaded archive"
}

// Keep title and draft/list tabs pinned while scrolling the task rows.
func (a *terminalApp) todoViewport(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	bodyEnd := height
	if a.todoSearch.active {
		bodyEnd--
	}
	pinned := min(2, max(0, bodyEnd))
	available := max(0, bodyEnd-pinned)
	selected := -1
	for row, index := range a.todoClickRows {
		if index == a.todoIndex {
			selected = row
		}
	}
	if !a.todoScrollManual && selected >= pinned && available > 0 {
		if selected < pinned+a.todoScroll {
			a.todoScroll = selected - pinned
		}
		if selected >= pinned+a.todoScroll+available {
			a.todoScroll = selected - pinned - available + 1
		}
	}
	a.todoScroll = max(0, min(a.todoScroll, max(0, len(lines)-pinned-available)))
	output := append([]string{}, lines[:min(pinned, len(lines))]...)
	start := min(len(lines), pinned+a.todoScroll)
	output = append(output, lines[start:min(len(lines), start+available)]...)
	mapped := map[int]int{}
	for row, index := range a.todoClickRows {
		visible := row - a.todoScroll
		if row >= start && visible < bodyEnd {
			mapped[visible] = index
		}
	}
	a.todoClickRows = mapped
	for len(output) < bodyEnd {
		output = append(output, "")
	}
	if a.todoSearch.active {
		output = append(output, a.todoSearchBar())
	}
	return strings.Join(output, "\n")
}
