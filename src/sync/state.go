package sync

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kloneets/tools/src/helpers"
)

type State struct {
	Provider       string           `json:"provider"`
	WorkspaceID    string           `json:"workspace_id"`
	LastEventToken string           `json:"last_event_token"`
	DeviceID       string           `json:"device_id"`
	Notes          map[string]int64 `json:"notes"`
	Todos          map[string]int64 `json:"todos"`
}

type TokenFile struct {
	UID          string `json:"uid"`
	Email        string `json:"email"`
	RefreshToken string `json:"refresh_token"`
}

func StatePath() string {
	return configPath("sync_state.json")
}

func TokenPath() string {
	return configPath("firebase_token.json")
}

func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultState(), nil
		}
		return State{}, fmt.Errorf("read sync state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode sync state: %w", err)
	}
	normalizeState(&state)
	return state, nil
}

func SaveState(path string, state State) error {
	normalizeState(&state)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create sync state directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write sync state: %w", err)
	}
	return nil
}

func LoadToken(path string) (TokenFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TokenFile{}, fmt.Errorf("read firebase token: %w", err)
	}
	var token TokenFile
	if err := json.Unmarshal(data, &token); err != nil {
		return TokenFile{}, fmt.Errorf("decode firebase token: %w", err)
	}
	token.Email = strings.TrimSpace(token.Email)
	token.UID = strings.TrimSpace(token.UID)
	token.RefreshToken = strings.TrimSpace(token.RefreshToken)
	return token, nil
}

func SaveToken(path string, token TokenFile) error {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal firebase token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create firebase token directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write firebase token: %w", err)
	}
	return nil
}

func defaultState() State {
	return State{
		Provider: ProviderFirebase,
		DeviceID: randomDeviceID(),
		Notes:    map[string]int64{},
		Todos:    map[string]int64{},
	}
}

func normalizeState(state *State) {
	if state.Provider == "" {
		state.Provider = ProviderFirebase
	}
	if state.DeviceID == "" {
		state.DeviceID = randomDeviceID()
	}
	if state.Notes == nil {
		state.Notes = map[string]int64{}
	}
	if state.Todos == nil {
		state.Todos = map[string]int64{}
	}
}

func randomDeviceID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "device-local"
	}
	return "device-" + hex.EncodeToString(buf[:])
}

func configPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, name)
}
