package ui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type PopoverMenu struct {
	Popover     *gtk.Popover
	AboutButton *gtk.Button
	QuitButton  *gtk.Button
	Settings    *Settings
}

func Popover(parent gtk.Widgetter) *PopoverMenu {
	pm := &PopoverMenu{
		Popover:     gtk.NewPopover(),
		AboutButton: gtk.NewButtonWithLabel("About"),
		QuitButton:  gtk.NewButtonWithLabel("Quit"),
	}
	pm.Popover.SetPosition(gtk.PosRight)
	pm.Popover.SetParent(parent)
	pm.Settings = pm.NewSettings()

	content := gtk.NewBox(gtk.OrientationVertical, DefaultMasterPadding)
	content.SetMarginTop(DefaultMasterPadding)
	content.SetMarginBottom(DefaultMasterPadding)
	content.SetMarginStart(DefaultMasterPadding)
	content.SetMarginEnd(DefaultMasterPadding)

	pm.AboutButton.ConnectClicked(pm.About)
	pm.QuitButton.ConnectClicked(func() {
		if !gtk.BaseWidget(parent).ActivateAction("app.quit", nil) {
			log.Println("could not activate app.quit action")
		}
		pm.Popover.Hide()
	})

	content.Append(pm.Settings.SettingsButton)
	content.Append(pm.AboutButton)
	content.Append(pm.QuitButton)
	pm.Popover.SetChild(content)

	return pm
}

// MenuPair creates a gtk.Menu out of the given menu pair. The returned Menu
// instance satisfies gio.MenuModeller. The first value of a pair should be the
// name.
func MenuPair(pairs [][2]string) *gio.Menu {
	menu := gio.NewMenu()
	for _, pair := range pairs {
		menu.Append(pair[0], pair[1])
	}
	return menu
}

// CustomMenu returns a new Menu from the given popover menu items. All menu
// items that have widgets are ignored.
func CustomMenu(items []PopoverMenuItem) *gio.Menu {
	menu := gio.NewMenu()
	addMenuItems(menu, items, nil)
	return menu
}

// CustomMenuItems is the variadic arguments variant of CustomMenu.
func CustomMenuItems(items ...PopoverMenuItem) *gio.Menu {
	return CustomMenu(items)
}

// PopoverMenuItem defines a popover menu item constructed from one of the
// constructors.
type PopoverMenuItem interface {
	menu()
}

type popoverMenuItem struct {
	label  string
	action string
	icon   string
	widget gtk.Widgetter
}

func (p popoverMenuItem) menu() {}

func setCustomMenuItem(item *gio.MenuItem, id string) {
	item.SetAttributeValue("custom", glib.NewVariantString(id))
}

type submenu struct {
	label string
	items []PopoverMenuItem
}

func (p submenu) menu() {}

func addMenuItems(menu *gio.Menu, items []PopoverMenuItem, widgets map[string]gtk.Widgetter) int {
	section := menu
	var added int

	for _, item := range items {
		if item == nil {
			continue
		}

		switch item := item.(type) {
		case popoverMenuItem:
			if item.widget != nil && widgets == nil {
				continue
			}

			if item.action == "---" {
				section = gio.NewMenu()
				menu.AppendSection(item.label, section)
				continue
			}

			menuItem := gio.NewMenuItem(item.label, item.action)
			if item.icon != "" {
				menuItem.SetIcon(gio.NewThemedIcon(item.icon))
			}
			if item.widget != nil {
				widgets[item.action] = item.widget
				setCustomMenuItem(menuItem, item.action)
			}
			added++
			section.AppendItem(menuItem)
		case submenu:
			sub := gio.NewMenu()
			if addMenuItems(sub, item.items, widgets) > 0 {
				added++
				section.AppendSubmenu(item.label, sub)
			}
		default:
			log.Panicf("unknown menu item type %T", item)
		}
	}

	return added
}
