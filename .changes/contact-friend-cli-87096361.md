---
category: Added
---

- **Contact friend CLI** — adds `dws contact +friend-list`, `+friend-request-list`, `+friend-request-send`, `+friend-request-accept`, `+friend-request-reject`, and `+friend-remove` shortcuts for the friend-link MCP tools published under `mcpId=2400`.
- **Friend commands now use openDingTalkId** — all friend shortcuts accept and return `openDingTalkId` (the open-platform pairwise identifier used in friend event payloads) instead of `dingtalkId`.
- **Contact user lookup by openDingTalkId** — adds `dws contact user get-by-open-dingtalk-id --id <openDingTalkId>` (alias `get-user-by-open-dingtalk-id`) to retrieve a user's `userId` from their openDingTalkId.
- **Friend shortcuts join the public catalog** — registers the six friend shortcuts in `semantic_catalog_contact.json` (reviewed, public) and regenerates `docs/shortcut-public-catalog.json` + `internal/shortcut/public_catalog_generated.go`, so they are visible to agents instead of hidden.
- **Contact skill documents the friend surface** — `skills/multi/dingtalk-contact/SKILL.md` gains a friend SOP, intent-table rows, and VISIBLE_SHORTCUTS entries; `references/contact.md` gains the friend command reference (risk levels, openDingTalkId identity-key rules, pagination semantics, `+friend-remove` confirmation policy) and the `get-by-open-dingtalk-id` command section, so AI agents can discover and route friend intents.
