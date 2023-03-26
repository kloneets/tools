package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

type PopoverMenu struct {
	Popover     *gtk.Popover
	AboutButton *gtk.Button
}

func Popover() *PopoverMenu {
	pm := &PopoverMenu{
		Popover:     gtk.NewPopover(),
		AboutButton: gtk.NewButton(),
	}

	pm.AboutButton.SetLabel("About")
	pm.AboutButton.ConnectClicked(pm.About)
	pm.Popover.SetChild(pm.AboutButton)

	return pm
}
