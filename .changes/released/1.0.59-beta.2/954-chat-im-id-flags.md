---
category: Changed
---

- **Chat IM ID flags** (#954) — standardizes chat command entry points on `--conversation-id` for conversation IDs and `--message-id` for message IDs, so help, Schema, and Agent recommendations use the same canonical flags.
- **Legacy chat flag compatibility** (#954) — keeps older chat IM ID flags such as `--group`, `--id`, `--chat`, `--open-conversation-id`, `--msg-id`, and `--open-message-id` working as compatibility aliases where applicable, while hiding migrated aliases from recommended help and Schema surfaces.
- **Chat group bots target flag** (#954) — keeps `dws chat group bots` on the visible `--group` flag; this command does not register `--group-name`, and `--group` accepts either an openConversationId or a uniquely resolved group name.
