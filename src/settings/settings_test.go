package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
