package helpers

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestClipboardSequencePlain(t *testing.T) {
	got := clipboardSequence("hello", "")
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("hello")) + "\a"
	if got != want {
		t.Fatalf("clipboardSequence() = %q, want %q", got, want)
	}
}

func TestClipboardSequenceTmuxWrapped(t *testing.T) {
	got := clipboardSequence("hello", "/tmp/tmux")
	if !strings.HasPrefix(got, "\x1bPtmux;") {
		t.Fatalf("clipboardSequence() = %q, want tmux prefix", got)
	}
	if !strings.Contains(got, base64.StdEncoding.EncodeToString([]byte("hello"))) {
		t.Fatalf("clipboardSequence() = %q, want encoded payload", got)
	}
}
