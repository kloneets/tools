package notes

import "testing"

func TestParseVimCommandSave(t *testing.T) {
	cmd, err := parseVimCommand("w")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandSave {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandSave)
	}
}

func TestParseVimCommandSearch(t *testing.T) {
	cmd, err := parseVimCommand("/needle")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandSearch || cmd.Query != "needle" {
		t.Fatalf("cmd = %#v, want search needle", cmd)
	}
}

func TestParseVimCommandReplace(t *testing.T) {
	cmd, err := parseVimCommand("%s/old/new/g")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandReplace || cmd.Query != "old" || cmd.Replacement != "new" || !cmd.Global || cmd.CurrentLine {
		t.Fatalf("cmd = %#v, want global replace old->new", cmd)
	}
}

func TestParseVimCommandCurrentLineReplace(t *testing.T) {
	cmd, err := parseVimCommand("s/old/new/g")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandReplace || !cmd.CurrentLine || !cmd.Global {
		t.Fatalf("cmd = %#v, want current-line global replace", cmd)
	}
}

func TestFindNext(t *testing.T) {
	if got := findNext("abc abc", "abc", 1); got != 4 {
		t.Fatalf("findNext() = %d, want 4", got)
	}
}

func TestFindNextIsCaseInsensitive(t *testing.T) {
	if got := findNext("Alpha beta ALPHA", "alpha", 1); got != 11 {
		t.Fatalf("findNext() case-insensitive = %d, want 11", got)
	}
}

func TestFindPrevious(t *testing.T) {
	if got := findPrevious("abc abc abc", "abc", 6); got != 4 {
		t.Fatalf("findPrevious() = %d, want 4", got)
	}
}

func TestFindPreviousIsCaseInsensitive(t *testing.T) {
	if got := findPrevious("Alpha beta ALPHA", "alpha", 14); got != 11 {
		t.Fatalf("findPrevious() case-insensitive = %d, want 11", got)
	}
}

func TestReplaceTextGlobal(t *testing.T) {
	got, count := replaceText("one two one", "one", "1", true)
	if got != "1 two 1" || count != 2 {
		t.Fatalf("replaceText() = %q,%d want %q,%d", got, count, "1 two 1", 2)
	}
}

func TestReplaceTextSingle(t *testing.T) {
	got, count := replaceText("one two one", "one", "1", false)
	if got != "1 two one" || count != 1 {
		t.Fatalf("replaceText() = %q,%d want %q,%d", got, count, "1 two one", 1)
	}
}

func TestReplaceTextInRange(t *testing.T) {
	got, count := replaceTextInRange("one\ntwo one\none", "one", "1", true, 4, 11)
	if got != "one\ntwo 1\none" || count != 1 {
		t.Fatalf("replaceTextInRange() = %q,%d want %q,%d", got, count, "one\ntwo 1\none", 1)
	}
}
