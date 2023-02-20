package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

const (
	DefaultBoxPadding    = 2
	DefaultMasterPadding = 3
)

func Frame(title string) *gtk.Frame {
	f := gtk.NewFrame(title)
	f.SetMarginTop(2)
	f.SetMarginStart(2)
	f.SetMarginEnd(2)

	return f
}

func FieldWrapper(element gtk.Widgetter, padding int) *gtk.Box {
	return Wrapper(element, gtk.OrientationHorizontal, padding)
}

func ToolsWrapper(element gtk.Widgetter, padding int) *gtk.Box {
	return Wrapper(element, gtk.OrientationVertical, padding)
}

func Wrapper(element gtk.Widgetter, orientation gtk.Orientation, padding int) *gtk.Box {
	box := gtk.NewBox(orientation, padding)
	box.SetMarginStart(padding)
	box.SetMarginEnd(padding)
	box.Append(element)

	return box
}

func MainArea() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, DefaultMasterPadding)
	box.SetMarginBottom(DefaultMasterPadding)

	return box
}
