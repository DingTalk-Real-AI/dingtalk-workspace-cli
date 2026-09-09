---
category: Added
---

- **SafeChat message encryption/decryption** (#1051) — `dws safechat` commands enable AnHeng SafeDing (安恒密盾) message encryption and decryption. Available only in builds with `-tags safechat` (requires CGO and platform-specific static libraries). Commands include `safechat selftest` for end-to-end self-check (real authCode fetch and key retrieval) and `safechat decrypt` to decrypt ciphertext messages. The PR also adds `internal/msgcrypto` package with cipher operations, vendorAuthCode portal integration, and key server client supporting both in-memory and file-based keystores with 0600 permissions on Unix and warning logs on Windows.
