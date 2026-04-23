package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
)

type view int

const (
	viewNotes view = iota
	viewFiles
	viewPages
	viewPassword
	viewSync
	viewSettings
)

const manualSyncTimeout = 20 * time.Second

type terminalApp struct {
	view               view
	ctx                context.Context
	notes              *notes.Workspace
	pages              *pages.Model
	password           *password.Model
	status             string
	width              int
	height             int
	settingIndex       int
	syncIndex          int
	tui                *tview.Application
	header             *tview.TextView
	help               *tview.TextView
	statusBar          *tview.TextView
	body               *tview.Flex
	pagesRoot          *tview.Pages
	sidebar            *tview.TextView
	editor             *tview.TextView
	preview            *tview.TextView
	commandBar         *tview.TextView
	single             *tview.TextView
	helpOverlay        *tview.TextView
	quitModal          *tview.Modal
	discardFilesModal  *tview.Modal
	deleteNoteModal    *tview.Modal
	openLinksModal     *tview.Modal
	root               *tview.Flex
	lastStatus         string
	tabSelect          bool
	showHelp           bool
	helpSearchMode     bool
	helpSearchQuery    string
	helpSearchIndex    int
	shuttingDown       bool
	settingsDirty      bool
	settingsEditMode   bool
	settingsEditBuffer string
	syncInProgress     bool
	syncProgressLabel  string
	syncOpID           atomic.Int64
	syncSpinnerTick    atomic.Int64
	syncTimeout        time.Duration
	openLinks          []string
	deleteNotePath     string
	deleteNoteLabel    string
	notesMouseDragging bool
	notesMouseMoved    bool
	notesMouseStartRow int
	notesMouseStartCol int
	inputSeqMu         sync.Mutex
	inputSeq           string
	inputSeqTimer      *time.Timer
}

type appTab struct {
	label string
	view  view
	key   string
}

var appTabs = []appTab{
	{"Notes", viewNotes, "1"},
	{"Files", viewFiles, "2"},
	{"Pages", viewPages, "3"},
	{"Password", viewPassword, "4"},
	{"Sync", viewSync, "5"},
	{"Settings", viewSettings, "6"},
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
	app := &terminalApp{
		view:        viewNotes,
		notes:       noteWS,
		pages:       pages.NewModel(),
		password:    password.NewModel(),
		width:       120,
		height:      36,
		syncTimeout: manualSyncTimeout,
	}
	app.initWidgets()
	return app, nil
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
			if err := a.notes.DeleteNoteByPath(a.deleteNotePath); err == nil {
				helpers.StatusBarInst().UpdateStatusBar("Deleted note: " + target)
			} else {
				helpers.StatusBarInst().UpdateStatusBar("Delete note failed")
			}
		}
		a.deleteNotePath = ""
		a.deleteNoteLabel = ""
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
			fmt.Println("views: notes, files, pages, password, sync, settings")
			fmt.Println("save: :w, quit: :q, save and quit: :wq")
		case "notes":
			a.view = viewNotes
		case "files":
			a.view = viewFiles
		case "pages":
			a.view = viewPages
		case "password":
			a.view = viewPassword
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
	if settings.Inst().NotesApp.SidebarVisible && a.sidebar != nil {
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
	if !pointInRect(x, y, ex, ey, ew, eh) && !a.notesMouseDragging {
		return event, action
	}
	row := y - ey - 1
	col := x - ex
	if row == -1 {
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
	"\x1b[9;5u":    {Name: "tab", Ctrl: true},
	"\x1b[27;5;9~": {Name: "tab", Ctrl: true},
	"\x1b[1;5I":    {Name: "tab", Ctrl: true},
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

func appTabViewForKey(key string) (view, bool) {
	for _, tab := range appTabs {
		if tab.key == key {
			return tab.view, true
		}
	}
	return viewNotes, false
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
	for i, tab := range appTabs {
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
		if i < len(appTabs)-1 {
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
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return notes.Key{Ctrl: true, Name: string(r), Rune: r}, true
		}
		return notes.Key{Name: string(r), Rune: r, Shift: event.Modifiers()&tcell.ModShift != 0, Alt: event.Modifiers()&tcell.ModAlt != 0}, true
	case tcell.KeyEnter:
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
	case tcell.KeyCtrlSpace:
		return notes.Key{Name: "2", Ctrl: true}, true
	case tcell.KeyCtrlBackslash:
		return notes.Key{Name: "4", Ctrl: true}, true
	case tcell.KeyCtrlRightSq:
		return notes.Key{Name: "5", Ctrl: true}, true
	case tcell.KeyCtrlCarat:
		return notes.Key{Name: "6", Ctrl: true}, true
	default:
		return notes.Key{}, false
	}
}

func (a *terminalApp) handleGlobalKey(key notes.Key) bool {
	if a.shuttingDown {
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
		case "1", "2", "3", "4", "5", "6":
			if target, ok := appTabViewForKey(key.Name); ok {
				a.switchAppTab(target)
				return true
			}
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
		switch key.Name {
		case "1":
			a.switchAppTab(viewNotes)
			return true
		case "2":
			a.switchAppTab(viewFiles)
			return true
		case "3":
			a.switchAppTab(viewPages)
			return true
		case "4":
			a.switchAppTab(viewPassword)
			return true
		case "5":
			a.switchAppTab(viewSync)
			return true
		case "6":
			a.switchAppTab(viewSettings)
			return true
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
	case viewSync:
		return a.handleSyncKey(key)
	case viewSettings:
		return a.handleSettingsKey(key)
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
	if path, label, ok := a.notes.TakePendingDeleteNote(); ok {
		a.deleteNotePath = path
		a.deleteNoteLabel = label
		a.showDeleteNoteModal()
		return
	}
	if a.notes.TakePendingSaveAll() {
		if err := a.saveLocalState(); err != nil {
			helpers.StatusBarInst().UpdateStatusBar("Save failed: " + err.Error())
		} else {
			if ed := a.notes.ActiveEditor(); ed != nil {
				ed.Status = "saved"
			}
			helpers.StatusBarInst().UpdateStatusBar("Saved locally at " + formatTimestampOrNever(settings.Inst().GDrive.LastLocalSaveAt))
		}
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
	switch a.view {
	case viewNotes:
		return viewFiles
	case viewFiles:
		return viewPages
	case viewPages:
		return viewPassword
	case viewPassword:
		return viewSync
	case viewSync:
		return viewSettings
	default:
		return viewNotes
	}
}

func (a *terminalApp) prevView() view {
	switch a.view {
	case viewSettings:
		return viewSync
	case viewSync:
		return viewPassword
	case viewPassword:
		return viewPages
	case viewPages:
		return viewFiles
	case viewFiles:
		return viewNotes
	default:
		return viewSettings
	}
}

func (a *terminalApp) wantsQuitOnQ() bool {
	switch a.view {
	case viewNotes:
		return a.notes == nil || !a.notes.IsEditableContext()
	case viewFiles:
		return a.notes == nil || !a.notes.IsFilesEditableContext()
	case viewPages:
		return a.pages == nil || !a.pages.IsEditing()
	case viewSettings:
		return !a.settingsEditMode
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
	case viewSettings:
		return !a.settingsEditMode
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
	case viewSettings:
		return !a.settingsEditMode
	default:
		return true
	}
}

func (a *terminalApp) handlePagesKey(key notes.Key) bool {
	if a.pages == nil {
		return false
	}
	if a.pages.IsEditing() {
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
			a.settingsDirty = true
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
	if a == nil || a.notes == nil || !a.notes.CanDeleteFocusedNote() {
		return
	}
	a.deleteNotePath = a.notes.FocusedNoteDeletePath()
	label := a.notes.FocusedNoteDeleteLabel()
	if strings.TrimSpace(label) == "" {
		label = "current note"
	}
	a.deleteNoteLabel = label
	if a.deleteNoteModal != nil {
		a.deleteNoteModal.SetText(fmt.Sprintf("Delete note %q?", label))
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
	items := []actionItem{
		{Label: fmt.Sprintf("drive sync enabled: %t", cfg.Enabled), Apply: func() {
			cfg.Enabled = !cfg.Enabled
			a.settingsDirty = true
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
			a.settingsDirty = true
		}},
		{Label: fmt.Sprintf("transparent background: %t", cfg.UI != nil && cfg.UI.TransparentBackground), Apply: func() {
			if cfg.UI == nil {
				cfg.UI = &settings.UISettings{}
			}
			cfg.UI.TransparentBackground = !cfg.UI.TransparentBackground
			applyGlobalBackgroundStyle()
			a.applyWidgetBackgroundStyle()
			a.settingsDirty = true
		}},
		{Label: fmt.Sprintf("vim mode: %t", cfg.NotesApp.VimMode), Apply: func() {
			cfg.NotesApp.VimMode = !cfg.NotesApp.VimMode
			a.settingsDirty = true
		}},
		{Label: fmt.Sprintf("tab spaces: %d", cfg.NotesApp.TabSpaces), Apply: func() {
			cfg.NotesApp.TabSpaces++
			if cfg.NotesApp.TabSpaces > 8 {
				cfg.NotesApp.TabSpaces = 2
			}
			a.settingsDirty = true
		}},
		{Label: a.settingsUndoLevelsLabel(cfg.NotesApp.UndoLevels), Apply: func() {
			a.settingsEditMode = true
			a.settingsEditBuffer = strconv.Itoa(cfg.NotesApp.UndoLevels)
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
			a.settingsDirty = true
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
					a.settingsDirty = true
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
			a.settingsDirty = true
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
	case viewSync:
		a.refreshSingle("Sync", a.renderSync(maxInt(3, a.height-10)))
	case viewSettings:
		a.refreshSingleMarkup("Settings", a.renderSettings(maxInt(3, a.height-10)))
	}
}

func (a *terminalApp) renderTabBar() string {
	parts := make([]string, 0, len(appTabs))
	for _, tab := range appTabs {
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
	case viewSettings, viewSync:
		return a.settingsDirty
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
	if col, ok := a.notes.CommandCursor(); ok {
		x, y, width, _ := a.commandBar.GetInnerRect()
		if col >= width {
			col = width - 1
		}
		if col < 0 {
			col = 0
		}
		screen.ShowCursor(x+col, y)
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

func joinTViewLines(lines []string) string {
	converted := make([]string, len(lines))
	for i, line := range lines {
		converted[i] = ansiToTView(line)
	}
	return strings.Join(converted, "\n")
}

func ansiToTView(s string) string {
	theme := currentTheme()
	escaped := tview.Escape(s)
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
		helpers.ANSIRoleSelection, themeMarkupPair(theme.SelectionFG, theme.SelectionBG),
		helpers.ANSIRoleSpellError, themeMarkupFGStyle(theme.ErrorAccent, "u"),
		"\x1b[0m", "[-:-:-]",
	)
	return replacer.Replace(escaped)
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
	lines := []string{
		"Pages calculator",
		"j/k move | e/enter edit | digits edit | esc stop edit | r recalc",
		fmt.Sprintf("%sfirst book:  %s", focusPrefix(0), a.pages.FirstBookInput),
		fmt.Sprintf("%sread pages:  %s", focusPrefix(1), a.pages.ReadInput),
		fmt.Sprintf("%sother book:  %s", focusPrefix(2), a.pages.SecondBookInput),
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

func (a *terminalApp) renderSync(height int) string {
	cfg := settings.Inst().GDrive
	folder := cfg.FolderName
	if strings.TrimSpace(folder) == "" {
		folder = cfg.FolderID
	}
	if strings.TrimSpace(folder) == "" {
		folder = "not selected"
	}
	lines := []string{
		"Google Drive sync",
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

func (a *terminalApp) renderSettings(height int) string {
	items := a.settingsItems()
	header := "Settings"
	if a.settingsEditMode {
		header = "Settings (editing undo levels)"
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
	escaped := tview.Escape(label)
	if index != 0 {
		return escaped
	}
	return escaped + "  " + renderThemePreview(themeByName(settings.CurrentTheme()))
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

func (a *terminalApp) settingsHelpLine() string {
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
	if a.settingsDirty || (a.notes == nil || !a.notes.HasDirty()) && (a.pages == nil || !a.pages.Dirty) && (a.password == nil || !a.password.Dirty) {
		settings.SaveSettingsLocal()
	}
	a.settingsDirty = false
	return nil
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
			return "tab select: left/right move | 1-6 jump | ctrl+1-6 direct jump | enter confirm | esc cancel"
		}
		return a.notes.HelpText() + " | ctrl+t tab bar | ctrl+tab next tab | ctrl+1-6 tabs"
	case viewFiles:
		if a.tabSelect {
			return "tab select: left/right move | 1-6 jump | ctrl+1-6 direct jump | enter confirm | esc cancel"
		}
		return "files: ctrl+tab next tab | ctrl+1-6 tabs | j/k move | / filter | a import into scope | f nested folder | F scope folder | D discard staged | i smart | I link | p image | o open | y copy md | Y copy path | M migrate | : command"
	case viewPages:
		if a.tabSelect {
			return "tab select: left/right move | 1-6 jump | ctrl+1-6 direct jump | enter confirm | esc cancel"
		}
		if a.pages != nil && a.pages.IsEditing() {
			return "pages/edit: digits edit | backspace delete | enter apply | esc stop edit"
		}
		return "pages: q quit | ctrl+t tab bar | ctrl+tab next tab | ctrl+1-6 tabs | ctrl+s save | j/k move | e edit | r recalc"
	case viewPassword:
		if a.tabSelect {
			return "tab select: left/right move | 1-6 jump | ctrl+1-6 direct jump | enter confirm | esc cancel"
		}
		return "password: ctrl+t tab bar | ctrl+tab next tab | ctrl+1-6 tabs | ctrl+s save | g generate | l/n/s toggle | +/- length"
	case viewSync:
		if a.tabSelect {
			return "tab select: left/right move | 1-6 jump | ctrl+1-6 direct jump | enter confirm | esc cancel"
		}
		return "sync: ctrl+t tab bar | ctrl+tab next tab | ctrl+1-6 tabs | j/k move | enter run action | save locally before upload"
	case viewSettings:
		if a.tabSelect {
			return "tab select: left/right move | 1-6 jump | ctrl+1-6 direct jump | enter confirm | esc cancel"
		}
		if a.settingsEditMode {
			return "settings/edit: digits edit | backspace delete | enter apply | esc cancel"
		}
		return "settings: ctrl+t tab bar | ctrl+tab next tab | ctrl+1-6 tabs | ctrl+s save | j/k move | enter change option"
	default:
		return "q quit | ctrl+t tab bar | ctrl+tab next tab | ctrl+1-6 tabs"
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
		{keys: "ctrl+1-6", desc: "jump directly to an app tab"},
		{keys: "1-6", desc: "jump to tab while tab bar is active"},
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
		{keys: "enter, l", desc: "open note or toggle folder"},
		{keys: "h", desc: "return to editor"},
		{keys: "a", desc: "switch to last accessed note and return to editor"},
		{keys: "1-9, 0", desc: "switch to numbered open note and return to editor"},
		{keys: "n", desc: "new note"},
		{keys: "f", desc: "new folder"},
		{keys: "d", desc: "delete selected note"},
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
		{keys: "esc", desc: "return to normal mode"},
	})...)
	lines = append(lines, renderSection("Notes command:", []helpEntry{
		{keys: ":w", desc: "save current local state"},
		{keys: "undo", desc: "undo last text change"},
		{keys: "redo", desc: "redo last undone text change"},
		{keys: "preview", desc: "toggle preview pane"},
		{keys: "/text", desc: "search for text"},
		{keys: "ol", desc: "review and open all unique external links"},
		{keys: "rename NAME", desc: "rename the current note"},
		{keys: "%s/old/new/g", desc: "replace all matches"},
		{keys: "enter", desc: "run command"},
		{keys: "esc", desc: "cancel command"},
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
	lines = append(lines, renderSection("Sync:", []helpEntry{
		{keys: "j/k", desc: "move selection"},
		{keys: "enter, space", desc: "run selected action"},
	})...)
	lines = append(lines, renderSection("Settings:", []helpEntry{
		{keys: "j/k", desc: "move selection"},
		{keys: "enter, space", desc: "change selected option"},
		{keys: "digits, backspace, enter, esc", desc: "edit numeric setting values while a numeric field is active"},
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
