package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kloneets/tools/src/todo"
)

func TestGoldenSyncHashes(t *testing.T) {
	var fixture struct {
		TodoStore      todo.Store            `json:"todo_store"`
		NoteRecords    map[string]NoteRecord `json:"note_records"`
		SharedSettings map[string]any        `json:"shared_settings"`
		Expected       struct {
			TodoStoreHash         string `json:"todo_store_hash"`
			TodoArchiveMonthsHash string `json:"todo_archive_months_hash"`
			NoteMetadataHash      string `json:"note_metadata_hash"`
			SharedSettingsHash    string `json:"shared_settings_hash"`
		} `json:"expected"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "golden_sync_fixture.json"))
	if err != nil {
		t.Fatalf("ReadFile(golden fixture) error = %v", err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("Unmarshal(golden fixture) error = %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"todo store", TodoStoreHash(fixture.TodoStore), fixture.Expected.TodoStoreHash},
		{"todo archive months", TodoArchiveMonthsHash(fixture.TodoStore.ArchiveMonths), fixture.Expected.TodoArchiveMonthsHash},
		{"note metadata", NoteMetadataHash(fixture.NoteRecords), fixture.Expected.NoteMetadataHash},
		{"shared settings", sharedSettingsHash(fixture.SharedSettings), fixture.Expected.SharedSettingsHash},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("hash = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
