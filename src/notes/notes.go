package notes

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

type Note struct {
	F                    *gtk.Frame
	notebook             *gtk.Notebook
	sidebar              *gtk.ListBox
	sidebarBox           *gtk.Box
	sidebarContent       *gtk.Box
	sidebarPane          *gtk.Paned
	sidebarScroll        *gtk.ScrolledWindow
	sidebarActions       *gtk.Box
	sidebarToggle        *gtk.ToggleButton
	sidebarDragToggle    *gtk.ToggleButton
	sidebarNewNote       *gtk.Button
	sidebarNewFolder     *gtk.Button
	sidebarImportFile    *gtk.Button
	sidebarImportFolder  *gtk.Button
	sidebarExportPDF     *gtk.Button
	sidebarDropHint      *gtk.Label
	sidebarMenu          *gtk.Popover
	sidebarMenuEntry     *gtk.Entry
	sidebarCreate        *gtk.Popover
	sidebarCreateEntry   *gtk.Entry
	sidebarRows          map[string]*gtk.ListBoxRow
	sidebarRowEntries    map[uintptr]sidebarEntry
	sidebarActionButtons map[string]*gtk.Button
	sidebarCollapsed     map[string]bool
	sidebarMenuKind      sidebarEntryKind
	sidebarMenuPath      string
	sidebarMenuFolder    string
	selectedFolder       string
	tabs                 []*noteTab
	currentTab           int
	note                 *gtk.TextView
	buffer               *gtk.TextBuffer
	preview              *gtk.TextView
	previewBuffer        *gtk.TextBuffer
	previewLinks         []markdownLink
	paned                *gtk.Paned
	previewToggle        *gtk.ToggleButton
	wrapToggle           *gtk.ToggleButton
	commandEntry         *gtk.Entry
	vimInsertMode        bool
	vimPendingOp         string
	selectionMode        vimSelectionMode
	selectionMark        int
	selectionCursor      int
	yankRegister         vimRegister
	commandMode          bool
	lastSearch           string
	lastSearchPos        int
	sidebarCreateMode    sidebarEntryKind
	sidebarCreateFolder  string
	sidebarDragSource    sidebarEntry
	sidebarDragPending   bool
	sidebarDragActive    bool
	sidebarDragTarget    sidebarDropTarget
	sidebarDragSeq       int64
	saveCommand          func()
	WaitingToSave        bool
}

var saveCounter atomic.Int64
var currentNote *Note
var noteWriteFile = os.WriteFile
var noteStartDriveSync = settings.StartDriveSync
var noteMarkDriveDirty = settings.MarkDriveDirty

func FlushCurrentNoteState() error {
	if currentNote == nil {
		return nil
	}
	return currentNote.flushAllTabs(false)
}

func GenerateUI() *Note {
	saveCounter.Store(0)
	n := Note{
		currentTab:           -1,
		WaitingToSave:        false,
		sidebarRows:          make(map[string]*gtk.ListBoxRow),
		sidebarRowEntries:    make(map[uintptr]sidebarEntry),
		sidebarActionButtons: make(map[string]*gtk.Button),
		sidebarCollapsed:     make(map[string]bool),
	}
	n.saveCommand = func() {
		glib.IdleAdd(func() {
			n.save()
		})
	}
	n.buildWorkspace()
	n.openInitialTabs()
	currentNote = &n
	n.updateStatusBar()

	return &n
}

func noteScrolledWindow(child gtk.Widgetter) *gtk.ScrolledWindow {
	scrollW := gtk.NewScrolledWindow()
	scrollW.SetHExpand(true)
	scrollW.SetVExpand(true)
	scrollW.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrollW.SetPropagateNaturalHeight(false)
	scrollW.SetPropagateNaturalWidth(false)
	scrollW.SetMinContentHeight(0)
	scrollW.SetMinContentWidth(0)
	scrollW.SetChild(child)
	return scrollW
}

func styledBuffer(preview bool) *gtk.TextBuffer {
	buffer := gtk.NewTextBuffer(nil)
	appearance := currentAppearance()
	configureMarkdownTags(buffer, appearance, preview)
	installNotesCSS(appearance)
	return buffer
}

func (n *Note) applyMarkdownFormatting() {
	if n.buffer == nil {
		return
	}
	configureMarkdownTags(n.buffer, currentAppearance(), false)
	if tab := n.activeTab(); tab != nil {
		n.applyEditorWrap(tab)
	}

	start := n.buffer.StartIter()
	end := n.buffer.EndIter()
	n.buffer.TagTable().ForEach(func(tag *gtk.TextTag) {
		n.buffer.RemoveTag(tag, start, end)
	})
	n.buffer.ApplyTagByName(tagEditorBody, start, end)

	text := n.buffer.Text(start, end, true)
	for _, span := range markdownSpans(text) {
		if span.Start >= span.End {
			continue
		}
		n.buffer.ApplyTagByName(span.Tag, n.buffer.IterAtOffset(span.Start), n.buffer.IterAtOffset(span.End))
	}
	n.applyVisualSelection()
}

func (n *Note) updatePreview() {
	if n.previewBuffer == nil || n.buffer == nil {
		return
	}

	appearance := currentAppearance()
	configureMarkdownTags(n.previewBuffer, appearance, true)
	installNotesCSS(appearance)
	render := markdownPreview(n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true), settings.Inst().NotesApp.TabSpaces)
	n.previewLinks = render.Links
	n.previewBuffer.SetText(render.Text)
	start := n.previewBuffer.StartIter()
	end := n.previewBuffer.EndIter()
	n.previewBuffer.TagTable().ForEach(func(tag *gtk.TextTag) {
		n.previewBuffer.RemoveTag(tag, start, end)
	})
	n.previewBuffer.ApplyTagByName(tagPreviewBody, start, end)
	for _, span := range render.Spans {
		if span.Start >= span.End {
			continue
		}
		n.previewBuffer.ApplyTagByName(span.Tag, n.previewBuffer.IterAtOffset(span.Start), n.previewBuffer.IterAtOffset(span.End))
	}
	n.renderPreviewImages(render.Images)
}

func (n *Note) refreshFromSettings() {
	if n == nil {
		return
	}
	n.applySidebarVisibility(settings.Inst().NotesApp.SidebarVisible)
	if len(n.tabs) > 0 {
		n.persistActiveTabState()
		for _, tab := range n.tabs {
			n.loadTab(tab)
			n.refreshEditorMode()
			n.applyMarkdownFormatting()
			n.updatePreview()
			n.persistActiveTabState()
		}
		if n.currentTab >= 0 && n.currentTab < len(n.tabs) {
			n.loadTab(n.tabs[n.currentTab])
		}
		return
	}
	n.refreshEditorMode()
	n.applyMarkdownFormatting()
	n.updatePreview()
}

func (n *Note) refreshEditorMode() {
	if !settings.Inst().NotesApp.VimMode {
		n.vimInsertMode = true
		n.vimPendingOp = ""
		n.selectionMode = vimSelectionNone
		n.commandMode = false
		n.hideCommandLine(false)
		n.syncCursorMode()
		n.updateStatusBar()
		return
	}
	n.vimInsertMode = false
	n.vimPendingOp = ""
	n.selectionMode = vimSelectionNone
	n.commandMode = false
	n.hideCommandLine(false)
	n.syncCursorMode()
	n.updateStatusBar()
}

func (n *Note) wrapSelection(prefix, suffix, placeholder string) {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		return applyWrap(text, start, end, prefix, suffix, placeholder)
	})
}

func (n *Note) prefixLines(prefix, placeholder string) {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		return applyLinePrefix(text, start, end, prefix, placeholder)
	})
}

func (n *Note) insertLink() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		return applyLink(text, start, end)
	})
}

func (n *Note) orderedList() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		return applyOrderedList(text, start, end)
	})
}

func (n *Note) toggleChecklist() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		return toggleChecklist(text, start, end)
	})
}

func (n *Note) transformBuffer(transform func(string, int, int) (string, int, int)) {
	if n.buffer == nil {
		return
	}

	start, end, ok := n.buffer.SelectionBounds()
	if !ok {
		cursor := n.buffer.IterAtMark(n.buffer.GetInsert())
		start = cursor
		end = cursor
	}

	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	updated, selStart, selEnd := transform(text, start.Offset(), end.Offset())

	n.buffer.SetText(updated)
	n.applyMarkdownFormatting()
	n.updatePreview()
	selectionStart := n.buffer.IterAtOffset(selStart)
	selectionEnd := n.buffer.IterAtOffset(selEnd)
	if selStart == selEnd {
		n.buffer.PlaceCursor(selectionStart)
	} else {
		n.buffer.SelectRange(selectionStart, selectionEnd)
	}
}

func (n *Note) insertTabSpaces() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		spaces := strings.Repeat(" ", settings.Inst().NotesApp.TabSpaces)
		return applyWrap(text, start, end, spaces, "", "")
	})
}

func (n *Note) markdownKeyController() *gtk.EventControllerKey {
	controller := gtk.NewEventControllerKey()
	controller.ConnectKeyPressed(func(keyval, _ uint, state gdk.ModifierType) bool {
		if settings.Inst().NotesApp.VimMode && state&gdk.ControlMask != 0 && (keyval == gdk.KEY_v || keyval == gdk.KEY_V) {
			n.startVisualSelection(vimSelectionBlock)
			return true
		}
		if hasControl(state) {
			return n.handleMarkdownShortcut(keyval, state)
		}

		if settings.Inst().NotesApp.VimMode {
			return n.handleVimKey(keyval)
		}

		if keyval == gdk.KEY_Tab && !hasControl(state) {
			n.insertTabSpaces()
			return true
		}
		if keyval == gdk.KEY_Return || keyval == gdk.KEY_KP_Enter {
			if n.continueMarkdownList() {
				return true
			}
		}
		return false
	})
	return controller
}

func (n *Note) handleMarkdownShortcut(keyval uint, state gdk.ModifierType) bool {
	switch keyval {
	case gdk.KEY_b, gdk.KEY_B:
		n.wrapSelection("**", "**", "bold")
	case gdk.KEY_i, gdk.KEY_I:
		n.wrapSelection("*", "*", "italic")
	case gdk.KEY_k, gdk.KEY_K:
		n.insertLink()
	case gdk.KEY_v, gdk.KEY_V:
		if n.tryPasteImageFromClipboard() {
			return true
		}
		return false
	case gdk.KEY_1:
		n.prefixLines("# ", "Heading")
	case gdk.KEY_2:
		n.prefixLines("## ", "Heading")
	case gdk.KEY_7:
		if hasShift(state) {
			n.orderedList()
			return true
		}
		return false
	case gdk.KEY_8:
		if hasShift(state) {
			n.prefixLines("- ", "item")
			return true
		}
		return false
	case gdk.KEY_9:
		if hasShift(state) {
			n.prefixLines("> ", "quote")
			return true
		}
		return false
	case gdk.KEY_0:
		if hasShift(state) {
			n.toggleChecklist()
			return true
		}
		return false
	case gdk.KEY_Return, gdk.KEY_KP_Enter:
		if hasShift(state) {
			n.toggleChecklist()
			return true
		}
		return false
	default:
		return false
	}
	return true
}

func (n *Note) handleVimKey(keyval uint) bool {
	if n.vimInsertMode {
		switch keyval {
		case gdk.KEY_Escape:
			n.vimInsertMode = false
			n.vimPendingOp = ""
			n.syncCursorMode()
			n.updateStatusBar()
			return true
		case gdk.KEY_Tab:
			n.insertTabSpaces()
			return true
		case gdk.KEY_Return, gdk.KEY_KP_Enter:
			if n.continueMarkdownList() {
				return true
			}
			return false
		default:
			return false
		}
	}

	if n.selectionMode != vimSelectionNone {
		return n.handleVisualKey(keyval)
	}

	if n.vimPendingOp != "" {
		op := n.vimPendingOp
		switch op {
		case "d", "c", "y":
			n.vimPendingOp = ""
			if keyval == gdk.KEY_d {
				n.deleteCurrentLine()
				if op == "c" {
					n.vimInsertMode = true
					n.syncCursorMode()
					n.updateStatusBar()
				}
			} else if keyval == gdk.KEY_w {
				switch op {
				case "d":
					n.deleteWord()
				case "c":
					n.changeWord()
				case "y":
					n.yankWord()
				}
			}
			return true
		}
	}

	switch keyval {
	case gdk.KEY_i:
		n.clearVisualSelection()
		n.vimInsertMode = true
		n.syncCursorMode()
		n.updateStatusBar()
	case gdk.KEY_a:
		n.clearVisualSelection()
		n.moveCursorRight()
		n.vimInsertMode = true
		n.syncCursorMode()
		n.updateStatusBar()
	case gdk.KEY_A:
		n.clearVisualSelection()
		n.moveCursorLineEnd()
		n.vimInsertMode = true
		n.syncCursorMode()
		n.updateStatusBar()
	case gdk.KEY_I:
		n.clearVisualSelection()
		n.moveCursorLineStart()
		n.vimInsertMode = true
		n.syncCursorMode()
		n.updateStatusBar()
	case gdk.KEY_o:
		n.clearVisualSelection()
		n.openLineBelow()
		n.vimInsertMode = true
		n.syncCursorMode()
		n.updateStatusBar()
	case gdk.KEY_O:
		n.clearVisualSelection()
		n.openLineAbove()
		n.vimInsertMode = true
		n.syncCursorMode()
		n.updateStatusBar()
	case gdk.KEY_h, gdk.KEY_Left:
		n.moveCursorLeft()
		n.refreshVisualSelection()
	case gdk.KEY_l, gdk.KEY_Right:
		n.moveCursorRight()
		n.refreshVisualSelection()
	case gdk.KEY_j, gdk.KEY_Down:
		n.moveCursorVertical(1)
		n.refreshVisualSelection()
	case gdk.KEY_k, gdk.KEY_Up:
		n.moveCursorVertical(-1)
		n.refreshVisualSelection()
	case gdk.KEY_0:
		n.moveCursorLineStart()
		n.refreshVisualSelection()
	case gdk.KEY_dollar:
		n.moveCursorLineEnd()
		n.refreshVisualSelection()
	case gdk.KEY_w:
		n.moveCursorWordForward()
		n.refreshVisualSelection()
	case gdk.KEY_b:
		n.moveCursorWordBackward()
		n.refreshVisualSelection()
	case gdk.KEY_x:
		n.clearVisualSelection()
		n.deleteCharUnderCursor()
	case gdk.KEY_d:
		n.clearVisualSelection()
		n.vimPendingOp = "d"
		n.updateStatusBar()
	case gdk.KEY_c:
		n.clearVisualSelection()
		n.vimPendingOp = "c"
		n.updateStatusBar()
	case gdk.KEY_v:
		n.startVisualSelection(vimSelectionChar)
	case gdk.KEY_V:
		n.startVisualSelection(vimSelectionLine)
	case gdk.KEY_y:
		n.vimPendingOp = "y"
		n.updateStatusBar()
	case gdk.KEY_p:
		n.pasteRegister()
	case gdk.KEY_colon:
		n.showCommandLine("")
	case gdk.KEY_slash:
		n.showCommandLine("/")
	case gdk.KEY_n:
		n.clearVisualSelection()
		n.repeatSearch(false)
	case gdk.KEY_N:
		n.clearVisualSelection()
		n.repeatSearch(true)
	case gdk.KEY_Escape:
		n.vimPendingOp = ""
		n.clearVisualSelection()
		n.updateStatusBar()
	default:
		n.vimPendingOp = ""
		n.updateStatusBar()
		return true
	}

	return true
}

func (n *Note) handleVisualKey(keyval uint) bool {
	switch keyval {
	case gdk.KEY_h, gdk.KEY_Left:
		n.moveCursorLeft()
		n.refreshVisualSelection()
	case gdk.KEY_l, gdk.KEY_Right:
		n.moveCursorRight()
		n.refreshVisualSelection()
	case gdk.KEY_j, gdk.KEY_Down:
		n.moveCursorVertical(1)
		n.refreshVisualSelection()
	case gdk.KEY_k, gdk.KEY_Up:
		n.moveCursorVertical(-1)
		n.refreshVisualSelection()
	case gdk.KEY_0:
		n.moveCursorLineStart()
		n.refreshVisualSelection()
	case gdk.KEY_dollar:
		n.moveCursorLineEnd()
		n.refreshVisualSelection()
	case gdk.KEY_w:
		n.moveCursorWordForward()
		n.refreshVisualSelection()
	case gdk.KEY_b:
		n.moveCursorWordBackward()
		n.refreshVisualSelection()
	case gdk.KEY_y:
		n.yankSelection()
	case gdk.KEY_d, gdk.KEY_x:
		n.deleteVisualSelection()
	case gdk.KEY_c:
		n.changeVisualSelection()
	case gdk.KEY_o:
		n.swapVisualAnchor()
	case gdk.KEY_p:
		n.replaceVisualSelectionWithRegister()
	case gdk.KEY_v:
		if n.selectionMode == vimSelectionChar {
			n.clearVisualSelection()
		} else {
			n.startVisualSelection(vimSelectionChar)
		}
	case gdk.KEY_V:
		if n.selectionMode == vimSelectionLine {
			n.clearVisualSelection()
		} else {
			n.startVisualSelection(vimSelectionLine)
		}
	case gdk.KEY_Escape:
		n.clearVisualSelection()
		n.updateStatusBar()
	default:
		return true
	}
	return true
}

func (n *Note) syncCursorMode() {
	if n.note == nil {
		return
	}
	n.note.SetOverwrite(settings.Inst().NotesApp.VimMode && !n.vimInsertMode)
}

func (n *Note) startVisualSelection(mode vimSelectionMode) {
	n.selectionMode = mode
	n.selectionMark = n.cursorOffset()
	n.selectionCursor = n.selectionMark
	n.vimPendingOp = ""
	n.applyMarkdownFormatting()
	n.updateStatusBar()
}

func (n *Note) clearVisualSelection() {
	if n.selectionMode == vimSelectionNone {
		return
	}
	n.selectionMode = vimSelectionNone
	n.selectionCursor = 0
	n.buffer.PlaceCursor(n.buffer.IterAtMark(n.buffer.GetInsert()))
	n.applyMarkdownFormatting()
}

func (n *Note) swapVisualAnchor() {
	if n.selectionMode == vimSelectionNone {
		return
	}
	n.selectionMark, n.selectionCursor = n.selectionCursor, n.selectionMark
	n.placeCursorOffset(n.selectionCursor)
	n.applyMarkdownFormatting()
	n.updateStatusBar()
}

func (n *Note) refreshVisualSelection() {
	if n.selectionMode == vimSelectionNone {
		return
	}
	n.selectionCursor = n.cursorOffset()
	n.applyMarkdownFormatting()
}

func (n *Note) applyVisualSelection() {
	if n.selectionMode == vimSelectionNone || n.buffer == nil {
		return
	}
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	switch n.selectionMode {
	case vimSelectionChar:
		start := minInt(n.selectionMark, n.selectionCursor)
		end := maxInt(n.selectionMark, n.selectionCursor) + 1
		n.buffer.ApplyTagByName(tagVisualSelection, n.buffer.IterAtOffset(start), n.buffer.IterAtOffset(end))
	case vimSelectionLine:
		start, end := vimLineRange(text, n.selectionMark, n.selectionCursor)
		n.buffer.ApplyTagByName(tagVisualSelection, n.buffer.IterAtOffset(start), n.buffer.IterAtOffset(end))
	case vimSelectionBlock:
		lines := vimLineInfos(text)
		runes := []rune(text)
		startIdx, endIdx, startCol, endCol := vimLineColumns(text, n.selectionMark, n.selectionCursor)
		for idx := startIdx; idx <= endIdx; idx++ {
			line := lines[idx]
			lineRunes := runes[line.start:line.end]
			from := startCol
			if from > len(lineRunes) {
				from = len(lineRunes)
			}
			to := endCol + 1
			if to > len(lineRunes) {
				to = len(lineRunes)
			}
			if from >= to {
				continue
			}
			n.buffer.ApplyTagByName(tagVisualSelection, n.buffer.IterAtOffset(line.start+from), n.buffer.IterAtOffset(line.start+to))
		}
	}
}

func (n *Note) updateStatusBar() {
	if settings.Inst() == nil || !settings.Inst().NotesApp.VimMode {
		return
	}
	switch {
	case n.commandMode:
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar(":")
		}
	case n.selectionMode == vimSelectionChar:
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("-- VISUAL --")
		}
	case n.selectionMode == vimSelectionLine:
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("-- VISUAL LINE --")
		}
	case n.selectionMode == vimSelectionBlock:
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("-- VISUAL BLOCK --")
		}
	case n.vimPendingOp != "":
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar(n.vimPendingOp)
		}
	case n.vimInsertMode:
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("-- INSERT --")
		}
	default:
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("-- NORMAL --")
		}
	}
}

func (n *Note) showCommandLine(prefix string) {
	if n.commandEntry == nil {
		return
	}
	n.commandMode = true
	parent := n.commandEntry.Parent()
	if parent != nil {
		gtk.BaseWidget(parent).SetVisible(true)
	}
	n.commandEntry.SetText(prefix)
	n.commandEntry.GrabFocus()
	n.commandEntry.SetPosition(-1)
	n.updateStatusBar()
}

func (n *Note) hideCommandLine(returnFocus bool) {
	if n.commandEntry == nil {
		return
	}
	n.commandMode = false
	n.commandEntry.SetText("")
	parent := n.commandEntry.Parent()
	if parent != nil {
		gtk.BaseWidget(parent).SetVisible(false)
	}
	if returnFocus && n.note != nil {
		n.note.GrabFocus()
	}
	n.updateStatusBar()
}

func (n *Note) executeCommandLine() {
	if n.commandEntry == nil {
		return
	}
	commandText := n.commandEntry.Text()
	cmd, err := parseVimCommand(commandText)
	if err != nil {
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("Vim command error: " + err.Error())
		}
		n.hideCommandLine(true)
		return
	}
	if err := n.runVimCommand(cmd); err != nil {
		if helpers.HasStatusBar() {
			helpers.StatusBarInst().UpdateStatusBar("Vim command error: " + err.Error())
		}
		n.hideCommandLine(true)
		return
	}
	n.hideCommandLine(true)
}

func (n *Note) runVimCommand(cmd vimCommand) error {
	switch cmd.Kind {
	case vimCommandSave:
		n.statusMessage("Saving notes...")
		go n.saveCommand()
		return nil
	case vimCommandSearch:
		return n.searchText(cmd.Query)
	case vimCommandReplace:
		return n.replaceText(cmd.Query, cmd.Replacement, cmd.Global, cmd.CurrentLine)
	default:
		return fmt.Errorf("unsupported command")
	}
}

func (n *Note) searchText(query string) error {
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	start := n.cursorOffset()
	if n.lastSearch == query {
		start++
	}
	index := findNext(text, query, start)
	if index < 0 && start > 0 {
		index = findNext(text, query, 0)
	}
	if index < 0 {
		return fmt.Errorf("pattern not found: %s", query)
	}
	n.lastSearch = query
	n.lastSearchPos = index
	return n.selectSearchResult(query, index)
}

func (n *Note) selectSearchResult(query string, index int) error {
	if index < 0 {
		return fmt.Errorf("pattern not found: %s", query)
	}
	startIter := n.buffer.IterAtOffset(index)
	endIter := n.buffer.IterAtOffset(index + len([]rune(query)))
	n.buffer.SelectRange(startIter, endIter)
	n.note.ScrollToIter(startIter, 0.2, false, 0, 0)
	if helpers.HasStatusBar() {
		n.statusMessage("Found: " + query)
	}
	return nil
}

func (n *Note) repeatSearch(reverse bool) {
	if n.lastSearch == "" {
		n.statusMessage("No active search")
		return
	}
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	index := -1
	if reverse {
		index = findPrevious(text, n.lastSearch, n.lastSearchPos-1)
		if index < 0 {
			index = findPrevious(text, n.lastSearch, len([]rune(text))-1)
		}
	} else {
		index = findNext(text, n.lastSearch, n.lastSearchPos+1)
		if index < 0 {
			index = findNext(text, n.lastSearch, 0)
		}
	}
	if index < 0 {
		n.statusMessage("Pattern not found: " + n.lastSearch)
		return
	}
	n.lastSearchPos = index
	_ = n.selectSearchResult(n.lastSearch, index)
}

func (n *Note) replaceText(oldValue string, newValue string, global bool, currentLine bool) error {
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	updated := text
	count := 0
	if currentLine {
		start := vimLineBoundaryOffset(text, n.cursorOffset(), false)
		end := vimLineBoundaryOffset(text, n.cursorOffset(), true)
		updated, count = replaceTextInRange(text, oldValue, newValue, global, start, end)
	} else {
		updated, count = replaceText(text, oldValue, newValue, global)
	}
	if count == 0 {
		return fmt.Errorf("pattern not found: %s", oldValue)
	}
	n.buffer.SetText(updated)
	n.applyMarkdownFormatting()
	n.updatePreview()
	n.statusMessage(fmt.Sprintf("Replaced %d occurrence(s)", count))
	return nil
}

func (n *Note) statusMessage(text string) {
	if !helpers.HasStatusBar() {
		return
	}
	glib.IdleAdd(func() {
		helpers.StatusBarInst().UpdateStatusBar(text)
	})
}

func (n *Note) yankSelection() {
	if n.selectionMode == vimSelectionNone {
		return
	}
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	switch n.selectionMode {
	case vimSelectionChar:
		n.yankRegister = vimYankChar(text, n.selectionMark, n.selectionCursor)
		n.copyRegisterToClipboard()
		n.statusMessage("Yanked selection")
	case vimSelectionLine:
		n.yankRegister = vimYankLine(text, n.selectionMark, n.selectionCursor)
		n.copyRegisterToClipboard()
		n.statusMessage("Yanked lines")
	case vimSelectionBlock:
		n.yankRegister = vimYankBlock(text, n.selectionMark, n.selectionCursor)
		n.copyRegisterToClipboard()
		n.statusMessage("Yanked block")
	}
	n.clearVisualSelection()
	n.updateStatusBar()
}

func (n *Note) yankWord() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		n.yankRegister = vimYankWord(text, start)
		n.copyRegisterToClipboard()
		n.statusMessage("Yanked word")
		return text, start, end
	})
	n.updateStatusBar()
}

func (n *Note) pasteRegister() {
	if n.yankRegister.Kind == "" {
		n.pasteFromClipboard()
		return
	}
	n.clearVisualSelection()
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		switch n.yankRegister.Kind {
		case vimRegisterChar:
			updated, cursor := vimPasteChar(text, start, n.yankRegister)
			return updated, cursor, cursor
		case vimRegisterLine:
			updated, cursor := vimPasteLine(text, start, n.yankRegister)
			return updated, cursor, cursor
		case vimRegisterBlock:
			updated, cursor := vimPasteBlock(text, start, n.yankRegister)
			return updated, cursor, cursor
		default:
			return text, start, end
		}
	})
}

func (n *Note) deleteVisualSelection() {
	if n.selectionMode == vimSelectionNone {
		return
	}
	mode := n.selectionMode
	mark := n.selectionMark
	cursor := n.selectionCursor
	n.clearVisualSelection()
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		switch mode {
		case vimSelectionChar:
			updated, pos := vimDeleteRange(text, mark, cursor)
			return updated, pos, pos
		case vimSelectionLine:
			lineStart, lineEnd := vimLineRange(text, mark, cursor)
			runes := []rune(text)
			updated := string(append(runes[:lineStart], runes[lineEnd:]...))
			return updated, lineStart, lineStart
		case vimSelectionBlock:
			updated, pos := vimDeleteBlock(text, mark, cursor)
			return updated, pos, pos
		default:
			return text, start, end
		}
	})
}

func (n *Note) changeVisualSelection() {
	if n.selectionMode == vimSelectionNone {
		return
	}
	n.deleteVisualSelection()
	n.vimInsertMode = true
	n.syncCursorMode()
	n.updateStatusBar()
}

func (n *Note) replaceVisualSelectionWithRegister() {
	if n.selectionMode == vimSelectionNone {
		n.pasteRegister()
		return
	}
	mode := n.selectionMode
	mark := n.selectionMark
	cursor := n.selectionCursor
	n.clearVisualSelection()
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		var updated string
		var pos int
		switch mode {
		case vimSelectionChar:
			updated, pos = vimDeleteRange(text, mark, cursor)
		case vimSelectionLine:
			lineStart, lineEnd := vimLineRange(text, mark, cursor)
			runes := []rune(text)
			updated = string(append(runes[:lineStart], runes[lineEnd:]...))
			pos = lineStart
		case vimSelectionBlock:
			updated, pos = vimDeleteBlock(text, mark, cursor)
		default:
			return text, start, end
		}
		switch n.yankRegister.Kind {
		case vimRegisterChar:
			updated, pos = vimPasteChar(updated, pos, n.yankRegister)
		case vimRegisterLine:
			updated, pos = vimPasteLine(updated, pos, n.yankRegister)
		case vimRegisterBlock:
			updated, pos = vimPasteBlock(updated, pos, n.yankRegister)
		}
		return updated, pos, pos
	})
}

func (n *Note) copyRegisterToClipboard() {
	if n.note == nil {
		return
	}
	clipboard := gtk.BaseWidget(n.note).Clipboard()
	if clipboard == nil {
		return
	}
	switch n.yankRegister.Kind {
	case vimRegisterBlock:
		clipboard.SetText(strings.Join(n.yankRegister.Lines, "\n"))
	default:
		clipboard.SetText(n.yankRegister.Text)
	}
}

func (n *Note) pasteFromClipboard() {
	if n.note == nil {
		return
	}
	clipboard := gtk.BaseWidget(n.note).Clipboard()
	if clipboard == nil {
		return
	}
	clipboard.ReadTextAsync(context.Background(), func(result gio.AsyncResulter) {
		text, err := clipboard.ReadTextFinish(result)
		if err != nil || text == "" {
			return
		}
		glib.IdleAdd(func() {
			n.transformBuffer(func(current string, start, end int) (string, int, int) {
				updated, cursor := vimPasteChar(current, start, vimRegister{Kind: vimRegisterChar, Text: text})
				return updated, cursor, cursor
			})
		})
	})
}

func (n *Note) cursorOffset() int {
	if n.buffer == nil {
		return 0
	}
	return n.buffer.IterAtMark(n.buffer.GetInsert()).Offset()
}

func (n *Note) placeCursorOffset(offset int) {
	if n.buffer == nil {
		return
	}
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	n.buffer.PlaceCursor(n.buffer.IterAtOffset(vimClampOffset(text, offset)))
}

func (n *Note) moveCursorLeft() {
	iter := n.buffer.IterAtMark(n.buffer.GetInsert())
	iter.BackwardCursorPosition()
	n.buffer.PlaceCursor(iter)
}

func (n *Note) moveCursorRight() {
	iter := n.buffer.IterAtMark(n.buffer.GetInsert())
	iter.ForwardCursorPosition()
	n.buffer.PlaceCursor(iter)
}

func (n *Note) moveCursorVertical(delta int) {
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	n.placeCursorOffset(vimVerticalMoveOffset(text, n.cursorOffset(), delta))
}

func (n *Note) moveCursorLineStart() {
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	n.placeCursorOffset(vimLineBoundaryOffset(text, n.cursorOffset(), false))
}

func (n *Note) moveCursorLineEnd() {
	text := n.buffer.Text(n.buffer.StartIter(), n.buffer.EndIter(), true)
	n.placeCursorOffset(vimLineBoundaryOffset(text, n.cursorOffset(), true))
}

func (n *Note) moveCursorWordForward() {
	iter := n.buffer.IterAtMark(n.buffer.GetInsert())
	iter.ForwardWordEnd()
	n.buffer.PlaceCursor(iter)
}

func (n *Note) moveCursorWordBackward() {
	iter := n.buffer.IterAtMark(n.buffer.GetInsert())
	iter.BackwardWordStart()
	n.buffer.PlaceCursor(iter)
}

func (n *Note) deleteCharUnderCursor() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor := vimDeleteChar(text, start)
		return updated, cursor, cursor
	})
}

func (n *Note) deleteCurrentLine() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor := vimDeleteLine(text, start)
		return updated, cursor, cursor
	})
}

func (n *Note) deleteWord() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor := vimDeleteWord(text, start)
		return updated, cursor, cursor
	})
}

func (n *Note) changeWord() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor := vimDeleteWord(text, start)
		return updated, cursor, cursor
	})
	n.vimInsertMode = true
	n.syncCursorMode()
	n.updateStatusBar()
}

func (n *Note) openLineBelow() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor := vimOpenLineBelow(text, start)
		return updated, cursor, cursor
	})
}

func (n *Note) openLineAbove() {
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor := vimOpenLineAbove(text, start)
		return updated, cursor, cursor
	})
}

func hasControl(state gdk.ModifierType) bool {
	return state&gdk.ControlMask != 0 || state&gdk.MetaMask != 0 || state&gdk.SuperMask != 0
}

func hasShift(state gdk.ModifierType) bool {
	return state&gdk.ShiftMask != 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (n *Note) continueMarkdownList() bool {
	if n.buffer == nil {
		return false
	}
	if _, _, ok := n.buffer.SelectionBounds(); ok {
		return false
	}
	handled := false
	n.transformBuffer(func(text string, start, end int) (string, int, int) {
		updated, cursor, ok := continueMarkdownList(text, start)
		handled = ok
		if !ok {
			return text, start, end
		}
		return updated, cursor, cursor
	})
	return handled
}

func continueMarkdownList(text string, cursor int) (string, int, bool) {
	runes := []rune(text)
	if cursor < 0 || cursor > len(runes) || insideMarkdownCodeFence(runes, cursor) {
		return text, cursor, false
	}

	lineStart := cursor
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	lineEnd := cursor
	for lineEnd < len(runes) && runes[lineEnd] != '\n' {
		lineEnd++
	}

	line := string(runes[lineStart:lineEnd])
	prefix, empty, ok := nextMarkdownListPrefix(line)
	if !ok {
		return text, cursor, false
	}
	if empty {
		updated := string(runes[:lineStart]) + string(runes[lineEnd:])
		return updated, lineStart, true
	}

	insert := "\n" + prefix
	updated := string(runes[:cursor]) + insert + string(runes[cursor:])
	return updated, cursor + len([]rune(insert)), true
}

func nextMarkdownListPrefix(line string) (string, bool, bool) {
	indentLen := 0
	for indentLen < len(line) && (line[indentLen] == ' ' || line[indentLen] == '\t') {
		indentLen++
	}
	indent := line[:indentLen]
	trimmed := line[indentLen:]

	switch {
	case strings.HasPrefix(trimmed, "- [ ] "):
		return indent + "- [ ] ", strings.TrimSpace(trimmed[6:]) == "", true
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		return indent + "- [ ] ", strings.TrimSpace(trimmed[6:]) == "", true
	case strings.HasPrefix(trimmed, "- "):
		return indent + "- ", strings.TrimSpace(trimmed[2:]) == "", true
	case strings.HasPrefix(trimmed, "* "):
		return indent + "* ", strings.TrimSpace(trimmed[2:]) == "", true
	}

	number := 0
	markerEnd := 0
	for markerEnd < len(trimmed) && trimmed[markerEnd] >= '0' && trimmed[markerEnd] <= '9' {
		number = number*10 + int(trimmed[markerEnd]-'0')
		markerEnd++
	}
	if markerEnd == 0 || markerEnd+1 >= len(trimmed) {
		return "", false, false
	}
	separator := trimmed[markerEnd]
	if (separator != '.' && separator != ')') || trimmed[markerEnd+1] != ' ' {
		return "", false, false
	}
	return fmt.Sprintf("%s%d%c ", indent, number+1, separator), strings.TrimSpace(trimmed[markerEnd+2:]) == "", true
}

func insideMarkdownCodeFence(runes []rune, cursor int) bool {
	inFence := false
	lineStart := 0
	for idx := 0; idx <= len(runes); idx++ {
		if idx < len(runes) && runes[idx] != '\n' {
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(string(runes[lineStart:idx]), " \t"), "```") {
			inFence = !inFence
		}
		if idx >= cursor {
			return inFence
		}
		lineStart = idx + 1
	}
	return inFence
}

func (n *Note) togglePreview() {
	if n.previewToggle == nil {
		return
	}
	n.previewToggle.SetActive(!n.previewToggle.Active())
}

func (n *Note) setPreviewVisible(visible bool) {
	if n.paned == nil {
		return
	}
	child := n.paned.EndChild()
	if child == nil {
		return
	}
	if !visible {
		if width, ok := editorWidthForPersistence(n.paned); ok {
			persistEditorWidth(width)
		}
	}
	gtk.BaseWidget(child).SetVisible(visible)
	if visible {
		applyEditorWidth(n.activeTab())
	} else {
		n.paned.SetPosition(9999)
	}
}

func (n *Note) previewClickController() *gtk.GestureClick {
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(_ int, x, y float64) {
		if uri := n.previewLinkAt(int(x), int(y)); uri != "" {
			helpers.OpenURI(uri)
		}
	})
	return click
}

func (n *Note) previewMotionController() *gtk.EventControllerMotion {
	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) {
		if n.previewLinkAt(int(x), int(y)) != "" {
			n.preview.SetCursorFromName("pointer")
			return
		}
		n.preview.SetCursorFromName("text")
	})
	motion.ConnectLeave(func() {
		n.preview.SetCursorFromName("text")
	})
	return motion
}

func (n *Note) previewLinkAt(x, y int) string {
	if n.preview == nil {
		return ""
	}
	bufferX, bufferY := n.preview.WindowToBufferCoords(gtk.TextWindowWidget, x, y)
	iter, ok := n.preview.IterAtLocation(bufferX, bufferY)
	if !ok {
		return ""
	}
	offset := iter.Offset()
	for _, link := range n.previewLinks {
		if offset >= link.Start && offset <= link.End {
			return link.URL
		}
	}
	return ""
}

func (n *Note) autoSave() {
	n.autoSaveForTab(n.activeTab())
}

func (n *Note) save() {
	n.saveSpecificTab(n.activeTab(), true)
}

func (n *Note) saveSpecificTab(tab *noteTab, sync bool) {
	if tab == nil || tab.buffer == nil {
		return
	}
	file := tab.path
	text := tab.buffer.Text(tab.buffer.StartIter(), tab.buffer.EndIter(), true)
	go n.persistTabSnapshot(tab, file, text, sync)
}

func (n *Note) flushAllTabs(sync bool) error {
	if n == nil {
		return nil
	}

	var wroteAny bool
	for _, tab := range n.tabs {
		if tab == nil || tab.buffer == nil {
			continue
		}
		file := tab.path
		text := tab.buffer.Text(tab.buffer.StartIter(), tab.buffer.EndIter(), true)
		if err := n.writeTabSnapshot(tab, file, text); err != nil {
			return err
		}
		wroteAny = true
	}

	if wroteAny {
		noteMarkDriveDirty()
	}
	if sync && wroteAny {
		noteStartDriveSync()
	}
	return nil
}

func (n *Note) persistTabSnapshot(tab *noteTab, file string, text string, sync bool) {
	if err := n.writeTabSnapshot(tab, file, text); err != nil {
		log.Println(err)
		return
	}
	count := saveCounter.Add(1)
	noteMarkDriveDirty()
	if sync {
		noteStartDriveSync()
	}
	n.statusMessage("Notes saved to: " + file + ", " + fmt.Sprint(count))
	tab.waitingToSave = false
	if n.activeTab() == tab {
		n.WaitingToSave = false
	}
}

func (n *Note) writeTabSnapshot(tab *noteTab, file string, text string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		tab.waitingToSave = false
		if n.activeTab() == tab {
			n.WaitingToSave = false
		}
		return fmt.Errorf("notes dir error: %w", err)
	}
	if helpers.Globals().Debug {
		log.Println(text)
	}
	if err := noteWriteFile(file, []byte(text), 0o644); err != nil {
		tab.waitingToSave = false
		if n.activeTab() == tab {
			n.WaitingToSave = false
		}
		return err
	}
	tab.waitingToSave = false
	if n.activeTab() == tab {
		n.WaitingToSave = false
	}
	return nil
}
