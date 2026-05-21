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
