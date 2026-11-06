# TUI input, note list search, and cursor line

## Task
- Request: Fix macOS Shift+Space, add `/` Note List search, highlight the editor cursor line, test and publish.
- Date: 2026-11-06.
- Target: main and its CI macOS artifact. No separate GitHub release or local binary replacement.

## Accepted plan
- Decode CSI-u Shift+Space and modifyOtherKeys Shift+Space as exactly one normal space in TUI and raw input; preserve literal text and existing shortcuts.
- `/` starts live, case-insensitive substring search of visible note names in the current sidebar/browser. Highlight matches, select the first match, do not filter or expand folders. Enter confirms, n/N wrap, Escape finishes typing then clears. Normal Enter opens the selected note. Search is in memory and cannot activate global text shortcuts while typing.
- Highlight the cursor's entire logical editor line including wrapped segments, gutter and padding using a subtle themed background. Keep syntax, search and selection readable; preview unchanged.
- Update help and automated coverage. Run Go tests, vet, build and diff checks; visually verify TUI. Review independently, fix findings, audit, push main, verify CI/macOS artifact.
- No storage, sync, Android or public API changes.

## Implementation notes
- Primary coder: cursor-line rendering, theme integration, app help, integration and release.
- Delegated coders: note_search (sidebar search), shift_space (terminal sequence handling).
- Model substitution: preferred GPT-5.5 is unavailable; coding/review/audit use the available inherited model fallback. Planning used the session's latest available model.
- Implementation: added focused sidebar search state/rendering and tests; extended raw and tcell input normalization including actual Alt+[ events; added semantic cursor-line ANSI role with per-theme background restoration and terminal-cell tests.
- Files: src/app/{tui.go,theme.go,raw_unix.go,raw_unix_test.go,tui_note_tabs_test.go,shift_space_test.go,cursor_line_test.go,sidebar_search_test.go}; src/helpers/ansi.go; src/notes/{tui_model.go,paste.go,sidebar_search.go,sidebar_search_test.go,cursor_line.go,cursor_line_test.go}.
- Checks passed: go test ./...; go vet ./...; go build -o /tmp/koko-tools-tui-fixes .; git diff --check; targeted go test -race ./src/app ./src/notes -run 'Test(ShiftSpace|InputSequence|IncompleteInput|DecodedCtrl|ConsecutiveEscapes|SidebarSearch|CursorLine|ParseCSI)'.
- PTY smoke: isolated HOME under /tmp, real tmux/tcell app. Both CSI-u and modifyOtherKeys Shift+Space produced `abc def X`; sidebar `/Note` showed one match and n repeated; captured ANSI showed full cursor-row background distinct from adjacent rows. Simulation test verifies exact backgrounds and no next-row leakage.
- Full race suite fails in existing sync/settings and spelling goroutines. The unchanged HEAD exported into /tmp/koko-tui-baseline fails with the same concurrency paths. Logs: /tmp/koko-tui-race.log and /tmp/koko-tui-baseline-race.log. Explicit waiver: unrelated pre-existing concurrency defects are deferred; targeted changed-feature race tests pass.
- Physical macOS keyboard validation is unavailable on this Linux host; protocol/PTTY tests cover input and macOS CI will run native tests/build. No claim of physical Mac testing.
- Plan deviations: none; actual tcell Alt+[ event support was essential to the accepted Shift+Space behavior.

## Review
- Independent reviewer: review.
- Round 1: needs changes. Search hid browser command prompt; decoded Ctrl events bypassed modal routing; consecutive Escape keys collapsed.
- Fixes: clear search on starting browser command, redispatch all decoded events through normal modal/help routing, flush pending Escape before buffering the next. Regression tests added for each.
- PTY verification additionally identified tcell Alt+[ collapsing; decoder now handles both shapes, with actual-event regressions and successful PTY retest.
- Final review: clean after re-review, including Alt+[ support. No unresolved code findings.

## Final audit and publication
- Independent done-auditor: audit confirmed code readiness and local checks; no remaining code findings. Overall status remains incomplete until publication verification below.
- Publication: pending main push and native macOS CI artifact verification.
