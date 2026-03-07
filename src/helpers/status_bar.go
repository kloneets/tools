package helpers

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type StatusBar struct {
	B         *gtk.Statusbar
	contextID uint
}

var statusBarInstance *StatusBar

func HasStatusBar() bool {
	return statusBarInstance != nil
}

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
	statusBarInstance.contextID = statusBarInstance.B.ContextID("kt")
}

func (s *StatusBar) UpdateStatusBar(text string) {
	s.B.Pop(s.contextID)
	s.B.Push(s.contextID, text)
}
