package notes

import (
	"fmt"
	"strings"

	"github.com/kloneets/tools/src/helpers"
	"github.com/mattn/go-runewidth"
)

type tabBarHit struct{ start, end, close, index int }

func (w *Workspace) TakePendingTabList() bool {
	requested := w.pendingTabList
	w.pendingTabList = false
	return requested
}

type tabBarState struct {
	start, width, end, picker int
	count, current            int
	labels                    string
	active                    *Editor
	hits                      []tabBarHit
}

// SelectTab uses the same navigation path as clicking a note tab.
func (w *Workspace) SelectTab(index int) bool {
	if w == nil || index < 0 || index >= len(w.Tabs) {
		return false
	}
	w.setCurrentTab(index)
	w.tabBar.active = nil
	w.leaveSidebar()
	w.ensureEditorVisible()
	w.updateSession()
	return true
}

func (w *Workspace) ScrollTabs(delta int) bool {
	if w == nil || len(w.Tabs) == 0 {
		return false
	}
	if delta > 0 && w.tabBar.end >= len(w.Tabs) {
		return false
	}
	next := min(max(0, w.tabBar.start+delta), len(w.Tabs)-1)
	if next == w.tabBar.start {
		return false
	}
	w.tabBar.start = next
	return true
}

func (w *Workspace) TabBarControlAtColumn(col int) string {
	if w == nil || col < 0 || col >= w.tabBar.width || len(w.Tabs) == 0 {
		return ""
	}
	if col >= w.tabBar.picker {
		return "list"
	}
	if col < 2 && w.tabBar.start > 0 {
		return "previous"
	}
	if col >= w.tabBar.picker-2 && w.tabBar.end < len(w.Tabs) {
		return "next"
	}
	return ""
}

func (w *Workspace) renderTabBar(width int) string {
	if w == nil {
		return ""
	}
	b := &w.tabBar
	oldWidth, oldActive := b.width, b.active
	var labels strings.Builder
	for i, tab := range w.Tabs {
		labels.WriteString(noteTabLabel(i, tab))
		labels.WriteByte(0)
	}
	changed := b.count != len(w.Tabs) || b.current != w.CurrentTab || b.labels != labels.String()
	b.labels = labels.String()
	b.count, b.current = len(w.Tabs), w.CurrentTab
	b.width = max(0, width)
	b.hits = nil
	if len(w.Tabs) == 0 {
		b.start, b.end, b.picker = 0, 0, b.width
		b.active = nil
		return runewidth.Truncate("No open notes", b.width, "")
	}
	b.active = w.ActiveEditor()
	picker := fmt.Sprintf("▾ %d", len(w.Tabs))
	pickerWidth := runewidth.StringWidth(picker)
	b.picker = max(0, width-pickerWidth)
	budget := width - pickerWidth - 4
	if budget < 4 {
		b.end = b.start
		b.picker = 0
		return runewidth.Truncate(picker, b.width, "")
	}
	b.start = min(max(0, b.start), len(w.Tabs)-1)
	build := func(start int) (string, []tabBarHit, int) {
		var out strings.Builder
		var hits []tabBarHit
		used, end := 0, start
		for i := start; i < len(w.Tabs); i++ {
			title := helpers.SanitizeSingleLine(noteTabLabel(i, w.Tabs[i]))
			n := runewidth.StringWidth(title) + 4
			if used == 0 && n > budget {
				if budget == 4 {
					title = ""
				} else {
					title = runewidth.Truncate(title, budget-4, "…")
				}
				n = runewidth.StringWidth(title) + 4
			}
			sep := 0
			if used > 0 {
				sep = 1
			}
			if used+sep+n > budget {
				break
			}
			if sep > 0 {
				out.WriteByte(' ')
				used++
			}
			hits = append(hits, tabBarHit{start: 2 + used, end: 2 + used + n, close: 2 + used + n - 2, index: i})
			label := "[" + title + " x]"
			if i == w.CurrentTab {
				label = helpers.ANSIRoleActiveTab + "[" + title + " " + helpers.ANSIRoleActiveTabClose + "x" + helpers.ANSIRoleActiveTab + "]\x1b[0m"
			}
			out.WriteString(label)
			used += n
			end = i + 1
		}
		out.WriteString(strings.Repeat(" ", budget-used))
		return out.String(), hits, end
	}
	line, hits, end := build(b.start)
	if changed || oldWidth != width || oldActive != b.active {
		if w.CurrentTab < b.start {
			b.start = w.CurrentTab
		}
		for w.CurrentTab >= end && b.start < w.CurrentTab {
			b.start++
			_, _, end = build(b.start)
		}
		line, hits, end = build(b.start)
	}
	b.hits, b.end = hits, end
	left, right := "  ", "  "
	if b.start > 0 {
		left = "‹ "
	}
	if end < len(w.Tabs) {
		right = "› "
	}
	return left + line + right + picker
}
