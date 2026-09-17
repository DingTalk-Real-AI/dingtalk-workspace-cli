---
category: Fixed
---

- **OAuth refresh falls back to the organization mirror** — when the server rejects the
  current identity's `refresh_token` with the reviewed `invalidParameter.authCode.notFound`
  business code, `dws` now retries once with the still-valid token mirrored in the same
  organization's slot (same corp, matching or backfilled user identity) before giving up,
  and writes the rotated credential back to both the identity and the organization slots so
  the fallback stays usable on later refreshes. Transient failures and direct-mode HTTP
  rejections without a reviewed business code do not trigger the fallback.
