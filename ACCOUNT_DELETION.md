# Koko Tools Account And Data Deletion

Public HTML version for Google Play: https://koko.lv/koko-tools/account-deletion.html

Last updated: July 17, 2026

Koko Tools uses Firebase Authentication and Firebase Realtime Database when Firebase sync is enabled.

## Request Account Or Cloud Data Deletion

Email janis@xit.lv from the email address used to log in to Koko Tools.

Include:

- The email address used for Firebase login.
- Whether you want the Firebase Authentication account deleted, synced workspace data deleted, or both.

Do not include passwords, notes, todos, or other private content in the request.

## Verification And Timing

Requests are verified privately using the Firebase login email address and any additional information needed to confirm account ownership.

Koko Tools will acknowledge deletion requests within 7 calendar days and complete verified deletion requests within 30 days.

## Data Deleted

Verified deletion requests can delete Firebase Authentication account data and synced Firebase workspace data, including notes, todos, shared app settings, workspace identifiers, update timestamps, and sync metadata.

## Data Retained

Deleted account and workspace content is not retained after the deletion request is completed. Minimal correspondence and security audit records may be retained for up to 90 days when needed to process, verify, or document the request.

## Local Data

Uninstalling the Android app removes local app-private data from the device. Desktop data is stored under the desktop app's local config directory and can be removed separately.
