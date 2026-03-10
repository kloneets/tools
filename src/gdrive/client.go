package gdrive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
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

type localNoteFile struct {
	RelPath string
	Path    string
}

type remoteDriveEntry struct {
	ID       string
	RelPath  string
	ParentID string
}

type remoteStateEntry struct {
	Path         string
	MimeType     string
	ModifiedTime string
	Checksum     string
}

const (
	defaultOAuthClientID     = "863485242223-ghqf2jt00v710rkt27oieivkg7h613nr.apps.googleusercontent.com"
	defaultOAuthClientSecret = "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"
)

func TokenPath() string {
	return filepath.Join(appConfigDir(), "gdrive_token.json")
}

func HasToken() bool {
	_, err := os.Stat(TokenPath())
	return err == nil
}

func OAuthClientID() string {
	if override := strings.TrimSpace(os.Getenv("KOKO_TOOLS_GOOGLE_CLIENT_ID")); override != "" {
		return override
	}
	return strings.TrimSpace(defaultOAuthClientID)
}

func OAuthClientSecret() string {
	if override := strings.TrimSpace(os.Getenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET")); override != "" {
		return override
	}
	return strings.TrimSpace(defaultOAuthClientSecret)
}

func OAuthClientLabel() string {
	clientID := OAuthClientID()
	if clientID == "" {
		return "not configured"
	}
	return clientID
}

func HasCredentials() bool {
	return OAuthClientID() != "" && OAuthClientSecret() != ""
}

func AuthorizationURL() (string, error) {
	config, err := oauthConfig()
	if err != nil {
		return "", err
	}

	return config.AuthCodeURL("koko-tools", oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}

func StartLocalAuthorization() (string, *AuthorizationSession, error) {
	config, err := oauthConfig()
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

func DownloadFile(folderID string, name string) ([]byte, bool, error) {
	if folderID == "" {
		return nil, false, errors.New("drive folder id is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return nil, false, err
	}

	return downloadDriveFile(service, folderID, name)
}

func RemoteStateSignature(folderID string) (string, error) {
	if folderID == "" {
		return "", errors.New("drive folder id is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return "", err
	}

	entries := make([]remoteStateEntry, 0, 64)
	if settingsEntry, err := remoteSettingsStateEntry(service, folderID); err != nil {
		return "", err
	} else if settingsEntry != nil {
		entries = append(entries, *settingsEntry)
	}

	notesFolderID, found, err := findFolder(service, folderID, "notes")
	if err != nil {
		return "", err
	}
	if found {
		noteEntries, err := remoteNotesStateEntries(service, notesFolderID, "")
		if err != nil {
			return "", err
		}
		entries = append(entries, noteEntries...)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		if entries[i].MimeType != entries[j].MimeType {
			return entries[i].MimeType < entries[j].MimeType
		}
		if entries[i].ModifiedTime != entries[j].ModifiedTime {
			return entries[i].ModifiedTime < entries[j].ModifiedTime
		}
		return entries[i].Checksum < entries[j].Checksum
	})

	data, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal drive state signature: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func RestoreNotesTree(folderID string) error {
	if folderID == "" {
		return errors.New("drive folder id is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return err
	}

	notesFolderID, found, err := findFolder(service, folderID, "notes")
	if err != nil {
		return err
	}

	localRoot := filepath.Join(appConfigDir(), "notes")
	if !found {
		return resetLocalNotesRoot(localRoot)
	}

	return restoreNotesTree(service, notesFolderID, localRoot)
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

func SyncAppData(folderID string, settingsData []byte) error {
	if folderID == "" {
		return errors.New("drive folder id is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return err
	}

	if len(settingsData) > 0 {
		if err := syncBytes(service, folderID, "settings.json", settingsData, "application/json"); err != nil {
			return err
		}
	}

	notesFolderID, err := ensureFolder(service, folderID, "notes")
	if err != nil {
		return err
	}

	return syncNotesTree(service, notesFolderID, filepath.Join(appConfigDir(), "notes"))
}

func SyncFile(folderID string, localPath string) error {
	if folderID == "" {
		return errors.New("drive folder id is required")
	}

	service, err := serviceFromCredentials()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return upsertDriveFileData(service, folderID, filepath.Base(localPath), data, contentType, false)
}

func syncNotesTree(service *drive.Service, notesFolderID string, localRoot string) error {
	localDirs, localFiles, err := scanLocalNotesTree(localRoot)
	if err != nil {
		return err
	}

	dirIDs := map[string]string{"": notesFolderID}
	for _, relDir := range localDirs {
		dirID, err := ensureFolderPath(service, notesFolderID, relDir)
		if err != nil {
			return err
		}
		dirIDs[relDir] = dirID
	}

	desiredFiles := make(map[string]struct{}, len(localFiles))
	for _, file := range localFiles {
		parentRel := filepath.ToSlash(filepath.Dir(file.RelPath))
		if parentRel == "." {
			parentRel = ""
		}
		parentID, ok := dirIDs[parentRel]
		if !ok {
			var err error
			parentID, err = ensureFolderPath(service, notesFolderID, parentRel)
			if err != nil {
				return err
			}
			dirIDs[parentRel] = parentID
		}
		if err := syncLocalFileToParent(service, parentID, filepath.Base(file.RelPath), file.Path); err != nil {
			return err
		}
		desiredFiles[file.RelPath] = struct{}{}
	}

	desiredDirs := make(map[string]struct{}, len(localDirs))
	for _, relDir := range localDirs {
		desiredDirs[relDir] = struct{}{}
	}

	remoteDirs, remoteFiles, err := listRemoteNotesTree(service, notesFolderID)
	if err != nil {
		return err
	}

	for relPath, file := range remoteFiles {
		if _, ok := desiredFiles[relPath]; ok {
			continue
		}
		if isDriveTrashRelativePath(relPath) {
			if err := service.Files.Delete(file.ID).Do(); err != nil {
				return fmt.Errorf("delete drive trash file %q: %w", relPath, err)
			}
			continue
		}
		if err := moveDriveEntryToTrash(service, notesFolderID, file); err != nil {
			return fmt.Errorf("move drive file %q to trash: %w", relPath, err)
		}
	}

	remoteDirPaths := sortedPathsDescending(remoteDirs)
	for _, relPath := range remoteDirPaths {
		if _, ok := desiredDirs[relPath]; ok {
			continue
		}
		if err := deleteDriveFolderIfEmpty(service, remoteDirs[relPath]); err != nil {
			return fmt.Errorf("delete drive folder %q: %w", relPath, err)
		}
	}

	return nil
}

func restoreNotesTree(service *drive.Service, notesFolderID string, localRoot string) error {
	if err := resetLocalNotesRoot(localRoot); err != nil {
		return err
	}

	type queueItem struct {
		folderID string
		localDir string
	}
	queue := []queueItem{{folderID: notesFolderID, localDir: localRoot}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		children, err := listDriveChildren(service, current.folderID)
		if err != nil {
			return err
		}
		for _, child := range children {
			localPath := filepath.Join(current.localDir, child.Name)
			if child.MimeType == "application/vnd.google-apps.folder" {
				if err := os.MkdirAll(localPath, 0o755); err != nil {
					return fmt.Errorf("create local notes folder %q: %w", localPath, err)
				}
				queue = append(queue, queueItem{folderID: child.Id, localDir: localPath})
				continue
			}

			data, err := downloadDriveFileByID(service, child.Id)
			if err != nil {
				return err
			}
			if err := os.WriteFile(localPath, data, 0o644); err != nil {
				return fmt.Errorf("write local note %q: %w", localPath, err)
			}
		}
	}

	return nil
}

func resetLocalNotesRoot(localRoot string) error {
	if err := os.RemoveAll(localRoot); err != nil {
		return fmt.Errorf("clear local notes tree: %w", err)
	}
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		return fmt.Errorf("create local notes root: %w", err)
	}
	return nil
}

func syncBytes(service *drive.Service, folderID string, name string, data []byte, contentType string) error {
	return upsertDriveFileData(service, folderID, name, data, contentType, false)
}

func syncLocalFileToParent(service *drive.Service, folderID string, name string, localPath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return upsertDriveFileData(service, folderID, name, data, contentType, true)
}

func upsertDriveFileData(service *drive.Service, folderID string, name string, data []byte, contentType string, versionOnConflict bool) error {
	query := fmt.Sprintf(
		"name='%s' and '%s' in parents and trashed=false and mimeType!='application/vnd.google-apps.folder'",
		escapeQueryValue(name),
		escapeQueryValue(folderID),
	)
	list, err := service.Files.List().
		Q(query).
		Fields("files(id,name)").
		PageSize(1).
		Do()
	if err != nil {
		return fmt.Errorf("query existing drive file: %w", err)
	}

	if len(list.Files) == 0 {
		_, err = service.Files.Create(driveCreateFile(name, folderID)).Media(bytes.NewReader(data), googleapi.ContentType(contentType)).Do()
		if err != nil {
			return fmt.Errorf("create drive file: %w", err)
		}
		return nil
	}

	if versionOnConflict {
		remoteData, err := downloadDriveFileByID(service, list.Files[0].Id)
		if err != nil {
			return err
		}
		if bytes.Equal(remoteData, data) {
			return nil
		}
		exists, err := hasMatchingVersionedDriveFile(service, folderID, name, data)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		versionName, err := nextVersionedDriveFileName(service, folderID, name)
		if err != nil {
			return err
		}
		_, err = service.Files.Create(driveCreateFile(versionName, folderID)).Media(bytes.NewReader(data), googleapi.ContentType(contentType)).Do()
		if err != nil {
			return fmt.Errorf("create drive versioned file: %w", err)
		}
		return nil
	}

	_, err = service.Files.Update(list.Files[0].Id, driveUpdateFile(name)).Media(bytes.NewReader(data), googleapi.ContentType(contentType)).Do()
	if err != nil {
		return fmt.Errorf("update drive file: %w", err)
	}
	return nil
}

func nextVersionedDriveFileName(service *drive.Service, folderID string, name string) (string, error) {
	for version := 2; ; version++ {
		candidate := versionedDriveFileName(name, version)
		_, found, err := findFile(service, folderID, candidate)
		if err != nil {
			return "", err
		}
		if !found {
			return candidate, nil
		}
	}
}

func versionedDriveFileName(name string, version int) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s (version %d)%s", base, version, ext)
}

func versionedDriveFolderName(name string, version int) string {
	return fmt.Sprintf("%s %d", name, version)
}

func hasMatchingVersionedDriveFile(service *drive.Service, folderID string, name string, data []byte) (bool, error) {
	children, err := listDriveChildren(service, folderID)
	if err != nil {
		return false, err
	}
	for _, child := range children {
		if child.MimeType == "application/vnd.google-apps.folder" {
			continue
		}
		if !isVersionedDriveFileName(name, child.Name) {
			continue
		}
		childData, err := downloadDriveFileByID(service, child.Id)
		if err != nil {
			return false, err
		}
		if bytes.Equal(childData, data) {
			return true, nil
		}
	}
	return false, nil
}

func isVersionedDriveFileName(name string, candidate string) bool {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if ext == "" {
		return strings.HasPrefix(candidate, base+" (version ") && strings.HasSuffix(candidate, ")")
	}
	return strings.HasPrefix(candidate, base+" (version ") && strings.HasSuffix(candidate, ")"+ext)
}

func ensureFolderPath(service *drive.Service, rootFolderID string, relPath string) (string, error) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || relPath == "." {
		return rootFolderID, nil
	}

	parentID := rootFolderID
	for _, part := range strings.Split(relPath, "/") {
		if part == "" || part == "." {
			continue
		}
		folderID, err := ensureFolder(service, parentID, part)
		if err != nil {
			return "", err
		}
		parentID = folderID
	}
	return parentID, nil
}

func ensureFolder(service *drive.Service, parentID string, name string) (string, error) {
	folderID, found, err := findFolder(service, parentID, name)
	if err != nil {
		return "", err
	}
	if found {
		return folderID, nil
	}

	created, err := service.Files.Create(&drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentID},
	}).Fields("id").Do()
	if err != nil {
		return "", fmt.Errorf("create drive folder %q: %w", name, err)
	}
	return created.Id, nil
}

func findFolder(service *drive.Service, parentID string, name string) (string, bool, error) {
	query := fmt.Sprintf(
		"name='%s' and '%s' in parents and trashed=false and mimeType='application/vnd.google-apps.folder'",
		escapeQueryValue(name),
		escapeQueryValue(parentID),
	)
	list, err := service.Files.List().
		Q(query).
		Fields("files(id,name)").
		PageSize(1).
		Do()
	if err != nil {
		return "", false, fmt.Errorf("query drive folder %q: %w", name, err)
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, true, nil
	}
	return "", false, nil
}

func findFile(service *drive.Service, parentID string, name string) (string, bool, error) {
	query := fmt.Sprintf(
		"name='%s' and '%s' in parents and trashed=false and mimeType!='application/vnd.google-apps.folder'",
		escapeQueryValue(name),
		escapeQueryValue(parentID),
	)
	list, err := service.Files.List().
		Q(query).
		Fields("files(id,name)").
		PageSize(1).
		Do()
	if err != nil {
		return "", false, fmt.Errorf("query drive file %q: %w", name, err)
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, true, nil
	}
	return "", false, nil
}

func listRemoteNotesTree(service *drive.Service, rootFolderID string) (map[string]remoteDriveEntry, map[string]remoteDriveEntry, error) {
	dirs := make(map[string]remoteDriveEntry)
	files := make(map[string]remoteDriveEntry)

	type queueItem struct {
		folderID string
		relPath  string
	}
	queue := []queueItem{{folderID: rootFolderID, relPath: ""}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		children, err := listDriveChildren(service, current.folderID)
		if err != nil {
			return nil, nil, err
		}
		for _, child := range children {
			childRel := child.Name
			if current.relPath != "" {
				childRel = current.relPath + "/" + child.Name
			}
			if child.MimeType == "application/vnd.google-apps.folder" {
				dirs[childRel] = remoteDriveEntry{ID: child.Id, RelPath: childRel, ParentID: current.folderID}
				queue = append(queue, queueItem{folderID: child.Id, relPath: childRel})
				continue
			}
			files[childRel] = remoteDriveEntry{ID: child.Id, RelPath: childRel, ParentID: current.folderID}
		}
	}

	return dirs, files, nil
}

func listDriveChildren(service *drive.Service, parentID string) ([]*drive.File, error) {
	children := make([]*drive.File, 0, 32)
	pageToken := ""
	for {
		call := service.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed=false", escapeQueryValue(parentID))).
			Fields("nextPageToken,files(id,name,mimeType,parents,modifiedTime,md5Checksum)").
			OrderBy("folder,name").
			PageSize(1000)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list drive children: %w", err)
		}
		children = append(children, resp.Files...)
		if resp.NextPageToken == "" {
			return children, nil
		}
		pageToken = resp.NextPageToken
	}
}

func remoteSettingsStateEntry(service *drive.Service, folderID string) (*remoteStateEntry, error) {
	query := fmt.Sprintf(
		"name='%s' and '%s' in parents and trashed=false and mimeType!='application/vnd.google-apps.folder'",
		escapeQueryValue("settings.json"),
		escapeQueryValue(folderID),
	)
	list, err := service.Files.List().
		Q(query).
		Fields("files(name,mimeType,modifiedTime,md5Checksum)").
		PageSize(1).
		Do()
	if err != nil {
		return nil, fmt.Errorf("query drive settings state: %w", err)
	}
	if len(list.Files) == 0 {
		return nil, nil
	}
	file := list.Files[0]
	return &remoteStateEntry{
		Path:         "settings.json",
		MimeType:     file.MimeType,
		ModifiedTime: file.ModifiedTime,
		Checksum:     file.Md5Checksum,
	}, nil
}

func remoteNotesStateEntries(service *drive.Service, folderID string, relPath string) ([]remoteStateEntry, error) {
	children, err := listDriveChildren(service, folderID)
	if err != nil {
		return nil, err
	}
	entries := make([]remoteStateEntry, 0, len(children))
	for _, child := range children {
		childRel := child.Name
		if relPath != "" {
			childRel = relPath + "/" + child.Name
		}
		entries = append(entries, remoteStateEntry{
			Path:         "notes/" + childRel,
			MimeType:     child.MimeType,
			ModifiedTime: child.ModifiedTime,
			Checksum:     child.Md5Checksum,
		})
		if child.MimeType == "application/vnd.google-apps.folder" {
			nested, err := remoteNotesStateEntries(service, child.Id, childRel)
			if err != nil {
				return nil, err
			}
			entries = append(entries, nested...)
		}
	}
	return entries, nil
}

func downloadDriveFile(service *drive.Service, folderID string, name string) ([]byte, bool, error) {
	query := fmt.Sprintf(
		"name='%s' and '%s' in parents and trashed=false and mimeType!='application/vnd.google-apps.folder'",
		escapeQueryValue(name),
		escapeQueryValue(folderID),
	)
	list, err := service.Files.List().
		Q(query).
		Fields("files(id,name)").
		PageSize(1).
		Do()
	if err != nil {
		return nil, false, fmt.Errorf("query drive file %q: %w", name, err)
	}
	if len(list.Files) == 0 {
		return nil, false, nil
	}

	data, err := downloadDriveFileByID(service, list.Files[0].Id)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func downloadDriveFileByID(service *drive.Service, fileID string) ([]byte, error) {
	resp, err := service.Files.Get(fileID).Download()
	if err != nil {
		return nil, fmt.Errorf("download drive file: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read drive file: %w", err)
	}
	return data, nil
}

func scanLocalNotesTree(root string) ([]string, []localNoteFile, error) {
	dirs := make([]string, 0, 16)
	files := make([]localNoteFile, 0, 32)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if d.IsDir() {
			dirs = append(dirs, relPath)
			return nil
		}
		files = append(files, localNoteFile{RelPath: relPath, Path: path})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("scan local notes tree: %w", err)
	}

	sort.Strings(dirs)
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return dirs, files, nil
}

func sortedPathsDescending(values map[string]remoteDriveEntry) []string {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		depthI := strings.Count(paths[i], "/")
		depthJ := strings.Count(paths[j], "/")
		if depthI != depthJ {
			return depthI > depthJ
		}
		return paths[i] > paths[j]
	})
	return paths
}

func moveDriveEntryToTrash(service *drive.Service, notesFolderID string, entry remoteDriveEntry) error {
	targetRel := driveTrashRelativePath(entry.RelPath)
	return moveDriveEntryToRelPath(service, notesFolderID, entry, targetRel, false)
}

func moveDriveEntryToRelPath(service *drive.Service, notesFolderID string, entry remoteDriveEntry, targetRel string, isDir bool) error {
	targetRel = filepath.ToSlash(filepath.Clean(targetRel))
	if targetRel == "." || targetRel == "" {
		return errors.New("target path is required")
	}

	targetParentRel := filepath.ToSlash(filepath.Dir(targetRel))
	if targetParentRel == "." {
		targetParentRel = ""
	}
	targetParentID, err := ensureFolderPath(service, notesFolderID, targetParentRel)
	if err != nil {
		return err
	}

	targetName, deleteSource, err := resolveDriveMoveTargetName(service, targetParentID, entry, filepath.Base(targetRel), isDir)
	if err != nil {
		return err
	}
	if deleteSource {
		return service.Files.Delete(entry.ID).Do()
	}
	if targetName == filepath.Base(entry.RelPath) && entry.ParentID == targetParentID {
		return nil
	}

	call := service.Files.Update(entry.ID, driveUpdateFile(targetName))
	if entry.ParentID != "" && entry.ParentID != targetParentID {
		call = call.AddParents(targetParentID).RemoveParents(entry.ParentID)
	} else if entry.ParentID == "" && targetParentID != "" {
		call = call.AddParents(targetParentID)
	}
	if _, err := call.Do(); err != nil {
		return fmt.Errorf("update drive entry location: %w", err)
	}
	return nil
}

func resolveDriveMoveTargetName(service *drive.Service, targetParentID string, entry remoteDriveEntry, targetName string, isDir bool) (string, bool, error) {
	existingID, found, err := findDriveEntry(service, targetParentID, targetName, isDir)
	if err != nil {
		return "", false, err
	}
	if !found || existingID == entry.ID {
		return targetName, false, nil
	}
	if !isDir {
		sourceData, err := downloadDriveFileByID(service, entry.ID)
		if err != nil {
			return "", false, err
		}
		targetData, err := downloadDriveFileByID(service, existingID)
		if err != nil {
			return "", false, err
		}
		if bytes.Equal(sourceData, targetData) {
			return "", true, nil
		}
	}

	nextName, err := nextAvailableDriveEntryName(service, targetParentID, targetName, isDir)
	if err != nil {
		return "", false, err
	}
	return nextName, false, nil
}

func nextAvailableDriveEntryName(service *drive.Service, parentID string, name string, isDir bool) (string, error) {
	for version := 2; ; version++ {
		candidate := versionedDriveFileName(name, version)
		if isDir {
			candidate = versionedDriveFolderName(name, version)
		}
		_, found, err := findDriveEntry(service, parentID, candidate, isDir)
		if err != nil {
			return "", err
		}
		if !found {
			return candidate, nil
		}
	}
}

func findDriveEntry(service *drive.Service, parentID string, name string, isDir bool) (string, bool, error) {
	if isDir {
		return findFolder(service, parentID, name)
	}
	return findFile(service, parentID, name)
}

func deleteDriveFolderIfEmpty(service *drive.Service, entry remoteDriveEntry) error {
	children, err := listDriveChildren(service, entry.ID)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return nil
	}
	if err := service.Files.Delete(entry.ID).Do(); err != nil {
		return err
	}
	return nil
}

func isDriveTrashRelativePath(relPath string) bool {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	return relPath == "trash" || strings.HasPrefix(relPath, "trash/")
}

func driveTrashRelativePath(relPath string) string {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || relPath == "" {
		return "trash"
	}
	if isDriveTrashRelativePath(relPath) {
		return relPath
	}
	return filepath.ToSlash(filepath.Join("trash", relPath))
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

func oauthConfig() (*oauth2.Config, error) {
	clientID := OAuthClientID()
	clientSecret := OAuthClientSecret()
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("missing Google OAuth client configuration")
	}

	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveScope},
	}, nil
}

func serviceFromCredentials() (*drive.Service, error) {
	config, err := oauthConfig()
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
