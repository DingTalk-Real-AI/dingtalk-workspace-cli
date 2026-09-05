---
category: Added
---

- **DeepSeek Harness (DSH) skill install target** — recognizes `dsh` as a non-universal Agent so DWS skills install to `${DSH_HOME:-$HOME/.dsh}/skills`, matching `<dshHome>/skills` discovery root used by `@deepseek-ai/dsh-skill-filesystem`. Upstream registry grows 76 → 77 (19 universal / 58 non-universal); test counts updated to match.