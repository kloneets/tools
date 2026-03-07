package ui

import (
	"sync"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	gtkInitOnce sync.Once
	gtkInitOK   bool
)

func requireGTK(t *testing.T) {
	t.Helper()
	gtkInitOnce.Do(func() {
		gtk.DisableSetlocale()
		gtkInitOK = gtk.InitCheck()
	})
	if !gtkInitOK {
		t.Skip("GTK could not be initialized in this environment")
	}
}

func TestFrameAppliesLabelAndMargins(t *testing.T) {
	requireGTK(t)

	frame := Frame("Notes")
	if frame.Label() != "Notes" {
		t.Fatalf("Frame() label = %q, want %q", frame.Label(), "Notes")
	}
	if frame.MarginTop() != 2 || frame.MarginStart() != 2 || frame.MarginEnd() != 2 {
		t.Fatalf("Frame() margins = top:%d start:%d end:%d, want 2/2/2", frame.MarginTop(), frame.MarginStart(), frame.MarginEnd())
	}
}

func TestWrapperAddsChildAndSpacing(t *testing.T) {
	requireGTK(t)

	label := gtk.NewLabel("hello")
	box := Wrapper(label, gtk.OrientationHorizontal, 7)

	if box.Spacing() != 7 {
		t.Fatalf("Wrapper() spacing = %d, want 7", box.Spacing())
	}
	if box.MarginStart() != 7 || box.MarginEnd() != 7 {
		t.Fatalf("Wrapper() margins = start:%d end:%d, want 7/7", box.MarginStart(), box.MarginEnd())
	}
	if box.FirstChild() == nil {
		t.Fatal("Wrapper() did not append a child")
	}
	if box.ObserveChildren().NItems() != 1 {
		t.Fatalf("Wrapper() children count = %d, want 1", box.ObserveChildren().NItems())
	}
}

func TestMainAreaUsesDefaultSpacing(t *testing.T) {
	requireGTK(t)

	box := MainArea()
	if box.Spacing() != DefaultMasterPadding {
		t.Fatalf("MainArea() spacing = %d, want %d", box.Spacing(), DefaultMasterPadding)
	}
	if box.MarginBottom() != DefaultMasterPadding {
		t.Fatalf("MainArea() bottom margin = %d, want %d", box.MarginBottom(), DefaultMasterPadding)
	}
}

func TestMenuPairCreatesMenuModel(t *testing.T) {
	menu := MenuPair([][2]string{
		{"_About", "pm.About"},
		{"_Quit", "app.Quit"},
	})
	model := gio.BaseMenuModel(menu)
	if model.NItems() != 2 {
		t.Fatalf("MenuPair() item count = %d, want 2", model.NItems())
	}
}

func TestCustomMenuSkipsWidgetItemsWithoutWidgetMap(t *testing.T) {
	menu := CustomMenu([]PopoverMenuItem{
		popoverMenuItem{label: "About", action: "pm.About"},
		popoverMenuItem{label: "Inline", action: "pm.Inline", widget: gtk.NewLabel("inline")},
	})
	model := gio.BaseMenuModel(menu)
	if model.NItems() != 1 {
		t.Fatalf("CustomMenu() item count = %d, want 1", model.NItems())
	}
}
