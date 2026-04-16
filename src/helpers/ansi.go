package helpers

import "strings"

const ansiReset = "\x1b[0m"

func StripANSI(s string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		if !in && s[i] == 0x1b {
			in = true
			continue
		}
		if in {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				in = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func VisibleRuneCount(s string) int {
	return len([]rune(StripANSI(s)))
}

func TruncateANSI(s string, width int) string {
	if width <= 0 {
		return ""
	}
	plain := []rune(StripANSI(s))
	if len(plain) <= width {
		return s
	}
	return string(plain[:width])
}

func PadANSI(s string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := VisibleRuneCount(s)
	if visible >= width {
		return TruncateANSI(s, width)
	}
	return s + strings.Repeat(" ", width-visible)
}

func SanitizeSingleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

func ANSI(style string, text string) string {
	if text == "" {
		return ""
	}
	return style + text + ansiReset
}

const (
	ANSIBold      = "\x1b[1m"
	ANSIItalic    = "\x1b[3m"
	ANSIDim       = "\x1b[2m"
	ANSIReverse   = "\x1b[7m"
	ANSIFgBlue    = "\x1b[38;5;75m"
	ANSIFgGreen   = "\x1b[38;5;114m"
	ANSIFgYellow  = "\x1b[38;5;221m"
	ANSIFgPurple  = "\x1b[38;5;141m"
	ANSIFgCyan    = "\x1b[38;5;117m"
	ANSIFgGray    = "\x1b[38;5;244m"
	ANSIFgOrange  = "\x1b[38;5;215m"
)
