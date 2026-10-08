---
category: Added
---

- **Contact friend CLI** — adds `dws contact +friend-list`, `+friend-request-list`, `+friend-request-send`, `+friend-request-accept`, `+friend-request-reject`, and `+friend-remove` shortcuts for the friend-link MCP tools published under `mcpId=2400`.
- **Friend commands now use openDingTalkId** — all friend shortcuts accept and return `openDingTalkId` (the open-platform pairwise identifier used in friend event payloads) instead of `dingtalkId`.
- **Contact user lookup by openDingTalkId** — adds `dws contact user get-by-open-dingtalk-id --id <openDingTalkId>` (alias `get-user-by-open-dingtalk-id`) to retrieve a user's `userId` from their openDingTalkId.
- **Friend shortcuts join the public catalog** — registers the six friend shortcuts in `semantic_catalog_contact.json` (reviewed, public) and regenerates `docs/shortcut-public-catalog.json` + `internal/shortcut/public_catalog_generated.go`, so they are visible to agents instead of hidden.
- **Contact skill documents the friend surface** — `skills/multi/dingtalk-contact/SKILL.md` gains a friend SOP, intent-table rows, and VISIBLE_SHORTCUTS entries; `references/contact.md` gains the friend command reference (risk levels, openDingTalkId identity-key rules, pagination semantics, `+friend-remove` confirmation policy) and the `get-by-open-dingtalk-id` command section, so AI agents can discover and route friend intents.

## Unreleased

---
category: Changed
---

- **Friend list projections expose the nickname** — `+friend-list` now projects `userProfileModel.nick` (friend nickname) from the MCP v3 response; `alias` is re-labeled as the friend remark name (备注名) and `status` is documented as the friend operation status (1=added, 0=removed).
- **Friend request list exposes the requester nickname** — `+friend-request-list` now projects `userProfileModel.nick` (requester nickname) from the MCP v3 response.
- **Friend request status is documented for agent translation** — `+friend-request-list` result schema documents the status enum (0=no relation, 1=pending accept, 2=sent, 3=already friends, 4=recommended) and instructs agents to translate the status into a Chinese description instead of showing the raw number.
- **openDingTalkId display guidance** — both list result schemas mark `openDingTalkId` as the internal identifier required for follow-up operations: always returned, but agents should not display it unless the user needs to act on it.
