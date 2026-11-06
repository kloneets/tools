package app

import (
	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
	"strings"
	"testing"
)

func TestSidebarSearchCapturesGlobalTextShortcuts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	ws := &notes.Workspace{FocusSidebar: true, Tabs: []*notes.Editor{{Text: "text", Mode: notes.ModeNormal}}}
	app := &terminalApp{view: viewNotes, notes: ws}
	for _, r := range "/q?123n" {
		app.captureInput(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	if app.showHelp || app.shuttingDown || app.view != viewNotes {
		t.Fatal("query activated global shortcut")
	}
	if got := ws.CommandLineText(100); !strings.HasPrefix(got, "/q?123n") {
		t.Fatalf("query=%q", got)
	}
	app.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if ws.IsEditableContext() {
		t.Fatal("Enter did not confirm search")
	}
}
