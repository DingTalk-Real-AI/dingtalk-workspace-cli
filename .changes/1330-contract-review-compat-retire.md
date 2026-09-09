---
category: Deprecated
---

- **Contract review CLI leaves** (#1330) — `dws contract review benefit|create|analysis|result` remain as historical argv compatibility shims but no longer call retired MCP tools. They now fail closed with a non-zero `command_retired` error, are marked Cobra Deprecated, and publish Schema interface availability `unavailable` with deprecation-only Agent selection.
