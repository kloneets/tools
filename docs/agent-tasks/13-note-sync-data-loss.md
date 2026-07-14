# Agent Task Record

## Task
- Request: Diagnose and fix note sync data loss for `Home/uz-Bebreni.md`, and restore content when possible.
- Date: 2026-07-14
- Branch or commit: `main`

## Incident Facts
- Android and TUI local copies are empty.
- Android was stopped during investigation to prevent further writes.
- The TUI state records an empty-content remote revision at `2026-07-14 20:01:05 EEST`; TUI pulled it at `20:04:07`.
- Current Firebase note storage is last-write-wins and contains no mutation history. Existing local note writes make no durable backup.
- Latest-model planning was attempted with GPT-5.6 but the planner did not return; Codex completed the incident plan to avoid delaying recovery.

## Accepted Plan
- Add durable local note version backups before overwrites/deletes on Android and TUI, stored outside the live notes tree so they never sync as notes.
- Add Firebase `note_versions` history: before replacing a current note record, preserve the previous remote record keyed by note ID and revision. History failure must block the destructive overwrite rather than silently lose the recovery point.
- Remove unsafe blanket note pushes immediately after Android pull; only explicit local saves/new notes/deletes push individual mutations.
- Preserve dirty/conflict behavior and never rewrite markdown content as part of migration.
- Add recovery helpers/tests and update Firebase security rules for authenticated owner/editor history writes and member reads.
- Search Firebase backups/PITR and existing Drive/local/device copies for the lost content; restore only from a verified non-empty version, then push normally.

## Implementation Notes
- Coding agents: Pauli (Android, GPT-5.6), Dalton (Go/TUI/shared sync, GPT-5.6), integrated by Codex.
- Summary: Added fail-closed local note backups outside live note trees, bounded recovery APIs, Firebase note history with ETag conditional replacement, and removed Android blanket pushes after pull.
- Files changed: Android note/sync repositories and tests; Go note backup, TUI mutation paths, Firebase provider/rules, and tests.
- Plan deviations: The latest-model planner did not return, so Codex finalized the conservative incident plan.
- Tests and checks run: `GOCACHE=/tmp/koko-go-cache go test ./...` and `./gradlew :app:testDebugUnitTest :app:lintDebug :app:assembleDebug` passed; `git diff --check` passed.

## Review
- Review agent: Lorentz (GPT-5.4 fallback after GPT-5.6 and GPT-5.5 reviewers failed to return).
- Status: Clean after round 2.
- Findings and resolutions: Android allowed an unconditional overwrite when Firebase omitted an ETag, and local delete failure could still trigger a remote tombstone/editor clear. Missing/blank ETags now abort, local delete failures throw, and remote tombstone/editor clearing is gated on confirmed deletion. Regression tests cover both findings.

## Final Audit
- Done auditor: Boole (GPT-5.4 fallback after higher-model agents repeatedly failed to return).
- Status: Complete. The reviewed Realtime Database rules were deployed to `koko-tools-default-rtdb` and read back byte-for-byte against `src/sync/security_rules.json`.
- Recovery result: Android cloud backup set `35df7820e616e1e5` restored a verified 119-byte non-empty copy. It was extracted to `/tmp/uz-Bebreni-recovered.md`, restored to TUI and Android, and written directly to only the matching Firebase note record. All three copies match SHA-256 `d674e12d67d319d769fc202d387b977299533ad16354cab2a6517ded49ceca6e`. Android Firebase was temporarily disabled to prevent the production build from pulling the former empty record during recovery.
- Waivers: None. Android sync remains disabled in the installed old production build as a safety measure; it must only be re-enabled after installing a release that contains this fix.
