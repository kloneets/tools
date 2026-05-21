package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTripUsesPrivateFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firebase_config.json")
	cfg := FirebaseConfig{
		Enabled:     true,
		Realtime:    true,
		APIKey:      "api",
		DatabaseURL: "https://example.firebaseio.com",
		WorkspaceID: "personal",
		Email:       "user@example.com",
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got != cfg {
		t.Fatalf("config = %#v, want %#v", got, cfg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestApplyDefaultsPopulatesBundledFirebaseConfig(t *testing.T) {
	cfg := ApplyDefaults(FirebaseConfig{})

	if cfg.APIKey != DefaultAPIKey {
		t.Fatalf("APIKey = %q, want bundled default", cfg.APIKey)
	}
	if cfg.DatabaseURL != DefaultDatabaseURL {
		t.Fatalf("DatabaseURL = %q, want bundled default", cfg.DatabaseURL)
	}
}

func TestApplyDefaultsPreservesCustomFirebaseConfig(t *testing.T) {
	cfg := ApplyDefaults(FirebaseConfig{
		APIKey:      "custom-key",
		DatabaseURL: "https://custom.firebaseio.com",
	})

	if cfg.APIKey != "custom-key" {
		t.Fatalf("APIKey = %q, want custom value", cfg.APIKey)
	}
	if cfg.DatabaseURL != "https://custom.firebaseio.com" {
		t.Fatalf("DatabaseURL = %q, want custom value", cfg.DatabaseURL)
	}
}
