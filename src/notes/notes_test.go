package notes

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

var (
	gtkInitOnce sync.Once
	gtkInitOK   bool
)

func requireGTK(t *testing.T) {
	t.Helper()
	gtkInitOnce.Do(func() {
		gtk.DisableSetlocale()
		gtkInitOK = gtk.InitCheck()
		if gtkInitOK {
			helpers.InitGlobals()
			helpers.InitStatusBar()
		}
	})
	if !gtkInitOK {
		t.Skip("GTK could not be initialized in this environment")
	}
}

func countSidebarRows(sidebar *gtk.ListBox) int {
	if sidebar == nil {
		return 0
	}
	total := 0
	for row := sidebar.RowAtIndex(0); row != nil; row = sidebar.RowAtIndex(total) {
		total++
	}
	return total
}

func TestFileNameUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := fileName()
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes", "Note 1.md")
	if got != want {
		t.Fatalf("fileName() = %q, want %q", got, want)
	}
}

func TestGetNoteTextReturnsFileContents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Dir(fileName())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	want := "remember the milk"
	if err := os.WriteFile(fileName(), []byte(want), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got := getNoteText()
	if got != want {
		t.Fatalf("getNoteText() = %q, want %q", got, want)
	}
}

func TestGetNoteTextMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if got := getNoteText(); got != "" {
		t.Fatalf("getNoteText() = %q, want empty string", got)
	}
}

func TestGenerateUIExpandsNotesPanel(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	if !note.F.HExpand() || !note.F.VExpand() {
		t.Fatal("notes frame should expand in both directions")
	}
	if note.notebook == nil {
		t.Fatal("notes workspace should create a notebook")
	}
	if note.sidebar == nil {
		t.Fatal("notes workspace should create a sidebar")
	}
}

func TestGenerateUICreatesPreviewPane(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	if len(note.tabs) != 1 {
		t.Fatalf("GenerateUI() tabs = %d, want 1", len(note.tabs))
	}
	if note.preview == nil || note.previewBuffer == nil {
		t.Fatal("GenerateUI() should create markdown preview widgets")
	}
	if tab := note.activeTab(); tab == nil || tab.previewScroll == nil || tab.editorScroll == nil {
		t.Fatal("GenerateUI() should create editor and preview scrollers")
	}
	if note.preview.Editable() {
		t.Fatal("preview should be read-only")
	}
	if note.preview.CursorVisible() {
		t.Fatal("preview cursor should be hidden")
	}
	commandRow := note.commandEntry.Parent()
	if commandRow == nil {
		t.Fatal("command entry should have a parent row")
	}
	if parent := gtk.BaseWidget(commandRow).Parent(); parent == nil {
		t.Fatal("command row should be attached to the notes layout")
	} else {
		if gtk.BaseWidget(commandRow).NextSibling() != nil {
			t.Fatal("command row should be the last child in the notes layout")
		}
	}
}

func TestGenerateUICreatesLineNumberGutterAndWrapToggle(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	tab := note.activeTab()
	if tab == nil {
		t.Fatal("expected an active tab")
	}
	if tab.lineNumberGutter == nil || tab.lineNumberScroll == nil {
		t.Fatal("editor should create a line number gutter")
	}
	if tab.wrapToggle == nil || !tab.wrapToggle.Active() {
		t.Fatal("editor should create an enabled wrap toggle by default")
	}
}

func TestGenerateUIUsesSavedEditorWidth(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.EditorWidth = 512

	note := GenerateUI()
	if len(note.tabs) != 1 {
		t.Fatalf("GenerateUI() tabs = %d, want 1", len(note.tabs))
	}
	if got := note.tabs[0].paned.Position(); got != 512 {
		t.Fatalf("editor paned position = %d, want 512", got)
	}
}

func TestResolvedEditorWidthUsesEqualSplitWhenUnset(t *testing.T) {
	if got := resolvedEditorWidth(0, 800); got != 400 {
		t.Fatalf("resolvedEditorWidth(0, 800) = %d, want 400", got)
	}
	if got := resolvedEditorWidth(512, 800); got != 512 {
		t.Fatalf("resolvedEditorWidth(512, 800) = %d, want 512", got)
	}
}

func TestResolvedEditorWidthClampsSavedWidthToKeepPreviewVisible(t *testing.T) {
	if got := resolvedEditorWidth(900, 800); got != 620 {
		t.Fatalf("resolvedEditorWidth(900, 800) = %d, want 620", got)
	}
}

func TestResolvedEditorWidthUsesEqualSplitForSmallWindows(t *testing.T) {
	if got := resolvedEditorWidth(500, 300); got != 150 {
		t.Fatalf("resolvedEditorWidth(500, 300) = %d, want 150", got)
	}
}

func TestNeedsEditorWidthReapplyDetectsResizeClamp(t *testing.T) {
	if !needsEditorWidthReapply(512, 512, 600) {
		t.Fatal("needsEditorWidthReapply() should request a reapply when resize clamp changes the desired width")
	}
}

func TestNeedsEditorWidthReapplySkipsWhenWidthAlreadyMatches(t *testing.T) {
	if needsEditorWidthReapply(620, 900, 800) {
		t.Fatal("needsEditorWidthReapply() should skip when current width already matches the clamped desired width")
	}
}

func TestContinueMarkdownListBullet(t *testing.T) {
	text, cursor, ok := continueMarkdownList("- item", len([]rune("- item")))
	if !ok {
		t.Fatal("continueMarkdownList() should continue bullet lists")
	}
	if text != "- item\n- " {
		t.Fatalf("continueMarkdownList() text = %q, want %q", text, "- item\n- ")
	}
	if cursor != len([]rune("- item\n- ")) {
		t.Fatalf("continueMarkdownList() cursor = %d", cursor)
	}
}

func TestContinueMarkdownListOrdered(t *testing.T) {
	text, _, ok := continueMarkdownList("9. item", len([]rune("9. item")))
	if !ok {
		t.Fatal("continueMarkdownList() should continue ordered lists")
	}
	if text != "9. item\n10. " {
		t.Fatalf("continueMarkdownList() text = %q, want %q", text, "9. item\n10. ")
	}
}

func TestContinueMarkdownListChecklistUnchecked(t *testing.T) {
	text, _, ok := continueMarkdownList("- [x] done", len([]rune("- [x] done")))
	if !ok {
		t.Fatal("continueMarkdownList() should continue checklist items")
	}
	if text != "- [x] done\n- [ ] " {
		t.Fatalf("continueMarkdownList() text = %q, want %q", text, "- [x] done\n- [ ] ")
	}
}

func TestContinueMarkdownListStopsOnEmptyItem(t *testing.T) {
	text, cursor, ok := continueMarkdownList("- ", len([]rune("- ")))
	if !ok {
		t.Fatal("continueMarkdownList() should handle empty list items")
	}
	if text != "" || cursor != 0 {
		t.Fatalf("continueMarkdownList() = (%q, %d), want empty text at cursor 0", text, cursor)
	}
}

func TestContinueMarkdownListSkipsCodeFence(t *testing.T) {
	text := "```go\n- item"
	if _, _, ok := continueMarkdownList(text, len([]rune(text))); ok {
		t.Fatal("continueMarkdownList() should not continue lists inside code fences")
	}
}

func TestPersistEditorWidthUpdatesSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	persistEditorWidth(520)

	if got := settings.Inst().NotesApp.EditorWidth; got != 520 {
		t.Fatalf("NotesApp.EditorWidth = %d, want 520", got)
	}
}

func TestShouldPersistEditorWidthSkipsBeforeRestore(t *testing.T) {
	tab := &noteTab{}
	if shouldPersistEditorWidth(tab, 492) {
		t.Fatal("shouldPersistEditorWidth() should skip startup notifications before width restore")
	}
}

func TestShouldPersistEditorWidthSkipsWhileApplying(t *testing.T) {
	tab := &noteTab{widthRestored: true, widthPersistOK: true, applyingWidth: true}
	if shouldPersistEditorWidth(tab, 492) {
		t.Fatal("shouldPersistEditorWidth() should skip programmatic width changes")
	}
}

func TestShouldPersistEditorWidthAllowsUserResizeAfterRestore(t *testing.T) {
	tab := &noteTab{widthRestored: true, widthPersistOK: true}
	if !shouldPersistEditorWidth(tab, 492) {
		t.Fatal("shouldPersistEditorWidth() should allow persistence after width restore completes")
	}
}

func TestEditorWidthForPersistenceIgnoresHiddenPreview(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.paned.SetPosition(9999)
	child := note.paned.EndChild()
	if child == nil {
		t.Fatal("expected preview child")
	}
	gtk.BaseWidget(child).SetVisible(false)

	if width, ok := editorWidthForPersistence(note.paned); ok || width != 0 {
		t.Fatalf("editorWidthForPersistence() = (%d, %t), want (0, false)", width, ok)
	}
}

func TestEnsureInitialNoteFilesMigratesLegacyNote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyDir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "notes.txt"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	files, err := ensureInitialNoteFiles()
	if err != nil {
		t.Fatalf("ensureInitialNoteFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].Path != fileName() {
		t.Fatalf("ensureInitialNoteFiles() = %#v", files)
	}
	if got := getNoteText(); got != "legacy" {
		t.Fatalf("migrated note text = %q, want legacy", got)
	}
}

func TestCloseAndReopenNoteTabs(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.newNote()
	if len(note.tabs) != 2 {
		t.Fatalf("tabs after newNote = %d, want 2", len(note.tabs))
	}
	if !note.notebook.ShowTabs() {
		t.Fatal("notebook should show tabs when multiple notes are open")
	}

	closedPath := note.tabs[0].path
	note.notebook.SetCurrentPage(0)
	note.switchToTab(0)
	note.closeCurrentTab()
	if len(note.tabs) != 1 {
		t.Fatalf("tabs after closeCurrentTab = %d, want 1", len(note.tabs))
	}
	if note.notebook.ShowTabs() {
		t.Fatal("notebook should hide tabs when only one note is open")
	}

	note.refreshSidebar()
	row := note.findSidebarTreePath(closedPath)
	if row == nil {
		t.Fatal("expected closed note path in sidebar tree")
	}
	note.activateSidebarPath(row)
	if len(note.tabs) != 2 {
		t.Fatalf("tabs after reopen = %d, want 2", len(note.tabs))
	}
	if note.pageForPath(closedPath) < 0 {
		t.Fatal("reopened note should exist in notebook")
	}
}

func TestRenameCurrentTabRenamesFileAndTab(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	oldPath := note.activeTab().path
	if err := note.renameCurrentTab("Project Notes"); err != nil {
		t.Fatalf("renameCurrentTab() error = %v", err)
	}

	newPath := filepath.Join(notesDir(), "Project Notes.md")
	if note.activeTab().path != newPath {
		t.Fatalf("active tab path = %q, want %q", note.activeTab().path, newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("renamed file stat error = %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be gone, stat err = %v", err)
	}
}

func TestRenameCurrentTabKeepsUnicodeLetters(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	if err := note.renameCurrentTab("Pārbaude ļ ķ ē ī ā"); err != nil {
		t.Fatalf("renameCurrentTab() error = %v", err)
	}

	want := filepath.Join(notesDir(), "Pārbaude ļ ķ ē ī ā.md")
	if note.activeTab().path != want {
		t.Fatalf("unicode rename path = %q, want %q", note.activeTab().path, want)
	}
}

func TestDeleteCurrentNoteMovesFileToTrashAndKeepsWorkspaceAlive(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.newNote()
	note.notebook.SetCurrentPage(1)
	note.switchToTab(1)
	deletePath := note.activeTab().path

	if err := note.deleteCurrentNote(); err != nil {
		t.Fatalf("deleteCurrentNote() error = %v", err)
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("deleted note should leave original path, stat err = %v", err)
	}
	trashPath := filepath.Join(settings.NotesTrashDir(), "Note 2.md")
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("trash note stat error = %v", err)
	}
	if len(note.tabs) == 0 {
		t.Fatal("workspace should keep at least one note open")
	}
}

func TestSidebarSelectionOpensSecondNote(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.newNote()
	notePath := note.tabs[1].path
	row := note.findSidebarTreePath(notePath)
	if row == nil {
		t.Fatal("sidebar should contain the second note")
	}
	note.activateSidebarPath(row)
	if note.currentTab != 1 {
		t.Fatalf("current tab = %d, want 1", note.currentTab)
	}
}

func TestSidebarToggleHidesPanel(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.setSidebarVisible(false)
	if gtk.BaseWidget(note.sidebarBox).Visible() {
		t.Fatal("sidebar pane content should be hidden")
	}
	if !gtk.BaseWidget(note.sidebarToggle).Visible() {
		t.Fatal("sidebar toggle should stay visible")
	}
	if got := note.sidebarPane.Position(); got != 0 {
		t.Fatalf("hidden sidebar pane position = %d, want 0", got)
	}
	note.setSidebarVisible(true)
	if !gtk.BaseWidget(note.sidebarBox).Visible() {
		t.Fatal("sidebar pane content should be visible")
	}
	if got := note.sidebarPane.Position(); got != 260 {
		t.Fatalf("shown sidebar pane position = %d, want 260", got)
	}
	if !gtk.BaseWidget(note.sidebarDragToggle).Visible() || !gtk.BaseWidget(note.sidebarNewNote).Visible() || !gtk.BaseWidget(note.sidebarNewFolder).Visible() {
		t.Fatal("bottom controls should stay visible")
	}
}

func TestSidebarTogglePersistsVisibility(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.sidebarToggle.SetActive(false)

	if settings.Inst().NotesApp.SidebarVisible {
		t.Fatal("sidebar visibility should persist as hidden after toggle")
	}

	note.sidebarToggle.SetActive(true)
	if !settings.Inst().NotesApp.SidebarVisible {
		t.Fatal("sidebar visibility should persist as visible after toggle")
	}
}

func TestSidebarBottomActionsOrderAndDragToggleIcon(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	parent := gtk.BaseWidget(note.sidebarToggle).Parent()
	if parent == nil {
		t.Fatal("sidebar toggle should be attached")
	}
	first := gtk.BaseWidget(parent).FirstChild()
	if first == nil || glib.InternObject(first).Native() != glib.InternObject(note.sidebarToggle).Native() {
		t.Fatal("hide sidebar toggle should be the first bottom action")
	}
	second := gtk.BaseWidget(first).NextSibling()
	if second == nil || glib.InternObject(second).Native() != glib.InternObject(note.sidebarDragToggle).Native() {
		t.Fatal("drag toggle should be the second bottom action")
	}
	image, ok := note.sidebarDragToggle.Child().(*gtk.Image)
	if !ok {
		t.Fatal("drag toggle should use an image icon")
	}
	if image.Paintable() == nil {
		t.Fatal("drag toggle icon should load a paintable")
	}
}

func TestListNoteFilesIncludesNestedFolders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Client"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	rootNote := filepath.Join(notesDir(), "Root.md")
	nestedNote := filepath.Join(notesDir(), "Work", "Client", "Spec.md")
	if err := os.WriteFile(rootNote, []byte("root"), 0o644); err != nil {
		t.Fatalf("WriteFile(root) error = %v", err)
	}
	if err := os.WriteFile(nestedNote, []byte("nested"), 0o644); err != nil {
		t.Fatalf("WriteFile(nested) error = %v", err)
	}

	files, err := listNoteFiles()
	if err != nil {
		t.Fatalf("listNoteFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("listNoteFiles() len = %d, want 2", len(files))
	}
	if files[1].Folder != filepath.Join("Work", "Client") {
		t.Fatalf("nested file folder = %q, want %q", files[1].Folder, filepath.Join("Work", "Client"))
	}
}

func TestListNoteFilesSkipsTrashTree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	trashDir := filepath.Join(settings.NotesTrashDir(), "Work")
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(trashDir, "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Root.md"), []byte("root"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	files, err := listNoteFiles()
	if err != nil {
		t.Fatalf("listNoteFiles() error = %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0].Path) != "Root.md" {
		t.Fatalf("listNoteFiles() = %#v, want only Root.md", files)
	}
}

func TestCreateFolderAndNewNoteUseSelectedFolder(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	if err := note.createFolder("Work/Client"); err != nil {
		t.Fatalf("createFolder() error = %v", err)
	}
	folderPath := filepath.Join(notesDir(), "Work", "Client")
	if row := note.findSidebarTreePath(folderPath); row == nil {
		t.Fatalf("sidebar should contain empty folder path %q", folderPath)
	}
	note.newNote()

	if got := relativeNoteFolder(note.activeTab().path); got != filepath.Join("Work", "Client") {
		t.Fatalf("active note folder = %q, want %q", got, filepath.Join("Work", "Client"))
	}
	if _, err := os.Stat(filepath.Join(notesDir(), "Work", "Client")); err != nil {
		t.Fatalf("created folder stat error = %v", err)
	}
}

func TestMoveCurrentNoteToFolderMovesFile(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	oldPath := note.activeTab().path
	if err := note.moveCurrentNoteToFolder("Projects/Alpha"); err != nil {
		t.Fatalf("moveCurrentNoteToFolder() error = %v", err)
	}

	newPath := filepath.Join(notesDir(), "Projects", "Alpha", "Note 1.md")
	if note.activeTab().path != newPath {
		t.Fatalf("active tab path = %q, want %q", note.activeTab().path, newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("moved file stat error = %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path should be gone, stat err = %v", err)
	}
}

func TestSidebarFolderRowCollapsesAndExpandsChildren(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.refreshSidebar()
	row := note.findSidebarTreePath(filepath.Join(notesDir(), "Work"))
	if row == nil {
		t.Fatal("expected Work folder entry in sidebar")
	}
	if note.sidebarCollapsed["Work"] {
		t.Fatal("folder should be expanded by default")
	}
	note.activateSidebarPath(row)
	if !note.sidebarCollapsed["Work"] {
		t.Fatal("folder should collapse on activation")
	}
	row = note.findSidebarTreePath(filepath.Join(notesDir(), "Work"))
	note.activateSidebarPath(row)
	if note.sidebarCollapsed["Work"] {
		t.Fatal("folder should expand on second activation")
	}
}

func TestRenameSelectedFolderMovesFilesAndOpenTabs(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldPath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(oldPath, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.openNotePath(oldPath)
	note.selectedFolder = "Work"

	if err := note.renameSelectedFolder("Projects/Work"); err != nil {
		t.Fatalf("renameSelectedFolder() error = %v", err)
	}

	newPath := filepath.Join(notesDir(), "Projects", "Work", "Plan.md")
	if note.activeTab().path != newPath {
		t.Fatalf("active tab path = %q, want %q", note.activeTab().path, newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("renamed folder file stat error = %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be gone, stat err = %v", err)
	}
	if note.selectedFolder != filepath.Join("Projects", "Work") {
		t.Fatalf("selectedFolder = %q, want %q", note.selectedFolder, filepath.Join("Projects", "Work"))
	}
}

func TestDeleteFolderPathMovesFolderToTrashAndClosesTabs(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Client"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	deletePath := filepath.Join(notesDir(), "Work", "Client", "Plan.md")
	if err := os.WriteFile(deletePath, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.openNotePath(deletePath)
	if err := note.deleteFolderPath(filepath.Join(notesDir(), "Work")); err != nil {
		t.Fatalf("deleteFolderPath() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(notesDir(), "Work")); !os.IsNotExist(err) {
		t.Fatalf("folder should leave original path, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(settings.NotesTrashDir(), "Work", "Client", "Plan.md")); err != nil {
		t.Fatalf("trash folder note stat error = %v", err)
	}
	if note.pageForPath(deletePath) >= 0 {
		t.Fatal("deleted folder note should not remain open")
	}
	if len(note.tabs) == 0 {
		t.Fatal("workspace should still have at least one note open")
	}
}

func TestDeleteSidebarContextItemDeletesFolderRowPath(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work", "Client"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	folderPath := filepath.Join(notesDir(), "Work")
	nestedNote := filepath.Join(notesDir(), "Work", "Client", "Plan.md")
	if err := os.WriteFile(nestedNote, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.sidebarMenuKind = sidebarEntryFolder
	note.sidebarMenuPath = folderPath
	if err := note.deleteSidebarContextItem(); err != nil {
		t.Fatalf("deleteSidebarContextItem() error = %v", err)
	}
	if _, err := os.Stat(folderPath); !os.IsNotExist(err) {
		t.Fatalf("folder should leave original path, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(settings.NotesTrashDir(), "Work", "Client", "Plan.md")); err != nil {
		t.Fatalf("trash folder note stat error = %v", err)
	}
}

func TestMoveSidebarItemMovesFolderIntoTargetFolder(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(notesDir(), "Archive"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldPath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(oldPath, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.openNotePath(oldPath)
	if err := note.moveSidebarItem(sidebarEntryFolder, filepath.Join(notesDir(), "Work"), "Archive"); err != nil {
		t.Fatalf("moveSidebarItem(folder) error = %v", err)
	}

	newPath := filepath.Join(notesDir(), "Archive", "Work", "Plan.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("moved folder file stat error = %v", err)
	}
	if note.pageForPath(newPath) < 0 {
		t.Fatal("moved folder note should update open tab path")
	}
}

func TestRenameSelectedFolderDoesNotDuplicateSidebarRows(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldPath := filepath.Join(notesDir(), "Work", "Plan.md")
	if err := os.WriteFile(oldPath, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.openNotePath(oldPath)
	note.selectedFolder = "Work"
	if err := note.renameSelectedFolder("Projects/Work"); err != nil {
		t.Fatalf("renameSelectedFolder() error = %v", err)
	}

	if got := countSidebarRows(note.sidebar); got != 3 {
		t.Fatalf("sidebar row count = %d, want 3", got)
	}
	if oldTreePath := note.findSidebarTreePath(filepath.Join(notesDir(), "Work")); oldTreePath != nil {
		t.Fatal("old folder path should not remain in sidebar")
	}
}

func TestApplySidebarContextRenameRenamesNote(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.sidebarMenuKind = sidebarEntryNote
	note.sidebarMenuPath = note.activeTab().path
	note.sidebarMenuFolder = relativeNoteFolder(note.activeTab().path)
	note.sidebarMenuEntry = gtk.NewEntry()
	note.sidebarMenuEntry.SetText("Renamed From Menu")

	if err := note.applySidebarContextRename(); err != nil {
		t.Fatalf("applySidebarContextRename() error = %v", err)
	}
	if got := note.activeTab().title; got != "Renamed From Menu" {
		t.Fatalf("active tab title = %q, want %q", got, "Renamed From Menu")
	}
}

func TestSidebarContextMoveMovesNoteToFolder(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Archive"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	note := GenerateUI()
	sourcePath := note.activeTab().path
	note.sidebarMenuKind = sidebarEntryNote
	note.sidebarMenuPath = sourcePath
	note.sidebarMenuFolder = relativeNoteFolder(sourcePath)
	note.sidebarMenuEntry = gtk.NewEntry()
	note.sidebarMenuEntry.SetText("Archive")

	if err := note.moveSidebarItem(note.sidebarMenuKind, note.sidebarMenuPath, sanitizeFolderPath(note.sidebarMenuEntry.Text())); err != nil {
		t.Fatalf("moveSidebarItem() error = %v", err)
	}
	want := filepath.Join(notesDir(), "Archive", "Note 1.md")
	if note.activeTab().path != want {
		t.Fatalf("active tab path = %q, want %q", note.activeTab().path, want)
	}
}

func TestSidebarContextMoveMovesNoteToRootFolder(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Archive"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	note := GenerateUI()
	if err := note.moveCurrentNoteToFolder("Archive"); err != nil {
		t.Fatalf("moveCurrentNoteToFolder() error = %v", err)
	}
	sourcePath := note.activeTab().path
	note.sidebarMenuKind = sidebarEntryNote
	note.sidebarMenuPath = sourcePath
	note.sidebarMenuFolder = relativeNoteFolder(sourcePath)
	note.sidebarMenuEntry = gtk.NewEntry()
	note.sidebarMenuEntry.SetText("")

	if err := note.moveSidebarItem(note.sidebarMenuKind, note.sidebarMenuPath, sanitizeFolderPath(note.sidebarMenuEntry.Text())); err != nil {
		t.Fatalf("moveSidebarItem() error = %v", err)
	}
	want := filepath.Join(notesDir(), "Note 1.md")
	if note.activeTab().path != want {
		t.Fatalf("active tab path = %q, want %q", note.activeTab().path, want)
	}
}

func TestShowSidebarRowMenuSetsRenamePlaceholder(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	row := note.findSidebarTreePath(note.activeTab().path)
	if row == nil {
		t.Fatal("expected note tree path")
	}
	note.showSidebarRowMenu(note.activeTab().path, sidebarEntryNote, "", row)
	if got := note.sidebarMenuEntry.PlaceholderText(); got != "Note name" {
		t.Fatalf("placeholder = %q, want %q", got, "Note name")
	}
}

func TestNewNoteNamedInFolderCreatesNamedNote(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	if err := note.newNoteNamedInFolder("Work", "Ideas"); err != nil {
		t.Fatalf("newNoteNamedInFolder() error = %v", err)
	}

	want := filepath.Join(notesDir(), "Work", "Ideas.md")
	if note.activeTab().path != want {
		t.Fatalf("active tab path = %q, want %q", note.activeTab().path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("created note stat error = %v", err)
	}
}

func TestSubmitSidebarCreateCreatesFolderAndNote(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.sidebarCreateEntry = gtk.NewEntry()
	note.sidebarCreateMode = sidebarEntryFolder
	note.sidebarCreateFolder = "Work"
	note.sidebarCreateEntry.SetText("Client")
	if err := note.submitSidebarCreate(); err != nil {
		t.Fatalf("submitSidebarCreate(folder) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(notesDir(), "Work", "Client")); err != nil {
		t.Fatalf("created folder stat error = %v", err)
	}

	note.sidebarCreateMode = sidebarEntryNote
	note.sidebarCreateFolder = filepath.Join("Work", "Client")
	note.sidebarCreateEntry.SetText("Plan")
	if err := note.submitSidebarCreate(); err != nil {
		t.Fatalf("submitSidebarCreate(note) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(notesDir(), "Work", "Client", "Plan.md")); err != nil {
		t.Fatalf("created note stat error = %v", err)
	}
}

func TestFolderRowsUseAddIcon(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	note := GenerateUI()
	row := note.findSidebarTreePath(filepath.Join(notesDir(), "Work"))
	if row == nil {
		t.Fatal("expected folder tree path")
	}
	button := note.sidebarActionButtons[sidebarEntryKey(sidebarEntryFolder, filepath.Join(notesDir(), "Work"))]
	if button == nil {
		t.Fatal("expected folder action button")
	}
	child := button.Child()
	image, ok := child.(*gtk.Image)
	if !ok {
		t.Fatal("expected folder action child image")
	}
	if image.Paintable() == nil {
		t.Fatal("folder action icon should load a paintable")
	}
}

func TestSidebarDragHintSupportsFolderAndRootTargets(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	if err := os.MkdirAll(filepath.Join(notesDir(), "Work"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(notesDir(), "Work", "Plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	note := GenerateUI()
	note.sidebarDragActive = true
	folderTarget := sidebarDropTarget{
		Folder: filepath.Join("Work"),
		Anchor: sidebarEntryKey(sidebarEntryFolder, filepath.Join(notesDir(), "Work")),
	}
	note.setSidebarDragTarget(folderTarget)
	if !gtk.BaseWidget(note.sidebarDropHint).Visible() {
		t.Fatal("drop hint should be visible during drag")
	}
	if got := note.sidebarDropHint.Text(); got != "Drop into Work" {
		t.Fatalf("folder hint label = %q, want %q", got, "Drop into Work")
	}

	note.setSidebarDragTarget(sidebarDropTarget{Root: true})
	if got := note.sidebarDropHint.Text(); got != "Drop into root" {
		t.Fatalf("root hint label = %q, want %q", got, "Drop into root")
	}
}

func TestSidebarUsesResizablePane(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	if note.sidebarPane == nil {
		t.Fatal("notes workspace should create a resizable sidebar pane")
	}
	if got := note.sidebarPane.Position(); got != 260 {
		t.Fatalf("sidebar pane position = %d, want 260", got)
	}
}

func TestQueueSidebarDragRequiresActiveDragMode(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	entry := sidebarEntry{
		Kind:   sidebarEntryNote,
		Path:   note.activeTab().path,
		Folder: relativeNoteFolder(note.activeTab().path),
	}
	note.queueSidebarDrag(entry)
	if note.sidebarDragPending {
		t.Fatal("drag should not queue while drag mode is disabled")
	}

	note.sidebarDragToggle.SetActive(true)
	note.queueSidebarDrag(entry)
	if !note.sidebarDragPending {
		t.Fatal("drag should queue while drag mode is enabled")
	}
	note.cancelPendingSidebarDrag()
}

func TestWorkspaceToolbarButtonSupportsGlyphFallback(t *testing.T) {
	requireGTK(t)

	button := workspaceToolbarButton("", "H3", "Heading 3")
	if button.Child() == nil {
		t.Fatal("glyph toolbar button should have a child")
	}
	if button.TooltipText() != "Heading 3" {
		t.Fatalf("tooltip = %q, want Heading 3", button.TooltipText())
	}
}

func TestSidebarTargetFolderUsesFolderRowsAndNoteParent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notePath := filepath.Join(notesDir(), "Work", "Plan.md")
	if got := sidebarTargetFolder(sidebarEntryFolder, filepath.Join("/tmp", "notes", "Work"), filepath.Join("Work")); got != filepath.Join("Work") {
		t.Fatalf("folder target = %q, want %q", got, filepath.Join("Work"))
	}
	if got := sidebarTargetFolder(sidebarEntryNote, notePath, filepath.Join("Work")); got != filepath.Join("Work") {
		t.Fatalf("note target = %q, want %q", got, filepath.Join("Work"))
	}
}

func TestPreviewUpdatesWhenBufferChanges(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.buffer.SetText("# Heading")

	if got := note.previewBuffer.Text(note.previewBuffer.StartIter(), note.previewBuffer.EndIter(), true); got != "Heading" {
		t.Fatalf("previewBuffer text = %q, want %q", got, "Heading")
	}
}

func TestTogglePreviewHidesPreviewPane(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.setPreviewVisible(false)
	if child := note.paned.EndChild(); child == nil || gtk.BaseWidget(child).Visible() {
		t.Fatal("preview pane should be hidden after toggle")
	}
	note.setPreviewVisible(true)
	if child := note.paned.EndChild(); child == nil || !gtk.BaseWidget(child).Visible() {
		t.Fatal("preview pane should be visible after toggle back on")
	}
}

func TestInsertTabSpacesUsesSettingsValue(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.TabSpaces = 3

	note := GenerateUI()
	note.buffer.SetText("abc")
	note.buffer.PlaceCursor(note.buffer.EndIter())
	note.insertTabSpaces()

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "abc   " {
		t.Fatalf("insertTabSpaces() text = %q, want %q", got, "abc   ")
	}
}

func TestGenerateUIStartsInNormalModeWhenVimEnabled(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	if note.vimInsertMode {
		t.Fatal("GenerateUI() should start in normal mode when vim bindings are enabled")
	}
	if !note.note.Overwrite() {
		t.Fatal("normal mode should use overwrite cursor for block caret")
	}
}

func TestGenerateUIStartsInInsertModeWhenVimDisabled(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.VimMode = false

	note := GenerateUI()
	if !note.vimInsertMode {
		t.Fatal("GenerateUI() should start in insert mode when vim bindings are disabled")
	}
	if note.note.Overwrite() {
		t.Fatal("non-vim mode should not use overwrite cursor")
	}
}

func TestHandleVimKeyDeletesLineOnlyOnDD(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("one\ntwo\nthree")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(4))

	note.handleVimKey(gdk.KEY_d)
	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "one\ntwo\nthree" {
		t.Fatalf("after first d, text = %q, want unchanged", got)
	}

	note.handleVimKey(gdk.KEY_d)
	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "one\nthree" {
		t.Fatalf("after dd, text = %q, want %q", got, "one\nthree")
	}
}

func TestHandleVimKeyColonShowsCommandLine(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	if !note.handleVimKey(gdk.KEY_colon) {
		t.Fatal("handleVimKey(:) should consume the event")
	}
	if !note.commandMode {
		t.Fatal("command mode should be active after :")
	}
	if parent := note.commandEntry.Parent(); parent == nil || !gtk.BaseWidget(parent).Visible() {
		t.Fatal("command entry row should be visible in command mode")
	}
}

func TestHandleVimKeyTogglesOverwriteCursorByMode(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	if !note.note.Overwrite() {
		t.Fatal("normal mode should start with overwrite cursor")
	}

	note.handleVimKey(gdk.KEY_i)
	if note.note.Overwrite() {
		t.Fatal("insert mode should disable overwrite cursor")
	}

	note.handleVimKey(gdk.KEY_Escape)
	if !note.note.Overwrite() {
		t.Fatal("escape back to normal mode should restore overwrite cursor")
	}
}

func TestHandleVimKeySlashShowsSearchCommandLine(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	if !note.handleVimKey(gdk.KEY_slash) {
		t.Fatal("handleVimKey(/) should consume the event")
	}
	if got := note.commandEntry.Text(); got != "/" {
		t.Fatalf("command entry text = %q, want /", got)
	}
}

func TestHandleVimKeyStartsVisualLineMode(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("one\ntwo")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(0))
	note.handleVimKey(gdk.KEY_V)

	if note.selectionMode != vimSelectionLine {
		t.Fatal("V should start visual line mode")
	}
	tag := note.buffer.TagTable().Lookup(tagVisualSelection)
	if tag == nil || !note.buffer.IterAtOffset(0).HasTag(tag) {
		t.Fatal("visual line selection should apply visual-selection tag")
	}
}

func TestHandleVimKeyStartsVisualCharMode(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("one\ntwo")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(0))
	note.handleVimKey(gdk.KEY_v)

	if note.selectionMode != vimSelectionChar {
		t.Fatal("v should start visual character mode")
	}
}

func TestStartVisualSelectionBlockMode(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("abcd\nwxyz")
	note.placeCursorOffset(1)
	note.startVisualSelection(vimSelectionBlock)
	note.placeCursorOffset(8)
	note.refreshVisualSelection()
	if note.selectionMode != vimSelectionBlock {
		t.Fatal("visual block mode should be active")
	}
	tag := note.buffer.TagTable().Lookup(tagVisualSelection)
	if tag == nil || !note.buffer.IterAtOffset(1).HasTag(tag) || !note.buffer.IterAtOffset(6).HasTag(tag) {
		t.Fatal("visual block selection should apply visual-selection tag to each selected row")
	}
}

func TestRepeatSearchMovesForwardAndBackward(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("alpha beta alpha gamma alpha")
	if err := note.searchText("alpha"); err != nil {
		t.Fatalf("searchText() error = %v", err)
	}
	if note.lastSearchPos != 0 {
		t.Fatalf("first search pos = %d, want 0", note.lastSearchPos)
	}

	note.repeatSearch(false)
	if note.lastSearchPos != 11 {
		t.Fatalf("forward repeat pos = %d, want 11", note.lastSearchPos)
	}

	note.repeatSearch(true)
	if note.lastSearchPos != 0 {
		t.Fatalf("backward repeat pos = %d, want 0", note.lastSearchPos)
	}
}

func TestReplaceTextCurrentLineOnly(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("one\none two one\none")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(5))

	if err := note.replaceText("one", "1", true, true); err != nil {
		t.Fatalf("replaceText() error = %v", err)
	}

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "one\n1 two 1\none" {
		t.Fatalf("buffer text = %q, want %q", got, "one\n1 two 1\none")
	}
}

func TestVisualLineYankAndPaste(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("one\ntwo\nthree")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(4))
	note.handleVimKey(gdk.KEY_V)
	note.moveCursorVertical(1)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_y)

	if note.yankRegister.Kind != vimRegisterLine {
		t.Fatal("visual line yank should store line register")
	}
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(0))
	note.handleVimKey(gdk.KEY_p)
	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "one\ntwo\nthree\ntwo\nthree" {
		t.Fatalf("buffer text = %q", got)
	}
}

func TestVisualCharYankAndPaste(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("alpha beta")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(1))
	note.handleVimKey(gdk.KEY_v)
	note.placeCursorOffset(3)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_y)

	if note.yankRegister.Kind != vimRegisterChar || note.yankRegister.Text != "lph" {
		t.Fatalf("char yank register = %#v", note.yankRegister)
	}
	note.buffer.PlaceCursor(note.buffer.EndIter())
	note.handleVimKey(gdk.KEY_p)
	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "alpha betalph" {
		t.Fatalf("buffer text = %q, want %q", got, "alpha betalph")
	}
}

func TestVisualCharDelete(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("abcdef")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(1))
	note.handleVimKey(gdk.KEY_v)
	note.placeCursorOffset(3)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_d)

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "aef" {
		t.Fatalf("buffer text = %q, want %q", got, "aef")
	}
}

func TestVisualLineDelete(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("one\ntwo\nthree")
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(4))
	note.handleVimKey(gdk.KEY_V)
	note.moveCursorVertical(1)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_d)

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "one\n" {
		t.Fatalf("buffer text = %q, want %q", got, "one\n")
	}
}

func TestVisualBlockYankAndPaste(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("abcd\nwxyz")
	note.placeCursorOffset(1)
	note.startVisualSelection(vimSelectionBlock)
	note.placeCursorOffset(8)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_y)

	if note.yankRegister.Kind != vimRegisterBlock {
		t.Fatal("visual block yank should store block register")
	}
	note.buffer.PlaceCursor(note.buffer.IterAtOffset(0))
	note.handleVimKey(gdk.KEY_p)
	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "bcdabcd\nxyzwxyz" {
		t.Fatalf("buffer text = %q", got)
	}
}

func TestVisualBlockDelete(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("abcd\nwxyz")
	note.placeCursorOffset(1)
	note.startVisualSelection(vimSelectionBlock)
	note.placeCursorOffset(8)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_d)

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "a\nw" {
		t.Fatalf("buffer text = %q, want %q", got, "a\nw")
	}
}

func TestVisualChangeEntersInsertMode(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("abcd")
	note.placeCursorOffset(0)
	note.handleVimKey(gdk.KEY_v)
	note.moveCursorRight()
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_c)

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "cd" {
		t.Fatalf("visual change text = %q, want %q", got, "cd")
	}
	if !note.vimInsertMode {
		t.Fatal("visual change should enter insert mode")
	}
}

func TestVisualSwapAnchor(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("abcd")
	note.placeCursorOffset(0)
	note.handleVimKey(gdk.KEY_v)
	note.placeCursorOffset(2)
	note.refreshVisualSelection()
	note.handleVimKey(gdk.KEY_o)

	if note.selectionMark != 2 || note.selectionCursor != 0 {
		t.Fatalf("visual swap = %d,%d want 2,0", note.selectionMark, note.selectionCursor)
	}
}

func TestHandleVimKeyDeletesWord(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("alpha beta")
	note.placeCursorOffset(0)
	note.handleVimKey(gdk.KEY_d)
	note.handleVimKey(gdk.KEY_w)

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "beta" {
		t.Fatalf("dw text = %q, want %q", got, "beta")
	}
}

func TestHandleVimKeyChangesWord(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("alpha beta")
	note.placeCursorOffset(0)
	note.handleVimKey(gdk.KEY_c)
	note.handleVimKey(gdk.KEY_w)

	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "beta" {
		t.Fatalf("cw text = %q, want %q", got, "beta")
	}
	if !note.vimInsertMode {
		t.Fatal("cw should enter insert mode")
	}
}

func TestHandleVimKeyYanksWordAndPastes(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	note.buffer.SetText("alpha beta")
	note.placeCursorOffset(0)
	note.handleVimKey(gdk.KEY_y)
	note.handleVimKey(gdk.KEY_w)
	note.placeCursorOffset(len([]rune("alpha beta")))
	note.handleVimKey(gdk.KEY_p)

	if note.yankRegister.Text != "alpha " {
		t.Fatalf("yw register = %q, want %q", note.yankRegister.Text, "alpha ")
	}
	if got := note.buffer.Text(note.buffer.StartIter(), note.buffer.EndIter(), true); got != "alpha betaalpha " {
		t.Fatalf("paste after yw text = %q, want %q", got, "alpha betaalpha ")
	}
}

func TestRunVimCommandSaveIsAsync(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	settings.Inst().NotesApp.VimMode = true

	note := GenerateUI()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	note.saveCommand = func() {
		once.Do(func() { started <- struct{}{} })
		<-release
	}

	done := make(chan struct{})
	go func() {
		err := note.runVimCommand(vimCommand{Kind: vimCommandSave})
		if err != nil {
			t.Errorf("runVimCommand(save) error = %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runVimCommand(save) should return before save completes")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("async save should start")
	}

	close(release)
}

func TestSaveSpecificTabRunsWriteAsync(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	originalWrite := noteWriteFile
	defer func() { noteWriteFile = originalWrite }()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	noteWriteFile = func(string, []byte, os.FileMode) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}

	note := GenerateUI()
	note.buffer.SetText("async")

	done := make(chan struct{})
	go func() {
		note.saveSpecificTab(note.activeTab(), false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("saveSpecificTab() should return before write completes")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background write should start")
	}

	close(release)
}

func TestFlushCurrentNoteStateWritesTabsAndSyncsOnce(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "client-secret-1")
	settings.Init()

	originalWrite := noteWriteFile
	originalSync := noteSyncDriveData
	defer func() {
		noteWriteFile = originalWrite
		noteSyncDriveData = originalSync
	}()

	var writes []string
	noteWriteFile = func(name string, data []byte, _ os.FileMode) error {
		writes = append(writes, name+":"+string(data))
		return nil
	}

	syncCalls := 0
	noteSyncDriveData = func() error {
		syncCalls++
		return nil
	}

	settings.Inst().GDrive.Enabled = true
	settings.Inst().GDrive.FolderID = "folder-1"
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}

	note := GenerateUI()
	note.buffer.SetText("first")
	note.newNote()
	note.notebook.SetCurrentPage(1)
	note.switchToTab(1)
	note.buffer.SetText("second")

	if err := FlushCurrentNoteState(); err != nil {
		t.Fatalf("FlushCurrentNoteState() error = %v", err)
	}
	if len(writes) != 2 {
		t.Fatalf("write calls = %d, want 2", len(writes))
	}
	if syncCalls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncCalls)
	}
}
