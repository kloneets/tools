package ui

import (
	"os"
	"sync"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
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

func TestSettingsOverlayUsesGlobalOverlay(t *testing.T) {
	requireGTK(t)
	t.Cleanup(func() {
		helpers.InitGlobals()
	})

	helpers.InitGlobals()
	overlay := gtk.NewOverlay()
	helpers.SetMainOverlay(overlay)

	if got := settingsOverlay(); got != overlay {
		t.Fatal("settingsOverlay() should return global overlay")
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
	requireGTK(t)

	menu := CustomMenu([]PopoverMenuItem{
		popoverMenuItem{label: "About", action: "pm.About"},
		popoverMenuItem{label: "Inline", action: "pm.Inline", widget: gtk.NewLabel("inline")},
	})
	model := gio.BaseMenuModel(menu)
	if model.NItems() != 1 {
		t.Fatalf("CustomMenu() item count = %d, want 1", model.NItems())
	}
}

func TestIconButtonUsesBundledFontAwesomeImage(t *testing.T) {
	requireGTK(t)

	button := IconButton("list-add-symbolic", "New note")
	image, ok := button.Child().(*gtk.Image)
	if !ok {
		t.Fatal("IconButton() should attach an image child")
	}
	if got := image.PixelSize(); got != 16 {
		t.Fatalf("IconButton() pixel size = %d, want 16", got)
	}
	if _, err := os.Stat(helpers.FontAwesomeIconPath(IconGlyph("list-add-symbolic"))); err != nil {
		t.Fatalf("IconButton() asset missing: %v", err)
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

func TestInjectIconColorAddsFillToPaths(t *testing.T) {
	svg := []byte(`<svg><path d="M0 0"/></svg>`)
	got := string(injectIconColor(svg, "#ffffff"))
	if got != `<svg><path fill="#ffffff" d="M0 0"/></svg>` {
		t.Fatalf("injectIconColor() = %q", got)
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

func TestFontButtonSelectUsesSystemFontChooserSettings(t *testing.T) {
	requireGTK(t)

	button := fontButtonSelect("Cantarell 13", "Choose notes font", false)

	if button.Font() != "Cantarell 13" {
		t.Fatalf("fontButtonSelect() font = %q, want Cantarell 13", button.Font())
	}
	if !button.UseSize() {
		t.Fatal("fontButtonSelect() should show the selected size")
	}
	if button.Level() != gtk.FontChooserLevelFamily|gtk.FontChooserLevelSize {
		t.Fatalf("fontButtonSelect() level = %v", button.Level())
	}
}

func TestEffectiveEditorFontSizeUsesOverrideThenCurrentFont(t *testing.T) {
	override := effectiveEditorFontSize(settings.NotesAppSettings{
		BodyFont:       "Cantarell 11",
		EditorFontSize: 17,
	})
	if override != 17 {
		t.Fatalf("effectiveEditorFontSize() override = %d, want 17", override)
	}

	mono := effectiveEditorFontSize(settings.NotesAppSettings{
		EditorMonospace: true,
		MonospaceFont:   "JetBrains Mono 14",
	})
	if mono != 14 {
		t.Fatalf("effectiveEditorFontSize() monospace = %d, want 14", mono)
	}

	body := effectiveEditorFontSize(settings.NotesAppSettings{
		BodyFont: "IBM Plex Sans 12",
	})
	if body != 12 {
		t.Fatalf("effectiveEditorFontSize() body = %d, want 12", body)
	}
}

func TestGDriveSettingsCreatesSyncIntervalControl(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().GDrive.SyncIntervalSec = 25

	s := &Settings{}
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	window := gtk.NewWindow()
	s.GDriveSettings(window, box)

	if s.driveSyncInterval == nil {
		t.Fatal("expected drive sync interval control")
	}
	if got := s.driveSyncInterval.ValueAsInt(); got != 25 {
		t.Fatalf("driveSyncInterval = %d, want 25", got)
	}
}

func TestTrashSettingsCreatesTrashListAndCleanButton(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	trashFile := settings.NotesTrashDir() + "/Plan.md"
	if err := os.MkdirAll(settings.NotesTrashDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(trashFile, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := &Settings{}
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	s.TrashSettings(box)

	if s.trashList == nil {
		t.Fatal("expected trash list control")
	}
	if s.cleanTrashButton == nil {
		t.Fatal("expected clean trash button")
	}
	if s.trashList.RowAtIndex(0) == nil {
		t.Fatal("expected at least one trash row")
	}
}
