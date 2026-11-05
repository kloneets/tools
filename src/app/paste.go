package app

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// pasteRoot keeps terminal paste separate from command-key capture.
type pasteRoot struct {
	tview.Primitive
	app *terminalApp
}

func (p *pasteRoot) PasteHandler() func(string, func(tview.Primitive)) {
	return func(text string, _ func(tview.Primitive)) {
		if p.app.handlePaste(text) {
			p.app.refresh()
		}
	}
}
func (a *terminalApp) handlePaste(text string) bool {
	if a.shuttingDown || a.recorderCapturing || a.tabSelect || a.noteTabsList != nil {
		return false
	}
	if a.pagesRoot != nil {
		front, _ := a.pagesRoot.GetFrontPage()
		switch front {
		case "delete-note", "delete-todo-list", "open-links":
			return false
		}
	}
	line := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(text)
	if a.showHelp {
		if a.helpSearchMode {
			a.helpSearchQuery += line
			a.helpSearchIndex = 0
			return true
		}
		return false
	}
	switch a.view {
	case viewNotes:
		return a.notes.HandlePaste(text)
	case viewTodo:
		if a.todoSearch.editing {
			a.todoSearch.query += line
			a.updateTodoSearch()
			return true
		}
		if a.todoInputMode != "" {
			runes := []rune(a.todoInputBuffer)
			a.clampTodoInputCursor(len(runes))
			at := a.todoInputCursorOffset
			a.todoInputBuffer = string(runes[:at]) + line + string(runes[at:])
			a.todoInputCursorOffset += len([]rune(line))
			return true
		}
	case viewPages:
		if a.pages != nil && a.pages.IsEditing() {
			for _, r := range line {
				a.pages.HandleEditKey(string(r), r)
			}
			return true
		}
	case viewSettings:
		if a.settingsEditMode {
			for _, r := range line {
				if r >= '0' && r <= '9' {
					a.settingsEditBuffer += string(r)
				}
			}
			return true
		}
	}
	return false
}

// terminalPasteScreen adapts LF events that tview's paste collector otherwise drops.
// CRLF is one newline; outside a paste all key events retain their usual meaning.
type terminalPasteScreen struct {
	tcell.Screen
	pasting, previousCR bool
	initErr             error
}

func (s *terminalPasteScreen) Init() error { s.initErr = s.Screen.Init(); return s.initErr }
func (s *terminalPasteScreen) PollEvent() tcell.Event {
	for {
		event := s.Screen.PollEvent()
		if paste, ok := event.(*tcell.EventPaste); ok {
			s.pasting = paste.Start()
			s.previousCR = false
			return event
		}
		if key, ok := event.(*tcell.EventKey); ok && s.pasting {
			if key.Key() == tcell.KeyLF {
				if s.previousCR {
					s.previousCR = false
					continue
				}
				return tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
			}
			s.previousCR = key.Key() == tcell.KeyEnter
		}
		return event
	}
}
