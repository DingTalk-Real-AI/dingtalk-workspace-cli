<h1 align="center">DingTalk Workspace CLI (dws)</h1>

<p align="center"><code>dws</code> — DingTalk Workspace on the command line, built for humans and AI agents.</p>

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i1/O1CN01oKAc2r28jOyyspcQt_!!6000000007968-2-tps-4096-1701.png" alt="DWS Product Overview" width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-green?logo=go&logoColor=white" alt="Go 1.25+">
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue" alt="License Apache-2.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases"><img src="https://img.shields.io/badge/release-v1.1.0-red" alt="v1.1.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml"><img src="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src=".github/badges/coverage.svg" alt="Coverage">
</p>

<p align="center">
  <a href="./README.md">中文版</a> · <a href="./README_en.md">English</a> · <a href="./docs/reference.md">Reference</a> · <a href="./CHANGELOG.md">Changelog</a>
</p>

> [!IMPORTANT]
> **Co-creation Phase**: This project accesses DingTalk enterprise data and requires enterprise admin authorization. Please join the DingTalk DWS co-creation group to complete whitelist configuration. See [Getting Started](#getting-started) below.
>
> <a href="https://qr.dingtalk.com/action/joingroup?code=v1,k1,v9/YMJG9qXhvFk5juktYnQziN70rF7QHebC/JLztTVRuRVJIwrSsXmL8oFqU5ajJ&_dt_no_comment=1&origin=11"><img src="https://img.alicdn.com/imgextra/i1/O1CN01ZqtgeV1cImFmTZPAH_!!6000000003578-2-tps-398-372.png" alt="DingTalk Group QR Code" width="150"></a>

<details>
<summary><strong>Table of Contents</strong></summary>

- [Why dws?](#why-dws)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Quick Start](#quick-start)
- [Key Services](#key-services)
- [Security by Design](#security-by-design)
- [AI Agent Skills](#ai-agent-skills)
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

**Build from source**:

```bash
git clone https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git
cd dingtalk-workspace-cli
make build
```

> The binary installs to `~/.local/bin` by default. If `dws` is not found, add it to PATH: `export PATH="$HOME/.local/bin:$PATH"`

</details>

## Getting Started

### Step 1: Create a DingTalk Application

Go to the [Open Platform Console](https://open-dev.dingtalk.com/fe/app?hash=%23%2Fcorp%2Fapp#/corp/app). Under "Internal Enterprise Apps - DingTalk Apps", click **Create App**.

<details>
<summary>View screenshot</summary>
<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01VIkwvV1a5NQzCIFO0_!!6000000003278-2-tps-2690-1462.png" alt="Create Application" width="600">
</p>
</details>

### Step 2: Configure Redirect URL

Go to app settings → **Security Settings**. Add the following redirect URLs and save:

```
http://127.0.0.1
https://login.dingtalk.com
```

> `http://127.0.0.1` is for local browser login; `https://login.dingtalk.com` is for `--device` device-flow login (Docker containers, remote servers, and other headless environments). We recommend configuring both.

<details>
<summary>View screenshot</summary>
<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN017xQGWb1ycrAG0uxBO_!!6000000006600-2-tps-2000-1032.png" alt="Configure Redirect URL" width="600">
</p>
</details>

### Step 3: Publish the Application

Click "App Release - Version Management & Release" to publish and go live.

<details>
<summary>View screenshot</summary>
<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01WOLZFz244P46B3FPu_!!6000000007337-2-tps-2000-1100.png" alt="Publish Application" width="600">
</p>
</details>

### Step 4: Request Whitelist Access

Join the DingTalk DWS co-creation group and provide your **Client ID** and **admin confirmation** to complete whitelist setup.

### Step 5: Authenticate

```bash
dws auth login --client-id <your-app-key> --client-secret <your-app-secret>
```

Or via environment variables:

```bash
export DWS_CLIENT_ID=<your-app-key>
export DWS_CLIENT_SECRET=<your-app-secret>
dws auth login
```

> CLI flags take precedence over environment variables. Credentials are used for DingTalk's OAuth device flow.

## Quick Start

```bash
dws contact user search --keyword "Alice"          # search contacts
dws calendar event list                            # list calendar events
dws todo task create --title "Quarterly report" --executors "<userId>"   # create a todo
dws todo task list --dry-run                       # preview without executing
```

## Key Services

| Service | Command | Description |
|---------|---------|-------------|
| Contact | `contact` | Users / departments |
| Chat | `chat` | Group management / members / bot messaging / webhook |
| Calendar | `calendar` | Events / meeting rooms / free-busy |
| Todo | `todo` | Task management |
| Approval | `approval` | Processes / forms / instances |
| Attendance | `attendance` | Clock-in / shifts / statistics |
| Ding | `ding` | DING messages / send / recall |
| Report | `report` | Reports / templates / statistics |
| AITable | `aitable` | AI table operations |
| Workbench | `workbench` | App query |
| DevDoc | `devdoc` | Open platform docs search |

Run `dws --help` for the full list, or `dws <service> --help` for subcommands.

<details>
<summary>Coming soon</summary>

`doc` (documents) · `mail` (email) · `minutes` (AI transcription) · `drive` (cloud drive) · `conference` (video) · `tb` (Teambition) · `aiapp` (AI apps) · `live` (streaming) · `skill` (marketplace)

</details>

<h2 id="security-by-design">Security by Design</h2>

`dws` treats security as a first-class architectural concern, not an afterthought. **Credentials never touch disk, tokens never leave trusted domains, permissions never exceed grants, operations never escape audit** — every API call must pass through DingTalk Open Platform's authentication and audit chain, no exceptions.

<details>
<summary><strong>For Developers</strong></summary>

| Mechanism | Details |
|-----------|---------|
| **Encrypted token storage** | **PBKDF2 (600,000 iterations) + AES-256-GCM**, keyed by device MAC address — tokens cannot be decrypted on another machine |
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

## AI Agent Skills

The repo ships Agent Skills (`SKILL.md` files) for every DingTalk product. The install script deploys them to `~/.agents/skills/dws` automatically.

```bash
# Install skills into the current project only
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
```

> `install.sh` installs to `$HOME/.agents/skills/dws` (global); `install-skills.sh` installs to `./.agents/skills/dws` (current project).

### ISV Skill Integration

Author your own Agent Skills and orchestrate them with dws skills for cross-product workflows: **ISV Skill → dws Skill → DingTalk Open Platform API (enforced auth + full audit)**.

**Example**: A CRM Skill invokes the Calendar Skill to create a client meeting, then triggers the Todo Skill to assign follow-ups — the AI agent completes cross-system collaboration in a single conversation.

## Reference & Docs

- [Reference](./docs/reference.md) — environment variables, exit codes, output formats, shell completion
- [Architecture](./docs/architecture.md) — discovery-driven pipeline, IR, transport layer
- [Changelog](./CHANGELOG.md) — release history and migration notes

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for build instructions, testing, and development workflow.

## License

Apache-2.0
