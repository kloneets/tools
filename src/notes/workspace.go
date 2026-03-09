package notes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	glib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
	"github.com/kloneets/tools/src/ui"
)

type noteTab struct {
	title           string
	path            string
	tabLabel        *gtk.Label
	root            *gtk.Box
	note            *gtk.TextView
	buffer          *gtk.TextBuffer
	preview         *gtk.TextView
	previewBuffer   *gtk.TextBuffer
	previewLinks    []markdownLink
	paned           *gtk.Paned
	previewToggle   *gtk.ToggleButton
	commandEntry    *gtk.Entry
	vimInsertMode   bool
	vimPendingOp    string
	selectionMode   vimSelectionMode
	selectionMark   int
	selectionCursor int
	yankRegister    vimRegister
	commandMode     bool
	lastSearch      string
	lastSearchPos   int
	waitingToSave   bool
	widthRestored   bool
	applyingWidth   bool
	widthPersistOK  bool
	widthApplySeq   int64
}

type noteFile struct {
	Title   string
	Path    string
	Folder  string
	RelDir  string
	RelPath string
}

type sidebarEntryKind int

const (
	sidebarEntryFolder sidebarEntryKind = iota
	sidebarEntryNote
	sidebarEntryPlaceholder
)

type sidebarEntry struct {
	Kind   sidebarEntryKind
	Path   string
	Label  string
	Depth  int
	Folder string
}

type sidebarDropTarget struct {
	Folder string
	Anchor string
	Root   bool
}

func (n *Note) buildWorkspace() {
	n.notebook = gtk.NewNotebook()
	n.notebook.SetHExpand(true)
	n.notebook.SetVExpand(true)
	n.notebook.SetScrollable(true)
	n.notebook.ConnectSwitchPage(func(_ gtk.Widgetter, pageNum uint) {
		n.switchToTab(int(pageNum))
	})

	n.sidebarToggle = ui.IconToggleButton("sidebar-show-symbolic", "Show or hide notes sidebar")
	n.sidebarToggle.SetActive(settings.Inst().NotesApp.SidebarVisible)
	n.sidebarToggle.ConnectToggled(func() {
		n.setSidebarVisible(n.sidebarToggle.Active())
		settings.Inst().NotesApp.SidebarVisible = n.sidebarToggle.Active()
		settings.SaveSettings()
	})

	n.sidebarDragToggle = ui.IconToggleButton("arrows-up-down", "Toggle drag mode")
	n.sidebarDragToggle.ConnectToggled(func() {
		n.cancelPendingSidebarDrag()
		if !n.sidebarDragToggle.Active() {
			n.sidebarDragActive = false
			n.sidebarDragSource = sidebarEntry{}
			n.sidebarDragTarget = sidebarDropTarget{}
			n.sidebar.DragUnhighlightRow()
			n.updateSidebarDropHint(sidebarDropTarget{})
		}
		n.sidebar.SetActivateOnSingleClick(!n.sidebarDragToggle.Active())
	})

	n.sidebar = gtk.NewListBox()
	n.sidebar.SetSelectionMode(gtk.SelectionBrowse)
	n.sidebar.SetActivateOnSingleClick(true)
	n.sidebar.SetVExpand(true)
	n.sidebar.SetSizeRequest(180, -1)
	n.sidebar.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		n.activateSidebarPath(row)
	})
	n.sidebar.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		n.handleSidebarSelectionChanged(row)
	})
	n.setupSidebarInteractions()

	n.sidebarScroll = gtk.NewScrolledWindow()
	n.sidebarScroll.SetHExpand(false)
	n.sidebarScroll.SetVExpand(true)
	n.sidebarScroll.SetMinContentWidth(180)
	n.sidebarScroll.SetChild(n.sidebar)

	n.sidebarDropHint = gtk.NewLabel("")
	n.sidebarDropHint.SetHAlign(gtk.AlignStart)
	n.sidebarDropHint.SetMarginStart(ui.DefaultBoxPadding)
	n.sidebarDropHint.SetMarginEnd(ui.DefaultBoxPadding)
	n.sidebarDropHint.SetVisible(false)
	n.sidebarDropHint.AddCSSClass("dim-label")

	n.sidebarActions = gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	n.sidebarActions.SetMarginStart(ui.DefaultBoxPadding)
	n.sidebarActions.SetMarginEnd(ui.DefaultBoxPadding)
	n.sidebarActions.SetMarginBottom(ui.DefaultBoxPadding)

	n.sidebarNewNote = workspaceIconButton("list-add-symbolic", "New note")
	n.sidebarNewNote.ConnectClicked(func() {
		n.showSidebarCreatePopover(n.sidebarNewNote, sidebarEntryNote)
	})
	n.sidebarActions.Append(n.sidebarToggle)
	n.sidebarActions.Append(n.sidebarDragToggle)
	n.sidebarActions.Append(n.sidebarNewNote)

	n.sidebarNewFolder = workspaceIconButton("folder-new-symbolic", "New folder")
	n.sidebarNewFolder.ConnectClicked(func() {
		n.showSidebarCreatePopover(n.sidebarNewFolder, sidebarEntryFolder)
	})
	n.sidebarActions.Append(n.sidebarNewFolder)

	n.sidebarContent = gtk.NewBox(gtk.OrientationVertical, ui.DefaultBoxPadding)
	n.sidebarContent.SetVExpand(true)
	n.sidebarContent.Append(n.sidebarScroll)
	n.sidebarContent.Append(n.sidebarDropHint)

	n.sidebarBox = gtk.NewBox(gtk.OrientationVertical, ui.DefaultBoxPadding)
	n.sidebarBox.SetVExpand(true)
	n.sidebarBox.Append(n.sidebarContent)

	mainArea := ui.MainArea()
	mainArea.SetHExpand(true)
	mainArea.SetVExpand(true)
	n.sidebarPane = gtk.NewPaned(gtk.OrientationHorizontal)
	n.sidebarPane.SetHExpand(true)
	n.sidebarPane.SetVExpand(true)
	n.sidebarPane.SetPosition(260)
	n.sidebarPane.SetResizeStartChild(false)
	n.sidebarPane.SetResizeEndChild(true)
	n.sidebarPane.SetShrinkStartChild(false)
	n.sidebarPane.SetShrinkEndChild(false)
	n.sidebarPane.SetStartChild(n.sidebarBox)
	n.sidebarPane.SetEndChild(n.notebook)
	mainArea.Append(n.sidebarPane)
	mainArea.Append(n.sidebarActions)

	n.F = ui.Frame("Notes:")
	n.F.SetHExpand(true)
	n.F.SetVExpand(true)
	n.F.SetChild(mainArea)
	n.applySidebarVisibility(settings.Inst().NotesApp.SidebarVisible)
}

func (n *Note) openInitialTabs() {
	files, err := ensureInitialNoteFiles()
	if err != nil {
		log.Println("notes init error:", err)
	}
	if len(files) == 0 {
		files = []noteFile{{Title: noteTitleFromPath(fileName()), Path: fileName()}}
	}
	n.openNotePath(files[0].Path)
	n.refreshSidebar()
}

func (n *Note) createTab(path string, text string) *noteTab {
	tab := &noteTab{
		title: noteTitleFromPath(path),
		path:  path,
	}

	tab.buffer = styledBuffer(false)
	tab.buffer.SetText(text)
	tab.note = gtk.NewTextView()
	tab.note.SetBuffer(tab.buffer)
	gtk.BaseWidget(tab.note).SetName("notes-editor")
	tab.note.SetHExpand(true)
	tab.note.SetVExpand(true)
	tab.note.SetWrapMode(gtk.WrapWord)
	tab.note.SetSizeRequest(300, 200)
	tab.note.SetMarginStart(ui.DefaultBoxPadding)
	tab.note.SetMarginEnd(ui.DefaultBoxPadding)
	tab.note.SetMarginTop(ui.DefaultBoxPadding)
	tab.note.SetMarginBottom(ui.DefaultBoxPadding)
	tab.note.AddController(n.markdownKeyController())

	tab.previewBuffer = styledBuffer(true)
	tab.preview = gtk.NewTextView()
	tab.preview.SetBuffer(tab.previewBuffer)
	gtk.BaseWidget(tab.preview).SetName("notes-preview")
	tab.preview.SetEditable(false)
	tab.preview.SetCursorVisible(false)
	tab.preview.SetWrapMode(gtk.WrapWord)
	tab.preview.SetMarginStart(ui.DefaultBoxPadding)
	tab.preview.SetMarginEnd(ui.DefaultBoxPadding)
	tab.preview.SetMarginTop(ui.DefaultBoxPadding)
	tab.preview.SetMarginBottom(ui.DefaultBoxPadding)
	tab.preview.AddController(n.previewClickController())
	tab.preview.AddController(n.previewMotionController())

	editorScroll := noteScrolledWindow(tab.note)
	previewScroll := noteScrolledWindow(tab.preview)

	tab.paned = gtk.NewPaned(gtk.OrientationHorizontal)
	tab.paned.SetHExpand(true)
	tab.paned.SetVExpand(true)
	tab.paned.SetResizeStartChild(true)
	tab.paned.SetResizeEndChild(true)
	glib.BaseObject(tab.paned).NotifyProperty("position", func() {
		if width, ok := editorWidthForPersistence(tab.paned); ok && shouldPersistEditorWidth(tab, width) {
			persistEditorWidth(width)
		}
	})
	gtk.BaseWidget(tab.paned).ConnectMap(func() {
		scheduleApplyEditorWidth(tab)
	})
	tab.paned.SetStartChild(editorScroll)
	tab.paned.SetEndChild(previewScroll)

	tab.commandEntry = gtk.NewEntry()
	tab.commandEntry.SetHExpand(true)
	tab.commandEntry.SetPlaceholderText("w, /pattern, search text, s/old/new/, %s/old/new/g")
	tab.commandEntry.ConnectActivate(func() {
		n.executeCommandLine()
	})
	commandKeyController := gtk.NewEventControllerKey()
	commandKeyController.ConnectKeyPressed(func(keyval, _ uint, _ gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			n.hideCommandLine(true)
			return true
		}
		return false
	})
	tab.commandEntry.AddController(commandKeyController)

	commandRow := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	commandRow.SetMarginStart(ui.DefaultBoxPadding)
	commandRow.SetMarginEnd(ui.DefaultBoxPadding)
	commandRow.Append(ui.SectionLabel(":"))
	commandRow.Append(tab.commandEntry)
	gtk.BaseWidget(commandRow).SetVisible(false)

	toolbar := n.markdownToolbarForTab(tab)
	tab.root = ui.MainArea()
	tab.root.SetHExpand(true)
	tab.root.SetVExpand(true)
	tab.root.Append(toolbar)
	tab.root.Append(tab.paned)
	tab.root.Append(commandRow)

	tab.buffer.ConnectChanged(func() {
		if n.activeTab() != tab {
			return
		}
		n.applyMarkdownFormatting()
		n.updatePreview()
		n.autoSaveForTab(tab)
	})

	n.loadTab(tab)
	n.refreshEditorMode()
	n.applyMarkdownFormatting()
	n.updatePreview()
	return tab
}

func (n *Note) markdownToolbarForTab(tab *noteTab) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	row.SetMarginStart(ui.DefaultBoxPadding)
	row.SetMarginEnd(ui.DefaultBoxPadding)
	row.SetMarginTop(ui.DefaultBoxPadding)

	for _, action := range []struct {
		icon    string
		glyph   string
		tooltip string
		fn      func()
	}{
		{icon: "document-preview-symbolic", tooltip: "Toggle preview", fn: func() { n.togglePreview() }},
		{icon: "format-text-bold-symbolic", tooltip: "Bold", fn: func() { n.wrapSelection("**", "**", "bold") }},
		{icon: "format-text-italic-symbolic", tooltip: "Italic", fn: func() { n.wrapSelection("*", "*", "italic") }},
		{icon: "text-x-script-symbolic", tooltip: "Inline code", fn: func() { n.wrapSelection("`", "`", "code") }},
		{glyph: "H1", tooltip: "Heading 1", fn: func() { n.prefixLines("# ", "Heading") }},
		{glyph: "H2", tooltip: "Heading 2", fn: func() { n.prefixLines("## ", "Heading") }},
		{glyph: "H3", tooltip: "Heading 3", fn: func() { n.prefixLines("### ", "Heading") }},
		{glyph: "•", tooltip: "Bullet list", fn: func() { n.prefixLines("- ", "item") }},
		{glyph: "1.", tooltip: "Ordered list", fn: func() { n.orderedList() }},
		{icon: "checkbox-symbolic", tooltip: "Checklist", fn: func() { n.toggleChecklist() }},
		{icon: "insert-link-symbolic", tooltip: "Insert link", fn: func() { n.insertLink() }},
		{icon: "format-justify-left-symbolic", tooltip: "Quote", fn: func() { n.prefixLines("> ", "quote") }},
	} {
		if action.icon == "document-preview-symbolic" {
			toggle := ui.IconToggleButton(action.icon, action.tooltip)
			toggle.SetActive(true)
			toggle.ConnectToggled(func() {
				n.setPreviewVisible(toggle.Active())
			})
			tab.previewToggle = toggle
			row.Append(toggle)
			continue
		}
		button := workspaceToolbarButton(action.icon, action.glyph, action.tooltip)
		button.ConnectClicked(action.fn)
		row.Append(button)
	}
	return row
}

func (n *Note) openNotePath(path string) {
	if page := n.pageForPath(path); page >= 0 {
		n.notebook.SetCurrentPage(page)
		n.switchToTab(page)
		return
	}

	text, err := readNoteFile(path)
	if err != nil {
		log.Println("note open error:", err)
	}
	tab := n.createTab(path, text)
	n.tabs = append(n.tabs, tab)

	page := n.notebook.AppendPage(tab.root, n.tabLabel(tab))
	n.notebook.SetTabReorderable(tab.root, true)
	n.notebook.SetTabDetachable(tab.root, false)
	n.notebook.SetCurrentPage(page)
	n.switchToTab(page)
	scheduleApplyEditorWidth(tab)
	n.syncNotebookVisibility()
	n.refreshSidebar()
}

func (n *Note) newNote() {
	path := nextNotePathInFolder(n.selectedFolder)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Println("note dir error:", err)
		return
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		log.Println("note create error:", err)
		return
	}
	n.openNotePath(path)
}

func (n *Note) closeCurrentTab() {
	idx := n.currentTab
	if idx < 0 || idx >= len(n.tabs) {
		return
	}
	if tab := n.activeTab(); tab != nil {
		n.saveSpecificTab(tab, false)
	}
	n.notebook.RemovePage(idx)
	n.tabs = append(n.tabs[:idx], n.tabs[idx+1:]...)
	if len(n.tabs) == 0 {
		n.currentTab = -1
		n.clearActivePointers()
		n.syncNotebookVisibility()
		n.refreshSidebar()
		return
	}
	if idx >= len(n.tabs) {
		idx = len(n.tabs) - 1
	}
	n.notebook.SetCurrentPage(idx)
	n.switchToTab(idx)
	n.syncNotebookVisibility()
	n.refreshSidebar()
}

func (n *Note) switchToTab(idx int) {
	if idx < 0 || idx >= len(n.tabs) {
		return
	}
	if n.currentTab >= 0 && n.currentTab < len(n.tabs) {
		n.persistActiveTabState()
	}
	n.currentTab = idx
	n.loadTab(n.tabs[idx])
	n.selectedFolder = relativeNoteFolder(n.tabs[idx].path)
	n.selectSidebarPath(n.tabs[idx].path)
	n.refreshFromSettings()
	scheduleApplyEditorWidth(n.tabs[idx])
}

func (n *Note) activeTab() *noteTab {
	if n.currentTab < 0 || n.currentTab >= len(n.tabs) {
		return nil
	}
	return n.tabs[n.currentTab]
}

func (n *Note) persistActiveTabState() {
	tab := n.activeTab()
	if tab == nil {
		return
	}
	tab.vimInsertMode = n.vimInsertMode
	tab.vimPendingOp = n.vimPendingOp
	tab.selectionMode = n.selectionMode
	tab.selectionMark = n.selectionMark
	tab.selectionCursor = n.selectionCursor
	tab.yankRegister = n.yankRegister
	tab.commandMode = n.commandMode
	tab.lastSearch = n.lastSearch
	tab.lastSearchPos = n.lastSearchPos
	tab.waitingToSave = n.WaitingToSave
	tab.previewLinks = slices.Clone(n.previewLinks)
}

func (n *Note) loadTab(tab *noteTab) {
	n.note = tab.note
	n.buffer = tab.buffer
	n.preview = tab.preview
	n.previewBuffer = tab.previewBuffer
	n.previewLinks = slices.Clone(tab.previewLinks)
	n.paned = tab.paned
	n.previewToggle = tab.previewToggle
	n.commandEntry = tab.commandEntry
	n.vimInsertMode = tab.vimInsertMode
	n.vimPendingOp = tab.vimPendingOp
	n.selectionMode = tab.selectionMode
	n.selectionMark = tab.selectionMark
	n.selectionCursor = tab.selectionCursor
	n.yankRegister = tab.yankRegister
	n.commandMode = tab.commandMode
	n.lastSearch = tab.lastSearch
	n.lastSearchPos = tab.lastSearchPos
	n.WaitingToSave = tab.waitingToSave
}

func (n *Note) clearActivePointers() {
	n.note = nil
	n.buffer = nil
	n.preview = nil
	n.previewBuffer = nil
	n.previewLinks = nil
	n.paned = nil
	n.previewToggle = nil
	n.commandEntry = nil
	n.vimPendingOp = ""
	n.selectionMode = vimSelectionNone
	n.yankRegister = vimRegister{}
	n.commandMode = false
	n.lastSearch = ""
	n.lastSearchPos = 0
	n.WaitingToSave = false
}

func (n *Note) tabLabel(tab *noteTab) *gtk.Box {
	label := gtk.NewLabel(tab.title)
	tab.tabLabel = label
	closeButton := workspaceIconButton("window-close-symbolic", "Close note tab")
	closeButton.SetCanFocus(false)
	closeButton.AddCSSClass("flat")
	closeButton.ConnectClicked(func() {
		page := n.pageForPath(tab.path)
		if page < 0 {
			return
		}
		n.notebook.SetCurrentPage(page)
		n.switchToTab(page)
		n.closeCurrentTab()
	})
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)
	row.Append(label)
	row.Append(closeButton)
	return row
}

func (n *Note) pageForPath(path string) int {
	for idx, tab := range n.tabs {
		if tab.path == path {
			return idx
		}
	}
	return -1
}

func (n *Note) setupSidebarInteractions() {
	if n.sidebar == nil {
		return
	}

	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(_ float64, y float64) {
		n.updateSidebarDragTarget(int(y))
	})
	motion.ConnectLeave(func() {
		if n.sidebarDragActive {
			n.setSidebarDragTarget(sidebarDropTarget{Root: true})
		}
	})
	n.sidebar.AddController(motion)

	release := gtk.NewGestureClick()
	release.SetButton(gdk.BUTTON_PRIMARY)
	release.ConnectPressed(func(_ int, _ float64, y float64) {
		if n.sidebarDragToggle == nil || !n.sidebarDragToggle.Active() {
			return
		}
		row := n.sidebar.RowAtY(int(y))
		if row == nil {
			return
		}
		entry, ok := n.sidebarEntryForRow(row)
		if !ok {
			return
		}
		n.sidebar.SelectRow(row)
		n.queueSidebarDrag(entry)
	})
	release.ConnectReleased(func(_ int, _ float64, y float64) {
		n.finishSidebarDrag(int(y))
	})
	n.sidebar.AddController(release)
}

func (n *Note) refreshSidebar() {
	if n.sidebar == nil {
		return
	}

	for row := n.sidebar.RowAtIndex(0); row != nil; row = n.sidebar.RowAtIndex(0) {
		n.sidebar.Remove(row)
	}
	clear(n.sidebarRows)
	clear(n.sidebarRowEntries)
	clear(n.sidebarActionButtons)

	entries, err := n.buildSidebarEntries()
	if err != nil {
		log.Println("sidebar build error:", err)
		return
	}
	for _, entry := range entries {
		row, actionButton := n.buildSidebarRow(entry)
		n.sidebar.Append(row)
		n.sidebarRows[sidebarEntryKey(entry.Kind, entry.Path)] = row
		n.sidebarRowEntries[widgetNativeKey(row)] = entry
		if actionButton != nil {
			n.sidebarActionButtons[sidebarEntryKey(entry.Kind, entry.Path)] = actionButton
		}
	}
	if tab := n.activeTab(); tab != nil {
		n.selectSidebarPath(tab.path)
	}
}

func (n *Note) buildSidebarEntries() ([]sidebarEntry, error) {
	files, err := listNoteFiles()
	if err != nil {
		return nil, err
	}
	folders, err := listNoteFolders()
	if err != nil {
		return nil, err
	}
	children := make(map[string][]string)
	for _, folder := range folders {
		parent := filepath.Dir(folder)
		if parent == "." {
			parent = ""
		}
		children[parent] = append(children[parent], folder)
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool {
			return filepath.ToSlash(children[parent][i]) < filepath.ToSlash(children[parent][j])
		})
	}
	filesByFolder := make(map[string][]noteFile)
	for _, file := range files {
		filesByFolder[file.Folder] = append(filesByFolder[file.Folder], file)
	}
	for folder := range filesByFolder {
		sort.Slice(filesByFolder[folder], func(i, j int) bool {
			return filepath.ToSlash(filesByFolder[folder][i].RelPath) < filepath.ToSlash(filesByFolder[folder][j].RelPath)
		})
	}

	entries := make([]sidebarEntry, 0, len(files)+len(folders))
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, folder := range children[parent] {
			entries = append(entries, sidebarEntry{
				Kind:   sidebarEntryFolder,
				Path:   noteFolderPath(folder),
				Label:  filepath.Base(folder),
				Depth:  depth,
				Folder: folder,
			})
			if !n.sidebarCollapsed[folder] {
				walk(folder, depth+1)
			}
		}
		for _, file := range filesByFolder[parent] {
			entries = append(entries, sidebarEntry{
				Kind:   sidebarEntryNote,
				Path:   file.Path,
				Label:  file.Title,
				Depth:  depth,
				Folder: file.Folder,
			})
		}
	}
	walk("", 0)
	return entries, nil
}

func (n *Note) buildSidebarRow(entry sidebarEntry) (*gtk.ListBoxRow, *gtk.Button) {
	row := gtk.NewListBoxRow()
	row.SetSelectable(true)
	row.SetActivatable(true)

	outer := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	outer.SetMarginStart(ui.DefaultBoxPadding + entry.Depth*16)
	outer.SetMarginEnd(ui.DefaultBoxPadding)
	outer.SetMarginTop(2)
	outer.SetMarginBottom(2)

	if entry.Kind == sidebarEntryFolder {
		expand := workspaceIconButton(n.sidebarFolderExpandIcon(entry.Folder), "Toggle folder")
		expand.AddCSSClass("flat")
		expand.ConnectClicked(func() {
			n.selectedFolder = entry.Folder
			n.sidebarCollapsed[entry.Folder] = !n.sidebarCollapsed[entry.Folder]
			n.refreshSidebar()
			if selected := n.findSidebarTreePath(entry.Path); selected != nil {
				n.sidebar.SelectRow(selected)
			}
		})
		outer.Append(expand)
	} else {
		spacer := gtk.NewLabel("")
		spacer.SetWidthChars(2)
		outer.Append(spacer)
	}

	icon := gtk.NewImageFromIconName(n.sidebarEntryIcon(entry))
	outer.Append(icon)

	label := gtk.NewLabel(entry.Label)
	label.SetHAlign(gtk.AlignStart)
	label.SetHExpand(true)
	outer.Append(label)

	actionButton := workspaceIconButton(n.sidebarEntryActionIcon(entry), n.sidebarEntryActionTooltip(entry))
	actionButton.AddCSSClass("flat")
	actionButton.ConnectClicked(func() {
		n.showSidebarRowActions(row, actionButton)
	})
	outer.Append(actionButton)

	contextClick := gtk.NewGestureClick()
	contextClick.SetButton(gdk.BUTTON_SECONDARY)
	contextClick.ConnectPressed(func(_ int, _ float64, _ float64) {
		n.sidebar.SelectRow(row)
		n.showSidebarContextMenu(row, row)
	})
	row.AddController(contextClick)

	row.SetChild(outer)
	return row, actionButton
}

func (n *Note) queueSidebarDrag(entry sidebarEntry) {
	if n.sidebarDragToggle == nil || !n.sidebarDragToggle.Active() {
		return
	}
	n.sidebarDragPending = true
	n.sidebarDragSource = entry
	n.sidebarDragSeq++
	seq := n.sidebarDragSeq
	time.AfterFunc(150*time.Millisecond, func() {
		glib.IdleAdd(func() {
			if !n.sidebarDragPending || n.sidebarDragSeq != seq {
				return
			}
			n.beginSidebarDrag(entry)
		})
	})
}

func (n *Note) cancelPendingSidebarDrag() {
	n.sidebarDragPending = false
}

func (n *Note) beginSidebarDrag(entry sidebarEntry) {
	if n.sidebarDragToggle == nil || !n.sidebarDragToggle.Active() {
		n.sidebarDragPending = false
		return
	}
	n.sidebarDragPending = false
	n.sidebarDragSource = entry
	n.sidebarDragActive = true
	n.setSidebarDragTarget(sidebarDropTarget{Folder: sidebarTargetFolder(entry.Kind, entry.Path, entry.Folder), Anchor: sidebarEntryKey(entry.Kind, entry.Path)})
}

func (n *Note) updateSidebarDragTarget(y int) {
	if !n.sidebarDragActive || n.sidebar == nil {
		return
	}
	row := n.sidebar.RowAtY(y)
	if row == nil {
		n.setSidebarDragTarget(sidebarDropTarget{Root: true})
		return
	}
	entry, ok := n.sidebarEntryForRow(row)
	if !ok || entry.Kind == sidebarEntryPlaceholder {
		n.setSidebarDragTarget(sidebarDropTarget{Root: true})
		return
	}
	target := sidebarDropTarget{
		Folder: sidebarTargetFolder(entry.Kind, entry.Path, entry.Folder),
		Anchor: sidebarEntryKey(entry.Kind, entry.Path),
	}
	if n.sidebarDragSource.Kind == sidebarEntryFolder && isPathWithin(target.Folder, relativeFolderFromPath(n.sidebarDragSource.Path)) {
		return
	}
	n.setSidebarDragTarget(target)
}

func (n *Note) setSidebarDragTarget(target sidebarDropTarget) {
	if n.sidebarDragTarget == target {
		return
	}
	oldRow := n.rowForDropTarget(n.sidebarDragTarget)
	n.sidebarDragTarget = target
	if oldRow != nil {
		n.sidebar.DragUnhighlightRow()
	}
	if row := n.rowForDropTarget(target); row != nil {
		n.sidebar.DragHighlightRow(row)
	} else {
		n.sidebar.DragUnhighlightRow()
	}
	n.updateSidebarDropHint(target)
}

func (n *Note) finishSidebarDrag(y int) {
	if !n.sidebarDragActive {
		n.cancelPendingSidebarDrag()
		return
	}
	n.updateSidebarDragTarget(y)
	target := n.sidebarDragTarget
	source := n.sidebarDragSource
	n.sidebarDragActive = false
	n.sidebarDragPending = false
	n.sidebarDragSource = sidebarEntry{}
	n.sidebarDragTarget = sidebarDropTarget{}
	n.sidebar.DragUnhighlightRow()
	n.updateSidebarDropHint(sidebarDropTarget{})
	if err := n.moveSidebarItem(source.Kind, source.Path, target.Folder); err != nil {
		n.statusMessage("Move failed: " + err.Error())
	}
	if n.sidebarDragToggle != nil {
		n.sidebarDragToggle.SetActive(false)
	}
}

func (n *Note) rowForDropTarget(target sidebarDropTarget) *gtk.ListBoxRow {
	if target.Root {
		return nil
	}
	return n.sidebarRows[target.Anchor]
}

func (n *Note) updateSidebarDropHint(target sidebarDropTarget) {
	if n.sidebarDropHint == nil {
		return
	}
	if !n.sidebarDragActive {
		n.sidebarDropHint.SetVisible(false)
		n.sidebarDropHint.SetText("")
		return
	}
	if target.Root || target.Folder == "" {
		n.sidebarDropHint.SetText("Drop into root")
		n.sidebarDropHint.SetVisible(true)
		return
	}
	n.sidebarDropHint.SetText("Drop into " + filepath.ToSlash(target.Folder))
	n.sidebarDropHint.SetVisible(true)
}

func (n *Note) sidebarEntryForRow(row *gtk.ListBoxRow) (sidebarEntry, bool) {
	entry, ok := n.sidebarRowEntries[widgetNativeKey(row)]
	return entry, ok
}

func widgetNativeKey(widget gtk.Widgetter) uintptr {
	return uintptr(glib.InternObject(widget).Native())
}

func sidebarEntryKey(kind sidebarEntryKind, path string) string {
	return fmt.Sprintf("%d:%s", kind, path)
}

func pathSidebarKind(path string) sidebarEntryKind {
	if filepath.Ext(path) == "" {
		return sidebarEntryFolder
	}
	return sidebarEntryNote
}

func (n *Note) sidebarEntryIcon(entry sidebarEntry) string {
	if entry.Kind == sidebarEntryFolder {
		return "folder-symbolic"
	}
	return "text-x-generic-symbolic"
}

func (n *Note) sidebarEntryActionIcon(entry sidebarEntry) string {
	if entry.Kind == sidebarEntryFolder {
		return "list-add-symbolic"
	}
	return "open-menu-symbolic"
}

func (n *Note) sidebarEntryActionTooltip(entry sidebarEntry) string {
	if entry.Kind == sidebarEntryFolder {
		return "New note in folder"
	}
	return "Open note actions"
}

func (n *Note) sidebarFolderExpandIcon(folder string) string {
	if n.sidebarCollapsed[folder] {
		return "pan-end-symbolic"
	}
	return "pan-down-symbolic"
}

func (n *Note) setSidebarVisible(visible bool) {
	if n.sidebarBox == nil || n.sidebarPane == nil {
		return
	}
	if n.sidebarToggle != nil && n.sidebarToggle.Active() != visible {
		n.sidebarToggle.SetActive(visible)
		return
	}
	n.applySidebarVisibility(visible)
}

func (n *Note) applySidebarVisibility(visible bool) {
	if n.sidebarBox == nil || n.sidebarPane == nil {
		return
	}
	gtk.BaseWidget(n.sidebarBox).SetVisible(visible)
	if visible {
		n.sidebarPane.SetPosition(260)
	} else {
		n.sidebarPane.SetPosition(0)
	}
}

func (n *Note) showSidebarContextMenu(row *gtk.ListBoxRow, parent gtk.Widgetter) {
	entry, ok := n.sidebarEntryForRow(row)
	if !ok {
		return
	}
	n.showSidebarRowMenu(entry.Path, entry.Kind, entry.Folder, parent)
}

func canPopupPopover(parent gtk.Widgetter) bool {
	if parent == nil {
		return false
	}
	widget := gtk.BaseWidget(parent)
	return widget.Root() != nil && widget.Native() != nil && widget.Mapped()
}

func (n *Note) showSidebarRowActions(row *gtk.ListBoxRow, parent gtk.Widgetter) {
	entry, ok := n.sidebarEntryForRow(row)
	if !ok {
		return
	}
	if entry.Kind == sidebarEntryFolder {
		n.sidebar.SelectRow(row)
		n.selectedFolder = entry.Folder
		if n.sidebarCreate == nil {
			n.sidebarCreate = n.buildSidebarCreatePopover()
		}
		n.sidebarCreateMode = sidebarEntryNote
		n.sidebarCreateFolder = entry.Folder
		if n.sidebarCreateEntry != nil {
			n.sidebarCreateEntry.SetPlaceholderText("New note name")
			n.sidebarCreateEntry.SetText("")
		}
		if existingParent := gtk.BaseWidget(n.sidebarCreate).Parent(); existingParent != nil {
			gtk.BaseWidget(n.sidebarCreate).Unparent()
		}
		n.sidebarCreate.SetParent(parent)
		if canPopupPopover(parent) {
			n.sidebarCreate.Popup()
		}
		return
	}
	n.showSidebarRowMenu(entry.Path, entry.Kind, entry.Folder, parent)
}

func (n *Note) showSidebarRowMenu(rowPath string, kind sidebarEntryKind, folder string, parent gtk.Widgetter) {
	n.sidebarMenuKind = kind
	n.sidebarMenuPath = rowPath
	n.sidebarMenuFolder = folder
	if n.sidebarMenu == nil {
		n.sidebarMenu = n.buildSidebarContextMenu()
	}
	if kind == sidebarEntryFolder {
		n.sidebarMenuEntry.SetText(filepath.ToSlash(folder))
		n.sidebarMenuEntry.SetPlaceholderText("Folder name")
		n.selectedFolder = folder
	} else {
		n.sidebarMenuEntry.SetText(noteTitleFromPath(rowPath))
		n.sidebarMenuEntry.SetPlaceholderText("Note name")
		n.selectedFolder = folder
		if page := n.pageForPath(rowPath); page >= 0 {
			n.notebook.SetCurrentPage(page)
			n.switchToTab(page)
		}
	}
	if existingParent := gtk.BaseWidget(n.sidebarMenu).Parent(); existingParent != nil {
		gtk.BaseWidget(n.sidebarMenu).Unparent()
	}
	n.sidebarMenu.SetParent(parent)
	if canPopupPopover(parent) {
		n.sidebarMenu.Popup()
	}
}

func (n *Note) buildSidebarContextMenu() *gtk.Popover {
	popover := gtk.NewPopover()
	popover.SetHasArrow(true)
	popover.SetAutohide(true)

	n.sidebarMenuEntry = gtk.NewEntry()
	n.sidebarMenuEntry.SetHExpand(true)
	n.sidebarMenuEntry.SetPlaceholderText("Name or folder")

	newNoteButton := workspaceIconButton("list-add-symbolic", "New note in folder")
	newNoteButton.ConnectClicked(func() {
		folder := n.sidebarContextFolder()
		if err := n.newNoteNamedInFolder(folder, n.sidebarMenuEntry.Text()); err != nil {
			n.statusMessage("Create note failed: " + err.Error())
			return
		}
		popover.Popdown()
	})

	renameButton := workspaceIconButton("document-edit-symbolic", "Rename selected item")
	renameButton.ConnectClicked(func() {
		if err := n.applySidebarContextRename(); err != nil {
			n.statusMessage("Rename failed: " + err.Error())
			return
		}
		popover.Popdown()
	})

	deleteButton := workspaceIconButton("user-trash-symbolic", "Delete selected item")
	deleteButton.ConnectClicked(func() {
		if err := n.deleteSidebarContextItem(); err != nil {
			n.statusMessage("Delete failed: " + err.Error())
			return
		}
		popover.Popdown()
	})

	row := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	row.Append(newNoteButton)
	row.Append(renameButton)
	row.Append(deleteButton)

	box := gtk.NewBox(gtk.OrientationVertical, ui.DefaultBoxPadding)
	box.SetMarginStart(ui.DefaultBoxPadding)
	box.SetMarginEnd(ui.DefaultBoxPadding)
	box.SetMarginTop(ui.DefaultBoxPadding)
	box.SetMarginBottom(ui.DefaultBoxPadding)
	box.Append(n.sidebarMenuEntry)
	box.Append(row)
	popover.SetChild(box)
	return popover
}

func (n *Note) sidebarContextFolder() string {
	if n.sidebarMenuKind == sidebarEntryFolder {
		return n.sidebarMenuFolder
	}
	return relativeNoteFolder(n.sidebarMenuPath)
}

func (n *Note) showSidebarCreatePopover(parent gtk.Widgetter, mode sidebarEntryKind) {
	if n.sidebarCreate == nil {
		n.sidebarCreate = n.buildSidebarCreatePopover()
	}
	n.sidebarCreateMode = mode
	n.sidebarCreateFolder = n.currentSidebarFolder()
	if n.sidebarCreateEntry != nil {
		if mode == sidebarEntryNote {
			n.sidebarCreateEntry.SetPlaceholderText("New note name")
		} else {
			n.sidebarCreateEntry.SetPlaceholderText("New folder name")
		}
		n.sidebarCreateEntry.SetText("")
	}
	if existingParent := gtk.BaseWidget(n.sidebarCreate).Parent(); existingParent != nil {
		gtk.BaseWidget(n.sidebarCreate).Unparent()
	}
	n.sidebarCreate.SetParent(parent)
	if canPopupPopover(parent) {
		n.sidebarCreate.Popup()
	}
}

func (n *Note) buildSidebarCreatePopover() *gtk.Popover {
	popover := gtk.NewPopover()
	popover.SetHasArrow(true)
	popover.SetAutohide(true)

	n.sidebarCreateEntry = gtk.NewEntry()
	n.sidebarCreateEntry.SetHExpand(true)
	n.sidebarCreateEntry.ConnectActivate(func() {
		if err := n.submitSidebarCreate(); err != nil {
			n.statusMessage(err.Error())
			return
		}
		popover.Popdown()
	})

	saveButton := workspaceIconButton("object-select-symbolic", "Create")
	saveButton.ConnectClicked(func() {
		if err := n.submitSidebarCreate(); err != nil {
			n.statusMessage(err.Error())
			return
		}
		popover.Popdown()
	})

	box := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	box.SetMarginStart(ui.DefaultBoxPadding)
	box.SetMarginEnd(ui.DefaultBoxPadding)
	box.SetMarginTop(ui.DefaultBoxPadding)
	box.SetMarginBottom(ui.DefaultBoxPadding)
	box.Append(n.sidebarCreateEntry)
	box.Append(saveButton)
	popover.SetChild(box)
	return popover
}

func (n *Note) submitSidebarCreate() error {
	name := ""
	if n.sidebarCreateEntry != nil {
		name = n.sidebarCreateEntry.Text()
	}
	switch n.sidebarCreateMode {
	case sidebarEntryFolder:
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("folder name is empty")
		}
		return n.createFolder(joinFolderParts(n.sidebarCreateFolder, name))
	case sidebarEntryNote:
		return n.newNoteNamedInFolder(n.sidebarCreateFolder, name)
	default:
		return fmt.Errorf("unknown sidebar create mode")
	}
}

func (n *Note) applySidebarContextRename() error {
	input := n.sidebarMenuEntry.Text()
	if n.sidebarMenuKind == sidebarEntryFolder {
		n.selectedFolder = n.sidebarMenuFolder
		return n.renameSelectedFolder(input)
	}
	page := n.pageForPath(n.sidebarMenuPath)
	if page < 0 {
		n.openNotePath(n.sidebarMenuPath)
		page = n.pageForPath(n.sidebarMenuPath)
	}
	if page >= 0 {
		n.notebook.SetCurrentPage(page)
		n.switchToTab(page)
	}
	return n.renameCurrentTab(input)
}

func (n *Note) deleteSidebarContextItem() error {
	if n.sidebarMenuKind == sidebarEntryFolder {
		return n.deleteFolderPath(n.sidebarMenuPath)
	}
	page := n.pageForPath(n.sidebarMenuPath)
	if page < 0 {
		if err := os.Remove(n.sidebarMenuPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		n.refreshSidebar()
		return nil
	}
	n.notebook.SetCurrentPage(page)
	n.switchToTab(page)
	return n.deleteCurrentNote()
}

func (n *Note) activateSidebarPath(row *gtk.ListBoxRow) {
	entry, ok := n.sidebarEntryForRow(row)
	if !ok {
		return
	}
	if entry.Kind == sidebarEntryFolder {
		n.selectedFolder = entry.Folder
		n.sidebarCollapsed[entry.Folder] = !n.sidebarCollapsed[entry.Folder]
		n.refreshSidebar()
		if selected := n.findSidebarTreePath(entry.Path); selected != nil {
			n.sidebar.SelectRow(selected)
		}
		return
	}
	n.openNotePath(entry.Path)
}

func (n *Note) selectSidebarPath(path string) {
	if n.sidebar == nil {
		return
	}
	if row := n.findSidebarTreePath(path); row != nil {
		n.sidebar.SelectRow(row)
	}
}

func (n *Note) handleSidebarSelectionChanged(row *gtk.ListBoxRow) {
	if n.sidebar == nil || row == nil {
		return
	}
	entry, ok := n.sidebarEntryForRow(row)
	if !ok {
		return
	}
	if entry.Kind == sidebarEntryFolder {
		n.selectedFolder = entry.Folder
		return
	}
	if active := n.activeTab(); active != nil && active.path == entry.Path {
		return
	}
	n.openNotePath(entry.Path)
}

func (n *Note) findSidebarTreePath(target string) *gtk.ListBoxRow {
	if n.sidebarRows == nil {
		return nil
	}
	return n.sidebarRows[sidebarEntryKey(pathSidebarKind(target), target)]
}

func (n *Note) syncNotebookVisibility() {
	if n.notebook == nil {
		return
	}
	n.notebook.SetShowTabs(len(n.tabs) > 1)
}

func (n *Note) renameCurrentTab(rawTitle string) error {
	tab := n.activeTab()
	if tab == nil {
		return fmt.Errorf("no active note")
	}
	title := sanitizeNoteTitle(rawTitle)
	if title == "" {
		return fmt.Errorf("empty note title")
	}
	targetPath := uniqueNotePath(title, tab.path)
	if targetPath == tab.path {
		tab.title = title
		n.refreshSidebar()
		return nil
	}
	if err := os.Rename(tab.path, targetPath); err != nil {
		return err
	}
	tab.path = targetPath
	tab.title = title
	n.updateTabLabel(tab)
	n.refreshSidebar()
	n.statusMessage("Renamed note to: " + title)
	return nil
}

func (n *Note) createFolder(rawFolder string) error {
	folder := sanitizeFolderPath(rawFolder)
	if folder == "" {
		return fmt.Errorf("empty folder path")
	}
	if err := os.MkdirAll(noteFolderPath(folder), 0o755); err != nil {
		return err
	}
	n.selectedFolder = folder
	n.refreshSidebar()
	n.statusMessage("Created folder: " + filepath.ToSlash(folder))
	return nil
}

func (n *Note) newNoteInFolder(folder string) error {
	return n.newNoteNamedInFolder(folder, "")
}

func (n *Note) newNoteNamedInFolder(folder string, rawTitle string) error {
	previous := n.selectedFolder
	n.selectedFolder = sanitizeFolderPath(folder)
	title := sanitizeNoteTitle(rawTitle)
	var path string
	if title == "" {
		path = nextNotePathInFolder(n.selectedFolder)
	} else {
		path = uniqueNotePathInFolder(n.selectedFolder, title, "")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		n.selectedFolder = previous
		return err
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		n.selectedFolder = previous
		return err
	}
	n.openNotePath(path)
	if tab := n.activeTab(); tab != nil && relativeNoteFolder(tab.path) == n.selectedFolder {
		return nil
	}
	n.selectedFolder = previous
	return fmt.Errorf("note was not created")
}

func (n *Note) renameSelectedFolder(rawFolder string) error {
	source := sanitizeFolderPath(n.selectedFolder)
	if source == "" {
		return fmt.Errorf("no selected folder")
	}
	target := sanitizeFolderPath(rawFolder)
	if target == "" {
		return fmt.Errorf("empty target folder path")
	}
	if source == target {
		n.selectedFolder = target
		return nil
	}
	sourcePath := noteFolderPath(source)
	targetPath := noteFolderPath(target)
	if isPathWithin(target, source) {
		return fmt.Errorf("cannot move folder into itself")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("target folder already exists")
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		return err
	}
	for _, tab := range n.tabs {
		if relativeNoteFolder(tab.path) == source || strings.HasPrefix(relativeNoteFolder(tab.path), source+string(filepath.Separator)) {
			rel, err := filepath.Rel(sourcePath, tab.path)
			if err != nil {
				continue
			}
			tab.path = filepath.Join(targetPath, rel)
		}
	}
	n.selectedFolder = target
	n.refreshSidebar()
	n.selectSidebarPathForActiveTab()
	n.statusMessage("Renamed folder to: " + filepath.ToSlash(target))
	return nil
}

func (n *Note) renameFolderPath(sourcePath string, rawTarget string) error {
	source := sanitizeFolderPath(relativeFolderFromPath(sourcePath))
	if source == "" {
		return fmt.Errorf("no selected folder")
	}
	target := sanitizeFolderPath(rawTarget)
	if target == "" {
		return fmt.Errorf("empty target folder path")
	}
	previous := n.selectedFolder
	n.selectedFolder = source
	err := n.renameSelectedFolder(target)
	if err != nil {
		n.selectedFolder = previous
	}
	return err
}

func (n *Note) moveCurrentNoteToFolder(rawFolder string) error {
	tab := n.activeTab()
	if tab == nil {
		return fmt.Errorf("no active note")
	}
	folder := sanitizeFolderPath(rawFolder)
	targetPath := uniqueNotePathInFolder(folder, tab.title, tab.path)
	if targetPath == tab.path {
		n.selectedFolder = folder
		n.refreshSidebar()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tab.path, targetPath); err != nil {
		return err
	}
	tab.path = targetPath
	n.selectedFolder = relativeNoteFolder(targetPath)
	n.updateTabLabel(tab)
	n.refreshSidebar()
	n.selectSidebarPath(targetPath)
	n.statusMessage("Moved note to: " + filepath.ToSlash(n.selectedFolder))
	return nil
}

func (n *Note) moveNotePathToFolder(sourcePath string, rawFolder string) error {
	page := n.pageForPath(sourcePath)
	if page >= 0 {
		n.notebook.SetCurrentPage(page)
		n.switchToTab(page)
		return n.moveCurrentNoteToFolder(rawFolder)
	}
	folder := sanitizeFolderPath(rawFolder)
	targetPath := uniqueNotePathInFolder(folder, noteTitleFromPath(sourcePath), sourcePath)
	if targetPath == sourcePath {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		return err
	}
	n.selectedFolder = relativeNoteFolder(targetPath)
	n.refreshSidebar()
	n.selectSidebarPath(targetPath)
	return nil
}

func (n *Note) moveFolderPathToFolder(sourcePath string, rawFolder string) error {
	sourceRel := sanitizeFolderPath(relativeFolderFromPath(sourcePath))
	if sourceRel == "" {
		return fmt.Errorf("cannot move notes root")
	}
	targetParent := sanitizeFolderPath(rawFolder)
	targetRel := joinFolderParts(targetParent, filepath.Base(sourceRel))
	if targetRel == sourceRel {
		return nil
	}
	if isPathWithin(targetRel, sourceRel) {
		return fmt.Errorf("cannot move folder into itself")
	}
	targetRel = uniqueFolderPath(targetRel, sourcePath)
	return n.renameFolderPath(sourcePath, targetRel)
}

func (n *Note) moveSidebarItem(kind sidebarEntryKind, sourcePath string, targetFolder string) error {
	switch kind {
	case sidebarEntryFolder:
		return n.moveFolderPathToFolder(sourcePath, targetFolder)
	case sidebarEntryNote:
		return n.moveNotePathToFolder(sourcePath, targetFolder)
	default:
		return fmt.Errorf("unsupported sidebar item")
	}
}

func (n *Note) deleteCurrentNote() error {
	tab := n.activeTab()
	if tab == nil {
		return fmt.Errorf("no active note")
	}
	path := tab.path
	n.closeCurrentTab()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	files, err := listNoteFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		n.newNote()
	}
	n.refreshSidebar()
	n.statusMessage("Deleted note: " + noteTitleFromPath(path))
	return nil
}

func (n *Note) deleteFolderPath(path string) error {
	sourceFolder := sanitizeFolderPath(relativeFolderFromPath(path))
	if sourceFolder == "" {
		return fmt.Errorf("cannot delete notes root")
	}
	for page := len(n.tabs) - 1; page >= 0; page-- {
		tab := n.tabs[page]
		tabFolder := relativeNoteFolder(tab.path)
		if tabFolder == sourceFolder || strings.HasPrefix(tabFolder, sourceFolder+string(filepath.Separator)) {
			n.notebook.SetCurrentPage(page)
			n.switchToTab(page)
			n.closeCurrentTab()
		}
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	files, err := listNoteFiles()
	if err != nil {
		return err
	}
	n.selectedFolder = sanitizeFolderPath(filepath.Dir(sourceFolder))
	if n.selectedFolder == "." {
		n.selectedFolder = ""
	}
	n.refreshSidebar()
	if len(files) == 0 {
		if err := n.newNoteInFolder(""); err != nil {
			return err
		}
	}
	n.statusMessage("Deleted folder: " + filepath.ToSlash(sourceFolder))
	return nil
}

func (n *Note) updateTabLabel(tab *noteTab) {
	if tab.tabLabel != nil {
		tab.tabLabel.SetText(tab.title)
	}
}

func (n *Note) selectSidebarPathForActiveTab() {
	if tab := n.activeTab(); tab != nil {
		n.selectSidebarPath(tab.path)
	}
}

func (n *Note) currentSidebarFolder() string {
	if n.sidebar == nil {
		return n.selectedFolder
	}
	row := n.sidebar.SelectedRow()
	if row == nil {
		return n.selectedFolder
	}
	entry, ok := n.sidebarEntryForRow(row)
	if !ok {
		return n.selectedFolder
	}
	if entry.Kind == sidebarEntryFolder {
		return entry.Folder
	}
	return relativeNoteFolder(entry.Path)
}

func notesDir() string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(dirname, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes")
}

func legacyFileName() string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(dirname, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes.txt")
}

func fileName() string {
	return filepath.Join(notesDir(), "Note 1.md")
}

func readNoteFile(path string) (string, error) {
	c, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(c), nil
}

func getNoteText() string {
	c, err := readNoteFile(fileName())
	if err != nil {
		log.Println("Didn't find notes file:", err)
		return ""
	}
	return c
}

func ensureInitialNoteFiles() ([]noteFile, error) {
	if err := os.MkdirAll(notesDir(), 0o755); err != nil {
		return nil, err
	}

	files, err := listNoteFiles()
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		return files, nil
	}

	legacy := legacyFileName()
	if _, err := os.Stat(legacy); err == nil {
		target := fileName()
		if err := os.Rename(legacy, target); err != nil {
			content, readErr := os.ReadFile(legacy)
			if readErr != nil {
				return nil, err
			}
			if writeErr := os.WriteFile(target, content, 0o644); writeErr != nil {
				return nil, writeErr
			}
		}
		return []noteFile{{Title: noteTitleFromPath(target), Path: target}}, nil
	}

	if err := os.WriteFile(fileName(), []byte(""), 0o644); err != nil {
		return nil, err
	}
	return []noteFile{{Title: noteTitleFromPath(fileName()), Path: fileName()}}, nil
}

func listNoteFiles() ([]noteFile, error) {
	files := make([]noteFile, 0)
	err := filepath.WalkDir(notesDir(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == notesDir() {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		relPath, err := filepath.Rel(notesDir(), path)
		if err != nil {
			return err
		}
		relDir := filepath.Dir(relPath)
		if relDir == "." {
			relDir = ""
		}
		files = append(files, noteFile{
			Title:   noteTitleFromPath(path),
			Path:    path,
			Folder:  relDir,
			RelDir:  relDir,
			RelPath: relPath,
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return filepath.ToSlash(files[i].RelPath) < filepath.ToSlash(files[j].RelPath)
	})
	return files, nil
}

func nextNotePathInFolder(folder string) string {
	dir := noteFolderPath(folder)
	files, err := listNoteFiles()
	if err != nil {
		return filepath.Join(dir, "Note 2.md")
	}

	used := make(map[int]struct{}, len(files))
	for _, file := range files {
		if filepath.Clean(file.Folder) != filepath.Clean(folder) {
			continue
		}
		title := file.Title
		if strings.HasPrefix(title, "Note ") {
			number, err := strconv.Atoi(strings.TrimPrefix(title, "Note "))
			if err == nil && number > 0 {
				used[number] = struct{}{}
			}
		}
	}

	for i := 1; ; i++ {
		if _, exists := used[i]; !exists {
			return filepath.Join(dir, fmt.Sprintf("Note %d.md", i))
		}
	}
}

func sanitizeNoteTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "/", "-")
	title = strings.ReplaceAll(title, string(filepath.Separator), "-")
	title = strings.ReplaceAll(title, "\\", "-")
	title = strings.Join(strings.Fields(title), " ")
	return title
}

func uniqueNotePath(title string, currentPath string) string {
	return uniqueNotePathInFolder(relativeNoteFolder(currentPath), title, currentPath)
}

func uniqueNotePathInFolder(folder string, title string, currentPath string) string {
	title = sanitizeNoteTitle(title)
	if title == "" {
		return currentPath
	}
	base := filepath.Join(noteFolderPath(folder), title+".md")
	if base == currentPath {
		return base
	}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	for i := 2; ; i++ {
		candidate := filepath.Join(noteFolderPath(folder), fmt.Sprintf("%s %d.md", title, i))
		if candidate == currentPath {
			return candidate
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func noteTitleFromPath(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "" {
		return "Untitled"
	}
	return name
}

func noteFolderPath(folder string) string {
	if folder == "" {
		return notesDir()
	}
	return filepath.Join(notesDir(), folder)
}

func relativeNoteFolder(path string) string {
	rel, err := filepath.Rel(notesDir(), filepath.Dir(path))
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

func relativeFolderFromPath(path string) string {
	rel, err := filepath.Rel(notesDir(), path)
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

func sanitizeFolderPath(folder string) string {
	folder = strings.TrimSpace(strings.ReplaceAll(folder, "\\", "/"))
	if folder == "" {
		return ""
	}
	parts := strings.Split(folder, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeNoteTitle(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return filepath.Join(cleaned...)
}

func uniqueFolders(files []noteFile) []string {
	folders := make(map[string]struct{})
	for _, file := range files {
		folder := file.Folder
		for folder != "" && folder != "." {
			folders[folder] = struct{}{}
			parent := filepath.Dir(folder)
			if parent == "." || parent == folder {
				break
			}
			folder = parent
		}
	}
	folderList := make([]string, 0, len(folders))
	for folder := range folders {
		folderList = append(folderList, folder)
	}
	sort.Slice(folderList, func(i, j int) bool {
		return filepath.ToSlash(folderList[i]) < filepath.ToSlash(folderList[j])
	})
	return folderList
}

func listNoteFolders() ([]string, error) {
	folders := make(map[string]struct{})
	err := filepath.WalkDir(notesDir(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == notesDir() || !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(notesDir(), path)
		if err != nil {
			return err
		}
		if rel == "." || rel == "" {
			return nil
		}
		folders[rel] = struct{}{}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	folderList := make([]string, 0, len(folders))
	for folder := range folders {
		folderList = append(folderList, folder)
	}
	sort.Slice(folderList, func(i, j int) bool {
		return filepath.ToSlash(folderList[i]) < filepath.ToSlash(folderList[j])
	})
	return folderList, nil
}

func sidebarTargetFolder(kind sidebarEntryKind, rowPath string, folder string) string {
	if kind == sidebarEntryFolder {
		return folder
	}
	return relativeNoteFolder(rowPath)
}

func joinFolderParts(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeFolderPath(part)
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	return filepath.Join(clean...)
}

func uniqueChildFolderName(parent string) string {
	base := "New Folder"
	candidate := joinFolderParts(parent, base)
	if _, err := os.Stat(noteFolderPath(candidate)); os.IsNotExist(err) {
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		candidate = joinFolderParts(parent, name)
		if _, err := os.Stat(noteFolderPath(candidate)); os.IsNotExist(err) {
			return name
		}
	}
}

func uniqueFolderPath(targetRel string, currentPath string) string {
	targetRel = sanitizeFolderPath(targetRel)
	if targetRel == "" {
		return targetRel
	}
	targetPath := noteFolderPath(targetRel)
	if targetPath == currentPath {
		return targetRel
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return targetRel
	}
	base := filepath.Base(targetRel)
	parent := filepath.Dir(targetRel)
	if parent == "." {
		parent = ""
	}
	for i := 2; ; i++ {
		candidateRel := joinFolderParts(parent, fmt.Sprintf("%s %d", base, i))
		candidatePath := noteFolderPath(candidateRel)
		if candidatePath == currentPath {
			return candidateRel
		}
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			return candidateRel
		}
	}
}

func isPathWithin(candidate string, root string) bool {
	if candidate == root {
		return true
	}
	return strings.HasPrefix(filepath.Clean(candidate), filepath.Clean(root)+string(filepath.Separator))
}

func (n *Note) autoSaveForTab(tab *noteTab) {
	if tab == nil || tab.waitingToSave {
		return
	}
	tab.waitingToSave = true
	if n.activeTab() == tab {
		n.WaitingToSave = true
	}
	time.AfterFunc(3*time.Second, func() {
		glib.IdleAdd(func() {
			n.saveSpecificTab(tab, true)
		})
	})
}

func workspaceIconButton(iconName string, tooltip string) *gtk.Button {
	return ui.IconButton(iconName, tooltip)
}

func workspaceToolbarButton(iconName string, glyph string, tooltip string) *gtk.Button {
	if iconName != "" {
		return workspaceIconButton(iconName, tooltip)
	}
	button := gtk.NewButton()
	button.SetTooltipText(tooltip)
	label := gtk.NewLabel(glyph)
	width := len([]rune(glyph))
	if width < 2 {
		width = 2
	}
	label.SetWidthChars(width)
	button.SetChild(label)
	return button
}

func persistEditorWidth(width int) {
	settings.SaveNotesEditorWidth(width)
}

func shouldPersistEditorWidth(tab *noteTab, width int) bool {
	if tab == nil {
		return false
	}
	if tab.applyingWidth {
		return false
	}
	if !tab.widthPersistOK {
		return false
	}
	return true
}

func applyEditorWidth(tab *noteTab) {
	if tab == nil || tab.paned == nil {
		return
	}
	allocated := gtk.BaseWidget(tab.paned).AllocatedWidth()
	saved := settings.PersistedNotesEditorWidth()
	width := resolvedEditorWidth(saved, allocated)
	if width <= 0 {
		return
	}
	tab.applyingWidth = true
	tab.paned.SetPosition(width)
	tab.applyingWidth = false
}

func scheduleApplyEditorWidth(tab *noteTab) {
	if tab == nil || tab.paned == nil {
		return
	}
	seq := time.Now().UnixNano()
	tab.widthApplySeq = seq
	tab.widthRestored = false
	tab.widthPersistOK = false
	applyEditorWidth(tab)
	glib.IdleAdd(func() {
		if tab.widthApplySeq != seq {
			return
		}
		applyEditorWidth(tab)
	})
	time.AfterFunc(150*time.Millisecond, func() {
		glib.IdleAdd(func() {
			if tab.widthApplySeq != seq {
				return
			}
			applyEditorWidth(tab)
			time.AfterFunc(100*time.Millisecond, func() {
				glib.IdleAdd(func() {
					if tab.widthApplySeq != seq {
						return
					}
					tab.widthRestored = true
					tab.widthPersistOK = true
				})
			})
		})
	})
}

func resolvedEditorWidth(savedWidth int, allocatedWidth int) int {
	if savedWidth > 0 {
		return savedWidth
	}
	if allocatedWidth > 0 {
		return allocatedWidth / 2
	}
	return 0
}

func editorWidthForPersistence(paned *gtk.Paned) (int, bool) {
	if paned == nil {
		return 0, false
	}
	child := paned.EndChild()
	if child == nil || !gtk.BaseWidget(child).Visible() {
		return 0, false
	}
	width := paned.Position()
	if width <= 0 || width > 5000 {
		return 0, false
	}
	return width, true
}
