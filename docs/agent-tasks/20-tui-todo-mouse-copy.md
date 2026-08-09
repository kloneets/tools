# TUI Todo Mouse Selection and Copy

## Task
- Request: Add mouse-only text selection and automatic clipboard copying to the TUI Todo view.
- Date: 2026-08-09
- Branch or commit: current worktree

## Accepted Plan
Support exact visible-text selection by dragging in the Todo body. Copy a non-empty selection on mouse release, retain themed highlighting for 180 ms, preserve newlines and visible markers, suppress link opening after a drag, and leave keyboard behavior and todo data unchanged.

## Implementation Notes
- Coding agent: GPT-5.5 Codex
- Summary: Added Todo-body mouse drag selection, exact visible-text extraction, automatic clipboard copy on release, themed transient highlighting, coordinate clamping, link-drag isolation, and selection cleanup on view changes.
- Files changed: `src/app/tui.go`, `src/app/tui_test.go`, and `README.md`.
- Plan deviations: None.
- Tests and checks run: `GOCACHE=/tmp/koko-go-cache go test ./...`; `GOCACHE=/tmp/koko-go-cache go vet ./...`; `GOCACHE=/tmp/koko-go-cache go build -o /tmp/koko-tools-todo-selection .`; targeted `go test -race`; `git diff --check`.

## Review Round 1
- Review agent: GPT-5.5 Codex, reviewer role.
- Status: Needs changes.
- Findings: Direct selection fields widened gofmt alignment across `terminalApp`; nil-TUI tests could schedule unnecessary background refresh timers.
- Fixes or waivers: Grouped state into `todoMouseSelection` and limited delayed UI clearing to a running TUI. Focused race tests cover the resulting path.

## Review Round 2
- Review agent: GPT-5.5 Codex, reviewer role.
- Status: Clean.
- Findings: None.
- Fixes or waivers: None.

## Final Audit
- Done auditor: GPT-5.5 Codex, done-auditor role.
- Status: Complete with waivers.
- Plan items confirmed: Todo-only mouse selection, exact visible text and newlines, automatic release copy, clipboard errors, reverse and Unicode mapping, transient generation-safe highlight, link drag isolation, empty-click behavior, and view-change cleanup are implemented and tested.
- Tests and checks confirmed: Full tests, vet, build, diff validation, and race-enabled Todo selection tests passed.
- Waivers or skipped checks: The optional full `go test -race ./src/app` remains red because of pre-existing Firebase/settings and sync-operation test races unrelated to this change. Manual terminal clipboard testing was not run; clipboard behavior uses the existing tested helper and is covered with injected writers.
