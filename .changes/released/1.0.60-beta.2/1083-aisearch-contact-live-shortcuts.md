---
category: Changed
---

- **AiSearch and Contact shortcuts** (#1083) — adds strict people search and reviewed unified results; people results must use the live-reviewed `person` source, and exact mobile lookups normalize accepted formatting before calling the dedicated mobile interface. Agent/public discovery keeps `contact +list-roles`, `contact +list-roster-fields`, `contact +get-roster`, and incomplete Live routes unavailable rather than publishing ambiguous results, while the historical Contact CLI commands retain legacy MCP execution and real error propagation. The legacy role-list projection preserves the service's reviewed null placeholder without exposing that ambiguous row through Agent Result contracts.
