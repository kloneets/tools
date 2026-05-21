package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		t.Fatalf("TabSpaces = %d, want 4", got.NotesApp.TabSpaces)
	}
	if got.NotesApp.UndoLevels != 1000 {
		t.Fatalf("UndoLevels = %d, want 1000", got.NotesApp.UndoLevels)
	}
	if !got.NotesApp.SidebarVisible {
		t.Fatal("SidebarVisible = false, want true")
	}
	if !got.NotesApp.VimMode {
		t.Fatal("VimMode = false, want true")
	}
	if got.NotesApp.SpellCheckEnabled {
		t.Fatal("SpellCheckEnabled = true, want false")
	}
	if len(got.NotesApp.SpellDictionaries) != 0 {
		t.Fatalf("SpellDictionaries = %v, want none", got.NotesApp.SpellDictionaries)
	}
	if got.UI == nil || got.UI.TransparentBackground {
		t.Fatalf("TransparentBackground = %v, want false", got.UI != nil && got.UI.TransparentBackground)
	}
	if got.UI.Theme != DefaultTheme {
		t.Fatalf("Theme = %q, want %q", got.UI.Theme, DefaultTheme)
	}
	if got := got.UI.TabOrder; strings.Join(got, ",") != strings.Join(DefaultTabOrder(), ",") {
		t.Fatalf("TabOrder = %v, want default order", got)
	}
	if got.Firebase == nil || !got.Firebase.Realtime || got.Firebase.Enabled {
		t.Fatalf("Firebase = %#v, want disabled realtime defaults", got.Firebase)
	}
}

func TestNormalizeSettings(t *testing.T) {
	cfg := &UserSettings{}
	normalizeSettings(cfg)
	if cfg.GDrive == nil || cfg.GDrive.SyncIntervalSec != 10 {
		t.Fatalf("GDrive = %#v, want initialized interval 10", cfg.GDrive)
	}
	if cfg.Firebase == nil || !cfg.Firebase.Realtime {
		t.Fatalf("Firebase = %#v, want initialized realtime settings", cfg.Firebase)
	}
	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want all widgets visible", cfg.UI)
	}
	if cfg.UI.Theme != DefaultTheme {
		t.Fatalf("Theme = %q, want %q", cfg.UI.Theme, DefaultTheme)
	}
	if got := cfg.UI.TabOrder; strings.Join(got, ",") != strings.Join(DefaultTabOrder(), ",") {
		t.Fatalf("TabOrder = %v, want default order", got)
	}
	if cfg.NotesApp.TabSpaces != 4 {
		t.Fatalf("TabSpaces = %d, want 4", cfg.NotesApp.TabSpaces)
	}
	if cfg.NotesApp.UndoLevels != 1000 {
		t.Fatalf("UndoLevels = %d, want 1000", cfg.NotesApp.UndoLevels)
	}
	if !cfg.NotesApp.SidebarVisible {
		t.Fatal("SidebarVisible = false, want true")
	}
	cfg.NotesApp.SpellDictionaries = []string{"EN", "lv", "en", " "}
	normalizeSettings(cfg)
	if got := cfg.NotesApp.SpellDictionaries; len(got) != 2 || got[0] != "en" || got[1] != "lv" {
		t.Fatalf("SpellDictionaries = %v, want normalized unique codes", got)
	}
}

func TestNormalizeTabOrderRepairsUnknownDuplicatesAndMissing(t *testing.T) {
	got := NormalizeTabOrder([]string{" sync ", "notes", "bogus", "sync", "todo"})
	want := []string{"sync", "notes", "todo", "files", "pages", "password", "settings"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("NormalizeTabOrder() = %v, want %v", got, want)
	}
}

func TestNormalizeSettingsRepairsMobileSnapshotDefaults(t *testing.T) {
	var cfg UserSettings
	if err := json.Unmarshal([]byte(`{
		"pages_app":{"first_book":10},
		"notes_app":{"current_note_path":"mobile.md"},
		"gdrive":{"folder_id":"folder-1"}
	}`), &cfg); err != nil {
		t.Fatalf("Unmarshal mobile settings error = %v", err)
	}

	normalizeSettings(&cfg)

	if !cfg.PasswordApp.Letters || !cfg.PasswordApp.Numbers || !cfg.PasswordApp.SpecialSymbols {
		t.Fatalf("PasswordApp = %#v, want default character groups enabled", cfg.PasswordApp)
	}
	if cfg.PasswordApp.SymbolCount != 16 {
		t.Fatalf("SymbolCount = %d, want 16", cfg.PasswordApp.SymbolCount)
	}
	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want default visible tools", cfg.UI)
	}
	if cfg.AppWindow.Width != 600 || cfg.AppWindow.Height != 300 {
		t.Fatalf("AppWindow = %#v, want default dimensions", cfg.AppWindow)
	}
	if cfg.NotesApp.TabSpaces != 4 || cfg.NotesApp.UndoLevels != 1000 {
		t.Fatalf("NotesApp = %#v, want editor defaults", cfg.NotesApp)
	}
}

func TestUISettingsUnmarshalDefaultsMissingVisibilityFields(t *testing.T) {
	var cfg UserSettings
	if err := json.Unmarshal([]byte(`{"ui":{"theme":"gruvbox"}}`), &cfg); err != nil {
		t.Fatalf("Unmarshal UI settings error = %v", err)
	}

	normalizeSettings(&cfg)

	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want missing visibility fields to default true", cfg.UI)
	}
	if cfg.UI.Theme != "gruvbox" {
		t.Fatalf("Theme = %q, want gruvbox", cfg.UI.Theme)
	}
}

func TestNormalizeSettingsResetsUnknownTheme(t *testing.T) {
	cfg := &UserSettings{UI: &UISettings{Theme: "unknown"}}
	normalizeSettings(cfg)
	if cfg.UI.Theme != DefaultTheme {
		t.Fatalf("Theme = %q, want %q", cfg.UI.Theme, DefaultTheme)
	}
}

func TestThemePersistsInJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.UI.Theme = "catppuccin"
	writeSettingsToDisk(false)
	data, err := os.ReadFile(fileName())
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !strings.Contains(string(data), `"theme":"catppuccin"`) {
		t.Fatalf("settings file = %q, want theme persisted", string(data))
	}
}

func TestInitMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settingsInstance = nil
	messages := Init()
	if settingsInstance == nil {
		t.Fatal("settingsInstance should be initialized")
	}
	if len(*messages) != 1 || !strings.Contains((*messages)[0], "Settings read error") {
		t.Fatalf("Init() messages = %#v, want read error", *messages)
	}
}

func TestPersistedNotesEditorWidth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.NotesApp.EditorWidth = 41
	writeSettingsToDisk(false)
	settingsInstance.NotesApp.EditorWidth = 0
	if got := PersistedNotesEditorWidth(); got != 41 {
		t.Fatalf("PersistedNotesEditorWidth() = %d, want 41", got)
	}
}

func TestSaveNotesEditorWidth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	SaveNotesEditorWidth(33)
	if settingsInstance.NotesApp.EditorWidth != 33 {
		t.Fatalf("EditorWidth = %d, want 33", settingsInstance.NotesApp.EditorWidth)
	}
	if _, err := os.Stat(fileName()); !os.IsNotExist(err) {
		t.Fatalf("settings file should not be written automatically, stat err = %v", err)
	}
}

func TestSaveNotesPreviewHiddenPersistsLocally(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.PendingSync = false
	SaveNotesPreviewHidden(true)
	if !settingsInstance.NotesApp.PreviewHidden {
		t.Fatal("PreviewHidden = false, want true")
	}
	if settingsInstance.GDrive.PendingSync {
		t.Fatal("PendingSync = true, want false for local UI preference persistence")
	}
	data, err := os.ReadFile(fileName())
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !strings.Contains(string(data), `"preview_hidden":true`) {
		t.Fatalf("settings file = %q, want preview_hidden persisted", string(data))
	}
}

func TestSaveNotesSessionPersistsLocally(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.PendingSync = false
	root := getFileName("notes")
	SaveNotesSession([]string{
		filepath.Join(root, "Note 1.md"),
		filepath.Join(root, "Projects", "Note 2.md"),
		filepath.Join(root, "Note 1.md"),
	}, filepath.Join(root, "Projects", "Note 2.md"))
	if settingsInstance.GDrive.PendingSync {
		t.Fatal("PendingSync = true, want false for note session persistence")
	}
	if got := settingsInstance.NotesApp.OpenNotePaths; len(got) != 2 || got[0] != "Note 1.md" || got[1] != "Projects/Note 2.md" {
		t.Fatalf("OpenNotePaths = %#v", got)
	}
	if got := settingsInstance.NotesApp.CurrentNotePath; got != "Projects/Note 2.md" {
		t.Fatalf("CurrentNotePath = %q, want Projects/Note 2.md", got)
	}
}

func TestNormalizeSettingsConvertsPortableAbsoluteNoteSessionPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := defaultSettings()
	s.NotesApp.OpenNotePaths = []string{
		"/old-home/.config/koko-tools/notes/Note 1.md",
		"/old-home/.config/koko-tools/notes/Projects/Note 2.md",
	}
	s.NotesApp.CurrentNotePath = "/old-home/.config/koko-tools/notes/Projects/Note 2.md"

	normalizeSettings(s)

	if got := s.NotesApp.OpenNotePaths; len(got) != 2 || got[0] != "Note 1.md" || got[1] != "Projects/Note 2.md" {
		t.Fatalf("OpenNotePaths = %#v", got)
	}
	if got := s.NotesApp.CurrentNotePath; got != "Projects/Note 2.md" {
		t.Fatalf("CurrentNotePath = %q, want Projects/Note 2.md", got)
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

func TestGDriveReady(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	cfg := &GDriveSettings{Enabled: true, FolderID: "folder-1"}
	if !cfg.Ready() {
		t.Fatal("Ready() should return true")
	}
}

func TestDriveSyncInterval(t *testing.T) {
	settingsInstance = nil
	if got := DriveSyncInterval(); got != 10*time.Second {
		t.Fatalf("DriveSyncInterval() = %v, want 10s", got)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.SyncIntervalSec = 25
	if got := DriveSyncInterval(); got != 25*time.Second {
		t.Fatalf("DriveSyncInterval() = %v, want 25s", got)
	}
}

func TestSyncDriveDataOnShutdownIsNoOp(t *testing.T) {
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := SyncDriveDataOnShutdown(ctx)
	if err != nil {
		t.Fatalf("SyncDriveDataOnShutdown() error = %v, want nil", err)
	}
}

func TestSaveSettingsLocalDoesNotStartDriveSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true

	originalDriveSyncFunc := driveSyncFunc
	defer func() { driveSyncFunc = originalDriveSyncFunc }()
	called := make(chan struct{}, 1)
	driveSyncFunc = func() error {
		called <- struct{}{}
		return nil
	}

	SaveSettingsLocal()
	select {
	case <-called:
		t.Fatal("SaveSettingsLocal() started drive sync, want local-only save")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStartDriveSyncDoesNothingDuringShutdown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.PendingSync = true

	originalDriveSyncFunc := driveSyncFunc
	defer func() {
		driveSyncFunc = originalDriveSyncFunc
		shuttingDown.Store(false)
	}()
	called := make(chan struct{}, 1)
	driveSyncFunc = func() error {
		called <- struct{}{}
		return nil
	}

	BeginShutdown()
	StartDriveSync()
	select {
	case <-called:
		t.Fatal("StartDriveSync() started sync during shutdown")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSyncDriveDataStoresUploadedSnapshotMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"

	origUpload := driveUploadSnapshotFunc
	origList := driveListSnapshotsFunc
	defer func() {
		driveUploadSnapshotFunc = origUpload
		driveListSnapshotsFunc = origList
	}()
	driveUploadSnapshotFunc = func(folderID string, settingsData []byte, retain int) (gdrive.SnapshotMeta, error) {
		return gdrive.SnapshotMeta{ID: "snap-1", Name: "snapshot-a", CreatedAt: "2026-04-15T10:00:00Z"}, nil
	}
	driveListSnapshotsFunc = func(folderID string) ([]gdrive.SnapshotMeta, error) {
		return []gdrive.SnapshotMeta{{ID: "snap-1", Name: "snapshot-a", CreatedAt: "2026-04-15T10:00:00Z"}}, nil
	}

	if err := SyncDriveData(); err != nil {
		t.Fatalf("SyncDriveData() error = %v", err)
	}
	if got := settingsInstance.GDrive.SelectedSnapshotID; got != "snap-1" {
		t.Fatalf("SelectedSnapshotID = %q, want snap-1", got)
	}
	if got := settingsInstance.GDrive.LastDriveSaveAt; got != "2026-04-15T10:00:00Z" {
		t.Fatalf("LastDriveSaveAt = %q, want created time", got)
	}
	if len(settingsInstance.GDrive.Snapshots) != 1 {
		t.Fatalf("Snapshots len = %d, want 1", len(settingsInstance.GDrive.Snapshots))
	}
}

func TestRestoreDriveSnapshotUsesHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.Snapshots = []DriveSnapshotMeta{{ID: "snap-2", Name: "snapshot-b", CreatedAt: "2026-04-15T11:00:00Z"}}

	origRestore := driveRestoreSnapshotFunc
	defer func() { driveRestoreSnapshotFunc = origRestore }()
	driveRestoreSnapshotFunc = func(snapshotID string) ([]byte, error) {
		restored := defaultSettings()
		restored.NotesApp.TabSpaces = 8
		restored.NotesApp.OpenNotePaths = []string{
			"/old-home/.config/koko-tools/notes/Note 1.md",
			"/old-home/.config/koko-tools/notes/Projects/Plan.md",
		}
		restored.NotesApp.CurrentNotePath = "/old-home/.config/koko-tools/notes/Projects/Plan.md"
		return json.Marshal(restored)
	}

	if err := RestoreDriveSnapshot("snap-2"); err != nil {
		t.Fatalf("RestoreDriveSnapshot() error = %v", err)
	}
	if got := settingsInstance.NotesApp.TabSpaces; got != 8 {
		t.Fatalf("TabSpaces = %d, want restored value 8", got)
	}
	if got := settingsInstance.GDrive.SelectedSnapshotID; got != "snap-2" {
		t.Fatalf("SelectedSnapshotID = %q, want snap-2", got)
	}
	if got := settingsInstance.NotesApp.OpenNotePaths; len(got) != 2 || got[0] != "Note 1.md" || got[1] != "Projects/Plan.md" {
		t.Fatalf("OpenNotePaths = %#v", got)
	}
	if got := settingsInstance.NotesApp.CurrentNotePath; got != "Projects/Plan.md" {
		t.Fatalf("CurrentNotePath = %q, want Projects/Plan.md", got)
	}
}

func TestRestoreDriveSnapshotRepairsMobileSettingsSubset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = true
	settingsInstance.GDrive.FolderID = "folder-1"
	settingsInstance.GDrive.FolderName = "Koko"
	settingsInstance.GDrive.Snapshots = []DriveSnapshotMeta{{ID: "snap-mobile", Name: "mobile", CreatedAt: "2026-05-14T10:00:00Z"}}

	origRestore := driveRestoreSnapshotFunc
	defer func() { driveRestoreSnapshotFunc = origRestore }()
	driveRestoreSnapshotFunc = func(snapshotID string) ([]byte, error) {
		return []byte(`{
			"pages_app":{"first_book":100,"second_book":200,"read_pages":10},
			"notes_app":{"current_note_path":"mobile.md"},
			"gdrive":{"folder_id":"mobile-folder"}
		}`), nil
	}

	if err := RestoreDriveSnapshot("snap-mobile"); err != nil {
		t.Fatalf("RestoreDriveSnapshot() error = %v", err)
	}
	if !settingsInstance.PasswordApp.Letters || !settingsInstance.PasswordApp.Numbers || !settingsInstance.PasswordApp.SpecialSymbols {
		t.Fatalf("PasswordApp = %#v, want restored desktop-safe defaults", settingsInstance.PasswordApp)
	}
	if settingsInstance.PasswordApp.SymbolCount != 16 {
		t.Fatalf("SymbolCount = %d, want 16", settingsInstance.PasswordApp.SymbolCount)
	}
	if settingsInstance.UI == nil || !settingsInstance.UI.ShowNotes || !settingsInstance.UI.ShowPages || !settingsInstance.UI.ShowPassword {
		t.Fatalf("UI = %#v, want restored desktop-safe defaults", settingsInstance.UI)
	}
	if settingsInstance.AppWindow.Width != 600 || settingsInstance.AppWindow.Height != 300 {
		t.Fatalf("AppWindow = %#v, want restored desktop-safe defaults", settingsInstance.AppWindow)
	}
	if settingsInstance.GDrive.FolderID != "folder-1" || settingsInstance.GDrive.FolderName != "Koko" {
		t.Fatalf("GDrive = %#v, want current desktop Drive context preserved", settingsInstance.GDrive)
	}
}
