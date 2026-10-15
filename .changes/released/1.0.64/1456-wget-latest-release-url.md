---
category: Fixed
---

- **wget-only install of the latest release** (#1456) — resolving `latest` without curl now requests the GitHub releases URL directly, so wget can read the release tag instead of failing before the download.
