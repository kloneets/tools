package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type NoteSyncer struct {
	Provider    Provider
	WorkspaceID string
	StatePath   string
	Session     Session
	DeviceID    string
	Now         func() time.Time
}

type NoteFile struct {
	ID      string
	Path    string
	Text    string
	Dirty   bool
	BaseRev int64
}

type NoteMergeResult struct {
	Notes        map[string]LocalNote
	Upserts      []LocalNote
	Deletes      []string
	ConflictCopy []LocalNote
	Changed      bool
	State        State
}

type NotePushResult struct {
	Pushed int
	State  State
}

func NoteID(path string) string {
	normalized := NormalizeNotePath(path)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func NormalizeNotePath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	parts := strings.Split(path, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/")
}

func (s *NoteSyncer) Ready() bool {
	return s != nil && s.Provider != nil && s.WorkspaceID != "" && s.Session.IDToken != ""
}

func (s *NoteSyncer) PushNotes(ctx context.Context, notes []NoteFile) (NotePushResult, error) {
	result := NotePushResult{}
	if !s.Ready() {
		return result, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return result, err
	}
	if state.WorkspaceID != "" && state.WorkspaceID != s.WorkspaceID {
		state.Notes = map[string]int64{}
		state.NoteHashes = map[string]string{}
	}
	deviceID := s.deviceID(state)
	now := s.now()
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	for _, note := range notes {
		path := NormalizeNotePath(note.Path)
		if path == "" || strings.ToLower(filepath.Ext(path)) != ".md" {
			continue
		}
		id := note.ID
		if id == "" {
			id = NoteID(path)
		}
		hash := NoteContentHash(note.Text)
		if state.WorkspaceID == s.WorkspaceID && state.NoteHashes[id] == hash {
			continue
		}
		if state.WorkspaceID == s.WorkspaceID && state.Notes[id] > 0 && state.NoteHashes[id] == "" {
			state.NoteHashes[id] = hash
			continue
		}
		rev := now.UnixMilli()
		if rev <= state.Notes[id] {
			rev = state.Notes[id] + 1
		}
		record := NoteRecord{
			ID:        id,
			Path:      path,
			Text:      note.Text,
			Rev:       rev,
			UpdatedAt: now,
			UpdatedBy: s.Session.UID,
		}
		if err := s.Provider.PushMutation(ctx, s.WorkspaceID, Mutation{
			EventID:   fmt.Sprintf("note-%s-%d", id, rev),
			DeviceID:  deviceID,
			Note:      &record,
			CreatedAt: now,
		}); err != nil {
			return result, err
		}
		state.Notes[id] = rev
		state.NoteHashes[id] = hash
		result.Pushed++
	}
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if err := SaveState(s.StatePath, state); err != nil {
		return result, err
	}
	result.State = state
	return result, nil
}

func (s *NoteSyncer) PushDelete(ctx context.Context, path string) error {
	if !s.Ready() {
		return nil
	}
	path = NormalizeNotePath(path)
	if path == "" {
		return nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return err
	}
	id := NoteID(path)
	now := s.now()
	rev := now.UnixMilli()
	if rev <= state.Notes[id] {
		rev = state.Notes[id] + 1
	}
	record := NoteRecord{
		ID:        id,
		Path:      path,
		Rev:       rev,
		UpdatedAt: now,
		UpdatedBy: s.Session.UID,
		Deleted:   true,
	}
	if err := s.Provider.PushMutation(ctx, s.WorkspaceID, Mutation{
		EventID:   fmt.Sprintf("note-delete-%s-%d", id, rev),
		DeviceID:  s.deviceID(state),
		Note:      &record,
		CreatedAt: now,
	}); err != nil {
		return err
	}
	state.Notes[id] = rev
	delete(state.NoteHashes, id)
	delete(state.SyncHashes, SyncFeatureNotes)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	return SaveState(s.StatePath, state)
}

func (s *NoteSyncer) PullNotes(ctx context.Context, local map[string]LocalNote) (NoteMergeResult, error) {
	result := NoteMergeResult{Notes: map[string]LocalNote{}}
	for path, note := range local {
		result.Notes[NormalizeNotePath(path)] = note
	}
	if !s.Ready() {
		return result, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return result, err
	}
	now := s.now()
	remoteNotes, err := pullRemoteNotes(ctx, s.Provider, s.WorkspaceID)
	if err != nil {
		return result, err
	}
	deviceID := s.deviceID(state)
	remoteIDs := make([]string, 0, len(remoteNotes))
	for id := range remoteNotes {
		remoteIDs = append(remoteIDs, id)
	}
	sort.Strings(remoteIDs)
	for _, id := range remoteIDs {
		remote := remoteNotes[id]
		if remote.Rev <= state.Notes[id] {
			continue
		}
		path := NormalizeNotePath(remote.Path)
		if path == "" {
			continue
		}
		localNote := result.Notes[path]
		localNote.ID = id
		localNote.Path = path
		localNote.BaseRev = state.Notes[id]
		if localNote.Dirty {
			continue
		}
		apply := ApplyRemoteNote(localNote, remote, deviceID, now)
		if apply.Deferred {
			continue
		}
		if apply.ConflictCopy != nil {
			result.ConflictCopy = append(result.ConflictCopy, *apply.ConflictCopy)
			result.Changed = true
		}
		if apply.Deleted {
			delete(result.Notes, path)
			result.Deletes = append(result.Deletes, path)
			state.Notes[id] = remote.Rev
			delete(state.NoteHashes, id)
			result.Changed = true
			continue
		}
		if apply.Applied {
			result.Notes[path] = apply.Note
			result.Upserts = append(result.Upserts, apply.Note)
			state.Notes[id] = remote.Rev
			state.NoteHashes[id] = NoteContentHash(apply.Note.Text)
			result.Changed = true
		}
	}
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	result.State = state
	return result, nil
}

func pullRemoteNotes(ctx context.Context, provider Provider, workspaceID string) (map[string]NoteRecord, error) {
	if p, ok := provider.(NotePullProvider); ok {
		return p.PullNotes(ctx, workspaceID)
	}
	snapshot, err := provider.PullSnapshot(ctx, workspaceID)
	return snapshot.Notes, err
}

func NoteContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (s *NoteSyncer) SaveState(state State) error {
	return SaveState(s.StatePath, state)
}

func (s *NoteSyncer) deviceID(state State) string {
	if strings.TrimSpace(s.DeviceID) != "" {
		return s.DeviceID
	}
	return state.DeviceID
}

func (s *NoteSyncer) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
