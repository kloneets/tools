package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
)

type UserSettings struct {
	PasswordApp PasswordAppSettings `json:"password_app"`
	PagesApp    PagesAppSettings    `json:"pages_app"`
	GDrive      *GDriveSettings     `json:"gdrive"`
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

type GDriveSettings struct {
	Enabled         bool   `json:"enabled"`
	FolderID        string `json:"folder_id"`
	FolderName      string `json:"folder_name"`
	LastSyncAt      string `json:"last_sync_at"`
	LastSyncStatus  string `json:"last_sync_status"`
	LastSyncMessage string `json:"last_sync_message"`
}

var settingsInstance *UserSettings

func Inst() *UserSettings {
	if settingsInstance == nil {
		log.Fatal("Settings is not initialized")
	}

	return settingsInstance
}

func Init() *[]string {
	var messages []string
	fn := fileName()
	c, err := os.ReadFile(fn)
	if err != nil {
		msg := fmt.Sprintf("Settings read error: %s", err)
		log.Println(msg)
		messages = append(messages, msg)
		settingsInstance = defaultSettings()
		return &messages
	}

	marshalError := json.Unmarshal(c, &settingsInstance)
	if marshalError != nil {
		log.Println("Settings unmarshal error: ", marshalError)
		ct := time.Now()
		backupFileName := getFileName(ct.Format("2006-01-02_15.04.0000") + "settings.json")
		err = os.Rename(fn, backupFileName)
		if err != nil {
			log.Println("Cannot back up settings: ", err)
		} else {
			msg := "Old settings backed up to: " + backupFileName
			messages = append(messages, msg)
			log.Println(msg)
		}
		settingsInstance = defaultSettings()
	}

	normalizeSettings(settingsInstance)

	return &messages
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
		GDrive: defaultGDriveSettings(),
	}
}

func defaultGDriveSettings() *GDriveSettings {
	return &GDriveSettings{}
}

func normalizeSettings(s *UserSettings) {
	if s == nil {
		return
	}
	if s.GDrive == nil {
		s.GDrive = defaultGDriveSettings()
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

func (g *GDriveSettings) Ready() bool {
	return g != nil && g.Enabled && g.FolderID != "" && gdrive.HasCredentials() && gdrive.HasToken()
}

func SaveSettings() {
	saveSettings(true)
}

func saveSettings(sync bool) {
	file := fileName()

	if err := os.Truncate(file, 0); err != nil {
		log.Println("Settings read file error: ", err)
	}

	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	dataString, err := json.Marshal(settingsInstance)
	if err != nil {
		log.Println(err)
		helpers.StatusBarInst().UpdateStatusBar("Couldn't stringify settings... :(")
	}
	if _, err := f.WriteString(string(dataString)); err != nil {
		log.Println(err)
		helpers.StatusBarInst().UpdateStatusBar("Couldn't save settings... :(")
		return
	}

	if sync && settingsInstance.GDrive.Ready() {
		if err := SyncDriveData(); err != nil {
			log.Println("gdrive settings sync error:", err)
			helpers.StatusBarInst().UpdateStatusBar("Settings saved locally, Drive sync failed")
			return
		}
	}

	helpers.StatusBarInst().UpdateStatusBar("Settings saved!")
}

func SyncDriveData() error {
	if settingsInstance == nil || !settingsInstance.GDrive.Ready() {
		return nil
	}

	err := gdrive.SyncAppData(settingsInstance.GDrive.FolderID)
	recordSyncResult(err)
	saveSettings(false)

	if err == nil {
		if syncErr := gdrive.SyncFile(settingsInstance.GDrive.FolderID, fileName()); syncErr != nil {
			recordSyncResult(syncErr)
			saveSettings(false)
			return syncErr
		}
		recordSyncResult(nil)
		saveSettings(false)
	}

	return err
}

func recordSyncResult(err error) {
	if settingsInstance == nil || settingsInstance.GDrive == nil {
		return
	}

	settingsInstance.GDrive.LastSyncAt = time.Now().Format(time.RFC3339)
	if err != nil {
		settingsInstance.GDrive.LastSyncStatus = "error"
		settingsInstance.GDrive.LastSyncMessage = err.Error()
		return
	}

	settingsInstance.GDrive.LastSyncStatus = "ok"
	settingsInstance.GDrive.LastSyncMessage = "Drive sync completed successfully"
}
