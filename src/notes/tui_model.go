package notes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
	"github.com/mattn/go-runewidth"
)

type Mode string

const (
	ModeNormal        Mode = "NORMAL"
	ModeInsert        Mode = "INSERT"
	ModeCommand       Mode = "COMMAND"
	ModeVisual        Mode = "VISUAL"
	tagSearch              = "md-search"
	tagReplaceCurrent      = "md-replace-current"
)

const yankHighlightDuration = 180 * time.Millisecond

type Key struct {
	Rune  rune
	Name  string
	Ctrl  bool
	Alt   bool
	Meta  bool
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
	treeSectionHeader treeEntryKind = iota
	treeOpenNote
	treeFolder
	treeNote
	treeManagedFolder
	treeManagedAsset
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
	Scope     string
	AssetRel  string
	Image     bool
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

type editorSnapshot struct {
	Text   string
	Cursor int
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
	NormalCount         string
	LastSearch          string
	LastSearchPos       int
	LastSearchBackward  bool
	Register            vimRegister
	SelectionMode       vimSelectionMode
	SelectionMark       int
	SelectionCursor     int
	YankHighlightStart  int
	YankHighlightEnd    int
	YankHighlightUntil  time.Time
	LastXText           string
	LastXCursor         int
	LastXArmed          bool
	AutoCompletePrefix  string
	AutoCompleteKind    string
	AutoCompleteMatches []string
	AutoCompleteIndex   int
	AutoCompleteStart   int
	AutoCompleteEnd     int
	SpellCacheText      string
	SpellCacheSpans     []markdownSpan
	SpellAsyncText      string
	SpellAsyncRunning   bool
	UndoStack           []editorSnapshot
	RedoStack           []editorSnapshot
	ReplaceConfirm      *replaceConfirmSession
}

type replaceConfirmSession struct {
	OriginalText string
	Before       editorSnapshot
	Candidates   []replaceCandidate
	Accepted     []bool
	Current      int
}

type Workspace struct {
	Tree                  []TreeEntry
	Selection             int
	BrowserTree           []TreeEntry
	BrowserSelection      int
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
	Register              vimRegister
	CurrentTab            int
	LastAccessedTab       int
	FocusSidebar          bool
	SidebarBrowsing       bool
	BrowserCommandMode    bool
	BrowserCommand        string
	SidebarWidth          int
	SidebarRenderHeight   int
	PreviewWidth          int
	EditorRenderWidth     int
	LastHeight            int
	EditorHeight          int
	SelectedFolder        string
	PreviewHidden         bool
	pendingOpenLinks      []string
	pendingRecordKeys     bool
	pendingQuit           bool
	pendingQuitForce      bool
	pendingSaveAll        bool
	pendingDeletePath     string
	pendingDeleteLabel    string
	pendingDeleteFolder   bool
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
	ws := &Workspace{
		FocusSidebar:    false,
		CurrentTab:      -1,
		LastAccessedTab: -1,
		PreviewHidden:   settings.Inst().NotesApp.PreviewHidden,
	}
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
	if !ws.restoreOpenTabs(files) && len(files) > 0 {
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

func (w *Workspace) setCurrentTab(index int) bool {
	if w == nil || index < 0 || index >= len(w.Tabs) {
		return false
	}
	if w.CurrentTab >= 0 && w.CurrentTab < len(w.Tabs) && w.CurrentTab != index {
		w.LastAccessedTab = w.CurrentTab
	}
	w.CurrentTab = index
	w.syncOpenSelectionToActive()
	return true
}

func (w *Workspace) refreshTree() {
	selectedPath := ""
	if entry := w.selectedOpenEntry(); entry != nil {
		selectedPath = entry.Path
	}
	entries := make([]TreeEntry, 0, len(w.Tabs))
	for _, tab := range w.Tabs {
		if tab == nil || strings.TrimSpace(tab.Path) == "" {
			continue
		}
		entries = append(entries, TreeEntry{
			Kind:   treeOpenNote,
			Path:   tab.Path,
			Label:  tab.Title,
			Folder: relativeNoteFolder(tab.Path),
		})
	}
	w.Tree = entries
	w.Selection = clampSidebarSelectionByPath(w.Tree, w.Selection, selectedPath)
	w.refreshBrowserTree()
}

func (w *Workspace) refreshBrowserTree() {
	files, _ := listNoteFiles()
	folders, _ := listNoteFolders()
	managedEntries, _ := listManagedFiles()
	selectedKind := treeSectionHeader
	selectedPath := ""
	selectedFolder := ""
	selectedLabel := ""
	if entry := w.selectedBrowserEntry(); entry != nil {
		selectedKind = entry.Kind
		selectedPath = entry.Path
		selectedFolder = entry.Folder
		selectedLabel = entry.Label
	}
	collapsed := make(map[string]bool)
	managedCollapsed := make(map[string]bool)
	for _, entry := range w.BrowserTree {
		if entry.Kind == treeFolder && entry.Collapsed {
			collapsed[entry.Folder] = true
		}
		if entry.Kind == treeManagedFolder && entry.Collapsed {
			managedCollapsed[entry.Path] = true
		}
	}
	baseEntries := make([]TreeEntry, 0, len(files)+len(folders))
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
		baseEntries = append(baseEntries, TreeEntry{Kind: treeFolder, Path: noteFolderPath(folder), Label: filepath.Base(folder), Depth: strings.Count(filepath.ToSlash(folder), "/"), Folder: folder, Collapsed: collapsed[folder]})
	}
	for _, file := range files {
		if isHiddenByCollapsed(file.Folder, collapsed) {
			continue
		}
		baseEntries = append(baseEntries, TreeEntry{Kind: treeNote, Path: file.Path, Label: file.Title, Depth: folderDepth(file.Folder), Folder: file.Folder})
	}
	sort.SliceStable(baseEntries, func(i, j int) bool {
		left := treeSortKey(baseEntries[i])
		right := treeSortKey(baseEntries[j])
		return left < right
	})
	managedByScope := browserManagedEntriesByScope(managedEntries, managedCollapsed)
	entries := make([]TreeEntry, 0, len(baseEntries)+len(managedEntries))
	for _, entry := range baseEntries {
		entries = append(entries, entry)
		if entry.Kind != treeNote {
			continue
		}
		entries = append(entries, managedByScope[noteRelPath(entry.Path)]...)
	}
	w.BrowserTree = entries
	for i, entry := range w.BrowserTree {
		if entry.Kind != selectedKind {
			continue
		}
		switch {
		case selectedPath != "" && entry.Path == selectedPath:
			w.BrowserSelection = i
		case selectedFolder != "" && entry.Folder == selectedFolder && entry.Path == selectedPath:
			w.BrowserSelection = i
		case selectedPath == "" && selectedFolder == "" && selectedLabel != "" && entry.Label == selectedLabel:
			w.BrowserSelection = i
		default:
			continue
		}
		break
	}
	if w.BrowserSelection >= len(w.BrowserTree) {
		w.BrowserSelection = len(w.BrowserTree) - 1
	}
	if w.BrowserSelection < 0 {
		w.BrowserSelection = 0
	}
}

func clampSidebarSelectionByPath(entries []TreeEntry, selection int, path string) int {
	if len(entries) == 0 {
		return 0
	}
	if path != "" {
		for i, entry := range entries {
			if entry.Path == path {
				return i
			}
		}
	}
	if selection >= len(entries) {
		selection = len(entries) - 1
	}
	if selection < 0 {
		selection = 0
	}
	return selection
}

func clampBrowserSelectionByFolder(entries []TreeEntry, selection int, folder string) int {
	if len(entries) == 0 {
		return 0
	}
	if folder != "" {
		for i, entry := range entries {
			if entry.Kind == treeFolder && entry.Folder == folder {
				return i
			}
		}
	}
	if selection >= len(entries) {
		selection = len(entries) - 1
	}
	if selection < 0 {
		selection = 0
	}
	return selection
}

func browserManagedEntriesByScope(files []FileEntry, collapsed map[string]bool) map[string][]TreeEntry {
	byScope := make(map[string][]TreeEntry)
	for _, file := range files {
		if file.Kind == fileEntryScope {
			continue
		}
		if isManagedHiddenByCollapsed(file.Path, collapsed) {
			continue
		}
		depth := folderDepth(file.Scope) + 1 + strings.Count(filepath.ToSlash(file.AssetRel), "/")
		kind := treeManagedAsset
		if file.Kind == fileEntryFolder {
			kind = treeManagedFolder
		}
		byScope[file.Scope] = append(byScope[file.Scope], TreeEntry{
			Kind:      kind,
			Path:      file.Path,
			Label:     file.Label,
			Depth:     depth,
			Collapsed: collapsed[file.Path],
			Scope:     file.Scope,
			AssetRel:  file.AssetRel,
			Image:     file.Image,
		})
	}
	return byScope
}

func isManagedHiddenByCollapsed(path string, collapsed map[string]bool) bool {
	for parent, isCollapsed := range collapsed {
		if !isCollapsed || parent == path {
			continue
		}
		if strings.HasPrefix(path, parent+string(filepath.Separator)) {
			return true
		}
	}
	return false
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
			w.setCurrentTab(i)
			w.SelectedFolder = relativeNoteFolder(path)
			w.refreshTree()
			w.syncOpenSelectionToActive()
			w.updateSession()
			return nil
		}
	}
	text, err := readNoteFile(path)
	if err != nil {
		return err
	}
	ed := &Editor{Path: path, Title: noteTitleFromPath(path), Text: text, Mode: defaultEditorMode(), LastSearchPos: -1}
	w.Tabs = append(w.Tabs, ed)
	w.setCurrentTab(len(w.Tabs) - 1)
	w.SelectedFolder = relativeNoteFolder(path)
	w.refreshTree()
	w.syncOpenSelectionToActive()
	w.persistSession()
	return nil
}

func (w *Workspace) Refresh() {
	if w == nil {
		return
	}
	w.refreshTree()
	w.refreshFiles()
	w.syncOpenSelectionToActive()
}

func (w *Workspace) OpenFolderNotes(folder string) error {
	if w == nil {
		return fmt.Errorf("no workspace")
	}
	folder = sanitizeFolderPath(folder)
	if folder == "" {
		return fmt.Errorf("select a folder")
	}
	files, err := listNoteFiles()
	if err != nil {
		return err
	}
	opened := 0
	for _, file := range files {
		if !folderContainsNote(folder, file.Folder) {
			continue
		}
		if err := w.Open(file.Path); err != nil {
			return err
		}
		opened++
	}
	if opened == 0 {
		if ed := w.ActiveEditor(); ed != nil {
			ed.Status = "no notes in folder"
		}
		return nil
	}
	w.leaveSidebar()
	w.ensureEditorVisible()
	return nil
}

func defaultEditorMode() Mode {
	if settings.Inst().NotesApp.VimMode {
		return ModeNormal
	}
	return ModeInsert
}

func (w *Workspace) persistSession() {
	paths, current := w.sessionState()
	settings.SaveNotesSession(paths, current)
}

func (w *Workspace) updateSession() {
	paths, current := w.sessionState()
	settings.UpdateNotesSession(paths, current)
}

func (w *Workspace) sessionState() ([]string, string) {
	if w == nil {
		return nil, ""
	}
	paths := make([]string, 0, len(w.Tabs))
	current := ""
	for i, tab := range w.Tabs {
		if tab == nil || strings.TrimSpace(tab.Path) == "" {
			continue
		}
		paths = append(paths, tab.Path)
		if i == w.CurrentTab {
			current = tab.Path
		}
	}
	return paths, current
}

func (w *Workspace) restoreOpenTabs(files []noteFile) bool {
	if w == nil {
		return false
	}
	available := make(map[string]struct{}, len(files))
	for _, file := range files {
		available[file.Path] = struct{}{}
	}
	session := settings.Inst().NotesApp
	restored := false
	for _, stored := range session.OpenNotePaths {
		path := resolveSessionNotePath(stored)
		if _, ok := available[path]; !ok {
			continue
		}
		if err := w.Open(path); err == nil {
			restored = true
		}
	}
	if !restored {
		return false
	}
	currentPath := resolveSessionNotePath(session.CurrentNotePath)
	if currentPath != "" {
		for i, tab := range w.Tabs {
			if tab != nil && tab.Path == currentPath {
				w.CurrentTab = i
				break
			}
		}
	}
	w.persistSession()
	return true
}

func resolveSessionNotePath(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return ""
	}
	if !filepath.IsAbs(stored) {
		return filepath.Join(notesDir(), filepath.FromSlash(stored))
	}
	if rel, err := filepath.Rel(notesDir(), stored); err == nil && rel != "." && rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.Join(notesDir(), rel)
	}
	marker := "/" + filepath.ToSlash(filepath.Join(helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes")) + "/"
	slashPath := filepath.ToSlash(filepath.Clean(stored))
	if idx := strings.Index(slashPath, marker); idx >= 0 {
		rel := slashPath[idx+len(marker):]
		rel = filepath.Clean(filepath.FromSlash(rel))
		if rel != "." && rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.Join(notesDir(), rel)
		}
	}
	return filepath.Clean(stored)
}

func currentEditorSnapshot(ed *Editor) editorSnapshot {
	return editorSnapshot{
		Text:   ed.Text,
		Cursor: ed.Cursor,
	}
}

func undoLimit() int {
	limit := settings.Inst().NotesApp.UndoLevels
	if limit <= 0 {
		return 1000
	}
	return limit
}

func trimSnapshots(snaps []editorSnapshot, limit int) []editorSnapshot {
	if limit <= 0 || len(snaps) <= limit {
		return snaps
	}
	return append([]editorSnapshot(nil), snaps[len(snaps)-limit:]...)
}

func rememberUndoState(ed *Editor) {
	if ed == nil {
		return
	}
	snapshot := currentEditorSnapshot(ed)
	if n := len(ed.UndoStack); n > 0 {
		last := ed.UndoStack[n-1]
		if last.Text == snapshot.Text && last.Cursor == snapshot.Cursor {
			ed.RedoStack = nil
			return
		}
	}
	ed.UndoStack = append(ed.UndoStack, snapshot)
	ed.UndoStack = trimSnapshots(ed.UndoStack, undoLimit())
	ed.RedoStack = nil
}

func applyUndo(ed *Editor) bool {
	if ed == nil || len(ed.UndoStack) == 0 {
		if ed != nil {
			ed.Status = "nothing to undo"
		}
		return false
	}
	ed.RedoStack = append(ed.RedoStack, currentEditorSnapshot(ed))
	ed.RedoStack = trimSnapshots(ed.RedoStack, undoLimit())
	snapshot := ed.UndoStack[len(ed.UndoStack)-1]
	ed.UndoStack = ed.UndoStack[:len(ed.UndoStack)-1]
	ed.Text = snapshot.Text
	ed.Cursor = snapshot.Cursor
	ed.Dirty = true
	ed.Status = "undo"
	clearAutoComplete(ed)
	clearVisualSelection(ed)
	ed.PendingOp = ""
	return true
}

func applyRedo(ed *Editor) bool {
	if ed == nil || len(ed.RedoStack) == 0 {
		if ed != nil {
			ed.Status = "nothing to redo"
		}
		return false
	}
	ed.UndoStack = append(ed.UndoStack, currentEditorSnapshot(ed))
	ed.UndoStack = trimSnapshots(ed.UndoStack, undoLimit())
	snapshot := ed.RedoStack[len(ed.RedoStack)-1]
	ed.RedoStack = ed.RedoStack[:len(ed.RedoStack)-1]
	ed.Text = snapshot.Text
	ed.Cursor = snapshot.Cursor
	ed.Dirty = true
	ed.Status = "redo"
	clearAutoComplete(ed)
	clearVisualSelection(ed)
	ed.PendingOp = ""
	return true
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
	w.setCurrentTab((w.CurrentTab + 1) % len(w.Tabs))
	w.updateSession()
	return true
}

func (w *Workspace) PrevTab() bool {
	if w == nil || len(w.Tabs) <= 1 {
		return false
	}
	next := w.CurrentTab - 1
	if next < 0 {
		next = len(w.Tabs) - 1
	}
	w.setCurrentTab(next)
	w.updateSession()
	return true
}

func (w *Workspace) SwitchToLastAccessedTab() bool {
	if w == nil || w.LastAccessedTab < 0 || w.LastAccessedTab >= len(w.Tabs) || w.LastAccessedTab == w.CurrentTab {
		return false
	}
	w.setCurrentTab(w.LastAccessedTab)
	w.leaveSidebar()
	w.ensureEditorVisible()
	w.updateSession()
	return true
}

func (w *Workspace) SwitchToTabShortcut(shortcut string) bool {
	index, ok := noteTabShortcutIndex(shortcut)
	if !ok || w == nil || index < 0 || index >= len(w.Tabs) {
		return false
	}
	w.setCurrentTab(index)
	w.leaveSidebar()
	w.ensureEditorVisible()
	w.updateSession()
	return true
}

func (w *Workspace) SwitchToTabAtColumn(col int) bool {
	index, ok := w.tabIndexAtColumn(col)
	if !ok {
		return false
	}
	w.setCurrentTab(index)
	w.leaveSidebar()
	w.ensureEditorVisible()
	w.updateSession()
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
	w.leaveSidebar()
	return true
}

func (w *Workspace) DeleteCurrentNote() bool {
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	return w.DeleteNoteByPath(ed.Path) == nil
}

func (w *Workspace) CanDeleteFocusedNote() bool {
	if w == nil {
		return false
	}
	if w.FocusSidebar {
		entry := w.selectedEntry()
		return entry != nil && (entry.Kind == treeNote || entry.Kind == treeOpenNote)
	}
	return w.ActiveEditor() != nil
}

func (w *Workspace) CanDeleteFocusedBrowserEntry() bool {
	if w == nil || !w.SidebarBrowsing {
		return false
	}
	entry := w.selectedBrowserEntry()
	return entry != nil && (entry.Kind == treeNote || entry.Kind == treeFolder)
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

func (w *Workspace) FocusedBrowserDeleteLabel() string {
	if !w.CanDeleteFocusedBrowserEntry() {
		return ""
	}
	return w.selectedBrowserEntry().Label
}

func (w *Workspace) requestDeleteFocusedNote() bool {
	path := w.FocusedNoteDeletePath()
	if strings.TrimSpace(path) == "" {
		return false
	}
	w.pendingDeletePath = path
	w.pendingDeleteLabel = w.FocusedNoteDeleteLabel()
	w.pendingDeleteFolder = false
	return true
}

func (w *Workspace) requestDeleteFocusedBrowserEntry() bool {
	entry := w.selectedBrowserEntry()
	if entry == nil {
		return false
	}
	switch entry.Kind {
	case treeNote:
		w.pendingDeletePath = entry.Path
		w.pendingDeleteLabel = entry.Label
		w.pendingDeleteFolder = false
		return true
	case treeFolder:
		w.pendingDeletePath = entry.Folder
		w.pendingDeleteLabel = entry.Label
		w.pendingDeleteFolder = true
		return true
	default:
		return false
	}
}

func (w *Workspace) FocusedNoteDeletePath() string {
	if !w.CanDeleteFocusedNote() {
		return ""
	}
	if w.FocusSidebar {
		entry := w.selectedEntry()
		if entry == nil || (entry.Kind != treeNote && entry.Kind != treeOpenNote) {
			return ""
		}
		return entry.Path
	}
	if ed := w.ActiveEditor(); ed != nil {
		return ed.Path
	}
	return ""
}

func (w *Workspace) DeleteFocusedNote() bool {
	return w.DeleteNoteByPath(w.FocusedNoteDeletePath()) == nil
}

func (w *Workspace) TakePendingDeleteNote() (string, string, bool) {
	path, label, folder, ok := w.TakePendingDeleteTarget()
	if !ok || folder {
		return "", "", false
	}
	return path, label, true
}

func (w *Workspace) TakePendingDeleteTarget() (string, string, bool, bool) {
	if w == nil || strings.TrimSpace(w.pendingDeletePath) == "" {
		return "", "", false, false
	}
	path := w.pendingDeletePath
	label := w.pendingDeleteLabel
	folder := w.pendingDeleteFolder
	w.pendingDeletePath = ""
	w.pendingDeleteLabel = ""
	w.pendingDeleteFolder = false
	return path, label, folder, true
}

func (w *Workspace) DeleteNoteByPath(path string) error {
	if w == nil || strings.TrimSpace(path) == "" {
		return fmt.Errorf("no note selected")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		if ed := w.ActiveEditor(); ed != nil {
			ed.Status = err.Error()
		}
		return err
	}
	w.closeTab(path)
	w.refreshTree()
	if w.CurrentTab >= 0 && w.CurrentTab < len(w.Tabs) {
		w.SelectedFolder = relativeNoteFolder(w.Tabs[w.CurrentTab].Path)
	}
	w.refreshFiles()
	w.persistSession()
	return nil
}

func (w *Workspace) DeleteFolderByRel(folder string) error {
	if w == nil || strings.TrimSpace(folder) == "" {
		return fmt.Errorf("no folder selected")
	}
	folder = sanitizeFolderPath(folder)
	if folder == "" {
		return fmt.Errorf("no folder selected")
	}
	if err := os.RemoveAll(noteFolderPath(folder)); err != nil {
		return err
	}
	w.closeTabsInFolder(folder)
	w.SelectedFolder = ""
	if ed := w.ActiveEditor(); ed != nil {
		w.SelectedFolder = relativeNoteFolder(ed.Path)
	}
	w.refreshTree()
	w.refreshFiles()
	return nil
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
	w.persistSession()
	return nil
}

func (w *Workspace) RenameBrowserEntry(name string) error {
	entry := w.selectedBrowserEntry()
	if entry == nil {
		return fmt.Errorf("select a note or folder")
	}
	name = strings.TrimSpace(name)
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	}
	switch entry.Kind {
	case treeNote:
		return w.renameNoteByPath(entry.Path, name)
	case treeFolder:
		return w.renameFolderByRel(entry.Folder, name)
	default:
		return fmt.Errorf("select a note or folder")
	}
}

func (w *Workspace) MoveBrowserEntry(target string) error {
	entry := w.selectedBrowserEntry()
	if entry == nil {
		return fmt.Errorf("select a note or folder")
	}
	switch entry.Kind {
	case treeNote:
		return w.moveNoteByPath(entry.Path, target)
	case treeFolder:
		return w.moveFolderByRel(entry.Folder, target)
	default:
		return fmt.Errorf("select a note or folder")
	}
}

func (w *Workspace) renameNoteByPath(path string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("rename requires a note name")
	}
	folder := relativeNoteFolder(path)
	target := uniqueNotePathInFolder(folder, name, path)
	if target == path {
		return nil
	}
	oldAssetPath := noteAssetsPath(path)
	newAssetPath := noteAssetsPath(target)
	if err := os.Rename(path, target); err != nil {
		return err
	}
	if _, err := os.Stat(oldAssetPath); err == nil {
		renamedAssetPath := uniquePathLike(newAssetPath, oldAssetPath, true)
		if renameErr := os.Rename(oldAssetPath, renamedAssetPath); renameErr != nil {
			return renameErr
		}
	}
	for _, tab := range w.Tabs {
		if tab == nil || tab.Path != path {
			continue
		}
		tab.Path = target
		tab.Title = noteTitleFromPath(target)
	}
	w.SelectedFolder = relativeNoteFolder(target)
	w.refreshTree()
	w.refreshFiles()
	w.BrowserSelection = clampSidebarSelectionByPath(w.BrowserTree, w.BrowserSelection, target)
	w.persistSession()
	return nil
}

func (w *Workspace) moveNoteByPath(path string, rawTarget string) error {
	targetFolder, targetTitle, err := resolveNoteMoveTarget(rawTarget, path)
	if err != nil {
		return err
	}
	target := uniqueNotePathInFolder(targetFolder, targetTitle, path)
	if target == "" {
		return fmt.Errorf("move requires a note target")
	}
	if target == path {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	oldAssetPath := noteAssetsPath(path)
	newAssetPath := noteAssetsPath(target)
	if err := os.Rename(path, target); err != nil {
		return err
	}
	if _, err := os.Stat(oldAssetPath); err == nil {
		renamedAssetPath := uniquePathLike(newAssetPath, oldAssetPath, true)
		if renameErr := os.Rename(oldAssetPath, renamedAssetPath); renameErr != nil {
			return renameErr
		}
	}
	for _, tab := range w.Tabs {
		if tab == nil || tab.Path != path {
			continue
		}
		tab.Path = target
		tab.Title = noteTitleFromPath(target)
	}
	w.SelectedFolder = relativeNoteFolder(target)
	w.expandBrowserAncestors(w.SelectedFolder)
	w.refreshTree()
	w.refreshFiles()
	w.BrowserSelection = clampSidebarSelectionByPath(w.BrowserTree, w.BrowserSelection, target)
	w.persistSession()
	return nil
}

func (w *Workspace) renameFolderByRel(folder string, name string) error {
	folder = sanitizeFolderPath(folder)
	name = strings.TrimSpace(name)
	if folder == "" || name == "" {
		return fmt.Errorf("rename requires a folder name")
	}
	parent := filepath.Dir(folder)
	if parent == "." {
		parent = ""
	}
	targetFolder := joinFolderParts(parent, name)
	if targetFolder == "" {
		return fmt.Errorf("rename requires a folder name")
	}
	oldPath := noteFolderPath(folder)
	targetPath := uniquePathLike(noteFolderPath(targetFolder), oldPath, true)
	if targetPath == oldPath {
		return nil
	}
	if err := os.Rename(oldPath, targetPath); err != nil {
		return err
	}
	actualRel, err := filepath.Rel(notesDir(), targetPath)
	if err != nil {
		actualRel = targetFolder
	}
	actualRel = sanitizeFolderPath(actualRel)
	for _, tab := range w.Tabs {
		if tab == nil {
			continue
		}
		rel := noteRelPath(tab.Path)
		if rel == "" {
			continue
		}
		if rel == folder || strings.HasPrefix(rel, folder+string(filepath.Separator)) {
			suffix := strings.TrimPrefix(rel, folder)
			suffix = strings.TrimPrefix(suffix, string(filepath.Separator))
			tab.Path = filepath.Join(noteFolderPath(actualRel), suffix)
			tab.Title = noteTitleFromPath(tab.Path)
		}
	}
	w.SelectedFolder = actualRel
	w.refreshTree()
	w.refreshFiles()
	w.BrowserSelection = clampBrowserSelectionByFolder(w.BrowserTree, w.BrowserSelection, actualRel)
	w.persistSession()
	return nil
}

func (w *Workspace) moveFolderByRel(folder string, rawTarget string) error {
	folder = sanitizeFolderPath(folder)
	targetParent, err := resolveFolderMoveTarget(rawTarget)
	if err != nil {
		return err
	}
	if folder == "" {
		return fmt.Errorf("move requires a folder")
	}
	if targetParent == folder || strings.HasPrefix(targetParent, folder+string(filepath.Separator)) {
		return fmt.Errorf("cannot move folder into itself")
	}
	targetFolder := joinFolderParts(targetParent, filepath.Base(folder))
	if targetFolder == "" {
		return fmt.Errorf("move requires a folder target")
	}
	oldPath := noteFolderPath(folder)
	targetPath := uniquePathLike(noteFolderPath(targetFolder), oldPath, true)
	if targetPath == oldPath {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(oldPath, targetPath); err != nil {
		return err
	}
	actualRel, err := filepath.Rel(notesDir(), targetPath)
	if err != nil {
		actualRel = targetFolder
	}
	actualRel = sanitizeFolderPath(actualRel)
	for _, tab := range w.Tabs {
		if tab == nil {
			continue
		}
		rel := noteRelPath(tab.Path)
		if rel == "" {
			continue
		}
		if rel == folder || strings.HasPrefix(rel, folder+string(filepath.Separator)) {
			suffix := strings.TrimPrefix(rel, folder)
			suffix = strings.TrimPrefix(suffix, string(filepath.Separator))
			tab.Path = filepath.Join(noteFolderPath(actualRel), suffix)
			tab.Title = noteTitleFromPath(tab.Path)
		}
	}
	w.SelectedFolder = actualRel
	w.expandBrowserAncestors(actualRel)
	w.refreshTree()
	w.refreshFiles()
	w.BrowserSelection = clampBrowserSelectionByFolder(w.BrowserTree, w.BrowserSelection, actualRel)
	w.persistSession()
	return nil
}

func (w *Workspace) HandleKey(key Key) bool {
	if key.Ctrl && key.Name == "s" {
		w.pendingSaveAll = true
		return true
	}
	if key.Ctrl && key.Name == "a" {
		w.FocusSidebar = !w.FocusSidebar
		if !w.FocusSidebar {
			w.SidebarBrowsing = false
		} else if !w.SidebarBrowsing {
			w.syncOpenSelectionToActive()
		}
		return true
	}
	if key.Ctrl && !w.FocusSidebar && key.Name == "e" {
		ed := w.ActiveEditor()
		if ed != nil {
			key = Key{Name: "right", Meta: true}
			handled := w.handleEditorKey(key)
			if handled {
				w.ensureEditorVisible()
			}
			return handled
		}
	}
	if key.Ctrl && key.Name == "n" {
		return w.NewNote()
	}
	if key.Ctrl && key.Name == "d" {
		if w.SidebarBrowsing {
			return w.requestDeleteFocusedBrowserEntry()
		}
		return w.requestDeleteFocusedNote()
	}
	if key.Ctrl && key.Name == "left" && w.FocusSidebar {
		if w.SidebarWidth > 20 {
			w.SidebarWidth--
			settings.SaveNotesEditorWidth(w.SidebarWidth)
		}
		return true
	}
	if key.Ctrl && key.Name == "right" && w.FocusSidebar {
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
	if w.SidebarBrowsing && w.BrowserCommandMode {
		return w.handleBrowserCommandKey(key)
	}
	switch key.Name {
	case "a":
		if w.SwitchToLastAccessedTab() {
			return true
		}
		w.leaveSidebar()
		return true
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if w.SwitchToTabShortcut(key.Name) {
			return true
		}
		w.leaveSidebar()
		return true
	case "e":
		w.toggleSidebarBrowser()
		return true
	case "h", "esc":
		if w.SidebarBrowsing {
			w.SidebarBrowsing = false
			w.BrowserCommandMode = false
			w.BrowserCommand = ""
			return true
		}
		w.leaveSidebar()
		return true
	case "n":
		if w.SidebarBrowsing {
			w.startBrowserCommand("new ")
			return true
		}
		_, _ = w.CreateNote("")
		return true
	case "f":
		if w.SidebarBrowsing {
			w.startBrowserCommand("new ")
			return true
		}
		_ = w.CreateFolder("")
		return true
	}
	if len(w.activeSidebarEntries()) == 0 {
		return false
	}
	switch key.Name {
	case "[":
		return w.PrevTab()
	case "]":
		return w.NextTab()
	case "down", "j":
		w.moveSidebarSelection(1)
		return true
	case "up", "k":
		w.moveSidebarSelection(-1)
		return true
	case "enter", "l":
		entry := w.selectedEntry()
		if entry == nil {
			return false
		}
		if w.SidebarBrowsing && entry.Kind == treeFolder {
			for i := range w.BrowserTree {
				if w.BrowserTree[i].Kind == treeFolder && w.BrowserTree[i].Folder == entry.Folder {
					w.BrowserTree[i].Collapsed = !w.BrowserTree[i].Collapsed
					break
				}
			}
			w.refreshBrowserTree()
			return true
		}
		if w.SidebarBrowsing && entry.Kind == treeManagedFolder {
			for i := range w.BrowserTree {
				if w.BrowserTree[i].Kind == treeManagedFolder && w.BrowserTree[i].Path == entry.Path {
					w.BrowserTree[i].Collapsed = !w.BrowserTree[i].Collapsed
					break
				}
			}
			w.refreshBrowserTree()
			return true
		}
		if w.SidebarBrowsing && entry.Kind == treeManagedAsset {
			helpers.OpenURI(pathToFileURI(entry.Path))
			return true
		}
		_ = w.Open(entry.Path)
		w.leaveSidebar()
		w.ensureEditorVisible()
		return true
	case "o":
		if !w.SidebarBrowsing {
			return false
		}
		entry := w.selectedBrowserEntry()
		if entry == nil || entry.Kind != treeFolder {
			return false
		}
		if err := w.OpenFolderNotes(entry.Folder); err != nil {
			if ed := w.ActiveEditor(); ed != nil {
				ed.Status = err.Error()
			}
		}
		return true
	case "d":
		if w.SidebarBrowsing {
			return w.requestDeleteFocusedBrowserEntry()
		}
		return w.requestDeleteFocusedNote()
	case "m":
		if w.SidebarBrowsing {
			entry := w.selectedBrowserEntry()
			if entry == nil || (entry.Kind != treeNote && entry.Kind != treeFolder) {
				return false
			}
			w.startBrowserCommand("move ")
			return true
		}
		return false
	case "x":
		entry := w.selectedEntry()
		if w.SidebarBrowsing || entry == nil || entry.Kind != treeOpenNote {
			return false
		}
		return w.CloseNoteByPath(entry.Path)
	case "r", "R":
		if w.SidebarBrowsing {
			entry := w.selectedBrowserEntry()
			if entry == nil || (entry.Kind != treeNote && entry.Kind != treeFolder) {
				return false
			}
			w.startBrowserCommand("rename " + entry.Label)
			return true
		}
		if key.Name != "R" {
			return false
		}
		ed := w.ActiveEditor()
		if ed == nil {
			return false
		}
		w.leaveSidebar()
		ed.Mode = ModeCommand
		ed.Command = "rename " + ed.Title
		return true
	}
	return false
}

func (w *Workspace) startBrowserCommand(command string) {
	if w == nil {
		return
	}
	w.BrowserCommandMode = true
	w.BrowserCommand = command
}

func (w *Workspace) handleBrowserCommandKey(key Key) bool {
	switch key.Name {
	case "esc":
		w.BrowserCommandMode = false
		w.BrowserCommand = ""
		return true
	case "backspace":
		if len(w.BrowserCommand) > 0 {
			_, size := utf8.DecodeLastRuneInString(w.BrowserCommand)
			w.BrowserCommand = w.BrowserCommand[:len(w.BrowserCommand)-size]
		}
		return true
	case "enter":
		err := w.executeBrowserCommand(strings.TrimSpace(w.BrowserCommand))
		w.BrowserCommandMode = false
		w.BrowserCommand = ""
		if err != nil {
			if ed := w.ActiveEditor(); ed != nil {
				ed.Status = err.Error()
			}
		}
		return true
	}
	if key.Rune != 0 {
		w.BrowserCommand += string(key.Rune)
		return true
	}
	return false
}

func (w *Workspace) executeBrowserCommand(command string) error {
	switch {
	case strings.HasPrefix(command, "new "):
		return w.CreateBrowserTarget(strings.TrimSpace(strings.TrimPrefix(command, "new ")))
	case strings.HasPrefix(command, "rename "):
		return w.RenameBrowserEntry(strings.TrimSpace(strings.TrimPrefix(command, "rename ")))
	case strings.HasPrefix(command, "move "):
		return w.MoveBrowserEntry(strings.TrimSpace(strings.TrimPrefix(command, "move ")))
	case command == "":
		return nil
	default:
		return fmt.Errorf("unknown browser command: %s", command)
	}
}

func (w *Workspace) CreateNote(title string) (string, error) {
	folder := w.SelectedFolder
	if entry := w.selectedEntry(); entry != nil {
		folder = sidebarTargetFolder(entry)
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
		parent = sidebarTargetFolder(entry)
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

func (w *Workspace) CreateBrowserTarget(raw string) error {
	if w == nil {
		return fmt.Errorf("no workspace")
	}
	target := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if target == "" {
		return fmt.Errorf("new requires a note or folder name")
	}
	isFolder := strings.HasSuffix(target, "/")
	rootRelative := strings.HasPrefix(target, "/")
	target = strings.Trim(target, "/")
	if target == "" {
		return fmt.Errorf("new requires a note or folder name")
	}

	parent := ""
	if !rootRelative {
		parent = browserTargetFolder(w.selectedBrowserEntry())
	}
	parts := strings.Split(target, "/")
	if isFolder {
		folder := joinFolderParts(parent, strings.Join(parts, "/"))
		if folder == "" {
			return fmt.Errorf("new requires a folder name")
		}
		if err := os.MkdirAll(noteFolderPath(folder), 0o755); err != nil {
			return err
		}
		w.SelectedFolder = folder
		w.expandBrowserAncestors(folder)
		w.refreshTree()
		w.BrowserSelection = clampBrowserSelectionByFolder(w.BrowserTree, w.BrowserSelection, folder)
		w.refreshFiles()
		return nil
	}

	title := strings.TrimSuffix(parts[len(parts)-1], ".md")
	folder := parent
	if len(parts) > 1 {
		folder = joinFolderParts(parent, strings.Join(parts[:len(parts)-1], "/"))
	}
	path := uniqueNotePathInFolder(folder, title, "")
	if path == "" {
		return fmt.Errorf("new requires a note name")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return err
	}
	w.SelectedFolder = folder
	w.expandBrowserAncestors(folder)
	w.refreshTree()
	w.refreshFiles()
	if err := w.Open(path); err != nil {
		return err
	}
	w.leaveSidebar()
	w.ensureEditorVisible()
	return nil
}

func (w *Workspace) expandBrowserAncestors(folder string) {
	if w == nil || folder == "" {
		return
	}
	folder = filepath.Clean(folder)
	for i := range w.BrowserTree {
		if w.BrowserTree[i].Kind != treeFolder {
			continue
		}
		entryFolder := filepath.Clean(w.BrowserTree[i].Folder)
		if entryFolder == "." || entryFolder == "" {
			continue
		}
		if folder == entryFolder || strings.HasPrefix(folder, entryFolder+string(filepath.Separator)) {
			w.BrowserTree[i].Collapsed = false
		}
	}
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
		if err := w.DeleteNoteByPath(entry.Path); err != nil {
			return err
		}
	}
	w.refreshTree()
	w.refreshFiles()
	return nil
}

func (w *Workspace) closeTabsInFolder(folder string) {
	kept := w.Tabs[:0]
	for _, tab := range w.Tabs {
		if !folderContainsNote(folder, relativeNoteFolder(tab.Path)) {
			kept = append(kept, tab)
		}
	}
	w.Tabs = kept
	w.normalizeTabSelectionAfterRemoval(-1)
	w.refreshTree()
	w.syncOpenSelectionToActive()
	w.persistSession()
}

func folderContainsNote(folder string, noteFolder string) bool {
	folder = filepath.Clean(folder)
	noteFolder = filepath.Clean(noteFolder)
	return noteFolder == folder || strings.HasPrefix(noteFolder, folder+string(filepath.Separator))
}

func (w *Workspace) closeTab(path string) {
	index := -1
	for i, tab := range w.Tabs {
		if tab != nil && tab.Path == path {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	w.Tabs = append(w.Tabs[:index], w.Tabs[index+1:]...)
	w.normalizeTabSelectionAfterRemoval(index)
	w.refreshTree()
	w.syncOpenSelectionToActive()
	w.persistSession()
}

func (w *Workspace) CloseNoteByPath(path string) bool {
	if w == nil || strings.TrimSpace(path) == "" {
		return false
	}
	w.closeTab(path)
	if ed := w.ActiveEditor(); ed != nil {
		w.SelectedFolder = relativeNoteFolder(ed.Path)
	}
	w.refreshFiles()
	return true
}

func (w *Workspace) CloseCurrentNote() bool {
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	return w.CloseNoteByPath(ed.Path)
}

func (w *Workspace) normalizeTabSelectionAfterRemoval(removed int) {
	if w == nil {
		return
	}
	if len(w.Tabs) == 0 {
		w.CurrentTab = -1
		w.LastAccessedTab = -1
		return
	}
	if removed >= 0 {
		switch {
		case removed < w.CurrentTab:
			w.CurrentTab--
		case removed == w.CurrentTab:
			switch {
			case w.LastAccessedTab >= 0 && w.LastAccessedTab < len(w.Tabs):
				w.CurrentTab = w.LastAccessedTab
			case removed >= len(w.Tabs):
				w.CurrentTab = len(w.Tabs) - 1
			default:
				w.CurrentTab = removed
			}
		}
	}
	if w.CurrentTab < 0 || w.CurrentTab >= len(w.Tabs) {
		w.CurrentTab = min(max(w.CurrentTab, 0), len(w.Tabs)-1)
	}
	switch {
	case len(w.Tabs) <= 1:
		w.LastAccessedTab = -1
	case removed >= 0 && removed < w.LastAccessedTab:
		w.LastAccessedTab--
	case removed == w.LastAccessedTab || w.LastAccessedTab >= len(w.Tabs) || w.LastAccessedTab == w.CurrentTab:
		w.LastAccessedTab = -1
	}
}

func (w *Workspace) selectedEntry() *TreeEntry {
	entries := w.activeSidebarEntries()
	selection := w.activeSidebarSelection()
	if selection < 0 || selection >= len(entries) {
		return nil
	}
	return &entries[selection]
}

func (w *Workspace) selectedOpenEntry() *TreeEntry {
	if w == nil || w.Selection < 0 || w.Selection >= len(w.Tree) {
		return nil
	}
	return &w.Tree[w.Selection]
}

func (w *Workspace) selectedBrowserEntry() *TreeEntry {
	if w == nil || w.BrowserSelection < 0 || w.BrowserSelection >= len(w.BrowserTree) {
		return nil
	}
	return &w.BrowserTree[w.BrowserSelection]
}

func (w *Workspace) activeSidebarEntries() []TreeEntry {
	if w != nil && w.SidebarBrowsing {
		return w.BrowserTree
	}
	if w == nil {
		return nil
	}
	return w.Tree
}

func (w *Workspace) activeSidebarSelection() int {
	if w != nil && w.SidebarBrowsing {
		return w.BrowserSelection
	}
	if w == nil {
		return 0
	}
	return w.Selection
}

func (w *Workspace) syncOpenSelectionToActive() {
	if w == nil {
		return
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return
	}
	w.Selection = clampSidebarSelectionByPath(w.Tree, w.Selection, ed.Path)
}

func (w *Workspace) moveSidebarSelection(delta int) {
	if w == nil {
		return
	}
	entries := w.activeSidebarEntries()
	if len(entries) == 0 {
		return
	}
	if w.SidebarBrowsing {
		w.BrowserSelection = max(0, min(len(entries)-1, w.BrowserSelection+delta))
		return
	}
	w.Selection = max(0, min(len(entries)-1, w.Selection+delta))
}

func (w *Workspace) leaveSidebar() {
	if w == nil {
		return
	}
	w.FocusSidebar = false
	w.SidebarBrowsing = false
	w.BrowserCommandMode = false
	w.BrowserCommand = ""
}

func (w *Workspace) toggleSidebarBrowser() {
	if w == nil {
		return
	}
	if w.SidebarBrowsing {
		w.SidebarBrowsing = false
		w.BrowserCommandMode = false
		w.BrowserCommand = ""
		return
	}
	w.SidebarBrowsing = true
	targetPath := ""
	if entry := w.selectedOpenEntry(); entry != nil {
		targetPath = entry.Path
	}
	if targetPath == "" {
		if ed := w.ActiveEditor(); ed != nil {
			targetPath = ed.Path
		}
	}
	w.BrowserSelection = clampSidebarSelectionByPath(w.BrowserTree, w.BrowserSelection, targetPath)
}

func (w *Workspace) handleEditorKey(key Key) bool {
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	if ed.ReplaceConfirm != nil {
		return handleReplaceConfirmKey(ed, key)
	}
	if autoCompleteActive(ed, autoCompleteSpell) {
		switch key.Name {
		case "down":
			return cycleSpellSuggestions(ed, 1)
		case "up":
			return cycleSpellSuggestions(ed, -1)
		case "enter":
			return acceptSpellSuggestion(ed)
		case "esc":
			clearAutoComplete(ed)
			ed.Status = "spelling suggestion cancelled"
			if settings.Inst().NotesApp.VimMode && ed.Mode == ModeInsert {
				ed.Mode = ModeNormal
			}
			return true
		}
	}
	if key.Ctrl && key.Name == "g" && ed.Mode != ModeCommand {
		return openSpellSuggestions(w, ed)
	}
	if (key.Meta || key.Ctrl) && (key.Name == "left" || key.Name == "right") {
		if key.Name == "left" {
			ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
		} else {
			ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		}
		if ed.Mode == ModeVisual {
			refreshVisualSelection(ed)
		}
		return true
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
		return handleVisualMode(w, ed, key)
	}
	return handleNormalMode(w, ed, key)
}

func handleCommandMode(w *Workspace, ed *Editor, key Key) bool {
	switch key.Name {
	case "esc":
		clearVisualSelection(ed)
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
			if helpers.HasStatusBar() {
				helpers.StatusBarInst().UpdateStatusBar(err.Error())
			}
			clearVisualSelection(ed)
			ed.Mode = ModeNormal
			return true
		}
		executeVimCommand(w, ed, cmd)
		ed.Command = ""
		clearVisualSelection(ed)
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
		rememberUndoState(ed)
		runes := []rune(ed.Text)
		ed.Text = string(append(runes[:ed.Cursor-1], runes[ed.Cursor:]...))
		ed.Cursor--
		ed.Dirty = true
		return true
	case "enter":
		clearAutoComplete(ed)
		return insertNewline(ed)
	case "tab":
		if key.Shift {
			if autoCompleteActive(ed, autoCompleteSpell) {
				return cycleSpellSuggestions(ed, -1)
			}
			if autoCompleteActive(ed, autoCompletePath) {
				return completeEditorPathReferenceBackward(w, ed)
			}
			return outdentListItem(ed)
		}
		if autoCompleteActive(ed, autoCompleteSpell) {
			return cycleSpellSuggestions(ed, 1)
		}
		if completeEditorPathReference(w, ed) {
			return true
		}
		if indentListItem(ed) {
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
		moveEditorCursorVertical(w, ed, -1)
		return true
	case "down":
		clearAutoComplete(ed)
		moveEditorCursorVertical(w, ed, 1)
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
		rememberUndoState(ed)
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
	rememberTypedInsertBoundary(ed, r)
	runes := []rune(ed.Text)
	idx := vimClampOffset(ed.Text, ed.Cursor)
	runes = append(runes[:idx], append([]rune{r}, runes[idx:]...)...)
	ed.Text = string(runes)
	ed.Cursor = idx + 1
	ed.Dirty = true
	return true
}

func rememberTypedInsertBoundary(ed *Editor, r rune) {
	if ed == nil {
		return
	}
	switch {
	case unicode.IsSpace(r):
		rememberWhitespaceInsertBoundary(ed)
	case isSentenceEndingRune(r):
		rememberUndoState(ed)
	default:
		rememberUndoState(ed)
	}
}

func rememberWhitespaceInsertBoundary(ed *Editor) {
	runes := []rune(ed.Text)
	cursor := vimClampOffset(ed.Text, ed.Cursor)
	if cursor == 0 {
		rememberUndoState(ed)
		return
	}
	prev := runes[cursor-1]
	switch {
	case isSentenceEndingRune(prev):
		rememberInsertBoundarySnapshot(ed, sentenceStartSnapshot(ed.Text, cursor))
	case isWordRune(prev):
		rememberInsertBoundarySnapshot(ed, wordStartSnapshot(ed.Text, cursor))
	default:
		rememberUndoState(ed)
	}
}

func rememberInsertBoundarySnapshot(ed *Editor, snapshot editorSnapshot) {
	if ed == nil {
		return
	}
	for i := len(ed.UndoStack) - 1; i >= 0; i-- {
		if ed.UndoStack[i].Text == snapshot.Text && ed.UndoStack[i].Cursor == snapshot.Cursor {
			ed.UndoStack = ed.UndoStack[:i+1]
			ed.RedoStack = nil
			return
		}
	}
	ed.UndoStack = append(ed.UndoStack, snapshot)
	ed.UndoStack = trimSnapshots(ed.UndoStack, undoLimit())
	ed.RedoStack = nil
}

func wordStartSnapshot(text string, cursor int) editorSnapshot {
	runes := []rune(text)
	cursor = vimClampOffset(text, cursor)
	start := cursor
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	withoutWord := append([]rune(nil), runes[:start]...)
	withoutWord = append(withoutWord, runes[cursor:]...)
	return editorSnapshot{Text: string(withoutWord), Cursor: start}
}

func sentenceStartSnapshot(text string, cursor int) editorSnapshot {
	runes := []rune(text)
	cursor = vimClampOffset(text, cursor)
	start := 0
	for i := cursor - 2; i >= 0; i-- {
		if isSentenceEndingRune(runes[i]) {
			start = i + 1
			for start < cursor && unicode.IsSpace(runes[start]) {
				start++
			}
			break
		}
	}
	withoutSentence := append([]rune(nil), runes[:start]...)
	withoutSentence = append(withoutSentence, runes[cursor:]...)
	return editorSnapshot{Text: string(withoutSentence), Cursor: start}
}

func isSentenceEndingRune(r rune) bool {
	return r == '.' || r == '!' || r == '?'
}

func insertNewline(ed *Editor) bool {
	if deleteEmptyListMarker(ed) {
		return true
	}
	rememberUndoState(ed)
	runes := []rune(ed.Text)
	idx := vimClampOffset(ed.Text, ed.Cursor)
	continuationPrefix := newlineContinuationPrefix(ed.Text, ed.Cursor)
	insert := []rune("\n" + continuationPrefix)
	runes = append(runes[:idx], append(insert, runes[idx:]...)...)
	ed.Text = string(runes)
	ed.Cursor = idx + len(insert)
	if _, _, ok := orderedListLineParts(continuationPrefix); ok {
		ed.Text = renumberFollowingOrderedListLines(ed.Text, ed.Cursor)
	}
	ed.Dirty = true
	return true
}

func indentListItem(ed *Editor) bool {
	if ed == nil {
		return false
	}
	lineStart, lineEnd, line := currentEditorLine(ed)
	prefix, content, ok := listPrefixParts(line)
	if !ok || strings.TrimSpace(prefix) == "" {
		return false
	}
	rememberUndoState(ed)
	indent := strings.Repeat(" ", settings.Inst().NotesApp.TabSpaces)
	replacement := indent + prefix + content
	runes := []rune(ed.Text)
	updated := string(runes[:lineStart]) + replacement + string(runes[lineEnd:])
	cursorShift := len([]rune(indent))
	ed.Text = updated
	ed.Cursor += cursorShift
	ed.Dirty = true
	return true
}

func outdentListItem(ed *Editor) bool {
	if ed == nil {
		return false
	}
	lineStart, lineEnd, line := currentEditorLine(ed)
	prefix, content, ok := listPrefixParts(line)
	if !ok {
		return true
	}
	indentLen := len(prefix) - len(strings.TrimLeft(prefix, " \t"))
	if indentLen <= 0 {
		return true
	}
	remove := min(settings.Inst().NotesApp.TabSpaces, indentLen)
	rememberUndoState(ed)
	replacement := prefix[remove:] + content
	runes := []rune(ed.Text)
	ed.Text = string(runes[:lineStart]) + replacement + string(runes[lineEnd:])
	if ed.Cursor >= lineStart+remove {
		ed.Cursor -= remove
	} else {
		ed.Cursor = lineStart
	}
	ed.Dirty = true
	return true
}

func deleteEmptyListMarker(ed *Editor) bool {
	if ed == nil {
		return false
	}
	lineStart, lineEnd, line := currentEditorLine(ed)
	_, content, ok := listPrefixParts(line)
	if !ok || strings.TrimSpace(content) != "" {
		return false
	}
	rememberUndoState(ed)
	runes := []rune(ed.Text)
	updated := string(runes[:lineStart]) + string(runes[lineEnd:])
	ed.Text = updated
	ed.Cursor = lineStart
	ed.Text = renumberOrderedListBlockAtOffset(ed.Text, ed.Cursor)
	ed.Dirty = true
	return true
}

func currentEditorLine(ed *Editor) (int, int, string) {
	lineStart := vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
	lineEnd := vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
	runes := []rune(ed.Text)
	if lineStart > len(runes) {
		lineStart = len(runes)
	}
	if lineEnd > len(runes) {
		lineEnd = len(runes)
	}
	return lineStart, lineEnd, string(runes[lineStart:lineEnd])
}

func listPrefixParts(line string) (prefix string, content string, ok bool) {
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := line[:indentLen]
	trimmed := line[indentLen:]
	switch {
	case strings.HasPrefix(trimmed, "- [ ] "):
		return indent + "- [ ] ", trimmed[len("- [ ] "):], true
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		return indent + trimmed[:len("- [x] ")], trimmed[len("- [x] "):], true
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "+ "):
		return indent + trimmed[:2], trimmed[2:], true
	default:
		i := 0
		for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
			i++
		}
		if i > 0 && i+1 < len(trimmed) && trimmed[i] == '.' && trimmed[i+1] == ' ' {
			return indent + trimmed[:i+2], trimmed[i+2:], true
		}
		return "", "", false
	}
}

func newlineContinuationPrefix(text string, cursor int) string {
	lineStart := vimLineBoundaryOffset(text, cursor, false)
	lineEnd := vimLineBoundaryOffset(text, cursor, true)
	runes := []rune(text)
	if lineStart > len(runes) {
		return ""
	}
	if lineEnd > len(runes) {
		lineEnd = len(runes)
	}
	line := string(runes[lineStart:lineEnd])
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := line[:indentLen]
	trimmed := strings.TrimLeft(line, " \t")

	switch {
	case strings.HasPrefix(trimmed, "- [ ] "), strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		return indent + "- [ ] "
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "+ "):
		return indent + trimmed[:2]
	default:
		number, suffix, ok := orderedListPrefix(trimmed)
		if ok {
			return indent + strconv.Itoa(number) + suffix
		}
		return ""
	}
}

func orderedListPrefix(trimmed string) (int, string, bool) {
	i := 0
	for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(trimmed) {
		return 0, "", false
	}
	if trimmed[i] != '.' || trimmed[i+1] != ' ' {
		return 0, "", false
	}
	number, err := strconv.Atoi(trimmed[:i])
	if err != nil || number <= 0 {
		return 0, "", false
	}
	return number + 1, ". ", true
}

func renumberFollowingOrderedListLines(text string, cursor int) string {
	lines := strings.Split(text, "\n")
	currentLine := lineIndexAtRuneOffset(text, cursor)
	if currentLine < 0 || currentLine >= len(lines) {
		return text
	}
	indent, number, ok := orderedListLineParts(lines[currentLine])
	if !ok {
		return text
	}
	nextNumber := number + 1
	changed := false
	for i := currentLine + 1; i < len(lines); i++ {
		currentIndent, _, ok := orderedListLineParts(lines[i])
		if !ok {
			if lineIndent := lineIndent(lines[i]); len(lineIndent) > len(indent) {
				continue
			}
			break
		}
		if len(currentIndent) > len(indent) {
			continue
		}
		if currentIndent != indent {
			break
		}
		lines[i] = replaceOrderedListNumber(lines[i], nextNumber)
		nextNumber++
		changed = true
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

func renumberOrderedListBlockAtOffset(text string, cursor int) string {
	lines := strings.Split(text, "\n")
	lineIdx := lineIndexAtRuneOffset(text, cursor)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return text
	}
	indent, _, ok := orderedListLineParts(lines[lineIdx])
	start := lineIdx
	firstNumber := 0
	if !ok {
		if lineIdx+1 >= len(lines) {
			return text
		}
		start = lineIdx + 1
		indent, firstNumber, ok = orderedListLineParts(lines[start])
		if !ok {
			return text
		}
		if lineIdx > 0 {
			if prevIndent, prevNumber, ok := orderedListLineParts(lines[lineIdx-1]); ok && prevIndent == indent {
				firstNumber = prevNumber + 1
			} else {
				firstNumber = 1
			}
		} else {
			firstNumber = 1
		}
	} else {
		for start > 0 {
			prevIndent, _, ok := orderedListLineParts(lines[start-1])
			if !ok {
				if lineIndent := lineIndent(lines[start-1]); len(lineIndent) > len(indent) {
					start--
					continue
				}
				break
			}
			if len(prevIndent) > len(indent) {
				start--
				continue
			}
			if prevIndent != indent {
				break
			}
			start--
		}
		_, firstNumber, ok = orderedListLineParts(lines[start])
		if !ok {
			return text
		}
	}
	nextNumber := firstNumber
	changed := false
	for i := start; i < len(lines); i++ {
		currentIndent, number, ok := orderedListLineParts(lines[i])
		if !ok {
			if lineIndent := lineIndent(lines[i]); len(lineIndent) > len(indent) {
				continue
			}
			break
		}
		if len(currentIndent) > len(indent) {
			continue
		}
		if currentIndent != indent {
			break
		}
		if number != nextNumber {
			lines[i] = replaceOrderedListNumber(lines[i], nextNumber)
			changed = true
		}
		nextNumber++
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

func lineIndexAtRuneOffset(text string, offset int) int {
	runes := []rune(text)
	offset = min(max(offset, 0), len(runes))
	line := 0
	for _, r := range runes[:offset] {
		if r == '\n' {
			line++
		}
	}
	return line
}

func orderedListLineParts(line string) (indent string, number int, ok bool) {
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	trimmed := line[indentLen:]
	i := 0
	for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(trimmed) || trimmed[i] != '.' || trimmed[i+1] != ' ' {
		return "", 0, false
	}
	number, err := strconv.Atoi(trimmed[:i])
	if err != nil || number <= 0 {
		return "", 0, false
	}
	return line[:indentLen], number, true
}

func replaceOrderedListNumber(line string, number int) string {
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	trimmed := line[indentLen:]
	i := 0
	for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
		i++
	}
	return line[:indentLen] + strconv.Itoa(number) + trimmed[i:]
}

func lineIndent(line string) string {
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	return line[:indentLen]
}

func clearAutoComplete(ed *Editor) {
	if ed == nil {
		return
	}
	ed.AutoCompletePrefix = ""
	ed.AutoCompleteKind = ""
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
		if ed.AutoCompleteKind == autoCompleteSpell {
			return ""
		}
		current := ed.AutoCompleteMatches[ed.AutoCompleteIndex%len(ed.AutoCompleteMatches)]
		extra := len(ed.AutoCompleteMatches) - 1
		label := "path complete: "
		line := label + current
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

const (
	autoCompletePath  = "path"
	autoCompleteSpell = "spell"
)

func autoCompleteActive(ed *Editor, kind string) bool {
	return ed != nil && ed.AutoCompleteKind == kind && len(ed.AutoCompleteMatches) > 0
}

func openSpellSuggestions(w *Workspace, ed *Editor) bool {
	if ed == nil {
		return false
	}
	word, start, end, ok := spellSuggestionTarget(ed)
	if !ok {
		clearAutoComplete(ed)
		ed.Status = "no word under cursor for spelling"
		return true
	}
	service, err := currentSpellService()
	if err != nil || service == nil || !service.ready() {
		clearAutoComplete(ed)
		ed.Status = "spell service unavailable"
		return true
	}
	if service.correct(word) {
		clearAutoComplete(ed)
		ed.Status = "word is correct: " + word
		return true
	}
	suggestions, err := service.suggestions(word)
	if err != nil {
		clearAutoComplete(ed)
		ed.Status = "spell suggestions failed"
		return true
	}
	suggestions = filterSpellSuggestions(word, suggestions)
	if len(suggestions) == 0 {
		clearAutoComplete(ed)
		ed.Status = "no spelling suggestions returned"
		return true
	}
	ed.AutoCompleteKind = autoCompleteSpell
	ed.AutoCompletePrefix = word
	ed.AutoCompleteMatches = suggestions
	ed.AutoCompleteIndex = 0
	ed.AutoCompleteStart = start
	ed.AutoCompleteEnd = end
	ed.Status = spellSuggestionsStatus(suggestions)
	_ = w
	return true
}

func cycleSpellSuggestions(ed *Editor, delta int) bool {
	if !autoCompleteActive(ed, autoCompleteSpell) {
		return false
	}
	count := len(ed.AutoCompleteMatches)
	if count == 0 {
		return false
	}
	index := ed.AutoCompleteIndex + delta
	for index < 0 {
		index += count
	}
	index %= count
	ed.AutoCompleteIndex = index
	ed.Status = spellSuggestionsStatus(ed.AutoCompleteMatches)
	return true
}

func spellSuggestionsStatus(suggestions []string) string {
	if len(suggestions) == 0 {
		return "no spelling suggestions available"
	}
	show := suggestions
	if len(show) > 4 {
		show = show[:4]
	}
	status := "spell suggestions ready"
	if len(suggestions) > len(show) {
		status += fmt.Sprintf(" (+%d more)", len(suggestions)-len(show))
	}
	return status
}

func acceptSpellSuggestion(ed *Editor) bool {
	if !autoCompleteActive(ed, autoCompleteSpell) {
		return false
	}
	index := ed.AutoCompleteIndex
	if index < 0 || index >= len(ed.AutoCompleteMatches) {
		return false
	}
	replacement := ed.AutoCompleteMatches[index]
	replaceRunes(ed, ed.AutoCompleteStart, ed.AutoCompleteEnd, replacement)
	ed.AutoCompleteEnd = ed.AutoCompleteStart + len([]rune(replacement))
	ed.Dirty = true
	ed.Status = "spelling applied: " + replacement
	ed.SpellCacheText = ""
	ed.SpellCacheSpans = nil
	clearAutoComplete(ed)
	return true
}

func filterSpellSuggestions(word string, suggestions []string) []string {
	if len(suggestions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(suggestions))
	filtered := make([]string, 0, len(suggestions))
	normalizedWord := normalizeSpellWord(word)
	for _, suggestion := range suggestions {
		trimmed := strings.TrimSpace(suggestion)
		if trimmed == "" {
			continue
		}
		normalized := normalizeSpellWord(trimmed)
		if normalized == "" || normalized == normalizedWord {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func spellSuggestionTarget(ed *Editor) (string, int, int, bool) {
	if ed == nil {
		return "", 0, 0, false
	}
	runes := []rune(ed.Text)
	if len(runes) == 0 {
		return "", 0, 0, false
	}
	cursor := vimClampOffset(ed.Text, ed.Cursor)
	anchor := -1
	if cursor > 0 && cursor-1 < len(runes) && isSpellWordRune(runes[cursor-1]) {
		anchor = cursor - 1
	} else if cursor < len(runes) && isSpellWordRune(runes[cursor]) {
		anchor = cursor
	}
	if anchor < 0 {
		return "", 0, 0, false
	}
	start := anchor
	for start > 0 && isSpellWordRune(runes[start-1]) {
		start--
	}
	end := anchor + 1
	for end < len(runes) && isSpellWordRune(runes[end]) {
		end++
	}
	word := normalizeSpellWord(string(runes[start:end]))
	if !shouldCheckSpellWord(word) {
		return "", 0, 0, false
	}
	return word, start, end, true
}

func handleNormalMode(w *Workspace, ed *Editor, key Key) bool {
	if ed.PendingOp == "r" {
		return handleReplacePending(ed, key)
	}
	if ed.PendingOp == "m" || ed.PendingOp == "ml" {
		if handleMoveLinePending(ed, key, false) {
			return true
		}
	}
	if ed.PendingOp == "z" {
		ed.PendingOp = ""
		if key.Name == "g" {
			ed.NormalCount = ""
			w.AddWordUnderCursor()
			return true
		}
	}
	if consumeXMotionOverride(w, ed, key) {
		return true
	}
	if key.Name >= "0" && key.Name <= "9" && len([]rune(key.Name)) == 1 && ed.PendingOp != "" {
		ed.NormalCount += key.Name
		return true
	}
	if ed.PendingOp != "" && key.Name != ed.PendingOp {
		handled := applyPendingOperator(w, ed, key.Name)
		if handled {
			return true
		}
	}
	if key.Name >= "0" && key.Name <= "9" && len([]rune(key.Name)) == 1 {
		if key.Name == "0" && ed.NormalCount == "" {
			ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
			return true
		}
		ed.NormalCount += key.Name
		return true
	}
	if moveLineKeyToken(key) == "m" {
		ed.NormalCount = ""
		ed.PendingOp = "m"
		return true
	}
	switch key.Name {
	case "i":
		ed.NormalCount = ""
		ed.Mode = ModeInsert
		return true
	case "a":
		ed.NormalCount = ""
		runes := []rune(ed.Text)
		if ed.Cursor < len(runes) && runes[ed.Cursor] != '\n' {
			ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		}
		ed.Mode = ModeInsert
		return true
	case "o":
		ed.NormalCount = ""
		rememberUndoState(ed)
		ed.Text, ed.Cursor = vimOpenLineBelow(ed.Text, ed.Cursor)
		ed.Mode = ModeInsert
		ed.Dirty = true
		return true
	case "O":
		ed.NormalCount = ""
		rememberUndoState(ed)
		ed.Text, ed.Cursor = vimOpenLineAbove(ed.Text, ed.Cursor)
		ed.Mode = ModeInsert
		ed.Dirty = true
		return true
	case "h", "left":
		ed.NormalCount = ""
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor-1)
		return true
	case "l", "right":
		ed.NormalCount = ""
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		return true
	case "j", "down":
		ed.NormalCount = ""
		moveEditorCursorVertical(w, ed, 1)
		return true
	case "k", "up":
		ed.NormalCount = ""
		moveEditorCursorVertical(w, ed, -1)
		return true
	case "home":
		ed.NormalCount = ""
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
		return true
	case "end":
		ed.NormalCount = ""
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		return true
	case "pageup":
		ed.NormalCount = ""
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, -10)
		return true
	case "pagedown":
		ed.NormalCount = ""
		ed.Cursor = vimPageMoveOffset(ed.Text, ed.Cursor, 10)
		return true
	case "$":
		ed.NormalCount = ""
		ed.Cursor = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		return true
	case "G":
		targetLine := normalModeCount(ed)
		ed.NormalCount = ""
		ed.Cursor = vimLineStartForNumber(ed.Text, targetLine)
		return true
	case "w":
		ed.NormalCount = ""
		ed.Cursor = moveWordForward(ed.Text, ed.Cursor)
		return true
	case "b":
		ed.NormalCount = ""
		ed.Cursor = moveWordBackward(ed.Text, ed.Cursor)
		return true
	case "x":
		ed.NormalCount = ""
		ed.LastXText = ed.Text
		ed.LastXCursor = ed.Cursor
		ed.LastXArmed = true
		ed.Register = vimYankChar(ed.Text, ed.Cursor, ed.Cursor)
		updateClipboardForRegister(w, ed, "deleted char")
		rememberUndoState(ed)
		ed.Text, ed.Cursor = vimDeleteChar(ed.Text, ed.Cursor)
		ed.Dirty = true
		return true
	case "delete":
		ed.NormalCount = ""
		ed.Register = vimYankChar(ed.Text, ed.Cursor, ed.Cursor)
		updateClipboardForRegister(w, ed, "deleted char")
		rememberUndoState(ed)
		ed.Text, ed.Cursor = vimDeleteChar(ed.Text, ed.Cursor)
		ed.Dirty = true
		return true
	case "r":
		ed.NormalCount = ""
		ed.PendingOp = "r"
		return true
	case ":":
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = ""
		return true
	case "/":
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = "/"
		return true
	case "?":
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = "?"
		return true
	case "R":
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = "rename " + ed.Title
		return true
	case "z":
		ed.NormalCount = ""
		ed.PendingOp = "z"
		return true
	case "n":
		ed.NormalCount = ""
		repeatSearch(ed, true)
		return true
	case "N":
		ed.NormalCount = ""
		repeatSearch(ed, false)
		return true
	case "u":
		ed.NormalCount = ""
		return applyUndo(ed)
	case " ":
		ed.NormalCount = ""
		return toggleCheckboxAtCursor(ed)
	case ">":
		ed.NormalCount = ""
		return shiftCurrentLine(ed, true)
	case "<":
		ed.NormalCount = ""
		return shiftCurrentLine(ed, false)
	case "tab":
		ed.NormalCount = ""
		w.FocusSidebar = true
		return true
	case "V":
		ed.NormalCount = ""
		startVisualSelection(ed, vimSelectionLine)
		return true
	case "v":
		ed.NormalCount = ""
		startVisualSelection(ed, vimSelectionChar)
		return true
	case "d", "y", "c":
		ed.NormalCount = ""
		if ed.PendingOp == key.Name {
			if key.Name == "d" || key.Name == "c" {
				ed.Register = vimYankLine(ed.Text, ed.Cursor, ed.Cursor)
				updateClipboardForRegister(w, ed, "deleted line")
				rememberUndoState(ed)
				ed.Text, ed.Cursor = vimDeleteLine(ed.Text, ed.Cursor)
				ed.Text = renumberOrderedListBlockAtOffset(ed.Text, ed.Cursor)
				ed.Dirty = true
			}
			if key.Name == "y" {
				start, end := vimLineRange(ed.Text, ed.Cursor, ed.Cursor)
				ed.Register = vimYankLine(ed.Text, ed.Cursor, ed.Cursor)
				updateClipboardForRegister(w, ed, "yanked line")
				flashYankRange(ed, start, end)
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
		ed.NormalCount = ""
		reg, err := pasteRegister(w, ed)
		if err != nil {
			ed.Status = "clipboard paste failed: " + err.Error()
			return true
		}
		ed.Register = reg
		rememberUndoState(ed)
		switch reg.Kind {
		case vimRegisterLine:
			ed.Text, ed.Cursor = vimPasteLine(ed.Text, ed.Cursor, reg)
		case vimRegisterBlock:
			ed.Text, ed.Cursor = vimPasteBlock(ed.Text, ed.Cursor, reg)
		default:
			ed.Text, ed.Cursor = vimPasteCharAfter(ed.Text, ed.Cursor, reg)
		}
		ed.Dirty = true
		return true
	}
	ed.NormalCount = ""
	return false
}

func normalModeCount(ed *Editor) int {
	if ed == nil || ed.NormalCount == "" {
		return 0
	}
	count, err := strconv.Atoi(ed.NormalCount)
	if err != nil {
		return 0
	}
	return count
}

func vimLineStartForNumber(text string, lineNumber int) int {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return 0
	}
	if lineNumber <= 0 {
		lineNumber = len(lines)
	}
	if lineNumber > len(lines) {
		lineNumber = len(lines)
	}
	return lines[lineNumber-1].start
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

func pasteRegister(w *Workspace, ed *Editor) (vimRegister, error) {
	if w != nil && registerHasContent(w.Register) {
		return normalizePasteRegister(w.Register), nil
	}
	if ed != nil && registerHasContent(ed.Register) {
		return normalizePasteRegister(ed.Register), nil
	}
	return clipboardPasteRegister()
}

func registerHasContent(reg vimRegister) bool {
	switch reg.Kind {
	case vimRegisterBlock:
		return len(reg.Lines) > 0
	case vimRegisterLine, vimRegisterChar:
		return reg.Text != ""
	default:
		return false
	}
}

func handleReplacePending(ed *Editor, key Key) bool {
	if key.Name == "esc" {
		ed.PendingOp = ""
		return true
	}
	if key.Rune == 0 {
		return false
	}
	rememberUndoState(ed)
	ed.Text, ed.Cursor = vimReplaceChar(ed.Text, ed.Cursor, key.Rune)
	ed.Dirty = true
	ed.PendingOp = ""
	return true
}

func consumeXMotionOverride(w *Workspace, ed *Editor, key Key) bool {
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
		ed.Register = vimYankChar(ed.LastXText, start, end-1)
		updateClipboardForRegister(w, ed, "deleted word")
		ed.Text, ed.Cursor = vimDeleteRange(ed.LastXText, start, end-1)
		ed.Dirty = true
		return true
	case "$", "end":
		start := ed.LastXCursor
		end := vimLineBoundaryOffset(ed.LastXText, ed.LastXCursor, true)
		if end <= start {
			return false
		}
		ed.Register = vimYankChar(ed.LastXText, start, end-1)
		updateClipboardForRegister(w, ed, "deleted to end of line")
		ed.Text, ed.Cursor = vimDeleteRange(ed.LastXText, start, end-1)
		ed.Dirty = true
		return true
	default:
		return false
	}
}

func handleVisualMode(w *Workspace, ed *Editor, key Key) bool {
	if ed.PendingOp == "m" || ed.PendingOp == "ml" {
		if handleMoveLinePending(ed, key, true) {
			return true
		}
	}
	switch key.Name {
	case "esc":
		clearVisualSelection(ed)
		ed.PendingOp = ""
		ed.NormalCount = ""
		ed.Mode = ModeNormal
		return true
	case ":":
		ed.PendingOp = ""
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = "'<,'>"
		return true
	case "/":
		ed.PendingOp = ""
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = "/"
		return true
	case "?":
		ed.PendingOp = ""
		ed.NormalCount = ""
		ed.Mode = ModeCommand
		ed.Command = "?"
		return true
	}
	if moveLineKeyToken(key) == "m" {
		if ed.SelectionMode == vimSelectionNone {
			return false
		}
		ed.NormalCount = ""
		ed.PendingOp = "m"
		return true
	}
	switch key.Name {
	case "h", "left":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor-1)
		refreshVisualSelection(ed)
		return true
	case "l", "right":
		ed.Cursor = vimClampOffset(ed.Text, ed.Cursor+1)
		refreshVisualSelection(ed)
		return true
	case "j", "down":
		moveEditorCursorVertical(w, ed, 1)
		refreshVisualSelection(ed)
		return true
	case "k", "up":
		moveEditorCursorVertical(w, ed, -1)
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
		deleteVisualSelection(w, ed)
		ed.Mode = ModeNormal
		return true
	case "y":
		yankVisualSelection(w, ed)
		ed.Mode = ModeNormal
		return true
	case ">":
		return shiftVisualSelection(ed, true)
	case "<":
		return shiftVisualSelection(ed, false)
	}
	return false
}

func handleMoveLinePending(ed *Editor, key Key, visual bool) bool {
	token := moveLineKeyToken(key)
	switch ed.PendingOp {
	case "m":
		if token == "l" {
			ed.PendingOp = "ml"
			ed.NormalCount = ""
			return true
		}
		ed.PendingOp = ""
		ed.NormalCount = ""
		return false
	case "ml":
		if token >= "0" && token <= "9" && len([]rune(token)) == 1 {
			if token == "0" && ed.NormalCount == "" {
				return true
			}
			ed.NormalCount += token
			return true
		}
		if token == "u" || token == "d" {
			count := normalModeCount(ed)
			if count <= 0 {
				count = 1
			}
			if token == "u" {
				count = -count
			}
			ed.PendingOp = ""
			ed.NormalCount = ""
			if visual {
				return moveVisualLineSelection(ed, count)
			}
			return moveCurrentLine(ed, count)
		}
		ed.PendingOp = ""
		ed.NormalCount = ""
		return false
	default:
		return false
	}
}

func moveLineKeyToken(key Key) string {
	if key.Rune != 0 && !key.Ctrl && !key.Meta && !key.Alt {
		r := unicode.ToLower(key.Rune)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return string(r)
		}
	}
	return key.Name
}

func moveCurrentLine(ed *Editor, delta int) bool {
	updated, offsets, changed := vimMoveLineBlockAndOffsets(ed.Text, ed.Cursor, ed.Cursor, delta, []int{ed.Cursor})
	if !changed {
		return true
	}
	rememberUndoState(ed)
	ed.Text = updated
	ed.Cursor = offsets[0]
	ed.Dirty = true
	return true
}

func moveCommandLines(ed *Editor, delta int) bool {
	if ed == nil {
		return false
	}
	if ed.SelectionMode != vimSelectionNone {
		return moveCommandLineRange(ed, delta)
	}
	updated, offsets, changed := vimMoveLineBlockAndOffsets(ed.Text, ed.Cursor, ed.Cursor, delta, []int{ed.Cursor})
	if changed {
		rememberUndoState(ed)
		ed.Text = updated
		ed.Cursor = offsets[0]
		ed.Dirty = true
	}
	ed.Status = lineMoveStatus(false, delta, changed)
	return true
}

func moveCommandLineRange(ed *Editor, delta int) bool {
	updated, offsets, changed := vimMoveLineBlockAndOffsets(ed.Text, ed.SelectionMark, ed.SelectionCursor, delta, []int{
		ed.Cursor,
		ed.SelectionMark,
		ed.SelectionCursor,
	})
	if changed {
		rememberUndoState(ed)
		ed.Text = updated
		ed.Cursor = offsets[0]
		ed.SelectionMark = offsets[1]
		ed.SelectionCursor = offsets[2]
		ed.Dirty = true
	}
	ed.Status = lineMoveStatus(true, delta, changed)
	clearVisualSelection(ed)
	ed.Mode = ModeNormal
	return true
}

func lineMoveStatus(plural bool, delta int, changed bool) string {
	if !changed {
		if plural {
			return "lines already at boundary"
		}
		return "line already at boundary"
	}
	direction := "down"
	if delta < 0 {
		direction = "up"
	}
	if plural {
		return "moved lines " + direction
	}
	return "moved line " + direction
}

func moveVisualLineSelection(ed *Editor, delta int) bool {
	if ed.SelectionMode == vimSelectionNone {
		return false
	}
	mode := ed.SelectionMode
	updated, offsets, changed := vimMoveLineBlockAndOffsets(ed.Text, ed.SelectionMark, ed.SelectionCursor, delta, []int{
		ed.Cursor,
		ed.SelectionMark,
		ed.SelectionCursor,
	})
	if changed {
		rememberUndoState(ed)
		ed.Text = updated
		ed.Cursor = offsets[0]
		ed.SelectionMark = offsets[1]
		ed.SelectionCursor = offsets[2]
		ed.Dirty = true
	}
	ed.Mode = ModeVisual
	ed.SelectionMode = mode
	return true
}

func vimMoveLineBlockAndOffsets(text string, startOffset int, endOffset int, delta int, offsets []int) (string, []int, bool) {
	transformed := append([]int(nil), offsets...)
	if delta == 0 {
		return text, transformed, false
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= 1 {
		return text, transformed, false
	}
	startIdx, endIdx := vimLineIndexRange(text, startOffset, endOffset)
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	blockLen := endIdx - startIdx + 1
	if blockLen >= len(lines) {
		return text, transformed, false
	}
	targetStart := startIdx + delta
	if targetStart < 0 {
		targetStart = 0
	}
	if maxStart := len(lines) - blockLen; targetStart > maxStart {
		targetStart = maxStart
	}
	if targetStart == startIdx {
		return text, transformed, false
	}

	lineInfos := vimLineInfos(text)
	lineRefs := make([]lineOffsetRef, len(offsets))
	for i, offset := range offsets {
		lineIdx := vimLineIndexAtOffset(text, offset)
		if lineIdx < startIdx {
			lineIdx = startIdx
		}
		if lineIdx > endIdx {
			lineIdx = endIdx
		}
		col := offset - lineInfos[lineIdx].start
		if col < 0 {
			col = 0
		}
		lineRefs[i] = lineOffsetRef{line: lineIdx - startIdx, col: col}
	}

	block := append([]string(nil), lines[startIdx:endIdx+1]...)
	remaining := make([]string, 0, len(lines)-blockLen)
	remaining = append(remaining, lines[:startIdx]...)
	remaining = append(remaining, lines[endIdx+1:]...)
	updatedLines := make([]string, 0, len(lines))
	updatedLines = append(updatedLines, remaining[:targetStart]...)
	updatedLines = append(updatedLines, block...)
	updatedLines = append(updatedLines, remaining[targetStart:]...)
	updated := strings.Join(updatedLines, "\n")
	updatedStarts := lineStartOffsets(updated)

	for i, ref := range lineRefs {
		lineIdx := targetStart + ref.line
		if lineIdx < 0 {
			lineIdx = 0
		}
		if lineIdx >= len(updatedLines) {
			lineIdx = len(updatedLines) - 1
		}
		col := min(ref.col, len([]rune(updatedLines[lineIdx])))
		transformed[i] = updatedStarts[lineIdx] + col
	}
	return updated, transformed, true
}

type lineOffsetRef struct {
	line int
	col  int
}

func shiftCurrentLine(ed *Editor, right bool) bool {
	updated, cursor, changed := vimShiftLines(ed.Text, ed.Cursor, ed.Cursor, ed.Cursor, settings.Inst().NotesApp.TabSpaces, right)
	if !changed {
		return true
	}
	rememberUndoState(ed)
	ed.Text = updated
	ed.Cursor = cursor
	ed.Dirty = true
	return true
}

func shiftVisualSelection(ed *Editor, right bool) bool {
	start := ed.SelectionMark
	end := ed.SelectionCursor
	updated, offsets, changed := vimShiftLinesAndOffsets(ed.Text, start, end, settings.Inst().NotesApp.TabSpaces, right, []int{
		ed.Cursor,
		ed.SelectionMark,
		ed.SelectionCursor,
	})
	if changed {
		rememberUndoState(ed)
		ed.Text = updated
		ed.Cursor = offsets[0]
		ed.SelectionMark = offsets[1]
		ed.SelectionCursor = offsets[2]
		ed.Dirty = true
	}
	ed.Mode = ModeVisual
	return true
}

func applyPendingOperator(w *Workspace, ed *Editor, key string) bool {
	if ed.PendingOp == "d" && key == "w" {
		ed.Register = vimYankWord(ed.Text, ed.Cursor)
		updateClipboardForRegister(w, ed, "deleted word")
		rememberUndoState(ed)
		ed.Text, ed.Cursor = vimDeleteWord(ed.Text, ed.Cursor)
		ed.Dirty = true
		ed.PendingOp = ""
		return true
	}
	if ed.PendingOp == "d" && (key == "$" || key == "end") {
		start := ed.Cursor
		end := vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
		if end <= start {
			ed.PendingOp = ""
			return true
		}
		ed.Register = vimYankChar(ed.Text, start, end-1)
		updateClipboardForRegister(w, ed, "deleted to end of line")
		rememberUndoState(ed)
		ed.Text, ed.Cursor = vimDeleteToLineEnd(ed.Text, ed.Cursor)
		ed.Dirty = true
		ed.PendingOp = ""
		return true
	}
	if ed.PendingOp == "d" && (key == "up" || key == "down") {
		delta := 1
		if count := normalModeCount(ed); count > 0 {
			delta = count
		}
		if key == "up" {
			delta = -delta
		}
		updated, cursor, reg, changed := vimDeleteLineSpan(ed.Text, ed.Cursor, delta)
		ed.NormalCount = ""
		ed.PendingOp = ""
		if !changed {
			return true
		}
		ed.Register = reg
		updateClipboardForRegister(w, ed, "deleted lines")
		rememberUndoState(ed)
		ed.Text = updated
		ed.Cursor = cursor
		ed.Text = renumberOrderedListBlockAtOffset(ed.Text, ed.Cursor)
		ed.Dirty = true
		return true
	}
	start := ed.Cursor
	end := ed.Cursor
	switch key {
	case "w":
		end = moveWordForward(ed.Text, ed.Cursor)
	case "b":
		end = moveWordBackward(ed.Text, ed.Cursor)
	case "$", "end":
		end = vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
	default:
		ed.PendingOp = ""
		return false
	}
	if ed.PendingOp == "y" {
		yankStart, yankEnd := start, end
		if key == "w" {
			yankStart, yankEnd = vimStrictWordRange(ed.Text, ed.Cursor)
		}
		ed.Register = vimYankChar(ed.Text, yankStart, max(yankStart, yankEnd-1))
		updateClipboardForRegister(w, ed, "yanked text")
		flashYankRange(ed, min(yankStart, yankEnd), max(yankStart, yankEnd))
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
	updateClipboardForRegister(w, ed, "deleted text")
	rememberUndoState(ed)
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

func toggleCheckboxAtCursor(ed *Editor) bool {
	if ed == nil {
		return false
	}
	lineStart := vimLineBoundaryOffset(ed.Text, ed.Cursor, false)
	lineEnd := vimLineBoundaryOffset(ed.Text, ed.Cursor, true)
	return toggleCheckboxLineRange(ed, lineStart, lineEnd)
}

func toggleCheckboxLineRange(ed *Editor, lineStart int, lineEnd int) bool {
	runes := []rune(ed.Text)
	if lineStart > len(runes) {
		return false
	}
	if lineEnd > len(runes) {
		lineEnd = len(runes)
	}
	line := string(runes[lineStart:lineEnd])
	if !lineHasCheckbox(line) {
		return false
	}
	rememberUndoState(ed)
	updated, _, _ := toggleChecklist(ed.Text, lineStart, lineStart)
	ed.Text = updated
	ed.Dirty = true
	ed.Status = "checkbox toggled"
	return true
}

func lineHasCheckbox(line string) bool {
	return strings.Contains(line, "[ ]") || strings.Contains(strings.ToLower(line), "[x]")
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

func moveEditorCursorVertical(w *Workspace, ed *Editor, delta int) {
	if ed == nil {
		return
	}
	if w == nil {
		ed.Cursor = vimVerticalMoveOffset(ed.Text, ed.Cursor, delta)
		return
	}
	row, col := editorVisualCursor(ed, w.editorRenderWidth())
	ed.Cursor = vimClampOffset(ed.Text, editorOffsetAtAbsoluteVisualPosition(ed.Text, w.editorRenderWidth(), row+delta, col))
}

func repeatSearch(ed *Editor, forward bool) {
	if ed.LastSearch == "" {
		return
	}
	if ed.LastSearchBackward {
		forward = !forward
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

func executeReplaceCommand(ed *Editor, cmd vimCommand) {
	var re *regexp.Regexp
	var err error
	if cmd.Literal {
		pattern := regexp.QuoteMeta(cmd.Query)
		if cmd.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		re, err = regexp.Compile(pattern)
	} else {
		re, err = compileVimRegex(cmd.Query, cmd.IgnoreCase)
	}
	if err != nil {
		ed.Status = "invalid pattern: " + err.Error()
		return
	}
	start, end := commandReplaceRange(ed, cmd)
	candidates := collectReplaceCandidates(ed.Text, re, cmd.Replacement, cmd.Global, start, end)
	if len(candidates) == 0 {
		ed.Status = "0 replacements"
		return
	}
	before := currentEditorSnapshot(ed)
	if cmd.Confirm {
		ed.ReplaceConfirm = &replaceConfirmSession{
			OriginalText: ed.Text,
			Before:       before,
			Candidates:   candidates,
			Accepted:     make([]bool, len(candidates)),
			Current:      0,
		}
		ed.Cursor = candidates[0].Start
		ed.Status = replaceConfirmStatus(ed.ReplaceConfirm)
		return
	}
	ed.Text = applyReplaceCandidates(ed.Text, candidates, nil)
	ed.Cursor = min(candidates[0].Start+len([]rune(candidates[0].Replacement)), len([]rune(ed.Text)))
	ed.UndoStack = append(ed.UndoStack, before)
	ed.UndoStack = trimSnapshots(ed.UndoStack, undoLimit())
	ed.RedoStack = nil
	ed.Dirty = true
	ed.Status = fmt.Sprintf("%d replacements", len(candidates))
}

func commandReplaceRange(ed *Editor, cmd vimCommand) (int, int) {
	switch cmd.Range.Kind {
	case vimRangeAll:
		return 0, len([]rune(ed.Text))
	case vimRangeVisual:
		if ed.SelectionMode != vimSelectionNone {
			return vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		}
		return vimLineRange(ed.Text, ed.Cursor, ed.Cursor)
	case vimRangeLines:
		return vimLineNumberRange(ed.Text, ed.Cursor, cmd.Range.Start, cmd.Range.End)
	case vimRangeCurrent, vimRangeDefault:
		return vimLineRange(ed.Text, ed.Cursor, ed.Cursor)
	default:
		return vimLineRange(ed.Text, ed.Cursor, ed.Cursor)
	}
}

func vimLineNumberRange(text string, cursor int, startLine int, endLine int) (int, int) {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return 0, 0
	}
	resolve := func(spec int) int {
		switch spec {
		case -1:
			return len(lines) - 1
		case 0:
			return vimLineIndexAtOffset(text, cursor)
		default:
			return spec - 1
		}
	}
	startIdx := resolve(startLine)
	endIdx := resolve(endLine)
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	startIdx = max(0, min(len(lines)-1, startIdx))
	endIdx = max(0, min(len(lines)-1, endIdx))
	return lines[startIdx].start, lines[endIdx].end
}

func handleReplaceConfirmKey(ed *Editor, key Key) bool {
	session := ed.ReplaceConfirm
	if session == nil {
		return false
	}
	switch key.Name {
	case "esc":
		ed.Text = session.OriginalText
		ed.ReplaceConfirm = nil
		ed.Status = "replace cancelled"
		return true
	}
	switch key.Rune {
	case 'y':
		session.Accepted[session.Current] = true
		return advanceReplaceConfirm(ed)
	case 'n':
		return advanceReplaceConfirm(ed)
	case 'a':
		for i := session.Current; i < len(session.Accepted); i++ {
			session.Accepted[i] = true
		}
		finishReplaceConfirm(ed)
		return true
	case 'q':
		finishReplaceConfirm(ed)
		return true
	case 'l':
		session.Accepted[session.Current] = true
		finishReplaceConfirm(ed)
		return true
	}
	return true
}

func advanceReplaceConfirm(ed *Editor) bool {
	session := ed.ReplaceConfirm
	if session == nil {
		return false
	}
	session.Current++
	if session.Current >= len(session.Candidates) {
		finishReplaceConfirm(ed)
		return true
	}
	ed.Cursor = session.Candidates[session.Current].Start
	ed.Status = replaceConfirmStatus(session)
	return true
}

func finishReplaceConfirm(ed *Editor) {
	session := ed.ReplaceConfirm
	if session == nil {
		return
	}
	count := 0
	for _, accepted := range session.Accepted {
		if accepted {
			count++
		}
	}
	if count > 0 {
		ed.Text = applyReplaceCandidates(session.OriginalText, session.Candidates, session.Accepted)
		ed.UndoStack = append(ed.UndoStack, session.Before)
		ed.UndoStack = trimSnapshots(ed.UndoStack, undoLimit())
		ed.RedoStack = nil
		ed.Dirty = true
	}
	ed.ReplaceConfirm = nil
	ed.Status = fmt.Sprintf("%d replacements", count)
}

func replaceConfirmStatus(session *replaceConfirmSession) string {
	if session == nil || len(session.Candidates) == 0 {
		return ""
	}
	return fmt.Sprintf("replace with %q? y/n/a/q/l/esc (%d/%d)", session.Candidates[session.Current].Replacement, session.Current+1, len(session.Candidates))
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

func yankVisualSelection(w *Workspace, ed *Editor) {
	switch ed.SelectionMode {
	case vimSelectionChar:
		ed.Register = vimYankChar(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		updateClipboardForRegister(w, ed, "yanked selection")
		flashYankRange(ed, min(ed.SelectionMark, ed.SelectionCursor), max(ed.SelectionMark, ed.SelectionCursor)+1)
	case vimSelectionLine:
		start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Register = vimYankLine(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		updateClipboardForRegister(w, ed, "yanked lines")
		flashYankRange(ed, start, end)
	case vimSelectionBlock:
		ed.Register = vimYankBlock(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		updateClipboardForRegister(w, ed, "yanked block")
	}
	clearVisualSelection(ed)
}

func flashYankRange(ed *Editor, start int, end int) {
	if ed == nil {
		return
	}
	runeCount := len([]rune(ed.Text))
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > runeCount {
		end = runeCount
	}
	if end <= start {
		ed.YankHighlightStart = 0
		ed.YankHighlightEnd = 0
		ed.YankHighlightUntil = time.Time{}
		return
	}
	ed.YankHighlightStart = start
	ed.YankHighlightEnd = end
	ed.YankHighlightUntil = time.Now().Add(yankHighlightDuration)
}

func clipboardPasteRegister() (vimRegister, error) {
	text, err := helpers.ReadFromClipboard()
	if err != nil {
		return vimRegister{}, err
	}
	reg := vimRegister{Kind: vimRegisterChar, Text: text}
	return normalizePasteRegister(reg), nil
}

func updateClipboardForRegister(w *Workspace, ed *Editor, success string) {
	if w != nil {
		w.Register = ed.Register
	}
	text := serializeRegisterForClipboard(ed.Register)
	if err := helpers.CopyToClipboard(text); err != nil {
		ed.Status = success + "; clipboard copy failed: " + err.Error()
		return
	}
	ed.Status = success
}

func serializeRegisterForClipboard(reg vimRegister) string {
	switch reg.Kind {
	case vimRegisterBlock:
		return strings.Join(reg.Lines, "\n")
	default:
		return reg.Text
	}
}

func deleteVisualSelection(w *Workspace, ed *Editor) {
	rememberUndoState(ed)
	switch ed.SelectionMode {
	case vimSelectionChar:
		ed.Register = vimYankChar(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		updateClipboardForRegister(w, ed, "deleted selection")
		ed.Text, ed.Cursor = vimDeleteRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	case vimSelectionLine:
		ed.Register = vimYankLine(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		updateClipboardForRegister(w, ed, "deleted lines")
		start, end := vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Text, ed.Cursor = vimDeleteRange(ed.Text, start, max(start, end-1))
		ed.Text = renumberOrderedListBlockAtOffset(ed.Text, ed.Cursor)
	case vimSelectionBlock:
		var reg vimRegister
		ed.Text, ed.Cursor, reg = vimDeleteBlockRegister(ed.Text, ed.SelectionMark, ed.SelectionCursor)
		ed.Register = reg
		updateClipboardForRegister(w, ed, "deleted block")
	default:
		return
	}
	ed.Dirty = true
	ed.Status = "deleted selection"
	clearVisualSelection(ed)
}

func executeVimCommand(w *Workspace, ed *Editor, cmd vimCommand) {
	switch cmd.Kind {
	case vimCommandSequence:
		for _, child := range cmd.Commands {
			executeVimCommand(w, ed, child)
			if child.Kind == vimCommandSave && ed.Status != "saved" {
				return
			}
		}
	case vimCommandSave:
		w.pendingSaveAll = true
		ed.Status = "saved"
	case vimCommandQuit:
		w.pendingQuit = true
		w.pendingQuitForce = cmd.Force
		ed.Status = "quit"
	case vimCommandSearch:
		ed.LastSearch = cmd.Query
		ed.LastSearchBackward = cmd.SearchBack
		idx := -1
		if cmd.SearchBack {
			idx = findPrevious(ed.Text, cmd.Query, ed.Cursor-1)
			if idx < 0 {
				idx = findPrevious(ed.Text, cmd.Query, len([]rune(ed.Text))-1)
			}
		} else {
			idx = findNext(ed.Text, cmd.Query, ed.Cursor)
			if idx < 0 {
				idx = findNext(ed.Text, cmd.Query, 0)
			}
		}
		if idx >= 0 {
			ed.Cursor = idx
			ed.LastSearchPos = idx
			ed.Status = "search hit"
		} else {
			ed.Status = "pattern not found"
		}
	case vimCommandReplace:
		executeReplaceCommand(ed, cmd)
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
	case vimCommandUndo:
		applyUndo(ed)
	case vimCommandRedo:
		applyRedo(ed)
	case vimCommandPreview:
		w.PreviewHidden = !w.PreviewHidden
		settings.SaveNotesPreviewHidden(w.PreviewHidden)
		if w.PreviewHidden {
			ed.Status = "preview hidden"
		} else {
			ed.Status = "preview shown"
		}
	case vimCommandSidebar:
		w.FocusSidebar = !w.FocusSidebar
		if w.FocusSidebar {
			ed.Status = "sidebar focused"
		} else {
			ed.Status = "editor focused"
		}
	case vimCommandAddWord:
		w.AddWordUnderCursor()
	case vimCommandSpell:
		openSpellSuggestions(w, ed)
	case vimCommandRecordKeys:
		w.pendingRecordKeys = true
		ed.Status = "key recording requested"
	case vimCommandBufferDelete:
		if w.CloseCurrentNote() {
			if active := w.ActiveEditor(); active != nil {
				active.Status = "buffer closed"
			}
		} else {
			ed.Status = "no buffer to close"
		}
	case vimCommandLineMove:
		moveCommandLines(ed, cmd.LineDelta)
	}
}

func (w *Workspace) AddWordUnderCursor() bool {
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	word := WordAtOffsetForSpell(ed.Text, ed.Cursor)
	if word == "" {
		ed.Status = "no word under cursor"
		return true
	}
	added, err := AddCustomWord(word)
	if err != nil {
		ed.Status = err.Error()
		return true
	}
	ed.SpellCacheText = ""
	ed.SpellCacheSpans = nil
	clearAutoComplete(ed)
	if added {
		ed.Status = "added word: " + word
	} else {
		ed.Status = "word already known: " + word
	}
	return true
}

func (w *Workspace) Render(width int, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 4 {
		height = 4
	}
	w.LastHeight = height
	if w.SidebarBrowsing {
		rows := w.BrowserRows(width, height)
		lines := make([]string, 0, height)
		for i := 0; i < height; i++ {
			lines = append(lines, helpers.PadANSI(lineAt(rows, i), width))
		}
		return strings.Join(lines, "\n")
	}
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
	left := w.SidebarRows(height)
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
	if w.SidebarBrowsing {
		if w.BrowserCommandMode {
			return "notes/browser command: enter run | esc cancel"
		}
		return "notes/browser: j k move | enter open/toggle/open file | o open folder notes | n/f new | r rename | m move | d delete note/folder | e close | esc close"
	}
	if w.FocusSidebar {
		return "notes/sidebar: j k move | enter focus note | e browser | x close open note | a last note | 1-9/0 note | n new note | f new folder | d delete | R rename current | [/] tabs | ctrl+a editor"
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return "notes: no note open | ctrl+a sidebar | ctrl+n new | ctrl+s save"
	}
	if ed.Mode == ModeInsert {
		return "notes/insert: tab complete or spaces | shift+tab reverse complete | ctrl+g/:spell spelling | up/down cycle suggestion | enter accept | esc normal/cancel | ctrl+s save | ctrl+a sidebar"
	}
	if ed.Mode == ModeCommand {
		return "notes/command: enter run | esc cancel | :w save | :q quit | :bd close note | :mld/:mlu move lines | /pat ?pat search | :s/pat/repl/gc replace | sidebar/sb | undo redo preview | spell"
	}
	if ed.Mode == ModeVisual {
		return "notes/visual: h j k l move | V line | :'<,'>s/pat/repl/gc replace | :mld/:mlu move selected lines | >/< indent | y yank | d/x delete | esc normal"
	}
	return "notes/normal: i insert | u undo | ctrl+g/:spell spelling | :mld/:mlu move line | >/< indent | r<char> replace | x delete | : command | /pat ?pat search | n next | N prev | :%s/pat/repl/g replace | R rename | ctrl+a sidebar"
}

func (w *Workspace) TakePendingOpenLinks() []string {
	if w == nil || len(w.pendingOpenLinks) == 0 {
		return nil
	}
	links := append([]string(nil), w.pendingOpenLinks...)
	w.pendingOpenLinks = nil
	return links
}

func (w *Workspace) TakePendingRecordKeys() bool {
	if w == nil || !w.pendingRecordKeys {
		return false
	}
	w.pendingRecordKeys = false
	return true
}

func (w *Workspace) TakePendingQuit() (bool, bool) {
	if w == nil || !w.pendingQuit {
		return false, false
	}
	force := w.pendingQuitForce
	w.pendingQuit = false
	w.pendingQuitForce = false
	return true, force
}

func (w *Workspace) TakePendingSaveAll() bool {
	if w == nil || !w.pendingSaveAll {
		return false
	}
	w.pendingSaveAll = false
	return true
}

func (w *Workspace) HasActiveYankHighlight() bool {
	if w == nil {
		return false
	}
	ed := w.ActiveEditor()
	if ed == nil || ed.YankHighlightEnd <= ed.YankHighlightStart || ed.YankHighlightUntil.IsZero() {
		return false
	}
	return time.Now().Before(ed.YankHighlightUntil)
}

func YankHighlightDuration() time.Duration {
	return yankHighlightDuration
}

func (w *Workspace) SidebarRows(height int) []string {
	width := normalizeSidebarWidth(w.SidebarWidth)
	w.SidebarRenderHeight = height
	return w.renderOpenNotes(height, width)
}

func (w *Workspace) BrowserRows(width int, height int) []string {
	w.SidebarRenderHeight = height
	return w.renderBrowserTree(height, width)
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
	if w != nil && w.BrowserCommandMode {
		return helpers.TruncateANSI(w.BrowserCommand, width)
	}
	if w != nil && (w.FocusSidebar || w.SidebarBrowsing) {
		return helpers.TruncateANSI(w.HelpText(), width)
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return helpers.TruncateANSI("no note open | ctrl+n new | ctrl+a sidebar", width)
	}
	if ed.ReplaceConfirm != nil {
		return helpers.TruncateANSI(replaceConfirmStatus(ed.ReplaceConfirm), width)
	}
	if command := pendingEditorCommandText(ed); command != "" {
		return helpers.TruncateANSI(command, width)
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
	return helpers.TruncateANSI(":w save | preview | / search | n next | N prev", width)
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
	if w.PreviewHidden {
		return sidebarWidth, contentWidth, 0
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
		row := 3 + w.sidebarVisibleSelectionRow()
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
	if w == nil || !w.FocusSidebar || len(w.activeSidebarEntries()) == 0 {
		return 0, 0, false
	}
	row := w.sidebarVisibleSelectionRow() + 1
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
	if w.BrowserCommandMode {
		return len([]rune(w.BrowserCommand)), true
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return 0, false
	}
	if ed.ReplaceConfirm != nil {
		return len([]rune(replaceConfirmStatus(ed.ReplaceConfirm))), true
	}
	if command := pendingEditorCommandText(ed); command != "" {
		return len([]rune(command)), true
	}
	if ed.Mode != ModeCommand {
		return 0, false
	}
	return len([]rune(ed.Command)), true
}

func pendingEditorCommandText(ed *Editor) string {
	if ed == nil {
		return ""
	}
	switch ed.PendingOp {
	case "m":
		return "m"
	case "ml":
		return "ml" + ed.NormalCount
	default:
		return ""
	}
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

func (w *Workspace) renderOpenNotes(height int, width int) []string {
	lines := []string{helpers.TruncateANSI(helpers.ANSI(helpers.ANSIBold, "Notes"), width)}
	start, end := w.sidebarVisibleRange(max(0, height-1))
	for i := start; i < end; i++ {
		entry := w.Tree[i]
		marker := " "
		if i == w.Selection {
			marker = helpers.ANSI(helpers.ANSIReverse, ">")
		}
		label := "o " + entry.Label
		lines = append(lines, helpers.TruncateANSI(marker+" "+label, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

func (w *Workspace) renderBrowserTree(height int, width int) []string {
	lines := []string{helpers.TruncateANSI(helpers.ANSI(helpers.ANSIBold, "Notes Browser"), width)}
	start, end := w.sidebarVisibleRange(max(0, height-1))
	for i := start; i < end; i++ {
		entry := w.BrowserTree[i]
		marker := " "
		if i == w.BrowserSelection {
			marker = helpers.ANSI(helpers.ANSIReverse, ">")
		}
		icon := "*"
		switch entry.Kind {
		case treeFolder, treeManagedFolder:
			if entry.Collapsed {
				icon = "+"
			} else {
				icon = "-"
			}
		case treeManagedAsset:
			if entry.Image {
				icon = "img"
			} else {
				icon = "file"
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

func (w *Workspace) sidebarVisibleSelectionRow() int {
	if w == nil || len(w.activeSidebarEntries()) == 0 {
		return 0
	}
	visibleRows := max(0, w.SidebarRenderHeight-1)
	if visibleRows == 0 {
		visibleRows = len(w.activeSidebarEntries())
	}
	start, _ := w.sidebarVisibleRange(visibleRows)
	row := w.activeSidebarSelection() - start
	if row < 0 {
		return 0
	}
	return row
}

func (w *Workspace) sidebarVisibleRange(visibleRows int) (int, int) {
	entries := w.activeSidebarEntries()
	if w == nil || len(entries) == 0 || visibleRows <= 0 {
		return 0, 0
	}
	if visibleRows >= len(entries) {
		return 0, len(entries)
	}
	start := w.activeSidebarSelection() - (visibleRows - 1)
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(entries) {
		end = len(entries)
		start = max(0, end-visibleRows)
	}
	return start, end
}

func (w *Workspace) renderEditor(height int, width int) []string {
	ed := w.ActiveEditor()
	if ed == nil {
		lines := []string{helpers.TruncateANSI(renderTabs(w), width)}
		for _, line := range fillLines(max(0, height-1), "No note open") {
			lines = append(lines, helpers.PadANSI(line, width))
		}
		return lines
	}
	if w.PreviewHidden {
		tabsLine := helpers.TruncateANSI(renderTabs(w), width)
		editorLines := renderEditorPane(ed, width, height-1)
		lines := []string{tabsLine}
		for i := 0; i < height-1; i++ {
			lines = append(lines, helpers.PadANSI(lineAt(editorLines, i), width))
		}
		return lines
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
	if w == nil || len(w.Tabs) == 0 {
		return "No open notes"
	}
	parts := make([]string, 0, len(w.Tabs))
	for i, tab := range w.Tabs {
		label := noteTabDisplayLabel(i, tab)
		if i == w.CurrentTab {
			label = helpers.ANSIRoleActiveTab + "[" + noteTabLabel(i, tab) + " " +
				helpers.ANSIRoleActiveTabClose + "x" +
				helpers.ANSIRoleActiveTab + "]" + "\x1b[0m"
		} else {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func (w *Workspace) tabIndexAtColumn(col int) (int, bool) {
	if w == nil || col < 0 {
		return 0, false
	}
	pos := 0
	for i, tab := range w.Tabs {
		label := "[" + noteTabDisplayLabel(i, tab) + "]"
		next := pos + len([]rune(label))
		if col >= pos && col < next {
			return i, true
		}
		pos = next
		if i < len(w.Tabs)-1 {
			if col == pos {
				return 0, false
			}
			pos++
		}
	}
	return 0, false
}

func (w *Workspace) CloseTabAtColumn(col int) bool {
	index, ok := w.tabCloseIndexAtColumn(col)
	if !ok || index < 0 || index >= len(w.Tabs) || w.Tabs[index] == nil {
		return false
	}
	return w.CloseNoteByPath(w.Tabs[index].Path)
}

func (w *Workspace) tabCloseIndexAtColumn(col int) (int, bool) {
	if w == nil || col < 0 {
		return 0, false
	}
	pos := 0
	for i, tab := range w.Tabs {
		labelWidth := len([]rune(noteTabLabel(i, tab)))
		closeCol := pos + labelWidth + 2
		next := pos + labelWidth + 4
		if col == closeCol {
			return i, true
		}
		pos = next
		if i < len(w.Tabs)-1 {
			pos++
		}
	}
	return 0, false
}

func noteTabDisplayLabel(index int, tab *Editor) string {
	return noteTabLabel(index, tab) + " x"
}

func noteTabLabel(index int, tab *Editor) string {
	label := ""
	if tab != nil {
		label = tab.Title
		if tab.Dirty {
			label += "*"
		}
	}
	if shortcut := noteTabShortcutLabel(index); shortcut != "" {
		label = shortcut + ":" + label
	}
	return label
}

func noteTabShortcutLabel(index int) string {
	if index < 0 || index > 9 {
		return ""
	}
	if index == 9 {
		return "0"
	}
	return strconv.Itoa(index + 1)
}

func noteTabShortcutIndex(shortcut string) (int, bool) {
	if shortcut == "0" {
		return 9, true
	}
	if len(shortcut) != 1 || shortcut[0] < '1' || shortcut[0] > '9' {
		return 0, false
	}
	return int(shortcut[0] - '1'), true
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
	overlaySpellSuggestionPopup(out, ed, width, ed.ScrollTop)
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
	searchQuery := editorHighlightSearchQuery(ed)
	searchSpans := groupSpansByLine(render.Text, searchHighlightSpans(render.Text, searchQuery))
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
		if searchQuery != "" && lineIdx < len(searchSpans) && len(searchSpans[lineIdx]) > 0 {
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
	start        int
	end          int
	text         string
	displayWidth int
}

func buildEditorVisualRows(ed *Editor, width int) []string {
	lines := strings.Split(ed.Text, "\n")
	lineSpans := groupSpansByLine(ed.Text, editorRenderSpans(ed.Text, settings.Inst().NotesApp.TabSpaces))
	searchQuery := editorHighlightSearchQuery(ed)
	searchSpans := groupSpansByLine(ed.Text, searchHighlightSpans(ed.Text, searchQuery))
	replaceSpans := groupSpansByLine(ed.Text, replaceConfirmHighlightSpans(ed))
	selectionSpans := groupSpansByLine(ed.Text, visualHighlightSpans(ed))
	yankSpans := groupSpansByLine(ed.Text, yankHighlightSpans(ed))
	spellSpans := groupSpansByLine(ed.Text, spellHighlightSpansForEditor(ed, ed.Text))
	gutterWidth := editorLineNumberWidth(lines, width)
	contentWidth := max(1, width-gutterWidth)
	rows := make([]string, 0, len(lines))
	for lineIdx, plainLine := range lines {
		baseSpans := []markdownSpan(nil)
		if lineIdx < len(lineSpans) {
			baseSpans = lineSpans[lineIdx]
		}
		if lineIdx < len(replaceSpans) && len(replaceSpans[lineIdx]) > 0 {
			baseSpans = replaceSpans[lineIdx]
		} else if searchQuery != "" && lineIdx < len(searchSpans) && len(searchSpans[lineIdx]) > 0 {
			baseSpans = searchSpans[lineIdx]
		} else if lineIdx < len(selectionSpans) && len(selectionSpans[lineIdx]) > 0 {
			baseSpans = selectionSpans[lineIdx]
		} else if lineIdx < len(yankSpans) && len(yankSpans[lineIdx]) > 0 {
			baseSpans = yankSpans[lineIdx]
		} else if lineIdx < len(spellSpans) && len(spellSpans[lineIdx]) > 0 {
			baseSpans = overlayMarkdownSpans(baseSpans, spellSpans[lineIdx])
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

func spellSuggestionPopupRows(ed *Editor, width int, anchorCol int) []string {
	if ed == nil || !autoCompleteActive(ed, autoCompleteSpell) || width <= 0 {
		return nil
	}
	lines := strings.Split(ed.Text, "\n")
	gutterWidth := editorLineNumberWidth(lines, width)
	contentWidth := max(1, width-gutterWidth)
	indent := anchorCol
	if indent > contentWidth-8 {
		indent = max(0, contentWidth-8)
	}
	if indent < 0 {
		indent = 0
	}
	maxItems := min(4, len(ed.AutoCompleteMatches))
	rows := make([]string, 0, maxItems)
	for i := 0; i < maxItems; i++ {
		prefix := "  "
		lineStyle := helpers.ANSIDim
		if i == ed.AutoCompleteIndex {
			prefix = "> "
			lineStyle = helpers.ANSIReverse
		}
		label := helpers.SanitizeSingleLine(ed.AutoCompleteMatches[i])
		text := prefix + label
		if helpers.VisibleRuneCount(text) > contentWidth-indent {
			text = helpers.TruncateANSI(text, max(1, contentWidth-indent))
		}
		line := strings.Repeat(" ", gutterWidth+indent) + helpers.ANSI(lineStyle, text)
		rows = append(rows, line)
	}
	return rows
}

func overlaySpellSuggestionPopup(rows []string, ed *Editor, width int, scrollTop int) {
	if ed == nil || len(rows) == 0 {
		return
	}
	anchorRow, anchorCol := editorVisualPosition(ed.Text, width, ed.AutoCompleteStart)
	popup := spellSuggestionPopupRows(ed, width, anchorCol)
	if len(popup) == 0 {
		return
	}
	start := anchorRow + 1 - max(0, scrollTop)
	for i, line := range popup {
		target := start + i
		if target < 0 || target >= len(rows) {
			continue
		}
		rows[target] = line
	}
}

func editorVisualPosition(text string, width int, offset int) (int, int) {
	lines := strings.Split(text, "\n")
	targetLine, targetCol := cursorLineCol(text, offset)
	gutterWidth := editorLineNumberWidth(lines, width)
	contentWidth := max(1, width-gutterWidth)
	rowOffset := 0
	for idx, line := range lines {
		segments := wrapPlainLine(line, contentWidth)
		if idx == targetLine {
			for segIdx, segment := range segments {
				if targetCol < segment.end || (segIdx == len(segments)-1 && targetCol <= segment.end) {
					return rowOffset + segIdx, segmentCellWidthUntil(segment, targetCol)
				}
			}
			last := segments[len(segments)-1]
			return rowOffset + len(segments) - 1, min(last.displayWidth, segmentCellWidthUntil(last, targetCol))
		}
		rowOffset += len(segments)
	}
	return 0, 0
}

func overlayMarkdownSpans(base []markdownSpan, overlays []markdownSpan) []markdownSpan {
	if len(overlays) == 0 {
		return base
	}
	out := append([]markdownSpan(nil), base...)
	for _, overlay := range overlays {
		if overlay.End <= overlay.Start {
			continue
		}
		next := make([]markdownSpan, 0, len(out)+1)
		for _, span := range out {
			if overlay.Start >= span.End || overlay.End <= span.Start {
				next = append(next, span)
				continue
			}
			if span.Start < overlay.Start {
				next = append(next, markdownSpan{Tag: span.Tag, Start: span.Start, End: overlay.Start})
			}
			if overlay.End < span.End {
				next = append(next, markdownSpan{Tag: span.Tag, Start: overlay.End, End: span.End})
			}
		}
		next = append(next, overlay)
		out = next
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].End < out[j].End
		}
		return out[i].Start < out[j].Start
	})
	return out
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
		return []wrappedSegment{{start: 0, end: 0, text: "", displayWidth: 0}}
	}
	segments := make([]wrappedSegment, 0, len(runes)/width+1)
	for start := 0; start < len(runes); {
		end := start
		cells := 0
		lastBreak := -1
		lastSoftBreak := -1
		for end < len(runes) {
			rw := runeCellWidth(runes[end])
			if cells > 0 && cells+rw > width {
				break
			}
			if cells == 0 && rw > width {
				rw = width
			}
			cells += rw
			if runes[end] == ' ' || runes[end] == '\t' {
				lastBreak = end
			}
			if isSoftWrapRune(runes[end]) {
				lastSoftBreak = end
			}
			end++
			if cells >= width {
				break
			}
		}
		segEnd := end
		next := end
		if end < len(runes) && lastBreak > start {
			segEnd = lastBreak
			for segEnd > start && (runes[segEnd-1] == ' ' || runes[segEnd-1] == '\t') {
				segEnd--
			}
			next = lastBreak + 1
			for next < len(runes) && (runes[next] == ' ' || runes[next] == '\t') {
				next++
			}
		} else if end < len(runes) && lastSoftBreak > start {
			segEnd = lastSoftBreak + 1
			next = segEnd
		}
		text := string(runes[start:segEnd])
		segments = append(segments, wrappedSegment{start: start, end: segEnd, text: text, displayWidth: runewidth.StringWidth(text)})
		if next <= start {
			next = start + 1
		}
		start = next
	}
	if len(segments) == 0 {
		return []wrappedSegment{{start: 0, end: 0, text: "", displayWidth: 0}}
	}
	return segments
}

func isSoftWrapRune(r rune) bool {
	switch r {
	case '/', '-', '_', '.', '?', '&', '=', '#':
		return true
	default:
		return false
	}
}

func runeCellWidth(r rune) int {
	if r == '\t' {
		return settings.Inst().NotesApp.TabSpaces
	}
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return 1
	}
	return w
}

func segmentCellWidthUntil(segment wrappedSegment, lineCol int) int {
	if lineCol <= segment.start {
		return 0
	}
	if lineCol >= segment.end {
		return segment.displayWidth
	}
	runes := []rune(segment.text)
	cells := 0
	for i := 0; i < lineCol-segment.start && i < len(runes); i++ {
		cells += runeCellWidth(runes[i])
	}
	return cells
}

func segmentRuneOffsetAtCell(segment wrappedSegment, cell int) int {
	if cell <= 0 {
		return segment.start
	}
	cells := 0
	for idx, r := range []rune(segment.text) {
		next := cells + runeCellWidth(r)
		if cell < next {
			return segment.start + idx
		}
		cells = next
	}
	return segment.end
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
					return rowOffset + segIdx, segmentCellWidthUntil(segment, cursorCol)
				}
			}
			last := segments[len(segments)-1]
			return rowOffset + len(segments) - 1, min(last.displayWidth, segmentCellWidthUntil(last, cursorCol))
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

func lineStartOffsets(text string) []int {
	runes := []rune(text)
	offsets := []int{0}
	for i, r := range runes {
		if r == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func (w *Workspace) EditorOffsetAtVisualPosition(row int, col int) (int, bool) {
	if w == nil {
		return 0, false
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return 0, false
	}
	lines := strings.Split(ed.Text, "\n")
	gutterWidth := editorLineNumberWidth(lines, w.editorRenderWidth())
	targetRow := ed.ScrollTop + max(0, row)
	cell := col - gutterWidth
	if cell < 0 {
		cell = 0
	}
	return editorOffsetAtAbsoluteVisualPosition(ed.Text, w.editorRenderWidth(), targetRow, cell), true
}

func editorOffsetAtAbsoluteVisualPosition(text string, width int, targetRow int, cell int) int {
	lines := strings.Split(text, "\n")
	offsets := lineStartOffsets(text)
	gutterWidth := editorLineNumberWidth(lines, width)
	contentWidth := max(1, width-gutterWidth)
	if targetRow < 0 {
		targetRow = 0
	}
	if cell < 0 {
		cell = 0
	}
	visualRow := 0
	for lineIdx, line := range lines {
		segments := wrapPlainLine(line, contentWidth)
		for _, segment := range segments {
			if visualRow == targetRow {
				if cell > segment.displayWidth {
					cell = segment.displayWidth
				}
				return offsets[lineIdx] + segmentRuneOffsetAtCell(segment, cell)
			}
			visualRow++
		}
	}
	return len([]rune(text))
}

func (w *Workspace) MoveEditorCursorToVisualPosition(row int, col int) bool {
	offset, ok := w.EditorOffsetAtVisualPosition(row, col)
	if !ok {
		return false
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	w.leaveSidebar()
	clearVisualSelection(ed)
	ed.Cursor = vimClampOffset(ed.Text, offset)
	w.ensureEditorVisible()
	return true
}

func (w *Workspace) MoveEditorCursorByVisualRows(delta int) bool {
	if w == nil {
		return false
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	currentRow, currentCol := editorVisualCursor(ed, w.editorRenderWidth())
	offset := editorOffsetAtAbsoluteVisualPosition(ed.Text, w.editorRenderWidth(), currentRow+delta, currentCol)
	w.leaveSidebar()
	clearVisualSelection(ed)
	ed.Cursor = vimClampOffset(ed.Text, offset)
	w.ensureEditorVisible()
	return true
}

func (w *Workspace) BeginMouseSelection(row int, col int) bool {
	if !w.MoveEditorCursorToVisualPosition(row, col) {
		return false
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	ed.SelectionMode = vimSelectionChar
	ed.SelectionMark = ed.Cursor
	ed.SelectionCursor = ed.Cursor
	return true
}

func (w *Workspace) DragMouseSelection(row int, col int) bool {
	offset, ok := w.EditorOffsetAtVisualPosition(row, col)
	if !ok {
		return false
	}
	ed := w.ActiveEditor()
	if ed == nil {
		return false
	}
	w.leaveSidebar()
	ed.Cursor = vimClampOffset(ed.Text, offset)
	ed.SelectionCursor = ed.Cursor
	ed.SelectionMode = vimSelectionChar
	ed.Mode = ModeVisual
	w.ensureEditorVisible()
	return true
}

func (w *Workspace) SelectSidebarRow(row int, open bool) bool {
	entries := w.activeSidebarEntries()
	if w == nil || len(entries) == 0 {
		return false
	}
	idx := row - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(entries) {
		idx = len(entries) - 1
	}
	w.FocusSidebar = true
	if w.SidebarBrowsing {
		w.BrowserSelection = idx
	} else {
		w.Selection = idx
	}
	if open {
		return w.handleSidebarKey(Key{Name: "enter"})
	}
	return true
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
	spans = resolvedMarkdownSpans(spans)
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

func resolvedMarkdownSpans(spans []markdownSpan) []markdownSpan {
	if len(spans) <= 1 {
		return spans
	}
	ordered := append([]markdownSpan(nil), spans...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftLen := ordered[i].End - ordered[i].Start
		rightLen := ordered[j].End - ordered[j].Start
		if leftLen == rightLen {
			if ordered[i].Start == ordered[j].Start {
				return ordered[i].End < ordered[j].End
			}
			return ordered[i].Start < ordered[j].Start
		}
		return leftLen > rightLen
	})
	out := make([]markdownSpan, 0, len(ordered))
	for _, span := range ordered {
		if span.End <= span.Start {
			continue
		}
		out = overlayMarkdownSpans(out, []markdownSpan{span})
	}
	return out
}

func styleForMarkdownTag(tag string, text string) string {
	switch tag {
	case tagHeading1:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleHeading1, text)
	case tagHeading2:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleHeading2, text)
	case tagHeading3:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleHeading3, text)
	case tagHeading4:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleHeading4, text)
	case tagHeading5:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleHeading5, text)
	case tagHeading6:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleHeading6, text)
	case tagBold:
		return helpers.ANSI(helpers.ANSIBold, text)
	case tagItalic:
		return helpers.ANSI(helpers.ANSIItalic, text)
	case tagQuote, tagCodeComment:
		return helpers.ANSI(helpers.ANSIDim+helpers.ANSIRoleComment, text)
	case tagCode:
		return helpers.ANSI(helpers.ANSIRoleCode, text)
	case tagCodeString:
		return helpers.ANSI(helpers.ANSIRoleString, text)
	case tagCodeKeyword:
		return helpers.ANSI(helpers.ANSIRoleKeyword, text)
	case tagList, tagOrdered, tagChecklist:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleListMarker, text)
	case tagCodeNumber:
		return helpers.ANSI(helpers.ANSIRoleNumber, text)
	case tagCodeType:
		return helpers.ANSI(helpers.ANSIRoleType, text)
	case tagCodeFunction:
		return helpers.ANSI(helpers.ANSIRoleFunction, text)
	case tagCodeProperty, tagLink:
		return helpers.ANSI(helpers.ANSIRoleProperty, text)
	case tagCodeConstant:
		return helpers.ANSI(helpers.ANSIBold+helpers.ANSIRoleConstant, text)
	case tagSearch:
		return helpers.ANSI(helpers.ANSIRoleSearch, text)
	case tagReplaceCurrent:
		return helpers.ANSI(helpers.ANSIRoleVisualSelection, text)
	case tagVisualSelection:
		return helpers.ANSI(helpers.ANSIRoleVisualSelection, text)
	case tagYankHighlight:
		return helpers.ANSI(helpers.ANSIRoleSelection, text)
	case tagSpellError:
		return helpers.ANSI(helpers.ANSIRoleSpellError, text)
	case tagCodeBlock:
		return text
	default:
		return text
	}
}

func yankHighlightSpans(ed *Editor) []markdownSpan {
	if ed == nil || ed.YankHighlightEnd <= ed.YankHighlightStart || ed.YankHighlightUntil.IsZero() {
		return nil
	}
	if time.Now().After(ed.YankHighlightUntil) {
		return nil
	}
	return []markdownSpan{{Tag: tagYankHighlight, Start: ed.YankHighlightStart, End: ed.YankHighlightEnd}}
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

func replaceConfirmHighlightSpans(ed *Editor) []markdownSpan {
	if ed == nil || ed.ReplaceConfirm == nil || ed.ReplaceConfirm.Current >= len(ed.ReplaceConfirm.Candidates) {
		return nil
	}
	candidate := ed.ReplaceConfirm.Candidates[ed.ReplaceConfirm.Current]
	return []markdownSpan{{Tag: tagReplaceCurrent, Start: candidate.Start, End: candidate.End}}
}

func editorHighlightSearchQuery(ed *Editor) string {
	if ed == nil {
		return ""
	}
	if ed.Mode == ModeCommand {
		if strings.HasPrefix(ed.Command, "/") || strings.HasPrefix(ed.Command, "?") {
			return strings.TrimSpace(ed.Command[1:])
		}
		if query, ok := liveSubstituteSearchQuery(ed.Command); ok {
			return query
		}
		return ""
	}
	return ed.LastSearch
}

func liveSubstituteSearchQuery(command string) (string, bool) {
	sIndex := substituteCommandIndex(command)
	if sIndex < 0 {
		return "", false
	}
	if sIndex+1 >= len(command) {
		return "", true
	}
	delimiter := rune(command[sIndex+1])
	if delimiter == '\\' || delimiter == 0 {
		return "", true
	}
	rest := command[sIndex+2:]
	pattern, _, ok := readDelimitedField(rest, delimiter)
	if !ok {
		pattern = rest
	}
	return strings.TrimSpace(unescapeDelimited(pattern, delimiter)), true
}

func searchHighlightSpans(text string, query string) []markdownSpan {
	if query == "" {
		return nil
	}
	if re, err := compileVimRegex(query, true); err == nil {
		matches := re.FindAllStringIndex(text, -1)
		spans := make([]markdownSpan, 0, len(matches))
		for _, match := range matches {
			if match[0] == match[1] {
				continue
			}
			spans = append(spans, markdownSpan{
				Tag:   tagSearch,
				Start: utf8.RuneCountInString(text[:match[0]]),
				End:   utf8.RuneCountInString(text[:match[1]]),
			})
		}
		return spans
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
	spans := editorMarkdownSpans(text)
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

func editorMarkdownSpans(text string) []markdownSpan {
	lines := strings.SplitAfter(text, "\n")
	spans := make([]markdownSpan, 0)
	offset := 0
	inCodeBlock := false
	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\n")
		lineEnd := offset + runeLen(line)
		trimmed := strings.TrimLeft(line, " \t")
		indent := runeLen(line[:len(line)-len(trimmed)])
		if strings.HasPrefix(trimmed, "```") {
			spans = append(spans, markdownSpan{Tag: tagCodeBlock, Start: offset, End: lineEnd})
			inCodeBlock = !inCodeBlock
			offset += runeLen(rawLine)
			continue
		}
		if inCodeBlock {
			spans = append(spans, markdownSpan{Tag: tagCodeBlock, Start: offset, End: lineEnd})
			offset += runeLen(rawLine)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "# "):
			spans = append(spans, markdownSpan{Tag: tagHeading1, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "## "):
			spans = append(spans, markdownSpan{Tag: tagHeading2, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "### "):
			spans = append(spans, markdownSpan{Tag: tagHeading3, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "#### "):
			spans = append(spans, markdownSpan{Tag: tagHeading4, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "##### "):
			spans = append(spans, markdownSpan{Tag: tagHeading5, Start: offset + indent, End: lineEnd})
		case strings.HasPrefix(trimmed, "###### "):
			spans = append(spans, markdownSpan{Tag: tagHeading6, Start: offset + indent, End: lineEnd})
		case isHorizontalRule(trimmed):
			spans = append(spans, markdownSpan{Tag: tagHorizontalRule, Start: offset + indent, End: lineEnd})
		case checklistMarkerLength(trimmed) > 0:
			spans = append(spans, markdownSpan{Tag: tagChecklist, Start: offset + indent, End: offset + indent + checklistMarkerLength(trimmed)})
		case unorderedListMarkerLength(trimmed) > 0:
			spans = append(spans, markdownSpan{Tag: tagList, Start: offset + indent, End: offset + indent + unorderedListMarkerLength(trimmed)})
		case orderedListPrefixLength(trimmed) > 0:
			spans = append(spans, markdownSpan{Tag: tagOrdered, Start: offset + indent, End: offset + indent + orderedListMarkerLength(trimmed)})
		case strings.HasPrefix(trimmed, "> "):
			spans = append(spans, markdownSpan{Tag: tagQuote, Start: offset + indent, End: lineEnd})
		}
		spans = append(spans, editorInlineMarkdownSpans(line, offset)...)
		offset += runeLen(rawLine)
	}
	return spans
}

func editorInlineMarkdownSpans(line string, offset int) []markdownSpan {
	spans := make([]markdownSpan, 0)
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "![") {
			if endLabel := strings.IndexByte(line[i+2:], ']'); endLabel >= 0 {
				labelEnd := i + 2 + endLabel
				if labelEnd+1 < len(line) && line[labelEnd+1] == '(' {
					if endURL := strings.IndexByte(line[labelEnd+2:], ')'); endURL >= 0 {
						i = labelEnd + 2 + endURL + 1
						continue
					}
				}
			}
		}
		if strings.HasPrefix(line[i:], "**") || strings.HasPrefix(line[i:], "__") {
			delim := line[i : i+2]
			if closeIdx := strings.Index(line[i+2:], delim); closeIdx >= 0 {
				start := runeLen(line[:i+2])
				end := runeLen(line[:i+2+closeIdx+2])
				spans = append(spans, markdownSpan{Tag: tagBold, Start: offset + start, End: offset + end})
				i += 2 + closeIdx + 2
				continue
			}
		}
		if line[i] == '`' {
			if closeIdx := strings.IndexByte(line[i+1:], '`'); closeIdx >= 0 {
				start := runeLen(line[:i])
				end := runeLen(line[:i+1+closeIdx+1])
				spans = append(spans, markdownSpan{Tag: tagCode, Start: offset + start, End: offset + end})
				i += closeIdx + 2
				continue
			}
		}
		if line[i] == '[' {
			if endLabel := strings.IndexByte(line[i:], ']'); endLabel >= 0 {
				labelEnd := i + endLabel
				if labelEnd+1 < len(line) && line[labelEnd+1] == '(' {
					if endURL := strings.IndexByte(line[labelEnd+2:], ')'); endURL >= 0 {
						start := runeLen(line[:i+1])
						end := runeLen(line[:labelEnd])
						spans = append(spans, markdownSpan{Tag: tagLink, Start: offset + start, End: offset + end})
						i = labelEnd + 2 + endURL + 1
						continue
					}
				}
			}
		}
		if (line[i] == '*' || line[i] == '_') && !isDoubleDelimiter(line, i, line[i]) {
			if closeIdx := strings.IndexByte(line[i+1:], line[i]); closeIdx >= 0 {
				start := runeLen(line[:i+1])
				end := runeLen(line[:i+1+closeIdx+1])
				spans = append(spans, markdownSpan{Tag: tagItalic, Start: offset + start, End: offset + end})
				i += closeIdx + 2
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		if size <= 0 {
			size = 1
		}
		i += size
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

func sidebarTargetFolder(entry *TreeEntry) string {
	if entry == nil {
		return ""
	}
	switch entry.Kind {
	case treeFolder:
		return entry.Folder
	case treeNote, treeOpenNote:
		return relativeNoteFolder(entry.Path)
	default:
		return ""
	}
}

func browserTargetFolder(entry *TreeEntry) string {
	if entry == nil {
		return ""
	}
	switch entry.Kind {
	case treeFolder:
		return entry.Folder
	case treeNote, treeOpenNote:
		return relativeNoteFolder(entry.Path)
	default:
		return ""
	}
}

func resolveNoteMoveTarget(raw string, currentPath string) (string, string, error) {
	target := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if target == "" {
		return "", "", fmt.Errorf("move requires a target")
	}
	target = strings.TrimLeft(target, "/")
	if target == "" {
		return "", "", fmt.Errorf("move requires a target")
	}
	forceFolder := strings.HasSuffix(target, "/")
	target = strings.Trim(target, "/")
	if target == "" {
		return "", "", fmt.Errorf("move requires a target")
	}
	parts := strings.Split(target, "/")
	currentTitle := noteTitleFromPath(currentPath)
	if forceFolder || len(parts) == 1 {
		return sanitizeFolderPath(target), currentTitle, nil
	}
	title := strings.TrimSuffix(parts[len(parts)-1], ".md")
	if strings.TrimSpace(title) == "" {
		return "", "", fmt.Errorf("move requires a note name")
	}
	folder := sanitizeFolderPath(strings.Join(parts[:len(parts)-1], "/"))
	return folder, title, nil
}

func resolveFolderMoveTarget(raw string) (string, error) {
	target := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if target == "" {
		return "", fmt.Errorf("move requires a target")
	}
	target = strings.Trim(target, "/")
	return sanitizeFolderPath(target), nil
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
