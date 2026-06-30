package sync

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MaxAssetBytes = 1 << 20

type AssetSyncer struct {
	Provider    Provider
	WorkspaceID string
	StatePath   string
	Session     Session
	DeviceID    string
	Now         func() time.Time
}

type LocalAsset struct {
	ID      string
	Path    string
	Bytes   []byte
	ModTime time.Time
}

type AssetPullResult struct {
	Upserts []LocalAsset
	Deletes []string
	Changed bool
	State   State
}

type AssetPushResult struct {
	Pushed  int
	Skipped []string
}

func AssetID(path string) string {
	sum := sha256.Sum256([]byte(NormalizeAssetPath(path)))
	return hex.EncodeToString(sum[:])
}

func NormalizeAssetPath(path string) string {
	return NormalizeNotePath(path)
}

func (s *AssetSyncer) Ready() bool {
	return s != nil && s.Provider != nil && s.WorkspaceID != "" && s.Session.IDToken != ""
}

func (s *AssetSyncer) PushAssets(ctx context.Context, assets []LocalAsset) (AssetPushResult, error) {
	result := AssetPushResult{}
	if !s.Ready() {
		return result, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return result, err
	}
	deviceID := s.deviceID(state)
	now := s.now()
	sort.SliceStable(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	for _, asset := range assets {
		path := NormalizeAssetPath(asset.Path)
		if path == "" {
			continue
		}
		if len(asset.Bytes) > MaxAssetBytes {
			result.Skipped = append(result.Skipped, path)
			continue
		}
		id := asset.ID
		if id == "" {
			id = AssetID(path)
		}
		revTime := asset.ModTime
		if revTime.IsZero() {
			revTime = now
		}
		rev := revTime.UTC().UnixMilli()
		if rev <= state.Assets[id] {
			rev = state.Assets[id] + 1
		}
		record := AssetRecord{
			ID:          id,
			Path:        path,
			BytesBase64: base64.StdEncoding.EncodeToString(asset.Bytes),
			SHA256:      assetSHA256(asset.Bytes),
			MIME:        assetMIME(path),
			Rev:         rev,
			UpdatedAt:   now,
			UpdatedBy:   s.Session.UID,
		}
		if err := s.Provider.PushMutation(ctx, s.WorkspaceID, Mutation{
			EventID:   fmt.Sprintf("asset-%s-%d", id, rev),
			DeviceID:  deviceID,
			Asset:     &record,
			CreatedAt: now,
		}); err != nil {
			return result, err
		}
		state.Assets[id] = rev
		result.Pushed++
	}
	records := make(map[string]AssetRecord, len(assets))
	for _, asset := range assets {
		path := NormalizeAssetPath(asset.Path)
		if path == "" || len(asset.Bytes) > MaxAssetBytes {
			continue
		}
		id := asset.ID
		if id == "" {
			id = AssetID(path)
		}
		records[id] = AssetRecord{ID: id, Path: path, Rev: state.Assets[id], SHA256: assetSHA256(asset.Bytes)}
	}
	featureHash := AssetMetadataHash(records)
	markFeaturePulled(&state, SyncFeatureAssets, featureHash, now)
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureAssets, featureHash, now, s.Session.UID)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	return result, SaveState(s.StatePath, state)
}

func (s *AssetSyncer) PullAssets(ctx context.Context) (AssetPullResult, error) {
	result := AssetPullResult{}
	if !s.Ready() {
		return result, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return result, err
	}
	now := s.now()
	if hashes, ok := pullSyncHashes(ctx, s.Provider, s.WorkspaceID); ok && shouldSkipFeaturePull(state, s.WorkspaceID, SyncFeatureAssets, hashes[SyncFeatureAssets], now) {
		result.State = state
		return result, nil
	}
	remoteAssets, err := pullRemoteAssets(ctx, s.Provider, s.WorkspaceID)
	if err != nil {
		return result, err
	}
	ids := make([]string, 0, len(remoteAssets))
	for id := range remoteAssets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		remote := remoteAssets[id]
		if remote.Rev <= state.Assets[id] {
			continue
		}
		path := NormalizeAssetPath(remote.Path)
		if path == "" {
			continue
		}
		if remote.Deleted {
			result.Deletes = append(result.Deletes, path)
			state.Assets[id] = remote.Rev
			result.Changed = true
			continue
		}
		data, err := base64.StdEncoding.DecodeString(remote.BytesBase64)
		if err != nil {
			continue
		}
		if len(data) > MaxAssetBytes || remote.SHA256 != "" && remote.SHA256 != assetSHA256(data) {
			continue
		}
		result.Upserts = append(result.Upserts, LocalAsset{ID: id, Path: path, Bytes: data})
		state.Assets[id] = remote.Rev
		result.Changed = true
	}
	featureHash := AssetMetadataHash(remoteAssets)
	markFeaturePulled(&state, SyncFeatureAssets, featureHash, now)
	pushSyncHashBestEffort(ctx, s.Provider, s.WorkspaceID, SyncFeatureAssets, featureHash, now, s.Session.UID)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	result.State = state
	return result, nil
}

func pullRemoteAssets(ctx context.Context, provider Provider, workspaceID string) (map[string]AssetRecord, error) {
	if p, ok := provider.(AssetPullProvider); ok {
		return p.PullAssets(ctx, workspaceID)
	}
	snapshot, err := provider.PullSnapshot(ctx, workspaceID)
	return snapshot.Assets, err
}

func (s *AssetSyncer) SaveState(state State) error {
	return SaveState(s.StatePath, state)
}

func (s *AssetSyncer) deviceID(state State) string {
	if strings.TrimSpace(s.DeviceID) != "" {
		return s.DeviceID
	}
	return state.DeviceID
}

func (s *AssetSyncer) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func assetSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assetMIME(path string) string {
	if typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); typ != "" {
		return typ
	}
	return "application/octet-stream"
}
