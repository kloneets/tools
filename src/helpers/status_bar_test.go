package helpers

import "testing"

func TestStatusBarInstInitializesWhenMissing(t *testing.T) {
	t.Cleanup(func() {
		statusBarInstance = nil
	})
	statusBarInstance = nil

	got := StatusBarInst()
	if got == nil {
		t.Fatal("StatusBarInst() returned nil")
	}
	if !HasStatusBar() {
		t.Fatal("HasStatusBar() = false, want true")
	}
}
