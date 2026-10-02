package todo

import "unicode"

// FuzzyMatch returns earliest rune positions for a case-insensitive subsequence.
// An empty query matches without highlighting any characters.
func FuzzyMatch(text, query string) ([]int, bool) {
	wanted := []rune(query)
	positions := make([]int, 0, len(wanted))
	if len(wanted) == 0 {
		return positions, true
	}
	fold := func(r rune) rune { return unicode.ToLower(unicode.ToUpper(r)) }
	for i, r := range []rune(text) {
		if fold(r) == fold(wanted[len(positions)]) {
			positions = append(positions, i)
			if len(positions) == len(wanted) {
				return positions, true
			}
		}
	}
	return nil, false
}
