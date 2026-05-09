# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/) and this project follows [Semantic Versioning](https://semver.org/).

## [1.0.23] - 2026-05-08

A single fix for HTTP proxy support across the CLI's custom HTTP transports. No behaviour changes elsewhere.

### Fixed

- **`HTTP_PROXY` / `HTTPS_PROXY` environment variables silently ignored by all custom transports** (#237, fixes #236) — the three custom `http.Transport` instances built by the CLI (`internal/transport/client.go` MCP transport, `internal/apiclient/client.go` DingTalk OpenAPI client, `internal/app/legacy.go` IPv4-forcing registry client) all set `DialContext` / `TLSClientConfig` / timeouts but omitted the `Proxy` field. Per Go's `net/http` contract, a non-nil Transport without an explicit `Proxy` means "no proxy" — env vars are silently ignored, breaking sandboxed or air-gapped deployments that route outbound through `HTTP_PROXY` / `HTTPS_PROXY`. All three transports now set `Proxy: http.ProxyFromEnvironment`.

### Tests

- Per-package regression test that pointer-compares the Transport's `Proxy` func against `http.ProxyFromEnvironment`, avoiding flakiness from Go's `envProxyOnce` memoisation when running alongside tests that read proxy env early. (#237)

## [1.0.22] - 2026-05-07

Two release-blocking bug fixes: `dws attendance summary` now exposes the server-required `--stats-type` flag (without it, every call returned C0002), and the install scripts finally populate `~/.hermes/skills/dws/` for users who already have Hermes.

### Fixed

- **`dws attendance summary` returned C0002 (统计类型错误) on every call** (#228, fixes #227) — the DingTalk MCP tool `get_attendance_summary` requires `statsType` at the business layer even though the schema marks it optional. The CLI did not expose any way to set it, so the command was 100% unusable. A new `--stats-type` flag (`week` / `month`) is now plumbed through to `QueryUserAttendVO.statsType`; the flag is documented as required in the long help, flag description, and `skills/references/products/attendance.md`.
- **Install scripts skipped `.hermes/skills/` when populating skill directories** (#221, fixes #188) — the `AGENT_DIRS` lists across `build/npm/install.js`, `scripts/install.sh`, `scripts/install.ps1`, `scripts/install-skills.sh` and the four upgrade-path mirrors (8 sources total once review feedback was addressed) did not include `.hermes/skills`, so users with Hermes installed were not getting `~/.hermes/skills/dws/` populated automatically. The existing parent-directory gate keeps this zero-side-effect for users without Hermes.

### Tests

- New `--stats-type` regression coverage in `test/cli_compat/attendance_test.go` — verifies `statsType` is written to `QueryUserAttendVO` when set to `month` or `week`, and is omitted when not provided. (#228)

## [1.0.21] - 2026-05-05

A single critical routing fix for `dws drive` commands. No new commands or behaviour changes elsewhere.

### Fixed

- **`dws drive mkdir` / `dws drive download` silently routed to the doc MCP server** (#220, fixes #219) — when two MCP servers register tools with the same name (e.g. both `drive` and `doc` expose `create_folder`), the tool-level endpoint map used last-writer-wins, so drive-side calls landed on the doc endpoint and returned mock-shaped responses (`success: true` with a fake `folderId`) without actually creating anything. `directRuntimeEndpoint` now resolves product-level first when the caller already knows the productID, and only falls back to the tool-level lookup when productID is empty. The wrong-server collision and the resulting "succeeded but didn't" behaviour are gone.

## [1.0.20] - 2026-05-04

Documentation polish and a login regression fix. No behaviour changes outside the login MCP refresh path.

### Fixed

- **Login no longer reuses stale `clientId` from an old MCP cache** (#213) — `dws login` now unconditionally re-fetches the MCP descriptor, so a previously cached client id can't keep producing auth errors after the server rotates it.

### Docs

- **`dws chat message list` pagination** (#218, fixes #195) — clarifies that `nextCursor` is opaque and must be passed back as `--cursor` exactly; warns against parsing or reusing it as an offset.
- **`dws contact search` examples** (#209) — switched from the removed `--keyword` flag to the current `--query`.
- **`dws todo` help text** (#205) — expanded field semantics so MCP wrappers generate accurate schemas.
- **`dws chat message send-by-bot` and `dws report create` help** (#217, #106, #107) — `--robot-code` / `--title` / `--text` now carry the `(必填)` marker; `report create --contents` documents the `key=field_name` requirement and rewrites examples as a `template detail → create` two-step pipeline.
- **CHANGELOG backfill for 1.0.19** (#204).

## [1.0.19] - 2026-04-30

Discovery hardening for edition overlays: `edition.SupplementServers` / `FallbackServers` hooks now consistently surface through the **runtime catalog loader**, not just the static command tree, so overlay products that live outside the Portal envelope (e.g. Wukong gray-release `conference`) resolve an endpoint on both the cold-cache and tool-not-in-catalog paths. Ships with per-edition cache partitioning to stop cross-edition disk-cache leakage, plus a small todo fix.

### Added

- **`pkg/config.EditionPartition(name)`** (#197) — returns the cache partition key for a given edition. Open-source core (`""` / `"open"`) keeps using `DefaultPartition` (`default/default`); every other edition gets its own namespace (`<edition>/default`), preventing cross-edition data leakage in the shared `~/.dws` disk cache. Lives in `pkg/config` as a leaf helper so `internal/cli`, `internal/app`, and `internal/cache` can all call it without risking import cycles.
- **`internal/editionmerge` shared package** (#197) — single source of truth for converting `edition.ServerInfo` into `market.ServerDescriptor` (`ToDescriptor`) and for merging `SupplementServers` / `FallbackServers` into a descriptor list. Both `internal/cli` (command tree) and `internal/app` (runtime catalog) now apply the edition hooks against the same discovery pipeline.

### Changed

- **`EnvironmentLoader.loadFromCache` honors `SupplementServers` even on empty registry** (#197) — when the Portal registry cache is missing or empty, the catalog loader still materialises the edition's `SupplementServers` as endpoint-only `discovery.RuntimeServer` entries (source: `edition_supplement`), so hardcoded overlay commands for supplement-only products can still resolve an endpoint via the catalog path. Previously `loadFromCache` short-circuited to an empty catalog whenever the registry snapshot was empty, silently dropping gray-release products.
- **Cache loader switches from `DefaultPartition` to `EditionPartition(edition.Get().Name)`** (#197) — the runtime catalog, registry snapshot, and tools snapshot are now partitioned per edition instead of all editions sharing `default/default`.
- **`loadFromCache` appends supplement servers alongside fresh-cache servers** (#197) — supplement entries whose `CLI.ID` / `Key` are already present in the cached registry are skipped, so the hook never shadows Portal-published servers; only new products are added.
- **`runtimeRunner.Run` falls through to `directRuntimeEndpoint` for supplement products** (#197) — when the catalog contains the product (e.g. supplied by `SupplementServers`) but the specific tool is not declared, the runner now trusts `directRuntimeEndpoint` to resolve a working endpoint for the tool before returning the explicit catalog-miss error. Supplement entries intentionally carry no tool list, so this is the path that makes overlay-only tools executable.
- **Legacy `mergeSupplementServers` / `fallbackToDescriptors` moved out of `internal/app/legacy.go`** (#197) — relocated into `internal/editionmerge` and reused by the catalog loader, eliminating the duplicate `edition.ServerInfo → market.ServerDescriptor` logic that previously only ran on the static command-tree path.

### Fixed

- **`dws todo task get` returns empty** (#202) — the helper was calling `query_todo_detail`, which is not a valid MCP tool and returns empty. Switched to `get_todo_detail` as declared in `discovery.json`, restoring correct task-detail behaviour.
- **Conference and other Wukong gray-release products miss endpoint on cold cache** (#197) — products registered only via `edition.SupplementServers` (not yet in the Portal envelope) now resolve an endpoint through the catalog path in both cold-start and tool-not-declared scenarios.

### Tests

- `internal/editionmerge/merge_test.go` — descriptor conversion + supplement/fallback merge semantics.
- `internal/cli/loader_partition_test.go` + `loader_supplement_test.go` — edition-partitioned cache reads and supplement hook surfacing from `loadFromCache` (including empty-registry cold path and existing-ID deduplication).
- `internal/app/legacy_wukong_partition_e2e_test.go` — end-to-end cache partition isolation for the Wukong edition.
- `internal/app/runner_supplement_fallback_test.go` — runner falls through to `directRuntimeEndpoint` when the tool isn't declared by a supplement-sourced catalog entry.
- `pkg/config/constants_test.go` — `EditionPartition` name handling (`""`, `"open"`, custom edition).

### Docs

- **CHANGELOG v1.0.18 rewrite** (#193) — previous release notes expanded to call out the PAT host-owned A-core flow, exit-code contract change (auth `4`, Discovery/cache/protocol `6`), `dws pat chmod` / `pat browser-policy` entry points, stderr-JSON classifier updates, and host-control metadata injection.

## [1.0.18] - 2026-04-28

Raw DingTalk OpenAPI access lands as a new `dws api` surface for both `api.dingtalk.com` and `oapi.dingtalk.com`, backed by app-level token caching and guarded host allowlists. PAT enters the host-owned **A-core** loop: agent hosts can own authorization UI through `DINGTALK_DWS_AGENTCODE`, parse single-line stderr JSON, call `dws pat chmod`, and replay the original command. Chat helper regressions are fixed, skill references are brought back in line with shipped commands, and the v1.0.17 Mail release notes are backfilled into README / CHANGELOG.

### Breaking

- **PAT exit-code contract** (#142) — PAT authorization interceptions now use exit code `4`; Discovery, cache, and protocol negotiation failures now use exit code `6`. Downstream scripts that previously treated `4` as Discovery must update their handling.

### Added

- **`dws api` raw DingTalk OpenAPI command** (#184) — direct DingTalk OpenAPI calls without writing an MCP wrapper first. Supports `GET` / `POST` / `PUT` / `PATCH` / `DELETE`, JSON `--params` / `--data`, stdin input, dry-run previews, `--jq`, field selection, `--page-all`, `--page-limit`, `--page-delay`, and `--base-url`.
- **Dual-form OpenAPI routing** (#184) — `api.dingtalk.com` requests use the `x-acs-dingtalk-access-token` header; `oapi.dingtalk.com` requests use the legacy `access_token` query parameter. The raw API client validates the target host before attaching credentials.
- **App-level token cache for raw API** (#184) — custom-app credentials now fetch app access tokens from the unified OAuth endpoint, cache them while valid, and refresh them before expiry. The same token provider works for new-style and legacy OpenAPI calls.
- **Host-owned PAT A-core flow** (#142) — when `DINGTALK_DWS_AGENTCODE` is set, PAT hits return `exit=4` plus single-line stderr JSON; the host renders authorization UI, calls `dws pat chmod <scope>...`, and replays the original command.
- **`dws pat chmod` authorization entry point** (#142) — grants scopes with `--agentCode`, `--grant-type`, and session fallback support; `DINGTALK_DWS_AGENTCODE` can supply the agent code when the flag is omitted.
- **PAT browser-open policy** (#142) — `dws pat browser-policy --enabled <true|false> [--agentCode <id>]` controls whether the CLI may open a browser, independently from `--format` output mode.

### Changed

- **README raw API guide** (#184) — English and Chinese READMEs now document custom-app prerequisites, api/oapi examples, auto-pagination, dry-run, jq filtering, security properties, and the new Raw API service-table row.
- **Raw API token retrieval path** (#184) — token lookup now goes through a single app-token interface; stale auth-refresh retry helpers were removed from the raw API path.
- **PAT stderr JSON classifier** (#142) — recognizes `code`, `errorCode`, and `error_code`, including `PAT_NO_PERMISSION`, risk-tier PAT errors, `PAT_SCOPE_AUTH_REQUIRED`, and `AGENT_CODE_NOT_EXISTS`.
- **Host-control metadata injection** (#142) — classifier and active-retry paths now share one mutation point for `data.hostControl` and `data.openBrowser`, keeping host-facing JSON shapes aligned.
- **Open-edition routing signals** (#142) — open edition pins `claw-type: openClaw`; `DINGTALK_AGENT`, `DWS_CHANNEL`, and host-owned PAT detection are kept as independent signals.
- **Behavior authorization endpoint fallback** (#142) — the PAT runtime can resolve the built-in behavior-authorization MCP endpoint before discovery data is available.
- **v1.0.17 documentation backfill** (#181) — the previous release notes and README service table now explicitly include the shipped Mail product, update the total to **163 commands across 14 products**, and remove Mail from "Coming soon".

### Fixed

- **CLI auth-denial attribution** — local CLI authorization denials are attributed to the channel before falling back to user-scope classification, avoiding user-scope misclassification for channel-level auth failures.
- **Opaque authorization URLs** (#182, #142) — PAT authorization links are preserved verbatim, including query/hash/fragment content required by the server.
- **Polling compatibility** (#182, #142) — device-flow result envelopes and no-`flowId` device-code fallback remain supported, with guarded debug output and envelope priority.
- **Group chat @-mentions restored** (#180) — `dws chat message send --group ...` again accepts and forwards `--at-users`, `--at-all`, and `--at-mobiles`; those flags are rejected outside group-chat mode so single-chat sends cannot silently drop @-mention intent.
- **Explicit members-list command restored** (#180) — `dws chat group members list --id <openConversationId>` is reachable after the helper/dynamic merge path changed. `cmdutil.MergeHardcodedLeaves` now honors higher-priority helper groups when a dynamic envelope contributes a leaf at the same path.
- **Skill reference command names** (#186) — `simple.md` now uses shipped OA command names (`list-pending`, `list-initiated`), removes a non-existent devdoc `search-error` command, and marks `workbench.md` as Draft because workbench commands are not available in the runtime.
- **Empty grant result handling** (#142) — `dws pat chmod` now returns an explicit error instead of treating `{"Content": null}` as success.
- **Session-id log safety** (#142) — raw `DWS_SESSION_ID` / `REWIND_SESSION_ID` values are no longer logged when the two env vars disagree.

### Tests

- Added raw API coverage for request validation, api/oapi routing, token management, pagination, response handling, dry-run output, JSON parsing, stdin handling, and command wiring. (#184)
- Added chat/cmdutil regression tests for group @-mention forwarding, single-chat rejection, `members list`, helper-vs-envelope shape mismatch, and merge-priority behavior. (#180)
- Added PAT contract coverage for host-owned signal selection, single-line stderr JSON, chmod env fallback and legacy alias fallback, browser policy, direct-runtime PAT endpoint fallback, and retry/poll behavior. (#142)
- Coverage badge refreshed after the post-v1.0.17 CI runs.

## [1.0.17] - 2026-04-27

New **Mail** product surface (mailbox list, KQL message search, message get, send) brings runtime command count to **163 across 14 products**. Plugin command-tree visibility hardening: stdio plugins shipping CLI overlays no longer wait on subprocess discovery to surface their commands, and overlay-registered plugin products are no longer hidden by edition `VisibleProducts` whitelists. Chat docs clarify that `--title` is required on `dws chat message send`.

### Added

- **`mail` product** (#167) — new top-level service for DingTalk Mail. Four leaf commands across two subgroups:
  - `dws mail mailbox list` — list mailbox addresses available to the current user (`list_user_mailboxes`)
  - `dws mail message search` — KQL search across folders / sender / date / attachments / read-state (`search_emails`); supports `--cursor` pagination
  - `dws mail message get` — fetch full message body + headers + attachments by message ID (`get_email_by_message_id`)
  - `dws mail message send` — send email to one or more recipients (`send_email`)
  - Skill reference at `skills/references/products/mail.md` registered in `skills/SKILL.md` master index and intent decision tree
- **Stdio plugin overlay-first command registration** (#179) — when a stdio plugin's `overlay.json` declares `toolOverrides`, command trees are built from manifest metadata synchronously at startup, no subprocess `Initialize` / `tools/list` handshake required. Previously, slow or failing subprocesses left plugin commands invisible in `dws --help`. Background discovery still runs to refresh the warm cache for richer flag types on subsequent startups.

### Changed

- **`hideNonDirectRuntimeCommands` / `visibleMCPRootCommands` / `visibleUtilityRootCommands`** (#179) — refactored to share a single `resolveVisibleProducts()` helper that **unions** the edition's `VisibleProducts` hook with `DirectRuntimeProductIDs()`, so plugins registered via `AppendDynamicServer` stay visible in `dws --help` even when an edition installs a static product whitelist. Previously the hook fully replaced the dynamic registry, silently hiding plugin commands.
- **`dws chat message send` documentation clarifies `--title` is required** (#174) — the helper command short text and the chat skill reference now state explicitly that `--title` is mandatory for both group and single-chat sends, matching the runtime validation.
- **`buildStdioCommands` refactored to share helpers with the overlay-first path** (#179) — overlay parsing (`resolveStdioOverlay`) and tools→DetailTool conversion (`toolsToDetails`) extracted as package-level helpers; the legacy discovery-first stdio path now delegates to them, eliminating duplicated overlay JSON / cache-snapshot logic.

### Fixed

- **Negative-cache poisoning guard for stdio plugin discovery** (#179) — `refreshStdioToolsCache` now skips `SaveTools` entirely when discovery returns an empty tool list (transient failure, subprocess not ready, RPC timeout), so a single bad refresh cannot overwrite a previously-good cache and degrade flag enrichment on the next startup.

### Tests

- 6 new test cases in `internal/app/plugin_stdio_overlay_test.go` and `internal/app/visibility_test.go` cover overlay-first registration without discovery, warm-cache flag enrichment from `InputSchema`, fallback when overlays lack `toolOverrides`, the cache-poisoning guard, and integration cases for plugin visibility under restrictive `VisibleProducts` whitelists.
- Coverage 49.8% → 52.8%.

## [1.0.16] - 2026-04-24

Discovery service abstraction with schema v3 extensions, open-edition helper-subtree restoration, and a defensive device-flow login reset.

### Added

- **`internal/discovery` service abstraction** (#156) — encapsulates market registry fetch, MCP runtime negotiation (`initialize → tools/list → detail` merge), and multi-level cache fallback. `EnvironmentLoader` now does cache-first startup, with degraded-mode reasons (`unauthenticated` / `market_unreachable` / `runtime_all_failed`) and `UpdatedAt`-based selective re-discovery.
- **Schema v3 extensions** (#156) — positional parameters with typed coercion, `Example` on `--help`, flag `Default` / `RuntimeDefault` (with `$currentUserId` / `$now` etc.), `BodyWrapper`, `MutuallyExclusive` / `RequireOneOf` flag groups, `OmitWhen`, explicit `Type` override, and detail-schema `default` propagation.
- **`dws chat message send` destination-flag routing** (#170) — open edition gains a hardcoded helper that dispatches by `--group` (→ `send_message_as_user`) vs `--user` / `--open-dingtalk-id` (→ `send_direct_message_as_user`), mirroring the closed-source overlay so single-chat sends finally work end-to-end.

### Changed

- **`pickCommands` → `cmdutil.MergeHardcodedLeaves`** (#169) — when a top-level product name collides between the dynamic overlay and a helper subtree, helper-only siblings are grafted into the dynamic tree instead of dropped. Restores `dws chat message send-by-bot` / `recall-by-bot` / `send-by-webhook` and `dws chat group members add-bot`, which had silently vanished from the open edition.
- **`OverridePriority` / `MergeHardcodedLeaves` promoted into `pkg/cmdutil`** (#170) — single source of truth for the merge layer; hardcoded leaves can opt into overriding the dynamic envelope via a strictly higher priority.

### Fixed

- **Device flow defensively resets credentials before login** (#157) — `--device` login now clears stale credential state and re-fetches `clientID` from the MCP server, regardless of what previous login methods (OAuth scan, PAT) left in `app.json`. Fixes the case where a prior OAuth login made `--device` fall back to direct mode and demand `clientSecret`.

## [1.0.15] - 2026-04-23

Compat layer gains **subcommand merging** under shared parents so multiple server entries can contribute into the same `dws <parent> <branch>` subtree without producing duplicate `--help` rows. Ships with a fresh auto-generated command index doc, a README sync to **159 commands across 13 products**, and a wide-ranging flag-naming cleanup that standardises CLI flags across chat, calendar, drive, minutes, contact, and devdoc commands.

### Added

- **`internal/compat` subcommand merging via `attachOrMerge`** — when two or more server entries attach to the same parent (e.g. `parent: "chat"`) and their `cli.command` collides with an existing subcommand in the parent's tree, the new subcommand's children are merged recursively into the existing one instead of creating a duplicate sibling. Leaf-name collisions resolve first-wins. Fixes the "double `group` / `message` rows in `dws chat --help`" symptom when bot capabilities are distributed across `chat.group.members` and `chat.message`.
- **`docs/command-index.md`** — a single, English, auto-generated listing of every runtime command the `dws` CLI exposes under the pre environment (159 total). Each entry carries a description and a "when to use" column aimed at AI agents. Replaces the earlier `command-index.pre.*` / `command-index.full.*` ad-hoc snapshots.

### Changed

- **README Key Services table** (`README.md` + `README_zh.md`) fully synced to the shipped command surface:
  - `Chat`: 20 → **23** (bot capabilities merged in; new `list-all` / `list-focused` / `list-unread-conversations` / `conversation-info` exposed)
  - `Calendar`: 13 → **14**
  - `AI Tables`: 37 → **41** (chart / dashboard public-share config rows)
  - `Doc`: 16 → **21** (comment subtree + `file create`)
  - `Minutes`: 22 → **19** (single-tool `record`, `list query`, `list-by-keyword-range` pruned)
  - New `Drive` row (6 commands) — promoted out of "Coming soon"
  - `Workbench` row and standalone `Bot` row removed
  - Total revised to **159 commands across 13 products**
- **Quick Start** expanded to 7 examples covering `doc`, `minutes`, `drive` in addition to `contact`, `calendar`, `todo`
- **Coming soon** trimmed to 5: `mail`, `conference`, `aiapp`, `live`, `wiki`
- **Reference & Docs** section now leads with a pointer to the new `docs/command-index.md`
- **Flag naming cleanup** — CLI flags across chat, calendar, drive, minutes, contact, and devdoc have been standardised so the names users type match the product-skill documentation. Notable flags:
  - `dws contact user search` / `dws contact dept search` / `dws devdoc article search` now take `--query` (previously `--keyword`)
  - `dws chat message list` / `dws chat message search` / `dws chat message list-mentions` / `dws chat conversation-info` / `dws chat message send` now take `--group` for the target conversation (previously `--id`) and `--open-dingtalk-id` (previously `--open-id`)
  - `dws chat message list-by-sender` now takes `--sender-user-id` / `--sender-open-dingtalk-id` (previously `--user` / `--open-id`)
  - `dws chat message list-topic-replies` now takes `--group` / `--topic-id` / `--limit` / `--time` (previously `--id` / `--topic` / `--size` / `--start`)
  - `dws chat search-common` now takes `--match-mode` (previously `--mode`)
  - `dws drive list` now takes `--max` / `--thumbnail` (previously `--max-results` / `--with-thumbnail`)
  - `dws calendar event suggest` now takes `--users` / `--duration` / `--timezone` (previously `--attendee-user-ids` / `--duration-minutes` / `--time-zone`)
  - `dws minutes list mine` / `dws minutes list shared` now take `--max` (previously `--max-results`) and gain `--query` / `--start` / `--end`
  - `dws minutes list all` no longer exposes the legacy `--__scope__` internal alias
- **Flag coverage additions** — `dws calendar event create` / `update` gain `--attendees`, `--open-dingtalk-ids`, `--timezone`; `dws chat message send` gains file-message flags (`--dentry-id`, `--file-name`, `--file-size`, `--file-type`, `--media-id`, `--msg-type`, `--space-id`) plus `--open-dingtalk-id` / `--user`; `dws chat message list` gains `--open-dingtalk-id` / `--user`; `dws aitable table delete` gains `--reason`; `dws calendar participant add` gains `--optional`; `dws todo task create` gains `--recurrence`.

### Tests

- 3 new unit tests in `internal/compat/dynamic_commands_test.go`:
  - `TestBuildDynamicCommands_ParentMergeSameName` — two servers with identical `command` + `parent` collapse into a single merged subcommand
  - `TestBuildDynamicCommands_ParentMergeRecursive` — recursive merge through nested groups (e.g. `chat.group.members`)
  - `TestBuildDynamicCommands_ParentMergeLeafCollision` — identical leaf paths resolve first-wins without producing duplicates

## [1.0.14] - 2026-04-22

Docs-only re-tag of v1.0.13. The single commit (#153) backfills the v1.0.13 release notes after the binary was already published; no functional or CLI surface change.

## [1.0.13] - 2026-04-22

IM / Messaging capability expansion: the `chat` (aka `im`) product surface grows from "group + bot messaging" into a full conversational layer — user-identity messaging, message reading & search, personal messages, topic replies, mentions, focused contacts, unread/top/common conversations, org-wide group creation, and first-class bot lifecycle.

### Added

- **`dws im` alias** — `dws im` is now registered as an alias of `dws chat` for intent clarity
- **User-identity messaging** (`chat message send`) — send group or 1-on-1 messages as the current user
  - Recipient selection is mutually exclusive: `--group <openConversationId>` / `--user <userId>` / `--open-dingtalk-id <openDingTalkId>`
  - Markdown text via `--text` (or positional arg), optional `--title`
  - Group-only: `--at-all` to @everyone, `--at-users` for per-member @mentions
  - Image messages via `--media-id` (obtained from `dt_media_upload`)
- **Personal messages** (`chat message send-personal`) — sensitive personal-channel send (⚠️ destructive/dangerous op, requires confirmation)
- **Conversation read paths**:
  - `chat message list` — pull group / 1-on-1 conversation messages
  - `chat message list-all` — pull all conversations for the current user in a time range
  - `chat message list-topic-replies` — pull group topic reply threads
  - `chat message list-by-sender` — messages by a specific sender
  - `chat message list-mentions` — messages where the current user was @-mentioned
  - `chat message list-focused` — messages from focused / starred contacts
  - `chat message list-unread-conversations` — unread conversation list
  - `chat message search` — keyword search across conversations
  - `chat message info` — conversation metadata
  - `chat list-top-conversations` — pinned conversation list
- **Group creation & discovery**:
  - `chat group create-org` — create an organization-wide group
  - `chat search-common` — search groups shared with a nickname list (`--nicks`, `--match-mode AND|OR`, cursor-based pagination)
- **Bot lifecycle**:
  - `chat bot create` — create an enterprise bot
  - `chat bot search-groups` — search the groups a bot is present in

### Changed

- **`chat` skill reference** (`skills/references/products/chat.md`, #148) restructured into three sub-groups — `group` (9) / `message` (15) / `bot` (3) — with refreshed intent-routing table, workflow examples, and context-passing rules aligned with `dws-service-endpoints.json` (16 new group-chat tool overrides + 2 new bot tool overrides)
- **README Key Services** sync:
  - `Chat` row: 10 → 20 commands; subcommand tags expanded to `message` `group` `search` `list-top-conversations`
  - `Bot` row: 6 → 7 commands; subcommand tags expanded with `create` `search-groups`
  - Total raised to **152 commands across 14 products**

## [1.0.12] - 2026-04-21

Product-surface expansion: first-class `doc` (DingTalk Docs) and `minutes` (AI Minutes) skill references, refreshed `aitable` guide aligned with the shipped binary (including dashboard / chart / export), and a README sync that brings the full command catalog to **141 commands across 14 products**.

### Added

- **`doc` skill reference** (`skills/references/products/doc.md`) — 16-command coverage of DingTalk Docs:
  - Discovery: `search`, `list`, `info`, `read`
  - Authoring: `create`, `update`, `folder create`
  - Files: `upload`, `download`
  - Block-level editing: block `query`, `insert`, `update`, `delete`
  - Comments: `comment list`, `create`, `reply`
  - URL → `doc_id` extraction rules and nodeId dual-format notes
- **`minutes` skill reference** (`skills/references/products/minutes.md`) — coverage of AI Minutes:
  - Lists: personal / shared-with-me / all-accessible
  - Content: basic info, AI summary, keywords, transcription, extracted todos, batch detail
  - Editing: title update
  - Recording control: start, pause, resume, stop
- **SKILL.md routing**:
  - Product overview table rows for `doc` and `minutes`
  - Intent decision tree routes — `钉钉文档/云文档/知识库/块级编辑/文档评论` → `doc`; `听记/AI听记/会议纪要/转写/摘要/思维导图/发言人/热词` → `minutes`
  - Danger-op table entries: `doc delete`, `doc block delete`
  - `aitable` description completed with the `附件` (attachment) group
- **`aitable` skill enhancements**:
  - `field create` single-field mode (`--name` / `--type` / `--config`) with examples
  - `base get` URL → `baseId` quick-tip
  - Dedicated "URL → baseId 提取" chapter
  - "`--filters` 筛选语法排错与使用规范" chapter
  - "相关产品" cross-link section pointing to `doc`
  - **"复杂操作" chapter** (#141) — dashboard / chart workflow (with two-call sequencing and `chart share get` vs `dashboard share get` error semantics) and two-stage `export data` polling (`scope=all/table/view` parameter constraints)
- **README Key Services sync** (#140):
  - New rows: `doc` (16 commands), `minutes` (22 commands — adds `hot-word`, `mind-graph`, `replace-text`, `speaker`, `upload` subgroups)
  - `aitable` expanded from 20 → 37 commands; surfaces `chart`, `dashboard`, `export`, `import`, `view` subgroups
  - Total command count updated from **86 → 141 across 14 products**
  - "Coming soon" list drops `doc` and `minutes`

### Changed

- `aitable record query` docs rename `--keyword` → `--query` to match the shipped binary
- `aitable record query` docs clarify `--sort` direction semantics (avoids misuse of `order`)
- `aitable base list` guidance strengthened — "only for recent browsing; use `base search` for lookups"; intent decision prioritizes `base search` for base discovery

## [1.0.11] - 2026-04-20

Plugin subsystem hardening: faster cold startup, cleaner lifecycle, stricter isolation, and polished UX for PAT / i18n / error routing.

### Added

- `feat: supports claw-like products` — overlay path for Claw-style embedded editions
- `feat(plugin): inject user identity (UserID, CorpID) into stdio plugin subprocesses`
- `feat(auth): improve login UX for terminal auth denial cases` — clearer messaging + retry affordance
- `feat: PAT scope error visualization and auto-retry with authorization polling` (#113)
  - Human-readable error output (lark-cli style) with type/message/hint/authorization command
  - JSON payload also available via `--format json`
  - Auto-retry once the user completes scope authorization

### Changed

- `perf(plugin): serve plugin MCP tool list from disk cache on startup` — hot path skips Initialize+ListTools when snapshot exists
- `perf(plugin): parallelize all plugin discovery and tighten cold timeouts` — HTTP cold budget 4s → 700ms (auth) / 500ms (plain); stdio and HTTP fan out concurrently
- `perf(plugin): share cache.Store across discovery` — single `*cache.Store` above the fan-out instead of per-goroutine instances
- `refactor(plugin): remove default/managed plugin privileged mechanism` (#124) — third-party plugins install on an equal footing via `dws plugin install`
- `refactor(plugin): purge removed plugin settings instead of merely disabling` — `RemovePlugin` now deletes `EnabledPlugins` and `PluginConfigs` entries

### Fixed

- `fix(transport): cap plugin MCP startup at ~4s when endpoints are unreachable` (#119) — eliminates the 10s `dws --help` stall caused by compounding transport timeouts
- `fix(plugin): stop stdio child processes on exit and before removal` — no more orphaned plugin subprocesses
- `fix(pat): avoid shared PAT command state in root registration` (#129)
- `fix: -f json 模式下错误 JSON 从 stdout 改为输出到 stderr` (#133) — restores CI stderr-based failure assertions
- `fix(cli): localize plugin/help command strings via i18n` (#118, #134) — zh locale now shows consistent Chinese `--help`; wraps plugin module, help command, and OAuth client-id/secret flag descriptions
- `chore: remove workspace and bundled artifacts` (#127) — clean local-only repository leftovers

## [1.0.9] - 2026-04-16

Plugin system launch + execution-pipeline overhaul. This is the largest release since 1.0.0: third-party MCP servers become first-class commands, the command pipeline grows to five stages, and the edition overlay gains the hooks needed for embedded hosts.

### Added

#### Plugin system (new)

- `plugin` command family: `install`, `list`, `info`, `enable`, `disable`, `remove`, `create`, `dev`, `config set/get/list/unset`
- Plugin manifest parsing/validation, managed/user directory-based identity
- MCP server conversion and injection into the dynamic routing registry
- Pipeline hook adapter for shell-based hooks
- Stdio transport: subprocess lifecycle, `DWS_PLUGIN_ROOT` / `DWS_PLUGIN_DATA` variable expansion
- Stdio server tools automatically registered as CLI subcommands (e.g. `dws hello greet --name Peter`)
- Streamable-HTTP MCP tool discovery via `registerHTTPServer`
- Updater: managed plugin update check on CLI startup (10 s timeout, best-effort)
- `dws plugin create` scaffold (plugin.json, SKILL.md, hooks.json); `dws plugin dev` source-dir registration without copy
- `SyncSkills` — copies plugin skills to agent directories on startup
- **Auth Token Registry**: per-server HTTP headers declared in `plugin.json` for third-party MCP servers (e.g. Alibaba Cloud Bailian) independent from DingTalk OAuth
- **Persistent plugin config** (`dws plugin config ...`): values persisted to `~/.dws/settings.json`, auto-injected as env vars; `${KEY}` in `plugin.json` resolves without manual `export`
- **Build lifecycle**: `build` field compiles stdio servers to native binaries at install time
- **Command-name conflict protection**: reserved built-in names (`auth`, `plugin`, `cache`, …) and plugin-vs-plugin duplicate detection
- Parallel service discovery (`sync.WaitGroup`) — startup reduced from sequential `N*10s` to parallel `max(10s)`

#### Core commands & diagnostics

- `dws doctor` — one-stop environment/auth/network diagnostics
- `dws config list` — centralized view of scattered configuration
- Structured perf tracing (upgraded from debug tool to diagnostics output)
- `feat(skill): restore find/get for legacy skill market API` — `skill find`, `skill get`; `skill add` still uses aihub download

#### Edition / overlay hooks

- `edition.Hooks.SaveToken` / `LoadToken` / `DeleteToken` — delegate token persistence with keychain fallback
- `edition.Hooks.AuthClientID` / `AuthClientFromMCP` — overlay can override the OAuth client ID and route auth through MCP endpoints
- `edition.Hooks.AfterPersistentPreRun` — wire non-MCP clients (e.g. A2A gateway) after root setup
- `edition.Hooks.ClassifyToolResult` — custom MCP result classification before the default business-error detection
- Token marker file (`token.json`) for embedded hosts to detect auth state without keychain access
- `pkg/runtimetoken.ResolveAccessToken` mirroring MCP auth resolution; MCP identity headers exported via `pkg/cli` for auxiliary HTTP transports
- `ExitCoder` interface — edition-specific errors carry custom exit codes
- `RawStderrError` interface — errors that bypass CLI formatting and emit raw stderr (for desktop runtimes)

### Changed

- **Command execution pipeline: 3 → 5 stages** (`Register → PreParse → PostParse → PreRequest → PostResponse`)
- `feat(schema): return structured degraded errors instead of silent empty catalog` — new `CatalogDegraded` error with reasons `unauthenticated` / `market_unreachable` / `runtime_all_failed`; auth pre-check short-circuits doomed MCP connections
- `refactor(auth): unify auxiliary token resolution with MCP cached path` — shared `resolveAccessTokenFromDir`; overlays reuse the process-level token cache
- `feat(plugin): improve CLI overlay resolution and plugin install robustness`
  - `plugin.json` `cli` field now accepts a file path (e.g. `"cli": "overlay.json"`) in addition to inline JSON
  - `description` field on `CLIToolOverride` for static fallback when MCP `tools/list` is unavailable
  - Windows install uses `cmd /C` instead of `sh -c` for build commands

### Fixed

- `fix(plugin): harden plugin system security boundaries`
  - Reject `file://` / local paths in git URLs; allow only `https` / `ssh`
  - Reject symlink entries during ZIP extraction (path-traversal defense)
  - `build.output` must be a relative path within the plugin directory
  - Reject absolute paths in stdio command declarations
  - Block dangerous env var names (`PATH`, `LD_PRELOAD`, …) from plugin config injection
- `fix(plugin): schema flag params, HTTP tool discovery, and integration tests`
- `fix(plugin): skip min version check in dev mode`

## [1.0.8] - 2026-04-07

AITable command surface expansion, installer alignment with npm conventions, and execution-timeout hardening.

### Added

- **AITable static helper commands** (20 commands in total) replacing dynamic routing:
  - `base`: `list`, `search`, `get`, `create`, `update`
  - `table`: `get`, `create`, `update`
  - `field`: `get`, `create`, `update`
  - `record`: `query`, `create`, `update`
  - `template`: `search`
  - `attachment`: `upload`
- `feat(install): align skill dirs with npm and add OpenClaw` — skill install paths follow npm conventions; OpenClaw added to supported agents
- Label rendering optimization for AITable records (`to #73551688`)
- README: npm install method documented
- README: note that `dws upgrade` requires v1.0.7+

### Changed

- `perf: optimize command timeout handling, instrumentation, and diagnostics`

## [1.0.7] - 2026-04-02

Self-upgrade, edition overlay foundation, and fail-closed auth enforcement.

### Added

- **`dws upgrade`** — self-upgrade via GitHub Releases; atomic replace; cross-platform (macOS/Linux/Windows)
- `feat: edition layer for Wukong overlay` — build-time edition hook lets downstream overlays customize auth UX, config dir, static server list, visible products, and extra root commands
  - `pkg/edition` defaults + `pkg/editiontest` contract tests
  - `Makefile` target `edition-test`; CI job `edition-tests`
  - Static server injection skips market discovery when configured
  - Deduplicates top-level commands so overlay wins
  - `hideNonDirectRuntimeCommands` respects edition `VisibleProducts`
  - Gated `auth login` subcommand + hints for embedded editions
  - Optional token auto-purge; edition `ConfigDir` override
- `dws version` — human-readable multi-line output plus JSON with edition, architecture, build, commit
- Tag reporting for case suites (`to #73551688`)
- `feat(auth): unify MCP retry constant and add retry to remaining endpoints`

### Changed

- `style(auth): redesign OAuth authorization pages UI`

### Fixed

- `fix(auth): switch CLI auth check from fail-open to fail-closed`
  - When `/cli/cliAuthEnabled` is unreachable (network error/timeout/5xx), OAuth callback now routes to the permission request page instead of silently marking "enabled"
  - Device Flow blocks login and asks the user to verify network connectivity
  - `CheckCLIAuthEnabled` retries with backoff (3 attempts, 0s/1s/2s) to tolerate transient issues

## [1.0.6] - 2026-04-01

Error diagnostics overhaul, destructive-command confirmation, and credential auto-persistence.

### Added

- **Interactive confirmation for destructive dynamic commands** — prompts before delete/remove operations unless `--yes` is set
- **Enhanced error diagnostics**
  - `ServerDiagnostics` struct extracts `trace_id`, `server_error_code`, `technical_detail`, `server_retryable` from MCP responses
  - Pulls diagnostics from JSON-RPC `error.data`, tool call result content, and HTTP headers (`X-Trace-Id`, `X-Request-Id`, `x-dingtalk-trace-id`)
  - Three verbosity levels for `PrintHuman`: Normal (trace ID + server code), Verbose (+ technical detail), Debug (+ RPC code / operation / reason)
  - Local logging now includes sanitized request body, response body on error, retry attempts, and classification events
  - `TruncateBody` / `SanitizeArguments` / `RedactHeaders` helpers with sensitive-key substring detection
- **Auth credential persistence**
  - `feat(auth): enhance device flow with CLI auth check and admin guidance`
  - `feat(auth): persist OAuth credentials for reliable token refresh`
  - `feat(auth): persist client credentials and optimize keychain access` — auto-persist `--client-id` / `--client-secret`; keychain credential cache to avoid repeated reads; enhanced logout cleans `app.json` + keychain secrets + `token.json`
- `add report helper with flexible date parsing and defaults`
- `feat: to #73551688 支持消息通知`
- README: Official App mode (recommended, direct login without creating an app) + Custom App mode; admin guide for enabling CLI access

### Changed

- Getting Started simplified with inline login commands; whitelist references removed from the IMPORTANT banner
- Version bump documentation updated to v1.0.5 internal; co-creation group QR code refreshed

### Fixed

- `fix: resolve verbosity flag lookup, FileLogger lazy binding, and business error logging`
  - `resolveVerbosity` uses `cmd.Flags()` instead of `PersistentFlags()` so subcommands inherit `--verbose` / `--debug`
  - `FileLogger` lazy-binds in `executeInvocation` (after `configureLogLevel` init)
  - Business errors (HTTP 200 + `success=false`) now written to the file logger for offline diagnosis
- OAuth callback race condition (write response before sending code)
- `import path for errors package in skill_command.go`

## [1.0.4] - 2026-03-30

Token-refresh reliability and onboarding clarity.

### Added

- `feat(auth): persist client credentials for token refresh` — `--client-id` / `--client-secret` are stored for automatic refresh after expiration; client secret lives in the system Keychain with a file reference
- README onboarding flow rewrite with step-by-step first-time setup and more realistic examples
- Agent skill reference polish: clearer examples, updated intent routing patterns, expanded `simple.md` onboarding, cross-skill reference fixes

## [1.0.3] - 2026-03-29

Filtering power, schema rendering, and a native `todo` command family.

### Added

- **Nested / array-indexed output filtering**
  - `--fields` now accepts dot-notation (e.g. `--fields response.content`) and array index access (e.g. `response.items[0]`)
  - New field-path parser with recursive extraction logic
- **`schema` command enhancements**
  - Table format output for human consumption
  - Product-level endpoint loading in the CLI loader
  - Schema-text rendering wired into the runner output pipeline
- **`todo` task helper family** — static `create` / `update` / `done` / `get` / `delete` with `preferLegacyLeaf` replacing dynamic commands
  - MCP tool alignment: `create_personal_todo`, `update_todo_task`, `update_todo_done_status`, `query_todo_detail`, `delete_todo`
  - ISO-8601 due-time parsing
  - Hidden title aliases and delete confirmation
  - Priority field on `todo` helper
  - Expanded zh / en i18n coverage (fixes `en.json` spacing/wording issues)
- README restructured with collapsible feature sections

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
