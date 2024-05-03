package ui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type PopoverMenu struct {
	Popover     *gtk.PopoverMenu
	Menu        *gio.Menu
	AboutButton *gtk.Button
	QuitButton  *gio.MenuItem
	Settings    *Settings
}

func Popover(parent gtk.Widgetter) *PopoverMenu {
	menu := gio.NewMenu()
	menu.AppendItem(gio.NewMenuItem("_Quit", "app.Quit"))
	pm := &PopoverMenu{
		Popover: NewPopoverMenu(parent, gtk.PosRight, [][2]string{
			{"_About", "pm.About"},
			// quit Button
			{"_Quit", "app.Quit"},
		}),
		Menu:        menu,
		AboutButton: gtk.NewButton(),
		QuitButton:  gio.NewMenuItem("_Quit", "app.Quit"),
	}
	pm.Settings = pm.NewSettings()

	pm.AboutButton.SetLabel("About")
	pm.AboutButton.Connect("pm.About", pm.About)
	// pm.AboutButton.ConnectClicked(pm.About)

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

// PopoverWidth is the default popover width.
const PopoverWidth = 150

// NewPopoverMenu creates a new Popover menu.
func NewPopoverMenu(w gtk.Widgetter, pos gtk.PositionType, pairs [][2]string) *gtk.PopoverMenu {
	popover := gtk.NewPopoverMenuFromModel(MenuPair(pairs))
	popover.SetMnemonicsVisible(true)
	popover.SetSizeRequest(PopoverWidth, -1)
	popover.SetPosition(pos)
	popover.SetParent(w)
	return popover
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
				// No widgets supported; skip this menu item.
				continue
			}

			if item.action == "---" {
				section = gio.NewMenu()
				menu.AppendSection(item.label, section)
				continue
			}

			menu := gio.NewMenuItem(item.label, item.action)
			if item.icon != "" {
				menu.SetIcon(gio.NewThemedIcon(item.icon))
			}
			if item.widget != nil {
				widgets[item.action] = item.widget
				setCustomMenuItem(menu, item.action)
			}
			added++
			section.AppendItem(menu)
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
