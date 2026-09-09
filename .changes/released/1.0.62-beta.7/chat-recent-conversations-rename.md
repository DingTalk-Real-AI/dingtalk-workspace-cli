---
category: Changed
---

- **Chat recent conversations** — makes `dws chat +recent-conversations` the preferred entry and retains `+active-conversations` as a hidden executable compatibility entry sharing the same implementation, without deprecation warnings in execution, Help, or Schema. Public Help, Schema primary CLI paths/examples, and Mono/Multi Skills recommend the new name; the stable Schema identity `chat.shortcut_active_conversations` and old CLI-path lookup remain supported. Query windows, pagination, and result semantics are unchanged by this rename.
