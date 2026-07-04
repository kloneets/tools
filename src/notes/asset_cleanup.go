package notes

import (
	"os"
	"path/filepath"
	"strings"
)

func CleanupManagedAssetDirs(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || !d.IsDir() {
			return nil
		}
		if isManagedAssetDirName(filepath.Base(path)) {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		return nil
	})
}

func isManagedAssetDirName(name string) bool {
	return name == "assets" || strings.HasSuffix(name, ".assets")
}
