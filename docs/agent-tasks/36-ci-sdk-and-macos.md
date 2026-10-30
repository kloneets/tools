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
- Independent review `ci_review` clean after icon corrections.
- PR #1 run 41033142867: Android, Apple Silicon build (including native icon packaging tests), and benchmarks passed. Go failed in a previously timing-sensitive async spell test: a retained native dictionary invoked global `spellRunCommand` after a later test set the hook to nil.
- Plan follow-up: capture command runner per native dictionary and add deterministic checks/suggestions hook-isolation regression coverage, then repeat local and remote tests.
- Follow-up fix: native dictionaries retain their initialized command runner for both checks and suggestions. Regression tests cover nil/replacement hooks for both native backends. Notes suite passed 30 repetitions; full Go suite, vet and build passed. Independent follow-up review is clean; no waivers.
- PR: https://github.com/kloneets/tools/pull/1. Final remote CI verification remains the completion gate; its result is recorded by the PR checks.
