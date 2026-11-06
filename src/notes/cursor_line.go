package notes

import (
	"strings"

	"github.com/kloneets/tools/src/helpers"
	"github.com/mattn/go-runewidth"
)

// Restore the line background after syntax and selection spans reset their style.
func highlightCursorLine(row string, width int) string {
	const reset = "\x1b[0m"
	padding := max(0, width-runewidth.StringWidth(helpers.StripANSI(row)))
	row += strings.Repeat(" ", padding)
	return helpers.ANSIRoleCursorLine + strings.ReplaceAll(row, reset, reset+helpers.ANSIRoleCursorLine) + reset
}
