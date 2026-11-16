---
category: Changed
---

- **VoIP MeetingSDK credentials** — reads the renamed `callerUserId`, keeps
  rolling compatibility with `callerUid`, projects the non-sensitive SDK app
  ID and expiry, reads the dynamic token from `body.sdkAuth.sdkToken` with a
  legacy message-header fallback, and supports an explicit
  `--include-voip-sdk-token` opt-in for digital employees that need the token
  to join a call. Default
  flattened and transport outputs continue to exclude sensitive credentials.
