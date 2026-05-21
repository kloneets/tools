package sync

import (
	"context"
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
	provider := fakeNoteProvider{snapshot: Snapshot{Notes: map[string]NoteRecord{
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
	provider := fakeNoteProvider{snapshot: Snapshot{Notes: map[string]NoteRecord{
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

type fakeNoteProvider struct {
	snapshot Snapshot
}

func (p fakeNoteProvider) Login(context.Context, string, string) (Session, error) {
	return Session{}, nil
}

func (p fakeNoteProvider) WatchWorkspace(context.Context, string, string, func(Change) error) error {
	return nil
}

func (p fakeNoteProvider) PushMutation(context.Context, string, Mutation) error {
	return nil
}

func (p fakeNoteProvider) PullSnapshot(context.Context, string) (Snapshot, error) {
	return p.snapshot, nil
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
