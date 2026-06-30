package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SettingsSyncer struct {
	Provider    Provider
	WorkspaceID string
	StatePath   string
	Session     Session
	DeviceID    string
	Now         func() time.Time
}

type SettingsPullResult struct {
	Values  map[string]any
	Changed bool
}

type SettingsPushResult struct {
	Pushed bool
}

func (s *SettingsSyncer) Ready() bool {
	return s != nil && s.Provider != nil && s.WorkspaceID != "" && s.Session.IDToken != ""
}

func (s *SettingsSyncer) PushSettings(ctx context.Context, fullSettings map[string]any) (SettingsPushResult, error) {
	if !s.Ready() {
		return SettingsPushResult{}, nil
	}
	shared := SharedWorkspaceSettings(fullSettings)
	if len(shared) == 0 {
		return SettingsPushResult{}, nil
	}
	hash := sharedSettingsHash(shared)
	state, err := LoadState(s.StatePath)
	if err != nil {
		return SettingsPushResult{}, err
	}
	if state.WorkspaceID == s.WorkspaceID && state.SettingsHash == hash {
		return SettingsPushResult{}, nil
	}
	now := s.now()
	rev := now.UnixMilli()
	if rev <= state.SettingsRev {
		rev = state.SettingsRev + 1
	}
	record := SharedSettingsRecord{
		Values:    shared,
		Rev:       rev,
		UpdatedAt: now,
		UpdatedBy: s.Session.UID,
	}
	if err := s.Provider.PushMutation(ctx, s.WorkspaceID, Mutation{
		EventID:   fmt.Sprintf("settings-%d", rev),
		DeviceID:  s.deviceID(state),
		Settings:  &record,
		CreatedAt: now,
	}); err != nil {
		return SettingsPushResult{}, err
	}
	state.SettingsRev = rev
	state.SettingsHash = hash
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if err := SaveState(s.StatePath, state); err != nil {
		return SettingsPushResult{}, err
	}
	return SettingsPushResult{Pushed: true}, nil
}

func (s *SettingsSyncer) PullSettings(ctx context.Context) (SettingsPullResult, error) {
	if !s.Ready() {
		return SettingsPullResult{}, nil
	}
	state, err := LoadState(s.StatePath)
	if err != nil {
		return SettingsPullResult{}, err
	}
	settings, err := pullRemoteSettings(ctx, s.Provider, s.WorkspaceID)
	if err != nil {
		return SettingsPullResult{}, err
	}
	record, ok := sharedSettingsFromSnapshot(settings)
	if !ok || (state.WorkspaceID == s.WorkspaceID && record.Rev <= state.SettingsRev) {
		return SettingsPullResult{}, nil
	}
	state.SettingsRev = record.Rev
	state.SettingsHash = sharedSettingsHash(record.Values)
	state.WorkspaceID = s.WorkspaceID
	state.Provider = ProviderFirebase
	if err := SaveState(s.StatePath, state); err != nil {
		return SettingsPullResult{}, err
	}
	return SettingsPullResult{Values: normalizeJSONMap(record.Values), Changed: true}, nil
}

func pullRemoteSettings(ctx context.Context, provider Provider, workspaceID string) (map[string]any, error) {
	if p, ok := provider.(SettingsPullProvider); ok {
		return p.PullSettings(ctx, workspaceID)
	}
	snapshot, err := provider.PullSnapshot(ctx, workspaceID)
	return snapshot.Settings, err
}

func sharedSettingsHash(values map[string]any) string {
	normalized := normalizeJSONMap(values)
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *SettingsSyncer) deviceID(state State) string {
	if strings.TrimSpace(s.DeviceID) != "" {
		return s.DeviceID
	}
	return state.DeviceID
}

func (s *SettingsSyncer) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeJSONMap(values map[string]any) map[string]any {
	data, err := json.Marshal(values)
	if err != nil {
		return values
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return values
	}
	return out
}
