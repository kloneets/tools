package helpers

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var clipboardWriter = defaultClipboardWriter
var clipboardReader = defaultClipboardReader

func CopyToClipboard(text string) error {
	return clipboardWriter(text)
}

func ReadFromClipboard() (string, error) {
	return clipboardReader()
}

func SetClipboardWriterForTesting(fn func(string) error) func() {
	previous := clipboardWriter
	if fn == nil {
		clipboardWriter = defaultClipboardWriter
	} else {
		clipboardWriter = fn
	}
	return func() {
		clipboardWriter = previous
	}
}

func SetClipboardReaderForTesting(fn func() (string, error)) func() {
	previous := clipboardReader
	if fn == nil {
		clipboardReader = defaultClipboardReader
	} else {
		clipboardReader = fn
	}
	return func() {
		clipboardReader = previous
	}
}

func defaultClipboardWriter(text string) error {
	if err := writeNativeClipboard(text); err == nil {
		return nil
	}
	seq := clipboardSequence(text, os.Getenv("TMUX"))
	if seq == "" {
		return fmt.Errorf("clipboard copy is not available")
	}
	_, err := os.Stdout.WriteString(seq)
	return err
}

func defaultClipboardReader() (string, error) {
	if text, err := readNativeClipboard(); err == nil {
		return text, nil
	}
	return "", fmt.Errorf("clipboard paste is not available")
}

func writeNativeClipboard(text string) error {
	for _, cmd := range clipboardWriteCommands() {
		if err := runClipboardWriteCommand(cmd[0], cmd[1:], text); err == nil {
			return nil
		}
	}
	return fmt.Errorf("clipboard copy is not available")
}

func readNativeClipboard() (string, error) {
	for _, cmd := range clipboardReadCommands() {
		if text, err := runClipboardReadCommand(cmd[0], cmd[1:]); err == nil {
			return text, nil
		}
	}
	return "", fmt.Errorf("clipboard paste is not available")
}

func clipboardWriteCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	default:
		return [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
	}
}

func clipboardReadCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbpaste"}}
	default:
		return [][]string{
			{"wl-paste", "-n"},
			{"xclip", "-selection", "clipboard", "-o"},
			{"xsel", "--clipboard", "--output"},
		}
	}
}

func runClipboardWriteCommand(name string, args []string, text string) error {
	if _, err := exec.LookPath(name); err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func runClipboardReadCommand(name string, args []string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func clipboardSequence(text string, tmux string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	if encoded == "" {
		encoded = base64.StdEncoding.EncodeToString([]byte{})
	}
	osc := "\x1b]52;c;" + encoded + "\a"
	if tmux != "" {
		return "\x1bPtmux;\x1b" + strings.ReplaceAll(osc, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	return osc
}
