---
category: Added
---

- Extend interactive cards with template rendering, summaries, forwarding, mentions and extension metadata.
- Support final A2UI replies across local Agent channels, preserving image attachments and interrupted-turn recovery boundaries.
- Add personal minutes and calendar event schemas and consumer support; service availability requires separate validation.
- Add reviewed LiteApp commands and improve AI table navigation, named links and field verification.
- Add document suggestions, atomic range operations and scoped whiteboard recovery.

- Bound upload PUT operations with the requested timeout and avoid nil-content next-step panics.
- Replace the staged executable inode after macOS signing during upgrade.

Credential rotation commands are outside this change because this public baseline does not expose them.
