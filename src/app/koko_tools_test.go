package app

import (
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
