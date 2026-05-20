package todo

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCheckedTodoStaysActiveBeforeDelay(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{newItem("1", "alpha", now)}}
	store.Items[0].CheckedAt = ptr(now)

	Cleanup(&store, now.Add(9*time.Second))

	if got := store.Items[0].Status; got != StatusTodo {
		t.Fatalf("status = %q, want todo", got)
	}
	if len(ActiveItems(store)) != 1 {
		t.Fatal("checked pending todo should remain in active list")
	}
}

func TestCheckedTodoPromotesToDoneAfterDelay(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{newItem("1", "alpha", now)}}
	store.Items[0].CheckedAt = ptr(now)

	Cleanup(&store, now.Add(10*time.Second))

	if got := store.Items[0].Status; got != StatusDone {
		t.Fatalf("status = %q, want done", got)
	}
	if store.Items[0].DoneAt == nil {
		t.Fatal("DoneAt = nil, want timestamp")
	}
}

func TestRepositoryToggleUnchecksDoneToActive(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json"))
	repo.SetNowForTests(func() time.Time { return now })
	store := Store{Items: []Item{newItem("1", "alpha", now)}}
	store.Items[0].Status = StatusDone
	store.Items[0].CheckedAt = ptr(now)
	store.Items[0].DoneAt = ptr(now)
	if err := repo.Save(store); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Toggle("1")
	if err != nil {
		t.Fatal(err)
	}

	item := got.Items[0]
	if item.Status != StatusTodo || item.CheckedAt != nil || item.DoneAt != nil || item.ArchivedAt != nil {
		t.Fatalf("item after toggle = %#v, want active unchecked todo", item)
	}
}

func TestDoneArchivesAfterSevenDays(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	doneAt := now.Add(-7 * 24 * time.Hour)
	store := Store{Items: []Item{newItem("1", "alpha", now)}}
	store.Items[0].Status = StatusDone
	store.Items[0].DoneAt = &doneAt

	Cleanup(&store, now)

	if got := store.Items[0].Status; got != StatusArchived {
		t.Fatalf("status = %q, want archived", got)
	}
	if store.Items[0].ArchivedAt == nil {
		t.Fatal("ArchivedAt = nil, want timestamp")
	}
}

func TestArchiveGroupsByMonth(t *testing.T) {
	may := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	april := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{
		archivedItem("1", "may", may),
		archivedItem("2", "april", april),
	}}

	months := ArchiveMonths(store)

	if len(months) != 2 || months[0] != "2026-05" || months[1] != "2026-04" {
		t.Fatalf("ArchiveMonths() = %#v", months)
	}
	if len(ArchiveGroups(store)["2026-05"]) != 1 {
		t.Fatal("May archive group missing item")
	}
}

func TestMoveActiveOnlyAffectsUncheckedActiveTodos(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	checked := newItem("2", "checked", now)
	checked.CheckedAt = ptr(now)
	done := newItem("4", "done", now)
	done.Status = StatusDone
	store := Store{Items: []Item{
		newItem("1", "a", now),
		checked,
		newItem("3", "b", now),
		done,
	}}
	store.Items[0].Order = 0
	store.Items[1].Order = 1
	store.Items[2].Order = 2

	if !MoveActive(&store, "3", -1, now) {
		t.Fatal("MoveActive() = false, want true")
	}

	active := ActiveItems(store)
	if active[0].ID != "3" || active[1].ID != "1" || active[2].ID != "2" {
		t.Fatalf("active order = %#v", []string{active[0].ID, active[1].ID, active[2].ID})
	}
	if MoveActive(&store, "2", -1, now) {
		t.Fatal("checked pending item should not reorder")
	}
	if MoveActive(&store, "4", -1, now) {
		t.Fatal("done item should not reorder")
	}
}

func newItem(id string, text string, now time.Time) Item {
	return Item{ID: id, Text: text, Status: StatusTodo, CreatedAt: now, UpdatedAt: now}
}

func archivedItem(id string, text string, archivedAt time.Time) Item {
	item := newItem(id, text, archivedAt)
	item.Status = StatusArchived
	item.ArchivedAt = &archivedAt
	return item
}

func ptr(t time.Time) *time.Time {
	return &t
}
