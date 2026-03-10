package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

func TestConfigDir(t *testing.T) {
	home := "/tmp/example"
	got := configDir(home)
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if got != want {
		t.Fatalf("configDir() = %q, want %q", got, want)
	}
}

func TestEnsureConfigDirExistsCreatesParents(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nested", helpers.AppConfigMainDir, helpers.AppConfigAppDir)

	ensureConfigDirExists(target)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", target)
	}
}

func TestApplyWidgetVisibility(t *testing.T) {
	requireGTK(t)

	tools := &kokoTools{
		pagesFrame:    gtk.NewFrame("Pages"),
		passwordFrame: gtk.NewFrame("Password"),
		notesFrame:    gtk.NewFrame("Notes"),
		leftColumn:    gtk.NewBox(gtk.OrientationVertical, 0),
	}
	cfg := &settings.UserSettings{
		UI: &settings.UISettings{
			ShowPages:    false,
			ShowPassword: true,
			ShowNotes:    false,
		},
	}

	tools.applyWidgetVisibility(cfg)

	if gtk.BaseWidget(tools.pagesFrame).Visible() {
		t.Fatal("pages frame should be hidden")
	}
	if !gtk.BaseWidget(tools.passwordFrame).Visible() {
		t.Fatal("password frame should remain visible")
	}
	if gtk.BaseWidget(tools.notesFrame).Visible() {
		t.Fatal("notes frame should be hidden")
	}
	if gtk.BaseWidget(tools.leftColumn).Visible() {
		t.Fatal("left column should be hidden when pages and notes are hidden")
	}
}

func TestRestoreAppWindowStateAppliesSavedSize(t *testing.T) {
	requireGTK(t)

	window := gtk.NewWindow()
	restoreAppWindowState(window, &settings.UserSettings{
		AppWindow: settings.AppWindowSettings{
			Width:  910,
			Height: 620,
		},
	})

	width, height := window.DefaultSize()
	if width != 910 || height != 620 {
		t.Fatalf("window default size = %dx%d, want 910x620", width, height)
	}
}

func TestPersistAppWindowStateStoresWindowDefaultSize(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	window := gtk.NewWindow()
	window.SetDefaultSize(840, 520)

	persistAppWindowState(window)

	if settings.Inst().AppWindow.Width != 840 || settings.Inst().AppWindow.Height != 520 {
		t.Fatalf("AppWindow = %#v, want width=840 height=520", settings.Inst().AppWindow)
	}
	if settings.Inst().AppWindow.Maximized {
		t.Fatal("AppWindow.Maximized = true, want false")
	}
}

func TestParseMacAppearanceOutputRecognizesDark(t *testing.T) {
	if !parseMacAppearanceOutput([]byte("Dark\n")) {
		t.Fatal("expected Dark output to enable dark appearance")
	}
	if parseMacAppearanceOutput([]byte("Light\n")) {
		t.Fatal("expected non-Dark output to disable dark appearance")
	}
}

func TestSystemPrefersDarkAppearanceReturnsFalseWhenReaderFails(t *testing.T) {
	original := macAppearanceReader
	defer func() {
		macAppearanceReader = original
	}()

	macAppearanceReader = func(context.Context) ([]byte, error) {
		return nil, errors.New("boom")
	}

	preferDark, ok := systemPrefersDarkAppearanceForOS(context.Background(), "darwin")
	if ok {
		t.Fatal("expected failed appearance probe to return ok=false")
	}
	if preferDark {
		t.Fatal("expected failed appearance probe to return preferDark=false")
	}
}

func TestSystemPrefersDarkAppearanceReadsDarkOnDarwin(t *testing.T) {
	original := macAppearanceReader
	defer func() {
		macAppearanceReader = original
	}()

	macAppearanceReader = func(context.Context) ([]byte, error) {
		return []byte("Dark\n"), nil
	}

	preferDark, ok := systemPrefersDarkAppearanceForOS(context.Background(), "darwin")
	if !ok || !preferDark {
		t.Fatalf("systemPrefersDarkAppearanceForOS() = (%t, %t), want (true, true)", preferDark, ok)
	}
}

func TestSystemPrefersDarkAppearanceSkipsNonDarwin(t *testing.T) {
	preferDark, ok := systemPrefersDarkAppearanceForOS(context.Background(), "linux")
	if ok {
		t.Fatal("expected non-darwin appearance probe to return ok=false")
	}
	if preferDark {
		t.Fatal("expected non-darwin appearance probe to return preferDark=false")
	}
}

func TestShowShutdownSyncOverlayUsesMainOverlay(t *testing.T) {
	requireGTK(t)
	helpers.InitGlobals()

	window := gtk.NewWindow()
	overlay := gtk.NewOverlay()
	window.SetChild(overlay)
	helpers.SetMainOverlay(overlay)

	tools := &kokoTools{appWindow: &gtk.ApplicationWindow{}}
	tools.showShutdownSyncOverlay()

	if tools.shutdownSyncBackdrop == nil || tools.shutdownSyncPanel == nil {
		t.Fatal("expected shutdown sync overlay widgets to be created")
	}

	tools.hideShutdownSyncOverlay()
	if tools.shutdownSyncBackdrop != nil || tools.shutdownSyncPanel != nil {
		t.Fatal("expected shutdown sync overlay widgets to be cleared")
	}
}
