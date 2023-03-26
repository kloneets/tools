package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func (pm *PopoverMenu) About() {
	about := gtk.NewAboutDialog()

	about.SetAuthors([]string{"Jānis Rubļevskis"})
	about.SetCopyright("Can be used and/or modified by anyone.")
	about.SetComments("This application is made to make my daily work easier.")
	about.SetDestroyWithParent(true)
	about.SetIconName("media-tape")
	about.SetLicense("MIT")
	about.SetLicenseType(gtk.LicenseMITX11)
	about.SetLogoIconName("media-tape")
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
	about.Show()

	pm.Popover.Hide()
}
