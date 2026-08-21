---
category: Added
---

- **Contact label create** — `dws contact label create --name <角色名称> --parent-id <父标签组ID>` creates a new role (label) under the specified parent label group. Pass `--parent-id 0` for the root group. The command calls the `add_label` MCP tool and requires confirmation; use `--yes` only after explicit user confirmation.
