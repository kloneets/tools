package app

import (
	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
	"github.com/rivo/tview"
	"strings"
	"testing"
	"time"
)

func TestTerminalPasteEvents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	a, err := newTerminalApp()
	if err != nil {
		t.Fatal(err)
	}
	ed := &notes.Editor{Mode: notes.ModeNormal}
	a.notes.Tabs = []*notes.Editor{ed}
	a.notes.CurrentTab = 0
	a.notes.FocusSidebar = false
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(80, 24)
	app := tview.NewApplication().SetScreen(&terminalPasteScreen{Screen: screen})
	root := &pasteRoot{Primitive: a.root, app: a}
	app.SetRoot(root, true).EnablePaste(true).SetInputCapture(a.captureInput)
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	defer app.Stop()
	text := "q?\t• ā  \n\r\nend\r"
	screen.PostEventWait(tcell.NewEventPaste(true))
	for _, r := range text {
		switch r {
		case '\n':
			screen.PostEventWait(tcell.NewEventKey(tcell.KeyLF, 0, 0))
		case '\r':
			screen.PostEventWait(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
		case '\t':
			screen.PostEventWait(tcell.NewEventKey(tcell.KeyTab, 0, 0))
		default:
			screen.PostEventWait(tcell.NewEventKey(tcell.KeyRune, r, 0))
		}
	}
	screen.PostEventWait(tcell.NewEventPaste(false))
	// The following key is processed after the complete paste and provides a barrier.
	checked := make(chan struct{})
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey { close(checked); app.Stop(); return nil })
	screen.PostEventWait(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	select {
	case <-checked:
	case <-time.After(5 * time.Second):
		t.Fatal("paste event timeout")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("application stop timeout")
	}
	if ed.Text != "q?\t• ā  \n\nend\n" || a.showHelp || a.shuttingDown || len(ed.UndoStack) != 1 {
		t.Fatalf("paste %q undo=%d help=%v shutdown=%v", ed.Text, len(ed.UndoStack), a.showHelp, a.shuttingDown)
	}
}
func TestPasteFieldsAndGuards(t *testing.T) {
	cases := []struct {
		name    string
		a       terminalApp
		want    string
		handled bool
	}{
		{"todo", terminalApp{view: viewTodo, todoInputMode: "new", todoInputBuffer: "āz", todoInputCursorOffset: 1}, "āx  qz", true},
		{"settings", terminalApp{view: viewSettings, settingsEditMode: true}, "12", true},
		{"normal", terminalApp{view: viewSettings}, "", false},
		{"modal", terminalApp{view: viewTodo, todoInputMode: "new", shuttingDown: true}, "", false},
	}
	for i := range cases {
		tc := &cases[i]
		t.Run(tc.name, func(t *testing.T) {
			input := "x\n\tq"
			if tc.name == "settings" {
				input = "1x2\n"
			}
			if got := tc.a.handlePaste(input); got != tc.handled {
				t.Fatalf("handled %v", got)
			}
			got := tc.a.todoInputBuffer + tc.a.settingsEditBuffer
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
func BenchmarkTerminalPasteField(b *testing.B) {
	text := strings.Repeat("text", 10000)
	for i := 0; i < b.N; i++ {
		a := terminalApp{view: viewTodo, todoInputMode: "new"}
		a.handlePaste(text)
	}
}
