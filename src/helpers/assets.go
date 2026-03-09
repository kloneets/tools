package helpers

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	AppIconRelativePathSVG = "assets/koko-tools-icon.svg"
	AppIconRelativePathPNG = "assets/koko-tools-icon-120.png"
	FontAwesomeSolidTTF    = "assets/fonts/fa-solid-900.ttf"
)

func appIconRelativePath(goos string) string {
	if goos == "darwin" {
		return AppIconRelativePathPNG
	}
	return AppIconRelativePathSVG
}

func AppIconPath() string {
	relativePath := appIconRelativePath(runtime.GOOS)
	return assetPath(relativePath)
}

func FontAwesomeSolidPath() string {
	return assetPath(FontAwesomeSolidTTF)
}

func FontAwesomeIconPath(name string) string {
	return assetPath(filepath.Join("assets", "fontawesome", name+".svg"))
}

func assetPath(relativePath string) string {
	var candidates []string
	if executable, err := os.Executable(); err == nil {
		execDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(execDir, relativePath),
			filepath.Join(execDir, "..", "Resources", relativePath),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 5; i++ {
			candidates = append(candidates, filepath.Join(dir, relativePath))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		if _, err := os.Stat(clean); err == nil {
			return clean
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
