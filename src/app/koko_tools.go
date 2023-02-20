package app

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/pages"
	"github.com/kloneets/tools/src/password"
	"github.com/kloneets/tools/src/ui"
)

type kokoTools struct {
	*gtk.Application
	window *gtk.ApplicationWindow
	// pages  *pages.KokoPages
}

func InitApp() {
	makeConfigDirIfNotExists()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	app := gtk.NewApplication("com.github.kloneets.tools", gio.ApplicationFlagsNone)
	app.ConnectActivate(func() {
		t := activate(ctx, app)
		t.window.Show()
	})

	go func() {
		<-ctx.Done()
		glib.IdleAdd(app.Quit)
	}()

	if code := app.Run(os.Args); code > 0 {
		cancel()
		os.Exit(code)
	}
}

func activate(ctx context.Context, app *gtk.Application) *kokoTools {
	tools := kokoTools{Application: app}

	tools.window = gtk.NewApplicationWindow(app)
	tools.window.SetDefaultSize(600, 300)
	tools.window.SetTitle("Koko tools")

	page := pages.PageUi()
	passw := password.GenerateUI()
	note := notes.GenerateUI()

	helpers.InitStatusBar()

	upperLeftWrapper := ui.ToolsWrapper(page.F, ui.DefaultBoxPadding)
	upperLeftWrapper.Append(note.F)

	upper := gtk.NewBox(gtk.OrientationHorizontal, 2)
	upper.Append(upperLeftWrapper)
	upper.Append(passw.F)

	mainWrap := gtk.NewBox(gtk.OrientationVertical, 0)
	mainWrap.Append(upper)
	mainWrap.Append(helpers.StatusBarInst().B)

	tools.window.SetChild(mainWrap)

	return &tools
}

func makeConfigDirIfNotExists() {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
		return
	}
	dir := filepath.Join(dirname, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(dir, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
}
