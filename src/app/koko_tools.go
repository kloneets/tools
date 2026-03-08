package app

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	_ "embed"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/pages"
	"github.com/kloneets/tools/src/password"
	"github.com/kloneets/tools/src/settings"
	"github.com/kloneets/tools/src/ui"
)

type kokoTools struct {
	*gtk.Application
	appWindow     *gtk.ApplicationWindow
	pagesFrame    *gtk.Frame
	passwordFrame *gtk.Frame
	notesFrame    *gtk.Frame
	leftColumn    *gtk.Box
}

//go:embed menu.ui
var menuXML string

func InitApp() {
	helpers.InitGlobals()
	flag.BoolVar(&helpers.Globals().Debug, "d", false, "Debug mode")
	flag.Parse()

	makeConfigDirIfNotExists()
	settings.Init()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	app := gtk.NewApplication("com.github.kloneets.tools", gio.ApplicationFlagsNone)
	app.ConnectActivate(func() {
		t := activate(ctx, app)
		t.appWindow.Show()
	})

	app.AddMainOption("Debug mode", []byte("d")[0], 0b1, 0, "", "")

	go func() {
		<-ctx.Done()
		glib.IdleAdd(app.Quit)
	}()

	if code := app.Run(os.Args); code > 0 {
		log.Println("A te mēs tikām!", code)
		cancel()
		os.Exit(code)
	}
}

func activate(ctx context.Context, app *gtk.Application) *kokoTools {
	tools := kokoTools{Application: app}

	menuBuilder := gtk.NewBuilderFromString(menuXML, len(menuXML))
	menu := menuBuilder.GetObject("header-bar").Cast().(*gtk.HeaderBar)

	gb := menuBuilder.GetObject("gears").Cast().(*gtk.MenuButton)
	gb.SetPopover(ui.Popover(gb).Popover)

	tools.appWindow = gtk.NewApplicationWindow(app)
	tools.appWindow.SetTitlebar(menu)
	if iconName := helpers.WindowIconName(); iconName != "" {
		tools.appWindow.SetIconName(iconName)
	}
	tools.appWindow.SetDefaultSize(600, 300)
	tools.appWindow.SetTitle("Koko tools")

	helpers.InitStatusBar()

	page := pages.PageUi()
	passw := password.GenerateUI()
	note := notes.GenerateUI()
	tools.pagesFrame = page.F
	tools.passwordFrame = passw.F
	tools.notesFrame = note.F

	upperLeftWrapper := ui.ToolsWrapper(page.F, ui.DefaultBoxPadding)
	upperLeftWrapper.Append(note.F)
	tools.leftColumn = upperLeftWrapper

	upper := gtk.NewBox(gtk.OrientationHorizontal, 2)
	upper.SetHExpand(true)
	upper.SetVExpand(true)
	upper.Append(upperLeftWrapper)
	upper.Append(passw.F)

	mainWrap := gtk.NewBox(gtk.OrientationVertical, 0)
	mainWrap.SetHExpand(true)
	mainWrap.SetVExpand(true)
	mainWrap.Append(upper)
	mainWrap.Append(helpers.StatusBarInst().B)

	tools.appWindow.SetChild(mainWrap)
	tools.applyWidgetVisibility(settings.Inst())
	settings.RegisterSaveHook(func(cfg *settings.UserSettings) {
		glib.IdleAdd(func() {
			tools.applyWidgetVisibility(cfg)
		})
	})

	return &tools
}

func (t *kokoTools) applyWidgetVisibility(cfg *settings.UserSettings) {
	if t == nil || cfg == nil || cfg.UI == nil {
		return
	}
	if t.pagesFrame != nil {
		gtk.BaseWidget(t.pagesFrame).SetVisible(cfg.UI.ShowPages)
	}
	if t.passwordFrame != nil {
		gtk.BaseWidget(t.passwordFrame).SetVisible(cfg.UI.ShowPassword)
	}
	if t.notesFrame != nil {
		gtk.BaseWidget(t.notesFrame).SetVisible(cfg.UI.ShowNotes)
	}
	if t.leftColumn != nil {
		gtk.BaseWidget(t.leftColumn).SetVisible(cfg.UI.ShowPages || cfg.UI.ShowNotes)
	}
}

func makeConfigDirIfNotExists() {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
		return
	}
	ensureConfigDirExists(configDir(dirname))
}

func configDir(home string) string {
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
}

func ensureConfigDirExists(dir string) {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
}
