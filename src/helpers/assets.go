package helpers

import (
	"os"
	"path/filepath"
)

const AppIconRelativePath = "assets/koko-tools-icon.svg"

func AppIconPath() string {
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), AppIconRelativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return AppIconRelativePath
}
