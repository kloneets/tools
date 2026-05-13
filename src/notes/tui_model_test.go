package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

func TestNormalizeSidebarWidth(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 28},
		{-1, 28},
		{28, 28},
		{396, 28},
		{80, 80},
	}
	for _, tc := range cases {
		if got := normalizeSidebarWidth(tc.in); got != tc.want {
			t.Fatalf("normalizeSidebarWidth(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestResolveEditorWidth(t *testing.T) {
	if got := resolveEditorWidth(0, 100); got != 49 {
		t.Fatalf("resolveEditorWidth(0, 100) = %d, want 49", got)
	}
	if got := resolveEditorWidth(396, 100); got != 49 {
		t.Fatalf("resolveEditorWidth(396, 100) = %d, want 49", got)
	}
	if got := resolveEditorWidth(12, 100); got != 12 {
		t.Fatalf("resolveEditorWidth(12, 100) = %d, want 12", got)
	}
}

func TestRenderUsesASCIITreeMarkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tree: []TreeEntry{
			{Kind: treeFolder, Label: "Work", Folder: "Work"},
			{Kind: treeNote, Label: "Plan", Path: "/tmp/Plan.md", Folder: "Work", Depth: 1},
		},
		Selection:    0,
		SidebarWidth: 28,
		Tabs: []*Editor{{
			Title: "Plan",
			Text:  "",
			Mode:  ModeNormal,
		}},
		CurrentTab: 0,
	}
	got := w.Render(80, 10)
	for _, bad := range []string{"▾", "▸", "•", "│", "─", "…"} {
		if strings.Contains(got, bad) {
			t.Fatalf("Render() contains non-ASCII marker %q in %q", bad, got)
		}
	}
}

func TestRenderPreviewIncludesANSIStylingForCode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs: []*Editor{{
			Title: "Plan",
			Text:  "```go\nfunc main() { return }\n```",
			Mode:  ModeNormal,
		}},
		CurrentTab: 0,
	}
	got := strings.Join(renderPreviewPane(w.Tabs[0], 80, 5), "\n")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("renderPreviewPane() = %q, want ANSI styling", got)
	}
}

func TestRenderEditorIncludesANSIStylingForUnclosedCodeFence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Title: "Plan",
		Text:  "```go\nfunc main() {\n\treturn\n}",
		Mode:  ModeNormal,
	}
	got := strings.Join(renderEditorPane(ed, 80, 6), "\n")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("renderEditorPane() = %q, want ANSI styling for editor code block", got)
	}
}

func TestHelpTextIncludesNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{}
	got := w.HelpText()
	if !strings.Contains(got, "ctrl+a") {
		t.Fatalf("HelpText() = %q, want sidebar navigation help", got)
	}
	w.Tabs = []*Editor{{Text: "alpha", Mode: ModeNormal}}
	w.CurrentTab = 0
	if got := w.HelpText(); !strings.Contains(got, "ctrl+g/:spell spelling") {
		t.Fatalf("HelpText() = %q, want spell shortcut and command help", got)
	}
}

func findTreeEntryIndex(t *testing.T, entries []TreeEntry, kind treeEntryKind, path string, label string) int {
	t.Helper()
	for i, entry := range entries {
		if entry.Kind != kind {
			continue
		}
		if path != "" && entry.Path != path {
			continue
		}
		if label != "" && entry.Label != label {
			continue
		}
		return i
	}
	t.Fatalf("could not find tree entry kind=%d path=%q label=%q", kind, path, label)
	return -1
}

func typeWorkspaceText(t *testing.T, w *Workspace, text string) {
	t.Helper()
	for _, r := range text {
		if !w.HandleKey(Key{Name: string(r), Rune: r}) {
			t.Fatalf("HandleKey(%q) = false, want true", string(r))
		}
	}
}

func TestSidebarRowsShowOnlyOpenNotesByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	path := w.ActiveEditor().Path
	rows := strings.Join(w.SidebarRows(12), "\n")
	if strings.Contains(rows, "All notes") || strings.Contains(rows, "Open notes") {
		t.Fatalf("SidebarRows() = %q, want no split sidebar sections", rows)
	}
	if findTreeEntryIndex(t, w.Tree, treeOpenNote, path, "") < 0 {
		t.Fatal("expected open-note entry in sidebar")
	}
	if strings.Contains(rows, noteTitleFromPath(path)) == false {
		t.Fatalf("SidebarRows() = %q, want open note visible", rows)
	}
}

func TestBrowserRowsScrollToSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if !w.NewNote() {
			t.Fatalf("NewNote() #%d = false, want true", i+1)
		}
	}
	lastPath := w.ActiveEditor().Path
	w.FocusSidebar = true
	w.toggleSidebarBrowser()
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, lastPath, "")
	w.LastHeight = 6
	rows := strings.Join(w.BrowserRows(80, 5), "\n")
	if !strings.Contains(rows, noteTitleFromPath(lastPath)) {
		t.Fatalf("SidebarRows() = %q, want selected note visible", rows)
	}
	row, _, ok := w.SidebarCursor()
	if !ok {
		t.Fatal("SidebarCursor() ok = false, want true")
	}
	if row <= 0 || row >= 5 {
		t.Fatalf("SidebarCursor() row = %d, want visible sidebar row", row)
	}
}

func TestSidebarETogglesBrowserScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	if !w.HandleKey(Key{Name: "e", Rune: 'e'}) {
		t.Fatal("HandleKey(e) = false, want true")
	}
	if !w.SidebarBrowsing {
		t.Fatal("SidebarBrowsing = false, want browser open")
	}
	browserRows := strings.Join(w.BrowserRows(80, 8), "\n")
	if !strings.Contains(browserRows, "Notes Browser") {
		t.Fatalf("BrowserRows() = %q, want browser title", browserRows)
	}
	sidebarRows := strings.Join(w.SidebarRows(8), "\n")
	if strings.Contains(sidebarRows, "Notes Browser") {
		t.Fatalf("SidebarRows() = %q, want normal open-note sidebar", sidebarRows)
	}
	if !w.HandleKey(Key{Name: "e", Rune: 'e'}) {
		t.Fatal("HandleKey(e) second = false, want true")
	}
	if w.SidebarBrowsing {
		t.Fatal("SidebarBrowsing = true, want browser closed")
	}
}

func TestBrowserCommandCreatesNoteAndReturnsToEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	if !w.HandleKey(Key{Name: "e", Rune: 'e'}) {
		t.Fatal("HandleKey(e) = false, want browser open")
	}
	if !w.HandleKey(Key{Name: "n", Rune: 'n'}) {
		t.Fatal("HandleKey(n) = false, want browser command")
	}
	if !w.BrowserCommandMode || w.BrowserCommand != "new " {
		t.Fatalf("browser command mode=%t command=%q, want new prompt", w.BrowserCommandMode, w.BrowserCommand)
	}
	typeWorkspaceText(t, w, "Plan")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want create note")
	}
	if w.SidebarBrowsing || w.FocusSidebar {
		t.Fatalf("browser/focus = %t/%t, want editor after note create", w.SidebarBrowsing, w.FocusSidebar)
	}
	if got := w.ActiveEditor(); got == nil || got.Title != "Plan" {
		t.Fatalf("active editor = %#v, want Plan note", got)
	}
	if _, err := os.Stat(filepath.Join(notesDir(), "Plan.md")); err != nil {
		t.Fatalf("created note missing: %v", err)
	}
}

func TestBrowserCommandTrailingSlashCreatesFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	if !w.HandleKey(Key{Name: "e", Rune: 'e'}) {
		t.Fatal("HandleKey(e) = false, want browser open")
	}
	if !w.HandleKey(Key{Name: "f", Rune: 'f'}) {
		t.Fatal("HandleKey(f) = false, want browser command")
	}
	typeWorkspaceText(t, w, "Projects/")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want create folder")
	}
	if !w.SidebarBrowsing || !w.FocusSidebar {
		t.Fatalf("browser/focus = %t/%t, want stay in browser", w.SidebarBrowsing, w.FocusSidebar)
	}
	if _, err := os.Stat(noteFolderPath("Projects")); err != nil {
		t.Fatalf("created folder missing: %v", err)
	}
	if got := w.selectedBrowserEntry(); got == nil || got.Kind != treeFolder || got.Folder != "Projects" {
		t.Fatalf("selected browser entry = %#v, want Projects folder", got)
	}
}

func TestBrowserCommandCreatesRelativeAndRootTargets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	if !w.HandleKey(Key{Name: "n", Rune: 'n'}) {
		t.Fatal("HandleKey(n) = false, want browser command")
	}
	typeWorkspaceText(t, w, "Plan")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want create relative note")
	}
	if _, err := os.Stat(filepath.Join(noteFolderPath("Projects"), "Plan.md")); err != nil {
		t.Fatalf("relative note missing: %v", err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	if !w.HandleKey(Key{Name: "n", Rune: 'n'}) {
		t.Fatal("HandleKey(n root) = false, want browser command")
	}
	typeWorkspaceText(t, w, "/Root")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter root) = false, want create root note")
	}
	if _, err := os.Stat(filepath.Join(notesDir(), "Root.md")); err != nil {
		t.Fatalf("root note missing: %v", err)
	}
}

func TestBrowserDeleteQueuesNoteAndFolderTargets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	notePath := w.ActiveEditor().Path
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, notePath, "")
	if !w.HandleKey(Key{Name: "d", Rune: 'd'}) {
		t.Fatal("HandleKey(d note) = false, want pending delete")
	}
	path, label, folder, ok := w.TakePendingDeleteTarget()
	if !ok || folder || path != notePath || label != noteTitleFromPath(notePath) {
		t.Fatalf("pending note delete = path:%q label:%q folder:%t ok:%t", path, label, folder, ok)
	}
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	if !w.HandleKey(Key{Name: "d", Rune: 'd'}) {
		t.Fatal("HandleKey(d folder) = false, want pending delete")
	}
	path, label, folder, ok = w.TakePendingDeleteTarget()
	if !ok || !folder || path != "Projects" || label != "Projects" {
		t.Fatalf("pending folder delete = path:%q label:%q folder:%t ok:%t", path, label, folder, ok)
	}
}

func TestDeleteFolderByRelRemovesFolderAndOpenTabs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	created, err := w.CreateNote("Plan")
	if err != nil {
		t.Fatal(err)
	}
	if relativeNoteFolder(created) != "Projects" {
		t.Fatalf("created folder = %q, want Projects", relativeNoteFolder(created))
	}
	if err := w.DeleteFolderByRel("Projects"); err != nil {
		t.Fatalf("DeleteFolderByRel() error = %v", err)
	}
	if _, err := os.Stat(noteFolderPath("Projects")); !os.IsNotExist(err) {
		t.Fatalf("Projects folder stat err = %v, want not exist", err)
	}
	for _, tab := range w.Tabs {
		if strings.HasPrefix(relativeNoteFolder(tab.Path), "Projects") {
			t.Fatalf("tab under deleted folder still open: %#v", tab)
		}
	}
}

func TestBrowserRenameNoteRenamesFileAssetsAndOpenTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	oldPath := w.ActiveEditor().Path
	oldAssets := noteAssetsPath(oldPath)
	if err := os.MkdirAll(oldAssets, 0o755); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, oldPath, "")
	if !w.HandleKey(Key{Name: "r", Rune: 'r'}) {
		t.Fatal("HandleKey(r) = false, want rename command")
	}
	if !strings.HasPrefix(w.BrowserCommand, "rename ") {
		t.Fatalf("BrowserCommand = %q, want rename prompt", w.BrowserCommand)
	}
	for !strings.HasSuffix(w.BrowserCommand, "rename ") {
		if !w.HandleKey(Key{Name: "backspace"}) {
			t.Fatal("HandleKey(backspace) = false, want edit rename command")
		}
	}
	typeWorkspaceText(t, w, "Renamed")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want rename")
	}
	newPath := filepath.Join(notesDir(), "Renamed.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("renamed note missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old note stat err = %v, want missing", err)
	}
	if got := w.ActiveEditor(); got == nil || got.Path != newPath || got.Title != "Renamed" {
		t.Fatalf("active editor = %#v, want renamed note", got)
	}
	if _, err := os.Stat(noteAssetsPath(newPath)); err != nil {
		t.Fatalf("renamed assets missing: %v", err)
	}
}

func TestBrowserRenameFolderUpdatesOpenTabs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	created, err := w.CreateNote("Plan")
	if err != nil {
		t.Fatal(err)
	}
	if relativeNoteFolder(created) != "Projects" {
		t.Fatalf("created folder = %q, want Projects", relativeNoteFolder(created))
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	if err := w.RenameBrowserEntry("Archive"); err != nil {
		t.Fatalf("RenameBrowserEntry() error = %v", err)
	}
	newPath := filepath.Join(noteFolderPath("Archive"), "Plan.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("renamed folder note missing: %v", err)
	}
	if got := w.ActiveEditor(); got == nil || got.Path != newPath {
		t.Fatalf("active editor = %#v, want path %q", got, newPath)
	}
	if got := w.selectedBrowserEntry(); got == nil || got.Kind != treeFolder || got.Folder != "Archive" {
		t.Fatalf("selected browser entry = %#v, want Archive folder", got)
	}
}

func TestBrowserMoveStartsCommandForNotesAndFoldersOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, w.ActiveEditor().Path, "")
	if !w.HandleKey(Key{Name: "m", Rune: 'm'}) {
		t.Fatal("HandleKey(m note) = false, want move command")
	}
	if !w.BrowserCommandMode || w.BrowserCommand != "move " {
		t.Fatalf("BrowserCommand = mode:%t command:%q, want move prompt", w.BrowserCommandMode, w.BrowserCommand)
	}

	w.BrowserCommandMode = false
	w.BrowserCommand = ""
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	if !w.HandleKey(Key{Name: "m", Rune: 'm'}) {
		t.Fatal("HandleKey(m folder) = false, want move command")
	}

	w.BrowserCommandMode = false
	w.BrowserCommand = ""
	w.BrowserTree = []TreeEntry{{Kind: treeManagedAsset, Path: "/tmp/image.png", Label: "image.png"}}
	w.BrowserSelection = 0
	if w.HandleKey(Key{Name: "m", Rune: 'm'}) {
		t.Fatal("HandleKey(m managed asset) = true, want false")
	}
	if w.BrowserCommandMode || w.BrowserCommand != "" {
		t.Fatalf("BrowserCommand = mode:%t command:%q, want unchanged", w.BrowserCommandMode, w.BrowserCommand)
	}
}

func TestBrowserMoveNoteMovesAssetsOpenTabAndRenames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	oldPath := w.ActiveEditor().Path
	oldAssets := noteAssetsPath(oldPath)
	if err := os.MkdirAll(oldAssets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldAssets, "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, oldPath, "")
	if err := w.MoveBrowserEntry("Projects/TODO.md"); err != nil {
		t.Fatalf("MoveBrowserEntry() error = %v", err)
	}

	newPath := filepath.Join(noteFolderPath("Projects"), "TODO.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("moved note missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old note stat err = %v, want missing", err)
	}
	if _, err := os.Stat(filepath.Join(noteAssetsPath(newPath), "diagram.png")); err != nil {
		t.Fatalf("moved asset missing: %v", err)
	}
	if got := w.ActiveEditor(); got == nil || got.Path != newPath || got.Title != "TODO" {
		t.Fatalf("active editor = %#v, want moved TODO note", got)
	}
}

func TestBrowserMoveNoteCommandCreatesNestedFolderAndKeepsTitle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	oldPath := w.ActiveEditor().Path
	title := w.ActiveEditor().Title
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, oldPath, "")
	if !w.HandleKey(Key{Name: "m", Rune: 'm'}) {
		t.Fatal("HandleKey(m) = false, want move command")
	}
	typeWorkspaceText(t, w, "Projects/Archive/")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want execute move")
	}
	newPath := filepath.Join(noteFolderPath(filepath.Join("Projects", "Archive")), title+".md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("moved note missing: %v", err)
	}
}

func TestBrowserMoveFolderUpdatesOpenTabs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	created, err := w.CreateNote("Plan")
	if err != nil {
		t.Fatal(err)
	}
	if relativeNoteFolder(created) != "Projects" {
		t.Fatalf("created folder = %q, want Projects", relativeNoteFolder(created))
	}
	if err := os.MkdirAll(noteFolderPath("Archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	w.refreshTree()

	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	if err := w.MoveBrowserEntry("Archive"); err != nil {
		t.Fatalf("MoveBrowserEntry() error = %v", err)
	}

	newPath := filepath.Join(noteFolderPath(filepath.Join("Archive", "Projects")), "Plan.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("moved folder note missing: %v", err)
	}
	if _, err := os.Stat(noteFolderPath("Projects")); !os.IsNotExist(err) {
		t.Fatalf("old folder stat err = %v, want missing", err)
	}
	if got := w.ActiveEditor(); got == nil || got.Path != newPath {
		t.Fatalf("active editor = %#v, want path %q", got, newPath)
	}
}

func TestBrowserMoveFolderRejectsDescendantTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFolder("Projects/Archive"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")
	if err := w.MoveBrowserEntry("Projects/Archive"); err == nil {
		t.Fatal("MoveBrowserEntry() error = nil, want descendant target rejected")
	}
	if _, err := os.Stat(noteFolderPath("Projects")); err != nil {
		t.Fatalf("Projects folder missing after rejected move: %v", err)
	}
}

func TestBrowserRowsShowManagedFilesUnderNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	notePath := w.ActiveEditor().Path
	assetDir := noteAssetsPath(notePath)
	if err := os.MkdirAll(filepath.Join(assetDir, "Images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "Images", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.refreshTree()
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	rows := strings.Join(w.BrowserRows(120, 20), "\n")
	if !strings.Contains(rows, "Images") || !strings.Contains(rows, "diagram.png") {
		t.Fatalf("BrowserRows() = %q, want managed folder and asset", rows)
	}
}

func TestBrowserEnterTogglesManagedFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	assetDir := noteAssetsPath(w.ActiveEditor().Path)
	if err := os.MkdirAll(filepath.Join(assetDir, "Images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "Images", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.refreshTree()
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeManagedFolder, filepath.Join(assetDir, "Images"), "")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want toggle managed folder")
	}
	rows := strings.Join(w.BrowserRows(120, 20), "\n")
	if strings.Contains(rows, "diagram.png") {
		t.Fatalf("BrowserRows() = %q, want collapsed managed asset hidden", rows)
	}
}

func TestBrowserOOpensAllNotesInsideSelectedFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(noteFolderPath("Projects"), "Plan.md")
	deepPath := filepath.Join(noteFolderPath(filepath.Join("Projects", "Archive")), "Deep.md")
	outsidePath := filepath.Join(notesDir(), "Outside.md")
	if err := os.MkdirAll(filepath.Dir(deepPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deepPath, []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.refreshTree()
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")

	if !w.HandleKey(Key{Name: "o", Rune: 'o'}) {
		t.Fatal("HandleKey(o) = false, want open folder notes")
	}
	if w.SidebarBrowsing || w.FocusSidebar {
		t.Fatalf("browser/focus = %t/%t, want editor focused after opening folder notes", w.SidebarBrowsing, w.FocusSidebar)
	}
	if !workspaceHasOpenTab(w, planPath) || !workspaceHasOpenTab(w, deepPath) {
		t.Fatalf("tabs = %#v, want folder notes opened", w.Tabs)
	}
	if workspaceHasOpenTab(w, outsidePath) {
		t.Fatalf("tabs = %#v, want outside note not opened", w.Tabs)
	}
	if got := w.ActiveEditor(); got == nil || got.Path != planPath {
		t.Fatalf("active editor = %#v, want last opened note %q", got, planPath)
	}
}

func TestBrowserOFolderOpenDoesNotDuplicateExistingTabs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(noteFolderPath("Projects"), "Plan.md")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Open(planPath); err != nil {
		t.Fatal(err)
	}
	before := len(w.Tabs)
	w.refreshTree()
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeFolder, "", "Projects")

	if !w.HandleKey(Key{Name: "o", Rune: 'o'}) {
		t.Fatal("HandleKey(o) = false, want open folder notes")
	}
	if len(w.Tabs) != before {
		t.Fatalf("tab count = %d, want %d without duplicate", len(w.Tabs), before)
	}
}

func TestBrowserOIgnoresNonFolderRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, w.ActiveEditor().Path, "")
	if w.HandleKey(Key{Name: "o", Rune: 'o'}) {
		t.Fatal("HandleKey(o note) = true, want false")
	}
}

func TestCommandLineTextReflectsCommandAndSearch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs: []*Editor{{
			Title:      "Plan",
			Text:       "alpha beta alpha",
			Mode:       ModeCommand,
			Command:    "/alpha",
			LastSearch: "alpha",
		}},
		CurrentTab: 0,
	}
	if got := w.CommandLineText(80); got != "/alpha" {
		t.Fatalf("CommandLineText() = %q, want command contents", got)
	}
	w.Tabs[0].Mode = ModeNormal
	if got := w.CommandLineText(80); got != "/alpha" {
		t.Fatalf("CommandLineText() = %q, want last search query", got)
	}
}

func TestStyleForMarkdownTagUsesDistinctHeadingColors(t *testing.T) {
	h1 := styleForMarkdownTag(tagHeading1, "one")
	h2 := styleForMarkdownTag(tagHeading2, "two")
	h3 := styleForMarkdownTag(tagHeading3, "three")
	h4 := styleForMarkdownTag(tagHeading4, "four")
	h5 := styleForMarkdownTag(tagHeading5, "five")
	h6 := styleForMarkdownTag(tagHeading6, "six")
	if h1 == h2 || h2 == h3 || h1 == h3 || h3 == h4 || h4 == h5 || h5 == h6 || h4 == h6 {
		t.Fatalf("heading styles should differ: h1=%q h2=%q h3=%q h4=%q h5=%q h6=%q", h1, h2, h3, h4, h5, h6)
	}
	list := styleForMarkdownTag(tagList, "item")
	for _, heading := range []string{h1, h2, h3, h4, h5, h6} {
		if list == heading {
			t.Fatalf("list style should differ from headings: list=%q heading=%q", list, heading)
		}
	}
}

func TestApplyANSIMarkdownPrefersTokenColorsOverCodeBlockSpan(t *testing.T) {
	line := "func main()"
	spans := []markdownSpan{
		{Tag: tagCodeBlock, Start: 0, End: len([]rune(line))},
		{Tag: tagCodeKeyword, Start: 0, End: 4},
		{Tag: tagCodeFunction, Start: 5, End: 9},
	}
	got := applyANSIMarkdown(line, spans)
	if !strings.Contains(got, helpers.ANSIRoleKeyword) && !strings.Contains(got, helpers.ANSIRoleFunction) {
		t.Fatalf("applyANSIMarkdown() = %q, want token styling inside code block", got)
	}
}

func TestApplyANSIMarkdownPrefersInlineCodeInsideListSpan(t *testing.T) {
	line := "- use `code` here"
	spans := []markdownSpan{
		{Tag: tagList, Start: 0, End: len([]rune(line))},
		{Tag: tagCode, Start: 6, End: 12},
	}
	got := applyANSIMarkdown(line, spans)
	if !strings.Contains(got, helpers.ANSIRoleCode) {
		t.Fatalf("applyANSIMarkdown() = %q, want inline code styling inside list line", got)
	}
}

func TestEditorRenderSpansHighlightOnlyMarkdownListMarkers(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		tag       string
		wantStart int
		wantEnd   int
	}{
		{name: "dash bullet", text: "- item", tag: tagList, wantStart: 0, wantEnd: 1},
		{name: "star bullet", text: "* item", tag: tagList, wantStart: 0, wantEnd: 1},
		{name: "plus bullet", text: "+ item", tag: tagList, wantStart: 0, wantEnd: 1},
		{name: "ordered", text: "23. item", tag: tagOrdered, wantStart: 0, wantEnd: 3},
		{name: "unchecked checklist", text: "- [ ] item", tag: tagChecklist, wantStart: 0, wantEnd: 5},
		{name: "checked checklist", text: "- [x] item", tag: tagChecklist, wantStart: 0, wantEnd: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := editorRenderSpans(tt.text, 4)
			assertHasSpan(t, spans, tt.tag, tt.wantStart, tt.wantEnd)
			for _, span := range spans {
				if span.Tag == tt.tag && span.Start == tt.wantStart && span.End > tt.wantEnd {
					t.Fatalf("editorRenderSpans(%q) span = %#v, want marker-only end %d", tt.text, span, tt.wantEnd)
				}
			}
		})
	}
}

func TestApplyANSIMarkdownStylesOnlyListMarkerBodyNormally(t *testing.T) {
	line := "- item"
	got := applyANSIMarkdown(line, []markdownSpan{{Tag: tagList, Start: 0, End: 1}})
	wantPrefix := helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleListMarker, "-") + " item"
	if got != wantPrefix {
		t.Fatalf("applyANSIMarkdown() = %q, want marker-only styling %q", got, wantPrefix)
	}
}

func TestCursorPositionForEditorFocus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		SidebarWidth: 28,
		Tabs: []*Editor{{
			Text:   "abc\ndef",
			Cursor: 5,
			Mode:   ModeInsert,
		}},
		CurrentTab: 0,
	}
	row, col, ok := w.CursorPosition(120)
	if !ok {
		t.Fatal("CursorPosition() ok = false, want true")
	}
	if row != 4 {
		t.Fatalf("CursorPosition() row = %d, want 4", row)
	}
	if col <= 30 {
		t.Fatalf("CursorPosition() col = %d, want editor column beyond sidebar", col)
	}
}

func TestEnsureEditorVisibleTracksScrollTop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		LastHeight: 5,
		Tabs: []*Editor{{
			Text: strings.Join([]string{
				"one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
			}, "\n"),
			Cursor: len([]rune("one\ntwo\nthree\nfour\nfive\nsix\nseven")),
			Mode:   ModeInsert,
		}},
		CurrentTab: 0,
	}
	w.ensureEditorVisible()
	if w.ActiveEditor().ScrollTop == 0 {
		t.Fatal("ScrollTop = 0, want scrolled viewport")
	}
}

func TestEnsureEditorVisibleUsesViewportBoundaries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorHeight: 4,
		Tabs: []*Editor{{
			Text:      "one\ntwo\nthree\nfour\nfive\nsix\nseven",
			ScrollTop: 2,
			Cursor:    len([]rune("one\ntwo\nthree\nfour\nfive")),
			Mode:      ModeInsert,
		}},
		CurrentTab: 0,
	}
	w.ensureEditorVisible()
	if got := w.ActiveEditor().ScrollTop; got != 2 {
		t.Fatalf("ScrollTop = %d, want 2 when cursor is still on last visible line", got)
	}
	w.ActiveEditor().Cursor = len([]rune("one\ntwo\nthree\nfour\nfive\nsix\nseven"))
	w.ensureEditorVisible()
	if got := w.ActiveEditor().ScrollTop; got != 3 {
		t.Fatalf("ScrollTop = %d, want 3 after moving below viewport", got)
	}
	w.ActiveEditor().ScrollTop = 2
	w.ActiveEditor().Cursor = len([]rune("one\ntwo\nthree"))
	w.ensureEditorVisible()
	if got := w.ActiveEditor().ScrollTop; got != 2 {
		t.Fatalf("ScrollTop = %d, want 2 when cursor is still on first visible line", got)
	}
	w.ActiveEditor().Cursor = len([]rune("one\ntwo"))
	w.ensureEditorVisible()
	if got := w.ActiveEditor().ScrollTop; got != 1 {
		t.Fatalf("ScrollTop = %d, want 1 after moving above viewport", got)
	}
}

func TestRenderEditorPaneUsesScrollTop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:      "one\ntwo\nthree\nfour",
		ScrollTop: 2,
		Mode:      ModeInsert,
	}
	got := renderEditorPane(ed, 20, 2)
	if !strings.Contains(got[0], "three") || !strings.Contains(got[1], "four") {
		t.Fatalf("renderEditorPane() = %#v, want lines starting from scroll top", got)
	}
	if !strings.Contains(helpers.StripANSI(got[0]), "3 ") {
		t.Fatalf("renderEditorPane() = %#v, want line number prefix", got)
	}
}

func TestRenderEditorPaneWrapsLongLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text: "alpha beta gamma delta",
		Mode: ModeInsert,
	}
	got := renderEditorPane(ed, 12, 3)
	if len(got) < 2 {
		t.Fatalf("renderEditorPane() = %#v, want wrapped visual rows", got)
	}
	if !strings.Contains(helpers.StripANSI(got[0]), "1 ") {
		t.Fatalf("first wrapped row = %q, want line number gutter", got[0])
	}
	if strings.Contains(helpers.StripANSI(got[1]), "1 ") {
		t.Fatalf("continuation row = %q, should not repeat line number", got[1])
	}
	if !strings.Contains(got[0]+got[1], "alpha") {
		t.Fatalf("wrapped rows = %#v, want source text preserved", got)
	}
}

func TestRenderPreviewPaneWrapsLongLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text: "This is a very long preview line that should wrap cleanly",
		Mode: ModeNormal,
	}
	got := renderPreviewPane(ed, 12, 3)
	if got[1] == "" {
		t.Fatalf("renderPreviewPane() = %#v, want wrapped continuation row", got)
	}
}

func TestWrapPlainLineHardWrapsLongUnbrokenLink(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	link := "https://example.com/abcdefghijklmnopqrstuvwxyz"
	segments := wrapPlainLine(link, 12)
	if len(segments) < 3 {
		t.Fatalf("wrapPlainLine() returned %d segment(s), want hard-wrapped long link", len(segments))
	}
	for _, segment := range segments {
		if segment.displayWidth > 12 {
			t.Fatalf("segment width = %d for %q, want <= 12", segment.displayWidth, segment.text)
		}
	}
}

func TestWrapPlainLinePrefersURLPunctuationBeforeHardBreak(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	line := "[LP-6926](https://lemonadefinance.atlassian.net/browse/LP-6926)"
	segments := wrapPlainLine(line, 62)
	if len(segments) < 2 {
		t.Fatalf("wrapPlainLine() returned %d segment(s), want wrapped URL", len(segments))
	}
	if strings.HasSuffix(segments[0].text, "LP-69") {
		t.Fatalf("first segment = %q, want break at URL punctuation instead of leaving a tiny suffix", segments[0].text)
	}
	for _, segment := range segments {
		if segment.displayWidth > 62 {
			t.Fatalf("segment width = %d for %q, want <= 62", segment.displayWidth, segment.text)
		}
	}
}

func TestEditorRenderSpansUseRawMarkdownOffsetsForLinks(t *testing.T) {
	text := "[LP-6926](https://lemonadefinance.atlassian.net/browse/LP-6926)"
	spans := editorRenderSpans(text, 4)
	found := false
	for _, span := range spans {
		if span.Tag != tagLink {
			continue
		}
		found = true
		if span.Start != 1 || span.End != len([]rune("LP-6926"))+1 {
			t.Fatalf("link span = %d..%d, want raw label offsets 1..8", span.Start, span.End)
		}
	}
	if !found {
		t.Fatal("editorRenderSpans() did not include link span")
	}
	rows := renderEditorPane(&Editor{Text: text, Mode: ModeInsert}, 24, 4)
	for _, row := range rows {
		plain := helpers.StripANSI(row)
		if helpers.VisibleRuneCount(row) > 24 {
			t.Fatalf("rendered row %q has visible width %d, want <= 24", plain, helpers.VisibleRuneCount(row))
		}
	}
}

func TestEditorCursorUsesWrappedRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 12,
		Tabs: []*Editor{{
			Text:   "alpha beta gamma",
			Cursor: len([]rune("alpha beta ")),
			Mode:   ModeInsert,
		}},
		CurrentTab: 0,
	}
	row, _, ok := w.EditorCursor()
	if !ok {
		t.Fatal("EditorCursor() ok = false, want true")
	}
	if row == 0 {
		t.Fatalf("EditorCursor() row = %d, want wrapped row below first line", row)
	}
}

func TestEditorCursorUsesCellWidthInsideHardWrappedLongLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 12,
		Tabs: []*Editor{{
			Text:   "abcdefghijklmnop",
			Cursor: 12,
			Mode:   ModeInsert,
		}},
		CurrentTab: 0,
	}
	row, col, ok := w.EditorCursor()
	if !ok {
		t.Fatal("EditorCursor() ok = false, want true")
	}
	if row != 1 || col != 2 {
		t.Fatalf("EditorCursor() = row %d col %d, want row 1 col 2", row, col)
	}
}

func TestEditorOffsetAtVisualPositionMapsWrappedTextAndLongLinks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 12,
		Tabs: []*Editor{{
			Text: "alpha beta gamma\nhttps://example.com/abcdef",
			Mode: ModeInsert,
		}},
		CurrentTab: 0,
	}
	offset, ok := w.EditorOffsetAtVisualPosition(1, 4)
	if !ok {
		t.Fatal("EditorOffsetAtVisualPosition() ok = false, want true")
	}
	if offset != len([]rune("alpha "))+2 {
		t.Fatalf("offset = %d, want mapped offset inside wrapped first line", offset)
	}
	offset, ok = w.EditorOffsetAtVisualPosition(3, 5)
	if !ok {
		t.Fatal("EditorOffsetAtVisualPosition() for link ok = false, want true")
	}
	want := len([]rune("alpha beta gamma\n")) + 11
	if offset != want {
		t.Fatalf("link offset = %d, want %d", offset, want)
	}
}

func TestMouseDragSelectionUsesVisualPositions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 20,
		Tabs: []*Editor{{
			Text: "abcdef",
			Mode: ModeInsert,
		}},
		CurrentTab: 0,
	}
	if !w.BeginMouseSelection(0, 3) {
		t.Fatal("BeginMouseSelection() = false, want true")
	}
	if !w.DragMouseSelection(0, 6) {
		t.Fatal("DragMouseSelection() = false, want true")
	}
	ed := w.ActiveEditor()
	if ed.Mode != ModeVisual || ed.SelectionMode != vimSelectionChar {
		t.Fatalf("mode = %s selection = %s, want visual char selection", ed.Mode, ed.SelectionMode)
	}
	if ed.SelectionMark != 1 || ed.SelectionCursor != 4 || ed.Cursor != 4 {
		t.Fatalf("selection mark/cursor = %d/%d cursor %d, want 1/4 cursor 4", ed.SelectionMark, ed.SelectionCursor, ed.Cursor)
	}
}

func TestMouseClickLeavesEditorOutsideVisualMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 20,
		Tabs: []*Editor{{
			Text: "abcdef",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	if !w.BeginMouseSelection(0, 3) {
		t.Fatal("BeginMouseSelection() = false, want true")
	}
	if !w.MoveEditorCursorToVisualPosition(0, 3) {
		t.Fatal("MoveEditorCursorToVisualPosition() = false, want true")
	}
	ed := w.ActiveEditor()
	if ed.Mode == ModeVisual || ed.SelectionMode != vimSelectionNone {
		t.Fatalf("mode = %s selection = %s, want click to clear visual selection", ed.Mode, ed.SelectionMode)
	}
	if ed.Cursor != 1 {
		t.Fatalf("cursor = %d, want clicked offset 1", ed.Cursor)
	}
}

func TestMoveEditorCursorByVisualRowsMovesCursorAndClearsSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 20,
		EditorHeight:      4,
		Tabs: []*Editor{{
			Text:            "alpha beta gamma\ndelta\nepsilon",
			Cursor:          0,
			Mode:            ModeVisual,
			SelectionMode:   vimSelectionChar,
			SelectionMark:   0,
			SelectionCursor: 5,
		}},
		CurrentTab: 0,
	}
	if !w.MoveEditorCursorByVisualRows(1) {
		t.Fatal("MoveEditorCursorByVisualRows() = false, want true")
	}
	ed := w.ActiveEditor()
	if ed.Cursor != len([]rune("alpha beta gamma\n")) {
		t.Fatalf("cursor = %d, want next visual row at following line start", ed.Cursor)
	}
	if ed.SelectionMode != vimSelectionNone {
		t.Fatalf("selection = %s, want cleared selection", ed.SelectionMode)
	}
	if got := ed.ScrollTop; got != 0 {
		t.Fatalf("ScrollTop = %d, want unchanged", got)
	}
}

func TestEnsureEditorVisibleUsesActualRenderWidthForWrappedRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 12,
		EditorHeight:      2,
		Tabs: []*Editor{{
			Text:   "alpha beta gamma delta",
			Cursor: len([]rune("alpha beta gamma ")),
			Mode:   ModeInsert,
		}},
		CurrentTab: 0,
	}
	w.ensureEditorVisible()
	if got := w.ActiveEditor().ScrollTop; got == 0 {
		t.Fatalf("ScrollTop = %d, want wrapped-row scrolling based on render width", got)
	}
}

func TestRenderEditorPaneHighlightsAllSearchOccurrences(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:       "alpha beta alpha",
		LastSearch: "alpha",
		Mode:       ModeNormal,
	}
	got := renderEditorPane(ed, 40, 1)[0]
	if strings.Count(got, helpers.ANSIRoleSearch) != 2 {
		t.Fatalf("renderEditorPane() = %q, want both search matches highlighted", got)
	}
}

func TestRenderEditorPaneHighlightsLiveSearchCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:       "alpha beta alpha",
		Mode:       ModeCommand,
		Command:    "/alpha",
		LastSearch: "beta",
	}
	got := renderEditorPane(ed, 40, 1)[0]
	if strings.Count(got, helpers.ANSIRoleSearch) != 2 {
		t.Fatalf("renderEditorPane() = %q, want live command matches highlighted", got)
	}
	if strings.Contains(got, helpers.ANSIRoleSearch+"beta") {
		t.Fatalf("renderEditorPane() = %q, want command query to override previous search", got)
	}
}

func TestRenderEditorPaneHighlightsLiveSubstituteCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:    "alpha beta alpha",
		Mode:    ModeCommand,
		Command: "%s/alpha/omega/",
	}
	got := renderEditorPane(ed, 40, 1)[0]
	if strings.Count(got, helpers.ANSIRoleSearch) != 2 {
		t.Fatalf("renderEditorPane() = %q, want live substitute matches highlighted", got)
	}
}

func TestRenderEditorPaneHighlightsPartialLiveSubstituteCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:    "alpha beta alpha",
		Mode:    ModeCommand,
		Command: "s#alpha",
	}
	got := renderEditorPane(ed, 40, 1)[0]
	if strings.Count(got, helpers.ANSIRoleSearch) != 2 {
		t.Fatalf("renderEditorPane() = %q, want partial substitute matches highlighted", got)
	}
}

func TestListManagedFilesSkipsNotesAndIncludesAssets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := listManagedFiles()
	if err != nil {
		t.Fatal(err)
	}
	foundFolder := false
	foundAsset := false
	for _, entry := range entries {
		if entry.Kind == fileEntryScope && entry.Scope == filepath.Join("Work", "Plan.md") {
			foundFolder = true
		}
		if entry.Kind == fileEntryAsset && filepath.ToSlash(entry.RelPath) == "Work/Plan.assets/diagram.png" {
			foundAsset = true
		}
		if entry.Kind == fileEntryAsset && strings.HasSuffix(entry.RelPath, "Plan.md") {
			t.Fatalf("listManagedFiles() should skip notes, got %#v", entry)
		}
	}
	if !foundFolder || !foundAsset {
		t.Fatalf("listManagedFiles() missing folder or asset: %#v", entries)
	}
}

func TestInsertSelectedFileReferenceUsesSmartMarkdown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	noteTitle := ws.ActiveEditor().Title
	if err := os.MkdirAll(filepath.Join(notesDir(), noteTitle+"."+managedAssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), noteTitle+"."+managedAssetsDir, "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws.refreshFiles()
	for i, entry := range ws.FileTree {
		if entry.Kind == fileEntryAsset && entry.Label == "diagram.png" {
			ws.FileSelection = i
			break
		}
	}
	if err := ws.InsertSelectedFileReference(); err != nil {
		t.Fatal(err)
	}
	wantRef := fmt.Sprintf("![diagram](%s.assets/diagram.png)", noteTitle)
	if got := ws.ActiveEditor().Text; !strings.Contains(got, wantRef) {
		t.Fatalf("InsertSelectedFileReference() text = %q", got)
	}
}

func TestMoveSelectedAssetFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "Images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "Images", "diagram.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.refreshFiles()
	for i, entry := range ws.FileTree {
		if entry.Kind == fileEntryFolder && filepath.ToSlash(entry.RelPath) == "Work/Plan.assets/Images" {
			ws.FileSelection = i
			break
		}
	}
	if err := ws.MoveSelectedFileEntry("Archive"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "Archive", "Images")
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("real moved folder should not exist before save, stat err = %v", err)
	}
	if !ws.FilesDirty {
		t.Fatal("FilesDirty = false, want true after staged move")
	}
	if _, err := ws.SavePendingFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("moved folder missing at %q after save: %v", want, err)
	}
}

func TestFileCommandLineTextShowsImportScopePrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.FileCommandMode = true
	ws.FileCommand = "import "
	got := ws.FileCommandLineText(120)
	if !strings.Contains(got, "Import into scope Note 1") {
		t.Fatalf("FileCommandLineText() = %q, want import scope prompt", got)
	}
}

func TestFileRowsShowStagedMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.FilesDirty = true
	rows := ws.FileRows(3)
	if !strings.Contains(rows[0], "[STAGED]") {
		t.Fatalf("FileRows()[0] = %q, want staged marker", rows[0])
	}
	if got := ws.FileCommandLineText(120); !strings.Contains(got, "D discard staged changes") {
		t.Fatalf("FileCommandLineText() = %q, want staged discard hint", got)
	}
}

func TestHandleFilesKeyScopeFolderShortcut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if !ws.HandleFilesKey(Key{Name: "F", Rune: 'F'}) {
		t.Fatal("HandleFilesKey(F) = false, want true")
	}
	if !ws.FileCommandMode || !ws.FileScopeOnly || ws.FileCommand != "mkdir " {
		t.Fatalf("scope folder prompt state = mode:%t scopeOnly:%t cmd:%q", ws.FileCommandMode, ws.FileScopeOnly, ws.FileCommand)
	}
}

func TestMigrateLooseManagedFilesMovesOldFilesIntoAssets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(notesDir(), "Work", "diagram.png")
	if err := os.WriteFile(oldPath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	moved, err := migrateLooseManagedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("migrateLooseManagedFiles() moved = %d, want 1", moved)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old loose file should remain until save, err = %v", err)
	}
	newPath := filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "diagram.png")
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("migrated file should not exist before save, stat err = %v", err)
	}
	ws.FilesDirty = true
	if _, err := ws.SavePendingFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old loose file still exists after save, err = %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("migrated file missing at %q after save: %v", newPath, err)
	}
}

func TestCountLooseManagedFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := countLooseManagedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("countLooseManagedFiles() = %d, want 1", got)
	}
}

func TestHandleFilesKeyStartsFilterMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if !ws.HandleFilesKey(Key{Name: "/"}) {
		t.Fatal("HandleFilesKey(/) = false, want true")
	}
	if !ws.FileFilterMode {
		t.Fatal("FileFilterMode = false, want true")
	}
}

func TestRefreshFilesWithFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "notes.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.FileFilter = "diagram"
	ws.refreshFiles()
	foundDiagram := false
	foundPDF := false
	for _, entry := range ws.FileTree {
		if entry.Kind == fileEntryAsset && entry.Label == "diagram.png" {
			foundDiagram = true
		}
		if entry.Kind == fileEntryAsset && entry.Label == "notes.pdf" {
			foundPDF = true
		}
	}
	if !foundDiagram || foundPDF {
		t.Fatalf("filtered tree unexpected: %#v", ws.FileTree)
	}
}

func TestManagedReferenceCandidates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(notePath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := managedReferenceCandidates(notePath, "Plan.assets/i")
	if len(got) == 0 || got[0] != "Plan.assets/img/" {
		t.Fatalf("managedReferenceCandidates() = %#v, want Plan.assets/img/ candidate", got)
	}
}

func TestCompleteEditorPathReference(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(notePath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Open(notePath); err != nil {
		t.Fatal(err)
	}
	ed := ws.ActiveEditor()
	ed.Mode = ModeInsert
	ed.Text = "![alt](Plan.assets/i)"
	ed.Cursor = len([]rune(ed.Text)) - 1
	if !completeEditorPathReference(ws, ed) {
		t.Fatal("completeEditorPathReference() = false, want true")
	}
	if !strings.Contains(ed.Text, "Plan.assets/img/") {
		t.Fatalf("editor text = %q, want completed assets path", ed.Text)
	}
}

func TestAutoCompleteStatusLineShowsSuggestions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(notePath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ed := &Editor{
		Path:   notePath,
		Mode:   ModeInsert,
		Text:   "![alt](Plan.assets/i)",
		Cursor: len([]rune("![alt](Plan.assets/i")),
	}
	got := autoCompleteStatusLine(ed, 120)
	if !strings.Contains(got, "path suggestions:") || !strings.Contains(got, "Plan.assets/img/") {
		t.Fatalf("autoCompleteStatusLine() = %q, want suggestion list", got)
	}
}

func TestSplitImportPaths(t *testing.T) {
	got := splitImportPaths(" /tmp/a.png | /tmp/b.pdf |  /tmp/c ")
	want := []string{"/tmp/a.png", "/tmp/b.pdf", "/tmp/c"}
	if len(got) != len(want) {
		t.Fatalf("splitImportPaths() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitImportPaths() = %#v, want %#v", got, want)
		}
	}
}

func TestPathToFileURI(t *testing.T) {
	got := pathToFileURI("/tmp/example file.txt")
	if got != "file:///tmp/example%20file.txt" {
		t.Fatalf("pathToFileURI() = %q", got)
	}
}

func TestManagedScopesUseNoteFoldersOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "LooseOnlyFolder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", managedAssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(notePath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	scopes, err := managedScopes()
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 1 || scopes[0].RelPath != filepath.Join("Work", "Plan.md") || scopes[0].Title != "Plan" {
		t.Fatalf("managedScopes() = %#v, want only Work/Plan.md scope", scopes)
	}
}

func TestShiftTabCompletesBackward(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(notePath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "aaa.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Open(notePath); err != nil {
		t.Fatal(err)
	}
	ed := ws.ActiveEditor()
	ed.Mode = ModeInsert
	ed.Text = "![alt](Plan.assets/)"
	ed.Cursor = len([]rune(ed.Text)) - 1
	if !completeEditorPathReferenceBackward(ws, ed) {
		t.Fatal("completeEditorPathReferenceBackward() = false, want true")
	}
	if !strings.Contains(ed.Text, "Plan.assets/img/") {
		t.Fatalf("editor text = %q, want backward completion candidate", ed.Text)
	}
}

func TestInsertModeShiftTabOutdentsListItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 2
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "unordered", text: "  - item", want: "- item"},
		{name: "checklist", text: "  - [ ] item", want: "- [ ] item"},
		{name: "ordered", text: "  1. item", want: "1. item"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &Editor{Text: tc.text, Cursor: len([]rune(tc.text)), Mode: ModeInsert}
			if !handleInsertMode(&Workspace{}, ed, Key{Name: "tab", Shift: true}) {
				t.Fatal("handleInsertMode(shift+tab) = false, want true")
			}
			if ed.Text != tc.want {
				t.Fatalf("text = %q, want %q", ed.Text, tc.want)
			}
		})
	}
}

func TestInsertModeShiftTabDoesNotOutdentBelowZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Text: "- item", Cursor: len([]rune("- item")), Mode: ModeInsert}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "tab", Shift: true}) {
		t.Fatal("handleInsertMode(shift+tab) = false, want true")
	}
	if ed.Text != "- item" {
		t.Fatalf("text = %q, want unchanged bullet", ed.Text)
	}
	if ed.Dirty {
		t.Fatal("Dirty = true, want no-op outdent to leave note clean")
	}
}

func TestInsertModeShiftTabCyclesActiveAutocompleteBackward(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(notePath, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "aaa.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan."+managedAssetsDir, "img", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Open(notePath); err != nil {
		t.Fatal(err)
	}
	ed := ws.ActiveEditor()
	ed.Mode = ModeInsert
	ed.Text = "![alt](Plan.assets/)"
	ed.Cursor = len([]rune(ed.Text)) - 1
	if !handleInsertMode(ws, ed, Key{Name: "tab"}) {
		t.Fatal("handleInsertMode(tab) = false, want autocomplete")
	}
	if !handleInsertMode(ws, ed, Key{Name: "tab", Shift: true}) {
		t.Fatal("handleInsertMode(shift+tab) = false, want autocomplete cycle")
	}
	if !strings.Contains(ed.Text, "Plan.assets/img/") {
		t.Fatalf("editor text = %q, want backward completion candidate", ed.Text)
	}
}

func TestAutoCompleteStatusLineShowsSpellSuggestions(t *testing.T) {
	ed := &Editor{
		Mode:                ModeInsert,
		AutoCompleteKind:    autoCompleteSpell,
		AutoCompleteMatches: []string{"color", "collar"},
		AutoCompleteIndex:   0,
	}
	got := autoCompleteStatusLine(ed, 120)
	if got != "" {
		t.Fatalf("autoCompleteStatusLine() = %q, want spell popup to hide command-line suggestion text", got)
	}
}

func TestInsertModeCtrlGOpensSpellSuggestionsAndTabCycles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& kokotoolsspellprobe 1 0: tool\n", nil
		}
		if strings.Contains(input, "collor") {
			return "& collor 7 0: color, collar, coll or, coll-or\n", nil
		}
		return "* OK\n", nil
	})

	ed := &Editor{
		Text:   "use collor ",
		Cursor: len([]rune("use collor")),
		Mode:   ModeInsert,
	}
	ws := &Workspace{Tabs: []*Editor{ed}}
	if !ws.handleEditorKey(Key{Name: "g", Ctrl: true}) {
		t.Fatal("handleEditorKey(ctrl+g) = false, want true")
	}
	if got := ed.Text; got != "use collor " {
		t.Fatalf("text = %q, want original text preserved before approval", got)
	}
	if ed.AutoCompleteKind != autoCompleteSpell {
		t.Fatalf("AutoCompleteKind = %q, want %q", ed.AutoCompleteKind, autoCompleteSpell)
	}
	if ed.Status != "spell suggestions ready" {
		t.Fatalf("status = %q, want spell suggestion readiness", ed.Status)
	}
	if !ws.handleEditorKey(Key{Name: "down"}) {
		t.Fatal("handleEditorKey(down) = false, want spell suggestion cycle")
	}
	if got := ed.Text; got != "use collor " {
		t.Fatalf("text = %q, want text unchanged while cycling suggestions", got)
	}
	if ed.AutoCompleteIndex != 1 {
		t.Fatalf("AutoCompleteIndex = %d, want 1 after cycling", ed.AutoCompleteIndex)
	}
	if !ws.handleEditorKey(Key{Name: "up"}) {
		t.Fatal("handleEditorKey(up) = false, want reverse spell suggestion cycle")
	}
	if ed.AutoCompleteIndex != 0 {
		t.Fatalf("AutoCompleteIndex = %d, want 0 after reverse cycle", ed.AutoCompleteIndex)
	}
	if !ws.handleEditorKey(Key{Name: "enter"}) {
		t.Fatal("handleEditorKey(enter) = false, want spell suggestion accept")
	}
	if got := ed.Text; got != "use color " {
		t.Fatalf("text = %q, want accepted spell suggestion", got)
	}
}

func TestInsertModeCtrlGWithoutNativeSuggestionsShowsStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	SetSpellNativeHooksForTests(func(string) (string, error) {
		return "", errors.New("missing")
	}, nil)
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")

	ed := &Editor{
		Text:   "use collor ",
		Cursor: len([]rune("use collor")),
		Mode:   ModeInsert,
	}
	ws := &Workspace{Tabs: []*Editor{ed}}
	if !ws.handleEditorKey(Key{Name: "g", Ctrl: true}) {
		t.Fatal("handleEditorKey(ctrl+g) = false, want true")
	}
	if ed.Status != "spell suggestions failed" {
		t.Fatalf("status = %q, want spell suggestions failed", ed.Status)
	}
}

func TestHandleEditorKeyCtrlGOpensSpellSuggestionsOutsideInsertMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& kokotoolsspellprobe 1 0: tool\n", nil
		}
		if strings.Contains(input, "Naice") {
			return "& Naice 2 0: Nice, Naive\n", nil
		}
		return "* OK\n", nil
	})

	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "Naice work",
			Cursor: 2,
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	if !w.handleEditorKey(Key{Name: "g", Ctrl: true}) {
		t.Fatal("handleEditorKey(ctrl+g) = false, want true")
	}
	if got := w.ActiveEditor().Text; got != "Naice work" {
		t.Fatalf("text = %q, want original text preserved in normal mode", got)
	}
	if w.ActiveEditor().AutoCompleteKind != autoCompleteSpell {
		t.Fatalf("AutoCompleteKind = %q, want %q", w.ActiveEditor().AutoCompleteKind, autoCompleteSpell)
	}
	if !w.handleEditorKey(Key{Name: "enter"}) {
		t.Fatal("handleEditorKey(enter) = false, want spell suggestion accepted in normal mode")
	}
	if got := w.ActiveEditor().Text; got != "Nice work" {
		t.Fatalf("text = %q, want accepted spell suggestion in normal mode", got)
	}
}

func TestExecuteVimCommandSpellAppliesSuggestions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& kokotoolsspellprobe 1 0: tool\n", nil
		}
		if strings.Contains(input, "Naice") {
			return "& Naice 2 0: Nice, Naive\n", nil
		}
		return "* OK\n", nil
	})

	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "Naice work",
			Cursor: 2,
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandSpell})
	if got := w.ActiveEditor().Text; got != "Naice work" {
		t.Fatalf("text = %q, want spell command to keep text unchanged before approval", got)
	}
	if w.ActiveEditor().Status != "spell suggestions ready" {
		t.Fatalf("status = %q, want spell suggestion readiness", w.ActiveEditor().Status)
	}
	if !w.handleEditorKey(Key{Name: "enter"}) {
		t.Fatal("handleEditorKey(enter) = false, want spell suggestion accept after :spell")
	}
	if got := w.ActiveEditor().Text; got != "Nice work" {
		t.Fatalf("text = %q, want accepted spell suggestion after :spell", got)
	}
}

func TestCommandModeSpellOpensSuggestions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& kokotoolsspellprobe 1 0: tool\n", nil
		}
		if strings.Contains(input, "Naice") {
			return "& Naice 2 0: Nice, Naive\n", nil
		}
		return "* OK\n", nil
	})

	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "Naice work",
			Cursor: 2,
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	if !submitEditorCommand(w, "spell") {
		t.Fatal("submitEditorCommand(spell) = false, want true")
	}
	ed := w.ActiveEditor()
	if ed.AutoCompleteKind != autoCompleteSpell {
		t.Fatalf("AutoCompleteKind = %q, want %q", ed.AutoCompleteKind, autoCompleteSpell)
	}
	if ed.Status != "spell suggestions ready" {
		t.Fatalf("status = %q, want spell suggestion readiness", ed.Status)
	}
}

func TestSpellCommandWithoutWordShowsSpecificStatus(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "   ",
			Cursor: 1,
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandSpell})
	if got := w.ActiveEditor().Status; got != "no word under cursor for spelling" {
		t.Fatalf("status = %q, want no word under cursor for spelling", got)
	}
}

func TestOpenSpellSuggestionsIgnoresIdentitySuggestions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& Wrong: kokotoolsspellprobe. How about: tool\n", nil
		}
		if strings.Contains(input, "Naice") {
			return "& Wrong: Naice. How about: Naice, naice\n", nil
		}
		return "* OK\n", nil
	})

	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "Naice work",
			Cursor: 2,
			Mode:   ModeInsert,
		}},
		CurrentTab: 0,
	}
	if !w.handleEditorKey(Key{Name: "g", Ctrl: true}) {
		t.Fatal("handleEditorKey(ctrl+g) = false, want true")
	}
	if got := w.ActiveEditor().Text; got != "Naice work" {
		t.Fatalf("text = %q, want unchanged text when no proper suggestions exist", got)
	}
	if w.ActiveEditor().AutoCompleteKind != "" {
		t.Fatalf("AutoCompleteKind = %q, want cleared autocomplete when no proper suggestions exist", w.ActiveEditor().AutoCompleteKind)
	}
	if w.ActiveEditor().Status != "no spelling suggestions returned" {
		t.Fatalf("status = %q, want no spelling suggestions returned", w.ActiveEditor().Status)
	}
}

func TestRenderEditorPaneShowsSpellSuggestionPopupUnderWord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:                "use collor here",
		Cursor:              len([]rune("use collor")),
		Mode:                ModeInsert,
		AutoCompleteKind:    autoCompleteSpell,
		AutoCompleteMatches: []string{"color", "collar"},
		AutoCompleteIndex:   0,
		AutoCompleteStart:   len([]rune("use ")),
		AutoCompleteEnd:     len([]rune("use collor")),
	}
	got := strings.Join(renderEditorPane(ed, 40, 4), "\n")
	if !strings.Contains(got, "> color") {
		t.Fatalf("renderEditorPane() = %q, want active spell suggestion popup row", got)
	}
	if !strings.Contains(got, "  collar") {
		t.Fatalf("renderEditorPane() = %q, want secondary spell suggestion popup row", got)
	}
}

func TestReferenceForFileOverrides(t *testing.T) {
	entry := &FileEntry{
		Path:  "/tmp/diagram.png",
		Label: "diagram.png",
		Image: true,
	}
	ws := &Workspace{}
	if got := ws.referenceForFile(entry, markdownInsertLink); got != "[diagram.png](diagram.png)" {
		t.Fatalf("referenceForFile(link) = %q", got)
	}
	if got := ws.referenceForFile(entry, markdownInsertImage); got != "![diagram](diagram.png)" {
		t.Fatalf("referenceForFile(image) = %q", got)
	}
}

func TestSaveAllDirtyWritesEveryDirtyTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	root := t.TempDir()
	first := filepath.Join(root, "one.md")
	second := filepath.Join(root, "two.md")
	w := &Workspace{
		Tabs: []*Editor{
			{Path: first, Text: "first", Dirty: true},
			{Path: second, Text: "second", Dirty: true},
		},
		CurrentTab: 0,
	}
	wrote, err := w.SaveAllDirty()
	if err != nil {
		t.Fatalf("SaveAllDirty() error = %v", err)
	}
	if !wrote {
		t.Fatal("SaveAllDirty() wrote = false, want true")
	}
	for _, path := range []string{first, second} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		if string(data) == "" {
			t.Fatalf("ReadFile(%q) = empty, want saved content", path)
		}
	}
}

func TestHandleNormalModePMultilineCharRegisterPastesBelowCurrentLine(t *testing.T) {
	restore := helpers.SetClipboardReaderForTesting(func() (string, error) {
		return "alpha\nbeta", nil
	})
	defer restore()
	ed := &Editor{
		Text:   "one\ntwo",
		Cursor: 1,
		Mode:   ModeNormal,
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "p", Rune: 'p'}) {
		t.Fatal("handleNormalMode(p) = false, want true")
	}
	if ed.Text != "one\nalpha\nbeta\ntwo" {
		t.Fatalf("editor text = %q, want multiline paste on next line", ed.Text)
	}
}

func TestHandleNormalModePSingleLineCharRegisterPastesInline(t *testing.T) {
	restore := helpers.SetClipboardReaderForTesting(func() (string, error) {
		return "ZZ", nil
	})
	defer restore()
	ed := &Editor{
		Text:   "one",
		Cursor: 1,
		Mode:   ModeNormal,
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "p", Rune: 'p'}) {
		t.Fatal("handleNormalMode(p) = false, want true")
	}
	if ed.Text != "onZZe" {
		t.Fatalf("editor text = %q, want inline paste", ed.Text)
	}
}

func TestWorkspaceRegisterSupportsPasteAcrossNotesWithoutClipboard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error {
		return os.ErrPermission
	})
	defer restoreWrite()
	restoreRead := helpers.SetClipboardReaderForTesting(func() (string, error) {
		return "", os.ErrPermission
	})
	defer restoreRead()

	ws := &Workspace{
		Tabs: []*Editor{
			{Text: "alpha beta", Cursor: 0, Mode: ModeNormal},
			{Text: "one", Cursor: 1, Mode: ModeNormal},
		},
		CurrentTab: 0,
	}
	if !handleNormalMode(ws, ws.Tabs[0], Key{Name: "y", Rune: 'y'}) {
		t.Fatal("first y should arm yank")
	}
	if !handleNormalMode(ws, ws.Tabs[0], Key{Name: "w", Rune: 'w'}) {
		t.Fatal("w after y should yank word")
	}
	ws.setCurrentTab(1)
	if !handleNormalMode(ws, ws.Tabs[1], Key{Name: "p", Rune: 'p'}) {
		t.Fatal("handleNormalMode(p) = false, want true")
	}
	if got := ws.Tabs[1].Text; got != "onalphae" {
		t.Fatalf("text = %q, want pasted workspace register", got)
	}
}

func TestWorkspaceRegisterOverridesStaleNoteRegisterOnPaste(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	restoreRead := helpers.SetClipboardReaderForTesting(func() (string, error) {
		return "", os.ErrPermission
	})
	defer restoreRead()
	ws := &Workspace{
		Register: vimRegister{Kind: vimRegisterChar, Text: "fresh"},
		Tabs: []*Editor{{
			Text:     "one",
			Cursor:   1,
			Mode:     ModeNormal,
			Register: vimRegister{Kind: vimRegisterChar, Text: "stale"},
		}},
		CurrentTab: 0,
	}
	if !handleNormalMode(ws, ws.Tabs[0], Key{Name: "p", Rune: 'p'}) {
		t.Fatal("handleNormalMode(p) = false, want true")
	}
	if got := ws.Tabs[0].Text; got != "onfreshe" {
		t.Fatalf("text = %q, want workspace register paste", got)
	}
}

func TestSaveAllDirtyLocalDoesNotStartDriveSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().GDrive.Enabled = true
	settings.Inst().GDrive.FolderID = "folder-1"
	settings.Inst().GDrive.PendingSync = true

	root := t.TempDir()
	first := filepath.Join(root, "one.md")
	w := &Workspace{
		Tabs:       []*Editor{{Path: first, Text: "first", Dirty: true}},
		CurrentTab: 0,
	}
	wrote, err := w.SaveAllDirtyLocal()
	if err != nil {
		t.Fatalf("SaveAllDirtyLocal() error = %v", err)
	}
	if !wrote {
		t.Fatal("SaveAllDirtyLocal() wrote = false, want true")
	}
	if !settings.Inst().GDrive.PendingSync {
		t.Fatal("PendingSync = false, want true after local-only save")
	}
}

func TestWorkspaceTabNavigationAndCurrentNoteActions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	if len(w.Tabs) != 2 {
		t.Fatalf("tab count = %d, want 2", len(w.Tabs))
	}
	secondPath := w.ActiveEditor().Path
	if secondPath == firstPath {
		t.Fatal("expected a different newly created note path")
	}
	if !w.PrevTab() || w.ActiveEditor().Path != firstPath {
		t.Fatal("PrevTab() should move back to first note")
	}
	if !w.NextTab() || w.ActiveEditor().Path != secondPath {
		t.Fatal("NextTab() should move forward to second note")
	}
	if !w.DeleteCurrentNote() {
		t.Fatal("DeleteCurrentNote() = false, want true")
	}
	if len(w.Tabs) != 1 {
		t.Fatalf("tab count after delete = %d, want 1", len(w.Tabs))
	}
	if _, err := os.Stat(secondPath); !os.IsNotExist(err) {
		t.Fatalf("deleted note should be gone, stat err = %v", err)
	}
}

func TestCloseSidebarNoteRemovesItFromOpenSessionOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	path := w.ActiveEditor().Path
	w.FocusSidebar = true
	w.Selection = findTreeEntryIndex(t, w.Tree, treeOpenNote, path, "")
	if !w.HandleKey(Key{Name: "x", Rune: 'x'}) {
		t.Fatal("HandleKey(x) = false, want true")
	}
	if len(w.Tabs) != 0 {
		t.Fatalf("tab count = %d, want 0 after closing last open note", len(w.Tabs))
	}
	if w.CurrentTab != -1 {
		t.Fatalf("CurrentTab = %d, want -1", w.CurrentTab)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("closed note file missing: %v", err)
	}
	if got := settings.Inst().NotesApp.OpenNotePaths; len(got) != 0 {
		t.Fatalf("OpenNotePaths = %v, want empty", got)
	}
	if got := settings.Inst().NotesApp.CurrentNotePath; got != "" {
		t.Fatalf("CurrentNotePath = %q, want empty", got)
	}
	if findTreeEntryIndex(t, w.BrowserTree, treeNote, path, "") < 0 {
		t.Fatal("expected closed note to remain in browser tree")
	}
}

func TestSidebarBrowserEnterReopensClosedNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	path := w.ActiveEditor().Path
	w.FocusSidebar = true
	w.Selection = findTreeEntryIndex(t, w.Tree, treeOpenNote, path, "")
	if !w.HandleKey(Key{Name: "x", Rune: 'x'}) {
		t.Fatal("HandleKey(x) = false, want true")
	}
	w.FocusSidebar = true
	w.toggleSidebarBrowser()
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, path, "")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want true")
	}
	if len(w.Tabs) != 1 {
		t.Fatalf("tab count = %d, want 1 after reopen", len(w.Tabs))
	}
	if got := w.ActiveEditor(); got == nil || got.Path != path {
		t.Fatalf("active editor = %#v, want path %q", got, path)
	}
	if settings.Inst().NotesApp.CurrentNotePath != "Note 1.md" {
		t.Fatalf("CurrentNotePath = %q, want Note 1.md", settings.Inst().NotesApp.CurrentNotePath)
	}
}

func TestSidebarBrowserEnterFocusesAlreadyOpenNoteWithoutDuplicatingTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	w.FocusSidebar = true
	w.toggleSidebarBrowser()
	w.BrowserSelection = findTreeEntryIndex(t, w.BrowserTree, treeNote, firstPath, "")
	if !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want true")
	}
	if len(w.Tabs) != 2 {
		t.Fatalf("tab count = %d, want 2", len(w.Tabs))
	}
	if got := w.ActiveEditor(); got == nil || got.Path != firstPath {
		t.Fatalf("active editor = %#v, want first path %q", got, firstPath)
	}
	if w.LastAccessedTab < 0 || w.Tabs[w.LastAccessedTab].Path != secondPath {
		t.Fatalf("LastAccessedTab = %d, want second note remembered", w.LastAccessedTab)
	}
}

func TestCtrlAThenASwitchesToLastAccessedNoteAndFocusesEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	if secondPath == firstPath {
		t.Fatal("expected second note path to differ from first")
	}
	if !w.HandleKey(Key{Name: "a", Ctrl: true}) {
		t.Fatal("HandleKey(ctrl+a) = false, want sidebar focus")
	}
	if !w.FocusSidebar {
		t.Fatal("FocusSidebar = false, want sidebar focused after ctrl+a")
	}
	if !w.HandleKey(Key{Name: "a", Rune: 'a'}) {
		t.Fatal("HandleKey(a) = false, want switch to last accessed note")
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want editor focused after switch")
	}
	if got := w.ActiveEditor().Path; got != firstPath {
		t.Fatalf("active path = %q, want previous note %q", got, firstPath)
	}
	if !w.HandleKey(Key{Name: "a", Ctrl: true}) || !w.HandleKey(Key{Name: "a", Rune: 'a'}) {
		t.Fatal("ctrl+a then a should switch back to second note")
	}
	if got := w.ActiveEditor().Path; got != secondPath {
		t.Fatalf("active path = %q, want second note %q", got, secondPath)
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want editor focused after switching back")
	}
}

func TestCtrlAThenNumberSwitchesToNumberedNoteAndFocusesEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() second = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() third = false, want true")
	}
	if !w.HandleKey(Key{Name: "a", Ctrl: true}) {
		t.Fatal("HandleKey(ctrl+a) = false, want sidebar focus")
	}
	if !w.HandleKey(Key{Name: "1", Rune: '1'}) {
		t.Fatal("HandleKey(1) = false, want numbered note switch")
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want editor focused after numbered switch")
	}
	if got := w.ActiveEditor().Path; got != firstPath {
		t.Fatalf("active path = %q, want first note %q", got, firstPath)
	}
	if !w.HandleKey(Key{Name: "a", Ctrl: true}) || !w.HandleKey(Key{Name: "2", Rune: '2'}) {
		t.Fatal("ctrl+a then 2 should switch to second note")
	}
	if got := w.ActiveEditor().Path; got != secondPath {
		t.Fatalf("active path = %q, want second note %q", got, secondPath)
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want editor focused after second numbered switch")
	}
}

func TestRenderTabsIncludesNoteShortcutIndicators(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{
			{Title: "Plan"},
			{Title: "Log"},
		},
		CurrentTab: 1,
	}
	got := helpers.StripANSI(renderTabs(w))
	if !strings.Contains(got, "[1:Plan x]") {
		t.Fatalf("renderTabs() = %q, want first note shortcut indicator", got)
	}
	if !strings.Contains(got, "[2:Log x]") {
		t.Fatalf("renderTabs() = %q, want second note shortcut indicator", got)
	}
}

func TestRenderTabsStylesOnlyActiveCloseControlWithCloseRole(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{
			{Title: "Plan"},
			{Title: "Log"},
		},
		CurrentTab: 1,
	}
	got := renderTabs(w)
	if strings.Count(got, helpers.ANSIRoleActiveTabClose) != 1 {
		t.Fatalf("renderTabs() = %q, want one active close role", got)
	}
	if !strings.Contains(got, helpers.ANSIRoleActiveTabClose+"x") {
		t.Fatalf("renderTabs() = %q, want active close x styled", got)
	}
	inactive := strings.Split(got, " ")[0]
	if strings.Contains(inactive, helpers.ANSIRoleActiveTabClose) {
		t.Fatalf("inactive tab = %q, want unchanged close styling", inactive)
	}
}

func TestSwitchToTabAtColumnSwitchesClickedNoteAndFocusesEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs: []*Editor{
			{Title: "Plan"},
			{Title: "Log"},
		},
		CurrentTab:   0,
		FocusSidebar: true,
	}
	if !w.SwitchToTabAtColumn(len("[1:Plan x] ")) {
		t.Fatal("SwitchToTabAtColumn(second tab start) = false, want true")
	}
	if w.CurrentTab != 1 {
		t.Fatalf("CurrentTab = %d, want 1", w.CurrentTab)
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want editor focused")
	}
	if w.SwitchToTabAtColumn(len("[1:Plan x]")) {
		t.Fatal("SwitchToTabAtColumn(separator) = true, want false")
	}
}

func TestCloseTabAtColumnClosesClickedNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs: []*Editor{
			{Path: "/tmp/plan.md", Title: "Plan"},
			{Path: "/tmp/log.md", Title: "Log"},
		},
		CurrentTab: 0,
	}
	if !w.CloseTabAtColumn(len("[1:Plan ")) {
		t.Fatal("CloseTabAtColumn(first close x) = false, want true")
	}
	if len(w.Tabs) != 1 || w.Tabs[0].Title != "Log" {
		t.Fatalf("tabs = %#v, want only Log tab", w.Tabs)
	}
}

func TestSwitchingOpenTabsUpdatesSessionInMemoryWithoutWritingSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settings.Init()
	first := filepath.Join(home, "first.md")
	second := filepath.Join(home, "second.md")
	w := &Workspace{
		Tabs: []*Editor{
			{Path: first, Title: "first", Mode: ModeNormal},
			{Path: second, Title: "second", Mode: ModeNormal},
		},
		CurrentTab: 0,
	}
	if !w.NextTab() {
		t.Fatal("NextTab() = false, want true")
	}
	if got := settings.Inst().NotesApp.CurrentNotePath; got != filepath.ToSlash(second) {
		t.Fatalf("CurrentNotePath = %q, want %q", got, filepath.ToSlash(second))
	}
	settingsPath := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings file exists after pure tab switch, err = %v", err)
	}
}

func TestNewWorkspaceRestoresOpenTabsSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	if secondPath == firstPath {
		t.Fatal("expected second note path to differ from first")
	}
	if !w.PrevTab() {
		t.Fatal("PrevTab() = false, want true")
	}
	restored, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(restored.Tabs); got != 2 {
		t.Fatalf("restored tab count = %d, want 2", got)
	}
	if restored.Tabs[0].Path != firstPath || restored.Tabs[1].Path != secondPath {
		t.Fatalf("restored paths = %q, %q", restored.Tabs[0].Path, restored.Tabs[1].Path)
	}
	if restored.ActiveEditor() == nil || restored.ActiveEditor().Path != firstPath {
		t.Fatalf("active path = %v, want %q", restored.ActiveEditor(), firstPath)
	}
}

func TestNewWorkspaceRestoresPortableSnapshotSessionPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	w.FocusSidebar = true
	w.toggleSidebarBrowser()
	for i, entry := range w.BrowserTree {
		if entry.Kind == treeFolder && entry.Folder == "Projects" {
			w.BrowserSelection = i
			break
		}
	}
	created, err := w.CreateNote("Plan")
	if err != nil {
		t.Fatalf("CreateNote() error = %v", err)
	}
	secondPath := created
	settings.Inst().NotesApp.OpenNotePaths = []string{
		"/old-home/.config/koko-tools/notes/Note 1.md",
		"/old-home/.config/koko-tools/notes/Projects/Plan.md",
	}
	settings.Inst().NotesApp.CurrentNotePath = "/old-home/.config/koko-tools/notes/Projects/Plan.md"

	restored, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(restored.Tabs); got != 2 {
		t.Fatalf("restored tab count = %d, want 2", got)
	}
	if restored.Tabs[0].Path != firstPath || restored.Tabs[1].Path != secondPath {
		t.Fatalf("restored paths = %q, %q", restored.Tabs[0].Path, restored.Tabs[1].Path)
	}
	if restored.ActiveEditor() == nil || restored.ActiveEditor().Path != secondPath {
		t.Fatalf("active path = %v, want %q", restored.ActiveEditor(), secondPath)
	}
}

func TestRenameCurrentNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	oldPath := w.ActiveEditor().Path
	oldAssets := noteAssetsPath(oldPath)
	if err := os.MkdirAll(oldAssets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldAssets, "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.RenameCurrentNote("Renamed Note"); err != nil {
		t.Fatalf("RenameCurrentNote() error = %v", err)
	}
	if w.ActiveEditor().Path == oldPath {
		t.Fatal("expected active note path to change after rename")
	}
	if w.ActiveEditor().Title != "Renamed Note" {
		t.Fatalf("title = %q, want %q", w.ActiveEditor().Title, "Renamed Note")
	}
	if _, err := os.Stat(w.ActiveEditor().Path); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	newAssets := noteAssetsPath(w.ActiveEditor().Path)
	if _, err := os.Stat(filepath.Join(newAssets, "diagram.png")); err != nil {
		t.Fatalf("renamed assets missing: %v", err)
	}
	if _, err := os.Stat(oldAssets); !os.IsNotExist(err) {
		t.Fatalf("old assets dir still exists, err = %v", err)
	}
}

func TestSidebarRenameLeavesSidebarFocusAndEntersCommandMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	if !w.HandleKey(Key{Name: "R", Rune: 'R', Shift: true}) {
		t.Fatal("HandleKey(R) = false, want true")
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want false so rename input receives keys")
	}
	ed := w.ActiveEditor()
	if ed.Mode != ModeCommand {
		t.Fatalf("Mode = %q, want %q", ed.Mode, ModeCommand)
	}
	if got := ed.Command; got != "rename "+ed.Title {
		t.Fatalf("Command = %q, want prefilled rename command", got)
	}
	if !w.HandleKey(Key{Name: "X", Rune: 'X', Shift: true}) {
		t.Fatal("HandleKey(X) = false, want true in command mode")
	}
	if !strings.HasSuffix(ed.Command, "X") {
		t.Fatalf("Command = %q, want typed input appended", ed.Command)
	}
}

func TestExecuteVimCommandOpenLinksQueuesPendingRequest(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Title: "Plan",
			Text:  "See https://example.com and ftp://example.com/pub",
			Mode:  ModeNormal,
		}},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandOpenLinks})
	got := w.TakePendingOpenLinks()
	if len(got) != 2 {
		t.Fatalf("TakePendingOpenLinks() len = %d, want 2 (%v)", len(got), got)
	}
	if got[0] != "https://example.com" || got[1] != "ftp://example.com/pub" {
		t.Fatalf("TakePendingOpenLinks() = %v", got)
	}
	if again := w.TakePendingOpenLinks(); len(again) != 0 {
		t.Fatalf("TakePendingOpenLinks() should clear request, got %v", again)
	}
}

func TestExecuteVimCommandRecordKeysQueuesPendingRequest(t *testing.T) {
	w := &Workspace{
		Tabs:       []*Editor{{Title: "Plan", Text: "x", Mode: ModeNormal}},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandRecordKeys})
	if !w.TakePendingRecordKeys() {
		t.Fatal("TakePendingRecordKeys() = false, want true")
	}
	if w.TakePendingRecordKeys() {
		t.Fatal("TakePendingRecordKeys() should clear request")
	}
	if got := w.ActiveEditor().Status; got != "key recording requested" {
		t.Fatalf("status = %q, want %q", got, "key recording requested")
	}
}

func TestExecuteVimCommandQuitQueuesPendingQuit(t *testing.T) {
	w := &Workspace{Tabs: []*Editor{{Title: "Plan", Text: "x"}}, CurrentTab: 0}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandQuit})
	quit, force := w.TakePendingQuit()
	if !quit {
		t.Fatal("TakePendingQuit() = false, want true")
	}
	if force {
		t.Fatal("force = true, want false for plain quit")
	}
	if quit, _ := w.TakePendingQuit(); quit {
		t.Fatal("TakePendingQuit() should clear request")
	}
}

func TestExecuteVimCommandBufferDeleteClosesActiveNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs: []*Editor{
			{Path: "/tmp/plan.md", Title: "Plan", Text: "plan"},
			{Path: "/tmp/log.md", Title: "Log", Text: "log"},
		},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandBufferDelete})
	if len(w.Tabs) != 1 || w.Tabs[0].Title != "Log" {
		t.Fatalf("tabs = %#v, want active Plan buffer closed", w.Tabs)
	}
}

func TestExecuteVimCommandChainSavesThenQueuesForcedQuit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ed := ws.ActiveEditor()
	ed.Text = "saved by wq"
	ed.Dirty = true
	executeVimCommand(ws, ed, vimCommand{Kind: vimCommandSequence, Commands: []vimCommand{
		{Kind: vimCommandSave},
		{Kind: vimCommandQuit, Force: true},
	}})
	if !ws.TakePendingSaveAll() {
		t.Fatal("TakePendingSaveAll() = false, want true")
	}
	quit, force := ws.TakePendingQuit()
	if !quit || !force {
		t.Fatalf("TakePendingQuit() = %t, %t; want forced quit", quit, force)
	}
}

func TestExecuteVimCommandPreviewTogglesPane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs:       []*Editor{{Title: "Plan", Text: "hello", Mode: ModeNormal}},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandPreview})
	if !w.PreviewHidden {
		t.Fatal("PreviewHidden = false, want true after first toggle")
	}
	if got := w.ActiveEditor().Status; got != "preview hidden" {
		t.Fatalf("status = %q, want %q", got, "preview hidden")
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandPreview})
	if w.PreviewHidden {
		t.Fatal("PreviewHidden = true, want false after second toggle")
	}
	if got := w.ActiveEditor().Status; got != "preview shown" {
		t.Fatalf("status = %q, want %q", got, "preview shown")
	}
}

func TestExecuteVimCommandSidebarTogglesFocus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs:       []*Editor{{Title: "Plan", Text: "hello", Mode: ModeNormal}},
		CurrentTab: 0,
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandSidebar})
	if !w.FocusSidebar {
		t.Fatal("FocusSidebar = false, want true after sidebar command")
	}
	if got := w.ActiveEditor().Status; got != "sidebar focused" {
		t.Fatalf("status = %q, want sidebar focused", got)
	}
	executeVimCommand(w, w.ActiveEditor(), vimCommand{Kind: vimCommandSidebar})
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want false after second sidebar command")
	}
	if got := w.ActiveEditor().Status; got != "editor focused" {
		t.Fatalf("status = %q, want editor focused", got)
	}
}

func TestNewWorkspaceUsesPersistedPreviewHiddenState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.PreviewHidden = true
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if !w.PreviewHidden {
		t.Fatal("PreviewHidden = false, want true from persisted settings")
	}
}

func TestCanDeleteFocusedNoteDistinguishesSidebarFolders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	w.FocusSidebar = true
	w.SidebarBrowsing = true
	for i, entry := range w.BrowserTree {
		if entry.Kind == treeFolder && entry.Label == "Projects" {
			w.BrowserSelection = i
			break
		}
	}
	if w.CanDeleteFocusedNote() {
		t.Fatal("CanDeleteFocusedNote() = true, want false for folder selection")
	}
}

func TestHandleSidebarKeyEscReturnsFocusToEditor(t *testing.T) {
	w := &Workspace{
		FocusSidebar: true,
		Tree:         []TreeEntry{{Kind: treeOpenNote, Label: "Plan"}},
	}
	if !w.handleSidebarKey(Key{Name: "esc"}) {
		t.Fatal("handleSidebarKey(esc) = false, want true")
	}
	if w.FocusSidebar {
		t.Fatal("FocusSidebar = true, want false")
	}
}

func TestHandleSidebarKeyEscClosesBrowserBeforeLeavingSidebar(t *testing.T) {
	w := &Workspace{
		FocusSidebar:     true,
		SidebarBrowsing:  true,
		BrowserTree:      []TreeEntry{{Kind: treeNote, Label: "Plan"}},
		BrowserSelection: 0,
	}
	if !w.handleSidebarKey(Key{Name: "esc"}) {
		t.Fatal("handleSidebarKey(esc) = false, want true")
	}
	if !w.FocusSidebar {
		t.Fatal("FocusSidebar = false, want sidebar to remain focused")
	}
	if w.SidebarBrowsing {
		t.Fatal("SidebarBrowsing = true, want browser closed")
	}
}

func TestDeleteFocusedNoteOnlyDeletesSelectedSidebarNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	if secondPath == firstPath {
		t.Fatal("expected different second note path")
	}
	w.FocusSidebar = true
	found := false
	for i, entry := range w.Tree {
		if entry.Kind == treeOpenNote && entry.Path == secondPath {
			w.Selection = i
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("could not find sidebar entry for %q", secondPath)
	}
	if !w.DeleteFocusedNote() {
		t.Fatal("DeleteFocusedNote() = false, want true")
	}
	if _, err := os.Stat(secondPath); !os.IsNotExist(err) {
		t.Fatalf("selected note still exists, stat err = %v", err)
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("other note missing after delete: %v", err)
	}
	files, err := listNoteFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != firstPath {
		t.Fatalf("remaining files = %#v, want only %q", files, firstPath)
	}
}

func TestNoOpenNoteStateRendersAndAllowsSidebarCreation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	path := w.ActiveEditor().Path
	w.FocusSidebar = true
	w.Selection = findTreeEntryIndex(t, w.Tree, treeOpenNote, path, "")
	if !w.HandleKey(Key{Name: "x", Rune: 'x'}) {
		t.Fatal("HandleKey(x) = false, want true")
	}
	w.FocusSidebar = false
	if got := w.HelpText(); !strings.Contains(got, "no note open") {
		t.Fatalf("HelpText() = %q, want no-open-note hint", got)
	}
	if got := w.CommandLineText(80); !strings.Contains(got, "no note open") {
		t.Fatalf("CommandLineText() = %q, want no-open-note hint", got)
	}
	if got := strings.Join(w.EditorRows(20, 3), "\n"); !strings.Contains(got, "No note open") {
		t.Fatalf("EditorRows() = %q, want empty editor state", got)
	}
	if err := w.SaveCurrent(); err != nil {
		t.Fatalf("SaveCurrent() error = %v, want nil when no note is open", err)
	}
	w.FocusSidebar = true
	if !w.HandleKey(Key{Name: "e", Rune: 'e'}) {
		t.Fatal("HandleKey(e) = false, want browser open with no active note")
	}
	if !w.SidebarBrowsing {
		t.Fatal("SidebarBrowsing = false, want browser open")
	}
	if !w.HandleKey(Key{Name: "e", Rune: 'e'}) {
		t.Fatal("HandleKey(e) second = false, want browser close")
	}
	if !w.HandleKey(Key{Name: "n", Rune: 'n'}) {
		t.Fatal("HandleKey(n) = false, want create note from sidebar")
	}
	if len(w.Tabs) != 1 {
		t.Fatalf("tab count = %d, want 1 after creating note", len(w.Tabs))
	}
}

func TestHandleKeyCtrlDQueuesDeleteConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	activePath := w.ActiveEditor().Path
	if !w.HandleKey(Key{Name: "d", Ctrl: true}) {
		t.Fatal("HandleKey(ctrl+d) = false, want true")
	}
	path, label, ok := w.TakePendingDeleteNote()
	if !ok {
		t.Fatal("TakePendingDeleteNote() ok = false, want true")
	}
	if path != activePath {
		t.Fatalf("delete path = %q, want %q", path, activePath)
	}
	if label != w.ActiveEditor().Title {
		t.Fatalf("delete label = %q, want %q", label, w.ActiveEditor().Title)
	}
	if _, _, ok := w.TakePendingDeleteNote(); ok {
		t.Fatal("pending delete request should clear after take")
	}
}

func TestHandleSidebarKeyDQueuesDeleteConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	w.FocusSidebar = true
	found := false
	for i, entry := range w.Tree {
		if entry.Kind == treeOpenNote && entry.Path == secondPath {
			w.Selection = i
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("could not find sidebar entry for %q", secondPath)
	}
	if !w.HandleKey(Key{Name: "d", Rune: 'd'}) {
		t.Fatal("HandleKey(d) = false, want true")
	}
	path, label, ok := w.TakePendingDeleteNote()
	if !ok {
		t.Fatal("TakePendingDeleteNote() ok = false, want true")
	}
	if path != secondPath {
		t.Fatalf("delete path = %q, want %q", path, secondPath)
	}
	if label != w.Tree[w.Selection].Label {
		t.Fatalf("delete label = %q, want %q", label, w.Tree[w.Selection].Label)
	}
}

func TestFocusedNoteDeletePathUsesActiveNoteWhenSidebarSelectionIsStale(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	for i, entry := range w.Tree {
		if entry.Kind == treeOpenNote && entry.Path == firstPath {
			w.Selection = i
			break
		}
	}
	w.FocusSidebar = false
	if got := w.ActiveEditor().Path; got != secondPath {
		t.Fatalf("active editor path = %q, want %q", got, secondPath)
	}
	if !w.HandleKey(Key{Name: "a", Ctrl: true}) {
		t.Fatal("HandleKey(ctrl+a) = false, want sidebar focus")
	}
	if got := w.FocusedNoteDeletePath(); got != secondPath {
		t.Fatalf("FocusedNoteDeletePath() = %q, want active path %q after sidebar focus", got, secondPath)
	}
	if !w.HandleKey(Key{Name: "d", Rune: 'd'}) {
		t.Fatal("HandleKey(d) = false, want delete request for active note")
	}
	path, _, ok := w.TakePendingDeleteNote()
	if !ok {
		t.Fatal("TakePendingDeleteNote() ok = false, want true")
	}
	if path != secondPath {
		t.Fatalf("pending delete path = %q, want active path %q", path, secondPath)
	}
}

func TestSidebarCloseUsesActiveNoteAfterTabSwitch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	firstPath := w.ActiveEditor().Path
	if !w.NewNote() {
		t.Fatal("NewNote() = false, want true")
	}
	secondPath := w.ActiveEditor().Path
	w.Selection = findTreeEntryIndex(t, w.Tree, treeOpenNote, firstPath, "")
	w.FocusSidebar = false
	if !w.HandleKey(Key{Name: "a", Ctrl: true}) {
		t.Fatal("HandleKey(ctrl+a) = false, want sidebar focus")
	}
	if !w.HandleKey(Key{Name: "x", Rune: 'x'}) {
		t.Fatal("HandleKey(x) = false, want close active note")
	}
	if len(w.Tabs) != 1 {
		t.Fatalf("tab count = %d, want 1", len(w.Tabs))
	}
	if got := w.Tabs[0].Path; got != firstPath {
		t.Fatalf("remaining tab = %q, want first note %q", got, firstPath)
	}
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("closed active note file should remain: %v", err)
	}
}

func TestHandleNormalModeRReplacesCharacter(t *testing.T) {
	ed := &Editor{Text: "abcd", Cursor: 1, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "r", Rune: 'r'}) {
		t.Fatal("handleNormalMode(r) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "Z", Rune: 'Z', Shift: true}) {
		t.Fatal("handleNormalMode(replacement) = false, want true")
	}
	if got := ed.Text; got != "aZcd" {
		t.Fatalf("text = %q, want %q", got, "aZcd")
	}
	if ed.PendingOp != "" {
		t.Fatalf("PendingOp = %q, want cleared", ed.PendingOp)
	}
}

func TestHandleNormalModeXDeletesCharacter(t *testing.T) {
	ed := &Editor{Text: "abcd", Cursor: 1, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "x", Rune: 'x'}) {
		t.Fatal("handleNormalMode(x) = false, want true")
	}
	if got := ed.Text; got != "acd" {
		t.Fatalf("text = %q, want %q", got, "acd")
	}
}

func TestHandleNormalModeXWDeletesNextWord(t *testing.T) {
	ed := &Editor{Text: "alpha beta gamma", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "x", Rune: 'x'}) {
		t.Fatal("handleNormalMode(x) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "w", Rune: 'w'}) {
		t.Fatal("handleNormalMode(w after x) = false, want true")
	}
	if got := ed.Text; got != "beta gamma" {
		t.Fatalf("text = %q, want %q", got, "beta gamma")
	}
}

func TestHandleNormalModeDWDeletesWord(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "alpha beta gamma", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "w", Rune: 'w'}) {
		t.Fatal("handleNormalMode(w) = false, want true after d")
	}
	if got := ed.Text; got != " beta gamma" {
		t.Fatalf("text = %q, want %q", got, " beta gamma")
	}
	if ed.PendingOp != "" {
		t.Fatalf("PendingOp = %q, want cleared", ed.PendingOp)
	}
}

func TestHandleNormalModeDWDeletesWordWithoutNewline(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "alpha\nbeta", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "w", Rune: 'w'}) {
		t.Fatal("handleNormalMode(w) = false, want true after d")
	}
	if got := ed.Text; got != "\nbeta" {
		t.Fatalf("text = %q, want %q", got, "\nbeta")
	}
}

func TestHandleNormalModeDWDeletesFromCursorToWordEndOnly(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "alpha beta", Cursor: 2, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "w", Rune: 'w'}) {
		t.Fatal("handleNormalMode(w) = false, want true after d")
	}
	if got := ed.Text; got != "al beta" {
		t.Fatalf("text = %q, want %q", got, "al beta")
	}
}

func TestHandleNormalModeDDollarDeletesToEndOfLine(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "alpha beta\ngamma", Cursor: 6, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "$", Rune: '$', Shift: true}) {
		t.Fatal("handleNormalMode($) = false, want true after d")
	}
	if got := ed.Text; got != "alpha \ngamma" {
		t.Fatalf("text = %q, want %q", got, "alpha \ngamma")
	}
}

func TestHandleNormalModeDDownDeletesCurrentAndNextLine(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "one\ntwo\nthree\nfour", Cursor: len([]rune("one\n")), Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "down"}) {
		t.Fatal("handleNormalMode(down) = false, want true after d")
	}
	if got := ed.Text; got != "one\nfour" {
		t.Fatalf("text = %q, want current and next line deleted", got)
	}
	if got := ed.Cursor; got != len([]rune("one\n")) {
		t.Fatalf("cursor = %d, want start of surviving line", got)
	}
}

func TestHandleNormalModeDDRenumbersFollowingOrderedListItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{
		Text:   "1. one\n2. two\n3. three",
		Cursor: len([]rune("1. one\n")),
		Mode:   ModeNormal,
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) second = false, want true")
	}
	if got := ed.Text; got != "1. one\n2. three" {
		t.Fatalf("text = %q, want ordered list renumbered after delete", got)
	}
}

func TestHandleNormalModeDDRenumbersOrderedListAcrossNestedBulletItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{
		Text: strings.Join([]string{
			"1. a",
			"2. b",
			"    - ba",
			"    - bb",
			"3. c",
			"4. d",
		}, "\n"),
		Cursor: len([]rune("1. a\n2. b\n    - ba\n    - bb\n")),
		Mode:   ModeNormal,
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) second = false, want true")
	}
	want := strings.Join([]string{
		"1. a",
		"2. b",
		"    - ba",
		"    - bb",
		"3. d",
	}, "\n")
	if got := ed.Text; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}

func TestHandleNormalModeDDownRenumbersFollowingOrderedListItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{
		Text:   "1. one\n2. two\n3. three\n4. four",
		Cursor: len([]rune("1. one\n")),
		Mode:   ModeNormal,
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "down"}) {
		t.Fatal("handleNormalMode(down) = false, want true after d")
	}
	if got := ed.Text; got != "1. one\n2. four" {
		t.Fatalf("text = %q, want ordered list renumbered after span delete", got)
	}
}

func TestHandleNormalModeDUpDeletesCurrentAndPreviousLine(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "one\ntwo\nthree\nfour", Cursor: len([]rune("one\ntwo\n")), Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "up"}) {
		t.Fatal("handleNormalMode(up) = false, want true after d")
	}
	if got := ed.Text; got != "one\nfour" {
		t.Fatalf("text = %q, want previous and current line deleted", got)
	}
	if got := ed.Cursor; got != len([]rune("one\n")) {
		t.Fatalf("cursor = %d, want start of surviving current line", got)
	}
}

func TestHandleNormalModeCountDDownDeletesLineSpan(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "one\ntwo\nthree\nfour\nfive", Cursor: len([]rune("one\n")), Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "2", Rune: '2'}) {
		t.Fatal("handleNormalMode(2) = false, want true while pending d")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "down"}) {
		t.Fatal("handleNormalMode(down) = false, want true after d2")
	}
	if got := ed.Text; got != "one\nfive" {
		t.Fatalf("text = %q, want current plus two lines below deleted", got)
	}
	if ed.NormalCount != "" || ed.PendingOp != "" {
		t.Fatalf("NormalCount/PendingOp = %q/%q, want cleared", ed.NormalCount, ed.PendingOp)
	}
}

func TestHandleNormalModeCountDUpDeletesLineSpan(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "one\ntwo\nthree\nfour\nfive", Cursor: len([]rune("one\ntwo\nthree\n")), Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleNormalMode(d) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "2", Rune: '2'}) {
		t.Fatal("handleNormalMode(2) = false, want true while pending d")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "up"}) {
		t.Fatal("handleNormalMode(up) = false, want true after d2")
	}
	if got := ed.Text; got != "one\nfive" {
		t.Fatalf("text = %q, want current plus two lines above deleted", got)
	}
	if got := ed.Cursor; got != len([]rune("one\n")) {
		t.Fatalf("cursor = %d, want start of surviving current line", got)
	}
}

func TestHandleNormalModeUUndoesLastEdit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Text: "abc", Cursor: 3, Mode: ModeInsert}
	if !insertRune(ed, 'd') {
		t.Fatal("insertRune() = false, want true")
	}
	ed.Mode = ModeNormal
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "u", Rune: 'u'}) {
		t.Fatal("handleNormalMode(u) = false, want true")
	}
	if got := ed.Text; got != "abc" {
		t.Fatalf("text = %q, want %q", got, "abc")
	}
	if got := ed.Cursor; got != 3 {
		t.Fatalf("cursor = %d, want 3", got)
	}
	if ed.Status != "undo" {
		t.Fatalf("status = %q, want undo", ed.Status)
	}
}

func TestExecuteVimCommandUndoRedo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "hello", Cursor: 5, Mode: ModeInsert}
	insertRune(ed, '!')
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	executeVimCommand(w, ed, vimCommand{Kind: vimCommandUndo})
	if got := ed.Text; got != "hello" {
		t.Fatalf("undo text = %q, want %q", got, "hello")
	}
	executeVimCommand(w, ed, vimCommand{Kind: vimCommandRedo})
	if got := ed.Text; got != "hello!" {
		t.Fatalf("redo text = %q, want %q", got, "hello!")
	}
}

func TestInsertUndoTypingInsideUnfinishedWordStepsByLetter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Mode: ModeInsert}
	insertRune(ed, 'a')
	insertRune(ed, 'b')
	insertRune(ed, 'c')

	if !applyUndo(ed) {
		t.Fatal("applyUndo() = false, want undo")
	}
	if got := ed.Text; got != "ab" {
		t.Fatalf("undo text = %q, want %q", got, "ab")
	}
}

func TestInsertUndoCompletedWordsStepByWordBoundary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Mode: ModeInsert}
	for _, r := range "alpha " {
		insertRune(ed, r)
	}

	if !applyUndo(ed) {
		t.Fatal("applyUndo() = false, want undo")
	}
	if got := ed.Text; got != "" {
		t.Fatalf("undo text = %q, want completed word removed", got)
	}
}

func TestInsertUndoCompletedSentenceStepsBySentenceBoundary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Mode: ModeInsert}
	for _, r := range "Hello world. " {
		insertRune(ed, r)
	}

	if !applyUndo(ed) {
		t.Fatal("applyUndo() = false, want undo")
	}
	if got := ed.Text; got != "" {
		t.Fatalf("undo text = %q, want completed sentence removed", got)
	}
}

func TestInsertUndoParagraphBreakCreatesBoundary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Text: "one\n", Cursor: len([]rune("one\n")), Mode: ModeInsert}
	if !insertNewline(ed) {
		t.Fatal("insertNewline() = false, want true")
	}
	if got := ed.Text; got != "one\n\n" {
		t.Fatalf("text = %q, want blank line paragraph break", got)
	}
	if !applyUndo(ed) {
		t.Fatal("applyUndo() = false, want undo")
	}
	if got := ed.Text; got != "one\n" {
		t.Fatalf("undo text = %q, want paragraph break removed", got)
	}
}

func TestInsertNewlineRenumbersFollowingOrderedListItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:   "1. one\n2. two\n3. three",
		Cursor: len([]rune("1. one")),
		Mode:   ModeInsert,
	}

	if !insertNewline(ed) {
		t.Fatal("insertNewline() = false, want true")
	}
	want := "1. one\n2. \n3. two\n4. three"
	if ed.Text != want {
		t.Fatalf("text = %q, want %q", ed.Text, want)
	}
	if ed.Cursor != len([]rune("1. one\n2. ")) {
		t.Fatalf("cursor = %d, want after inserted ordered marker", ed.Cursor)
	}
}

func TestInsertNewlineRenumbersOnlySameIndentOrderedListBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text: strings.Join([]string{
			"  1. one",
			"  2. two",
			"    1. nested",
			"  3. three",
		}, "\n"),
		Cursor: len([]rune("  1. one")),
		Mode:   ModeInsert,
	}

	if !insertNewline(ed) {
		t.Fatal("insertNewline() = false, want true")
	}
	want := strings.Join([]string{
		"  1. one",
		"  2. ",
		"  3. two",
		"    1. nested",
		"  4. three",
	}, "\n")
	if ed.Text != want {
		t.Fatalf("text = %q, want %q", ed.Text, want)
	}
}

func TestReplaceCommandCanBeUndone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "alpha beta", Cursor: 0, Mode: ModeCommand}
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	executeVimCommand(w, ed, vimCommand{Kind: vimCommandReplace, Query: "alpha", Replacement: "omega", Global: true})
	if got := ed.Text; got != "omega beta" {
		t.Fatalf("replace text = %q, want %q", got, "omega beta")
	}
	executeVimCommand(w, ed, vimCommand{Kind: vimCommandUndo})
	if got := ed.Text; got != "alpha beta" {
		t.Fatalf("undo text = %q, want %q", got, "alpha beta")
	}
}

func TestVimSubstituteRangeAndCaptures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "one\ntwo two\nthree two", Cursor: 0, Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	if !submitEditorCommand(w, `2,$s/\(two\)/[\1:&]/g`) {
		t.Fatal("submitEditorCommand(range substitute) = false, want true")
	}
	if got := ed.Text; got != "one\n[two:two] [two:two]\nthree [two:two]" {
		t.Fatalf("text = %q, want ranged capture replacement", got)
	}
}

func TestVimSubstituteConfirmKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "one one one", Cursor: 0, Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	if !submitEditorCommand(w, "%s/one/two/gc") {
		t.Fatal("submitEditorCommand(confirm substitute) = false, want true")
	}
	if ed.ReplaceConfirm == nil {
		t.Fatal("ReplaceConfirm = nil, want active confirm session")
	}
	if !w.HandleKey(Key{Name: "n", Rune: 'n'}) || !w.HandleKey(Key{Name: "y", Rune: 'y'}) || !w.HandleKey(Key{Name: "q", Rune: 'q'}) {
		t.Fatal("confirm keys were not handled")
	}
	if got := ed.Text; got != "one two one" {
		t.Fatalf("text = %q, want only accepted replacement applied", got)
	}
	if ed.Status != "1 replacements" {
		t.Fatalf("Status = %q, want 1 replacements", ed.Status)
	}
	executeVimCommand(w, ed, vimCommand{Kind: vimCommandUndo})
	if got := ed.Text; got != "one one one" {
		t.Fatalf("undo text = %q, want original", got)
	}
}

func TestVimSubstituteConfirmEscCancels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "one one", Cursor: 0, Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	if !submitEditorCommand(w, "%s/one/two/gc") {
		t.Fatal("submitEditorCommand(confirm substitute) = false, want true")
	}
	if !w.HandleKey(Key{Name: "esc"}) {
		t.Fatal("HandleKey(esc) = false, want handled")
	}
	if got := ed.Text; got != "one one" {
		t.Fatalf("text = %q, want unchanged", got)
	}
	if ed.ReplaceConfirm != nil {
		t.Fatal("ReplaceConfirm still active after esc")
	}
}

func TestVisualSubstituteUsesSelectedLineRange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "one\ntwo\none", Cursor: len([]rune("one\n")), Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	startVisualSelection(ed, vimSelectionLine)
	if !submitEditorCommand(w, "s/two/2/") {
		t.Fatal("submitEditorCommand(visual substitute) = false, want true")
	}
	if got := ed.Text; got != "one\n2\none" {
		t.Fatalf("text = %q, want selected line replaced", got)
	}
}

func TestBackwardSearchAndRepeatDirection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Title: "Plan", Text: "alpha beta alpha", Cursor: len([]rune("alpha beta alpha")), Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	if !submitEditorCommand(w, "?alpha") {
		t.Fatal("submitEditorCommand(?alpha) = false, want true")
	}
	if ed.Cursor != len([]rune("alpha beta ")) {
		t.Fatalf("Cursor = %d, want second alpha", ed.Cursor)
	}
	if !w.HandleKey(Key{Name: "n", Rune: 'n'}) {
		t.Fatal("HandleKey(n) = false, want repeat search")
	}
	if ed.Cursor != 0 {
		t.Fatalf("Cursor after n = %d, want first alpha", ed.Cursor)
	}
	if !w.HandleKey(Key{Name: "N", Rune: 'N', Shift: true}) {
		t.Fatal("HandleKey(N) = false, want reverse repeat search")
	}
	if ed.Cursor != len([]rune("alpha beta ")) {
		t.Fatalf("Cursor after N = %d, want second alpha", ed.Cursor)
	}
}

func TestUndoLimitTrimsOldestSnapshots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.UndoLevels = 2
	ed := &Editor{Text: "", Cursor: 0, Mode: ModeInsert}
	insertRune(ed, 'a')
	insertRune(ed, 'b')
	insertRune(ed, 'c')
	if got := len(ed.UndoStack); got != 2 {
		t.Fatalf("undo stack len = %d, want 2", got)
	}
	if ed.UndoStack[0].Text != "a" || ed.UndoStack[1].Text != "ab" {
		t.Fatalf("undo stack = %#v, want oldest snapshot trimmed", ed.UndoStack)
	}
}

func TestHandleNormalModeXDollarDeletesToEndOfLine(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	ed := &Editor{Text: "alpha beta\ngamma", Cursor: 6, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "x", Rune: 'x'}) {
		t.Fatal("handleNormalMode(x) = false, want true")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "$", Rune: '$', Shift: true}) {
		t.Fatal("handleNormalMode($ after x) = false, want true")
	}
	if got := ed.Text; got != "alpha \ngamma" {
		t.Fatalf("text = %q, want %q", got, "alpha \ngamma")
	}
}

func TestHandleNormalModeIAndAStayOnCurrentLine(t *testing.T) {
	ed := &Editor{Text: "abcd", Cursor: 1, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "i", Rune: 'i'}) {
		t.Fatal("handleNormalMode(i) = false, want true")
	}
	if ed.Mode != ModeInsert || ed.Cursor != 1 {
		t.Fatalf("i -> mode=%q cursor=%d, want insert at 1", ed.Mode, ed.Cursor)
	}

	ed = &Editor{Text: "abcd", Cursor: 1, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "a", Rune: 'a'}) {
		t.Fatal("handleNormalMode(a) = false, want true")
	}
	if ed.Mode != ModeInsert || ed.Cursor != 2 {
		t.Fatalf("a -> mode=%q cursor=%d, want insert at 2", ed.Mode, ed.Cursor)
	}
}

func TestHandleNormalModeCountGJumpsToLineStart(t *testing.T) {
	lines := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i))
	}
	text := strings.Join(lines, "\n")
	ed := &Editor{Text: text, Cursor: 0, Mode: ModeNormal}
	w := &Workspace{}
	for _, key := range []Key{
		{Name: "1", Rune: '1'},
		{Name: "2", Rune: '2'},
		{Name: "G", Rune: 'G', Shift: true},
	} {
		if !handleNormalMode(w, ed, key) {
			t.Fatalf("handleNormalMode(%q) = false, want true", key.Name)
		}
	}
	want := len([]rune(strings.Join(lines[:11], "\n"))) + 1
	if ed.Cursor != want {
		t.Fatalf("cursor = %d, want start of line 12 at %d", ed.Cursor, want)
	}
	if ed.NormalCount != "" {
		t.Fatalf("NormalCount = %q, want cleared", ed.NormalCount)
	}
}

func TestHandleNormalModeGDefaultsToLastLine(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "G", Rune: 'G', Shift: true}) {
		t.Fatal("handleNormalMode(G) = false, want true")
	}
	if want := len([]rune("one\ntwo\n")); ed.Cursor != want {
		t.Fatalf("cursor = %d, want last line start %d", ed.Cursor, want)
	}
}

func TestHandleNormalModeCountGClampsToLastLine(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree", Cursor: 0, Mode: ModeNormal}
	for _, key := range []Key{
		{Name: "9", Rune: '9'},
		{Name: "9", Rune: '9'},
		{Name: "G", Rune: 'G', Shift: true},
	} {
		if !handleNormalMode(&Workspace{}, ed, key) {
			t.Fatalf("handleNormalMode(%q) = false, want true", key.Name)
		}
	}
	if want := len([]rune("one\ntwo\n")); ed.Cursor != want {
		t.Fatalf("cursor = %d, want clamped last line start %d", ed.Cursor, want)
	}
}

func TestHandleNormalModeZeroWithoutCountKeepsLineStartBehavior(t *testing.T) {
	ed := &Editor{Text: "one\ntwo", Cursor: len([]rune("one\nt")), Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "0", Rune: '0'}) {
		t.Fatal("handleNormalMode(0) = false, want true")
	}
	if want := len([]rune("one\n")); ed.Cursor != want {
		t.Fatalf("cursor = %d, want current line start %d", ed.Cursor, want)
	}
}

func TestInsertModeEnterContinuesIndentedBullet(t *testing.T) {
	ed := &Editor{
		Text:   "- item\n  - child",
		Cursor: len([]rune("- item\n  - child")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "enter"}) {
		t.Fatal("handleInsertMode(enter) = false, want true")
	}
	if got := ed.Text; got != "- item\n  - child\n  - " {
		t.Fatalf("text = %q, want indented continued bullet", got)
	}
}

func TestInsertModeTabIndentsBulletToNextLevel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 2
	ed := &Editor{
		Text:   "- item",
		Cursor: len([]rune("- item")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "tab"}) {
		t.Fatal("handleInsertMode(tab) = false, want true")
	}
	if got := ed.Text; got != "  - item" {
		t.Fatalf("text = %q, want indented bullet", got)
	}
}

func TestInsertModeTabIndentsOrderedItemToNextLevel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 2
	ed := &Editor{
		Text:   "1. item",
		Cursor: len([]rune("1. item")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "tab"}) {
		t.Fatal("handleInsertMode(tab) = false, want true")
	}
	if got := ed.Text; got != "  1. item" {
		t.Fatalf("text = %q, want indented ordered item", got)
	}
}

func TestInsertModeEnterOnEmptyBulletRemovesMarker(t *testing.T) {
	ed := &Editor{
		Text:   "- ",
		Cursor: len([]rune("- ")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "enter"}) {
		t.Fatal("handleInsertMode(enter) = false, want true")
	}
	if got := ed.Text; got != "" {
		t.Fatalf("text = %q, want empty line after leaving bullet", got)
	}
}

func TestInsertModeEnterOnEmptyOrderedItemRemovesMarker(t *testing.T) {
	ed := &Editor{
		Text:   "1. ",
		Cursor: len([]rune("1. ")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "enter"}) {
		t.Fatal("handleInsertMode(enter) = false, want true")
	}
	if got := ed.Text; got != "" {
		t.Fatalf("text = %q, want empty line after leaving ordered item", got)
	}
}

func TestInsertModeEnterOnEmptyOrderedItemRenumbersFollowingItems(t *testing.T) {
	ed := &Editor{
		Text:   "1. one\n2. \n3. three",
		Cursor: len([]rune("1. one\n2. ")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "enter"}) {
		t.Fatal("handleInsertMode(enter) = false, want true")
	}
	if got := ed.Text; got != "1. one\n\n2. three" {
		t.Fatalf("text = %q, want following ordered item renumbered", got)
	}
}

func TestInsertModeEnterOnEmptyBulletKeepsFollowingLine(t *testing.T) {
	ed := &Editor{
		Text:   "- \nnext",
		Cursor: len([]rune("- ")),
		Mode:   ModeInsert,
	}
	if !handleInsertMode(&Workspace{}, ed, Key{Name: "enter"}) {
		t.Fatal("handleInsertMode(enter) = false, want true")
	}
	if got := ed.Text; got != "\nnext" {
		t.Fatalf("text = %q, want blank line preserved before next line", got)
	}
	if got := ed.Cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0", got)
	}
}

func TestVisualLineYankAndDelete(t *testing.T) {
	restore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restore()
	ed := &Editor{Text: "one\ntwo\nthree", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "V", Rune: 'V', Shift: true}) {
		t.Fatal("V should enter visual line mode")
	}
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "y", Rune: 'y'}) {
		t.Fatal("y in visual line mode should succeed")
	}
	if ed.Register.Kind != vimRegisterLine || ed.Register.Text != "one\ntwo\n" {
		t.Fatalf("line register = %#v", ed.Register)
	}

	ed2 := &Editor{Text: "one\ntwo\nthree", Cursor: 0, Mode: ModeNormal}
	startVisualSelection(ed2, vimSelectionLine)
	ed2.Cursor = vimVerticalMoveOffset(ed2.Text, ed2.Cursor, 1)
	refreshVisualSelection(ed2)
	if !handleVisualMode(&Workspace{}, ed2, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("d in visual line mode should succeed")
	}
	if got := ed2.Text; got != "three" {
		t.Fatalf("text = %q, want %q", got, "three")
	}
}

func TestNormalModeShiftCurrentLineRightAndLeft(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 2
	ed := &Editor{Text: "one\ntwo", Cursor: 5, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: ">", Rune: '>'}) {
		t.Fatal("handleNormalMode(>) = false, want true")
	}
	if got := ed.Text; got != "one\n  two" {
		t.Fatalf("text = %q, want indented current line", got)
	}
	if got := ed.Cursor; got != 7 {
		t.Fatalf("cursor = %d, want shifted cursor", got)
	}
	if !ed.Dirty {
		t.Fatal("Dirty = false, want true")
	}
	ed.Dirty = false
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "<", Rune: '<'}) {
		t.Fatal("handleNormalMode(<) = false, want true")
	}
	if got := ed.Text; got != "one\ntwo" {
		t.Fatalf("text = %q, want outdented current line", got)
	}
	if got := ed.Cursor; got != 5 {
		t.Fatalf("cursor = %d, want restored cursor", got)
	}
	if !ed.Dirty {
		t.Fatal("Dirty = false, want true after outdent")
	}
}

func TestNormalModeShiftLeftNoopsWithoutIndent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 4
	ed := &Editor{Text: "one", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "<", Rune: '<'}) {
		t.Fatal("handleNormalMode(<) = false, want true")
	}
	if got := ed.Text; got != "one" {
		t.Fatalf("text = %q, want unchanged", got)
	}
	if ed.Dirty {
		t.Fatal("Dirty = true, want no-op outdent to remain clean")
	}
}

func TestVisualModeShiftSelectionRightAndLeft(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 2
	ed := &Editor{Text: "one\ntwo\nthree", Cursor: 0, Mode: ModeNormal}
	startVisualSelection(ed, vimSelectionLine)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	if !handleVisualMode(&Workspace{}, ed, Key{Name: ">", Rune: '>'}) {
		t.Fatal("handleVisualMode(>) = false, want true")
	}
	if got := ed.Text; got != "  one\n  two\nthree" {
		t.Fatalf("text = %q, want selected lines indented", got)
	}
	if ed.Mode != ModeVisual || ed.SelectionMode != vimSelectionLine {
		t.Fatalf("mode = %s selection = %s, want visual line selection", ed.Mode, ed.SelectionMode)
	}
	start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	if got := ed.Text[start:end]; got != "  one\n  two\n" {
		t.Fatalf("selected text = %q, want shifted visual selection", got)
	}
	if !ed.Dirty {
		t.Fatal("Dirty = false, want true")
	}

	ed.Dirty = false
	ed.Cursor = 2
	startVisualSelection(ed, vimSelectionChar)
	ed.Cursor = strings.Index(ed.Text, "two")
	refreshVisualSelection(ed)
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "<", Rune: '<'}) {
		t.Fatal("handleVisualMode(<) = false, want true")
	}
	if got := ed.Text; got != "one\ntwo\nthree" {
		t.Fatalf("text = %q, want selected lines outdented", got)
	}
	if ed.Mode != ModeVisual || ed.SelectionMode != vimSelectionChar {
		t.Fatalf("mode = %s selection = %s, want visual char selection", ed.Mode, ed.SelectionMode)
	}
	if !ed.Dirty {
		t.Fatal("Dirty = false, want true after visual outdent")
	}
}

func TestNormalModeMoveLineDownAndUp(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree", Cursor: len([]rune("one\n")), Mode: ModeNormal}
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "d", Rune: 'd'}} {
		if !handleNormalMode(&Workspace{}, ed, key) {
			t.Fatalf("handleNormalMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\nthree\ntwo" {
		t.Fatalf("text = %q, want line moved down", got)
	}
	if got, want := ed.Cursor, len([]rune("one\nthree\n")); got != want {
		t.Fatalf("cursor = %d, want moved line start %d", got, want)
	}
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "u", Rune: 'u'}} {
		if !handleNormalMode(&Workspace{}, ed, key) {
			t.Fatalf("handleNormalMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\ntwo\nthree" {
		t.Fatalf("text = %q, want line moved back up", got)
	}
}

func TestWorkspaceNormalModeMoveLineAtCursor(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "one\ntwo\nthree",
			Cursor: len([]rune("one\n")),
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "d", Rune: 'd'}} {
		if !w.HandleKey(key) {
			t.Fatalf("HandleKey(%q) = false, want true", key.Name)
		}
	}
	if got := w.ActiveEditor().Text; got != "one\nthree\ntwo" {
		t.Fatalf("text = %q, want current cursor line moved down", got)
	}
}

func TestWorkspaceNormalModeMoveLineUsesRuneToken(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "one\ntwo\nthree",
			Cursor: len([]rune("one\n")),
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	for _, key := range []Key{{Rune: 'm'}, {Rune: 'l'}, {Rune: 'd'}} {
		if !w.HandleKey(key) {
			t.Fatalf("HandleKey(%q/%q) = false, want true", key.Name, key.Rune)
		}
	}
	if got := w.ActiveEditor().Text; got != "one\nthree\ntwo" {
		t.Fatalf("text = %q, want current cursor line moved down", got)
	}
}

func TestMoveLinePendingCommandShownInCommandLine(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text: "one\ntwo",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	if !w.HandleKey(Key{Name: "m", Rune: 'm'}) {
		t.Fatal("HandleKey(m) = false, want pending command")
	}
	if got := w.CommandLineText(80); got != "m" {
		t.Fatalf("CommandLineText() = %q, want m", got)
	}
	if col, ok := w.CommandCursor(); !ok || col != 1 {
		t.Fatalf("CommandCursor() = %d/%t, want 1/true", col, ok)
	}
	if !w.HandleKey(Key{Name: "l", Rune: 'l'}) || !w.HandleKey(Key{Name: "3", Rune: '3'}) {
		t.Fatal("HandleKey(ml3) = false, want pending command")
	}
	if got := w.CommandLineText(80); got != "ml3" {
		t.Fatalf("CommandLineText() = %q, want ml3", got)
	}
	if col, ok := w.CommandCursor(); !ok || col != 3 {
		t.Fatalf("CommandCursor() = %d/%t, want 3/true", col, ok)
	}
}

func TestNormalModeMoveLineCountAndUndo(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree\nfour", Cursor: 0, Mode: ModeNormal}
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "3", Rune: '3'}, {Name: "d", Rune: 'd'}} {
		if !handleNormalMode(&Workspace{}, ed, key) {
			t.Fatalf("handleNormalMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "two\nthree\nfour\none" {
		t.Fatalf("text = %q, want line moved to bottom", got)
	}
	if got, want := ed.Cursor, len([]rune("two\nthree\nfour\n")); got != want {
		t.Fatalf("cursor = %d, want moved line start %d", got, want)
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "u", Rune: 'u'}) {
		t.Fatal("handleNormalMode(u) = false, want undo")
	}
	if got := ed.Text; got != "one\ntwo\nthree\nfour" {
		t.Fatalf("text after undo = %q, want original", got)
	}
}

func TestNormalModeMoveLineAtEdgeNoopsCleanly(t *testing.T) {
	ed := &Editor{Text: "one\ntwo", Cursor: 0, Mode: ModeNormal}
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "u", Rune: 'u'}} {
		if !handleNormalMode(&Workspace{}, ed, key) {
			t.Fatalf("handleNormalMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\ntwo" {
		t.Fatalf("text = %q, want unchanged", got)
	}
	if ed.Dirty {
		t.Fatal("Dirty = true, want no-op move to remain clean")
	}
}

func TestCommandModeMoveLineDownAndUp(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "one\ntwo\nthree",
			Cursor: len([]rune("one\n")),
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	if !submitEditorCommand(w, "mld") {
		t.Fatal("submitEditorCommand(mld) = false, want true")
	}
	ed := w.ActiveEditor()
	if got := ed.Text; got != "one\nthree\ntwo" {
		t.Fatalf("text = %q, want current line moved down", got)
	}
	if ed.Status != "moved line down" {
		t.Fatalf("Status = %q, want moved line down", ed.Status)
	}
	if !submitEditorCommand(w, "mlu") {
		t.Fatal("submitEditorCommand(mlu) = false, want true")
	}
	if got := ed.Text; got != "one\ntwo\nthree" {
		t.Fatalf("text = %q, want current line moved back up", got)
	}
	if ed.Status != "moved line up" {
		t.Fatalf("Status = %q, want moved line up", ed.Status)
	}
}

func TestCommandModeMoveLineCountClampAndUndo(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "one\ntwo\nthree\nfour",
			Cursor: 0,
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	if !submitEditorCommand(w, "ml3d") {
		t.Fatal("submitEditorCommand(ml3d) = false, want true")
	}
	if got := ed.Text; got != "two\nthree\nfour\none" {
		t.Fatalf("text = %q, want line moved to bottom", got)
	}
	if !submitEditorCommand(w, "mld") {
		t.Fatal("submitEditorCommand(mld) = false, want boundary no-op")
	}
	if got := ed.Text; got != "two\nthree\nfour\none" {
		t.Fatalf("text after boundary move = %q, want unchanged", got)
	}
	if ed.Status != "line already at boundary" {
		t.Fatalf("Status = %q, want boundary message", ed.Status)
	}
	executeVimCommand(w, ed, vimCommand{Kind: vimCommandUndo})
	if got := ed.Text; got != "one\ntwo\nthree\nfour" {
		t.Fatalf("text after undo = %q, want original", got)
	}
}

func TestVisualLineModeMoveSelectionDownKeepsSelection(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree\nfour", Cursor: len([]rune("one\n")), Mode: ModeNormal}
	startVisualSelection(ed, vimSelectionLine)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "d", Rune: 'd'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("handleVisualMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\nfour\ntwo\nthree" {
		t.Fatalf("text = %q, want selected block moved down", got)
	}
	if ed.Mode != ModeVisual || ed.SelectionMode != vimSelectionLine {
		t.Fatalf("mode = %s selection = %s, want visual line", ed.Mode, ed.SelectionMode)
	}
	start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	if got := ed.Text[start:end]; got != "two\nthree" {
		t.Fatalf("selected text = %q, want moved block selected", got)
	}
}

func TestVisualLineMovePendingCommandShownInCommandLine(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text: "one\ntwo\nthree",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	startVisualSelection(ed, vimSelectionLine)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	if !w.HandleKey(Key{Name: "m", Rune: 'm'}) || !w.HandleKey(Key{Name: "l", Rune: 'l'}) {
		t.Fatal("HandleKey(ml) = false, want pending visual move")
	}
	if got := w.CommandLineText(80); got != "ml" {
		t.Fatalf("CommandLineText() = %q, want ml", got)
	}
	if col, ok := w.CommandCursor(); !ok || col != 2 {
		t.Fatalf("CommandCursor() = %d/%t, want 2/true", col, ok)
	}
}

func TestVisualCharModeMoveSelectedLines(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree\nfour", Cursor: len([]rune("one\n")), Mode: ModeNormal}
	startVisualSelection(ed, vimSelectionChar)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "d", Rune: 'd'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("handleVisualMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\nfour\ntwo\nthree" {
		t.Fatalf("text = %q, want selected lines moved down", got)
	}
	if ed.Mode != ModeVisual || ed.SelectionMode != vimSelectionChar {
		t.Fatalf("mode = %s selection = %s, want visual char", ed.Mode, ed.SelectionMode)
	}
}

func TestCommandModeMoveVisualSelectionsByTouchedLines(t *testing.T) {
	cases := []struct {
		name string
		mode vimSelectionMode
	}{
		{"char", vimSelectionChar},
		{"line", vimSelectionLine},
		{"block", vimSelectionBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &Workspace{
				Tabs: []*Editor{{
					Text:   "one\ntwo\nthree\nfour",
					Cursor: len([]rune("one\n")),
					Mode:   ModeNormal,
				}},
				CurrentTab: 0,
			}
			ed := w.ActiveEditor()
			startVisualSelection(ed, tc.mode)
			ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
			refreshVisualSelection(ed)
			if !submitEditorCommand(w, "mld") {
				t.Fatal("submitEditorCommand(mld) = false, want true")
			}
			if got := ed.Text; got != "one\nfour\ntwo\nthree" {
				t.Fatalf("text = %q, want selected touched lines moved down", got)
			}
			if ed.Mode != ModeNormal || ed.SelectionMode != vimSelectionNone {
				t.Fatalf("Mode/SelectionMode = %q/%q, want normal/no selection", ed.Mode, ed.SelectionMode)
			}
			if ed.Status != "moved lines down" {
				t.Fatalf("Status = %q, want moved lines down", ed.Status)
			}
		})
	}
}

func TestCommandModeMoveVisualSelectionCountUp(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text:   "one\ntwo\nthree\nfour",
			Cursor: len([]rune("one\ntwo\n")),
			Mode:   ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	startVisualSelection(ed, vimSelectionLine)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	if !submitEditorCommand(w, "ml2u") {
		t.Fatal("submitEditorCommand(ml2u) = false, want true")
	}
	if got := ed.Text; got != "three\nfour\none\ntwo" {
		t.Fatalf("text = %q, want selected lines moved to top", got)
	}
	if ed.SelectionMode != vimSelectionNone {
		t.Fatalf("SelectionMode = %q, want cleared", ed.SelectionMode)
	}
}

func TestCommandModeUnknownCommandUpdatesStatusBar(t *testing.T) {
	helpers.InitStatusBar()
	w := &Workspace{
		Tabs: []*Editor{{
			Text: "one",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	if !submitEditorCommand(w, "doesnotexist") {
		t.Fatal("submitEditorCommand(doesnotexist) = false, want handled")
	}
	want := "unknown command: doesnotexist"
	if got := w.ActiveEditor().Status; got != want {
		t.Fatalf("editor Status = %q, want %q", got, want)
	}
	if got := helpers.StatusBarInst().Text(); got != want {
		t.Fatalf("status bar = %q, want %q", got, want)
	}
}

func TestVisualSelectionCommandRunsOnEnter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.VimMode = true

	w := &Workspace{
		Tabs: []*Editor{{
			Title: "Plan",
			Text:  "one\ntwo",
			Mode:  ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	startVisualSelection(ed, vimSelectionLine)
	if !w.HandleKey(Key{Name: ":", Rune: ':'}) {
		t.Fatal("HandleKey(:) = false, want command mode")
	}
	if ed.Mode != ModeCommand {
		t.Fatalf("Mode = %q, want command", ed.Mode)
	}
	if got := w.CommandLineText(80); got != "'<,'>" {
		t.Fatalf("CommandLineText() = %q, want visual range command prefix", got)
	}
	if !w.HandleKey(Key{Name: "w", Rune: 'w'}) || !w.HandleKey(Key{Name: "enter"}) {
		t.Fatal("HandleKey(:w enter) = false, want command executed")
	}
	if !w.pendingSaveAll {
		t.Fatal("pendingSaveAll = false, want save requested")
	}
	if ed.Mode != ModeNormal {
		t.Fatalf("Mode = %q, want normal after command", ed.Mode)
	}
	if ed.SelectionMode != vimSelectionNone {
		t.Fatalf("SelectionMode = %q, want cleared", ed.SelectionMode)
	}
}

func TestVisualSelectionSearchCommandRunsOnEnter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.VimMode = true

	w := &Workspace{
		Tabs: []*Editor{{
			Text: "one\ntwo",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	startVisualSelection(ed, vimSelectionChar)
	if !w.HandleKey(Key{Name: "/", Rune: '/'}) {
		t.Fatal("HandleKey(/) = false, want search command mode")
	}
	if got := w.CommandLineText(80); got != "/" {
		t.Fatalf("CommandLineText() = %q, want search prompt", got)
	}
	for _, key := range []Key{{Name: "t", Rune: 't'}, {Name: "w", Rune: 'w'}, {Name: "o", Rune: 'o'}, {Name: "enter"}} {
		if !w.HandleKey(key) {
			t.Fatalf("HandleKey(%q) = false, want search command executed", key.Name)
		}
	}
	if ed.LastSearch != "two" {
		t.Fatalf("LastSearch = %q, want two", ed.LastSearch)
	}
	if ed.Cursor != len([]rune("one\n")) {
		t.Fatalf("Cursor = %d, want start of match", ed.Cursor)
	}
	if ed.Mode != ModeNormal || ed.SelectionMode != vimSelectionNone {
		t.Fatalf("Mode/SelectionMode = %q/%q, want normal/no selection", ed.Mode, ed.SelectionMode)
	}
}

func TestVisualSelectionCommandEscCancelsAndClearsSelection(t *testing.T) {
	w := &Workspace{
		Tabs: []*Editor{{
			Text: "one\ntwo",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	startVisualSelection(ed, vimSelectionLine)
	if !w.HandleKey(Key{Name: ":", Rune: ':'}) || !w.HandleKey(Key{Name: "esc"}) {
		t.Fatal("HandleKey(: esc) = false, want command cancelled")
	}
	if ed.Mode != ModeNormal || ed.Command != "" || ed.SelectionMode != vimSelectionNone {
		t.Fatalf("Mode/Command/SelectionMode = %q/%q/%q, want normal empty no selection", ed.Mode, ed.Command, ed.SelectionMode)
	}
}

func TestVisualLineModeMoveSelectionCountUp(t *testing.T) {
	ed := &Editor{Text: "one\ntwo\nthree\nfour", Cursor: len([]rune("one\ntwo\n")), Mode: ModeNormal}
	startVisualSelection(ed, vimSelectionLine)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	refreshVisualSelection(ed)
	for _, key := range []Key{{Name: "m", Rune: 'm'}, {Name: "l", Rune: 'l'}, {Name: "2", Rune: '2'}, {Name: "u", Rune: 'u'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("handleVisualMode(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "three\nfour\none\ntwo" {
		t.Fatalf("text = %q, want selected block moved to top", got)
	}
	start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	if got := ed.Text[start:end]; got != "three\nfour\n" {
		t.Fatalf("selected text = %q, want moved block selected", got)
	}
}

func TestVisualBlockYankAndDelete(t *testing.T) {
	restore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restore()
	ed := &Editor{Text: "abcd\nwxyz", Cursor: 1, Mode: ModeNormal}
	if !handleEditorKeyForTest(ed, Key{Name: "v", Ctrl: true}) {
		t.Fatal("ctrl+v should enter visual block mode")
	}
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
	refreshVisualSelection(ed)
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "y", Rune: 'y'}) {
		t.Fatal("y in visual block mode should succeed")
	}
	if ed.Register.Kind != vimRegisterBlock || len(ed.Register.Lines) != 2 || ed.Register.Lines[0] != "bc" || ed.Register.Lines[1] != "xy" {
		t.Fatalf("block register = %#v", ed.Register)
	}

	ed2 := &Editor{Text: "abcd\nwxyz", Cursor: 1, Mode: ModeNormal}
	startVisualSelection(ed2, vimSelectionBlock)
	ed2.Cursor = vimVerticalMoveOffset(ed2.Text, ed2.Cursor, 1)
	ed2.Cursor = vimClampOffset(ed2.Text, ed2.Cursor+1)
	refreshVisualSelection(ed2)
	if !handleVisualMode(&Workspace{}, ed2, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("d in visual block mode should succeed")
	}
	if got := ed2.Text; got != "ad\nwz" {
		t.Fatalf("text = %q, want %q", got, "ad\nwz")
	}
}

func TestVisualBlockDeleteThenPPastesDeletedBlock(t *testing.T) {
	restoreWrite := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restoreWrite()
	restoreRead := helpers.SetClipboardReaderForTesting(func() (string, error) {
		return "external clipboard", nil
	})
	defer restoreRead()

	ed := &Editor{Text: "abcd\nwxyz", Cursor: 1, Mode: ModeNormal}
	startVisualSelection(ed, vimSelectionBlock)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
	refreshVisualSelection(ed)
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) {
		t.Fatal("d in visual block mode should succeed")
	}
	if got := ed.Text; got != "ad\nwz" {
		t.Fatalf("after delete text = %q, want %q", got, "ad\nwz")
	}
	ed.Mode = ModeNormal
	ed.Cursor = 1
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "p", Rune: 'p'}) {
		t.Fatal("handleNormalMode(p) = false, want true")
	}
	if got := ed.Text; got != "abcd\nwxyz" {
		t.Fatalf("after paste text = %q, want original block restored", got)
	}
}

func TestHandleNormalModeYYCopiesToClipboard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	var copied string
	restore := helpers.SetClipboardWriterForTesting(func(text string) error {
		copied = text
		return nil
	})
	defer restore()
	ed := &Editor{Text: "one\ntwo", Cursor: 1, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "y", Rune: 'y'}) {
		t.Fatal("first y should arm yank")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "y", Rune: 'y'}) {
		t.Fatal("second y should yank line")
	}
	if copied != "one\n" {
		t.Fatalf("copied = %q, want %q", copied, "one\n")
	}
	if ed.Status != "yanked line" {
		t.Fatalf("status = %q, want %q", ed.Status, "yanked line")
	}
	got := strings.Join(renderEditorPane(ed, 40, 2), "\n")
	if !strings.Contains(got, helpers.ANSIRoleSelection) {
		t.Fatalf("renderEditorPane() = %q, want yank highlight", got)
	}
}

func TestHandleNormalModeYWCopiesToClipboard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	var copied string
	restore := helpers.SetClipboardWriterForTesting(func(text string) error {
		copied = text
		return nil
	})
	defer restore()
	ed := &Editor{Text: "alpha beta", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "y", Rune: 'y'}) {
		t.Fatal("y should arm yank")
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "w", Rune: 'w'}) {
		t.Fatal("w after y should yank word")
	}
	if copied != "alpha" {
		t.Fatalf("copied = %q, want %q", copied, "alpha")
	}
	if ed.Status != "yanked text" {
		t.Fatalf("status = %q, want %q", ed.Status, "yanked text")
	}
	got := renderEditorPane(ed, 40, 1)[0]
	if !strings.Contains(got, helpers.ANSIRoleSelection) {
		t.Fatalf("renderEditorPane() = %q, want yank highlight", got)
	}
	if strings.Contains(got, helpers.ANSIRoleSelection+" ") {
		t.Fatalf("renderEditorPane() = %q, want highlight to stop before trailing space", got)
	}
}

func TestYankHighlightExpires(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:               "alpha beta",
		YankHighlightStart: 0,
		YankHighlightEnd:   5,
		YankHighlightUntil: time.Now().Add(-time.Millisecond),
	}
	got := renderEditorPane(ed, 40, 1)[0]
	if strings.Contains(got, helpers.ANSIRoleSelection) {
		t.Fatalf("renderEditorPane() = %q, want expired yank highlight hidden", got)
	}
}

func TestHandleNormalModePPasteReportsClipboardFailure(t *testing.T) {
	restore := helpers.SetClipboardReaderForTesting(func() (string, error) {
		return "", os.ErrPermission
	})
	defer restore()
	ed := &Editor{Text: "one", Cursor: 0, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: "p", Rune: 'p'}) {
		t.Fatal("handleNormalMode(p) = false, want true")
	}
	if ed.Text != "one" {
		t.Fatalf("text = %q, want unchanged on clipboard failure", ed.Text)
	}
	if !strings.Contains(ed.Status, "clipboard paste failed") {
		t.Fatalf("status = %q, want clipboard failure", ed.Status)
	}
}

func TestHandleNormalModeSpaceTogglesCheckbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Text: "- [ ] task", Cursor: 4, Mode: ModeNormal}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: " ", Rune: ' '}) {
		t.Fatal("handleNormalMode(space) = false, want true")
	}
	if got := ed.Text; got != "- [x] task" {
		t.Fatalf("text = %q, want checked task", got)
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: " ", Rune: ' '}) {
		t.Fatal("second handleNormalMode(space) = false, want true")
	}
	if got := ed.Text; got != "- [ ] task" {
		t.Fatalf("text = %q, want unchecked task", got)
	}
}

func TestHandleNormalModeSpaceTogglesCheckboxAfterWrappedLineAtClickedCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 12,
		Tabs: []*Editor{{
			Text: "alpha beta gamma\n- [ ] one\n- [ ] two",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	// First line wraps into two visual rows at this width, so the third logical line starts on visual row 3.
	if !w.MoveEditorCursorToVisualPosition(3, 4) {
		t.Fatal("MoveEditorCursorToVisualPosition() = false, want true")
	}
	if !handleNormalMode(w, ed, Key{Name: " ", Rune: ' '}) {
		t.Fatal("handleNormalMode(space) = false, want true")
	}
	if got := ed.Text; got != "alpha beta gamma\n- [ ] one\n- [x] two" {
		t.Fatalf("text = %q, want only the clicked checkbox line toggled", got)
	}
}

func TestNormalModeDownMovesByVisualRowsBeforeCheckboxToggle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 12,
		Tabs: []*Editor{{
			Text: "alpha beta gamma\n- [ ] one\n- [ ] two",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	rows := renderEditorPane(w.ActiveEditor(), 12, 5)
	firstCheckboxRow := -1
	for i, row := range rows {
		if strings.Contains(helpers.StripANSI(row), "- [ ] one") {
			firstCheckboxRow = i
			break
		}
	}
	if firstCheckboxRow < 2 {
		t.Fatalf("first checkbox visual row = %d, want below wrapped line in %#v", firstCheckboxRow, rows)
	}
	for i := 0; i < firstCheckboxRow; i++ {
		if !handleNormalMode(w, w.ActiveEditor(), Key{Name: "down"}) {
			t.Fatalf("handleNormalMode(down #%d) = false, want true", i+1)
		}
	}
	if !handleNormalMode(w, w.ActiveEditor(), Key{Name: " ", Rune: ' '}) {
		t.Fatal("handleNormalMode(space) = false, want true")
	}
	if got := w.ActiveEditor().Text; got != "alpha beta gamma\n- [x] one\n- [ ] two" {
		t.Fatalf("text = %q, want first checkbox toggled after visual-row movement", got)
	}
}

func TestHandleNormalModeSpaceTogglesWrappedChecklistFromContinuationRow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 14,
		Tabs: []*Editor{{
			Text: "intro line\n- [ ] task that wraps widely",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	ed := w.ActiveEditor()
	if !w.MoveEditorCursorToVisualPosition(2, 6) {
		t.Fatal("MoveEditorCursorToVisualPosition() = false, want true on wrapped continuation row")
	}
	if !handleNormalMode(w, ed, Key{Name: " ", Rune: ' '}) {
		t.Fatal("handleNormalMode(space) = false, want wrapped checklist toggled")
	}
	if got := ed.Text; got != "intro line\n- [x] task that wraps widely" {
		t.Fatalf("text = %q, want wrapped checklist toggled on current logical line", got)
	}
}

func TestEditorOffsetAtVisualPositionIgnoresSpellPopupOverlayRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		EditorRenderWidth: 20,
		Tabs: []*Editor{{
			Text:                "Naice\n- [ ] one\n- [ ] two",
			Mode:                ModeInsert,
			AutoCompleteKind:    autoCompleteSpell,
			AutoCompleteMatches: []string{"Nice", "Naive"},
			AutoCompleteIndex:   0,
			AutoCompleteStart:   0,
			AutoCompleteEnd:     len([]rune("Naice")),
		}},
		CurrentTab: 0,
	}
	offset, ok := w.EditorOffsetAtVisualPosition(2, 4)
	if !ok {
		t.Fatal("EditorOffsetAtVisualPosition() ok = false, want true")
	}
	want := len([]rune("Naice\n- [ ] one\n")) + 2
	if offset != want {
		t.Fatalf("offset = %d, want %d with spell popup overlay active", offset, want)
	}
}

func TestCheckboxToggleNearLineStartBelowSpellHighlightUsesCurrentLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	if _, err := AddCustomWord("known"); err != nil {
		t.Fatal(err)
	}
	w := &Workspace{
		EditorRenderWidth: 24,
		Tabs: []*Editor{{
			Text: "known badwrd\n- [ ] first\n- [ ] second",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	_ = renderEditorPane(w.ActiveEditor(), 24, 4)
	if !w.MoveEditorCursorToVisualPosition(2, 3) {
		t.Fatal("MoveEditorCursorToVisualPosition() = false, want true")
	}
	if !handleNormalMode(w, w.ActiveEditor(), Key{Name: " ", Rune: ' '}) {
		t.Fatal("handleNormalMode(space) = false, want true")
	}
	if got := w.ActiveEditor().Text; got != "known badwrd\n- [ ] first\n- [x] second" {
		t.Fatalf("text = %q, want second checkbox toggled", got)
	}
}

func TestCheckboxToggleExactWrappedChecklistScenarioUsesVisibleLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	if _, err := AddCustomWord("ask"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("Dmitrijs"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("about"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("ownership"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("Read"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("Lusine"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("document"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("transition"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("LemFi"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCustomWord("EU"); err != nil {
		t.Fatal(err)
	}
	w := &Workspace{
		EditorRenderWidth: 50,
		Tabs: []*Editor{{
			Text: "- [ ] ask Dmitrijs about Simetric ownership\n- [ ] Read Lusine document about transition to LemFi EU",
			Mode: ModeNormal,
		}},
		CurrentTab: 0,
	}
	rows := renderEditorPane(w.ActiveEditor(), 50, 5)
	secondLineRow := -1
	for i, row := range rows {
		if strings.Contains(helpers.StripANSI(row), "- [ ] Read") {
			secondLineRow = i
			break
		}
	}
	if secondLineRow < 0 {
		t.Fatalf("could not find second checklist row in %#v", rows)
	}
	for _, col := range []int{0, 2, 6, 10} {
		w.ActiveEditor().Text = "- [ ] ask Dmitrijs about Simetric ownership\n- [ ] Read Lusine document about transition to LemFi EU"
		if !w.MoveEditorCursorToVisualPosition(secondLineRow, col) {
			t.Fatalf("MoveEditorCursorToVisualPosition(row=%d, col=%d) = false, want true", secondLineRow, col)
		}
		if !handleNormalMode(w, w.ActiveEditor(), Key{Name: " ", Rune: ' '}) {
			t.Fatalf("handleNormalMode(space) = false at col %d, want true", col)
		}
		if got := w.ActiveEditor().Text; got != "- [ ] ask Dmitrijs about Simetric ownership\n- [x] Read Lusine document about transition to LemFi EU" {
			t.Fatalf("col %d text = %q, want second checkbox toggled", col, got)
		}
	}
}

func TestCheckboxToggleAtLineBoundaryKeepsCurrentChecklistLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:   "- [ ] ask Dmitrijs about Simetric ownership\n- [ ] Read Lusine document about transition to LemFi EU",
		Cursor: len([]rune("- [ ] ask Dmitrijs about Simetric ownership")),
		Mode:   ModeNormal,
	}
	if !handleNormalMode(&Workspace{}, ed, Key{Name: " ", Rune: ' '}) {
		t.Fatal("handleNormalMode(space) = false, want true")
	}
	if got := ed.Text; got != "- [x] ask Dmitrijs about Simetric ownership\n- [ ] Read Lusine document about transition to LemFi EU" {
		t.Fatalf("text = %q, want current checklist line toggled at boundary", got)
	}
}

func TestVisualBlockYankCopiesJoinedTextToClipboard(t *testing.T) {
	var copied string
	restore := helpers.SetClipboardWriterForTesting(func(text string) error {
		copied = text
		return nil
	})
	defer restore()
	ed := &Editor{Text: "abcd\nwxyz", Cursor: 1, Mode: ModeNormal}
	startVisualSelection(ed, vimSelectionBlock)
	ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
	ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
	refreshVisualSelection(ed)
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "y", Rune: 'y'}) {
		t.Fatal("y in visual block mode should succeed")
	}
	if copied != "bc\nxy" {
		t.Fatalf("copied = %q, want %q", copied, "bc\nxy")
	}
}

func TestRenderEditorPaneHighlightsVisualSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{
		Text:            "alpha\nbeta",
		Cursor:          0,
		Mode:            ModeVisual,
		SelectionMode:   vimSelectionLine,
		SelectionMark:   0,
		SelectionCursor: 6,
	}
	got := strings.Join(renderEditorPane(ed, 40, 3), "\n")
	if !strings.Contains(got, helpers.ANSIRoleVisualSelection) {
		t.Fatalf("renderEditorPane() = %q, want visual selection highlight", got)
	}
}

func handleEditorKeyForTest(ed *Editor, key Key) bool {
	w := &Workspace{Tabs: []*Editor{ed}, CurrentTab: 0}
	return w.handleEditorKey(key)
}

func submitEditorCommand(w *Workspace, command string) bool {
	if w == nil || !w.HandleKey(Key{Name: ":", Rune: ':'}) {
		return false
	}
	for _, r := range command {
		if !w.HandleKey(Key{Name: string(r), Rune: r}) {
			return false
		}
	}
	return w.HandleKey(Key{Name: "enter"})
}

func workspaceHasOpenTab(w *Workspace, path string) bool {
	for _, tab := range w.Tabs {
		if tab != nil && tab.Path == path {
			return true
		}
	}
	return false
}
