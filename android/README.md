# Koko Tools Android

This directory contains an independent Kotlin Android companion app for a focused subset of Koko Tools:

- edit local Markdown notes without preview
- calculate Pages values
- sync desktop-compatible snapshots to and from Google Drive

The app uses plain Android SDK widgets and app-private file storage. It does not use Compose.

## Import And Build

Open `android/` in Android Studio as its own Gradle project.

Command-line tasks from this directory with an installed Gradle:

```sh
gradle :app:assembleDebug
gradle :app:testDebugUnitTest
gradle :app:connectedDebugAndroidTest
```

This repository does not require the Android app to be built when building the Go desktop app.

## Android SDK

The project currently uses:

- package name: `com.kloneets.kokotools`
- min SDK: 26
- compile SDK: 35
- target SDK: 35

If your installed Android SDK differs, install API 35 with Android Studio SDK Manager or adjust `compileSdk` and `targetSdk` in `app/build.gradle.kts`.

## Local Data

Android stores data in the app-private files directory:

- notes root: `filesDir/notes/`
- settings: `filesDir/settings.json`

Notes are plain `.md` files. Nested notes use slash-relative paths such as `books/current.md`.

The settings JSON keeps the desktop-compatible subset used by the Android app:

- `pages_app.first_book`
- `pages_app.second_book`
- `pages_app.read_pages`
- `notes_app.current_note_path`
- `gdrive.folder_id`
- `gdrive.folder_name`
- `gdrive.selected_snapshot_id`
- `gdrive.snapshots`

## Google Drive Setup

Android uses Google Play services authorization and direct Drive REST calls. Android must use its own OAuth client; do not reuse the desktop OAuth client.

Google Cloud setup:

1. Enable the Google Drive API for the project.
2. Create an OAuth client of type Android.
3. Use package name `com.kloneets.kokotools`.
4. Add the signing certificate SHA-1 for the debug or release keystore used to build the app.
5. Request Drive scope `https://www.googleapis.com/auth/drive`.

The full Drive scope is required because the desktop app works with a user-selected folder and a full snapshot tree.

## Sync Workflow

In the Android app:

1. Open `Sync`.
2. Tap `Connect Google` and grant Drive access.
3. Paste the existing desktop Drive folder ID into `Folder ID`.
4. Tap `Set Drive folder ID`.
5. Use `Upload snapshot`, `Refresh snapshots`, or select a listed snapshot and tap `Restore selected snapshot`.

The folder ID is the Drive file ID of the shared Koko Tools folder. In a browser Drive URL, it is the ID after `/folders/`.

## Google Connect Troubleshooting

If `Connect Google` flashes, opens Settings, or returns without connecting on an emulator:

- Use an emulator system image with Google Play, not a plain AOSP image.
- Open the Play Store or Settings on the emulator and sign into a Google account first.
- Complete any screen-lock prompt Google shows during account setup.
- Prefer a stable API image such as API 35 or 36 if a preview image has Google Play services crashes.
- Confirm the Android OAuth client in Google Cloud uses package `com.kloneets.kokotools` and the SHA-1 of the keystore used by Android Studio.

## Desktop Snapshot Interoperability

The shared Drive folder is expected to contain:

```text
snapshots/
  <timestamp>/
    settings.json
    notes/
      ...
```

Android-created snapshots should be restorable by the desktop app, and desktop-created snapshots should be restorable by Android. Snapshot retention should keep the latest 5 snapshots when Drive upload is implemented.

## Manual Test Checklist

Verify:

- create a note locally on Android
- upload a snapshot
- restore a snapshot
- verify desktop can list and restore an Android-created snapshot
- verify Android can restore a desktop-created snapshot
