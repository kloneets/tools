package sync

import (
	"context"
	"errors"
	"os"
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
	for _, mutation := range provider.pushed {
		if mutation.TodoListID != "" {
			t.Fatalf("default mutation TodoListID = %q, want empty legacy wire field", mutation.TodoListID)
		}
	}
	if provider.hashes[SyncFeatureTodos].Hash == "" || provider.hashes[SyncFeatureTodoArchiveMonths].Hash == "" {
		t.Fatalf("default sync hashes = %#v, want legacy todo feature keys", provider.hashes)
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

func TestTodoSyncerPushStoreScopesNamedListMutationsAndHashes(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	provider := &fakeTodoProvider{}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   filepath.Join(t.TempDir(), "state.json"),
		Session:     Session{IDToken: "token", UID: "uid"},
		ListID:      "list-1",
	}

	err := syncer.PushStore(context.Background(), todo.Store{Items: []todo.Item{
		{ID: "active", Text: "active", Status: todo.StatusTodo, CreatedAt: now, UpdatedAt: now},
	}})

	if err != nil {
		t.Fatalf("PushStore() error = %v", err)
	}
	if len(provider.pushed) != 1 || provider.pushed[0].TodoListID != "list-1" {
		t.Fatalf("pushed mutations = %#v, want named list id", provider.pushed)
	}
	if provider.hashes[todoFeature("list-1")].Hash == "" || provider.hashes[todoArchiveMonthsFeature("list-1")].Hash == "" {
		t.Fatalf("named sync hashes = %#v, want list-scoped feature keys", provider.hashes)
	}
	if provider.hashes[SyncFeatureTodos].Hash != "" || provider.hashes[SyncFeatureTodoArchiveMonths].Hash != "" {
		t.Fatalf("legacy hashes populated for named list: %#v", provider.hashes)
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

func TestTodoSyncerSyncRepositoryMergesListTombstoneAndRemovesNamedData(t *testing.T) {
	start := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	repo := todo.NewRepositoryAt(filepath.Join(dir, "todos.json"))
	repo.SetNowForTests(func() time.Time { return start })
	repo.SetListIDGeneratorForTests(func() (string, error) { return "list-1", nil })
	_, meta, err := repo.CreateList("Work")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Add("local item"); err != nil {
		t.Fatal(err)
	}
	deletedAt := start.Add(time.Minute)
	provider := &fakeTodoProvider{
		todoLists: map[string]todo.ListMeta{
			meta.ID: {
				ID:        meta.ID,
				Name:      meta.Name,
				Rev:       deletedAt.UnixMilli(),
				CreatedAt: meta.CreatedAt,
				UpdatedAt: deletedAt,
				Deleted:   true,
				DeletedAt: &deletedAt,
			},
		},
	}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   filepath.Join(t.TempDir(), "state.json"),
		Session:     Session{IDToken: "token", UID: "uid"},
	}

	catalog, err := syncer.SyncRepository(context.Background(), repo)

	if err != nil {
		t.Fatalf("SyncRepository() error = %v", err)
	}
	got, ok := todo.ListByID(catalog, meta.ID)
	if !ok || !got.Deleted {
		t.Fatalf("merged catalog = %#v, want deleted tombstone for named list", catalog)
	}
	if active := todo.ActiveLists(catalog); len(active) != 1 || active[0].ID != todo.DefaultListID {
		t.Fatalf("active lists = %#v, want default only", active)
	}
	if _, err := repo.SelectList(meta.ID); err == nil {
		t.Fatal("SelectList(deleted) error = nil, want tombstone inactive")
	}
	if _, err := os.Stat(filepath.Join(dir, todo.NamedListsDir, meta.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("named data stat err = %v, want removed", err)
	}
	if len(provider.deletedListData) == 0 || provider.deletedListData[0] != meta.ID {
		t.Fatalf("deleted remote list data = %#v, want %q", provider.deletedListData, meta.ID)
	}
}

func TestTodoSyncerPushListsSwallowsStaleMetadataWithoutMarkingHash(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	provider := &fakeTodoProvider{staleListIDs: map[string]bool{"work": true}}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   filepath.Join(t.TempDir(), "state.json"),
		Session:     Session{IDToken: "token", UID: "uid"},
	}
	catalog := todo.ListsStore{Lists: []todo.ListMeta{{
		ID:        "work",
		Name:      "Work",
		Rev:       10,
		CreatedAt: now,
		UpdatedAt: now,
	}}}
	todo.NormalizeLists(&catalog, now)

	pushed, err := syncer.PushLists(context.Background(), catalog)
	if err != nil {
		t.Fatalf("PushLists() error = %v, want stale metadata swallowed", err)
	}
	if pushed {
		t.Fatal("PushLists() pushed = true, want false after stale metadata")
	}
	if provider.hashes[SyncFeatureTodoLists].Hash != "" {
		t.Fatalf("todo list hash = %q, want not marked after stale metadata", provider.hashes[SyncFeatureTodoLists].Hash)
	}
}

func TestTodoSyncerSyncRepositoryRepullsCatalogAfterStaleListPush(t *testing.T) {
	start := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	repo := todo.NewRepositoryAt(filepath.Join(dir, "todos.json"))
	repo.SetNowForTests(func() time.Time { return start })
	repo.SetListIDGeneratorForTests(func() (string, error) { return "work", nil })
	_, meta, err := repo.CreateList("Work")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Add("local item"); err != nil {
		t.Fatal(err)
	}
	deletedAt := start.Add(time.Minute)
	provider := &fakeTodoProvider{
		todoListsPullSequence: []map[string]todo.ListMeta{
			{
				meta.ID: {
					ID:        meta.ID,
					Name:      meta.Name,
					Rev:       meta.Rev,
					CreatedAt: meta.CreatedAt,
					UpdatedAt: meta.UpdatedAt,
				},
			},
			{
				meta.ID: {
					ID:        meta.ID,
					Name:      meta.Name,
					Rev:       deletedAt.UnixMilli(),
					CreatedAt: meta.CreatedAt,
					UpdatedAt: deletedAt,
					Deleted:   true,
					DeletedAt: &deletedAt,
				},
			},
		},
		staleListIDs: map[string]bool{meta.ID: true},
	}
	syncer := &TodoSyncer{
		Provider:    provider,
		WorkspaceID: "ws",
		StatePath:   filepath.Join(t.TempDir(), "state.json"),
		Session:     Session{IDToken: "token", UID: "uid"},
	}

	catalog, err := syncer.SyncRepository(context.Background(), repo)

	if err != nil {
		t.Fatalf("SyncRepository() error = %v", err)
	}
	if active := todo.ActiveLists(catalog); len(active) != 1 || active[0].ID != todo.DefaultListID {
		t.Fatalf("active lists = %#v, want default only after stale push refresh", active)
	}
	if repo.CurrentListID() != todo.DefaultListID {
		t.Fatalf("current list = %q, want default after stale push refresh", repo.CurrentListID())
	}
	if _, err := os.Stat(filepath.Join(dir, todo.NamedListsDir, meta.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("named store stat err = %v, want removed after stale push refresh", err)
	}
	for _, mutation := range provider.pushed {
		if mutation.TodoListID == meta.ID {
			t.Fatalf("pushed mutation = %#v, want stale list payload skipped", mutation)
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
	listTodos             map[string]map[string]TodoRecord
	archiveMonths         map[string]map[string]TodoRecord
	listArchiveMonths     map[string]map[string]map[string]TodoRecord
	archiveMonthList      []string
	listArchiveMonthList  map[string][]string
	todoLists             map[string]todo.ListMeta
	todoListsPullSequence []map[string]todo.ListMeta
	pushed                []Mutation
	pushedLists           []todo.ListMeta
	deletedListData       []string
	hashes                map[string]SyncHashRecord
	staleListIDs          map[string]bool
	todoPulls             int
	todoListPulls         int
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

func (p *fakeTodoProvider) PullTodosForList(ctx context.Context, workspaceID string, listID string) (map[string]TodoRecord, error) {
	if todo.NormalizeListID(listID) == todo.DefaultListID {
		return p.PullTodos(ctx, workspaceID)
	}
	return p.listTodos[listID], nil
}

func (p *fakeTodoProvider) PullTodoArchiveMonth(_ context.Context, _ string, month string) (map[string]TodoRecord, error) {
	return p.archiveMonths[month], nil
}

func (p *fakeTodoProvider) PullTodoArchiveMonthForList(ctx context.Context, workspaceID string, listID string, month string) (map[string]TodoRecord, error) {
	if todo.NormalizeListID(listID) == todo.DefaultListID {
		return p.PullTodoArchiveMonth(ctx, workspaceID, month)
	}
	if p.listArchiveMonths == nil || p.listArchiveMonths[listID] == nil {
		return nil, nil
	}
	return p.listArchiveMonths[listID][month], nil
}

func (p *fakeTodoProvider) PullTodoArchiveMonths(context.Context, string) ([]string, error) {
	p.archiveMonthListPulls++
	return p.archiveMonthList, nil
}

func (p *fakeTodoProvider) PullTodoArchiveMonthsForList(ctx context.Context, workspaceID string, listID string) ([]string, error) {
	if todo.NormalizeListID(listID) == todo.DefaultListID {
		return p.PullTodoArchiveMonths(ctx, workspaceID)
	}
	return p.listArchiveMonthList[listID], nil
}

func (p *fakeTodoProvider) PushTodoArchiveMonths(_ context.Context, _ string, months []string) error {
	if p.archiveMonthPushErr != nil {
		return p.archiveMonthPushErr
	}
	p.archiveMonthList = months
	return nil
}

func (p *fakeTodoProvider) PushTodoArchiveMonthsForList(ctx context.Context, workspaceID string, listID string, months []string) error {
	if todo.NormalizeListID(listID) == todo.DefaultListID {
		return p.PushTodoArchiveMonths(ctx, workspaceID, months)
	}
	if p.archiveMonthPushErr != nil {
		return p.archiveMonthPushErr
	}
	if p.listArchiveMonthList == nil {
		p.listArchiveMonthList = map[string][]string{}
	}
	p.listArchiveMonthList[listID] = months
	return nil
}

func (p *fakeTodoProvider) PullTodoLists(context.Context, string) (map[string]todo.ListMeta, error) {
	if len(p.todoListsPullSequence) > 0 {
		index := p.todoListPulls
		if index >= len(p.todoListsPullSequence) {
			index = len(p.todoListsPullSequence) - 1
		}
		p.todoListPulls++
		return p.todoListsPullSequence[index], nil
	}
	return p.todoLists, nil
}

func (p *fakeTodoProvider) PushTodoListMeta(_ context.Context, _ string, meta todo.ListMeta) error {
	if p.staleListIDs[meta.ID] {
		return ErrStaleTodoListMeta
	}
	p.pushedLists = append(p.pushedLists, meta)
	if p.todoLists == nil {
		p.todoLists = map[string]todo.ListMeta{}
	}
	p.todoLists[meta.ID] = meta
	return nil
}

func (p *fakeTodoProvider) DeleteTodoListData(_ context.Context, _ string, listID string) error {
	p.deletedListData = append(p.deletedListData, todo.NormalizeListID(listID))
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
