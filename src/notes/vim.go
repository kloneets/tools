package notes

import (
	"strings"
	"unicode"
)

type vimLineInfo struct {
	start int
	end   int
}

type vimSelectionMode string

const (
	vimSelectionNone  vimSelectionMode = ""
	vimSelectionChar  vimSelectionMode = "char"
	vimSelectionLine  vimSelectionMode = "line"
	vimSelectionBlock vimSelectionMode = "block"
)

type vimRegisterKind string

const (
	vimRegisterChar  vimRegisterKind = "char"
	vimRegisterLine  vimRegisterKind = "line"
	vimRegisterBlock vimRegisterKind = "block"
)

type vimRegister struct {
	Kind  vimRegisterKind
	Text  string
	Lines []string
	Width int
}

func vimLineInfos(text string) []vimLineInfo {
	runes := []rune(text)
	lines := make([]vimLineInfo, 0, 8)
	start := 0
	for i, r := range runes {
		if r == '\n' {
			lines = append(lines, vimLineInfo{start: start, end: i})
			start = i + 1
		}
	}
	lines = append(lines, vimLineInfo{start: start, end: len(runes)})
	return lines
}

func vimClampOffset(text string, offset int) int {
	runeCount := len([]rune(text))
	if offset < 0 {
		return 0
	}
	if offset > runeCount {
		return runeCount
	}
	return offset
}

func vimLineIndexAtOffset(text string, offset int) int {
	offset = vimClampOffset(text, offset)
	lines := vimLineInfos(text)
	for i, line := range lines {
		if offset >= line.start && offset <= line.end {
			return i
		}
	}
	return len(lines) - 1
}

func vimLineBoundaryOffset(text string, offset int, end bool) int {
	lines := vimLineInfos(text)
	lineIdx := vimLineIndexAtOffset(text, offset)
	if end {
		return lines[lineIdx].end
	}
	return lines[lineIdx].start
}

func vimVerticalMoveOffset(text string, offset int, delta int) int {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return 0
	}
	currentIdx := vimLineIndexAtOffset(text, offset)
	targetIdx := currentIdx + delta
	if targetIdx < 0 {
		targetIdx = 0
	}
	if targetIdx >= len(lines) {
		targetIdx = len(lines) - 1
	}
	column := offset - lines[currentIdx].start
	if offset == lines[currentIdx].end && column > 0 {
		column--
	}
	targetWidth := lines[targetIdx].end - lines[targetIdx].start
	if column > targetWidth {
		column = targetWidth
	}
	return lines[targetIdx].start + column
}

func vimDeleteChar(text string, offset int) (string, int) {
	runes := []rune(text)
	offset = vimClampOffset(text, offset)
	if offset >= len(runes) || runes[offset] == '\n' {
		return text, offset
	}
	return string(append(runes[:offset], runes[offset+1:]...)), offset
}

func vimReplaceChar(text string, offset int, replacement rune) (string, int) {
	runes := []rune(text)
	offset = vimClampOffset(text, offset)
	if offset >= len(runes) || runes[offset] == '\n' {
		return text, offset
	}
	runes[offset] = replacement
	return string(runes), offset
}

func vimDeleteLine(text string, offset int) (string, int) {
	runes := []rune(text)
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return text, 0
	}
	lineIdx := vimLineIndexAtOffset(text, offset)
	start := lines[lineIdx].start
	end := lines[lineIdx].end
	if lineIdx < len(lines)-1 {
		end++
	} else if lineIdx > 0 && start > 0 {
		start--
	}
	updated := string(append(runes[:start], runes[end:]...))
	if len(updated) == 0 {
		return "", 0
	}
	newOffset := start
	if lineIdx >= len(vimLineInfos(updated)) {
		newOffset = len([]rune(updated))
	}
	return updated, vimLineBoundaryOffset(updated, newOffset, false)
}

func vimDeleteLineSpan(text string, offset int, deltaLines int) (string, int, vimRegister, bool) {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return text, 0, vimRegister{}, false
	}
	currentIdx := vimLineIndexAtOffset(text, offset)
	startIdx := currentIdx
	endIdx := currentIdx + deltaLines
	if endIdx < startIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	start := lines[startIdx].start
	end := lines[endIdx].end
	if endIdx < len(lines)-1 {
		end++
	} else if startIdx > 0 && start > 0 {
		start--
	}
	reg := vimYankLine(text, lines[startIdx].start, lines[endIdx].start)
	runes := []rune(text)
	if start >= len(runes) {
		return text, vimClampOffset(text, start), reg, false
	}
	updated := string(append(runes[:start], runes[end:]...))
	if len(updated) == 0 {
		return "", 0, reg, true
	}
	newOffset := start
	if startIdx >= len(vimLineInfos(updated)) {
		newOffset = len([]rune(updated))
	}
	return updated, vimLineBoundaryOffset(updated, newOffset, false), reg, true
}

func vimDeleteToLineEnd(text string, offset int) (string, int) {
	runes := []rune(text)
	start := vimClampOffset(text, offset)
	end := vimLineBoundaryOffset(text, offset, true)
	if start >= len(runes) || start >= end {
		return text, start
	}
	return string(append(runes[:start], runes[end:]...)), start
}

func vimOpenLineBelow(text string, offset int) (string, int) {
	runes := []rune(text)
	insertAt := vimLineBoundaryOffset(text, offset, true)
	if insertAt == len(runes) {
		updatedRunes := make([]rune, 0, len(runes)+1)
		updatedRunes = append(updatedRunes, runes...)
		updatedRunes = append(updatedRunes, '\n')
		updated := string(updatedRunes)
		return updated, len(runes) + 1
	}
	updatedRunes := make([]rune, 0, len(runes)+1)
	updatedRunes = append(updatedRunes, runes[:insertAt]...)
	updatedRunes = append(updatedRunes, '\n')
	updatedRunes = append(updatedRunes, runes[insertAt:]...)
	updated := string(updatedRunes)
	return updated, insertAt + 1
}

func vimOpenLineAbove(text string, offset int) (string, int) {
	runes := []rune(text)
	insertAt := vimLineBoundaryOffset(text, offset, false)
	updatedRunes := make([]rune, 0, len(runes)+1)
	updatedRunes = append(updatedRunes, runes[:insertAt]...)
	updatedRunes = append(updatedRunes, '\n')
	updatedRunes = append(updatedRunes, runes[insertAt:]...)
	updated := string(updatedRunes)
	return updated, insertAt
}

func vimLineRange(text string, startOffset int, endOffset int) (int, int) {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return 0, 0
	}
	startIdx := vimLineIndexAtOffset(text, startOffset)
	endIdx := vimLineIndexAtOffset(text, endOffset)
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	start := lines[startIdx].start
	end := lines[endIdx].end
	if endIdx < len(lines)-1 {
		end++
	}
	return start, end
}

func vimLineIndexRange(text string, startOffset int, endOffset int) (int, int) {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return 0, 0
	}
	startIdx := vimLineIndexAtOffset(text, startOffset)
	endIdx := vimLineIndexAtOffset(text, endOffset)
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	return startIdx, endIdx
}

func vimShiftLines(text string, cursor int, startOffset int, endOffset int, indentWidth int, right bool) (string, int, bool) {
	updated, offsets, changed := vimShiftLinesAndOffsets(text, startOffset, endOffset, indentWidth, right, []int{cursor})
	return updated, offsets[0], changed
}

func vimShiftLinesAndOffsets(text string, startOffset int, endOffset int, indentWidth int, right bool, offsets []int) (string, []int, bool) {
	if indentWidth <= 0 {
		indentWidth = 1
	}
	transformed := append([]int(nil), offsets...)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return text, transformed, false
	}
	startIdx, endIdx := vimLineIndexRange(text, startOffset, endOffset)
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	changed := false
	lineDeltas := make([]int, len(lines))
	for idx := startIdx; idx <= endIdx; idx++ {
		if right {
			lines[idx] = strings.Repeat(" ", indentWidth) + lines[idx]
			lineDeltas[idx] = indentWidth
			changed = true
			continue
		}
		remove := removableIndentWidth(lines[idx], indentWidth)
		if remove == 0 {
			continue
		}
		lines[idx] = lines[idx][remove:]
		lineDeltas[idx] = -remove
		changed = true
	}
	if !changed {
		return text, transformed, false
	}
	updated := strings.Join(lines, "\n")
	for idx, offset := range offsets {
		transformed[idx] = vimTransformOffsetAfterLineShifts(text, updated, offset, lineDeltas)
	}
	return updated, transformed, true
}

func vimTransformOffsetAfterLineShifts(original string, updated string, offset int, lineDeltas []int) int {
	offset = vimClampOffset(original, offset)
	lines := vimLineInfos(original)
	if len(lines) == 0 {
		return vimClampOffset(updated, offset)
	}
	lineIdx := vimLineIndexAtOffset(original, offset)
	prefixShift := 0
	for idx := 0; idx < lineIdx && idx < len(lineDeltas); idx++ {
		prefixShift += lineDeltas[idx]
	}
	line := lines[lineIdx]
	col := offset - line.start
	lineDelta := 0
	if lineIdx < len(lineDeltas) {
		lineDelta = lineDeltas[lineIdx]
	}
	switch {
	case lineDelta > 0:
		offset = line.start + prefixShift + col + lineDelta
	case lineDelta < 0:
		removed := -lineDelta
		if col <= removed {
			offset = line.start + prefixShift
		} else {
			offset = line.start + prefixShift + col - removed
		}
	default:
		offset += prefixShift
	}
	return vimClampOffset(updated, offset)
}

func removableIndentWidth(line string, maxWidth int) int {
	remove := 0
	for remove < len(line) && remove < maxWidth && line[remove] == ' ' {
		remove++
	}
	return remove
}

func vimLineColumns(text string, startOffset int, endOffset int) (int, int, int, int) {
	lines := vimLineInfos(text)
	if len(lines) == 0 {
		return 0, 0, 0, 0
	}
	startIdx := vimLineIndexAtOffset(text, startOffset)
	endIdx := vimLineIndexAtOffset(text, endOffset)
	startCol := startOffset - lines[startIdx].start
	endCol := endOffset - lines[endIdx].start
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
		startCol, endCol = endCol, startCol
	}
	if startCol > endCol {
		startCol, endCol = endCol, startCol
	}
	return startIdx, endIdx, startCol, endCol
}

func vimYankLine(text string, startOffset int, endOffset int) vimRegister {
	start, end := vimLineRange(text, startOffset, endOffset)
	yanked := string([]rune(text)[start:end])
	if yanked != "" && []rune(yanked)[len([]rune(yanked))-1] != '\n' {
		yanked += "\n"
	}
	return vimRegister{
		Kind: vimRegisterLine,
		Text: yanked,
	}
}

func vimYankChar(text string, startOffset int, endOffset int) vimRegister {
	start := startOffset
	end := endOffset
	if start > end {
		start, end = end, start
	}
	end++
	runes := []rune(text)
	if end > len(runes) {
		end = len(runes)
	}
	return vimRegister{
		Kind: vimRegisterChar,
		Text: string(runes[start:end]),
	}
}

func vimYankBlock(text string, startOffset int, endOffset int) vimRegister {
	lines := vimLineInfos(text)
	startIdx, endIdx, startCol, endCol := vimLineColumns(text, startOffset, endOffset)
	width := endCol - startCol + 1
	blockLines := make([]string, 0, endIdx-startIdx+1)
	runes := []rune(text)
	for idx := startIdx; idx <= endIdx; idx++ {
		line := lines[idx]
		lineRunes := runes[line.start:line.end]
		from := startCol
		if from > len(lineRunes) {
			from = len(lineRunes)
		}
		to := endCol + 1
		if to > len(lineRunes) {
			to = len(lineRunes)
		}
		if from > to {
			from = to
		}
		blockLines = append(blockLines, string(lineRunes[from:to]))
	}
	return vimRegister{
		Kind:  vimRegisterBlock,
		Lines: blockLines,
		Width: width,
	}
}

func vimDeleteBlockRegister(text string, startOffset int, endOffset int) (string, int, vimRegister) {
	reg := vimYankBlock(text, startOffset, endOffset)
	updated, cursor := vimDeleteBlock(text, startOffset, endOffset)
	return updated, cursor, reg
}

func vimPasteLine(text string, offset int, reg vimRegister) (string, int) {
	if reg.Kind != vimRegisterLine || reg.Text == "" {
		return text, offset
	}
	runes := []rune(text)
	insertAt := vimLineBoundaryOffset(text, offset, true)
	if insertAt < len(runes) {
		insertAt++
	}
	insert := []rune(reg.Text)
	updatedRunes := make([]rune, 0, len(runes)+len(insert))
	updatedRunes = append(updatedRunes, runes[:insertAt]...)
	updatedRunes = append(updatedRunes, insert...)
	updatedRunes = append(updatedRunes, runes[insertAt:]...)
	return string(updatedRunes), insertAt
}

func vimPasteBlock(text string, offset int, reg vimRegister) (string, int) {
	if reg.Kind != vimRegisterBlock || len(reg.Lines) == 0 {
		return text, offset
	}
	lines := vimLineInfos(text)
	runes := []rune(text)
	lineIdx := vimLineIndexAtOffset(text, offset)
	col := offset - lines[lineIdx].start
	if lineIdx+len(reg.Lines) > len(lines) {
		for len(lines) < lineIdx+len(reg.Lines) {
			runes = append(runes, '\n')
			lines = vimLineInfos(string(runes))
		}
	}
	for i, piece := range reg.Lines {
		lines = vimLineInfos(string(runes))
		idx := lineIdx + i
		line := lines[idx]
		lineRunes := runes[line.start:line.end]
		insertAt := col
		if insertAt > len(lineRunes) {
			padding := make([]rune, insertAt-len(lineRunes))
			for p := range padding {
				padding[p] = ' '
			}
			lineRunes = append(lineRunes, padding...)
		}
		newLine := append([]rune{}, lineRunes[:insertAt]...)
		newLine = append(newLine, []rune(piece)...)
		newLine = append(newLine, lineRunes[insertAt:]...)
		updated := make([]rune, 0, len(runes)-len(lineRunes)+len(newLine))
		updated = append(updated, runes[:line.start]...)
		updated = append(updated, newLine...)
		updated = append(updated, runes[line.end:]...)
		runes = updated
	}
	return string(runes), lines[lineIdx].start + col
}

func vimPasteChar(text string, offset int, reg vimRegister) (string, int) {
	if reg.Kind != vimRegisterChar || reg.Text == "" {
		return text, offset
	}
	runes := []rune(text)
	insert := []rune(reg.Text)
	offset = vimClampOffset(text, offset)
	updated := make([]rune, 0, len(runes)+len(insert))
	updated = append(updated, runes[:offset]...)
	updated = append(updated, insert...)
	updated = append(updated, runes[offset:]...)
	return string(updated), offset + len(insert)
}

func vimPasteCharAfter(text string, offset int, reg vimRegister) (string, int) {
	if reg.Kind != vimRegisterChar || reg.Text == "" {
		return text, offset
	}
	runes := []rune(text)
	offset = vimClampOffset(text, offset)
	insertAt := offset
	if insertAt < len(runes) && runes[insertAt] != '\n' {
		insertAt++
	}
	return vimPasteChar(text, insertAt, reg)
}

func vimDeleteRange(text string, startOffset int, endOffset int) (string, int) {
	start := startOffset
	end := endOffset
	if start > end {
		start, end = end, start
	}
	end++
	runes := []rune(text)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return text, start
	}
	return string(append(runes[:start], runes[end:]...)), start
}

func vimDeleteBlock(text string, startOffset int, endOffset int) (string, int) {
	lines := vimLineInfos(text)
	runes := []rune(text)
	startIdx, endIdx, startCol, endCol := vimLineColumns(text, startOffset, endOffset)
	width := endCol - startCol + 1
	for idx := endIdx; idx >= startIdx; idx-- {
		line := lines[idx]
		lineRunes := runes[line.start:line.end]
		from := startCol
		if from > len(lineRunes) {
			from = len(lineRunes)
		}
		to := from + width
		if to > len(lineRunes) {
			to = len(lineRunes)
		}
		if from >= to {
			continue
		}
		updatedLine := append([]rune{}, lineRunes[:from]...)
		updatedLine = append(updatedLine, lineRunes[to:]...)
		updated := make([]rune, 0, len(runes)-(to-from))
		updated = append(updated, runes[:line.start]...)
		updated = append(updated, updatedLine...)
		updated = append(updated, runes[line.end:]...)
		runes = updated
		lines = vimLineInfos(string(runes))
	}
	return string(runes), startOffset
}

func vimIsWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func vimWordForwardOffset(text string, offset int) int {
	runes := []rune(text)
	offset = vimClampOffset(text, offset)
	if offset >= len(runes) {
		return len(runes)
	}
	if vimIsWordRune(runes[offset]) {
		for offset < len(runes) && vimIsWordRune(runes[offset]) {
			offset++
		}
		for offset < len(runes) && unicode.IsSpace(runes[offset]) {
			offset++
		}
		return offset
	}
	for offset < len(runes) && !vimIsWordRune(runes[offset]) {
		offset++
	}
	return offset
}

func vimWordRange(text string, offset int) (int, int) {
	start := vimClampOffset(text, offset)
	end := vimWordForwardOffset(text, start)
	return start, end
}

func vimStrictWordRange(text string, offset int) (int, int) {
	runes := []rune(text)
	start := vimClampOffset(text, offset)
	for start < len(runes) && !vimIsWordRune(runes[start]) {
		start++
	}
	end := start
	for end < len(runes) && vimIsWordRune(runes[end]) {
		end++
	}
	return start, end
}

func vimYankWord(text string, offset int) vimRegister {
	start, end := vimStrictWordRange(text, offset)
	if start == end {
		return vimRegister{}
	}
	runes := []rune(text)
	return vimRegister{
		Kind: vimRegisterChar,
		Text: string(runes[start:end]),
	}
}

func vimDeleteWord(text string, offset int) (string, int) {
	start, end := vimWordRange(text, offset)
	if start == end {
		return text, start
	}
	runes := []rune(text)
	return string(append(runes[:start], runes[end:]...)), start
}
