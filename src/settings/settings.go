package settings

import (
	"encoding/json"
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
	TabSpaces       int    `json:"tab_spaces"`
	EditorWidth     int    `json:"editor_width,omitempty"`
	EditorFontSize  int    `json:"editor_font_size,omitempty"`
	SidebarVisible  bool   `json:"sidebar_visible"`
	BodyFont        string `json:"body_font"`
	MonospaceFont   string `json:"monospace_font"`
	EditorMonospace bool   `json:"editor_monospace"`
	PreviewTheme    string `json:"preview_theme"`
	VimMode         bool   `json:"vim_mode"`
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
	ShowPages    bool `json:"show_pages"`
	ShowPassword bool `json:"show_password"`
	ShowNotes    bool `json:"show_notes"`
}

type GDriveSettings struct {
	Enabled         bool   `json:"enabled"`
	FolderID        string `json:"folder_id"`
	FolderName      string `json:"folder_name"`
	LastSyncAt      string `json:"last_sync_at"`
	LastSyncStatus  string `json:"last_sync_status"`
	LastSyncMessage string `json:"last_sync_message"`
}

var settingsInstance *UserSettings
var saveHooks []func(*UserSettings)
var driveSyncInFlight atomic.Bool
var driveSyncFunc func() error
var statusUpdater = func(text string) {
	glib.IdleAdd(func() {
		helpers.StatusBarInst().UpdateStatusBar(text)
	})
}

func init() {
	driveSyncFunc = SyncDriveData
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
			PreviewTheme:   "ide-dark",
		},
		UI:     defaultUISettings(),
		GDrive: defaultGDriveSettings(),
	}
}

func defaultGDriveSettings() *GDriveSettings {
	return &GDriveSettings{}
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
	if s.UI == nil {
		s.UI = defaultUISettings()
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
	if s.NotesApp.EditorFontSize < 0 {
		s.NotesApp.EditorFontSize = 0
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
	saveSettings(true, true)
}

func SaveNotesEditorWidth(width int) {
	if settingsInstance == nil || width <= 0 {
		return
	}
	if settingsInstance.NotesApp.EditorWidth == width {
		return
	}
	settingsInstance.NotesApp.EditorWidth = width
	saveSettings(false, false)
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
	saveSettings(false, false)
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

func saveSettings(sync bool, notifyHooks bool) {
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

	if sync && settingsInstance.GDrive.Ready() {
		startDriveSyncAsync()
		return
	}

	statusUpdater("Settings saved!")
}

func startDriveSyncAsync() {
	if !driveSyncInFlight.CompareAndSwap(false, true) {
		statusUpdater("Settings saved locally. Drive sync is already in progress.")
		return
	}

	statusUpdater("Settings saved locally. Syncing with Google Drive...")
	go func() {
		defer driveSyncInFlight.Store(false)

		if err := driveSyncFunc(); err != nil {
			log.Println("gdrive settings sync error:", err)
			statusUpdater("Settings saved locally, Drive sync failed")
			return
		}

		statusUpdater("Settings saved and synced to Drive!")
	}()
}

func SyncDriveData() error {
	if settingsInstance == nil || !settingsInstance.GDrive.Ready() {
		return nil
	}

	err := gdrive.SyncAppData(settingsInstance.GDrive.FolderID)
	recordSyncResult(err)
	saveSettings(false, false)

	if err == nil {
		if syncErr := gdrive.SyncFile(settingsInstance.GDrive.FolderID, fileName()); syncErr != nil {
			recordSyncResult(syncErr)
			saveSettings(false, false)
			return syncErr
		}
		recordSyncResult(nil)
		saveSettings(false, false)
	}

	return err
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
	settingsInstance.GDrive.LastSyncMessage = "Drive sync completed successfully"
}
