package notes

import (
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
	"github.com/mattn/go-runewidth"
	"strings"
	"testing"
)

func TestCursorLineTracksLogicalLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	for _, tc := range []struct {
		name, text                 string
		cursor, width, highlighted int
	}{
		{"first", "abc\ndef", 0, 12, 1},
		{"second", "abc\ndef", 4, 12, 1},
		{"empty", "abc\n", 4, 12, 1},
		{"wrapped", "abcdefghijk\nx", 3, 8, 2},
		{"wide", "界界界界\nx", 0, 8, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ed := &Editor{Text: tc.text, Cursor: tc.cursor, Mode: ModeInsert}
			rows := buildEditorVisualRows(ed, tc.width)
			count := 0
			for _, row := range rows {
				if strings.Contains(row, helpers.ANSIRoleCursorLine) {
					count++
					if got := runewidth.StringWidth(helpers.StripANSI(row)); got != tc.width {
						t.Fatalf("width=%d want %d: %q", got, tc.width, row)
					}
				}
			}
			if count != tc.highlighted {
				t.Fatalf("highlighted=%d want %d: %q", count, tc.highlighted, rows)
			}
			if strings.Contains(strings.Join(renderPreviewPane(ed, tc.width, 4), ""), helpers.ANSIRoleCursorLine) {
				t.Fatal("preview highlighted")
			}
		})
	}
}

func TestCursorLineRestoresBackgroundAfterSpans(t *testing.T) {
	row := highlightCursorLine(helpers.ANSI(helpers.ANSIRoleSearch, "hit")+" tail", 12)
	if !strings.Contains(row, "\x1b[0m"+helpers.ANSIRoleCursorLine+" tail") {
		t.Fatalf("background lost: %q", row)
	}
	if !strings.HasSuffix(row, "\x1b[0m") {
		t.Fatal("style leaks")
	}
}
