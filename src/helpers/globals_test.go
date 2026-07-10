package helpers

import "testing"

func TestInitGlobalsSetsDefaultDebug(t *testing.T) {
	t.Cleanup(func() {
		goInstance = nil
	})

	got := InitGlobals()
	if got == nil {
		t.Fatal("InitGlobals() returned nil")
	}
	if got.Debug {
		t.Fatal("expected Debug to default to false")
	}
	if Globals() != got {
		t.Fatal("Globals() did not return initialized instance")
	}
}

func TestGlobalsInitializesWhenMissing(t *testing.T) {
	t.Cleanup(func() {
		goInstance = nil
	})
	goInstance = nil

	got := Globals()
	if got == nil {
		t.Fatal("Globals() returned nil")
	}
	if CurrentGlobals() != got {
		t.Fatal("Globals() did not store initialized instance")
	}
}
