# Agent Task Record

## Task
- Request: Prepare production Android release 0.1.4 and publish it to Google Play.
- Date: 2026-09-05
- Branch or commit: pending local dirty worktree

## Accepted Plan
- Preserve the in-progress multiple-todo-list work and avoid unrelated source changes.
- Prepare a new Google Play artifact with `versionCode = 5`, `versionName = "0.1.4"`, `targetSdk = 36`, and version-specific Play release notes.
- Keep Android storage backward-compatible: the default list continues to use `filesDir/todos.json`; new named lists use `filesDir/todo-lists/<list-id>.json` plus a catalog file.
- Add release-safety fixes before publishing: validate Firebase database URLs as absolute `https` URLs with a host, prevent invalid legacy Firebase config from starting background sync, and run instrumentation against an isolated package with Firebase defaults disabled.
- Preserve local-only todos during normal and manual full Firebase sync; allow dropping local-only todos only from the explicit "Replace local from Firebase" path.
- Add a release-build gate that validates shipped Firebase defaults, rather than relying on the intentionally blank isolated test configuration.
- Build and verify the signed AAB, then publish the exact verified version code 5 artifact to the Production track using the Android Publisher API if authentication is available.
- Do not reuse version code 4 or publish if Play reports version code 5 is already used.

## Implementation Notes
- Operator: Codex.
- Summary: Release candidate prepared, connected-device verified, uploaded to Google Play, and committed to Production as a 20% staged rollout.
- Files changed:
  - `android/app/build.gradle.kts`
  - `android/app/src/main/java/com/kloneets/kokotools/FirebaseSyncRepository.kt`
  - `android/app/src/main/java/com/kloneets/kokotools/SettingsRepository.kt`
  - `android/app/src/androidTest/java/com/kloneets/kokotools/MainActivityTest.kt`
  - `android/app/src/androidTest/java/com/kloneets/kokotools/HunspellSpellEngineInstrumentedTest.kt`
  - `android/app/src/test/java/com/kloneets/kokotools/FirebaseSyncRepositoryTest.kt`
  - `android/app/src/test/java/com/kloneets/kokotools/SettingsRepositoryTest.kt`
  - `android/RELEASE.md`
  - `android/README.md`
  - `android/PLAY_STORE_PUBLISHING.md`
  - `android/play-store/release-notes/5-en-US.txt`
  - `docs/agent-tasks/26-android-release-0.1.4.md`
- Plan deviations: Production was published as a 20% staged rollout (`status = inProgress`, `userFraction = 0.2`) rather than an immediate 100% completed rollout so the release can be monitored and halted if needed.
- Play preflight: Authenticated Publisher API readback found bundle version codes `1`, `2`, `3`, and `4`; version code `5` is unused. Production, Internal, and beta serve completed release `0.1.3` / version code `4`; alpha is empty.
- Tests and checks run:
  - `./gradlew :app:testDebugUnitTest` passed before release-prep edits.
  - `./gradlew :app:testDebugUnitTest --stacktrace` failed after setting `testBuildType = "isolated"` because the task is no longer generated; replaced by `testIsolatedUnitTest`.
  - `./gradlew :app:testIsolatedUnitTest --stacktrace` initially failed because `ApplicationProvider` was only an instrumentation dependency; added `androidx.test:core` for unit tests.
  - `./gradlew :app:testIsolatedUnitTest --stacktrace` then failed because two JVM tests used instrumentation context and the previous bundled-defaults test did not account for isolated Firebase defaults; tests were adjusted to use pure validation logic and assert empty defaults for the isolated target.
  - `go test ./...` passed.
  - `./gradlew :app:assembleDebug :app:assembleIsolated :app:lintIsolated :app:lintRelease --stacktrace` passed.
  - `./gradlew :app:assembleIsolatedAndroidTest :app:bundleRelease --stacktrace` passed before review fixes.
  - `adb devices -l` showed no connected devices; `./gradlew :app:connectedIsolatedAndroidTest --stacktrace` failed with `No connected devices!`.
  - Signed AAB before review fixes: SHA-256 `85710e45e32965740332f2e95d12d2109f42b9eb75544e05848a4716540c5ea4`, 14,956,604 bytes; release manifest reported package `com.kloneets.kokotools`, version code `5`, version name `0.1.4`, target SDK `36`.
  - `go test ./...` passed after review fixes.
  - `./gradlew :app:testIsolatedUnitTest :app:verifyReleaseFirebaseConfig --stacktrace` failed once because the first gate hook referenced `preReleaseBuild` before Android Gradle Plugin created it; the hook was changed to lazy task configuration.
  - `./gradlew --no-parallel :app:testIsolatedUnitTest :app:verifyReleaseFirebaseConfig :app:bundleRelease` passed after review fixes and the caller-semantics correction.
  - `./gradlew --no-parallel :app:testIsolatedUnitTest :app:lintIsolated :app:assembleIsolatedAndroidTest :app:bundleRelease` passed before the final caller-semantics correction; the final correction was recompiled and covered by the subsequent unit-test and bundle command.
  - Final signed AAB: SHA-256 `d4aa82e8dc8f474122f34c1545756f1328cd22ddb29ef0ad1b86040e23a96ccf`, 14,957,184 bytes, timestamp `2026-09-05 13:00:57 +0300`.
  - `jarsigner -verify` passed. The merged release manifest reports package `com.kloneets.kokotools`, version code `5`, version name `0.1.4`, and target SDK `36`.
  - Release `arm64-v8a` and `x86_64` `libkoko_spell.so` LOAD segments report `0x4000` alignment, confirming 16 KB native alignment.
  - The final connected attempt could not start because wireless ADB had disconnected; no production data was cleared or modified.
  - On 2026-09-07, `./gradlew --no-parallel :app:connectedIsolatedAndroidTest --stacktrace` passed on RMO-NX1 (Android 13): 15 tests completed, 0 skipped, 0 failed, and Gradle reported `BUILD SUCCESSFUL`. Unified Test Platform warned that it could not retrieve per-test logcat, which did not affect test results.
  - Final local verification after the navigation and instrumentation fixes: `go test ./...`, `go vet ./...`, and `git diff --check` passed.
  - `./gradlew --no-parallel :app:testIsolatedUnitTest :app:lintIsolated :app:lintRelease :app:assembleIsolatedAndroidTest :app:bundleRelease --stacktrace` passed and reran the release Firebase configuration gate.
  - Rebuilt signed AAB: SHA-256 `3234e4f62355a5a45471d850fa7a83d9ab9a2c3e4a694f4399074a95e121d564`, 14,957,787 bytes, timestamp `2026-09-07 15:17:21 +0300`; `jarsigner -verify` passed. The merged release manifest reports package `com.kloneets.kokotools`, version code `5`, version name `0.1.4`, and target SDK `36`.
  - Final Play preflight on 2026-09-07 confirmed version code `5` remains unused. Production, Internal, and beta still serve completed release `0.1.3` / version code `4`; alpha remains empty.
  - Final verification on 2026-09-07: `./gradlew :app:connectedIsolatedAndroidTest --stacktrace`, `go test ./...`, `go vet ./...`, `git diff --check`, `go build -o koko-tools`, and `./gradlew --no-parallel :app:testIsolatedUnitTest :app:lintIsolated :app:lintRelease :app:verifyReleaseFirebaseConfig :app:bundleRelease --stacktrace` all passed.
  - Google Play edit `15592369776770724592` uploaded version code `5`, validated, and committed.
  - Post-commit Play readback confirmed Production release `0.1.4` has version code `5`, status `inProgress`, `userFraction = 0.2`, release notes from `android/play-store/release-notes/5-en-US.txt`, and bundle SHA-256 `3234e4f62355a5a45471d850fa7a83d9ab9a2c3e4a694f4399074a95e121d564`.

## Review Round 1
- Status: Needs changes.
- Findings:
  - Normal/manual full Android todo sync could drop local-only todos when remote returned no item records, because `pullTodos(forceFull = true)` initialized the merge from an empty map.
  - Isolated tests proved the blank test defaults but did not add a release-build gate proving shipped Firebase defaults remain populated and valid.
  - Android README listed the wrong named-list storage path.
- Fixes or waivers: Fixed the merge model so full sync preserves local active/archive data while explicit replacement has a separate `replaceLocal` path; corrected the two UI callers so only Replace Local uses destructive semantics; added focused default/named/replace regression tests; added `verifyReleaseFirebaseConfig` to the release build graph; corrected README storage paths. No waivers for code findings.

## Review Round 2
- Review agent: `/root/reviewer_android_release_v5` (GPT-5.6 Sol).
- Status: Clean, with publication still blocked on connected-device evidence unless the user explicitly accepts that risk.
- Findings: No remaining implementation defects. Connected isolated instrumentation has not run; the earlier debug-package Hunspell run hung and is not passing evidence.
- Fixes or waivers: No code waivers. The connected-device gate remains open.

## Review Round 3
- Review agent: `/root/release_final_review` (GPT-5.5).
- Status: Clean after the connected-device gate passed.
- Findings: No release-blocking implementation defects were reported in the final pre-publication review.
- Fixes or waivers: No remaining code findings or test waivers.

## Final Audit
- Status: Complete with documented rollout-scope deviation.
- Plan items confirmed: Version code `5`, version name `0.1.4`, target SDK `36`, release notes, backward-compatible todo storage, Firebase URL validation, isolated instrumentation package, release Firebase config gate, non-destructive normal Firebase sync semantics, signed AAB verification, Play upload, and Production track assignment were all completed.
- Tests and checks confirmed: Go unit tests, Go vet, desktop build, Android isolated unit tests, Android isolated/release lint, release Firebase config verification, connected isolated Android instrumentation, release bundle build, signature verification, manifest/version checks, native 16 KB alignment checks, Play preflight, upload validation, commit, and post-commit readback.
- Waivers or skipped checks: Production was staged at 20% instead of immediately completing to 100%; this preserves the ability to halt rollout while still publishing the production release requested by the user. Play policy review/rollout availability can take time after commit and should be monitored in Play Console.
