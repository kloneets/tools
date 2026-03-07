package gdrive

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestTokenFromFileRoundTrip(t *testing.T) {
	tok := &oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
	}
	path := filepath.Join(t.TempDir(), "token.json")

	if err := saveToken(path, tok); err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}

	got, err := tokenFromFile(path)
	if err != nil {
		t.Fatalf("tokenFromFile() error = %v", err)
	}
	if got.AccessToken != tok.AccessToken || got.RefreshToken != tok.RefreshToken || got.TokenType != tok.TokenType {
		t.Fatalf("token mismatch: got %#v want %#v", got, tok)
	}
}

func TestTokenFromFileMissingFile(t *testing.T) {
	_, err := tokenFromFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestDefaultCredentialsPath(t *testing.T) {
	if got := filepath.Base(DefaultCredentialsPath()); got != "credentials.json" {
		t.Fatalf("DefaultCredentialsPath() basename = %q, want credentials.json", got)
	}
}

func TestSyncablePathsSkipsCredentialsAndToken(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"settings.json", "notes.txt", "credentials.json", "gdrive_token.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	paths, err := syncablePaths(dir)
	if err != nil {
		t.Fatalf("syncablePaths() error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("syncablePaths() len = %d, want 2", len(paths))
	}
}

func TestDecorateFolderPathsBuildsBreadcrumbs(t *testing.T) {
	folders := []Folder{
		{ID: "root-child", Name: "Projects", Parents: []string{"root"}},
		{ID: "nested", Name: "Koko", Parents: []string{"root-child"}},
	}

	got := decorateFolderPaths(folders)

	if got[0].Path != "Drive / Projects" {
		t.Fatalf("first path = %q, want %q", got[0].Path, "Drive / Projects")
	}
	if got[0].TopLevel != "Projects" {
		t.Fatalf("first top-level = %q, want %q", got[0].TopLevel, "Projects")
	}
	if got[1].Path != "Drive / Projects / Koko" {
		t.Fatalf("second path = %q, want %q", got[1].Path, "Drive / Projects / Koko")
	}
	if got[1].TopLevel != "Projects" {
		t.Fatalf("second top-level = %q, want %q", got[1].TopLevel, "Projects")
	}
}

func TestDriveCreateFileIncludesParent(t *testing.T) {
	file := driveCreateFile("settings.json", "folder-1")

	if file.Name != "settings.json" {
		t.Fatalf("driveCreateFile() name = %q, want settings.json", file.Name)
	}
	if len(file.Parents) != 1 || file.Parents[0] != "folder-1" {
		t.Fatalf("driveCreateFile() parents = %#v, want [folder-1]", file.Parents)
	}
}

func TestDriveUpdateFileDoesNotIncludeParent(t *testing.T) {
	file := driveUpdateFile("settings.json")

	if file.Name != "settings.json" {
		t.Fatalf("driveUpdateFile() name = %q, want settings.json", file.Name)
	}
	if len(file.Parents) != 0 {
		t.Fatalf("driveUpdateFile() parents = %#v, want empty", file.Parents)
	}
}
