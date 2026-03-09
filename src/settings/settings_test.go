package settings

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
)

func TestDefaultSettings(t *testing.T) {
	got := defaultSettings()

	if !got.PasswordApp.Letters || !got.PasswordApp.Numbers || !got.PasswordApp.SpecialSymbols {
		t.Fatal("expected password defaults to enable all character groups")
	}
	if got.PasswordApp.SymbolCount != 16 {
		t.Fatalf("SymbolCount = %d, want 16", got.PasswordApp.SymbolCount)
	}
	if got.NotesApp.TabSpaces != 4 {
		t.Fatalf("NotesApp.TabSpaces = %d, want 4", got.NotesApp.TabSpaces)
	}
	if got.NotesApp.EditorWidth != 0 {
		t.Fatalf("NotesApp.EditorWidth = %d, want 0", got.NotesApp.EditorWidth)
	}
	if got.NotesApp.EditorFontSize != 0 {
		t.Fatalf("NotesApp.EditorFontSize = %d, want 0", got.NotesApp.EditorFontSize)
	}
	if got.AppWindow.Width != 600 || got.AppWindow.Height != 300 {
		t.Fatalf("AppWindow = %#v, want 600x300", got.AppWindow)
	}
	if got.AppWindow.Maximized {
		t.Fatal("AppWindow.Maximized = true, want false by default")
	}
	if got.NotesApp.BodyFont != "Cantarell 11" {
		t.Fatalf("NotesApp.BodyFont = %q, want Cantarell 11", got.NotesApp.BodyFont)
	}
	if got.NotesApp.MonospaceFont != "Noto Sans Mono 11" {
		t.Fatalf("NotesApp.MonospaceFont = %q, want Noto Sans Mono 11", got.NotesApp.MonospaceFont)
	}
	if got.NotesApp.EditorMonospace {
		t.Fatal("NotesApp.EditorMonospace = true, want false by default")
	}
	if got.NotesApp.PreviewTheme != "ide-dark" {
		t.Fatalf("NotesApp.PreviewTheme = %q, want ide-dark", got.NotesApp.PreviewTheme)
	}
	if got.NotesApp.VimMode {
		t.Fatal("NotesApp.VimMode = true, want false by default")
	}
	if !got.NotesApp.SidebarVisible {
		t.Fatal("NotesApp.SidebarVisible = false, want true by default")
	}
	if got.GDrive == nil {
		t.Fatal("expected default GDrive settings to be initialized")
	}
	if got.UI == nil {
		t.Fatal("expected default UI settings to be initialized")
	}
	if !got.UI.ShowPages || !got.UI.ShowPassword || !got.UI.ShowNotes {
		t.Fatalf("unexpected widget visibility defaults: %#v", got.UI)
	}
}

func TestInitMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settingsInstance = nil

	messages := Init()

	if settingsInstance == nil {
		t.Fatal("expected settingsInstance to be initialized")
	}
	if len(*messages) != 1 || !strings.Contains((*messages)[0], "Settings read error") {
		t.Fatalf("Init() messages = %#v, want read error", *messages)
	}
	if !settingsInstance.PasswordApp.Letters || settingsInstance.PasswordApp.SymbolCount != 16 {
		t.Fatalf("unexpected default settings: %#v", settingsInstance)
	}
}

func TestInitInvalidJSONBacksUpFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = nil

	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	messages := Init()

	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("expected invalid settings file to be moved, stat err = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*settings.json"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %d (%v)", len(matches), matches)
	}
	if len(*messages) != 1 || !strings.Contains((*messages)[0], "Old settings backed up to:") {
		t.Fatalf("Init() messages = %#v, want backup message", *messages)
	}
	if settingsInstance == nil {
		t.Fatal("expected defaults after invalid JSON")
	}
}

func TestGetFileNameUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := getFileName("settings.json")
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json")
	if got != want {
		t.Fatalf("getFileName() = %q, want %q", got, want)
	}
}

func TestNormalizeSettingsInitializesGDrive(t *testing.T) {
	config := &UserSettings{}

	normalizeSettings(config)

	if config.GDrive == nil {
		t.Fatal("normalizeSettings() should initialize GDrive")
	}
	if config.UI == nil {
		t.Fatal("normalizeSettings() should initialize UI settings")
	}
	if config.AppWindow.Width != 600 || config.AppWindow.Height != 300 {
		t.Fatalf("normalizeSettings() AppWindow = %#v, want 600x300", config.AppWindow)
	}
	if !config.UI.ShowPages || !config.UI.ShowPassword || !config.UI.ShowNotes {
		t.Fatalf("normalizeSettings() should default all widgets visible, got %#v", config.UI)
	}
	if config.NotesApp.TabSpaces != 4 {
		t.Fatalf("normalizeSettings() NotesApp.TabSpaces = %d, want 4", config.NotesApp.TabSpaces)
	}
	if config.NotesApp.EditorWidth != 0 {
		t.Fatalf("normalizeSettings() NotesApp.EditorWidth = %d, want 0", config.NotesApp.EditorWidth)
	}
	if config.NotesApp.EditorFontSize != 0 {
		t.Fatalf("normalizeSettings() NotesApp.EditorFontSize = %d, want 0", config.NotesApp.EditorFontSize)
	}
	if config.NotesApp.BodyFont != "Cantarell 11" {
		t.Fatalf("normalizeSettings() NotesApp.BodyFont = %q", config.NotesApp.BodyFont)
	}
	if config.NotesApp.MonospaceFont != "Noto Sans Mono 11" {
		t.Fatalf("normalizeSettings() NotesApp.MonospaceFont = %q", config.NotesApp.MonospaceFont)
	}
	if config.NotesApp.EditorMonospace {
		t.Fatal("normalizeSettings() should leave EditorMonospace disabled by default")
	}
	if config.NotesApp.PreviewTheme != "ide-dark" {
		t.Fatalf("normalizeSettings() NotesApp.PreviewTheme = %q", config.NotesApp.PreviewTheme)
	}
	if config.NotesApp.VimMode {
		t.Fatal("normalizeSettings() should leave VimMode disabled by default")
	}
	if !config.NotesApp.SidebarVisible {
		t.Fatal("normalizeSettings() should default sidebar visible")
	}

	config.NotesApp.PreviewTheme = "neon-burst"
	normalizeSettings(config)
	if config.NotesApp.PreviewTheme != "neon-burst" {
		t.Fatalf("normalizeSettings() should keep neon-burst, got %q", config.NotesApp.PreviewTheme)
	}
}

func TestGDriveReady(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}

	config := &GDriveSettings{
		Enabled:  true,
		FolderID: "folder-1",
	}
	if !config.Ready() {
		t.Fatal("Ready() should return true for enabled configured sync")
	}
	config.Enabled = false
	if config.Ready() {
		t.Fatal("Ready() should return false when sync is disabled")
	}
}

func TestRecordSyncResult(t *testing.T) {
	settingsInstance = defaultSettings()

	recordSyncResult(nil)
	if settingsInstance.GDrive.LastSyncStatus != "ok" {
		t.Fatalf("LastSyncStatus = %q, want ok", settingsInstance.GDrive.LastSyncStatus)
	}
	if settingsInstance.GDrive.LastSyncAt == "" {
		t.Fatal("expected LastSyncAt to be set")
	}

	recordSyncResult(os.ErrPermission)
	if settingsInstance.GDrive.LastSyncStatus != "error" {
		t.Fatalf("LastSyncStatus = %q, want error", settingsInstance.GDrive.LastSyncStatus)
	}
	if !strings.Contains(settingsInstance.GDrive.LastSyncMessage, "permission") {
		t.Fatalf("LastSyncMessage = %q, want permission text", settingsInstance.GDrive.LastSyncMessage)
	}
	if _, err := time.Parse(time.RFC3339, settingsInstance.GDrive.LastSyncAt); err != nil {
		t.Fatalf("LastSyncAt parse error = %v", err)
	}
}

func TestRunSaveHooksCanSkipHooks(t *testing.T) {
	settingsInstance = defaultSettings()

	originalHooks := saveHooks
	defer func() { saveHooks = originalHooks }()
	saveHooks = nil

	var calls atomic.Int32
	RegisterSaveHook(func(*UserSettings) {
		calls.Add(1)
	})

	runSaveHooks(false)
	if got := calls.Load(); got != 0 {
		t.Fatalf("runSaveHooks(false) hook calls = %d, want 0", got)
	}

	runSaveHooks(true)
	if got := calls.Load(); got != 1 {
		t.Fatalf("runSaveHooks(true) hook calls = %d, want 1", got)
	}
}

func TestSaveNotesEditorWidthPersistsValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	settingsInstance = defaultSettings()

	SaveNotesEditorWidth(512)

	if settingsInstance.NotesApp.EditorWidth != 512 {
		t.Fatalf("NotesApp.EditorWidth = %d, want 512", settingsInstance.NotesApp.EditorWidth)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"editor_width":512`) {
		t.Fatalf("saved settings missing editor_width: %s", string(data))
	}
}

func TestPersistedNotesEditorWidthReadsFromDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	settingsInstance = nil
	data := `{"notes_app":{"editor_width":377}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if got := PersistedNotesEditorWidth(); got != 377 {
		t.Fatalf("PersistedNotesEditorWidth() = %d, want 377", got)
	}
}

func TestSaveAppWindowStatePersistsValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	settingsInstance = defaultSettings()

	SaveAppWindowState(1234, 777, true)

	if settingsInstance.AppWindow.Width != 1234 || settingsInstance.AppWindow.Height != 777 || !settingsInstance.AppWindow.Maximized {
		t.Fatalf("AppWindow = %#v, want width=1234 height=777 maximized=true", settingsInstance.AppWindow)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"app_window":{"width":1234,"height":777,"maximized":true}`) {
		t.Fatalf("saved settings missing app_window: %s", string(data))
	}
}

func TestInitOldSettingsDefaultsSidebarVisible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = nil

	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data := `{"notes_app":{"tab_spaces":2}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	Init()

	if !settingsInstance.NotesApp.SidebarVisible {
		t.Fatal("old settings should default notes sidebar to visible")
	}
}

func TestNormalizeSettingsClearsNegativeEditorFontSize(t *testing.T) {
	config := &UserSettings{
		NotesApp: NotesAppSettings{
			EditorFontSize: -4,
		},
	}

	normalizeSettings(config)

	if config.NotesApp.EditorFontSize != 0 {
		t.Fatalf("normalizeSettings() EditorFontSize = %d, want 0", config.NotesApp.EditorFontSize)
	}
}

func TestSaveSettingsStartsDriveSyncAsync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}

	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.FolderName = "Drive / Tests"

	originalStatusUpdater := statusUpdater
	originalDriveSyncFunc := driveSyncFunc
	driveSyncInFlight.Store(false)
	defer func() {
		statusUpdater = originalStatusUpdater
		driveSyncFunc = originalDriveSyncFunc
		driveSyncInFlight.Store(false)
	}()

	statusUpdater = func(string) {}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	driveSyncFunc = func() error {
		once.Do(func() { started <- struct{}{} })
		<-release
		return nil
	}

	done := make(chan struct{})
	go func() {
		saveSettings(true, false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("saveSettings() should return before Drive sync completes")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected async Drive sync to start")
	}

	close(release)
}
