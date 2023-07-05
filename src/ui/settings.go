package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

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

	settingsFrame := Frame("Settings")
	dialog.SetChild(settingsFrame)

	dialog.AddButton("Save", int(gtk.ResponseOK))
	dialog.AddButton("Cancel", int(gtk.ResponseCancel))

	dialog.Connect("response", func(dialog *gtk.Dialog, responseId gtk.ResponseType) {
		// Handle dialog response here, e.g. update the app's settings
		dialog.Destroy()
	})

	dialog.Show()

	pm.Popover.Hide()
}
