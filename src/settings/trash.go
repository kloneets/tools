package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type TrashItem struct {
	Path    string
	RelPath string
	IsDir   bool
}

func NotesRootDir() string {
	return getFileName("notes")
}

func NotesTrashDir() string {
	return filepath.Join(NotesRootDir(), "trash")
}

func IsTrashPath(path string) bool {
	trashRoot := filepath.Clean(NotesTrashDir())
	cleanPath := filepath.Clean(path)
	return cleanPath == trashRoot || strings.HasPrefix(cleanPath, trashRoot+string(filepath.Separator))
}

func IsTrashRelativePath(relPath string) bool {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	return relPath == "trash" || strings.HasPrefix(relPath, "trash/")
}

func MoveNoteToTrash(path string) (string, error) {
	return movePathToTrash(path)
}

func MoveFolderToTrash(path string) (string, error) {
	return movePathToTrash(path)
}

func movePathToTrash(path string) (string, error) {
	if IsTrashPath(path) {
		return path, nil
	}

	relPath, err := filepath.Rel(NotesRootDir(), path)
	if err != nil || relPath == "." || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path is outside notes root")
	}

	target := uniqueTrashPath(relPath, isDirPath(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	cleanupEmptyParents(filepath.Dir(path), NotesRootDir())
	return target, nil
}

func RestoreTrashItem(path string) (string, error) {
	if !IsTrashPath(path) {
		return "", fmt.Errorf("path is not in trash")
	}

	relPath, err := filepath.Rel(NotesTrashDir(), path)
	if err != nil || relPath == "." || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path is outside trash")
	}

	target := uniqueRestorePath(relPath, isDirPath(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	cleanupEmptyParents(filepath.Dir(path), NotesTrashDir())
	return target, nil
}

func CleanTrash() error {
	if err := os.RemoveAll(NotesTrashDir()); err != nil {
		return err
	}
	return os.MkdirAll(NotesTrashDir(), 0o755)
}

func ListTrashItems() ([]TrashItem, error) {
	items := make([]TrashItem, 0, 16)
	err := filepath.WalkDir(NotesTrashDir(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == NotesTrashDir() {
			return nil
		}
		relPath, err := filepath.Rel(NotesTrashDir(), path)
		if err != nil {
			return err
		}
		items = append(items, TrashItem{
			Path:    path,
			RelPath: relPath,
			IsDir:   d.IsDir(),
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return filepath.ToSlash(items[i].RelPath) < filepath.ToSlash(items[j].RelPath)
	})
	return items, nil
}

func uniqueTrashPath(relPath string, isDir bool) string {
	base := filepath.Join(NotesTrashDir(), relPath)
	return uniqueFilesystemPath(base, isDir)
}

func uniqueRestorePath(relPath string, isDir bool) string {
	base := filepath.Join(NotesRootDir(), relPath)
	return uniqueFilesystemPath(base, isDir)
}

func uniqueFilesystemPath(base string, isDir bool) string {
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}

	ext := filepath.Ext(base)
	name := strings.TrimSuffix(filepath.Base(base), ext)
	parent := filepath.Dir(base)
	for i := 2; ; i++ {
		candidateName := fmt.Sprintf("%s %d%s", name, i, ext)
		if isDir {
			candidateName = fmt.Sprintf("%s %d", filepath.Base(base), i)
		}
		candidate := filepath.Join(parent, candidateName)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func isDirPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func cleanupEmptyParents(dir string, stop string) {
	stop = filepath.Clean(stop)
	for dir != "" && filepath.Clean(dir) != stop {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
