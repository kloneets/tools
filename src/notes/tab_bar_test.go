package notes

import (
	"strings"
	"testing"

	"github.com/kloneets/tools/src/helpers"
	"github.com/mattn/go-runewidth"
)

func tabBarWorkspace() *Workspace {
	w := &Workspace{}
	for _, title := range []string{"Plan", "仕事", "Notes", "Fourth", "A very long note title", "Last"} {
		w.Tabs = append(w.Tabs, &Editor{Title: title})
	}
	return w
}

func TestTabBarActiveVisibilityAndResize(t *testing.T) {
	for _, width := range []int{60, 30, 20, 12} {
		w := tabBarWorkspace()
		w.CurrentTab = 5
		line := helpers.StripANSI(w.TabsText(width))
		if runewidth.StringWidth(line) > width {
			t.Fatalf("width %d: %q", width, line)
		}
		if len(w.tabBar.hits) == 0 || w.tabBar.hits[len(w.tabBar.hits)-1].index != 5 {
			t.Fatalf("active hidden at width %d: %q", width, line)
		}
		w.TabsText(width + 10)
		found := false
		for _, h := range w.tabBar.hits {
			found = found || h.index == 5
		}
		if !found {
			t.Fatal("resize hid active tab")
		}
	}
}

func TestTabBarScrollAndCellTargets(t *testing.T) {
	w := tabBarWorkspace()
	w.TabsText(30)
	if !w.ScrollTabs(1) {
		t.Fatal("cannot scroll right")
	}
	w.TabsText(30)
	if w.tabBar.start != 1 {
		t.Fatal("render reset manual scrolling")
	}
	for _, hit := range w.tabBar.hits {
		for col := hit.start; col < hit.end; col++ {
			if index, ok := w.tabIndexAtColumn(col); !ok || index != hit.index {
				t.Fatalf("bad hit at cell %d", col)
			}
		}
		if index, ok := w.tabCloseIndexAtColumn(hit.close); !ok || index != hit.index {
			t.Fatal("close target mismatch")
		}
	}
	if w.TabBarControlAtColumn(0) != "previous" || w.TabBarControlAtColumn(29) != "list" {
		t.Fatal("control hit mismatch")
	}
	w.CurrentTab = 5
	w.TabsText(30)
	if w.tabBar.end != 6 {
		t.Fatal("switch did not reveal active")
	}
}

func TestTabBarLongTitleAndNarrowWidths(t *testing.T) {
	w := tabBarWorkspace()
	w.CurrentTab = 4
	for _, width := range []int{0, 1, 3, 9, 16, 25} {
		line := helpers.StripANSI(w.TabsText(width))
		if runewidth.StringWidth(line) > width {
			t.Fatalf("overflow at %d: %q", width, line)
		}
		if width >= 12 && !strings.Contains(line, "x]") {
			t.Fatalf("missing close: %q", line)
		}
	}
}

func TestTabBarRevealsActiveAfterLabelChanges(t *testing.T) {
	for _, change := range []string{"dirty", "rename"} {
		t.Run(change, func(t *testing.T) {
			w := tabBarWorkspace()
			w.CurrentTab = 1
			w.TabsText(28)
			if change == "dirty" {
				w.Tabs[1].Dirty = true
			} else {
				w.Tabs[1].Title = "A much longer renamed note"
			}
			w.TabsText(28)
			for _, hit := range w.tabBar.hits {
				if hit.index == 1 {
					return
				}
			}
			t.Fatal("active tab disappeared after label change")
		})
	}
}
