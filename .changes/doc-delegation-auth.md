---
category: Added
---

- **Doc-business delegation auth** — the `drive`, `doc`, `sheet`, `wiki`, and `markdown` command groups now accept a persistent `--principal-user-id` flag. When set, the first invocation of each doc-business tool key per node within a session is gated by a `check_capability` verification on behalf of the principal; granting the capability is an out-of-band action the principal completes on the server side, and the CLI never calls `grant_capability`. A denied check surfaces the server's denial message and blocks the original call.
- **Dry-run consistency** — `checkCapability` now executes in dry-run mode as well, ensuring preview and execution behaviors are consistent. In dry-run, the check routes through the `ReadTool` channel (real network request) instead of `CallTool` (which would go through EchoRunner and always deny).
- **Dry-run pre-check in helpers** — dry-run mode now invokes the delegation auth validator before rendering the preview, ensuring commands that would be denied at execution time are also blocked at preview time.
- **Local rejection for node-less commands** — commands that lack a node identifier (e.g. search/list/create without nodeId) now return a clear client-side error (`DELEGATION_AUTH_NOT_SUPPORTED`, exit code 3) when `--principal-user-id` is set, instead of forwarding an incomplete request to the server.
- **Concurrency safety** — the per-session `checked` map in the delegation auth decorator is now protected by a `sync.Mutex`, preventing data races under concurrent tool invocations.
- **Markdown dry-run parity** — the `markdown` fetch/create/overwrite/patch/diff commands now run the same `check_capability` delegation gate on their dry-run previews as `doc`/`drive` do; a dry-run combined with `--principal-user-id` is verified against the command's real first delegated call before any preview is rendered, and a denied principal blocks the preview.
