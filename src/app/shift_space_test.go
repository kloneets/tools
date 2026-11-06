package app

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
	"github.com/rivo/tview"
)

func sendShiftSpaceTestInput(a *terminalApp, text string) {
	for _, r := range text {
		event := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
		if r == '\x1b' {
			event = tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone)
		}
		a.captureInput(event)
	}
}

func TestShiftSpaceInEditor(t *testing.T) {
	for _, input := range []string{"\x1b[32;2u", "\x1b[27;2;32~", " ", "32;2u"} {
		t.Run(input, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings.Init()
			ed := &notes.Editor{Text: "ab", Cursor: 1, Mode: notes.ModeInsert}
			a := &terminalApp{view: viewNotes, notes: &notes.Workspace{Tabs: []*notes.Editor{ed}}}
			sendShiftSpaceTestInput(a, input)
			want := "a b"
			if input == "32;2u" {
				want = "a32;2ub"
			}
			if ed.Text != want || ed.Mode != notes.ModeInsert {
				t.Fatalf("text = %q, mode = %v; want %q in insert mode", ed.Text, ed.Mode, want)
			}
		})
	}
}

func TestShiftSpaceBeforeHelpSearchRouting(t *testing.T) {
	for _, input := range []string{"\x1b[32;2u", "\x1b[27;2;32~"} {
		t.Run(input, func(t *testing.T) {
			a := &terminalApp{showHelp: true, helpSearchMode: true, helpSearchQuery: "a", helpOverlay: tview.NewTextView()}
			sendShiftSpaceTestInput(a, input)
			if !a.helpSearchMode || a.helpSearchQuery != "a " {
				t.Fatalf("help search = %q, editing = %t", a.helpSearchQuery, a.helpSearchMode)
			}
			a.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
			a.flushInputSequence()
			if a.helpSearchMode {
				t.Fatal("standalone Escape did not leave help search")
			}
		})
	}
}

func TestInputSequenceInterruptionKeepsCurrentEvent(t *testing.T) {
	a := noteTabsTestApp(t)
	a.showNoteTabsList()
	a.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	a.captureInput(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if a.noteTabsList != nil {
		t.Fatal("interrupted escape did not close picker")
	}
	// The arrow is routed after Escape rather than swallowed by decoder flushing.
	if a.notes.ActiveEditor().Cursor == 0 {
		t.Fatal("interrupting down key was lost")
	}
}

func TestIncompleteInputSequencePreservesText(t *testing.T) {
	for _, input := range []string{"\x1b[32;X", "\x1b[32;"} {
		t.Run(input, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings.Init()
			settings.Inst().NotesApp.VimMode = false
			ed := &notes.Editor{Mode: notes.ModeInsert}
			a := &terminalApp{view: viewNotes, notes: &notes.Workspace{Tabs: []*notes.Editor{ed}}}
			sendShiftSpaceTestInput(a, input)
			a.flushInputSequence()
			if ed.Text != input[1:] {
				t.Fatalf("text = %q; want %q", ed.Text, input[1:])
			}
		})
	}
}

func TestDecodedCtrlSequencesRespectModalAndHelp(t *testing.T) {
	for _, seq := range []string{"\x1b[9;5u", "\x1b[27;5;9~", "\x1b[103;5u", "\x1b[27;5;103~"} {
		t.Run(seq, func(t *testing.T) {
			a := noteTabsTestApp(t)
			a.showNoteTabsList()
			sendShiftSpaceTestInput(a, seq)
			if a.view != viewNotes || a.noteTabsList == nil || a.notes.ActiveEditor().Status != "" {
				t.Fatal("decoded Ctrl key escaped note picker routing")
			}
			a.captureDecodedInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
			a.showHelp = true
			a.helpOverlay = tview.NewTextView()
			a.helpSearchMode = true
			sendShiftSpaceTestInput(a, seq)
			if a.view != viewNotes || !a.helpSearchMode || a.notes.ActiveEditor().Status != "" {
				t.Fatal("decoded Ctrl key escaped help search routing")
			}
		})
	}
}

func TestConsecutiveEscapesRemainSeparateKeys(t *testing.T) {
	a := noteTabsTestApp(t)
	a.notes.FocusSidebar = true
	sendShiftSpaceTestInput(a, "/note")
	sendShiftSpaceTestInput(a, "\x1b\x1b")
	a.flushInputSequence()
	if !a.notes.FocusSidebar {
		t.Fatal("two Escapes should finish and clear search, leaving sidebar focused")
	}
	if got := a.notes.CommandLineText(300); len(got) > 0 && got[0] == '/' {
		t.Fatalf("search was not cleared: %q", got)
	}
	sendShiftSpaceTestInput(a, "\x1b")
	a.flushInputSequence()
	if a.notes.FocusSidebar {
		t.Fatal("third Escape should leave sidebar")
	}
}

func TestShiftSpaceWithTcellCollapsedCSIIntroducer(t *testing.T) {
	// tcell v2.7.4 emits Alt+[ for an unrecognized CSI sequence received in
	// one terminal read, followed by unmodified runes for the remaining bytes.
	for _, suffix := range []string{"32;2u", "27;2;32~"} {
		t.Run(suffix, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings.Init()
			ed := &notes.Editor{Text: "ab", Cursor: 1, Mode: notes.ModeInsert}
			a := &terminalApp{view: viewNotes, notes: &notes.Workspace{Tabs: []*notes.Editor{ed}}}
			a.captureInput(tcell.NewEventKey(tcell.KeyRune, '[', tcell.ModAlt))
			sendShiftSpaceTestInput(a, suffix)
			if ed.Text != "a b" || ed.Mode != notes.ModeInsert {
				t.Fatalf("text = %q, mode = %v; want a b in insert mode", ed.Text, ed.Mode)
			}
		})
	}
}
