package sync

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kloneets/tools/src/todo"
)

type LocalNote struct {
	ID      string
	Path    string
	Text    string
	BaseRev int64
	Dirty   bool
}

type NoteApplyResult struct {
	Note         LocalNote
	ConflictCopy *LocalNote
	Deleted      bool
	Applied      bool
	Deferred     bool
}

func ApplyRemoteNote(local LocalNote, remote NoteRecord, deviceID string, now time.Time) NoteApplyResult {
	result := NoteApplyResult{Note: local}
	if remote.Deleted {
		if local.Dirty && local.BaseRev < remote.Rev {
			result.ConflictCopy = conflictCopy(local, deviceID, now)
			result.Note = LocalNote{ID: local.ID, Path: local.Path, BaseRev: remote.Rev}
		}
		result.Deleted = true
		result.Applied = true
		return result
	}
	if local.Dirty && local.BaseRev < remote.Rev {
		result.Note = LocalNote{
			ID:      remote.ID,
			Path:    remote.Path,
			Text:    remote.Text,
			BaseRev: remote.Rev,
		}
		result.ConflictCopy = conflictCopy(local, deviceID, now)
		result.Applied = true
		return result
	}
	if local.Dirty {
		result.Deferred = true
		return result
	}
	result.Note = LocalNote{
		ID:      remote.ID,
		Path:    remote.Path,
		Text:    remote.Text,
		BaseRev: remote.Rev,
	}
	result.Applied = true
	return result
}

func MergeTodos(local todo.Store, remote map[string]TodoRecord) todo.Store {
	out := todo.Store{Version: todo.SchemaVersion, Items: append([]todo.Item(nil), local.Items...)}
	index := make(map[string]int, len(out.Items))
	for i := range out.Items {
		index[out.Items[i].ID] = i
	}
	for id, record := range remote {
		if id == "" {
			id = record.Item.ID
		}
		if id == "" {
			continue
		}
		localIndex, exists := index[id]
		if record.Deleted {
			if exists {
				out.Items = append(out.Items[:localIndex], out.Items[localIndex+1:]...)
				index = make(map[string]int, len(out.Items))
				for i := range out.Items {
					index[out.Items[i].ID] = i
				}
			}
			continue
		}
		record.Item.ID = id
		if !exists {
			out.Items = append(out.Items, record.Item)
			index[id] = len(out.Items) - 1
			continue
		}
		if remoteWins(out.Items[localIndex], record.Item) {
			out.Items[localIndex] = record.Item
		}
	}
	todo.Normalize(&out)
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].Status == out.Items[j].Status && out.Items[i].Order != out.Items[j].Order {
			return out.Items[i].Order < out.Items[j].Order
		}
		return out.Items[i].CreatedAt.Before(out.Items[j].CreatedAt)
	})
	return out
}

func SharedWorkspaceSettings(settings map[string]any) map[string]any {
	shared := map[string]any{}
	for _, key := range []string{"pages_app", "password_app"} {
		if value, ok := settings[key]; ok {
			shared[key] = value
		}
	}
	if value, ok := settings["notes_app"].(map[string]any); ok {
		notes := map[string]any{}
		for _, key := range []string{"tab_spaces", "undo_levels", "vim_mode", "spell_check_enabled", "spell_dictionaries"} {
			if field, ok := value[key]; ok {
				notes[key] = field
			}
		}
		if len(notes) > 0 {
			shared["notes_app"] = notes
		}
	}
	return shared
}

func ApplySharedWorkspaceSettings(local map[string]any, shared map[string]any) map[string]any {
	out := cloneMap(local)
	for _, key := range []string{"pages_app", "password_app"} {
		if value, ok := shared[key]; ok {
			out[key] = value
		}
	}
	if value, ok := shared["notes_app"].(map[string]any); ok {
		notes, _ := out["notes_app"].(map[string]any)
		next := cloneMap(notes)
		for _, key := range []string{"tab_spaces", "undo_levels", "vim_mode", "spell_check_enabled", "spell_dictionaries"} {
			if field, ok := value[key]; ok {
				next[key] = field
			}
		}
		out["notes_app"] = next
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func conflictCopy(local LocalNote, deviceID string, now time.Time) *LocalNote {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		deviceID = "device"
	}
	ext := filepath.Ext(local.Path)
	stem := strings.TrimSuffix(local.Path, ext)
	if stem == "" {
		stem = "note"
	}
	stamp := now.UTC().Format("20060102-150405")
	return &LocalNote{
		ID:    fmt.Sprintf("%s-conflict-%s", local.ID, stamp),
		Path:  fmt.Sprintf("%s.conflict-%s-%s%s", stem, sanitizePathPart(deviceID), stamp, ext),
		Text:  local.Text,
		Dirty: true,
	}
}

func remoteWins(local todo.Item, remote todo.Item) bool {
	if remote.UpdatedAt.IsZero() {
		return false
	}
	if local.UpdatedAt.IsZero() {
		return true
	}
	return remote.UpdatedAt.After(local.UpdatedAt) || remote.UpdatedAt.Equal(local.UpdatedAt)
}

func sanitizePathPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return strings.Trim(b.String(), "-")
}
