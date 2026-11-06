package notes

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kloneets/tools/src/helpers"
)

func sidebarSearchWorkspace() *Workspace {
	return &Workspace{FocusSidebar: true, Selection: 2, Tree: []TreeEntry{
		{Kind: treeOpenNote, Label: "Ābols"},
		{Kind: treeOpenNote, Label: "Other"},
		{Kind: treeOpenNote, Label: "ĀBOLS 😀"},
	}}
}

func searchSidebar(w *Workspace, query string) {
	w.HandleKey(Key{Name: "/", Rune: '/'})
	for _, r := range query {
		w.HandleKey(Key{Name: string(r), Rune: r})
	}
}

func TestSidebarSearchEditingAndNavigation(t *testing.T) {
	w := sidebarSearchWorkspace()
	searchSidebar(w, "ābols")
	if !w.IsEditableContext() || w.Selection != 0 {
		t.Fatalf("editing=%v selection=%d", w.IsEditableContext(), w.Selection)
	}
	if strings.Count(strings.Join(w.SidebarRows(4), "\n"), helpers.ANSIRoleSearch) != 2 {
		t.Fatal("missing case insensitive highlights")
	}
	w.HandleKey(Key{Name: "enter"})
	if w.sidebarSearch.editing || !w.FocusSidebar {
		t.Fatal("enter should confirm without opening")
	}
	for _, tc := range []struct {
		key  string
		want int
	}{{"n", 2}, {"n", 0}, {"N", 2}, {"N", 0}} {
		w.HandleKey(Key{Name: tc.key, Rune: []rune(tc.key)[0]})
		if w.Selection != tc.want {
			t.Fatalf("%s selection=%d want %d", tc.key, w.Selection, tc.want)
		}
	}
	w.HandleKey(Key{Name: "esc"})
	if w.sidebarSearch.active || !w.FocusSidebar {
		t.Fatal("escape should clear and retain sidebar")
	}
	w.HandleKey(Key{Name: "esc"})
	if w.FocusSidebar {
		t.Fatal("subsequent escape should leave sidebar")
	}
}

func TestSidebarSearchUnicodeBackspacePasteAndNoMatches(t *testing.T) {
	w := sidebarSearchWorkspace()
	searchSidebar(w, "ābols 😀")
	if w.Selection != 2 {
		t.Fatal("unicode title not found")
	}
	w.HandleKey(Key{Name: "backspace"})
	if w.sidebarSearch.query != "ābols " {
		t.Fatal("backspace split rune")
	}
	w.HandleKey(Key{Name: "esc"})
	if w.sidebarSearch.editing || !w.sidebarSearch.active {
		t.Fatal("escape should finish editing")
	}
	searchSidebar(w, "")
	if len(w.sidebarSearchMatches()) != 0 || strings.Contains(w.CommandLineText(200), "No matches") {
		t.Fatal("empty query should not match or warn")
	}
	if !w.HandlePaste("missing\r\nquery") || w.sidebarSearch.query != "missing query" {
		t.Fatal("paste failed")
	}
	if !strings.Contains(w.CommandLineText(200), "No matches") {
		t.Fatal("missing no-match prompt")
	}
	w.HandleKey(Key{Name: "enter"})
	if !w.HandleKey(Key{Name: "n"}) {
		t.Fatal("no match repeat should consume new-note shortcut")
	}
}

func TestSidebarSearchBrowserAndViewport(t *testing.T) {
	w := &Workspace{FocusSidebar: true, SidebarBrowsing: true, SidebarWidth: 40, BrowserTree: []TreeEntry{{Kind: treeFolder, Label: "needle"}}}
	for i := 0; i < 15; i++ {
		w.BrowserTree = append(w.BrowserTree, TreeEntry{Kind: treeNote, Label: fmt.Sprintf("Note %d", i)})
	}
	w.BrowserTree = append(w.BrowserTree, TreeEntry{Kind: treeNote, Label: "needle note"})
	searchSidebar(w, "needle")
	if w.BrowserSelection != 16 || len(w.sidebarSearchMatches()) != 1 {
		t.Fatal("search should skip folders and reach last note")
	}
	rows := w.BrowserRows(40, 5)
	if !strings.Contains(helpers.StripANSI(strings.Join(rows, "\n")), "needle note") {
		t.Fatal("selected match outside viewport")
	}
	if row, _, ok := w.SidebarCursor(); !ok || row >= 5 {
		t.Fatal("selected cursor outside viewport")
	}
	w.sidebarSearch.editing = false
	w.HandleKey(Key{Name: "e"})
	if w.sidebarSearch.active || w.SidebarBrowsing {
		t.Fatal("browser toggle should reset search")
	}
}

func TestSidebarMatchSpans(t *testing.T) {
	for _, tc := range []struct {
		text, query string
		count       int
	}{{"Āā 😀 Āā", "āĀ", 2}, {"Other", "", 0}, {"ababa", "aba", 1}, {"hello", "no", 0}} {
		if spans := sidebarMatchSpans(tc.text, tc.query); len(spans) != tc.count {
			t.Fatalf("%q/%q=%v", tc.text, tc.query, spans)
		}
	}
}

func TestSidebarSearchHighlightsTruncatedTitles(t *testing.T) {
	w := &Workspace{FocusSidebar: true, SidebarWidth: 20, Tree: []TreeEntry{{Kind: treeOpenNote, Label: "needle and a very long note title"}}}
	searchSidebar(w, "needle")
	row := w.SidebarRows(2)[1]
	if !strings.Contains(row, helpers.ANSIRoleSearch) || len([]rune(helpers.StripANSI(row))) > 20 {
		t.Fatalf("truncated highlight=%q", row)
	}
}

func TestSidebarSearchYieldsToBrowserCommands(t *testing.T) {
	for _, tc := range []struct{ key, prompt string }{{"m", "move "}, {"r", "rename needle"}} {
		t.Run(tc.key, func(t *testing.T) {
			w := &Workspace{FocusSidebar: true, SidebarBrowsing: true, BrowserTree: []TreeEntry{{Kind: treeNote, Label: "needle", Path: "/tmp/needle.md"}}}
			searchSidebar(w, "needle")
			w.HandleKey(Key{Name: "enter"})
			w.HandleKey(Key{Name: tc.key, Rune: []rune(tc.key)[0]})
			if w.sidebarSearch.active || !w.BrowserCommandMode || w.CommandLineText(100) != tc.prompt {
				t.Fatalf("search=%+v command=%t prompt=%q", w.sidebarSearch, w.BrowserCommandMode, w.CommandLineText(100))
			}
			w.HandleKey(Key{Name: "x", Rune: 'x'})
			if got := w.CommandLineText(100); got != tc.prompt+"x" {
				t.Fatalf("edited command prompt=%q", got)
			}
		})
	}
}
