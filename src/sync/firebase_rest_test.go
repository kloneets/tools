package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersonalWorkspaceIDUsesFirebaseUID(t *testing.T) {
	got := PersonalWorkspaceID(" uid-123 ")

	if got != "user_uid-123" {
		t.Fatalf("PersonalWorkspaceID() = %q, want user_uid-123", got)
	}
}

func TestMigrateWorkspaceToPersonalRequiresSourceOwner(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "/members/uid-1.json"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`null`))
		case strings.Contains(r.URL.Path, "/meta.json"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`null`))
		case strings.Contains(r.URL.Path, "/workspaces/source.json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Snapshot{
				Members: map[string]Member{
					"uid-1": {UID: "uid-1", Email: "user@example.com", Role: RoleViewer},
				},
			})
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFirebaseRESTProvider("api", server.URL)
	_, err := provider.MigrateWorkspaceToPersonal(context.Background(), "source", Session{
		UID:     "uid-1",
		Email:   "user@example.com",
		IDToken: "token",
	})

	if err == nil || !strings.Contains(err.Error(), "requires owner role") {
		t.Fatalf("error = %v, want owner role failure", err)
	}
	for _, path := range requests {
		if strings.Contains(path, "/notes/") || strings.Contains(path, "/todos/") {
			t.Fatalf("migration wrote data path %s before owner check", path)
		}
	}
}
