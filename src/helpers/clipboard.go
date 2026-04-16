package helpers

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func CopyToClipboard(text string) error {
	seq := clipboardSequence(text, os.Getenv("TMUX"))
	if seq == "" {
		return fmt.Errorf("clipboard copy is not available")
	}
	_, err := os.Stdout.WriteString(seq)
	return err
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
