# Desktop Hunspell Library Removal

## Task
- Request: Remove the Go Hunspell library if Android does not depend on it; keep native desktop spell checking.
- Date: 2026-10-13
- Branch or commit: Current working branch

## Accepted Plan
- Remove `github.com/shuLhan/share/lib/hunspell` from the desktop/TUI spell service and Go module dependencies.
- Keep desktop native `nuspell` and `hunspell` command support and the lightweight `.dic` fallback.
- Leave Android's separately bundled C++ Hunspell engine and JNI bridge unchanged.
- Update tests and README wording; run Go tests and build.

## Implementation Notes
- Coding agent: Codex primary agent (GPT-6.1-sol; separate role agents were unavailable under the active collaboration instruction).
- Summary: Removed the Go Hunspell parser fallback. Desktop dictionary status and spell checks now use native commands when available and the existing simple `.dic` fallback otherwise. Documented Android's independent bundled Hunspell engine.
- Files changed: `src/notes/spell.go`, `src/notes/spell_test.go`, `go.mod`, `go.sum`, `README.md`.
- Plan deviations: None.
- Tests and checks run: `GOCACHE=/tmp/koko-tools-gocache go test ./...` passed; `GOCACHE=/tmp/koko-tools-gocache go build -o /tmp/koko-tools-check .` passed. The first sandboxed full-suite attempt could not bind loopback for sync tests; the approved elevated rerun passed.

## Review Round 1
- Reviewer: Codex primary agent (manual diff review; separate role agents unavailable under active collaboration instruction).
- Status: clean.
- Findings: Verified no Go Hunspell imports or dependency remain; native spell checker detection remains unchanged; `.dic` fallback remains used by status and spell service; Android C++ sources were not modified.
- Fixes or waivers: None.

## Review Round 2
- Reviewer: Not needed; round 1 found no findings.
- Status: clean.
- Findings: None.
- Fixes or waivers: None.

## Final Audit
- Done auditor: Codex primary agent (manual audit; separate role agents unavailable under active collaboration instruction).
- Status: complete.
- Plan items confirmed: Go library and checksums removed; native desktop engines and `.dic` fallback preserved; Android Hunspell implementation unchanged; docs/tests updated.
- Tests and checks confirmed: Full Go suite and Go build passed.
- Waivers or skipped checks: None.
