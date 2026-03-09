package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveNoteToTrashMovesFileUnderTrashRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	source := filepath.Join(NotesRootDir(), "Work", "Plan.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(source, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	target, err := MoveNoteToTrash(source)
	if err != nil {
		t.Fatalf("MoveNoteToTrash() error = %v", err)
	}

	if !IsTrashPath(target) {
		t.Fatalf("target = %q, want trash path", target)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target stat error = %v", err)
	}
}

func TestRestoreTrashItemMovesFileBackToNotesRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	trashPath := filepath.Join(NotesTrashDir(), "Work", "Plan.md")
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(trashPath, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	target, err := RestoreTrashItem(trashPath)
	if err != nil {
		t.Fatalf("RestoreTrashItem() error = %v", err)
	}

	want := filepath.Join(NotesRootDir(), "Work", "Plan.md")
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("restored target stat error = %v", err)
	}
}

func TestListTrashItemsReturnsFilesAndFolders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	folder := filepath.Join(NotesTrashDir(), "Work")
	file := filepath.Join(folder, "Plan.md")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(file, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	items, err := ListTrashItems()
	if err != nil {
		t.Fatalf("ListTrashItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	if !items[0].IsDir || filepath.ToSlash(items[0].RelPath) != "Work" {
		t.Fatalf("items[0] = %#v, want Work folder", items[0])
	}
	if items[1].IsDir || filepath.ToSlash(items[1].RelPath) != "Work/Plan.md" {
		t.Fatalf("items[1] = %#v, want Work/Plan.md file", items[1])
	}
}

func TestCleanTrashRemovesItemsAndKeepsDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	file := filepath.Join(NotesTrashDir(), "Plan.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(file, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := CleanTrash(); err != nil {
		t.Fatalf("CleanTrash() error = %v", err)
	}
	if _, err := os.Stat(NotesTrashDir()); err != nil {
		t.Fatalf("trash root stat error = %v", err)
	}
	items, err := ListTrashItems()
	if err != nil {
		t.Fatalf("ListTrashItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items len = %d, want 0", len(items))
	}
}
