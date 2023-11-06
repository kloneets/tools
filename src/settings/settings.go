package settings

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kloneets/tools/src/helpers"
)

type UserSettings struct {
	PasswordApp PasswordAppSettings `json:"password_app"`
	PagesApp    PagesAppSettings    `json:"pages_app"`
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

var settingsInstance *UserSettings

func Inst() *UserSettings {
	if settingsInstance == nil {
		log.Fatal("Settings is not initialized")
	}

	return settingsInstance
}

func Init() {
	fn := fileName()
	c, err := os.ReadFile(fn)
	if err != nil {
		log.Println("Settings read error: ", err)
		settingsInstance = defaultSettings()
		return
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
			msg := "Old settings backed up to: "+ backupFileName
			// helpers.StatusBarInst().UpdateStatusBar(msg)
			log.Println(msg)
		}
		settingsInstance = defaultSettings()
	}
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

	helpers.StatusBarInst().UpdateStatusBar("Settings saved!")
}
