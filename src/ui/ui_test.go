package ui

import (
	"sync"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/settings"
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
	if frame.HExpand() || frame.VExpand() {
		t.Fatal("Frame() should not expand by default")
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
	if box.HExpand() || box.VExpand() {
		t.Fatal("Wrapper() should not expand by default")
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
	if box.HExpand() || box.VExpand() {
		t.Fatal("MainArea() should not expand by default")
	}
}

func TestInfoLabelLeftAlignsWrappedText(t *testing.T) {
	requireGTK(t)

	label := InfoLabel("status text")
	if label.XAlign() != 0 {
		t.Fatalf("InfoLabel() xalign = %v, want 0", label.XAlign())
	}
	if !label.Wrap() {
		t.Fatal("InfoLabel() should wrap text")
	}
}

func TestNewSettingsWindowUsesPreferencesStyleDefaults(t *testing.T) {
	requireGTK(t)

	parent := gtk.NewWindow()
	window := newSettingsWindow(parent)

	if window.Title() != "Settings" {
		t.Fatalf("newSettingsWindow() title = %q, want Settings", window.Title())
	}
	if window.Modal() {
		t.Fatal("newSettingsWindow() should not be modal")
	}
	if !window.DestroyWithParent() {
		t.Fatal("newSettingsWindow() should be destroyed with parent")
	}
	if window.HideOnClose() {
		t.Fatal("newSettingsWindow() should destroy on close")
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

func TestFolderDisplayLabelAddsSuffixOnlyForDuplicates(t *testing.T) {
	folder := folderDisplayLabel(
		gdrive.Folder{ID: "1234567890", Path: "Drive / Projects / Koko"},
		map[string]int{"Drive / Projects / Koko": 2},
	)
	if folder != "Drive / Projects / Koko (12345678)" {
		t.Fatalf("folderDisplayLabel() = %q", folder)
	}

	unique := folderDisplayLabel(
		gdrive.Folder{ID: "1234567890", Path: "Drive / Unique"},
		map[string]int{"Drive / Unique": 1},
	)
	if unique != "Drive / Unique" {
		t.Fatalf("folderDisplayLabel() unique = %q", unique)
	}
}

func TestLastSyncSummaryIncludesErrorMessage(t *testing.T) {
	summary := lastSyncSummary(&settings.GDriveSettings{
		LastSyncAt:      "2026-03-07T17:35:34+02:00",
		LastSyncStatus:  "error",
		LastSyncMessage: "create drive file: 403 forbidden",
	})

	want := "Last sync: 2026-03-07T17:35:34+02:00 (error)\ncreate drive file: 403 forbidden"
	if summary != want {
		t.Fatalf("lastSyncSummary() = %q, want %q", summary, want)
	}
}
