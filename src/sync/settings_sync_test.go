package sync

import (
	"context"
	"testing"
	"time"
)

func TestSettingsSyncerPushWritesSharedSettingsRecord(t *testing.T) {
	provider := &fakeAssetProvider{}
	syncer := SettingsSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{UID: "uid", IDToken: "token"},
		Now:         func() time.Time { return time.Unix(10, 0).UTC() },
	}

	err := syncer.PushSettings(context.Background(), map[string]any{
		"pages_app": map[string]any{"first_book": 10},
		"ui":        map[string]any{"theme": "gruvbox"},
	})
	if err != nil {
		t.Fatalf("PushSettings() error = %v", err)
	}

	if len(provider.mutations) != 1 || provider.mutations[0].Settings == nil {
		t.Fatalf("mutations = %#v, want one settings mutation", provider.mutations)
	}
	if _, ok := provider.mutations[0].Settings.Values["ui"]; ok {
		t.Fatalf("shared settings included local-only ui: %#v", provider.mutations[0].Settings.Values)
	}
}

func TestSettingsSyncerPullReturnsNewerSharedSettings(t *testing.T) {
	provider := &fakeAssetProvider{snapshot: Snapshot{Settings: map[string]any{
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
