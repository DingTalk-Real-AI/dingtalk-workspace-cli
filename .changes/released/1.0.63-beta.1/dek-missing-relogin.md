---
category: Fixed
---

- **DEK missing relogin** — Allow reauthorization when the local login encryption key is missing. Keep old ciphertext until a fresh credential is available, then replace only the login write targets and create a new encryption key as needed. Refresh and transient Keychain failures retain their existing protection.
