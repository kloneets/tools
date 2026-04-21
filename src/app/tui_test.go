package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/pages"
	"github.com/kloneets/tools/src/password"
	"github.com/kloneets/tools/src/settings"
	"github.com/rivo/tview"
)

func TestConfigDir(t *testing.T) {
	home := "/tmp/example"
	got := configDir(home)
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if got != want {
		t.Fatalf("configDir() = %q, want %q", got, want)
	}
}

func TestEnsureConfigDirExistsCreatesParents(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nested", helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	ensureConfigDirExists(target)
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("expected directory to exist, err = %v", err)
	}
}

func TestHelpTextForSettings(t *testing.T) {
	app := &terminalApp{view: viewSettings}
	got := app.helpText()
	if !strings.Contains(got, "enter change option") {
		t.Fatalf("helpText() = %q, want settings instructions", got)
	}
}

func TestHelpTextForSettingsEditMode(t *testing.T) {
	app := &terminalApp{view: viewSettings, settingsEditMode: true}
	got := app.helpText()
	if !strings.Contains(got, "digits edit") {
		t.Fatalf("helpText() = %q, want numeric edit instructions", got)
	}
}

func TestRenderTabBarHighlightsActiveView(t *testing.T) {
	app := &terminalApp{view: viewSync}
	got := app.renderTabBar()
	if !strings.Contains(got, "5:Sync") {
		t.Fatalf("renderTabBar() = %q, want sync tab label", got)
	}
	if !strings.Contains(got, "[black:white::b]") {
		t.Fatalf("renderTabBar() = %q, want active tab styling", got)
	}
}

func TestMapTCellKey(t *testing.T) {
	cases := []struct {
		event *tcell.EventKey
		want  notes.Key
	}{
		{tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), notes.Key{Name: "up"}},
		{tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModCtrl), notes.Key{Name: "s", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModCtrl), notes.Key{Name: "t", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModCtrl), notes.Key{Name: "n", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl), notes.Key{Name: "d", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone), notes.Key{Name: "a", Rune: 'a'}},
		{tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModCtrl), notes.Key{Name: "e", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModCtrl), notes.Key{Name: "a", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlV, 0, tcell.ModCtrl), notes.Key{Name: "v", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 't', tcell.ModCtrl), notes.Key{Name: "t", Ctrl: true, Rune: 't'}},
		{tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModMeta), notes.Key{Name: "right", Meta: true}},
		{tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt), notes.Key{Name: "right", Meta: true}},
	}
	for _, tc := range cases {
		got, ok := mapTCellKey(tc.event)
		if !ok {
			t.Fatalf("mapTCellKey(%v) returned ok=false", tc.event)
		}
		if got != tc.want {
			t.Fatalf("mapTCellKey(%v) = %#v, want %#v", tc.event, got, tc.want)
		}
	}
}

func TestHandleGlobalKeyMetaRightMovesEditorToLineEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Text:   "alpha beta\nnext",
			Cursor: 2,
			Mode:   notes.ModeInsert,
		}},
		CurrentTab:   0,
		SidebarWidth: 28,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "right", Meta: true}) {
		t.Fatal("handleGlobalKey(meta+right) = false, want true")
	}
	if got := ws.ActiveEditor().Cursor; got != len([]rune("alpha beta")) {
		t.Fatalf("cursor = %d, want line end", got)
	}
}

func TestHandleGlobalKeyMetaRightIgnoresTabSelectAndMovesEditorToLineEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Text:   "alpha beta\nnext",
			Cursor: 2,
			Mode:   notes.ModeInsert,
		}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "right", Meta: true}) {
		t.Fatal("handleGlobalKey(meta+right) = false, want true")
	}
	if app.view != viewNotes {
		t.Fatalf("view = %v, want notes", app.view)
	}
	if got := ws.ActiveEditor().Cursor; got != len([]rune("alpha beta")) {
		t.Fatalf("cursor = %d, want line end", got)
	}
}

func TestHandleGlobalKeyCtrlEVariantMovesEditorToLineEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Text:   "alpha beta\nnext",
			Cursor: 2,
			Mode:   notes.ModeInsert,
		}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "e", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+e) = false, want line end in editor")
	}
	if ws.FocusSidebar {
		t.Fatal("FocusSidebar = true, want editor to keep focus")
	}
	if got := ws.ActiveEditor().Cursor; got != len([]rune("alpha beta")) {
		t.Fatalf("cursor = %d, want line end", got)
	}
}

func TestHandleGlobalKeyCtrlATogglesNotesSidebar(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs:       []*notes.Editor{{Text: "alpha", Mode: notes.ModeInsert}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "a", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+a) = false, want sidebar toggle")
	}
	if !ws.FocusSidebar {
		t.Fatal("FocusSidebar = false, want true")
	}
	if !app.handleGlobalKey(notes.Key{Name: "a", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+a) second = false, want sidebar toggle")
	}
	if ws.FocusSidebar {
		t.Fatal("FocusSidebar = true, want false")
	}
}

func TestHandleGlobalKeyEditorCtrlRightDoesNotResizeSidebar(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Text:   "alpha beta\nnext",
			Cursor: 2,
			Mode:   notes.ModeInsert,
		}},
		CurrentTab:   0,
		SidebarWidth: 28,
		FocusSidebar: false,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "right", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+right) = false, want editor line navigation")
	}
	if got := ws.SidebarWidth; got != 28 {
		t.Fatalf("SidebarWidth = %d, want unchanged", got)
	}
	if got := ws.ActiveEditor().Cursor; got != len([]rune("alpha beta")) {
		t.Fatalf("cursor = %d, want line end", got)
	}
}

func TestHandleGlobalKeySidebarCtrlArrowsResizeSidebar(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs:         []*notes.Editor{{Text: "alpha", Mode: notes.ModeNormal}},
		CurrentTab:   0,
		SidebarWidth: 28,
		FocusSidebar: true,
		Tree:         []notes.TreeEntry{{Label: "Plan"}},
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "right", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+right) = false, want resize")
	}
	if ws.SidebarWidth != 29 {
		t.Fatalf("SidebarWidth after ctrl+right = %d, want 29", ws.SidebarWidth)
	}
	if !app.handleGlobalKey(notes.Key{Name: "left", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+left) = false, want resize")
	}
	if ws.SidebarWidth != 28 {
		t.Fatalf("SidebarWidth after ctrl+left = %d, want 28", ws.SidebarWidth)
	}
}

func TestHandleGlobalKeyAllowsPlainNumbersInInsertMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.FocusSidebar = false
	ws.ActiveEditor().Mode = notes.ModeInsert
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "1", Rune: '1'}) {
		t.Fatal("handleGlobalKey() = false, want true for plain digit in insert mode")
	}
	if !strings.Contains(ws.ActiveEditor().Text, "1") {
		t.Fatalf("editor text = %q, want inserted digit", ws.ActiveEditor().Text)
	}
}

func TestGeneratePasswordAndNotifyCopiesToClipboard(t *testing.T) {
	helpers.InitStatusBar()
	clipboardRestore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer clipboardRestore()

	app := &terminalApp{password: &password.Model{
		Letters:     true,
		SymbolCount: 8,
	}}
	app.generatePasswordAndNotify()
	if app.password.Password == "" {
		t.Fatal("password = empty, want generated password")
	}
	if got := helpers.StatusBarInst().Text(); got != "Password generated and copied to clipboard" {
		t.Fatalf("status = %q, want clipboard success notice", got)
	}
}

func TestGeneratePasswordAndNotifyReportsClipboardFailure(t *testing.T) {
	helpers.InitStatusBar()
	clipboardRestore := helpers.SetClipboardWriterForTesting(func(string) error { return os.ErrPermission })
	defer clipboardRestore()

	app := &terminalApp{password: &password.Model{
		Letters:     true,
		SymbolCount: 8,
	}}
	app.generatePasswordAndNotify()
	if !strings.Contains(helpers.StatusBarInst().Text(), "clipboard copy failed") {
		t.Fatalf("status = %q, want clipboard failure notice", helpers.StatusBarInst().Text())
	}
}

func TestHandleGlobalKeyActivatesTabSelectOnCtrlT(t *testing.T) {
	app := &terminalApp{view: viewNotes}
	if !app.handleGlobalKey(notes.Key{Name: "t", Ctrl: true, Rune: 't'}) {
		t.Fatal("handleGlobalKey() = false, want true for ctrl+t")
	}
	if !app.tabSelect {
		t.Fatal("tabSelect = false, want true")
	}
}

func TestHandleGlobalKeySwitchesTabAfterTabSelect(t *testing.T) {
	app := &terminalApp{view: viewNotes, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "4", Rune: '4'}) {
		t.Fatal("handleGlobalKey() = false, want true for selected tab digit")
	}
	if app.view != viewPassword {
		t.Fatalf("view = %v, want %v", app.view, viewPassword)
	}
	if app.tabSelect {
		t.Fatal("tabSelect = true, want false after switch")
	}
}

func TestHandleGlobalKeyMovesTabSelectionWithArrows(t *testing.T) {
	app := &terminalApp{view: viewNotes, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "right"}) {
		t.Fatal("handleGlobalKey() = false, want true for right in tab select")
	}
	if app.view != viewFiles {
		t.Fatalf("view = %v, want %v", app.view, viewFiles)
	}
	if !app.tabSelect {
		t.Fatal("tabSelect = false, want true while moving through tab bar")
	}
	if !app.handleGlobalKey(notes.Key{Name: "left"}) {
		t.Fatal("handleGlobalKey() = false, want true for left in tab select")
	}
	if app.view != viewNotes {
		t.Fatalf("view = %v, want %v", app.view, viewNotes)
	}
}

func TestHandleGlobalKeyConfirmsTabSelectionOnEnter(t *testing.T) {
	app := &terminalApp{view: viewSync, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleGlobalKey() = false, want true for enter in tab select")
	}
	if app.tabSelect {
		t.Fatal("tabSelect = true, want false after enter")
	}
}

func TestRefreshHighlightsHeaderBorderDuringTabSelect(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	helpers.InitStatusBar()
	settings.Init()
	app := &terminalApp{
		view:        viewSettings,
		tabSelect:   true,
		header:      tview.NewTextView(),
		statusBar:   tview.NewTextView(),
		body:        tview.NewFlex(),
		pagesRoot:   tview.NewPages(),
		helpOverlay: tview.NewTextView(),
		single:      tview.NewTextView(),
	}
	app.pagesRoot.AddPage("main", app.body, true, true)
	app.pagesRoot.AddPage("help", app.helpOverlay, true, false)
	app.pagesRoot.AddPage("shutdown", tview.NewTextView(), true, false)
	app.refresh()
	if got := app.header.GetBorderColor(); got != tcell.ColorYellow {
		t.Fatalf("header border color = %v, want %v", got, tcell.ColorYellow)
	}
}

func TestRefreshNotesBodyShowsCommandBar(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.ActiveEditor().Mode = notes.ModeCommand
	ws.ActiveEditor().Command = "/alpha"
	app := &terminalApp{
		view:        viewNotes,
		notes:       ws,
		width:       120,
		height:      40,
		header:      tview.NewTextView(),
		statusBar:   tview.NewTextView(),
		body:        tview.NewFlex(),
		pagesRoot:   tview.NewPages(),
		sidebar:     tview.NewTextView(),
		editor:      tview.NewTextView(),
		preview:     tview.NewTextView(),
		commandBar:  tview.NewTextView(),
		helpOverlay: tview.NewTextView(),
	}
	app.pagesRoot.AddPage("main", app.body, true, true)
	app.pagesRoot.AddPage("help", app.helpOverlay, true, false)
	app.pagesRoot.AddPage("shutdown", tview.NewTextView(), true, false)
	app.refreshNotesBody()
	if got := app.commandBar.GetText(false); !strings.Contains(got, "/alpha") {
		t.Fatalf("command bar = %q, want active command text", got)
	}
	if app.notes.EditorHeight != 28 {
		t.Fatalf("EditorHeight = %d, want 28 for height 40 layout", app.notes.EditorHeight)
	}
}

func TestRefreshNotesBodyHidesPreviewPaneWhenPreviewIsToggledOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.PreviewHidden = true
	app := &terminalApp{
		view:        viewNotes,
		notes:       ws,
		width:       120,
		height:      40,
		header:      tview.NewTextView(),
		statusBar:   tview.NewTextView(),
		body:        tview.NewFlex(),
		pagesRoot:   tview.NewPages(),
		sidebar:     tview.NewTextView(),
		editor:      tview.NewTextView(),
		preview:     tview.NewTextView(),
		commandBar:  tview.NewTextView(),
		helpOverlay: tview.NewTextView(),
	}
	app.pagesRoot.AddPage("main", app.body, true, true)
	app.pagesRoot.AddPage("help", app.helpOverlay, true, false)
	app.pagesRoot.AddPage("shutdown", tview.NewTextView(), true, false)
	app.refreshNotesBody()
	if got := app.body.GetItemCount(); got != 2 {
		t.Fatalf("body item count = %d, want 2", got)
	}
	content, ok := app.body.GetItem(0).(*tview.Flex)
	if !ok {
		t.Fatalf("content item type = %T, want *tview.Flex", app.body.GetItem(0))
	}
	if got := content.GetItemCount(); got != 2 {
		t.Fatalf("content item count = %d, want 2 when preview is hidden", got)
	}
}

func TestHandleSettingsKeyEditsUndoLevels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	app := &terminalApp{view: viewSettings, settingIndex: 2}
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter) = false, want true")
	}
	if !app.settingsEditMode || app.settingsEditBuffer != "1000" {
		t.Fatalf("settings edit state = mode:%t buffer:%q", app.settingsEditMode, app.settingsEditBuffer)
	}
	app.settingsEditBuffer = ""
	if !app.handleSettingsKey(notes.Key{Name: "5", Rune: '5'}) {
		t.Fatal("handleSettingsKey(5) = false, want true")
	}
	if !app.handleSettingsKey(notes.Key{Name: "0", Rune: '0'}) {
		t.Fatal("handleSettingsKey(0) = false, want true")
	}
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter apply) = false, want true")
	}
	if got := settings.Inst().NotesApp.UndoLevels; got != 50 {
		t.Fatalf("UndoLevels = %d, want 50", got)
	}
	if !app.settingsDirty {
		t.Fatal("settingsDirty = false, want true")
	}
}

func TestHelpOverlayToggle(t *testing.T) {
	app := &terminalApp{view: viewSettings, helpOverlay: tview.NewTextView()}
	if !app.handleGlobalKey(notes.Key{Name: "?"}) {
		t.Fatal("handleGlobalKey(?) = false, want true")
	}
	if !app.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	if !app.handleGlobalKey(notes.Key{Name: "esc"}) {
		t.Fatal("handleGlobalKey(esc) = false, want true while help is open")
	}
	if app.showHelp {
		t.Fatal("showHelp = true, want false after esc")
	}
}

func TestInitWidgetsConfiguresWrappedScrollableHelpOverlay(t *testing.T) {
	app := &terminalApp{}
	app.initWidgets()
	if app.helpOverlay == nil {
		t.Fatal("helpOverlay = nil")
	}
	app.helpOverlay.SetSize(5, 20)
	app.helpOverlay.SetText("this is a long help line that should wrap")
	if app.helpOverlay.GetWrappedLineCount() <= 1 {
		t.Fatal("helpOverlay did not wrap long text")
	}
	if row, col := app.helpOverlay.GetScrollOffset(); row > 0 || col != 0 {
		t.Fatalf("helpOverlay initial scroll = %d,%d, want top position", row, col)
	}
}

func TestCaptureInputUsesHelpOverlayScrolling(t *testing.T) {
	app := &terminalApp{}
	app.initWidgets()
	app.showHelp = true
	app.helpOverlay.SetText(strings.Repeat("line\n", 40))
	app.helpOverlay.ScrollToBeginning()
	app.captureInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	row, _ := app.helpOverlay.GetScrollOffset()
	if row == 0 {
		t.Fatal("help overlay did not scroll on down key")
	}
}

func TestRenderHelpOverlayIncludesBottomSections(t *testing.T) {
	app := &terminalApp{view: viewNotes}
	got, _ := app.renderHelpOverlay(40, 8)
	if !strings.Contains(got, "Notes command:") {
		t.Fatalf("renderHelpOverlay() missing notes command section: %q", got)
	}
	if !strings.Contains(got, "Files:") {
		t.Fatalf("renderHelpOverlay() missing files section: %q", got)
	}
	if !strings.Contains(got, "Settings:") {
		t.Fatalf("renderHelpOverlay() missing settings section: %q", got)
	}
	if !strings.Contains(got, "undo") || !strings.Contains(got, "redo") {
		t.Fatalf("renderHelpOverlay() missing undo/redo help: %q", got)
	}
}

func TestRenderHelpOverlayFormatsBindingsAsAlignedRows(t *testing.T) {
	app := &terminalApp{view: viewNotes}
	got, _ := app.renderHelpOverlay(80, 20)
	if !strings.Contains(got, "  ctrl+t") {
		t.Fatalf("renderHelpOverlay() = %q, want binding row for ctrl+t", got)
	}
	if !strings.Contains(got, "activate tab bar") {
		t.Fatalf("renderHelpOverlay() = %q, want aligned description text", got)
	}
	if strings.Contains(got, "ctrl+t activate tab bar |") {
		t.Fatalf("renderHelpOverlay() = %q, want one binding per row, not inline pipe list", got)
	}
}

func TestAnnotateHelpSearchAddsRegions(t *testing.T) {
	text, regions := annotateHelpSearch("alpha beta alpha", "alpha")
	if len(regions) != 2 {
		t.Fatalf("regions = %#v, want 2 matches", regions)
	}
	if !strings.Contains(text, `["help-hit-0"]alpha[""]`) || !strings.Contains(text, `["help-hit-1"]alpha[""]`) {
		t.Fatalf("annotateHelpSearch() = %q, want region markup", text)
	}
}

func TestCaptureInputStartsHelpSearchOnSlash(t *testing.T) {
	app := &terminalApp{view: viewSettings}
	app.initWidgets()
	app.showHelp = true
	app.captureInput(tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))
	if !app.helpSearchMode {
		t.Fatal("helpSearchMode = false, want true after /")
	}
}

func TestWantsQuitOnQ(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.ActiveEditor().Mode = notes.ModeNormal
	app := &terminalApp{view: viewNotes, notes: ws, pages: pages.NewModel()}
	if !app.wantsQuitOnQ() {
		t.Fatal("wantsQuitOnQ() = false, want true in notes normal mode")
	}
	ws.ActiveEditor().Mode = notes.ModeInsert
	if app.wantsQuitOnQ() {
		t.Fatal("wantsQuitOnQ() = true, want false in notes insert mode")
	}
	app.view = viewPages
	app.pages.StartEditing()
	if app.wantsQuitOnQ() {
		t.Fatal("wantsQuitOnQ() = true, want false while editing pages")
	}
	app.pages.StopEditing()
	if !app.wantsQuitOnQ() {
		t.Fatal("wantsQuitOnQ() = false, want true when pages are not editing")
	}
	app.view = viewFiles
	ws.FileCommandMode = true
	if app.wantsQuitOnQ() {
		t.Fatal("wantsQuitOnQ() = true, want false while editing files command")
	}
}

func TestHandleGlobalKeyRoutesToFilesView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{view: viewFiles, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "a", Rune: 'a'}) {
		t.Fatal("handleGlobalKey() = false, want file command activation")
	}
	if !ws.FileCommandMode || ws.FileCommand != "import " {
		t.Fatalf("files command = %q mode=%t, want import prompt", ws.FileCommand, ws.FileCommandMode)
	}
}

func TestHandleGlobalKeyEnterOnOpenLinksCommandShowsModal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Title:   "Plan",
			Text:    "https://example.com",
			Mode:    notes.ModeCommand,
			Command: "ol",
		}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleGlobalKey(enter) = false, want true")
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "open-links" {
		t.Fatalf("front page = %q, want open-links", front)
	}
	if len(app.openLinks) != 1 || app.openLinks[0] != "https://example.com" {
		t.Fatalf("openLinks = %v, want extracted URL", app.openLinks)
	}
}

func TestOpenLinksModalOpenAllUsesURIOpener(t *testing.T) {
	var opened []string
	restore := helpers.SetURIOpenerForTesting(func(uri string) {
		opened = append(opened, uri)
	})
	defer restore()
	app := &terminalApp{openLinks: []string{"https://example.com", "file:///tmp/example.txt"}}
	app.openAllPendingLinks()
	if len(opened) != 2 {
		t.Fatalf("opened = %v, want 2 links", opened)
	}
	if opened[0] != "https://example.com" || opened[1] != "file:///tmp/example.txt" {
		t.Fatalf("opened = %v, want ordered links", opened)
	}
}

func TestHelpTextForFilesIncludesScopeFolderShortcut(t *testing.T) {
	app := &terminalApp{view: viewFiles}
	got := app.helpText()
	if !strings.Contains(got, "F scope folder") {
		t.Fatalf("helpText() = %q, want scope folder shortcut", got)
	}
	if !strings.Contains(got, "o open") {
		t.Fatalf("helpText() = %q, want open shortcut", got)
	}
	if !strings.Contains(got, "I link") || !strings.Contains(got, "p image") {
		t.Fatalf("helpText() = %q, want insert override shortcuts", got)
	}
	if !strings.Contains(got, "D discard staged") {
		t.Fatalf("helpText() = %q, want staged discard shortcut", got)
	}
}

func TestShutdownAndStopDoesNotSyncOnClose(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().GDrive.Enabled = true
	settings.Inst().GDrive.FolderID = "folder-1"
	settings.Inst().GDrive.PendingSync = true

	app := &terminalApp{}
	start := time.Now()
	app.shutdownAndStop()
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("shutdownAndStop() took too long: %v", time.Since(start))
	}
	if !settings.Inst().GDrive.PendingSync {
		t.Fatal("PendingSync = false, want unchanged because close should not sync")
	}
}

func TestStopTUIClearsShutdownState(t *testing.T) {
	app := &terminalApp{
		shuttingDown: true,
		pagesRoot:    tview.NewPages(),
	}
	app.pagesRoot.AddPage("quit", tview.NewModal(), true, true)
	app.stopTUI()
	if app.shuttingDown {
		t.Fatal("shuttingDown = true, want false after stopTUI")
	}
}

func TestRequestShutdownShowsModalForUnsyncedState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().GDrive.LastLocalSaveAt = "2026-04-15T12:00:00Z"
	settings.Inst().GDrive.LastDriveSaveAt = "2026-04-15T11:00:00Z"

	app := &terminalApp{}
	app.initWidgets()
	app.requestShutdown()
	if !app.shuttingDown {
		t.Fatal("shuttingDown = false, want true for unsynced state")
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "quit" {
		t.Fatalf("front page = %q, want quit", front)
	}
}

func TestRenderTabBarShowsDirtyMarkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.ActiveEditor().Dirty = true
	app := &terminalApp{
		view:     viewNotes,
		notes:    ws,
		pages:    pages.NewModel(),
		password: password.NewModel(),
	}
	got := app.renderTabBar()
	if !strings.Contains(got, "1:Notes*") {
		t.Fatalf("renderTabBar() = %q, want notes dirty marker", got)
	}
	ws.ActiveEditor().Dirty = false
	ws.FilesDirty = true
	got = app.renderTabBar()
	if !strings.Contains(got, "2:Files*") {
		t.Fatalf("renderTabBar() = %q, want files dirty marker", got)
	}
}

func TestFormatTimestampOrNever(t *testing.T) {
	got := formatTimestampOrNever("2026-04-15T13:45:00Z")
	if got == "2026-04-15T13:45:00Z" || got == "never" {
		t.Fatalf("formatTimestampOrNever() = %q, want formatted local time", got)
	}
	if got := formatTimestampOrNever(""); got != "never" {
		t.Fatalf("formatTimestampOrNever(\"\") = %q, want never", got)
	}
}

func TestHandleGlobalKeyFilesDiscardShowsConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.FilesDirty = true
	app := &terminalApp{view: viewFiles, notes: ws}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "D", Rune: 'D', Shift: true}) {
		t.Fatal("handleGlobalKey(D) = false, want true")
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "discard-files" {
		t.Fatalf("front page = %q, want discard-files", front)
	}
	if !ws.FilesDirty {
		t.Fatal("FilesDirty = false before confirmation, want staged changes preserved")
	}
}

func TestHandleGlobalKeyCtrlDShowsNoteDeleteConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "d", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+d) = false, want true")
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "delete-note" {
		t.Fatalf("front page = %q, want delete-note", front)
	}
}

func TestHandleGlobalKeySidebarDShowsNoteDeleteConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.FocusSidebar = true
	app := &terminalApp{view: viewNotes, notes: ws}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleGlobalKey(d) = false, want true")
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "delete-note" {
		t.Fatalf("front page = %q, want delete-note", front)
	}
}

func TestRenderSyncHighlightsSelectedSnapshot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().GDrive.Snapshots = []settings.DriveSnapshotMeta{
		{ID: "snap-1", Name: "snapshot-a", CreatedAt: "2026-04-15T12:00:00Z"},
	}
	settings.Inst().GDrive.SelectedSnapshotID = "snap-1"
	app := &terminalApp{}
	got := app.renderSync(20)
	if !strings.Contains(got, "[selected]") {
		t.Fatalf("renderSync() = %q, want selected snapshot marker", got)
	}
	if strings.Contains(got, "[green::b]") {
		t.Fatalf("renderSync() = %q, want no raw tview color markup", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("renderSync() = %q, want ANSI styling for sync selection", got)
	}
}

func TestRenderSyncIncludesFullDriveErrorMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().GDrive.LastSyncStatus = "error"
	settings.Inst().GDrive.LastSyncMessage = "drive upload failed because the remote folder rejected the snapshot metadata payload"
	app := &terminalApp{}
	got := app.renderSync(20)
	if !strings.Contains(got, settings.Inst().GDrive.LastSyncMessage) {
		t.Fatalf("renderSync() = %q, want full drive error message", got)
	}
}

func TestRenderSyncShowsProgressState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	app := &terminalApp{
		syncInProgress:    true,
		syncProgressLabel: "upload local save to drive",
	}
	got := app.renderSync(20)
	if !strings.Contains(got, "progress: / running upload local save to drive") {
		t.Fatalf("renderSync() = %q, want sync progress line", got)
	}
}

func TestSyncSpinnerFrameCycles(t *testing.T) {
	app := &terminalApp{}
	if got := app.syncSpinnerFrame(); got != "/" {
		t.Fatalf("syncSpinnerFrame() = %q, want /", got)
	}
	app.syncSpinnerTick.Store(1)
	if got := app.syncSpinnerFrame(); got != "-" {
		t.Fatalf("syncSpinnerFrame() = %q, want -", got)
	}
	app.syncSpinnerTick.Store(2)
	if got := app.syncSpinnerFrame(); got != "\\" {
		t.Fatalf("syncSpinnerFrame() = %q, want \\\\", got)
	}
	app.syncSpinnerTick.Store(3)
	if got := app.syncSpinnerFrame(); got != "|" {
		t.Fatalf("syncSpinnerFrame() = %q, want |", got)
	}
}

func TestCaptureInputIgnoresQuitModalWhileSyncing(t *testing.T) {
	app := &terminalApp{
		shuttingDown:   true,
		syncInProgress: true,
		pagesRoot:      tview.NewPages(),
		quitModal:      tview.NewModal(),
	}
	if got := app.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); got != nil {
		t.Fatal("captureInput() should swallow input while save+sync is in progress")
	}
}

func TestStartSyncOperationMarksProgressImmediately(t *testing.T) {
	app := &terminalApp{syncTimeout: 200 * time.Millisecond}
	block := make(chan struct{})
	app.startSyncOperation("upload local save to drive", func() error {
		<-block
		return nil
	}, nil, nil)
	if !app.syncInProgress {
		t.Fatal("syncInProgress = false, want true immediately after start")
	}
	if app.syncProgressLabel != "upload local save to drive" {
		t.Fatalf("syncProgressLabel = %q, want operation label", app.syncProgressLabel)
	}
	close(block)
	time.Sleep(20 * time.Millisecond)
}

func TestStartSyncOperationIgnoresDuplicateStart(t *testing.T) {
	app := &terminalApp{syncTimeout: 200 * time.Millisecond}
	block := make(chan struct{})
	startedCh := make(chan struct{}, 2)
	var started atomic.Int32
	app.startSyncOperation("upload", func() error {
		started.Add(1)
		startedCh <- struct{}{}
		<-block
		return nil
	}, nil, nil)
	app.startSyncOperation("upload", func() error {
		started.Add(1)
		startedCh <- struct{}{}
		return nil
	}, nil, nil)
	select {
	case <-startedCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for first sync start")
	}
	if got := started.Load(); got != 1 {
		t.Fatalf("started = %d, want 1 running sync", got)
	}
	close(block)
	time.Sleep(20 * time.Millisecond)
}

func TestStartSyncOperationTimesOutAndClearsProgress(t *testing.T) {
	app := &terminalApp{syncTimeout: 20 * time.Millisecond}
	errCh := make(chan error, 1)
	app.startSyncOperation("upload", func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}, nil, func(err error) {
		errCh <- err
	})
	time.Sleep(60 * time.Millisecond)
	if app.syncInProgress {
		t.Fatal("syncInProgress = true, want false after timeout")
	}
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("timeout error = %v, want timeout", err)
		}
	default:
		t.Fatal("expected timeout callback error")
	}
}
