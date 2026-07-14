package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	kokosync "github.com/kloneets/tools/src/sync"
	"github.com/kloneets/tools/src/todo"
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

func TestApplyPulledNotesBacksUpEmptyRemoteOverwriteForRecovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	app := &terminalApp{}
	notePath := app.noteAbsPath("Home/uz-Bebreni.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("verified local content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := app.applyPulledNotes(kokosync.NoteMergeResult{
		Upserts: []kokosync.LocalNote{{Path: "Home/uz-Bebreni.md", Text: ""}},
	})
	if err != nil {
		t.Fatalf("applyPulledNotes() error = %v", err)
	}
	backup, err := notes.LatestNoteBackup(notePath)
	if err != nil {
		t.Fatalf("LatestNoteBackup() error = %v", err)
	}
	if err := notes.RestoreNoteBackup(notePath, backup); err != nil {
		t.Fatalf("RestoreNoteBackup() error = %v", err)
	}
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "verified local content" {
		t.Fatalf("restored content = %q, want verified local content", content)
	}
	files, err := app.localNoteFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "Home/uz-Bebreni.md" {
		t.Fatalf("synced note files = %#v, want only live note", files)
	}
}

func TestApplyPulledNotesBacksUpRemoteDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app := &terminalApp{}
	notePath := app.noteAbsPath("Home/delete-me.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("recover after delete"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := app.applyPulledNotes(kokosync.NoteMergeResult{Deletes: []string{"Home/delete-me.md"}}); err != nil {
		t.Fatalf("applyPulledNotes() error = %v", err)
	}
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Fatalf("deleted note stat error = %v, want not exist", err)
	}
	backup, err := notes.LatestNoteBackup(notePath)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(backup.Path)
	if err != nil || string(content) != "recover after delete" {
		t.Fatalf("backup content = %q, error = %v", content, err)
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

func TestHelpTextForPagesEditMode(t *testing.T) {
	app := &terminalApp{view: viewPages, pages: &pages.Model{Editing: true}}
	got := app.helpText()
	for _, want := range []string{"tab/shift+tab field", "left/right cursor", "enter apply"} {
		if !strings.Contains(got, want) {
			t.Fatalf("helpText() = %q, want %q", got, want)
		}
	}
}

func TestRenderTabBarHighlightsActiveView(t *testing.T) {
	app := &terminalApp{view: viewSync}
	got := app.renderTabBar()
	if !strings.Contains(got, "5:Sync") {
		t.Fatalf("renderTabBar() = %q, want sync tab label", got)
	}
	if !strings.Contains(got, themeMarkupFGStyleBG(currentTheme().ActiveTabFG, currentTheme().ActiveTabBG, ":b")) {
		t.Fatalf("renderTabBar() = %q, want active tab styling", got)
	}
}

func TestRenderTabBarShowsRecorderWhenVisible(t *testing.T) {
	app := &terminalApp{view: viewRecorder, recorderVisible: true}
	got := app.renderTabBar()
	if !strings.Contains(got, "7:Recorder") {
		t.Fatalf("renderTabBar() = %q, want recorder tab label", got)
	}
}

func TestRenderTabBarUsesConfiguredTabOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().UI.TabOrder = []string{"settings", "notes", "todo", "pages", "password", "sync"}
	app := &terminalApp{view: viewTodo}

	got := app.renderTabBar()

	for _, want := range []string{"1:Settings", "2:Notes", "3:Todo", "6:Sync"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderTabBar() = %q, want %q", got, want)
		}
	}
	if target, ok := app.appTabViewForKey("3"); !ok || target != viewTodo {
		t.Fatalf("appTabViewForKey(3) = %v, %t; want todo, true", target, ok)
	}
}

func TestFirebaseSyncUnhealthy(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *settings.FirebaseSettings
		writeToken bool
		password   bool
		want       bool
	}{
		{name: "disabled hides stale error", cfg: &settings.FirebaseSettings{LastSyncStatus: "error"}},
		{name: "enabled missing config", cfg: &settings.FirebaseSettings{Enabled: true, Realtime: true}, want: true},
		{name: "enabled missing auth", cfg: &settings.FirebaseSettings{Enabled: true, Realtime: true, APIKey: "key", DatabaseURL: "https://db", WorkspaceID: "workspace"}, want: true},
		{name: "latest error", cfg: &settings.FirebaseSettings{Enabled: true, Realtime: true, APIKey: "key", DatabaseURL: "https://db", WorkspaceID: "workspace", LastSyncStatus: "error"}, writeToken: true, want: true},
		{name: "password auth", cfg: &settings.FirebaseSettings{Enabled: true, Realtime: true, APIKey: "key", DatabaseURL: "https://db", WorkspaceID: "workspace", LastSyncStatus: "ok"}, password: true},
		{name: "healthy", cfg: &settings.FirebaseSettings{Enabled: true, Realtime: true, APIKey: "key", DatabaseURL: "https://db", WorkspaceID: "workspace", LastSyncStatus: "ok"}, writeToken: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("KOKO_FIREBASE_EMAIL", "")
			t.Setenv("KOKO_FIREBASE_PASSWORD", "")
			settings.Init()
			settings.Inst().Firebase = tc.cfg
			if tc.writeToken {
				if err := kokosync.SaveToken(kokosync.TokenPath(), kokosync.TokenFile{RefreshToken: "refresh"}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.password {
				t.Setenv("KOKO_FIREBASE_EMAIL", "user@example.com")
				t.Setenv("KOKO_FIREBASE_PASSWORD", "secret")
			}
			if got := (&terminalApp{}).firebaseSyncUnhealthy(); got != tc.want {
				t.Fatalf("firebaseSyncUnhealthy() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSyncWarningDotRendersAndMapsToSyncTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().Firebase = &settings.FirebaseSettings{Enabled: true, Realtime: true}
	app := &terminalApp{view: viewNotes}

	if got := app.renderTabBar(); !strings.Contains(got, themeMarkupFG(currentTheme().ErrorAccent)+"●") {
		t.Fatalf("renderTabBar() = %q, want colored warning dot", got)
	}
	pos := 0
	for i, tab := range app.visibleAppTabs() {
		label := fmt.Sprintf(" %s:%s ", tab.key, app.appTabPlainLabel(tab))
		if tab.view == viewSync {
			dotCol := pos + len([]rune(label)) - 2
			if got, ok := app.appTabAtColumn(dotCol); !ok || got != viewSync {
				t.Fatalf("appTabAtColumn(dot) = %v, %t; want sync, true", got, ok)
			}
			return
		}
		pos += len([]rune(label))
		if i < len(app.visibleAppTabs())-1 {
			pos++
		}
	}
	t.Fatal("sync tab not found")
}

func TestRecordFirebaseSyncOutcomeKeepsWarningUntilSuccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().Firebase = &settings.FirebaseSettings{Enabled: true}
	app := &terminalApp{}

	app.recordFirebaseSyncOutcome(errors.New("offline"), "")
	if got := settings.Inst().Firebase.LastSyncStatus; got != "error" {
		t.Fatalf("status after error = %q, want error", got)
	}
	// Starting a retry does not write status, so the prior warning remains.
	if got := settings.Inst().Firebase.LastSyncStatus; got != "error" {
		t.Fatalf("status during retry = %q, want error", got)
	}
	app.recordFirebaseSyncOutcome(nil, "Firebase sync complete")
	if got := settings.Inst().Firebase.LastSyncStatus; got != "ok" {
		t.Fatalf("status after success = %q, want ok", got)
	}
}

func TestDeferredSyncCallersKeepPriorError(t *testing.T) {
	tests := []struct {
		name string
		run  func(*terminalApp)
	}{
		{name: "manual", run: func(app *terminalApp) {
			app.manualFirebaseSync = func(context.Context) error { return errFirebaseSyncDeferred }
			app.startManualFirebaseSync()
		}},
		{name: "view", run: func(app *terminalApp) {
			app.viewFirebaseSync = func(context.Context, view) error { return errFirebaseSyncDeferred }
			app.startViewFirebaseSync(viewTodo, true)
		}},
		{name: "periodic", run: func(app *terminalApp) {
			app.runFirebaseSyncTick(context.Background())
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings.Init()
			settings.Inst().Firebase = &settings.FirebaseSettings{Enabled: true, Realtime: true, LastSyncStatus: "error", LastSyncMessage: "offline"}
			done := make(chan struct{}, 1)
			app := &terminalApp{syncTimeout: time.Second, settingsDirty: true, firebaseSyncDone: done}
			if tc.name == "periodic" {
				app.periodicFirebaseSync = func(context.Context) error { return errFirebaseSyncDeferred }
			}
			tc.run(app)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for sync caller")
			}
			if got := settings.Inst().Firebase.LastSyncStatus; got != "error" {
				t.Fatalf("status = %q, want prior error", got)
			}
		})
	}
}

func TestAsyncFirebasePushesPersistErrorsAndComplete(t *testing.T) {
	tests := []struct {
		name string
		run  func(*terminalApp)
	}{
		{name: "todos", run: func(app *terminalApp) { app.pushTodosToFirebaseSoon() }},
		{name: "notes", run: func(app *terminalApp) { app.pushNotesToFirebaseSoon() }},
		{name: "note delete", run: func(app *terminalApp) {
			app.pushNoteDeleteToFirebaseSoon(filepath.Join(app.notesRootPath(), "gone.md"))
		}},
		{name: "settings", run: func(app *terminalApp) { app.pushSettingsToFirebaseSoon() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings.Init()
			settings.Inst().Firebase = &settings.FirebaseSettings{Enabled: true, Realtime: true}
			done := make(chan struct{}, 1)
			app := &terminalApp{
				asyncFirebasePushDone: done,
				asyncFirebasePush: func(context.Context, string, string) error {
					return errors.New("push failed")
				},
			}
			tc.run(app)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for async push")
			}
			if got := settings.Inst().Firebase.LastSyncStatus; got != "error" {
				t.Fatalf("status = %q, want error", got)
			}
			if got := settings.Inst().Firebase.LastSyncMessage; got != "push failed" {
				t.Fatalf("message = %q, want push failure", got)
			}
		})
	}
}

func TestVisibleAppTabsKeepsRecorderLastAfterConfiguredOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().UI.TabOrder = []string{"settings", "notes", "todo", "pages", "password", "sync"}
	app := &terminalApp{view: viewRecorder, recorderVisible: true}

	tabs := app.visibleAppTabs()

	if got := tabs[len(tabs)-1]; got.view != viewRecorder || got.key != "7" {
		t.Fatalf("last tab = %#v, want recorder with key 7", got)
	}
}

func TestDefaultAppViewUsesFirstConfiguredTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().UI.TabOrder = []string{"todo", "notes", "pages", "password", "sync", "settings"}

	if got := defaultAppView(); got != viewTodo {
		t.Fatalf("defaultAppView() = %v, want %v", got, viewTodo)
	}
}

func TestRefreshNotesBodyKeepsActiveNoteTabHighlight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{
			{Title: "One", Text: "one", Mode: notes.ModeNormal},
			{Title: "Two", Text: "two", Mode: notes.ModeNormal},
		},
		CurrentTab:   1,
		SidebarWidth: 28,
	}
	app := &terminalApp{view: viewNotes, notes: ws, width: 120, height: 36}
	app.initWidgets()
	app.refreshNotesBody()
	got := app.editor.GetText(false)
	activeTabStyle := themeMarkupPair(currentTheme().ActiveTabFG, currentTheme().ActiveTabBG)
	activeCloseStyle := themeMarkupPair(currentTheme().ErrorAccent, currentTheme().ActiveTabBG)
	if !strings.Contains(got, activeTabStyle+"[2:Two "+activeCloseStyle+"x"+activeTabStyle+"]") {
		t.Fatalf("editor text = %q, want highlighted current note tab", got)
	}
	if strings.Contains(got, activeCloseStyle+"x"+activeTabStyle+"[]") {
		t.Fatalf("editor text = %q, want active close followed by literal closing bracket", got)
	}
}

func TestHandleGlobalKeySidebarETogglesFullNotesBrowser(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	closedPath, err := ws.CreateNote("Closed Note")
	if err != nil {
		t.Fatal(err)
	}
	if !ws.CloseNoteByPath(closedPath) {
		t.Fatal("CloseNoteByPath() = false, want closed note for browser-only assertion")
	}
	ws.FocusSidebar = true
	app := &terminalApp{view: viewNotes, notes: ws, width: 120, height: 36}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "e", Rune: 'e'}) {
		t.Fatal("handleGlobalKey(e) = false, want true")
	}
	app.refreshNotesBody()
	if !ws.SidebarBrowsing {
		t.Fatal("SidebarBrowsing = false, want true")
	}
	if got := app.editor.GetTitle(); got != "Notes Browser" {
		t.Fatalf("editor title = %q, want browser title", got)
	}
	if got := app.sidebar.GetTitle(); got != "Notes" {
		t.Fatalf("sidebar title = %q, want normal notes title", got)
	}
	if got := app.editor.GetText(false); !strings.Contains(got, "Notes Browser") || !strings.Contains(got, "Closed Note") {
		t.Fatalf("editor text = %q, want full browser contents including closed note", got)
	}
	if got := app.sidebar.GetText(false); strings.Contains(got, "Closed Note") {
		t.Fatalf("sidebar text = %q, want closed note outside normal sidebar", got)
	}
	if got := app.commandBar.GetText(false); !strings.Contains(got, "notes/browser") {
		t.Fatalf("command bar = %q, want browser help", got)
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
		{tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), notes.Key{Name: "space", Rune: ' '}},
		{tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModCtrl), notes.Key{Name: "e", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModCtrl), notes.Key{Name: "tab", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModCtrl), notes.Key{Name: "a", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlV, 0, tcell.ModCtrl), notes.Key{Name: "v", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModCtrl), notes.Key{Name: "g", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModCtrl), notes.Key{Name: "r", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 0x07, tcell.ModNone), notes.Key{Name: "g", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 't', tcell.ModCtrl), notes.Key{Name: "t", Ctrl: true, Rune: 't'}},
		{tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModCtrl), notes.Key{Name: "1", Ctrl: true, Rune: '1'}},
		{tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModCtrl), notes.Key{Name: "g", Ctrl: true, Rune: 'g'}},
		{tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModCtrl), notes.Key{Name: "3", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 0x1c, tcell.ModNone), notes.Key{Name: "4", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 0x1d, tcell.ModNone), notes.Key{Name: "5", Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 0x1e, tcell.ModNone), notes.Key{Name: "6", Ctrl: true}},
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

func TestMapControlRuneDecodesLinuxCtrlG(t *testing.T) {
	got, ok := mapControlRune(0x07)
	want := notes.Key{Name: "g", Rune: 'g', Ctrl: true}
	if !ok || got != want {
		t.Fatalf("mapControlRune(0x07) = %#v, %t want %#v, true", got, ok, want)
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

func TestHandleGlobalKeyCtrlSSavesAllNotesAndSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	helpers.InitStatusBar()
	settings.Init()
	first := filepath.Join(home, "first.md")
	second := filepath.Join(home, "second.md")
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{
			{Path: first, Title: "first", Text: "first saved", Dirty: true, Mode: notes.ModeNormal},
			{Path: second, Title: "second", Text: "second saved", Dirty: true, Mode: notes.ModeNormal},
		},
		CurrentTab: 0,
	}
	settings.Inst().UI.Theme = "gruvbox"
	app := &terminalApp{view: viewNotes, notes: ws, settingsDirty: true}
	if !app.handleGlobalKey(notes.Key{Name: "s", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+s) = false, want true")
	}
	assertFileContent(t, first, "first saved")
	assertFileContent(t, second, "second saved")
	if ws.Tabs[0].Dirty || ws.Tabs[1].Dirty {
		t.Fatal("dirty notes were not cleared")
	}
	if app.settingsDirty {
		t.Fatal("settingsDirty = true, want false")
	}
	data, err := os.ReadFile(filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved settings.UserSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.UI.Theme != "gruvbox" {
		t.Fatalf("saved theme = %q, want gruvbox", saved.UI.Theme)
	}
}

func TestHandleGlobalKeyCtrlRStartsQuickFirebaseSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	helpers.InitStatusBar()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{}, 1)
	app := &terminalApp{
		firebaseSyncDone: done,
		manualFirebaseSync: func(context.Context) error {
			close(started)
			<-release
			return nil
		},
	}

	if !app.handleGlobalKey(notes.Key{Name: "r", Ctrl: true}) {
		t.Fatal("handleGlobalKey(ctrl+r) = false, want true")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("quick Firebase sync did not start")
	}
	if !app.syncInProgress {
		t.Fatal("syncInProgress = false, want quick Firebase sync to start")
	}
	if app.syncProgressLabel != "sync to firebase" {
		t.Fatalf("syncProgressLabel = %q, want %q", app.syncProgressLabel, "sync to firebase")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("quick Firebase sync did not finish")
	}
}

func TestNotesVimWriteRequestsSaveAllState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	helpers.InitStatusBar()
	settings.Init()
	first := filepath.Join(home, "first.md")
	second := filepath.Join(home, "second.md")
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{
			{Path: first, Title: "first", Text: "first saved", Dirty: true, Mode: notes.ModeCommand, Command: "w"},
			{Path: second, Title: "second", Text: "second saved", Dirty: true, Mode: notes.ModeNormal},
		},
		CurrentTab: 0,
	}
	settings.Inst().UI.Theme = "kanagawa"
	app := &terminalApp{view: viewNotes, notes: ws, settingsDirty: true}
	if !app.handleGlobalKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleGlobalKey(enter) = false, want command handling")
	}
	assertFileContent(t, first, "first saved")
	assertFileContent(t, second, "second saved")
	if ws.Tabs[0].Dirty || ws.Tabs[1].Dirty {
		t.Fatal("dirty notes were not cleared by :w")
	}
	if app.settingsDirty {
		t.Fatal("settingsDirty = true, want false after :w")
	}
	data, err := os.ReadFile(filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved settings.UserSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.UI.Theme != "kanagawa" {
		t.Fatalf("saved theme = %q, want kanagawa", saved.UI.Theme)
	}
}

func TestNotesVimWriteQuitSavesAndConsumesQuitImmediately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	helpers.InitStatusBar()
	settings.Init()
	notePath := filepath.Join(home, "note.md")
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Path:    notePath,
			Title:   "note",
			Text:    "saved by wq",
			Dirty:   true,
			Mode:    notes.ModeCommand,
			Command: "wq",
		}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleGlobalKey(enter) = false, want command handling")
	}
	assertFileContent(t, notePath, "saved by wq")
	if ws.ActiveEditor().Dirty {
		t.Fatal("Dirty = true, want false after :wq save")
	}
	if quit, _ := ws.TakePendingQuit(); quit {
		t.Fatal("pending quit remains after :wq, want consumed in same input")
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
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

func TestHandleGlobalKeyMovesNormalModeLineWithMLCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.VimMode = true

	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ed := ws.ActiveEditor()
	ed.Text = "one\ntwo\nthree\nfour"
	ed.Cursor = len([]rune("one\n"))
	ed.Mode = notes.ModeNormal
	ws.FocusSidebar = false
	app := &terminalApp{view: viewNotes, notes: ws}

	for _, key := range []notes.Key{
		{Name: "m", Rune: 'm'},
		{Name: "l", Rune: 'l'},
		{Name: "2", Rune: '2'},
		{Name: "d", Rune: 'd'},
	} {
		if !app.handleGlobalKey(key) {
			t.Fatalf("handleGlobalKey(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\nthree\nfour\ntwo" {
		t.Fatalf("editor text = %q, want cursor line moved down through app routing", got)
	}

	for _, key := range []notes.Key{
		{Name: "m", Rune: 'm'},
		{Name: "l", Rune: 'l'},
		{Name: "u", Rune: 'u'},
	} {
		if !app.handleGlobalKey(key) {
			t.Fatalf("handleGlobalKey(%q) = false, want true", key.Name)
		}
	}
	if got := ed.Text; got != "one\nthree\ntwo\nfour" {
		t.Fatalf("editor text = %q, want cursor line moved up through app routing", got)
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
	if !app.handleGlobalKey(notes.Key{Name: "3", Rune: '3'}) {
		t.Fatal("handleGlobalKey() = false, want true for selected tab digit")
	}
	if app.view != viewPassword {
		t.Fatalf("view = %v, want %v", app.view, viewPassword)
	}
	if app.tabSelect {
		t.Fatal("tabSelect = true, want false after switch")
	}
}

func TestHandleGlobalKeyCtrlNumberSwitchesTabDirectly(t *testing.T) {
	app := &terminalApp{view: viewNotes, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "5", Rune: '5', Ctrl: true}) {
		t.Fatal("handleGlobalKey() = false, want true for ctrl+number")
	}
	if app.view != viewSync {
		t.Fatalf("view = %v, want %v", app.view, viewSync)
	}
	if app.tabSelect {
		t.Fatal("tabSelect = true, want false after direct switch")
	}
}

func TestHandleGlobalKeyCtrlNumberSwitchesRecorderTabWhenVisible(t *testing.T) {
	app := &terminalApp{view: viewNotes, recorderVisible: true}
	if !app.handleGlobalKey(notes.Key{Name: "7", Rune: '7', Ctrl: true}) {
		t.Fatal("handleGlobalKey() = false, want true for ctrl+7")
	}
	if app.view != viewRecorder {
		t.Fatalf("view = %v, want %v", app.view, viewRecorder)
	}
}

func TestSwitchAppTabStartsCurrentViewSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().Firebase = &settings.FirebaseSettings{Enabled: true, Realtime: true}
	called := make(chan view, 1)
	done := make(chan struct{}, 1)
	app := &terminalApp{
		view:             viewNotes,
		firebaseSyncDone: done,
		viewFirebaseSync: func(_ context.Context, target view) error {
			called <- target
			return nil
		},
	}

	app.switchAppTab(viewTodo)

	select {
	case got := <-called:
		if got != viewTodo {
			t.Fatalf("synced view = %v, want %v", got, viewTodo)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for view sync")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for view sync completion")
	}
}

func TestSwitchAppTabSameViewDoesNotSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().Firebase = &settings.FirebaseSettings{Enabled: true, Realtime: true}
	called := make(chan view, 1)
	app := &terminalApp{
		view: viewTodo,
		viewFirebaseSync: func(_ context.Context, target view) error {
			called <- target
			return nil
		},
	}

	app.switchAppTab(viewTodo)

	select {
	case got := <-called:
		t.Fatalf("unexpected sync for unchanged view: %v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSyncViewToFirebaseUsesExpectedScopes(t *testing.T) {
	tests := []struct {
		name   string
		target view
		want   string
	}{
		{name: "todo", target: viewTodo, want: "pullTodos,pushTodos"},
		{name: "notes", target: viewNotes, want: "pullNotes,pushNotes"},
		{name: "pages", target: viewPages, want: "pullSettings,pushSettings"},
		{name: "password", target: viewPassword, want: "pullSettings,pushSettings"},
		{name: "settings", target: viewSettings, want: "pullSettings,pushSettings"},
		{name: "sync", target: viewSync, want: "full"},
		{name: "recorder", target: viewRecorder, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := viewFirebaseSyncKind(tc.target)
			if got != tc.want {
				t.Fatalf("viewFirebaseSyncKind(%v) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

func TestHandleGlobalKeyTodoNewAndToggle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	app := &terminalApp{view: viewTodo, todos: repo}

	for _, key := range []notes.Key{
		{Name: "n", Rune: 'n'},
		{Name: "a", Rune: 'a'},
		{Name: "l", Rune: 'l'},
		{Name: "p", Rune: 'p'},
		{Name: "h", Rune: 'h'},
		{Name: "a", Rune: 'a'},
		{Name: "enter"},
	} {
		if !app.handleGlobalKey(key) {
			t.Fatalf("handleGlobalKey(%#v) = false, want true", key)
		}
	}
	if got := todo.ActiveItems(app.todoStore); len(got) != 1 || got[0].Text != "alpha" {
		t.Fatalf("active todos = %#v, want alpha", got)
	}
	if !app.handleGlobalKey(notes.Key{Name: "space"}) {
		t.Fatal("handleGlobalKey(space) = false, want true")
	}
	if got := todo.ActiveItems(app.todoStore); got[0].CheckedAt == nil {
		t.Fatalf("checked_at = nil, want pending checked todo")
	}
}

func TestHandleGlobalKeyTodoSpaceRuneToggles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	app := &terminalApp{view: viewTodo, todos: repo}
	store, _, err := repo.Add("alpha")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	app.todoStore = store

	if !app.handleGlobalKey(notes.Key{Name: "space", Rune: ' '}) {
		t.Fatal("handleGlobalKey(space rune) = false, want true")
	}
	if got := todo.ActiveItems(app.todoStore); got[0].CheckedAt == nil {
		t.Fatalf("checked_at = nil, want pending checked todo")
	}
}

func TestRenderTodoShowsSections(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	archivedAt := now
	long := todo.Item{ID: "4", Text: "later", Status: todo.StatusTodo, Term: todo.TermLong, CreatedAt: now, UpdatedAt: now}
	app := &terminalApp{
		view: viewTodo,
		todoStore: todo.Store{Items: []todo.Item{
			{ID: "1", Text: "active", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now},
			{ID: "2", Text: "done", Status: todo.StatusDone, CreatedAt: now, UpdatedAt: now, DoneAt: &now},
			{ID: "3", Text: "old", Status: todo.StatusArchived, CreatedAt: now, UpdatedAt: now, ArchivedAt: &archivedAt},
			long,
		}},
	}
	got := app.renderTodo(20)
	for _, want := range []string{"Short term", "Long term", "Done", "Archive", "[+] 2026-05", "active", "later", "~done~"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderTodo() = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "~old~") {
		t.Fatalf("renderTodo() = %q, should hide archived item text before month is opened", got)
	}
}

func TestRenderTodoShowsExpandedArchiveMonthFromLocalCache(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	archivedAt := now
	app := &terminalApp{
		view:                viewTodo,
		todoArchiveExpanded: map[string]bool{"2026-05": true},
		todoStore: todo.Store{Items: []todo.Item{
			{ID: "3", Text: "old", Status: todo.StatusArchived, CreatedAt: now, UpdatedAt: now, ArchivedAt: &archivedAt},
		}},
	}

	got := app.renderTodo(20)

	for _, want := range []string{"[-] 2026-05", "~old~"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderTodo() = %q, want %q", got, want)
		}
	}
}

func TestHandleGlobalKeyTodoArchiveMonthExpandsLocalCacheWithoutFirebase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	archivedAt := now
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	store := todo.Store{Items: []todo.Item{
		{ID: "3", Text: "old", Status: todo.StatusArchived, CreatedAt: now, UpdatedAt: now, ArchivedAt: &archivedAt},
	}}
	if err := repo.Save(store); err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{
		view:      viewTodo,
		todos:     repo,
		todoStore: store,
	}

	if !app.handleGlobalKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleGlobalKey(enter) = false, want archive month handled")
	}
	if !app.todoArchiveExpanded["2026-05"] {
		t.Fatal("todoArchiveExpanded[2026-05] = false, want true")
	}
}

func TestMoveSelectedTodoKeepsSelectionOnMovedItem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	store := todo.Store{Items: []todo.Item{
		{ID: "a", Text: "a", Status: todo.StatusTodo, Order: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "b", Text: "b", Status: todo.StatusTodo, Order: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "c", Text: "c", Status: todo.StatusTodo, Order: 2, CreatedAt: now, UpdatedAt: now},
	}}
	if err := repo.Save(store); err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{view: viewTodo, todos: repo, todoStore: store, todoIndex: 1}

	if !app.moveSelectedTodo(1) {
		t.Fatal("moveSelectedTodo() = false, want true")
	}

	if item, ok := app.selectedTodoItem(); !ok || item.ID != "b" {
		t.Fatalf("selectedTodoItem() = %#v, %t; want b, true", item, ok)
	}
	if got := app.todoIndex; got != 2 {
		t.Fatalf("todoIndex = %d, want moved item index 2", got)
	}
}

func TestMoveSelectedTodoStaysWithinCurrentTerm(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	store := todo.Store{Items: []todo.Item{
		{ID: "short-a", Text: "short a", Status: todo.StatusTodo, Order: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "long-a", Text: "long a", Status: todo.StatusTodo, Term: todo.TermLong, Order: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "short-b", Text: "short b", Status: todo.StatusTodo, Order: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "long-b", Text: "long b", Status: todo.StatusTodo, Term: todo.TermLong, Order: 1, CreatedAt: now, UpdatedAt: now},
	}}
	if err := repo.Save(store); err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{view: viewTodo, todos: repo, todoStore: store, todoIndex: 3}

	if !app.moveSelectedTodo(-1) {
		t.Fatal("moveSelectedTodo() = false, want true")
	}

	if got := todoItemIDs(todo.ActiveItems(app.todoStore)); strings.Join(got, ",") != "short-a,short-b,long-b,long-a" {
		t.Fatalf("active todos = %#v, want section-local reorder", got)
	}
	if item, ok := app.selectedTodoItem(); !ok || item.ID != "long-b" {
		t.Fatalf("selectedTodoItem() = %#v, %t; want long-b", item, ok)
	}
}

func todoItemIDs(items []todo.Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestHandleGlobalKeyTodoMoveSection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	store, item, err := repo.Add("alpha")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	app := &terminalApp{view: viewTodo, todos: repo, todoStore: store}

	if !app.handleGlobalKey(notes.Key{Name: "m", Rune: 'm'}) {
		t.Fatal("handleGlobalKey(m) = false, want true")
	}

	moved, ok := app.selectedTodoItem()
	if !ok || moved.ID != item.ID || moved.Term != todo.TermLong {
		t.Fatalf("selectedTodoItem() = %#v, %t; want moved long item", moved, ok)
	}
}

func TestTodoInputCursorPointsToEditBufferEnd(t *testing.T) {
	app := &terminalApp{view: viewTodo, todoInputMode: "edit", todoInputBuffer: "alpha"}
	app.todoInputCursorOffset = len([]rune(app.todoInputBuffer))

	row, col := app.todoInputCursor()

	if row != 2 {
		t.Fatalf("row = %d, want 2", row)
	}
	if col != len([]rune("edit: alpha")) {
		t.Fatalf("col = %d, want edit buffer end", col)
	}
}

func TestTodoInputCursorMovesAndInsertsInNewTodo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	app := &terminalApp{view: viewTodo, todos: repo}

	for _, key := range []notes.Key{
		{Name: "n", Rune: 'n'},
		{Name: "a", Rune: 'a'},
		{Name: "c", Rune: 'c'},
		{Name: "left"},
		{Name: "b", Rune: 'b'},
		{Name: "enter"},
	} {
		if !app.handleGlobalKey(key) {
			t.Fatalf("handleGlobalKey(%#v) = false, want true", key)
		}
	}

	if got := todo.ActiveItems(app.todoStore); len(got) != 1 || got[0].Text != "abc" {
		t.Fatalf("active todos = %#v, want abc", got)
	}
}

func TestTodoInputCursorMovesAndDeletesInEditTodo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	repo := todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	store, _, err := repo.Add("axbc")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	app := &terminalApp{view: viewTodo, todos: repo, todoStore: store}

	for _, key := range []notes.Key{
		{Name: "e", Rune: 'e'},
		{Name: "right"},
		{Name: "left"},
		{Name: "home"},
		{Name: "right"},
		{Name: "delete"},
		{Name: "end"},
		{Name: "d", Rune: 'd'},
		{Name: "enter"},
	} {
		if !app.handleGlobalKey(key) {
			t.Fatalf("handleGlobalKey(%#v) = false, want true", key)
		}
	}

	if got := todo.ActiveItems(app.todoStore); len(got) != 1 || got[0].Text != "abcd" {
		t.Fatalf("active todos = %#v, want abcd", got)
	}
}

func TestHandleGlobalKeyCtrlTabCyclesAppTabs(t *testing.T) {
	app := &terminalApp{view: viewSettings, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "tab", Ctrl: true}) {
		t.Fatal("handleGlobalKey() = false, want true for ctrl+tab")
	}
	if app.view != viewNotes {
		t.Fatalf("view = %v, want %v", app.view, viewNotes)
	}
	if app.tabSelect {
		t.Fatal("tabSelect = true, want false after ctrl+tab")
	}
	if !app.handleGlobalKey(notes.Key{Name: "tab", Ctrl: true}) {
		t.Fatal("handleGlobalKey() second = false, want true for ctrl+tab")
	}
	if app.view != viewPages {
		t.Fatalf("view = %v, want %v", app.view, viewPages)
	}
}

func TestHandleGlobalKeyPlainTabFallbackCyclesOutsideEditing(t *testing.T) {
	ws := &notes.Workspace{
		Tabs:       []*notes.Editor{{Text: "alpha", Mode: notes.ModeNormal}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "tab"}) {
		t.Fatal("handleGlobalKey(tab) = false, want true for fallback app tab cycle")
	}
	if app.view != viewPages {
		t.Fatalf("view = %v, want %v", app.view, viewPages)
	}
	if ws.FocusSidebar {
		t.Fatal("FocusSidebar = true, want tab fallback to avoid sidebar focus")
	}
}

func TestHandleGlobalKeyPlainTabStillReachesNotesInsertMode(t *testing.T) {
	ws := &notes.Workspace{
		Tabs:       []*notes.Editor{{Text: "alpha", Mode: notes.ModeInsert}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	if !app.handleGlobalKey(notes.Key{Name: "tab"}) {
		t.Fatal("handleGlobalKey(tab) = false, want notes insert tab handling")
	}
	if app.view != viewNotes {
		t.Fatalf("view = %v, want notes", app.view)
	}
}

func TestHandleGlobalKeyPlainTabReachesPagesEditMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	app := &terminalApp{
		view:  viewPages,
		pages: &pages.Model{FirstBookInput: "100", ReadInput: "25", SecondBookInput: "320", Editing: true},
	}
	if !app.handleGlobalKey(notes.Key{Name: "tab"}) {
		t.Fatal("handleGlobalKey(tab) = false, want pages edit tab handling")
	}
	if app.view != viewPages {
		t.Fatalf("view = %v, want pages", app.view)
	}
	if app.pages.Focus != 1 || !app.pages.SelectionActive {
		t.Fatalf("pages focus/selection = %d/%t, want read field selected", app.pages.Focus, app.pages.SelectionActive)
	}
	if !app.handleGlobalKey(notes.Key{Name: "tab", Shift: true}) {
		t.Fatal("handleGlobalKey(shift+tab) = false, want pages edit reverse tab handling")
	}
	if app.pages.Focus != 0 || !app.pages.SelectionActive {
		t.Fatalf("pages focus/selection = %d/%t, want first field selected", app.pages.Focus, app.pages.SelectionActive)
	}
}

func TestRenderPagesHighlightsSelectedEditValue(t *testing.T) {
	app := &terminalApp{
		pages: &pages.Model{
			FirstBookInput:  "100",
			ReadInput:       "25",
			SecondBookInput: "320",
			Editing:         true,
			SelectionActive: true,
		},
	}
	got := app.renderPages(6)
	if !strings.Contains(got, helpers.ANSIRoleSelection+"100") {
		t.Fatalf("renderPages() = %q, want selected first book value highlighted", got)
	}
}

func TestCaptureInputCtrlTabCSIuCyclesAppTabs(t *testing.T) {
	app := &terminalApp{view: viewNotes}
	events := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '[', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '9', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, ';', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '5', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'u', tcell.ModNone),
	}
	for _, event := range events {
		if got := app.captureInput(event); got != nil {
			t.Fatalf("captureInput(%v) returned event, want consumed", event)
		}
	}
	if app.view != viewPages {
		t.Fatalf("view = %v, want %v", app.view, viewPages)
	}
}

func TestCaptureInputTmuxCtrlGCSIuOpensSpellSuggestions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Text:   "mispelled",
			Cursor: 1,
			Mode:   notes.ModeNormal,
		}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	events := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '[', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '0', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '3', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, ';', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '5', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'u', tcell.ModNone),
	}
	for _, event := range events {
		if got := app.captureInput(event); got != nil {
			t.Fatalf("captureInput(%v) returned event, want consumed", event)
		}
	}
	if got := ws.ActiveEditor().Status; got == "" {
		t.Fatal("editor status is empty, want ctrl+g to reach spell suggestions handler")
	}
}

func TestConsumePendingNoteActionsStartsRecorderCapture(t *testing.T) {
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Title:   "Plan",
			Text:    "x",
			Mode:    notes.ModeCommand,
			Command: "recordkeys",
		}},
		CurrentTab: 0,
	}
	if !ws.HandleKey(notes.Key{Name: "enter"}) {
		t.Fatal("HandleKey(enter) = false, want recordkeys command handled")
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	app.consumePendingNoteActions()
	if !app.recorderVisible || !app.recorderCapturing {
		t.Fatal("recorderVisible/recorderCapturing = false, want active recorder")
	}
	if app.view != viewRecorder {
		t.Fatalf("view = %v, want %v", app.view, viewRecorder)
	}
}

func TestCaptureInputRecordsKeysAndBlocksOtherBindingsDuringRecorderCapture(t *testing.T) {
	ws := &notes.Workspace{
		Tabs:       []*notes.Editor{{Text: "alpha", Mode: notes.ModeNormal}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewRecorder, notes: ws, recorderVisible: true}
	app.startRecorderCapture()
	if got := app.captureInput(tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModCtrl)); got != nil {
		t.Fatal("captureInput(ctrl+t) returned event, want consumed during recording")
	}
	if app.view != viewRecorder {
		t.Fatalf("view = %v, want recorder to remain active", app.view)
	}
	if len(app.recorderEvents) == 0 {
		t.Fatal("recorderEvents = 0, want captured key")
	}
	if app.recorderLastEvent.KeyName != "t" {
		t.Fatalf("last key = %q, want %q", app.recorderLastEvent.KeyName, "t")
	}
}

func TestAppTabAtColumnUsesRenderedTabTargets(t *testing.T) {
	app := &terminalApp{view: viewNotes}
	target, ok := app.appTabAtColumn(1)
	if !ok || target != viewNotes {
		t.Fatalf("appTabAtColumn(1) = %v, %v; want notes", target, ok)
	}
	target, ok = app.appTabAtColumn(len(" 1:Notes "))
	if ok {
		t.Fatalf("appTabAtColumn(separator) = %v, true; want no target", target)
	}
	target, ok = app.appTabAtColumn(len(" 1:Notes ") + 1)
	if !ok || target != viewPages {
		t.Fatalf("appTabAtColumn(pages start) = %v, %v; want pages", target, ok)
	}
}

func TestCaptureMouseSwitchesNoteTabOnEditorTabRow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	helpers.InitStatusBar()
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{
			{Title: "Plan", Mode: notes.ModeNormal},
			{Title: "Log", Mode: notes.ModeNormal},
		},
		CurrentTab: 0,
	}
	editor := tview.NewTextView()
	editor.SetRect(0, 0, 80, 10)
	app := &terminalApp{
		view:   viewNotes,
		notes:  ws,
		editor: editor,
	}
	event := tcell.NewEventMouse(len("[1:Plan x] "), 0, tcell.Button1, 0)
	returned, _ := app.captureMouse(event, tview.MouseLeftClick)
	if returned != nil {
		t.Fatal("captureMouse() returned event, want consumed note-tab click")
	}
	if ws.CurrentTab != 1 {
		t.Fatalf("CurrentTab = %d, want 1", ws.CurrentTab)
	}
}

func TestCaptureMouseClosesNoteTabOnCloseX(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	helpers.InitStatusBar()
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{
			{Path: "/tmp/plan.md", Title: "Plan", Mode: notes.ModeNormal},
			{Path: "/tmp/log.md", Title: "Log", Mode: notes.ModeNormal},
		},
		CurrentTab: 0,
	}
	editor := tview.NewTextView()
	editor.SetRect(0, 0, 80, 10)
	app := &terminalApp{
		view:   viewNotes,
		notes:  ws,
		editor: editor,
	}
	event := tcell.NewEventMouse(len("[1:Plan "), 0, tcell.Button1, 0)
	returned, _ := app.captureMouse(event, tview.MouseLeftClick)
	if returned != nil {
		t.Fatal("captureMouse() returned event, want consumed close click")
	}
	if len(ws.Tabs) != 1 || ws.Tabs[0].Title != "Log" {
		t.Fatalf("tabs = %#v, want Plan tab closed", ws.Tabs)
	}
}

func TestCaptureMouseScrollMovesNoteCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		EditorRenderWidth: 20,
		EditorHeight:      4,
		Tabs: []*notes.Editor{{
			Text:   "alpha beta gamma\ndelta\nepsilon",
			Cursor: 0,
			Mode:   notes.ModeNormal,
		}},
		CurrentTab: 0,
	}
	editor := tview.NewTextView()
	editor.SetRect(0, 0, 20, 10)
	app := &terminalApp{
		view:   viewNotes,
		notes:  ws,
		editor: editor,
	}
	event := tcell.NewEventMouse(3, 1, tcell.ButtonNone, 0)
	returned, _ := app.captureMouse(event, tview.MouseScrollDown)
	if returned != nil {
		t.Fatal("captureMouse() returned event, want consumed scroll")
	}
	if got := ws.ActiveEditor().Cursor; got == 0 {
		t.Fatal("cursor = 0, want moved by scroll")
	}
}

func TestAnsiToTViewSpellErrorUsesStableForegroundOnly(t *testing.T) {
	got := ansiToTView(helpers.ANSI(helpers.ANSIRoleSpellError, "badwrd"))
	if !strings.Contains(got, themeMarkupFG(currentTheme().ErrorAccent)+"badwrd") {
		t.Fatalf("ansiToTView(spell error) = %q, want error foreground styling", got)
	}
	if strings.Contains(got, "::u") {
		t.Fatalf("ansiToTView(spell error) = %q, want no underline style tag", got)
	}
}

func TestAnsiToTViewActiveTabCloseUsesErrorOnActiveBackground(t *testing.T) {
	got := ansiToTView(helpers.ANSI(helpers.ANSIRoleActiveTabClose, "x"))
	want := themeMarkupPair(currentTheme().ErrorAccent, currentTheme().ActiveTabBG) + "x"
	if !strings.Contains(got, want) {
		t.Fatalf("ansiToTView(active tab close) = %q, want %q", got, want)
	}
}

func TestHandleGlobalKeyMovesTabSelectionWithArrows(t *testing.T) {
	settings.Inst().Firebase.Enabled = false
	app := &terminalApp{view: viewNotes, tabSelect: true}
	if !app.handleGlobalKey(notes.Key{Name: "right"}) {
		t.Fatal("handleGlobalKey() = false, want true for right in tab select")
	}
	if app.view != viewPages {
		t.Fatalf("view = %v, want %v", app.view, viewPages)
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
	if want := themeColor(currentTheme().StatusAccent); app.header.GetBorderColor() != want {
		t.Fatalf("header border color = %v, want %v", app.header.GetBorderColor(), want)
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
	app := &terminalApp{view: viewSettings, settingIndex: 4}
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

func TestHandleSettingsKeyCyclesTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settings.Init()
	app := &terminalApp{view: viewSettings, settingIndex: 0}
	for _, want := range settings.BuiltInThemes[1:] {
		if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
			t.Fatal("handleSettingsKey(enter) = false, want true")
		}
		if got := settings.Inst().UI.Theme; got != want {
			t.Fatalf("Theme = %q, want %q", got, want)
		}
	}
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter wrap) = false, want true")
	}
	if got := settings.Inst().UI.Theme; got != settings.BuiltInThemes[0] {
		t.Fatalf("Theme = %q, want wrap to %q", got, settings.BuiltInThemes[0])
	}
	if !app.settingsDirty {
		t.Fatal("settingsDirty = false, want true")
	}
	data, err := os.ReadFile(filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !strings.Contains(string(data), `"theme":"`+settings.BuiltInThemes[0]+`"`) {
		t.Fatalf("settings file = %q, want persisted theme", string(data))
	}
}

func TestRenderSettingsShowsThemePreview(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().UI.Theme = "rose-pine"
	app := &terminalApp{view: viewSettings}
	got := app.renderSettings(8)
	if !strings.Contains(got, "theme: rose-pine") {
		t.Fatalf("renderSettings() = %q, want theme label", got)
	}
	theme := themeByName("rose-pine")
	if !strings.Contains(got, themeMarkupPair(theme.Primary, theme.Panel)+" txt ") {
		t.Fatalf("renderSettings() = %q, want primary text swatch", got)
	}
	if !strings.Contains(got, themeMarkupPair(theme.Title, theme.Panel)+" ttl ") {
		t.Fatalf("renderSettings() = %q, want title text swatch", got)
	}
	if !strings.Contains(got, themeMarkupPair(theme.Primary, theme.Background)+" bg ") {
		t.Fatalf("renderSettings() = %q, want background swatch", got)
	}
	if !strings.Contains(got, themeMarkupPair(theme.ActiveTabFG, theme.ActiveTabBG)+" tab ") {
		t.Fatalf("renderSettings() = %q, want active tab swatch", got)
	}
}

func TestHandleSettingsKeyEditsTabOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settings.Init()
	app := &terminalApp{view: viewSettings, settingIndex: 5}

	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter) = false, want true")
	}
	if !app.tabOrderEditMode {
		t.Fatal("tabOrderEditMode = false, want true")
	}
	if !app.handleSettingsKey(notes.Key{Name: "j", Rune: 'j'}) {
		t.Fatal("handleSettingsKey(j) = false, want true")
	}
	if !app.handleSettingsKey(notes.Key{Name: "K", Rune: 'K', Shift: true}) {
		t.Fatal("handleSettingsKey(K) = false, want true")
	}
	if got := settings.Inst().UI.TabOrder; len(got) < 2 || got[0] != "pages" || got[1] != "notes" {
		t.Fatalf("TabOrder = %v, want pages moved before notes", got)
	}
	if !app.settingsDirty {
		t.Fatal("settingsDirty = false, want true")
	}
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter done) = false, want true")
	}
	if app.tabOrderEditMode {
		t.Fatal("tabOrderEditMode = true, want false")
	}
	data, err := os.ReadFile(filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !strings.Contains(string(data), `"tab_order":["pages","notes"`) {
		t.Fatalf("settings file = %q, want persisted tab order", string(data))
	}
}

func TestRenderThemePreviewUsesThemePalette(t *testing.T) {
	preview := renderThemePreview(themeByName("gruvbox"))
	theme := themeByName("gruvbox")
	if !strings.Contains(preview, themeMarkupPair(theme.Secondary, theme.Panel)+" sub ") {
		t.Fatalf("renderThemePreview() = %q, want secondary text swatch", preview)
	}
	if !strings.Contains(preview, themeMarkupPair(theme.Dim, theme.Panel)+" dim ") {
		t.Fatalf("renderThemePreview() = %q, want dim text swatch", preview)
	}
	if !strings.Contains(preview, themeMarkupPair(theme.SelectionFG, theme.SelectionBG)+" sel ") {
		t.Fatalf("renderThemePreview() = %q, want selection swatch", preview)
	}
	if !strings.Contains(preview, themeMarkupPair(theme.Background, theme.ErrorAccent)+" err ") {
		t.Fatalf("renderThemePreview() = %q, want error swatch", preview)
	}
}

func TestHandleSettingsKeyTogglesTransparentBackground(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer func() {
		settings.Inst().UI.TransparentBackground = false
		applyGlobalBackgroundStyle()
	}()
	app := &terminalApp{view: viewSettings, settingIndex: 1}
	app.initWidgets()
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter) = false, want true")
	}
	if settings.Inst().UI == nil || !settings.Inst().UI.TransparentBackground {
		t.Fatal("TransparentBackground = false, want true")
	}
	if got := tview.Styles.PrimitiveBackgroundColor; got != tcell.ColorDefault {
		t.Fatalf("PrimitiveBackgroundColor = %v, want ColorDefault", got)
	}
	if got := tview.Styles.ContrastBackgroundColor; got != tcell.ColorDefault {
		t.Fatalf("ContrastBackgroundColor = %v, want ColorDefault", got)
	}
	if got := app.editor.GetBackgroundColor(); got != tcell.ColorDefault {
		t.Fatalf("editor background = %v, want ColorDefault", got)
	}
	if got, want := tview.Styles.PrimaryTextColor, themeColor(currentTheme().Primary); got != want {
		t.Fatalf("PrimaryTextColor = %v, want themed foreground %v", got, want)
	}
	if !app.settingsDirty {
		t.Fatal("settingsDirty = false, want true")
	}
	if got := app.renderSettings(6); !strings.Contains(got, "transparent background: true") {
		t.Fatalf("renderSettings() = %q, want transparent background label", got)
	}
}

func TestHandleSettingsKeyTogglesSpellChecking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	app := &terminalApp{view: viewSettings, settingIndex: 6}
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter) = false, want true")
	}
	if !settings.Inst().NotesApp.SpellCheckEnabled {
		t.Fatal("SpellCheckEnabled = false, want true")
	}
	if !app.settingsDirty {
		t.Fatal("settingsDirty = false, want true")
	}
}

func TestHandleSettingsKeyInstallsSpellDictionary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	helpers.InitStatusBar()
	settings.Init()
	defer notes.ResetSpellTestHooksForTests()
	notes.SetSpellDownloadURLForTests(func(pkg string, file string) string {
		return pkg + "/" + file
	})
	notes.SetSpellHTTPGetForTests(func(url string) (*http.Response, error) {
		body := "SET UTF-8\n"
		if strings.HasSuffix(url, "index.dic") {
			body = "1\nknown\n"
		}
		if strings.HasSuffix(url, "license") {
			body = "license\n"
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	app := &terminalApp{view: viewSettings, settingIndex: 7}
	if !app.handleSettingsKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleSettingsKey(enter) = false, want true")
	}
	if !notes.SpellDictionaryInstalled("en") {
		t.Fatal("SpellDictionaryInstalled(en) = false, want true")
	}
	if !settings.Inst().NotesApp.SpellCheckEnabled {
		t.Fatal("SpellCheckEnabled = false, want true")
	}
	if !app.settingsDirty {
		t.Fatal("settingsDirty = false, want true")
	}
}

func TestRenderSettingsShowsSpellDictionaryLoadFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer notes.ResetSpellTestHooksForTests()
	notes.SetSpellNativeHooksForTests(func(string) (string, error) {
		return "", errors.New("missing")
	}, nil)
	notes.SetSpellDownloadURLForTests(func(pkg string, file string) string {
		return pkg + "/" + file
	})
	notes.SetSpellHTTPGetForTests(func(url string) (*http.Response, error) {
		body := "SET UTF-8\n"
		if strings.HasSuffix(url, "index.dic") {
			body = "1\nknown/nm\n"
		}
		if strings.HasSuffix(url, "license") {
			body = "license\n"
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	if err := notes.InstallSpellDictionary("en"); err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{view: viewSettings}
	got := app.renderSettings(20)
	if !strings.Contains(got, "spell en (English): installed (fallback; install native checker: brew install nuspell)") {
		t.Fatalf("renderSettings() = %q, want fallback load status", got)
	}
}

func TestRenderSettingsShowsNativeSpellDictionaryBackend(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer notes.ResetSpellTestHooksForTests()
	notes.SetSpellDownloadURLForTests(func(pkg string, file string) string {
		return pkg + "/" + file
	})
	notes.SetSpellHTTPGetForTests(func(url string) (*http.Response, error) {
		body := "SET UTF-8\n"
		if strings.HasSuffix(url, "index.dic") {
			body = "1\nknown/nm\n"
		}
		if strings.HasSuffix(url, "license") {
			body = "license\n"
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	notes.SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(string, []string, string) (string, error) {
		return "badwrd\n", nil
	})
	if err := notes.InstallSpellDictionary("en"); err != nil {
		t.Fatal(err)
	}
	app := &terminalApp{view: viewSettings}
	got := app.renderSettings(20)
	if !strings.Contains(got, "spell en (English): installed (native: nuspell)") {
		t.Fatalf("renderSettings() = %q, want native backend status", got)
	}
}

func TestApplyWidgetBackgroundStyleRestoresOpaqueBackground(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer func() {
		settings.Inst().UI.TransparentBackground = false
		applyGlobalBackgroundStyle()
	}()
	app := &terminalApp{}
	app.initWidgets()
	settings.Inst().UI.TransparentBackground = true
	applyGlobalBackgroundStyle()
	app.applyWidgetBackgroundStyle()
	settings.Inst().UI.TransparentBackground = false
	applyGlobalBackgroundStyle()
	app.applyWidgetBackgroundStyle()
	if got, want := tview.Styles.PrimitiveBackgroundColor, themeColor(currentTheme().Background); got != want {
		t.Fatalf("PrimitiveBackgroundColor = %v, want default background", got)
	}
	if got, want := app.editor.GetBackgroundColor(), themeColor(currentTheme().Background); got != want {
		t.Fatalf("editor background = %v, want default background", got)
	}
	if got, want := app.editor.GetBorderColor(), themeColor(currentTheme().Border); got != want {
		t.Fatalf("editor border color = %v, want themed border %v", got, want)
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

func TestRenderHelpOverlayIncludesSpellAutocompleteHelp(t *testing.T) {
	app := &terminalApp{view: viewNotes}
	got, _ := app.renderHelpOverlay(100, 30)
	if !strings.Contains(got, "ctrl+g, :spell") {
		t.Fatalf("renderHelpOverlay() = %q, want ctrl+g and :spell help", got)
	}
	if !strings.Contains(got, "open spelling suggestions") {
		t.Fatalf("renderHelpOverlay() = %q, want spell suggestion description", got)
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

func TestHandleGlobalKeyEnterOnQuitCommandShowsQuitModal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{
		Tabs: []*notes.Editor{{
			Title:   "Plan",
			Text:    "unsaved",
			Dirty:   true,
			Mode:    notes.ModeCommand,
			Command: "q",
		}},
		CurrentTab: 0,
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "enter"}) {
		t.Fatal("handleGlobalKey(enter) = false, want true")
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "quit" {
		t.Fatalf("front page = %q, want quit", front)
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

func TestShutdownAndStopDoesNotSyncOnClose(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	app := &terminalApp{}
	start := time.Now()
	app.shutdownAndStop()
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("shutdownAndStop() took too long: %v", time.Since(start))
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

func TestRequestShutdownShowsModalForUnsavedState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	app := &terminalApp{settingsDirty: true}
	app.initWidgets()
	app.requestShutdown()
	if !app.shuttingDown {
		t.Fatal("shuttingDown = false, want true for unsaved state")
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
	ws.Selection = 0
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

func TestHandleGlobalKeyBrowserDShowsFolderDeleteConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws, err := notes.NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.CreateFolder("Projects"); err != nil {
		t.Fatal(err)
	}
	ws.FocusSidebar = true
	ws.SidebarBrowsing = true
	for i, entry := range ws.BrowserTree {
		if entry.Label == "Projects" {
			ws.BrowserSelection = i
			break
		}
	}
	app := &terminalApp{view: viewNotes, notes: ws}
	app.initWidgets()
	if !app.handleGlobalKey(notes.Key{Name: "d", Rune: 'd'}) {
		t.Fatal("handleGlobalKey(d) = false, want true")
	}
	if !app.deleteNoteFolder {
		t.Fatal("deleteNoteFolder = false, want true")
	}
	if app.deleteNotePath != "Projects" || app.deleteNoteLabel != "Projects" {
		t.Fatalf("delete target path=%q label=%q, want Projects folder", app.deleteNotePath, app.deleteNoteLabel)
	}
	front, _ := app.pagesRoot.GetFrontPage()
	if front != "delete-note" {
		t.Fatalf("front page = %q, want delete-note", front)
	}
}

func TestRenderSyncShowsProgressState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	app := &terminalApp{
		syncInProgress:    true,
		syncProgressLabel: "pull todos from firebase",
	}
	got := app.renderSync(20)
	if !strings.Contains(got, "progress: / running pull todos from firebase") {
		t.Fatalf("renderSync() = %q, want sync progress line", got)
	}
}

func TestSyncItemsExposeSingleManualFirebaseAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	app := &terminalApp{}

	items := app.syncItems()
	manualSyncCount := 0
	for _, item := range items {
		switch item.Label {
		case "sync to firebase":
			manualSyncCount++
		case "pull todos from firebase",
			"push todos to firebase",
			"pull notes from firebase",
			"push notes to firebase",
			"pull settings from firebase",
			"push settings to firebase",
			"pull assets from firebase",
			"push assets to firebase":
			t.Fatalf("syncItems() includes old granular action %q", item.Label)
		}
	}
	if manualSyncCount != 1 {
		t.Fatalf("sync to firebase action count = %d, want 1", manualSyncCount)
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
