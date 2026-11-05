# Terminal paste

## Task
- Request: Fast terminal paste preserving newlines, tabs, spaces, and bullets.
- Date: 2026-11-05.

## Accepted Plan
Enable bracketed paste and route a complete paste to a dedicated application handler, bypassing command keys and per-character redraws. Add Workspace.HandlePaste for literal, atomic insertion with CRLF/CR normalized to LF and all other whitespace/Unicode retained. Support Insert and Normal modes without changing mode, replace selections, mirror insertion at multiple cursors, and provide one Undo/Redo transaction. Respect active input fields, sidebars and modals; retain Vim p behavior and existing storage/sync formats. Verify model behavior, simulated terminal events, large paste benchmark, full tests and build.

## Implementation Notes
- Coding agent: inherited latest available model; preferred GPT-5.5 is unavailable, so inherited strongest available fallback used.
- Added a primitive wrapper and EnablePaste(true), a complete-block model insertion API, context-aware app routing, model and terminal simulation regression tests and benchmarks.
- Files: src/app/tui.go, src/app/paste.go, src/app/paste_test.go, src/notes/paste.go, src/notes/paste_test.go, src/notes/tui_model.go, this record.
- Fields restricted to one line map line breaks/tabs to spaces; numeric fields retain existing digit restrictions. Pages numeric input uses its existing handler without per-character refresh.
- Checks: GOCACHE=/tmp/koko-paste-go-cache go test ./... passed (escalated for local HTTP test servers); go build -o /tmp/koko-tools-paste passed; git diff --check passed. Default Go cache initially falsely reported missing testing stdlib; clean cache resolves it.
- BenchmarkHandlePaste: 10,000 Unicode/bullet/tab lines, one iteration ~16.8 ms; diagnostic result, no timing assertion.
- Browser manual paste not run: no interactive terminal/browser integration in this execution; simulated terminal paste covers begin/content/end path.

## Review Round 1
- Independent reviewer /root/implement/review: clean after all three findings were fixed; see resolution below.

## Final Audit
- Done auditor: /root/implement/audit; inherited strongest available fallback because GPT-5.5 is unavailable.
- Status: complete with waivers.
- Confirmed: bracketed block paste, LF/CRLF/CR normalization, exact whitespace and Unicode, Normal/Insert contexts, selection and multicursor/padding replacement, atomic Undo/Redo including subsequent typing, field/modal/sidebar routing, unchanged Vim p and storage/sync behavior.
- Confirmed: full Go suite, build, paste regression tests, screen-event simulation, benchmark and focused race tests passed.
- Waivers: manual browser paste not performed because no interactive terminal/browser integration is available; automated screen-event simulation covers the path. Optional broad race run reveals unrelated existing background Firebase/settings races; fixing them is outside this change. No paste-focused race failures.

## Review Resolution
- Independent reviewer: /root/implement/review (inherited strongest available fallback; GPT-5.5 unavailable).
- Final status: clean.
- Fixed: tview drops raw LF in its collector; added paste-only screen event adaptation and CRLF coalescing, with real screen event simulation using LF/CRLF/CR.
- Fixed: paste during replacement confirmation could invalidate pending offsets; ignore it, with regression test.
- Fixed: synthetic whitespace Undo snapshots could cross a paste and delete original text; after a paste that editor session uses actual snapshots for whitespace boundaries. This intentionally trades synthetic word/sentence grouping for safe transaction boundaries.
- Added boundary-position, empty-paste, block-padding and typing-after-paste tests.
- Final full suite and build rerun after review fixes: passed. Paste-focused race tests passed (`go test -race ./src/app ./src/notes -run 'Paste|TerminalPaste'`). Broader optional package race run exposes unrelated existing background Firebase/theme/settings races (e.g. TestSwitchAppTabSameViewDoesNotSync); not changed in this scoped task. Diagnostic log: /tmp/koko-all-race.log.
