# TUI Todo List Label Clicks

## Task
- Request: Make the inline Todo list labels in the main TUI pane clickable so labels like `[Default]` and `Yob` switch the active Todo list scope.
- Date: 2026-09-04
- Branch or commit: current worktree

## Accepted Plan
Planner: `/root/planner_todo_list_title_clicks`.

- Record click hit regions while rendering the inline `lists: ...` summary in the Todo pane.
- Treat each visible list label, including the current bracketed label, as clickable.
- Preserve normal `MouseLeftDown` text-selection setup.
- On `MouseLeftUp`, after updating selection, switch Todo list only when there was no visible drag selection and release is on an inline list label.
- Retain `MouseLeftClick` handling as a fallback for synthesized click-only paths.
- Reuse the existing Todo list selection path so storage, settings, visible todos, archive state, and Firebase list scope update consistently.
- Keep display/storage behavior backwards-compatible and avoid changing Todo list serialization.
- Preserve existing drag-to-copy behavior, sidebar list clicks, and todo/archive row clicks.
- Clear stale inline list click regions when the list summary row is hidden or replaced by Todo list input.
- Add focused automated regression tests for down/up clicks, drags that start on labels, inert text, bracket boundaries, and wide Unicode coordinates.

## Implementation Notes
- Coding agent: `/root/coder_todo_list_title_clicks` (GPT-5.5).
- Summary: Added renderer-derived inline Todo-list click regions, handled them in the Todo pane mouse dispatcher after no-drag `MouseLeftUp` and before task-row click fallback, and covered current/non-current label clicks plus stale-region clearing. Follow-up fix: label activation was moved off `MouseLeftDown` so text selection can start normally on a list label; drags copy without switching lists.
- Files changed: `src/app/tui.go`, `src/app/tui_test.go`, and this task record.
- Plan deviations: None.
- Tests and checks run:
  - `gofmt -w src/app/tui.go src/app/tui_test.go`
  - `go test ./src/app -run 'TestTodoMouseDownUpSelectsInlineTodoListLabel|TestTodoMouseDragStartingOnInlineTodoListLabelCopiesWithoutSwitching|TestTodoMouseInlineTodoListLabelHitTargetsExcludeInertText|TestTodoMouseInlineTodoListLabelWideUnicodeCoordinates|TestRenderTodoClearsInlineTodoListClickRegionsWhenHidden|TestTodoMouseClickSelectsTodoTitle|TestTodoMouseClickSelectsArchiveMonthTitle|TestTodoListMouseClickOpensVisibleList' -count=1`
  - `go test ./...`
  - `go build -o /tmp/koko-tools-inline-list-clicks`
  - `git diff --check`
  - `./gradlew :app:testDebugUnitTest` from `android/`
  - `./gradlew :app:assembleDebug` from `android/`

## Review Round 1
- Review agent: `/root/reviewer_todo_list_title_clicks_v2` (GPT-5.5).
- Status: Clean.
- Findings: None. Coverage includes no-drag down/up selection, synthesized-click fallback, drag-to-copy without switching, inert prefix/separator areas, bracket boundaries, wide Unicode coordinates, and stale-region clearing.
- Fixes or waivers: None.

## Review Round 2
- Review agent: not needed.
- Status: not run.
- Findings: None.
- Fixes or waivers: Round 1 was clean.

## Final Audit
- Done auditor: `/root/auditor_todo_list_title_clicks_fallback` (GPT-5.6 Luna fallback; the preferred GPT-5.5 auditor hit its usage limit).
- Status: Complete.
- Plan items confirmed: Renderer-derived label regions include current brackets; no-drag mouse release and synthesized-click fallback switch through `selectTodoList`; drag selection, inert separators, Unicode widths, stale-region clearing, sidebar clicks, and todo/archive title clicks are preserved.
- Tests and checks confirmed: Focused TUI regressions, full `go test ./...`, Go build, gofmt, `git diff --check`, Android unit tests, and Android debug build.
- Waivers or skipped checks: None. Model substitution was recorded as required by the workflow.
