package app

import (
	"context"
	"os"
	"os/signal"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/pages"
)

type kokoTools struct {
	*gtk.Application
	window *gtk.ApplicationWindow
	// pages  *pages.KokoPages
}

func InitApp() {
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
	tools.window.SetDefaultSize(300, 200)
	tools.window.SetTitle("Koko tools")

	p := pages.PageUi()
	tools.window.SetChild(p.F)

	return &tools
}
