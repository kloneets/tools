package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kloneets/tools/src/helpers"
)

func BenchmarkNormalizeSettings(b *testing.B) {
	config := &UserSettings{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		candidate := *config
		normalizeSettings(&candidate)
	}
}

func BenchmarkInit(b *testing.B) {
	home := b.TempDir()
	b.Setenv("HOME", home)
	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatalf("MkdirAll() error = %v", err)
	}

	cfg := defaultSettings()
	cfg.NotesApp.BodyFont = "Cantarell 11"
	cfg.GDrive.LastSyncMessage = strings.Repeat("ok", 40)
	data, err := json.Marshal(cfg)
	if err != nil {
		b.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		b.Fatalf("WriteFile() error = %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		settingsInstance = nil
		_ = Init()
	}
}

func BenchmarkSaveSettingsNoSync(b *testing.B) {
	home := b.TempDir()
	b.Setenv("HOME", home)
	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatalf("MkdirAll() error = %v", err)
	}
	settingsInstance = defaultSettings()
	settingsInstance.GDrive.Enabled = false

	originalStatusUpdater := statusUpdater
	statusUpdater = func(string) {}
	defer func() { statusUpdater = originalStatusUpdater }()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0o644); err != nil {
		b.Fatalf("WriteFile() error = %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		saveSettings(false, false)
	}
}
