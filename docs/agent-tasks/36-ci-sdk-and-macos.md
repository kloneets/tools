# CI SDK and macOS Setup Repair

## Accepted plan
- Date: 2026-10-30. Request: investigate failing GitHub CI and fix it.
- Evidence: run 41026217438 failed Android setup because SDK package `tools` is unavailable, and macOS Homebrew setup because Python 3.15 failed to build on macos-14. Latest run 41030760382 reproduced the Android failure; Linux tests/vet passed.
- Explicitly install only SDK `platform-tools`; remove obsolete GTK/Homebrew/pkg-config setup from TUI builds; use supported macos-15 Apple Silicon runner; fix stale benchmark target `src/gdrive`.
- Add standalone/bundle packaging regression coverage. Verify Go, Android and workflow checks, then run GitHub CI on a PR.

## Implementation and review
- Changed workflow, macOS packaging script, and added `macos_build_test.go`.
- Independent `ci_review` found three incorrect icon dimensions in the old macOS script. Fixed 128, 256 and 1024 pixel outputs. Added native icon artifact assertion and packaging tests to macOS CI.
- Preferred GPT-5.5 unavailable; inherited agent model fallback used.
- Local checks passed: full Go tests, vet, isolated Android unit tests, debug lint, actionlint v1.7.7, shell syntax, packaging tests, diff check.
- Native macOS tools are unavailable on this Linux host; PR CI verifies native icon generation and Apple Silicon build.
- Final review/audit and remote CI pending.
