package notes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kloneets/tools/src/helpers"
)

func TestFileNameUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := fileName()
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes.txt")
	if got != want {
		t.Fatalf("fileName() = %q, want %q", got, want)
	}
}

func TestGetNoteTextReturnsFileContents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	want := "remember the milk"
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(want), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got := getNoteText()
	if got != want {
		t.Fatalf("getNoteText() = %q, want %q", got, want)
	}
}

func TestGetNoteTextMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if got := getNoteText(); got != "" {
		t.Fatalf("getNoteText() = %q, want empty string", got)
	}
}
