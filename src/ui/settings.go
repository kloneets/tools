package ui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Settings struct {
	SettingsButton *gtk.Button
}

func (pm *PopoverMenu) NewSettings() *Settings {
	s := &Settings{}
	s.SettingsButton = gtk.NewButton()
	s.SettingsButton.SetLabel("Settings")
	s.SettingsButton.ConnectClicked(func() {
		s.SettingsWindow(pm)
	})
	return s
}

func (s *Settings) SettingsWindow(pm *PopoverMenu) {
	dialog := gtk.NewDialog()
	dialog.SetTitle("Settings")
	// Add widgets to the dialog to create the settings UI

	settingsFrame := MainArea()

	cancelButton := gtk.NewButton()
	cancelButton.SetLabel("Cancel")
	cancelButton.ConnectClicked(func() {
		dialog.Destroy()
	})

	dialog.AddButton("Save", int(gtk.ResponseOK))
	dialog.AddButton("Cancel", int(gtk.ResponseCancel))

	s.GDriveSettings(settingsFrame)
	settingsFrame.Append(cancelButton)
	dialog.SetChild(settingsFrame)

	dialog.Connect("response", func(dialog *gtk.Dialog, responseId gtk.ResponseType) {
		log.Println("Response:", responseId)
		// Handle dialog response here, e.g. update the app's settings
		dialog.Destroy()
	})

	dialog.Show()

	pm.Popover.Hide()
}

func (s *Settings) GDriveSettings(placeholder *gtk.Box) {
	gDriveFrame := Frame("Google Drive")

	placeholder.Append(gDriveFrame)
}
