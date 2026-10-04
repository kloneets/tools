package notes

import (
	"fmt"
	"sort"

	"github.com/kloneets/tools/src/settings"
)

// EnterMultiCursor uses the current visual selection, or the word at the
// current cursor, as the text to select at every cursor.
func EnterMultiCursor(ed *Editor) bool {
	if ed == nil {
		return false
	}
	start, end := 0, 0
	if ed.SelectionMode == vimSelectionChar {
		start = min(ed.SelectionMark, ed.SelectionCursor)
		end = max(ed.SelectionMark, ed.SelectionCursor) + 1
	} else if ed.SelectionMode == vimSelectionLine {
		start, end = vimLineRange(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	} else if ed.SelectionMode == vimSelectionBlock {
		return EnterBlockInsertMultiCursor(ed)
	} else {
		start, end = multiCursorWordRange(ed.Text, ed.Cursor)
	}
	if end <= start {
		ed.Status = "multi-cursor needs a word or selection"
		return false
	}
	ExitMultiCursor(ed)
	ed.MultiCursorActive = true
	ed.MultiCursorStarts = []int{start}
	ed.MultiCursorLengths = []int{end - start}
	ed.MultiCursorQuery = string([]rune(ed.Text)[start:end])
	ed.MultiCursorBlockMode = false
	ed.MultiCursorPadPending = false
	ed.MultiCursorPadColumn = 0
	ed.Cursor = start
	ed.SelectionMode = vimSelectionNone
	ed.SelectionMark = 0
	ed.SelectionCursor = 0
	ed.NormalCount = ""
	ed.PendingOp = ""
	ed.Mode = ModeInsert
	ed.Status = "multi-cursor: 1 cursor; Alt+N adds next match"
	return true
}

// EnterBlockInsertMultiCursor starts mirrored insertion before the selected
// rectangle on every selected row. Short rows are padded lazily on first edit.
func EnterBlockInsertMultiCursor(ed *Editor) bool {
	if ed == nil || ed.SelectionMode != vimSelectionBlock {
		return false
	}
	lines := vimLineInfos(ed.Text)
	startRow, endRow, startCol, _ := vimLineColumns(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	if startRow < 0 || endRow >= len(lines) || startRow > endRow {
		return false
	}
	starts := make([]int, 0, endRow-startRow+1)
	lengths := make([]int, 0, endRow-startRow+1)
	for row := startRow; row <= endRow; row++ {
		line := lines[row]
		lineColumn := min(startCol, line.end-line.start)
		starts = append(starts, line.start+lineColumn)
		lengths = append(lengths, 0)
	}
	ExitMultiCursor(ed)
	ed.MultiCursorActive = true
	ed.MultiCursorStarts, ed.MultiCursorLengths = starts, lengths
	ed.MultiCursorQuery = ""
	ed.MultiCursorBlockMode = true
	ed.MultiCursorPadColumn = startCol
	ed.MultiCursorPadPending = true
	ed.Cursor = starts[0]
	ed.SelectionMode = vimSelectionNone
	ed.SelectionMark = 0
	ed.SelectionCursor = 0
	ed.NormalCount = ""
	ed.PendingOp = ""
	ed.Mode = ModeInsert
	ed.Status = fmt.Sprintf("block insert: %d cursors; type once, Esc exits", len(starts))
	return true
}

// applyBlockVisualOperator applies d/c motion independently on each selected
// row, with all ranges calculated from the original rune offsets.
func applyBlockVisualOperator(w *Workspace, ed *Editor, motion string) bool {
	if ed == nil || ed.SelectionMode != vimSelectionBlock ||
		(ed.PendingOp != "d" && ed.PendingOp != "c") {
		return false
	}
	lines := vimLineInfos(ed.Text)
	startRow, endRow, startCol, _ := vimLineColumns(ed.Text, ed.SelectionMark, ed.SelectionCursor)
	if startRow < 0 || endRow >= len(lines) || startRow > endRow {
		return false
	}
	runes := []rune(ed.Text)
	operator := ed.PendingOp
	edits := make([]blockRowEdit, 0, endRow-startRow+1)
	count := normalModeCount(ed)
	if count <= 0 {
		count = 1
	}
	for row := startRow; row <= endRow; row++ {
		line := lines[row]
		column := startCol
		if column >= line.end-line.start {
			edits = append(edits, blockRowEdit{start: line.end, end: line.end})
			continue
		}
		start := line.start + column
		end := start
		switch motion {
		case "w":
			end = start
			for step := 0; step < count; step++ {
				wordStart, wordEnd := vimStrictWordRangeWithinLine(runes, end, line.end)
				if wordStart >= line.end || wordEnd <= end {
					break
				}
				end = wordEnd
			}
		case "b":
			for step := 0; step < count; step++ {
				previous := vimWordStartWithinLine(runes, start, line.start)
				if previous == start {
					break
				}
				start = previous
			}
		case "$", "end":
			end = line.end
		default:
			return false
		}
		if end < start {
			start, end = end, start
		}
		edits = append(edits, blockRowEdit{start: start, end: end, deleted: string(runes[start:end])})
	}
	ed.PendingOp = ""
	ed.NormalCount = ""
	changed := false
	for _, edit := range edits {
		if edit.start < edit.end {
			changed = true
			break
		}
	}
	if !changed {
		ed.Status = "block motion made no changes"
		return true
	}
	return applyBlockRowEdits(w, ed, edits, operator == "c")
}

type blockRowEdit struct {
	start, end int
	deleted    string
}

func vimStrictWordRangeWithinLine(runes []rune, offset int, lineEnd int) (int, int) {
	start := offset
	for start < lineEnd && !vimIsWordRune(runes[start]) {
		start++
	}
	end := start
	for end < lineEnd && vimIsWordRune(runes[end]) {
		end++
	}
	return start, end
}

func vimWordStartWithinLine(runes []rune, offset int, lineStart int) int {
	start := offset
	if start > lineStart {
		start--
	}
	for start > lineStart && !vimIsWordRune(runes[start]) {
		start--
	}
	for start > lineStart && vimIsWordRune(runes[start-1]) {
		start--
	}
	if !vimIsWordRune(runes[start]) {
		return offset
	}
	return start
}

func applyBlockRowEdits(w *Workspace, ed *Editor, edits []blockRowEdit, change bool) bool {
	lines := make([]string, 0, len(edits))
	width := 0
	changedEdits := make([]blockRowEdit, 0, len(edits))
	for _, edit := range edits {
		lines = append(lines, edit.deleted)
		width = max(width, len([]rune(edit.deleted)))
		if edit.start < edit.end {
			changedEdits = append(changedEdits, edit)
		}
	}
	if len(changedEdits) == 0 {
		return true
	}
	register := vimRegister{Kind: vimRegisterBlock, Lines: lines, Width: width}
	ed.Register = register
	if w != nil {
		w.Register = register
	}
	updateClipboardForRegister(w, ed, "deleted block motion")
	rememberUndoState(ed)
	runes := []rune(ed.Text)
	for i := len(changedEdits) - 1; i >= 0; i-- {
		edit := changedEdits[i]
		runes = append(runes[:edit.start], runes[edit.end:]...)
	}
	ed.Text = string(runes)
	starts := make([]int, 0, len(changedEdits))
	for _, edit := range changedEdits {
		removedBefore := 0
		for _, prior := range changedEdits {
			if prior.start >= edit.start {
				break
			}
			removedBefore += prior.end - prior.start
		}
		starts = append(starts, edit.start-removedBefore)
	}
	ed.Cursor = starts[0]
	ed.Dirty = true
	clearVisualSelection(ed)
	if change {
		ed.MultiCursorActive = true
		ed.MultiCursorStarts = starts
		ed.MultiCursorLengths = make([]int, len(starts))
		ed.MultiCursorQuery = ""
		ed.MultiCursorBlockMode = true
		ed.MultiCursorPadColumn = 0
		ed.MultiCursorPadPending = false
		ed.Mode = ModeInsert
		ed.Status = fmt.Sprintf("block change: %d cursors", len(starts))
	} else {
		ed.Mode = ModeNormal
		ed.Status = "deleted block motion"
	}
	return true
}

func multiCursorWordRange(text string, offset int) (int, int) {
	runes := []rune(text)
	start := vimClampOffset(text, offset)
	if start >= len(runes) || !vimIsWordRune(runes[start]) {
		if start > 0 && vimIsWordRune(runes[start-1]) {
			start--
		} else {
			for start < len(runes) && !vimIsWordRune(runes[start]) {
				start++
			}
		}
	}
	if start >= len(runes) {
		return start, start
	}
	for start > 0 && vimIsWordRune(runes[start-1]) {
		start--
	}
	end := start
	for end < len(runes) && vimIsWordRune(runes[end]) {
		end++
	}
	return start, end
}

// AddNextMultiCursor selects the next exact, non-overlapping occurrence after
// the final cursor. It deliberately does not wrap around.
func AddNextMultiCursor(ed *Editor) bool {
	if ed == nil || !ed.MultiCursorActive || ed.MultiCursorBlockMode || len(ed.MultiCursorStarts) == 0 || ed.MultiCursorQuery == "" {
		return false
	}
	runes := []rune(ed.Text)
	query := []rune(ed.MultiCursorQuery)
	last := len(ed.MultiCursorStarts) - 1
	minimum := ed.MultiCursorStarts[last] + ed.MultiCursorLengths[last]
	for start := minimum; start+len(query) <= len(runes); start++ {
		if equalRunesAt(runes, query, start) {
			ed.MultiCursorStarts = append(ed.MultiCursorStarts, start)
			ed.MultiCursorLengths = append(ed.MultiCursorLengths, len(query))
			ed.MultiCursorStarts, ed.MultiCursorLengths = normalizeMultiCursorRanges(ed.MultiCursorStarts, ed.MultiCursorLengths)
			ed.Status = fmt.Sprintf("multi-cursor: %d cursors", len(ed.MultiCursorStarts))
			return true
		}
	}
	ed.Status = "no more occurrences"
	return false
}

func equalRunesAt(text, query []rune, start int) bool {
	if start < 0 || start+len(query) > len(text) {
		return false
	}
	for i, r := range query {
		if text[start+i] != r {
			return false
		}
	}
	return true
}

func ExitMultiCursor(ed *Editor) {
	if ed == nil || !ed.MultiCursorActive {
		return
	}
	primary := ed.Cursor
	ed.MultiCursorActive = false
	ed.MultiCursorStarts = nil
	ed.MultiCursorLengths = nil
	ed.MultiCursorQuery = ""
	ed.MultiCursorBlockMode = false
	ed.MultiCursorPadColumn = 0
	ed.MultiCursorPadPending = false
	ed.Cursor = vimClampOffset(ed.Text, primary)
	if settings.Inst().NotesApp.VimMode {
		ed.Mode = ModeNormal
	} else {
		ed.Mode = ModeInsert
	}
}

func handleMultiCursorInsert(ed *Editor, key Key) bool {
	if key.Alt && key.Name == "n" {
		AddNextMultiCursor(ed)
		return true
	}
	if key.Ctrl || key.Meta || key.Alt {
		return true
	}
	if key.Name == "esc" {
		ExitMultiCursor(ed)
		return true
	}
	runes := []rune(ed.Text)
	starts := append([]int(nil), ed.MultiCursorStarts...)
	lengths := append([]int(nil), ed.MultiCursorLengths...)
	if len(starts) == 0 {
		ExitMultiCursor(ed)
		return true
	}
	if key.Name == "left" || key.Name == "right" || key.Name == "home" || key.Name == "end" || key.Name == "pageup" || key.Name == "pagedown" {
		ed.MultiCursorPadPending = false
		for i, start := range starts {
			if key.Name == "pageup" {
				starts[i] = vimPageMoveOffset(ed.Text, start, -10)
			} else if key.Name == "pagedown" {
				starts[i] = vimPageMoveOffset(ed.Text, start, 10)
			} else if key.Name == "right" {
				starts[i] = min(len(runes), start+max(1, lengths[i]))
			} else if key.Name == "left" {
				starts[i] = max(0, start-1)
			} else if key.Name == "home" {
				starts[i] = vimLineBoundaryOffset(ed.Text, start, false)
			} else {
				starts[i] = vimLineBoundaryOffset(ed.Text, start, true)
			}
		}
		for i := range lengths {
			lengths[i] = 0
		}
		ed.MultiCursorStarts, ed.MultiCursorLengths = normalizeMultiCursorRanges(starts, lengths)
		ed.Cursor = ed.MultiCursorStarts[0]
		return true
	}
	if key.Name == "up" || key.Name == "down" {
		ed.MultiCursorPadPending = false
		delta := -1
		if key.Name == "down" {
			delta = 1
		}
		for i, start := range starts {
			starts[i] = vimVerticalMoveOffset(ed.Text, start, delta)
		}
		for i := range lengths {
			lengths[i] = 0
		}
		ed.MultiCursorStarts, ed.MultiCursorLengths = normalizeMultiCursorRanges(starts, lengths)
		ed.Cursor = ed.MultiCursorStarts[0]
		return true
	}

	var insert []rune
	switch key.Name {
	case "backspace", "delete":
	case "enter":
		insert = []rune{'\n'}
	case "tab":
		insert = []rune{'\t'}
	default:
		if key.Rune != 0 {
			insert = []rune{key.Rune}
		} else {
			return true
		}
	}
	changed := len(insert) > 0
	if !changed {
		for i, cursor := range starts {
			if lengths[i] > 0 || (key.Name == "backspace" && cursor > 0) || (key.Name == "delete" && cursor < len(runes)) {
				changed = true
				break
			}
		}
	}
	if !changed {
		return true
	}

	// Build from right to left so all original rune offsets remain valid.
	if len(insert) > 0 {
		rememberTypedInsertBoundary(ed, insert[0])
		if ed.MultiCursorPadPending {
			starts = materializeMultiCursorPadding(ed, starts)
			runes = []rune(ed.Text)
			ed.MultiCursorPadPending = false
		}
	} else {
		rememberUndoState(ed)
		ed.MultiCursorPadPending = false
	}
	newRunes := append([]rune(nil), runes...)
	newStarts := make([]int, len(starts))
	for i := len(starts) - 1; i >= 0; i-- {
		originalStart := starts[i]
		length := lengths[i]
		start := originalStart
		end := start + length
		if length == 0 {
			switch key.Name {
			case "backspace":
				start = max(0, start-1)
			case "delete":
				end = min(len(runes), start+1)
			}
		}
		start = max(0, min(start, len(newRunes)))
		end = max(start, min(end, len(newRunes)))
		replacementEnd := start + len(insert)
		newRunes = append(newRunes[:start], append(append([]rune(nil), insert...), newRunes[end:]...)...)
		newStarts[i] = replacementEnd
		delta := replacementEnd - end
		for j := i + 1; j < len(newStarts); j++ {
			newStarts[j] += delta
		}
	}
	newStarts = uniqueSortedOffsets(newStarts)
	ed.Text = string(newRunes)
	ed.MultiCursorStarts, ed.MultiCursorLengths = normalizeMultiCursorRanges(newStarts, make([]int, len(newStarts)))
	if len(insert) > 0 {
		ed.MultiCursorQuery = string(insert)
	} else {
		ed.MultiCursorQuery = ""
	}
	ed.Cursor = newStarts[0]
	ed.Dirty = true
	return true
}

// materializeMultiCursorPadding extends short block rows to the insertion
// column, processing bottom-up so original offsets on earlier rows stay valid.
func materializeMultiCursorPadding(ed *Editor, starts []int) []int {
	if ed == nil || !ed.MultiCursorPadPending || len(starts) == 0 {
		return starts
	}
	lines := vimLineInfos(ed.Text)
	runes := []rune(ed.Text)
	for i := len(starts) - 1; i >= 0; i-- {
		lineIndex := vimLineIndexAtOffset(ed.Text, starts[i])
		if lineIndex < 0 || lineIndex >= len(lines) {
			continue
		}
		line := lines[lineIndex]
		column := starts[i] - line.start
		padding := ed.MultiCursorPadColumn - column
		if padding <= 0 {
			continue
		}
		spaces := make([]rune, padding)
		for j := range spaces {
			spaces[j] = ' '
		}
		runes = append(runes[:line.end], append(spaces, runes[line.end:]...)...)
		ed.Text = string(runes)
		lines = vimLineInfos(ed.Text)
		starts[i] += padding
		for j := i + 1; j < len(starts); j++ {
			starts[j] += padding
		}
	}
	return starts
}

func uniqueSortedOffsets(offsets []int) []int {
	sort.Ints(offsets)
	unique := offsets[:0]
	for _, offset := range offsets {
		if len(unique) == 0 || unique[len(unique)-1] != offset {
			unique = append(unique, offset)
		}
	}
	return unique
}

func normalizeMultiCursorRanges(starts, lengths []int) ([]int, []int) {
	type cursorRange struct{ start, end int }
	ranges := make([]cursorRange, 0, len(starts))
	for i, start := range starts {
		length := 0
		if i < len(lengths) {
			length = lengths[i]
		}
		ranges = append(ranges, cursorRange{start: start, end: start + max(0, length)})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	merged := make([]cursorRange, 0, len(ranges))
	for _, current := range ranges {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}
		last := &merged[len(merged)-1]
		if current.start == last.start || current.start < last.end {
			if current.end > last.end {
				last.end = current.end
			}
			continue
		}
		merged = append(merged, current)
	}
	starts = starts[:0]
	lengths = lengths[:0]
	for _, current := range merged {
		starts = append(starts, current.start)
		lengths = append(lengths, current.end-current.start)
	}
	return starts, lengths
}
