# Agent Task Record

## Task
- Request: Preserve Notes and Todo cursor positions when switching between those screens in both Android and TUI.
- Date: 2026-09-24
- Branch or commit: `main` working tree; no commit created by the coding agent.

## Accepted Plan

# Preserve Cursor Positions Across Notes and Todo Screens

## Summary

Preserve in-session editor state when switching between Android Notes and Todo screens. Add TUI regression coverage confirming its existing model-backed cursor state remains intact.

## Implementation Changes

- Android:
  - Capture the outgoing screen's editor state before rebuilding or switching screens.
  - For each note, retain the caret/selection offsets, scroll position, and focus state in memory.
  - Restore that state when reopening the note, clamping offsets if its content changed.
  - Preserve the Todo draft field's caret/selection, scroll, and focus state when leaving and returning.
  - Clear obsolete state when a note is deleted or a Todo draft is submitted.
  - Extend `HybridMarkdownEditor` with internal capture/restore methods using document-relative offsets so rich Markdown mode behaves like raw mode.
  - Keep cursor state session-only; do not alter settings or persisted storage formats.
- TUI:
  - Add regression coverage for Notes editor cursor state, selected Todo row, Todo input caret, and Todo-list sidebar focus across tab switches.
  - Retain the existing model-backed implementation unless those tests reveal an actual reset; any necessary correction will be limited to tab-switch state preservation.
- Record this work in this task record and complete the required coding, review, and done-audit stages.

## Test Plan

- Android instrumentation tests cover raw and rich note state, per-note state, clamping after shorter content, Todo draft state, scroll state, and clearing a submitted Todo draft.
- TUI tests cover Notes and Todo cursor/selection preservation across app-tab switching.
- Run Go tests, Android isolated unit and connected tests, Android lint, and `git diff --check`.

## Assumptions

- Cursor state only needs to survive navigation within the current app process, not app restarts.
- Selection and scroll position are preserved alongside the caret.
- Existing note, Todo, settings, and sync storage formats remain unchanged.

## Implementation Notes
- Coding agent: `/root/cursor_state_coder`.
- Summary: Added session-only Android view-state capture and restoration for raw notes, rich Markdown notes, and Todo drafts. Rich editor positions use document-relative offsets. Added safe offset clamping, state migration when a blank draft becomes `untitled.md`, and stale-state cleanup for all local or remote note deletions, submitted drafts, and list switches. Confirmed TUI state already lives in persistent models and added direct plus routed app-tab regression tests without production TUI changes.
- Files changed: `HybridMarkdownEditor.kt`, `MainActivity.kt`, `MainActivityTest.kt`, `src/app/tui_test.go`, and this task record.
- Plan deviations: None. Connected instrumentation execution was skipped because no ADB device was attached; the instrumentation test source compiles.
- Tests and checks run:
  - `GOCACHE=/tmp/koko-tools-go-cache go test ./src/app -run 'TestSwitchAppTabPreserves(NotesEditorCursor|TodoCursorState)$'` (passed).
  - `GOCACHE=/tmp/koko-tools-go-cache go test ./...` (all packages through `src/settings` and `src/todo` passed; `src/sync` could not open the loopback listener required by `httptest` under the filesystem/network sandbox).
  - `./gradlew :app:compileIsolatedAndroidTestKotlin :app:testIsolatedUnitTest :app:lintIsolated` (passed).
  - `git diff --check` (passed).
  - `adb devices -l` (no attached device, so connected instrumentation was skipped).
  - Independent final verification after Review Round 1 fixes: `go test ./...` passed outside the network sandbox so `src/sync` could bind its loopback `httptest` listeners.
  - Independent final verification after Review Round 1 fixes: `./gradlew --no-parallel :app:testIsolatedUnitTest :app:lintIsolated :app:assembleIsolatedAndroidTest --rerun-tasks` passed.
  - Independent final `git diff --check` passed.
  - `go build -o /tmp/koko-tools-cursor-check ./` passed.
  - `./gradlew --no-parallel :app:assembleDebug` passed.

## Review Round 1
- Review agent: Delegated review reported by `/root`.
- Status: Changes requested; both blocking findings fixed.
- Findings:
  - Cursor state captured under the blank path was not migrated when saving assigned `untitled.md`.
  - Remote deletions did not remove remembered cursor state for non-current notes or deferred deletes applied with `updateEditor = false`.
  - Low-risk coverage gap: TUI tests called `switchAppTab` directly but did not exercise the normal `handleGlobalKey` route.
- Fixes or waivers:
  - `saveCurrentNoteInternal` now migrates the newest state from the previous path to the saved normalized path; instrumentation covers switching an unsaved blank draft away and back before autosave.
  - Direct and deferred remote deletion paths now discard state for the deleted path, and full remote replacement clears all note states; instrumentation covers non-current and deferred deletion before recreating the same path.
  - Added a `handleGlobalKey` Ctrl-number round-trip test that verifies Notes and Todo state remains intact.
  - Focused Go tests, Android instrumentation compilation/unit tests, and `git diff --check` pass after the fixes.

## Review Round 2
- Review agent: `/root/cursor_state_review` (GPT-5.6 Sol).
- Status: Clean.
- Findings: No remaining correctness, storage-compatibility, lifecycle/rebuild, clamping, or stale-state defects. The reviewer confirmed both Round 1 findings and the TUI keyboard-route coverage were addressed.
- Fixes or waivers: No code findings remain. Connected instrumentation execution is recorded as an environmental verification limitation because no ADB device was attached; the instrumentation APK compiles successfully.

## Final Audit
- Done auditor: `/root/cursor_state_done_audit` (GPT-5.6 Sol).
- Status: Complete with waivers.
- Plan items confirmed: Android raw and rich note state, per-note restoration, Todo draft state, safe clamping, state migration/cleanup, session-only storage compatibility, and direct plus keyboard-routed TUI regression coverage are complete. Review Round 2 is clean.
- Tests and checks confirmed: Focused TUI tests, full Go tests, Go build, Android isolated unit tests, isolated lint, instrumentation compilation/assembly, debug assembly, and `git diff --check` passed.
- Waivers or skipped checks: Connected Android instrumentation was not executed because `adb devices -l` reported no attached device. The test APK and all new instrumentation sources compile; on-device runtime remains the sole residual verification risk.
