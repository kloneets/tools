package sync

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSecurityRulesProtectTodoListMetadataRevisions(t *testing.T) {
	data, err := os.ReadFile("security_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var rules map[string]any
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatalf("security_rules.json is not valid JSON: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"newData.child('rev').val() > data.child('rev').val()",
		"newData.child('rev').val() == data.child('rev').val()",
		"newData.child('deleted').val() == true && data.child('deleted').val() != true",
		"newData.child('deleted').val() != true && data.child('deleted').val() != true",
		"newData.child('name').val() == data.child('name').val()",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("security rules missing todo list metadata guard %q", want)
		}
	}
}
