package gdrive

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func BenchmarkSyncablePaths(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 200; i++ {
		name := filepath.Join(dir, "notes", "note-"+string(rune('a'+(i%26)))+"-"+filepath.Base(filepath.Clean(filepath.Join("x", ".."))))
		_ = name
	}
	for i := 0; i < 100; i++ {
		path := filepath.Join(dir, "notes", "note-"+itoa(i)+".md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte("note"), 0o644); err != nil {
			b.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	for i := 0; i < 30; i++ {
		path := filepath.Join(dir, "root-"+itoa(i)+".json")
		if err := os.WriteFile(path, []byte("cfg"), 0o644); err != nil {
			b.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(TokenPath())), []byte("token"), 0o644); err != nil {
		b.Fatalf("WriteFile(token) error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		paths, err := syncablePaths(dir)
		if err != nil {
			b.Fatalf("syncablePaths() error = %v", err)
		}
		if len(paths) == 0 {
			b.Fatal("syncablePaths() returned no paths")
		}
	}
}

func BenchmarkDecorateFolderPaths(b *testing.B) {
	folders := make([]Folder, 0, 500)
	for i := 0; i < 100; i++ {
		rootID := "root-" + itoa(i)
		folders = append(folders, Folder{ID: rootID, Name: "Root " + itoa(i)})
		for j := 0; j < 4; j++ {
			childID := rootID + "-child-" + itoa(j)
			folders = append(folders, Folder{ID: childID, Name: "Child " + itoa(j), Parents: []string{rootID}})
			folders = append(folders, Folder{ID: childID + "-leaf", Name: "Leaf", Parents: []string{childID}})
		}
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decorateFolderPaths(append([]Folder(nil), folders...))
	}
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
