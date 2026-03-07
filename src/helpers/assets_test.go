package helpers

import (
	"path/filepath"
	"testing"
)

func TestAppIconPathUsesExpectedAssetName(t *testing.T) {
	if got := filepath.Base(AppIconPath()); got != "koko-tools-icon.svg" {
		t.Fatalf("AppIconPath() basename = %q, want %q", got, "koko-tools-icon.svg")
	}
}
