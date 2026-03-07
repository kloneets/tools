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
