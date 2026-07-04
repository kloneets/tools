package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/kloneets/tools/src/todo"
)

const (
	SyncFeatureTodos             = "todos"
	SyncFeatureTodoArchiveMonths = "todo_archive_months"
	SyncFeatureNotes             = "notes"
	SyncFeatureSettings          = "settings"

	hashFullValidationInterval = 24 * time.Hour
)

func archiveMonthFeature(month string) string {
	return "todo_archive_month:" + strings.TrimSpace(month)
}

func pullSyncHashes(ctx context.Context, provider Provider, workspaceID string) (map[string]SyncHashRecord, bool) {
	p, ok := provider.(SyncHashPullProvider)
	if !ok {
		return nil, false
	}
	hashes, err := p.PullSyncHashes(ctx, workspaceID)
	if err != nil {
		return nil, false
	}
	if hashes == nil {
		hashes = map[string]SyncHashRecord{}
	}
	return hashes, true
}

func pushSyncHashBestEffort(ctx context.Context, provider Provider, workspaceID string, feature string, hash string, updatedAt time.Time, updatedBy string) {
	p, ok := provider.(SyncHashPushProvider)
	if !ok || strings.TrimSpace(hash) == "" {
		return
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_ = p.PushSyncHash(ctx, workspaceID, feature, SyncHashRecord{
		Hash:      hash,
		UpdatedAt: updatedAt.UTC(),
		UpdatedBy: updatedBy,
	})
}

func shouldSkipFeaturePull(state State, workspaceID string, feature string, remote SyncHashRecord, now time.Time) bool {
	if workspaceID == "" || state.WorkspaceID != workspaceID || remote.Hash == "" {
		return false
	}
	local := state.SyncHashes[feature]
	if local.Hash == "" || local.Hash != remote.Hash {
		return false
	}
	if local.LastFullPullAt.IsZero() {
		return false
	}
	return now.Sub(local.LastFullPullAt) < hashFullValidationInterval
}

func markFeaturePulled(state *State, feature string, hash string, now time.Time) {
	if state.SyncHashes == nil {
		state.SyncHashes = map[string]HashState{}
	}
	state.SyncHashes[feature] = HashState{Hash: hash, LastFullPullAt: now.UTC()}
}

func hashCanonical(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TodoStoreHash(store todo.Store) string {
	type metadata struct {
		ID      string `json:"id"`
		Rev     int64  `json:"rev"`
		Deleted bool   `json:"deleted"`
	}
	records := make([]metadata, 0, len(store.Items))
	for _, item := range store.Items {
		records = append(records, metadata{ID: item.ID, Rev: item.UpdatedAt.UnixMilli(), Deleted: item.Status == todo.StatusArchived})
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return hashCanonical(records)
}

func TodoRecordsHash(records map[string]TodoRecord) string {
	type metadata struct {
		ID      string `json:"id"`
		Rev     int64  `json:"rev"`
		Deleted bool   `json:"deleted"`
	}
	items := make([]metadata, 0, len(records))
	for id, record := range records {
		if record.Item.Status == todo.StatusArchived && !record.Deleted {
			continue
		}
		if record.Item.ID == "" {
			record.Item.ID = id
		}
		items = append(items, metadata{ID: record.Item.ID, Rev: record.Rev, Deleted: record.Deleted})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return hashCanonical(items)
}

func TodoArchiveMonthsHash(months []string) string {
	normalized := append([]string(nil), months...)
	sort.Strings(normalized)
	return hashCanonical(normalized)
}

func TodoArchiveMonthHash(records map[string]TodoRecord) string {
	type metadata struct {
		ID      string `json:"id"`
		Rev     int64  `json:"rev"`
		Deleted bool   `json:"deleted"`
	}
	items := make([]metadata, 0, len(records))
	for id, record := range records {
		if record.Item.ID == "" {
			record.Item.ID = id
		}
		items = append(items, metadata{ID: record.Item.ID, Rev: record.Rev, Deleted: record.Deleted})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return hashCanonical(items)
}

func NoteMetadataHash(records map[string]NoteRecord) string {
	type metadata struct {
		ID      string `json:"id"`
		Path    string `json:"path"`
		Rev     int64  `json:"rev"`
		Deleted bool   `json:"deleted"`
	}
	items := make([]metadata, 0, len(records))
	for id, record := range records {
		if record.ID == "" {
			record.ID = id
		}
		items = append(items, metadata{ID: record.ID, Path: NormalizeNotePath(record.Path), Rev: record.Rev, Deleted: record.Deleted})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return hashCanonical(items)
}
