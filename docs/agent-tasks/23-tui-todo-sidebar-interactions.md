# TUI Todo Sidebar Interactions

## Task
- Request: Hide the TUI Todo list sidebar on the second `Ctrl+A` and support mouse selection of Todo list tabs.
- Date: 2026-09-04
- Branch or commit: current worktree

## Accepted Plan
- Make `Ctrl+A` a two-state Todo sidebar toggle: show and focus when hidden, then hide and return to tasks on the second press.
- Open a Todo list when its visible sidebar row is clicked, using the same visible-range calculation for rendering and hit testing.
- Preserve keyboard list management, task text selection, storage, Android, and sync behavior.
- Add focused toggle and mouse-selection tests, then run the Go verification suite and workflow review.

## Implementation Notes
- Coding agent: Primary Codex.
- Summary: Updated the Todo sidebar toggle and added list-row mouse hit testing with shared scrolling calculations.
- Files changed: `src/app/tui.go`, `src/app/tui_test.go`, and this task record.
- Plan deviations: None.
- Tests and checks run: `gofmt -w src/app/tui.go src/app/tui_test.go`; `go test ./...`; `go build -o /tmp/koko-tools-todo-sidebar-click .`; `git diff --check`.

## Review Round 1
- Review agent: GPT-5.6 Luna (`/root/reviewer_sidebar_interactions`).
- Status: Needs changes.
- Findings: The sidebar renderer and mouse hit testing used the scrolled visible-range offset, but cursor positioning used the absolute list index and could highlight the wrong row in an oversized catalog.
- Fixes or waivers: Cursor positioning now subtracts the shared visible-range start, with focused regression coverage for a scrolled catalog. No waiver.

## Review Round 2
- Review agent: GPT-5.6 Luna (`/root/reviewer_sidebar_interactions`).
- Status: Clean.
- Findings: None. Rendering, mouse hit testing, and cursor placement now share the same visible-range offset.
- Fixes or waivers: None.

## Final Audit
- Done auditor: GPT-5.6 Luna (`/root/auditor_sidebar_interactions`).
- Status: Complete.
- Plan items confirmed: First `Ctrl+A` shows and focuses the sidebar; the second hides it and returns to tasks; visible Todo list rows open by mouse click; rendering, hit testing, and cursor placement share one scroll-range calculation; existing keyboard, storage, Android, and sync behavior is preserved.
- Tests and checks confirmed: Focused toggle, mouse-selection, and scrolled-cursor tests; `go test ./...`; Go build; `git diff --check`; clean Review Round 2.
- Waivers or skipped checks: None.
