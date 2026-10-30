package app

import (
	"fmt"

	"github.com/kloneets/tools/src/helpers"
	"github.com/rivo/tview"
)

func (a *terminalApp) showNoteTabsList() {
	if a.notes == nil || len(a.notes.Tabs) == 0 || a.pagesRoot == nil {
		return
	}
	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" Open notes · Enter select · Esc close ")
	theme := currentTheme()
	list.SetBackgroundColor(appBackgroundColor())
	list.SetMainTextColor(themeColor(theme.Primary))
	list.SetSelectedTextColor(themeColor(theme.SelectionFG))
	list.SetSelectedBackgroundColor(themeColor(theme.SelectionBG))
	list.SetBorderColor(themeColor(theme.Border))
	list.SetTitleColor(themeColor(theme.Title))
	for index, tab := range a.notes.Tabs {
		title := "Untitled"
		if tab != nil {
			title = helpers.SanitizeSingleLine(tab.Title)
		}
		label := fmt.Sprintf("%d: %s", index+1, title)
		if tab != nil && tab.Dirty {
			label += " *"
		}
		list.AddItem(tview.Escape(label), "", 0, nil)
	}
	list.SetCurrentItem(a.notes.CurrentTab)
	closeList := func() {
		a.pagesRoot.RemovePage("note-tabs")
		a.noteTabsList = nil
		if a.tui != nil && a.editor != nil {
			a.tui.SetFocus(a.editor)
		}
	}
	list.SetDoneFunc(closeList)
	list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		closeList()
		a.notes.SelectTab(index)
		a.refresh()
	})
	// Weighted margins keep the picker usable when the terminal is resized.
	row := tview.NewFlex().AddItem(nil, 0, 1, false).AddItem(list, 0, 8, true).AddItem(nil, 0, 1, false)
	panel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).AddItem(row, 0, 8, true).AddItem(nil, 0, 1, false)
	a.noteTabsList = list
	a.pagesRoot.AddPage("note-tabs", panel, true, true)
	if a.tui != nil {
		a.tui.SetFocus(list)
	}
}
