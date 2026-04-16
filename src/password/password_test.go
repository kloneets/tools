package password

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/settings"
)

func TestBuildCharPoolIncludesSelectedGroups(t *testing.T) {
	pool := buildCharPool(true, true, true)

	for _, want := range []string{"a", "z", "A", "Z", "0", "9", "!", "~"} {
		if !strings.Contains(pool, want) {
			t.Fatalf("expected pool to contain %q", want)
		}
	}
}

func TestBuildCharPoolEmptyWhenNothingSelected(t *testing.T) {
	if got := buildCharPool(false, false, false); got != "" {
		t.Fatalf("buildCharPool() = %q, want empty string", got)
	}
}

func TestGeneratePasswordUsesPoolAndLength(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	got := generatePassword("ab", 8, random)

	if len(got) != 8 {
		t.Fatalf("generatePassword() length = %d, want 8", len(got))
	}
	for _, ch := range got {
		if ch != 'a' && ch != 'b' {
			t.Fatalf("generatePassword() produced rune %q outside pool", ch)
		}
	}
}

func TestGeneratePersistsSettingsLocally(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "secret-1")
	if err := os.MkdirAll(filepath.Dir(gdrive.TokenPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(token dir) error = %v", err)
	}
	if err := os.WriteFile(gdrive.TokenPath(), []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}
	settings.Init()
	settings.Inst().GDrive.Enabled = true
	settings.Inst().GDrive.FolderID = "folder-1"
	settings.Inst().GDrive.PendingSync = false

	m := NewModel()
	m.Letters = true
	m.Numbers = false
	m.SpecialSymbols = false
	m.SymbolCount = 12
	m.Generate()

	if !settings.Inst().PasswordApp.Letters || settings.Inst().PasswordApp.Numbers || settings.Inst().PasswordApp.SpecialSymbols {
		t.Fatalf("password settings = %#v, want local updates applied", settings.Inst().PasswordApp)
	}
	if settings.Inst().PasswordApp.SymbolCount != 12 {
		t.Fatalf("SymbolCount = %d, want 12", settings.Inst().PasswordApp.SymbolCount)
	}
	if settings.Inst().GDrive.PendingSync {
		t.Fatal("PendingSync = true, want false after password-only change")
	}
}
