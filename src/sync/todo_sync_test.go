package sync

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kloneets/tools/src/todo"
)

func TestTodoSyncerPushStoreWritesArchivedTombstoneWithoutBody(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	archivedAt := now
	provider := &fakeTodoProvider{}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   filepath.Join(t.TempDir(), "state.json"),
		Session:     Session{IDToken: "token", UID: "uid"},
	}

	err := syncer.PushStore(context.Background(), todo.Store{Items: []todo.Item{
		{ID: "active", Text: "active", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now},
		{ID: "old", Text: "old", Status: todo.StatusArchived, CreatedAt: now, UpdatedAt: now, ArchivedAt: &archivedAt},
	}})

	if err != nil {
		t.Fatalf("PushStore() error = %v", err)
	}
	if len(provider.pushed) != 2 {
		t.Fatalf("pushed mutations = %#v, want active todo plus archived tombstone", provider.pushed)
	}
	if provider.pushed[0].Todo.Item.ID != "active" || provider.pushed[0].Todo.Deleted {
		t.Fatalf("first mutation = %#v, want active upsert", provider.pushed[0])
	}
	tombstone := provider.pushed[1].Todo
	if tombstone.Item.ID != "old" || !tombstone.Deleted || tombstone.Item.Text != "" {
		t.Fatalf("archived mutation = %#v, want bodyless delete tombstone", tombstone)
	}
}

func TestTodoSyncerPushStoreIgnoresArchiveMonthWriteFailure(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	provider := &fakeTodoProvider{archiveMonthPushErr: errors.New("permission denied")}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   filepath.Join(t.TempDir(), "state.json"),
		Session:     Session{IDToken: "token", UID: "uid"},
	}

	err := syncer.PushStore(context.Background(), todo.Store{Items: []todo.Item{
		{ID: "active", Text: "active", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now},
	}})

	if err != nil {
		t.Fatalf("PushStore() error = %v, want nil", err)
	}
	if len(provider.pushed) != 1 || provider.pushed[0].Todo.Item.ID != "active" {
		t.Fatalf("pushed mutations = %#v, want active todo", provider.pushed)
	}
}

func TestTodoSyncerPullStoreIgnoresRemoteArchivedAndPreservesLocalArchive(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	archivedAt := now
	provider := &fakeTodoProvider{
		todos: map[string]TodoRecord{
			"remote-active": {Item: todo.Item{ID: "remote-active", Text: "remote", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now}},
			"remote-old":    {Item: todo.Item{ID: "remote-old", Text: "old", Status: todo.StatusArchived, CreatedAt: now, UpdatedAt: now, ArchivedAt: &archivedAt}},
		},
		archiveMonthList: []string{"2026-05"},
	}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		Session:     Session{IDToken: "token", UID: "uid"},
	}
	local := todo.Store{Items: []todo.Item{
		{ID: "local-old", Text: "local old", Status: todo.StatusArchived, CreatedAt: now, UpdatedAt: now, ArchivedAt: &archivedAt},
	}}

	got, changed, err := syncer.PullStore(context.Background(), local)

	if err != nil {
		t.Fatalf("PullStore() error = %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %#v, want remote active plus local archive", got.Items)
	}
	if months := todo.ArchiveMonths(got); len(months) != 1 || months[0] != "2026-05" {
		t.Fatalf("ArchiveMonths() = %#v, want remote archive title", months)
	}
	for _, item := range got.Items {
		if item.ID == "remote-old" {
			t.Fatalf("remote archived item was auto-pulled: %#v", got.Items)
		}
	}
}

func TestTodoSyncerPullArchiveMonthMergesRequestedMonth(t *testing.T) {
	may := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	april := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	provider := &fakeTodoProvider{
		archiveMonths: map[string]map[string]TodoRecord{
			"2026-05": {
				"may": {Item: todo.Item{ID: "may", Text: "may", Status: todo.StatusArchived, CreatedAt: may, UpdatedAt: may, ArchivedAt: &may}},
			},
		},
	}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		Session:     Session{IDToken: "token", UID: "uid"},
	}
	local := todo.Store{Items: []todo.Item{
		{ID: "active", Text: "active", Status: todo.StatusTodo, CreatedAt: may, UpdatedAt: may},
		{ID: "april", Text: "april", Status: todo.StatusArchived, CreatedAt: april, UpdatedAt: april, ArchivedAt: &april},
	}}

	got, changed, err := syncer.PullArchiveMonth(context.Background(), local, "2026-05")

	if err != nil {
		t.Fatalf("PullArchiveMonth() error = %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if len(todo.ArchiveGroups(got)["2026-05"]) != 1 || todo.ArchiveGroups(got)["2026-05"][0].ID != "may" {
		t.Fatalf("May archive = %#v, want pulled may item", todo.ArchiveGroups(got)["2026-05"])
	}
	if len(todo.ArchiveGroups(got)["2026-04"]) != 1 {
		t.Fatalf("April archive not preserved: %#v", got.Items)
	}
}

func TestTodoSyncerPullStoreSkipsFreshMatchingHashes(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	local := todo.Store{Items: []todo.Item{{ID: "local", Text: "local", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now}}, ArchiveMonths: []string{"2026-05"}}
	hashes := map[string]SyncHashRecord{
		SyncFeatureTodos:             {Hash: TodoStoreHash(local)},
		SyncFeatureTodoArchiveMonths: {Hash: TodoArchiveMonthsHash(local.ArchiveMonths)},
	}
	provider := &fakeTodoProvider{hashes: hashes}
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := defaultState()
	state.WorkspaceID = "ws"
	markFeaturePulled(&state, SyncFeatureTodos, hashes[SyncFeatureTodos].Hash, time.Now().UTC())
	markFeaturePulled(&state, SyncFeatureTodoArchiveMonths, hashes[SyncFeatureTodoArchiveMonths].Hash, time.Now().UTC())
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	syncer := &TodoSyncer{Provider: provider, WorkspaceID: "ws", StatePath: statePath, Session: Session{IDToken: "token", UID: "uid"}}

	got, changed, err := syncer.PullStore(context.Background(), local)

	if err != nil {
		t.Fatalf("PullStore() error = %v", err)
	}
	if changed || !sameTodoStore(got, local) {
		t.Fatalf("PullStore() = %#v, %v; want unchanged local", got, changed)
	}
	if provider.todoPulls != 0 || provider.archiveMonthListPulls != 0 {
		t.Fatalf("pull counts = todos %d months %d, want skipped", provider.todoPulls, provider.archiveMonthListPulls)
	}
}

func TestTodoSyncerPullStoreMissingHashFallsBackAndRecordsHash(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	provider := &fakeTodoProvider{
		hashes: map[string]SyncHashRecord{},
		todos: map[string]TodoRecord{
			"remote": {Item: todo.Item{ID: "remote", Text: "remote", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now}},
		},
		archiveMonthList: []string{"2026-05"},
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	syncer := &TodoSyncer{Provider: provider, WorkspaceID: "ws", StatePath: statePath, Session: Session{IDToken: "token", UID: "uid"}}

	_, changed, err := syncer.PullStore(context.Background(), todo.Store{})

	if err != nil {
		t.Fatalf("PullStore() error = %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want fallback pull to apply remote")
	}
	state, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.SyncHashes[SyncFeatureTodos].Hash == "" || state.SyncHashes[SyncFeatureTodoArchiveMonths].Hash == "" {
		t.Fatalf("sync hashes not recorded: %#v", state.SyncHashes)
	}
	if provider.hashes[SyncFeatureTodos].Hash == "" || provider.hashes[SyncFeatureTodoArchiveMonths].Hash == "" {
		t.Fatalf("remote hashes not populated: %#v", provider.hashes)
	}
}

type fakeTodoProvider struct {
	todos                 map[string]TodoRecord
	archiveMonths         map[string]map[string]TodoRecord
	archiveMonthList      []string
	pushed                []Mutation
	hashes                map[string]SyncHashRecord
	todoPulls             int
	archiveMonthListPulls int
	archiveMonthPushErr   error
}

func (p *fakeTodoProvider) Login(context.Context, string, string) (Session, error) {
	return Session{}, nil
}

func (p *fakeTodoProvider) WatchWorkspace(context.Context, string, string, func(Change) error) error {
	return nil
}

func (p *fakeTodoProvider) PushMutation(_ context.Context, _ string, mutation Mutation) error {
	p.pushed = append(p.pushed, mutation)
	return nil
}

func (p *fakeTodoProvider) PullSnapshot(context.Context, string) (Snapshot, error) {
	return Snapshot{Todos: p.todos}, nil
}

func (p *fakeTodoProvider) PullTodos(context.Context, string) (map[string]TodoRecord, error) {
	p.todoPulls++
	return p.todos, nil
}

func (p *fakeTodoProvider) PullTodoArchiveMonth(_ context.Context, _ string, month string) (map[string]TodoRecord, error) {
	return p.archiveMonths[month], nil
}

func (p *fakeTodoProvider) PullTodoArchiveMonths(context.Context, string) ([]string, error) {
	p.archiveMonthListPulls++
	return p.archiveMonthList, nil
}

func (p *fakeTodoProvider) PushTodoArchiveMonths(_ context.Context, _ string, months []string) error {
	if p.archiveMonthPushErr != nil {
		return p.archiveMonthPushErr
	}
	p.archiveMonthList = months
	return nil
}

func (p *fakeTodoProvider) PullSyncHashes(context.Context, string) (map[string]SyncHashRecord, error) {
	return p.hashes, nil
}

func (p *fakeTodoProvider) PushSyncHash(_ context.Context, _ string, feature string, record SyncHashRecord) error {
	if p.hashes == nil {
		p.hashes = map[string]SyncHashRecord{}
	}
	p.hashes[feature] = record
	return nil
}

func (p *fakeTodoProvider) CreateWorkspace(context.Context, string) (WorkspaceMeta, error) {
	return WorkspaceMeta{}, nil
}

func (p *fakeTodoProvider) GrantMember(context.Context, string, string, string) error {
	return nil
}

func (p *fakeTodoProvider) RevokeMember(context.Context, string, string) error {
	return nil
}
