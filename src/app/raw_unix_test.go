//go:build darwin || linux

package app

import "testing"

func TestParseCSI(t *testing.T) {
	cases := []struct {
		seq  string
		name string
		ctrl bool
	}{
		{"A", "up", false},
		{"B", "down", false},
		{"C", "right", false},
		{"D", "left", false},
		{"H", "home", false},
		{"F", "end", false},
		{"5~", "pageup", false},
		{"6~", "pagedown", false},
		{"3~", "delete", false},
	}
	for _, tc := range cases {
		got := parseCSI([]byte(tc.seq))
		if got.Name != tc.name || got.Ctrl != tc.ctrl {
			t.Fatalf("parseCSI(%q) = %#v, want name=%q ctrl=%t", tc.seq, got, tc.name, tc.ctrl)
		}
	}
}
