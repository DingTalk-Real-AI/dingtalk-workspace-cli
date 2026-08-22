---
category: Added
---

- **Permission and member list pagination** (#1085) — `drive/doc permission
  list` and `wiki member list` now accept `--next-token` to follow the
  server-side cursor (output carries `totalCount`/`hasMore`/`nextToken`) and
  map `--limit` to `pageSize` capped at 50 instead of the rejected `maxResults
  200` path; `permission add/update/remove` and `wiki member add/update/remove`
  additionally accept a `--members` JSON array covering USER/DEPT/CONVERSATION/TAG
  grantee types. The optional `--notify` defaults to `false` and is omitted from
  the server request unless passed explicitly, so member grants no longer notify
  recipients by default. These commands also declare cursor pagination
  (`next-token`) in the Agent schema contract, mirroring the internal CLI parity
  change.
