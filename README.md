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
> <a href="https://qr.dingtalk.com/action/joingroup?code=v1,k1,v9/YMJG9qXhvFk5juktYnQziN70rF7QHebC/JLztTVRuRVJIwrSsXmL8oFqU5ajJ&_dt_no_comment=1&origin=11"><img src="https://img.alicdn.com/imgextra/i4/O1CN01Rijgk81gKqVSKMzdx_!!6000000004124-2-tps-654-644.png" alt="DingTalk Group QR Code" width="150"></a>

<details>
<summary><strong>Table of Contents</strong></summary>

- [Why dws?](#why-dws)
- [Installation](#installation)
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
<summary>Other install methods</summary>

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

> Requires Go 1.25+. Use `make package` to cross-compile for all platforms (macOS / Linux / Windows x amd64 / arm64).

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

## Quick Start

```bash
dws contact user search --keyword "engineering"     # search contacts
dws calendar event list                            # list calendar events
dws todo task create --title "Quarterly report" --executors "<your-userId>"   # create a todo (replace <your-userId>)
dws todo task list --dry-run                       # preview without executing
```

## Using with Agents

dws is designed as an AI-native CLI. Complete [Installation](#installation) and [Getting Started](#getting-started) first, then configure your agent:

### Agent Invocation Patterns

```bash
# Use --yes to skip confirmation prompts (required for agents)
dws todo task create --title "Review PR" --executors "<your-userId>" --yes

# Use --dry-run to preview operations (safe execution)
dws contact user search --keyword "engineering" --dry-run

# Use --jq to extract precisely (save tokens)
dws contact user get-self --jq '.result[0].orgEmployeeModel | {name: .orgUserName, dept: .depts[0].deptName, userId}'
```

### Schema Discovery

Agents don't need pre-built knowledge of every command. Use `dws schema` to dynamically discover capabilities:

```bash
# Step 1: Discover all available products
dws schema --jq '.products[] | {id, tool_count: (.tools | length)}'

# Step 2: Inspect target tool's parameter schema
dws schema aitable.query_records --jq '.tool.parameters'

# Step 3: Construct the correct call
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --limit 10
```

### Agent Skills

The repo ships a complete Agent Skill system (`skills/`). After installing, AI tools like Claude Code / Cursor can operate DingTalk directly through natural language:

```bash
# Install skills into current project
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
```

> `install.sh` installs to `$HOME/.agents/skills/dws` (global); `install-skills.sh` installs to `./.agents/skills/dws` (current project).

**What's included:**

| Component | Path | Description |
|-----------|------|-------------|
| Master Skill | `SKILL.md` | Intent routing, decision tree, safety rules, error handling |
| Product references | `references/products/*.md` | Per-product command reference (aitable, chat, calendar, etc.) |
| Intent guide | `references/intent-guide.md` | Disambiguation for confusing scenarios (e.g. report vs todo) |
| Global reference | `references/global-reference.md` | Auth, output formats, global flags |
| Error codes | `references/error-codes.md` | Error codes + debugging workflows |
| Recovery guide | `references/recovery-guide.md` | `RECOVERY_EVENT_ID` handling |
| Ready-made scripts | `scripts/*.py` | 13 batch operation scripts (see below) |

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
<summary><strong>Smart Input Correction</strong> — auto-corrects common AI model parameter mistakes</summary>

Built-in pipeline engine that normalizes flag names, splits sticky arguments, and fuzzy-matches typos:

```bash
# Naming convention auto-conversion (camelCase / snake_case / UPPER -> kebab-case)
dws aitable record query --baseId BASE_ID --tableId TABLE_ID         # auto-corrected to --base-id --table-id

# Sticky argument splitting
dws contact user search --keyword "engineering" --timeout30           # auto-split to --timeout 30

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
dws schema --jq '.products[] | {id, tools: (.tools | length)}'

# Return only specific fields
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --fields invocation,response
```

</details>

<details>
<summary><strong>Schema Introspection</strong> — query parameter schemas before making calls</summary>

```bash
dws schema                                              # list all products and tools
dws schema aitable.query_records                        # view parameter schema
dws schema aitable.query_records --jq '.tool.required'   # view required fields
dws schema --jq '.products[].id'                        # extract all product IDs
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

</details>

## Key Services

| Service | Command | Commands | Subcommands | Description |
|---------|---------|:--------:|-------------|-------------|
| Contact | `contact` | 6 | `user` `dept` | Search users by name/mobile, batch query, departments, current user profile |
| Chat | `chat` | 10 | `message` `group` `search` | Group CRUD, member management, bot messaging, webhook |
| Bot | `chat bot` | 6 | `bot` `group` `message` `search` | Robot creation/search, group/single messaging, webhook, message recall |
| Calendar | `calendar` | 13 | `event` `room` `participant` `busy` | Events CRUD, meeting room booking, free-busy query, participant management |
| Todo | `todo` | 6 | `task` | Create, list, update, done, get detail, delete |
| Approval | `oa` | 9 | `approval` | Approve/reject/revoke, pending tasks, initiated instances, process list |
| Attendance | `attendance` | 4 | `record` `shift` `summary` `rules` | Clock-in records, shift schedules, attendance summary, group rules |
| Ding | `ding` | 2 | `message` | Send/recall DING messages |
| Report | `report` | 7 | `create` `list` `detail` `template` `stats` `sent` | Create reports, sent/received list, templates, statistics |
| AITable | `aitable` | 20 | `base` `table` `record` `field` `attachment` `template` | Full CRUD for bases/tables/records/fields, templates |
| Workbench | `workbench` | 2 | `app` | Batch query app details |
| DevDoc | `devdoc` | 1 | `article` | Search platform docs and error codes |

> 86 commands across 12 products. Run `dws --help` for the full list, or `dws <service> --help` for subcommands.

<details>
<summary>Coming soon</summary>

`doc` (documents) · `mail` (email) · `minutes` (AI transcription) · `drive` (cloud drive) · `conference` (video) · `tb` (Teambition) · `aiapp` (AI apps) · `live` (streaming) · `skill` (marketplace)

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

- [Reference](./docs/reference.md) — environment variables, exit codes, output formats, shell completion
- [Architecture](./docs/architecture.md) — discovery-driven pipeline, IR, transport layer
- [Changelog](./CHANGELOG.md) — release history and migration notes

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for build instructions, testing, and development workflow.

## License

Apache-2.0
