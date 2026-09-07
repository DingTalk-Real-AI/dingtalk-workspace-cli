---
category: Changed
---

- **Chat read shortcuts decrypt third-party messages** (#1318) — the
  policy-driven DingTalk + SafeChat message decryption previously limited to
  the atomic read commands is now applied by the smart-layer read shortcuts
  `chat +chat-messages`, `chat +at-me`, `chat +search-msg` and
  `chat +thread-replies` as well. Decrypted rows return plaintext in `text`
  and carry the per-row markers `contentDecrypted`, `cryptoLayer` and
  `dingKeyVersion`, matching the atomic commands.
- **Additive decrypt ledger** — these shortcuts now publish the same
  top-level ledger as the atomic commands: the four counters
  `decryptCandidateCount` (candidates before policy filtering),
  `decryptAllowedCount`, `decryptedCount` and `decryptFailedCount`, plus the
  per-message `decryptFailures[]` entries and `partial=true` when any failure
  is recorded. The counters appear whenever the decrypt pipeline runs, even
  as zeros when no candidate message exists.
- **Forwarded nested messages are decrypted** — encrypted sub-messages nested
  under `forwardMessages` are now decrypted too, with the plaintext written
  back to the `content` key of regular messages and the `text` key of
  forwarded children.
- **Atomic decrypt failure records are itemized** — when a batch decrypt hits
  a transport error on the atomic read commands, one failure entry with the
  message's `messageId` and `conversationId` is recorded per message instead
  of a single entry without IDs.
