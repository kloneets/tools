package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kloneets/tools/src/helpers"
)

func TestDefaultSettings(t *testing.T) {
	got := defaultSettings()
	if !got.PasswordApp.Letters || !got.PasswordApp.Numbers || !got.PasswordApp.SpecialSymbols {
		t.Fatal("expected password defaults to enable all character groups")
	}
	if got.PasswordApp.SymbolCount != 16 {
		t.Fatalf("SymbolCount = %d, want 16", got.PasswordApp.SymbolCount)
	}
	if got.NotesApp.TabSpaces != 4 {
		t.Fatalf("TabSpaces = %d, want 4", got.NotesApp.TabSpaces)
	}
	if got.NotesApp.UndoLevels != 1000 {
		t.Fatalf("UndoLevels = %d, want 1000", got.NotesApp.UndoLevels)
	}
	if !got.NotesApp.SidebarVisible {
		t.Fatal("SidebarVisible = false, want true")
	}
	if !got.NotesApp.VimMode {
		t.Fatal("VimMode = false, want true")
	}
	if got.NotesApp.SpellCheckEnabled {
		t.Fatal("SpellCheckEnabled = true, want false")
	}
	if len(got.NotesApp.SpellDictionaries) != 0 {
		t.Fatalf("SpellDictionaries = %v, want none", got.NotesApp.SpellDictionaries)
	}
	if got.UI == nil || got.UI.TransparentBackground {
		t.Fatalf("TransparentBackground = %v, want false", got.UI != nil && got.UI.TransparentBackground)
	}
	if got.UI.Theme != DefaultTheme {
		t.Fatalf("Theme = %q, want %q", got.UI.Theme, DefaultTheme)
	}
	if got := got.UI.TabOrder; strings.Join(got, ",") != strings.Join(DefaultTabOrder(), ",") {
		t.Fatalf("TabOrder = %v, want default order", got)
	}
	if got.Firebase == nil || !got.Firebase.Realtime || got.Firebase.Enabled {
		t.Fatalf("Firebase = %#v, want disabled realtime defaults", got.Firebase)
	}
}

func TestNormalizeSettings(t *testing.T) {
	cfg := &UserSettings{}
	normalizeSettings(cfg)
	if cfg.Firebase == nil || !cfg.Firebase.Realtime {
		t.Fatalf("Firebase = %#v, want initialized realtime settings", cfg.Firebase)
	}
	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want all widgets visible", cfg.UI)
	}
	if cfg.UI.Theme != DefaultTheme {
		t.Fatalf("Theme = %q, want %q", cfg.UI.Theme, DefaultTheme)
	}
	if got := cfg.UI.TabOrder; strings.Join(got, ",") != strings.Join(DefaultTabOrder(), ",") {
		t.Fatalf("TabOrder = %v, want default order", got)
	}
	if cfg.NotesApp.TabSpaces != 4 {
		t.Fatalf("TabSpaces = %d, want 4", cfg.NotesApp.TabSpaces)
	}
	if cfg.NotesApp.UndoLevels != 1000 {
		t.Fatalf("UndoLevels = %d, want 1000", cfg.NotesApp.UndoLevels)
	}
	if !cfg.NotesApp.SidebarVisible {
		t.Fatal("SidebarVisible = false, want true")
	}
	cfg.NotesApp.SpellDictionaries = []string{"EN", "lv", "en", " "}
	normalizeSettings(cfg)
	if got := cfg.NotesApp.SpellDictionaries; len(got) != 2 || got[0] != "en" || got[1] != "lv" {
		t.Fatalf("SpellDictionaries = %v, want normalized unique codes", got)
	}
}

func TestNormalizeTabOrderRepairsUnknownDuplicatesAndMissing(t *testing.T) {
	got := NormalizeTabOrder([]string{" sync ", "notes", "bogus", "sync", "todo"})
	want := []string{"sync", "notes", "todo", "pages", "password", "settings"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("NormalizeTabOrder() = %v, want %v", got, want)
	}
}

func TestNormalizeSettingsRepairsMobileSnapshotDefaults(t *testing.T) {
	var cfg UserSettings
	if err := json.Unmarshal([]byte(`{
		"pages_app":{"first_book":10},
		"notes_app":{"current_note_path":"mobile.md"},
		"gdrive":{"folder_id":"folder-1"}
	}`), &cfg); err != nil {
		t.Fatalf("Unmarshal mobile settings error = %v", err)
	}

	normalizeSettings(&cfg)

	if !cfg.PasswordApp.Letters || !cfg.PasswordApp.Numbers || !cfg.PasswordApp.SpecialSymbols {
		t.Fatalf("PasswordApp = %#v, want default character groups enabled", cfg.PasswordApp)
	}
	if cfg.PasswordApp.SymbolCount != 16 {
		t.Fatalf("SymbolCount = %d, want 16", cfg.PasswordApp.SymbolCount)
	}
	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want default visible tools", cfg.UI)
	}
	if cfg.AppWindow.Width != 600 || cfg.AppWindow.Height != 300 {
		t.Fatalf("AppWindow = %#v, want default dimensions", cfg.AppWindow)
	}
	if cfg.NotesApp.TabSpaces != 4 || cfg.NotesApp.UndoLevels != 1000 {
		t.Fatalf("NotesApp = %#v, want editor defaults", cfg.NotesApp)
	}
}

func TestUISettingsUnmarshalDefaultsMissingVisibilityFields(t *testing.T) {
	var cfg UserSettings
	if err := json.Unmarshal([]byte(`{"ui":{"theme":"gruvbox"}}`), &cfg); err != nil {
		t.Fatalf("Unmarshal UI settings error = %v", err)
	}

	normalizeSettings(&cfg)

	if cfg.UI == nil || !cfg.UI.ShowNotes || !cfg.UI.ShowPages || !cfg.UI.ShowPassword {
		t.Fatalf("UI = %#v, want missing visibility fields to default true", cfg.UI)
	}
	if cfg.UI.Theme != "gruvbox" {
		t.Fatalf("Theme = %q, want gruvbox", cfg.UI.Theme)
	}
}

func TestNormalizeSettingsResetsUnknownTheme(t *testing.T) {
	cfg := &UserSettings{UI: &UISettings{Theme: "unknown"}}
	normalizeSettings(cfg)
	if cfg.UI.Theme != DefaultTheme {
		t.Fatalf("Theme = %q, want %q", cfg.UI.Theme, DefaultTheme)
	}
}

func TestThemePersistsInJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.UI.Theme = "catppuccin"
	writeSettingsToDisk(false)
	data, err := os.ReadFile(fileName())
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !strings.Contains(string(data), `"theme":"catppuccin"`) {
		t.Fatalf("settings file = %q, want theme persisted", string(data))
	}
}

func TestInitMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settingsInstance = nil
	messages := Init()
	if settingsInstance == nil {
		t.Fatal("settingsInstance should be initialized")
	}
	if len(*messages) != 1 || !strings.Contains((*messages)[0], "Settings read error") {
		t.Fatalf("Init() messages = %#v, want read error", *messages)
	}
}

func TestPersistedNotesEditorWidth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.NotesApp.EditorWidth = 41
	writeSettingsToDisk(false)
	settingsInstance.NotesApp.EditorWidth = 0
	if got := PersistedNotesEditorWidth(); got != 41 {
		t.Fatalf("PersistedNotesEditorWidth() = %d, want 41", got)
	}
}

func TestSaveNotesEditorWidth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	SaveNotesEditorWidth(33)
	if settingsInstance.NotesApp.EditorWidth != 33 {
		t.Fatalf("EditorWidth = %d, want 33", settingsInstance.NotesApp.EditorWidth)
	}
	if _, err := os.Stat(fileName()); !os.IsNotExist(err) {
		t.Fatalf("settings file should not be written automatically, stat err = %v", err)
	}
}

func TestSaveNotesPreviewHiddenPersistsLocally(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	SaveNotesPreviewHidden(true)
	if !settingsInstance.NotesApp.PreviewHidden {
		t.Fatal("PreviewHidden = false, want true")
	}
	data, err := os.ReadFile(fileName())
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !strings.Contains(string(data), `"preview_hidden":true`) {
		t.Fatalf("settings file = %q, want preview_hidden persisted", string(data))
	}
}

func TestSaveSettingsLocalPersistsPreviewAndTabOrderAcrossInit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	settingsInstance.NotesApp.PreviewHidden = true
	settingsInstance.UI.TabOrder = []string{"todo", "notes", "sync", "settings", "pages", "password"}

	SaveSettingsLocal()
	settingsInstance = nil
	Init()

	if !settingsInstance.NotesApp.PreviewHidden {
		t.Fatal("PreviewHidden = false after Init, want true")
	}
	wantOrder := []string{"todo", "notes", "sync", "settings", "pages", "password"}
	if got := settingsInstance.UI.TabOrder; strings.Join(got, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("TabOrder = %v after Init, want %v", got, wantOrder)
	}
}

func TestInitRecoversSettingsFromValidBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Dir(fileName()), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir) error = %v", err)
	}
	if err := os.WriteFile(fileName(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile(empty settings) error = %v", err)
	}
	backup := getFileName("settings.json_bup")
	backupSettings := defaultSettings()
	backupSettings.NotesApp.PreviewHidden = true
	backupSettings.UI.TabOrder = []string{"todo", "notes", "sync", "settings", "pages", "password"}
	data, err := json.Marshal(backupSettings)
	if err != nil {
		t.Fatalf("Marshal(backup settings) error = %v", err)
	}
	if err := os.WriteFile(backup, data, 0o644); err != nil {
		t.Fatalf("WriteFile(settings backup) error = %v", err)
	}

	settingsInstance = nil
	messages := Init()

	if !settingsInstance.NotesApp.PreviewHidden {
		t.Fatal("PreviewHidden = false after recovery, want true")
	}
	wantOrder := []string{"todo", "notes", "sync", "settings", "pages", "password"}
	if got := settingsInstance.UI.TabOrder; strings.Join(got, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("TabOrder = %v after recovery, want %v", got, wantOrder)
	}
	if !containsMessage(*messages, "Settings recovered from backup") {
		t.Fatalf("Init() messages = %#v, want recovery message", *messages)
	}
	persisted, err := os.ReadFile(fileName())
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if len(persisted) == 0 {
		t.Fatal("settings.json is empty after recovery")
	}
}

func TestSaveNotesSessionPersistsLocally(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsInstance = defaultSettings()
	root := getFileName("notes")
	SaveNotesSession([]string{
		filepath.Join(root, "Note 1.md"),
		filepath.Join(root, "Projects", "Note 2.md"),
		filepath.Join(root, "Note 1.md"),
	}, filepath.Join(root, "Projects", "Note 2.md"))
	if got := settingsInstance.NotesApp.OpenNotePaths; len(got) != 2 || got[0] != "Note 1.md" || got[1] != "Projects/Note 2.md" {
		t.Fatalf("OpenNotePaths = %#v", got)
	}
	if got := settingsInstance.NotesApp.CurrentNotePath; got != "Projects/Note 2.md" {
		t.Fatalf("CurrentNotePath = %q, want Projects/Note 2.md", got)
	}
}

func TestNormalizeSettingsConvertsPortableAbsoluteNoteSessionPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := defaultSettings()
	s.NotesApp.OpenNotePaths = []string{
		"/old-home/.config/koko-tools/notes/Note 1.md",
		"/old-home/.config/koko-tools/notes/Projects/Note 2.md",
	}
	s.NotesApp.CurrentNotePath = "/old-home/.config/koko-tools/notes/Projects/Note 2.md"

	normalizeSettings(s)

	if got := s.NotesApp.OpenNotePaths; len(got) != 2 || got[0] != "Note 1.md" || got[1] != "Projects/Note 2.md" {
		t.Fatalf("OpenNotePaths = %#v", got)
	}
	if got := s.NotesApp.CurrentNotePath; got != "Projects/Note 2.md" {
		t.Fatalf("CurrentNotePath = %q, want Projects/Note 2.md", got)
	}
}

func TestGetFileNameUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := getFileName("settings.json")
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "settings.json")
	if got != want {
		t.Fatalf("getFileName() = %q, want %q", got, want)
	}
}

func containsMessage(messages []string, substr string) bool {
	for _, message := range messages {
		if strings.Contains(message, substr) {
			return true
		}
	}
	return false
}
