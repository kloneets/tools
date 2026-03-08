package helpers

import (
	"path/filepath"
	"testing"
)

func TestAppIconPathUsesExpectedAssetName(t *testing.T) {
	got := filepath.Base(AppIconPath())
	if got != "koko-tools-icon.svg" && got != "koko-tools-icon-120.png" {
		t.Fatalf("AppIconPath() basename = %q, want supported icon asset", got)
	}
}

func TestAppIconRelativePathByPlatform(t *testing.T) {
	if got := appIconRelativePath("darwin"); got != AppIconRelativePathPNG {
		t.Fatalf("appIconRelativePath(darwin) = %q, want %q", got, AppIconRelativePathPNG)
	}
	if got := appIconRelativePath("linux"); got != AppIconRelativePathSVG {
		t.Fatalf("appIconRelativePath(linux) = %q, want %q", got, AppIconRelativePathSVG)
	}
}

func TestWindowIconNameByPlatform(t *testing.T) {
	if got := windowIconName("darwin"); got != "" {
		t.Fatalf("windowIconName(darwin) = %q, want empty string", got)
	}
	if got := windowIconName("linux"); got != "media-tape" {
		t.Fatalf("windowIconName(linux) = %q, want %q", got, "media-tape")
	}
}
