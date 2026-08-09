# TUI Clickable Links

## Task
- Request: Make supported links clickable in every TUI section.
- Date: 2026-08-09
- Branch or commit: current worktree

## Accepted Plan
Make bare and Markdown `http`, `https`, `ftp`, and `file` links visually identifiable and clickable throughout the TUI. Read-only surfaces open links with a normal click; the editable note pane requires Ctrl+click so cursor placement and drag selection remain unchanged. Coordinate mapping must account for wrapping, scrolling, formatting, gutters, and wide characters. Update tests and README documentation.

## Implementation Notes
- Coding agent: GPT-5.5 Codex
- Summary: Added shared supported-link spans, themed link rendering, coordinate-aware hit testing for note editor/preview and all single-pane TUI views, read-only click activation, and Ctrl+click activation in the editor.
- Files changed: `src/notes/vim_command.go`, `src/notes/tui_model.go`, `src/app/tui.go`, their tests, and `README.md`.
- Plan deviations: None.
- Tests and checks run: `GOCACHE=/tmp/koko-go-cache go test ./...`; `GOCACHE=/tmp/koko-go-cache go vet ./...`; `GOCACHE=/tmp/koko-go-cache go build -o /tmp/koko-tools-clickable-links .`; `git diff --check`.

## Review Round 1
- Review agent: GPT-5.5 Codex, reviewer role.
- Status: Needs changes.
- Findings: A real Ctrl+click event sequence sends button-down and button-up before click, allowing the editor selection handler to change state before opening the link.
- Fixes or waivers: Link-targeted Ctrl button-down/up events are now consumed without changing editor state; the final click opens the URI. A regression test covers the full sequence.

## Review Round 2
- Review agent: GPT-5.5 Codex, reviewer role.
- Status: Clean.
- Findings: None.
- Fixes or waivers: None.

## Final Audit
- Done auditor: GPT-5.5 Codex, done-auditor role.
- Status: Complete.
- Plan items confirmed: Existing schemes and Markdown/bare links share one parser; all primary single-pane views use the clickable rendering path; Notes Preview uses normal click; Notes Editor uses Ctrl+click; wrapping, scrolling, gutters, ANSI formatting, Unicode width, and unrelated mouse behavior are covered; documentation is updated.
- Tests and checks confirmed: Full Go tests, vet, build, and diff validation passed.
- Waivers or skipped checks: Manual terminal/browser verification was not run because it would launch external handlers; URI opening is covered through the injectable opener in automated tests.
