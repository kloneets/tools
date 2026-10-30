# TUI Note Tab Scrolling

## Task and accepted plan
- Request: Implement the accepted single-row scrollable Notes tab bar, keeping active notes visible with many tabs open.
- Date: 2026-10-30.
- Horizontal viewport with clickable left/right indicators and mouse-wheel scrolling over the tab row. Manual scrolling leaves the active editor and its cursor unchanged.
- Reveal the active tab on navigation, resize, tab mutations, and title/dirty-label changes. Truncate oversized titles while retaining the close target.
- Add a count button opening a themed, scrollable list of every open note; Enter/click selects and Esc cancels.
- Use terminal cell widths for rendering and hit targets, including wide Unicode titles. Keep state in memory; no storage or sync changes.
- Verify with automated Notes/app tests, the full Go suite, and a Go build.

## Implementation
- Primary agent: tab viewport, layout/cache, cell targets, model integration, tests.
- `tabs_app`: app mouse routing, picker, pane-width correction, app tests.
- Preferred GPT-5.5 unavailable; agents used inherited model fallback.
- Files: `src/notes/tab_bar.go`, `tab_bar_test.go`, `tui_model.go`, `tui_model_test.go`; `src/app/tui_note_tabs.go`, `tui_note_tabs_test.go`, `tui.go`.
- Existing ASCII tree-marker test now checks sidebar rows rather than the whole editor, which intentionally includes a Unicode picker indicator.
- No unrelated Android release changes altered.

## Review
- Independent reviewer: `tabs_review`.
- Finding: Active tab could disappear after the dirty marker or a renamed title increased its width.
- Fix: Detect label changes and reveal active tab; added dirty/rename regression cases.
- Final status: clean after independent re-review; no remaining findings or waivers.

## Verification and audit
- Notes/app tests passed. Final `GOCACHE=/tmp/koko-tools-gocache go test ./...`, `go build -o /tmp/koko-tools-tabbar .`, and `git diff --check` passed after the review fix.
- Independent done auditor `tabs_audit`: complete. Viewport, active visibility, Unicode targets, close controls, picker, review fix, and final verification confirmed. No waivers.
