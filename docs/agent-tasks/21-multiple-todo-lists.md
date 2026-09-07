# Multiple Todo Lists

## Task
- Request: Add support for creating and selecting multiple todo lists in both the TUI and Android app, using note-like selection without folders and without breaking existing todo storage compatibility.
- Date: 2026-09-02
- Branch or commit: main worktree

## Accepted Plan
Add a flat todo-list catalog while preserving the legacy default todo store. Existing `todos.json` remains the default list. New metadata is stored additively in `todo_lists.json`, and named list data is stored under `todo-lists/<id>.json`. Store the currently selected todo list in local client settings, not shared settings.

For the TUI, add a note-sidebar-like list selector for Todo with create, select, rename, and delete actions, plus persistence of the current list. For Android, add equivalent list selection and management through the Todo actions menu. For Firebase sync, keep the default list on existing legacy paths and scope named lists under `todo_lists/<id>/...`; sync list metadata and tombstones additively, deploy the updated rules when credentials permit, and verify the live rules. Add automated coverage for storage compatibility, settings locality, sync path/hash derivation, list metadata merge/tombstones, and UI list-selection behavior.

## Implementation Notes
- Coding agent: GPT-5 fallback coding agent (`/root/coder_fallback`); substituted after the previous strongest-model coding run hit a usage limit.
- Summary: Added a backward-compatible todo-list catalog and named list storage, TUI list sidebar management, Android list picker/actions, local-only todo selection settings, list-aware Firebase todo sync with legacy default paths, additive Firebase rules for named lists, and Go/Kotlin tests for the new behavior.
- Files changed: `src/todo/todo.go`, `src/todo/todo_test.go`, `src/app/tui.go`, `src/app/tui_test.go`, `src/settings/settings.go`, `src/settings/settings_test.go`, `src/sync/hash_sync.go`, `src/sync/types.go`, `src/sync/todo_sync.go`, `src/sync/todo_sync_test.go`, `src/sync/firebase_rest.go`, `src/sync/firebase_rest_test.go`, `src/sync/security_rules.json`, `src/sync/security_rules_test.go`, `src/sync/merge_test.go`, `android/app/src/main/java/com/kloneets/kokotools/TodoRepository.kt`, `android/app/src/main/java/com/kloneets/kokotools/MainActivity.kt`, `android/app/src/main/java/com/kloneets/kokotools/Models.kt`, `android/app/src/main/java/com/kloneets/kokotools/SettingsRepository.kt`, `android/app/src/main/java/com/kloneets/kokotools/FirebaseSyncRepository.kt`, `android/app/src/test/java/com/kloneets/kokotools/TodoRepositoryTest.kt`, `android/app/src/test/java/com/kloneets/kokotools/SettingsRepositoryTest.kt`, and `android/app/src/test/java/com/kloneets/kokotools/FirebaseSyncRepositoryTest.kt`.
- Plan deviations: The strongest coding and first two reviewer attempts hit service usage limits, so the recorded fallback agents and primary Codex completed the required stages. No behavior or compatibility deviation.
- Tests and checks run: `gofmt`; `go test ./...`; `go vet ./...`; `go test -race ./src/app -run TestPullTodoArchiveMonthUsesCapturedListStore`; `go build -o koko-tools`; final clean build to `/tmp/koko-tools-multi-list-check`; `python3 -m json.tool src/sync/security_rules.json`; `git diff --check`; `./gradlew :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`. The reviewed rules were deployed to `koko-tools-default-rtdb` and a normalized live readback matched `src/sync/security_rules.json` exactly; the pre-deployment rules were saved under `/tmp`.

## Review Round 1
- Review agent: GPT-5.5 Codex reviewer.
- Status: Needs changes; all findings fixed.
- Findings: Synthesized Default metadata used current wall-clock timestamps/revisions and could beat a real synced Default rename on a fresh client; Android archive-month loading captured a list for the remote pull but then loaded/saved through the current-list shortcut; TUI archive loading had the same class of async list-switch risk; todo-list metadata PUTs were unconditional and could overwrite newer renames or tombstones from stale clients.
- Fixes or waivers: Fixed with a stable implicit Default metadata baseline in Go and Android plus merge tests proving remote Default rename wins; changed Android and TUI archive loading to capture and use explicit list IDs/files and only update visible state if the captured list is still selected; added conditional/ETag metadata writes and stale-conflict handling in Go and Android; made Firebase rules require monotonic metadata revisions with delete-wins-on-tie semantics while treating missing and false `deleted` values equivalently; added focused tests for repository merges, metadata conflict handling, rule guards, and TUI explicit-list archive loading. No waivers.

## Review Round 2
- Review agent: Primary Codex after the assigned fallback reviewer hit a service usage limit; final confirmation by GPT-5.4 fallback reviewer.
- Status: Clean after fixes.
- Findings: The first stale-write fix rejected obsolete metadata, but a stale active client could still continue into item upload and recreate child data underneath a remote list tombstone.
- Fixes or waivers: Both clients now refresh and merge the remote catalog after a stale metadata rejection, remove local stores whose tombstones win, and enumerate data uploads only from the reconciled active catalog. Regression tests prove a stale list payload is skipped. The final reviewer found no remaining blocking issues. No waivers.

## Final Audit
- Done auditor: GPT-5.4 fallback done auditor (`/root/done_auditor_multi_todo`) after higher-model service limits.
- Status: complete.
- Plan items confirmed: The legacy default todo store remains backward-compatible at `todos.json`; additive `todo_lists.json` metadata plus `todo-lists/<id>.json` named-list storage are implemented; both TUI and Android support create/select/rename/delete flows with persisted local current-list selection; Firebase sync keeps the Default list on legacy paths and scopes named lists under `todo_lists/<id>/...`; stale metadata conflicts now reconcile remote catalog state before any follow-on data upload, including tombstone cleanup and stale-payload suppression; automated coverage exists for storage compatibility, local-only settings persistence, sync path/hash derivation, metadata merge/tombstones, and client list-selection behavior.
- Tests and checks confirmed: Fresh `go test ./...` and `./gradlew :app:testDebugUnitTest :app:assembleDebug :app:lintDebug` passed on 2026-09-03. Earlier in this task, `gofmt`, `go vet ./...`, `go test -race ./src/app -run TestPullTodoArchiveMonthUsesCapturedListStore`, `go build -o koko-tools`, the clean build to `/tmp/koko-tools-multi-list-check`, `python3 -m json.tool src/sync/security_rules.json`, and `git diff --check` also passed. The reviewed Firebase Realtime Database rules were deployed to `koko-tools-default-rtdb` on 2026-09-03 and the normalized live readback matched `src/sync/security_rules.json` exactly.
- Waivers or skipped checks: None.
