# Note list fuzzy search

## Task
- Request: Search nested notes, expand matching tree paths, hide unrelated titles, and support fuzzy search in the TUI note list.
- Date: 2026-11-10
- Base: main, 34da654.

## Accepted Plan
1. Preserve `/`, Enter, Escape, `n`/`N`, paste, and Unicode backspace behavior. Match note titles, retain only matching notes and ancestor folders, and show the ordinary list for an empty query.
2. Match case-insensitive Unicode ordered subsequences; preserve contiguous highlighting when available and otherwise highlight the matched characters. Preserve tree order.
3. Search the complete browser inventory, including collapsed descendants. Expand matching paths during search and restore ordinary collapse choices when clearing.
4. Filter the open-note list as well. Use consistent visible indices for rendering, viewport, keyboard, mouse, opening, and commands. Preserve selection identity when available.
5. Centralize update/reset handling. Keep rename/move targeted at the selected result after clearing the search.
6. Cover nested matches, unrelated entries, expansion/restoration, fuzzy and Unicode matches, navigation, no matches, paste/backspace, filtered opening/rename, and mouse scrolling with automated tests.
7. Update Notes documentation and sidebar keyboard help.
8. Format Go, run focused and full tests, build, review independently, fix findings, and audit completion.

## Implementation Notes
- Planner: separate planner agent. Coding: separate coding agent.
- Model substitution: GPT-5.5 is unavailable; coding, review, and audit use the strongest available inherited model. Planning also uses the inherited available model.
- Search materializes filtered `Tree` and `BrowserTree` entries so existing input/rendering paths use the same visible indices. Search state keeps the ordinary lists; the browser caches its complete inventory before collapsed-note suppression.
- Rename/move from a nested result expands the ordinary target path when the command starts, preserving its selected target after the filter is cleared.
- Plan deviations: none.
- Files: `src/notes/sidebar_search.go`, `src/notes/sidebar_search_test.go`, `src/notes/tui_model.go`, `src/notes/paste.go`, `src/app/tui.go`, `README.md`, and this record.
- Checks: `gofmt` completed; `go test ./src/notes ./src/app` passed; final `go test ./...` passed; final `go build -o /tmp/koko-tools` passed; `git diff --check` passed. Initial sandbox attempts could not write Go caches; checks subsequently ran successfully with approved access.

## Review Round 1
- Reviewer: separate review agent.
- Status: needs changes.
- Finding: confirmed browser search followed by `h` bypassed reset, retaining the filter across mode exit.
- Fix: clear search before leaving browser mode; regression checks ordinary tree/collapse restoration.
- Finding: `SelectSidebarRow` ignored viewport start, selecting an earlier result after scrolling.
- Fix: include the visible range start; regressions check both browser and open-note filtered lists after scrolling.
- Waivers: none.

## Review Round 2
- Reviewer: separate review agent.
- Finding: down/click/double-click could shift the viewport between events and select different results.
- Fix: retain separate browser/open-note viewport starts; scroll only when selection leaves the visible range and clamp on list changes. Tests simulate down, redraw, click, redraw, and double-click in both modes with real note files.
- Status: clean. All findings fixed; no waivers.

## Final Audit
- Done auditor: separate planner agent reassigned to the done-auditor role.
- Status: complete. All accepted plan items confirmed against final code, tests, documentation, clean review, and successful verification.
- Tests/checks considered: final go test ./..., go build -o /tmp/koko-tools, gofmt, and git diff --check. No skipped required checks.
- Waivers: none.
