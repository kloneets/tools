package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kloneets/tools/src/helpers"
)

type UserSettings struct {
	PasswordApp PasswordAppSettings `json:"password_app"`
	PagesApp    PagesAppSettings    `json:"pages_app"`
	NotesApp    NotesAppSettings    `json:"notes_app"`
	AppWindow   AppWindowSettings   `json:"app_window"`
	UI          *UISettings         `json:"ui"`
	Firebase    *FirebaseSettings   `json:"firebase,omitempty"`
}

type AppWindowSettings struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

type PagesAppSettings struct {
	FirstBookPages  int `json:"first_book"`
	SecondBookPages int `json:"second_book"`
	ReadPages       int `json:"read_pages"`
}

type PasswordAppSettings struct {
	Letters        bool `json:"letters"`
	Numbers        bool `json:"numbers"`
	SpecialSymbols bool `json:"special_symbols"`
	SymbolCount    int  `json:"symbol_count"`
}

type NotesAppSettings struct {
	TabSpaces         int      `json:"tab_spaces"`
	UndoLevels        int      `json:"undo_levels"`
	EditorWidth       int      `json:"editor_width,omitempty"`
	PreviewHidden     bool     `json:"preview_hidden,omitempty"`
	OpenNotePaths     []string `json:"open_note_paths,omitempty"`
	CurrentNotePath   string   `json:"current_note_path,omitempty"`
	SidebarVisible    bool     `json:"sidebar_visible"`
	VimMode           bool     `json:"vim_mode"`
	SpellCheckEnabled bool     `json:"spell_check_enabled,omitempty"`
	SpellDictionaries []string `json:"spell_dictionaries,omitempty"`
}

func (n *NotesAppSettings) UnmarshalJSON(data []byte) error {
	type notesAppAlias NotesAppSettings
	aux := struct {
		SidebarVisible *bool `json:"sidebar_visible"`
		*notesAppAlias
	}{
		notesAppAlias: (*notesAppAlias)(n),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.SidebarVisible == nil {
		n.SidebarVisible = true
	}
	return nil
}

type UISettings struct {
	ShowPages             bool     `json:"show_pages"`
	ShowPassword          bool     `json:"show_password"`
	ShowNotes             bool     `json:"show_notes"`
	Theme                 string   `json:"theme,omitempty"`
	TransparentBackground bool     `json:"transparent_background,omitempty"`
	TabOrder              []string `json:"tab_order,omitempty"`
}

func (u *UISettings) UnmarshalJSON(data []byte) error {
	type uiAlias UISettings
	aux := struct {
		ShowPages    *bool `json:"show_pages"`
		ShowPassword *bool `json:"show_password"`
		ShowNotes    *bool `json:"show_notes"`
		*uiAlias
	}{
		uiAlias: (*uiAlias)(u),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.ShowPages == nil {
		u.ShowPages = true
	}
	if aux.ShowPassword == nil {
		u.ShowPassword = true
	}
	if aux.ShowNotes == nil {
		u.ShowNotes = true
	}
	return nil
}

type FirebaseSettings struct {
	Enabled         bool   `json:"enabled"`
	Realtime        bool   `json:"realtime"`
	APIKey          string `json:"api_key,omitempty"`
	DatabaseURL     string `json:"database_url,omitempty"`
	UserEmail       string `json:"user_email,omitempty"`
	WorkspaceID     string `json:"workspace_id"`
	WorkspaceName   string `json:"workspace_name"`
	LastSyncAt      string `json:"last_sync_at"`
	LastSyncStatus  string `json:"last_sync_status"`
	LastSyncMessage string `json:"last_sync_message"`
}

var settingsInstance *UserSettings
var saveHooks []func(*UserSettings)
var shuttingDown atomic.Bool
var statusUpdater = func(text string) {
	if helpers.HasStatusBar() {
		helpers.StatusBarInst().UpdateStatusBar(text)
	}
}

const DefaultTheme = "tokyo-night"

var defaultTabOrder = []string{"notes", "files", "pages", "password", "todo", "sync", "settings"}

var BuiltInThemes = []string{
	"tokyo-night",
	"catppuccin",
	"kanagawa",
	"gruvbox",
	"rose-pine",
	"flexoki",
}

func Inst() *UserSettings {
	if settingsInstance == nil {
		log.Fatal("Settings is not initialized")
	}

	return settingsInstance
}

func Init() *[]string {
	shuttingDown.Store(false)
	var messages []string
	fn := fileName()
	c, err := os.ReadFile(fn)
	if err != nil {
		msg := fmt.Sprintf("Settings read error: %s", err)
		log.Println(msg)
		messages = append(messages, msg)
		settingsInstance = defaultSettings()
		normalizeSettings(settingsInstance)
		return &messages
	}

	loaded, marshalError := decodeSettings(c)
	if marshalError != nil {
		msg := fmt.Sprintf("Settings unmarshal error: %s", marshalError)
		log.Println(msg)
		messages = append(messages, msg)
		if backupFileName, backupErr := backupInvalidSettings(fn); backupErr != nil {
			log.Println("Cannot back up settings: ", backupErr)
		} else {
			msg := "Old settings backed up to: " + backupFileName
			messages = append(messages, msg)
			log.Println(msg)
		}
		if recovered, backupFileName, recoverErr := recoverSettingsFromBackup(); recoverErr == nil {
			settingsInstance = recovered
			msg := "Settings recovered from backup: " + backupFileName
			messages = append(messages, msg)
			log.Println(msg)
			writeSettingsToDisk(false)
			return &messages
		}
		settingsInstance = defaultSettings()
	} else {
		settingsInstance = loaded
	}

	normalizeSettings(settingsInstance)

	return &messages
}

func decodeSettings(data []byte) (*UserSettings, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, errors.New("settings file is empty")
	}
	if trimmed == "null" {
		return nil, errors.New("settings file is null")
	}
	var loaded UserSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	normalizeSettings(&loaded)
	return &loaded, nil
}

func backupInvalidSettings(path string) (string, error) {
	backupFileName := getFileName(time.Now().Format("2006-01-02_15.04.0000") + "settings.json")
	return backupFileName, os.Rename(path, backupFileName)
}

func recoverSettingsFromBackup() (*UserSettings, string, error) {
	candidates, err := settingsBackupCandidates()
	if err != nil {
		return nil, "", err
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate.path)
		if err != nil {
			log.Println("Cannot read settings backup: ", err)
			continue
		}
		settings, err := decodeSettings(data)
		if err != nil {
			log.Println("Cannot decode settings backup: ", candidate.path, err)
			continue
		}
		return settings, candidate.path, nil
	}
	return nil, "", errors.New("no valid settings backup found")
}

type settingsBackupCandidate struct {
	path    string
	modTime time.Time
}

func settingsBackupCandidates() ([]settingsBackupCandidate, error) {
	dir := filepath.Dir(fileName())
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	candidates := make([]settingsBackupCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "settings.json" || !strings.Contains(name, "settings.json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, settingsBackupCandidate{
			path:    filepath.Join(dir, name),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates, nil
}

func defaultSettings() *UserSettings {
	return &UserSettings{
		PasswordApp: PasswordAppSettings{
			Letters:        true,
			Numbers:        true,
			SpecialSymbols: true,
			SymbolCount:    16,
		},
		PagesApp: PagesAppSettings{
			FirstBookPages:  0,
			SecondBookPages: 0,
			ReadPages:       0,
		},
		AppWindow: AppWindowSettings{
			Width:  600,
			Height: 300,
		},
		NotesApp: NotesAppSettings{
			TabSpaces:      4,
			UndoLevels:     1000,
			SidebarVisible: true,
			VimMode:        true,
		},
		UI:       defaultUISettings(),
		Firebase: defaultFirebaseSettings(),
	}
}

func defaultFirebaseSettings() *FirebaseSettings {
	return &FirebaseSettings{
		Realtime: true,
	}
}

func defaultUISettings() *UISettings {
	return &UISettings{
		ShowPages:             true,
		ShowPassword:          true,
		ShowNotes:             true,
		Theme:                 DefaultTheme,
		TransparentBackground: false,
		TabOrder:              DefaultTabOrder(),
	}
}

func DefaultTabOrder() []string {
	return append([]string(nil), defaultTabOrder...)
}

func NormalizeTabOrder(order []string) []string {
	valid := make(map[string]struct{}, len(defaultTabOrder))
	for _, id := range defaultTabOrder {
		valid[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(defaultTabOrder))
	normalized := make([]string, 0, len(defaultTabOrder))
	for _, id := range order {
		id = strings.ToLower(strings.TrimSpace(id))
		if _, ok := valid[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	for _, id := range defaultTabOrder {
		if _, ok := seen[id]; ok {
			continue
		}
		normalized = append(normalized, id)
	}
	return normalized
}

func UITabOrder() []string {
	if settingsInstance == nil || settingsInstance.UI == nil {
		return DefaultTabOrder()
	}
	settingsInstance.UI.TabOrder = NormalizeTabOrder(settingsInstance.UI.TabOrder)
	return append([]string(nil), settingsInstance.UI.TabOrder...)
}

func normalizeSettings(s *UserSettings) {
	if s == nil {
		return
	}
	if !s.PasswordApp.Letters && !s.PasswordApp.Numbers && !s.PasswordApp.SpecialSymbols {
		s.PasswordApp.Letters = true
		s.PasswordApp.Numbers = true
		s.PasswordApp.SpecialSymbols = true
	}
	if s.PasswordApp.SymbolCount <= 0 {
		s.PasswordApp.SymbolCount = 16
	}
	if s.Firebase == nil {
		s.Firebase = defaultFirebaseSettings()
	}
	if s.UI == nil {
		s.UI = defaultUISettings()
	}
	if !ValidTheme(s.UI.Theme) {
		s.UI.Theme = DefaultTheme
	}
	s.UI.TabOrder = NormalizeTabOrder(s.UI.TabOrder)
	if s.AppWindow.Width <= 0 {
		s.AppWindow.Width = 600
	}
	if s.AppWindow.Height <= 0 {
		s.AppWindow.Height = 300
	}
	if s.NotesApp.TabSpaces <= 0 {
		s.NotesApp.TabSpaces = 4
	}
	if s.NotesApp.UndoLevels <= 0 {
		s.NotesApp.UndoLevels = 1000
	}
	if len(s.NotesApp.OpenNotePaths) > 0 {
		normalized := make([]string, 0, len(s.NotesApp.OpenNotePaths))
		seen := make(map[string]struct{}, len(s.NotesApp.OpenNotePaths))
		for _, path := range s.NotesApp.OpenNotePaths {
			path = normalizeNoteSessionPath(path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			normalized = append(normalized, path)
		}
		s.NotesApp.OpenNotePaths = normalized
	}
	s.NotesApp.CurrentNotePath = normalizeNoteSessionPath(s.NotesApp.CurrentNotePath)
	if len(s.NotesApp.SpellDictionaries) > 0 {
		normalized := make([]string, 0, len(s.NotesApp.SpellDictionaries))
		seen := make(map[string]struct{}, len(s.NotesApp.SpellDictionaries))
		for _, code := range s.NotesApp.SpellDictionaries {
			code = strings.ToLower(strings.TrimSpace(code))
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			normalized = append(normalized, code)
		}
		s.NotesApp.SpellDictionaries = normalized
	}
	if s.NotesApp.TabSpaces == 4 && !s.NotesApp.VimMode && s.NotesApp.EditorWidth == 0 && !s.NotesApp.SidebarVisible {
		s.NotesApp.SidebarVisible = true
	}
}

func CurrentTheme() string {
	if settingsInstance == nil || settingsInstance.UI == nil || !ValidTheme(settingsInstance.UI.Theme) {
		return DefaultTheme
	}
	return settingsInstance.UI.Theme
}

func ValidTheme(theme string) bool {
	for _, candidate := range BuiltInThemes {
		if theme == candidate {
			return true
		}
	}
	return false
}

func NextTheme(theme string) string {
	for i, candidate := range BuiltInThemes {
		if theme == candidate {
			return BuiltInThemes[(i+1)%len(BuiltInThemes)]
		}
	}
	return DefaultTheme
}

func RegisterSaveHook(fn func(*UserSettings)) {
	saveHooks = append(saveHooks, fn)
}

func runSaveHooks(notifyHooks bool) {
	if !notifyHooks {
		return
	}
	for _, hook := range saveHooks {
		hook(settingsInstance)
	}
}

func getFileName(n string) string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	return filepath.Join(
		dirname,
		helpers.AppConfigMainDir,
		helpers.AppConfigAppDir,
		n)
}

func fileName() string {
	return getFileName("settings.json")
}

func SaveSettings() {
	SaveSettingsLocal()
}

func SaveSettingsLocal() {
	writeSettingsToDisk(false)
}

func saveSettings(sync bool, notifyHooks bool) {
	writeSettingsToDisk(notifyHooks)
}

func BeginShutdown() {
	shuttingDown.Store(true)
}

func IsShuttingDown() bool {
	return shuttingDown.Load()
}

func SaveNotesEditorWidth(width int) {
	if settingsInstance == nil || width <= 0 {
		return
	}
	if settingsInstance.NotesApp.EditorWidth == width {
		return
	}
	settingsInstance.NotesApp.EditorWidth = width
}

func SaveNotesPreviewHidden(hidden bool) {
	if settingsInstance == nil {
		return
	}
	if settingsInstance.NotesApp.PreviewHidden == hidden {
		return
	}
	settingsInstance.NotesApp.PreviewHidden = hidden
	writeSettingsToDisk(false)
}

func SaveNotesSession(paths []string, currentPath string) {
	if UpdateNotesSession(paths, currentPath) {
		writeSettingsToDisk(false)
	}
}

func UpdateNotesSession(paths []string, currentPath string) bool {
	if settingsInstance == nil {
		return false
	}
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = normalizeNoteSessionPath(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}
	currentPath = normalizeNoteSessionPath(currentPath)
	if slicesEqual(settingsInstance.NotesApp.OpenNotePaths, normalized) && settingsInstance.NotesApp.CurrentNotePath == currentPath {
		return false
	}
	settingsInstance.NotesApp.OpenNotePaths = normalized
	settingsInstance.NotesApp.CurrentNotePath = currentPath
	return true
}

func normalizeNoteSessionPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		cleaned := filepath.ToSlash(filepath.Clean(path))
		if cleaned == "." {
			return ""
		}
		return cleaned
	}

	notesRoot := getFileName("notes")
	if rel, err := filepath.Rel(notesRoot, path); err == nil && rel != "." && rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.ToSlash(rel)
	}

	marker := "/" + filepath.ToSlash(filepath.Join(helpers.AppConfigMainDir, helpers.AppConfigAppDir, "notes")) + "/"
	slashPath := filepath.ToSlash(filepath.Clean(path))
	if idx := strings.Index(slashPath, marker); idx >= 0 {
		rel := slashPath[idx+len(marker):]
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel != "." && rel != "" && rel != ".." && !strings.HasPrefix(rel, "../") {
			return rel
		}
	}

	return filepath.ToSlash(filepath.Clean(path))
}

func slicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func SaveAppWindowState(width int, height int, maximized bool) {
	if settingsInstance == nil {
		return
	}
	if width > 0 {
		settingsInstance.AppWindow.Width = width
	}
	if height > 0 {
		settingsInstance.AppWindow.Height = height
	}
	settingsInstance.AppWindow.Maximized = maximized
}

func PersistedNotesEditorWidth() int {
	if settingsInstance != nil && settingsInstance.NotesApp.EditorWidth > 0 {
		return settingsInstance.NotesApp.EditorWidth
	}

	data, err := os.ReadFile(fileName())
	if err != nil {
		return 0
	}

	var persisted struct {
		NotesApp struct {
			EditorWidth int `json:"editor_width"`
		} `json:"notes_app"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		return 0
	}
	if persisted.NotesApp.EditorWidth > 0 {
		return persisted.NotesApp.EditorWidth
	}
	return 0
}

func writeSettingsToDisk(notifyHooks bool) {
	file := fileName()
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Println(err)
		statusUpdater("Couldn't prepare settings directory... :(")
		return
	}
	if settingsInstance == nil {
		settingsInstance = defaultSettings()
	}
	normalizeSettings(settingsInstance)
	dataString, err := json.Marshal(settingsInstance)
	if err != nil {
		log.Println(err)
		statusUpdater("Couldn't stringify settings... :(")
		return
	}
	if err := writeFileAtomic(file, dataString, 0o644); err != nil {
		log.Println(err)
		statusUpdater("Couldn't save settings... :(")
		return
	}
	runSaveHooks(notifyHooks)
	statusUpdater("Settings saved!")
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".settings.json.tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
