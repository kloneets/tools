package pages

import (
	"testing"

	"github.com/kloneets/tools/src/settings"
)

func TestSanitizeInputs(t *testing.T) {
	tests := []struct {
		name       string
		readText   string
		firstText  string
		secondText string
		wantRead   int
		wantFirst  int
		wantSecond int
	}{
		{name: "valid", readText: "20", firstText: "100", secondText: "250", wantRead: 20, wantFirst: 100, wantSecond: 250},
		{name: "invalid defaults", readText: "x", firstText: "0", secondText: "y", wantRead: 1, wantFirst: 1, wantSecond: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRead, gotFirst, gotSecond := sanitizeInputs(tt.readText, tt.firstText, tt.secondText)
			if gotRead != tt.wantRead || gotFirst != tt.wantFirst || gotSecond != tt.wantSecond {
				t.Fatalf("sanitizeInputs() = (%d, %d, %d), want (%d, %d, %d)", gotRead, gotFirst, gotSecond, tt.wantRead, tt.wantFirst, tt.wantSecond)
			}
		})
	}
}

func TestCalculateResult(t *testing.T) {
	gotPages, gotPercent := calculateResult(25, 100, 320)
	if gotPages != 80 || gotPercent != 25 {
		t.Fatalf("calculateResult() = (%d, %d), want (80, 25)", gotPages, gotPercent)
	}
}

func TestCursorVisibleWhenEditing(t *testing.T) {
	m := &Model{
		FirstBookInput:  "120",
		ReadInput:       "40",
		SecondBookInput: "300",
		Focus:           1,
	}
	m.StartEditing()
	row, col, ok := m.Cursor()
	if !ok {
		t.Fatal("Cursor() ok = false, want true while editing")
	}
	if row != 3 {
		t.Fatalf("Cursor() row = %d, want 3", row)
	}
	if col <= len([]rune("> read pages:  ")) {
		t.Fatalf("Cursor() col = %d, want cursor after field contents", col)
	}
}

func TestModelEditing(t *testing.T) {
	m := &Model{FirstBookInput: "10", ReadInput: "2", SecondBookInput: "20"}
	m.StartEditing()
	if !m.IsEditing() {
		t.Fatal("IsEditing() = false, want true")
	}
	if !m.SelectionActive || m.EditCursor != 2 {
		t.Fatalf("selection/cursor = %t/%d, want selected at end", m.SelectionActive, m.EditCursor)
	}
	if !m.HandleEditKey("", '5') {
		t.Fatal("HandleEditKey() = false, want digit accepted")
	}
	if m.FirstBookInput != "5" {
		t.Fatalf("FirstBookInput = %q, want %q", m.FirstBookInput, "5")
	}
	if m.SelectionActive || m.EditCursor != 1 {
		t.Fatalf("selection/cursor = %t/%d, want cleared at end", m.SelectionActive, m.EditCursor)
	}
	if !m.HandleEditKey("backspace", 0) {
		t.Fatal("HandleEditKey(backspace) = false, want true")
	}
	if m.FirstBookInput != "" {
		t.Fatalf("FirstBookInput = %q, want empty after backspace", m.FirstBookInput)
	}
	if !m.HandleEditKey("esc", 0) {
		t.Fatal("HandleEditKey(esc) = false, want true")
	}
	if m.IsEditing() {
		t.Fatal("IsEditing() = true, want false after esc")
	}
}

func TestModelEditingInsertsAtCursorAndMovesLeftRight(t *testing.T) {
	m := &Model{FirstBookInput: "123", ReadInput: "2", SecondBookInput: "20"}
	m.StartEditing()
	if !m.HandleEditKey("left", 0) {
		t.Fatal("HandleEditKey(left) = false, want true")
	}
	if m.SelectionActive || m.EditCursor != 0 {
		t.Fatalf("selection/cursor = %t/%d, want collapsed at start", m.SelectionActive, m.EditCursor)
	}
	if !m.HandleEditKey("right", 0) || !m.HandleEditKey("right", 0) {
		t.Fatal("HandleEditKey(right) = false, want true")
	}
	if m.EditCursor != 2 {
		t.Fatalf("EditCursor = %d, want 2", m.EditCursor)
	}
	if !m.HandleEditKey("", '9') {
		t.Fatal("HandleEditKey(digit) = false, want true")
	}
	if m.FirstBookInput != "1293" {
		t.Fatalf("FirstBookInput = %q, want 1293", m.FirstBookInput)
	}
}

func TestModelEditingTabAndShiftTabSelectFields(t *testing.T) {
	m := &Model{FirstBookInput: "10", ReadInput: "2", SecondBookInput: "20"}
	m.StartEditing()
	if !m.HandleEditKey("tab", 0) {
		t.Fatal("HandleEditKey(tab) = false, want true")
	}
	if m.Focus != 1 || !m.SelectionActive || m.EditCursor != 1 {
		t.Fatalf("focus/selection/cursor = %d/%t/%d, want read field selected", m.Focus, m.SelectionActive, m.EditCursor)
	}
	if !m.HandleEditKey("shift+tab", 0) {
		t.Fatal("HandleEditKey(shift+tab) = false, want true")
	}
	if m.Focus != 0 || !m.SelectionActive || m.EditCursor != 2 {
		t.Fatalf("focus/selection/cursor = %d/%t/%d, want first field selected", m.Focus, m.SelectionActive, m.EditCursor)
	}
}

func TestModelEditingUpDownNoop(t *testing.T) {
	m := &Model{FirstBookInput: "10", ReadInput: "2", SecondBookInput: "20"}
	m.StartEditing()
	if !m.HandleEditKey("down", 0) || !m.HandleEditKey("up", 0) {
		t.Fatal("HandleEditKey(up/down) = false, want true")
	}
	if m.Focus != 0 || !m.SelectionActive {
		t.Fatalf("focus/selection = %d/%t, want unchanged selected first field", m.Focus, m.SelectionActive)
	}
}

func TestModelEditingEnterRecalculates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	m := &Model{FirstBookInput: "100", ReadInput: "25", SecondBookInput: "320"}
	m.StartEditing()
	if !m.HandleEditKey("enter", 0) {
		t.Fatal("HandleEditKey(enter) = false, want true")
	}
	if m.IsEditing() {
		t.Fatal("IsEditing() = true, want false after enter")
	}
	if m.Result != "80 pages, 25%" {
		t.Fatalf("Result = %q, want recalculated", m.Result)
	}
	if !m.Dirty {
		t.Fatal("Dirty = false, want true")
	}
}
