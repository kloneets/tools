package settings

import (
	"context"
	"encoding/json"
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
	if got.NotesApp.LineSpacing != 1 {
		t.Fatalf("NotesApp.LineSpacing = %v, want 1", got.NotesApp.LineSpacing)
	}
	if got.NotesApp.PDFBodyFontSize != DefaultNotesPDFBodyFontSize {
		t.Fatalf("NotesApp.PDFBodyFontSize = %v, want %v", got.NotesApp.PDFBodyFontSize, DefaultNotesPDFBodyFontSize)
	}
	if got.NotesApp.PDFCodeFontSize != DefaultNotesPDFCodeFontSize {
		t.Fatalf("NotesApp.PDFCodeFontSize = %v, want %v", got.NotesApp.PDFCodeFontSize, DefaultNotesPDFCodeFontSize)
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
	if got.GDrive.SyncIntervalSec != 10 {
		t.Fatalf("GDrive.SyncIntervalSec = %d, want 10", got.GDrive.SyncIntervalSec)
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
	if config.GDrive.SyncIntervalSec != 10 {
		t.Fatalf("normalizeSettings() GDrive.SyncIntervalSec = %d, want 10", config.GDrive.SyncIntervalSec)
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
	if config.NotesApp.LineSpacing != 1 {
		t.Fatalf("normalizeSettings() NotesApp.LineSpacing = %v, want 1", config.NotesApp.LineSpacing)
	}
	if config.NotesApp.PDFBodyFontSize != DefaultNotesPDFBodyFontSize {
		t.Fatalf("normalizeSettings() NotesApp.PDFBodyFontSize = %v", config.NotesApp.PDFBodyFontSize)
	}
	if config.NotesApp.PDFCodeFontSize != DefaultNotesPDFCodeFontSize {
		t.Fatalf("normalizeSettings() NotesApp.PDFCodeFontSize = %v", config.NotesApp.PDFCodeFontSize)
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

func TestNormalizeSettingsDefaultsInvalidPDFFontSizes(t *testing.T) {
	config := &UserSettings{
		NotesApp: NotesAppSettings{
			PDFBodyFontSize: 0,
			PDFCodeFontSize: -1,
		},
	}

	normalizeSettings(config)

	if config.NotesApp.PDFBodyFontSize != DefaultNotesPDFBodyFontSize {
		t.Fatalf("normalizeSettings() PDFBodyFontSize = %v, want %v", config.NotesApp.PDFBodyFontSize, DefaultNotesPDFBodyFontSize)
	}
	if config.NotesApp.PDFCodeFontSize != DefaultNotesPDFCodeFontSize {
		t.Fatalf("normalizeSettings() PDFCodeFontSize = %v, want %v", config.NotesApp.PDFCodeFontSize, DefaultNotesPDFCodeFontSize)
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

func TestDriveSyncIntervalDefaultsToTenSeconds(t *testing.T) {
	settingsInstance = nil
	if got := DriveSyncInterval(); got != 10*time.Second {
		t.Fatalf("DriveSyncInterval() = %v, want 10s", got)
	}

	settingsInstance = defaultSettings()
	settingsInstance.GDrive.SyncIntervalSec = 45
	if got := DriveSyncInterval(); got != 45*time.Second {
		t.Fatalf("DriveSyncInterval() = %v, want 45s", got)
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

	recordSyncResult(errDriveConflict)
	if settingsInstance.GDrive.LastSyncStatus != "conflict" {
		t.Fatalf("LastSyncStatus = %q, want conflict", settingsInstance.GDrive.LastSyncStatus)
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
			LineSpacing:    -3,
		},
		GDrive: &GDriveSettings{
			SyncIntervalSec: -1,
		},
	}

	normalizeSettings(config)

	if config.NotesApp.EditorFontSize != 0 {
		t.Fatalf("normalizeSettings() EditorFontSize = %d, want 0", config.NotesApp.EditorFontSize)
	}
	if config.NotesApp.LineSpacing != 1 {
		t.Fatalf("normalizeSettings() LineSpacing = %v, want 1", config.NotesApp.LineSpacing)
	}
	if config.GDrive.SyncIntervalSec != 10 {
		t.Fatalf("normalizeSettings() SyncIntervalSec = %d, want 10", config.GDrive.SyncIntervalSec)
	}
}

func TestNotesAppSettingsUnmarshalDefaultsMissingLineSpacingToOne(t *testing.T) {
	var got NotesAppSettings
	if err := json.Unmarshal([]byte(`{"tab_spaces":2}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.LineSpacing != 1 {
		t.Fatalf("LineSpacing = %v, want 1", got.LineSpacing)
	}
}

func TestNotesAppSettingsUnmarshalKeepsExplicitZeroLineSpacing(t *testing.T) {
	var got NotesAppSettings
	if err := json.Unmarshal([]byte(`{"line_spacing":0}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.LineSpacing != 0 {
		t.Fatalf("LineSpacing = %v, want 0", got.LineSpacing)
	}
}

func TestDriveSyncSettingsJSONExcludesFontSettings(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.NotesApp.BodyFont = "IBM Plex Sans 13"
	settingsInstance.NotesApp.MonospaceFont = "JetBrains Mono 15"
	settingsInstance.NotesApp.EditorFontSize = 19
	settingsInstance.NotesApp.EditorMonospace = true
	settingsInstance.NotesApp.PreviewTheme = "paper-light"

	data, err := driveSyncSettingsJSON()
	if err != nil {
		t.Fatalf("driveSyncSettingsJSON() error = %v", err)
	}

	var got UserSettings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.NotesApp.BodyFont != "" {
		t.Fatalf("BodyFont = %q, want empty", got.NotesApp.BodyFont)
	}
	if got.NotesApp.MonospaceFont != "" {
		t.Fatalf("MonospaceFont = %q, want empty", got.NotesApp.MonospaceFont)
	}
	if got.NotesApp.EditorFontSize != 0 {
		t.Fatalf("EditorFontSize = %d, want 0", got.NotesApp.EditorFontSize)
	}
	if got.NotesApp.EditorMonospace {
		t.Fatal("EditorMonospace = true, want false")
	}
	if got.NotesApp.PreviewTheme != "paper-light" {
		t.Fatalf("PreviewTheme = %q, want paper-light", got.NotesApp.PreviewTheme)
	}
}

func TestRestoreDriveSourceOfTruthPrefersRemoteButKeepsLocalFonts(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.NotesApp.BodyFont = "IBM Plex Sans 13"
	settingsInstance.NotesApp.MonospaceFont = "JetBrains Mono 14"
	settingsInstance.NotesApp.EditorFontSize = 18
	settingsInstance.NotesApp.EditorMonospace = true
	settingsInstance.UI.ShowPages = false

	originalLoader := driveSettingsLoader
	originalRestorer := driveNotesRestorer
	defer func() {
		driveSettingsLoader = originalLoader
		driveNotesRestorer = originalRestorer
	}()

	driveSettingsLoader = func(string) ([]byte, bool, error) {
		return []byte(`{"notes_app":{"tab_spaces":2,"preview_theme":"paper-light"},"ui":{"show_pages":true,"show_password":false,"show_notes":true},"gdrive":{"enabled":true,"folder_id":"folder-1"}}`), true, nil
	}
	called := false
	driveNotesRestorer = func(folderID string) error {
		called = folderID == "folder-1"
		return nil
	}

	messages := restoreDriveSourceOfTruth()

	if !called {
		t.Fatal("expected driveNotesRestorer to be called")
	}
	if settingsInstance.NotesApp.TabSpaces != 2 {
		t.Fatalf("TabSpaces = %d, want 2", settingsInstance.NotesApp.TabSpaces)
	}
	if settingsInstance.UI.ShowPages != true || settingsInstance.UI.ShowPassword != false {
		t.Fatalf("UI = %#v, want remote values", settingsInstance.UI)
	}
	if settingsInstance.NotesApp.BodyFont != "IBM Plex Sans 13" {
		t.Fatalf("BodyFont = %q, want preserved local font", settingsInstance.NotesApp.BodyFont)
	}
	if settingsInstance.NotesApp.MonospaceFont != "JetBrains Mono 14" {
		t.Fatalf("MonospaceFont = %q, want preserved local font", settingsInstance.NotesApp.MonospaceFont)
	}
	if settingsInstance.NotesApp.EditorFontSize != 18 || !settingsInstance.NotesApp.EditorMonospace {
		t.Fatalf("editor font settings = %#v, want preserved local values", settingsInstance.NotesApp)
	}
	if len(messages) != 1 || messages[0] != "Drive data restored" {
		t.Fatalf("messages = %#v, want Drive data restored", messages)
	}
}

func TestRestoreDriveSourceOfTruthFallsBackOnInvalidRemoteSettings(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.UI.ShowPages = false

	originalLoader := driveSettingsLoader
	originalRestorer := driveNotesRestorer
	defer func() {
		driveSettingsLoader = originalLoader
		driveNotesRestorer = originalRestorer
	}()

	driveSettingsLoader = func(string) ([]byte, bool, error) {
		return []byte("{broken"), true, nil
	}
	restored := false
	driveNotesRestorer = func(string) error {
		restored = true
		return nil
	}

	messages := restoreDriveSourceOfTruth()

	if restored {
		t.Fatal("driveNotesRestorer should not run when remote settings are invalid")
	}
	if settingsInstance.UI.ShowPages != false {
		t.Fatal("local settings should remain unchanged")
	}
	if len(messages) != 1 || !strings.Contains(messages[0], "Drive settings are invalid") {
		t.Fatalf("messages = %#v, want invalid drive settings message", messages)
	}
}

func TestEnsureDriveSyncSafeToPushAllowsMatchingRemoteState(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true
	settingsInstance.GDrive.LastRemoteState = "state-a"

	originalSignature := driveStateSignature
	defer func() { driveStateSignature = originalSignature }()
	driveStateSignature = func(string) (string, error) {
		return "state-a", nil
	}

	if err := ensureDriveSyncSafeToPush(); err != nil {
		t.Fatalf("ensureDriveSyncSafeToPush() error = %v, want nil", err)
	}
}

func TestEnsureDriveSyncSafeToPushRejectsRemoteConflict(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true
	settingsInstance.GDrive.LastRemoteState = "state-a"

	originalSignature := driveStateSignature
	defer func() { driveStateSignature = originalSignature }()
	driveStateSignature = func(string) (string, error) {
		return "state-b", nil
	}

	err := ensureDriveSyncSafeToPush()
	if err == nil {
		t.Fatal("ensureDriveSyncSafeToPush() should reject conflicting remote changes")
	}
	if !strings.Contains(err.Error(), "changed remotely") {
		t.Fatalf("ensureDriveSyncSafeToPush() error = %q", err.Error())
	}
}

func TestStartDriveRefreshSkipsWhenLocalChangesArePending(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true

	originalLoader := driveSettingsLoader
	defer func() { driveSettingsLoader = originalLoader }()

	called := make(chan struct{}, 1)
	driveSettingsLoader = func(string) ([]byte, bool, error) {
		called <- struct{}{}
		return nil, false, nil
	}

	StartDriveRefresh()

	select {
	case <-called:
		t.Fatal("StartDriveRefresh() should skip remote refresh while local changes are pending")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestResolveDriveConflictUseLocalQueuesSync(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true
	settingsInstance.GDrive.LastSyncStatus = "conflict"
	settingsInstance.GDrive.ConflictRemoteState = "remote-b"

	originalDriveSyncFunc := driveSyncFunc
	originalStatusUpdater := statusUpdater
	defer func() {
		driveSyncFunc = originalDriveSyncFunc
		statusUpdater = originalStatusUpdater
		driveSyncInFlight.Store(false)
	}()
	statusUpdater = func(string) {}
	driveSyncInFlight.Store(false)

	started := make(chan struct{}, 1)
	driveSyncFunc = func() error {
		started <- struct{}{}
		return nil
	}

	if err := ResolveDriveConflictUseLocal(); err != nil {
		t.Fatalf("ResolveDriveConflictUseLocal() error = %v", err)
	}
	if settingsInstance.GDrive.ConflictRemoteState != "" {
		t.Fatal("expected conflict state to be cleared")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected sync to start after resolving in favor of local")
	}
}

func TestResolveDriveConflictUseRemoteBacksUpLocalState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true
	settingsInstance.GDrive.LastSyncStatus = "conflict"
	settingsInstance.GDrive.ConflictRemoteState = "remote-b"

	if err := os.MkdirAll(filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fileName(), []byte(`{"gdrive":{"enabled":true}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes", "Plan.md"), []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile(note) error = %v", err)
	}

	originalLoader := driveSettingsLoader
	originalRestorer := driveNotesRestorer
	originalSignature := driveStateSignature
	defer func() {
		driveSettingsLoader = originalLoader
		driveNotesRestorer = originalRestorer
		driveStateSignature = originalSignature
	}()
	driveSettingsLoader = func(string) ([]byte, bool, error) {
		return []byte(`{"notes_app":{"tab_spaces":2},"gdrive":{"enabled":true,"folder_id":"folder-1"}}`), true, nil
	}
	driveNotesRestorer = func(string) error { return nil }
	driveStateSignature = func(string) (string, error) { return "remote-b", nil }

	backupDir, err := ResolveDriveConflictUseRemote()
	if err != nil {
		t.Fatalf("ResolveDriveConflictUseRemote() error = %v", err)
	}
	if backupDir == "" {
		t.Fatal("expected backup directory")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "settings.json")); err != nil {
		t.Fatalf("expected backed up settings file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "notes", "Plan.md")); err != nil {
		t.Fatalf("expected backed up notes file: %v", err)
	}
	if settingsInstance.GDrive.PendingSync {
		t.Fatal("expected pending sync to be cleared after using Drive version")
	}
}

func TestNeedsDriveSyncOnCloseRequiresPendingReadyDriveSync(t *testing.T) {
	settingsInstance = defaultSettings()
	if NeedsDriveSyncOnClose() {
		t.Fatal("expected close sync to be skipped when Drive sync is not configured")
	}

	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true
	if !NeedsDriveSyncOnClose() {
		t.Fatal("expected close sync to be required for pending configured Drive changes")
	}
}

func TestSyncDriveDataOnShutdownPerformsPendingSync(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true

	originalDriveSyncFunc := driveSyncFunc
	defer func() {
		driveSyncFunc = originalDriveSyncFunc
		driveSyncInFlight.Store(false)
	}()
	driveSyncInFlight.Store(false)

	called := false
	driveSyncFunc = func() error {
		called = true
		settingsInstance.GDrive.PendingSync = false
		recordSyncResult(nil)
		return nil
	}

	if err := SyncDriveDataOnShutdown(context.Background()); err != nil {
		t.Fatalf("SyncDriveDataOnShutdown() error = %v", err)
	}
	if !called {
		t.Fatal("expected shutdown sync to call Drive sync")
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
