package notes

import (
	"github.com/kloneets/tools/src/settings"
	"strings"
	"testing"
)

func TestHandlePastePreservesText(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	for _, mode := range []Mode{ModeInsert, ModeNormal} {
		for _, tc := range []struct{ name, input, want string }{
			{"whitespace", "  a\t b  \n\n• ā\n◦ c\n", "  a\t b  \n\n• ā\n◦ c\n"},
			{"line endings", "a\r\nb\rc\n", "a\nb\nc\n"},
			{"commands", "q?\t:wq\n", "q?\t:wq\n"},
		} {
			t.Run(string(mode)+tc.name, func(t *testing.T) {
				ed := &Editor{Text: "āz", Cursor: 1, Mode: mode}
				w := &Workspace{Tabs: []*Editor{ed}}
				if !w.HandlePaste(tc.input) || ed.Text != "ā"+tc.want+"z" || ed.Cursor != 1+len([]rune(tc.want)) || ed.Mode != mode {
					t.Fatalf("paste: %+v", ed)
				}
				if !applyUndo(ed) || ed.Text != "āz" {
					t.Fatal("undo failed")
				}
				if !applyRedo(ed) || ed.Text != "ā"+tc.want+"z" {
					t.Fatal("redo failed")
				}
			})
		}
	}
}
func TestHandlePasteSelections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	for _, tc := range []struct {
		name string
		ed   Editor
		want string
	}{
		{"char", Editor{Text: "abcd", Mode: ModeVisual, SelectionMode: vimSelectionChar, SelectionMark: 1, SelectionCursor: 2}, "aX\nYd"},
		{"line", Editor{Text: "abc\ndef", Mode: ModeVisual, SelectionMode: vimSelectionLine, SelectionMark: 0, SelectionCursor: 2}, "X\nYdef"},
		{"block", Editor{Text: "abcd\nefgh", Mode: ModeVisual, SelectionMode: vimSelectionBlock, SelectionMark: 1, SelectionCursor: 7}, "aX\nYd\neX\nYh"},
		{"multi", Editor{Text: "abc abc", Mode: ModeInsert, MultiCursorActive: true, MultiCursorStarts: []int{0, 4}, MultiCursorLengths: []int{3, 3}}, "X\nY X\nY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ed := tc.ed
			w := &Workspace{Tabs: []*Editor{&ed}}
			w.HandlePaste("X\nY")
			if ed.Text != tc.want {
				t.Fatalf("got %q want %q", ed.Text, tc.want)
			}
			if !applyUndo(&ed) || ed.Text != tc.ed.Text {
				t.Fatal("undo failed")
			}
		})
	}
}
func TestHandlePasteSidebarAndCommand(t *testing.T) {
	ed := &Editor{Text: "keep", Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed}, FocusSidebar: true}
	if w.HandlePaste("q\n") || ed.Text != "keep" {
		t.Fatal("sidebar changed note")
	}
	w.FocusSidebar = false
	ed.Mode = ModeCommand
	w.HandlePaste("q\n\t")
	if ed.Command != "q  " || ed.Text != "keep" || ed.Mode != ModeCommand {
		t.Fatal("command paste executed")
	}
}
func BenchmarkHandlePaste(b *testing.B) {
	text := strings.Repeat("• ā\ttext  \n", 10000)
	for i := 0; i < b.N; i++ {
		ed := &Editor{Mode: ModeInsert}
		w := &Workspace{Tabs: []*Editor{ed}}
		w.HandlePaste(text)
	}
}

func TestHandlePasteCursorPositions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	for _, tc := range []struct {
		cursor int
		want   string
	}{{0, "•\nāz"}, {1, "ā•\nz"}, {2, "āz•\n"}} {
		ed := &Editor{Text: "āz", Cursor: tc.cursor, Mode: ModeInsert}
		w := &Workspace{Tabs: []*Editor{ed}}
		w.HandlePaste("•\n")
		if ed.Text != tc.want || ed.Cursor != tc.cursor+2 {
			t.Fatalf("cursor %d got %q at %d", tc.cursor, ed.Text, ed.Cursor)
		}
		before := len(ed.UndoStack)
		if w.HandlePaste("") || len(ed.UndoStack) != before {
			t.Fatal("empty paste changed state")
		}
	}
}
func TestHandlePasteBlockPadding(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ed := &Editor{Text: "abcd\nx", Mode: ModeInsert, MultiCursorActive: true, MultiCursorStarts: []int{3, 6}, MultiCursorLengths: []int{0, 0}, MultiCursorPadPending: true, MultiCursorPadColumn: 3}
	w := &Workspace{Tabs: []*Editor{ed}}
	w.HandlePaste("•\t")
	if ed.Text != "abc•\td\nx  •\t" {
		t.Fatalf("got %q", ed.Text)
	}
	if !applyUndo(ed) || ed.Text != "abcd\nx" {
		t.Fatal("padding not included in undo")
	}
}

func TestHandlePasteReplaceConfirmation(t *testing.T) {
	ed := &Editor{Text: "keep", Mode: ModeNormal, ReplaceConfirm: &replaceConfirmSession{}}
	w := &Workspace{Tabs: []*Editor{ed}}
	if w.HandlePaste("new") || ed.Text != "keep" || ed.ReplaceConfirm == nil || len(ed.UndoStack) != 0 {
		t.Fatal("paste edited replacement confirmation")
	}
}

func TestTypingAfterPasteKeepsUndoBoundary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	for _, suffix := range []string{" ", ". ", "more "} {
		ed := &Editor{Text: "abc", Cursor: 3, Mode: ModeInsert}
		w := &Workspace{Tabs: []*Editor{ed}}
		w.HandlePaste("xyz")
		for _, r := range suffix {
			insertRune(ed, r)
		}
		for ed.Text != "abcxyz" && len(ed.UndoStack) > 0 {
			applyUndo(ed)
		}
		if ed.Text != "abcxyz" {
			t.Fatalf("suffix %q undo crossed paste: %q", suffix, ed.Text)
		}
		if !applyUndo(ed) || ed.Text != "abc" {
			t.Fatalf("paste undo %q", ed.Text)
		}
	}
}
