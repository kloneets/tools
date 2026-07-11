# Agent Task Record

## Task
- Request: Fix Android Google SSO on a Wi-Fi-debugging device.
- Date: 2026-07-11
- Branch or commit: `main`

## Accepted Plan
Diagnose Google sign-in on the device, repair Firebase package/certificate and Google-provider configuration, align Android's ID-token audience, improve sign-in error diagnostics, and verify login plus session restoration with `janis@xit.lv`.

## Implementation Notes
- Coding agent: Locke, integrated by Codex.
- Summary: Registered debug and upload signing fingerprints in Firebase, enabled the Firebase Google provider, aligned Android with the exported web OAuth client, refreshed `google-services.json`, and made non-success Google result intents report their real status code instead of being labeled cancellation.
- Files changed: `android/google-services.json`, `android/gradle.properties`, `MainActivity.kt`, `GoogleSignInErrorFormatter.kt`, its unit test, Android README, and this task record.
- External changes: Firebase project `koko-tools` now has debug/upload SHA-1 and SHA-256 certificates and the built-in Google provider enabled.
- Plan deviations: Play app-signing fingerprints were initially unavailable. During release preparation they were extracted from Play's generated version-1 APK and registered in Firebase.
- Tests and checks run: Android unit tests, debug install, live Google login with `janis@xit.lv`, post-login sync, persisted-account inspection, and cold session restoration.

## Review Round 1
- Review agent: Kierkegaard.
- Status: clean with waivers.
- Findings: No blocking findings.
- Fixes or waivers: The original Play-signing waiver was resolved during release preparation by registering Play App Signing SHA-1/SHA-256 in Firebase. Play-installed SSO still requires confirmation from Internal testing.

## Final Audit
- Done auditor: Gibbs.
- Status: complete with waivers.
- Plan items confirmed: Firebase debug/upload certificates and Google provider repaired, Android OAuth configuration aligned, status-aware result handling tested, and live `janis@xit.lv` login plus cold restoration verified.
- Tests and checks confirmed: Android unit tests, debug assembly/install, lint, Go tests/vet, live device login/sync, persisted-account inspection, and cold restart.
- Waivers or skipped checks: Play App Signing fingerprints are now registered. Play-distributed Google SSO remains pending until version 2 is installed from Internal testing.
