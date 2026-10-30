package app

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
)

func noteTabsTestApp(t *testing.T) *terminalApp {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	helpers.InitStatusBar()
	settings.Init()
	ws := &notes.Workspace{}
	for index := 0; index < 12; index++ {
		ws.Tabs = append(ws.Tabs, &notes.Editor{Title: fmt.Sprintf("Note %d", index+1), Text: "unchanged", Mode: notes.ModeNormal})
	}
	editor := tview.NewTextView()
	editor.SetRect(0, 0, 32, 10)
	ws.TabsText(32)
	return &terminalApp{view: viewNotes, notes: ws, editor: editor, pagesRoot: tview.NewPages()}
}

func TestNoteTabMouseWheelScrollsWithoutMovingEditorCursor(t *testing.T) {
	a := noteTabsTestApp(t)
	before := a.notes.TabsText(32)
	event, _ := a.captureMouse(tcell.NewEventMouse(4, 0, tcell.ButtonNone, 0), tview.MouseScrollDown)
	if event != nil {
		t.Fatal("tab-row scroll not consumed")
	}
	if after := a.notes.TabsText(32); after == before {
		t.Fatal("tab-row scroll did not change viewport")
	}
	if a.notes.CurrentTab != 0 || a.notes.ActiveEditor().Cursor != 0 {
		t.Fatal("scroll changed the active note or its cursor")
	}
	a.captureMouse(tcell.NewEventMouse(4, 0, tcell.ButtonNone, 0), tview.MouseScrollUp)
	if got := a.notes.TabsText(32); got != before {
		t.Fatal("reverse scroll did not restore viewport")
	}
}

func TestNoteTabListClickSelectionAndEscape(t *testing.T) {
	a := noteTabsTestApp(t)
	listCol := -1
	for col := 0; col < 32; col++ {
		if a.notes.TabBarControlAtColumn(col) == "list" {
			listCol = col
			break
		}
	}
	if listCol < 0 {
		t.Fatal("missing open-notes control")
	}
	a.captureMouse(tcell.NewEventMouse(listCol, 0, tcell.Button1, 0), tview.MouseLeftClick)
	if a.noteTabsList == nil || a.noteTabsList.GetItemCount() != 12 {
		t.Fatal("picker does not contain every open note")
	}
	a.captureInput(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	a.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if a.notes.CurrentTab != 1 || a.noteTabsList != nil || a.pagesRoot.HasPage("note-tabs") {
		t.Fatal("picker selection did not switch and close")
	}
	a.showNoteTabsList()
	a.captureInput(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	a.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if a.notes.CurrentTab != 1 || a.noteTabsList != nil {
		t.Fatal("escape changed note or left picker open")
	}
	for _, tab := range a.notes.Tabs {
		if tab.Text != "unchanged" {
			t.Fatal("picker input reached note editor")
		}
	}
}

func TestNoteTabKeyboardBindingsAndInsertText(t *testing.T) {
	a := noteTabsTestApp(t)
	settings.Inst().NotesApp.VimMode = true
	send := func(text string) {
		for _, r := range text {
			a.captureInput(tcell.NewEventKey(tcell.KeyRune, r, 0))
		}
	}
	before := a.notes.TabsText(32)
	send("g>")
	if a.notes.TabsText(32) == before {
		t.Fatal("g> did not scroll tabs")
	}
	send("g<")
	if a.notes.TabsText(32) != before {
		t.Fatal("g< did not restore viewport")
	}
	send("gN")
	if a.noteTabsList == nil {
		t.Fatal("gN did not open picker")
	}
	a.captureInput(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	a.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if a.notes.CurrentTab != 1 {
		t.Fatal("keyboard picker failed to select")
	}
	ed := a.notes.ActiveEditor()
	ed.Text = ""
	ed.Cursor = 0
	ed.Mode = notes.ModeInsert
	send("<,>,Ng<g>gN")
	if ed.Text != "<,>,Ng<g>gN" || a.noteTabsList != nil {
		t.Fatalf("insert bindings consumed text: %q", ed.Text)
	}
}
