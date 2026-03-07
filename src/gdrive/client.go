package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/kloneets/tools/src/helpers"
)

type Folder struct {
	ID       string
	Name     string
	Parents  []string
	Path     string
	TopLevel string
}

type AuthorizationSession struct {
	server *http.Server
	result chan error
}

func DefaultCredentialsPath() string {
	if override := os.Getenv("KOKO_TOOLS_CREDENTIALS_FILE"); override != "" {
		return override
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(executable), "credentials.json")
	}
	return "credentials.json"
}

func TokenPath() string {
	return filepath.Join(appConfigDir(), "gdrive_token.json")
}

func HasToken() bool {
	_, err := os.Stat(TokenPath())
	return err == nil
}

func HasCredentials() bool {
	_, err := os.Stat(DefaultCredentialsPath())
	return err == nil
}

func AuthorizationURL() (string, error) {
	config, err := configFromFile(DefaultCredentialsPath())
	if err != nil {
		return "", err
	}

	return config.AuthCodeURL("koko-tools", oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}

func StartLocalAuthorization() (string, *AuthorizationSession, error) {
	config, err := configFromFile(DefaultCredentialsPath())
	if err != nil {
		return "", nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("start local callback listener: %w", err)
	}
	state := fmt.Sprintf("koko-tools-%d", time.Now().UnixNano())
	config.RedirectURL = fmt.Sprintf("http://%s/oauth2/callback", listener.Addr().String())

	session := &AuthorizationSession{
		result: make(chan error, 1),
	}

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	session.server = server

	mux.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid OAuth state.", http.StatusBadRequest)
			select {
			case session.result <- errors.New("invalid OAuth state"):
			default:
			}
			return
		}

		authCode := r.URL.Query().Get("code")
		if authCode == "" {
			http.Error(w, "Missing authorization code.", http.StatusBadRequest)
			select {
			case session.result <- errors.New("missing authorization code in callback"):
			default:
			}
			return
		}

		token, err := config.Exchange(context.Background(), authCode)
		if err != nil {
			http.Error(w, "Authorization failed.", http.StatusBadRequest)
			select {
			case session.result <- fmt.Errorf("exchange authorization code: %w", err):
			default:
			}
			return
		}
		if err := saveToken(TokenPath(), token); err != nil {
			http.Error(w, "Failed to save token.", http.StatusInternalServerError)
			select {
			case session.result <- err:
			default:
			}
			return
		}

		_, _ = fmt.Fprintf(w, "<html><body><h1>Google Drive connected</h1><p>You can return to %s.</p></body></html>", html.EscapeString("Koko Tools"))
		select {
		case session.result <- nil:
		default:
		}
		go server.Shutdown(context.Background())
	})

	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case session.result <- fmt.Errorf("authorization callback server: %w", err):
			default:
			}
		}
	}()

	return config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), session, nil
}

func (s *AuthorizationSession) Wait() <-chan error {
	return s.result
}

func (s *AuthorizationSession) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(context.Background())
}

func ListFolders() ([]Folder, error) {
	service, err := serviceFromCredentials()
	if err != nil {
		return nil, err
	}

	resp, err := service.Files.List().
		Q("mimeType='application/vnd.google-apps.folder' and trashed=false").
		Fields("files(id,name,parents)").
		OrderBy("name").
		PageSize(500).
		Do()
	if err != nil {
		return nil, fmt.Errorf("list drive folders: %w", err)
	}

	folders := make([]Folder, 0, len(resp.Files))
	for _, file := range resp.Files {
		folders = append(folders, Folder{ID: file.Id, Name: file.Name, Parents: file.Parents})
	}

	folders = decorateFolderPaths(folders)
	return folders, nil
}

func CreateFolder(name string, parentID string) (Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Folder{}, errors.New("folder name is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return Folder{}, err
	}

	file := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
	}
	if parentID != "" {
		file.Parents = []string{parentID}
	}

	created, err := service.Files.Create(file).Fields("id,name,parents").Do()
	if err != nil {
		return Folder{}, fmt.Errorf("create drive folder: %w", err)
	}

	folder := Folder{ID: created.Id, Name: created.Name, Parents: created.Parents}
	folders := decorateFolderPaths([]Folder{folder})
	return folders[0], nil
}

func SyncAppData(folderID string) error {
	paths, err := syncablePaths(appConfigDir())
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	for _, path := range paths {
		if err := SyncFile(folderID, path); err != nil {
			return err
		}
	}
	return nil
}

func SyncFile(folderID string, localPath string) error {
	if folderID == "" {
		return errors.New("drive folder id is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return err
	}

	name := filepath.Base(localPath)
	query := fmt.Sprintf("name='%s' and '%s' in parents and trashed=false", escapeQueryValue(name), escapeQueryValue(folderID))
	list, err := service.Files.List().
		Q(query).
		Fields("files(id,name)").
		PageSize(1).
		Do()
	if err != nil {
		return fmt.Errorf("query existing drive file: %w", err)
	}

	fileHandle, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer fileHandle.Close()

	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if len(list.Files) == 0 {
		_, err = service.Files.Create(driveCreateFile(name, folderID)).Media(fileHandle, googleapi.ContentType(contentType)).Do()
		if err != nil {
			return fmt.Errorf("create drive file: %w", err)
		}
		return nil
	}

	_, err = service.Files.Update(list.Files[0].Id, driveUpdateFile(name)).Media(fileHandle, googleapi.ContentType(contentType)).Do()
	if err != nil {
		return fmt.Errorf("update drive file: %w", err)
	}

	return nil
}

func driveCreateFile(name string, folderID string) *drive.File {
	file := &drive.File{Name: name}
	if folderID != "" {
		file.Parents = []string{folderID}
	}
	return file
}

func driveUpdateFile(name string) *drive.File {
	return &drive.File{Name: name}
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open token file: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		return fmt.Errorf("encode token: %w", err)
	}

	return nil
}

func appConfigDir() string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(helpers.AppConfigMainDir, helpers.AppConfigAppDir)
	}

	return filepath.Join(dirname, helpers.AppConfigMainDir, helpers.AppConfigAppDir)
}

func configFromFile(credentialsPath string) (*oauth2.Config, error) {
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials file: %w", err)
	}

	return config, nil
}

func serviceFromCredentials() (*drive.Service, error) {
	config, err := configFromFile(DefaultCredentialsPath())
	if err != nil {
		return nil, err
	}

	token, err := tokenFromFile(TokenPath())
	if err != nil {
		return nil, fmt.Errorf("load token: %w", err)
	}

	client := config.Client(context.Background(), token)
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}

	return service, nil
}

func syncablePaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read app config dir: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == filepath.Base(DefaultCredentialsPath()) || name == filepath.Base(TokenPath()) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}

	return paths, nil
}

func escapeQueryValue(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}

func decorateFolderPaths(folders []Folder) []Folder {
	index := make(map[string]*Folder, len(folders))
	for i := range folders {
		index[folders[i].ID] = &folders[i]
	}

	var buildPath func(folder *Folder) string
	var buildTopLevel func(folder *Folder) string
	buildPath = func(folder *Folder) string {
		if folder.Path != "" {
			return folder.Path
		}

		parentName := "Drive"
		if len(folder.Parents) > 0 {
			if parent, ok := index[folder.Parents[0]]; ok {
				parentName = buildPath(parent)
			}
		}

		folder.Path = parentName + " / " + folder.Name
		return folder.Path
	}
	buildTopLevel = func(folder *Folder) string {
		if folder.TopLevel != "" {
			return folder.TopLevel
		}
		if len(folder.Parents) == 0 {
			folder.TopLevel = folder.Name
			return folder.TopLevel
		}
		if parent, ok := index[folder.Parents[0]]; ok {
			if len(parent.Parents) == 0 {
				folder.TopLevel = parent.Name
				return folder.TopLevel
			}
			folder.TopLevel = buildTopLevel(parent)
			return folder.TopLevel
		}
		folder.TopLevel = folder.Name
		return folder.TopLevel
	}

	for i := range folders {
		buildPath(&folders[i])
		buildTopLevel(&folders[i])
	}

	sort.Slice(folders, func(i, j int) bool {
		if folders[i].TopLevel != folders[j].TopLevel {
			return folders[i].TopLevel < folders[j].TopLevel
		}
		return folders[i].Path < folders[j].Path
	})

	return folders
}
