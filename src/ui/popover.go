package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

type PopoverMenu struct {
	Popover     *gtk.Popover
	AboutButton *gtk.Button
	Settings    *Settings
}

func Popover() *PopoverMenu {
	pm := &PopoverMenu{
		Popover:     gtk.NewPopover(),
		AboutButton: gtk.NewButton(),
	}
	pm.Settings = pm.NewSettings()

	pm.AboutButton.SetLabel("About")
	pm.AboutButton.ConnectClicked(pm.About)

	box := gtk.NewBox(gtk.OrientationVertical, 3)
	box.Append(pm.Settings.SettingsButton)
	box.Append(pm.AboutButton)
	pm.Popover.SetChild(box)

	return pm
}
