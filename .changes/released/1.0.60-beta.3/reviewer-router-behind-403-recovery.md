---
category: Fixed
---

- **Reviewer Router recovery** — keeps exact App-owned PRs that are behind `main` retriable when GitHub reports the protected merge denial as `Resource not accessible by integration`, while preserving every other 403 as a hard failure.
