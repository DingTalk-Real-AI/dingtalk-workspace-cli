---
category: Fixed
---

- **Login with unreadable token slots** (#1172) — after a fresh OAuth, device, PAT, or `--token` login, legacy global, identity, and organization token slots whose ciphertext no longer decrypts with the current data-encryption key are removed so the new credential can be persisted instead of stranding a completed login at the write preflight.
