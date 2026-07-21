# Agent Task Record

## Task
- Request: Implement accepted Android API 36 policy compliance plan for target SDK, back handling, instrumentation tests, and release documentation.
- Date: 2026-07-16
- Branch or commit: working tree

## Accepted Plan
- Set Android `targetSdk` to 36 while keeping `compileSdk = 36`, `minSdk = 26`, `versionCode = 3`, and `versionName = "0.1.2"` unchanged.
- Add a stable `androidx.activity` dependency appropriate for the repository.
- Migrate `MainActivity` from platform `Activity` and deprecated `onBackPressed()` override to `ComponentActivity` plus a lifecycle-aware `OnBackPressedCallback`.
- Enable the drawer back callback only while the navigation drawer is open, disable it when drawer close starts, preserve default back behavior when the drawer is closed, and do not add a manifest opt-out.
- Add instrumentation coverage for back behavior with the drawer open and closed, using bounded polling.
- Update Android README stale SDK/version metadata and release documentation for API 36 target policy, developer registration, content rating, and 16 KB native compatibility gates.

## Implementation Notes
- Coding agent: Ptolemy (`gpt-5.5`).
- Summary: Raised target SDK to 36, added `androidx.activity:activity:1.12.2`, migrated `MainActivity` to `ComponentActivity` with a drawer-scoped `OnBackPressedCallback`, added instrumentation tests for drawer/open closed back behavior, refreshed Android metadata, and added release policy gates.
- Files changed:
  - `android/app/build.gradle.kts`
  - `android/app/src/main/java/com/kloneets/kokotools/MainActivity.kt`
  - `android/app/src/androidTest/java/com/kloneets/kokotools/MainActivityTest.kt`
  - `android/README.md`
  - `android/RELEASE.md`
  - `docs/agent-tasks/16-android-api-36-policy-compliance.md`
- Plan deviations: None.
- Tests and checks run:
  - `./gradlew :app:testDebugUnitTest` passed.
  - `./gradlew :app:assembleDebugAndroidTest` passed.
  - `./gradlew :app:assembleDebug` passed.
  - `./gradlew :app:bundleRelease` passed.
  - `./gradlew :app:lintDebug` passed when run sequentially. A prior combined parallel lint/bundle invocation hit a generated Prism source race in lint; neither sequential command reproduced it.
  - `go test ./...` passed.
  - `./gradlew :app:connectedDebugAndroidTest` failed because no connected devices were available.

## Review Round 1
- Review agent: Boyle (`gpt-5.5`).
- Status: Clean.
- Findings: None. Residual device risk remains because predictive back gestures could not be exercised without a connected device.
- Fixes or waivers: Device instrumentation is waived for this task because ADB reported no connected devices. Dispatcher-level instrumentation tests compile, and Android unit, debug, and release builds pass.

## Review Round 2
- Review agent:
- Status:
- Findings:
- Fixes or waivers:

## Final Audit
- Done auditor: Pascal (`gpt-5.5`).
- Status: Complete with waivers.
- Plan items confirmed: Target SDK 36, AndroidX predictive back integration, drawer-scoped callback behavior, instrumentation coverage, SDK/version documentation, and release policy gates.
- Tests and checks confirmed: Android unit tests, instrumentation APK compilation, debug assembly, release bundle, lint, Go tests, merged manifest target SDK 36, `git diff --check`, and 16 KB zip alignment.
- Waivers or skipped checks: Connected-device instrumentation and physical predictive-back gesture verification were skipped because ADB reported no connected devices.
