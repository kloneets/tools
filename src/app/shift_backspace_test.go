package app

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
	"github.com/kloneets/tools/src/todo"
)

func TestShiftBackspaceKeepsTodoEditing(t *testing.T) {
	inputs := []struct {
		name string
		send func(*terminalApp)
	}{
		{"CSI-u split", func(a *terminalApp) { sendShiftSpaceTestInput(a, "\x1b[127;2u") }},
		{"modifyOtherKeys split", func(a *terminalApp) { sendShiftSpaceTestInput(a, "\x1b[27;2;127~") }},
		{"CSI-u collapsed", func(a *terminalApp) {
			a.captureInput(tcell.NewEventKey(tcell.KeyRune, '[', tcell.ModAlt))
			sendShiftSpaceTestInput(a, "127;2u")
		}},
		{"modifyOtherKeys collapsed", func(a *terminalApp) {
			a.captureInput(tcell.NewEventKey(tcell.KeyRune, '[', tcell.ModAlt))
			sendShiftSpaceTestInput(a, "27;2;127~")
		}},
		{"native backspace", func(a *terminalApp) { a.captureInput(tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModShift)) }},
		{"native backspace2", func(a *terminalApp) { a.captureInput(tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModShift)) }},
	}
	for _, input := range inputs {
		for _, mode := range []string{"new", "edit", "new-list", "rename-list"} {
			for _, tc := range []struct {
				name, text, want   string
				cursor, wantCursor int
			}{
				{"Unicode end", "aāž", "aā", 3, 2},
				{"Unicode middle", "aāž", "až", 2, 1},
				{"empty", "", "", 0, 0},
				{"start", "āž", "āž", 0, 0},
			} {
				t.Run(input.name+"/"+mode+"/"+tc.name, func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					settings.Init()
					a := &terminalApp{
						view:                  viewTodo,
						todos:                 todo.NewRepositoryAt(filepath.Join(t.TempDir(), "todos.json")),
						todoInputMode:         mode,
						todoInputBuffer:       tc.text,
						todoInputCursorOffset: tc.cursor,
					}
					input.send(a)
					if a.todoInputBuffer != tc.want || a.todoInputCursorOffset != tc.wantCursor || a.todoInputMode != mode {
						t.Fatalf("input state = %q/%d/%q; want %q/%d/%q", a.todoInputBuffer, a.todoInputCursorOffset, a.todoInputMode, tc.want, tc.wantCursor, mode)
					}
					// A genuine standalone Escape must still cancel, unlike shifted backspace.
					a.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
					a.flushInputSequence()
					if a.todoInputMode != "" || a.todoInputBuffer != "" {
						t.Fatalf("Escape did not cancel editing: %q/%q", a.todoInputMode, a.todoInputBuffer)
					}
				})
			}
		}
	}
}

func TestInputSequenceBackspaceUsesNativeEvent(t *testing.T) {
	event := inputSequenceEvent(notes.Key{Name: "backspace"})
	if event.Key() != tcell.KeyBackspace2 || event.Rune() != 0 || event.Modifiers() != tcell.ModNone {
		t.Fatalf("decoded backspace event = %#v; want native backspace", event)
	}
	if key, ok := mapTCellKey(event); !ok || key != (notes.Key{Name: "backspace"}) {
		t.Fatalf("mapped decoded backspace = %#v/%t", key, ok)
	}
}
