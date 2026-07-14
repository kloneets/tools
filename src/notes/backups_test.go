package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceNoteFileBacksUpEmptyOverwriteAndRestoresContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notePath := filepath.Join(notesDir(), "Home", "uz-Bebreni.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("verified non-empty content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceNoteFile(notePath, nil); err != nil {
		t.Fatalf("ReplaceNoteFile() error = %v", err)
	}
	backup, err := LatestNoteBackup(notePath)
	if err != nil {
		t.Fatalf("LatestNoteBackup() error = %v", err)
	}
	if strings.HasPrefix(backup.Path, notesDir()+string(filepath.Separator)) {
		t.Fatalf("backup path %q is inside live notes tree", backup.Path)
	}
	content, err := os.ReadFile(backup.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "verified non-empty content" {
		t.Fatalf("backup content = %q, want original", content)
	}
	if err := RestoreNoteBackup(notePath, backup); err != nil {
		t.Fatalf("RestoreNoteBackup() error = %v", err)
	}
	restored, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "verified non-empty content" {
		t.Fatalf("restored content = %q, want original", restored)
	}
}

func TestSaveCurrentLocalBacksUpReplacedContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notePath := filepath.Join(notesDir(), "Save.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("before save"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace := &Workspace{
		Tabs:       []*Editor{{Path: notePath, Text: "after save", Dirty: true}},
		CurrentTab: 0,
	}

	if err := workspace.SaveCurrentLocal(); err != nil {
		t.Fatalf("SaveCurrentLocal() error = %v", err)
	}
	backup, err := LatestNoteBackup(notePath)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(backup.Path)
	if err != nil || string(content) != "before save" {
		t.Fatalf("backup content = %q, error = %v", content, err)
	}
}

func TestReplaceNoteFileKeepsBoundedHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notePath := filepath.Join(notesDir(), "Plan.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("version-0"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= noteBackupRetention+3; i++ {
		if err := ReplaceNoteFile(notePath, []byte(strings.Repeat("x", i))); err != nil {
			t.Fatalf("ReplaceNoteFile(version %d) error = %v", i, err)
		}
	}
	backups, err := ListNoteBackups(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != noteBackupRetention {
		t.Fatalf("backup count = %d, want %d", len(backups), noteBackupRetention)
	}
}

func TestRemoveNoteFolderBacksUpNestedNotes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	folder := filepath.Join(notesDir(), "Projects")
	notePath := filepath.Join(folder, "Archive", "Plan.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveNoteFolder(folder); err != nil {
		t.Fatalf("RemoveNoteFolder() error = %v", err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("deleted folder stat error = %v, want not exist", err)
	}
	backup, err := LatestNoteBackup(notePath)
	if err != nil {
		t.Fatalf("LatestNoteBackup() error = %v", err)
	}
	content, err := os.ReadFile(backup.Path)
	if err != nil || string(content) != "nested" {
		t.Fatalf("backup content = %q, error = %v", content, err)
	}
}

func TestListNoteBackupsRejectsPathOutsideNotesTree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	outside := filepath.Join(filepath.Dir(notesDir()), "outside.md")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ListNoteBackups(outside)
	if err == nil || !strings.Contains(err.Error(), "outside notes directory") {
		t.Fatalf("ListNoteBackups() error = %v, want safe path rejection", err)
	}
	if err := ReplaceNoteFile(outside, []byte("replacement")); err == nil {
		t.Fatal("ReplaceNoteFile() error = nil, want backup failure to block overwrite")
	}
	content, readErr := os.ReadFile(outside)
	if readErr != nil || string(content) != "original" {
		t.Fatalf("outside content = %q, error = %v; want original untouched", content, readErr)
	}
}

func TestReplaceNoteFileRejectsSymlinkTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(notesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(notesDir(), "link.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	err := ReplaceNoteFile(link, []byte("replacement"))
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ReplaceNoteFile() error = %v, want symlink rejection", err)
	}
	content, readErr := os.ReadFile(outside)
	if readErr != nil || string(content) != "outside" {
		t.Fatalf("outside content = %q, error = %v", content, readErr)
	}
}
