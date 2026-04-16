package pages

import "testing"

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
		Editing:         true,
	}
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
	if !m.HandleEditKey("", '5') {
		t.Fatal("HandleEditKey() = false, want digit accepted")
	}
	if m.FirstBookInput != "105" {
		t.Fatalf("FirstBookInput = %q, want %q", m.FirstBookInput, "105")
	}
	if !m.HandleEditKey("backspace", 0) {
		t.Fatal("HandleEditKey(backspace) = false, want true")
	}
	if m.FirstBookInput != "10" {
		t.Fatalf("FirstBookInput = %q, want %q", m.FirstBookInput, "10")
	}
	if !m.HandleEditKey("esc", 0) {
		t.Fatal("HandleEditKey(esc) = false, want true")
	}
	if m.IsEditing() {
		t.Fatal("IsEditing() = true, want false after esc")
	}
}
