package sync

import (
	"context"
	"time"

	"github.com/kloneets/tools/src/todo"
)

const (
	ProviderFirebase = "firebase"

	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

type Provider interface {
	Login(ctx context.Context, email string, password string) (Session, error)
	WatchWorkspace(ctx context.Context, workspaceID string, sinceToken string, onChange func(Change) error) error
	PushMutation(ctx context.Context, workspaceID string, mutation Mutation) error
	PullSnapshot(ctx context.Context, workspaceID string) (Snapshot, error)
	CreateWorkspace(ctx context.Context, name string) (WorkspaceMeta, error)
	GrantMember(ctx context.Context, workspaceID string, email string, role string) error
	RevokeMember(ctx context.Context, workspaceID string, email string) error
}

type Session struct {
	UID          string    `json:"uid"`
	Email        string    `json:"email"`
	IDToken      string    `json:"id_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

type WorkspaceMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	OwnerUID  string    `json:"owner_uid"`
}

type Member struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type NoteRecord struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Text      string    `json:"text"`
	Rev       int64     `json:"rev"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
	Deleted   bool      `json:"deleted"`
}

type TodoRecord struct {
	Item      todo.Item `json:"item"`
	Rev       int64     `json:"rev"`
	UpdatedBy string    `json:"updated_by"`
	Deleted   bool      `json:"deleted"`
}

type SharedSettingsRecord struct {
	Values    map[string]any `json:"values"`
	Rev       int64          `json:"rev"`
	UpdatedAt time.Time      `json:"updated_at"`
	UpdatedBy string         `json:"updated_by"`
}

type AssetRecord struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	BytesBase64 string    `json:"bytes_base64,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	MIME        string    `json:"mime,omitempty"`
	Rev         int64     `json:"rev"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   string    `json:"updated_by"`
	Deleted     bool      `json:"deleted"`
}

type Snapshot struct {
	Meta     WorkspaceMeta          `json:"meta"`
	Members  map[string]Member      `json:"members"`
	Notes    map[string]NoteRecord  `json:"notes"`
	Todos    map[string]TodoRecord  `json:"todos"`
	Settings map[string]any         `json:"settings"`
	Assets   map[string]AssetRecord `json:"assets"`
}

type Mutation struct {
	EventID   string                `json:"event_id"`
	DeviceID  string                `json:"device_id"`
	Kind      string                `json:"kind"`
	Note      *NoteRecord           `json:"note,omitempty"`
	Todo      *TodoRecord           `json:"todo,omitempty"`
	Settings  *SharedSettingsRecord `json:"settings,omitempty"`
	Asset     *AssetRecord          `json:"asset,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
}

type Change struct {
	EventID string      `json:"event_id"`
	Path    string      `json:"path"`
	Value   interface{} `json:"value"`
}
