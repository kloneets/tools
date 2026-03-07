package notes

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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
	})
	if !gtkInitOK {
		t.Skip("GTK could not be initialized in this environment")
	}
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
	note.openSidebarRow(note.sidebar.RowAtIndex(0))
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
	if note.renameEntry.Text() != "Project Notes" {
		t.Fatalf("rename entry = %q, want Project Notes", note.renameEntry.Text())
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

func TestDeleteCurrentNoteRemovesFileAndKeepsWorkspaceAlive(t *testing.T) {
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
		t.Fatalf("deleted note should be removed, stat err = %v", err)
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
	if note.sidebar.RowAtIndex(1) == nil {
		t.Fatal("sidebar should list the second note")
	}

	note.openSidebarRow(note.sidebar.RowAtIndex(1))
	if note.currentTab != 1 {
		t.Fatalf("current tab = %d, want 1", note.currentTab)
	}
	if note.renameEntry.Text() != note.activeTab().title {
		t.Fatalf("rename entry = %q, want active title %q", note.renameEntry.Text(), note.activeTab().title)
	}
}

func TestSidebarToggleHidesPanel(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	note := GenerateUI()
	note.setSidebarVisible(false)
	if gtk.BaseWidget(note.sidebarScroll).Visible() {
		t.Fatal("sidebar should be hidden")
	}
	note.setSidebarVisible(true)
	if !gtk.BaseWidget(note.sidebarScroll).Visible() {
		t.Fatal("sidebar should be visible")
	}
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
