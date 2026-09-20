---
category: Fixed
---

- **Windows Agent skill link readability** (#1425) — uses directory junctions with absolute canonical targets for Agent skill adapters on Windows and validates readability before publication, preventing `EPERM` errors when followed by Node.js and Agent skill loaders.
