# Google Play Publishing Guide

Follow these steps to publish Koko Tools Android to Google Play.

## 1. Create A Play Console Account

1. Go to Google Play Console: <https://play.google.com/console>
2. Create or finish your developer account.
3. Check your account type:
   - Organization account: usually can publish after normal review.
   - New personal account created after November 13, 2023: must run closed testing with at least 12 opted-in testers for 14 continuous days before production access.

Google reference: <https://support.google.com/googleplay/android-developer/answer/14151465>

## 2. Create The App

1. In Play Console, click `Create app`.
2. App name: `Koko Tools`.
3. Default language: choose the intended listing language.
4. App or game: `App`.
5. Free or paid: choose carefully. Free apps cannot later become paid.
6. Confirm the required declarations.

Use the package name already in code:

```text
com.kloneets.kokotools
```

## 3. Generate The Upload Keystore

From the repo root:

```sh
cd android
keytool -genkeypair -v \
  -storetype JKS \
  -keystore ~/koko-tools-upload.jks \
  -alias koko-tools-upload \
  -keyalg RSA \
  -keysize 2048 \
  -validity 10000
```

Then create local signing config:

```sh
cp keystore.properties.example keystore.properties
```

Edit `android/keystore.properties`:

```properties
storeFile=/home/koko/koko-tools-upload.jks
storePassword=replace-with-store-password
keyAlias=koko-tools-upload
keyPassword=replace-with-key-password
```

Do not commit `keystore.properties` or the `.jks` keystore.

## 4. Build The Signed Release AAB

Run:

```sh
cd android
./gradlew :app:bundleRelease
```

Verify the bundle is signed:

```sh
jarsigner -verify -verbose -certs app/build/outputs/bundle/release/app-release.aab
```

Upload this file to Play Console:

```text
android/app/build/outputs/bundle/release/app-release.aab
```

## 5. Set Up Play App Signing

1. In Play Console, go to `Setup -> App integrity`.
2. Enable or use Google Play App Signing.
3. After uploading the first bundle, copy:
   - App signing SHA-1
   - App signing SHA-256
   - Upload key SHA-1/SHA-256, if shown

Google reference: <https://support.google.com/googleplay/android-developer/answer/9842756>

## 6. Update Firebase For Release Sign-In

1. Open Firebase Console project `koko-tools`.
2. Go to `Project settings -> Your apps -> Android app`.
3. Confirm the package name is `com.kloneets.kokotools`.
4. Add SHA-1 and SHA-256 fingerprints:
   - Upload keystore fingerprints
   - Play app signing fingerprints
5. Enable Firebase Authentication providers:
   - Email/password
   - Google
6. Download the updated `google-services.json`.
7. Replace:

```text
android/google-services.json
```

8. Rebuild and upload a new AAB.

Firebase Google sign-in reference: <https://firebase.google.com/docs/auth/android/google-signin>

## 7. Lock Down Google Cloud And Firebase

Before public release:

1. In Google Cloud Console, restrict the API key to:
   - Android app package: `com.kloneets.kokotools`
   - Release SHA fingerprints
2. Restrict API usage to only required APIs.
3. Review Firebase Realtime Database rules so users cannot read or write other users' workspaces.

Google Cloud API key reference: <https://cloud.google.com/docs/authentication/api-keys>

## 8. Complete Play Console Store Setup

Complete these Play Console sections.

Store listing:

- Short description
- Full description
- App icon: `android/play-store/koko-tools-icon-512.png`
- Feature graphic
- At least 2 phone screenshots
- Category, likely `Productivity`
- Contact email

App content:

- Privacy policy URL: `https://koko.lv/koko-tools/privacy-policy.html`
- Data safety form.
- Data deletion URL: `https://koko.lv/koko-tools/account-deletion.html`
- Content rating questionnaire.
- Target audience.
- Ads: `No`, unless ads are added later.
- App access: explain Firebase login if reviewers need credentials.

Important: upload `docs/privacy-policy.html` and `docs/account-deletion.html` to `https://koko.lv/koko-tools/` before entering these URLs in Play Console. The pages must be public without login.

Google references:

- Data safety: <https://support.google.com/googleplay/android-developer/answer/10787469>
- Account deletion: <https://support.google.com/googleplay/android-developer/answer/13327111>

## 9. Run Internal Testing

1. Go to `Test and release -> Testing -> Internal testing`.
2. Create a release.
3. Upload the signed AAB.
4. Add your tester email.
5. Install from Google Play, not adb.
6. Test:
   - App opens.
   - Firebase email login works.
   - Google login works.
   - `Sync to Firebase` works.
   - Notes rich text rendering works.
   - Todo sync works.
   - Settings persist after restart.

## 10. Run Closed Testing If Required

If Play Console says production is locked:

1. Go to `Closed testing`.
2. Create a tester list with at least 12 Google accounts.
3. Publish the closed test.
4. Ensure 12 testers opt in and remain opted in for 14 continuous days.
5. Ask testers to open and use the app and report issues.
6. After the requirement is met, apply for production access.

## 11. Publish To Production

1. Go to `Production`.
2. Create a release.
3. Upload the final signed AAB.
4. Add release notes from `android/play-store/release-notes/4-en-US.txt`.
5. Set rollout status to full Production release for this version. Do not configure a staged rollout fraction for release 0.1.3.
6. Watch:
   - Pre-launch report
   - Crashes and ANRs
   - Firebase login and sync behavior
   - Play review feedback
