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
	if !got.NotesApp.SidebarVisible {
		t.Fatal("SidebarVisible = false, want true")
	}
	if !got.NotesApp.VimMode {
		t.Fatal("VimMode = false, want true")
	}
}

func TestNormalizeSettings(t *testing.T) {
	cfg := &UserSettings{}
	normalizeSettings(cfg)
	if cfg.GDrive == nil || cfg.GDrive.SyncIntervalSec != 10 {
		t.Fatalf("GDrive = %#v, want initialized interval 10", cfg.GDrive)
	}
	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want all widgets visible", cfg.UI)
	}
	if cfg.NotesApp.TabSpaces != 4 {
		t.Fatalf("TabSpaces = %d, want 4", cfg.NotesApp.TabSpaces)
	}
	if !cfg.NotesApp.SidebarVisible {
		t.Fatal("SidebarVisible = false, want true")
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
}
