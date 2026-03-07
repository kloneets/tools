package password

import (
	"math/rand"
	"strings"
	"testing"
)

func TestBuildCharPoolIncludesSelectedGroups(t *testing.T) {
	pool := buildCharPool(true, true, true)

	for _, want := range []string{"a", "z", "A", "Z", "0", "9", "!", "~"} {
		if !strings.Contains(pool, want) {
			t.Fatalf("expected pool to contain %q", want)
		}
	}
}

func TestBuildCharPoolEmptyWhenNothingSelected(t *testing.T) {
	if got := buildCharPool(false, false, false); got != "" {
		t.Fatalf("buildCharPool() = %q, want empty string", got)
	}
}

func TestGeneratePasswordUsesPoolAndLength(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	got := generatePassword("ab", 8, random)

	if len(got) != 8 {
		t.Fatalf("generatePassword() length = %d, want 8", len(got))
	}
	for _, ch := range got {
		if ch != 'a' && ch != 'b' {
			t.Fatalf("generatePassword() produced rune %q outside pool", ch)
		}
	}
}
