# Koko Tools

Koko Tools is a Go terminal desktop app that collects a few everyday utilities in one keyboard-driven interface:

- Markdown notes with Vim-style editing, syntax highlighting, note tabs, and link opening.
- Page conversion for comparing progress between different editions of a book.
- Password generation.
- Optional Firebase sync for todos, notes, and shared settings.
- App settings for editor behavior, theme, transparent background, and local preferences.

The app is built as a local desktop binary and stores its configuration under the user home directory.

## Building and Running

Requirements:

- Go 1.23+

Build the binary:

```sh
go build -o koko-tools
```

Run the terminal app:

```sh
./koko-tools
```

Run all tests:

```sh
go test ./...
```

The first build or test run can take longer while Go downloads and compiles dependencies.

## Android Companion App

A Kotlin Android companion app lives in `android/`. It supports Markdown note editing, todos, Pages calculations, and Firebase sync.

See `android/README.md` for Android Studio import, build commands, local data paths, Firebase setup, and Play Store release notes.

## CLI Commands

### Open Links

`ol` extracts all unique supported links from input and opens them in the default browser or system handler.

From stdin:

```sh
cat notes.md | ./koko-tools ol
```

From a file:

```sh
./koko-tools ol notes.md
```

From multiple inputs:

```sh
./koko-tools ol first.md second.md
```

Use `-` to read stdin as one of the inputs:

```sh
cat extra.md | ./koko-tools ol saved.md -
```

Supported link schemes are `http`, `https`, `ftp`, and `file`. Duplicate links are opened once, preserving first-seen order.

### Firebase Push Local

`firebase-push-local` pushes the current desktop todos, notes, and shared app settings to the configured Firebase personal workspace. Use this when the desktop data is the source of truth and another device needs to pull the latest data.

```sh
./koko-tools firebase-push-local
```

The command uses the saved Firebase refresh token when available. If the token is missing or expired, run it once with:

```sh
KOKO_FIREBASE_EMAIL="you@example.com" KOKO_FIREBASE_PASSWORD="password" ./koko-tools firebase-push-local
```

### Firebase Migrate

`firebase-migrate` copies notes, todos, and shared settings from an old workspace into the current Firebase user's personal workspace, named `user_<uid>`, and updates the desktop Firebase config to use that personal workspace.

```sh
KOKO_FIREBASE_EMAIL="you@example.com" KOKO_FIREBASE_PASSWORD="password" ./koko-tools firebase-migrate <old-workspace-id> --confirm-owner-copy
```

Safety checks:

- The explicit `--confirm-owner-copy` flag is required.
- The source workspace must be different from the target personal workspace.
- The logged-in Firebase user must be an `owner` in `workspaces/<old-workspace-id>/members/<uid>`.
- The command copies data into `user_<uid>` and does not delete the source workspace.

## Terminal UI

The app has six main views:

- `1` Notes
- `2` Pages
- `3` Password
- `4` Todo
- `5` Sync
- `6` Settings

Common keys:

- `ctrl+t`: activate the tab bar.
- `ctrl+tab`: cycle to the next view.
- `1` through `6`: jump to a view while the tab bar is active.
- `ctrl+1` through `ctrl+6`: jump directly to a view without activating the tab bar.
- Mouse click an app tab: jump directly to that view.
- `left` / `right`: move through views while the tab bar is active.
- `enter`: confirm a tab selection.
- `esc`: cancel prompts or mode-specific UI states.
- `?`: open keyboard help where supported.
- `q`: quit when the current view is not in an editing context.

Ghostty on macOS binds `ctrl+tab` to its own `next_tab` action by default, so the app will not receive that key. Add this to your Ghostty config if you want `ctrl+tab` to reach koko-tools:

```ini
keybind = ctrl+tab=csi:9;5u
```
- `ctrl+s`: save local state.

If there are unsaved or unsynced changes, quitting shows a confirmation dialog.

## Notes

The Notes view is the main Markdown workspace. It supports:

- Markdown editing.
- Vim-style normal, insert, visual, and command modes.
- Markdown preview.
- Markdown syntax highlighting and code syntax highlighting.
- Search and visual selection highlighting.
- Quick highlight feedback after yanking words, lines, or visual selections. Word yanks highlight and copy only the word itself, without trailing spaces or punctuation.
- Optional spell checking with installable Hunspell dictionaries, underline markers for misspellings, and one shared custom word file.
- Multiple open note tabs.
- Mouse close buttons on note tabs.
- Sidebar folder and note navigation.
- Note rename, create, delete, save, and restore of open note session.
- Opening all unique external links in the current note.

Useful normal-mode keys:

- `i`: enter insert mode.
- `esc`: return to normal mode.
- `:`: enter command mode.
- `/`: search.
- `n` / `N`: next or previous search match.
- `u`: undo.
- `v` / `V`: visual character or line selection.
- `>` / `<`: indent or outdent the current line, or all selected lines in visual mode.
- `ctrl+a`: switch focus between editor and sidebar.
- `ctrl+a`, then `a`: switch between the two most recently accessed notes and return focus to the editor.
- `ctrl+a`, then `1` through `9` or `0`: switch to that numbered open note and return focus to the editor.
- `zg`: add the word under the cursor to the shared custom spell dictionary.
- `[` / `]`: switch open note tabs.
- `ctrl+s`: save local state.

Useful command-mode commands:

- `:w`: save all local state.
- `:q`: quit through the normal shutdown flow.
- `:wq`: save all local state and close the app.
- `:bd`: close the active note tab without deleting its file.
- `:addword` or `:spelladd`: add the word under the cursor to the shared custom spell dictionary.
- `:ol`: collect external links from the current note and show a confirmation prompt before opening them.
- `:preview`: toggle the preview pane.
- `:sidebar` or `:sb`: toggle sidebar focus.
- `:undo` / `:redo`: undo or redo.
- `:rename <name>`: rename the active note.
- `:%s/old/new/g`: replace all matches in the active note.
- `:s/old/new/g`: replace matches on the current line.

One-character command chaining is supported for commands such as `:wq`.

Spell checking is configured from Settings. Dictionaries are downloaded only when selected in Settings, starting with English, Russian, and Latvian plus several additional Hunspell dictionaries. The app uses native `nuspell` when available, then native `hunspell`, then a basic `.dic` fallback. On macOS, install the preferred native checker with `brew install nuspell` for full Hunspell dictionary support. Settings shows whether each installed dictionary is using a native checker, fallback mode, or cannot load. Words are accepted if they exist in any loaded dictionary or in `.config/koko-tools/spell/custom.txt`.

## Pages

The Pages view helps convert reading progress between two editions of a book.

It tracks:

- Page count for the first book.
- Page count for the second book.
- Current read page.
- Calculated equivalent progress.

Useful keys:

- `j` / `k`: move between fields.
- `e` or `enter`: edit a field.
- digits: enter values while editing.
- `esc`: stop editing.
- `r`: recalculate.

## Password

The Password view generates random passwords.

It supports:

- Toggling letters.
- Toggling numbers.
- Toggling special symbols.
- Changing password length.
- Generating a new password.

Useful keys:

- `g`: generate.
- `l`: toggle letters.
- `n`: toggle numbers.
- `s`: toggle symbols.
- `+` / `-`: adjust length.

## Sync

The Sync view handles Firebase realtime sync.

It shows:

- Whether Firebase realtime sync is enabled.
- The effective Firebase workspace ID.
- Last Firebase sync status.

Actions include:

- Toggle Firebase realtime sync.
- Log in with Google.
- Sync to Firebase.

Firebase uses the app-owned backend by default and syncs data under a personal workspace named `user_<uid>`. Realtime sync covers todos, notes, and shared settings. Shared settings include pages, password generator options, and cross-device notes behavior such as Vim mode, tab spaces, undo levels, spell checking, and dictionaries. Device-local state stays local, including window/UI layout, Android theme, Markdown rendering mode, Firebase credentials, and the currently open note session. The app can save locally without uploading to Firebase.

Remote Firebase data is not applied over active local edits. If pages, notes, todos, or settings are currently being edited or have unsaved local changes, the app defers the remote apply and retries on a later sync after the local action is saved or finished.

## Settings

The Settings view controls local app preferences.

Current settings include:

- Theme.
- Transparent background.
- Vim mode.
- Tab spaces.
- Undo levels.
- Spell checking.
- Spell dictionary downloads.

Built-in themes:

- `tokyo-night`
- `catppuccin`
- `kanagawa`
- `gruvbox`
- `rose-pine`
- `flexoki`

Transparent background is independent from the theme. When transparent background is enabled, app backgrounds use the terminal default background while theme foregrounds, accents, borders, tabs, syntax colors, search, and selection colors remain active.

## Data and Configuration

The app creates its config directory under the user home directory at startup. Settings and note session state are persisted locally. Firebase sync, when enabled and configured, can sync todos, notes, and shared settings.

Changes to settings, file paths, notes, or sync behavior should handle missing directories and read failures safely.

## Privacy And Data Deletion

- Privacy policy: https://koko.lv/koko-tools/privacy-policy.html
- Account and data deletion: https://koko.lv/koko-tools/account-deletion.html

## Development

Project structure:

- `main.go`: process entrypoint and CLI dispatch.
- `src/app`: terminal app shell, views, CLI helpers, theming, and TUI integration.
- `src/notes`: notes workspace, Markdown rendering, Vim commands, and editor behavior.
- `src/pages`: page conversion model.
- `src/password`: password generation model.
- `src/settings`: persisted settings and local sync state.
- `src/helpers`: shared helpers for ANSI styling, assets, clipboard, URI opening, and status.

Common development commands:

```sh
go build -o koko-tools
go test ./...
gofmt -w main.go $(find src -name '*.go')
make build
make test
```

Documentation rule:

- Every new user-facing feature must update this README in the same change.
- New CLI commands should be documented under `CLI Commands`.
- New terminal UI behavior should be documented under the relevant view section.
- New settings should be documented under `Settings`.
- New sync or persistence behavior should be documented under `Sync` or `Data and Configuration`.

## Technologies Used

This project is built with free and open technologies:

- Go: application language and build toolchain. Reference: https://go.dev/
- tcell and tview: terminal UI libraries used by the application shell. References: https://github.com/gdamore/tcell and https://github.com/rivo/tview
- Tree-sitter: parsing and syntax-highlighting foundation for the notes editor, together with free language grammars for Bash, CSS, Go, HTML, Java, JavaScript, JSON, Lua, PHP, Python, Rust, and TypeScript. Reference: https://tree-sitter.github.io/tree-sitter/
- Firebase and Google OAuth 2.0: used for optional cross-device sync and Google sign-in. References: https://firebase.google.com/ and https://developers.google.com/identity/protocols/oauth2
- Font Awesome Free: bundled icon set used for app assets and generated outputs. Reference: https://fontawesome.com/

AI coding agents are used as a development aid in this project for implementation support, refactoring, and test/debug iteration. They are used during development, not as part of the shipped application runtime.
