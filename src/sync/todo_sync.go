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
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	return SaveState(s.StatePath, state)
}

func (s *TodoSyncer) PullStore(ctx context.Context, local todo.Store) (todo.Store, bool, error) {
	if !s.Ready() {
		return local, false, nil
	}
	snapshot, err := s.Provider.PullSnapshot(ctx, s.WorkspaceID)
	if err != nil {
		return local, false, err
	}
	merged := MergeTodos(local, snapshot.Todos)
	if sameTodoStore(local, merged) {
		return local, false, nil
	}
	return merged, true, nil
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
