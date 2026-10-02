package app

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
	"github.com/kloneets/tools/src/todo"
	"github.com/rivo/tview"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func searchTestApp() *terminalApp {
	return &terminalApp{view: viewTodo, todoStore: todo.Store{Items: []todo.Item{
		{ID: "1", Text: "groceries", Status: todo.StatusTodo}, {ID: "2", Text: "great carrots", Status: todo.StatusTodo},
	}}}
}
func TestTodoSearchRoutingAndUnicode(t *testing.T) {
	a := searchTestApp()
	for _, r := range "/?qā😀" {
		if !a.handleGlobalKey(notes.Key{Name: string(r), Rune: r}) {
			t.Fatal("key not consumed")
		}
	}
	if a.showHelp || a.shuttingDown || a.todoSearch.query != "?qā😀" {
		t.Fatalf("bad search state: %+v", a.todoSearch)
	}
	a.handleGlobalKey(notes.Key{Name: "backspace"})
	if a.todoSearch.query != "?qā" {
		t.Fatal(a.todoSearch.query)
	}
	a.handleGlobalKey(notes.Key{Name: "esc"})
	if a.todoSearch.active {
		t.Fatal("cancel should clear first query")
	}
	a.handleGlobalKey(notes.Key{Name: "?", Rune: '?'})
	if a.todoSearch.direction != -1 || a.showHelp {
		t.Fatal("backward search intercepted by help")
	}
}
func TestTodoSearchNavigationAndUpdates(t *testing.T) {
	a := searchTestApp()
	a.todoSearch = todoSearchState{active: true, query: "grc", direction: 1}
	a.updateTodoSearch()
	if len(a.todoSearch.results) != 2 || a.todoSearch.selectedID != "1" {
		t.Fatal(a.todoSearch)
	}
	a.handleGlobalKey(notes.Key{Name: "n", Rune: 'n'})
	if a.todoSearch.selectedID != "2" || a.todoInputMode != "" {
		t.Fatal(a.todoSearch)
	}
	a.handleGlobalKey(notes.Key{Name: "n", Rune: 'n'})
	if a.todoSearch.selectedID != "1" {
		t.Fatal("no wrap")
	}
	a.handleGlobalKey(notes.Key{Name: "N", Rune: 'N'})
	if a.todoSearch.selectedID != "2" {
		t.Fatal("no reverse")
	}
	a.todoStore.Items[0].Text = "changed"
	a.updateTodoSearch()
	if a.todoSearch.selectedID != "2" || len(a.todoSearch.results) != 1 {
		t.Fatal("did not retain task ID")
	}
	a.todoStore.Items = nil
	a.updateTodoSearch()
	if !strings.Contains(a.todoSearchBar(), "No matches") {
		t.Fatal(a.todoSearchBar())
	}
	a.todoSearch.query = ""
	a.updateTodoSearch()
	if a.todoSearchText("text") != "text" || strings.Contains(a.todoSearchBar(), "No matches") {
		t.Fatal("empty query")
	}
}
func TestTodoSearchCollapsedArchiveAndViewport(t *testing.T) {
	a := searchTestApp()
	now := time.Now()
	for i := 0; i < 30; i++ {
		a.todoStore.Items = append(a.todoStore.Items, todo.Item{ID: fmt.Sprint(i + 10), Text: "filler", Status: todo.StatusTodo})
	}
	a.todoStore.Items = append(a.todoStore.Items, todo.Item{ID: "archive", Text: "archived target", Status: todo.StatusArchived, ArchivedAt: &now})
	a.todoSearch = todoSearchState{active: true, query: "atg", direction: 1}
	rendered := helpers.StripANSI(a.renderTodo(10))
	if !a.todoArchiveExpanded[now.UTC().Format("2006-01")] || !strings.Contains(rendered, "archived target") {
		t.Fatal(rendered)
	}
	selected, ok := a.selectedTodoItem()
	if !ok || selected.ID != "archive" {
		t.Fatal(selected)
	}
	if a.todoScroll == 0 {
		t.Fatal("did not scroll")
	}
	for row, index := range a.todoClickRows {
		if row < 2 || row >= 9 || index < 0 {
			t.Fatal("invalid mouse map")
		}
	}
	if !strings.Contains(rendered, "1/1") || !strings.Contains(rendered, "Current list · active + loaded archive") {
		t.Fatal(rendered)
	}
	// The selected archive remains read-only.
	a.handleTodoKey(notes.Key{Name: "e", Rune: 'e'})
	if a.todoInputMode != "" {
		t.Fatal("archived task editable")
	}
}
func TestTodoSearchScrolledMouseCoordinates(t *testing.T) {
	a := searchTestApp()
	for i := 0; i < 30; i++ {
		a.todoStore.Items = append(a.todoStore.Items, todo.Item{ID: fmt.Sprint(i + 10), Text: fmt.Sprint("task ", i), Status: todo.StatusTodo})
	}
	a.todoIndex = 25
	a.single = tview.NewTextView()
	a.single.SetRect(0, 0, 80, 10)
	a.singleLinkText = helpers.StripANSI(a.renderTodo(10))
	for row, index := range a.todoClickRows {
		if !a.handleTodoMouse(tcell.NewEventMouse(5, row, tcell.Button1, 0), tview.MouseLeftClick) || a.todoIndex != index {
			t.Fatalf("row %d selected %d, want %d", row, a.todoIndex, index)
		}
		break
	}
}

func TestTodoSearchDropsCompletedResult(t *testing.T) {
	a := searchTestApp()
	a.todoSearch = todoSearchState{active: true, query: "grc", direction: 1}
	a.updateTodoSearch()
	if a.todoSearch.selectedID != "1" {
		t.Fatalf("initial result = %q, want 1", a.todoSearch.selectedID)
	}
	now := time.Now()
	a.todoStore.Items[0].Status = todo.StatusDone
	a.todoStore.Items[0].DoneAt = &now
	a.updateTodoSearch()
	if a.todoSearch.selectedID != "2" || len(a.todoSearch.results) != 1 || a.todoSearch.results[0].id != "2" {
		t.Fatalf("completion did not move to next active result: selected=%q results=%v", a.todoSearch.selectedID, a.todoSearch.results)
	}
}
func TestTodoSearchListIsolationAndContextualNew(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	store, _, err := repo.Add("groceries")
	if err != nil {
		t.Fatal(err)
	}
	a := &terminalApp{view: viewTodo, todos: repo, todoStore: store}
	a.handleGlobalKey(notes.Key{Name: "/", Rune: '/'})
	for _, r := range "grc" {
		a.handleGlobalKey(notes.Key{Name: string(r), Rune: r})
	}
	a.handleGlobalKey(notes.Key{Name: "enter"})
	a.view = viewNotes
	a.view = viewTodo
	a.updateTodoSearch()
	if a.todoSearch.query != "grc" {
		t.Fatal("navigation cleared query")
	}
	a.handleGlobalKey(notes.Key{Name: "esc"})
	a.handleGlobalKey(notes.Key{Name: "n", Rune: 'n'})
	if a.todoInputMode != "new" {
		t.Fatal("n outside search failed")
	}
	a.clearTodoInput()
	a.todoSearch = todoSearchState{active: true, query: "grc", direction: 1}
	a.updateTodoSearch()
	if _, _, err := repo.CreateList("Other"); err != nil {
		t.Fatal(err)
	}
	a.reloadTodosForRender()
	if a.todoSearch.active || a.todoSearch.query != "" {
		t.Fatal("search leaked between lists")
	}
}

func TestTodoSearchRetainsArchiveSelectionWhenEmptyMonthArrives(t *testing.T) {
	a := searchTestApp()
	archived := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	a.todoStore.Items = append(a.todoStore.Items, todo.Item{ID: "old", Text: "archived target", Status: todo.StatusArchived, ArchivedAt: &archived})
	a.todoSearch = todoSearchState{active: true, query: "atg", direction: 1}
	a.updateTodoSearch()
	a.todoStore.ArchiveMonths = []string{"2026-01"}
	a.updateTodoSearch()
	item, ok := a.selectedTodoItem()
	if !ok || item.ID != "old" {
		t.Fatalf("empty header changed selected result: %+v", item)
	}
}
