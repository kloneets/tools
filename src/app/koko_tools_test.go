package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kloneets/tools/src/helpers"
)

func TestConfigDir(t *testing.T) {
	home := "/tmp/example"
	got := configDir(home)
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if got != want {
		t.Fatalf("configDir() = %q, want %q", got, want)
	}
}

func TestEnsureConfigDirExistsCreatesParents(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nested", helpers.AppConfigMainDir, helpers.AppConfigAppDir)

	ensureConfigDirExists(target)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", target)
	}
}
