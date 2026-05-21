package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kloneets/tools/src/helpers"
)

type FirebaseConfig struct {
	Enabled     bool   `json:"enabled"`
	Realtime    bool   `json:"realtime"`
	APIKey      string `json:"api_key"`
	DatabaseURL string `json:"database_url"`
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email,omitempty"`
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "firebase_config.json"
	}
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "firebase_config.json")
}

func LoadConfig(path string) (FirebaseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FirebaseConfig{}, nil
		}
		return FirebaseConfig{}, fmt.Errorf("read firebase config: %w", err)
	}
	var cfg FirebaseConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FirebaseConfig{}, fmt.Errorf("decode firebase config: %w", err)
	}
	if cfg.Realtime == false {
		cfg.Realtime = true
	}
	return cfg, nil
}

func SaveConfig(path string, cfg FirebaseConfig) error {
	if cfg.Realtime == false {
		cfg.Realtime = true
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal firebase config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create firebase config directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write firebase config: %w", err)
	}
	return nil
}
