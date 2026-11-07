---
category: Fixed
---

- **Personal events** now obtain managed AppKey metadata when it is missing locally in open-source normal mode, without requiring an AppSecret or changing saved credentials. Existing application identities retain precedence, background listeners reuse the resolved AppKey, and metadata failures report actionable retryability.
