package app

import (
	"github.com/gdamore/tcell/v2"
	"github.com/kloneets/tools/src/helpers"
	"github.com/rivo/tview"
	"strings"
	"testing"
)

func TestCursorLineThemeMarkup(t *testing.T) {
	for name, theme := range themesByName {
		t.Run(name, func(t *testing.T) {
			bg := cursorLineBackground(theme)
			if bg == theme.Background || bg == theme.SelectionBG {
				t.Fatalf("indistinct cursor background %q", bg)
			}
		})
	}
	got := ansiToTView(helpers.ANSIRoleCursorLine + helpers.ANSI(helpers.ANSIRoleKeyword, "go") + helpers.ANSIRoleCursorLine + " tail\x1b[0m")
	if strings.Contains(got, "\x1b") || !strings.Contains(got, "[:"+cursorLineBackground(currentTheme())+"] tail") {
		t.Fatalf("markup=%q", got)
	}
}

func TestCursorLineRenderedCells(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(12, 2)
	line := helpers.ANSIRoleCursorLine + helpers.ANSI(helpers.ANSIRoleKeyword, "go") + helpers.ANSIRoleCursorLine + "          \x1b[0m"
	view := tview.NewTextView().SetDynamicColors(true).SetText(ansiToTView(line) + "\nnext")
	view.SetRect(0, 0, 12, 2)
	view.Draw(screen)
	want := themeColor(cursorLineBackground(currentTheme()))
	for _, x := range []int{0, 1, 2, 11} {
		_, _, style, _ := screen.GetContent(x, 0)
		_, bg, _ := style.Decompose()
		if bg != want {
			t.Fatalf("cell %d background=%v want %v", x, bg, want)
		}
	}
	_, _, style, _ := screen.GetContent(0, 1)
	_, bg, _ := style.Decompose()
	if bg == want {
		t.Fatal("cursor line background leaked into next row")
	}
}
