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

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
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
}

type noteFile struct {
	Title string
	Path  string
}

func (n *Note) buildWorkspace() {
	n.notebook = gtk.NewNotebook()
	n.notebook.SetHExpand(true)
	n.notebook.SetVExpand(true)
	n.notebook.SetScrollable(true)
	n.notebook.ConnectSwitchPage(func(_ gtk.Widgetter, pageNum uint) {
		n.switchToTab(int(pageNum))
	})

	newButton := workspaceIconButton("list-add-symbolic", "New note")
	newButton.ConnectClicked(func() {
		n.newNote()
	})

	n.renameEntry = gtk.NewEntry()
	n.renameEntry.SetHExpand(true)
	n.renameEntry.SetPlaceholderText("Rename current note")

	renameButton := workspaceIconButton("document-edit-symbolic", "Rename current note")
	renameButton.ConnectClicked(func() {
		if err := n.renameCurrentTab(n.renameEntry.Text()); err != nil {
			n.statusMessage("Rename failed: " + err.Error())
		}
	})

	deleteButton := workspaceIconButton("user-trash-symbolic", "Delete current note")
	deleteButton.ConnectClicked(func() {
		if err := n.deleteCurrentNote(); err != nil {
			n.statusMessage("Delete failed: " + err.Error())
		}
	})

	n.sidebarToggle = gtk.NewToggleButton()
	n.sidebarToggle.SetIconName("sidebar-show-symbolic")
	n.sidebarToggle.SetActive(true)
	n.sidebarToggle.SetTooltipText("Show or hide notes sidebar")
	n.sidebarToggle.ConnectToggled(func() {
		n.setSidebarVisible(n.sidebarToggle.Active())
	})

	actionRow := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	actionRow.SetMarginStart(ui.DefaultBoxPadding)
	actionRow.SetMarginEnd(ui.DefaultBoxPadding)
	actionRow.SetMarginTop(ui.DefaultBoxPadding)
	actionRow.Append(n.sidebarToggle)
	actionRow.Append(newButton)
	actionRow.Append(n.renameEntry)
	actionRow.Append(renameButton)
	actionRow.Append(deleteButton)

	n.sidebar = gtk.NewListBox()
	n.sidebar.SetVExpand(true)
	n.sidebar.SetSizeRequest(180, -1)
	n.sidebar.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		n.openSidebarRow(row)
	})
	n.sidebar.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil {
			return
		}
		n.openSidebarRow(row)
	})

	n.sidebarScroll = gtk.NewScrolledWindow()
	n.sidebarScroll.SetHExpand(false)
	n.sidebarScroll.SetVExpand(true)
	n.sidebarScroll.SetMinContentWidth(180)
	n.sidebarScroll.SetChild(n.sidebar)

	mainArea := ui.MainArea()
	mainArea.SetHExpand(true)
	mainArea.SetVExpand(true)
	mainArea.Append(actionRow)
	contentRow := gtk.NewBox(gtk.OrientationHorizontal, ui.DefaultBoxPadding)
	contentRow.SetHExpand(true)
	contentRow.SetVExpand(true)
	contentRow.Append(n.sidebarScroll)
	contentRow.Append(n.notebook)
	mainArea.Append(contentRow)

	n.F = ui.Frame("Notes:")
	n.F.SetHExpand(true)
	n.F.SetVExpand(true)
	n.F.SetChild(mainArea)
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
	tab.paned.SetPosition(430)
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
			toggle := gtk.NewToggleButton()
			toggle.SetIconName(action.icon)
			toggle.SetActive(true)
			toggle.SetTooltipText(action.tooltip)
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
	n.syncNotebookVisibility()
	n.refreshSidebar()
}

func (n *Note) newNote() {
	path := nextNotePath()
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
	n.syncRenameEntry()
	n.selectSidebarPath(n.tabs[idx].path)
	n.refreshFromSettings()
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
	if n.renameEntry != nil {
		n.renameEntry.SetText("")
	}
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

func (n *Note) refreshSidebar() {
	if n.sidebar == nil {
		return
	}
	for row := n.sidebar.RowAtIndex(0); row != nil; row = n.sidebar.RowAtIndex(0) {
		n.sidebar.Remove(row)
	}
	n.sidebarPaths = n.sidebarPaths[:0]
	files, err := listNoteFiles()
	if err != nil {
		log.Println("note list error:", err)
		return
	}
	for _, file := range files {
		label := gtk.NewLabel(file.Title)
		label.SetXAlign(0)
		row := gtk.NewListBoxRow()
		row.SetChild(label)
		gtk.BaseWidget(row).SetTooltipText(file.Path)
		n.sidebar.Append(row)
		n.sidebarPaths = append(n.sidebarPaths, file.Path)
	}
	if tab := n.activeTab(); tab != nil {
		n.selectSidebarPath(tab.path)
	}
}

func (n *Note) setSidebarVisible(visible bool) {
	if n.sidebarScroll == nil {
		return
	}
	gtk.BaseWidget(n.sidebarScroll).SetVisible(visible)
}

func (n *Note) openSidebarRow(row *gtk.ListBoxRow) {
	if row == nil {
		return
	}
	index := row.Index()
	if index < 0 || index >= len(n.sidebarPaths) {
		return
	}
	n.openNotePath(n.sidebarPaths[index])
}

func (n *Note) selectSidebarPath(path string) {
	if n.sidebar == nil {
		return
	}
	for idx, candidate := range n.sidebarPaths {
		if candidate == path {
			n.sidebar.SelectRow(n.sidebar.RowAtIndex(idx))
			return
		}
	}
}

func (n *Note) syncRenameEntry() {
	if n.renameEntry == nil {
		return
	}
	if tab := n.activeTab(); tab != nil {
		n.renameEntry.SetText(tab.title)
		return
	}
	n.renameEntry.SetText("")
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
		n.syncRenameEntry()
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
	n.syncRenameEntry()
	n.statusMessage("Renamed note to: " + title)
	return nil
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

func (n *Note) updateTabLabel(tab *noteTab) {
	if tab.tabLabel != nil {
		tab.tabLabel.SetText(tab.title)
	}
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
	entries, err := os.ReadDir(notesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]noteFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			continue
		}
		path := filepath.Join(notesDir(), name)
		files = append(files, noteFile{
			Title: noteTitleFromPath(path),
			Path:  path,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Title < files[j].Title
	})
	return files, nil
}

func nextNotePath() string {
	files, err := listNoteFiles()
	if err != nil {
		return filepath.Join(notesDir(), "Note 2.md")
	}

	used := make(map[int]struct{}, len(files))
	for _, file := range files {
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
			return filepath.Join(notesDir(), fmt.Sprintf("Note %d.md", i))
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
	title = sanitizeNoteTitle(title)
	if title == "" {
		return currentPath
	}
	base := filepath.Join(notesDir(), title+".md")
	if base == currentPath {
		return base
	}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	for i := 2; ; i++ {
		candidate := filepath.Join(notesDir(), fmt.Sprintf("%s %d.md", title, i))
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
	button := gtk.NewButtonFromIconName(iconName)
	button.SetTooltipText(tooltip)
	return button
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
