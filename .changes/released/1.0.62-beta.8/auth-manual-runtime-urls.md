---
category: Fixed
---

- Include the available runtime context in manual OAuth and device authorization links, including `--no-browser`, so copied links match browser authorization. Display device links outside a frame to keep long URLs copyable on narrow terminals.
- Stop adding the CLI locale as a `lang` query parameter to browser, manual, and reauthorization login links.
