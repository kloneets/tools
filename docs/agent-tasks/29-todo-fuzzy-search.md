# Agent Task Record

## Task
- Request: Fuzzy search within the open Todo list on TUI and Android.
- Date: 2026-09-27.
- Branch: Existing working tree; preserve task 28's uncommitted cursor fixes.

## Accepted Plan
- Implement the user-provided, previously accepted plan without replanning.
- Case-insensitive, ordered Unicode character subsequence matching, choosing earliest positions; punctuation is literal. Search task text only in the current list, active and locally loaded archived tasks, including collapsed months. Never fetch archives for search.
- Preserve task order. Each matching task is one result. Highlight letters, show current/total or “No matches”, clear highlights for empty queries, wrap previous/next navigation, expand and scroll to selected results.
- Keep search in memory through Notes/Todo navigation; clear on list changes. Recompute after mutation, archive loading, or sync, retaining the result by task ID when possible.
- Label scope “Current list · active + loaded archive.”
- TUI: `/` forward, `?` backward, live query highlights, Enter confirms, contextual `n`/`N`, Esc cancels editing or clears confirmed search. Consume query input before shortcuts; support Unicode/backspace. Add viewport scrolling and correct mouse hit testing; archived results remain read-only.
- Android: Todo actions Search entry and find bar with query/count/previous/next/close. Theme-based highlights and current-result distinction. Preserve query view, keyboard focus, cursor and Todo draft while updating. Scroll after layout; closing clears highlights.
- Equivalent pure Go/Kotlin match helpers with rune/UTF-16 offsets. Tests cover matching, Unicode/repeats/punctuation, list isolation, loaded collapsed archives, wrapping/no results/mutations, TUI routing/scrolling/mouse/cursor, Android find/navigation/draft/cursor/list/update behavior.
- Run Go tests/build; Android isolated unit tests, lint, instrumentation compilation and connected instrumentation when available. Complete independent review and done audit.
- No storage or sync format changes.

## Implementation Notes
- Coding agents: `/root/tui` and `/root/android`, using repository role instructions.
- Model selection: GPT-5.5 is unavailable; the strongest available inherited model is used as fallback for coding and subsequent review/audit. The supplied planner output is the accepted plan.
- Summary: Added pure Unicode match helpers and in-memory search state on both platforms. TUI adds bottom search bar, contextual key handling, archive result selection, viewport scrolling and translated mouse rows. Android adds a persistent find bar, styled matches and result scrolling while preserving draft and query editor state. Final audit scope finding fixed: completed tasks remain visible on TUI, while search candidates and highlights follow the plan and include only active plus loaded archived tasks.
- Files changed: `src/todo/search.go`, `src/todo/search_test.go`, `src/app/tui.go`, `src/app/tui_todo_selection.go`, `src/app/tui_todo_search.go`, `src/app/tui_todo_search_test.go`, Android `MainActivity.kt`, `TodoSearch.kt`, `TodoSearchTest.kt`, `MainActivityTest.kt`, `res/values/ids.xml`, and this record. Earlier cursor task modifications remain preserved.
- Tests and checks:
  - Android `:app:testIsolatedUnitTest :app:lintIsolated :app:compileIsolatedAndroidTestKotlin`: passed, including recheck after review fixes and added instrumentation cases.
  - `adb devices -l`: no attached device; connected instrumentation cannot run. Instrumentation sources compile, but on-device runtime remains unverified.
  - `git diff --check`: passed.
  - `go test ./...`: passed.
  - `go build -o /tmp/koko-tools-search-check ./`: passed.
  - After final audit scope fix, `go test ./src/app ./src/todo`: passed; full `go test ./...` passed with loopback access for the sync package; `go build` and `git diff --check`: passed.
  - An initial sandboxed Go invocation could not run the network-dependent tests; a combined escalation waited for approval and was aborted. Direct approved Go commands then passed.
- Plan deviations: None.

## Review Round 1
- Reviewer: `/root/review`.
- Status: Changes requested; all findings subsequently fixed.
- Findings and resolutions:
  - Suppress deferred Android Todo draft focus restoration while search is open; fixed and regression added for focus after idle.
  - Avoid restoring stale scroll after sync when a selected search result is being revealed; fixed.
  - TUI retained result IDs could become detached from actionable row selection after task reordering; added order tracking and selection realignment with completion coverage.
  - Include archive month headers in order tracking so newly synced empty months cannot shift the actionable selected row; fixed with regression coverage.
  - Expanded Android instrumentation coverage for loaded archive reveal, mutation refresh, list switching and focus stability.

## Review Round 2
- Reviewer: `/root/review`.
- Status: Clean.
- All correctness findings and coverage requests resolved; final header-order regression inspected. Existing platform display behavior is preserved: Android displays active tasks and archive, while TUI also has its preexisting Done section.
- Residual verification limitation: no device for connected Android instrumentation.

## Final Audit
- Done auditor: `/root/audit` (initial audit reported the now-fixed scope mismatch; follow-up pending).
- Status: Complete with waivers.
- Finding resolution: Completed-but-unarchived tasks were included by TUI search despite the active + loaded archive contract and Android behavior. Fixed by removing Done items from TUI candidates and search highlights; regression confirms selection advances to the next matching active item after completion.
- Plan check: Search scope now matches on both platforms: active tasks plus locally loaded archive. Search state, navigation, loaded archive reveal, highlighting, list isolation, mutation refresh, viewport behavior, and editor/draft preservation are implemented and covered by the recorded tests.
- Verification: Review round 2 is clean. The task record reports focused and full Go tests, Go build, diff check, Android isolated unit tests, lint, and instrumentation compilation passed after applicable changes.
- Waiver: Connected Android instrumentation could not run because no device was attached; Android instrumentation sources compile. No other plan deviations or unresolved review findings remain.
