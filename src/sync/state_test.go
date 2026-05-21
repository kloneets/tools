package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTripUsesPrivateFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync_state.json")
	state := State{WorkspaceID: "workspace-1", Notes: map[string]int64{"n": 2}}

	if err := SaveState(path, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if got.WorkspaceID != "workspace-1" || got.Notes["n"] != 2 || got.DeviceID == "" {
		t.Fatalf("state = %#v, want persisted workspace, note rev, and device id", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestTokenRoundTripUsesPrivateFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firebase_token.json")
	token := TokenFile{UID: "uid", Email: "user@example.com", RefreshToken: "refresh"}

	if err := SaveToken(path, token); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}
	got, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken() error = %v", err)
	}
	if got != token {
		t.Fatalf("token = %#v, want %#v", got, token)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}
