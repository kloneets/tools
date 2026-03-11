package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	gio "github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
)

var iconGlyphs = map[string]string{
	"arrows-up-down":               "up-down-left-right",
	"checkbox-symbolic":            "square-check",
	"document-edit-symbolic":       "pen-to-square",
	"document-preview-symbolic":    "eye",
	"folder-new-symbolic":          "folder-plus",
	"folder-symbolic":              "folder",
	"format-justify-left-symbolic": "quote-left",
	"format-text-bold-symbolic":    "bold",
	"format-text-italic-symbolic":  "italic",
	"input-dialpad":                "calculator",
	"insert-link-symbolic":         "link",
	"list-add-symbolic":            "plus",
	"object-select-symbolic":       "check",
	"open-menu-symbolic":           "ellipsis-vertical",
	"pan-down-symbolic":            "angle-down",
	"pan-end-symbolic":             "angle-right",
	"sidebar-show-symbolic":        "bars",
	"text-x-generic-symbolic":      "file-lines",
	"text-x-script-symbolic":       "code",
	"user-trash-symbolic":          "trash",
	"window-close-symbolic":        "xmark",
}

func IconGlyph(iconName string) string {
	if file, ok := iconGlyphs[iconName]; ok {
		return file
	}
	return "circle-question"
}

func iconImage(iconName string) *gtk.Image {
	image := gtk.NewImageFromPixbuf(loadIconPixbuf(iconName))
	image.SetPixelSize(16)
	return image
}

func loadIconPixbuf(iconName string) *gdkpixbuf.Pixbuf {
	path := helpers.FontAwesomeIconPath(IconGlyph(iconName))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	stream := gio.NewMemoryInputStreamFromBytes(glib.NewBytes(injectIconColor(data, iconForegroundColor())))
	pixbuf, err := gdkpixbuf.NewPixbufFromStreamAtScale(context.Background(), stream, 16, 16, true)
	if err != nil {
		return nil
	}
	return pixbuf
}

func iconForegroundColor() string {
	gtkSettings := gtk.SettingsGetDefault()
	if gtkSettings == nil {
		return "#1f1f1f"
	}
	if preferDark, ok := gtkSettings.ObjectProperty("gtk-application-prefer-dark-theme").(bool); ok && preferDark {
		return "#f5f7ff"
	}
	return "#1f1f1f"
}

func injectIconColor(svg []byte, color string) []byte {
	replacement := fmt.Sprintf(`<path fill="%s" `, color)
	text := strings.ReplaceAll(string(svg), "<path ", replacement)
	return []byte(text)
}

func IconButton(iconName string, tooltip string) *gtk.Button {
	button := gtk.NewButton()
	button.SetTooltipText(tooltip)
	button.SetChild(iconImage(iconName))
	return button
}

func IconTextButton(iconName string, label string, tooltip string) *gtk.Button {
	button := gtk.NewButton()
	button.SetTooltipText(tooltip)

	content := gtk.NewBox(gtk.OrientationHorizontal, 6)
	content.Append(iconImage(iconName))

	text := gtk.NewLabel(label)
	text.SetXAlign(0)
	content.Append(text)

	button.SetChild(content)
	return button
}

func IconToggleButton(iconName string, tooltip string) *gtk.ToggleButton {
	button := gtk.NewToggleButton()
	button.SetTooltipText(tooltip)
	button.SetChild(iconImage(iconName))
	return button
}
