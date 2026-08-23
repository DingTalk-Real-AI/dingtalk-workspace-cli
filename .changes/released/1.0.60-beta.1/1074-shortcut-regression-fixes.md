---
category: Fixed
---

- **Calendar empty windows** (#1074) — returns a legitimate empty result when the service emits its exact exhausted empty-event sentinel.
- **Task update verification** (#1074) — compares due-time readback as exact milliseconds so committed updates are no longer reported as failures.
- **Comment reaction validation** (#1074) — narrows accepted reaction input to reviewed DingTalk emoji names and rejects Unicode emoji and unsupported names such as `like` and `heart` before the RPC.
