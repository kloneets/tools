package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
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
	TabSpaces       int     `json:"tab_spaces"`
	EditorWidth     int     `json:"editor_width,omitempty"`
	EditorFontSize  int     `json:"editor_font_size,omitempty"`
	LineSpacing     float64 `json:"line_spacing,omitempty"`
	SidebarVisible  bool    `json:"sidebar_visible"`
	BodyFont        string  `json:"body_font"`
	MonospaceFont   string  `json:"monospace_font"`
	EditorMonospace bool    `json:"editor_monospace"`
	PreviewTheme    string  `json:"preview_theme"`
	VimMode         bool    `json:"vim_mode"`
}

func (n *NotesAppSettings) UnmarshalJSON(data []byte) error {
	type notesAppAlias NotesAppSettings
	aux := struct {
		SidebarVisible *bool    `json:"sidebar_visible"`
		LineSpacing    *float64 `json:"line_spacing"`
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
	if aux.LineSpacing == nil {
		n.LineSpacing = 1
	}
	return nil
}

type UISettings struct {
	ShowPages    bool `json:"show_pages"`
	ShowPassword bool `json:"show_password"`
	ShowNotes    bool `json:"show_notes"`
}

type GDriveSettings struct {
	Enabled             bool   `json:"enabled"`
	SyncIntervalSec     int    `json:"sync_interval_sec"`
	FolderID            string `json:"folder_id"`
	FolderName          string `json:"folder_name"`
	PendingSync         bool   `json:"pending_sync,omitempty"`
	LastRemoteState     string `json:"last_remote_state,omitempty"`
	ConflictRemoteState string `json:"conflict_remote_state,omitempty"`
	LastSyncAt          string `json:"last_sync_at"`
	LastSyncStatus      string `json:"last_sync_status"`
	LastSyncMessage     string `json:"last_sync_message"`
}

var settingsInstance *UserSettings
var saveHooks []func(*UserSettings)
var driveSyncInFlight atomic.Bool
var driveRefreshInFlight atomic.Bool
var driveSyncFunc func() error
var driveSettingsLoader func(string) ([]byte, bool, error)
var driveNotesRestorer func(string) error
var driveStateSignature func(string) (string, error)
var statusUpdater = func(text string) {
	glib.IdleAdd(func() {
		helpers.StatusBarInst().UpdateStatusBar(text)
	})
}

func init() {
	driveSyncFunc = syncDriveDataCore
	driveSettingsLoader = func(folderID string) ([]byte, bool, error) {
		return gdrive.DownloadFile(folderID, "settings.json")
	}
	driveNotesRestorer = gdrive.RestoreNotesTree
	driveStateSignature = gdrive.RemoteStateSignature
}

func Inst() *UserSettings {
	if settingsInstance == nil {
		log.Fatal("Settings is not initialized")
	}

	return settingsInstance
}

func Init() *[]string {
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
			SidebarVisible: true,
			BodyFont:       "Cantarell 11",
			MonospaceFont:  "Noto Sans Mono 11",
			LineSpacing:    1,
			PreviewTheme:   "ide-dark",
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
		ShowPages:    true,
		ShowPassword: true,
		ShowNotes:    true,
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
	if s.AppWindow.Width <= 0 {
		s.AppWindow.Width = 600
	}
	if s.AppWindow.Height <= 0 {
		s.AppWindow.Height = 300
	}
	if s.NotesApp.LineSpacing == 0 && s.NotesApp.TabSpaces == 0 && s.NotesApp.BodyFont == "" && s.NotesApp.MonospaceFont == "" && s.NotesApp.PreviewTheme == "" && !s.NotesApp.VimMode && !s.NotesApp.EditorMonospace && s.NotesApp.EditorWidth == 0 && !s.NotesApp.SidebarVisible {
		s.NotesApp.LineSpacing = 1
	}
	if s.NotesApp.TabSpaces <= 0 {
		s.NotesApp.TabSpaces = 4
	}
	if s.NotesApp.EditorFontSize < 0 {
		s.NotesApp.EditorFontSize = 0
	}
	if s.NotesApp.LineSpacing < 0 {
		s.NotesApp.LineSpacing = 1
	}
	if s.NotesApp.BodyFont == "" && s.NotesApp.MonospaceFont == "" && s.NotesApp.PreviewTheme == "" && !s.NotesApp.VimMode && !s.NotesApp.EditorMonospace && s.NotesApp.EditorWidth == 0 && !s.NotesApp.SidebarVisible {
		s.NotesApp.SidebarVisible = true
	}
	if s.NotesApp.BodyFont == "" {
		s.NotesApp.BodyFont = "Cantarell 11"
	}
	if s.NotesApp.MonospaceFont == "" {
		s.NotesApp.MonospaceFont = "Noto Sans Mono 11"
	}
	switch s.NotesApp.PreviewTheme {
	case "ide-dark", "neon-burst", "paper-light", "mono":
	default:
		s.NotesApp.PreviewTheme = "ide-dark"
	}
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
	MarkDriveDirty()
	writeSettingsToDisk(true)
	startDriveSyncAsync()
}

func StartDriveSync() {
	if settingsInstance == nil || settingsInstance.GDrive == nil || !settingsInstance.GDrive.Ready() || !settingsInstance.GDrive.PendingSync {
		return
	}
	startDriveSyncAsync()
}

func NeedsDriveSyncOnClose() bool {
	return settingsInstance != nil && settingsInstance.GDrive != nil && settingsInstance.GDrive.Ready() && settingsInstance.GDrive.PendingSync
}

func SyncDriveDataOnShutdown(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !NeedsDriveSyncOnClose() {
			return lastDriveSyncError()
		}
		if driveSyncInFlight.CompareAndSwap(false, true) {
			err := syncDriveDataOnce()
			driveSyncInFlight.Store(false)
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func StartDriveRefresh() {
	if settingsInstance == nil || settingsInstance.GDrive == nil || !settingsInstance.GDrive.Ready() || settingsInstance.GDrive.PendingSync {
		return
	}
	if !driveRefreshInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer driveRefreshInFlight.Store(false)
		messages := restoreDriveSourceOfTruth()
		if len(messages) == 0 {
			return
		}
		writeSettingsToDisk(true)
		statusUpdater(messages[len(messages)-1])
	}()
}

func ResolveDriveConflictUseLocal() error {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return errors.New("settings are not initialized")
	}
	if settingsInstance.GDrive.ConflictRemoteState == "" {
		return errors.New("no Drive conflict to resolve")
	}

	settingsInstance.GDrive.LastRemoteState = settingsInstance.GDrive.ConflictRemoteState
	settingsInstance.GDrive.ConflictRemoteState = ""
	settingsInstance.GDrive.PendingSync = true
	settingsInstance.GDrive.LastSyncStatus = "pending"
	settingsInstance.GDrive.LastSyncMessage = "Conflict resolved in favor of local data. Drive sync queued."
	writeSettingsToDisk(true)
	startDriveSyncAsync()
	return nil
}

func ResolveDriveConflictUseRemote() (string, error) {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return "", errors.New("settings are not initialized")
	}
	if settingsInstance.GDrive.ConflictRemoteState == "" {
		return "", errors.New("no Drive conflict to resolve")
	}

	backupDir, err := backupLocalStateSnapshot()
	if err != nil {
		return "", err
	}
	settingsInstance.GDrive.PendingSync = false
	settingsInstance.GDrive.ConflictRemoteState = ""
	messages := restoreDriveSourceOfTruthForced()
	if len(messages) == 0 {
		settingsInstance.GDrive.LastSyncStatus = "ok"
		settingsInstance.GDrive.LastSyncMessage = "Drive data restored successfully"
	} else {
		settingsInstance.GDrive.LastSyncMessage = messages[len(messages)-1]
	}
	writeSettingsToDisk(true)
	return backupDir, nil
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
	MarkDriveDirty()
	writeSettingsToDisk(false)
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
	MarkDriveDirty()
	writeSettingsToDisk(false)
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

func saveSettings(sync bool, notifyHooks bool) {
	if sync {
		MarkDriveDirty()
	}
	writeSettingsToDisk(notifyHooks)
	if sync {
		startDriveSyncAsync()
	}
}

func startDriveSyncAsync() {
	if settingsInstance == nil || settingsInstance.GDrive == nil || !settingsInstance.GDrive.Ready() || !settingsInstance.GDrive.PendingSync {
		return
	}
	if !driveSyncInFlight.CompareAndSwap(false, true) {
		statusUpdater("Settings saved locally. Drive sync is already in progress.")
		return
	}

	statusUpdater("Settings saved locally. Syncing with Google Drive...")
	go func() {
		defer driveSyncInFlight.Store(false)

		if err := syncDriveDataOnce(); err != nil {
			log.Println("gdrive settings sync error:", err)
			statusUpdater("Settings saved locally, Drive sync failed")
			return
		}

		statusUpdater("Settings saved and synced to Drive!")
	}()
}

func SyncDriveData() error {
	if !driveSyncInFlight.CompareAndSwap(false, true) {
		return nil
	}
	defer driveSyncInFlight.Store(false)
	return syncDriveDataOnce()
}

func syncDriveDataOnce() error {
	if driveSyncFunc != nil {
		return driveSyncFunc()
	}
	return syncDriveDataCore()
}

func syncDriveDataCore() error {
	if settingsInstance == nil || !settingsInstance.GDrive.Ready() || !settingsInstance.GDrive.PendingSync {
		return nil
	}
	if err := ensureDriveSyncSafeToPush(); err != nil {
		recordSyncResult(err)
		writeSettingsToDisk(false)
		return err
	}

	settingsData, dataErr := driveSyncSettingsJSON()
	if dataErr != nil {
		recordSyncResult(dataErr)
		writeSettingsToDisk(false)
		return dataErr
	}

	err := gdrive.SyncAppData(settingsInstance.GDrive.FolderID, settingsData)
	recordSyncResult(err)
	if err == nil {
		settingsInstance.GDrive.PendingSync = false
		updateLastRemoteState()
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

	clone.NotesApp.BodyFont = ""
	clone.NotesApp.MonospaceFont = ""
	clone.NotesApp.EditorFontSize = 0
	clone.NotesApp.EditorMonospace = false
	if clone.GDrive != nil {
		clone.GDrive.PendingSync = false
	}

	return json.Marshal(&clone)
}

type localFontSettings struct {
	BodyFont        string
	MonospaceFont   string
	EditorFontSize  int
	EditorMonospace bool
}

func restoreDriveSourceOfTruth() []string {
	if settingsInstance != nil && settingsInstance.GDrive != nil && settingsInstance.GDrive.PendingSync {
		return nil
	}
	return restoreDriveSourceOfTruthForced()
}

func restoreDriveSourceOfTruthForced() []string {
	if settingsInstance == nil || !settingsInstance.GDrive.Ready() {
		return nil
	}

	data, found, err := driveSettingsLoader(settingsInstance.GDrive.FolderID)
	if err != nil {
		msg := "Drive settings fetch failed, using local settings: " + err.Error()
		log.Println(msg)
		return []string{msg}
	}
	if !found || len(data) == 0 {
		return nil
	}

	localFonts := localFontSettings{
		BodyFont:        settingsInstance.NotesApp.BodyFont,
		MonospaceFont:   settingsInstance.NotesApp.MonospaceFont,
		EditorFontSize:  settingsInstance.NotesApp.EditorFontSize,
		EditorMonospace: settingsInstance.NotesApp.EditorMonospace,
	}

	var remote UserSettings
	if err := json.Unmarshal(data, &remote); err != nil {
		msg := "Drive settings are invalid, using local settings: " + err.Error()
		log.Println(msg)
		return []string{msg}
	}

	normalizeSettings(&remote)
	remote.NotesApp.BodyFont = localFonts.BodyFont
	remote.NotesApp.MonospaceFont = localFonts.MonospaceFont
	remote.NotesApp.EditorFontSize = localFonts.EditorFontSize
	remote.NotesApp.EditorMonospace = localFonts.EditorMonospace
	remote.GDrive.PendingSync = false
	remote.GDrive.ConflictRemoteState = ""
	settingsInstance = &remote

	if err := driveNotesRestorer(settingsInstance.GDrive.FolderID); err != nil {
		msg := "Drive notes restore failed, using local notes: " + err.Error()
		log.Println(msg)
		return []string{msg}
	}
	updateLastRemoteState()

	return []string{"Drive data restored"}
}

func recordSyncResult(err error) {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return
	}

	settingsInstance.GDrive.LastSyncAt = time.Now().Format(time.RFC3339)
	if err != nil {
		if errors.Is(err, errDriveConflict) {
			settingsInstance.GDrive.LastSyncStatus = "conflict"
		} else {
			settingsInstance.GDrive.LastSyncStatus = "error"
		}
		settingsInstance.GDrive.LastSyncMessage = err.Error()
		return
	}

	settingsInstance.GDrive.LastSyncStatus = "ok"
	settingsInstance.GDrive.LastSyncMessage = "Drive sync completed successfully"
	settingsInstance.GDrive.ConflictRemoteState = ""
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

func ensureDriveSyncSafeToPush() error {
	if settingsInstance == nil || settingsInstance.GDrive == nil || settingsInstance.GDrive.FolderID == "" {
		return nil
	}
	if settingsInstance.GDrive.LastRemoteState == "" {
		return nil
	}
	if driveStateSignature == nil {
		return nil
	}
	currentState, err := driveStateSignature(settingsInstance.GDrive.FolderID)
	if err != nil {
		return err
	}
	if currentState == settingsInstance.GDrive.LastRemoteState {
		return nil
	}
	settingsInstance.GDrive.ConflictRemoteState = currentState
	return fmt.Errorf("%w: Drive data changed remotely since the last sync; local changes were kept and not uploaded", errDriveConflict)
}

func updateLastRemoteState() {
	if settingsInstance == nil || settingsInstance.GDrive == nil || settingsInstance.GDrive.FolderID == "" || driveStateSignature == nil {
		return
	}
	state, err := driveStateSignature(settingsInstance.GDrive.FolderID)
	if err != nil {
		log.Println("drive state signature error:", err)
		return
	}
	settingsInstance.GDrive.LastRemoteState = state
}

var errDriveConflict = errors.New("drive conflict")

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
