package sync

import (
	"context"
	"testing"
	"time"
)

func TestAssetSyncerPushSkipsOversizedAssets(t *testing.T) {
	provider := &fakeAssetProvider{}
	syncer := AssetSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{UID: "uid", IDToken: "token"},
		Now:         func() time.Time { return time.Unix(10, 0).UTC() },
	}

	result, err := syncer.PushAssets(context.Background(), []LocalAsset{
		{Path: "note.assets/small.txt", Bytes: []byte("ok")},
		{Path: "note.assets/large.bin", Bytes: make([]byte, MaxAssetBytes+1)},
	})
	if err != nil {
		t.Fatalf("PushAssets() error = %v", err)
	}

	if result.Pushed != 1 {
		t.Fatalf("Pushed = %d, want 1", result.Pushed)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "note.assets/large.bin" {
		t.Fatalf("Skipped = %#v, want oversized asset path", result.Skipped)
	}
	if len(provider.mutations) != 1 || provider.mutations[0].Asset == nil {
		t.Fatalf("mutations = %#v, want one asset mutation", provider.mutations)
	}
}

func TestAssetSyncerPullAppliesNewerRemoteAssets(t *testing.T) {
	provider := &fakeAssetProvider{snapshot: Snapshot{Assets: map[string]AssetRecord{
		"asset": {
			ID:          "asset",
			Path:        "note.assets/image.txt",
			BytesBase64: "aGVsbG8=",
			SHA256:      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
			Rev:         5,
		},
	}}}
	syncer := AssetSyncer{
		Provider:    provider,
		WorkspaceID: "workspace",
		StatePath:   t.TempDir() + "/state.json",
		Session:     Session{UID: "uid", IDToken: "token"},
	}

	result, err := syncer.PullAssets(context.Background())
	if err != nil {
		t.Fatalf("PullAssets() error = %v", err)
	}

	if !result.Changed || len(result.Upserts) != 1 {
		t.Fatalf("result = %#v, want one changed upsert", result)
	}
	if string(result.Upserts[0].Bytes) != "hello" {
		t.Fatalf("upsert bytes = %q, want hello", string(result.Upserts[0].Bytes))
	}
}

type fakeAssetProvider struct {
	snapshot  Snapshot
	mutations []Mutation
}

func (p *fakeAssetProvider) Login(context.Context, string, string) (Session, error) {
	return Session{}, nil
}
func (p *fakeAssetProvider) WatchWorkspace(context.Context, string, string, func(Change) error) error {
	return nil
}
func (p *fakeAssetProvider) PushMutation(_ context.Context, _ string, mutation Mutation) error {
	p.mutations = append(p.mutations, mutation)
	return nil
}
func (p *fakeAssetProvider) PullSnapshot(context.Context, string) (Snapshot, error) {
	return p.snapshot, nil
}
func (p *fakeAssetProvider) CreateWorkspace(context.Context, string) (WorkspaceMeta, error) {
	return WorkspaceMeta{}, nil
}
func (p *fakeAssetProvider) GrantMember(context.Context, string, string, string) error {
	return nil
}
func (p *fakeAssetProvider) RevokeMember(context.Context, string, string) error {
	return nil
}
