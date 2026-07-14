<h1 align="center">DingTalk Workspace CLI (dws)</h1>

<p align="center"><code>dws</code> — DingTalk Workspace on the command line, built for humans and AI agents.</p>

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i1/O1CN01oKAc2r28jOyyspcQt_!!6000000007968-2-tps-4096-1701.png" alt="DWS Product Overview" width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-green?logo=go&logoColor=white" alt="Go 1.25+">
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue" alt="License Apache-2.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases"><img src="https://img.shields.io/github/v/release/DingTalk-Real-AI/dingtalk-workspace-cli?color=red&label=release" alt="Latest Release"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml"><img src="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src=".github/badges/coverage.svg" alt="Coverage">
</p>

<p align="center">
  <a href="./README_zh.md">中文版</a> · <a href="./README.md">English</a> · <a href="./docs/reference.md">Reference</a> · <a href="./CHANGELOG.md">Changelog</a>
</p>

> [!IMPORTANT]
> **Co-creation Phase**: This project accesses DingTalk enterprise data and requires enterprise admin authorization. Join the DingTalk DWS co-creation group for support and updates. See [Getting Started](#getting-started) below.
>
> <img src="https://img.alicdn.com/imgextra/i1/O1CN01WJyAsJ1prD2ovQACM_!!6000000005413-2-tps-718-720.png" alt="dws Open Source Community DingTalk Group QR Code" width="150">

<details>
<summary><strong>Table of Contents</strong></summary>

- [Why dws?](#why-dws)
- [Installation](#installation)
- [Upgrade](#upgrade)
- [Getting Started](#getting-started)
- [Quick Start](#quick-start)
- [Using with Agents](#using-with-agents)
- [Features](#features)
- [Key Services](#key-services)
- [Security by Design](#security-by-design)
- [Reference & Docs](#reference--docs)
- [Contributing](#contributing)

</details>


---

<h2 id="why-dws">Why dws?</h2>

- **For humans** — `--help` for usage, `--dry-run` to preview requests, `-f table/json/raw` for output formats.
- **For AI agents** — structured JSON responses + built-in Agent Skills, ready out of the box.
- **For enterprise admins** — zero-trust architecture: OAuth device-flow auth + domain allowlisting + least-privilege scoping. **Not a single byte can bypass authentication and audit.**

## Installation

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex
```

<details>
<summary><strong>Skill mode: mono vs multi</strong></summary>

The installer ships skills in one of two layouts. CLI commands (`dws aitable ...`, `dws calendar ...`) are identical in both modes — only the agent-side skill layout differs.

| Mode | What gets installed | Best for |
|------|----------------------|----------|
| **mono** (stable, default) | One `dws` skill covering all products | Cross-product workflows; single entry point |
| **multi** 🧪 **EXPERIMENTAL** | Per-product skills (`dingtalk-aitable`, `dingtalk-calendar`, `dingtalk-chat`, ...) | Single-product tasks; smaller context per call |

> 🧪 **`multi` is currently EXPERIMENTAL / preview.** All product-scoped skills pass the dispatch verifier, but interface, naming and cross-skill references may change in future releases. For production / shared environments, prefer `mono`. File issues if you hit problems.

How to pick:

- **Quick install** (one-liner above): non-interactive, installs `mono`.
- **TTY install** (download then run): `curl -O .../install.sh && bash install.sh` — prompts `1) mono  2) multi` (default 1).
- **Override via env**: `DWS_SKILL_MODE=multi curl -fsSL ... | sh`.
- **Switch later**: `dws skill setup --mode multi` (or `--mode mono`) — re-run any time.

</details>

<details>
<summary>Other install methods</summary>

**npm** (requires Node.js (npm/npx)):

```bash
npm install -g dingtalk-workspace-cli
```

**Pre-built binary**: download from [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases).

> **macOS users**: If you see "cannot be opened because Apple cannot check it for malicious software", run:
> ```bash
> xattr -d com.apple.quarantine /path/to/dws
> ```

**Build from source**:

```bash
git clone https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git
cd dingtalk-workspace-cli
go build -o dws ./cmd       # build to current directory
cp dws ~/.local/bin/         # install to PATH
```

Static endpoint data is generated from the Wukong baseline and committed in this
repository under `internal/syncdata`, so source builds do not require a sibling
data checkout.

> Requires Go 1.25+. Use `make package` to cross-compile for all platforms (macOS / Linux / Windows x amd64 / arm64).

</details>

## China mirror

For users in mainland China, the following channels avoid GitHub network issues. By default (without setting these environment variables) the installer pulls from GitHub.

**1. Install script + pre-built binary (Gitee mirror):**

Repository mirror: `https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli`

```bash
DWS_GITEE_REPO=DingTalk-Real-AI/dingtalk-workspace-cli curl -fsSL https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli/raw/main/scripts/install.sh | sh
```

> With `DWS_GITEE_REPO` set, the installer resolves the latest version and every release asset (binary, checksums, skills) from the Gitee API instead of GitHub. If it is unset, installation defaults to GitHub.

**2. npm package (npmmirror mirror):**

```bash
npm install -g dingtalk-workspace-cli --registry=https://registry.npmmirror.com
```

> npmmirror automatically syncs public packages from the public npm registry, so this works directly in China.

**3. Skills only (Gitee mirror):**

```bash
DWS_GITEE_REPO=DingTalk-Real-AI/dingtalk-workspace-cli curl -fsSL https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli/raw/main/scripts/install-skills.sh | sh
```

> With `DWS_GITEE_REPO` set, `install-skills.sh` resolves the version and skills package from Gitee; it also auto-falls back to the Gitee mirror when GitHub is unreachable.

## Upgrade

> Requires **v1.0.7** or later. For earlier versions, please re-run the [install script](#installation) to upgrade.

dws has built-in self-upgrade capability. Updates are pulled directly from [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases) with SHA256 integrity verification and automatic backup.

```bash
dws upgrade                    # interactive upgrade to latest version
dws upgrade --check            # check for new versions without installing
dws upgrade --list             # list stable release versions
dws upgrade --beta             # upgrade to the latest beta pre-release
dws upgrade --check --beta     # check the beta track without installing
dws upgrade --list --beta      # list beta pre-release versions
dws upgrade --version v1.0.7   # upgrade to a specific version
dws upgrade --version v1.0.8-beta.1  # upgrade to a specific beta version
dws upgrade --rollback         # rollback to the previous version
dws upgrade -y                 # skip confirmation prompt
```

By default, `dws upgrade` follows the stable release track. Use `--beta` only when you explicitly want the newest GitHub pre-release build.

<details>
<summary><strong>How it works</strong></summary>

The upgrade process follows a two-phase atomic flow to ensure consistency:

1. **Prepare** — downloads the platform-specific binary and skill packages to a temporary directory, verifies SHA256 checksums, and extracts/validates all files. If any step fails, the upgrade aborts without modifying the existing installation.
2. **Apply** — only after all preparations succeed, the binary is replaced and skill packages are installed to all detected agent directories (`~/.agents/skills/dws`, `~/.claude/skills/dws`, `~/.cursor/skills/dws`, etc.).

A backup of the current version is automatically created before each upgrade. Use `dws upgrade --rollback` to restore the previous version if needed.

| Flag | Description |
|------|-------------|
| `--check` | Check for updates without installing |
| `--list` | List available stable release versions with changelogs |
| `--beta` | Use the beta pre-release track for `upgrade`, `--check`, or `--list` |
| `--version` | Upgrade to a specific version (e.g. `v1.0.7` or `v1.0.8-beta.1`) |
| `--rollback` | Rollback to the previous backed-up version |
| `--force` | Force reinstall even if already on the latest version |
| `--skip-skills` | Skip skill package update |
| `-y` | Skip confirmation prompt |

</details>

## Getting Started

```bash
dws auth login            # browser opens automatically
dws auth login --device   # for headless environments (Docker, SSH, CI)
```

Select your organization and authorize. That's it.

> If your organization hasn't enabled CLI access, you'll be prompted to send an access request to your admin. Once approved, re-run `dws auth login`.

<details>
<summary><strong>Organization hasn't enabled CLI access?</strong></summary>

1. After selecting your organization, click "Apply Now" to notify the admin
2. The admin receives a request card and can approve with one click
3. Once approved, re-run `dws auth login`

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i2/O1CN01wtsYuQ1CTbboVTlsD_!!6000000000082-2-tps-2696-1544.png" alt="Apply for Access" width="600">
</p>

</details>

<details>
<summary><strong>Admin: Enable CLI access for your organization</strong></summary>

Go to [Developer Platform](https://open-dev.dingtalk.com) → "CLI Access Management" → Enable.

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01M8K7Wj1rZ0WikrZby_!!6000000005644-2-tps-2940-1596.png" alt="CLI Access Management" width="600">
</p>

</details>

<details>
<summary><strong>Custom App mode (CI/CD, ISV integration)</strong></summary>

For enterprise-managed scenarios, create your own DingTalk app:

1. [Open Platform Console](https://open-dev.dingtalk.com/fe/app#/corp/app) → Create App
2. Security Settings → Add redirect URLs: `http://127.0.0.1,https://login.dingtalk.com`
3. Publish the app
4. Login:

```bash
dws auth login --client-id <your-app-key> --client-secret <your-app-secret>
```

Credentials are securely persisted after first login (Keychain). Subsequent runs auto-refresh tokens.

</details>

<details>
<summary><strong>Multiple organizations (profiles)</strong></summary>

`dws` can stay logged in to several DingTalk organizations at once. Each organization is one **profile**; the current profile decides which org a command runs against (credentials are stored per organization).

```bash
dws auth login                              # log in to another org → adds a profile (first login becomes the primary)
dws profile list                            # list logged-in orgs (primary / current marker, status)
dws profile switch <name|corpId>            # switch the default org (use - to toggle back to the previous one)
dws --profile <name|corpId> contact user search --query "..."   # run one command against a specific org, without changing the default
```

Cross-org reads are orchestrated by the agent rather than a built-in `--all-orgs`: list the profiles, run the query per org with `--profile`, then merge. Writes default to the current org only — confirm the target org before writing across orgs.

On macOS, an unreadable registered token slot blocks a new OAuth login rather than risking a mixed Keychain/file-DEK state. If normal terminal commands can still read the login while a sandbox using `DWS_DISABLE_KEYCHAIN=1` cannot, migrate the legacy and profile auth entries without exposing tokens:

```bash
env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run --format json
env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --yes --format json
DWS_DISABLE_KEYCHAIN=1 dws auth status --format json
```

The migration validates every selected auth ciphertext before writing, ignores unrelated application secrets, and can be rerun after an interrupted commit. If validation identifies genuinely damaged ciphertext, remove only the affected profile with `dws auth logout --profile <name|corpId>`, then log in again. Use `dws auth reset` only when you intend to discard every local profile.

</details>

<details>
<summary><strong>Migrate auth between Linux sandboxes</strong></summary>

Copying only `~/.dws/app.json` does not carry the refresh token; access tokens expire after ~2 hours. Use the official export/import flow:

```bash
# Sandbox A (already logged in)
dws auth export -o /tmp/dws-auth.tar.gz
# Or for copy/paste: dws auth export --base64 -o /tmp/dws-auth.b64

# Sandbox B
dws auth import -i /tmp/dws-auth.tar.gz
# Or: dws auth import -i /tmp/dws-auth.b64 --base64
dws auth status   # confirm "Refresh Token: valid"
```

The bundle includes the encrypted keychain under `~/.local/share/dws-cli` (with `auth-token.enc` and `dek`) plus required `~/.dws` config files.

</details>

## Quick Start

```bash
dws contact user search --query "engineering"      # search contacts
dws calendar event list                            # list today's calendar events
dws doc search --query "quarterly"                 # search DingTalk Docs
dws minutes list mine                              # list AI meeting notes I created
dws drive list                                     # list DingTalk drive files
dws todo task create --title "Quarterly report" --executors "<your-userId>"   # create a todo (replace <your-userId>)
dws todo task list --dry-run                       # preview without executing
```

> **Full command list**: [`docs/command-index.md`](./docs/command-index.md) — all commands with descriptions and when-to-use guidance.

## Using with Agents

dws is designed as an AI-native CLI. Complete [Installation](#installation) and [Getting Started](#getting-started) first, then configure your agent:

### Agent Invocation Patterns

```bash
# Use --yes to skip confirmation prompts (required for agents)
dws todo task create --title "Review PR" --executors "<your-userId>" --yes

# Use --dry-run to preview operations (safe execution)
dws contact user search --query "engineering" --dry-run

# Use --jq to extract precisely (save tokens)
dws contact user get-self --jq '.result[0].orgEmployeeModel | {name: .orgUserName, dept: .depts[0].deptName, userId}'
```

### Command Help and Schema

Use Cobra help and Schema for different parts of the command contract:

- `dws <path> --help` is the source of truth for whether a command exists and which flags the binary accepts.
- `dws schema "<path>"` is the Agent contract for command selection, parameter mappings and constraints, risk, and confirmation semantics.
- If Help and Schema disagree, treat it as contract drift: pass only flags accepted by Cobra and use the more conservative safety semantics.
- Schema describes commands; it does not read or search DingTalk business data. Execute the real product command after discovery.

```bash
# Confirm that the command exists and inspect accepted flags
dws aitable record query --help

# Discover within a product, then inspect the selected leaf contract
dws schema aitable
dws schema "aitable record query"

# Execute the real business query
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --limit 10
```

`dws schema --all` exports the complete contract for tooling, CI, audits, and compatibility baselines. Agents should prefer product/group discovery followed by a leaf query to avoid loading the full Catalog into context.

### Agent Skills

The repo ships a complete Agent Skill system under `skills/`, organized into two layouts:

- `skills/mono/` — single-skill layout (one `SKILL.md` + `references/products/`), recommended default.
- `skills/multi/` — per-product skills (`dingtalk-aitable/`, `dingtalk-calendar/`, `dingtalk-chat/`, ...), each with its own `SKILL.md`. 🧪 **EXPERIMENTAL / preview — see banner in each multi `SKILL.md` for caveats.**

Shared reviewed inputs for Schema generation live separately under `internal/cli/schema_hints/`. They are not Agent Skills and are excluded from binaries and release skill bundles.

After installing, AI tools like Claude Code / Cursor can operate DingTalk directly through natural language:

```bash
# Install skills into current project (defaults to mono)
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
```

> `install.sh` installs to `$HOME/.agents/skills/dws` (global); `install-skills.sh` installs to `./.agents/skills/dws` (current project).
>
> China users: prefix `DWS_GITEE_REPO` to use the Gitee mirror — see [China mirror](#china-mirror).

**Switching or re-installing with `dws skill setup`:**

```bash
# Interactive: prompts for mode + target agents
dws skill setup

# Install mono skill to every detected agent home (claude / cursor / codex / opencode / qoder)
dws skill setup --mode mono --target all --yes

# Install multi skills to a single agent home
dws skill setup --mode multi --target cursor --yes

# Point at a local source tree (e.g. a fork or work-in-progress)
DWS_SKILL_SOURCE=/path/to/skills dws skill setup --mode multi
```

| Flag | Values | Description |
|------|--------|-------------|
| `--mode` | `mono` \| `multi` | Skill layout; defaults to interactive prompt |
| `--target` | `all` \| `claude` \| `cursor` \| `codex` \| `opencode` \| `qoder` | Where to install; `all` covers every detected agent home |
| `--source` | path | Local source directory (overrides bundled skills) |
| `--yes` | — | Skip confirmation prompts |

Env vars: `DWS_SKILL_MODE=mono|multi` (also honored by `install.sh` / `install.ps1`), `DWS_SKILL_SOURCE=<path>`.

**What's included (mono layout):**

| Component | Path | Description |
|-----------|------|-------------|
| Master Skill | `skills/mono/SKILL.md` | Intent routing, decision tree, safety rules, error handling |
| Product references | `skills/mono/references/products/*.md` | Per-product command reference (aitable, chat, calendar, etc.) |
| Intent guide | `skills/mono/references/intent-guide.md` | Disambiguation for confusing scenarios (e.g. report vs todo) |
| Global reference | `skills/mono/references/global-reference.md` | Auth, output formats, global flags |
| Error codes | `skills/mono/references/error-codes.md` | Error codes + debugging workflows |
| Recovery guide | `skills/mono/references/recovery-guide.md` | `RECOVERY_EVENT_ID` handling |
| Ready-made scripts | `skills/mono/scripts/*.py` | 13 batch operation scripts (see below) |

<details>
<summary><strong>Ready-made scripts</strong> — 13 Python scripts for common multi-step workflows</summary>

| Script | Description |
|--------|-------------|
| `calendar_schedule_meeting.py` | Create event + add participants + find & book available meeting room |
| `calendar_free_slot_finder.py` | Find common free slots across multiple people, recommend best meeting time |
| `calendar_today_agenda.py` | View today/tomorrow/this week's schedule |
| `import_records.py` | Batch import records from CSV/JSON into AITable |
| `bulk_add_fields.py` | Batch add fields to an AITable data table |
| `upload_attachment.py` | Upload attachment to AITable attachment field |
| `todo_batch_create.py` | Batch create todos from JSON (with priority, due date, executors) |
| `todo_daily_summary.py` | Summarize today/this week's incomplete todos |
| `todo_overdue_check.py` | Scan overdue todos and output overdue list |
| `contact_dept_members.py` | Search department by name and list all members |
| `attendance_my_record.py` | View my attendance records for today/this week/specific date |
| `attendance_team_shift.py` | Query team shift schedules and attendance statistics |
| `report_inbox_today.py` | View today's received reports with details |

</details>

**ISV Integration**: Author your own Agent Skills and orchestrate them with dws skills for cross-product workflows: **ISV Skill → dws Skill → DingTalk Open Platform API (enforced auth + full audit)**.

## Features

<details>
<summary><strong>Personal Event Subscription</strong> — real-time DingTalk messages for event-driven agents</summary>

`dws event consume` subscribes as the currently logged-in user over a managed Stream WebSocket and emits each event as one NDJSON line on stdout. The public catalog currently covers messages that mention the current user, one-to-one messages with a specified user, and messages in a specified group.

> **Prerequisite**: run `dws auth login`. Personal identity is resolved from the OAuth token and cannot be supplied through command-line identity flags.

For an event-focused installation, use the official convenience installer:

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-event.sh | sh
```

```bash
# Inspect the public personal event catalog and schema
dws event list
dws event schema user_im_message_receive_o2o

# Listen for messages that mention the current user
dws event consume user_im_message_receive_at -f ndjson

# Listen for one-to-one messages with a specified user
dws event consume user_im_message_receive_o2o --user <userId> -f ndjson

# Listen for messages in a specified group
dws event consume user_im_message_receive_group --group <openConversationId> -f ndjson

# Inspect local consumers and cancel a subscription
dws event status
dws event stop <subscribe_id>
```

| Feature | Details |
|---------|---------|
| Managed lifecycle | `consume` creates or reuses the personal subscription; `stop` cancels it and cleans local state |
| Shared connection | Consumers for the same user share one local bus and cloud connection |
| Subscription isolation | Normal consumers match both event type and `subscribe_id` |
| Agent-friendly output | Stream events are written to stdout as NDJSON; status and diagnostics use stderr |
| Observability | `status` shows remote subscriptions, the personal bus, and local consumers |
| Cross-platform | Unix Socket on macOS/Linux, Windows Named Pipe on Windows |

See `skills/multi/dingtalk-event/SKILL.md` for the Agent workflow and supported event parameters.

</details>

<details>
<summary><strong>Raw API Access</strong> — call any DingTalk OpenAPI directly</summary>

`dws api` lets you call any DingTalk OpenAPI without an SDK. Tokens are automatically acquired and refreshed.

> **Prerequisite**: Must login with your own app credentials (see [Custom App mode](#getting-started)). Encrypted tokens from MCP default-credential login are not supported for raw API calls.

```bash
# Login (first time only)
dws auth login --client-id <APP_KEY> --client-secret <APP_SECRET>

# === api.dingtalk.com ===

# List all enterprise apps
dws api GET /v1.0/microApp/allApps

# Search users (POST + JSON body)
dws api POST /v1.0/contact/users/search \
  --data '{"queryWord":"engineering","offset":0,"size":10}'

# === oapi.dingtalk.com ===

# Get user details (use --base-url to specify domain)
dws api POST /topapi/v2/user/get \
  --base-url https://oapi.dingtalk.com \
  --data '{"userid":"<USER_ID>"}'

# Or use the full URL directly
dws api POST https://oapi.dingtalk.com/topapi/v2/user/get \
  --data '{"userid":"<USER_ID>"}'

# === General ===
dws api GET /v1.0/microApp/allApps --page-all   # auto-paginate
dws api GET /v1.0/microApp/allApps --dry-run     # preview request
dws api GET /v1.0/microApp/allApps --jq '.agentId'  # jq filtering
```

| Feature | Details |
|---------|----------|
| Dual-form auto-detection | Automatically selects api.dingtalk.com (header auth) or oapi.dingtalk.com (query-param auth) based on URL |
| Automatic token management | App-level accessToken is fetched on first call, cached while valid, auto-refreshed on expiry |
| Domain allowlist | Only `api.dingtalk.com` and `oapi.dingtalk.com` permitted — prevents token leakage |
| Auto-pagination | `--page-all` iterates all pages. `--page-limit` caps the maximum (default 10, set to 0 for unlimited, hard cap at 500 to prevent infinite loops) |

</details>

<details>
<summary><strong>Smart Input Correction</strong> — auto-corrects common AI model parameter mistakes</summary>

Built-in pipeline engine that normalizes flag names, splits sticky arguments, and fuzzy-matches typos:

```bash
# Naming convention auto-conversion (camelCase / snake_case / UPPER -> kebab-case)
dws aitable record query --baseId BASE_ID --tableId TABLE_ID         # auto-corrected to --base-id --table-id

# Sticky argument splitting
dws contact user search --query "engineering" --timeout30           # auto-split to --timeout 30

# Fuzzy flag name matching
dws aitable record query --base-id BASE_ID --tabel-id TABLE_ID       # --tabel-id -> --table-id

# Value normalization (boolean / number / date / enum)
# "yes" -> true, "1,000" -> 1000, "2024/03/29" -> "2024-03-29", "ACTIVE" -> "active"
```

| Agent Output | dws Auto-Corrects To |
|-----------|--------------|
| `--userId` | `--user-id` |
| `--limit100` | `--limit 100` |
| `--tabel-id` | `--table-id` |
| `--USER-ID` | `--user-id` |
| `--user_name` | `--user-name` |

</details>

<details>
<summary><strong>jq Filtering & Field Selection</strong> — fine-grained output control to reduce token consumption</summary>

```bash
# Built-in jq expressions
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --jq '.invocation.params'
dws schema "dev app create" --jq '.tool.required'

# Return only specific fields
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --fields invocation,response
```

</details>

<details>
<summary><strong>Schema Introspection</strong> — Agent command discovery and execution contracts</summary>

```bash
dws schema aitable                                      # discover product commands
dws schema "aitable record query"                       # view the selected leaf contract
dws schema "aitable record query" --jq '.tool.required' # view required fields
dws schema --all                                        # full export for CI/audit/baselines
```

</details>

<details>
<summary><strong>Pipe & File Input</strong> — read flag values from files or stdin</summary>

```bash
# Read message body from a file
dws chat message send-by-bot --robot-code BOT_CODE --group GROUP_ID \
  --title "Weekly Report" --text @report.md

# Pipe content via stdin
cat report.md | dws chat message send-by-bot --robot-code BOT_CODE --group GROUP_ID \
  --title "Weekly Report"

# Read from stdin explicitly
dws chat message send-by-bot --robot-code BOT_CODE --group GROUP_ID \
  --title "Weekly Report" --text @-
```

> **Note**: `@` is treated as the `@<path>` file-injection prefix only when the next character is an ASCII path-shaped character (`A-Z` / `a-z` / `0-9` / `.` / `/` / `~` / `_` / `-`), or `@-` for stdin. Chat-bot payloads like `--text "@所有人 周报"` or `--text "@张三 看一下"` pass through unchanged, so literal mentions reach the API as-is.

</details>

## DingTalk bot — connect a robot to your local AI

`dws dev connect` bridges a DingTalk robot to a local AI CLI (Claude Code / Codex / opencode / Qoder / Gemini, or any tool via `--agent-cmd`): @-mention the bot in a chat and it answers using your local agent, keeping per-conversation multi-turn memory.

```bash
dws dev connect --channel auto --unified-app-id <unifiedAppId>
```

> `--unified-app-id` resolves `clientSecret` at runtime via `dev app credentials get`,
> so the secret never appears in argv (`ps` / journald / shell history). The
> legacy `--robot-client-id <id> --robot-client-secret <secret>` still works but
> the CLI will warn you.

In-chat **session commands** (send the bare command as the whole message — no agent turn, no tokens):

| Command | Effect |
|---------|--------|
| `/new` (aliases `/start`, `/reset`) | Start a fresh session; the previous one is left intact (resumable where the agent supports it) |
| `/clear` | Wipe the current session — disposed through the agent's real session op (opencode issues `DELETE /session/:id`); channels whose agent exposes no delete primitive fall back to a reset |

See [`docs/robot-quickstart.md`](./docs/robot-quickstart.md) for the full 4-step walkthrough (install → create robot → connect → add to a group).

## Key Services

| Service | Command | Capabilities |
|---------|---------|--------------|
| Contact | `contact` | Look up users by name / mobile / job-number, departments, labels & roles, roster profiles & dismissals |
| Chat / IM | `chat` (`im`) | Send / reply / search messages, group & member management, bot & webhook messaging, reactions, recall |
| Calendar | `calendar` | Events CRUD, attendees, meeting rooms, free/busy & time suggestions |
| Todo | `todo` | Create / list / update / complete tasks and comments |
| Approval | `oa` | Approve / reject / revoke / transfer; query pending / initiated / CC instances and forms |
| Attendance | `attendance` | Clock-in records, shifts, summaries, group rules (read-only) |
| Ding | `ding` | Send / recall DING messages |
| Report | `report` | Create / submit logs, inbox & outbox, templates, statistics |
| AI Tables | `aitable` | Bases / tables / records / fields / views, permissions & roles, automation, charts & dashboards, import / export |
| Doc | `doc` | Search / read / write docs, block-level editing, comments, permissions, media, up / download |
| Drive | `drive` | List / search / download, folders, upload, copy / move / rename, permissions |
| Minutes | `minutes` | AI meeting notes: list, summary / keywords / transcription / todos, mind map, speakers, tags |
| Mail | `mail` | Mailboxes, KQL search, read / send, drafts, folders, templates, contacts |
| Sheet | `sheet` | Online spreadsheets: worksheet & range read / write, filters, conditional format, images, CSV |
| Wiki | `wiki` | Knowledge bases: spaces, members, node tree, docs & files |
| DevDoc | `devdoc` | Search the Open Platform docs and diagnose API errors |
| AI Search | `aisearch` | Enterprise people search by name / dept / role / duty / supervisor / phone / job-number |
| Live | `live` | List my live streams |
| Raw API | `api` | Call any DingTalk OpenAPI directly, with managed app-level token |

> Full command listing with usage scenarios: [`docs/command-index.md`](./docs/command-index.md). Run `dws --help` for the top-level tree, or `dws <service> --help` for any service's subcommands.

> **Note on `chat bot`**: bot capabilities (`send-by-bot` / `recall-by-bot` / `add-bot` / `send-by-webhook` / bot search) are merged into the relevant `chat` subtrees (e.g. `dws chat message send-by-bot`, `dws chat group members add-bot`) so the agent-facing command surface stays flat and discoverable. There is no longer a separate top-level `bot` product.

<details>
<summary>Coming soon</summary>

- `conference` (video meetings)
- Multi-skill mode (experimental) — per-product skills under `skills/multi/`; opt in via `dws skill setup --mode multi`

</details>

<h2 id="security-by-design">Security by Design</h2>

`dws` treats security as a first-class architectural concern, not an afterthought. **Credentials never touch disk, tokens never leave trusted domains, permissions never exceed grants, operations never escape audit** — every API call must pass through DingTalk Open Platform's authentication and audit chain, no exceptions.

<details>
<summary><strong>For Developers</strong></summary>

| Mechanism | Details |
|-----------|----------|
| **Encrypted token storage** | **PBKDF2 + AES-256-GCM** encryption, keyed by device physical MAC address; cross-platform Keychain/DPAPI integration provides additional protection — tokens cannot be decrypted on another machine |
| **Input security** | Path traversal protection (symlink resolution + working directory containment), CRLF injection blocking, Unicode visual spoofing filtering — prevents AI Agents from being tricked by malicious instructions |
| **Domain allowlist** | `DWS_TRUSTED_DOMAINS` defaults to `*.dingtalk.com`; bearer tokens are never sent to non-allowlisted domains |
| **HTTPS enforced** | All requests require TLS; HTTP only permitted for loopback during development |
| **Dry-run preview** | `--dry-run` shows call parameters without executing, preventing accidental mutations |
| **Zero credential persistence** | Client ID / Secret used in memory only — never written to config files or logs |

</details>

<details>
<summary><strong>For Enterprise Admins</strong></summary>

| Mechanism | Details |
|-----------|---------|
| **OAuth device-flow auth** | Users must authenticate through an admin-authorized DingTalk application |
| **Least-privilege scoping** | CLI can only invoke APIs granted to the application — no privilege escalation |
| **Allowlist gating** | Admin confirmation required during co-creation phase; self-service approval planned |
| **Full-chain audit** | Every data read/write passes through the DingTalk Open Platform API — enterprise admins can trace complete call logs in real time; no anomalous operation can hide |

</details>

<details>
<summary><strong>For ISVs</strong></summary>

| Mechanism | Details |
|-----------|---------|
| **Tenant data isolation** | Operates under authorized app identity; cross-tenant access is impossible |
| **Skill sandbox** | Agent Skills are Markdown documents (`SKILL.md`) — prompt descriptions only, no arbitrary code execution |
| **Zero blind spots** | Every API call during ISV–dws skill orchestration is forced through DingTalk Open Platform authentication — full call chain is traceable with no bypass path |

</details>

> Found a vulnerability? Report via [GitHub Security Advisories](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/security/advisories/new). See [SECURITY.md](./SECURITY.md).

## Reference & Docs

- [Command Index](./docs/command-index.md) — every runtime command with description and when-to-use guidance
- [Reference](./docs/reference.md) — environment variables, exit codes, output formats, shell completion
- [Architecture](./docs/architecture.md) — static endpoint pipeline, command surface, transport layer
- [Open Platform App Command Routing](./docs/dev-yulan-command-routing.md) — yulan dev app command design, MCP overlay, permission flow, and Agent routing
- [Changelog](./CHANGELOG.md) — release history and migration notes

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for build instructions, testing, and development workflow.

## License

Apache-2.0
