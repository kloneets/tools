package sync

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNoteIDStableForNormalizedPath(t *testing.T) {
	left := NoteID("Work\\Plan.md")
	right := NoteID("Work/./Plan.md")
	if left != right {
		t.Fatalf("NoteID normalized mismatch: %q != %q", left, right)
	}
	if len(left) != 64 {
		t.Fatalf("NoteID length = %d, want sha256 hex", len(left))
	}
}

func TestNoteSyncerPullAppliesCleanRemoteNote(t *testing.T) {
	id := NoteID("Work/Plan.md")
	provider := &fakeNoteProvider{snapshot: Snapshot{Notes: map[string]NoteRecord{
		id: {ID: id, Path: "Work/Plan.md", Text: "remote", Rev: 2},
	}}}
	syncer := NoteSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{IDToken: "token"},
		Now:         func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) },
	}

	got, err := syncer.PullNotes(context.Background(), map[string]LocalNote{
		"Work/Plan.md": {ID: id, Path: "Work/Plan.md", Text: "local"},
	})
	if err != nil {
		t.Fatalf("PullNotes() error = %v", err)
	}
	if len(got.Upserts) != 1 || got.Upserts[0].Text != "remote" {
		t.Fatalf("Upserts = %#v, want remote note", got.Upserts)
	}
	if got.State.Notes[id] != 2 {
		t.Fatalf("state note rev = %d, want 2", got.State.Notes[id])
	}
}

func TestNoteSyncerPullDefersDirtyRemoteNote(t *testing.T) {
	id := NoteID("Work/Plan.md")
	provider := &fakeNoteProvider{snapshot: Snapshot{Notes: map[string]NoteRecord{
		id: {ID: id, Path: "Work/Plan.md", Text: "remote", Rev: 2},
	}}}
	syncer := NoteSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{IDToken: "token"},
	}

	got, err := syncer.PullNotes(context.Background(), map[string]LocalNote{
		"Work/Plan.md": {ID: id, Path: "Work/Plan.md", Text: "dirty local", Dirty: true},
	})
	if err != nil {
		t.Fatalf("PullNotes() error = %v", err)
	}
	if len(got.Upserts) != 0 || len(got.ConflictCopy) != 0 || got.Changed {
		t.Fatalf("result = %#v, want deferred dirty note without changes", got)
	}
}

func TestNoteSyncerPushSkipsUnchangedNoteContent(t *testing.T) {
	provider := &fakeNoteProvider{}
	syncer := NoteSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{IDToken: "token"},
		Now:         func() time.Time { return time.Unix(10, 0).UTC() },
	}
	files := []NoteFile{{Path: "Work/Plan.md", Text: "same"}}

	first, err := syncer.PushNotes(context.Background(), files)
	if err != nil {
		t.Fatalf("first PushNotes() error = %v", err)
	}
	second, err := syncer.PushNotes(context.Background(), files)
	if err != nil {
		t.Fatalf("second PushNotes() error = %v", err)
	}

	if first.Pushed != 1 || second.Pushed != 0 {
		t.Fatalf("push results = %#v then %#v, want one push then no-op", first, second)
	}
	if len(provider.mutations) != 1 {
		t.Fatalf("mutations len = %d, want unchanged note skipped", len(provider.mutations))
	}
	if provider.hashPushes != 0 {
		t.Fatalf("note hash pushes = %d, want none", provider.hashPushes)
	}
}

func TestNoteSyncerPullRecordsAppliedNoteHash(t *testing.T) {
	id := NoteID("Work/Plan.md")
	provider := &fakeNoteProvider{snapshot: Snapshot{Notes: map[string]NoteRecord{
		id: {ID: id, Path: "Work/Plan.md", Text: "remote", Rev: 2},
	}}}
	syncer := NoteSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{IDToken: "token"},
	}

	got, err := syncer.PullNotes(context.Background(), nil)
	if err != nil {
		t.Fatalf("PullNotes() error = %v", err)
	}

	if got.State.NoteHashes[id] != NoteContentHash("remote") {
		t.Fatalf("note hash = %q, want remote content hash", got.State.NoteHashes[id])
	}
}

func TestNoteSyncerPullIgnoresLegacyMatchingHashAcrossDirtyDeferral(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	id := NoteID("Work/Plan.md")
	remote := map[string]NoteRecord{id: {ID: id, Path: "Work/Plan.md", Text: "remote", Rev: 2}}
	hash := NoteMetadataHash(remote)
	provider := &fakeNoteProvider{
		snapshot: Snapshot{Notes: remote},
		hashes:   map[string]SyncHashRecord{SyncFeatureNotes: {Hash: hash}},
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := defaultState()
	state.WorkspaceID = "ws"
	markFeaturePulled(&state, SyncFeatureNotes, hash, now)
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	syncer := NoteSyncer{Provider: provider, WorkspaceID: "ws", StatePath: statePath, Session: Session{IDToken: "token"}, Now: func() time.Time { return now }}
	dirty := map[string]LocalNote{"Work/Plan.md": {ID: id, Path: "Work/Plan.md", Text: "dirty local", Dirty: true}}

	deferred, err := syncer.PullNotes(context.Background(), dirty)
	if err != nil {
		t.Fatalf("PullNotes() error = %v", err)
	}
	if deferred.Changed || deferred.State.Notes[id] != 0 || provider.notePulls != 1 {
		t.Fatalf("deferred result = %#v, pulls = %d; want fetched and deferred", deferred, provider.notePulls)
	}
	if err := syncer.SaveState(deferred.State); err != nil {
		t.Fatalf("SaveState() after deferred pull error = %v", err)
	}

	applied, err := syncer.PullNotes(context.Background(), map[string]LocalNote{
		"Work/Plan.md": {ID: id, Path: "Work/Plan.md", Text: "clean local"},
	})
	if err != nil {
		t.Fatalf("second PullNotes() error = %v", err)
	}
	if !applied.Changed || len(applied.Upserts) != 1 || applied.Upserts[0].Text != "remote" || provider.notePulls != 2 {
		t.Fatalf("second result = %#v, pulls = %d; want fetched remote applied", applied, provider.notePulls)
	}
	if provider.hashPulls != 0 || provider.hashPushes != 0 {
		t.Fatalf("note hash calls = pulls %d pushes %d, want none", provider.hashPulls, provider.hashPushes)
	}
}

type fakeNoteProvider struct {
	snapshot   Snapshot
	mutations  []Mutation
	hashes     map[string]SyncHashRecord
	notePulls  int
	hashPulls  int
	hashPushes int
}

func (p fakeNoteProvider) Login(context.Context, string, string) (Session, error) {
	return Session{}, nil
}

func (p fakeNoteProvider) WatchWorkspace(context.Context, string, string, func(Change) error) error {
	return nil
}

func (p *fakeNoteProvider) PushMutation(_ context.Context, _ string, mutation Mutation) error {
	p.mutations = append(p.mutations, mutation)
	return nil
}

func (p *fakeNoteProvider) PullSnapshot(context.Context, string) (Snapshot, error) {
	p.notePulls++
	return p.snapshot, nil
}

func (p *fakeNoteProvider) PullSyncHashes(context.Context, string) (map[string]SyncHashRecord, error) {
	p.hashPulls++
	return p.hashes, nil
}

func (p *fakeNoteProvider) PushSyncHash(_ context.Context, _ string, feature string, record SyncHashRecord) error {
	p.hashPushes++
	if p.hashes == nil {
		p.hashes = map[string]SyncHashRecord{}
	}
	p.hashes[feature] = record
	return nil
}

func (p fakeNoteProvider) CreateWorkspace(context.Context, string) (WorkspaceMeta, error) {
	return WorkspaceMeta{}, nil
}

func (p fakeNoteProvider) GrantMember(context.Context, string, string, string) error {
	return nil
}

func (p fakeNoteProvider) RevokeMember(context.Context, string, string) error {
	return nil
}
