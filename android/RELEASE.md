# Android Play Store Release Checklist

## One-Time Setup

1. Create the Play Console app with package name `com.kloneets.kokotools`.
2. Use Google Play App Signing.
3. Generate an upload keystore outside git:

```sh
keytool -genkeypair -v -storetype JKS -keystore koko-tools-upload.jks -alias koko-tools-upload -keyalg RSA -keysize 2048 -validity 10000
```

4. Copy `android/keystore.properties.example` to `android/keystore.properties` and fill it locally:

```properties
storeFile=/absolute/path/to/koko-tools-upload.jks
storePassword=your-store-password
keyAlias=koko-tools-upload
keyPassword=your-key-password
```

5. Build the release bundle:

```sh
./gradlew :app:bundleRelease
```

The bundle is written under `android/app/build/outputs/bundle/release/`. If `android/keystore.properties` is missing, Gradle can still produce an unsigned bundle for compilation checks, but that file is not uploadable to Google Play.

## Firebase And Google Cloud

1. In Firebase, keep the Android package name as `com.kloneets.kokotools`.
2. Enable Firebase Authentication providers:
   - Email/password
   - Google
3. Upload the first AAB to Play internal testing.
4. In Play Console, open Setup -> App integrity and copy the app signing SHA-1 and SHA-256.
5. Add both Play app signing fingerprints and upload key fingerprints to the Firebase Android app.
6. Download the updated `google-services.json` and replace `android/google-services.json`.
7. Restrict the Google Cloud API key to the Android package and release certificate fingerprints.
8. Review Firebase Realtime Database rules before production.

## Play Console Declarations

Complete these before production:

- App content -> Privacy policy: use the public `PRIVACY.md` URL.
- App content -> Data safety: declare account info, user content, and cloud sync.
- App content -> Data deletion: use the public `ACCOUNT_DELETION.md` URL.
- App content -> Content rating.
- App content -> Target audience.
- Store listing: screenshots, short description, full description, category, contact email.

If this is a new personal developer account, complete the required closed test with 12 opted-in testers for 14 days before applying for production access.

## Release Validation

Run:

```sh
./gradlew :app:testDebugUnitTest
./gradlew :app:assembleDebug
./gradlew :app:bundleRelease
```

Then install the internal testing build from Google Play and verify:

- Email/password Firebase login.
- Google Firebase login.
- Sync to Firebase.
- Notes rich text rendering.
- Todo sync and scroll preservation.
- Settings survive app restart.
