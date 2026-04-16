package notes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

type Mode string

const (
	ModeNormal  Mode = "NORMAL"
	ModeInsert  Mode = "INSERT"
	ModeCommand Mode = "COMMAND"
	ModeVisual  Mode = "VISUAL"
	tagSearch        = "md-search"
)

type Key struct {
	Rune  rune
	Name  string
	Ctrl  bool
	Alt   bool
	Shift bool
}

type noteFile struct {
	Title   string
	Path    string
	Folder  string
	RelDir  string
	RelPath string
}

type treeEntryKind int

const (
	treeFolder treeEntryKind = iota
	treeNote
)

type fileEntryKind int

const (
	fileEntryScope fileEntryKind = iota
	fileEntryFolder
	fileEntryAsset
)

type TreeEntry struct {
	Kind      treeEntryKind
	Path      string
	Label     string
	Depth     int
	Collapsed bool
	Folder    string
}

type FileEntry struct {
	Kind       fileEntryKind
	Path       string
	RelPath    string
	Scope      string
	ScopeLabel string
	AssetRel   string
	Label      string
	Depth      int
	Collapsed  bool
	Folder     string
	Size       int64
	Image      bool
}

type Editor struct {
	Path                string
	Title               string
	Text                string
	Cursor              int
	ScrollTop           int
	Mode                Mode
	Command             string
	Status              string
	Dirty               bool
	PendingOp           string
	LastSearch          string
	LastSearchPos       int
	Register            vimRegister
	SelectionMode       vimSelectionMode
	SelectionMark       int
	SelectionCursor     int
	LastXText           string
	LastXCursor         int
	LastXArmed          bool
	AutoCompletePrefix  string
	AutoCompleteMatches []string
	AutoCompleteIndex   int
	AutoCompleteStart   int
	AutoCompleteEnd     int
}

type Workspace struct {
	Tree                  []TreeEntry
	Selection             int
	FileTree              []FileEntry
	FileSelection         int
	FileCommand           string
	FileCommandMode       bool
	FileFilter            string
	FileFilterMode        bool
	FileStatus            string
	FileScopeOnly         bool
	FilesDirty            bool
	PendingMigrationCount int
	Tabs                  []*Editor
	CurrentTab            int
	FocusSidebar          bool
	SidebarWidth          int
	PreviewWidth          int
	EditorRenderWidth     int
	LastHeight            int
	EditorHeight          int
	SelectedFolder        string
	pendingOpenLinks      []string
}

var currentNote *Workspace

func GenerateUI() *Workspace {
	ws, err := NewWorkspace()
	if err != nil {
		log.Println("notes init error:", err)
	}
	currentNote = ws
	return ws
}

func NewWorkspace() (*Workspace, error) {
	files, err := ensureInitialNoteFiles()
	if err != nil {
		return nil, err
	}
	_ = discardManagedFilesDraft()
	ws := &Workspace{FocusSidebar: false, CurrentTab: -1}
	ws.SidebarWidth = normalizeSidebarWidth(settings.PersistedNotesEditorWidth())
	if pending, err := countLooseManagedFiles(); err != nil {
		return nil, err
	} else {
		ws.PendingMigrationCount = pending
		if pending > 0 {
			ws.FileStatus = fmt.Sprintf("%d loose file(s) outside assets/; press M to migrate", pending)
		}
	}
	ws.refreshTree()
	ws.refreshFiles()
	if len(files) > 0 {
		if err := ws.Open(files[0].Path); err != nil {
			return nil, err
		}
	}
	return ws, nil
}

func FlushCurrentNoteState() error { return nil }

func (w *Workspace) ActiveEditor() *Editor {
	if w == nil || w.CurrentTab < 0 || w.CurrentTab >= len(w.Tabs) {
		return nil
	}
	return w.Tabs[w.CurrentTab]
}

func (w *Workspace) refreshTree() {
	files, _ := listNoteFiles()
	folders, _ := listNoteFolders()
	collapsed := make(map[string]bool)
	for _, entry := range w.Tree {
		if entry.Kind == treeFolder && entry.Collapsed {
			collapsed[entry.Folder] = true
		}
	}
	entries := make([]TreeEntry, 0, len(files)+len(folders))
	folderSet := make(map[string]struct{}, len(folders))
	for _, folder := range folders {
		folderSet[folder] = struct{}{}
	}
	folderList := make([]string, 0, len(folderSet))
	for folder := range folderSet {
		folderList = append(folderList, folder)
	}
	sort.Slice(folderList, func(i, j int) bool { return filepath.ToSlash(folderList[i]) < filepath.ToSlash(folderList[j]) })
	for _, folder := range folderList {
		entries = append(entries, TreeEntry{Kind: treeFolder, Path: noteFolderPath(folder), Label: filepath.Base(folder), Depth: strings.Count(filepath.ToSlash(folder), "/"), Folder: folder, Collapsed: collapsed[folder]})
	}
	for _, file := range files {
		if isHiddenByCollapsed(file.Folder, collapsed) {
			continue
		}
		entries = append(entries, TreeEntry{Kind: treeNote, Path: file.Path, Label: file.Title, Depth: folderDepth(file.Folder), Folder: file.Folder})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left := treeSortKey(entries[i])
		right := treeSortKey(entries[j])
		return left < right
	})
	w.Tree = entries
	if w.Selection >= len(w.Tree) {
		w.Selection = len(w.Tree) - 1
	}
	if w.Selection < 0 {
		w.Selection = 0
	}
}

func (w *Workspace) refreshFiles() {
	entries, _ := listManagedFiles()
	entries = w.refreshFilesWithFilter(entries)
	collapsed := make(map[string]bool)
	for _, entry := range w.FileTree {
		if (entry.Kind == fileEntryFolder || entry.Kind == fileEntryScope) && entry.Collapsed {
			collapsed[entry.Path] = true
		}
	}
	for i := range entries {
		if entries[i].Kind == fileEntryFolder || entries[i].Kind == fileEntryScope {
			entries[i].Collapsed = collapsed[entries[i].Path]
		}
	}
	filtered := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		hide := false
		for parent, isCollapsed := range collapsed {
			if !isCollapsed || parent == entry.Path {
				continue
			}
			if strings.HasPrefix(entry.Path, parent+string(filepath.Separator)) {
				hide = true
				break
			}
		}
		if hide {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := filepath.ToSlash(filtered[i].RelPath)
		right := filepath.ToSlash(filtered[j].RelPath)
		if left == right && filtered[i].Kind != filtered[j].Kind {
			return filtered[i].Kind == fileEntryFolder
		}
		return left < right
	})
	w.FileTree = filtered
	if w.FileSelection >= len(w.FileTree) {
		w.FileSelection = len(w.FileTree) - 1
	}
	if w.FileSelection < 0 {
		w.FileSelection = 0
	}
}

func treeSortKey(entry TreeEntry) string {
	prefix := entry.Folder
	if entry.Kind == treeFolder {
		return filepath.ToSlash(prefix) + "/"
	}
	return filepath.ToSlash(filepath.Join(prefix, entry.Label))
}

func folderDepth(folder string) int {
	if folder == "" {
		return 0
	}
	return strings.Count(filepath.ToSlash(folder), "/") + 1
}

func isHiddenByCollapsed(folder string, collapsed map[string]bool) bool {
	for folder != "" && folder != "." {
		if collapsed[folder] {
			return true
		}
		parent := filepath.Dir(folder)
		if parent == "." || parent == folder {
			break
		}
		folder = parent
	}
	return false
}

func (w *Workspace) Open(path string) error {
	for i, tab := range w.Tabs {
		if tab.Path == path {
			w.CurrentTab = i
			return nil
		}
	}
	text, err := readNoteFile(path)
	if err != nil {
		return err
	}
	ed := &Editor{Path: path, Title: noteTitleFromPath(path), Text: text, Mode: defaultEditorMode(), LastSearchPos: -1}
	w.Tabs = append(w.Tabs, ed)
	w.CurrentTab = len(w.Tabs) - 1
	w.SelectedFolder = relativeNoteFolder(path)
	return nil
}

func defaultEditorMode() Mode {
	if settings.Inst().NotesApp.VimMode {
		return ModeNormal
	}
	return ModeInsert
}

func (w *Workspace) SaveCurrent() error {
	return w.saveCurrent(true)
}

func (w *Workspace) SaveCurrentLocal() error {
	return w.saveCurrent(false)
}

func (w *Workspace) saveCurrent(sync bool) error {
	ed := w.ActiveEditor()
	if ed == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ed.Path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(ed.Path, []byte(ed.Text), 0o644); err != nil {
		return err
	}
	ed.Dirty = false
	settings.MarkDriveDirty()
	if sync {
		settings.SaveSettings()
	} else {
		settings.SaveSettingsLocal()
	}
	return nil
}

func (w *Workspace) SaveAllDirty() (bool, error) {
	return w.saveAllDirty(true)
}

func (w *Workspace) SaveAllDirtyLocal() (bool, error) {
	return w.saveAllDirty(false)
}

func (w *Workspace) HasDirty() bool {
	if w == nil {
		return false
	}
	if w.FilesDirty {
		return true
	}
	for _, ed := range w.Tabs {
		if ed != nil && ed.Dirty {
			return true
		}
	}
	return false
}

func (w *Workspace) SavePendingFiles() (bool, error) {
	if w == nil || !w.FilesDirty {
		return false, nil
	}
	if err := commitManagedFilesDraft(); err != nil {
		return false, err
	}
	w.FilesDirty = false
	if pending, err := countLooseManagedFiles(); err == nil {
		w.PendingMigrationCount = pending
	}
	w.refreshFiles()
	return true, nil
}

func (w *Workspace) DiscardPendingFiles() error {
	if err := discardManagedFilesDraft(); err != nil {
		return err
	}
	w.FilesDirty = false
	if pending, err := countLooseManagedFiles(); err == nil {
		w.PendingMigrationCount = pending
	}
	w.refreshFiles()
	return nil
}

func (w *Workspace) saveAllDirty(sync bool) (bool, error) {
	if w == nil {
		return false, nil
	}
	wroteAny := false
	for _, ed := range w.Tabs {
		if ed == nil || !ed.Dirty {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(ed.Path), 0o755); err != nil {
			return wroteAny, err
		}
		if err := os.WriteFile(ed.Path, []byte(ed.Text), 0o644); err != nil {
			return wroteAny, err
		}
		ed.Dirty = false
		wroteAny = true
	}
	if wroteAny {
		settings.MarkDriveDirty()
		if sync {
			settings.SaveSettings()
		} else {
			settings.SaveSettingsLocal()
		}
	}
	return wroteAny, nil
}

func (w *Workspace) IsEditableContext() bool {
	ed := w.ActiveEditor()
	return ed != nil && !w.FocusSidebar && (ed.Mode == ModeInsert || ed.Mode == ModeCommand || ed.Mode == ModeVisual)
}

func (w *Workspace) NextTab() bool {
	if w == nil || len(w.Tabs) <= 1 {
		return false
	}
	w.CurrentTab = (w.CurrentTab + 1) % len(w.Tabs)
	return true
}

func (w *Workspace) PrevTab() bool {
	if w == nil || len(w.Tabs) <= 1 {
		return false
	}
	w.CurrentTab--
	if w.CurrentTab < 0 {
		w.CurrentTab = len(w.Tabs) - 1
	}
	return true
}

func (w *Workspace) NewNote() bool {
	path, err := w.CreateNote("")
	if err != nil {
		if ed := w.ActiveEditor(); ed != nil {
			ed.Status = err.Error()
		}
		return false
	}
	_ = w.Open(path)
	w.FocusSidebar = false
	return true
}

func (w *Workspace) DeleteCurrentNote() bool {
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	path := ed.Path
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		ed.Status = err.Error()
		return false
	}
	w.closeTab(path)
	w.refreshTree()
	if len(w.Tabs) == 0 {
		created, err := w.CreateNote("")
		if err == nil {
			_ = w.Open(created)
		}
	}
	if w.CurrentTab >= 0 && w.CurrentTab < len(w.Tabs) {
		w.SelectedFolder = relativeNoteFolder(w.Tabs[w.CurrentTab].Path)
	}
	w.refreshFiles()
	return true
}

func (w *Workspace) CanDeleteFocusedNote() bool {
	if w == nil {
		return false
	}
	if w.FocusSidebar {
		entry := w.selectedEntry()
		return entry != nil && entry.Kind == treeNote
	}
	return w.ActiveEditor() != nil
}

func (w *Workspace) FocusedNoteDeleteLabel() string {
	if !w.CanDeleteFocusedNote() {
		return ""
	}
	if w.FocusSidebar {
		return w.selectedEntry().Label
	}
	return w.ActiveEditor().Title
}

func (w *Workspace) DeleteFocusedNote() bool {
	if !w.CanDeleteFocusedNote() {
		return false
	}
	if w.FocusSidebar {
		return w.DeleteSelection() == nil
	}
	return w.DeleteCurrentNote()
}

func (w *Workspace) RenameCurrentNote(name string) error {
	ed := w.ActiveEditor()
	if ed == nil {
		return fmt.Errorf("no active note")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("rename requires a note name")
	}
	folder := relativeNoteFolder(ed.Path)
	target := uniqueNotePathInFolder(folder, name, ed.Path)
	if target == ed.Path {
		ed.Title = noteTitleFromPath(target)
		return nil
	}
	oldAssetPath := noteAssetsPath(ed.Path)
	newAssetPath := noteAssetsPath(target)
	if err := os.Rename(ed.Path, target); err != nil {
		return err
	}
	if _, err := os.Stat(oldAssetPath); err == nil {
		renamedAssetPath := uniquePathLike(newAssetPath, oldAssetPath, true)
		if renameErr := os.Rename(oldAssetPath, renamedAssetPath); renameErr != nil {
			return renameErr
		}
	}
	ed.Path = target
	ed.Title = noteTitleFromPath(target)
	w.SelectedFolder = relativeNoteFolder(target)
	w.refreshTree()
	w.refreshFiles()
	return nil
}

func (w *Workspace) HandleKey(key Key) bool {
	if key.Ctrl && key.Name == "s" {
		_ = w.SaveCurrent()
		return true
	}
	if key.Ctrl && key.Name == "n" {
		return w.NewNote()
	}
	if key.Ctrl && key.Name == "d" {
		return w.DeleteCurrentNote()
	}
	if key.Ctrl && key.Name == "e" {
		w.FocusSidebar = !w.FocusSidebar
		return true
	}
	if key.Ctrl && key.Name == "left" {
		if w.SidebarWidth > 20 {
			w.SidebarWidth--
			settings.SaveNotesEditorWidth(w.SidebarWidth)
		}
		return true
	}
	if key.Ctrl && key.Name == "right" {
		w.SidebarWidth++
		settings.SaveNotesEditorWidth(normalizeSidebarWidth(w.SidebarWidth))
		return true
	}
	if w.FocusSidebar {
		return w.handleSidebarKey(key)
	}
	handled := w.handleEditorKey(key)
	if handled {
		w.ensureEditorVisible()
	}
	return handled
}

func (w *Workspace) handleSidebarKey(key Key) bool {
	if len(w.Tree) == 0 {
		return false
	}
	switch key.Name {
	case "[":
		return w.PrevTab()
	case "]":
		return w.NextTab()
	case "down", "j":
		if w.Selection < len(w.Tree)-1 {
			w.Selection++
		}
		return true
	case "up", "k":
		if w.Selection > 0 {
			w.Selection--
		}
		return true
	case "enter", "l":
		entry := w.Tree[w.Selection]
		if entry.Kind == treeFolder {
			for i := range w.Tree {
				if w.Tree[i].Kind == treeFolder && w.Tree[i].Folder == entry.Folder {
					w.Tree[i].Collapsed = !w.Tree[i].Collapsed
					break
				}
			}
			w.refreshTree()
			return true
		}
		_ = w.Open(entry.Path)
		w.FocusSidebar = false
		w.ensureEditorVisible()
		return true
	case "h":
		w.FocusSidebar = false
		return true
	case "n":
		_, _ = w.CreateNote("")
		return true
	case "f":
		_ = w.CreateFolder("")
		return true
	case "d":
		return w.DeleteSelection() == nil
	case "R":
		ed := w.ActiveEditor()
		if ed == nil {
			return false
		}
		w.FocusSidebar = false
		ed.Mode = ModeCommand
		ed.Command = "rename " + ed.Title
		return true
	}
	return false
}

func (w *Workspace) CreateNote(title string) (string, error) {
	folder := w.SelectedFolder
	if entry := w.selectedEntry(); entry != nil {
		folder = sidebarTargetFolder(entry.Kind, entry.Path, entry.Folder)
	}
	var path string
	if strings.TrimSpace(title) == "" {
		path = nextNotePathInFolder(folder)
	} else {
		path = uniqueNotePathInFolder(folder, title, "")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return "", err
	}
	w.SelectedFolder = folder
	w.refreshTree()
	w.refreshFiles()
	return path, w.Open(path)
}

func (w *Workspace) CreateFolder(name string) error {
	parent := w.SelectedFolder
	if entry := w.selectedEntry(); entry != nil {
		parent = sidebarTargetFolder(entry.Kind, entry.Path, entry.Folder)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = uniqueChildFolderName(parent)
	}
	rel := joinFolderParts(parent, name)
	if err := os.MkdirAll(noteFolderPath(rel), 0o755); err != nil {
		return err
	}
	w.SelectedFolder = rel
	w.refreshTree()
	w.refreshFiles()
	return nil
}

func (w *Workspace) DeleteSelection() error {
	entry := w.selectedEntry()
	if entry == nil {
		return nil
	}
	if entry.Kind == treeFolder {
		if err := os.RemoveAll(noteFolderPath(entry.Folder)); err != nil {
			return err
		}
		w.closeTabsInFolder(entry.Folder)
	} else {
		if err := os.Remove(entry.Path); err != nil {
			return err
		}
		w.closeTab(entry.Path)
	}
	w.refreshTree()
	w.refreshFiles()
	if len(w.Tabs) == 0 {
		path, err := w.CreateNote("")
		if err == nil {
			_ = w.Open(path)
		}
	}
	return nil
}

func (w *Workspace) closeTabsInFolder(folder string) {
	kept := w.Tabs[:0]
	for _, tab := range w.Tabs {
		if !strings.HasPrefix(relativeNoteFolder(tab.Path), folder) && relativeNoteFolder(tab.Path) != folder {
			kept = append(kept, tab)
		}
	}
	w.Tabs = kept
	if w.CurrentTab >= len(w.Tabs) {
		w.CurrentTab = len(w.Tabs) - 1
	}
	if w.CurrentTab < 0 && len(w.Tabs) > 0 {
		w.CurrentTab = 0
	}
}

func (w *Workspace) closeTab(path string) {
	kept := w.Tabs[:0]
	for _, tab := range w.Tabs {
		if tab.Path != path {
			kept = append(kept, tab)
		}
	}
	w.Tabs = kept
	if w.CurrentTab >= len(w.Tabs) {
		w.CurrentTab = len(w.Tabs) - 1
	}
}

func (w *Workspace) selectedEntry() *TreeEntry {
	if w.Selection < 0 || w.Selection >= len(w.Tree) {
		return nil
	}
	return &w.Tree[w.Selection]
}

func (w *Workspace) handleEditorKey(key Key) bool {
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	if key.Ctrl && key.Name == "v" {
		if ed.Mode == ModeVisual && ed.SelectionMode == vimSelectionBlock {
			clearVisualSelection(ed)
			ed.Mode = ModeNormal
		} else {
			startVisualSelection(ed, vimSelectionBlock)
		}
		return true
	}
	if ed.Mode == ModeCommand {
		return handleCommandMode(w, ed, key)
	}
	if !settings.Inst().NotesApp.VimMode {
		ed.Mode = ModeInsert
	}
	if ed.Mode == ModeInsert {
		return handleInsertMode(w, ed, key)
	}
	if ed.Mode == ModeVisual {
		return handleVisualMode(ed, key)
	}
	return handleNormalMode(w, ed, key)
}

func handleCommandMode(w *Workspace, ed *Editor, key Key) bool {
	switch key.Name {
	case "esc":
		ed.Mode = ModeNormal
		ed.Command = ""
		return true
	case "backspace":
		if len(ed.Command) > 0 {
			_, size := utf8.DecodeLastRuneInString(ed.Command)
			ed.Command = ed.Command[:len(ed.Command)-size]
		}
		return true
	case "enter":
		cmd, err := parseVimCommand(ed.Command)
		if err != nil {
			ed.Status = err.Error()
			ed.Mode = ModeNormal
			return true
		}
		executeVimCommand(w, ed, cmd)
		ed.Command = ""
		if settings.Inst().NotesApp.VimMode {
			ed.Mode = ModeNormal
		} else {
			ed.Mode = ModeInsert
		}
		return true
	}
	if key.Rune != 0 {
		ed.Command += string(key.Rune)
		return true
	}
	return false
}

func handleInsertMode(w *Workspace, ed *Editor, key Key) bool {
	switch key.Name {
	case "esc":
		clearAutoComplete(ed)
		if settings.Inst().NotesApp.VimMode {
			ed.Mode = ModeNormal
		}
		return true
	case "backspace":
		clearAutoComplete(ed)
		if ed.Cursor <= 0 {
			return true
		}
		runes := []rune(ed.Text)
		ed.Text = string(append(runes[:ed.Cursor-1], runes[ed.Cursor:]...))
		ed.Cursor--
		ed.Dirty = true
		return true
	case "enter":
		clearAutoComplete(ed)
		return insertRune(ed, '\n')
	case "tab":
		if key.Shift {
			return completeEditorPathReferenceBackward(w, ed)
		}
		if completeEditorPathReference(w, ed) {
			return true
		}
		for i := 0; i < settings.Inst().NotesApp.TabSpaces; i++ {
			insertRune(ed, ' ')
		}
		return true
	case "left":
		clearAutoComplete(ed)
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor-1)
		return true
	case "right":
		clearAutoComplete(ed)
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		return true
	case "home":
		clearAutoComplete(ed)
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
		return true
	case "end":
		clearAutoComplete(ed)
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		return true
	case "up":
		clearAutoComplete(ed)
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, -1)
		return true
	case "down":
		clearAutoComplete(ed)
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
		return true
	case "pageup":
		clearAutoComplete(ed)
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, -10)
		return true
	case "pagedown":
		clearAutoComplete(ed)
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, 10)
		return true
	case "delete":
		clearAutoComplete(ed)
		ed.Text, ed.Cursor = vimDeleteChar(ed.Text, ed.Cursor)
		ed.Dirty = true
		return true
	}
	if key.Rune != 0 {
		clearAutoComplete(ed)
		return insertRune(ed, key.Rune)
	}
	return false
}

func insertRune(ed *Editor, r rune) bool {
	runes := []rune(ed.Text)
	idx := vimClampOffset(ed.Text, ed.Cursor)
	runes = append(runes[:idx], append([]rune{r}, runes[idx:]...)...)
	ed.Text = string(runes)
	ed.Cursor = idx + 1
	ed.Dirty = true
	return true
}

func clearAutoComplete(ed *Editor) {
	if ed == nil {
		return
	}
	ed.AutoCompletePrefix = ""
	ed.AutoCompleteMatches = nil
	ed.AutoCompleteIndex = 0
	ed.AutoCompleteStart = 0
	ed.AutoCompleteEnd = 0
}

func autoCompleteStatusLine(ed *Editor, width int) string {
	if ed == nil {
		return ""
	}
	if len(ed.AutoCompleteMatches) > 0 {
		current := ed.AutoCompleteMatches[ed.AutoCompleteIndex%len(ed.AutoCompleteMatches)]
		extra := len(ed.AutoCompleteMatches) - 1
		line := "path complete: " + current
		if extra > 0 {
			line += fmt.Sprintf(" (+%d more, tab cycles)", extra)
		}
		return helpers.TruncateANSI(line, width)
	}
	ctx, ok := markdownPathCompletionContext(ed.Text, ed.Cursor)
	if !ok {
		return ""
	}
	matches := managedReferenceCandidates(ed.Path, ctx.Prefix)
	if len(matches) == 0 {
		return ""
	}
	show := matches
	if len(show) > 3 {
		show = show[:3]
	}
	line := "path suggestions: " + strings.Join(show, " | ")
	if len(matches) > 3 {
		line += fmt.Sprintf(" (+%d more)", len(matches)-3)
	}
	line += " | tab complete"
	return helpers.TruncateANSI(line, width)
}

func handleNormalMode(w *Workspace, ed *Editor, key Key) bool {
	if ed.PendingOp == "r" {
		return handleReplacePending(ed, key)
	}
	if consumeXMotionOverride(ed, key) {
		return true
	}
	if ed.PendingOp != "" && key.Name != ed.PendingOp {
		handled := applyPendingOperator(ed, key.Name)
		if handled {
			return true
		}
		ed.PendingOp = ""
	}
	switch key.Name {
	case "i":
		ed.Mode = ModeInsert
		return true
	case "a":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		ed.Mode = ModeInsert
		return true
	case "o":
		ed.Text, ed.Cursor = vimOpenLineBelow(ed.Text, ed.Cursor)
		ed.Mode = ModeInsert
		ed.Dirty = true
		return true
	case "O":
		ed.Text, ed.Cursor = vimOpenLineAbove(ed.Text, ed.Cursor)
		ed.Mode = ModeInsert
		ed.Dirty = true
		return true
	case "h", "left":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor-1)
		return true
	case "l", "right":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		return true
	case "j", "down":
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
		return true
	case "k", "up":
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, -1)
		return true
	case "home":
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
		return true
	case "end":
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		return true
	case "pageup":
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, -10)
		return true
	case "pagedown":
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, 10)
		return true
	case "0":
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
		return true
	case "$":
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		return true
	case "w":
		ed.Cursor = moveWordForward(ed.Text, ed.Cursor)
		return true
	case "b":
		ed.Cursor = moveWordBackward(ed.Text, ed.Cursor)
		return true
	case "x":
		ed.LastXText = ed.Text
		ed.LastXCursor = ed.Cursor
		ed.LastXArmed = true
		ed.Text, ed.Cursor = vimDeleteChar(ed.Text, ed.Cursor)
		ed.Dirty = true
		return true
	case "delete":
		ed.Text, ed.Cursor = vimDeleteChar(ed.Text, ed.Cursor)
		ed.Dirty = true
		return true
	case "r":
		ed.PendingOp = "r"
		return true
	case ":":
		ed.Mode = ModeCommand
		ed.Command = ""
		return true
	case "/":
		ed.Mode = ModeCommand
		ed.Command = "/"
		return true
	case "R":
		ed.Mode = ModeCommand
		ed.Command = "rename " + ed.Title
		return true
	case "n":
		repeatSearch(ed, true)
		return true
	case "N":
		repeatSearch(ed, false)
		return true
	case "tab":
		w.FocusSidebar = true
		return true
	case "V":
		startVisualSelection(ed, vimSelectionLine)
		return true
	case "v":
		startVisualSelection(ed, vimSelectionChar)
		return true
	case "d", "y", "c":
		if ed.PendingOp == key.Name {
			if key.Name == "d" || key.Name == "c" {
				ed.Text, ed.Cursor = vimDeleteLine(ed.Text, ed.Cursor)
				ed.Dirty = true
			}
			if key.Name == "y" {
				ed.Register = vimYankLine(ed.Text, ed.Cursor, ed.Cursor)
			}
			if key.Name == "c" {
				ed.Mode = ModeInsert
			}
			ed.PendingOp = ""
			return true
		}
		ed.PendingOp = key.Name
		return true
	case "p":
		reg := normalizePasteRegister(ed.Register)
		switch reg.Kind {
		case vimRegisterLine:
			ed.Text, ed.Cursor = vimPasteLine(ed.Text, ed.Cursor, reg)
		case vimRegisterBlock:
			ed.Text, ed.Cursor = vimPasteBlock(ed.Text, ed.Cursor, reg)
		default:
			ed.Text, ed.Cursor = vimPasteChar(ed.Text, ed.Cursor, reg)
		}
		ed.Dirty = true
		return true
	}
	return false
}

func normalizePasteRegister(reg vimRegister) vimRegister {
	if reg.Kind == vimRegisterChar && strings.Contains(reg.Text, "\n") {
		reg.Kind = vimRegisterLine
		if !strings.HasSuffix(reg.Text, "\n") {
			reg.Text += "\n"
		}
	}
	return reg
}

func handleReplacePending(ed *Editor, key Key) bool {
	if key.Name == "esc" {
		ed.PendingOp = ""
		return true
	}
	if key.Rune == 0 {
		return false
	}
	ed.Text, ed.Cursor = vimReplaceChar(ed.Text, ed.Cursor, key.Rune)
	ed.Dirty = true
	ed.PendingOp = ""
	return true
}

func consumeXMotionOverride(ed *Editor, key Key) bool {
	if !ed.LastXArmed {
		return false
	}
	defer func() {
		ed.LastXArmed = false
		ed.LastXText = ""
		ed.LastXCursor = 0
	}()
	switch key.Name {
	case "w":
		start := ed.LastXCursor
		end := moveWordForward(ed.LastXText, ed.LastXCursor)
		if end <= start {
			return false
		}
		ed.Text, ed.Cursor = vimDeleteRange(ed.LastXText, start, end-1)
		ed.Dirty = true
		return true
	case "$", "end":
		start := ed.LastXCursor
		end := vimLineBoundaryOffset(ed.LastXText, ed.LastXCursor, true)
		if end <= start {
			return false
		}
		ed.Text, ed.Cursor = vimDeleteRange(ed.LastXText, start, end-1)
		ed.Dirty = true
		return true
	default:
		return false
	}
}

func handleVisualMode(ed *Editor, key Key) bool {
	switch key.Name {
	case "esc":
		clearVisualSelection(ed)
		ed.Mode = ModeNormal
		return true
	case "h", "left":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor-1)
		refreshVisualSelection(ed)
		return true
	case "l", "right":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		refreshVisualSelection(ed)
		return true
	case "j", "down":
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, 1)
		refreshVisualSelection(ed)
		return true
	case "k", "up":
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, -1)
		refreshVisualSelection(ed)
		return true
	case "w":
		ed.Cursor = moveWordForward(ed.Text, ed.Cursor)
		refreshVisualSelection(ed)
		return true
	case "b":
		ed.Cursor = moveWordBackward(ed.Text, ed.Cursor)
		refreshVisualSelection(ed)
		return true
	case "0", "home":
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
		refreshVisualSelection(ed)
		return true
	case "$", "end":
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		refreshVisualSelection(ed)
		return true
	case "pageup":
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, -10)
		refreshVisualSelection(ed)
		return true
	case "pagedown":
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, 10)
		refreshVisualSelection(ed)
		return true
	case "V":
		if ed.SelectionMode == vimSelectionLine {
			clearVisualSelection(ed)
			ed.Mode = ModeNormal
			return true
		}
		startVisualSelection(ed, vimSelectionLine)
		return true
	case "v":
		if ed.SelectionMode == vimSelectionChar {
			clearVisualSelection(ed)
			ed.Mode = ModeNormal
			return true
		}
		startVisualSelection(ed, vimSelectionChar)
		return true
	case "d", "x":
		deleteVisualSelection(ed)
		ed.Mode = ModeNormal
		return true
	case "y":
		yankVisualSelection(ed)
		ed.Mode = ModeNormal
		return true
	}
	return false
}

func applyPendingOperator(ed *Editor, key string) bool {
	if ed.PendingOp == "d" && key == "w" {
		ed.Text, ed.Cursor = vimDeleteWord(ed.Text, ed.Cursor)
		ed.Dirty = true
		ed.PendingOp = ""
		return true
	}
	start := ed.Cursor
	end := ed.Cursor
	switch key {
	case "w":
		end = moveWordForward(ed.Text, ed.Cursor)
	case "b":
		end = moveWordBackward(ed.Text, ed.Cursor)
	default:
		ed.PendingOp = ""
		return false
	}
	if ed.PendingOp == "y" {
		ed.Register = vimYankChar(ed.Text, start, max(start, end-1))
		ed.PendingOp = ""
		return true
	}
	if start > end {
		start, end = end, start
	}
	runes := []rune(ed.Text)
	if end > len(runes) {
		end = len(runes)
	}
	deleted := string(runes[start:end])
	ed.Register = vimRegister{Kind: vimRegisterChar, Text: deleted}
	ed.Text = string(append(runes[:start], runes[end:]...))
	ed.Cursor = start
	ed.Dirty = true
	mode := ed.PendingOp
	ed.PendingOp = ""
	if mode == "c" {
		ed.Mode = ModeInsert
	}
	return true
}

func moveWordForward(text string, offset int) int {
	runes := []rune(text)
	i := vimClampOffset(text, offset)
	for i < len(runes) && isWordRune(runes[i]) {
		i++
	}
	for i < len(runes) && !isWordRune(runes[i]) {
		i++
	}
	return i
}

func moveWordBackward(text string, offset int) int {
	runes := []rune(text)
	i := vimClampOffset(text, offset)
	if i > 0 {
		i--
	}
	for i > 0 && !isWordRune(runes[i]) {
		i--
	}
	for i > 0 && isWordRune(runes[i-1]) {
		i--
	}
	return i
}

func isWordRune(r rune) bool {
	return r == '_' || r == '-' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || r > 127
}

func vimPageMoveOffset(text string, offset int, delta int) int {
	return vimVerticalMoveOffset(text, offset, delta)
}

func repeatSearch(ed *Editor, forward bool) {
	if ed.LastSearch == "" {
		return
	}
	if forward {
		idx := findNext(ed.Text, ed.LastSearch, ed.Cursor+1)
		if idx < 0 {
			idx = findNext(ed.Text, ed.LastSearch, 0)
		}
		if idx >= 0 {
			ed.Cursor = idx
			ed.LastSearchPos = idx
		}
		return
	}
	idx := findPrevious(ed.Text, ed.LastSearch, ed.Cursor-1)
	if idx < 0 {
		idx = findPrevious(ed.Text, ed.LastSearch, len([]rune(ed.Text))-1)
	}
	if idx >= 0 {
		ed.Cursor = idx
		ed.LastSearchPos = idx
	}
}

func startVisualSelection(ed *Editor, mode vimSelectionMode) {
	ed.SelectionMode = mode
	ed.SelectionMark = ed.Cursor
	ed.SelectionCursor = ed.Cursor
	ed.Mode = ModeVisual
	ed.PendingOp = ""
}

func refreshVisualSelection(ed *Editor) {
	if ed.SelectionMode == vimSelectionNone {
		return
	}
	ed.SelectionCursor = ed.Cursor
}

func clearVisualSelection(ed *Editor) {
	ed.SelectionMode = vimSelectionNone
	ed.SelectionMark = 0
	ed.SelectionCursor = 0
}

func yankVisualSelection(ed *Editor) {
	switch ed.SelectionMode {
	case vimSelectionChar:
		ed.Register = vimYankChar(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Status = "yanked selection"
	case vimSelectionLine:
		ed.Register = vimYankLine(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Status = "yanked lines"
	case vimSelectionBlock:
		ed.Register = vimYankBlock(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Status = "yanked block"
	}
	clearVisualSelection(ed)
}

func deleteVisualSelection(ed *Editor) {
	switch ed.SelectionMode {
	case vimSelectionChar:
		ed.Text, ed.Cursor = vimDeleteRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	case vimSelectionLine:
		start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Text, ed.Cursor = vimDeleteRange(ed.Text, start, max(start, end-1))
	case vimSelectionBlock:
		ed.Text, ed.Cursor = vimDeleteBlock(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	default:
		return
	}
	ed.Dirty = true
	ed.Status = "deleted selection"
	clearVisualSelection(ed)
}

func executeVimCommand(w *Workspace, ed *Editor, cmd vimCommand) {
	switch cmd.Kind {
	case vimCommandSave:
		if err := w.SaveCurrent(); err != nil {
			ed.Status = err.Error()
		} else {
			ed.Status = "saved"
		}
	case vimCommandSearch:
		ed.LastSearch = cmd.Query
		idx := findNext(ed.Text, cmd.Query, ed.Cursor)
		if idx < 0 {
			idx = findNext(ed.Text, cmd.Query, 0)
		}
		if idx >= 0 {
			ed.Cursor = idx
			ed.LastSearchPos = idx
			ed.Status = "search hit"
		} else {
			ed.Status = "pattern not found"
		}
	case vimCommandReplace:
		var count int
		if cmd.CurrentLine {
			start, end := vimLineRange(ed.Text, ed.Cursor, ed.Cursor)
			ed.Text, count = replaceTextInRange(ed.Text, cmd.Query, cmd.Replacement, cmd.Global, start, end)
		} else {
			ed.Text, count = replaceText(ed.Text, cmd.Query, cmd.Replacement, cmd.Global)
		}
		if count > 0 {
			ed.Dirty = true
		}
		ed.Status = fmt.Sprintf("%d replacements", count)
	case vimCommandRename:
		if err := w.RenameCurrentNote(cmd.Name); err != nil {
			ed.Status = err.Error()
		} else {
			ed.Status = "renamed"
		}
	case vimCommandOpenLinks:
		links := collectSupportedLinks(ed.Text)
		if len(links) == 0 {
			ed.Status = "no supported links found"
		} else {
			w.pendingOpenLinks = append([]string(nil), links...)
			ed.Status = fmt.Sprintf("%d links ready", len(links))
		}
	}
}

func (w *Workspace) Render(width int, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 4 {
		height = 4
	}
	w.LastHeight = height
	sidebarWidth := normalizeSidebarWidth(w.SidebarWidth)
	if !settings.Inst().NotesApp.SidebarVisible {
		sidebarWidth = 0
	}
	if sidebarWidth <= 0 {
		sidebarWidth = 1
	}
	if sidebarWidth > width/3 {
		sidebarWidth = width / 3
	}
	contentWidth := width - sidebarWidth - 1
	left := w.renderTree(height, sidebarWidth)
	right := w.renderEditor(height, contentWidth)
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		l := helpers.PadANSI(lineAt(left, i), sidebarWidth)
		r := helpers.PadANSI(lineAt(right, i), contentWidth)
		if !settings.Inst().NotesApp.SidebarVisible {
			lines = append(lines, r)
			continue
		}
		lines = append(lines, l+helpers.ANSI(helpers.ANSIDim, "|")+r)
	}
	return strings.Join(lines, "\n")
}

func (w *Workspace) HelpText() string {
	if w == nil {
		return ""
	}
	if w.FocusSidebar {
		return "notes/sidebar: j k move | enter open/toggle | n new note | f new folder | d delete | R rename current | [/] tabs | ctrl+e editor"
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return "notes: i insert | ctrl+e sidebar | ctrl+n new | ctrl+d delete | R rename | [/] tabs | ctrl+s save"
	}
	if ed.Mode == ModeInsert {
		return "notes/insert: tab complete or spaces | shift+tab reverse complete | esc normal | ctrl+s save | ctrl+e sidebar"
	}
	if ed.Mode == ModeCommand {
		return "notes/command: enter run | esc cancel | :w save | /text search | ol open links | rename name | n next | N prev | %s/old/new/g replace"
	}
	if ed.Mode == ModeVisual {
		return "notes/visual: h j k l move | V line | ctrl+v block | y yank | d/x delete | esc normal"
	}
	return "notes/normal: i insert | r<char> replace | x delete | xw word | x$ eol | : command | / search | R rename | n next | N prev | ctrl+n new | ctrl+d delete | [/] tabs | ctrl+s save | ctrl+e sidebar"
}

func (w *Workspace) TakePendingOpenLinks() []string {
	if w == nil || len(w.pendingOpenLinks) == 0 {
		return nil
	}
	links := append([]string(nil), w.pendingOpenLinks...)
	w.pendingOpenLinks = nil
	return links
}

func (w *Workspace) SidebarRows(height int) []string {
	width := normalizeSidebarWidth(w.SidebarWidth)
	return w.renderTree(height, width)
}

func (w *Workspace) EditorRows(width int, height int) []string {
	ed := w.ActiveEditor()
	if ed == nil {
		return fillLines(height, "No note open")
	}
	return renderEditorPane(ed, width, height)
}

func (w *Workspace) PreviewRows(width int, height int) []string {
	ed := w.ActiveEditor()
	if ed == nil {
		return fillLines(height, "")
	}
	return renderPreviewPane(ed, width, height)
}

func (w *Workspace) TabsText(width int) string {
	return helpers.TruncateANSI(renderTabs(w), width)
}

func (w *Workspace) StatusText(width int) string {
	return w.statusLine(width)
}

func (w *Workspace) CommandLineText(width int) string {
	ed := w.ActiveEditor()
	if ed == nil {
		return helpers.TruncateANSI(":w save | / search", width)
	}
	if ed.Mode == ModeCommand {
		return helpers.TruncateANSI(ed.Command, width)
	}
	if ed.Mode == ModeInsert {
		if line := autoCompleteStatusLine(ed, width); line != "" {
			return line
		}
	}
	if ed.LastSearch != "" {
		return helpers.TruncateANSI("/"+ed.LastSearch, width)
	}
	return helpers.TruncateANSI(":w save | / search | n next | N prev", width)
}

func (w *Workspace) PaneWidths(totalWidth int) (int, int, int) {
	if totalWidth < 20 {
		return 0, totalWidth / 2, totalWidth - (totalWidth / 2)
	}
	sidebarWidth := normalizeSidebarWidth(w.SidebarWidth)
	if !settings.Inst().NotesApp.SidebarVisible {
		sidebarWidth = 0
	}
	if sidebarWidth > totalWidth/3 {
		sidebarWidth = totalWidth / 3
	}
	contentWidth := totalWidth - sidebarWidth
	if sidebarWidth > 0 {
		contentWidth--
	}
	if contentWidth < 20 {
		contentWidth = 20
	}
	editorWidth := resolveEditorWidth(settings.Inst().NotesApp.EditorWidth, contentWidth)
	previewWidth := contentWidth - editorWidth
	if previewWidth < 10 {
		previewWidth = 10
		editorWidth = max(10, contentWidth-previewWidth)
	}
	return sidebarWidth, editorWidth, previewWidth
}

func (w *Workspace) CursorPosition(totalWidth int) (int, int, bool) {
	if w == nil {
		return 0, 0, false
	}
	if settings.Inst().NotesApp.SidebarVisible && w.FocusSidebar {
		row := 3 + w.Selection
		col := 2
		return row, col, true
	}
	ed := w.ActiveEditor()
	if ed == nil || w.FocusSidebar {
		return 0, 0, false
	}
	rowOffset, cursorCol := editorVisualCursor(ed, w.editorRenderWidth())
	row := 3 + (rowOffset - ed.ScrollTop)
	col := 1
	if settings.Inst().NotesApp.SidebarVisible {
		sidebarWidth := normalizeSidebarWidth(w.SidebarWidth)
		if sidebarWidth > totalWidth/3 {
			sidebarWidth = totalWidth / 3
		}
		col += sidebarWidth + 1
	}
	col += cursorCol
	return row, col, true
}

func (w *Workspace) SidebarCursor() (int, int, bool) {
	if w == nil || !w.FocusSidebar || len(w.Tree) == 0 {
		return 0, 0, false
	}
	row := w.Selection + 1
	if row < 0 {
		row = 0
	}
	return row, 0, true
}

func (w *Workspace) EditorCursor() (int, int, bool) {
	if w == nil || w.FocusSidebar {
		return 0, 0, false
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return 0, 0, false
	}
	rowOffset, cursorCol := editorVisualCursor(ed, w.editorRenderWidth())
	row := rowOffset - ed.ScrollTop
	if row < 0 {
		row = 0
	}
	if cursorCol < 0 {
		cursorCol = 0
	}
	return row, cursorCol, true
}

func (w *Workspace) CommandCursor() (int, bool) {
	if w == nil {
		return 0, false
	}
	ed := w.ActiveEditor()
	if ed == nil || ed.Mode != ModeCommand {
		return 0, false
	}
	return len([]rune(ed.Command)), true
}

func (w *Workspace) statusLine(width int) string {
	status := "notes"
	if ed := w.ActiveEditor(); ed != nil {
		mode := string(ed.Mode)
		if ed.Mode == ModeVisual {
			switch ed.SelectionMode {
			case vimSelectionLine:
				mode = "VISUAL LINE"
			case vimSelectionBlock:
				mode = "VISUAL BLOCK"
			default:
				mode = "VISUAL"
			}
		}
		status = fmt.Sprintf("%s | %s", mode, ed.Title)
		if ed.Status != "" {
			status += " | " + ed.Status
		}
		if ed.Dirty {
			status += " | modified"
		}
	}
	return helpers.TruncateANSI(status, width)
}

func (w *Workspace) renderTree(height int, width int) []string {
	lines := []string{helpers.TruncateANSI(helpers.ANSI(helpers.ANSIBold, "Notes"), width)}
	for i, entry := range w.Tree {
		marker := " "
		if i == w.Selection {
			marker = helpers.ANSI(helpers.ANSIReverse, ">")
		}
		icon := "*"
		if entry.Kind == treeFolder {
			if entry.Collapsed {
				icon = "+"
			} else {
				icon = "-"
			}
		}
		label := strings.Repeat("  ", entry.Depth) + icon + " " + entry.Label
		lines = append(lines, helpers.TruncateANSI(marker+" "+label, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

func (w *Workspace) renderEditor(height int, width int) []string {
	ed := w.ActiveEditor()
	if ed == nil {
		return fillLines(height, "No note open")
	}
	editorWidth := resolveEditorWidth(settings.Inst().NotesApp.EditorWidth, width)
	previewWidth := width - editorWidth - 1
	tabsLine := helpers.TruncateANSI(renderTabs(w), width)
	editorLines := renderEditorPane(ed, editorWidth, height-1)
	previewLines := renderPreviewPane(ed, previewWidth, height-1)
	lines := []string{tabsLine}
	for i := 0; i < height-1; i++ {
		lines = append(lines, helpers.PadANSI(lineAt(editorLines, i), editorWidth)+helpers.ANSI(helpers.ANSIDim, "|")+helpers.PadANSI(lineAt(previewLines, i), previewWidth))
	}
	return lines
}

func renderTabs(w *Workspace) string {
	parts := make([]string, 0, len(w.Tabs))
	for i, tab := range w.Tabs {
		label := tab.Title
		if tab.Dirty {
			label += "*"
		}
		if i == w.CurrentTab {
			label = helpers.ANSI(helpers.ANSIReverse, "["+label+"]")
		} else {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func renderEditorPane(ed *Editor, width int, height int) []string {
	rows := buildEditorVisualRows(ed, width)
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		rowIdx := ed.ScrollTop + i
		if rowIdx >= len(rows) {
			out = append(out, "")
			continue
		}
		out = append(out, rows[rowIdx])
	}
	return out
}

func editorLineNumberWidth(lines []string, width int) int {
	if width < 8 {
		return 0
	}
	digits := len(fmt.Sprintf("%d", max(1, len(lines))))
	return digits + 1
}

func renderPreviewPane(ed *Editor, width int, height int) []string {
	render := markdownPreview(ed.Text, settings.Inst().NotesApp.TabSpaces)
	lines := strings.Split(render.Text, "\n")
	out := make([]string, 0, height)
	lineSpans := groupSpansByLine(render.Text, render.Spans)
	searchSpans := groupSpansByLine(render.Text, searchHighlightSpans(render.Text, ed.LastSearch))
	rows := make([]string, 0, len(lines))
	for lineIdx := range lines {
		spans := []markdownSpan(nil)
		if lineIdx < len(lineSpans) {
			spans = lineSpans[lineIdx]
		}
		if hasTag(spans, tagHorizontalRule) {
			rows = append(rows, strings.Repeat("-", max(1, width)))
			continue
		}
		base := spans
		if ed.LastSearch != "" && lineIdx < len(searchSpans) && len(searchSpans[lineIdx]) > 0 {
			base = searchSpans[lineIdx]
		}
		for _, segment := range wrapPlainLine(lines[lineIdx], max(1, width)) {
			rows = append(rows, applySegmentMarkdown(segment.text, base, segment.start, segment.end))
		}
	}
	for i := 0; i < height; i++ {
		rowIdx := ed.ScrollTop + i
		if rowIdx >= len(rows) {
			out = append(out, "")
			continue
		}
		out = append(out, rows[rowIdx])
	}
	return out
}

type wrappedSegment struct {
	start int
	end   int
	text  string
}

func buildEditorVisualRows(ed *Editor, width int) []string {
	lines := strings.Split(ed.Text, "\n")
	lineSpans := groupSpansByLine(ed.Text, editorRenderSpans(ed.Text, settings.Inst().NotesApp.TabSpaces))
	searchSpans := groupSpansByLine(ed.Text, searchHighlightSpans(ed.Text, ed.LastSearch))
	selectionSpans := groupSpansByLine(ed.Text, visualHighlightSpans(ed))
	gutterWidth := editorLineNumberWidth(lines, width)
	contentWidth := max(1, width-gutterWidth)
	rows := make([]string, 0, len(lines))
	for lineIdx, plainLine := range lines {
		baseSpans := []markdownSpan(nil)
		if lineIdx < len(lineSpans) {
			baseSpans = lineSpans[lineIdx]
		}
		if ed.LastSearch != "" && lineIdx < len(searchSpans) && len(searchSpans[lineIdx]) > 0 {
			baseSpans = searchSpans[lineIdx]
		} else if lineIdx < len(selectionSpans) && len(selectionSpans[lineIdx]) > 0 {
			baseSpans = selectionSpans[lineIdx]
		}
		segments := wrapPlainLine(plainLine, contentWidth)
		for segIdx, segment := range segments {
			prefix := ""
			if gutterWidth > 0 {
				if segIdx == 0 {
					prefix = helpers.ANSI(helpers.ANSIDim, fmt.Sprintf("%*d ", gutterWidth-1, lineIdx+1))
				} else {
					prefix = helpers.ANSI(helpers.ANSIDim, strings.Repeat(" ", gutterWidth))
				}
			}
			rows = append(rows, prefix+applySegmentMarkdown(segment.text, baseSpans, segment.start, segment.end))
		}
	}
	return rows
}

func applySegmentMarkdown(text string, spans []markdownSpan, start int, end int) string {
	if len(spans) == 0 {
		return text
	}
	local := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if span.End <= start || span.Start >= end {
			continue
		}
		local = append(local, markdownSpan{
			Tag:   span.Tag,
			Start: max(span.Start-start, 0),
			End:   min(span.End-start, end-start),
		})
	}
	return applyANSIMarkdown(text, local)
}

func wrapPlainLine(line string, width int) []wrappedSegment {
	if width <= 0 {
		width = 1
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []wrappedSegment{{start: 0, end: 0, text: ""}}
	}
	segments := make([]wrappedSegment, 0, (len(runes)/width)+1)
	for start := 0; start < len(runes); {
		end := min(start+width, len(runes))
		if end < len(runes) {
			breakAt := -1
			for i := end - 1; i > start; i-- {
				if runes[i] == ' ' || runes[i] == '\t' {
					breakAt = i
					break
				}
			}
			if breakAt > start {
				trimEnd := breakAt
				for trimEnd > start && (runes[trimEnd-1] == ' ' || runes[trimEnd-1] == '\t') {
					trimEnd--
				}
				segments = append(segments, wrappedSegment{start: start, end: max(trimEnd, start), text: string(runes[start:trimEnd])})
				start = breakAt + 1
				for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
					start++
				}
				if start >= len(runes) {
					segments = append(segments, wrappedSegment{start: len(runes), end: len(runes), text: ""})
				}
				continue
			}
		}
		segments = append(segments, wrappedSegment{start: start, end: end, text: string(runes[start:end])})
		start = end
	}
	if len(segments) == 0 {
		return []wrappedSegment{{start: 0, end: 0, text: ""}}
	}
	return segments
}

func editorVisualCursor(ed *Editor, width int) (int, int) {
	lines := strings.Split(ed.Text, "\n")
	cursorLine, cursorCol := cursorLineCol(ed.Text, ed.Cursor)
	gutterWidth := editorLineNumberWidth(lines, width)
	contentWidth := max(1, width-gutterWidth)
	rowOffset := 0
	for idx, line := range lines {
		segments := wrapPlainLine(line, contentWidth)
		if idx == cursorLine {
			for segIdx, segment := range segments {
				if cursorCol < segment.end || (segIdx == len(segments)-1 && cursorCol <= segment.end) {
					return rowOffset + segIdx, cursorCol - segment.start
				}
			}
			last := segments[len(segments)-1]
			return rowOffset + len(segments) - 1, max(0, cursorCol-last.start)
		}
		rowOffset += len(segments)
	}
	return 0, 0
}

func editorWrapWidth() int {
	if saved := settings.Inst().NotesApp.EditorWidth; saved > 0 {
		return saved
	}
	return 40
}

func (w *Workspace) editorRenderWidth() int {
	if w != nil && w.EditorRenderWidth > 0 {
		return w.EditorRenderWidth
	}
	return editorWrapWidth()
}

func hasTag(spans []markdownSpan, tag string) bool {
	for _, span := range spans {
		if span.Tag == tag {
			return true
		}
	}
	return false
}

func cursorLineCol(text string, offset int) (int, int) {
	runes := []rune(text)
	if offset > len(runes) {
		offset = len(runes)
	}
	line, col := 0, 0
	for i := 0; i < offset; i++ {
		if runes[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return line, col
}

func lineAt(lines []string, idx int) string {
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}
func fillLines(height int, text string) []string {
	out := make([]string, height)
	if height > 0 {
		out[0] = text
	}
	return out
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func normalizeSidebarWidth(width int) int {
	if width <= 0 {
		return 28
	}
	if width > 120 {
		return 28
	}
	return width
}

func resolveEditorWidth(saved int, total int) int {
	if total < 20 {
		return total / 2
	}
	if saved <= 0 || saved >= total-10 {
		return max(10, (total-1)/2)
	}
	if saved < 10 {
		return 10
	}
	return saved
}

func groupSpansByLine(text string, spans []markdownSpan) [][]markdownSpan {
	lines := strings.Split(text, "\n")
	grouped := make([][]markdownSpan, len(lines))
	offset := 0
	for lineIndex, line := range lines {
		lineLen := len([]rune(line))
		lineStart := offset
		lineEnd := offset + lineLen
		for _, span := range spans {
			if span.End <= lineStart || span.Start >= lineEnd {
				continue
			}
			grouped[lineIndex] = append(grouped[lineIndex], markdownSpan{
				Tag:   span.Tag,
				Start: max(span.Start-lineStart, 0),
				End:   min(span.End-lineStart, lineLen),
			})
		}
		offset = lineEnd + 1
	}
	return grouped
}

func applyANSIMarkdown(line string, spans []markdownSpan) string {
	if len(spans) == 0 || line == "" {
		return line
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].Start == spans[j].Start {
			return spans[i].End < spans[j].End
		}
		return spans[i].Start < spans[j].Start
	})
	runes := []rune(line)
	var b strings.Builder
	pos := 0
	for _, span := range spans {
		if span.Tag == tagCodeBlock {
			continue
		}
		if span.Start < pos || span.Start >= len(runes) {
			continue
		}
		if span.Start > pos {
			b.WriteString(string(runes[pos:span.Start]))
		}
		end := span.End
		if end > len(runes) {
			end = len(runes)
		}
		if end <= span.Start {
			continue
		}
		b.WriteString(styleForMarkdownTag(span.Tag, string(runes[span.Start:end])))
		pos = end
	}
	if pos < len(runes) {
		b.WriteString(string(runes[pos:]))
	}
	return b.String()
}

func styleForMarkdownTag(tag string, text string) string {
	switch tag {
	case tagHeading1:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgBlue, text)
	case tagHeading2:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgPurple, text)
	case tagHeading3:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgYellow, text)
	case tagHeading4:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgCyan, text)
	case tagHeading5:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgOrange, text)
	case tagHeading6:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgGreen, text)
	case tagBold:
		return helpers.ANSI(helpers.ANSIBold, text)
	case tagItalic:
		return helpers.ANSI(helpers.ANSIItalic, text)
	case tagQuote, tagCodeComment:
		return helpers.ANSI(helpers.ANSIDim+helpers.ANSIFgGray, text)
	case tagCode, tagCodeString:
		return helpers.ANSI(helpers.ANSIFgGreen, text)
	case tagCodeKeyword:
		return helpers.ANSI(helpers.ANSIFgPurple, text)
	case tagList, tagOrdered, tagChecklist:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgGray, text)
	case tagCodeNumber:
		return helpers.ANSI(helpers.ANSIFgOrange, text)
	case tagCodeType:
		return helpers.ANSI(helpers.ANSIFgCyan, text)
	case tagCodeFunction:
		return helpers.ANSI(helpers.ANSIFgBlue, text)
	case tagCodeProperty, tagLink:
		return helpers.ANSI(helpers.ANSIFgYellow, text)
	case tagCodeConstant:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIFgYellow, text)
	case tagSearch:
		return helpers.ANSI(helpers.ANSIReverse, text)
	case tagVisualSelection:
		return helpers.ANSI(helpers.ANSIReverse, text)
	case tagCodeBlock:
		return text
	default:
		return text
	}
}

func visualHighlightSpans(ed *Editor) []markdownSpan {
	if ed == nil || ed.SelectionMode == vimSelectionNone {
		return nil
	}
	switch ed.SelectionMode {
	case vimSelectionChar:
		start := min(ed.SelectionMark, ed.SelectionCursor)
		end := max(ed.SelectionMark, ed.SelectionCursor) + 1
		return []markdownSpan{{Tag: tagVisualSelection, Start: start, End: end}}
	case vimSelectionLine:
		start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		return []markdownSpan{{Tag: tagVisualSelection, Start: start, End: end}}
	case vimSelectionBlock:
		lines := vimLineInfos(ed.Text)
		startIdx, endIdx, startCol, endCol := vimLineColumns(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		spans := make([]markdownSpan, 0, endIdx-startIdx+1)
		for idx := startIdx; idx <= endIdx; idx++ {
			line := lines[idx]
			lineWidth := line.end - line.start
			from := startCol
			if from > lineWidth {
				from = lineWidth
			}
			to := endCol + 1
			if to > lineWidth {
				to = lineWidth
			}
			if from >= to {
				continue
			}
			spans = append(spans, markdownSpan{Tag: tagVisualSelection, Start: line.start + from, End: line.start + to})
		}
		return spans
	default:
		return nil
	}
}

func searchHighlightSpans(text string, query string) []markdownSpan {
	if query == "" {
		return nil
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	textRunes := []rune(lowerText)
	queryRunes := []rune(lowerQuery)
	if len(queryRunes) == 0 || len(textRunes) < len(queryRunes) {
		return nil
	}
	spans := make([]markdownSpan, 0, 8)
	for i := 0; i+len(queryRunes) <= len(textRunes); i++ {
		if string(textRunes[i:i+len(queryRunes)]) == lowerQuery {
			spans = append(spans, markdownSpan{Tag: tagSearch, Start: i, End: i + len(queryRunes)})
		}
	}
	return spans
}

func editorRenderSpans(text string, tabSpaces int) []markdownSpan {
	render := markdownRenderFromText(text)
	spans := append([]markdownSpan(nil), render.Spans...)
	lines := strings.Split(text, "\n")
	offset := 0
	inCodeBlock := false
	codeLanguage := ""
	blockStartOffset := 0
	codeLines := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				blockText := strings.Join(codeLines, "\n")
				spans = append(spans, treeSitterSpans(blockText, blockStartOffset, codeLanguage)...)
				codeLines = codeLines[:0]
				inCodeBlock = false
				codeLanguage = ""
			} else {
				inCodeBlock = true
				codeLanguage = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				blockStartOffset = offset + len([]rune(line)) + 1
			}
			offset += len([]rune(line)) + 1
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
		}
		offset += len([]rune(line)) + 1
	}
	if inCodeBlock {
		blockText := strings.Join(codeLines, "\n")
		spans = append(spans, treeSitterSpans(blockText, blockStartOffset, codeLanguage)...)
	}
	return spans
}

func (w *Workspace) ensureEditorVisible() {
	ed := w.ActiveEditor()
	if ed == nil {
		return
	}
	cursorRow, _ := editorVisualCursor(ed, w.editorRenderWidth())
	viewportHeight := w.EditorHeight
	if viewportHeight <= 0 {
		viewportHeight = w.LastHeight - 1
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	if cursorRow < ed.ScrollTop {
		ed.ScrollTop = cursorRow
		return
	}
	if cursorRow >= ed.ScrollTop+viewportHeight {
		ed.ScrollTop = cursorRow - viewportHeight + 1
	}
	if ed.ScrollTop < 0 {
		ed.ScrollTop = 0
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

func fileName() string { return filepath.Join(notesDir(), "Note 1.md") }

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
		relPath, err := filepath.Rel(notesDir(), path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if settings.IsTrashRelativePath(relPath) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		relDir := filepath.Dir(relPath)
		if relDir == "." {
			relDir = ""
		}
		if settings.IsTrashRelativePath(relPath) || settings.IsTrashRelativePath(relDir) {
			return nil
		}
		files = append(files, noteFile{Title: noteTitleFromPath(path), Path: path, Folder: relDir, RelDir: relDir, RelPath: relPath})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return filepath.ToSlash(files[i].RelPath) < filepath.ToSlash(files[j].RelPath) })
	return files, nil
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
		if settings.IsTrashRelativePath(rel) {
			return filepath.SkipDir
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
	sort.Slice(folderList, func(i, j int) bool { return filepath.ToSlash(folderList[i]) < filepath.ToSlash(folderList[j]) })
	return folderList, nil
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
		if strings.HasPrefix(file.Title, "Note ") {
			number, err := strconv.Atoi(strings.TrimPrefix(file.Title, "Note "))
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

func sidebarTargetFolder(kind treeEntryKind, rowPath string, folder string) string {
	if kind == treeFolder {
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
