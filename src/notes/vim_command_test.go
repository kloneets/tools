package notes

import (
	"strings"
	"testing"
)

func TestParseVimCommandSave(t *testing.T) {
	cmd, err := parseVimCommand("w")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandSave {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandSave)
	}
}

func TestParseVimCommandQuit(t *testing.T) {
	cmd, err := parseVimCommand("q")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandQuit {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandQuit)
	}
}

func TestParseVimCommandOneCharChain(t *testing.T) {
	cmd, err := parseVimCommand("wq")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandSequence || len(cmd.Commands) != 2 {
		t.Fatalf("cmd = %#v, want two-command sequence", cmd)
	}
	if cmd.Commands[0].Kind != vimCommandSave || cmd.Commands[1].Kind != vimCommandQuit || !cmd.Commands[1].Force {
		t.Fatalf("commands = %#v, want save then forced quit", cmd.Commands)
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

func TestParseVimCommandOpenLinks(t *testing.T) {
	cmd, err := parseVimCommand("ol")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandOpenLinks {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandOpenLinks)
	}
}

func TestParseVimCommandRecordKeys(t *testing.T) {
	cmd, err := parseVimCommand("recordkeys")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandRecordKeys {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandRecordKeys)
	}
}

func TestParseVimCommandSpell(t *testing.T) {
	cmd, err := parseVimCommand("spell")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandSpell {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandSpell)
	}
}

func TestParseVimCommandBufferDelete(t *testing.T) {
	cmd, err := parseVimCommand("bd")
	if err != nil {
		t.Fatalf("parseVimCommand() error = %v", err)
	}
	if cmd.Kind != vimCommandBufferDelete {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandBufferDelete)
	}
}

func TestParseVimCommandUndoRedo(t *testing.T) {
	cmd, err := parseVimCommand("undo")
	if err != nil {
		t.Fatalf("parseVimCommand(undo) error = %v", err)
	}
	if cmd.Kind != vimCommandUndo {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandUndo)
	}
	cmd, err = parseVimCommand("redo")
	if err != nil {
		t.Fatalf("parseVimCommand(redo) error = %v", err)
	}
	if cmd.Kind != vimCommandRedo {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandRedo)
	}
	cmd, err = parseVimCommand("preview")
	if err != nil {
		t.Fatalf("parseVimCommand(preview) error = %v", err)
	}
	if cmd.Kind != vimCommandPreview {
		t.Fatalf("kind = %q, want %q", cmd.Kind, vimCommandPreview)
	}
}

func TestParseVimCommandSidebarAliases(t *testing.T) {
	for _, raw := range []string{"sidebar", "sb"} {
		cmd, err := parseVimCommand(raw)
		if err != nil {
			t.Fatalf("parseVimCommand(%q) error = %v", raw, err)
		}
		if cmd.Kind != vimCommandSidebar {
			t.Fatalf("parseVimCommand(%q) kind = %q, want %q", raw, cmd.Kind, vimCommandSidebar)
		}
	}
}

func TestCollectSupportedLinksDedupesAndFilters(t *testing.T) {
	text := strings.Join([]string{
		"[docs](https://example.com/docs)",
		"https://example.com/docs",
		"ftp://example.com/pub/file.txt",
		"file:///tmp/example.txt",
		"[relative](assets/file.png)",
		"mailto:test@example.com",
	}, "\n")
	got := collectSupportedLinks(text)
	want := []string{
		"https://example.com/docs",
		"ftp://example.com/pub/file.txt",
		"file:///tmp/example.txt",
	}
	if len(got) != len(want) {
		t.Fatalf("collectSupportedLinks() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collectSupportedLinks()[%d] = %q, want %q", i, got[i], want[i])
		}
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
