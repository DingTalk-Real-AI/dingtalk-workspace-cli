---
category: Fixed
---

- `chat message send-a2ui-card` now sends `supportForward=true` by default. Explicit `--support-forward=false` still disables forwarding; explicit true remains supported. Digital employee replies explicitly preserve their independent `--reply-card-forward` setting, and approval cards remain non-forwardable.
