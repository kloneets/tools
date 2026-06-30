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
		if item.Status == todo.StatusArchived {
			continue
		}
		rev := item.UpdatedAt.UnixMilli()
		if rev <= 0 {
			rev = time.Now().UnixMilli()
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
		if err := p.PushTodoArchiveMonths(ctx, s.WorkspaceID, todo.ArchiveMonths(store)); err != nil {
			return err
		}
	}
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	return SaveState(s.StatePath, state)
}

func (s *TodoSyncer) PullStore(ctx context.Context, local todo.Store) (todo.Store, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	remote, err := pullRemoteTodos(ctx, s.Provider, s.WorkspaceID)
	if err != nil {
		return local, false, err
	}
	merged := todo.PreserveArchived(local, MergeTodos(todo.NonArchivedStore(local), nonArchivedTodoRecords(remote)))
	months, err := pullRemoteTodoArchiveMonths(ctx, s.Provider, s.WorkspaceID)
	if err != nil {
		return local, false, err
	}
	merged.ArchiveMonths = append(merged.ArchiveMonths, months...)
	todo.Normalize(&merged)
	if sameTodoStore(local, merged) {
		return local, false, nil
	}
	return merged, true, nil
}

func (s *TodoSyncer) PullArchiveMonth(ctx context.Context, local todo.Store, month string) (todo.Store, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	remote, err := pullRemoteTodoArchiveMonth(ctx, s.Provider, s.WorkspaceID, month)
	if err != nil {
		return local, false, err
	}
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
		if record.Item.Status == todo.StatusArchived {
			continue
		}
		out[id] = record
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
	return true
}
