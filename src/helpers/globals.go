package helpers

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type GlobalOptions struct {
	Debug       bool
	MainWindow  *gtk.Window
	MainOverlay *gtk.Overlay
}

var goInstance *GlobalOptions

func CurrentGlobals() *GlobalOptions {
	return goInstance
}

func Globals() *GlobalOptions {
	if goInstance == nil {
		log.Fatal("We have no global options!")
	}

	return goInstance
}

func InitGlobals() *GlobalOptions {
	goInstance = &GlobalOptions{
		Debug:       false,
		MainWindow:  nil,
		MainOverlay: nil,
	}

	return goInstance
}

func SetMainWindow(window *gtk.Window) {
	Globals().MainWindow = window
}

func SetMainOverlay(overlay *gtk.Overlay) {
	Globals().MainOverlay = overlay
}
