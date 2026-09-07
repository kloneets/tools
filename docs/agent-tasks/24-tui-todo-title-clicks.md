# TUI Todo Title Clicks

## Task
- Request: Add mouse click handling for todo titles in the main TUI pane, not only Todo list sidebar entries.
- Date: 2026-09-04
- Branch or commit: current worktree

## Accepted Plan
- Make rendered todo-item titles and archive-month titles selectable with a single mouse click.
- A click moves keyboard focus and selection to the clicked row but does not check, edit, or expand it; existing `Enter` and `Space` actions remain explicit.
- Preserve drag-to-copy behavior and sidebar clicks.
- Derive hit targets while rendering so empty lines and section headings remain inert.

## Implementation Notes
- Coding agent: Primary Codex.
- Summary: Added renderer-derived main-pane click targets for todo items and archive months, with focused tests.
- Files changed: `src/app/tui.go`, `src/app/tui_test.go`, and this task record.
- Plan deviations: None.
- Tests and checks run: `gofmt -w src/app/tui.go src/app/tui_test.go`; `GOCACHE=/tmp/koko-go-cache-title-clicks go test ./...` passed with localhost access for existing Firebase HTTP tests; Go build to `/tmp/koko-tools-todo-title-clicks` passed; `git diff --check` passed.

## Review Round 1
- Review agent: `/root/reviewer_todo_title_clicks` (GPT-5.6 Luna)
- Status: Clean.
- Findings: None. Click targets follow selectable rows, headings remain inert, archive-month selection is non-activating, and drag-to-copy is preserved.
- Fixes or waivers: None.

## Final Audit
- Done auditor: `/root/auditor_todo_title_clicks`
- Status: Pass.
- Plan items confirmed: Todo-item and archive-month titles are clickable; clicks select without activating; task-pane focus, sidebar behavior, and drag-to-copy are preserved.
- Tests and checks confirmed: Focused click tests, full `go test ./...`, Go build, gofmt, and `git diff --check`.
- Waivers or skipped checks: None.
