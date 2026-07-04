package sync

import (
	"context"
	"testing"
	"time"
)

func TestSettingsSyncerPushWritesSharedSettingsRecord(t *testing.T) {
	provider := &fakeSettingsProvider{}
	syncer := SettingsSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{UID: "uid", IDToken: "token"},
		Now:         func() time.Time { return time.Unix(10, 0).UTC() },
	}

	result, err := syncer.PushSettings(context.Background(), map[string]any{
		"pages_app": map[string]any{"first_book": 10},
		"ui":        map[string]any{"theme": "gruvbox"},
	})
	if err != nil {
		t.Fatalf("PushSettings() error = %v", err)
	}
	if !result.Pushed {
		t.Fatal("Pushed = false, want true")
	}

	if len(provider.mutations) != 1 || provider.mutations[0].Settings == nil {
		t.Fatalf("mutations = %#v, want one settings mutation", provider.mutations)
	}
	if _, ok := provider.mutations[0].Settings.Values["ui"]; ok {
		t.Fatalf("shared settings included local-only ui: %#v", provider.mutations[0].Settings.Values)
	}
}

func TestSettingsSyncerPullReturnsNewerSharedSettings(t *testing.T) {
	provider := &fakeSettingsProvider{snapshot: Snapshot{Settings: map[string]any{
		"shared": map[string]any{
			"values": map[string]any{"pages_app": map[string]any{"first_book": 10}},
			"rev":    float64(5),
		},
	}}}
	syncer := SettingsSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{UID: "uid", IDToken: "token"},
	}

	result, err := syncer.PullSettings(context.Background())
	if err != nil {
		t.Fatalf("PullSettings() error = %v", err)
	}

	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if _, ok := result.Values["pages_app"]; !ok {
		t.Fatalf("Values = %#v, want pages_app", result.Values)
	}
}

func TestSettingsSyncerPushSkipsUnchangedSharedSettings(t *testing.T) {
	provider := &fakeSettingsProvider{}
	syncer := SettingsSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{UID: "uid", IDToken: "token"},
		Now:         func() time.Time { return time.Unix(10, 0).UTC() },
	}
	settings := map[string]any{
		"pages_app": map[string]any{"first_book": 10},
		"notes_app": map[string]any{
			"current_note_path": "local.md",
			"preview_hidden":    true,
		},
	}

	if _, err := syncer.PushSettings(context.Background(), settings); err != nil {
		t.Fatalf("first PushSettings() error = %v", err)
	}
	settings["notes_app"].(map[string]any)["current_note_path"] = "other.md"
	settings["notes_app"].(map[string]any)["preview_hidden"] = false
	result, err := syncer.PushSettings(context.Background(), settings)
	if err != nil {
		t.Fatalf("second PushSettings() error = %v", err)
	}
	if result.Pushed {
		t.Fatal("second Pushed = true, want unchanged shared settings skipped")
	}

	if len(provider.mutations) != 1 {
		t.Fatalf("mutations len = %d, want unchanged local-only settings push skipped", len(provider.mutations))
	}
}

func TestSettingsSyncerPullSkipsFreshMatchingHash(t *testing.T) {
	values := map[string]any{"pages_app": map[string]any{"first_book": 10}}
	hash := sharedSettingsHash(values)
	provider := &fakeSettingsProvider{
		hashes: map[string]SyncHashRecord{SyncFeatureSettings: {Hash: hash}},
		snapshot: Snapshot{Settings: map[string]any{
			"shared": map[string]any{"values": values, "rev": float64(5)},
		}},
	}
	statePath := t.TempDir() + "/state.json"
	state := defaultState()
	state.WorkspaceID = "workspace"
	markFeaturePulled(&state, SyncFeatureSettings, hash, time.Now().UTC())
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	syncer := SettingsSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   statePath,
		Session:     Session{UID: "uid", IDToken: "token"},
	}

	result, err := syncer.PullSettings(context.Background())
	if err != nil {
		t.Fatalf("PullSettings() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Changed = true, want skipped settings pull")
	}
	if provider.snapshotPulls != 0 {
		t.Fatalf("snapshotPulls = %d, want no settings pull", provider.snapshotPulls)
	}
}

type fakeSettingsProvider struct {
	snapshot      Snapshot
	mutations     []Mutation
	hashes        map[string]SyncHashRecord
	snapshotPulls int
}

func (p *fakeSettingsProvider) Login(context.Context, string, string) (Session, error) {
	return Session{}, nil
}

func (p *fakeSettingsProvider) WatchWorkspace(context.Context, string, string, func(Change) error) error {
	return nil
}

func (p *fakeSettingsProvider) PushMutation(_ context.Context, _ string, mutation Mutation) error {
	p.mutations = append(p.mutations, mutation)
	return nil
}

func (p *fakeSettingsProvider) PullSnapshot(context.Context, string) (Snapshot, error) {
	p.snapshotPulls++
	return p.snapshot, nil
}

func (p *fakeSettingsProvider) PullSettings(context.Context, string) (map[string]any, error) {
	p.snapshotPulls++
	return p.snapshot.Settings, nil
}

func (p *fakeSettingsProvider) PullSyncHashes(context.Context, string) (map[string]SyncHashRecord, error) {
	return p.hashes, nil
}

func (p *fakeSettingsProvider) PushSyncHash(_ context.Context, _ string, feature string, record SyncHashRecord) error {
	if p.hashes == nil {
		p.hashes = map[string]SyncHashRecord{}
	}
	p.hashes[feature] = record
	return nil
}

func (p *fakeSettingsProvider) CreateWorkspace(context.Context, string) (WorkspaceMeta, error) {
	return WorkspaceMeta{}, nil
}

func (p *fakeSettingsProvider) GrantMember(context.Context, string, string, string) error {
	return nil
}

func (p *fakeSettingsProvider) RevokeMember(context.Context, string, string) error {
	return nil
}
