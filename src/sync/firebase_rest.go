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
	APIKey      string
	DatabaseURL string
	Client      *http.Client
	session     Session
}

func NewFirebaseRESTProvider(apiKey string, databaseURL string) *FirebaseRESTProvider {
	return &FirebaseRESTProvider{
		APIKey:      strings.TrimSpace(apiKey),
		DatabaseURL: strings.TrimRight(strings.TrimSpace(databaseURL), "/"),
		Client:      &http.Client{Timeout: 30 * time.Second},
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
	if err := p.postJSON(ctx, "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key="+url.QueryEscape(p.APIKey), body, &response); err != nil {
		return Session{}, err
	}
	session := Session{
		UID:          response.LocalID,
		Email:        response.Email,
		IDToken:      response.IDToken,
		RefreshToken: response.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	p.session = session
	return session, nil
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
	if err := p.postForm(ctx, "https://securetoken.googleapis.com/v1/token?key="+url.QueryEscape(p.APIKey), body, &response); err != nil {
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
	if mutation.Note != nil {
		path := fmt.Sprintf("workspaces/%s/notes/%s", url.PathEscape(workspaceID), url.PathEscape(mutation.Note.ID))
		if err := p.putRTDB(ctx, path, mutation.Note, nil); err != nil {
			return err
		}
	}
	if mutation.Todo != nil {
		path := fmt.Sprintf("workspaces/%s/todos/%s", url.PathEscape(workspaceID), url.PathEscape(mutation.Todo.Item.ID))
		if err := p.putRTDB(ctx, path, mutation.Todo, nil); err != nil {
			return err
		}
	}
	if mutation.Settings != nil {
		path := fmt.Sprintf("workspaces/%s/settings/%s", url.PathEscape(workspaceID), url.PathEscape(mutation.DeviceID))
		if err := p.putRTDB(ctx, path, mutation.Settings, nil); err != nil {
			return err
		}
	}
	path := fmt.Sprintf("workspaces/%s/events/%s", url.PathEscape(workspaceID), url.PathEscape(mutation.EventID))
	return p.putRTDB(ctx, path, mutation, nil)
}

func (p *FirebaseRESTProvider) PullSnapshot(ctx context.Context, workspaceID string) (Snapshot, error) {
	var snapshot Snapshot
	err := p.getRTDB(ctx, "workspaces/"+url.PathEscape(workspaceID), &snapshot)
	return snapshot, err
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
	return p.doJSON(ctx, http.MethodGet, p.rtdbURL(path), nil, out)
}

func (p *FirebaseRESTProvider) putRTDB(ctx context.Context, path string, in any, out any) error {
	return p.doJSON(ctx, http.MethodPut, p.rtdbURL(path), in, out)
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
