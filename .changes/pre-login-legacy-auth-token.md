---
category: Fixed
---

- **Legacy token pre-login recovery** — Allow OAuth, device, auth-code, PAT, and `--token` reauthorization to start when the legacy `auth-token` ciphertext has a confirmed DEK mismatch. The old ciphertext remains untouched until fresh credentials are available, then only the login's target slots are replaced; transient and unclassified Keychain failures still fail closed.
