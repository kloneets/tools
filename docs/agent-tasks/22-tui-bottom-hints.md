# TUI Bottom Hints

## Task
- Request: Move TUI hints to the bottom like the Todo tab and keep one consistent style.
- Date: 2026-09-04
- Branch or commit: current worktree

## Accepted Plan
Remove the inline shortcut rows from `renderPages`, `renderPassword`, and `renderTodo`; add a canonical shared bottom `currentHelpLine` with a consistent lowercase pipe-separated style; keep Todo help contextual for task mode versus list/input mode; do not touch Android, storage, sync behavior, or unrelated multi-list changes; update tests.

## Implementation Notes
- Coding agent: GPT-5.4 fallback Codex; the repository-preferred higher models were unavailable or usage-limited during this continuation.
- Summary: Added the bottom Help bar to the TUI layout, moved Pages/Password/Todo shortcut text into a shared contextual `currentHelpLine`, normalized the bottom hint style to lowercase pipe-separated segments, removed the inline shortcut rows from the affected renderers, and aligned every Todo text-input cursor with the input's new rendered row.
- Files changed: `src/app/tui.go`, `src/app/tui_test.go`, and `docs/agent-tasks/22-tui-bottom-hints.md`.
- Plan deviations: None.
- Tests and checks run: `gofmt -w src/app/tui.go src/app/tui_test.go`; fresh `go test ./...` passed; `go build -o /tmp/koko-tools-bottom-hints .` passed; `git diff --check` passed.

## Review Round 1
- Review agent: GPT-5.4 fallback reviewer (`/root/reviewer_bottom_hints`), with bounded verification by primary Codex while its response was delayed.
- Status: Clean.
- Findings: None. The shared Help bar is included in layout and theme refresh, affected renderers no longer duplicate shortcut rows, Todo list/task/input modes produce contextual bottom hints, and body-height calculations account for the added bar.
- Fixes or waivers: None.

## Final Audit
- Done auditor: GPT-5.6 Luna (`/root/auditor_bottom_hints`).
- Status: complete
- Plan items confirmed:
  - Pages, Password, and Todo no longer render inline shortcut hint rows inside their main panes.
  - The TUI uses the bottom Help bar as the shared shortcut surface.
  - `currentHelpLine` keeps one lowercase pipe-separated style and stays contextual for Todo task, list, and input modes.
  - Height calculations were updated so the extra Help row fits in Notes, Todo, and single-pane views.
  - Focused tests cover the moved hints, contextual help text, and the updated Notes layout height.
- Tests and checks confirmed:
  - `go test ./...`
  - `go build -o /tmp/koko-tools-bottom-hints .`
  - `git diff --check`
- Waivers or skipped checks: None.

## Follow-up Cursor Fix
- Finding: Moving the Todo hint out of the content shifted its input from row 2 to row 1, but `todoInputCursor` retained the old row and displayed the cursor below new-list text.
- Resolution: Updated the cursor row to match the renderer and added table-driven coverage for new todo, edit todo, new list, and rename list input modes.
- Verification: `go test ./...`, `go build -o /tmp/koko-tools-cursor-check .`, and `git diff --check` passed. GPT-5.6 Luna review was clean and the follow-up done audit reported `complete` with no waivers.
