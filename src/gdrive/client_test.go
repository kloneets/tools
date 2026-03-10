package gdrive

import (
	"os"
	"path/filepath"
	"sort"
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

func TestOAuthClientIDUsesEnvOverride(t *testing.T) {
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")

	if got := OAuthClientID(); got != "client-id-1" {
		t.Fatalf("OAuthClientID() = %q, want client-id-1", got)
	}
}

func TestOAuthClientSecretUsesEnvOverride(t *testing.T) {
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "client-secret-1")

	if got := OAuthClientSecret(); got != "client-secret-1" {
		t.Fatalf("OAuthClientSecret() = %q, want client-secret-1", got)
	}
}

func TestOAuthClientLabelUsesBuiltInDefault(t *testing.T) {
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "")

	if got := OAuthClientLabel(); got != defaultOAuthClientID {
		t.Fatalf("OAuthClientLabel() = %q, want %q", got, defaultOAuthClientID)
	}
}

func TestHasCredentialsUsesBuiltInSecret(t *testing.T) {
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "")

	if !HasCredentials() {
		t.Fatal("HasCredentials() should be true when built-in client secret is available")
	}
}

func TestScanLocalNotesTreeReturnsRelativeDirectoriesAndFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Projects", "Alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Root.md"), []byte("root"), 0o644); err != nil {
		t.Fatalf("WriteFile(Root.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Projects", "Alpha", "Spec.md"), []byte("spec"), 0o644); err != nil {
		t.Fatalf("WriteFile(Spec.md) error = %v", err)
	}

	dirs, files, err := scanLocalNotesTree(root)
	if err != nil {
		t.Fatalf("scanLocalNotesTree() error = %v", err)
	}
	if len(dirs) != 2 || dirs[0] != "Projects" || dirs[1] != "Projects/Alpha" {
		t.Fatalf("dirs = %#v, want [Projects Projects/Alpha]", dirs)
	}
	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2", len(files))
	}
	if files[0].RelPath != "Projects/Alpha/Spec.md" {
		t.Fatalf("files[0].RelPath = %q, want Projects/Alpha/Spec.md", files[0].RelPath)
	}
	if files[1].RelPath != "Root.md" {
		t.Fatalf("files[1].RelPath = %q, want Root.md", files[1].RelPath)
	}
}

func TestScanLocalNotesTreeMissingDirectoryIsEmpty(t *testing.T) {
	dirs, files, err := scanLocalNotesTree(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("scanLocalNotesTree() error = %v", err)
	}
	if len(dirs) != 0 || len(files) != 0 {
		t.Fatalf("scanLocalNotesTree() = %#v / %#v, want empty", dirs, files)
	}
}

func TestSortedPathsDescendingDeletesDeepestFoldersFirst(t *testing.T) {
	paths := sortedPathsDescending(map[string]remoteDriveEntry{
		"Projects":             {ID: "1", RelPath: "Projects"},
		"Projects/Alpha":       {ID: "2", RelPath: "Projects/Alpha"},
		"Projects/Alpha/Specs": {ID: "3", RelPath: "Projects/Alpha/Specs"},
	})

	if len(paths) != 3 {
		t.Fatalf("sortedPathsDescending() len = %d, want 3", len(paths))
	}
	if paths[0] != "Projects/Alpha/Specs" || paths[1] != "Projects/Alpha" || paths[2] != "Projects" {
		t.Fatalf("sortedPathsDescending() = %#v", paths)
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

func TestVersionedDriveFileNameAppendsVersionSuffix(t *testing.T) {
	if got := versionedDriveFileName("Plan.md", 3); got != "Plan (version 3).md" {
		t.Fatalf("versionedDriveFileName() = %q, want %q", got, "Plan (version 3).md")
	}
}

func TestDriveTrashRelativePathWrapsNotesUnderTrash(t *testing.T) {
	if got := driveTrashRelativePath("Work/Plan.md"); got != "trash/Work/Plan.md" {
		t.Fatalf("driveTrashRelativePath() = %q, want %q", got, "trash/Work/Plan.md")
	}
	if got := driveTrashRelativePath("trash/Work/Plan.md"); got != "trash/Work/Plan.md" {
		t.Fatalf("driveTrashRelativePath() existing trash = %q", got)
	}
}

func TestIsVersionedDriveFileNameMatchesExpectedPattern(t *testing.T) {
	if !isVersionedDriveFileName("Plan.md", "Plan (version 2).md") {
		t.Fatal("expected versioned filename to match")
	}
	if isVersionedDriveFileName("Plan.md", "Plan 2.md") {
		t.Fatal("unexpected non-versioned filename match")
	}
	if isVersionedDriveFileName("Plan.md", "Another (version 2).md") {
		t.Fatal("unexpected base-name mismatch")
	}
}

func TestRemoteNotesStateEntriesSortsByPathAfterCallerSort(t *testing.T) {
	entries := []remoteStateEntry{
		{Path: "notes/B.md", MimeType: "text/markdown", ModifiedTime: "2"},
		{Path: "notes/A.md", MimeType: "text/markdown", ModifiedTime: "1"},
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].ModifiedTime < entries[j].ModifiedTime
	})

	if entries[0].Path != "notes/A.md" || entries[1].Path != "notes/B.md" {
		t.Fatalf("sorted remoteStateEntry paths = %#v", entries)
	}
}
