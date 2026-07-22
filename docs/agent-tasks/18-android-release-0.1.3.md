# Agent Task Record

## Task
- Request: Build Android release 0.1.3 and promote it to Google Play Production while preserving task 16 and task 17 changes.
- Date: 2026-07-17
- Branch or commit: `e732b8f`

## Accepted Plan
- Preserve task 16 Android API 36 compatibility and task 17 privacy/account deletion work.
- Release `versionCode = 4` and `versionName = "0.1.3"` with version-specific Play notes.
- Verify the public policy pages, reviewer access, Play declarations, release tests, signing, version metadata, and 16 KB native compatibility.
- Review and commit the release candidate, upload the exact verified AAB through Android Publisher, set Production status to `completed` without a staged fraction, validate and commit the edit, then verify the resulting Production track.
- Abort the Play edit on pre-commit failure and never reuse or silently replace version code 4.
- Record checks, artifact hashes, Play responses, review, waivers, and final audit without committing generated binaries or credentials.

## Implementation Notes
- Coding agent: Maxwell (`gpt-5.5`).
- Release validation operator: Codex.
- Summary: Prepared Android release 0.1.3 metadata while preserving task 16 API 36 compatibility work and task 17 privacy/deletion URL work. Set release metadata to `versionCode = 4` and `versionName = "0.1.3"`, added version-code 4 Play release notes, refreshed Android release documentation, verified live policy pages, and captured release artifact evidence.
- Files changed:
  - `android/app/build.gradle.kts`
  - `android/README.md`
  - `android/app/src/main/AndroidManifest.xml`
  - `android/app/src/main/res/xml/full_backup_content.xml`
  - `android/app/src/main/res/xml/data_extraction_rules.xml`
  - `docs/privacy-policy.html`
  - `PRIVACY.md`
  - `policy_urls_test.go`
  - `android/play-store/release-notes/4-en-US.txt`
  - `android/PLAY_STORE_PUBLISHING.md`
  - `docs/agent-tasks/18-android-release-0.1.3.md`
- Plan deviations: Android Auto Backup was disabled during review, with explicit backup rule resources requesting exclusion of app-private notes, todos, settings, and token material from Android cloud backup and device-to-device transfer. Connected-device validation was unavailable because no Android device was connected.
- Review 2 fix summary: Kept `android:allowBackup="false"` and added both legacy `android:fullBackupContent` and API 31+ `android:dataExtractionRules` resources. The rule files exclude app data from legacy full backup, Android cloud backup, and device-to-device transfer across root, files, databases, shared preferences, external app files, and device-protected storage domains. Privacy wording now says the app requests exclusion from Android cloud backup and device-to-device transfer, notes that manufacturers may control migration behavior, and keeps Firebase sync separate.
- Tests and checks run:
  - `./gradlew :app:testDebugUnitTest` passed.
  - `./gradlew :app:assembleDebug` passed.
  - `./gradlew :app:assembleDebugAndroidTest` passed.
  - `./gradlew :app:lintDebug` passed.
  - `./gradlew :app:bundleRelease` passed.
  - `gofmt -w policy_urls_test.go` passed.
  - `go test ./...` passed.
  - Review 2 verification: `gofmt -w policy_urls_test.go` passed; `go test ./...` passed; `git diff` and `git diff --no-index /dev/null ...` were inspected for tracked and new files.
  - `go vet ./...` passed.
  - `go build -o /tmp/koko-tools-release-check .` passed.
  - `git diff --check` passed.
  - Final rebuilt AAB SHA-256: `c1d8d90362994afc911ef01342439391790ed59579018f564cf6b6c271716f76`.
  - `jarsigner -verify -verbose -certs android/app/build/outputs/bundle/release/app-release.aab` completed and reported signer `CN=Janis Rublevskis, OU=Unknown, O=Unknown, L=Carnikava, ST=Adazu nov., C=LV`, signature algorithm `SHA384withRSA`, `2048-bit RSA` key, valid through `2053-10-12`; the local trust store does not contain the certificate chain.
  - Release artifact evidence: `android/app/build/outputs/bundle/release/app-release.aab`, 14,939,830 bytes, timestamp `2026-07-17 18:11:47 +0300`.
  - Target SDK evidence: `android/app/build/intermediates/merged_manifests/release/processReleaseManifest/AndroidManifest.xml` contains `android:targetSdkVersion="36"`.
  - 16 KB native compatibility evidence: bundle metadata includes `PAGE_ALIGNMENT_16K`; extracted native libraries for `arm64-v8a` and `x86_64` report `0x4000` load alignment.
  - Final live policy verification on July 21, 2026 confirmed the privacy page contains the Android backup/OEM migration disclosure and the deletion page contains the private request path and 30-day completion statement.
  - Release candidate commit: `e732b8f release: prepare Android 0.1.3`.
  - Android Publisher edit `00491301437055390367` uploaded version code 4 with Play SHA-256 `c1d8d90362994afc911ef01342439391790ed59579018f564cf6b6c271716f76`, exactly matching the local signed AAB.
  - The Production track was set to release name `0.1.3`, version code 4, status `completed`, English release notes, and no staged rollout fraction. The edit validated and committed successfully.
  - A fresh readback edit confirmed Production version code 4, status `completed`, and no `userFraction`; the temporary edit was deleted with HTTP 204.
  - The read-only Production release endpoint reports version 4 lifecycle `RELEASE_LIFECYCLE_STATE_IN_REVIEW`; version 3 remains `RELEASE_LIFECYCLE_STATE_PUBLISHED` while Google reviews 0.1.3.

## Review Round 1
- Review agent: Sartre (`gpt-5.6-sol`).
- Status: Needs changes.
- Findings:
  - `android/app/src/main/AndroidManifest.xml` still enabled Android Auto Backup with `android:allowBackup="true"`.
  - `docs/privacy-policy.html` and `PRIVACY.md` did not disclose that Android system backup is disabled or that local app-private data remains on device unless optional Firebase sync is enabled.
  - `policy_urls_test.go` did not assert backup-disabled manifest behavior or backup disclosure text in the privacy docs.
  - `android/play-store/release-notes/4-en-US.txt` overstated the change as Play listing access instead of in-app privacy/deletion links, and `android/PLAY_STORE_PUBLISHING.md` still used stale initial-release prose.
- Fixes or waivers: Android Auto Backup is disabled and covered by the policy regression test; privacy docs disclose local-only behavior; release notes and publishing instructions now agree; this task record contains the release evidence. Connected-device testing remains waived because ADB reports no device.

## Review Round 2
- Review agent: Galileo (`gpt-5.6`)
- Status: Needs changes.
- Findings:
  - `android/app/src/main/AndroidManifest.xml` kept `allowBackup=false` but did not reference backup rule resources, leaving no explicit API 30-and-lower or API 31+ exclusion contract.
  - Backup rule resources were missing, so app-private data was not explicitly excluded from legacy full backup, Android cloud backup, or device-to-device transfer across files, shared preferences, databases, root, device-protected storage, and external app-file domains.
  - Privacy wording overstated Android backup behavior as absolute instead of saying the app requests exclusion, did not mention manufacturer-controlled migration behavior, and did not clearly separate optional Firebase sync from Android system backup.
  - `policy_urls_test.go` forbade manifest backup rule attributes instead of asserting both resources are referenced and that rule files exclude app data.
  - `android/PLAY_STORE_PUBLISHING.md` still described a staged rollout; release 0.1.3 must be a full Production rollout with no staged fraction.
  - Task evidence had stale artifact size/timestamp and did not record the 16 KB `PAGE_ALIGNMENT_16K` / `0x4000` `arm64-v8a` and `x86_64` evidence.
- Fixes or waivers: Added manifest rule references and both backup rule XML resources; updated privacy wording, policy tests, publishing guide rollout instructions, and release evidence. No waivers recorded for review2 findings.

## Review Round 3
- Review agent: Linnaeus (`gpt-5.6-sol`).
- Status: Clean with connected-device waiver.
- Findings: The live privacy policy must be reuploaded with final backup/OEM migration wording. No Android device is connected for physical predictive-back and login verification.
- Fixes or waivers: The final privacy page was uploaded and verified live on July 21, resolving the deployment finding. Connected-device testing is explicitly waived for release 0.1.3 because ADB reports no device; Android unit tests, instrumentation APK compilation, lint, debug assembly, and release bundle assembly pass, and predictive-back dispatcher behavior has focused instrumentation coverage.

## Final Audit
- Done auditors: Herschel (`gpt-5.6-sol`) for submission; Newton (`gpt-5.5`) for publication follow-up.
- Status: Complete. Android Publisher reports Production release `0.1.3`, version code 4, lifecycle `RELEASE_LIFECYCLE_STATE_PUBLISHED`.
- Plan items confirmed: Candidate commit, release metadata, final live policies, test/build/signing/16 KB checks, exact bundle upload, validated and committed Production edit, full-rollout track state, and post-commit readback.
- Tests and checks confirmed: Go test/vet/build; Android unit/lint/debug/instrumentation APK/release bundle; signature, manifest, native alignment, live URL, upload hash, track, and lifecycle checks.
- Waivers or skipped checks: Connected-device verification was waived because ADB reported no device. The former Play-review blocker is resolved; a fresh lifecycle query on July 22, 2026 reported version 4 as `RELEASE_LIFECYCLE_STATE_PUBLISHED`.
