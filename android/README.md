# Koko Tools Android

This directory contains an independent Kotlin Android companion app for a focused subset of Koko Tools:

- edit local Markdown notes with optional rich rendering
- import, list, and delete managed note assets
- manage todos
- calculate Pages values
- sync notes, todos, settings, and managed assets with Firebase

The app uses plain Android SDK widgets and app-private file storage. It does not use Compose.

## Import And Build

Open `android/` in Android Studio as its own Gradle project.

Command-line tasks from this directory:

```sh
./gradlew :app:assembleDebug
./gradlew :app:testDebugUnitTest
./gradlew :app:connectedDebugAndroidTest
```

This repository does not require the Android app to be built when building the Go desktop app.

## Android SDK

The project currently uses:

- package name: `com.kloneets.kokotools`
- min SDK: 26
- compile SDK: 35
- target SDK: 35
- version code: 1
- version name: 0.1.0

If your installed Android SDK differs, install API 35 with Android Studio SDK Manager or adjust `compileSdk` and `targetSdk` in `app/build.gradle.kts`.

## Local Data

Android stores data in the app-private files directory:

- notes root: `filesDir/notes/`
- settings: `filesDir/settings.json`
- todos: `filesDir/todos.json`

Notes are plain `.md` files. Nested notes use slash-relative paths such as `books/current.md`. Managed assets live under the notes root in either `assets/` or note-specific `<note>.assets/` folders.

The settings JSON keeps the desktop-compatible subset used by the Android app:

- `pages_app.first_book`
- `pages_app.second_book`
- `pages_app.read_pages`
- `notes_app.current_note_path`
- `notes_app.preview_hidden`
- `notes_app.spell_check_enabled`
- `notes_app.spell_dictionaries`
- `android_app.theme_mode`
- `firebase.*`

## Firebase Setup

The app reads bundled Firebase defaults from `google-services.json` or Gradle properties:

- `KOKO_FIREBASE_API_KEY`
- `KOKO_FIREBASE_DATABASE_URL`
- `KOKO_FIREBASE_PROJECT_ID`
- `KOKO_GOOGLE_WEB_CLIENT_ID`

Firebase requirements:

1. Android package name must be `com.kloneets.kokotools`.
2. Firebase Authentication must enable Email/password and Google providers.
3. Realtime Database must be configured with production-safe rules.
4. Release builds need SHA-1 and SHA-256 fingerprints added to the Firebase Android app for the upload key and Play app signing key.
5. After changing Firebase fingerprints or OAuth clients, download a fresh `google-services.json` into `android/google-services.json`.

## Play Store Release

See [RELEASE.md](RELEASE.md) for the release signing, Firebase, Play Console, privacy, and validation checklist.

Release signing uses local `android/keystore.properties`, which is ignored by git.

## Assets Tab

The Assets tab lists files that are eligible for Firebase asset sync. Actions include:

- `Import asset`: choose a file from Android's document picker and copy it into app-private managed asset storage.
- `Refresh`: reload the managed asset list.
- Tap an asset and choose `Delete` to remove it locally and push a Firebase tombstone when Firebase is configured.

Assets larger than 1 MiB can be imported locally, but Firebase asset push skips them.

## Manual Test Checklist

Verify:

- create and edit notes locally on Android
- rich text rendering can be toggled from Settings
- create, edit, complete, and reorder todos
- log in with Firebase email/password
- log in with Google Firebase SSO
- use `Sync to Firebase`
- restart the app and confirm settings are restored
- confirm the same Firebase workspace syncs with the desktop TUI
