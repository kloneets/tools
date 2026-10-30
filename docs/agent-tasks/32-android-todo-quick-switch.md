# Android Todo Quick List Switch

## Task
- Request: Make switching Todo lists quick on Android.
- Date: 2026-10-30

## Accepted Plan
- Add a horizontally scrollable row of list buttons above the Todo input when multiple active lists exist.
- Highlight the current list and scroll it into view after layout. Use themed colors, accessible labels, 48dp button height, and ellipsis for long names.
- Switch immediately through existing `selectTodoList`, retaining its draft/search reset and persisted selection behavior. Tapping the current list does nothing.
- Keep list management in the actions menu; no storage, sync, or desktop changes.
- Add instrumentation coverage for switching, selection persistence, current-list draft preservation, search clearing, and single-list visibility. Run isolated unit tests, lint, instrumentation compilation, and connected tests when available.

## Implementation Notes
- Added `addTodoListSwitcher` to MainActivity and a resource ID for the row.
- Added two MainActivity instrumentation tests.
- Model: inherited Codex model; preferred GPT-5.5 unavailable. Primary agent performed planning/coding; independent reviewer and auditor use inherited fallback.
- Plan deviations: None.
- Verification: `:app:testIsolatedUnitTest`, `:app:lintIsolated`, and `:app:compileIsolatedAndroidTestKotlin` passed. Both new tests passed on connected RMO-NX1 (Android 13), including a rerun after the review improvement. `git diff --check` passed.

## Review Round 1
- Reviewer: `quick_switch_review`.
- Status: clean.
- Findings: No implementation defects. Suggested re-fetching the switcher after opening search in the test, because the screen is rebuilt.
- Resolution: Test now taps the visible switcher; connected tests passed again.

## Final Audit
- Auditor: `quick_switch_audit`.
- Status: complete.
- Confirmed: All plan items implemented, independent review clean, test improvement applied, final connected rerun passed.
- Waivers: None. Go tests/build not repeated because this change touches only Android UI/resources/tests.
