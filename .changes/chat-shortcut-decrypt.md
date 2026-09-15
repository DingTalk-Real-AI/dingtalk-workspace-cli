---
category: Changed
---

- **Smart chat read shortcuts** (`+chat-messages`, `+at-me`, `+search-msg`, `+thread-replies`) now decrypt third-party encrypted (SafeChat) message ciphertext before projection when the message crypto policy allows it. Message projections gain `contentDecrypted`, `cryptoLayer`, and `dingKeyVersion` (when > 0), and payloads gain an additive decrypt ledger (`decryptCandidateCount`, `decryptAllowedCount`, `decryptedCount`, `decryptFailedCount`, `decryptFailures[]`). Single-item failures land in the ledger with `partial: true` and never change the command exit code; policy-off, stub builds, and `--dry-run` keep output byte-identical to the previous behavior.
- **`intField` in the message crypto module now accepts `json.Number`**, so `keyVersion` fields surfaced as JSON numbers by the MCP runtime are parsed instead of being silently dropped. The atomic chat read path may now emit `dingKeyVersion` on decrypted message projections (additive and within the existing result contract).
