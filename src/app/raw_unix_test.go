//go:build darwin || linux

package app

import (
	"bytes"
	"testing"

	"github.com/kloneets/tools/src/notes"
)

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

func TestParseCSIDecodesTmuxCtrlGSequences(t *testing.T) {
	for _, seq := range []string{"103;5u", "7;5u", "27;5;103~"} {
		got := parseCSI([]byte(seq))
		want := notes.Key{Name: "g", Rune: 'g', Ctrl: true}
		if got != want {
			t.Fatalf("parseCSI(%q) = %#v, want %#v", seq, got, want)
		}
	}
}

func TestDecodeCtrlByte(t *testing.T) {
	cases := []struct {
		in   byte
		want notes.Key
		ok   bool
	}{
		{7, notes.Key{Name: "g", Rune: 'g', Ctrl: true}, true},
		{28, notes.Key{Name: "4", Ctrl: true}, true},
		{29, notes.Key{Name: "5", Ctrl: true}, true},
		{30, notes.Key{Name: "6", Ctrl: true}, true},
		{9, notes.Key{}, false},
		{13, notes.Key{}, false},
	}
	for _, tc := range cases {
		got, ok := decodeCtrlByte(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("decodeCtrlByte(%d) = %#v, %t want %#v, %t", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReadKeysDecodesCtrlShortcuts(t *testing.T) {
	out := make(chan notes.Key, 4)
	readKeys(bytes.NewReader([]byte{7, 28}), out)
	got := make([]notes.Key, 0, 2)
	for key := range out {
		got = append(got, key)
	}
	want := []notes.Key{
		{Name: "g", Rune: 'g', Ctrl: true},
		{Name: "4", Ctrl: true},
	}
	if len(got) != len(want) {
		t.Fatalf("readKeys count = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("readKeys[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
