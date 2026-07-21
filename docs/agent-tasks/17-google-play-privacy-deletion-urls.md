# Agent Task Record

## Task
- Request: Implement accepted Google Play privacy policy and account deletion URL plan, preserving existing uncommitted API36 changes.
- Date: 2026-07-17
- Branch or commit: working tree

## Accepted Plan
- Update public and repository privacy/account deletion docs to use canonical URLs:
  - `https://koko.lv/koko-tools/privacy-policy.html`
  - `https://koko.lv/koko-tools/account-deletion.html`
- Use private support email `janis@xit.lv` for privacy and deletion requests.
- Document Firebase Auth plus workspace notes, todos, settings, and sync metadata as the cloud data scope.
- Require deletion requests to come from the Firebase login email address and describe private verification.
- Tell users not to include passwords, notes, todos, or other private content.
- Document deletion timing: acknowledge requests within 7 calendar days and complete verified deletion requests within 30 days.
- Document retention: deleted content is not retained; minimal correspondence and security audit records may be retained for up to 90 days as needed.
- Update Android policy URL constants only.
- Preserve API36 release documentation additions while updating policy URLs.
- Remove stale managed-assets feature claims from owned docs.
- Add a table-driven Go test checking canonical policy/deletion URLs and stale root URLs in active files.
- Run tests/checks and record model and results.

## Implementation Notes
- Coding agent: Noether (`gpt-5.5`).
- Summary: Updated public and repository privacy/account deletion docs to the canonical `/koko-tools/` URLs, replaced public issue-based deletion requests with private email verification through `janis@xit.lv`, documented the requested Firebase data scope, deletion timing, and retention terms, updated Android policy URL constants only, preserved existing API36 release additions while changing policy URLs, removed stale managed-assets feature claims from owned docs, and added a table-driven URL regression test.
- Files changed:
  - `docs/privacy-policy.html`
  - `docs/account-deletion.html`
  - `PRIVACY.md`
  - `ACCOUNT_DELETION.md`
  - `android/app/src/main/java/com/kloneets/kokotools/MainActivity.kt`
  - `android/RELEASE.md`
  - `android/PLAY_STORE_PUBLISHING.md`
  - `README.md`
  - `policy_urls_test.go`
  - `docs/agent-tasks/17-google-play-privacy-deletion-urls.md`
- Plan deviations: None.
- Tests and checks run:
  - `gofmt -w policy_urls_test.go` completed.
  - `go test ./...` passed.
  - `./gradlew :app:testDebugUnitTest` passed with existing Google sign-in and Activity Result API deprecation warnings.
  - `./gradlew :app:assembleDebug` passed.
  - `git diff --check` passed.
  - `rg -n "https://koko\\.lv/(privacy-policy|account-deletion)\\.html|managed assets|Managed file assets|GitHub issue|open an issue|open a GitHub issue" docs/privacy-policy.html docs/account-deletion.html PRIVACY.md ACCOUNT_DELETION.md android/RELEASE.md android/PLAY_STORE_PUBLISHING.md README.md android/app/src/main/java/com/kloneets/kokotools/MainActivity.kt` returned no matches.

## Review Round 1
- Review agent: Beauvoir (`gpt-5.5`).
- Status: Clean in repository; external deployment required.
- Findings: The canonical live URLs still serve the previous May 27 pages because the updated HTML files have not yet been uploaded.
- Fixes or waivers: Live deployment is explicitly assigned to the user and is outside repository write access. The repository pages, links, app route, and tests are complete. The task remains incomplete operationally until `docs/privacy-policy.html` and `docs/account-deletion.html` are uploaded to `https://koko.lv/koko-tools/` and the live content is verified.

## Review Round 2
- Review agent: Release follow-up.
- Status: Clean; final live pages verified.
- Findings: Release review added Android backup/OEM migration wording after the initial July 17 upload.
- Fixes or waivers: The latest privacy policy and account deletion pages were verified live on July 21, 2026.

## Final Audit
- Done auditor: Hegel (`gpt-5.5`).
- Status: Complete.
- Plan items confirmed: Canonical URLs, private deletion workflow, deletion scope/timing/retention, Android Settings destinations, stale feature removal, URL regression coverage, and preservation of task 16 API 36 work.
- Tests and checks confirmed: `go test ./...`, Android unit tests, Android debug assembly, stale-text scan, and `git diff --check` passed.
- Waivers or skipped checks: None. Final privacy policy and account deletion pages were verified live on July 21, 2026.
