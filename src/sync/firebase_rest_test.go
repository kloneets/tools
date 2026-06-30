package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kloneets/tools/src/todo"
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

func TestLoginWithGoogleIDTokenUsesSignInWithIdp(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts:signInWithIdp" {
			t.Fatalf("path = %s, want /accounts:signInWithIdp", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "api-key" {
			t.Fatalf("api key = %q, want api-key", got)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		requestBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"localId":"uid-google",
			"email":"user@example.com",
			"idToken":"firebase-id",
			"refreshToken":"firebase-refresh"
		}`))
	}))
	defer server.Close()

	provider := NewFirebaseRESTProvider("api-key", "https://db.example")
	provider.IdentityToolkitURL = server.URL
	session, err := provider.LoginWithGoogleIDToken(context.Background(), "google-id")
	if err != nil {
		t.Fatalf("LoginWithGoogleIDToken() error = %v", err)
	}

	if session.UID != "uid-google" || session.Email != "user@example.com" || session.IDToken != "firebase-id" || session.RefreshToken != "firebase-refresh" {
		t.Fatalf("session = %+v, want Firebase auth response fields", session)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(requestBody), &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	postBody, _ := url.ParseQuery(body["postBody"].(string))
	if got := postBody.Get("id_token"); got != "google-id" {
		t.Fatalf("id_token = %q, want google-id", got)
	}
	if got := postBody.Get("providerId"); got != "google.com" {
		t.Fatalf("providerId = %q, want google.com", got)
	}
	if body["requestUri"] != "http://localhost" {
		t.Fatalf("requestUri = %v, want http://localhost", body["requestUri"])
	}
	if body["returnSecureToken"] != true {
		t.Fatalf("returnSecureToken = %v, want true", body["returnSecureToken"])
	}
}

func TestLoginWithGoogleIDTokenRequiresToken(t *testing.T) {
	provider := NewFirebaseRESTProvider("api-key", "https://db.example")

	_, err := provider.LoginWithGoogleIDToken(context.Background(), " ")

	if err == nil || !strings.Contains(err.Error(), "google id token is required") {
		t.Fatalf("error = %v, want missing token failure", err)
	}
}

func TestFirebaseRESTProviderPullsWorkspaceChildrenIndividually(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/workspaces/ws/todos.json":
			_, _ = w.Write([]byte(`{"todo-1":{"item":{"id":"todo-1","text":"one","status":"todo","created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z"},"rev":1}}`))
		case "/workspaces/ws/todo_archives/2026-05.json":
			_, _ = w.Write([]byte(`{"todo-2":{"item":{"id":"todo-2","text":"old","status":"archived","created_at":"2026-05-01T00:00:00Z","updated_at":"2026-05-01T00:00:00Z","archived_at":"2026-05-02T00:00:00Z"},"rev":1}}`))
		case "/workspaces/ws/todo_archive_months.json":
			_, _ = w.Write([]byte(`["2026-05"]`))
		case "/workspaces/ws/notes.json":
			_, _ = w.Write([]byte(`{"note-1":{"id":"note-1","path":"a.md","text":"note","rev":1}}`))
		case "/workspaces/ws/settings.json":
			_, _ = w.Write([]byte(`{"shared":{"values":{"pages_app":{"first_book":1}},"rev":1}}`))
		case "/workspaces/ws/assets.json":
			_, _ = w.Write([]byte(`{"asset-1":{"id":"asset-1","path":"a.assets/image.png","rev":1,"deleted":true}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFirebaseRESTProvider("api-key", server.URL)
	if _, err := provider.PullTodos(context.Background(), "ws"); err != nil {
		t.Fatalf("PullTodos() error = %v", err)
	}
	if _, err := provider.PullTodoArchiveMonth(context.Background(), "ws", "2026-05"); err != nil {
		t.Fatalf("PullTodoArchiveMonth() error = %v", err)
	}
	if _, err := provider.PullTodoArchiveMonths(context.Background(), "ws"); err != nil {
		t.Fatalf("PullTodoArchiveMonths() error = %v", err)
	}
	if _, err := provider.PullNotes(context.Background(), "ws"); err != nil {
		t.Fatalf("PullNotes() error = %v", err)
	}
	if _, err := provider.PullSettings(context.Background(), "ws"); err != nil {
		t.Fatalf("PullSettings() error = %v", err)
	}
	if _, err := provider.PullAssets(context.Background(), "ws"); err != nil {
		t.Fatalf("PullAssets() error = %v", err)
	}

	want := []string{
		"/workspaces/ws/todos.json",
		"/workspaces/ws/todo_archives/2026-05.json",
		"/workspaces/ws/todo_archive_months.json",
		"/workspaces/ws/notes.json",
		"/workspaces/ws/settings.json",
		"/workspaces/ws/assets.json",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestPushMutationDoesNotWriteUnusedEventRecord(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, "/events/") {
			t.Fatalf("unexpected event write to %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`null`))
	}))
	defer server.Close()

	provider := NewFirebaseRESTProvider("api-key", server.URL)
	err := provider.PushMutation(context.Background(), "ws", Mutation{
		EventID: "todo-1",
		Todo: &TodoRecord{
			Item: todoItemForTest("todo-1"),
			Rev:  1,
		},
	})
	if err != nil {
		t.Fatalf("PushMutation() error = %v", err)
	}

	want := []string{"/workspaces/ws/todos/todo-1.json"}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func todoItemForTest(id string) todo.Item {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	return todo.Item{
		ID:        id,
		Text:      "one",
		Status:    todo.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
