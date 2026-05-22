package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
	"github.com/kloneets/tools/src/settings"
	kokosync "github.com/kloneets/tools/src/sync"
	"github.com/kloneets/tools/src/todo"
)

func RunCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	if args[0] == "firebase-migrate" {
		return runFirebaseMigrateCLI(args[1:], stdout, stderr), true
	}
	if args[0] == "firebase-push-local" {
		return runFirebasePushLocalCLI(args[1:], stdout, stderr), true
	}
	if args[0] != "ol" {
		return 0, false
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	text, err := readOpenLinksInput(args[1:], stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1, true
	}
	links := notes.CollectSupportedLinks(text)
	for _, link := range links {
		helpers.OpenURI(link)
	}
	fmt.Fprintf(stdout, "opened %d link(s)\n", len(links))
	return 0, true
}

func runFirebasePushLocalCLI(args []string, stdout io.Writer, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: koko-tools firebase-push-local")
		return 1
	}
	cfg, err := kokosync.LoadConfig(kokosync.ConfigPath())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("KOKO_FIREBASE_API_KEY"))
	}
	if apiKey == "" {
		apiKey = kokosync.DefaultAPIKey
	}
	databaseURL := strings.TrimSpace(cfg.DatabaseURL)
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("KOKO_FIREBASE_DATABASE_URL"))
	}
	if databaseURL == "" {
		databaseURL = kokosync.DefaultDatabaseURL
	}
	provider := kokosync.NewFirebaseRESTProvider(apiKey, databaseURL)
	session, err := firebaseSession(context.Background(), provider)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	workspaceID := strings.TrimSpace(cfg.WorkspaceID)
	if workspaceID == "" {
		workspaceID = kokosync.PersonalWorkspaceID(session.UID)
	}
	todoRepo := todo.NewRepository()
	store, err := todoRepo.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	todoSyncer := kokosync.TodoSyncer{
		Provider:    provider,
		WorkspaceID: workspaceID,
		StatePath:   kokosync.StatePath(),
		TokenPath:   kokosync.TokenPath(),
		Session:     session,
	}
	if err := todoSyncer.PushStore(context.Background(), store); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	noteSyncer := kokosync.NoteSyncer{Provider: provider, WorkspaceID: workspaceID, StatePath: kokosync.StatePath(), Session: session}
	app := &terminalApp{}
	noteFiles, err := app.localNoteFiles()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := noteSyncer.PushNotes(context.Background(), noteFiles); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	assetSyncer := kokosync.AssetSyncer{Provider: provider, WorkspaceID: workspaceID, StatePath: kokosync.StatePath(), Session: session}
	assetFiles, err := app.localAssetFiles()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	assetResult, err := assetSyncer.PushAssets(context.Background(), assetFiles)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	settings.Init()
	settingsSyncer := kokosync.SettingsSyncer{Provider: provider, WorkspaceID: workspaceID, StatePath: kokosync.StatePath(), Session: session}
	settingsMap, err := currentSettingsMap()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := settingsSyncer.PushSettings(context.Background(), settingsMap); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cfg.Enabled = true
	cfg.Realtime = true
	cfg.APIKey = apiKey
	cfg.DatabaseURL = databaseURL
	cfg.WorkspaceID = workspaceID
	cfg.Email = session.Email
	if err := kokosync.SaveConfig(kokosync.ConfigPath(), cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "pushed %d todo(s), %d note(s), %d asset(s), and shared settings to %s", len(store.Items), len(noteFiles), assetResult.Pushed, workspaceID)
	if len(assetResult.Skipped) > 0 {
		fmt.Fprintf(stdout, " (skipped %d oversized asset(s))", len(assetResult.Skipped))
	}
	fmt.Fprintln(stdout)
	return 0
}

func runFirebaseMigrateCLI(args []string, stdout io.Writer, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) != "--confirm-owner-copy" {
		fmt.Fprintln(stderr, "usage: koko-tools firebase-migrate <old-workspace-id> --confirm-owner-copy")
		return 1
	}
	cfg, err := kokosync.LoadConfig(kokosync.ConfigPath())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("KOKO_FIREBASE_API_KEY"))
	}
	if apiKey == "" {
		apiKey = kokosync.DefaultAPIKey
	}
	databaseURL := strings.TrimSpace(cfg.DatabaseURL)
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("KOKO_FIREBASE_DATABASE_URL"))
	}
	if databaseURL == "" {
		databaseURL = kokosync.DefaultDatabaseURL
	}
	provider := kokosync.NewFirebaseRESTProvider(apiKey, databaseURL)
	session, err := firebaseSession(context.Background(), provider)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	sourceWorkspaceID := strings.TrimSpace(args[0])
	targetWorkspaceID := kokosync.PersonalWorkspaceID(session.UID)
	if sourceWorkspaceID == targetWorkspaceID {
		fmt.Fprintln(stderr, "source workspace is already the personal workspace")
		return 1
	}
	fmt.Fprintf(stderr, "copying from %s to %s as %s\n", sourceWorkspaceID, targetWorkspaceID, session.Email)
	result, err := provider.MigrateWorkspaceToPersonal(context.Background(), sourceWorkspaceID, session)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cfg.Enabled = true
	cfg.Realtime = true
	cfg.APIKey = apiKey
	cfg.DatabaseURL = databaseURL
	cfg.WorkspaceID = result.TargetWorkspaceID
	cfg.Email = session.Email
	if err := kokosync.SaveConfig(kokosync.ConfigPath(), cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(
		stdout,
		"migrated %d note(s), %d todo(s), %d asset(s), and %d shared settings record(s) from %s to %s\n",
		result.Notes,
		result.Todos,
		result.Assets,
		result.Settings,
		result.SourceWorkspaceID,
		result.TargetWorkspaceID,
	)
	return 0
}

func readOpenLinksInput(paths []string, stdin io.Reader) (string, error) {
	if len(paths) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var b strings.Builder
	for _, path := range paths {
		var data []byte
		var err error
		if path == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return "", err
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.Write(data)
	}
	return b.String(), nil
}
