# TUI Tab Keyboard Navigation

## Request and accepted plan
- Date: 2026-10-30.
- Make the Notes tab bar and open-tabs picker usable over SSH/tmux without intercepting typed `<`, `>`, or `N`.
- Add normal-mode sequences `g<` / `g>` to scroll left/right, and `gN` to open the existing picker. Preserve plain indentation/search keys and all insert/command/visual text routing.
- Picker keeps arrows/Enter/Escape behavior. Add on-screen help and end-to-end key-routing tests. No terminal modifier sequences, storage, or sync changes.

## Implementation
- Added normal-mode `g` prefix routing and a one-shot pending picker action consumed by the app.
- Added normal-mode help entries and a test sending plain terminal rune events through application input capture, including picker navigation and literal `<,>,Ng<g>gN` insertion.
- Built repository `koko-tools` binary for local testing.
- Preferred GPT-5.5 unavailable; inherited model fallback used for coding/review/audit.

## Verification
- `GOCACHE=/tmp/koko-tools-gocache go test ./...` passed.
- `GOCACHE=/tmp/koko-tools-gocache go build -o koko-tools .` passed.
- `git diff --check` passed.

## Review and audit
- Independent reviewer: `tabs_review`; clean. Normal-only routing, plain keys, picker isolation and focused tests confirmed. No waivers.
- Done auditor: `tabs_audit`; complete. Plan, implementation, review and final verification confirmed.
