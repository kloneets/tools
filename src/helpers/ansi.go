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
	ANSIBold     = "\x1b[1m"
	ANSIItalic   = "\x1b[3m"
	ANSIDim      = "\x1b[2m"
	ANSIReverse  = "\x1b[7m"
	ANSIFgBlue   = "\x1b[38;5;75m"
	ANSIFgGreen  = "\x1b[38;5;114m"
	ANSIFgYellow = "\x1b[38;5;221m"
	ANSIFgPurple = "\x1b[38;5;141m"
	ANSIFgCyan   = "\x1b[38;5;117m"
	ANSIFgGray   = "\x1b[38;5;244m"
	ANSIFgOrange = "\x1b[38;5;215m"
)

const (
	ANSIRoleHeading1        = "\x1b[9001m"
	ANSIRoleHeading2        = "\x1b[9002m"
	ANSIRoleHeading3        = "\x1b[9003m"
	ANSIRoleHeading4        = "\x1b[9004m"
	ANSIRoleHeading5        = "\x1b[9005m"
	ANSIRoleHeading6        = "\x1b[9006m"
	ANSIRoleListMarker      = "\x1b[9007m"
	ANSIRoleLink            = "\x1b[9008m"
	ANSIRoleCode            = "\x1b[9009m"
	ANSIRoleString          = "\x1b[9010m"
	ANSIRoleKeyword         = "\x1b[9011m"
	ANSIRoleNumber          = "\x1b[9012m"
	ANSIRoleComment         = "\x1b[9013m"
	ANSIRoleType            = "\x1b[9014m"
	ANSIRoleFunction        = "\x1b[9015m"
	ANSIRoleProperty        = "\x1b[9016m"
	ANSIRoleConstant        = "\x1b[9017m"
	ANSIRoleSearch          = "\x1b[9018m"
	ANSIRoleVisualSelection = "\x1b[9019m"
	ANSIRoleActiveTab       = "\x1b[9020m"
	ANSIRoleSelection       = "\x1b[9021m"
	ANSIRoleSpellError      = "\x1b[9022m"
)
