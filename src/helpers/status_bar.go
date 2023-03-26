package helpers

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)
type StatusBar struct {
	B *gtk.Statusbar
}

var statusBarInstance *StatusBar

func StatusBarInst() *StatusBar {
	if statusBarInstance == nil {
		log.Fatal("We have no status bar instance!")
	}

	return statusBarInstance
}

func InitStatusBar() {
	statusBarInstance = &StatusBar{
		B: gtk.NewStatusbar(),
	}
}

func (s *StatusBar) UpdateStatusBar(text string) {
	s.B.Push(s.B.ContextID("kt"), text)
}
