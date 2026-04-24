package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
)

type UserSettings struct {
	PasswordApp PasswordAppSettings `json:"password_app"`
	PagesApp    PagesAppSettings    `json:"pages_app"`
	NotesApp    NotesAppSettings    `json:"notes_app"`
	AppWindow   AppWindowSettings   `json:"app_window"`
	UI          *UISettings         `json:"ui"`
	GDrive      *GDriveSettings     `json:"gdrive"`
}

type AppWindowSettings struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

type PagesAppSettings struct {
	FirstBookPages  int `json:"first_book"`
	SecondBookPages int `json:"second_book"`
	ReadPages       int `json:"read_pages"`
}

type PasswordAppSettings struct {
	Letters        bool `json:"letters"`
	Numbers        bool `json:"numbers"`
	SpecialSymbols bool `json:"special_symbols"`
	SymbolCount    int  `json:"symbol_count"`
}

type NotesAppSettings struct {
	TabSpaces         int      `json:"tab_spaces"`
	UndoLevels        int      `json:"undo_levels"`
	EditorWidth       int      `json:"editor_width,omitempty"`
	PreviewHidden     bool     `json:"preview_hidden,omitempty"`
	OpenNotePaths     []string `json:"open_note_paths,omitempty"`
	CurrentNotePath   string   `json:"current_note_path,omitempty"`
	SidebarVisible    bool     `json:"sidebar_visible"`
	VimMode           bool     `json:"vim_mode"`
	SpellCheckEnabled bool     `json:"spell_check_enabled,omitempty"`
	SpellDictionaries []string `json:"spell_dictionaries,omitempty"`
}

func (n *NotesAppSettings) UnmarshalJSON(data []byte) error {
	type notesAppAlias NotesAppSettings
	aux := struct {
		SidebarVisible *bool `json:"sidebar_visible"`
		*notesAppAlias
	}{
		notesAppAlias: (*notesAppAlias)(n),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.SidebarVisible == nil {
		n.SidebarVisible = true
	}
	return nil
}

type UISettings struct {
	ShowPages             bool   `json:"show_pages"`
	ShowPassword          bool   `json:"show_password"`
	ShowNotes             bool   `json:"show_notes"`
	Theme                 string `json:"theme,omitempty"`
	TransparentBackground bool   `json:"transparent_background,omitempty"`
}

type GDriveSettings struct {
	Enabled             bool                `json:"enabled"`
	SyncIntervalSec     int                 `json:"sync_interval_sec"`
	FolderID            string              `json:"folder_id"`
	FolderName          string              `json:"folder_name"`
	PendingSync         bool                `json:"pending_sync,omitempty"`
	LastRemoteState     string              `json:"last_remote_state,omitempty"`
	ConflictRemoteState string              `json:"conflict_remote_state,omitempty"`
	LastSyncAt          string              `json:"last_sync_at"`
	LastSyncStatus      string              `json:"last_sync_status"`
	LastSyncMessage     string              `json:"last_sync_message"`
	LastLocalSaveAt     string              `json:"last_local_save_at"`
	LastDriveSaveAt     string              `json:"last_drive_save_at"`
	LastDriveRefreshAt  string              `json:"last_drive_refresh_at"`
	SelectedSnapshotID  string              `json:"selected_snapshot_id,omitempty"`
	Snapshots           []DriveSnapshotMeta `json:"snapshots,omitempty"`
}

type DriveSnapshotMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

var settingsInstance *UserSettings
var saveHooks []func(*UserSettings)
var driveSyncInFlight atomic.Bool
var driveRefreshInFlight atomic.Bool
var shuttingDown atomic.Bool
var driveSyncFunc func() error
var driveUploadSnapshotFunc func(folderID string, settingsData []byte, retain int) (gdrive.SnapshotMeta, error)
var driveListSnapshotsFunc func(folderID string) ([]gdrive.SnapshotMeta, error)
var driveRestoreSnapshotFunc func(snapshotID string) ([]byte, error)
var statusUpdater = func(text string) {
	if helpers.HasStatusBar() {
		helpers.StatusBarInst().UpdateStatusBar(text)
	}
}

const DefaultTheme = "tokyo-night"

var BuiltInThemes = []string{
	"tokyo-night",
	"catppuccin",
	"kanagawa",
	"gruvbox",
	"rose-pine",
	"flexoki",
}

func Inst() *UserSettings {
	if settingsInstance == nil {
		log.Fatal("Settings is not initialized")
	}

	return settingsInstance
}

func Init() *[]string {
	shuttingDown.Store(false)
	var messages []string
	fn := fileName()
	c, err := os.ReadFile(fn)
	if err != nil {
		msg := fmt.Sprintf("Settings read error: %s", err)
		log.Println(msg)
		messages = append(messages, msg)
		settingsInstance = defaultSettings()
		normalizeSettings(settingsInstance)
		return &messages
	}

	marshalError := json.Unmarshal(c, &settingsInstance)
	if marshalError != nil {
		log.Println("Settings unmarshal error: ", marshalError)
		ct := time.Now()
		backupFileName := getFileName(ct.Format("2006-01-02_15.04.0000") + "settings.json")
		err = os.Rename(fn, backupFileName)
		if err != nil {
			log.Println("Cannot back up settings: ", err)
		} else {
			msg := "Old settings backed up to: " + backupFileName
			messages = append(messages, msg)
			log.Println(msg)
		}
		settingsInstance = defaultSettings()
	}

	normalizeSettings(settingsInstance)

	return &messages
}

func defaultSettings() *UserSettings {
	return &UserSettings{
		PasswordApp: PasswordAppSettings{
			Letters:        true,
			Numbers:        true,
			SpecialSymbols: true,
			SymbolCount:    16,
		},
		PagesApp: PagesAppSettings{
			FirstBookPages:  0,
			SecondBookPages: 0,
			ReadPages:       0,
		},
		AppWindow: AppWindowSettings{
			Width:  600,
			Height: 300,
		},
		NotesApp: NotesAppSettings{
			TabSpaces:      4,
			UndoLevels:     1000,
			SidebarVisible: true,
			VimMode:        true,
		},
		UI:     defaultUISettings(),
		GDrive: defaultGDriveSettings(),
	}
}

func defaultGDriveSettings() *GDriveSettings {
	return &GDriveSettings{
		SyncIntervalSec: 10,
	}
}

func defaultUISettings() *UISettings {
	return &UISettings{
		ShowPages:             true,
		ShowPassword:          true,
		ShowNotes:             true,
		Theme:                 DefaultTheme,
		TransparentBackground: false,
	}
}

func normalizeSettings(s *UserSettings) {
	if s == nil {
		return
	}
	if s.GDrive == nil {
		s.GDrive = defaultGDriveSettings()
	}
	if s.GDrive.SyncIntervalSec <= 0 {
		s.GDrive.SyncIntervalSec = 10
	}
	if s.UI == nil {
		s.UI = defaultUISettings()
	}
	if !ValidTheme(s.UI.Theme) {
		s.UI.Theme = DefaultTheme
	}
	if s.AppWindow.Width <= 0 {
		s.AppWindow.Width = 600
	}
	if s.AppWindow.Height <= 0 {
		s.AppWindow.Height = 300
	}
	if s.NotesApp.TabSpaces <= 0 {
		s.NotesApp.TabSpaces = 4
	}
	if s.NotesApp.UndoLevels <= 0 {
		s.NotesApp.UndoLevels = 1000
	}
	if len(s.NotesApp.OpenNotePaths) > 0 {
		normalized := make([]string, 0, len(s.NotesApp.OpenNotePaths))
		seen := make(map[string]struct{}, len(s.NotesApp.OpenNotePaths))
		for _, path := range s.NotesApp.OpenNotePaths {
			path = normalizeNoteSessionPath(path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			normalized = append(normalized, path)
		}
		s.NotesApp.OpenNotePaths = normalized
	}
	s.NotesApp.CurrentNotePath = normalizeNoteSessionPath(s.NotesApp.CurrentNotePath)
	if len(s.NotesApp.SpellDictionaries) > 0 {
		normalized := make([]string, 0, len(s.NotesApp.SpellDictionaries))
		seen := make(map[string]struct{}, len(s.NotesApp.SpellDictionaries))
		for _, code := range s.NotesApp.SpellDictionaries {
			code = strings.ToLower(strings.TrimSpace(code))
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			normalized = append(normalized, code)
		}
		s.NotesApp.SpellDictionaries = normalized
	}
	if s.NotesApp.TabSpaces == 4 && !s.NotesApp.VimMode && s.NotesApp.EditorWidth == 0 && !s.NotesApp.SidebarVisible {
		s.NotesApp.SidebarVisible = true
	}
}

func CurrentTheme() string {
	if settingsInstance == nil || settingsInstance.UI == nil || !ValidTheme(settingsInstance.UI.Theme) {
		return DefaultTheme
	}
	return settingsInstance.UI.Theme
}

func ValidTheme(theme string) bool {
	for _, candidate := range BuiltInThemes {
		if theme == candidate {
			return true
		}
	}
	return false
}

func NextTheme(theme string) string {
	for i, candidate := range BuiltInThemes {
		if theme == candidate {
			return BuiltInThemes[(i+1)%len(BuiltInThemes)]
		}
	}
	return DefaultTheme
}

func RegisterSaveHook(fn func(*UserSettings)) {
	saveHooks = append(saveHooks, fn)
}

func runSaveHooks(notifyHooks bool) {
	if !notifyHooks {
		return
	}
	for _, hook := range saveHooks {
		hook(settingsInstance)
	}
}

func getFileName(n string) string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	return filepath.Join(
		dirname,
		helpers.AppConfigMainDir,
		helpers.AppConfigAppDir,
		n)
}

func fileName() string {
	return getFileName("settings.json")
}

func (g *GDriveSettings) Ready() bool {
	return g != nil && g.Enabled && g.FolderID != "" && gdrive.HasCredentials() && gdrive.HasToken()
}

func SaveSettings() {
	SaveSettingsLocal()
}

func SaveSettingsLocal() {
	MarkDriveDirty()
	if settingsInstance != nil && settingsInstance.GDrive != nil {
		settingsInstance.GDrive.LastLocalSaveAt = time.Now().Format(time.RFC3339)
	}
	writeSettingsToDisk(false)
}

func saveSettings(sync bool, notifyHooks bool) {
	if sync {
		MarkDriveDirty()
		if settingsInstance != nil && settingsInstance.GDrive != nil {
			settingsInstance.GDrive.LastLocalSaveAt = time.Now().Format(time.RFC3339)
		}
	}
	writeSettingsToDisk(notifyHooks)
}

func StartDriveSync() {
}

func StartDriveRefresh() {
}

func SyncDriveDataOnShutdown(ctx context.Context) error {
	return ctx.Err()
}

func BeginShutdown() {
	shuttingDown.Store(true)
}

func IsShuttingDown() bool {
	return shuttingDown.Load()
}

func DriveSyncInterval() time.Duration {
	if settingsInstance == nil || settingsInstance.GDrive == nil || settingsInstance.GDrive.SyncIntervalSec <= 0 {
		return 10 * time.Second
	}
	return time.Duration(settingsInstance.GDrive.SyncIntervalSec) * time.Second
}

func SaveNotesEditorWidth(width int) {
	if settingsInstance == nil || width <= 0 {
		return
	}
	if settingsInstance.NotesApp.EditorWidth == width {
		return
	}
	settingsInstance.NotesApp.EditorWidth = width
}

func SaveNotesPreviewHidden(hidden bool) {
	if settingsInstance == nil {
		return
	}
	if settingsInstance.NotesApp.PreviewHidden == hidden {
		return
	}
	settingsInstance.NotesApp.PreviewHidden = hidden
	writeSettingsToDisk(false)
}

func SaveNotesSession(paths []string, currentPath string) {
	if UpdateNotesSession(paths, currentPath) {
		writeSettingsToDisk(false)
	}
}

func UpdateNotesSession(paths []string, currentPath string) bool {
	if settingsInstance == nil {
		return false
	}
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = normalizeNoteSessionPath(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}
	currentPath = normalizeNoteSessionPath(currentPath)
	if slicesEqual(settingsInstance.NotesApp.OpenNotePaths, normalized) && settingsInstance.NotesApp.CurrentNotePath == currentPath {
		return false
	}
	settingsInstance.NotesApp.OpenNotePaths = normalized
	settingsInstance.NotesApp.CurrentNotePath = currentPath
	return true
}

func normalizeNoteSessionPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		cleaned := filepath.ToSlash(filepath.Clean(path))
		if cleaned == "." {
			return ""
		}
		return cleaned
	}

	notesRoot := getFileName("notes")
	if rel, err := filepath.Rel(notesRoot, path); err == nil && rel != "." && rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.ToSlash(rel)
	}

	marker := "/" + filepath.ToSlash(filepath.Join(helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes")) + "/"
	slashPath := filepath.ToSlash(filepath.Clean(path))
	if idx := strings.Index(slashPath, marker); idx >= 0 {
		rel := slashPath[idx+len(marker):]
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel != "." && rel != "" && rel != ".." && !strings.HasPrefix(rel, "../") {
			return rel
		}
	}

	return filepath.ToSlash(filepath.Clean(path))
}

func slicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func SaveAppWindowState(width int, height int, maximized bool) {
	if settingsInstance == nil {
		return
	}
	if width > 0 {
		settingsInstance.AppWindow.Width = width
	}
	if height > 0 {
		settingsInstance.AppWindow.Height = height
	}
	settingsInstance.AppWindow.Maximized = maximized
}

func PersistedNotesEditorWidth() int {
	if settingsInstance != nil && settingsInstance.NotesApp.EditorWidth > 0 {
		return settingsInstance.NotesApp.EditorWidth
	}

	data, err := os.ReadFile(fileName())
	if err != nil {
		return 0
	}

	var persisted struct {
		NotesApp struct {
			EditorWidth int `json:"editor_width"`
		} `json:"notes_app"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		return 0
	}
	if persisted.NotesApp.EditorWidth > 0 {
		return persisted.NotesApp.EditorWidth
	}
	return 0
}

func writeSettingsToDisk(notifyHooks bool) {
	file := fileName()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		log.Println(err)
		statusUpdater("Couldn't prepare settings directory... :(")
		return
	}
	dataString, err := json.Marshal(settingsInstance)
	if err != nil {
		log.Println(err)
		statusUpdater("Couldn't stringify settings... :(")
		return
	}
	if err := os.WriteFile(file, dataString, 0o644); err != nil {
		log.Println(err)
		statusUpdater("Couldn't save settings... :(")
		return
	}
	runSaveHooks(notifyHooks)
	statusUpdater("Settings saved!")
}

func SyncDriveData() error {
	if settingsInstance == nil || settingsInstance.GDrive == nil || !settingsInstance.GDrive.Ready() {
		return errors.New("Google Drive is not ready")
	}
	if !driveSyncInFlight.CompareAndSwap(false, true) {
		return nil
	}
	defer driveSyncInFlight.Store(false)

	settingsData, dataErr := driveSyncSettingsJSON()
	if dataErr != nil {
		recordSyncResult(dataErr)
		writeSettingsToDisk(false)
		return dataErr
	}

	if driveSyncFunc != nil {
		err := driveSyncFunc()
		recordSyncResult(err)
		writeSettingsToDisk(false)
		return err
	}

	uploadSnapshot := driveUploadSnapshotFunc
	if uploadSnapshot == nil {
		uploadSnapshot = gdrive.UploadAppSnapshot
	}
	snapshot, err := uploadSnapshot(settingsInstance.GDrive.FolderID, settingsData, 5)
	recordSyncResult(err)
	if err == nil {
		settingsInstance.GDrive.PendingSync = false
		settingsInstance.GDrive.LastDriveSaveAt = snapshot.CreatedAt
		settingsInstance.GDrive.SelectedSnapshotID = snapshot.ID
		if err := RefreshDriveSnapshots(); err != nil {
			log.Println("drive snapshot refresh error:", err)
		}
	}
	writeSettingsToDisk(false)
	return err
}

func driveSyncSettingsJSON() ([]byte, error) {
	if settingsInstance == nil {
		return nil, nil
	}

	clone := *settingsInstance
	if settingsInstance.UI != nil {
		uiCopy := *settingsInstance.UI
		clone.UI = &uiCopy
	}
	if settingsInstance.GDrive != nil {
		gDriveCopy := *settingsInstance.GDrive
		clone.GDrive = &gDriveCopy
	}

	if clone.GDrive != nil {
		clone.GDrive.PendingSync = false
	}

	return json.Marshal(&clone)
}

func RefreshDriveSnapshots() error {
	if settingsInstance == nil || settingsInstance.GDrive == nil || !settingsInstance.GDrive.Ready() {
		return errors.New("Google Drive is not ready")
	}
	if !driveRefreshInFlight.CompareAndSwap(false, true) {
		return nil
	}
	defer driveRefreshInFlight.Store(false)
	listSnapshots := driveListSnapshotsFunc
	if listSnapshots == nil {
		listSnapshots = gdrive.ListAppSnapshots
	}
	snapshots, err := listSnapshots(settingsInstance.GDrive.FolderID)
	if err != nil {
		recordSyncResult(err)
		writeSettingsToDisk(false)
		return err
	}
	settingsInstance.GDrive.Snapshots = make([]DriveSnapshotMeta, 0, len(snapshots))
	for _, snapshot := range snapshots {
		settingsInstance.GDrive.Snapshots = append(settingsInstance.GDrive.Snapshots, DriveSnapshotMeta{
			ID:        snapshot.ID,
			Name:      snapshot.Name,
			CreatedAt: snapshot.CreatedAt,
		})
	}
	settingsInstance.GDrive.LastDriveRefreshAt = time.Now().Format(time.RFC3339)
	settingsInstance.GDrive.LastSyncStatus = "ok"
	settingsInstance.GDrive.LastSyncMessage = "Drive snapshot list refreshed"
	writeSettingsToDisk(false)
	return nil
}

func RestoreDriveSnapshot(snapshotID string) error {
	if settingsInstance == nil || settingsInstance.GDrive == nil || !settingsInstance.GDrive.Ready() {
		return errors.New("Google Drive is not ready")
	}
	if strings.TrimSpace(snapshotID) == "" {
		return errors.New("no Drive snapshot selected")
	}
	restoreSnapshot := driveRestoreSnapshotFunc
	if restoreSnapshot == nil {
		restoreSnapshot = gdrive.RestoreAppSnapshot
	}
	data, err := restoreSnapshot(snapshotID)
	if err != nil {
		recordSyncResult(err)
		writeSettingsToDisk(false)
		return err
	}
	var remote UserSettings
	if err := json.Unmarshal(data, &remote); err != nil {
		return fmt.Errorf("restore snapshot settings decode: %w", err)
	}
	normalizeSettings(&remote)
	if remote.GDrive == nil {
		remote.GDrive = defaultGDriveSettings()
	}
	remote.GDrive.Enabled = settingsInstance.GDrive.Enabled
	remote.GDrive.FolderID = settingsInstance.GDrive.FolderID
	remote.GDrive.FolderName = settingsInstance.GDrive.FolderName
	remote.GDrive.LastDriveSaveAt = settingsInstance.GDrive.LastDriveSaveAt
	remote.GDrive.LastDriveRefreshAt = settingsInstance.GDrive.LastDriveRefreshAt
	remote.GDrive.LastLocalSaveAt = time.Now().Format(time.RFC3339)
	remote.GDrive.LastSyncStatus = "ok"
	remote.GDrive.LastSyncMessage = "Drive snapshot restored"
	remote.GDrive.PendingSync = false
	remote.GDrive.SelectedSnapshotID = snapshotID
	remote.GDrive.Snapshots = settingsInstance.GDrive.Snapshots
	settingsInstance = &remote
	writeSettingsToDisk(true)
	return nil
}

func recordSyncResult(err error) {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return
	}

	settingsInstance.GDrive.LastSyncAt = time.Now().Format(time.RFC3339)
	if err != nil {
		settingsInstance.GDrive.LastSyncStatus = "error"
		settingsInstance.GDrive.LastSyncMessage = err.Error()
		return
	}

	settingsInstance.GDrive.LastSyncStatus = "ok"
	settingsInstance.GDrive.LastSyncMessage = "Drive operation completed successfully"
}

func lastDriveSyncError() error {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return nil
	}
	switch settingsInstance.GDrive.LastSyncStatus {
	case "error", "conflict":
		if settingsInstance.GDrive.LastSyncMessage == "" {
			return errors.New("Drive sync failed")
		}
		return errors.New(settingsInstance.GDrive.LastSyncMessage)
	default:
		return nil
	}
}

func MarkDriveDirty() {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return
	}
	settingsInstance.GDrive.PendingSync = true
}

func backupLocalStateSnapshot() (string, error) {
	stamp := time.Now().Format("2006-01-02_15-04-05")
	root := getFileName(filepath.Join("conflicts", stamp))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create conflict backup directory: %w", err)
	}

	if err := copyFileIfExists(fileName(), filepath.Join(root, "settings.json")); err != nil {
		return "", err
	}
	notesRoot := filepath.Join(filepath.Dir(fileName()), "notes")
	if err := copyDirIfExists(notesRoot, filepath.Join(root, "notes")); err != nil {
		return "", err
	}
	return root, nil
}

func copyFileIfExists(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read backup source file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create backup file directory: %w", err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("write backup file: %w", err)
	}
	return nil
}

func copyDirIfExists(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat backup source directory: %w", err)
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}
		return copyFileIfExists(path, destPath)
	})
}
