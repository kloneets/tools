package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type FirebaseRESTProvider struct {
	APIKey             string
	DatabaseURL        string
	IdentityToolkitURL string
	SecureTokenURL     string
	Client             *http.Client
	session            Session
}

func NewFirebaseRESTProvider(apiKey string, databaseURL string) *FirebaseRESTProvider {
	return &FirebaseRESTProvider{
		APIKey:             strings.TrimSpace(apiKey),
		DatabaseURL:        strings.TrimRight(strings.TrimSpace(databaseURL), "/"),
		IdentityToolkitURL: "https://identitytoolkit.googleapis.com/v1",
		SecureTokenURL:     "https://securetoken.googleapis.com/v1",
		Client:             &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *FirebaseRESTProvider) Login(ctx context.Context, email string, password string) (Session, error) {
	if p.APIKey == "" {
		return Session{}, errors.New("firebase api key is required")
	}
	body := map[string]any{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	}
	var response struct {
		LocalID      string `json:"localId"`
		Email        string `json:"email"`
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    string `json:"expiresIn"`
	}
	if err := p.postJSON(ctx, p.identityToolkitEndpoint("accounts:signInWithPassword"), body, &response); err != nil {
		return Session{}, err
	}
	return p.sessionFromAuthResponse(response.LocalID, response.Email, response.IDToken, response.RefreshToken), nil
}

func (p *FirebaseRESTProvider) LoginWithGoogleIDToken(ctx context.Context, googleIDToken string) (Session, error) {
	if p.APIKey == "" {
		return Session{}, errors.New("firebase api key is required")
	}
	googleIDToken = strings.TrimSpace(googleIDToken)
	if googleIDToken == "" {
		return Session{}, errors.New("google id token is required")
	}
	postBody := url.Values{}
	postBody.Set("id_token", googleIDToken)
	postBody.Set("providerId", "google.com")
	body := map[string]any{
		"postBody":          postBody.Encode(),
		"requestUri":        "http://localhost",
		"returnSecureToken": true,
	}
	var response struct {
		LocalID      string `json:"localId"`
		Email        string `json:"email"`
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    string `json:"expiresIn"`
	}
	if err := p.postJSON(ctx, p.identityToolkitEndpoint("accounts:signInWithIdp"), body, &response); err != nil {
		return Session{}, err
	}
	return p.sessionFromAuthResponse(response.LocalID, response.Email, response.IDToken, response.RefreshToken), nil
}

func (p *FirebaseRESTProvider) Refresh(ctx context.Context, refreshToken string) (Session, error) {
	if p.APIKey == "" {
		return Session{}, errors.New("firebase api key is required")
	}
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}
	var response struct {
		UserID       string `json:"user_id"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
	}
	if err := p.postForm(ctx, p.secureTokenEndpoint("token"), body, &response); err != nil {
		return Session{}, err
	}
	session := Session{
		UID:          response.UserID,
		IDToken:      response.IDToken,
		RefreshToken: response.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	p.session = session
	return session, nil
}

func (p *FirebaseRESTProvider) sessionFromAuthResponse(uid string, email string, idToken string, refreshToken string) Session {
	session := Session{
		UID:          uid,
		Email:        email,
		IDToken:      idToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	p.session = session
	return session
}

func (p *FirebaseRESTProvider) identityToolkitEndpoint(method string) string {
	return strings.TrimRight(p.IdentityToolkitURL, "/") + "/" + method + "?key=" + url.QueryEscape(p.APIKey)
}

func (p *FirebaseRESTProvider) secureTokenEndpoint(method string) string {
	return strings.TrimRight(p.SecureTokenURL, "/") + "/" + method + "?key=" + url.QueryEscape(p.APIKey)
}

func PersonalWorkspaceID(uid string) string {
	return "user_" + strings.TrimSpace(uid)
}

func (p *FirebaseRESTProvider) EnsurePersonalWorkspace(ctx context.Context, session Session, name string) (WorkspaceMeta, error) {
	if session.UID == "" {
		return WorkspaceMeta{}, errors.New("firebase uid is required")
	}
	if session.IDToken != "" {
		p.session = session
	}
	workspaceID := PersonalWorkspaceID(session.UID)
	workspaceName := strings.TrimSpace(name)
	if workspaceName == "" {
		workspaceName = "Personal workspace"
	}
	now := time.Now().UTC()
	member := map[string]any{
		"email":     session.Email,
		"role":      RoleOwner,
		"joined_at": now.Format(time.RFC3339),
	}
	if err := p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/members/%s", url.PathEscape(workspaceID), url.PathEscape(session.UID)), member, nil); err != nil {
		return WorkspaceMeta{}, err
	}
	meta := WorkspaceMeta{
		ID:        workspaceID,
		Name:      workspaceName,
		CreatedAt: now,
		OwnerUID:  session.UID,
	}
	if err := p.putRTDB(ctx, "workspaces/"+url.PathEscape(workspaceID)+"/meta", map[string]any{
		"name":       meta.Name,
		"owner":      meta.OwnerUID,
		"created_at": meta.CreatedAt.Format(time.RFC3339),
	}, nil); err != nil {
		return WorkspaceMeta{}, err
	}
	return meta, nil
}

type MigrationResult struct {
	SourceWorkspaceID string
	TargetWorkspaceID string
	Notes             int
	Todos             int
	Settings          int
}

func (p *FirebaseRESTProvider) MigrateWorkspaceToPersonal(ctx context.Context, sourceWorkspaceID string, session Session) (MigrationResult, error) {
	sourceWorkspaceID = strings.TrimSpace(sourceWorkspaceID)
	if sourceWorkspaceID == "" {
		return MigrationResult{}, errors.New("source workspace id is required")
	}
	targetWorkspaceID := PersonalWorkspaceID(session.UID)
	if sourceWorkspaceID == targetWorkspaceID {
		return MigrationResult{}, errors.New("source workspace is already the personal workspace")
	}
	if _, err := p.EnsurePersonalWorkspace(ctx, session, "Personal workspace"); err != nil {
		return MigrationResult{}, err
	}
	snapshot, err := p.PullSnapshot(ctx, sourceWorkspaceID)
	if err != nil {
		return MigrationResult{}, err
	}
	member, ok := snapshot.Members[session.UID]
	if !ok || member.Role != RoleOwner {
		return MigrationResult{}, fmt.Errorf("migration requires owner role in source workspace %q", sourceWorkspaceID)
	}
	result := MigrationResult{SourceWorkspaceID: sourceWorkspaceID, TargetWorkspaceID: targetWorkspaceID}
	for id, note := range snapshot.Notes {
		if note.ID == "" {
			note.ID = id
		}
		if note.UpdatedAt.IsZero() {
			note.UpdatedAt = time.Now().UTC()
		}
		note.UpdatedBy = session.UID
		if err := p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/notes/%s", url.PathEscape(targetWorkspaceID), url.PathEscape(note.ID)), note, nil); err != nil {
			return result, err
		}
		result.Notes++
	}
	for id, todo := range snapshot.Todos {
		if todo.Item.ID == "" {
			todo.Item.ID = id
		}
		if todo.Item.Status == "archived" {
			continue
		}
		todo.UpdatedBy = session.UID
		if err := p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/todos/%s", url.PathEscape(targetWorkspaceID), url.PathEscape(todo.Item.ID)), todo, nil); err != nil {
			return result, err
		}
		result.Todos++
	}
	if record, ok := sharedSettingsFromSnapshot(snapshot.Settings); ok {
		record.UpdatedBy = session.UID
		if err := p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/settings/shared", url.PathEscape(targetWorkspaceID)), record, nil); err != nil {
			return result, err
		}
		result.Settings = 1
	}
	event := map[string]any{
		"device_id":  "desktop",
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"kind":       "workspace_migration",
		"source":     sourceWorkspaceID,
	}
	_ = p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/events/migration-%d", url.PathEscape(targetWorkspaceID), time.Now().UnixNano()), event, nil)
	return result, nil
}

func (p *FirebaseRESTProvider) WatchWorkspace(ctx context.Context, workspaceID string, sinceToken string, onChange func(Change) error) error {
	if onChange == nil {
		return errors.New("change callback is required")
	}
	snapshot, err := p.PullSnapshot(ctx, workspaceID)
	if err != nil {
		return err
	}
	return onChange(Change{EventID: sinceToken, Path: "workspaces/" + workspaceID, Value: snapshot})
}

func (p *FirebaseRESTProvider) PushMutation(ctx context.Context, workspaceID string, mutation Mutation) error {
	if mutation.EventID == "" {
		mutation.EventID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if mutation.CreatedAt.IsZero() {
		mutation.CreatedAt = time.Now().UTC()
	}
	if mutation.Kind == "" {
		switch {
		case mutation.Note != nil:
			mutation.Kind = "note_push"
		case mutation.Todo != nil:
			mutation.Kind = "todo_push"
		case mutation.Settings != nil:
			mutation.Kind = "settings_push"
		default:
			mutation.Kind = "sync"
		}
	}
	if mutation.Note != nil {
		path := fmt.Sprintf("workspaces/%s/notes/%s", url.PathEscape(workspaceID), url.PathEscape(mutation.Note.ID))
		if err := p.putRTDB(ctx, path, mutation.Note, nil); err != nil {
			return err
		}
	}
	if mutation.Todo != nil {
		if mutation.Todo.Item.Status == "archived" && !mutation.Todo.Deleted {
			return nil
		}
		path := fmt.Sprintf("workspaces/%s/todos/%s", url.PathEscape(workspaceID), url.PathEscape(mutation.Todo.Item.ID))
		if err := p.putRTDB(ctx, path, mutation.Todo, nil); err != nil {
			return err
		}
	}
	if mutation.Settings != nil {
		path := fmt.Sprintf("workspaces/%s/settings/shared", url.PathEscape(workspaceID))
		if err := p.putRTDB(ctx, path, mutation.Settings, nil); err != nil {
			return err
		}
	}
	return nil
}

func (p *FirebaseRESTProvider) PullSnapshot(ctx context.Context, workspaceID string) (Snapshot, error) {
	var snapshot Snapshot
	err := p.getRTDB(ctx, "workspaces/"+url.PathEscape(workspaceID), &snapshot)
	return snapshot, err
}

func (p *FirebaseRESTProvider) PullTodos(ctx context.Context, workspaceID string) (map[string]TodoRecord, error) {
	var records map[string]TodoRecord
	err := p.getRTDB(ctx, fmt.Sprintf("workspaces/%s/todos", url.PathEscape(workspaceID)), &records)
	if records == nil {
		records = map[string]TodoRecord{}
	}
	return records, err
}

func (p *FirebaseRESTProvider) PullTodoArchiveMonth(ctx context.Context, workspaceID string, month string) (map[string]TodoRecord, error) {
	var records map[string]TodoRecord
	err := p.getRTDB(ctx, fmt.Sprintf("workspaces/%s/todo_archives/%s", url.PathEscape(workspaceID), url.PathEscape(month)), &records)
	if records == nil {
		records = map[string]TodoRecord{}
	}
	return records, err
}

func (p *FirebaseRESTProvider) PullTodoArchiveMonths(ctx context.Context, workspaceID string) ([]string, error) {
	var months []string
	err := p.getRTDB(ctx, fmt.Sprintf("workspaces/%s/todo_archive_months", url.PathEscape(workspaceID)), &months)
	if months == nil {
		months = []string{}
	}
	return months, err
}

func (p *FirebaseRESTProvider) PushTodoArchiveMonths(ctx context.Context, workspaceID string, months []string) error {
	if err := p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/todo_archive_months", url.PathEscape(workspaceID)), months, nil); err != nil {
		return err
	}
	pushSyncHashBestEffort(ctx, p, workspaceID, SyncFeatureTodoArchiveMonths, TodoArchiveMonthsHash(months), time.Now().UTC(), p.session.UID)
	return nil
}

func (p *FirebaseRESTProvider) PullNotes(ctx context.Context, workspaceID string) (map[string]NoteRecord, error) {
	var records map[string]NoteRecord
	err := p.getRTDB(ctx, fmt.Sprintf("workspaces/%s/notes", url.PathEscape(workspaceID)), &records)
	if records == nil {
		records = map[string]NoteRecord{}
	}
	return records, err
}

func (p *FirebaseRESTProvider) PullSettings(ctx context.Context, workspaceID string) (map[string]any, error) {
	var records map[string]any
	err := p.getRTDB(ctx, fmt.Sprintf("workspaces/%s/settings", url.PathEscape(workspaceID)), &records)
	if records == nil {
		records = map[string]any{}
	}
	return records, err
}

func (p *FirebaseRESTProvider) PullSyncHashes(ctx context.Context, workspaceID string) (map[string]SyncHashRecord, error) {
	var records map[string]SyncHashRecord
	err := p.getRTDB(ctx, fmt.Sprintf("workspaces/%s/sync_hashes", url.PathEscape(workspaceID)), &records)
	if records == nil {
		records = map[string]SyncHashRecord{}
	}
	return records, err
}

func (p *FirebaseRESTProvider) PushSyncHash(ctx context.Context, workspaceID string, feature string, record SyncHashRecord) error {
	if strings.TrimSpace(feature) == "" {
		return nil
	}
	return p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/sync_hashes/%s", url.PathEscape(workspaceID), url.PathEscape(feature)), record, nil)
}

func (p *FirebaseRESTProvider) DeleteLegacyAssetsBestEffort(ctx context.Context, workspaceID string) {
	if strings.TrimSpace(workspaceID) == "" {
		return
	}
	escapedWorkspace := url.PathEscape(workspaceID)
	_ = p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/assets", escapedWorkspace), nil, nil)
	_ = p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/sync_hashes/assets", escapedWorkspace), nil, nil)
}

func sharedSettingsFromSnapshot(settings map[string]any) (SharedSettingsRecord, bool) {
	raw, ok := settings["shared"]
	if !ok {
		return SharedSettingsRecord{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return SharedSettingsRecord{}, false
	}
	var record SharedSettingsRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return SharedSettingsRecord{}, false
	}
	return record, record.Values != nil
}

func (p *FirebaseRESTProvider) CreateWorkspace(ctx context.Context, name string) (WorkspaceMeta, error) {
	meta := WorkspaceMeta{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:      strings.TrimSpace(name),
		CreatedAt: time.Now().UTC(),
		OwnerUID:  p.session.UID,
	}
	if meta.Name == "" {
		meta.Name = "Koko Workspace"
	}
	if err := p.putRTDB(ctx, "workspaces/"+url.PathEscape(meta.ID)+"/meta", meta, nil); err != nil {
		return WorkspaceMeta{}, err
	}
	return meta, nil
}

func (p *FirebaseRESTProvider) GrantMember(ctx context.Context, workspaceID string, email string, role string) error {
	if role != RoleOwner && role != RoleEditor && role != RoleViewer {
		return fmt.Errorf("unsupported role %q", role)
	}
	member := Member{Email: strings.TrimSpace(email), Role: role}
	if member.Email == "" {
		return errors.New("member email is required")
	}
	key := strings.NewReplacer(".", ",", "#", ",", "$", ",", "[", ",", "]", ",").Replace(strings.ToLower(member.Email))
	return p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/member_invites/%s", url.PathEscape(workspaceID), url.PathEscape(key)), member, nil)
}

func (p *FirebaseRESTProvider) RevokeMember(ctx context.Context, workspaceID string, email string) error {
	key := strings.NewReplacer(".", ",", "#", ",", "$", ",", "[", ",", "]", ",").Replace(strings.ToLower(strings.TrimSpace(email)))
	return p.putRTDB(ctx, fmt.Sprintf("workspaces/%s/member_invites/%s", url.PathEscape(workspaceID), url.PathEscape(key)), nil, nil)
}

func (p *FirebaseRESTProvider) getRTDB(ctx context.Context, path string, out any) error {
	if err := p.doJSON(ctx, http.MethodGet, p.rtdbURL(path), nil, out); err != nil {
		return fmt.Errorf("firebase GET %s failed: %w", path, err)
	}
	return nil
}

func (p *FirebaseRESTProvider) putRTDB(ctx context.Context, path string, in any, out any) error {
	if err := p.doJSON(ctx, http.MethodPut, p.rtdbURL(path), in, out); err != nil {
		return fmt.Errorf("firebase PUT %s failed: %w", path, err)
	}
	return nil
}

func (p *FirebaseRESTProvider) postJSON(ctx context.Context, endpoint string, in any, out any) error {
	return p.doJSON(ctx, http.MethodPost, endpoint, in, out)
}

func (p *FirebaseRESTProvider) postForm(ctx context.Context, endpoint string, in map[string]string, out any) error {
	values := url.Values{}
	for key, value := range in {
		values.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("firebase request failed: %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}

func (p *FirebaseRESTProvider) doJSON(ctx context.Context, method string, endpoint string, in any, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("firebase request failed: %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	if out == nil || len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (p *FirebaseRESTProvider) rtdbURL(path string) string {
	base := strings.TrimRight(p.DatabaseURL, "/")
	token := p.session.IDToken
	if token == "" {
		return base + "/" + strings.TrimLeft(path, "/") + ".json"
	}
	return base + "/" + strings.TrimLeft(path, "/") + ".json?auth=" + url.QueryEscape(token)
}
