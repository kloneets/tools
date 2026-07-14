package notes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const noteBackupRetention = 20

var noteMutationMu sync.Mutex

type NoteBackup struct {
	Path      string
	NotePath  string
	CreatedAt time.Time
}

// ReplaceNoteFile preserves changed existing content before atomically replacing it.
func ReplaceNoteFile(path string, content []byte) error {
	noteMutationMu.Lock()
	defer noteMutationMu.Unlock()

	current, mode, exists, err := readExistingNote(path)
	if err != nil {
		return err
	}
	if exists && bytes.Equal(current, content) {
		return nil
	}
	if exists {
		if _, err := writeNoteBackupLocked(path, current); err != nil {
			return fmt.Errorf("back up note before replace: %w", err)
		}
	}
	if !exists {
		mode = 0o644
	}
	return writeFileAtomic(path, content, mode)
}

// RemoveNoteFile preserves existing content before deleting the note.
func RemoveNoteFile(path string) error {
	noteMutationMu.Lock()
	defer noteMutationMu.Unlock()

	content, _, exists, err := readExistingNote(path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := writeNoteBackupLocked(path, content); err != nil {
		return fmt.Errorf("back up note before delete: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

// RemoveNoteFolder preserves every Markdown note before deleting the folder.
func RemoveNoteFolder(path string) error {
	noteMutationMu.Lock()
	defer noteMutationMu.Unlock()

	paths := make([]string, 0)
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(current), ".md") {
			paths = append(paths, current)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, notePath := range paths {
		content, _, exists, err := readExistingNote(notePath)
		if err != nil {
			return err
		}
		if exists {
			if _, err := writeNoteBackupLocked(notePath, content); err != nil {
				return fmt.Errorf("back up note before folder delete: %w", err)
			}
		}
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func ListNoteBackups(notePath string) ([]NoteBackup, error) {
	noteMutationMu.Lock()
	defer noteMutationMu.Unlock()
	return listNoteBackupsLocked(notePath)
}

func LatestNoteBackup(notePath string) (NoteBackup, error) {
	backups, err := ListNoteBackups(notePath)
	if err != nil {
		return NoteBackup{}, err
	}
	if len(backups) == 0 {
		return NoteBackup{}, fs.ErrNotExist
	}
	return backups[len(backups)-1], nil
}

func RestoreNoteBackup(notePath string, backup NoteBackup) error {
	backupDir, err := noteBackupDir(notePath)
	if err != nil {
		return err
	}
	backupPath, err := filepath.Abs(backup.Path)
	if err != nil {
		return err
	}
	if !pathWithin(backupDir, backupPath) || filepath.Dir(backupPath) != backupDir {
		return fmt.Errorf("backup path is outside note history")
	}
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	return ReplaceNoteFile(notePath, content)
}

func readExistingNote(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("note path is not a regular file: %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return content, info.Mode().Perm(), true, nil
}

func writeNoteBackupLocked(notePath string, content []byte) (NoteBackup, error) {
	dir, err := noteBackupDir(notePath)
	if err != nil {
		return NoteBackup{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return NoteBackup{}, err
	}
	sum := sha256.Sum256(content)
	prefix := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(sum[:6]) + "-"
	tmp, err := os.CreateTemp(dir, prefix+"*.md")
	if err != nil {
		return NoteBackup{}, err
	}
	path := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return NoteBackup{}, err
	}
	if _, err := tmp.Write(content); err != nil {
		return NoteBackup{}, err
	}
	if err := tmp.Sync(); err != nil {
		return NoteBackup{}, err
	}
	if err := tmp.Close(); err != nil {
		return NoteBackup{}, err
	}
	keep = true
	if err := syncDirectory(dir); err != nil {
		return NoteBackup{}, err
	}
	if err := pruneNoteBackupsLocked(notePath); err != nil {
		return NoteBackup{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return NoteBackup{}, err
	}
	return NoteBackup{Path: path, NotePath: notePath, CreatedAt: info.ModTime().UTC()}, nil
}

func pruneNoteBackupsLocked(notePath string) error {
	backups, err := listNoteBackupsLocked(notePath)
	if err != nil {
		return err
	}
	for len(backups) > noteBackupRetention {
		if err := os.Remove(backups[0].Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		backups = backups[1:]
	}
	dir, _ := noteBackupDir(notePath)
	return syncDirectory(dir)
}

func listNoteBackupsLocked(notePath string) ([]NoteBackup, error) {
	dir, err := noteBackupDir(notePath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []NoteBackup{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]NoteBackup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		backups = append(backups, NoteBackup{
			Path:      filepath.Join(dir, entry.Name()),
			NotePath:  notePath,
			CreatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		return filepath.Base(backups[i].Path) < filepath.Base(backups[j].Path)
	})
	return backups, nil
}

func noteBackupDir(notePath string) (string, error) {
	root, err := filepath.Abs(notesDir())
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(notePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("note path is outside notes directory: %s", notePath)
	}
	rel = filepath.Clean(rel)
	backupRoot := filepath.Join(filepath.Dir(root), "note_backups")
	dir := filepath.Join(backupRoot, rel+".versions")
	if !pathWithin(backupRoot, dir) {
		return "", fmt.Errorf("invalid note backup path: %s", notePath)
	}
	return dir, nil
}

func pathWithin(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".note-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return err
	}
	return nil
}
