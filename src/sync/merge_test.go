package sync

import (
	"strings"
	"testing"
	"time"

	"github.com/kloneets/tools/src/todo"
)

func TestApplyRemoteNoteCreatesConflictCopyWhenLocalAndRemoteChanged(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC)
	local := LocalNote{ID: "note-1", Path: "Project/Plan.md", Text: "local edit", BaseRev: 1, Dirty: true}
	remote := NoteRecord{ID: "note-1", Path: "Project/Plan.md", Text: "remote edit", Rev: 2}

	got := ApplyRemoteNote(local, remote, "Laptop A", now)

	if got.ConflictCopy == nil {
		t.Fatal("ConflictCopy = nil, want local conflict copy")
	}
	if got.Note.Text != "remote edit" || got.Note.BaseRev != 2 {
		t.Fatalf("Note = %#v, want remote canonical note at rev 2", got.Note)
	}
	if got.ConflictCopy.Text != "local edit" {
		t.Fatalf("ConflictCopy.Text = %q, want local edit", got.ConflictCopy.Text)
	}
	if !strings.Contains(got.ConflictCopy.Path, "Plan.conflict-laptop-a-20260520-123000.md") {
		t.Fatalf("ConflictCopy.Path = %q, want timestamped conflict copy", got.ConflictCopy.Path)
	}
}

func TestApplyRemoteNoteUpdatesCleanNoteInPlace(t *testing.T) {
	local := LocalNote{ID: "n", Path: "a.md", Text: "old", BaseRev: 1}
	remote := NoteRecord{ID: "n", Path: "a.md", Text: "new", Rev: 2}

	got := ApplyRemoteNote(local, remote, "device", time.Time{})

	if !got.Applied || got.Deferred || got.ConflictCopy != nil {
		t.Fatalf("result = %#v, want clean apply without conflict", got)
	}
	if got.Note.Text != "new" || got.Note.BaseRev != 2 {
		t.Fatalf("Note = %#v, want remote text and rev", got.Note)
	}
}

func TestMergeTodosPreservesUnrelatedItems(t *testing.T) {
	base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	local := todo.Store{Items: []todo.Item{
		{ID: "a", Text: "local only", Status: todo.StatusTodo, Order: 0, CreatedAt: base, UpdatedAt: base},
		{ID: "b", Text: "old", Status: todo.StatusTodo, Order: 1, CreatedAt: base, UpdatedAt: base},
	}}
	remote := map[string]TodoRecord{
		"b": {Item: todo.Item{ID: "b", Text: "remote", Status: todo.StatusTodo, Order: 1, CreatedAt: base, UpdatedAt: base.Add(time.Minute)}, Rev: 2},
	}

	got := MergeTodos(local, remote)

	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(got.Items))
	}
	if got.Items[0].ID != "a" || got.Items[0].Text != "local only" {
		t.Fatalf("first item = %#v, want unrelated local item preserved", got.Items[0])
	}
	if got.Items[1].ID != "b" || got.Items[1].Text != "remote" {
		t.Fatalf("second item = %#v, want remote update for item b", got.Items[1])
	}
}

func TestMergeTodosTombstonePreventsResurrection(t *testing.T) {
	base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	local := todo.Store{Items: []todo.Item{
		{ID: "deleted", Text: "should go away", Status: todo.StatusTodo, CreatedAt: base, UpdatedAt: base},
	}}

	got := MergeTodos(local, map[string]TodoRecord{"deleted": {Deleted: true, Rev: 3}})

	if len(got.Items) != 0 {
		t.Fatalf("items = %#v, want tombstoned item removed", got.Items)
	}
}

func TestSharedWorkspaceSettingsKeepsUILocal(t *testing.T) {
	got := SharedWorkspaceSettings(map[string]any{
		"pages_app":    map[string]any{"first_book": 10},
		"password_app": map[string]any{"symbol_count": 20},
		"notes_app": map[string]any{
			"preview_hidden":      true,
			"current_note_path":   "private.md",
			"open_note_paths":     []any{"private.md"},
			"spell_check_enabled": true,
		},
		"ui": map[string]any{"theme": "gruvbox"},
	})

	if _, ok := got["pages_app"]; !ok {
		t.Fatal("pages_app missing from shared settings")
	}
	if _, ok := got["password_app"]; !ok {
		t.Fatal("password_app missing from shared settings")
	}
	notes, ok := got["notes_app"].(map[string]any)
	if !ok {
		t.Fatal("shared notes_app missing")
	}
	if _, ok := notes["preview_hidden"]; ok {
		t.Fatal("preview_hidden should stay local")
	}
	if _, ok := notes["current_note_path"]; ok {
		t.Fatal("current_note_path should stay local")
	}
	if _, ok := got["ui"]; ok {
		t.Fatal("ui should stay local")
	}
}

func TestApplySharedWorkspaceSettingsPreservesNotesLocalOnlyState(t *testing.T) {
	got := ApplySharedWorkspaceSettings(
		map[string]any{
			"pages_app": map[string]any{"first_book": 1},
			"notes_app": map[string]any{
				"current_note_path": "local.md",
				"preview_hidden":    false,
			},
			"firebase": map[string]any{"workspace_id": "user_1"},
		},
		map[string]any{
			"pages_app": map[string]any{"first_book": 2},
			"notes_app": map[string]any{
				"preview_hidden":      true,
				"spell_check_enabled": true,
			},
		},
	)

	notes := got["notes_app"].(map[string]any)
	if notes["current_note_path"] != "local.md" {
		t.Fatalf("current_note_path = %v, want preserved local path", notes["current_note_path"])
	}
	if notes["preview_hidden"] != false {
		t.Fatalf("preview_hidden = %v, want preserved local rendering mode", notes["preview_hidden"])
	}
	if notes["spell_check_enabled"] != true {
		t.Fatalf("spell_check_enabled = %v, want remote shared setting applied", notes["spell_check_enabled"])
	}
	if _, ok := got["firebase"]; !ok {
		t.Fatal("firebase config should stay local")
	}
}
