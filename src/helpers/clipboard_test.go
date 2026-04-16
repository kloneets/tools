package helpers

import (
	"encoding/base64"
	"errors"
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

func TestCopyToClipboardUsesTestingHook(t *testing.T) {
	var copied string
	restore := SetClipboardWriterForTesting(func(text string) error {
		copied = text
		return nil
	})
	defer restore()
	if err := CopyToClipboard("hello"); err != nil {
		t.Fatalf("CopyToClipboard() error = %v", err)
	}
	if copied != "hello" {
		t.Fatalf("copied = %q, want %q", copied, "hello")
	}
}

func TestReadFromClipboardUsesTestingHook(t *testing.T) {
	restore := SetClipboardReaderForTesting(func() (string, error) {
		return "hello", nil
	})
	defer restore()
	got, err := ReadFromClipboard()
	if err != nil {
		t.Fatalf("ReadFromClipboard() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("ReadFromClipboard() = %q, want %q", got, "hello")
	}
}

func TestReadFromClipboardPropagatesError(t *testing.T) {
	restore := SetClipboardReaderForTesting(func() (string, error) {
		return "", errors.New("no clipboard")
	})
	defer restore()
	if _, err := ReadFromClipboard(); err == nil || err.Error() != "no clipboard" {
		t.Fatalf("ReadFromClipboard() error = %v, want propagated error", err)
	}
}
