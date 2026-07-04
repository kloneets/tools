package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupManagedAssetDirsDeletesOnlyManagedAssetDirs(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "Home", "Plan.assets", "image.png"),
		filepath.Join(root, "assets", "file.bin"),
		filepath.Join(root, "Home", "Plan.md"),
		filepath.Join(root, "Home", "regular", "file.bin"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	if err := CleanupManagedAssetDirs(root); err != nil {
		t.Fatalf("CleanupManagedAssetDirs() error = %v", err)
	}

	for _, removed := range []string{
		filepath.Join(root, "Home", "Plan.assets"),
		filepath.Join(root, "assets"),
	} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("%q still exists, err = %v", removed, err)
		}
	}
	for _, kept := range []string{
		filepath.Join(root, "Home", "Plan.md"),
		filepath.Join(root, "Home", "regular", "file.bin"),
	} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%q missing after cleanup: %v", kept, err)
		}
	}
}
