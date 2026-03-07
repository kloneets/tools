package notes

import "testing"

func TestVimLineBoundaryOffset(t *testing.T) {
	text := "alpha\nbeta\n"
	if got := vimLineBoundaryOffset(text, 8, false); got != 6 {
		t.Fatalf("vimLineBoundaryOffset(start) = %d, want 6", got)
	}
	if got := vimLineBoundaryOffset(text, 8, true); got != 10 {
		t.Fatalf("vimLineBoundaryOffset(end) = %d, want 10", got)
	}
}

func TestVimVerticalMoveOffset(t *testing.T) {
	text := "abcde\nxy\nlast"
	if got := vimVerticalMoveOffset(text, 4, 1); got != 8 {
		t.Fatalf("vimVerticalMoveOffset() = %d, want 8", got)
	}
	if got := vimVerticalMoveOffset(text, 8, -1); got != 1 {
		t.Fatalf("vimVerticalMoveOffset() up = %d, want 1", got)
	}
}

func TestVimDeleteChar(t *testing.T) {
	got, cursor := vimDeleteChar("abcd", 1)
	if got != "acd" || cursor != 1 {
		t.Fatalf("vimDeleteChar() = %q,%d want %q,%d", got, cursor, "acd", 1)
	}
}

func TestVimDeleteLine(t *testing.T) {
	got, cursor := vimDeleteLine("one\ntwo\nthree", 5)
	if got != "one\nthree" || cursor != 4 {
		t.Fatalf("vimDeleteLine() = %q,%d want %q,%d", got, cursor, "one\nthree", 4)
	}
}

func TestVimOpenLineBelow(t *testing.T) {
	got, cursor := vimOpenLineBelow("one\ntwo", 1)
	if got != "one\n\ntwo" || cursor != 4 {
		t.Fatalf("vimOpenLineBelow() = %q,%d want %q,%d", got, cursor, "one\n\ntwo", 4)
	}
}

func TestVimOpenLineAbove(t *testing.T) {
	got, cursor := vimOpenLineAbove("one\ntwo", 5)
	if got != "one\n\ntwo" || cursor != 4 {
		t.Fatalf("vimOpenLineAbove() = %q,%d want %q,%d", got, cursor, "one\n\ntwo", 4)
	}
}

func TestVimYankLine(t *testing.T) {
	reg := vimYankLine("one\ntwo\nthree", 5, 10)
	if reg.Kind != vimRegisterLine || reg.Text != "two\nthree\n" {
		t.Fatalf("vimYankLine() = %#v", reg)
	}
}

func TestVimYankChar(t *testing.T) {
	reg := vimYankChar("alpha", 1, 3)
	if reg.Kind != vimRegisterChar || reg.Text != "lph" {
		t.Fatalf("vimYankChar() = %#v", reg)
	}
}

func TestVimYankBlock(t *testing.T) {
	reg := vimYankBlock("abcd\nwxyz", 1, 7)
	if reg.Kind != vimRegisterBlock || reg.Width != 2 || len(reg.Lines) != 2 || reg.Lines[0] != "bc" || reg.Lines[1] != "xy" {
		t.Fatalf("vimYankBlock() = %#v", reg)
	}
}

func TestVimPasteChar(t *testing.T) {
	got, cursor := vimPasteChar("abcd", 2, vimRegister{Kind: vimRegisterChar, Text: "ZZ"})
	if got != "abZZcd" || cursor != 4 {
		t.Fatalf("vimPasteChar() = %q,%d want %q,%d", got, cursor, "abZZcd", 4)
	}
}

func TestVimDeleteRange(t *testing.T) {
	got, cursor := vimDeleteRange("abcdef", 1, 3)
	if got != "aef" || cursor != 1 {
		t.Fatalf("vimDeleteRange() = %q,%d want %q,%d", got, cursor, "aef", 1)
	}
}

func TestVimDeleteBlock(t *testing.T) {
	got, cursor := vimDeleteBlock("abcd\nwxyz", 1, 8)
	if got != "a\nw" || cursor != 1 {
		t.Fatalf("vimDeleteBlock() = %q,%d want %q,%d", got, cursor, "a\nw", 1)
	}
}

func TestVimWordForwardOffset(t *testing.T) {
	text := "alpha beta"
	if got := vimWordForwardOffset(text, 0); got != 6 {
		t.Fatalf("vimWordForwardOffset() = %d, want 6", got)
	}
}

func TestVimYankWord(t *testing.T) {
	reg := vimYankWord("alpha beta", 0)
	if reg.Kind != vimRegisterChar || reg.Text != "alpha " {
		t.Fatalf("vimYankWord() = %#v", reg)
	}
}

func TestVimDeleteWord(t *testing.T) {
	got, cursor := vimDeleteWord("alpha beta", 0)
	if got != "beta" || cursor != 0 {
		t.Fatalf("vimDeleteWord() = %q,%d want %q,%d", got, cursor, "beta", 0)
	}
}

func TestVimPasteLine(t *testing.T) {
	got, cursor := vimPasteLine("one\ntwo", 1, vimRegister{Kind: vimRegisterLine, Text: "aaa\n"})
	if got != "one\naaa\ntwo" || cursor != 4 {
		t.Fatalf("vimPasteLine() = %q,%d want %q,%d", got, cursor, "one\naaa\ntwo", 4)
	}
}

func TestVimPasteBlock(t *testing.T) {
	got, cursor := vimPasteBlock("abcd\nwxyz", 1, vimRegister{Kind: vimRegisterBlock, Lines: []string{"12", "34"}})
	if got != "a12bcd\nw34xyz" || cursor != 1 {
		t.Fatalf("vimPasteBlock() = %q,%d want %q,%d", got, cursor, "a12bcd\nw34xyz", 1)
	}
}
