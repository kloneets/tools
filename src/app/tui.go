package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/pages"
	"github.com/kloneets/tools/src/password"
	"github.com/kloneets/tools/src/settings"
	kokosync "github.com/kloneets/tools/src/sync"
	"github.com/kloneets/tools/src/todo"
)

type view int

const (
	viewNotes view = iota
	viewFiles
	viewPages
	viewPassword
	viewTodo
	viewSync
	viewSettings
	viewRecorder
)

const manualSyncTimeout = 20 * time.Second
const recorderDuration = 5 * time.Second

type terminalApp struct {
	view                   view
	ctx                    context.Context
	notes                  *notes.Workspace
	pages                  *pages.Model
	password               *password.Model
	todos                  *todo.Repository
	todoStore              todo.Store
	todoIndex              int
	todoInputMode          string
	todoInputBuffer        string
	todoInputCursorOffset  int
	todoEditID             string
	todoDirty              bool
	status                 string
	width                  int
	height                 int
	settingIndex           int
	syncIndex              int
	tui                    *tview.Application
	header                 *tview.TextView
	help                   *tview.TextView
	statusBar              *tview.TextView
	body                   *tview.Flex
	pagesRoot              *tview.Pages
	sidebar                *tview.TextView
	editor                 *tview.TextView
	preview                *tview.TextView
	commandBar             *tview.TextView
	single                 *tview.TextView
	helpOverlay            *tview.TextView
	quitModal              *tview.Modal
	discardFilesModal      *tview.Modal
	deleteNoteModal        *tview.Modal
	deleteNoteFolder       bool
	openLinksModal         *tview.Modal
	root                   *tview.Flex
	lastStatus             string
	tabSelect              bool
	showHelp               bool
	helpSearchMode         bool
	helpSearchQuery        string
	helpSearchIndex        int
	shuttingDown           bool
	settingsDirty          bool
	settingsEditMode       bool
	settingsEditBuffer     string
	tabOrderEditMode       bool
	tabOrderIndex          int
	syncInProgress         bool
	syncProgressLabel      string
	syncOpID               atomic.Int64
	syncSpinnerTick        atomic.Int64
	syncTimeout            time.Duration
	firebaseTodoSyncer     *kokosync.TodoSyncer
	firebaseNoteSyncer     *kokosync.NoteSyncer
	firebaseSettingsSyncer *kokosync.SettingsSyncer
	firebaseAssetSyncer    *kokosync.AssetSyncer
	openLinks              []string
	deleteNotePath         string
	deleteNoteLabel        string
	recorderVisible        bool
	recorderCapturing      bool
	recorderStartedAt      time.Time
	recorderEndsAt         time.Time
	recorderEvents         []recordedKeyEvent
	recorderLastEvent      recordedKeyEvent
	recorderCaptureID      atomic.Int64
	notesMouseDragging     bool
	notesMouseMoved        bool
	notesMouseStartRow     int
	notesMouseStartCol     int
	inputSeqMu             sync.Mutex
	inputSeq               string
	inputSeqTimer          *time.Timer
}

type appTab struct {
	id    string
	label string
	view  view
	key   string
}

type recordedKeyEvent struct {
	At        time.Time
	Source    string
	KeyName   string
	TCellKey  string
	Rune      string
	Modifiers string
}

var baseAppTabs = []appTab{
	{"notes", "Notes", viewNotes, ""},
	{"files", "Files", viewFiles, ""},
	{"pages", "Pages", viewPages, ""},
	{"password", "Password", viewPassword, ""},
	{"todo", "Todo", viewTodo, ""},
	{"sync", "Sync", viewSync, ""},
	{"settings", "Settings", viewSettings, ""},
}

func InitApp() {
	helpers.InitGlobals()
	helpers.InitStatusBar()
	makeConfigDirIfNotExists()
	configureRuntimeLogging()
	settings.Init()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app, err := newTerminalApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newTerminalApp() (*terminalApp, error) {
	noteWS, err := notes.NewWorkspace()
	if err != nil {
		return nil, err
	}
	applyGlobalBackgroundStyle()
	todoRepo := todo.NewRepository()
	todoStore, err := todoRepo.Load()
	if err != nil {
		return nil, err
	}
	app := &terminalApp{
		view:        defaultAppView(),
		notes:       noteWS,
		pages:       pages.NewModel(),
		password:    password.NewModel(),
		todos:       todoRepo,
		todoStore:   todoStore,
		width:       120,
		height:      36,
		syncTimeout: manualSyncTimeout,
	}
	app.initWidgets()
	notes.SetSpellRefreshHook(func() {
		if app.tui == nil {
			return
		}
		app.tui.QueueUpdateDraw(func() {
			app.refresh()
		})
	})
	return app, nil
}

func defaultAppView() view {
	for _, id := range settings.UITabOrder() {
		for _, tab := range baseAppTabs {
			if tab.id == id {
				return tab.view
			}
		}
	}
	return viewNotes
}

func (a *terminalApp) initWidgets() {
	applyGlobalBackgroundStyle()
	a.header = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.header.SetBorder(true).SetTitle("Koko Tools")
	a.help = tview.NewTextView().SetWrap(false)
	a.help.SetBorder(true).SetTitle("Help")
	a.statusBar = tview.NewTextView().SetWrap(false)
	a.statusBar.SetBorder(true).SetTitle("Status")
	a.sidebar = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.sidebar.SetBorder(true).SetTitle("Notes")
	a.editor = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.editor.SetBorder(true)
	a.preview = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.preview.SetBorder(true).SetTitle("Preview")
	a.commandBar = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.commandBar.SetBorder(true).SetTitle("Command / Search")
	a.single = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.single.SetBorder(true)
	a.body = tview.NewFlex()
	a.pagesRoot = tview.NewPages()
	a.pagesRoot.AddPage("main", a.body, true, true)
	a.helpOverlay = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(true)
	a.helpOverlay.SetRegions(true)
	a.helpOverlay.SetBorder(true).SetTitle("Keyboard Shortcuts")
	a.pagesRoot.AddPage("help", a.helpOverlay, true, false)
	a.quitModal = tview.NewModal().
		SetText("Unsaved or unsynced changes. Save before closing?").
		AddButtons([]string{"Save", "Save + Sync", "Discard", "Cancel"})
	a.quitModal.SetDoneFunc(func(_ int, label string) {
		switch label {
		case "Save":
			if err := a.saveLocalState(); err != nil {
				helpers.StatusBarInst().UpdateStatusBar("Save failed: " + err.Error())
				a.cancelShutdown()
				return
			}
			a.stopTUI()
		case "Save + Sync":
			a.startSyncOperation("save + sync before close", func() error {
				if err := a.saveLocalState(); err != nil {
					return err
				}
				return settings.SyncDriveData()
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Saved locally and uploaded Drive snapshot at " + formatTimestampOrNever(settings.Inst().GDrive.LastDriveSaveAt))
				a.shuttingDown = false
				if a.pagesRoot != nil {
					a.pagesRoot.HidePage("quit")
				}
				a.stopTUI()
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Save + Sync failed: " + err.Error())
				a.cancelShutdown()
			})
		case "Discard":
			if a.notes != nil {
				_ = a.notes.DiscardPendingFiles()
			}
			a.stopTUI()
		default:
			a.cancelShutdown()
		}
	})
	a.pagesRoot.AddPage("quit", a.quitModal, true, false)
	a.discardFilesModal = tview.NewModal().
		SetText("Discard staged file changes?").
		AddButtons([]string{"Discard", "Cancel"})
	a.discardFilesModal.SetDoneFunc(func(_ int, label string) {
		switch label {
		case "Discard":
			if a.notes != nil {
				if err := a.notes.DiscardPendingFiles(); err != nil {
					helpers.StatusBarInst().UpdateStatusBar("Discard staged files failed: " + err.Error())
				} else {
					helpers.StatusBarInst().UpdateStatusBar("Discarded staged file changes")
				}
			}
		}
		if a.pagesRoot != nil {
			a.pagesRoot.HidePage("discard-files")
		}
	})
	a.pagesRoot.AddPage("discard-files", a.discardFilesModal, true, false)
	a.deleteNoteModal = tview.NewModal().
		AddButtons([]string{"Delete", "Cancel"})
	a.deleteNoteModal.SetDoneFunc(func(_ int, label string) {
		if label == "Delete" && a.notes != nil {
			target := a.deleteNoteLabel
			if target == "" {
				target = a.notes.FocusedNoteDeleteLabel()
			}
			var err error
			if a.deleteNoteFolder {
				err = a.notes.DeleteFolderByRel(a.deleteNotePath)
			} else {
				err = a.notes.DeleteNoteByPath(a.deleteNotePath)
			}
			if err == nil {
				if a.deleteNoteFolder {
					helpers.StatusBarInst().UpdateStatusBar("Deleted folder: " + target)
				} else {
					helpers.StatusBarInst().UpdateStatusBar("Deleted note: " + target)
					a.pushNoteDeleteToFirebaseSoon(a.deleteNotePath)
				}
			} else if a.deleteNoteFolder {
				helpers.StatusBarInst().UpdateStatusBar("Delete folder failed")
			} else {
				helpers.StatusBarInst().UpdateStatusBar("Delete note failed")
			}
		}
		a.deleteNotePath = ""
		a.deleteNoteLabel = ""
		a.deleteNoteFolder = false
		if a.pagesRoot != nil {
			a.pagesRoot.HidePage("delete-note")
		}
	})
	a.pagesRoot.AddPage("delete-note", a.deleteNoteModal, true, false)
	a.openLinksModal = tview.NewModal().
		AddButtons([]string{"Open all", "Cancel"})
	a.openLinksModal.SetDoneFunc(func(_ int, label string) {
		switch label {
		case "Open all":
			a.openAllPendingLinks()
		}
		a.openLinks = nil
		if a.pagesRoot != nil {
			a.pagesRoot.HidePage("open-links")
		}
	})
	a.pagesRoot.AddPage("open-links", a.openLinksModal, true, false)
	a.root = tview.NewFlex().SetDirection(tview.FlexRow)
	a.root.AddItem(a.header, 3, 0, false)
	a.root.AddItem(a.pagesRoot, 0, 1, false)
	a.root.AddItem(a.statusBar, 3, 0, false)
	a.applyWidgetBackgroundStyle()
}

func (a *terminalApp) Run(ctx context.Context) error {
	if !isTerminal(int(os.Stdin.Fd())) {
		return a.runLineMode(ctx)
	}
	app := tview.NewApplication()
	a.tui = app
	app.SetRoot(a.root, true)
	app.EnableMouse(true)
	app.SetInputCapture(a.captureInput)
	app.SetMouseCapture(a.captureMouse)
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		applyGlobalBackgroundStyle()
		screen.SetStyle(tcell.StyleDefault.Background(appBackgroundColor()).Foreground(tview.Styles.PrimaryTextColor))
		w, h := screen.Size()
		if w != a.width || h != a.height {
			a.width, a.height = w, h
			a.refresh()
		}
		return false
	})
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		a.showCursor(screen)
	})
	go a.watchStatus(ctx)
	a.startFirebaseTodoPolling(ctx)
	a.refresh()
	return app.Run()
}

func transparentBackgroundEnabled() bool {
	cfg := settings.Inst()
	return cfg != nil && cfg.UI != nil && cfg.UI.TransparentBackground
}

func appBackgroundColor() tcell.Color {
	if transparentBackgroundEnabled() {
		return tcell.ColorDefault
	}
	return themeColor(currentTheme().Background)
}

func applyGlobalBackgroundStyle() {
	theme := currentTheme()
	background := appBackgroundColor()
	contrastBackground := themeColor(theme.Panel)
	moreContrastBackground := themeColor(theme.SelectionBG)
	if transparentBackgroundEnabled() {
		contrastBackground = tcell.ColorDefault
		moreContrastBackground = tcell.ColorDefault
	}
	tview.Styles.PrimitiveBackgroundColor = background
	tview.Styles.ContrastBackgroundColor = contrastBackground
	tview.Styles.MoreContrastBackgroundColor = moreContrastBackground
	tview.Styles.PrimaryTextColor = themeColor(theme.Primary)
	tview.Styles.SecondaryTextColor = themeColor(theme.Secondary)
	tview.Styles.TertiaryTextColor = themeColor(theme.Dim)
	tview.Styles.BorderColor = themeColor(theme.Border)
	tview.Styles.TitleColor = themeColor(theme.Title)
	tview.Styles.GraphicsColor = themeColor(theme.Border)
	tview.Styles.ContrastSecondaryTextColor = themeColor(theme.SelectionFG)
}

func (a *terminalApp) applyWidgetBackgroundStyle() {
	if a == nil {
		return
	}
	theme := currentTheme()
	background := appBackgroundColor()
	for _, view := range []*tview.TextView{a.header, a.help, a.statusBar, a.sidebar, a.editor, a.preview, a.commandBar, a.single, a.helpOverlay} {
		if view != nil {
			view.SetBackgroundColor(background)
			view.SetTextColor(themeColor(theme.Primary))
			view.SetBorderColor(themeColor(theme.Border))
			view.SetTitleColor(themeColor(theme.Title))
		}
	}
	for _, box := range []*tview.Box{boxFromFlex(a.body), boxFromFlex(a.root), boxFromPages(a.pagesRoot)} {
		if box != nil {
			box.SetBackgroundColor(background)
		}
	}
	for _, modal := range []*tview.Modal{a.quitModal, a.discardFilesModal, a.deleteNoteModal, a.openLinksModal} {
		if modal != nil {
			modal.SetBackgroundColor(background)
			modal.SetTextColor(themeColor(theme.Primary))
			modal.SetButtonTextColor(themeColor(theme.Primary))
			modal.SetButtonBackgroundColor(themeColor(theme.SelectionBG))
		}
	}
	if a.statusBar != nil {
		a.statusBar.SetBorderColor(themeColor(theme.StatusAccent))
		a.statusBar.SetTitleColor(themeColor(theme.StatusAccent))
	}
	if a.commandBar != nil {
		a.commandBar.SetBorderColor(themeColor(theme.CommandAccent))
		a.commandBar.SetTitleColor(themeColor(theme.CommandAccent))
	}
}

func boxFromFlex(f *tview.Flex) *tview.Box {
	if f == nil {
		return nil
	}
	return f.Box
}

func boxFromPages(p *tview.Pages) *tview.Box {
	if p == nil {
		return nil
	}
	return p.Box
}

func (a *terminalApp) runLineMode(ctx context.Context) error {
	reader := bufio.NewScanner(os.Stdin)
	fmt.Println("Koko Tools TUI")
	fmt.Println("Non-interactive terminal detected. Type 'help' for commands, 'quit' to exit.")
	for {
		fmt.Print("> ")
		if !reader.Scan() {
			return reader.Err()
		}
		line := strings.TrimSpace(reader.Text())
		switch line {
		case "quit", "q", ":q":
			a.requestShutdown()
			return nil
		case "help":
			fmt.Println("views: notes, files, pages, password, todo, sync, settings")
			fmt.Println("save: :w, quit: :q, save and quit: :wq")
		case "notes":
			a.view = viewNotes
		case "files":
			a.view = viewFiles
		case "pages":
			a.view = viewPages
		case "password":
			a.view = viewPassword
		case "todo":
			a.view = viewTodo
		case "sync":
			a.view = viewSync
		case "settings":
			a.view = viewSettings
		case ":w":
			_ = a.saveLocalState()
		case ":wq":
			_ = a.saveLocalState()
			a.stopTUI()
			return nil
		case "save":
			_ = a.saveLocalState()
		default:
			fmt.Println("unknown command")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (a *terminalApp) captureInput(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return nil
	}
	if a.shuttingDown && a.quitModal != nil && a.pagesRoot != nil {
		if a.syncInProgress {
			return nil
		}
		if handler := a.quitModal.InputHandler(); handler != nil {
			handler(event, func(tview.Primitive) {})
		}
		return nil
	}
	if a.pagesRoot != nil && a.pagesRoot.HasPage("discard-files") {
		if front, _ := a.pagesRoot.GetFrontPage(); front == "discard-files" && a.discardFilesModal != nil {
			if handler := a.discardFilesModal.InputHandler(); handler != nil {
				handler(event, func(tview.Primitive) {})
			}
			return nil
		}
	}
	if a.pagesRoot != nil && a.pagesRoot.HasPage("delete-note") {
		if front, _ := a.pagesRoot.GetFrontPage(); front == "delete-note" && a.deleteNoteModal != nil {
			if handler := a.deleteNoteModal.InputHandler(); handler != nil {
				handler(event, func(tview.Primitive) {})
			}
			return nil
		}
	}
	if a.pagesRoot != nil && a.pagesRoot.HasPage("open-links") {
		if front, _ := a.pagesRoot.GetFrontPage(); front == "open-links" && a.openLinksModal != nil {
			if handler := a.openLinksModal.InputHandler(); handler != nil {
				handler(event, func(tview.Primitive) {})
			}
			return nil
		}
	}
	if a.showHelp && a.helpOverlay != nil {
		if a.helpSearchMode {
			if event.Key() == tcell.KeyEsc {
				a.helpSearchMode = false
				a.refresh()
				return nil
			}
			if event.Key() == tcell.KeyEnter {
				a.helpSearchMode = false
				a.refresh()
				return nil
			}
			if event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2 {
				if len(a.helpSearchQuery) > 0 {
					_, size := utf8.DecodeLastRuneInString(a.helpSearchQuery)
					a.helpSearchQuery = a.helpSearchQuery[:len(a.helpSearchQuery)-size]
					a.helpSearchIndex = 0
					a.refresh()
				}
				return nil
			}
			if event.Key() == tcell.KeyRune && event.Modifiers() == 0 {
				a.helpSearchQuery += string(event.Rune())
				a.helpSearchIndex = 0
				a.refresh()
				return nil
			}
			return nil
		}
		if event.Key() == tcell.KeyEsc || (event.Key() == tcell.KeyRune && (event.Rune() == '?' || event.Rune() == 'q')) {
			if key, ok := mapTCellKey(event); ok {
				a.handleGlobalKey(key)
				a.refresh()
			}
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Modifiers() == 0 {
			switch event.Rune() {
			case '/':
				a.helpSearchMode = true
				a.helpSearchQuery = ""
				a.helpSearchIndex = 0
				a.refresh()
				return nil
			case 'n':
				a.advanceHelpSearch(1)
				a.refresh()
				return nil
			case 'N':
				a.advanceHelpSearch(-1)
				a.refresh()
				return nil
			}
		}
		if handler := a.helpOverlay.InputHandler(); handler != nil {
			handler(event, func(tview.Primitive) {})
		}
		return nil
	}
	if a.consumeInputSequenceShortcut(event) {
		return nil
	}
	if a.recorderCapturing {
		if key, ok := mapTCellKey(event); ok {
			a.recordRecorderKey(key, "tcell", recorderTCellKeyName(event.Key()))
		} else {
			a.recordRecorderKey(notes.Key{Name: "unknown"}, "tcell", recorderTCellKeyName(event.Key()))
		}
		a.refresh()
		return nil
	}
	if key, ok := mapTCellKey(event); ok {
		a.handleGlobalKey(key)
		a.refresh()
		return nil
	}
	return nil
}

func (a *terminalApp) captureMouse(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if event == nil || a == nil || a.shuttingDown {
		return event, action
	}
	x, y := event.Position()
	if a.header != nil {
		hx, hy, hw, hh := a.header.GetInnerRect()
		if pointInRect(x, y, hx, hy, hw, hh) {
			if action == tview.MouseLeftClick || action == tview.MouseLeftDown {
				if target, ok := a.appTabAtColumn(x - hx); ok {
					a.switchAppTab(target)
					a.refresh()
					return nil, action
				}
			}
			return event, action
		}
	}
	if a.view != viewNotes || a.notes == nil || a.showHelp {
		return event, action
	}
	if !a.notes.SidebarBrowsing && settings.Inst().NotesApp.SidebarVisible && a.sidebar != nil {
		sx, sy, sw, sh := a.sidebar.GetInnerRect()
		if pointInRect(x, y, sx, sy, sw, sh) {
			row := y - sy
			switch action {
			case tview.MouseLeftClick, tview.MouseLeftDown:
				a.notes.SelectSidebarRow(row, false)
			case tview.MouseLeftDoubleClick:
				a.notes.SelectSidebarRow(row, true)
				a.consumePendingNoteActions()
			}
			a.refresh()
			return nil, action
		}
	}
	if a.editor == nil {
		return event, action
	}
	ex, ey, ew, eh := a.editor.GetInnerRect()
	if a.notes.SidebarBrowsing {
		if !pointInRect(x, y, ex, ey, ew, eh) {
			return event, action
		}
		row := y - ey
		switch action {
		case tview.MouseLeftClick, tview.MouseLeftDown:
			a.notes.SelectSidebarRow(row, false)
		case tview.MouseLeftDoubleClick:
			a.notes.SelectSidebarRow(row, true)
			a.consumePendingNoteActions()
		}
		a.refresh()
		return nil, action
	}
	if !pointInRect(x, y, ex, ey, ew, eh) && !a.notesMouseDragging {
		return event, action
	}
	row := y - ey - 1
	col := x - ex
	if row == -1 {
		if action == tview.MouseLeftClick {
			if a.notes.CloseTabAtColumn(col) {
				a.notesMouseDragging = false
				a.notesMouseMoved = false
				a.refresh()
				return nil, action
			}
		}
		if action == tview.MouseLeftClick || action == tview.MouseLeftDown {
			if a.notes.SwitchToTabAtColumn(col) {
				a.notesMouseDragging = false
				a.notesMouseMoved = false
				a.refresh()
				return nil, action
			}
		}
		return event, action
	}
	if row < 0 {
		row = 0
	}
	switch action {
	case tview.MouseLeftDown:
		a.notesMouseDragging = true
		a.notesMouseMoved = false
		a.notesMouseStartRow = row
		a.notesMouseStartCol = col
		a.notes.BeginMouseSelection(row, col)
	case tview.MouseMove:
		if a.notesMouseDragging {
			if row != a.notesMouseStartRow || col != a.notesMouseStartCol {
				a.notesMouseMoved = true
				a.notes.DragMouseSelection(row, col)
			}
		}
	case tview.MouseLeftUp:
		if a.notesMouseDragging {
			if a.notesMouseMoved || row != a.notesMouseStartRow || col != a.notesMouseStartCol {
				a.notesMouseMoved = true
				a.notes.DragMouseSelection(row, col)
			} else {
				a.notes.MoveEditorCursorToVisualPosition(row, col)
			}
			a.notesMouseDragging = false
		}
	case tview.MouseLeftClick:
		if a.notesMouseMoved {
			a.notesMouseMoved = false
			break
		}
		a.notesMouseDragging = false
		a.notes.MoveEditorCursorToVisualPosition(row, col)
	case tview.MouseScrollUp:
		a.notesMouseDragging = false
		a.notesMouseMoved = false
		a.notes.MoveEditorCursorByVisualRows(-3)
	case tview.MouseScrollDown:
		a.notesMouseDragging = false
		a.notesMouseMoved = false
		a.notes.MoveEditorCursorByVisualRows(3)
	}
	a.refresh()
	return nil, action
}

var inputSequenceShortcuts = map[string]notes.Key{
	"\x1b[9;5u":      {Name: "tab", Ctrl: true},
	"\x1b[27;5;9~":   {Name: "tab", Ctrl: true},
	"\x1b[1;5I":      {Name: "tab", Ctrl: true},
	"\x1b[103;5u":    {Name: "g", Rune: 'g', Ctrl: true},
	"\x1b[7;5u":      {Name: "g", Rune: 'g', Ctrl: true},
	"\x1b[27;5;103~": {Name: "g", Rune: 'g', Ctrl: true},
}

func (a *terminalApp) consumeInputSequenceShortcut(event *tcell.EventKey) bool {
	if a == nil || event == nil {
		return false
	}
	if event.Key() == tcell.KeyEsc && event.Modifiers() == 0 {
		a.startInputSequence()
		return true
	}
	token, ok := inputSequenceToken(event)
	if !ok {
		return a.flushInputSequence()
	}
	a.inputSeqMu.Lock()
	if a.inputSeq == "" {
		a.inputSeqMu.Unlock()
		return false
	}
	candidate := a.inputSeq + token
	if key, ok := inputSequenceShortcuts[candidate]; ok {
		a.clearInputSequenceLocked()
		a.inputSeqMu.Unlock()
		if a.recorderCapturing {
			a.recordRecorderKey(key, "sequence", candidate)
			a.refresh()
			return true
		}
		a.handleGlobalKey(key)
		a.refresh()
		return true
	}
	if inputSequenceHasPrefix(candidate) {
		a.inputSeq = candidate
		a.resetInputSequenceTimerLocked()
		a.inputSeqMu.Unlock()
		return true
	}
	a.clearInputSequenceLocked()
	a.inputSeqMu.Unlock()
	if a.recorderCapturing {
		a.recordRecorderKey(notes.Key{Name: "esc"}, "sequence", candidate)
		a.refresh()
		return true
	}
	a.handleGlobalKey(notes.Key{Name: "esc"})
	a.refresh()
	return false
}

func inputSequenceToken(event *tcell.EventKey) (string, bool) {
	if event == nil {
		return "", false
	}
	if event.Key() == tcell.KeyRune {
		return string(event.Rune()), true
	}
	if event.Key() == tcell.KeyTab {
		return "\t", true
	}
	return "", false
}

func inputSequenceHasPrefix(seq string) bool {
	for shortcut := range inputSequenceShortcuts {
		if strings.HasPrefix(shortcut, seq) {
			return true
		}
	}
	return false
}

func (a *terminalApp) startInputSequence() {
	a.inputSeqMu.Lock()
	a.inputSeq = "\x1b"
	a.resetInputSequenceTimerLocked()
	a.inputSeqMu.Unlock()
}

func (a *terminalApp) resetInputSequenceTimerLocked() {
	if a.inputSeqTimer != nil {
		a.inputSeqTimer.Stop()
	}
	a.inputSeqTimer = time.AfterFunc(40*time.Millisecond, func() {
		a.flushInputSequenceAsync()
	})
}

func (a *terminalApp) clearInputSequenceLocked() {
	if a.inputSeqTimer != nil {
		a.inputSeqTimer.Stop()
		a.inputSeqTimer = nil
	}
	a.inputSeq = ""
}

func (a *terminalApp) flushInputSequence() bool {
	a.inputSeqMu.Lock()
	if a.inputSeq == "" {
		a.inputSeqMu.Unlock()
		return false
	}
	a.clearInputSequenceLocked()
	a.inputSeqMu.Unlock()
	if a.recorderCapturing {
		a.recordRecorderKey(notes.Key{Name: "esc"}, "sequence", "esc")
		a.refresh()
		return true
	}
	a.handleGlobalKey(notes.Key{Name: "esc"})
	a.refresh()
	return true
}

func (a *terminalApp) flushInputSequenceAsync() {
	run := func() {
		_ = a.flushInputSequence()
	}
	if a.tui != nil {
		a.tui.QueueUpdateDraw(run)
		return
	}
	run()
}

func pointInRect(x int, y int, rx int, ry int, width int, height int) bool {
	return x >= rx && x < rx+width && y >= ry && y < ry+height
}

func (a *terminalApp) visibleAppTabs() []appTab {
	byID := make(map[string]appTab, len(baseAppTabs))
	for _, tab := range baseAppTabs {
		byID[tab.id] = tab
	}
	tabs := make([]appTab, 0, len(baseAppTabs)+1)
	for _, id := range settings.UITabOrder() {
		tab, ok := byID[id]
		if !ok {
			continue
		}
		tab.key = strconv.Itoa(len(tabs) + 1)
		tabs = append(tabs, tab)
	}
	if a != nil && a.recorderVisible {
		tabs = append(tabs, appTab{id: "recorder", label: "Recorder", view: viewRecorder, key: strconv.Itoa(len(tabs) + 1)})
	}
	return tabs
}

func (a *terminalApp) appTabViewForKey(key string) (view, bool) {
	for _, tab := range a.visibleAppTabs() {
		if tab.key == key {
			return tab.view, true
		}
	}
	return viewNotes, false
}

func (a *terminalApp) appTabKeyHint() string {
	tabs := a.visibleAppTabs()
	if len(tabs) == 0 {
		return "1"
	}
	return "1-" + tabs[len(tabs)-1].key
}

func (a *terminalApp) switchAppTab(target view) {
	a.view = target
	a.tabSelect = false
}

func (a *terminalApp) appTabAtColumn(col int) (view, bool) {
	if col < 0 {
		return viewNotes, false
	}
	pos := 0
	tabs := a.visibleAppTabs()
	for i, tab := range tabs {
		labelName := tab.label
		if a != nil && a.viewDirty(tab.view) {
			labelName += "*"
		}
		label := fmt.Sprintf(" %s:%s ", tab.key, labelName)
		next := pos + len([]rune(label))
		if col >= pos && col < next {
			return tab.view, true
		}
		pos = next
		if i < len(tabs)-1 {
			if col == pos {
				return viewNotes, false
			}
			pos++
		}
	}
	return viewNotes, false
}

func mapTCellKey(event *tcell.EventKey) (notes.Key, bool) {
	switch event.Key() {
	case tcell.KeyRune:
		r := event.Rune()
		if key, ok := mapControlRune(r); ok {
			return key, true
		}
		if r == ' ' && event.Modifiers() == tcell.ModNone {
			return notes.Key{Name: "space", Rune: ' '}, true
		}
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Ctrl: true, Name: string(r), Rune: r}, true
		}
		return notes.Key{Name: string(r), Rune: r, Shift: event.Modifiers()&tcell.ModShift != 0, Alt: event.Modifiers()&tcell.ModAlt != 0}, true
	case tcell.KeyEnter:
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Name: "enter", Ctrl: true}, true
		}
		return notes.Key{Name: "enter"}, true
	case tcell.KeyTab:
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Name: "tab", Ctrl: true}, true
		}
		return notes.Key{Name: "tab"}, true
	case tcell.KeyBacktab:
		return notes.Key{Shift: true, Name: "tab"}, true
	case tcell.KeyEsc:
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Name: "3", Ctrl: true}, true
		}
		return notes.Key{Name: "esc"}, true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return notes.Key{Name: "backspace"}, true
	case tcell.KeyDelete:
		return notes.Key{Name: "delete"}, true
	case tcell.KeyUp:
		return notes.Key{Name: "up"}, true
	case tcell.KeyDown:
		return notes.Key{Name: "down"}, true
	case tcell.KeyLeft:
		if event.Modifiers()&(tcell.ModMeta|tcell.ModAlt) != 0 {
			return notes.Key{Name: "left", Meta: true}, true
		}
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Name: "left", Ctrl: true}, true
		}
		return notes.Key{Name: "left"}, true
	case tcell.KeyRight:
		if event.Modifiers()&(tcell.ModMeta|tcell.ModAlt) != 0 {
			return notes.Key{Name: "right", Meta: true}, true
		}
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Name: "right", Ctrl: true}, true
		}
		return notes.Key{Name: "right"}, true
	case tcell.KeyHome:
		return notes.Key{Name: "home", Meta: event.Modifiers()&(tcell.ModMeta|tcell.ModAlt) != 0, Ctrl: event.Modifiers()&tcell.ModCtrl != 0}, true
	case tcell.KeyEnd:
		return notes.Key{Name: "end", Meta: event.Modifiers()&(tcell.ModMeta|tcell.ModAlt) != 0, Ctrl: event.Modifiers()&tcell.ModCtrl != 0}, true
	case tcell.KeyPgUp:
		return notes.Key{Name: "pageup"}, true
	case tcell.KeyPgDn:
		return notes.Key{Name: "pagedown"}, true
	case tcell.KeyCtrlS:
		return notes.Key{Name: "s", Ctrl: true}, true
	case tcell.KeyCtrlT:
		return notes.Key{Name: "t", Ctrl: true}, true
	case tcell.KeyCtrlN:
		return notes.Key{Name: "n", Ctrl: true}, true
	case tcell.KeyCtrlD:
		return notes.Key{Name: "d", Ctrl: true}, true
	case tcell.KeyCtrlE:
		return notes.Key{Name: "e", Ctrl: true}, true
	case tcell.KeyCtrlA:
		return notes.Key{Name: "a", Ctrl: true}, true
	case tcell.KeyCtrlV:
		return notes.Key{Name: "v", Ctrl: true}, true
	case tcell.KeyCtrlBackslash:
		return notes.Key{Name: "4", Ctrl: true}, true
	case tcell.KeyCtrlRightSq:
		return notes.Key{Name: "5", Ctrl: true}, true
	case tcell.KeyCtrlCarat:
		return notes.Key{Name: "6", Ctrl: true}, true
	default:
		return mapControlTCellKey(event)
	}
}

func mapControlRune(r rune) (notes.Key, bool) {
	switch r {
	case 1, 2, 4, 5, 6, 7, 8, 11, 12, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26:
		keyRune := rune('a' + (r - 1))
		return notes.Key{Name: string(keyRune), Rune: keyRune, Ctrl: true}, true
	case 28:
		return notes.Key{Name: "4", Ctrl: true}, true
	case 29:
		return notes.Key{Name: "5", Ctrl: true}, true
	case 30:
		return notes.Key{Name: "6", Ctrl: true}, true
	case 31:
		return notes.Key{Name: "7", Ctrl: true}, true
	default:
		return notes.Key{}, false
	}
}

func mapControlTCellKey(event *tcell.EventKey) (notes.Key, bool) {
	if event == nil {
		return notes.Key{}, false
	}
	switch key := event.Key(); {
	case key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ:
		r := rune('a' + (key - tcell.KeyCtrlA))
		return notes.Key{Name: string(r), Ctrl: true}, true
	case key == tcell.KeyCtrlSpace:
		return notes.Key{Name: "space", Ctrl: true}, true
	case key == tcell.KeyCtrlBackslash:
		return notes.Key{Name: "4", Ctrl: true}, true
	case key == tcell.KeyCtrlRightSq:
		return notes.Key{Name: "5", Ctrl: true}, true
	case key == tcell.KeyCtrlCarat:
		return notes.Key{Name: "6", Ctrl: true}, true
	case key == tcell.KeyCtrlUnderscore:
		return notes.Key{Name: "7", Ctrl: true}, true
	default:
		return notes.Key{}, false
	}
}

func (a *terminalApp) handleGlobalKey(key notes.Key) bool {
	if a.shuttingDown {
		return true
	}
	if a.recorderCapturing {
		return true
	}
	if key.Ctrl && key.Name == "s" {
		if err := a.saveLocalState(); err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Save failed: " + err.Error())
		} else {
			helpers.StatusBarInst().UpdateStatusBar("Saved locally at " + formatTimestampOrNever(settings.Inst().GDrive.LastLocalSaveAt))
		}
		return true
	}
	if key.Ctrl {
		switch key.Name {
		case "tab":
			a.switchAppTab(a.nextView())
			return true
		case "t":
			a.tabSelect = !a.tabSelect
			return true
		}
		if target, ok := a.appTabViewForKey(key.Name); ok {
			a.switchAppTab(target)
			return true
		}
	}
	if !key.Ctrl && !key.Shift && !key.Meta && !key.Alt && key.Name == "tab" && a.wantsPlainTabAppCycle() {
		a.switchAppTab(a.nextView())
		return true
	}
	if !key.Ctrl && key.Name == "?" && a.wantsHelpToggle() {
		a.showHelp = !a.showHelp
		if a.showHelp && a.helpOverlay != nil {
			a.helpSearchMode = false
			a.helpOverlay.ScrollToBeginning()
		}
		return true
	}
	if a.showHelp {
		if key.Name == "esc" || key.Name == "?" || key.Name == "q" {
			a.showHelp = false
			a.helpSearchMode = false
			return true
		}
		return true
	}
	if !key.Ctrl && key.Name == "q" && a.wantsQuitOnQ() {
		a.requestShutdown()
		return true
	}
	if a.tabSelect && !key.Meta && !key.Ctrl {
		if target, ok := a.appTabViewForKey(key.Name); ok {
			a.switchAppTab(target)
			return true
		}
		switch key.Name {
		case "left":
			a.view = a.prevView()
			return true
		case "right":
			a.view = a.nextView()
			return true
		case "enter":
			a.tabSelect = false
			return true
		case "esc":
			a.tabSelect = false
			return true
		}
	}
	switch a.view {
	case viewNotes:
		handled := a.notes.HandleKey(key)
		a.consumePendingNoteActions()
		a.scheduleYankHighlightClear()
		return handled
	case viewFiles:
		if !key.Ctrl && key.Name == "D" && a.notes != nil {
			if !a.notes.FilesDirty {
				helpers.StatusBarInst().UpdateStatusBar("No staged file changes")
				return true
			}
			if a.pagesRoot != nil {
				a.pagesRoot.ShowPage("discard-files")
			}
			if a.tui != nil && a.discardFilesModal != nil {
				a.tui.SetFocus(a.discardFilesModal)
			}
			return true
		}
		return a.notes.HandleFilesKey(key)
	case viewPages:
		return a.handlePagesKey(key)
	case viewPassword:
		return a.handlePasswordKey(key)
	case viewTodo:
		return a.handleTodoKey(key)
	case viewSync:
		return a.handleSyncKey(key)
	case viewSettings:
		return a.handleSettingsKey(key)
	case viewRecorder:
		return false
	default:
		return false
	}
}

func (a *terminalApp) scheduleYankHighlightClear() {
	if a == nil || a.notes == nil || a.tui == nil || !a.notes.HasActiveYankHighlight() {
		return
	}
	go func() {
		time.Sleep(notes.YankHighlightDuration() + 20*time.Millisecond)
		a.tui.QueueUpdateDraw(func() {
			a.refresh()
		})
	}()
}

func (a *terminalApp) consumePendingNoteActions() {
	if a == nil || a.notes == nil {
		return
	}
	if path, label, folder, ok := a.notes.TakePendingDeleteTarget(); ok {
		a.deleteNotePath = path
		a.deleteNoteLabel = label
		a.deleteNoteFolder = folder
		a.showDeleteNoteModal()
		return
	}
	if a.notes.TakePendingSaveAll() {
		if err := a.saveLocalState(); err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Save failed: " + err.Error())
			a.notes.TakePendingQuit()
			return
		} else {
			if ed := a.notes.ActiveEditor(); ed != nil {
				ed.Status = "saved"
			}
			helpers.StatusBarInst().UpdateStatusBar("Saved locally at " + formatTimestampOrNever(settings.Inst().GDrive.LastLocalSaveAt))
		}
	}
	if a.notes.TakePendingRecordKeys() {
		a.startRecorderCapture()
		return
	}
	if quit, force := a.notes.TakePendingQuit(); quit {
		if force {
			a.stopTUI()
		} else {
			a.requestShutdown()
		}
		return
	}
	links := a.notes.TakePendingOpenLinks()
	if len(links) == 0 {
		return
	}
	a.openLinks = links
	if a.openLinksModal != nil {
		title := "Active note"
		if ed := a.notes.ActiveEditor(); ed != nil && strings.TrimSpace(ed.Title) != "" {
			title = ed.Title
		}
		a.openLinksModal.SetText(formatOpenLinksModalText(title, links))
	}
	if a.pagesRoot != nil {
		a.pagesRoot.ShowPage("open-links")
	}
	if a.tui != nil && a.openLinksModal != nil {
		a.tui.SetFocus(a.openLinksModal)
	}
}

func (a *terminalApp) startRecorderCapture() {
	if a == nil {
		return
	}
	captureID := a.recorderCaptureID.Add(1)
	a.recorderVisible = true
	a.view = viewRecorder
	a.tabSelect = false
	a.recorderCapturing = true
	a.recorderStartedAt = time.Now()
	a.recorderEndsAt = a.recorderStartedAt.Add(recorderDuration)
	a.recorderEvents = nil
	a.recorderLastEvent = recordedKeyEvent{}
	helpers.StatusBarInst().UpdateStatusBar("Recording keys for 5 seconds")
	time.AfterFunc(recorderDuration, func() {
		if a.recorderCaptureID.Load() != captureID {
			return
		}
		run := func() {
			if a.recorderCaptureID.Load() != captureID {
				return
			}
			a.stopRecorderCapture()
			a.refresh()
		}
		if a.tui != nil {
			a.tui.QueueUpdateDraw(run)
			return
		}
		run()
	})
}

func (a *terminalApp) stopRecorderCapture() {
	if a == nil || !a.recorderCapturing {
		return
	}
	a.recorderCapturing = false
	helpers.StatusBarInst().UpdateStatusBar(fmt.Sprintf("Recorded %d key event(s)", len(a.recorderEvents)))
}

func (a *terminalApp) openAllPendingLinks() {
	if a == nil || len(a.openLinks) == 0 {
		return
	}
	for _, link := range a.openLinks {
		helpers.OpenURI(link)
	}
	helpers.StatusBarInst().UpdateStatusBar(fmt.Sprintf("Opened %d links", len(a.openLinks)))
}

func (a *terminalApp) nextView() view {
	tabs := a.visibleAppTabs()
	for i, tab := range tabs {
		if tab.view == a.view {
			return tabs[(i+1)%len(tabs)].view
		}
	}
	return tabs[0].view
}

func (a *terminalApp) prevView() view {
	tabs := a.visibleAppTabs()
	for i, tab := range tabs {
		if tab.view == a.view {
			return tabs[(i+len(tabs)-1)%len(tabs)].view
		}
	}
	return tabs[len(tabs)-1].view
}

func (a *terminalApp) wantsQuitOnQ() bool {
	switch a.view {
	case viewNotes:
		return a.notes == nil || !a.notes.IsEditableContext()
	case viewFiles:
		return a.notes == nil || !a.notes.IsFilesEditableContext()
	case viewPages:
		return a.pages == nil || !a.pages.IsEditing()
	case viewTodo:
		return a.todoInputMode == ""
	case viewSettings:
		return !a.settingsEditMode && !a.tabOrderEditMode
	default:
		return true
	}
}

func (a *terminalApp) wantsPlainTabAppCycle() bool {
	if a == nil {
		return false
	}
	if a.tabSelect {
		return true
	}
	switch a.view {
	case viewNotes:
		return a.notes == nil || !a.notes.IsEditableContext()
	case viewFiles:
		return a.notes == nil || !a.notes.IsFilesEditableContext()
	case viewPages:
		return a.pages == nil || !a.pages.IsEditing()
	case viewTodo:
		return a.todoInputMode == ""
	case viewSettings:
		return !a.settingsEditMode && !a.tabOrderEditMode
	default:
		return true
	}
}

func (a *terminalApp) wantsHelpToggle() bool {
	if a.tabSelect {
		return true
	}
	switch a.view {
	case viewNotes:
		return a.notes == nil || !a.notes.IsEditableContext()
	case viewFiles:
		return a.notes == nil || !a.notes.IsFilesEditableContext()
	case viewPages:
		return a.pages == nil || !a.pages.IsEditing()
	case viewTodo:
		return a.todoInputMode == ""
	case viewSettings:
		return !a.settingsEditMode && !a.tabOrderEditMode
	default:
		return true
	}
}

func (a *terminalApp) handlePagesKey(key notes.Key) bool {
	if a.pages == nil {
		return false
	}
	if a.pages.IsEditing() {
		if key.Name == "tab" && key.Shift {
			return a.pages.HandleEditKey("shift+tab", key.Rune)
		}
		return a.pages.HandleEditKey(key.Name, key.Rune)
	}
	switch key.Name {
	case "down", "j", "tab":
		a.pages.Move(1)
		return true
	case "up", "k":
		a.pages.Move(-1)
		return true
	case "enter", "e":
		a.pages.StartEditing()
		return true
	case "r":
		a.pages.Recalculate()
		a.pages.Dirty = true
		return true
	}
	return false
}

func (a *terminalApp) handlePasswordKey(key notes.Key) bool {
	switch key.Name {
	case "g":
		a.generatePasswordAndNotify()
		return true
	case "l":
		a.password.Letters = !a.password.Letters
		a.generatePasswordAndNotify()
		return true
	case "n":
		a.password.Numbers = !a.password.Numbers
		a.generatePasswordAndNotify()
		return true
	case "s":
		a.password.SpecialSymbols = !a.password.SpecialSymbols
		a.generatePasswordAndNotify()
		return true
	case "+":
		a.password.SymbolCount++
		a.generatePasswordAndNotify()
		return true
	case "-":
		if a.password.SymbolCount > 1 {
			a.password.SymbolCount--
		}
		a.generatePasswordAndNotify()
		return true
	}
	return false
}

func (a *terminalApp) generatePasswordAndNotify() {
	if a == nil || a.password == nil {
		return
	}
	a.password.Generate()
	if strings.TrimSpace(a.password.Password) == "" {
		helpers.StatusBarInst().UpdateStatusBar("Password was not generated")
		return
	}
	if err := helpers.CopyToClipboard(a.password.Password); err != nil {
		helpers.StatusBarInst().UpdateStatusBar("Password generated, but clipboard copy failed: " + err.Error())
		return
	}
	helpers.StatusBarInst().UpdateStatusBar("Password generated and copied to clipboard")
}

func (a *terminalApp) handleTodoKey(key notes.Key) bool {
	if a.todos == nil {
		return false
	}
	if a.todoInputMode != "" {
		switch key.Name {
		case "enter":
			text := strings.TrimSpace(a.todoInputBuffer)
			if text == "" {
				a.todoInputMode = ""
				a.todoInputBuffer = ""
				a.todoInputCursorOffset = 0
				a.todoEditID = ""
				return true
			}
			var err error
			if a.todoInputMode == "edit" {
				a.todoStore, err = a.todos.Edit(a.todoEditID, text)
			} else {
				a.todoStore, _, err = a.todos.Add(text)
			}
			if err != nil {
				helpers.StatusBarInst().UpdateStatusBar("Todo save failed: " + err.Error())
			} else {
				a.markTodoChanged()
			}
			a.todoInputMode = ""
			a.todoInputBuffer = ""
			a.todoInputCursorOffset = 0
			a.todoEditID = ""
			a.clampTodoIndex()
			return true
		case "esc":
			a.todoInputMode = ""
			a.todoInputBuffer = ""
			a.todoInputCursorOffset = 0
			a.todoEditID = ""
			return true
		case "backspace":
			runes := []rune(a.todoInputBuffer)
			a.clampTodoInputCursor(len(runes))
			if a.todoInputCursorOffset > 0 {
				a.todoInputBuffer = string(append(runes[:a.todoInputCursorOffset-1], runes[a.todoInputCursorOffset:]...))
				a.todoInputCursorOffset--
			}
			return true
		case "delete":
			runes := []rune(a.todoInputBuffer)
			a.clampTodoInputCursor(len(runes))
			if a.todoInputCursorOffset < len(runes) {
				a.todoInputBuffer = string(append(runes[:a.todoInputCursorOffset], runes[a.todoInputCursorOffset+1:]...))
			}
			return true
		case "left":
			if a.todoInputCursorOffset > 0 {
				a.todoInputCursorOffset--
			}
			return true
		case "right":
			if a.todoInputCursorOffset < len([]rune(a.todoInputBuffer)) {
				a.todoInputCursorOffset++
			}
			return true
		case "home":
			a.todoInputCursorOffset = 0
			return true
		case "end":
			a.todoInputCursorOffset = len([]rune(a.todoInputBuffer))
			return true
		}
		if key.Rune != 0 && !key.Ctrl && !key.Meta && !key.Alt {
			runes := []rune(a.todoInputBuffer)
			a.clampTodoInputCursor(len(runes))
			next := append([]rune{}, runes[:a.todoInputCursorOffset]...)
			next = append(next, key.Rune)
			next = append(next, runes[a.todoInputCursorOffset:]...)
			a.todoInputBuffer = string(next)
			a.todoInputCursorOffset++
			return true
		}
		return true
	}
	a.reloadTodosForRender()
	rows := a.todoSelectableItems()
	switch key.Name {
	case "down", "j":
		if a.todoIndex < len(rows)-1 {
			a.todoIndex++
		}
		return true
	case "up", "k":
		if a.todoIndex > 0 {
			a.todoIndex--
		}
		return true
	case "n":
		a.todoInputMode = "new"
		a.todoInputBuffer = ""
		a.todoInputCursorOffset = 0
		return true
	case "e":
		if item, ok := a.selectedTodoItem(); ok && item.Status == todo.StatusTodo && item.CheckedAt == nil {
			a.todoInputMode = "edit"
			a.todoInputBuffer = item.Text
			a.todoInputCursorOffset = len([]rune(a.todoInputBuffer))
			a.todoEditID = item.ID
		}
		return true
	case "enter", "space", " ":
		if item, ok := a.selectedTodoItem(); ok && item.Status != todo.StatusArchived {
			store, err := a.todos.Toggle(item.ID)
			if err != nil {
				helpers.StatusBarInst().UpdateStatusBar("Todo update failed: " + err.Error())
			} else {
				a.todoStore = store
				a.markTodoChanged()
				a.scheduleTodoRefresh()
			}
		}
		return true
	case "J":
		return a.moveSelectedTodo(1)
	case "K":
		return a.moveSelectedTodo(-1)
	}
	return false
}

func (a *terminalApp) clampTodoInputCursor(bufferLen int) {
	if a.todoInputCursorOffset < 0 {
		a.todoInputCursorOffset = 0
	}
	if a.todoInputCursorOffset > bufferLen {
		a.todoInputCursorOffset = bufferLen
	}
}

func (a *terminalApp) moveSelectedTodo(delta int) bool {
	item, ok := a.selectedTodoItem()
	if !ok || item.Status != todo.StatusTodo || item.CheckedAt != nil || a.todos == nil {
		return true
	}
	store, err := a.todos.Move(item.ID, delta)
	if err != nil {
		helpers.StatusBarInst().UpdateStatusBar("Todo reorder failed: " + err.Error())
		return true
	}
	a.todoStore = store
	a.markTodoChanged()
	a.selectTodoByID(item.ID)
	return true
}

func (a *terminalApp) reloadTodosForRender() {
	if a == nil || a.todos == nil {
		return
	}
	store, err := a.todos.Load()
	if err != nil {
		helpers.StatusBarInst().UpdateStatusBar("Todo load failed: " + err.Error())
		return
	}
	a.todoStore = store
	a.clampTodoIndex()
}

func (a *terminalApp) markTodoChanged() {
	a.todoDirty = true
	a.settingsDirty = true
	settings.MarkDriveDirty()
	helpers.StatusBarInst().UpdateStatusBar("Todo saved")
	a.pushTodosToFirebaseSoon()
}

func (a *terminalApp) scheduleTodoRefresh() {
	if a == nil || a.tui == nil {
		return
	}
	time.AfterFunc(todo.CheckedDelay+50*time.Millisecond, func() {
		a.tui.QueueUpdateDraw(func() {
			a.reloadTodosForRender()
			a.refresh()
		})
	})
}

func (a *terminalApp) todoSelectableItems() []todo.Item {
	items := make([]todo.Item, 0)
	items = append(items, todo.ActiveItems(a.todoStore)...)
	items = append(items, todo.DoneItems(a.todoStore)...)
	for _, month := range todo.ArchiveMonths(a.todoStore) {
		items = append(items, todo.ArchiveGroups(a.todoStore)[month]...)
	}
	return items
}

func (a *terminalApp) selectedTodoItem() (todo.Item, bool) {
	items := a.todoSelectableItems()
	if len(items) == 0 {
		return todo.Item{}, false
	}
	a.clampTodoIndex()
	return items[a.todoIndex], true
}

func (a *terminalApp) selectTodoByID(id string) {
	items := a.todoSelectableItems()
	for i, item := range items {
		if item.ID == id {
			a.todoIndex = i
			return
		}
	}
	a.clampTodoIndex()
}

func (a *terminalApp) clampTodoIndex() {
	items := a.todoSelectableItems()
	if len(items) == 0 {
		a.todoIndex = 0
		return
	}
	if a.todoIndex < 0 {
		a.todoIndex = 0
	}
	if a.todoIndex >= len(items) {
		a.todoIndex = len(items) - 1
	}
}

func (a *terminalApp) handleSyncKey(key notes.Key) bool {
	items := a.syncItems()
	switch key.Name {
	case "down", "j":
		if a.syncIndex < len(items)-1 {
			a.syncIndex++
		}
		return true
	case "up", "k":
		if a.syncIndex > 0 {
			a.syncIndex--
		}
		return true
	case "enter", "space":
		items[a.syncIndex].Apply()
		return true
	}
	return false
}

func (a *terminalApp) queueUIDraw(fn func()) {
	if fn == nil {
		return
	}
	if a != nil && a.tui != nil {
		a.tui.QueueUpdateDraw(fn)
		return
	}
	fn()
}

func (a *terminalApp) stopTUI() {
	if a == nil {
		return
	}
	a.shuttingDown = false
	if a.pagesRoot != nil {
		a.pagesRoot.HidePage("quit")
	}
	if a.tui != nil {
		go a.tui.Stop()
	}
}

func (a *terminalApp) cancelShutdown() {
	if a == nil {
		return
	}
	a.shuttingDown = false
	if a.pagesRoot != nil {
		a.pagesRoot.HidePage("quit")
	}
}

func (a *terminalApp) startSyncOperation(label string, work func() error, onSuccess func(), onError func(error)) {
	if a == nil || work == nil {
		return
	}
	if a.syncInProgress {
		helpers.StatusBarInst().UpdateStatusBar("Sync already in progress")
		return
	}
	timeout := a.syncTimeout
	if timeout <= 0 {
		timeout = manualSyncTimeout
	}
	opID := a.syncOpID.Add(1)
	a.syncInProgress = true
	a.syncProgressLabel = label
	a.syncSpinnerTick.Store(0)
	done := make(chan struct{})
	finish := func(err error) {
		a.queueUIDraw(func() {
			if a.syncOpID.Load() != opID {
				return
			}
			a.syncOpID.Store(0)
			a.syncInProgress = false
			a.syncProgressLabel = ""
			a.syncSpinnerTick.Store(0)
			if err != nil {
				if onError != nil {
					onError(err)
				}
				return
			}
			if onSuccess != nil {
				onSuccess()
			}
		})
	}
	go func() {
		select {
		case <-time.After(timeout):
			finish(fmt.Errorf("manual Drive operation timed out after %s", timeout))
		case <-done:
		}
	}()
	go func() {
		err := work()
		close(done)
		finish(err)
	}()
}

func (a *terminalApp) syncSpinnerFrame() string {
	frames := []string{"/", "-", "\\", "|"}
	idx := a.syncSpinnerTick.Load()
	return frames[idx%int64(len(frames))]
}

func (a *terminalApp) handleSettingsKey(key notes.Key) bool {
	if a.tabOrderEditMode {
		switch key.Name {
		case "esc", "enter":
			a.tabOrderEditMode = false
			helpers.StatusBarInst().UpdateStatusBar("Tab order edit closed")
			return true
		case "down", "j":
			a.moveTabOrderSelection(1, false)
			return true
		case "up", "k":
			a.moveTabOrderSelection(-1, false)
			return true
		case "J":
			a.moveTabOrderSelection(1, true)
			return true
		case "K":
			a.moveTabOrderSelection(-1, true)
			return true
		}
		return true
	}
	if a.settingsEditMode {
		switch key.Name {
		case "esc":
			a.settingsEditMode = false
			a.settingsEditBuffer = ""
			helpers.StatusBarInst().UpdateStatusBar("Undo levels edit canceled")
			return true
		case "backspace":
			if len(a.settingsEditBuffer) > 0 {
				_, size := utf8.DecodeLastRuneInString(a.settingsEditBuffer)
				a.settingsEditBuffer = a.settingsEditBuffer[:len(a.settingsEditBuffer)-size]
			}
			return true
		case "enter":
			value, err := strconv.Atoi(strings.TrimSpace(a.settingsEditBuffer))
			if err != nil || value <= 0 {
				helpers.StatusBarInst().UpdateStatusBar("Undo levels must be a positive integer")
				a.settingsEditMode = false
				a.settingsEditBuffer = ""
				return true
			}
			settings.Inst().NotesApp.UndoLevels = value
			a.markSettingsChanged()
			a.settingsEditMode = false
			a.settingsEditBuffer = ""
			helpers.StatusBarInst().UpdateStatusBar(fmt.Sprintf("Undo levels set to %d", value))
			return true
		}
		if key.Rune >= '0' && key.Rune <= '9' {
			a.settingsEditBuffer += string(key.Rune)
			return true
		}
		return true
	}
	items := a.settingsItems()
	switch key.Name {
	case "down", "j":
		if a.settingIndex < len(items)-1 {
			a.settingIndex++
		}
		return true
	case "up", "k":
		if a.settingIndex > 0 {
			a.settingIndex--
		}
		return true
	case "enter", "space":
		items[a.settingIndex].Apply()
		return true
	}
	return false
}

func (a *terminalApp) moveTabOrderSelection(delta int, reorder bool) {
	cfg := settings.Inst()
	if cfg.UI == nil {
		cfg.UI = &settings.UISettings{}
	}
	cfg.UI.TabOrder = settings.NormalizeTabOrder(cfg.UI.TabOrder)
	if len(cfg.UI.TabOrder) == 0 {
		cfg.UI.TabOrder = settings.DefaultTabOrder()
	}
	if a.tabOrderIndex < 0 {
		a.tabOrderIndex = 0
	}
	if a.tabOrderIndex >= len(cfg.UI.TabOrder) {
		a.tabOrderIndex = len(cfg.UI.TabOrder) - 1
	}
	next := a.tabOrderIndex + delta
	if next < 0 || next >= len(cfg.UI.TabOrder) {
		return
	}
	if reorder {
		cfg.UI.TabOrder[a.tabOrderIndex], cfg.UI.TabOrder[next] = cfg.UI.TabOrder[next], cfg.UI.TabOrder[a.tabOrderIndex]
		a.markSettingsChanged()
		helpers.StatusBarInst().UpdateStatusBar("Tab order updated")
	}
	a.tabOrderIndex = next
}

func (a *terminalApp) markSettingsChanged() {
	a.settingsDirty = true
	settings.SaveSettingsLocal()
	a.pushSettingsToFirebaseSoon()
}

func (a *terminalApp) requestShutdown() {
	if a.shuttingDown {
		return
	}
	if a.hasUnsavedChanges() || a.hasUnsyncedChanges() {
		a.shuttingDown = true
		a.showHelp = false
		if a.quitModal != nil {
			a.quitModal.SetText("Unsaved or unsynced changes. Save before closing?")
		}
		if a.pagesRoot != nil {
			a.pagesRoot.ShowPage("quit")
		}
		if a.tui != nil && a.quitModal != nil {
			a.tui.SetFocus(a.quitModal)
		}
		return
	}
	if a.tui != nil {
		a.tui.Stop()
	}
}

func (a *terminalApp) shutdownAndStop() {
	if a != nil {
		_ = a.saveLocalState()
		if a.tui != nil {
			a.tui.Stop()
		}
	}
}

func (a *terminalApp) showDeleteNoteModal() {
	if a == nil || a.notes == nil {
		return
	}
	if a.deleteNotePath == "" {
		if !a.notes.CanDeleteFocusedNote() {
			return
		}
		a.deleteNotePath = a.notes.FocusedNoteDeletePath()
		a.deleteNoteFolder = false
	}
	label := a.deleteNoteLabel
	if strings.TrimSpace(label) == "" {
		label = a.notes.FocusedNoteDeleteLabel()
	}
	if strings.TrimSpace(label) == "" {
		if a.deleteNoteFolder {
			label = "selected folder"
		} else {
			label = "current note"
		}
	}
	a.deleteNoteLabel = label
	if a.deleteNoteModal != nil {
		if a.deleteNoteFolder {
			a.deleteNoteModal.SetText(fmt.Sprintf("Delete folder %q and all notes inside?", label))
		} else {
			a.deleteNoteModal.SetText(fmt.Sprintf("Delete note %q?", label))
		}
	}
	if a.pagesRoot != nil {
		a.pagesRoot.ShowPage("delete-note")
	}
	if a.tui != nil && a.deleteNoteModal != nil {
		a.tui.SetFocus(a.deleteNoteModal)
	}
}

type actionItem struct {
	Label string
	Apply func()
}

func (a *terminalApp) syncItems() []actionItem {
	cfg := settings.Inst().GDrive
	firebase := settings.Inst().Firebase
	if firebase == nil {
		firebase = &settings.FirebaseSettings{Realtime: true}
		settings.Inst().Firebase = firebase
	}
	items := []actionItem{
		{Label: fmt.Sprintf("firebase realtime enabled: %t", firebase.Enabled), Apply: func() {
			firebase.Enabled = !firebase.Enabled
			if firebase.Realtime == false {
				firebase.Realtime = true
			}
			a.markSettingsChanged()
			if firebase.Enabled {
				if err := a.configureFirebaseTodoSyncer(context.Background()); err != nil {
					helpers.StatusBarInst().UpdateStatusBar("Firebase sync not ready: " + err.Error())
				} else {
					helpers.StatusBarInst().UpdateStatusBar("Firebase realtime sync enabled")
				}
			}
		}},
		{Label: fmt.Sprintf("firebase workspace: %s", firebaseWorkspaceLabel(firebase)), Apply: func() {
			helpers.StatusBarInst().UpdateStatusBar("Set firebase.workspace_id in settings.json or KOKO_FIREBASE_WORKSPACE_ID")
		}},
		{Label: "login firebase with google", Apply: func() {
			a.loginFirebaseWithGoogle()
		}},
		{Label: "pull todos from firebase", Apply: func() {
			a.startSyncOperation("pull todos from firebase", func() error {
				return a.pullTodosFromFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase todos pulled")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase pull failed: " + err.Error())
			})
		}},
		{Label: "push todos to firebase", Apply: func() {
			a.startSyncOperation("push todos to firebase", func() error {
				return a.pushTodosToFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase todos pushed")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase push failed: " + err.Error())
			})
		}},
		{Label: "pull notes from firebase", Apply: func() {
			a.startSyncOperation("pull notes from firebase", func() error {
				return a.pullNotesFromFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase notes pulled")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase notes pull failed: " + err.Error())
			})
		}},
		{Label: "push notes to firebase", Apply: func() {
			a.startSyncOperation("push notes to firebase", func() error {
				return a.pushNotesToFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase notes pushed")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase notes push failed: " + err.Error())
			})
		}},
		{Label: "pull settings from firebase", Apply: func() {
			a.startSyncOperation("pull settings from firebase", func() error {
				return a.pullSettingsFromFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase settings pulled")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase settings pull failed: " + err.Error())
			})
		}},
		{Label: "push settings to firebase", Apply: func() {
			a.startSyncOperation("push settings to firebase", func() error {
				return a.pushSettingsToFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase settings pushed")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase settings push failed: " + err.Error())
			})
		}},
		{Label: "pull assets from firebase", Apply: func() {
			a.startSyncOperation("pull assets from firebase", func() error {
				return a.pullAssetsFromFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase assets pulled")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase assets pull failed: " + err.Error())
			})
		}},
		{Label: "push assets to firebase", Apply: func() {
			a.startSyncOperation("push assets to firebase", func() error {
				return a.pushAssetsToFirebase(context.Background())
			}, func() {
				helpers.StatusBarInst().UpdateStatusBar("Firebase assets pushed")
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Firebase assets push failed: " + err.Error())
			})
		}},
		{Label: "legacy drive manual backup", Apply: func() {
			helpers.StatusBarInst().UpdateStatusBar("Google Drive is legacy manual snapshot backup")
		}},
		{Label: fmt.Sprintf("drive sync enabled: %t", cfg.Enabled), Apply: func() {
			cfg.Enabled = !cfg.Enabled
			a.markSettingsChanged()
		}},
		{Label: "connect google drive", Apply: func() {
			if !gdrive.HasCredentials() {
				helpers.StatusBarInst().UpdateStatusBar("Google OAuth credentials are not configured")
				return
			}
			url, session, err := gdrive.StartLocalAuthorization()
			if err != nil {
				helpers.StatusBarInst().UpdateStatusBar("Google authorization failed to start")
				return
			}
			helpers.OpenURI(url)
			helpers.StatusBarInst().UpdateStatusBar("Browser opened for Google Drive authorization")
			go func() {
				err := <-session.Wait()
				if err != nil {
					helpers.StatusBarInst().UpdateStatusBar("Google Drive authorization failed")
					return
				}
				helpers.StatusBarInst().UpdateStatusBar("Google Drive connected")
			}()
		}},
		{Label: "upload local save to drive", Apply: func() {
			if a.hasUnsavedChanges() {
				helpers.StatusBarInst().UpdateStatusBar("Save locally first before uploading to Drive")
				return
			}
			a.startSyncOperation("upload local save to drive", settings.SyncDriveData, func() {
				helpers.StatusBarInst().UpdateStatusBar("Drive snapshot uploaded at " + formatTimestampOrNever(settings.Inst().GDrive.LastDriveSaveAt))
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Drive upload failed: " + err.Error())
			})
		}},
		{Label: "refresh snapshot list from drive", Apply: func() {
			a.startSyncOperation("refresh snapshot list from drive", settings.RefreshDriveSnapshots, func() {
				helpers.StatusBarInst().UpdateStatusBar("Drive snapshot list refreshed at " + formatTimestampOrNever(settings.Inst().GDrive.LastDriveRefreshAt))
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Drive refresh failed: " + err.Error())
			})
		}},
		{Label: "restore selected drive snapshot", Apply: func() {
			if a.selectedSnapshotID() == "" {
				helpers.StatusBarInst().UpdateStatusBar("No Drive snapshot selected")
				return
			}
			selectedID := a.selectedSnapshotID()
			selectedLabel := a.selectedSnapshotLabel()
			a.startSyncOperation("restore selected drive snapshot", func() error {
				return settings.RestoreDriveSnapshot(selectedID)
			}, func() {
				if ws, err := notes.NewWorkspace(); err == nil {
					a.notes = ws
				}
				a.pages = pages.NewModel()
				a.password = password.NewModel()
				a.settingsDirty = false
				helpers.StatusBarInst().UpdateStatusBar("Drive snapshot restored from " + selectedLabel)
			}, func(err error) {
				helpers.StatusBarInst().UpdateStatusBar("Drive restore failed: " + err.Error())
			})
		}},
	}
	for _, snapshot := range cfg.Snapshots {
		snap := snapshot
		label := fmt.Sprintf("snapshot %s", formatSnapshotLabel(snap.Name, snap.CreatedAt))
		if snap.ID == cfg.SelectedSnapshotID {
			label = helpers.ANSI(helpers.ANSIReverse+helpers.ANSIBold, label+" [selected]")
		}
		items = append(items, actionItem{Label: label, Apply: func() {
			cfg.SelectedSnapshotID = snap.ID
			helpers.StatusBarInst().UpdateStatusBar("Selected snapshot: " + formatSnapshotLabel(snap.Name, snap.CreatedAt))
		}})
	}
	return items
}

func firebaseWorkspaceLabel(cfg *settings.FirebaseSettings) string {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if id := firebaseWorkspaceID(cfg, fileCfg); id != "" {
		return id
	}
	return "not configured"
}

func (a *terminalApp) settingsItems() []actionItem {
	cfg := settings.Inst()
	items := []actionItem{
		{Label: fmt.Sprintf("theme: %s", settings.CurrentTheme()), Apply: func() {
			if cfg.UI == nil {
				cfg.UI = &settings.UISettings{}
			}
			cfg.UI.Theme = settings.NextTheme(settings.CurrentTheme())
			applyGlobalBackgroundStyle()
			a.applyWidgetBackgroundStyle()
			a.markSettingsChanged()
		}},
		{Label: fmt.Sprintf("transparent background: %t", cfg.UI != nil && cfg.UI.TransparentBackground), Apply: func() {
			if cfg.UI == nil {
				cfg.UI = &settings.UISettings{}
			}
			cfg.UI.TransparentBackground = !cfg.UI.TransparentBackground
			applyGlobalBackgroundStyle()
			a.applyWidgetBackgroundStyle()
			a.markSettingsChanged()
		}},
		{Label: fmt.Sprintf("vim mode: %t", cfg.NotesApp.VimMode), Apply: func() {
			cfg.NotesApp.VimMode = !cfg.NotesApp.VimMode
			a.markSettingsChanged()
		}},
		{Label: fmt.Sprintf("tab spaces: %d", cfg.NotesApp.TabSpaces), Apply: func() {
			cfg.NotesApp.TabSpaces++
			if cfg.NotesApp.TabSpaces > 8 {
				cfg.NotesApp.TabSpaces = 2
			}
			a.markSettingsChanged()
		}},
		{Label: a.settingsUndoLevelsLabel(cfg.NotesApp.UndoLevels), Apply: func() {
			a.settingsEditMode = true
			a.settingsEditBuffer = strconv.Itoa(cfg.NotesApp.UndoLevels)
		}},
		{Label: a.settingsTabOrderLabel(), Apply: func() {
			if cfg.UI == nil {
				cfg.UI = &settings.UISettings{}
			}
			cfg.UI.TabOrder = settings.NormalizeTabOrder(cfg.UI.TabOrder)
			if a.tabOrderIndex >= len(cfg.UI.TabOrder) {
				a.tabOrderIndex = len(cfg.UI.TabOrder) - 1
			}
			if a.tabOrderIndex < 0 {
				a.tabOrderIndex = 0
			}
			a.tabOrderEditMode = true
		}},
	}
	items = append(items, a.spellSettingsItems()...)
	return items
}

func (a *terminalApp) spellSettingsItems() []actionItem {
	cfg := settings.Inst()
	items := []actionItem{
		{Label: fmt.Sprintf("spell checking: %t", cfg.NotesApp.SpellCheckEnabled), Apply: func() {
			cfg.NotesApp.SpellCheckEnabled = !cfg.NotesApp.SpellCheckEnabled
			a.markSettingsChanged()
		}},
	}
	for _, dict := range notes.SpellCatalog() {
		d := dict
		installed := notes.SpellDictionaryInstalled(d.Code)
		state := "install"
		if installed {
			state = spellDictionarySettingsState(notes.SpellDictionaryStatus(d.Code))
		}
		label := fmt.Sprintf("spell %s (%s): %s", d.Code, d.Name, state)
		items = append(items, actionItem{Label: label, Apply: func() {
			if notes.SpellDictionaryInstalled(d.Code) {
				if notes.EnableSpellDictionary(d.Code) {
					cfg.NotesApp.SpellCheckEnabled = true
					a.markSettingsChanged()
				}
				helpers.StatusBarInst().UpdateStatusBar(fmt.Sprintf("Spell dictionary already installed: %s", d.Name))
				return
			}
			helpers.StatusBarInst().UpdateStatusBar(fmt.Sprintf("Downloading spell dictionary: %s", d.Name))
			if err := notes.InstallSpellDictionary(d.Code); err != nil {
				helpers.StatusBarInst().UpdateStatusBar("Spell dictionary download failed: " + err.Error())
				return
			}
			cfg.NotesApp.SpellCheckEnabled = true
			a.markSettingsChanged()
			helpers.StatusBarInst().UpdateStatusBar(fmt.Sprintf("Installed spell dictionary: %s", d.Name))
		}})
	}
	return items
}

func spellDictionarySettingsState(status notes.SpellDictionaryLoadStatus) string {
	if !status.Installed {
		return "install"
	}
	if status.Loaded {
		if status.Backend == "nuspell" || status.Backend == "hunspell" {
			return fmt.Sprintf("installed (native: %s)", status.Backend)
		}
		return "installed"
	}
	if status.Fallback {
		if status.Action != "" {
			return fmt.Sprintf("installed (fallback; install native checker: %s)", status.Action)
		}
		return "installed (fallback)"
	}
	if status.Error != "" {
		return "cannot load (reinstall dictionary)"
	}
	return "cannot load (reinstall dictionary)"
}

func (a *terminalApp) refresh() {
	applyGlobalBackgroundStyle()
	a.applyWidgetBackgroundStyle()
	if a.header != nil {
		if a.tabSelect {
			a.header.SetBorderColor(themeColor(currentTheme().StatusAccent))
		} else {
			a.header.SetBorderColor(themeColor(currentTheme().Border))
		}
		a.header.SetText(a.renderTabBar())
	}
	status := a.currentStatusText()
	a.lastStatus = status
	if a.statusBar != nil {
		a.statusBar.SetText(status)
	}
	if a.helpOverlay != nil {
		text, regions := a.renderHelpOverlay(maxInt(20, a.width-6), maxInt(8, a.height-6))
		a.helpOverlay.SetText(text)
		if len(regions) > 0 {
			if a.helpSearchIndex >= len(regions) {
				a.helpSearchIndex = 0
			}
			a.helpOverlay.Highlight(regions[a.helpSearchIndex])
			a.helpOverlay.ScrollToHighlight()
		} else {
			a.helpSearchIndex = 0
			a.helpOverlay.Highlight()
		}
		title := "Keyboard Shortcuts"
		if a.helpSearchMode {
			title += " / " + a.helpSearchQuery
		} else if a.helpSearchQuery != "" {
			title += " / " + a.helpSearchQuery
		}
		a.helpOverlay.SetTitle(title)
	}
	if a.pagesRoot != nil {
		a.pagesRoot.ShowPage("main")
		if a.showHelp {
			a.pagesRoot.ShowPage("help")
		} else {
			a.pagesRoot.HidePage("help")
		}
		if a.shuttingDown {
			a.pagesRoot.ShowPage("quit")
			if a.quitModal != nil {
				if a.syncInProgress {
					a.quitModal.SetText(fmt.Sprintf("Saving locally and uploading to Google Drive... %s", a.syncSpinnerFrame()))
				} else {
					a.quitModal.SetText("Unsaved or unsynced changes. Save before closing?")
				}
			}
		} else {
			a.pagesRoot.HidePage("quit")
		}
	}

	if a.body == nil {
		return
	}
	a.body.Clear()
	switch a.view {
	case viewNotes:
		a.refreshNotesBody()
	case viewFiles:
		a.refreshFilesBody()
	case viewPages:
		a.refreshSingle("Pages", a.renderPages(maxInt(3, a.height-10)))
	case viewPassword:
		a.refreshSingle("Password", a.renderPassword(maxInt(3, a.height-10)))
	case viewTodo:
		a.refreshSingle("Todo", a.renderTodo(maxInt(3, a.height-10)))
	case viewSync:
		a.refreshSingle("Sync", a.renderSync(maxInt(3, a.height-10)))
	case viewSettings:
		a.refreshSingleMarkup("Settings", a.renderSettings(maxInt(3, a.height-10)))
	case viewRecorder:
		a.refreshSingleMarkup("Recorder", a.renderRecorder(maxInt(3, a.height-10)))
	}
}

func (a *terminalApp) renderTabBar() string {
	tabs := a.visibleAppTabs()
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		labelName := tab.label
		if a.viewDirty(tab.view) {
			labelName += "*"
		}
		label := fmt.Sprintf(" %s:%s ", tab.key, labelName)
		if a.tabSelect && tab.view == a.view {
			parts = append(parts, themeMarkupFGStyleBG(currentTheme().Background, currentTheme().StatusAccent, ":b")+tview.Escape(label)+"[-:-:-]")
			continue
		}
		if tab.view == a.view {
			parts = append(parts, themeMarkupFGStyleBG(currentTheme().ActiveTabFG, currentTheme().ActiveTabBG, ":b")+tview.Escape(label)+"[-:-:-]")
			continue
		}
		parts = append(parts, themeMarkupFG(currentTheme().Dim)+tview.Escape(label)+"[-:-:-]")
	}
	return strings.Join(parts, " ")
}

func (a *terminalApp) viewDirty(v view) bool {
	switch v {
	case viewNotes:
		return a.notes != nil && a.notes.HasDirty()
	case viewFiles:
		return a.notes != nil && a.notes.FilesDirty
	case viewPages:
		return a.pages != nil && a.pages.Dirty
	case viewPassword:
		return a.password != nil && a.password.Dirty
	case viewTodo:
		return a.todoDirty
	case viewSettings, viewSync:
		return a.settingsDirty
	case viewRecorder:
		return false
	default:
		return false
	}
}

func (a *terminalApp) hasUnsyncedChanges() bool {
	cfg := settings.Inst().GDrive
	if cfg == nil {
		return false
	}
	if cfg.PendingSync {
		return true
	}
	localText := strings.TrimSpace(cfg.LastLocalSaveAt)
	if localText == "" {
		return false
	}
	localTime, err := time.Parse(time.RFC3339, localText)
	if err != nil {
		return false
	}
	driveText := strings.TrimSpace(cfg.LastDriveSaveAt)
	if driveText == "" {
		return true
	}
	driveTime, err := time.Parse(time.RFC3339, driveText)
	if err != nil {
		return true
	}
	return localTime.After(driveTime)
}

func (a *terminalApp) currentStatusText() string {
	status := helpers.SanitizeSingleLine(helpers.StatusBarInst().Text())
	if status == "" {
		status = a.status
	}
	return a.oneLine(status)
}

func (a *terminalApp) oneLine(text string) string {
	text = helpers.SanitizeSingleLine(text)
	width := a.width - 4
	if width <= 0 {
		return text
	}
	if helpers.VisibleRuneCount(text) > width {
		return helpers.TruncateANSI(text, width)
	}
	return text
}

func (a *terminalApp) watchStatus(ctx context.Context) {
	if a.tui == nil {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.recorderCapturing {
				if time.Now().After(a.recorderEndsAt) {
					a.stopRecorderCapture()
				}
				a.tui.QueueUpdateDraw(func() {
					a.refresh()
				})
				continue
			}
			if settings.IsShuttingDown() {
				continue
			}
			if a.syncInProgress {
				a.syncSpinnerTick.Add(1)
				a.tui.QueueUpdateDraw(func() {
					a.refresh()
				})
				continue
			}
			if a.shuttingDown {
				continue
			}
			status := a.currentStatusText()
			if status == a.lastStatus {
				continue
			}
			a.lastStatus = status
			a.tui.QueueUpdateDraw(func() {
				a.refresh()
			})
		}
	}
}

func (a *terminalApp) refreshNotesBody() {
	_, editorWidth, previewWidth := a.notesPaneSizes()
	bodyHeight := maxInt(3, a.height-6)
	contentHeight := maxInt(1, bodyHeight-3)
	editorInnerHeight := maxInt(1, contentHeight-2)

	if a.notes.SidebarBrowsing {
		browserWidth := maxInt(10, a.width-4)
		a.editor.SetTitle("Notes Browser")
		a.editor.SetText(joinTViewLines(a.notes.BrowserRows(browserWidth, editorInnerHeight)))
		a.commandBar.SetText(joinTViewLines([]string{a.notes.CommandLineText(maxInt(10, a.width-6))}))

		content := tview.NewFlex().SetDirection(tview.FlexColumn)
		content.AddItem(a.editor, 0, 1, false)

		a.body.SetDirection(tview.FlexRow)
		a.body.AddItem(content, 0, 1, false)
		a.body.AddItem(a.commandBar, 3, 0, false)
		return
	}

	a.sidebar.SetTitle(a.notesSidebarTitle())
	a.sidebar.SetText(joinTViewLines(a.notes.SidebarRows(editorInnerHeight)))
	a.editor.SetTitle(a.notesEditorTitle(editorWidth - 2))
	editorLines := make([]string, 0, editorInnerHeight)
	a.notes.EditorHeight = maxInt(1, editorInnerHeight-1)
	a.notes.EditorRenderWidth = maxInt(1, editorWidth-2)
	editorLines = append(editorLines, a.notes.TabsText(maxInt(10, editorWidth-3)))
	editorLines = append(editorLines, a.notes.EditorRows(maxInt(1, editorWidth-2), maxInt(1, editorInnerHeight-1))...)
	a.editor.SetText(joinTViewLines(editorLines))
	a.preview.SetText(joinTViewLines(a.notes.PreviewRows(maxInt(1, previewWidth-2), editorInnerHeight)))
	a.commandBar.SetText(joinTViewLines([]string{a.notes.CommandLineText(maxInt(10, a.width-6))}))

	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	sidebarWidth, editorOuter, previewOuter := a.notesPaneSizes()
	if settings.Inst().NotesApp.SidebarVisible && sidebarWidth > 0 {
		content.AddItem(a.sidebar, sidebarWidth, 0, false)
	}
	content.AddItem(a.editor, editorOuter, 0, false)
	if previewOuter > 0 {
		content.AddItem(a.preview, 0, 1, false)
	}

	a.body.SetDirection(tview.FlexRow)
	a.body.AddItem(content, 0, 1, false)
	a.body.AddItem(a.commandBar, 3, 0, false)
}

func (a *terminalApp) refreshFilesBody() {
	sidebarOuter := a.notes.SidebarWidth + 2
	if sidebarOuter <= 2 || sidebarOuter > 122 {
		sidebarOuter = 30
	}
	if sidebarOuter > a.width/3 {
		sidebarOuter = a.width / 3
	}
	if sidebarOuter < 20 {
		sidebarOuter = 20
	}
	bodyHeight := maxInt(3, a.height-6)
	contentHeight := maxInt(1, bodyHeight-3)

	a.sidebar.SetTitle("Files")
	a.editor.SetTitle("Details")
	a.sidebar.SetText(joinTViewLines(a.notes.FileRows(maxInt(1, contentHeight-2))))
	a.editor.SetText(joinTViewLines(a.notes.FilePreviewRows(maxInt(10, a.width-sidebarOuter-4), maxInt(1, contentHeight-2))))
	a.commandBar.SetTitle("File Command")
	a.commandBar.SetText(joinTViewLines([]string{a.notes.FileCommandLineText(maxInt(10, a.width-6))}))

	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(a.sidebar, sidebarOuter, 0, false)
	content.AddItem(a.editor, 0, 1, false)

	a.body.SetDirection(tview.FlexRow)
	a.body.AddItem(content, 0, 1, false)
	a.body.AddItem(a.commandBar, 3, 0, false)
}

func (a *terminalApp) notesSidebarTitle() string {
	if a.notes.FocusSidebar {
		return "Notes [focus]"
	}
	return "Notes"
}

func (a *terminalApp) notesEditorTitle(width int) string {
	ed := a.notes.ActiveEditor()
	if ed == nil {
		return "Editor"
	}
	title := fmt.Sprintf("%s | %s", ed.Mode, ed.Title)
	if a.notes.FocusSidebar {
		title += " | Ctrl+E editor"
	} else if ed.Mode == notes.ModeNormal {
		title += " | i insert"
	} else if ed.Mode == notes.ModeCommand {
		title += " | enter run"
	} else {
		title += " | Esc normal"
	}
	if width <= 0 {
		return title
	}
	return tview.Escape(helpers.TruncateANSI(title, width))
}

func (a *terminalApp) notesPaneSizes() (int, int, int) {
	contentWidth := maxInt(40, a.width)
	sidebarInner, editorInner, previewInner := a.notes.PaneWidths(contentWidth - 2)
	sidebarOuter := 0
	if settings.Inst().NotesApp.SidebarVisible && sidebarInner > 0 {
		sidebarOuter = sidebarInner + 2
	}
	editorOuter := editorInner + 2
	previewOuter := 0
	if previewInner > 0 {
		previewOuter = previewInner + 2
	}
	return sidebarOuter, editorOuter, previewOuter
}

func (a *terminalApp) refreshSingle(title string, text string) {
	a.single.SetTitle(title)
	a.single.SetWrap(title == "Sync")
	a.single.SetWordWrap(title == "Sync")
	a.single.SetText(joinTViewLines(strings.Split(text, "\n")))
	a.body.AddItem(a.single, 0, 1, false)
}

func (a *terminalApp) refreshSingleMarkup(title string, text string) {
	a.single.SetTitle(title)
	a.single.SetWrap(title == "Sync")
	a.single.SetWordWrap(title == "Sync")
	a.single.SetText(text)
	a.body.AddItem(a.single, 0, 1, false)
}

func (a *terminalApp) renderRecorder(height int) string {
	lines := make([]string, 0, maxInt(8, height))
	state := "idle"
	if a.recorderCapturing {
		state = "recording"
	} else if len(a.recorderEvents) > 0 {
		state = "finished"
	}
	lines = append(lines, fmt.Sprintf("State: %s", state))
	if a.recorderCapturing {
		remaining := time.Until(a.recorderEndsAt)
		if remaining < 0 {
			remaining = 0
		}
		lines = append(lines, fmt.Sprintf("Recording active: %.1fs remaining", remaining.Seconds()))
		lines = append(lines, "All other key bindings are blocked while recording.")
	} else {
		lines = append(lines, "Run :recordkeys from Notes to start a 5-second key capture.")
	}
	lines = append(lines, fmt.Sprintf("Captured events: %d", len(a.recorderEvents)))
	lines = append(lines, "")
	lines = append(lines, "Latest key:")
	if len(a.recorderEvents) == 0 {
		lines = append(lines, "  none")
	} else {
		latest := a.recorderLastEvent
		lines = append(lines, "  key: "+latest.KeyName)
		lines = append(lines, "  source: "+latest.Source)
		lines = append(lines, "  raw: "+latest.TCellKey)
		lines = append(lines, "  rune: "+latest.Rune)
		lines = append(lines, "  modifiers: "+latest.Modifiers)
	}
	lines = append(lines, "")
	lines = append(lines, "Recent events:")
	if len(a.recorderEvents) == 0 {
		lines = append(lines, "  none")
	} else {
		start := maxInt(0, len(a.recorderEvents)-min(12, height/2))
		for _, item := range a.recorderEvents[start:] {
			lines = append(lines, fmt.Sprintf("  %s  %s  [%s]", item.At.Format("15:04:05.000"), item.KeyName, item.Modifiers))
		}
	}
	return strings.Join(lines, "\n")
}

func (a *terminalApp) recordRecorderKey(key notes.Key, source string, rawKey string) {
	if a == nil || !a.recorderCapturing {
		return
	}
	event := recordedKeyEvent{
		At:        time.Now(),
		Source:    source,
		KeyName:   key.Name,
		TCellKey:  rawKey,
		Rune:      recorderRuneText(key.Rune),
		Modifiers: recorderModifierText(key),
	}
	if event.KeyName == "" {
		event.KeyName = "unknown"
	}
	if event.TCellKey == "" {
		event.TCellKey = "n/a"
	}
	a.recorderEvents = append(a.recorderEvents, event)
	a.recorderLastEvent = event
}

func recorderRuneText(r rune) string {
	if r == 0 {
		return "none"
	}
	return strconv.QuoteRune(r)
}

func recorderModifierText(key notes.Key) string {
	parts := make([]string, 0, 4)
	if key.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if key.Alt {
		parts = append(parts, "Alt")
	}
	if key.Meta {
		parts = append(parts, "Meta")
	}
	if key.Shift {
		parts = append(parts, "Shift")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}

func recorderTCellKeyName(key tcell.Key) string {
	if name, ok := tcell.KeyNames[key]; ok {
		return name
	}
	return fmt.Sprintf("Key(%d)", key)
}

func (a *terminalApp) showCursor(screen tcell.Screen) {
	if a.tabSelect {
		screen.SetCursorStyle(tcell.CursorStyleDefault)
		screen.HideCursor()
		return
	}
	if a.showHelp || a.shuttingDown {
		screen.SetCursorStyle(tcell.CursorStyleDefault)
		screen.HideCursor()
		return
	}
	if a.view != viewNotes {
		if a.view == viewPages && a.pages != nil {
			if row, col, ok := a.pages.Cursor(); ok {
				x, y, width, height := a.single.GetInnerRect()
				if row >= height {
					row = height - 1
				}
				if row < 0 {
					row = 0
				}
				if col >= width {
					col = width - 1
				}
				if col < 0 {
					col = 0
				}
				screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
				screen.ShowCursor(x+col, y+row)
				return
			}
		}
		if a.view == viewFiles {
			if col, ok := a.notes.FileCommandCursor(); ok {
				x, y, width, _ := a.commandBar.GetInnerRect()
				if col >= width {
					col = width - 1
				}
				if col < 0 {
					col = 0
				}
				screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
				screen.ShowCursor(x+col, y)
				return
			}
			if row, col, ok := a.notes.FileCursor(); ok {
				x, y, _, _ := a.sidebar.GetInnerRect()
				screen.SetCursorStyle(tcell.CursorStyleSteadyBlock)
				screen.ShowCursor(x+col, y+row)
				return
			}
		}
		if a.view == viewTodo && a.todoInputMode != "" {
			x, y, width, height := a.single.GetInnerRect()
			row, col := a.todoInputCursor()
			if row >= height {
				row = height - 1
			}
			if row < 0 {
				row = 0
			}
			if col >= width {
				col = width - 1
			}
			if col < 0 {
				col = 0
			}
			screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
			screen.ShowCursor(x+col, y+row)
			return
		}
		screen.SetCursorStyle(tcell.CursorStyleDefault)
		screen.HideCursor()
		return
	}
	if ed := a.notes.ActiveEditor(); ed != nil && !a.notes.FocusSidebar {
		if ed.Mode == notes.ModeInsert {
			screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
		} else {
			screen.SetCursorStyle(tcell.CursorStyleSteadyBlock)
		}
	} else {
		screen.SetCursorStyle(tcell.CursorStyleSteadyBlock)
	}
	if col, ok := a.notes.CommandCursor(); ok {
		x, y, width, _ := a.commandBar.GetInnerRect()
		if col >= width {
			col = width - 1
		}
		if col < 0 {
			col = 0
		}
		screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
		screen.ShowCursor(x+col, y)
		return
	}
	if a.notes.SidebarBrowsing && a.notes.FocusSidebar {
		row, col, ok := a.notes.SidebarCursor()
		if !ok {
			screen.HideCursor()
			return
		}
		x, y, _, _ := a.editor.GetInnerRect()
		screen.ShowCursor(x+col, y+row)
		return
	}
	if settings.Inst().NotesApp.SidebarVisible && a.notes.FocusSidebar {
		row, col, ok := a.notes.SidebarCursor()
		if !ok {
			screen.HideCursor()
			return
		}
		x, y, _, _ := a.sidebar.GetInnerRect()
		screen.ShowCursor(x+col, y+row)
		return
	}
	row, col, ok := a.notes.EditorCursor()
	if !ok {
		screen.HideCursor()
		return
	}
	x, y, width, height := a.editor.GetInnerRect()
	row++
	if ed := a.notes.ActiveEditor(); ed != nil {
		col += notesEditorGutterWidth(ed, width)
	}
	if row >= height {
		row = height - 1
	}
	if row < 0 {
		row = 0
	}
	if col >= width {
		col = width - 1
	}
	if col < 0 {
		col = 0
	}
	screen.ShowCursor(x+col, y+row)
}

func (a *terminalApp) todoInputCursor() (int, int) {
	prefix := "new: "
	if a.todoInputMode == "edit" {
		prefix = "edit: "
	}
	runes := []rune(a.todoInputBuffer)
	a.clampTodoInputCursor(len(runes))
	return 2, len([]rune(prefix)) + a.todoInputCursorOffset
}

func joinTViewLines(lines []string) string {
	converted := make([]string, len(lines))
	for i, line := range lines {
		converted[i] = ansiToTView(line)
	}
	return strings.Join(converted, "\n")
}

func ansiToTView(s string) string {
	theme := currentTheme()
	replacer := strings.NewReplacer(
		helpers.ANSIBold, "[::b]",
		helpers.ANSIItalic, "[::i]",
		helpers.ANSIDim, themeMarkupFG(theme.Dim),
		helpers.ANSIReverse, themeMarkupPair(theme.SelectionFG, theme.SelectionBG),
		helpers.ANSIFgBlue, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading1]),
		helpers.ANSIFgGreen, themeMarkupFG(theme.Syntax[helpers.ANSIRoleString]),
		helpers.ANSIFgYellow, themeMarkupFG(theme.Syntax[helpers.ANSIRoleProperty]),
		helpers.ANSIFgPurple, themeMarkupFG(theme.Syntax[helpers.ANSIRoleKeyword]),
		helpers.ANSIFgCyan, themeMarkupFG(theme.Syntax[helpers.ANSIRoleType]),
		helpers.ANSIFgGray, themeMarkupFG(theme.Dim),
		helpers.ANSIFgOrange, themeMarkupFG(theme.Syntax[helpers.ANSIRoleNumber]),
		helpers.ANSIRoleHeading1, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading1]),
		helpers.ANSIRoleHeading2, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading2]),
		helpers.ANSIRoleHeading3, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading3]),
		helpers.ANSIRoleHeading4, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading4]),
		helpers.ANSIRoleHeading5, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading5]),
		helpers.ANSIRoleHeading6, themeMarkupFG(theme.Syntax[helpers.ANSIRoleHeading6]),
		helpers.ANSIRoleListMarker, themeMarkupFGStyle(theme.Syntax[helpers.ANSIRoleListMarker], "b"),
		helpers.ANSIRoleLink, themeMarkupFG(theme.Syntax[helpers.ANSIRoleLink]),
		helpers.ANSIRoleCode, themeMarkupFG(theme.Syntax[helpers.ANSIRoleCode]),
		helpers.ANSIRoleString, themeMarkupFG(theme.Syntax[helpers.ANSIRoleString]),
		helpers.ANSIRoleKeyword, themeMarkupFG(theme.Syntax[helpers.ANSIRoleKeyword]),
		helpers.ANSIRoleNumber, themeMarkupFG(theme.Syntax[helpers.ANSIRoleNumber]),
		helpers.ANSIRoleComment, themeMarkupFG(theme.Syntax[helpers.ANSIRoleComment]),
		helpers.ANSIRoleType, themeMarkupFG(theme.Syntax[helpers.ANSIRoleType]),
		helpers.ANSIRoleFunction, themeMarkupFG(theme.Syntax[helpers.ANSIRoleFunction]),
		helpers.ANSIRoleProperty, themeMarkupFG(theme.Syntax[helpers.ANSIRoleProperty]),
		helpers.ANSIRoleConstant, themeMarkupFGStyle(theme.Syntax[helpers.ANSIRoleConstant], "b"),
		helpers.ANSIRoleSearch, themeMarkupPair(theme.Syntax[helpers.ANSIRoleSearch], theme.SelectionBG),
		helpers.ANSIRoleVisualSelection, themeMarkupPair(theme.Syntax[helpers.ANSIRoleVisualSelection], theme.SelectionBG),
		helpers.ANSIRoleActiveTab, themeMarkupPair(theme.ActiveTabFG, theme.ActiveTabBG),
		helpers.ANSIRoleActiveTabClose, themeMarkupPair(theme.ErrorAccent, theme.ActiveTabBG),
		helpers.ANSIRoleSelection, themeMarkupPair(theme.SelectionFG, theme.SelectionBG),
		helpers.ANSIRoleSpellError, themeMarkupFG(theme.ErrorAccent),
		"\x1b[0m", "[-:-:-]",
	)
	return replaceANSIWithTViewMarkup(s, replacer)
}

func replaceANSIWithTViewMarkup(s string, replacer *strings.Replacer) string {
	var out strings.Builder
	for len(s) > 0 {
		idx := strings.IndexByte(s, '\x1b')
		if idx < 0 {
			out.WriteString(tview.Escape(s))
			break
		}
		if idx > 0 {
			out.WriteString(tview.Escape(s[:idx]))
			s = s[idx:]
		}
		end := strings.IndexByte(s, 'm')
		if end < 0 {
			out.WriteString(tview.Escape(s))
			break
		}
		out.WriteString(replacer.Replace(s[:end+1]))
		s = s[end+1:]
	}
	return out.String()
}

func (a *terminalApp) renderPages(height int) string {
	focusPrefix := func(idx int) string {
		if a.pages.Focus == idx {
			if a.pages.Editing {
				return "> "
			}
			return "* "
		}
		return "  "
	}
	fieldValue := func(idx int, value string) string {
		if a.pages.Editing && a.pages.Focus == idx && a.pages.SelectionActive {
			return helpers.ANSI(helpers.ANSIRoleSelection, value)
		}
		return value
	}
	lines := []string{
		"Pages calculator",
		"j/k move | e/enter edit | tab fields | enter apply | r recalc",
		fmt.Sprintf("%sfirst book:  %s", focusPrefix(0), fieldValue(0, a.pages.FirstBookInput)),
		fmt.Sprintf("%sread pages:  %s", focusPrefix(1), fieldValue(1, a.pages.ReadInput)),
		fmt.Sprintf("%sother book:  %s", focusPrefix(2), fieldValue(2, a.pages.SecondBookInput)),
		fmt.Sprintf("result: %s", a.pages.Result),
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

func (a *terminalApp) renderPassword(height int) string {
	lines := []string{
		"Password generator",
		"g generate | l letters | n numbers | s symbols | +/- length",
		fmt.Sprintf("letters: %t", a.password.Letters),
		fmt.Sprintf("numbers: %t", a.password.Numbers),
		fmt.Sprintf("symbols: %t", a.password.SpecialSymbols),
		fmt.Sprintf("length: %d", a.password.SymbolCount),
		fmt.Sprintf("password: %s", a.password.Password),
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

func (a *terminalApp) renderTodo(height int) string {
	a.reloadTodosForRender()
	lines := []string{"Todo", "j/k move | n new | enter/space check | e edit | J/K reorder active"}
	if a.todoInputMode != "" {
		label := "new"
		if a.todoInputMode == "edit" {
			label = "edit"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", label, a.todoInputBuffer))
	} else {
		lines = append(lines, "")
	}
	selectedID := ""
	if item, ok := a.selectedTodoItem(); ok {
		selectedID = item.ID
	}
	addSection := func(title string, items []todo.Item, archived bool) {
		lines = append(lines, title)
		if len(items) == 0 {
			lines = append(lines, "  none")
			return
		}
		for _, item := range items {
			prefix := "  "
			if item.ID == selectedID {
				prefix = helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgGreen, "> ")
			}
			box := "[ ]"
			text := item.Text
			if item.CheckedAt != nil || item.Status == todo.StatusDone || item.Status == todo.StatusArchived {
				box = "[x]"
				text = "~" + text + "~"
			}
			if archived {
				box = "[-]"
			}
			lines = append(lines, prefix+box+" "+text)
		}
	}
	addSection("Todo", todo.ActiveItems(a.todoStore), false)
	lines = append(lines, "")
	addSection("Done", todo.DoneItems(a.todoStore), false)
	lines = append(lines, "")
	lines = append(lines, "Archive")
	months := todo.ArchiveMonths(a.todoStore)
	if len(months) == 0 {
		lines = append(lines, "  none")
	} else {
		groups := todo.ArchiveGroups(a.todoStore)
		for _, month := range months {
			lines = append(lines, "  "+month)
			for _, item := range groups[month] {
				prefix := "    "
				if item.ID == selectedID {
					prefix = helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgGreen, ">   ")
				}
				lines = append(lines, prefix+"[-] ~"+item.Text+"~")
			}
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

func (a *terminalApp) renderSync(height int) string {
	cfg := settings.Inst().GDrive
	firebase := settings.Inst().Firebase
	if firebase == nil {
		firebase = &settings.FirebaseSettings{Realtime: true}
	}
	folder := cfg.FolderName
	if strings.TrimSpace(folder) == "" {
		folder = cfg.FolderID
	}
	if strings.TrimSpace(folder) == "" {
		folder = "not selected"
	}
	lines := []string{
		"Firebase realtime sync",
		fmt.Sprintf("enabled: %t", firebase.Enabled),
		fmt.Sprintf("configured: %t", firebaseConfigured(firebase)),
		fmt.Sprintf("workspace: %s", firebaseWorkspaceLabel(firebase)),
		fmt.Sprintf("last firebase action: %s", strings.TrimSpace(firebase.LastSyncStatus+" "+firebase.LastSyncMessage)),
		"",
		"Google Drive legacy manual backup",
		fmt.Sprintf("enabled: %t", cfg.Enabled),
		fmt.Sprintf("credentials: %t", gdrive.HasCredentials()),
		fmt.Sprintf("token: %t", gdrive.HasToken()),
		fmt.Sprintf("folder: %s", folder),
		fmt.Sprintf("last local save: %s", formatTimestampOrNever(cfg.LastLocalSaveAt)),
		fmt.Sprintf("last drive save: %s", formatTimestampOrNever(cfg.LastDriveSaveAt)),
		fmt.Sprintf("last drive refresh: %s", formatTimestampOrNever(cfg.LastDriveRefreshAt)),
		fmt.Sprintf("last drive action: %s", strings.TrimSpace(cfg.LastSyncStatus+" "+cfg.LastSyncMessage)),
	}
	if a.syncInProgress {
		lines = append(lines, fmt.Sprintf("progress: %s running %s", a.syncSpinnerFrame(), a.syncProgressLabel))
	} else {
		lines = append(lines, "progress: idle")
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("snapshots: %d", len(cfg.Snapshots)))
	if cfg.SelectedSnapshotID != "" {
		lines = append(lines, helpers.ANSI(helpers.ANSIReverse+helpers.ANSIBold, fmt.Sprintf("selected snapshot: %s [selected]", a.selectedSnapshotLabel())))
	}
	lines = append(lines, "Actions")
	items := a.syncItems()
	for i, item := range items {
		prefix := "  "
		if i == a.syncIndex {
			prefix = helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgGreen, "> ")
		}
		lines = append(lines, prefix+item.Label)
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("selected snapshot: %s", a.selectedSnapshotLabel()))
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

func firebaseConfigured(cfg *settings.FirebaseSettings) bool {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	return firebaseAPIKey(cfg, fileCfg) != "" && firebaseDatabaseURL(cfg, fileCfg) != "" && firebaseWorkspaceID(cfg, fileCfg) != ""
}

func (a *terminalApp) renderSettings(height int) string {
	items := a.settingsItems()
	header := "Settings"
	if a.settingsEditMode {
		header = "Settings (editing undo levels)"
	} else if a.tabOrderEditMode {
		header = "Settings (editing tab order)"
	}
	lines := []string{header, a.settingsHelpLine()}
	for i, item := range items {
		prefix := "  "
		if i == a.settingIndex {
			prefix = "> "
		}
		lines = append(lines, tview.Escape(prefix)+a.renderSettingsItemLabel(i, item.Label))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

func (a *terminalApp) renderSettingsItemLabel(index int, label string) string {
	if strings.HasPrefix(label, "tab order:") {
		return a.renderSettingsTabOrderLabel()
	}
	escaped := tview.Escape(label)
	if index != 0 {
		return escaped
	}
	return escaped + "  " + renderThemePreview(themeByName(settings.CurrentTheme()))
}

func (a *terminalApp) renderSettingsTabOrderLabel() string {
	order := settings.UITabOrder()
	parts := make([]string, 0, len(order))
	for i, id := range order {
		label := tview.Escape(tabLabelByID(id))
		if a.tabOrderEditMode && i == a.tabOrderIndex {
			label = themeMarkupPair(currentTheme().SelectionFG, currentTheme().SelectionBG) + " " + label + " " + "[-:-:-]"
		}
		parts = append(parts, label)
	}
	return "tab order: " + strings.Join(parts, ", ")
}

func renderThemePreview(theme appTheme) string {
	parts := []string{
		themePreviewSwatch("txt", theme.Primary, theme.Panel),
		themePreviewSwatch("sub", theme.Secondary, theme.Panel),
		themePreviewSwatch("dim", theme.Dim, theme.Panel),
		themePreviewSwatch("ttl", theme.Title, theme.Panel),
		themePreviewSwatch("bg", theme.Primary, theme.Background),
		themePreviewSwatch("tab", theme.ActiveTabFG, theme.ActiveTabBG),
		themePreviewSwatch("sel", theme.SelectionFG, theme.SelectionBG),
		themePreviewSwatch("cmd", theme.Background, theme.CommandAccent),
		themePreviewSwatch("err", theme.Background, theme.ErrorAccent),
	}
	return strings.Join(parts, " ")
}

func themePreviewSwatch(label string, fg string, bg string) string {
	return themeMarkupPair(fg, bg) + tview.Escape(" "+label+" ") + "[-:-:-]"
}

func (a *terminalApp) settingsUndoLevelsLabel(value int) string {
	if a.settingsEditMode {
		return fmt.Sprintf("undo levels: %s", a.settingsEditBuffer)
	}
	return fmt.Sprintf("undo levels: %d", value)
}

func (a *terminalApp) settingsTabOrderLabel() string {
	order := settings.UITabOrder()
	labels := make([]string, 0, len(order))
	for _, id := range order {
		labels = append(labels, tabLabelByID(id))
	}
	return "tab order: " + strings.Join(labels, ", ")
}

func tabLabelByID(id string) string {
	for _, tab := range baseAppTabs {
		if tab.id == id {
			return tab.label
		}
	}
	return id
}

func (a *terminalApp) settingsHelpLine() string {
	if a.tabOrderEditMode {
		return "tab order: j/k select tab | J/K move selected | enter/esc done"
	}
	if a.settingsEditMode {
		return "digits edit value | backspace delete | enter apply | esc cancel"
	}
	return "enter changes selected option"
}

func valueOrUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "never"
	}
	return value
}

func formatTimestampOrNever(value string) string {
	if strings.TrimSpace(value) == "" {
		return "never"
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return value
}

func (a *terminalApp) selectedSnapshotID() string {
	cfg := settings.Inst().GDrive
	if cfg == nil {
		return ""
	}
	return cfg.SelectedSnapshotID
}

func (a *terminalApp) selectedSnapshotLabel() string {
	cfg := settings.Inst().GDrive
	if cfg == nil || cfg.SelectedSnapshotID == "" {
		return "none"
	}
	for _, snapshot := range cfg.Snapshots {
		if snapshot.ID == cfg.SelectedSnapshotID {
			return formatSnapshotLabel(snapshot.Name, snapshot.CreatedAt)
		}
	}
	return "none"
}

func (a *terminalApp) hasUnsavedChanges() bool {
	if a == nil {
		return false
	}
	if a.notes != nil && a.notes.HasDirty() {
		return true
	}
	if a.pages != nil && a.pages.Dirty {
		return true
	}
	if a.password != nil && a.password.Dirty {
		return true
	}
	if a.todoDirty {
		return true
	}
	return a.settingsDirty
}

func (a *terminalApp) saveLocalState() error {
	if a.notes != nil {
		if _, err := a.notes.SaveAllDirtyLocal(); err != nil {
			return err
		}
		if _, err := a.notes.SavePendingFiles(); err != nil {
			return err
		}
	}
	if a.pages != nil && a.pages.Dirty {
		a.pages.Save()
	}
	if a.password != nil && a.password.Dirty {
		a.password.Save()
	}
	if a.todos != nil {
		store, err := a.todos.Load()
		if err != nil {
			return err
		}
		a.todoStore = store
	}
	if a.settingsDirty || (a.notes == nil || !a.notes.HasDirty()) && (a.pages == nil || !a.pages.Dirty) && (a.password == nil || !a.password.Dirty) && !a.todoDirty {
		settings.SaveSettingsLocal()
	}
	a.settingsDirty = false
	a.todoDirty = false
	a.pushTodosToFirebaseSoon()
	a.pushNotesToFirebaseSoon()
	a.pushSettingsToFirebaseSoon()
	a.pushAssetsToFirebaseSoon()
	return nil
}

func (a *terminalApp) startFirebaseTodoPolling(ctx context.Context) {
	if a == nil {
		return
	}
	go func() {
		// Run initial sync on startup if enabled
		fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
		if firebaseEnabled(settings.Inst().Firebase, fileCfg) {
			a.runFirebaseSyncTick(ctx)
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fileCfg, _ = kokosync.LoadConfig(kokosync.ConfigPath())
				if firebaseEnabled(settings.Inst().Firebase, fileCfg) {
					a.runFirebaseSyncTick(ctx)
				}
			}
		}
	}()
}

func (a *terminalApp) runFirebaseSyncTick(ctx context.Context) {
	var syncErr error
	if err := a.pullTodosFromFirebase(ctx); err != nil {
		syncErr = err
	} else if err := a.pullNotesFromFirebase(ctx); err != nil {
		syncErr = err
	} else if err := a.pullSettingsFromFirebase(ctx); err != nil {
		syncErr = err
	} else if err := a.pullAssetsFromFirebase(ctx); err != nil {
		syncErr = err
	}

	if syncErr != nil {
		cfg := settings.Inst().Firebase
		if cfg != nil {
			if cfg.LastSyncStatus != "error" || cfg.LastSyncMessage != syncErr.Error() {
				cfg.LastSyncStatus = "error"
				cfg.LastSyncMessage = syncErr.Error()
				settings.SaveSettingsLocal()
				a.queueUIDraw(func() {
					a.refresh()
				})
			}
		}
	}
}

func (a *terminalApp) ensureFirebaseSyncer(ctx context.Context) error {
	if a == nil {
		return fmt.Errorf("app is nil")
	}
	if a.firebaseTodoSyncer == nil ||
		a.firebaseNoteSyncer == nil ||
		a.firebaseSettingsSyncer == nil ||
		a.firebaseAssetSyncer == nil ||
		a.firebaseTodoSyncer.Session.IDToken == "" ||
		a.firebaseTodoSyncer.Session.ExpiresAt.Before(time.Now().Add(5*time.Minute)) {
		return a.configureFirebaseTodoSyncer(ctx)
	}
	return nil
}

func (a *terminalApp) configureFirebaseTodoSyncer(ctx context.Context) error {
	cfg := settings.Inst().Firebase
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if cfg == nil {
		cfg = &settings.FirebaseSettings{Realtime: true}
		settings.Inst().Firebase = cfg
	}
	apiKey := firebaseAPIKey(cfg, fileCfg)
	databaseURL := firebaseDatabaseURL(cfg, fileCfg)
	if apiKey == "" || databaseURL == "" {
		return fmt.Errorf("api_key and database_url are required")
	}
	provider := kokosync.NewFirebaseRESTProvider(apiKey, databaseURL)
	session, err := firebaseSession(ctx, provider)
	if err != nil {
		return err
	}
	workspaceID := firebaseWorkspaceID(cfg, fileCfg)
	if workspaceID == "" {
		meta, err := provider.EnsurePersonalWorkspace(ctx, session, "Personal workspace")
		if err != nil {
			return err
		}
		workspaceID = meta.ID
	}
	a.firebaseTodoSyncer = &kokosync.TodoSyncer{
		Provider:    provider,
		WorkspaceID: workspaceID,
		StatePath:   kokosync.StatePath(),
		TokenPath:   kokosync.TokenPath(),
		Session:     session,
		DeviceID:    "",
	}
	a.firebaseNoteSyncer = &kokosync.NoteSyncer{
		Provider:    provider,
		WorkspaceID: workspaceID,
		StatePath:   kokosync.StatePath(),
		Session:     session,
		DeviceID:    "",
	}
	a.firebaseSettingsSyncer = &kokosync.SettingsSyncer{
		Provider:    provider,
		WorkspaceID: workspaceID,
		StatePath:   kokosync.StatePath(),
		Session:     session,
		DeviceID:    "",
	}
	a.firebaseAssetSyncer = &kokosync.AssetSyncer{
		Provider:    provider,
		WorkspaceID: workspaceID,
		StatePath:   kokosync.StatePath(),
		Session:     session,
		DeviceID:    "",
	}
	cfg.WorkspaceID = workspaceID
	cfg.LastSyncStatus = "ok"
	cfg.LastSyncMessage = "Firebase sync configured"
	settings.SaveSettingsLocal()
	return nil
}

func firebaseSession(ctx context.Context, provider *kokosync.FirebaseRESTProvider) (kokosync.Session, error) {
	var refreshErr error
	if token, err := kokosync.LoadToken(kokosync.TokenPath()); err == nil && token.RefreshToken != "" {
		session, err := provider.Refresh(ctx, token.RefreshToken)
		if err == nil {
			session.Email = token.Email
			return session, nil
		}
		refreshErr = err
	}
	email := strings.TrimSpace(os.Getenv("KOKO_FIREBASE_EMAIL"))
	password := os.Getenv("KOKO_FIREBASE_PASSWORD")
	if email == "" || password == "" {
		if refreshErr != nil {
			return kokosync.Session{}, fmt.Errorf("refresh saved Firebase token failed: %w; set KOKO_FIREBASE_EMAIL and KOKO_FIREBASE_PASSWORD once", refreshErr)
		}
		return kokosync.Session{}, fmt.Errorf("login requires KOKO_FIREBASE_EMAIL and KOKO_FIREBASE_PASSWORD once")
	}
	session, err := provider.Login(ctx, email, password)
	if err != nil {
		return kokosync.Session{}, err
	}
	if err := kokosync.SaveToken(kokosync.TokenPath(), kokosync.TokenFile{UID: session.UID, Email: session.Email, RefreshToken: session.RefreshToken}); err != nil {
		return kokosync.Session{}, err
	}
	return session, nil
}

func (a *terminalApp) loginFirebaseWithGoogle() {
	cfg := settings.Inst().Firebase
	if cfg == nil {
		cfg = &settings.FirebaseSettings{Realtime: true}
		settings.Inst().Firebase = cfg
	}
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	apiKey := firebaseAPIKey(cfg, fileCfg)
	databaseURL := firebaseDatabaseURL(cfg, fileCfg)
	if apiKey == "" || databaseURL == "" {
		helpers.StatusBarInst().UpdateStatusBar("Firebase config unavailable")
		return
	}
	if !gdrive.HasCredentials() {
		helpers.StatusBarInst().UpdateStatusBar("Google OAuth credentials are not configured")
		return
	}
	url, googleSession, err := gdrive.StartLocalIDTokenAuthorization()
	if err != nil {
		helpers.StatusBarInst().UpdateStatusBar("Google sign-in failed to start")
		return
	}
	helpers.OpenURI(url)
	helpers.StatusBarInst().UpdateStatusBar("Browser opened for Firebase Google sign-in")
	go func() {
		result := <-googleSession.Wait()
		if result.Err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Firebase Google sign-in failed: " + result.Err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		provider := kokosync.NewFirebaseRESTProvider(apiKey, databaseURL)
		session, err := provider.LoginWithGoogleIDToken(ctx, result.IDToken)
		if err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Firebase Google login failed: " + err.Error())
			return
		}
		if err := kokosync.SaveToken(kokosync.TokenPath(), kokosync.TokenFile{UID: session.UID, Email: session.Email, RefreshToken: session.RefreshToken}); err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Save Firebase token failed: " + err.Error())
			return
		}
		meta, err := provider.EnsurePersonalWorkspace(ctx, session, "Personal workspace")
		if err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Firebase workspace setup failed: " + err.Error())
			return
		}
		cfg.Enabled = true
		cfg.Realtime = true
		cfg.UserEmail = session.Email
		cfg.WorkspaceID = meta.ID
		cfg.WorkspaceName = meta.Name
		cfg.LastSyncStatus = "ok"
		cfg.LastSyncMessage = "Firebase Google login configured"
		settings.SaveSettingsLocal()
		if err := a.configureFirebaseTodoSyncer(context.Background()); err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Firebase sync not ready: " + err.Error())
			return
		}
		helpers.StatusBarInst().UpdateStatusBar("Firebase Google login ready: " + meta.Name)
	}()
}

func (a *terminalApp) pullTodosFromFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.todos == nil || a.firebaseTodoSyncer == nil || !a.firebaseTodoSyncer.Ready() {
		return nil
	}
	local, err := a.todos.Load()
	if err != nil {
		return err
	}
	merged, changed, err := a.firebaseTodoSyncer.PullStore(ctx, local)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := a.todos.Save(merged); err != nil {
		return err
	}
	a.todoStore = merged
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase todos pulled"
		settings.SaveSettingsLocal()
	}
	a.queueUIDraw(func() {
		a.reloadTodosForRender()
		a.refresh()
	})
	return nil
}

func (a *terminalApp) pushTodosToFirebaseSoon() {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if a == nil || !firebaseEnabled(settings.Inst().Firebase, fileCfg) {
		return
	}
	go func() {
		_ = a.pushTodosToFirebase(context.Background())
	}()
}

func (a *terminalApp) pushTodosToFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.todos == nil || a.firebaseTodoSyncer == nil || !a.firebaseTodoSyncer.Ready() {
		return nil
	}
	store, err := a.todos.Load()
	if err != nil {
		return err
	}
	if err := a.firebaseTodoSyncer.PushStore(ctx, store); err != nil {
		return err
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase todos pushed"
		settings.SaveSettingsLocal()
	}
	return nil
}

func (a *terminalApp) pullNotesFromFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.firebaseNoteSyncer == nil || !a.firebaseNoteSyncer.Ready() {
		return nil
	}
	local, err := a.localNoteMap()
	if err != nil {
		return err
	}
	result, err := a.firebaseNoteSyncer.PullNotes(ctx, local)
	if err != nil {
		return err
	}
	if !result.Changed {
		return nil
	}
	for _, rel := range result.Deletes {
		if err := os.Remove(a.noteAbsPath(rel)); err != nil && !os.IsNotExist(err) {
			return err
		}
		a.removeOpenNoteTab(rel)
	}
	for _, note := range result.Upserts {
		if strings.ToLower(filepath.Ext(note.Path)) != ".md" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(a.noteAbsPath(note.Path)), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(a.noteAbsPath(note.Path), []byte(note.Text), 0o644); err != nil {
			return err
		}
		a.updateOpenCleanNote(note.Path, note.Text)
	}
	for _, conflict := range result.ConflictCopy {
		path := a.noteAbsPath(conflict.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(conflict.Text), 0o644); err != nil {
			return err
		}
	}
	if err := a.firebaseNoteSyncer.SaveState(result.State); err != nil {
		return err
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase notes pulled"
		settings.SaveSettingsLocal()
	}
	a.queueUIDraw(func() {
		if a.notes != nil {
			a.notes.Refresh()
		}
		a.refresh()
	})
	return nil
}

func (a *terminalApp) pushNotesToFirebaseSoon() {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if a == nil || !firebaseEnabled(settings.Inst().Firebase, fileCfg) {
		return
	}
	go func() {
		_ = a.pushNotesToFirebase(context.Background())
	}()
}

func (a *terminalApp) pushNotesToFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.firebaseNoteSyncer == nil || !a.firebaseNoteSyncer.Ready() {
		return nil
	}
	files, err := a.localNoteFiles()
	if err != nil {
		return err
	}
	result, err := a.firebaseNoteSyncer.PushNotes(ctx, files)
	if err != nil {
		return err
	}
	if result.Pushed == 0 {
		return nil
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase notes pushed"
		settings.SaveSettingsLocal()
	}
	return nil
}

func (a *terminalApp) pushNoteDeleteToFirebaseSoon(absPath string) {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if a == nil || !firebaseEnabled(settings.Inst().Firebase, fileCfg) {
		return
	}
	rel := a.noteRelPath(absPath)
	go func() {
		if err := a.ensureFirebaseSyncer(context.Background()); err != nil {
			return
		}
		if a.firebaseNoteSyncer != nil {
			_ = a.firebaseNoteSyncer.PushDelete(context.Background(), rel)
		}
	}()
}

func (a *terminalApp) pushSettingsToFirebaseSoon() {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if a == nil || !firebaseEnabled(settings.Inst().Firebase, fileCfg) {
		return
	}
	go func() {
		_ = a.pushSettingsToFirebase(context.Background())
	}()
}

func (a *terminalApp) pushSettingsToFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.firebaseSettingsSyncer == nil || !a.firebaseSettingsSyncer.Ready() {
		return nil
	}
	values, err := currentSettingsMap()
	if err != nil {
		return err
	}
	result, err := a.firebaseSettingsSyncer.PushSettings(ctx, values)
	if err != nil {
		return err
	}
	if !result.Pushed {
		return nil
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase settings pushed"
		settings.SaveSettingsLocal()
	}
	return nil
}

func (a *terminalApp) pullSettingsFromFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.firebaseSettingsSyncer == nil || !a.firebaseSettingsSyncer.Ready() {
		return nil
	}
	if a.deferRemoteSettingsApply() {
		if settings.Inst().Firebase != nil {
			settings.Inst().Firebase.LastSyncStatus = "deferred"
			settings.Inst().Firebase.LastSyncMessage = "Firebase settings pull deferred while local edits are active"
			settings.SaveSettingsLocal()
		}
		return nil
	}
	result, err := a.firebaseSettingsSyncer.PullSettings(ctx)
	if err != nil || !result.Changed {
		return err
	}
	local, err := currentSettingsMap()
	if err != nil {
		return err
	}
	if err := applySettingsMap(kokosync.ApplySharedWorkspaceSettings(local, result.Values)); err != nil {
		return err
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase settings pulled"
		settings.SaveSettingsLocal()
	}
	a.queueUIDraw(func() {
		if a.pages != nil && !a.viewDirty(viewPages) {
			a.pages = pages.NewModel()
		}
		if a.password != nil && !a.viewDirty(viewPassword) {
			a.password = password.NewModel()
		}
		if a.notes != nil && !a.viewDirty(viewNotes) && !a.viewDirty(viewFiles) {
			a.notes.Refresh()
		}
		a.refresh()
	})
	return nil
}

func (a *terminalApp) deferRemoteSettingsApply() bool {
	if a == nil {
		return false
	}
	return a.hasUnsavedChanges()
}

func (a *terminalApp) pushAssetsToFirebaseSoon() {
	fileCfg, _ := kokosync.LoadConfig(kokosync.ConfigPath())
	if a == nil || !firebaseEnabled(settings.Inst().Firebase, fileCfg) {
		return
	}
	go func() {
		_ = a.pushAssetsToFirebase(context.Background())
	}()
}

func (a *terminalApp) pushAssetsToFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.firebaseAssetSyncer == nil || !a.firebaseAssetSyncer.Ready() {
		return nil
	}
	assets, err := a.localAssetFiles()
	if err != nil {
		return err
	}
	result, err := a.firebaseAssetSyncer.PushAssets(ctx, assets)
	if err != nil {
		return err
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = fmt.Sprintf("Firebase assets pushed: %d", result.Pushed)
		if len(result.Skipped) > 0 {
			settings.Inst().Firebase.LastSyncMessage += fmt.Sprintf(", skipped %d oversized", len(result.Skipped))
		}
		settings.SaveSettingsLocal()
	}
	return nil
}

func (a *terminalApp) pullAssetsFromFirebase(ctx context.Context) error {
	if err := a.ensureFirebaseSyncer(ctx); err != nil {
		return err
	}
	if a.firebaseAssetSyncer == nil || !a.firebaseAssetSyncer.Ready() {
		return nil
	}
	if a.notes != nil && a.notes.FilesDirty {
		if settings.Inst().Firebase != nil {
			settings.Inst().Firebase.LastSyncStatus = "deferred"
			settings.Inst().Firebase.LastSyncMessage = "Firebase assets pull deferred while local file edits are active"
			settings.SaveSettingsLocal()
		}
		return nil
	}
	result, err := a.firebaseAssetSyncer.PullAssets(ctx)
	if err != nil || !result.Changed {
		return err
	}
	for _, rel := range result.Deletes {
		if err := os.Remove(a.assetAbsPath(rel)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, asset := range result.Upserts {
		path := a.assetAbsPath(asset.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, asset.Bytes, 0o644); err != nil {
			return err
		}
	}
	if err := a.firebaseAssetSyncer.SaveState(result.State); err != nil {
		return err
	}
	if settings.Inst().Firebase != nil {
		settings.Inst().Firebase.LastSyncAt = time.Now().Format(time.RFC3339)
		settings.Inst().Firebase.LastSyncStatus = "ok"
		settings.Inst().Firebase.LastSyncMessage = "Firebase assets pulled"
		settings.SaveSettingsLocal()
	}
	a.queueUIDraw(func() {
		if a.notes != nil {
			a.notes.Refresh()
		}
		a.refresh()
	})
	return nil
}

func (a *terminalApp) localNoteFiles() ([]kokosync.NoteFile, error) {
	root := a.notesRootPath()
	files := make([]kokosync.NoteFile, 0)
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		rel := a.noteRelPath(path)
		if rel == "" || settings.IsTrashRelativePath(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, kokosync.NoteFile{ID: kokosync.NoteID(rel), Path: rel, Text: string(data)})
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return files, nil
		}
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func (a *terminalApp) localNoteMap() (map[string]kokosync.LocalNote, error) {
	files, err := a.localNoteFiles()
	if err != nil {
		return nil, err
	}
	out := make(map[string]kokosync.LocalNote, len(files))
	for _, file := range files {
		out[file.Path] = kokosync.LocalNote{ID: file.ID, Path: file.Path, Text: file.Text}
	}
	if a.notes != nil {
		for _, tab := range a.notes.Tabs {
			if tab == nil {
				continue
			}
			rel := a.noteRelPath(tab.Path)
			if rel == "" || strings.ToLower(filepath.Ext(rel)) != ".md" {
				continue
			}
			out[rel] = kokosync.LocalNote{ID: kokosync.NoteID(rel), Path: rel, Text: tab.Text, Dirty: tab.Dirty}
		}
	}
	return out, nil
}

func (a *terminalApp) localAssetFiles() ([]kokosync.LocalAsset, error) {
	root := a.notesRootPath()
	assets := make([]kokosync.LocalAsset, 0)
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := kokosync.NormalizeAssetPath(a.noteRelPath(path))
		if rel == "" || settings.IsTrashRelativePath(rel) || !isManagedAssetRel(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assets = append(assets, kokosync.LocalAsset{ID: kokosync.AssetID(rel), Path: rel, Bytes: data, ModTime: info.ModTime()})
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return assets, nil
		}
		return nil, err
	}
	sort.SliceStable(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, nil
}

func (a *terminalApp) assetAbsPath(rel string) string {
	return filepath.Join(a.notesRootPath(), filepath.FromSlash(kokosync.NormalizeAssetPath(rel)))
}

func isManagedAssetRel(rel string) bool {
	for _, part := range strings.Split(kokosync.NormalizeAssetPath(rel), "/") {
		if part == "assets" || strings.HasSuffix(part, ".assets") {
			return true
		}
	}
	return false
}

func (a *terminalApp) updateOpenCleanNote(rel string, text string) {
	if a.notes == nil {
		return
	}
	abs := a.noteAbsPath(rel)
	for _, tab := range a.notes.Tabs {
		if tab == nil || tab.Path != abs || tab.Dirty {
			continue
		}
		tab.Text = text
		tab.Dirty = false
	}
}

func (a *terminalApp) removeOpenNoteTab(rel string) {
	if a.notes == nil {
		return
	}
	abs := a.noteAbsPath(rel)
	next := a.notes.Tabs[:0]
	removedBeforeCurrent := 0
	for i, tab := range a.notes.Tabs {
		if tab != nil && tab.Path == abs && !tab.Dirty {
			if i < a.notes.CurrentTab {
				removedBeforeCurrent++
			}
			continue
		}
		next = append(next, tab)
	}
	a.notes.Tabs = next
	a.notes.CurrentTab -= removedBeforeCurrent
	if a.notes.CurrentTab >= len(a.notes.Tabs) {
		a.notes.CurrentTab = len(a.notes.Tabs) - 1
	}
	if a.notes.CurrentTab < 0 {
		a.notes.CurrentTab = 0
	}
}

func (a *terminalApp) notesRootPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "notes"
	}
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes")
}

func (a *terminalApp) noteRelPath(path string) string {
	rel, err := filepath.Rel(a.notesRootPath(), path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return kokosync.NormalizeNotePath(filepath.ToSlash(rel))
}

func (a *terminalApp) noteAbsPath(rel string) string {
	return filepath.Join(a.notesRootPath(), filepath.FromSlash(kokosync.NormalizeNotePath(rel)))
}

func currentSettingsMap() (map[string]any, error) {
	data, err := json.Marshal(settings.Inst())
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func applySettingsMap(values map[string]any) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	var next settings.UserSettings
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	*settings.Inst() = next
	settings.SaveSettingsLocal()
	return nil
}

func firebaseEnabled(cfg *settings.FirebaseSettings, fileCfg kokosync.FirebaseConfig) bool {
	if fileCfg.Enabled && fileCfg.Realtime {
		return true
	}
	return cfg != nil && cfg.Enabled && cfg.Realtime
}

func firebaseAPIKey(cfg *settings.FirebaseSettings, fileCfg kokosync.FirebaseConfig) string {
	if strings.TrimSpace(fileCfg.APIKey) != "" {
		return strings.TrimSpace(fileCfg.APIKey)
	}
	if cfg != nil && strings.TrimSpace(cfg.APIKey) != "" {
		return strings.TrimSpace(cfg.APIKey)
	}
	if env := strings.TrimSpace(os.Getenv("KOKO_FIREBASE_API_KEY")); env != "" {
		return env
	}
	return kokosync.DefaultAPIKey
}

func firebaseDatabaseURL(cfg *settings.FirebaseSettings, fileCfg kokosync.FirebaseConfig) string {
	if strings.TrimSpace(fileCfg.DatabaseURL) != "" {
		return strings.TrimSpace(fileCfg.DatabaseURL)
	}
	if cfg != nil && strings.TrimSpace(cfg.DatabaseURL) != "" {
		return strings.TrimSpace(cfg.DatabaseURL)
	}
	if env := strings.TrimSpace(os.Getenv("KOKO_FIREBASE_DATABASE_URL")); env != "" {
		return env
	}
	return kokosync.DefaultDatabaseURL
}

func firebaseWorkspaceID(cfg *settings.FirebaseSettings, fileCfg kokosync.FirebaseConfig) string {
	if strings.TrimSpace(fileCfg.WorkspaceID) != "" {
		return strings.TrimSpace(fileCfg.WorkspaceID)
	}
	if cfg != nil && strings.TrimSpace(cfg.WorkspaceID) != "" {
		return strings.TrimSpace(cfg.WorkspaceID)
	}
	return strings.TrimSpace(os.Getenv("KOKO_FIREBASE_WORKSPACE_ID"))
}

func formatSnapshotLabel(name string, createdAt string) string {
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		return fmt.Sprintf("%s (%s)", name, t.Local().Format("2006-01-02 15:04"))
	}
	if createdAt != "" {
		return fmt.Sprintf("%s (%s)", name, createdAt)
	}
	return name
}

func (a *terminalApp) helpText() string {
	switch a.view {
	case viewNotes:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		return a.notes.HelpText() + " | ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs"
	case viewFiles:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		return "files: ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs | j/k move | / filter | a import into scope | f nested folder | F scope folder | D discard staged | i smart | I link | p image | o open | y copy md | Y copy path | M migrate | : command"
	case viewPages:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		if a.pages != nil && a.pages.IsEditing() {
			return "pages/edit: digits replace selection | tab/shift+tab field | left/right cursor | backspace delete | enter apply | esc stop edit"
		}
		return "pages: q quit | ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs | ctrl+s save | j/k move | e edit | r recalc"
	case viewPassword:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		return "password: ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs | ctrl+s save | g generate | l/n/s toggle | +/- length"
	case viewTodo:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		if a.todoInputMode != "" {
			return "todo/edit: text input | enter save | esc cancel"
		}
		return "todo: ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs | ctrl+s save | j/k move | n new | enter/space check | e edit | J/K reorder"
	case viewSync:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		return "sync: ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs | j/k move | enter run action | save locally before upload"
	case viewSettings:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		if a.tabOrderEditMode {
			return "settings/tab order: j/k select tab | J/K move selected | enter/esc done"
		}
		if a.settingsEditMode {
			return "settings/edit: digits edit | backspace delete | enter apply | esc cancel"
		}
		return "settings: ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs | ctrl+s save | j/k move | enter change option"
	case viewRecorder:
		if a.tabSelect {
			return fmt.Sprintf("tab select: left/right move | %s jump | ctrl+%s direct jump | enter confirm | esc cancel", a.appTabKeyHint(), a.appTabKeyHint())
		}
		if a.recorderCapturing {
			return "recorder: capturing keys for 5s | all other key bindings blocked"
		}
		return "recorder: started by :recordkeys | ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs"
	default:
		return "q quit | ctrl+t tab bar | ctrl+tab next tab | ctrl+" + a.appTabKeyHint() + " tabs"
	}
}

func (a *terminalApp) renderHelpOverlay(width int, height int) (string, []string) {
	type helpEntry struct {
		keys string
		desc string
	}
	renderSection := func(title string, entries []helpEntry) []string {
		lines := []string{title}
		keyWidth := 24
		if width > 0 {
			keyWidth = min(28, max(18, width/3))
		}
		descWidth := max(12, width-keyWidth-3)
		for _, entry := range entries {
			descLines := wrapHelpText(entry.desc, descWidth)
			keyCell := fmt.Sprintf("  %-*s", keyWidth, entry.keys)
			if len(descLines) == 0 {
				lines = append(lines, strings.TrimRight(keyCell, " "))
				continue
			}
			lines = append(lines, keyCell+descLines[0])
			for _, line := range descLines[1:] {
				lines = append(lines, strings.Repeat(" ", keyWidth+2)+line)
			}
		}
		return append(lines, "")
	}
	lines := []string{"Keyboard Shortcuts", ""}
	lines = append(lines, renderSection("Global:", []helpEntry{
		{keys: "q", desc: "quit when not editing"},
		{keys: "ctrl+t", desc: "activate tab bar"},
		{keys: "ctrl+tab", desc: "cycle to the next app tab"},
		{keys: "ctrl+" + a.appTabKeyHint(), desc: "jump directly to an app tab when visible"},
		{keys: a.appTabKeyHint(), desc: "jump to tab while tab bar is active"},
		{keys: "mouse click tab", desc: "jump directly to an app tab"},
		{keys: "left/right", desc: "move tab selection while tab bar is active"},
		{keys: "enter", desc: "confirm tab selection"},
		{keys: "esc", desc: "cancel tab selection or close active prompt/help state"},
		{keys: "?", desc: "toggle keyboard help"},
		{keys: "j/k, arrows, g/G, ctrl+b/ctrl+f", desc: "scroll help overlay"},
	})...)
	lines = append(lines, renderSection("Notes sidebar:", []helpEntry{
		{keys: "ctrl+a, :sidebar, :sb", desc: "toggle sidebar/editor focus"},
		{keys: "j/k, arrows", desc: "move selection"},
		{keys: "enter, l", desc: "focus selected open note or open selected browser note"},
		{keys: "o", desc: "open all notes inside selected browser folder"},
		{keys: "e", desc: "toggle the full note browser while the sidebar is focused"},
		{keys: "h", desc: "return to editor"},
		{keys: "a", desc: "switch to last accessed note and return to editor"},
		{keys: "1-9, 0", desc: "switch to numbered open note and return to editor"},
		{keys: "x", desc: "close selected open note without deleting its file"},
		{keys: "n", desc: "new note"},
		{keys: "f", desc: "new folder"},
		{keys: "d", desc: "delete selected note"},
		{keys: "m", desc: "move selected browser note or folder"},
		{keys: "[, ]", desc: "switch open note tabs"},
		{keys: "R", desc: "rename current note"},
	})...)
	lines = append(lines, renderSection("Notes normal:", []helpEntry{
		{keys: "i, a", desc: "enter insert mode at cursor or after cursor"},
		{keys: "o, O", desc: "open a new line below or above"},
		{keys: "h j k l, arrows", desc: "move cursor"},
		{keys: "0, $", desc: "jump to line start or end"},
		{keys: "w, b", desc: "move by word"},
		{keys: "u", desc: "undo"},
		{keys: "x", desc: "delete character under cursor"},
		{keys: "xw", desc: "delete next word"},
		{keys: "x$", desc: "delete to end of line"},
		{keys: "r<char>", desc: "replace character under cursor"},
		{keys: "dd, yy", desc: "delete line or yank line"},
		{keys: "dw, cw, yw", desc: "operator motions for word delete, change, yank"},
		{keys: "p", desc: "paste register"},
		{keys: ":, /", desc: "enter command or search"},
		{keys: "n, N", desc: "next or previous search match"},
		{keys: "ctrl+g, :spell", desc: "open spelling suggestions for the word under cursor"},
		{keys: "zg", desc: "add word under cursor to the shared spell dictionary"},
		{keys: "v, V, ctrl+v", desc: "start visual char, line, or block selection"},
		{keys: "ctrl+s", desc: "save locally"},
		{keys: "ctrl+n", desc: "new note"},
		{keys: "ctrl+d", desc: "delete current note"},
		{keys: "R", desc: "rename current note"},
	})...)
	lines = append(lines, renderSection("Notes visual:", []helpEntry{
		{keys: "h j k l, arrows, w/b, 0/$, home/end, pageup/pagedown", desc: "move selection"},
		{keys: "y", desc: "yank selection"},
		{keys: "d, x", desc: "delete selection"},
		{keys: "esc", desc: "leave visual mode"},
	})...)
	lines = append(lines, renderSection("Notes insert:", []helpEntry{
		{keys: "text input", desc: "insert text at cursor"},
		{keys: "backspace, delete", desc: "edit text"},
		{keys: "enter", desc: "insert newline"},
		{keys: "tab", desc: "path-complete inside markdown refs, otherwise insert spaces"},
		{keys: "shift+tab", desc: "reverse path-complete inside markdown refs"},
		{keys: "ctrl+g, :spell", desc: "open spelling suggestions for the word under cursor"},
		{keys: "esc", desc: "return to normal mode"},
	})...)
	lines = append(lines, renderSection("Notes command:", []helpEntry{
		{keys: ":w", desc: "save current local state"},
		{keys: "bd", desc: "close current note tab without deleting its file"},
		{keys: "undo", desc: "undo last text change"},
		{keys: "redo", desc: "redo last undone text change"},
		{keys: "preview", desc: "toggle preview pane"},
		{keys: ":spell", desc: "open spelling suggestions for the word under cursor"},
		{keys: "/text", desc: "search for text"},
		{keys: "ol", desc: "review and open all unique external links"},
		{keys: "recordkeys", desc: "open the on-demand recorder tab and capture keys for 5 seconds"},
		{keys: "rename NAME", desc: "rename the current note"},
		{keys: "%s/old/new/g", desc: "replace all matches"},
		{keys: "enter", desc: "run command"},
		{keys: "esc", desc: "cancel command"},
	})...)
	lines = append(lines, renderSection("Recorder:", []helpEntry{
		{keys: ":recordkeys", desc: "open the recorder tab from Notes and start a timed capture"},
		{keys: "5-second capture", desc: "record keys and block all other app key bindings while active"},
		{keys: "status + history", desc: "show latest key details and recent captured events"},
	})...)
	lines = append(lines, renderSection("Files:", []helpEntry{
		{keys: "j/k, arrows", desc: "move selection"},
		{keys: "enter, l", desc: "toggle folder"},
		{keys: "/", desc: "filter files"},
		{keys: "a", desc: "import path(s) into current scope"},
		{keys: "f", desc: "create nested folder"},
		{keys: "F", desc: "create scope-root folder"},
		{keys: "r", desc: "rename selected item"},
		{keys: "m", desc: "move selected item"},
		{keys: "d", desc: "delete selected item"},
		{keys: "D", desc: "discard staged file changes"},
		{keys: "i", desc: "smart insert selected file into current note"},
		{keys: "I", desc: "force link insert"},
		{keys: "p", desc: "force image insert"},
		{keys: "o", desc: "open selected file externally"},
		{keys: "y", desc: "copy markdown reference"},
		{keys: "Y", desc: "copy relative path"},
		{keys: "M", desc: "confirm loose-file migration"},
		{keys: ":", desc: "enter file command mode"},
		{keys: "import | mkdir | rename | move | insert | migrate", desc: "available file commands"},
	})...)
	lines = append(lines, renderSection("Pages:", []helpEntry{
		{keys: "j/k, tab", desc: "move field focus"},
		{keys: "e, enter", desc: "edit selected field"},
		{keys: "digits", desc: "edit value"},
		{keys: "backspace", desc: "delete from value"},
		{keys: "enter", desc: "apply and recalculate"},
		{keys: "esc", desc: "stop editing"},
		{keys: "r", desc: "recalculate"},
	})...)
	lines = append(lines, renderSection("Password:", []helpEntry{
		{keys: "g", desc: "generate password"},
		{keys: "l, n, s", desc: "toggle letters, numbers, symbols"},
		{keys: "+, -", desc: "change password length"},
	})...)
	lines = append(lines, renderSection("Todo:", []helpEntry{
		{keys: "j/k, arrows", desc: "move selection"},
		{keys: "n", desc: "create a todo"},
		{keys: "enter, space", desc: "check or uncheck selected todo"},
		{keys: "e", desc: "edit selected active todo"},
		{keys: "J/K", desc: "reorder selected active unchecked todo"},
	})...)
	lines = append(lines, renderSection("Sync:", []helpEntry{
		{keys: "j/k", desc: "move selection"},
		{keys: "enter, space", desc: "run selected action"},
	})...)
	lines = append(lines, renderSection("Settings:", []helpEntry{
		{keys: "j/k", desc: "move selection"},
		{keys: "enter, space", desc: "change selected option"},
		{keys: "digits, backspace, enter, esc", desc: "edit numeric setting values while a numeric field is active"},
		{keys: "tab order J/K", desc: "move the selected app tab while editing tab order"},
	})...)
	return annotateHelpSearch(strings.TrimRight(strings.Join(lines, "\n"), "\n"), a.helpSearchQuery)
}

func formatOpenLinksModalText(noteTitle string, links []string) string {
	lines := []string{fmt.Sprintf("Open all links from %q?", noteTitle), ""}
	lines = append(lines, links...)
	return strings.Join(lines, "\n")
}

func wrapHelpText(text string, width int) []string {
	if width <= 0 {
		width = 12
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, 4)
	current := words[0]
	for _, word := range words[1:] {
		if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func (a *terminalApp) advanceHelpSearch(delta int) {
	_, regions := annotateHelpSearch(a.renderHelpBody(), a.helpSearchQuery)
	if len(regions) == 0 {
		a.helpSearchIndex = 0
		return
	}
	a.helpSearchIndex = (a.helpSearchIndex + delta) % len(regions)
	if a.helpSearchIndex < 0 {
		a.helpSearchIndex += len(regions)
	}
}

func (a *terminalApp) renderHelpBody() string {
	text, _ := a.renderHelpOverlay(maxInt(20, a.width-6), maxInt(8, a.height-6))
	return stripHelpRegions(text)
}

func annotateHelpSearch(text string, query string) (string, []string) {
	if query == "" {
		return text, nil
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	regions := make([]string, 0, 8)
	last := 0
	index := 0
	for {
		found := strings.Index(lowerText[last:], lowerQuery)
		if found < 0 {
			b.WriteString(text[last:])
			break
		}
		start := last + found
		end := start + len(query)
		region := "help-hit-" + strconv.Itoa(index)
		regions = append(regions, region)
		b.WriteString(text[last:start])
		b.WriteString(`["` + region + `"]`)
		b.WriteString(text[start:end])
		b.WriteString(`[""]`)
		last = end
		index++
	}
	return b.String(), regions
}

func stripHelpRegions(text string) string {
	text = strings.ReplaceAll(text, `[""]`, "")
	for {
		start := strings.Index(text, `["`)
		if start < 0 {
			return text
		}
		end := strings.Index(text[start:], `"]`)
		if end < 0 {
			return text
		}
		text = text[:start] + text[start+end+2:]
	}
}

func configDir(home string) string {
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
}

func configureRuntimeLogging() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.SetOutput(io.Discard)
		return
	}
	path := filepath.Join(configDir(home), "koko-tools.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.SetOutput(io.Discard)
		return
	}
	log.SetOutput(file)
}

func ensureConfigDirExists(target string) {
	_ = os.MkdirAll(target, 0o755)
}

func makeConfigDirIfNotExists() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	ensureConfigDirExists(configDir(home))
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func notesEditorGutterWidth(ed *notes.Editor, width int) int {
	if ed == nil || width < 8 {
		return 0
	}
	lineCount := 1 + strings.Count(ed.Text, "\n")
	digits := len(fmt.Sprintf("%d", lineCount))
	return digits + 1
}
