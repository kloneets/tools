# Agent Task Record

## Task
- Request: Add multi-cursor mode for Notes and suggest best practices; user accepted both platforms and the occurrence-based workflow in the plan.
- Date: 2026-10-02.

## Accepted Plan
- Implement temporary multi-cursor editing in TUI and Android Notes.
- Start from selected text, or the word at the primary cursor. Add the next exact occurrence after the primary selection, without wrapping; report when exhausted.
- Keep sorted in-memory cursor/selection offsets; mirror typing, Backspace, and Delete; maintain the primary cursor; coalesce overlapping selections; use Unicode-safe offsets; group mirrored edits as one undo action where supported.
- TUI exposes `:mc` and a documented add-next key; Escape clears secondary cursors while retaining the primary cursor.
- Android exposes a Multi-cursor action and Add next occurrence. Rich mode temporarily enters whole-note Markdown source editing and restores rich rendering on exit.
- Clear secondary cursors on note switch, document replacement, or exit. No persistent format changes.
- Tests cover occurrence bounds, repeats, Unicode, overlaps, offset shifts, mirrored edits, undo, TUI routing/highlights/exit, Android IME/source-mode restoration/content/scroll.
- Run Go tests/build and Android isolated tests, lint, and instrumentation compilation; connected instrumentation when a device is available.

## Implementation Notes
- Coding agents: `/root` (TUI) and `/root/android_multi_cursor` (Android).
- Model fallback: GPT-5.5 unavailable; strongest inherited available model used and recorded.
- TUI implementation: rune-offset cursor selections on the note model; `:mc` and Ctrl+Alt+M enter; Alt+N adds a later exact match; Escape leaves the original primary cursor. Mirrored insert, newline, tab, Backspace and Delete transform all ranges right-to-left. Existing undo snapshots capture one whole mirrored action. Tabs clear mode when switching notes. Added help hints and route/model tests.
- TUI checks: focused `go test ./src/notes ./src/app` passed; final full Go test and build passed.
- Android summary: in-memory UTF-16 cursor ranges, exact next-occurrence selection, mirrored native EditText edits, secondary selection highlighting, actions-menu entry/exit, temporary whole-note source editor while multi-cursor mode is active, and rich editor selection/scroll restoration on exit. Cursor state clears on note switch, remote document replacement, and exit. No stored format changes.
- Files changed: `src/notes/tui_model.go`, `src/notes/multicursor.go`, `src/notes/multicursor_test.go`, `src/app/tui.go`, `src/app/tui_test.go`, `android/app/src/main/java/com/kloneets/kokotools/MainActivity.kt`, `android/app/src/main/java/com/kloneets/kokotools/MultiCursorEditing.kt`, `android/app/src/test/java/com/kloneets/kokotools/MultiCursorEditingTest.kt`, `android/app/src/androidTest/java/com/kloneets/kokotools/MainActivityTest.kt`.
- Deviations: TUI visual block selections are explicitly rejected with an explanation; word, character, and line selections work. Connected Android instrumentation was attempted but no device is connected, so runtime instrumentation remains unverified.
- Verification: `GOCACHE=/tmp/koko-tools-go-cache go test ./...` passed; `GOCACHE=/tmp/koko-tools-go-cache go build -o /tmp/koko-tools-multicursor` passed; `git diff --check` passed. `./gradlew :app:testIsolatedUnitTest :app:lintIsolated :app:compileIsolatedAndroidTestKotlin` passed after final edits. `./gradlew :app:connectedIsolatedAndroidTest` could not run because Gradle reported `No connected devices!`. The initial generic `:app:testDebugUnitTest` task name does not exist in this flavor configuration; the corresponding isolated tasks were run successfully.

## Review
- Status: Clean.
- Findings addressed: TUI movement now preserves the occurrence query and line selections work; block selection is explicitly rejected. Android computes adjacent-delete bounds against old text then shifts through the primary edit, saves/restores rich view state across source mode, promotes the native edited cursor including match-boundary carets, and covers IME composition, rich scroll restoration, and highlight cleanup in instrumentation tests. Final independent re-review found no blocking issue.

## Final Audit
- Status: Complete with waivers.
- Model fallback: GPT-5.5 unavailable; latest available model used for audit.
- Waiver: Connected Android instrumentation could not run because no device is attached. The instrumentation suite compiles and includes the planned integration assertions.
