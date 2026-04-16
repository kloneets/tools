package helpers

import "testing"

func TestSanitizeSingleLine(t *testing.T) {
	got := SanitizeSingleLine("one\n two\tthree\rfour")
	if got != "one two three four" {
		t.Fatalf("SanitizeSingleLine() = %q", got)
	}
}

func TestPadANSIUsesVisibleWidth(t *testing.T) {
	styled := ANSI(ANSIBold, "abc")
	got := PadANSI(styled, 5)
	if VisibleRuneCount(got) != 5 {
		t.Fatalf("VisibleRuneCount() = %d, want 5", VisibleRuneCount(got))
	}
}
