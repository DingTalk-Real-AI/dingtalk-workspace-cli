---
category: Added
---

- **Permission and member list pagination** (#1065) — `drive/doc permission
  list` and `wiki member list` now accept `--next-token` to follow the
  server-side cursor (output carries `totalCount`/`hasMore`/`nextToken`) and
  map `--limit` to `pageSize` capped at 50 instead of the rejected `maxResults
  200` path; `permission add/update/remove` and `wiki member add/update/remove`
  additionally accept a `--members` JSON array covering USER/DEPT/CONVERSATION/TAG
  grantee types with an optional `--notify`, mirroring the internal CLI parity
  change.
