package helpers

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	AppIconRelativePathSVG = "assets/koko-tools-icon.svg"
	AppIconRelativePathPNG = "assets/koko-tools-icon-120.png"
)

func appIconRelativePath(goos string) string {
	if goos == "darwin" {
		return AppIconRelativePathPNG
	}
	return AppIconRelativePathSVG
}

func AppIconPath() string {
	relativePath := appIconRelativePath(runtime.GOOS)
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return relativePath
}

func windowIconName(goos string) string {
	if goos == "darwin" {
		return ""
	}
	return "media-tape"
}

func WindowIconName() string {
	return windowIconName(runtime.GOOS)
}
