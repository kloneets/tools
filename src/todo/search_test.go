package todo

import (
	"reflect"
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	for _, test := range []struct {
		text, query string
		positions   []int
		match       bool
	}{
		{"groceries", "grc", []int{0, 1, 3}, true},
		{"Groceries", "GRC", []int{0, 1, 3}, true},
		{"banana", "aaa", []int{1, 3, 5}, true},
		{"banana", "aaaa", nil, false},
		{"hello", "", []int{}, true},
		{"", "x", nil, false},
		{"a.b[c]", ".[", []int{1, 3}, true},
		{"Āβ😀Z", "ā😀z", []int{0, 2, 3}, true},
		{"ΣςıK", "σσιk", nil, false},
		{"ΣςıK", "σσik", []int{0, 1, 2, 3}, true},
		{"ba", "ab", nil, false},
	} {
		t.Run(test.text+"/"+test.query, func(t *testing.T) {
			positions, match := FuzzyMatch(test.text, test.query)
			if match != test.match || !reflect.DeepEqual(positions, test.positions) {
				t.Fatalf("got %v %v, want %v %v", positions, match, test.positions, test.match)
			}
		})
	}
}
