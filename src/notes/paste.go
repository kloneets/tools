package notes

import "strings"

// HandlePaste inserts terminal paste data without interpreting it as keystrokes.
func (w *Workspace) HandlePaste(text string) bool {
	if w == nil || text == "" {
		return false
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if w.FocusSidebar {
		if w.sidebarSearch.editing {
			w.sidebarSearch.query += singleLinePaste(text)
			w.updateSidebarSearch()
			return true
		}
		if w.BrowserCommandMode {
			w.BrowserCommand += singleLinePaste(text)
			return true
		}
		return false
	}
	ed := w.ActiveEditor()
	if ed == nil || ed.ReplaceConfirm != nil {
		return false
	}
	if ed.Mode == ModeCommand {
		ed.Command += singleLinePaste(text)
		return true
	}
	rememberUndoState(ed)
	clearAutoComplete(ed)
	spans := visualHighlightSpans(ed)
	if ed.MultiCursorActive {
		starts := append([]int(nil), ed.MultiCursorStarts...)
		if ed.MultiCursorPadPending {
			starts = materializeMultiCursorPadding(ed, starts)
			ed.MultiCursorPadPending = false
		}
		spans = nil
		for i, start := range starts {
			spans = append(spans, markdownSpan{Start: start, End: start + ed.MultiCursorLengths[i]})
		}
	}
	if len(spans) == 0 {
		at := vimClampOffset(ed.Text, ed.Cursor)
		spans = []markdownSpan{{Start: at, End: at}}
	}
	runes := []rune(ed.Text)
	insert := []rune(text)
	var result strings.Builder
	positions := make([]int, 0, len(spans))
	previous, offset := 0, 0
	for _, span := range spans {
		start := max(previous, min(span.Start, len(runes)))
		end := max(start, min(span.End, len(runes)))
		result.WriteString(string(runes[previous:start]))
		result.WriteString(text)
		offset += start - previous + len(insert)
		positions = append(positions, offset)
		previous = end
	}
	result.WriteString(string(runes[previous:]))
	ed.Text = result.String()
	ed.Cursor = positions[0]
	ed.Dirty = true
	ed.PasteBoundaryActive = true
	if ed.MultiCursorActive {
		ed.MultiCursorStarts = positions
		ed.MultiCursorLengths = make([]int, len(positions))
		ed.MultiCursorQuery = text
	}
	clearVisualSelection(ed)
	ed.PendingOp = ""
	ed.NormalCount = ""
	if ed.Mode == ModeVisual {
		ed.Mode = ModeInsert
	}
	w.ensureEditorVisible()
	return true
}

func singleLinePaste(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, text)
}
