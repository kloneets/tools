package todo

import (
	"fmt"
	"path/filepath"
	"strings"
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

func TestArchiveMonthsIncludesCachedMonthTitlesWithoutItems(t *testing.T) {
	store := Store{ArchiveMonths: []string{"2026-05"}}

	months := ArchiveMonths(store)

	if len(months) != 1 || months[0] != "2026-05" {
		t.Fatalf("ArchiveMonths() = %#v, want cached title", months)
	}
}

func TestNonArchivedStoreFiltersArchivedItems(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{
		newItem("active", "active", now),
		archivedItem("old", "old", now),
	}}

	got := NonArchivedStore(store)

	if len(got.Items) != 1 || got.Items[0].ID != "active" {
		t.Fatalf("NonArchivedStore() = %#v, want only active item", got.Items)
	}
}

func TestMergeArchiveMonthReplacesOnlyRequestedMonth(t *testing.T) {
	may := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	april := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{
		newItem("active", "active", may),
		archivedItem("old-may", "old may", may),
		archivedItem("old-april", "old april", april),
	}}

	got := MergeArchiveMonth(store, "2026-05", []Item{archivedItem("new-may", "new may", may)})

	if len(got.Items) != 3 {
		t.Fatalf("items = %#v, want 3", got.Items)
	}
	if len(ArchiveGroups(got)["2026-05"]) != 1 || ArchiveGroups(got)["2026-05"][0].ID != "new-may" {
		t.Fatalf("May archive = %#v, want new-may only", ArchiveGroups(got)["2026-05"])
	}
	if len(ArchiveGroups(got)["2026-04"]) != 1 || ArchiveGroups(got)["2026-04"][0].ID != "old-april" {
		t.Fatalf("April archive = %#v, want old-april preserved", ArchiveGroups(got)["2026-04"])
	}
}

func TestMergeArchiveMonthKeepsLocalCacheWhenRemoteMonthIsEmpty(t *testing.T) {
	may := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{
		newItem("active", "active", may),
		archivedItem("old-may", "old may", may),
	}}

	got := MergeArchiveMonth(store, "2026-05", nil)

	if len(ArchiveGroups(got)["2026-05"]) != 1 || ArchiveGroups(got)["2026-05"][0].ID != "old-may" {
		t.Fatalf("May archive = %#v, want local cache preserved", ArchiveGroups(got)["2026-05"])
	}
	if months := ArchiveMonths(got); len(months) != 1 || months[0] != "2026-05" {
		t.Fatalf("ArchiveMonths() = %#v, want 2026-05 title preserved", months)
	}
}

func TestMergeArchiveMonthRemembersTitleWhenRemoteMonthIsEmpty(t *testing.T) {
	store := Store{}

	got := MergeArchiveMonth(store, "2026-05", nil)

	if months := ArchiveMonths(got); len(months) != 1 || months[0] != "2026-05" {
		t.Fatalf("ArchiveMonths() = %#v, want 2026-05 title", months)
	}
}

func TestPreserveArchivedKeepsLocalArchiveCacheAfterSync(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	local := Store{Items: []Item{
		newItem("local-active", "local active", now),
		archivedItem("old", "old", now),
	}}
	synced := Store{Items: []Item{newItem("remote-active", "remote active", now)}}

	got := PreserveArchived(local, synced)

	if len(got.Items) != 2 {
		t.Fatalf("items = %#v, want synced active plus local archive", got.Items)
	}
	if len(ArchiveGroups(got)["2026-05"]) != 1 {
		t.Fatalf("archive cache missing: %#v", got.Items)
	}
}

func TestPreserveArchivedDropsArchiveCacheWhenSyncedItemHasSameID(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	local := Store{Items: []Item{archivedItem("same", "old", now)}}
	synced := Store{Items: []Item{newItem("same", "active", now)}}

	got := PreserveArchived(local, synced)

	if len(got.Items) != 1 || got.Items[0].Status == StatusArchived {
		t.Fatalf("items = %#v, want synced non-archived item only", got.Items)
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

func TestNormalizeDefaultsInvalidTermToShort(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{
		{ID: "blank", Text: "blank", Status: StatusTodo, CreatedAt: now, UpdatedAt: now},
		{ID: "bad", Text: "bad", Status: StatusTodo, Term: "later", CreatedAt: now, UpdatedAt: now},
		{ID: "long", Text: "long", Status: StatusTodo, Term: TermLong, CreatedAt: now, UpdatedAt: now},
	}}

	Normalize(&store)

	if store.Items[0].Term != TermShort || store.Items[1].Term != TermShort || store.Items[2].Term != TermLong {
		t.Fatalf("terms = %#v, want short, short, long", []string{store.Items[0].Term, store.Items[1].Term, store.Items[2].Term})
	}
}

func TestShortAndLongItemsSplitActiveTodos(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	short := newItem("short", "short", now)
	long := newItem("long", "long", now)
	long.Term = TermLong
	done := newItem("done", "done", now)
	done.Status = StatusDone
	store := Store{Items: []Item{long, done, short}}

	if got := ShortItems(store); len(got) != 1 || got[0].ID != "short" {
		t.Fatalf("ShortItems() = %#v, want short", got)
	}
	if got := LongItems(store); len(got) != 1 || got[0].ID != "long" {
		t.Fatalf("LongItems() = %#v, want long", got)
	}
	if got := ActiveItems(store); len(got) != 2 || got[0].ID != "short" || got[1].ID != "long" {
		t.Fatalf("ActiveItems() = %#v, want short before long", got)
	}
}

func TestMoveActiveStaysWithinTerm(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	shortA := newItem("short-a", "short a", now)
	shortB := newItem("short-b", "short b", now)
	longA := newItem("long-a", "long a", now)
	longB := newItem("long-b", "long b", now)
	longA.Term = TermLong
	longB.Term = TermLong
	shortA.Order = 0
	shortB.Order = 1
	longA.Order = 0
	longB.Order = 1
	store := Store{Items: []Item{shortA, longA, shortB, longB}}

	if !MoveActive(&store, "long-b", -1, now.Add(time.Hour)) {
		t.Fatal("MoveActive() = false, want true")
	}

	if got := ActiveItems(store); strings.Join(itemIDs(got), ",") != "short-a,short-b,long-b,long-a" {
		t.Fatalf("ActiveItems() = %#v, want short section unchanged and long reordered", itemIDs(got))
	}
}

func TestMoveActiveToOtherTermMovesUncheckedTodo(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := Store{Items: []Item{newItem("a", "a", now)}}

	if !MoveActiveToOtherTerm(&store, "a", now.Add(time.Hour)) {
		t.Fatal("MoveActiveToOtherTerm() = false, want true")
	}

	if store.Items[0].Term != TermLong {
		t.Fatalf("term = %q, want long", store.Items[0].Term)
	}
	if !MoveActiveToOtherTerm(&store, "a", now.Add(2*time.Hour)) {
		t.Fatal("MoveActiveToOtherTerm() back = false, want true")
	}
	if store.Items[0].Term != TermShort {
		t.Fatalf("term = %q, want short", store.Items[0].Term)
	}
}

func TestMoveActiveReordersDuplicateOrderItemsPastTen(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := Store{}
	for i := 1; i <= 12; i++ {
		item := newItem(fmt.Sprintf("%02d", i), fmt.Sprintf("item %02d", i), now.Add(time.Duration(i)*time.Minute))
		item.Order = 0
		store.Items = append(store.Items, item)
	}

	if !MoveActive(&store, "11", -1, now.Add(time.Hour)) {
		t.Fatal("MoveActive() = false, want true")
	}

	active := ActiveItems(store)
	got := make([]string, 0, len(active))
	orders := make([]int, 0, len(active))
	for _, item := range active {
		got = append(got, item.ID)
		orders = append(orders, item.Order)
	}
	want := []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "11", "10", "12"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("active IDs = %#v, want %#v", got, want)
	}
	for i, order := range orders {
		if order != i {
			t.Fatalf("orders = %#v, want normalized numeric order at index %d", orders, i)
		}
	}
}

func newItem(id string, text string, now time.Time) Item {
	return Item{ID: id, Text: text, Status: StatusTodo, CreatedAt: now, UpdatedAt: now}
}

func itemIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
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
