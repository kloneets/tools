package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/kloneets/tools/src/todo"
)

type TodoSyncer struct {
	Provider    Provider
	WorkspaceID string
	StatePath   string
	TokenPath   string
	Session     Session
	DeviceID    string
}

func (s *TodoSyncer) Ready() bool {
	return s != nil && s.Provider != nil && s.WorkspaceID != "" && s.Session.IDToken != ""
}

func (s *TodoSyncer) PushStore(ctx context.Context, store todo.Store) error {
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
				EventID:   fmt.Sprintf("todo-delete-%s-%d", item.ID, rev),
				DeviceID:  state.DeviceID,
				Todo:      &record,
				CreatedAt: time.Now().UTC(),
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
			EventID:   fmt.Sprintf("todo-%s-%d", item.ID, rev),
			DeviceID:  state.DeviceID,
			Todo:      &record,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		state.Todos[item.ID] = rev
	}
	if p, ok := s.Provider.(TodoArchiveMonthPushProvider); ok {
		_ = p.PushTodoArchiveMonths(ctx, s.WorkspaceID, todo.ArchiveMonths(store))
	}
	now := time.Now().UTC()
	markFeaturePulled(&state, SyncFeatureTodos, TodoStoreHash(store), now)
	markFeaturePulled(&state, SyncFeatureTodoArchiveMonths, TodoArchiveMonthsHash(todo.ArchiveMonths(store)), now)
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureTodos, TodoStoreHash(store), now, s.Session.UID)
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureTodoArchiveMonths, TodoArchiveMonthsHash(todo.ArchiveMonths(store)), now, s.Session.UID)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	return SaveState(s.StatePath, state)
}

func (s *TodoSyncer) PullStore(ctx context.Context, local todo.Store) (todo.Store, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return local, false, err
	}
	now := time.Now().UTC()
	hashes, hasHashes := pullSyncHashes(ctx, s.Provider, s.WorkspaceID)
	skipTodos := hasHashes && shouldSkipFeaturePull(state, s.WorkspaceID, SyncFeatureTodos, hashes[SyncFeatureTodos], now)
	skipMonths := hasHashes && shouldSkipFeaturePull(state, s.WorkspaceID, SyncFeatureTodoArchiveMonths, hashes[SyncFeatureTodoArchiveMonths], now)
	merged := local
	if !skipTodos {
		remote, err := pullRemoteTodos(ctx, s.Provider, s.WorkspaceID)
		if err != nil {
			return local, false, err
		}
		merged = todo.PreserveArchived(local, MergeTodos(todo.NonArchivedStore(local), remoteTodoRecordsForLocal(local, remote)))
		hash := TodoRecordsHash(remote)
		markFeaturePulled(&state, SyncFeatureTodos, hash, now)
		if !hasHashes || hashes[SyncFeatureTodos].Hash == "" {
			pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureTodos, hash, now, s.Session.UID)
		}
	}
	if !skipMonths {
		months, err := pullRemoteTodoArchiveMonths(ctx, s.Provider, s.WorkspaceID)
		if err != nil {
			return local, false, err
		}
		merged.ArchiveMonths = append(merged.ArchiveMonths, months...)
		hash := TodoArchiveMonthsHash(months)
		markFeaturePulled(&state, SyncFeatureTodoArchiveMonths, hash, now)
		if !hasHashes || hashes[SyncFeatureTodoArchiveMonths].Hash == "" {
			pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureTodoArchiveMonths, hash, now, s.Session.UID)
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
	if !s.Ready() {
		return local, false, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return local, false, err
	}
	now := time.Now().UTC()
	feature := archiveMonthFeature(month)
	if hashes, ok := pullSyncHashes(ctx, s.Provider, s.WorkspaceID); ok && shouldSkipFeaturePull(state, s.WorkspaceID, feature, hashes[feature], now) {
		return local, false, nil
	}
	remote, err := pullRemoteTodoArchiveMonth(ctx, s.Provider, s.WorkspaceID, month)
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

func pullRemoteTodos(ctx context.Context, provider Provider, workspaceID string) (map[string]TodoRecord, error) {
	if p, ok := provider.(TodoPullProvider); ok {
		return p.PullTodos(ctx, workspaceID)
	}
	snapshot, err := provider.PullSnapshot(ctx, workspaceID)
	return snapshot.Todos, err
}

func pullRemoteTodoArchiveMonth(ctx context.Context, provider Provider, workspaceID string, month string) (map[string]TodoRecord, error) {
	if p, ok := provider.(TodoArchivePullProvider); ok {
		return p.PullTodoArchiveMonth(ctx, workspaceID, month)
	}
	return map[string]TodoRecord{}, nil
}

func pullRemoteTodoArchiveMonths(ctx context.Context, provider Provider, workspaceID string) ([]string, error) {
	if p, ok := provider.(TodoArchivePullProvider); ok {
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
