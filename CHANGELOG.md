# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/) and this project follows [Semantic Versioning](https://semver.org/).

## [1.0.2] - 2026-03-29

Deep workspace tooling upgrade: pipeline-based input correction, output filtering, enhanced stdin handling, and multi-endpoint routing.

### Added

- Pipeline engine (`internal/pipeline`) for pre-parse and post-parse input correction
  - `AliasHandler`: normalises model-generated flag casing (e.g. `--userId` → `--user-id`)
  - `StickyHandler`: splits glued flag values (e.g. `--limit100` → `--limit 100`)
  - `ParamNameHandler`: fixes near-miss flag typos (e.g. `--limt` → `--limit`)
  - `ParamValueHandler`: normalises structured parameter values after parsing
- Output filtering via `--fields` and `--jq` global flags (`internal/output/filter.go`)
  - `--fields`: comma-separated field selection for top-level keys (case-insensitive)
  - `--jq`: jq expression filtering powered by `gojq` library
- `StdinGuard` for safe single-read stdin across multiple flags in one invocation
- `ResolveInputSource` unified resolver supporting `@file`, `@-` (explicit stdin), and implicit pipe fallback
- `@file` / `@-` syntax support for all string-typed override flags in tool commands
- Chat helper support for `@file` input to read message content from files
- Tool-level endpoint routing (`dynamicToolEndpoints`) for multi-endpoint products
- Comprehensive test suites for pipeline handlers, stdin guard, canonical commands, and chat input

### Changed

- `directRuntimeEndpoint` now accepts tool name for finer-grained endpoint resolution
- `collectOverrides` resolves `@file` / `@-` for all string-typed flags
- `NewRootCommand` refactored to `NewRootCommandWithEngine` with optional pipeline engine
- `schema` command no longer hidden (visible in help output)
- Default output format changed from `table` to `json`

## [1.0.1] - 2026-03-28

Backward-compatible feature and security update after the initial 1.0.0 release.

### Added

- JSON output support for `dws auth login` and `dws auth status`
- Cross-platform keychain-backed secure storage and migration helpers
- Atomic file write helpers to avoid partial config and download writes
- Stronger path and input validation helpers for local file operations
- Install-script coverage for local-source installs

### Changed

- Improved `auth login` help text, hidden compatibility flags, and interactive UX
- Added root-level flag suggestions for common compatibility mistakes such as `--json` and legacy auth flags
- Updated AITable upload parsing to accept nested `content` payloads
- Refreshed bundled skills metadata for the new CLI version

## [1.0.0] - 2026-03-27

First public release of DingTalk Workspace CLI.

### Core

- Discovery-driven CLI pipeline: Market → Discovery → IR → CLI → Transport
- MCP JSON-RPC transport with retries, auth injection, and response size limits
- Disk-based discovery cache with TTL and stale-fallback for offline resilience
- OAuth device flow authentication with PBKDF2 + AES-256-GCM encrypted token storage
- Structured output formats: JSON, table, raw
- Global flags: `--format`, `--verbose`, `--debug`, `--dry-run`, `--yes`, `--timeout`
- Exit codes with structured error payloads (category, reason, hint, actions)

### Supported Services

- **aitable** — AI table: bases, tables, fields, records, templates
- **approval** — Approval processes, forms, instances
- **attendance** — Attendance records, shifts, statistics
- **calendar** — Events, participants, meeting rooms, free-busy
- **chat** — Bot messaging (group/batch), webhook, bot management
- **contact** — Users, departments, org structure
- **devdoc** — Open platform docs search
- **ding** — DING messages: send, recall
- **report** — Reports, templates, statistics
- **todo** — Task management: create, update, complete, delete
- **workbench** — Workbench app query

### Agent Skills

- Bundled `SKILL.md` with product reference docs, intent routing guide, error codes, and batch scripts
- One-line installer for macOS / Linux / Windows
- Skills installed to `~/.agents/skills/dws` (home) or `./.agents/skills/dws` (project)

### Packaging

- Pre-built binaries for macOS (arm64/amd64), Linux (arm64/amd64), Windows (amd64)
- One-line install scripts (`install.sh`, `install.ps1`)
- Project-level skill installer (`install-skills.sh`)
- Shell completion: Bash, Zsh, Fish
