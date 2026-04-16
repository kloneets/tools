package helpers

import (
	"log"
	"sync"
)

type StatusBar struct {
	mu   sync.RWMutex
	text string
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
	statusBarInstance = &StatusBar{}
}

func (s *StatusBar) UpdateStatusBar(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
}

func (s *StatusBar) Text() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.text
}
