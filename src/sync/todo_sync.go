package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kloneets/tools/src/todo"
)

var ErrStaleTodoListMeta = errors.New("stale todo list metadata")

type TodoSyncer struct {
	Provider    Provider
	WorkspaceID string
	StatePath   string
	TokenPath   string
	Session     Session
	DeviceID    string
	ListID      string
}

func (s *TodoSyncer) Ready() bool {
	return s != nil && s.Provider != nil && s.WorkspaceID != "" && s.Session.IDToken != ""
}

func (s *TodoSyncer) PushStore(ctx context.Context, store todo.Store) error {
	return s.PushStoreForList(ctx, s.listID(), store)
}

func (s *TodoSyncer) PushStoreForList(ctx context.Context, listID string, store todo.Store) error {
	if !s.Ready() {
		return nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return err
	}
	if s.DeviceID != "" {
		state.DeviceID = s.DeviceID
	}
	listID = normalizeSyncListID(listID)
	for _, item := range store.Items {
		rev := item.UpdatedAt.UnixMilli()
		if rev <= 0 {
			rev = time.Now().UnixMilli()
		}
		if item.Status == todo.StatusArchived {
			record := TodoRecord{
				Item:      todo.Item{ID: item.ID, Status: todo.StatusArchived, UpdatedAt: item.UpdatedAt},
				Rev:       rev,
				UpdatedBy: s.Session.UID,
				Deleted:   true,
			}
			if err := s.Provider.PushMutation(ctx, s.WorkspaceID, Mutation{
				EventID:    fmt.Sprintf("todo-delete-%s-%d", item.ID, rev),
				DeviceID:   state.DeviceID,
				TodoListID: mutationTodoListID(listID),
				Todo:       &record,
				CreatedAt:  time.Now().UTC(),
			}); err != nil {
				return err
			}
			state.Todos[item.ID] = rev
			continue
		}
		record := TodoRecord{
			Item:      item,
			Rev:       rev,
			UpdatedBy: s.Session.UID,
		}
		if err := s.Provider.PushMutation(ctx, s.WorkspaceID, Mutation{
			EventID:    fmt.Sprintf("todo-%s-%d", item.ID, rev),
			DeviceID:   state.DeviceID,
			TodoListID: mutationTodoListID(listID),
			Todo:       &record,
			CreatedAt:  time.Now().UTC(),
		}); err != nil {
			return err
		}
		state.Todos[item.ID] = rev
	}
	if p, ok := s.Provider.(TodoListArchiveMonthPushProvider); ok {
		_ = p.PushTodoArchiveMonthsForList(ctx, s.WorkspaceID, listID, todo.ArchiveMonths(store))
	} else if p, ok := s.Provider.(TodoArchiveMonthPushProvider); ok && listID == todo.DefaultListID {
		_ = p.PushTodoArchiveMonths(ctx, s.WorkspaceID, todo.ArchiveMonths(store))
	}
	now := time.Now().UTC()
	markFeaturePulled(&state, todoFeature(listID), TodoStoreHash(store), now)
	markFeaturePulled(&state, todoArchiveMonthsFeature(listID), TodoArchiveMonthsHash(todo.ArchiveMonths(store)), now)
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, todoFeature(listID), TodoStoreHash(store), now, s.Session.UID)
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, todoArchiveMonthsFeature(listID), TodoArchiveMonthsHash(todo.ArchiveMonths(store)), now, s.Session.UID)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	return SaveState(s.StatePath, state)
}

func (s *TodoSyncer) PullStore(ctx context.Context, local todo.Store) (todo.Store, bool, error) {
	return s.PullStoreForList(ctx, s.listID(), local)
}

func (s *TodoSyncer) PullStoreForList(ctx context.Context, listID string, local todo.Store) (todo.Store, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return local, false, err
	}
	now := time.Now().UTC()
	hashes, hasHashes := pullSyncHashes(ctx, s.Provider, s.WorkspaceID)
	listID = normalizeSyncListID(listID)
	todosFeature := todoFeature(listID)
	monthsFeature := todoArchiveMonthsFeature(listID)
	skipTodos := hasHashes && shouldSkipFeaturePull(state, s.WorkspaceID, todosFeature, hashes[todosFeature], now)
	skipMonths := hasHashes && shouldSkipFeaturePull(state, s.WorkspaceID, monthsFeature, hashes[monthsFeature], now)
	merged := local
	if !skipTodos {
		remote, err := pullRemoteTodos(ctx, s.Provider, s.WorkspaceID, listID)
		if err != nil {
			return local, false, err
		}
		merged = todo.PreserveArchived(local, MergeTodos(todo.NonArchivedStore(local), remoteTodoRecordsForLocal(local, remote)))
		hash := TodoRecordsHash(remote)
		markFeaturePulled(&state, todosFeature, hash, now)
		if !hasHashes || hashes[todosFeature].Hash == "" {
			pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, todosFeature, hash, now, s.Session.UID)
		}
	}
	if !skipMonths {
		months, err := pullRemoteTodoArchiveMonths(ctx, s.Provider, s.WorkspaceID, listID)
		if err != nil {
			return local, false, err
		}
		merged.ArchiveMonths = append(merged.ArchiveMonths, months...)
		hash := TodoArchiveMonthsHash(months)
		markFeaturePulled(&state, monthsFeature, hash, now)
		if !hasHashes || hashes[monthsFeature].Hash == "" {
			pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, monthsFeature, hash, now, s.Session.UID)
		}
	}
	todo.Normalize(&merged)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if s.StatePath != "" {
		if err := SaveState(s.StatePath, state); err != nil {
			return local, false, err
		}
	}
	if sameTodoStore(local, merged) {
		return local, false, nil
	}
	return merged, true, nil
}

func (s *TodoSyncer) PullArchiveMonth(ctx context.Context, local todo.Store, month string) (todo.Store, bool, error) {
	return s.PullArchiveMonthForList(ctx, s.listID(), local, month)
}

func (s *TodoSyncer) PullArchiveMonthForList(ctx context.Context, listID string, local todo.Store, month string) (todo.Store, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return local, false, err
	}
	now := time.Now().UTC()
	listID = normalizeSyncListID(listID)
	feature := archiveMonthFeature(listID, month)
	if hashes, ok := pullSyncHashes(ctx, s.Provider, s.WorkspaceID); ok && shouldSkipFeaturePull(state, s.WorkspaceID, feature, hashes[feature], now) {
		return local, false, nil
	}
	remote, err := pullRemoteTodoArchiveMonth(ctx, s.Provider, s.WorkspaceID, listID, month)
	if err != nil {
		return local, false, err
	}
	hash := TodoArchiveMonthHash(remote)
	markFeaturePulled(&state, feature, hash, now)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if s.StatePath != "" {
		if err := SaveState(s.StatePath, state); err != nil {
			return local, false, err
		}
	}
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, feature, hash, now, s.Session.UID)
	items := make([]todo.Item, 0, len(remote))
	for id, record := range remote {
		if record.Deleted {
			continue
		}
		item := record.Item
		if item.ID == "" {
			item.ID = id
		}
		if item.Status != todo.StatusArchived || item.ArchivedAt == nil || item.ArchivedAt.UTC().Format("2006-01") != month {
			continue
		}
		items = append(items, item)
	}
	merged := todo.MergeArchiveMonth(local, month, items)
	if sameTodoStore(local, merged) {
		return local, false, nil
	}
	return merged, true, nil
}

func (s *TodoSyncer) PullLists(ctx context.Context, local todo.ListsStore) (todo.ListsStore, bool, error) {
	return s.pullLists(ctx, local, true)
}

func (s *TodoSyncer) PullListsFull(ctx context.Context, local todo.ListsStore) (todo.ListsStore, bool, error) {
	return s.pullLists(ctx, local, false)
}

func (s *TodoSyncer) pullLists(ctx context.Context, local todo.ListsStore, allowHashSkip bool) (todo.ListsStore, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	p, ok := s.Provider.(TodoListMetaProvider)
	if !ok {
		return local, false, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return local, false, err
	}
	now := time.Now().UTC()
	hashes, hasHashes := pullSyncHashes(ctx, s.Provider, s.WorkspaceID)
	if allowHashSkip && hasHashes && shouldSkipFeaturePull(state, s.WorkspaceID, SyncFeatureTodoLists, hashes[SyncFeatureTodoLists], now) {
		return local, false, nil
	}
	remote, err := p.PullTodoLists(ctx, s.WorkspaceID)
	if err != nil {
		return local, false, err
	}
	remoteCatalog := todo.ListsStore{Version: todo.SchemaVersion, Lists: make([]todo.ListMeta, 0, len(remote))}
	for _, meta := range remote {
		remoteCatalog.Lists = append(remoteCatalog.Lists, meta)
	}
	merged := todo.MergeLists(local, remoteCatalog, now)
	hash := TodoListsHash(remoteCatalog)
	markFeaturePulled(&state, SyncFeatureTodoLists, hash, now)
	if !hasHashes || hashes[SyncFeatureTodoLists].Hash == "" {
		pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureTodoLists, hash, now, s.Session.UID)
	}
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if s.StatePath != "" {
		if err := SaveState(s.StatePath, state); err != nil {
			return local, false, err
		}
	}
	if sameTodoLists(local, merged) {
		return local, false, nil
	}
	return merged, true, nil
}

func (s *TodoSyncer) PushLists(ctx context.Context, catalog todo.ListsStore) (bool, error) {
	if !s.Ready() {
		return true, nil
	}
	p, ok := s.Provider.(TodoListMetaProvider)
	if !ok {
		return true, nil
	}
	todo.NormalizeLists(&catalog, time.Now().UTC())
	stale := false
	for _, meta := range catalog.Lists {
		if err := p.PushTodoListMeta(ctx, s.WorkspaceID, meta); err != nil {
			if errors.Is(err, ErrStaleTodoListMeta) {
				stale = true
				continue
			}
			return false, err
		}
		if meta.Deleted && meta.ID != todo.DefaultListID {
			if err := p.DeleteTodoListData(ctx, s.WorkspaceID, meta.ID); err != nil {
				return false, err
			}
		}
	}
	if stale {
		return false, nil
	}
	now := time.Now().UTC()
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureTodoLists, TodoListsHash(catalog), now, s.Session.UID)
	state, err := LoadState(s.StatePath)
	if err != nil {
		return false, err
	}
	markFeaturePulled(&state, SyncFeatureTodoLists, TodoListsHash(catalog), now)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if s.StatePath == "" {
		return true, nil
	}
	return true, SaveState(s.StatePath, state)
}

func (s *TodoSyncer) SyncRepository(ctx context.Context, repo *todo.Repository) (todo.ListsStore, error) {
	if repo == nil || !s.Ready() {
		return todo.ListsStore{}, nil
	}
	catalog, err := repo.LoadLists()
	if err != nil {
		return todo.ListsStore{}, err
	}
	mergedCatalog, changed, err := s.PullLists(ctx, catalog)
	if err != nil {
		return todo.ListsStore{}, err
	}
	if changed {
		if err := repo.SaveLists(mergedCatalog); err != nil {
			return todo.ListsStore{}, err
		}
		if err := repo.RemoveDeletedListData(mergedCatalog); err != nil {
			return todo.ListsStore{}, err
		}
		catalog, err = repo.LoadLists()
		if err != nil {
			return todo.ListsStore{}, err
		}
	} else {
		catalog = mergedCatalog
	}
	listsPushed, err := s.PushLists(ctx, catalog)
	if err != nil {
		return todo.ListsStore{}, err
	}
	if !listsPushed {
		catalog, err = s.refreshCatalogAfterStaleListPush(ctx, repo, catalog)
		if err != nil {
			return todo.ListsStore{}, err
		}
		listsPushed, err = s.PushLists(ctx, catalog)
		if err != nil {
			return todo.ListsStore{}, err
		}
		if !listsPushed {
			catalog, err = s.refreshCatalogAfterStaleListPush(ctx, repo, catalog)
			if err != nil {
				return todo.ListsStore{}, err
			}
			return catalog, nil
		}
	}
	for _, list := range todo.ActiveLists(catalog) {
		store, err := repo.LoadList(list.ID)
		if err != nil {
			return todo.ListsStore{}, err
		}
		merged, storeChanged, err := s.PullStoreForList(ctx, list.ID, store)
		if err != nil {
			return todo.ListsStore{}, err
		}
		if storeChanged {
			if err := repo.SaveList(list.ID, merged); err != nil {
				return todo.ListsStore{}, err
			}
			store = merged
		}
		if err := s.PushStoreForList(ctx, list.ID, store); err != nil {
			return todo.ListsStore{}, err
		}
	}
	return repo.LoadLists()
}

func (s *TodoSyncer) refreshCatalogAfterStaleListPush(ctx context.Context, repo *todo.Repository, catalog todo.ListsStore) (todo.ListsStore, error) {
	mergedCatalog, changed, err := s.PullListsFull(ctx, catalog)
	if err != nil {
		return todo.ListsStore{}, err
	}
	if changed {
		if err := repo.SaveLists(mergedCatalog); err != nil {
			return todo.ListsStore{}, err
		}
		if err := repo.RemoveDeletedListData(mergedCatalog); err != nil {
			return todo.ListsStore{}, err
		}
		return repo.LoadLists()
	}
	return mergedCatalog, nil
}

func pullRemoteTodos(ctx context.Context, provider Provider, workspaceID string, listID string) (map[string]TodoRecord, error) {
	listID = normalizeSyncListID(listID)
	if p, ok := provider.(TodoListPullProvider); ok {
		return p.PullTodosForList(ctx, workspaceID, listID)
	}
	if p, ok := provider.(TodoPullProvider); ok && listID == todo.DefaultListID {
		return p.PullTodos(ctx, workspaceID)
	}
	snapshot, err := provider.PullSnapshot(ctx, workspaceID)
	if listID != todo.DefaultListID {
		return map[string]TodoRecord{}, err
	}
	return snapshot.Todos, err
}

func pullRemoteTodoArchiveMonth(ctx context.Context, provider Provider, workspaceID string, listID string, month string) (map[string]TodoRecord, error) {
	listID = normalizeSyncListID(listID)
	if p, ok := provider.(TodoListPullProvider); ok {
		return p.PullTodoArchiveMonthForList(ctx, workspaceID, listID, month)
	}
	if p, ok := provider.(TodoArchivePullProvider); ok && listID == todo.DefaultListID {
		return p.PullTodoArchiveMonth(ctx, workspaceID, month)
	}
	return map[string]TodoRecord{}, nil
}

func pullRemoteTodoArchiveMonths(ctx context.Context, provider Provider, workspaceID string, listID string) ([]string, error) {
	listID = normalizeSyncListID(listID)
	if p, ok := provider.(TodoListPullProvider); ok {
		return p.PullTodoArchiveMonthsForList(ctx, workspaceID, listID)
	}
	if p, ok := provider.(TodoArchivePullProvider); ok && listID == todo.DefaultListID {
		return p.PullTodoArchiveMonths(ctx, workspaceID)
	}
	return nil, nil
}

func nonArchivedTodoRecords(records map[string]TodoRecord) map[string]TodoRecord {
	out := make(map[string]TodoRecord, len(records))
	for id, record := range records {
		if record.Item.Status == todo.StatusArchived && !record.Deleted {
			continue
		}
		out[id] = record
	}
	return out
}

func remoteTodoRecordsForLocal(local todo.Store, records map[string]TodoRecord) map[string]TodoRecord {
	out := nonArchivedTodoRecords(records)
	archived := map[string]todo.Item{}
	for _, item := range local.Items {
		if item.Status == todo.StatusArchived {
			archived[item.ID] = item
		}
	}
	for id, record := range out {
		if record.Deleted {
			continue
		}
		localArchived, ok := archived[id]
		if ok && !localArchived.UpdatedAt.Before(record.Item.UpdatedAt) {
			delete(out, id)
		}
	}
	return out
}

func sameTodoStore(left todo.Store, right todo.Store) bool {
	if len(left.Items) != len(right.Items) {
		return false
	}
	for i := range left.Items {
		if left.Items[i] != right.Items[i] {
			return false
		}
	}
	if len(left.ArchiveMonths) != len(right.ArchiveMonths) {
		return false
	}
	for i := range left.ArchiveMonths {
		if left.ArchiveMonths[i] != right.ArchiveMonths[i] {
			return false
		}
	}
	return true
}

func (s *TodoSyncer) listID() string {
	if s == nil {
		return todo.DefaultListID
	}
	return normalizeSyncListID(s.ListID)
}

func normalizeSyncListID(id string) string {
	id = todo.NormalizeListID(id)
	if id == "" {
		return todo.DefaultListID
	}
	return id
}

func mutationTodoListID(listID string) string {
	listID = normalizeSyncListID(listID)
	if listID == todo.DefaultListID {
		return ""
	}
	return listID
}

func sameTodoLists(left todo.ListsStore, right todo.ListsStore) bool {
	todo.NormalizeLists(&left, time.Time{})
	todo.NormalizeLists(&right, time.Time{})
	if len(left.Lists) != len(right.Lists) || left.CurrentListID != right.CurrentListID {
		return false
	}
	for i := range left.Lists {
		if left.Lists[i].ID != right.Lists[i].ID ||
			left.Lists[i].Name != right.Lists[i].Name ||
			left.Lists[i].Rev != right.Lists[i].Rev ||
			!left.Lists[i].CreatedAt.Equal(right.Lists[i].CreatedAt) ||
			!left.Lists[i].UpdatedAt.Equal(right.Lists[i].UpdatedAt) ||
			left.Lists[i].Deleted != right.Lists[i].Deleted {
			return false
		}
		if (left.Lists[i].DeletedAt == nil) != (right.Lists[i].DeletedAt == nil) {
			return false
		}
		if left.Lists[i].DeletedAt != nil && !left.Lists[i].DeletedAt.Equal(*right.Lists[i].DeletedAt) {
			return false
		}
	}
	return true
}

func todoListMetaCanReplaceRemote(candidate todo.ListMeta, remote todo.ListMeta) bool {
	candidateRev := todoListMetaRevision(candidate)
	remoteRev := todoListMetaRevision(remote)
	if remoteRev <= 0 {
		return true
	}
	if candidateRev != remoteRev {
		return candidateRev > remoteRev
	}
	if candidate.Deleted != remote.Deleted {
		return candidate.Deleted
	}
	return candidate.Name == remote.Name && candidate.Deleted == remote.Deleted
}

func todoListMetaRevision(meta todo.ListMeta) int64 {
	if meta.Rev > 0 {
		return meta.Rev
	}
	if !meta.UpdatedAt.IsZero() {
		return meta.UpdatedAt.UTC().UnixMilli()
	}
	return 0
}
