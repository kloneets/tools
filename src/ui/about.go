package ui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
)

func (pm *PopoverMenu) About() {
	about := gtk.NewAboutDialog()

	about.SetAuthors([]string{"Jānis Rubļevskis"})
	about.SetCopyright("Can be used and/or modified by anyone.")
	about.SetComments("This application is made to make my daily work easier.")
	about.SetDestroyWithParent(true)
	about.SetLicense("MIT")
	about.SetLicenseType(gtk.LicenseMITX11)
	about.SetMarginBottom(10)
	about.SetMarginEnd(10)
	about.SetMarginStart(10)
	about.SetModal(true)
	about.SetName("Koko Tools")
	about.SetProgramName("Koko Tools")
	about.SetTitle("About Koko Tools")
	about.SetVersion("0.0.1")
	about.SetWebsite("https://nobody.lv/tools")
	about.SetWebsiteLabel("Koko tools home page")
	if logo, err := gdk.NewTextureFromFile(gio.NewFileForPath(helpers.AppIconPath())); err == nil {
		about.SetLogo(logo)
	} else {
		log.Println("could not load app icon:", err)
	}
	about.Show()

	pm.Popover.Hide()
}
