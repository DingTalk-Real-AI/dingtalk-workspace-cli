# Agent identification (agent_code & agentId)

dws tags every MCP request with **which agent host is driving it** and a
**per-instance id**, so usage can be sliced by channel/instance in the data
warehouse. This page is the integration contract.

## What dws sends on the wire

| Header | Meaning | Granularity |
|--------|---------|-------------|
| `x-dingtalk-dws-agent-code` | which agent host (claudecode / codex / qoder / cursor / custom …) | channel |
| `x-dws-agent-instance-id` | `dwsa_<base62>` derived from `machineId + agent_code` | machine × channel |
| `x-dws-agent-id` | stable per-install machine id (v1-compatible) | machine |
| `X-Cli-Version` | dws CLI version (segments old vs new clients) | — |

`x-dws-agent-id` keeps its original machine-level meaning for backward
compatibility; `x-dws-agent-instance-id` is the new per-channel value. Old
clients send no `agent_code` / instance id — treat their absence as
"legacy/unknown", not an error.

## How `agent_code` is resolved (confidence ladder)

1. **T0 — explicit declaration:** `DINGTALK_DWS_AGENTCODE=<code>`. **Use this.**
2. **T1 — verified env signature:** an agent that auto-sets a distinctive var
   (`CLAUDECODE`, `CODEX_SANDBOX`, `OPENCLAW_BUNDLE_ROOT`, `HERMES_HOME`).
3. **T2 — `VSCODE_BRAND`:** every VS Code fork declares its brand — one rule
   covers Cursor / Windsurf / Trae / Qoder / Kiro / … incl. future forks.
4. **T3 — macOS `__CFBundleIdentifier`:** known agent app bundles.
5. **T4 — `custom`:** unknown host. Never guessed.

## Declaring your agent (recommended — the only fully-general path)

Auto-detection cannot cover every agent: most terminal agents (gemini/
antigravity, aider, opencode, qwen-code, crush, goose, kimi, amazon-q,
continue, …) expose **no reliable self-identifying env var** — only user-set
API keys, which must not be used as identity. The robust answer is: **the host
sets `DINGTALK_DWS_AGENTCODE` in the env block where it launches dws as an MCP
server.** This is accurate for any agent, on any OS, and is future-proof.

MCP server config example (JSON-style hosts):
```jsonc
{
  "mcpServers": {
    "dingtalk-workspace": {
      "command": "dws",
      "args": ["mcp", "..."],
      "env": { "DINGTALK_DWS_AGENTCODE": "your-agent-code" }
    }
  }
}
```

### Canonical codes

`claudecode`, `codex`, `cursor`, `vscode`, `qoder`, `windsurf`, `trae`,
`workbuddy`, `openclaw`, `hermes`, `codebuddy`, `comate`, `lingma`, `gemini`,
`aider`, `opencode`, `goose`, `crush`, `kimi`, `amazonq`, `continue`, …
Use a stable lowercase slug; unknown values are kept as-is (lowercased,
spaces stripped), so a new agent name flows through cleanly.

## Trust & limitations — READ THIS

**`agent_code` AND the ids (`x-dws-agent-id`, `x-dws-agent-instance-id`) are
self-reported, best-effort signals, NOT an authenticated identity.**

- `agent_code`: every declaration/auto-detect signal is an env var the
  host/user controls — spoofable (`export CLAUDECODE=1` → dws reports
  `claudecode`).
- The ids are **even easier to forge**: they are generated, stored, and sent
  entirely client-side. `machineId` is a random UUID in the plaintext
  `~/.dws/identity.json` (which the user owns), and the instance id is just
  `sha256(machineId + agent_code)`. Editing that one file — or rewriting the
  header — lets anyone mint, split, rotate, or impersonate ids at will. The
  `dwsa_` prefix does NOT make it a secure identifier.

- ✅ **Fit for statistics / observability** (the intended use): there is no
  incentive to misreport one's own agent, and real hosts emit real signals, so
  aggregate per-channel metrics are reliable in practice.
- ❌ **NOT fit for authentication, authorization, rate-limiting, billing, or
  revocation.** Anything where a party benefits from lying must not trust this
  field. For control-plane use you need a gateway-issued **authoritative**
  agentId bound to a verified credential (clientId / PAT / OAuth) — a separate,
  heavier mechanism, deliberately out of scope here.

Treat `agent_code` / `x-dws-agent-instance-id` as analytics dimensions only.

## Gateway side (required for the data to land)

dws sending the headers is necessary but not sufficient. The gateway must:
1. add `x-dingtalk-dws-agent-code`, `x-dws-agent-instance-id`, `X-Cli-Version`
   to the upstream-header pass-through allowlist (otherwise they are stripped);
2. log them as fields, and deliver them to the warehouse (alongside the
   existing flow-control / execution logs).
