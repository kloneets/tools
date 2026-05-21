package sync

import (
	"os"
	"strings"
	"testing"
)

func TestSecurityRulesRestrictWorkspaceToMembersAndOwnersManageMembers(t *testing.T) {
	data, err := os.ReadFile("security_rules.json")
	if err != nil {
		t.Fatalf("ReadFile(security_rules.json) error = %v", err)
	}
	rules := string(data)
	for _, want := range []string{
		"auth != null",
		"members",
		"\".write\": false",
		"== 'editor'",
		"child('role').val() == 'owner'",
	} {
		if !strings.Contains(rules, want) {
			t.Fatalf("security rules missing %q: %s", want, rules)
		}
	}
}
