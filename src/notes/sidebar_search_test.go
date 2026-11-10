package notes

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
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
	}{{"n", 1}, {"n", 0}, {"N", 1}, {"N", 0}} {
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
	if w.Selection != 0 {
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
	if w.BrowserSelection != 0 || len(w.sidebarSearchMatches()) != 1 {
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

func TestSidebarFuzzyMatchSpans(t *testing.T) {
	for _, tc := range []struct {
		label, query string
		want         []markdownSpan
	}{
		{"Note list", "ntls", []markdownSpan{{Tag: tagSearch, Start: 0, End: 1}, {Tag: tagSearch, Start: 2, End: 3}, {Tag: tagSearch, Start: 5, End: 6}, {Tag: tagSearch, Start: 7, End: 8}}},
		{"Ābolu saraksts", "Āsr", []markdownSpan{{Tag: tagSearch, Start: 0, End: 1}, {Tag: tagSearch, Start: 6, End: 7}, {Tag: tagSearch, Start: 8, End: 9}}},
		{"Note list", "ltn", nil}, {"Note list", "nn", nil},
	} {
		if got := sidebarMatchSpans(tc.label, tc.query); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q/%q = %#v, want %#v", tc.label, tc.query, got, tc.want)
		}
	}
}

func nestedSidebarSearchWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	target := filepath.Join(notesDir(), "Projects", "Deep", "Note list.md")
	for _, path := range []string{target, filepath.Join(notesDir(), "Projects", "Deep", "Unrelated.md"), filepath.Join(notesDir(), "Other", "Hidden.md")} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("text"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	w := &Workspace{FocusSidebar: true, SidebarBrowsing: true, SidebarWidth: 40}
	w.refreshBrowserTree()
	for i := range w.BrowserTree {
		if w.BrowserTree[i].Kind == treeFolder {
			w.BrowserTree[i].Collapsed = true
		}
	}
	w.refreshBrowserTree()
	return w, target
}

func TestSidebarSearchExpandsNestedMatchesAndRestoresCollapse(t *testing.T) {
	w, target := nestedSidebarSearchWorkspace(t)
	ordinary := append([]TreeEntry(nil), w.BrowserTree...)
	searchSidebar(w, "ntls")
	if len(w.BrowserTree) != 3 {
		t.Fatalf("filtered tree = %#v", w.BrowserTree)
	}
	for _, entry := range w.BrowserTree {
		if entry.Kind == treeFolder && entry.Collapsed {
			t.Fatalf("matching ancestor collapsed: %#v", entry)
		}
		if entry.Label == "Other" || entry.Label == "Unrelated" {
			t.Fatalf("unrelated entry visible: %#v", entry)
		}
	}
	if entry := w.selectedBrowserEntry(); entry == nil || entry.Path != target {
		t.Fatalf("selected = %#v", entry)
	}
	// Refreshing while searching must retain the complete source and collapse snapshot.
	w.refreshBrowserTree()
	if len(w.BrowserTree) != 3 {
		t.Fatalf("refresh lost results: %#v", w.BrowserTree)
	}
	w.HandleKey(Key{Name: "enter"})
	w.HandleKey(Key{Name: "esc"})
	if !reflect.DeepEqual(w.BrowserTree, ordinary) {
		t.Fatalf("collapse state changed: %#v, want %#v", w.BrowserTree, ordinary)
	}
}

func TestSidebarSearchPasteBackspaceAndEmptyView(t *testing.T) {
	w, _ := nestedSidebarSearchWorkspace(t)
	ordinary := append([]TreeEntry(nil), w.BrowserTree...)
	searchSidebar(w, "")
	w.HandlePaste("ntls")
	if len(w.sidebarSearchMatches()) != 1 {
		t.Fatal("paste did not filter")
	}
	w.HandleKey(Key{Name: "x", Rune: 'x'})
	if len(w.BrowserTree) != 0 || !strings.Contains(w.sidebarSearchPrompt(), "No matches") {
		t.Fatalf("unmatched tree = %#v", w.BrowserTree)
	}
	w.HandleKey(Key{Name: "backspace"})
	if len(w.sidebarSearchMatches()) != 1 {
		t.Fatal("backspace did not restore result")
	}
	for range 4 {
		w.HandleKey(Key{Name: "backspace"})
	}
	if !reflect.DeepEqual(w.BrowserTree, ordinary) {
		t.Fatal("empty query did not restore ordinary tree")
	}
}

func TestSidebarSearchOpensNestedFilteredSelection(t *testing.T) {
	w, target := nestedSidebarSearchWorkspace(t)
	searchSidebar(w, "ntls")
	w.HandleKey(Key{Name: "enter"})
	w.HandleKey(Key{Name: "enter"})
	if w.ActiveEditor() == nil || w.ActiveEditor().Path != target || w.FocusSidebar {
		t.Fatalf("wrong note opened: %#v", w.ActiveEditor())
	}
}

func TestSidebarSearchRenamesNestedFilteredSelection(t *testing.T) {
	w, target := nestedSidebarSearchWorkspace(t)
	searchSidebar(w, "ntls")
	w.HandleKey(Key{Name: "enter"})
	w.HandleKey(Key{Name: "r", Rune: 'r'})
	if entry := w.selectedBrowserEntry(); entry == nil || entry.Path != target {
		t.Fatalf("rename lost target: %#v", entry)
	}
	w.BrowserCommand = "rename Renamed"
	w.HandleKey(Key{Name: "enter"})
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), "Renamed.md")); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("old target still present: %v", err)
	}
}

func TestSidebarSearchOpenListFiltersAndRestoresSelectedIdentity(t *testing.T) {
	w := sidebarSearchWorkspace()
	w.Tree[0].Path = "/tmp/a.md"
	w.Tree[1].Path = "/tmp/b.md"
	w.Tree[2].Path = "/tmp/c.md"
	searchSidebar(w, "ābs")
	if len(w.Tree) != 2 {
		t.Fatalf("filtered open list = %#v", w.Tree)
	}
	w.HandleKey(Key{Name: "enter"})
	w.HandleKey(Key{Name: "n", Rune: 'n'})
	if entry := w.selectedOpenEntry(); entry == nil || entry.Path != "/tmp/c.md" {
		t.Fatalf("wrong filtered selection = %#v", entry)
	}
	w.HandleKey(Key{Name: "esc"})
	if len(w.Tree) != 3 || w.Selection != 2 {
		t.Fatalf("restored list/selection = %#v/%d", w.Tree, w.Selection)
	}
}

func TestSidebarSearchBrowserExitRestoresOrdinaryTree(t *testing.T) {
	w, _ := nestedSidebarSearchWorkspace(t)
	ordinary := append([]TreeEntry(nil), w.BrowserTree...)
	searchSidebar(w, "ntls")
	w.HandleKey(Key{Name: "enter"})
	w.HandleKey(Key{Name: "h", Rune: 'h'})
	if w.SidebarBrowsing || w.sidebarSearch.active || !reflect.DeepEqual(w.BrowserTree, ordinary) {
		t.Fatalf("browser exit retained search: state=%+v tree=%#v", w.sidebarSearch, w.BrowserTree)
	}
}

func TestSidebarSearchMouseSelectionUsesScrolledFilteredRows(t *testing.T) {
	for _, browser := range []bool{false, true} {
		t.Run(fmt.Sprintf("browser=%t", browser), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings.Init()
			if err := os.MkdirAll(notesDir(), 0755); err != nil {
				t.Fatal(err)
			}
			w := &Workspace{FocusSidebar: true, SidebarBrowsing: browser}
			kind := treeOpenNote
			if browser {
				kind = treeNote
			}
			var entries []TreeEntry
			for i := range 20 {
				entries = append(entries, TreeEntry{Kind: kind, Label: "Unrelated", Path: fmt.Sprintf("/tmp/other%d.md", i)})
				path := filepath.Join(notesDir(), fmt.Sprintf("note%d.md", i))
				if err := os.WriteFile(path, []byte("text"), 0644); err != nil {
					t.Fatal(err)
				}
				entries = append(entries, TreeEntry{Kind: kind, Label: fmt.Sprintf("Note list %02d", i), Path: path})
			}
			if browser {
				w.BrowserTree = entries
			} else {
				w.Tree = entries
			}
			searchSidebar(w, "ntls")
			w.HandleKey(Key{Name: "enter"})
			for range 19 {
				w.HandleKey(Key{Name: "n", Rune: 'n'})
			}
			if browser {
				w.BrowserRows(40, 5)
			} else {
				w.SidebarRows(5)
			}
			if !w.SelectSidebarRow(1, false) {
				t.Fatal("mouse selection not handled")
			}
			if entry := w.selectedEntry(); entry == nil || entry.Path != filepath.Join(notesDir(), "note16.md") {
				t.Fatalf("clicked first visible row selected %#v, want note16", entry)
			}
			// TUI mouse down selects and redraws before click on release and double-click.
			if browser {
				w.BrowserRows(40, 5)
			} else {
				w.SidebarRows(5)
			}
			w.SelectSidebarRow(1, false)
			if entry := w.selectedEntry(); entry == nil || entry.Path != filepath.Join(notesDir(), "note16.md") {
				t.Fatalf("mouse release shifted selection: %#v", entry)
			}
			if browser {
				w.BrowserRows(40, 5)
			} else {
				w.SidebarRows(5)
			}
			w.SelectSidebarRow(1, true)
			if w.ActiveEditor() == nil || w.ActiveEditor().Path != filepath.Join(notesDir(), "note16.md") {
				t.Fatalf("double-click opened wrong note: %#v", w.ActiveEditor())
			}
		})
	}
}
