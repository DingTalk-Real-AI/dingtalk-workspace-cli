# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/) and this project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.0.58-beta.2] - 2026-08-10

### Added

- **`dws sheet create-with-data`（新命令）** — 建表并写入初始数据与样式：`--values`（二维数组写默认表）/ `--sheets`（typed table 多工作表，二者必须给一个）/ `--styles`（`cell_styles` / `row_sizes` / `col_sizes` / `cell_merges`，顶层键对齐飞书 snake_case、列表项内字段兼容 camelCase）。所有结构、字段类型与枚举在创建文档之前校验，非法配置不会留下白建的空文档：`--sheets` 按 `table_put` 的输入契约逐字段校验（`columns` 必填且列名非空不重复、`data` 为二维且行宽与 `columns` 一致、单元格仅限字符串/数字/布尔/null、`dtypes`/`formats` 的键须是列名、`mode`/`header`/`allowOverwrite`/`startCell` 类型与取值、单表 30000 单元格上限），并拒绝未知键、snake_case 变体、`{"sheets":"bad"}` 这类畸形包装与 `sheetId`（服务端会静默丢弃写错的键，导致"只写了表头却报成功"的静默丢数据）；`--values` 校验单元格为标量并受 30000 单元格 / 2000000 字符上限约束；`--styles` 的顶层键与列表项内字段同样拒绝未知键，避免样式只应用一半。写入后回读校验按 `startCell` / `header` / `mode` 推算的首个预期非空单元格，而非固定 A1。该命令是多步编排（建文档 → 探活 → 定位默认工作表 → 写数据 → 回读 → 可选样式），因此如实声明为独立叶子 `sheet.create_with_data` + `interface_mode: composite`（附评审 reason，按契约不带 `interface_ref`）；**`dws sheet create` 保持原样不变**——仍是一次 `create_workspace_sheet` 直连（`interface_mode: mcp`），不新增 flag，避免让 Schema 消费者把编排步骤的参数误当成该 RPC 的入参。
- **`dws sheet export-csv`（新命令）** — 同步导出单个工作表为 RFC4180 CSV，支持 `--sheet-id` 选表、`--range` 限定范围、`--value-render-option` 选取值模式；`--output` 落盘（为目录时按 `sheet-export.csv` 命名，落盘走 `AtomicWrite` 原子替换，写入失败时已有文件保持原样；父目录不存在按错误处理，不会自动创建），不传则把纯 CSV 打到 stdout（警告只走 stderr）。数据超出单次读取上限时默认报错、既不输出也不写文件，需 `--allow-truncated` 显式接受不完整结果。响应缺 `csv` 字段或类型不对一律报错，不会用 0 字节覆盖已有文件。该分支读的是 `get_range_as_csv`、与 xlsx 的异步导出任务毫无关系，因此独立成叶子 `sheet.export_csv` 并如实声明 `interface_mode: mcp` + `interface_ref: get_range_as_csv`；**`dws sheet export` 保持原样不变**——仍只导 xlsx（`interface_ref: submit_export_job`），flag 面仍是 `--node` / `--output`，csv 专属 flag 不会出现在它上面（此前挂在同一条命令上时，漏写 `--export-format csv` 会让 `--range` 被静默丢弃而导出整篇工作簿）。
- **`sheet update-dimension --size-type`** — `pixel` / `standard`（恢复默认行高列宽）/ `auto`（按内容自适应行高，仅 ROWS）。
- **`sheet replace --match-formula`** — 在公式文本中查找替换。
- **`sheet range set-style` 扩展样式维度** — 新增 `--font-style`（斜体）/ `--font-line`（下划线、删除线）/ `--font-family` / `--border-styles-json`（四边边框；每条边只接受 `style` / `color`，未知键与非字符串 `color` 直接报错，不再静默忽略而画出无颜色的边框，`set-style` / `batch-set-style` / `create-with-data --styles` 三条路径同源校验）。
- **`sheet range batch-set-style --ranges`** — 一组样式刷多个带工作表前缀的区域，组装为一次原子 `batch_update`。

### Changed

- **Chat message post-send ID handoff** (#897) — CLI Help and bundled Skills
  now document the `send` → `query-send-status` → `edit`/`recall` workflow,
  so callers can reuse returned task, message, and conversation IDs instead
  of searching message history by content.
- **Sheet mono/multi Skill alignment** — replaces the oversized mono Sheet
  reference with the progressive routing layout, aligns all 20 Sheet topic
  references across the mono and multi bundles, and adds a content-policy guard
  that prevents the paired topic trees from drifting again.
- **`sheet range set-style` 后端切换为 `set_cell_range`** — 样式统一走 cellStyles 路径（仅设样式、保留原值），这是斜体/下划线删除线/字体族/边框唯一可用的通道。`interface_ref` 由 `update_range` 变为 `set_cell_range`，12 个样式 flag 改为 reviewed mapping exclusion。CLI 用法向后兼容、无 flag 删除；schema-compatibility 经 reviewed 豁免判定为兼容（0 changed fields）。
- **`sheet range batch-set-style` 改为单次原子提交** — 由本地循环多次 `update_range` 改为一次 `batch_update`，任一项失败默认整批回滚；`--continue-on-error` 由本地控制改为透传服务端。新增批量上限：最多 100 个区域且累计不超过 200000 个单元格。
- **`sheet range batch-clear` / `batch-set-style` 的 `--ranges` 拒绝空白工作表前缀**（用户可见行为变更）— 此前只按原始串里 `!` 的位置判断，`" !A1:B2"` 修剪后工作表名成了空串，操作却照样带着 `sheetId: ""` 提交：服务端要么让整批 `batch_update` 失败，要么更糟——落到默认工作表而不是用户指定的那张表，且命令报成功。现在工作表名与范围都必须在修剪之后仍非空，否则在发起任何请求之前报错。`batch-set-style --batch` 的纯空白 `sheetId` / `range` 同样拒绝（此前只挡空字符串）；`--batch` 下发仍用原值不替用户修剪，因为 `sheetId` 可以是允许带首尾空格的工作表**名**。两条 `--ranges` 路径现在共用同一个拆分器。
- **`sheet insert-dimension` / `delete-dimension` / `update-dimension` 的 `--length` 严格校验**（用户可见行为变更）— 解析由 `fmt.Sscanf("%d")` 改为 `strconv.Atoi`。此前只消费前缀数字，`--length 2x` / `3foo` 会被静默当成 `2` / `3` 并对错误的行列数执行操作（删除方向不可回滚）；现在整个值必须是合法正整数，否则报错「`--length` 必须为正整数（>= 1）」且不发起任何请求。**升级影响**：原先依赖这种宽松解析、在传畸形 `--length` 的脚本会开始报错，请把参数修正为纯数字。合法数字值行为不变，上限仍为 5000。`add-dimension` 的 `--length` 是 `Int` 类型 flag，一直由 cobra 严格校验，不受影响。
- **CLI 接口兼容门禁支持 reviewed flag 类型豁免**（无用户可见变更）— `authoritative-interface-integrity` 与 `check-command-compatibility.sh` 此前一律拒绝历史命令的 flag 类型变更，即使新类型只是把同一套校验从 RunE 前移到解析期，也没有任何评审通道。现在两道门禁各带一张精确豁免表：命令路径 + flag 名 + 旧类型 → 新类型四元组全等才命中、方向敏感（`string`→`int` 与 `int`→`string` 是两个不同的键，只有被评审的方向可用），且仅当该 flag 的其他契约（shorthand / required / hidden / no-opt / scope）纹丝不动时才放行，因此豁免夹带不了别的破坏。首条也是目前唯一一条登记的是 `dws minutes permission apply --policy` 的 `string` → `int`（配合 #912）：旧实现在 RunE 里做 `strconv.ParseInt(v, 10, 64)` 再校验 `[2,4]`，新实现由 pflag 以 `strconv.ParseInt(s, 0, 64)` 解析后仍校验 `[2,4]`，**历史上能成功的调用集是新调用集的子集**（base 0 额外接受 `0x3` 这类写法，只放宽不收紧），非法值依然失败、只是报错文案与时机前移；flag 默认值由 `""` 变 `"0"` 是类型的必然结果，两道门禁都不比较默认值，且该 flag 必须显式给出、默认值不可达。两张表必须逐字一致并有守卫测试锚定漂移——重复是被迫的而非选择：`check-authoritative-interface-baselines.sh` 会把整个 `scripts/policy/interface-baseline` 目录复制进检出历史版本的 worktree 再编译，那份拷贝不能 import 本分支新增的包。

## [1.0.58-beta.1] - 2026-08-07

### Added

- **Robot image and file messages** (#867) — `dws chat message send-by-bot`
  now supports image URLs and local-file uploads through explicit message
  types, while retaining Markdown as the default and preserving its existing
  title and text requirements.
- **Conversation shortcut-bar management** (#877) — adds `dws chat toolbar`
  commands to list, add, hide, sort, and manage custom conversation shortcuts,
  with validation and confirmation for destructive removal.
- **Complete AI Table Shortcut surface** (#901) — makes all 92 supported
  AI Table Shortcuts discoverable through Runtime Schema and adds reliable
  Base, table, record, attachment, view, dashboard, and workflow operations
  with explicit confirmation and result-verification semantics for writes.

### Changed

- **Doc import upload fallback** — `dws doc import` no longer fails on file
  formats outside the conversion whitelist (html, pdf, zip, extensionless,
  and any future format): it now hands the file to the document-space upload
  chain (the same primitive as `dws drive upload --workspace`), stores the
  original file at the requested `--folder`/`--workspace` target, and prints
  an explicit stderr notice with the supported-format list and the
  convert-to-md alternative. The fallback shares the import file checks
  (20MB cap, empty-file guard), keeps `--format json` / `--dry-run` output as
  a single JSON document, and marks the machine-readable result with
  `fallback: "upload"` and `converted: false` so agents never mistake the
  stored file for a converted online document. The fallback fails closed
  unless the commit response parses as JSON and carries a file identity
  (exposed as `dentry_id`); empty or unverifiable responses surface as
  errors instead of fabricated success. Importable formats and
  `dws sheet import` validation are unchanged.
- **IM natural-target and history alignment** — Chat shortcuts can resolve natural user/group targets before execution, and message-history workflows expose bounded time ranges, ordering, explicit all-page controls, continuation ledgers, safe local export, and thread-reply pagination without treating empty or incomplete reads as successful results. Bundled mono/multi Skills and intent routing now describe the same executable surface.
- **Sheet CSV formula writes** — `dws sheet csv-put` and batch `csv-put` now expose the service contract that CSV fields beginning with `=` are written as formulas. Prefix the field with an apostrophe to write literal text beginning with `=`; CSV content continues to pass through unchanged.
- **Release-equivalent PR compatibility gate** (#889) — pull-request
  admission now runs command-surface compatibility checks against the current
  release baseline before code reaches `main`.
- **Reviewer routing governance** (#903) — updates the Reviewer Router pool
  used for new ready PRs while retaining the existing current-head review and
  required-check gates.

### Fixed

- **Fail-closed IM pagination and audit evidence** — `+chat-messages`, `+search-msg`, `+thread-replies`, `+at-me`, `+my-groups`, conversation lists, and favorites preserve partial-read failures, reject missing or stalled continuation state, deduplicate page boundaries, and publish completion evidence. The live-audit regression suite now rejects empty projections and incomplete reads instead of promoting them to passing results.
- **Sheet formula verification** (#873) — `dws sheet formula-verify` now calls
  the registered remote tool name `verify_formula`; the previous
  `formula_verify` name failed at gateway dispatch.
- **CLI and parameter recovery boundaries** (#864) — command and parameter
  recovery now fail closed when an Agent-provided path or flag cannot be
  reconciled with the executable CLI surface, reducing unsafe hallucinated
  retries.

## [1.0.57-beta.4] - 2026-08-06

### Added

- **Expanded open CLI workflows** (#887) — adds calendar event-instance
  queries, Drive latest-file selection, Markdown diff, Mail calendar/export/
  share-to-chat workflows, and Minutes hot-word, permission, and audio-memo
  operations, with matching Schema and cross-platform coverage.

### Changed

- **Multi-skill framework alignment** (#887) — folds long-tail skills into
  `dingtalk-misc`, renames the shared package to `dingtalk-shared`, removes
  stale Preview guidance, and reorganizes shared recipes and routing for more
  predictable Agent selection.
- **Bounded Agent Schema delivery** (#887) — keeps compact and wire projections
  focused on executable contract facts, retires stale MCP metadata candidates,
  and teaches Agents to prefer `dws schema --compact` for bounded context.

### Deprecated

- **Recovery and discovery-cache compatibility surfaces** (#887) — keeps
  visible Deprecated `dws recovery` and `dws cache` compatibility stubs while
  retiring their former recovery engine and dynamic discovery-cache behavior.
  Recovery plan/execute/finalize now return an explicit “不再支持” notice, and
  Skills no longer teach either retired workflow.

### Fixed

- **Mail share-to-chat confirmation** (#887) — requires explicit confirmation
  before the first remote write, while preserving the confirmed sign-retry
  flow and covering both direct-success and retry responses.

## [1.0.57] - 2026-08-06

This stable release promotes the fully delivered `v1.0.57-beta.4` baseline.
It includes the v1.0.57 beta-line command-contract, document, chat, OA, Wiki,
and compatibility improvements, plus the multi-skill framework alignment and
expanded calendar, Drive, Markdown, Mail, and Minutes workflows validated in
the final prerelease.

- **Promote v1.0.57-beta.4** — publishes the final validated prerelease
  baseline as stable `v1.0.57` without adding post-beta product changes.

## [1.0.57-beta.3] - 2026-08-06

### Added

- **Reviewed document shortcuts** (#880) — adds public document shortcuts for
  safe local downloads, content and history, review, media and style, and
  document access/sharing workflows, while retaining reviewed compatibility
  identities and confirmation safeguards for writes.
- **Mentions in chat replies** (#881) — `chat message reply` now supports
  `--at-open-dingtalk-ids` and `--at-all`, forwarding reply mention fields and
  adding any required mention placeholders without changing existing send
  behavior.

## [1.0.57-beta.2] - 2026-08-05

### Fixed

- **Stable Chat command compatibility** (#876) — restores the hidden migration
  entries for `chat send`, `chat history`, and their `im` aliases, preserving
  the v1.0.56 command surface while directing callers to the supported
  `chat message send/list` commands. Legacy flags now reach the same migration
  hints instead of failing during flag parsing.
- **Drive download cancellation-test stability** (#876) — replaces a
  timing-sensitive worker-cancellation coverage test with a deterministic seam,
  reducing flaky CI without changing download behavior.

## [1.0.57-beta.1] - 2026-08-05

This beta starts the v1.0.57 line on top of v1.0.56. It packages the unified
command-contract and runtime Schema architecture, complete Multi IM Chat
coverage, document whiteboard and OA approval workflows, Wiki activity feeds,
and compatibility and CI reliability fixes.

### Added

- **Contact personal-status updates** (#872) — adds `contact user update-ownness`
  (alias `set-ownness`) for updating a user's personal status text. The write
  operation maps reviewed `userId` and `ownnessText` parameters to the service
  contract and requires confirmation unless `--yes` is explicitly supplied.
- **Document whiteboard workflows** (#861) — adds `doc whiteboard insert`,
  `whiteboard query/update`, and `doc media upload`. These commands support
  confirmed document-embedded whiteboard creation and updates, structured
  OpenNodes reads, and preparation of node-bound Vector/SVG resources.
- **Complete Multi IM Chat coverage** (#860) — hardens deterministic group and
  stable-ID resolution, sending, querying, downloading, pagination, and JSON
  export. The remaining reviewed Chat Shortcuts enter Schema coverage, with
  destructive delete and clear operations aligned to confirmation gates.
- **OA approval form workflows** (#853) — adds OA form-schema lookup,
  process forecast, and confirmed approval-instance creation, supporting both
  simple flags and complete `--request` payloads.
- **Wiki activity-feed queries** (#862) — adds `wiki feed list` to retrieve
  workspace document activity, with cursor paging and optional file exclusion.

### Changed

- **Unified command and Schema contract framework** (#830) — Leaf commands and
  Shortcuts now use the shared typed `corecmd` base for flags, constraints,
  confirmation, Help, and runtime Schema projection. Schema delivery assembles
  from leaf Contract declarations at runtime; the retired hint overlays,
  pinned MCP metadata, and committed Catalog artifacts are no longer delivery
  authorities.
- **Faster macOS CI without reducing native coverage** (#857) — narrows the
  macOS race suite to Keychain, codesign, and Darwin-only tests while adding a
  reachability contract that prevents native-only tests from being silently
  excluded.

### Fixed

- **Chat media-download JSON compatibility** (#854) — restores parseable
  `success`, `downloadUrl`, and `output` fields for
  `chat message download-media --format json` after a successful download,
  without progress output corrupting JSON stdout.

### Added

- **Document-embedded whiteboard workflows** — adds `doc whiteboard insert` for confirmed creation and part-ID verification, `whiteboard query/update` for structured OpenNodes reads and confirmed writes, and `doc media upload` for preparing node-bound Vector/SVG resources. The public adapter uses an explicit helper-only whiteboard endpoint, validates update envelopes locally, decodes `resultJson`, and publishes the full command, Schema, Skill, and safety contract migrated from `dws-wukong@e2da8ab947c6`.
- **Robot image and file messages** — extends `chat message send-by-bot` with public image URL delivery through `--msg-type image --image-url` and local-file upload/send through `--msg-type file --file-path`, while preserving Markdown as the default message type.

### Changed

- **Chat reply mentions** — `dws chat message reply` can @ specified group members with `--at-open-dingtalk-ids` or @ everyone with `--at-all`, forwarding the existing `send_personal_message` mention fields and automatically adding missing current-user `<@id>` / `<@all>` placeholders.
- **Pinned MCP metadata retired** — deletes `internal/cli/schema_mcp_metadata.json` and removes its embed/loader/fallback role from Schema assembly. Catalog now assembles from Contract/ParamDecl/Interface + Cobra only; `make fetch-mcp-metadata` remains an optional diagnostic dump under `artifacts/` and refuses the retired pin path. Policy bans the pin from reappearing.
- **MCP service review retired** — deletes `schema_mcp_service_review.json` and removes its policy jq / outputguard / test disposition gate (`notify` → `out_of_surface`, snapshot hash pin). No replacement ledger.
- **Hints retired; ContractDecl is the leaf Schema source** (#830) — `schema_hints/`, Manual/Schema hint overlays, and `schema_agent_metadata/` delivery are removed. Selection, safety, parameters, and interface facts declare on ProductDecl / leaf `Contract` (`corecmd.ContractDecl` + `contract.ParamDecl` / `Safety`). Authoring renamed `SchemaDecl` → `ContractDecl`; nested fields reuse `contract.*` directly.
- **Contract package seam** (#830) — types / ProductDecl live under `internal/corecmd/contract` (DTO only). Annotate writers live in `internal/corecmd/runtimeannotate`; Cobra-keyed ContractFinal store + Register live in `internal/corecmd/contractfinal`; homology gates in `internal/cli/homology`. All packages import `corecmd/*` directly; the former `cli/runtimeannotate` / `cli/contractfinal` shim packages are removed, and the `cli` root keeps only package-local aliases (`runtime_schema_seam.go`). Catalog/`ResolveMeta` stay on the `cli` delivery root. `internal/corecmd` must not import any `internal/cli` package.
- **CommandMeta cache for ResolveMeta** — production `ResolveMeta` / leaf `--help` Safety project from the runtime-assembled `SchemaRegistry` into a `map[cli_path]CommandMeta` installed during `deliverySchemaCatalog` sync.Once. Steady-state lookups are O(1); full Catalog wire maps stay deferred. Registry `Source` stamps `runtime-assembled`.

### Fixed

- **Fail-closed IM pagination and audit evidence** — `+chat-messages`, `+search-msg`, `+thread-replies`, `+at-me`, `+my-groups`, conversation lists, and favorites preserve partial-read failures, reject missing or stalled continuation state, deduplicate page boundaries, and publish completion evidence. The live-audit regression suite now rejects empty projections and incomplete reads instead of promoting them to passing results.
- **Unified command safety and Shortcut runtime (H0)** — Shortcut leaves now execute through `corecmd.New`, sharing the same typed Safety confirmation gate as Leaf commands. EOF / closed stdin returns `confirmation_required`, and interactive `no` returns the existing non-zero cancellation validation error instead of reporting success for an operation that did not run. Pass `--yes` or `--dry-run` to skip the prompt.
- **Constraint "provided" for `at_least_one` / `exactly_one` (H0)** — a flag set to an empty string (`--flag ""`) no longer counts as provided; previously bare Cobra `Changed` satisfied the constraint. Pass a non-blank value for a member of the group.
- **Chat media download JSON compatibility** — `dws chat message download-media --format json` once again returns a clean `{success, downloadUrl, output}` result after the file is saved, preserving the temporary URL and resolved local path without progress text corrupting JSON stdout.

## [1.0.56] - 2026-08-04

This stable release promotes the fully delivered `v1.0.56-beta.4` baseline.
It includes PR #852's resilient multipart Drive download implementation,
together with the v1.0.56 beta-line command, Schema, Skill, and runtime
improvements already validated through the prerelease channel.

### Added

- **Resilient multipart Drive downloads** (#852) — `drive download` and
  `drive download-version` support parallel chunk transfer, Range probing,
  fingerprint-validated checkpoint resume, automatic 401/403 credential
  refresh, and graceful interruption with checkpoint preservation.

## [1.0.56-beta.4] - 2026-08-04

This beta adds PR #852 on top of v1.0.56-beta.3. It makes Drive downloads
resilient for large files through parallel transfer, validated resumable
checkpoints, and automatic credential refresh.

### Added

- **Multipart Drive downloads** (#852) — adds `--part-size`, `--parallel`, and
  `--no-resume` to `drive download` and `drive download-version`. Files above
  the part-size threshold use a Range probe and parallel chunks, resume from a
  fingerprint-validated checkpoint, refresh credentials on 401/403, and keep
  the checkpoint when Ctrl+C interrupts a transfer.

## [1.0.56-beta.3] - 2026-08-03

This beta adds PRs #846 and #851 on top of v1.0.56-beta.2. It adds a
service-provided Aitable workflow-editing reference command and makes local
event-bus IPC reliable on shared filesystems by placing Unix sockets in a
validated private runtime directory.

### Added

- **Aitable workflow editing reference** (#851) — adds `dws aitable workflow edit-example`, a parameter-free read command that returns the service-provided workflow editing documentation and `workflow-dsl/v1` examples through `aitable/edit_workflow_example`.

### Fixed

- **Event bus sockets on shared filesystems** (#846) — Unix event buses now place their local IPC socket in a private per-user runtime directory (`XDG_RUNTIME_DIR` when available, otherwise a `0700` per-UID directory under the system temporary directory) while retaining locks, metadata, logs, and subscription state in the configured Workdir. Listener and dial paths validate directory ownership and permissions before use. This prevents `dws event consume` from failing with `bind: errno 524` when `~/.dws` is hosted on NFS, CSI, FUSE, or another filesystem that does not support Unix Domain Sockets without exposing the socket directly in a shared `/tmp` root. When `XDG_RUNTIME_DIR` is unavailable, the per-UID directory name is deterministic: ownership validation prevents endpoint hijacking, but another local user can pre-create the directory to deny service; multi-user deployments should provide a private `XDG_RUNTIME_DIR`.

## [1.0.56-beta.2] - 2026-07-30

This beta adds PRs #831 and #835 on top of v1.0.56-beta.1. It separates
Agent Product observability and IM display identity from the stable
edition-owned PAT and routing identity, and reduces common-path Skill context
loading without changing the public command or Runtime Schema surface.

### Changed

- **Agent Product identity separation** (#831) — sends `DWS_AGENT_PRODUCT` through the new `x-dws-agent-product` observability Header and uses a valid non-empty value for the IM `clawType` display label whenever `--ai-tag` is enabled. Because `--ai-tag` defaults to `true`, callers that set `DWS_AGENT_PRODUCT` change the displayed label by default. With `--ai-tag=false`, native `chat message send` / `reply` calls preserve their existing wire shape by sending an empty IM `clawType`, while shortcut calls omit the argument. Unset or empty Product values omit the Header and preserve the active edition's IM display default.
- **Agent Host dimension convention** (#831) — new integrations should send the runtime form (`cloud` or `desktop`) through `DWS_AGENT_HOST` and report the product separately through `DWS_AGENT_PRODUCT`. Legacy combined labels such as `qwenwork_cloud` remain syntactically valid for compatibility.
- **Reduced common-path Skill context** (#835) — keeps the complete 97-command Chat Shortcut inventory in Runtime Catalog and leaf Schema while routing common intents through compact Skill tables and references. When an exact command path is already known, the mono Skill no longer requires eager loading of a complete product reference. The generated Skill policy now detects drift, forced full-reference loading, and context-budget regressions; the common Chat plus shared activation estimate drops from 7,301 to 4,771 `o200k_base` tokens without changing the 845-tool Schema surface.

### Fixed

- **Stable PAT/routing identity** (#831) — restores the CLI-emitted open-source HTTP `claw-type` and PAT `hostControl.clawType` to the edition-fixed `openClaw` value. `DWS_AGENT_PRODUCT` no longer changes those wire values, and the client continues to derive PAT, authentication, routing, and Discovery behaviour from the existing independent signals.
- **Portable generated Skill validation** (#835) — resolves the mono Skill name by scanning upward from the generated target, keeping `--check` independent of the repository checkout path and preventing false drift failures when an ancestor directory resembles a Skill name.

## [1.0.56-beta.1] - 2026-07-30

This beta starts the v1.0.56 line on top of v1.0.55 and packages PRs #817,
#806, and #834, together with release-validation fixes #838 and #839. It closes
the remaining Agent-visible IM shortcut gaps, introduces reviewed
command-scoped parameter normalization without guessing business identifiers
or values, and prevents deterministic personal-event subscription failures
from becoming unbounded retry storms.

### Added

- **Complete IM shortcut workflows** (#817) — publishes the previously excluded `+chat-messages`, `+messages-send`, `+messages-send-card`, `+search-msg`, and `+thread-replies` shortcuts in Runtime Schema. Unified send, streaming-card delivery, advanced search, thread replies, and opt-in resource downloads now share reviewed parameters, selection guidance, and runtime-aligned safety semantics.
- **Reviewed parameter concept normalization** (#806) — adds a closed parameter-concept dictionary and generated command-level alias table, covering reviewed IM synonyms while preserving the boundaries between group, conversation, user, open-user, cursor, and paging identifiers.

### Fixed

- **Message delivery and resource handling** (#817) — resolves direct recipients through exact contact search, preserves rich and nested message resources, avoids same-name download overwrites, and prevents read shortcuts from silently returning empty results on non-interactive input.
- **Parameter parsing safety** (#806) — rejects ambiguous, blocked, or conflicting aliases before dispatch, normalizes explicit boolean values such as `--dry-run false`, and keeps internal pre-parse handler details out of user-visible errors.
- **Personal-event subscription retry safety** (#834) — adds cross-process attempt claims, deterministic backoff and jitter, `Retry-After` handling, terminal holds, compare-and-swap completion, and fail-closed state handling across all public personal-event subscriptions, preventing deterministic failures from causing unbounded callback retries.
- **Scoped CI and release validation reliability** (#838, #839) — keeps scoped coverage aligned with intentionally skipped supporting profiles, gives focused race and Multi-profile E2E suites enough time for the current `internal/app` workload, and preserves hidden E2E diagnostics on failure.

## [1.0.55-beta.8] - 2026-07-30

This beta revalidates the `v1.0.55-beta.7` product baseline through a complete
guarded release delivery. It carries no new product-facing command behavior;
the new version is required because the published beta.7 artifacts succeeded
on GitHub, npm, and Homebrew, but its enabled optional Gitee mirror failed and
left that Release run ineligible for stable promotion.

### Changed

- **Complete promotion evidence** — republishes the validated v1.0.55 command, Runtime Schema, Skill, authentication, and projection changes with the optional Gitee upload fallback disabled, so the release can produce one successful auditable delivery proof before stable promotion.

## [1.0.55] - 2026-07-30

This release promotes the validated `v1.0.55-beta.8` baseline to stable. It
expands the public Workspace command surface and personal event consumption,
makes the full built-in shortcut catalog available to Agents, and hardens
multi-account routing, authentication compatibility, command safety, and
response projection across the CLI.

### Added

- **Broader Workspace command surface** (#621, #676) — adds roughly 30 reviewed Drive, Doc, Sheet, and Chat leaf commands synchronized from Wukong, including Drive version and permission operations, document styling, Sheet comment/version/formula verification, and in-place text-emotion updates. A reusable declarative `LeafSpec` framework now delivers command identity, safety, selection, and guarded Help metadata consistently.
- **Complete Agent-visible shortcut delivery** (#802, #815) — publishes all 210 built-in shortcuts as reviewed Runtime Schema leaves across 16 products, including 88 validated Chat shortcuts, with executable paths, parameters, constraints, selection guidance, dry-run capabilities, and runtime-aligned confirmation semantics.
- **Expanded enterprise and event capabilities** (#790) — adds the HR Brain talent-pool, employee-profile, and structured-search command families; `dws mcp url get` resolves MCP Market endpoints; personal event consumption supports eight additional IM event keys, multi-key consumers, and targeted shutdown.
- **Agent integration identity** (#804, #816) — adds validated `DWS_AGENT_HOST` and `DWS_AGENT_PRODUCT` labels for observability and product attribution while keeping them separate from authentication and authorization.

### Changed

- **Progressive multi-Skill guidance and account safety** (#621, #821) — reorganizes bundled product guidance for progressive discovery and restores the mandatory rule that Agents must not guess an account when a multi-account organization has no unique current default.
- **Supported Chat file delivery** — retires the legacy AppKey/AppSecret-backed `chat media upload` command from discovery and routes local files through `chat message send --msg-type file --file-path`, while callers with an existing media ID can continue sending images directly.
- **Guarded release delivery** (#791) — strengthens immutable GitHub, npm, Homebrew, optional mirror, recovery, and version-allocation checks while keeping beta and stable publication role-gated and auditable.

### Fixed

- **Shortcut and message projection correctness** (#706, #783, #795) — prevents successful read shortcuts from silently projecting non-empty backend responses to empty results, renders rich, forwarded, and encrypted message forms safely, and fixes group-bot, bot-search, mail-thread, media-ID alias, and Todo paging response handling.
- **Command contract edge cases** (#803) — makes approval revocation and document rollback honor dry-run before confirmation or preflight, fixes Drive and Doc rename semantics, restores Drive-specific metadata, and validates Todo reminder rules.
- **Authentication and external-contact compatibility** (#756, #757) — migrates legacy global and organization-scoped credentials without cross-account token borrowing, preserves contacts that expose only `openDingTalkId`, and aligns message-resource flags with message-list output fields.

## [1.0.55-beta.7] - 2026-07-29

This beta supersedes the unpublished `v1.0.55-beta.6` candidate and packages
PRs #621, #676, #757, #815, #816, and #821. It restores the mandatory
multi-account safety rule caught by the sealed-release E2E gate while retaining
the reviewed Wukong capability and multi-Skill synchronization, declarative
command and Schema delivery, hardened Chat shortcuts, external contact
resolution, and Agent product identity on top of the `v1.0.55-beta.5` baseline.

### Added

- **Wukong capability and multi-Skill synchronization** (#621) — ports roughly 30 reviewed leaf commands into the open-source CLI across Drive, Doc, Sheet, and Chat, including in-place text-emotion updates, Drive version and permission operations, document styling, and Sheet comment/version/formula verification. The bundled multi-Skill framework is reorganized into progressive product references and routing guidance while retaining current open-source command, response, safety, and Runtime Schema contracts.
- **Declarative leaf commands and unified metadata delivery** (#676) — adds the reusable `LeafSpec` command framework and migrates 27 DevApp commands without changing their paths or flags. Runtime consumers now resolve identity, safety, and selection through one embedded Catalog-backed API, and guarded Help output publishes the command's safety/confirmation annotation.
- **Agent product identity** (#816) — adds the optional `DWS_AGENT_PRODUCT` override for the existing HTTP `claw-type` header while preserving each edition's default when unset. Product and runtime labels are caller-declared signals, not authentication credentials; services must validate supported values and must not grant access solely from them. The override does not change the separate IM message-display `clawType` parameter controlled by the edition and `--ai-tag`.

### Changed

- **Reviewed Chat shortcut delivery** (#815) — publishes 88 currently available Chat shortcuts after real-business validation, keeps three confirmed lower-service failures unavailable, strengthens semantic availability and dry-run contracts, and adds safe message-resource download plus group-member listing. Conversation filtering, IM routing/reporting, and member mute resolution are aligned with the validated backend identities.
- **Agent identity label hardening** (#816) — limits `DWS_AGENT_PRODUCT` and `DWS_AGENT_HOST` to 64 ASCII bytes, trims only surrounding ASCII spaces and tabs, and rejects other control or Unicode whitespace. QwenWork integrations should report the two dimensions separately as `DWS_AGENT_PRODUCT=qwenwork` plus `DWS_AGENT_HOST=cloud` or `desktop`; previously used combined Host labels such as `qwenwork_cloud` remain syntactically valid for compatibility.

### Fixed

- **External-contact and message-resource chaining** (#757) — the shared name-to-ID resolver keeps external or cross-organization contacts that expose only `openDingTalkId`, applies reviewed display-name fallbacks, and preserves organization-only filtering for commands that require `userId`. `chat +messages-resource-url` now accepts `--msg-id` and `--open-message-id` as aliases for `--message-id`, matching message-list response fields.
- **Multi-account Skill safety contract** (#821) — restores the mandatory rule that an Agent must never choose the first, most recently logged-in, or most recently used account when an organization has multiple accounts without one unique `isOrgCurrent=true` default. A PR-level embedded-Skill regression test now catches removal before the full sealed-release E2E gate.

## [1.0.55-beta.6] - 2026-07-29

This beta packages PRs #621, #676, #757, #815, and #816, validating the Wukong
capability and multi-Skill synchronization, declarative command and Schema
delivery, hardened Chat shortcuts, external contact resolution, and Agent
product identity on top of the `v1.0.55-beta.5` baseline.

### Added

- **Wukong capability and multi-Skill synchronization** (#621) — ports roughly 30 reviewed leaf commands into the open-source CLI across Drive, Doc, Sheet, and Chat, including in-place text-emotion updates, Drive version and permission operations, document styling, and Sheet comment/version/formula verification. The bundled multi-Skill framework is reorganized into progressive product references and routing guidance while retaining current open-source command, response, safety, and Runtime Schema contracts.
- **Declarative leaf commands and unified metadata delivery** (#676) — adds the reusable `LeafSpec` command framework and migrates 27 DevApp commands without changing their paths or flags. Runtime consumers now resolve identity, safety, and selection through one embedded Catalog-backed API, and guarded Help output publishes the command's safety/confirmation annotation.
- **Agent product identity** (#816) — adds the optional `DWS_AGENT_PRODUCT` override for the existing HTTP `claw-type` header while preserving each edition's default when unset. Product and runtime labels are caller-declared signals, not authentication credentials; services must validate supported values and must not grant access solely from them. The override does not change the separate IM message-display `clawType` parameter controlled by the edition and `--ai-tag`.

### Changed

- **Reviewed Chat shortcut delivery** (#815) — publishes 88 currently available Chat shortcuts after real-business validation, keeps three confirmed lower-service failures unavailable, strengthens semantic availability and dry-run contracts, and adds safe message-resource download plus group-member listing. Conversation filtering, IM routing/reporting, and member mute resolution are aligned with the validated backend identities.
- **Agent identity label hardening** (#816) — limits `DWS_AGENT_PRODUCT` and `DWS_AGENT_HOST` to 64 ASCII bytes, trims only surrounding ASCII spaces and tabs, and rejects other control or Unicode whitespace. QwenWork integrations should report the two dimensions separately as `DWS_AGENT_PRODUCT=qwenwork` plus `DWS_AGENT_HOST=cloud` or `desktop`; previously used combined Host labels such as `qwenwork_cloud` remain syntactically valid for compatibility.

### Fixed

- **External-contact and message-resource chaining** (#757) — the shared name-to-ID resolver keeps external or cross-organization contacts that expose only `openDingTalkId`, applies reviewed display-name fallbacks, and preserves organization-only filtering for commands that require `userId`. `chat +messages-resource-url` now accepts `--msg-id` and `--open-message-id` as aliases for `--message-id`, matching message-list response fields.

## [1.0.55-beta.5] - 2026-07-28

This beta validates expanded personal event consumption, complete Agent-visible
Runtime Schema coverage for all 210 built-in shortcuts, Agent host
observability, and hardened document, Drive, approval, and Todo command
contracts on top of the `v1.0.55-beta.4` baseline.

### Added

- **Expanded personal event consumption** (#790) — adds eight IM personal event keys, supports subscribing to and consuming multiple event keys in one `dws event consume` invocation, and adds targeted local-consumer shutdown when a subscription is stopped so other consumers can continue on the shared event bus.
- **Shortcut Runtime Schema delivery** (#802) — publishes all 210 public built-in shortcuts as reviewed Agent-visible leaf tools across 16 product groups, with stable canonical identities, executable `+shortcut` CLI paths, parameter and cross-parameter constraints, selection guidance, interface metadata, and runtime-aligned safety/confirmation semantics. `dws shortcut list` remains the lightweight batch-discovery view, while leaf Schema now carries the complete Agent contract; declared string-slice defaults are also preserved consistently in Cobra and Schema.
- **Agent host observability** (#804) — accepts an optional, validated `DWS_AGENT_HOST` label and sends it as `x-dws-agent-host` for logs and BI only; invalid values fail before CLI network activity, and the label never participates in authentication or routing.

### Fixed

- **Command contract edge cases** (#803) — approval revocation and document-version rollback now honor `--dry-run` before confirmation or remote preflight; `drive rename` removes only a suffix matching the node's current extension to avoid duplicate extensions while `doc rename` preserves the caller's exact display name; `doc info` keeps its stable MCP contract while `drive info` restores Drive-only metadata such as a non-null `fileSize`; and Todo reminder writes now reject invalid rule JSON while Help, Schema, and Skills distinguish a due time from an independently unreadable reminder rule.

## [1.0.55-beta.4] - 2026-07-27

This beta validates the shortcut projection fixes for group bots, bot search,
and mail threads, together with hardened release delivery to Gitee and npm on
top of the `v1.0.55-beta.3` baseline.

### Fixed

- **Shortcut projection fixes** (#795) — `chat +chat-bots` no longer projects a non-empty `list_group_bots` response to an empty list, `+bot-find` recognizes the `search_bots` response shape (`result.bots` entries with `botOpenDingTalkId`), and mail thread listings keep `lastUpdated` when the backend returns `lastModifiedDateTime`.

### Changed

- **Hardened release delivery** — the Gitee mirror workflow can synchronize a specific release's assets on demand, release lookup tolerates Gitee's HTTP 200 null-body response for missing releases, npm dist-tag verification waits through slow registry CDN propagation with incremental backoff, and beta/stable release operations are role-enforced (#791).

## [1.0.55-beta.3] - 2026-07-24

This beta validates the HR Brain command surface, smoother guarded release
automation, and deterministic Markdown test coverage on top of the
`v1.0.55-beta.2` baseline.

### Added

- **HR Brain (`dws hrbrain`) command surface** — adds 11 commands across three groups: `talent-pool list/detail/employees` for talent pool browsing, `profile metadata/query/labels/career/performance` for employee profile data, and `search employees/employees-structured/fields` for basic and advanced (rule-based) people search. Ships with bundled mono/multi Skill guidance (`dingtalk-hrbrain`, `cli_version: ">=1.0.54"`); `search employees-structured` validates `--origin-json` as a JSON object and `--fields` as a JSON array before dispatch.

### Changed

- **Smoother guarded releases** — publishes verified stable and beta Homebrew Formula updates directly from the release workflow, retries transient tag-ref visibility failures, lets an exact same-run retry reuse its sealed tag, and allows machine-verified rebuild recovery without a separate approval wait.

### Fixed

- **Deterministic Markdown coverage** — replaces timing-dependent temporary-file deletion tests with synchronized file-stat failures so release admission no longer flakes on scheduler timing.

### Changed

- **Faster guarded releases** — trusts an independently revalidated, exact `CHANGELOG.md`-only successor of an already admitted `main` commit, runs cloud planning alongside governance, and executes sealed-release automation, compatibility, and multi-profile validation in parallel with artifact compilation. Normal cloud publication no longer requires an unshareable local packaging preflight.
- **Scoped document reads and group mentions** — `doc read --content-format jsonml` can return `outline`, `range`, `section`, or custom-tag fragments with depth and block-boundary controls; document comment create, reply, and update can mention groups through `--mentioned-open-conversation-id`.
- **Drive overwrite uploads** — `drive upload --node <fileId>` can replace an existing Drive or document-space file, is mutually exclusive with `--folder`, supports dry-run, and requires confirmation before writing.
- **Chat nickname clearing and cross-organization todos** — omitting `--nick` from `chat group update-nick` now clears the current user's group nickname, while `todo task list --query-all` queries todos across organizations.

### Fixed

- **Legacy authentication compatibility** (#756) — migrates pre-v1.0.53 global and organization-scoped login state into the identity-aware token store, including all legacy organizations, while keeping unresolved accounts isolated from exact `corpId:userId` credentials so external or no-directory identities can complete login without borrowing another user's token.

## [1.0.55-beta.1] - 2026-07-23

This beta validates MCP Market URL resolution, the supported Wukong local-file
send path after retiring the legacy credential-based media upload command from
discovery, and reliable message-read rendering for rich content, forwarded
records, encrypted messages, and media-download ID aliases.

### Added

- **MCP URL resolution** — adds `dws mcp url get <mcpId>` for resolving a DingTalk MCP Market ID to the current user and organization scoped Streamable HTTP URL, while keeping the helper-only `mcp-meta` endpoint out of the public product command surface.

### Changed

- **Chat local-file sending** — hides the open-source-only `chat media upload` compatibility command from Help, Schema, and bundled Skills, and removes its legacy AppKey/AppSecret OAPI path. Historical argv still receives an actionable migration error. Send local images and files through `chat message send --msg-type file --file-path`; callers that already hold a mediaId may continue to use `--msg-type image --media-id`.

### Fixed

- **Shortcut projection silent-empty returns** (#783) — a batch of read shortcuts returned an empty list with exit 0 and no error envelope even when the underlying MCP tool returned data, so agents misread "no data". The projection resolvers now probe the real container keys (`processCodeList`, `values`, `wikiSpaces`, `itemList`, `groupList`, `recentItems`, `emailAccounts`, `deptUserList`, `labelUserList`, `roles`, `report_list`, and the grouped `get_org_labels` `labels[]`), unwrap items nested under a VO wrapper (`shiftVO` / `entityVO` / `userInfo`), and `todo +created-todos` uses the shared pager (`pageSize=20`) because the backend silently returns an empty page for `pageSize>20`. Affects contact/oa/wiki/drive/minutes/calendar/attendance/chat/report/smart shortcuts, each with a guard test asserting the real response shape projects non-empty. `scripts/shortcut_real_result.py` also gains an upper-vs-lower layer comparison so an exit-0 empty projection over a non-empty backend is scored as `projection-data-loss` in the real read-audit path rather than `real-ok`.
- **Message-read shortcut projection** (#706) — the message-list shortcuts (`chat +chat-messages` / `+messages-list` / `+messages-list-direct` / `+at-me` / `+search-msg` / `+thread-replies`) now render card and out-of-office rich-content JSON as readable text (without ever rewriting ordinary text that merely embeds a JSON fragment), expand a forwarded chat record's nested `forwardMessages` instead of collapsing to a "[卡片]" summary, and mark undecryptable encrypted card messages as `[加密消息]`; the speaker is read from the bare `sender` key, nested `{name:…}` sender objects yield their display name, and the literal string `"null"` is treated as absent. Shared projection helpers now live in `internal/shortcut/chatmsg`. `chat message download-media` also gains `--msg-id` / `--open-message-id` aliases for its `--message-id` flag so agents copying the `openMessageId`/`msgId` output field no longer hit "unknown flag".

## [1.0.54] - 2026-07-21

This release promotes the validated `v1.0.54-beta.2` baseline to stable. It restores the default transport envelope for personal event output with opt-in flattening, plus Schema CLI path and plugin overlay compatibility fixes.

### Changed

- **Personal event output compatibility** (#743) — `event consume` once again preserves the transport envelope by default for `ndjson`/`json`/`pretty`, while retaining the existing `compact` processor. New Agent workflows opt into the event-specific top-level DTO with `--flatten`, which is mutually exclusive with `-f raw` and `--debug-raw-events`; `event schema --flatten` describes that DTO, while the default schema describes `type/event_type/data/headers` and points to `.data | fromjson`.

### Fixed

- **Schema CLI path compatibility** (#738) — user-facing Schema lookups once again accept space-, dot-, and slash-separated CLI paths without weakening strict canonical identity resolution.
- **Plugin CLI overlays** (#701) — installed plugins register their manifest-authored command trees again for HTTP and stdio servers, and a plugin may now replace a hidden compatibility fallback (for example `conference`) instead of being skipped as a distribution conflict.

## [1.0.54-beta.2] - 2026-07-21

This beta revalidates the same `v1.0.54-beta.1` source through the cloud release path with a sealed `OSS-Mirror: deferred` policy, because the manually tagged `v1.0.54-beta.1` push run failed on the unavailable OSS mirror channel after GitHub and npm delivery.

### Changed

- **Release delivery only** — no source changes since `v1.0.54-beta.1`; see that section for the user-visible changes under validation (#743, #738, #701).

## [1.0.54-beta.1] - 2026-07-21

This beta validates the restored default transport envelope for personal event output with opt-in flattening, plus Schema CLI path and plugin overlay compatibility fixes, on top of the validated `v1.0.53-beta.7` baseline.

### Changed

- **Personal event output compatibility** (#743) — `event consume` once again preserves the transport envelope by default for `ndjson`/`json`/`pretty`, while retaining the existing `compact` processor. New Agent workflows opt into the event-specific top-level DTO with `--flatten`, which is mutually exclusive with `-f raw` and `--debug-raw-events`; `event schema --flatten` describes that DTO, while the default schema describes `type/event_type/data/headers` and points to `.data | fromjson`.

### Fixed

- **Schema CLI path compatibility** (#738) — user-facing Schema lookups once again accept space-, dot-, and slash-separated CLI paths without weakening strict canonical identity resolution.
- **Plugin CLI overlays** (#701) — installed plugins register their manifest-authored command trees again for HTTP and stdio servers, and a plugin may now replace a hidden compatibility fallback (for example `conference`) instead of being skipped as a distribution conflict.

## [1.0.53] - 2026-07-21

This release promotes the validated `v1.0.53-beta.7` baseline to stable. It adds enterprise onboarding, declarative shortcuts, Sheet/Aitable writes, multi-account profiles, and broader personal IM events, while hardening authentication and the guarded release path.

### Added

- **Enterprise and office command coverage** — adds enterprise creation, employee invitation, and account provisioning commands; 366 declarative service shortcuts; Sheet import commands; and Aitable workflow create/update support with reviewed Schema contracts.
- **Multiple accounts in one DingTalk organization** — profiles can distinguish accounts by organization and user, select them explicitly, and log out one account or an entire organization without overwriting another account's credentials.
- **Expanded personal IM event subscriptions** (#651) — adds read-receipt, recall, and reaction events for one-to-one and group chats, plus specified-sender subscriptions by staff ID or OpenDingTalk ID.
- **Official multi-platform Homebrew channel** — ships separate stable and keg-only beta Formulae for macOS and Linux across amd64 and arm64, with isolated update PRs.

### Changed

- **Personal event output contract** (#651) — `event consume` now emits event-specific top-level structured fields; scripts that consumed the former transport envelope must use the flat fields or select `-f raw`, while `--debug-raw-events` retains the diagnostic envelope.
- **Guarded release lifecycle** — beta/stable publication now uses explicit promotion, immutable delivery proofs, protected recovery, and tag-bound optional OSS policy; an unprovisioned OSS mirror is sealed as `deferred` so GitHub, npm, and Homebrew are not blocked.
- **Relaxed stable promotion contract** (#729) — a stable release still requires a delivered, non-withdrawn beta baseline in its commit history, but no longer requires a byte-identical tree with that beta; reviewed commits merged to `main` after the beta can now ship in the stable release. Local releases now accept any sealed commit contained in `main` history and push only the release tag, so `main` is never frozen during the beta-to-stable window.

### Fixed

- **Authentication and credential reliability** — organization-policy denials stop before mutation or polling, long-running clients reload and refresh access tokens consistently, concurrent credential writes are atomic, and Windows portable-auth commands fail before reading or writing unsupported credential bundles.
- **Command validation and compatibility** — invalid Sheet/task targets fail locally, IM shortcuts preserve AI-tag and alias compatibility, and Aitable import uploads require and forward a positive file size.
- **Release publication reliability** — GitHub draft publication is bound to one verified release ID and exact assets, preflight uses isolated installer worktrees, guarded local tags remain compatible, cloud planning fingerprints the actual allocated release refs, and npm channel verification waits for bounded registry propagation without moving tags.
- **Package-manager version verification** (#735) — npm-vendored, Homebrew-installed, and packaged release binaries are now verified by searching their raw bytes for the injected version marker, so a correctly versioned stable binary is no longer rejected when the short version marker coalesces with adjacent printable linker metadata; incorrect or missing markers still fail closed.

## [1.0.53-beta.7] - 2026-07-21

This beta validates bounded npm channel verification after registry publication.

### Fixed

- **npm dist-tag eventual consistency** — Release delivery now tolerates a briefly stale `latest` or `beta` read after publishing by retrying only when npm reports a valid older version. Registry errors, invalid or incomparable tags, and channels that never converge still fail closed without moving any tag during verification.

## [1.0.53-beta.6] - 2026-07-21

This beta validates guarded local release compatibility and tag-bound OSS deferral so an unprovisioned mirror cannot block the primary release channels.

### Changed

- **Tag-bound optional OSS release mirror** — Official cloud Release runs no longer block GitHub, npm, and Homebrew delivery when an OSS bucket has not been provisioned. Cloud tags immutably record `OSS-Mirror: enabled|deferred`; publication, repair, and withdrawal consume that sealed policy instead of the current repository variable. Enabled releases remain fail-closed, while deferred releases skip the nonexistent channel and cannot be backfilled without a future audited repair proof.

### Fixed

- **Guarded local release compatibility** — The tag-push Release workflow now accepts the `Channel`-only annotated tags created by the guarded local release entry while continuing to reject any partial cloud-only seal metadata.
- **Cloud release tag allocation fingerprint** — Release planning now fingerprints the actual `v*` and `withdrawn/v*` refs fetched from GitHub, matching the seal job's API view instead of hashing an empty non-wildcard ref prefix and rejecting every publish before tag creation.

## [1.0.53-beta.5] - 2026-07-21

This beta validates long-running access-token recovery and the faster, recoverable guarded release path introduced after v1.0.53-beta.4.

### Changed

- **Fast guarded beta and stable releases** — successful local release checks now leave a six-hour proof bound to the exact version, commit, repository identity, remote `main`, and stable baseline, so the subsequent guarded `--publish` invocation revalidates authority without repeating tests and packaging. A default-branch governance smoke uses the same dedicated immutable-release credential as the tag workflow before any tag is allocated.
- **Protected existing-tag recovery** — `dws-release recover <version>` can resume a failed, unpublished annotated tag through the normal contract, build, Developer ID signing, immutable GitHub Release, Homebrew, npm, and OSS jobs. Recovery requires the exact tag object, peeled commit, failed tag-push run, typed version confirmation, and the protected `release-recovery` environment; successful runs are accepted as future beta/stable delivery evidence.

### Fixed

- **Long-running event authentication recovery** — personal and portal event streams resolve the current access token for every ticket request, refresh a server-rejected token with compare-and-refresh semantics, and reconnect with backoff when refresh is temporarily blocked by network failures, rate limits, or 5xx responses.
- **Consistent access-token caching and errors** — runtime, recovery, Skill, PAT polling, and personal/portal event clients now resolve user access tokens through one expiry- and publication-aware manager, so long-running processes reload rotated credentials while keychain, refresh, parse, permission, and cancellation failures remain observable instead of being collapsed into “not authenticated.”
- **Tag-push GitHub Release publication** — Draft publication now locks one GitHub Release database ID, verifies its exact tag, channel, notes, recovery marker, asset set, and uploaded bytes, then publishes and rechecks that same ID as immutable. Recovery runs use the trusted default-branch release helpers instead of the sealed tag's historical scripts, fixing the Draft-only `GET /releases/tags/{tag}` 404 without allowing the release identity to drift during recovery.
- **Release preflight reliability** — source-mode installer tests now use isolated temporary checkouts and HOME directories instead of overwriting and deleting the real repository `dws` binary, release preflight explicitly rebuilds before policy checks, and the full-suite runner gives the growing script package a non-flaky five-minute per-suite budget.

## [1.0.53-beta.4] - 2026-07-17

This beta validates the expanded personal IM event subscriptions and the flattened `event consume` structured output introduced after v1.0.53-beta.3.

### Added

- **Expanded personal IM event subscriptions** (#651) — adds one-to-one and group events for message read receipts, recalls, and reactions; publishes the specified-sender receive event; and lets one-to-one/sender subscriptions target either a staff `--user` or an `--open-dingtalk-id`. Event Schema now exposes these alternatives through machine-readable parameter constraints.

### Changed

- **Personal event structured output is now flat** (#651) — `event consume` projects NDJSON/JSON/pretty/compact output into event-specific top-level DTOs, so consumers read fields such as `content`, `sender`, and `conversation_id` directly instead of parsing `.data | fromjson`. This is a breaking change for scripts using the former transport envelope; the original server payload remains available through `-f raw`, while `--debug-raw-events` preserves the full diagnostic envelope.

## [1.0.53-beta.3] - 2026-07-17

This beta validates multi-account profile support and the post-v1.0.53-beta.2 compatibility fixes for Windows portable authentication, IM shortcuts, and Aitable import uploads.

### Added

- **Multiple accounts in one DingTalk organization** — profiles are keyed by `corpId:userId`, `--profile` accepts organization IDs/names plus user IDs/names, and organization-only selection uses its explicitly remembered current account or asks for an exact account when ambiguous.

### Changed

- **Profile-scoped logout and consistent token storage** — `dws auth logout --profile` can remove one account or every account in an organization, while identity token slots remain the source of truth and legacy organization/global mirrors stay compatible without overwriting newer account credentials.

### Fixed

- **Windows portable-auth contract** — `dws auth export` and `dws auth import` now fail early without reading credentials, bundles, or writing files instead of claiming portable-bundle support for DPAPI-protected HKCU Registry credentials.
- **IM shortcut message tags and compatibility aliases** (#646) — IM send shortcuts now add the same AI-sent marker as `chat message send` by default, support `--ai-tag=false` to opt out, and preserve compatible search, conversation-ID, and page-size aliases.
- **Aitable import upload file-size validation** (#654) — `dws aitable import upload` and `dws aitable +import-upload` now require a positive `--file-size` and always send it to the upload-preparation API, preventing invalid requests without the actual file size.

## [1.0.53-beta.2] - 2026-07-16

This beta validates the accumulated post-v1.0.52 command surface, release automation, and runtime hardening changes, including enterprise contact onboarding, declarative shortcuts, Sheet/Aitable writes, multi-platform Homebrew formulas, and credential and target-validation fixes.

### Added

- **Contact enterprise onboarding commands** — adds `contact org create`, `contact user invite`, and `contact account create` for creating a DingTalk enterprise, inviting an employee by mobile, and provisioning an enterprise login account, with reviewed Schema contracts and mono/multi Skill routing.
- **Declarative shortcut commands** (#592) — adds 366 `dws <service> +<command>` shortcuts across 16 services, including one-to-one MCP wrappers and multi-step smart workflows. Shortcuts publish stable Agent-visible contracts with named flags, validation and confirmation metadata, dry-run protection for writes, catalog/help routing, and optional local YAML extensions and usage recording.
- **Sheet imports and Aitable workflow writes** (#624) — adds `dws sheet import` / `sheet import create` for converting local xlsx/xls files into new online sheets, `sheet import get` for polling import tasks, and `dws aitable workflow create/update` for applying validated `workflow-dsl/v1` definitions, with matching reviewed Agent Schema and bundled Skill guidance.
- **Official multi-platform Homebrew channel** — stable `Formula/dingtalk-workspace-cli.rb` and keg-only `Formula/dingtalk-workspace-cli-beta.rb` live in this repository and select signed macOS Intel/Apple Silicon or Linux amd64/arm64 artifacts at install time. Stable and beta releases open isolated Formula update PRs after final artifact signing, so beta never replaces the stable Formula. Agent Skills stay under `pkgshare` without mutating the user's home directory, and both tracks are covered by the six-channel post-release verifier.

### Changed

- **Guarded prerelease and stable automation** — adds the guided `dws-release` entry for one-command CHANGELOG preparation, validation-only and annotated-tag publication flows; promotes only an explicitly validated beta; verifies command-tree compatibility and all six packaged binaries; and serializes immutable GitHub Release, npm channel, OSS, Homebrew, and optional Gitee delivery with fail-closed recovery checks.
- **Reviewed historical release recovery proofs** — release preflight can recognize an explicitly pinned successful recovery delivery for a historical stable tag while still rejecting arbitrary workflow dispatches, mismatched commits, and incomplete release, signing, or publication jobs.

### Fixed

- **PAT organization-policy denials stop immediately** — `PAT_ORG_POLICY_DENIED` now remains terminal even if a backend also returns `flowId`, authorization URLs, or client credentials; the CLI does not mutate process credentials, open a browser, poll, or retry until an organization administrator changes the policy.
- **Sheet and task invalid-target failures** — `sheet range read/get` now rejects a null cell-info response instead of printing `null` and exiting successfully, while task completion and attachment listing verify that a task exists before calling lenient backend endpoints. Attachment listing is also published through Runtime Schema for schema-first Agent discovery.
- **Concurrent credential writes and reentrant CLI execution** — secure-token writers now use isolated, exclusive temporary files before atomic replacement so concurrent processes cannot remove each other's in-flight data, and repeated in-process CLI runs close the previous file logger before replacing it instead of retaining the prior log-file handle.

## [1.0.52] - 2026-07-14

This release seals the `v1.0.52` line with personal event subscriptions, a deterministic 22-product Agent command catalog, local user-operation auditing, expanded Open product commands, safer macOS credentials and release signing, and more reliable Connect and IM delivery.

### Added

- **Personal event subscriptions** (#589) — adds `dws event list/schema/consume/status/stop` for user @ mentions, selected one-to-one chats, and selected group chats. `consume` can create or reuse a personal subscription, multiple local consumers share one bus while keeping outputs isolated by event type and subscription, and the mono/multi event Skills ship with the binary.
- **Open product command capabilities** (#608) — adds Sheet table, pivot-table, and gridline commands; Chat message favorites; Drive statistics and shortcuts; and Doc comment update/delete, with matching mono/multi Skill documentation and command-contract coverage.
- **Local user-operation audit log** (#555) — operations executed through `dws` now produce redacted daily JSONL records with actor, command and endpoint, result or error category, duration, CLI/platform metadata, and a SHA-256 previous-hash chain for tamper evidence. Writers coordinate through a cross-process file lock and rotate logs safely; `dws audit tail` inspects recent records, `dws audit export` emits date-filtered JSONL or CSV, and `dws audit verify` reports the first broken link in a file's hash chain.
- **Stable Agent command catalog** (#598) — `dws schema` now ships a deterministic 22-product / 564-tool catalog generated from the executable Cobra tree, with progressive product/group/leaf queries, complete parameter contracts, reviewed command identity and aliases, safety/confirmation metadata, field provenance, and final-delivery completeness/drift gates. The catalog is embedded at build time and does not require runtime MCP `tools/list` discovery.
- **Reviewed Schema for local commands** (#598, #609) — `event consume/list/schema/status/stop` and `audit export/tail/verify` enter the reviewed `CommandRegistry`, bind to the real Cobra tree at generation time, and ship through the same typed `ToolSpec` and embedded Catalog path as public MCP-backed commands. Leaf, group, product, and `--all` queries are projections of that single delivered model.
- **Safe macOS Keychain → file-DEK migration** (#597) — `dws auth migrate-keychain --to file-dek` preflights every legacy/profile auth entry before rewriting, ignores unrelated application secrets, supports side-effect-free `--dry-run`, requires explicit `--yes`, and lets sandboxed and normal processes share an existing login without exposing tokens.

### Changed

- **`event consume` AI-subprocess contract** (#609) — emits a fixed ready line and a final controlled-exit summary, supports parent-pipe stdin EOF as graceful shutdown, forwards `--profile` to the detached bus, surfaces bus startup errors, and cleans up subscriptions according to ownership so orchestrators can drive event streams without sleeps or leaked server-side subscriptions.
- **Wukong IM read-result parity** (#618) — `chat message list` preserves quoted merged-forward and image context; message-search entitlement failures retain the server-provided friendly hint and action URL; and `ding message list` exposes each DING's content alongside its ID and status.
- **Developer ID signing for official macOS archives** (#605) — official releases now require both Darwin archives to be signed with the configured Apple Developer ID certificate, timestamp, and hardened runtime. The release job validates credentials and signatures and fails closed instead of silently publishing ad-hoc-signed official binaries.

### Fixed

- **Smart-category mappings and runtime network diagnostics** (#591) — `chat category create-smart` now maps category names, group-name keywords, and member OpenDingTalk IDs to the live MCP contract, rejects blank or empty supplied values locally, and reports runtime `tools/call` connection failures as actionable API/network errors instead of internal discovery failures.
- **Connect daemon restart lifecycle** (#599) — pins the Stream SDK reconnect-race fix, snapshots the running executable before detaching, uses a real 30-second keepalive, and manages each worker as its own Unix process group so launcher cleanup or worker panics no longer cause restart loops or orphan local-agent processes.
- **Complex Connect messages and attachments** (#606, #612) — rich-text messages retain all embedded pictures in order, queued turns keep every pending attachment, and unknown or future callback shapes reach each Agent backend with their message type and raw JSON instead of being discarded. Attachment recovery is locator-based, nested `chatRecord` pictures/audio/video/files can be recovered from message APIs after Stream ACK, and OpenCode uses a full-duration storyboard for large videos to avoid base64 OOMs while preserving the original download for the turn.
- **macOS auth survives Keychain mode changes** (#597) — credential reads try existing compatible DEKs without creating key material, updates preserve the DEK that decrypted existing ciphertext, unreadable slots fail closed before token exchange, profile slots use the canonical auth backend, and `auth status` reports ciphertext/key mismatches instead of treating them as ordinary logout. Dedicated macOS race and Windows DPAPI coverage protect the cross-platform paths.

## [1.0.51] - 2026-07-10

This release promotes the sealed `v1.0.51-beta.1` contents to stable. It syncs the hardcoded Wukong command surface, prevents `dev connect` conversations from blocking on messages received mid-turn, and makes local credential failures diagnosable without mutating key material.

### Added

- **Agoal product commands** (#585) — adds `dws agoal` strategy, contract, scorecard, user-objective, report, and objective-template command groups, together with static routing and the bundled mono/multi Agoal skills.
- **Wukong chat command parity** (#585) — adds `chat group notice create|edit|get|list`, `group share-invite`, `text translate`, `category create-smart`, and `message list-emotion-replies`.
- **Wukong document import commands** (#585) — adds `doc import` for starting imports and `doc import get` for querying import tasks.
- **Wukong mail command parity** (#585) — adds mailbox profile, message batch-get, sent-message recall and recall-detail, auto-reply update, plus allow-list and block-list management.
- **Wukong sheet grouping commands** (#585) — adds `sheet group-dimension` and `sheet ungroup-dimension` for whole-row or whole-column ranges.
- **Keychain health diagnostics** (#578) — `dws doctor` now includes a keychain check, while `dws auth status` distinguishes ordinary logged-out state from `keychain_unavailable` and `dek_missing` failures and returns remediation hints in table and JSON output.

### Changed

- **`dws pat chmod` defaults to permanent grants** (#584) — running `dws pat chmod <scope>` without `--grant-type` now requests a `permanent` grant instead of `session`, aligning the direct CLI path with the recommend-authorization helper. Session grants remain available by passing `--grant-type session --session-id <id>`.
- **The `dev connect --channel gemini` path now uses the Gemini `generateContent` API** (#587) — configure it with `GEMINI_API_KEY` or `GOOGLE_API_KEY`, optionally override the compatible endpoint with `GEMINI_API_BASE_URL` or `GOOGLE_GEMINI_API_BASE_URL`, and select a model with `--agent-model` or `GEMINI_MODEL`; a local `gemini` executable is no longer required.

### Fixed

- **Non-blocking `dev connect` turn scheduling** (#587) — stream and `@`-poll callbacks no longer wait for the active turn to finish. Turns stay serialized per conversation, messages received mid-turn are coalesced into one pending follow-up, and different conversations can continue in parallel.
- **Connect agent recovery and headless execution** (#587) — stale addressable sessions retry once with a fresh session, unsupported Qoder control requests receive an immediate response instead of hanging, OpenCode and bypass-mode channels receive non-interactive permission settings, and backend/API failures are no longer posted as successful assistant replies.
- **Side-effect-free credential reads** (#578) — keychain reads inspect encrypted credential data before looking up the DEK and never generate a replacement key on a read path. Missing DEKs and unavailable macOS Keychains are surfaced as explicit diagnostic failures instead of silently mutating credential state.

## [1.0.50] - 2026-07-08

This release fixes a long-standing gap where the global `--jq` / `--fields` output filters were silently ignored on product commands, lands a JSON-mode output path for the sheet batch-style command, and aligns the bundled skill surface with the real command semantics uncovered by the round-2 real-machine QA sweep.

### Fixed

- **Global `--jq` / `--fields` are honored on product commands** (#575) — `Formatter.PrintJSON` / `PrintJSONUnescaped` now route through `output.WriteFiltered` when either flag is set, so product commands accept the same filters that `dws api` has always supported. The tool-caller adapter exposes `Fields()` / `JQ()` so helpers can read the flags without re-parsing.
- **`skill setup --dry-run` is a no-op preview** (#575) — it now prints what would be written without touching the skill directory, the registry, or the agent config. Help text and docs are updated to match.
- **Skill docs alignment to the real command surface** (#575) — per-product references and the cross-product intent guide clarify that `--fields` projects top-level / list keys only (use `--jq` for nested paths); `minutes_extract_todos.py`, `calendar_free_slot_finder.py`, `chat_export_messages.py` / `chat_history_with_user.py`, and `contact_dept_members.py` are rewritten against the current response shapes; `aisearch` / `aitable` / `attendance` / `calendar` / `chat` / `contact` / `dev` / `doc` / `doc-comment` / `doc-file-ops` / `doc-list` / `doc-search` / `drive` / `mail` / `minutes` / `oa` / `sheet` / `sheet-export` / `url-patterns` / `best_practices/lite-recipes.md` / `global-reference.md` / `intent-guide.md` are re-synced; the QA voice ("真机" phrasing) and environment-specific quirks stated as absolute rules are removed from the docs.

### Changed

- **`sheet range batch-set-style` emits per-row JSON in JSON mode** (#575) — when `--format json` is set, each update is reported as `{index, sheetId, range, ok, error}` instead of only the final aggregate, so callers can programmatically track partial failures under `--continue-on-error`.
- **Command-merge helpers exported** — `pkg/cmdutil.LeafMerge*` and the provenance helpers are now public so downstream command trees can reuse the same merge semantics.

## [1.0.49] - 2026-07-08

This release lands a full real-machine QA sweep across the CLI, helper scripts, and skill docs (#572), and hardens the release pipeline so npm publishing can no longer be blocked by Gitee mirror issues (#570).

### Fixed

- **Real-machine QA fixes across CLI commands** (#572) — `aitable chart/dashboard share update --enabled` now takes a string so `--enabled false` disables; `chat conversation-info --user` resolves openDingTalkId and registers `--id/--conversation-id/--chat` aliases; `chat list-all-conversations --limit` is capped at 100 and rejects larger values; custom-robot webhook failures surface `errcode` instead of masquerading as success; `contact` registers `--dept/--depts` as the primary flags so the documented spelling actually works; `sheet media-upload` and `sheet export` emit clean JSON under `--format json` (progress lines no longer leak); `wiki node create --type` enum is corrected (drops unsupported `asheet`, adds `axls/able/appt/adraw/amind`); `ding message list --type` defaults to `ALL` since the server rejects empty type.
- **Helper script fixes (mono and multi)** (#572) — aitable import/export flag names and the tableId regex (7-char default tables were rejected); mail search `--limit`, contact dept response keys (`deptList`/`deptUserList`) and `userInfo` nesting; `attendance_my_record` whoami compatibility; `calendar_schedule_meeting` event-id unwrapping; `drive_tree_list` recursion via `fileId`; report scripts migrated off the deprecated `report list`/`report detail`.
- **Skill docs sync (mono and multi)** (#572) — command indexes, flag names, enums, return-structure keys and cross-product intent routing are re-aligned to real-machine behavior across all products. Genuinely server-side limitations (permission gates, org-level restrictions, unregistered tool keys) are annotated instead of code-patched, and the cross-cutting hazards (`success` always true, `--jq`/`--fields` currently no-op) are documented.

### Changed

- **Release pipeline unblocks npm publish from Gitee mirror** (#570) — the Release workflow now publishes to npm before touching the Gitee mirror, so Gitee upload issues cannot block `npm/latest`. GitHub→Gitee attachment upload is disabled by default (unreliable from US runners) and only runs when `ENABLE_GITEE_UPLOAD_FALLBACK=true`; the legacy upload fallback path is guarded with timeout and retry so it fails fast when re-enabled.
- **Repair modes for release republish** (#570) — the Release workflow gains a repair input and a standalone npm-only repair workflow, used to republish an existing release to npm without re-running the full pipeline.

## [1.0.48] - 2026-07-07

This release promotes the sealed **remove-discovery delivery** from the beta line to the stable `v1.0.48` package. It removes dynamic service discovery from the open-edition runtime, keeps legacy CLI compatibility aliases, syncs the open command/help/skill surface with the dws-wukong baseline, and includes the `dev connect` default-yolo behavior on the stable upgrade track.

### Changed

- **Remove-discovery delivery is now formal/stable** — the beta validation line is ready to cut as `v1.0.48`; normal stable channels (`dws upgrade`, GitHub `releases/latest`, install scripts, and npm `latest`) should receive this release after the official tag is published.
- **Static endpoint runtime sealed for stable delivery** — the open edition no longer depends on dynamic service discovery at runtime, while preserving legacy command compatibility aliases and the synced help/skill surface from the beta.
- **`contact label` is restored as real wukong-compatible functionality** — `dws contact label list/get/list-members` now call `get_org_labels`, `search_label_by_name`, and `get_label_members_by_labelId`; `contact role` remains an alias, and the common top-level compatibility entries (`contact search/find/list/get/self/me/whoami/get-self`) now dispatch to real user/dept/label tools where unambiguous.
- **Skill docs match the sealed command surface** — contact docs again describe the real `contact label` three-step role lookup flow; video-conference start/invite/share flows remain explicitly unsupported and point users to the DingTalk client.

### Fixed

- **`calendar event list --dry-run` no longer executes the real list call** — the sorted event-list wrapper now respects dry-run and prints the `list_calendar_events` preview instead of calling the backend.
- **`chat file upload` is downlined** — the hidden compatibility entry now returns a clear downline message and never calls `chat/upload_conversation_file_by_url`; the supported file path remains `chat message send --msg-type file --file-path`.
- **Optional plugin version validation no longer pollutes every command** — incompatible local plugins such as conference are skipped at debug level during command-tree construction instead of printing a WARN on unrelated commands.
- **PR #45 review follow-ups are folded into the release** — doc version rollback pagination now unwraps nested result/content/data envelopes for `nextCursor`, mail helper scripts handle `{result:{emailAccounts:[...]}}`, and the generated attendance `.xlsx` fixture is removed from the skill scripts.

### Tests

- **Command-surface regression tests** — root-command tests now cover real `contact label`/`role` dry-runs, hidden top-level contact compatibility entries, `chat file upload` downline behavior, and `calendar event list --dry-run`.
- **Release hygiene tests** — skill markdown policy still blocks unsupported conference routes, plugin loader tests assert optional validation failures stay quiet at WARN level, and doc version cursor extraction has nested-envelope coverage.
## [1.0.47] - 2026-07-05

This release adds **connector supervision & health monitoring** (`dev connect list/status/restart/stop`) and fixes **bot-to-bot @-mention** delivery end-to-end.

### Added

- **`dev connect list`** — PM2-style colored table enumerating all local connectors with state (healthy / degraded / down / not_running), PID, channel, and uptime.
- **`dev connect status`** — panel view with heartbeat, last recv timestamp, session webhook age, and `--json` for external monitoring.
- **`dev connect restart`** — restarts a daemon via persisted `daemon-state.json` (unified-app-id credential fetch, no local secret storage).
- **`dev connect stop`** — graceful SIGTERM shutdown releasing the single-instance lock and Stream connection.
- **Health watchdog** — background goroutine writes `heartbeat.json`; `status`/`list` derive state from heartbeat freshness + process liveness + pid-reuse detection.
- **`--alwayson` flag** — opt-in auto-restart: supervisor relaunches the worker on crash (requires `--daemon`).
- **`--notify-staff-id`** — state-change notifications (start / stop / crash) sent as DingTalk messages to the specified staffId.
- **`--unified-app-id` credential flow for `dev connect`** — fetches clientId/clientSecret at startup via `dev app credentials get`, keeping secrets off the command line and out of `daemon-state.json`.
- **API-sent file download** (`feat(connect): download API-sent files via storage v2 API`) — file messages sent via `dws chat message send --msg-type file --dentry-id --space-id` are now downloaded by the connector through the storage v2 `getDownloadInfo` API (dentryId + spaceId → presigned URL → local temp file), so file-based Q&A works regardless of how the file was sent.
- **`--at-open-dingtalk-ids` for `chat message send-by-bot`** — @-mention bots or cross-org users by openDingTalkId in group messages.

### Fixed

- **Bot-to-bot @-mention send side** — `atOpendingtalkIds` (the server's lowercase spelling) is now used instead of the camelCase `atOpenDingTalkIds` which was silently ignored. The unnecessary `openDingTalkId → userId` reverse lookup (always failed for bots) is removed; the id is forwarded verbatim.
- **Bot-to-bot @-mention receive side** — `interactiveCard` messages (how DingTalk delivers a bot @-mentioning another bot) are now parsed: `extractInteractiveCardText` flattens `cardContent[].children[].value` leaves and strips the leading @-mention by leaf boundary. The `emotion/reply` reaction (which 500s on bot-sent cards) is skipped for `interactiveCard` turns.
- **Markdown/richText body extraction** — `extractCallbackText` gains a `cardContent` fallback so structured-text messages are no longer silently dropped.
- **Send-by-bot @ chip rendering** — `<@id>` placeholders in the markdown body are rewritten to `@id` for both userIds and openDingTalkIds so the mention chip renders in all cases.
- **Connector retry on transient network errors** — `sendBySession` retries on transient failures instead of dropping the reply.
- **Orphan worker cleanup & watchdog deadlock** — stale workers from a crashed supervisor are detected and cleaned; a channel-capacity fix prevents the watchdog from blocking.
- **Idle connector false-down** — heartbeat ticker now advances `updatedUnix` so a connector with no inbound traffic is not marked degraded.
- **FD limit check** — `checkFDLimit` split into platform files for Windows cross-compilation.
- **Default agent timeout removed** — no timeout by default (was incorrectly defaulting to a low value).
- **keepAlive shortened to 30 µs** — aligns with Stream SDK expectations; adds `ulimit` check for multi-agent stability.

## [1.0.46] - 2026-07-01

### Fixed

- **PAT agentCode grants no longer split from follow-up command checks** (`internal/auth/agent_code_detect.go`, `internal/app/runner.go`, `internal/pat/chmod_test.go`) — explicit `DINGTALK_DWS_AGENTCODE` declarations are now forwarded verbatim as the common cross-host contract, and unknown hosts no longer synthesize `custom` into `x-dingtalk-dws-agent-code` / `x-dws-agent-instance-id`. `pat chmod --agentCode` remains the highest-priority grant target and still wins over the env fallback.

## [1.0.45] - 2026-06-29

This release adds **multi-organization (profile) support** (#500): `dws` can stay logged in to several DingTalk organizations at once and switch between them, while staying fully backward/forward compatible with the previous single-org token. A profile is one logged-in organization (corp); the current profile decides which org a command runs against. The release also hardens the new credential store for concurrency and corruption recovery, documents the capability in both the mono and multi skill sets, and flips `--ai-tag` on by default so messages sent through `dws` carry the DingTalk 「通过AI发送」 badge (#524).

### Added

- **Multi-organization login & `profile` management** (`internal/auth/profiles.go`, `internal/app/profile_command.go`) — `dws auth login` against a new organization adds a profile (the first login becomes the primary); `dws profile list` shows logged-in orgs with primary / current markers, status and validity; `dws profile switch <name|corpId|->` persistently switches the default org (`-` toggles back to the previous one, no-arg opens a TUI selector on a terminal); `dws profile use` is an alias of `switch`. `dws auth status [--profile <name>]` reports a specific profile. Credentials are stored per organization in keychain slots keyed by corpId (`auth-token:<corpId>`), with a plaintext `profiles.json` registry holding only metadata and the primary/current/previous pointers (no tokens).
- **Global `--profile <name|corpId>` flag** — run a single command against a specific organization without changing the default (one-shot; does not move currentProfile). Cross-org reads are orchestrated by the agent (list profiles → query each with `--profile` → merge); there is intentionally no built-in `--all-orgs`.
- **Backward / forward compatibility with the legacy single token slot** — a pre-existing single-slot token is migrated into `auth-token:<corpId>` and marked primary on first multi-profile use; the current (or primary) profile's token is mirrored back into the legacy slot so older binaries and the embedded host keep working. `profiles.json` is additive and ignored by older versions.
- **`dingtalk-profile` and `dws-shared` skills + multi-org documentation** (`skills/`) — a standalone `dingtalk-profile` skill plus a new `dws-shared` skill that carries auth, global flags and the multi-org rule, so every multi-mode product skill's PREREQUISITE resolves and all read/search skills inherit cross-org behavior. The mono skill gains a "multi-org / profile" section, trigger conditions, a decision-tree entry and a corrected logout danger note. Multi-mode install now always ships `dws-shared` even when `--skill` / `--exclude` narrows the set.

### Changed

- **`--ai-tag` now defaults on — DingTalk 「通过AI发送」 badge for dws-sent messages** (`internal/helpers/chat.go`, #524) — `chat message send` / `reply` flip the `--ai-tag` default from false to true, attaching the AI `clawType` by default so messages sent through `dws` (and by AI agents) transparently carry the 「通过AI发送」 badge; pass `--ai-tag=false` to send as the user with no badge.
- **Concurrency-safe, self-healing `profiles.json`** (`internal/auth/profiles.go`, `internal/auth/token.go`) — every read-modify-write on `profiles.json` and the legacy mirror is serialized under the existing dual-layer (process + cross-process) lock, split into public (locking) entry points and lock-free `*Locked` variants so the non-reentrant lock is never re-acquired (the refresh path and the load-path migration use the lock-free savers). `profiles.json` and the token marker are written via per-write random temp names + atomic rename so concurrent writers can no longer corrupt a fixed `.tmp`. An unparseable `profiles.json` is quarantined (`*.corrupt-*`) and rebuilt empty so the CLI self-heals; `auth reset` / `logout` proceed even when it cannot be read and sweep the quarantined files.

### Fixed

- **No silent fallback to a different org's token** (`internal/auth/token.go`) — when the resolved current/primary profile's keychain slot fails to read and no `--profile` was given, the loader now only falls back to the legacy single slot if it belongs to the same organization; otherwise it surfaces the error instead of acting as a different org.
- **Legacy mirror no longer wiped on a transient keychain read error** (`internal/auth/profiles.go`) — `SyncLegacyTokenMirror` distinguishes "token genuinely absent" from "keychain momentarily unreadable" and keeps the existing mirror in the latter case, so a host app's login state is not dropped by a transient failure.

## [1.0.44] - 2026-06-28

This release hardens the dynamic-command surface and finishes the dws-wukong parity pass for structured input. Phantom override commands whose backing MCP tool isn't deployed are hidden from `--help`; `report entry submit` reads `--contents-file` / stdin natively; structured JSON flags accept `@file` / `@-`; and `sheet range update` / `range read` now accept the same plain shapes wukong does (scalar cells, flat `values`, null-clears-cell, a `--hyperlinks` flag). On the wukong01 sandbox this lifts the full open-edition cli_to_mcp pass rate from 77.6% to 95.5% (sheet 28.5% → 99.8%, report → 100%); the remaining failures are account / org / out-of-scope, not CLI defects.

### Added

- **`dingtalk-dev` skill: image-upload → `mediaId` recipe + per-resource command discovery** (`skills/multi/dingtalk-dev/references/`) — documents how to obtain a `mediaId` for app / robot icons via the DingTalk OpenAPI (`credentials get` → `gettoken` → `/media/upload?type=image` → `--icon-media-id` → read back), since the dev command set has no upload command; and adds a "discovering commands" block to all 10 product refs pointing at each group's `--help` and `dws schema dev.app.<group>.<method>` (`dws schema dev.connect` for connect), so agents inspect commands instead of relying on memory.
- **`report entry submit --contents-file <path>` / `--contents -` (stdin) read natively** (#514, `internal/compat/report_hooks.go`) — the envelope publishes `entry submit` (MCP `create_report`) with a `--contents` (json_parse, required) flag plus a sibling `--contents-file` that had no transform / mapsTo, so a `--contents-file`-only submit silently sent `contents: [null]` and the report failed (only inline `--contents` worked, which is why `report create` succeeded while `report entry submit --contents-file` did not). A build-time compat hook now resolves the file / stdin natively (10MB cap, UTF-8 check, wukong priority `--contents-file` > `--contents -` > inline) and relaxes the individual `required` on `--contents` into a `contents` / `contents-file` one-of group. No discovery-config change needed.
- **`@file` / `@-` input for structured JSON flags** (`internal/compat/transform.go`) — `json_parse` / `json_parse_strict` now expand a leading `@` before parsing (`@-` reads stdin, `@<path>` reads a file), so long / complex payloads (many records, big 2D cell ranges, filter criteria) skip shell-quoting hell. A JSON / YAML value never starts with `@`, so the sentinel is unambiguous; the error hint that already advertised `@path/to/file.json` is now truthful. `sheet`'s shared `sheetParseJSONFlag` routes through `cli.ResolveInputSource` so the same support reaches `--values` / `--criteria` / `--sort-keys`.
- **`sheet range update --hyperlinks`** (`internal/helpers/sheet.go`) — a wukong-shaped 2D hyperlink grid (`[[{"type":"path","link":"...","text":"..."}]]`) overlaid onto the cells grid as each cell's `hyperlink` field; `--values` or `--hyperlinks` is now required (at least one).

### Changed

- **Phantom override commands hidden from `--help`** (#515, `internal/compat/dynamic_commands.go`) — override leaves whose backing MCP tool isn't actually deployed used to render in `dws <svc> --help` and then fail at invocation with *tool not found*. A tool-existence guard now hides them, and command groups left empty by the hidden leaves are collapsed, so `--help` reflects only invokable commands. Skill references are re-aligned to the real CLI surface (phantom commands dropped; role/duty "who is responsible" queries routed to `aisearch`, not `contact`).
- **`sheet range update` accepts scalar cells; `sheet range read` projects a flat `values`; `--values '[[null]]'` clears a cell** (`internal/helpers/sheet.go`, `internal/helpers/sheet_cell_validation.go`) — dws-wukong parity. `range update` (set_cell_range) auto-wraps a scalar cell (string / number / bool) into `{type:text,text:"..."}` instead of rejecting it, so the plain `[["姓名","部门"]]` shape that `sheet append` and wukong's update_range accept now works; a null cell clears content (matching wukong); `{}` still means keep-original. `range read` (get_cell_infos) now also exposes a flat `values` 2D array next to the rich `cells` payload, matching wukong's get_range shape without dropping cell styles.
- **report skill aligned to `entry submit` / `inbox list` / `outbox list`** (`skills/multi/dingtalk-report/`, `skills/mono/references/intent-guide.md`) — the multi skill tree was two versions behind and still taught the deprecated flat aliases (`report create` / `sent` / `list` / `detail` / `stats`) and falsely claimed `report inbox` was unimplemented. Re-aligned to the canonical resource.verb commands consistently (old aliases still execute with a stderr deprecation notice).

## [1.0.43] - 2026-06-26

This release aligns the open edition's CLI surface with **dws-wukong** across the communication domain (chat / mail / minutes / todo / calendar / contact / aisearch / live / report / ding) and the structured-office domain (aitable / sheet / drive / wiki / doc), and switches the discovery version code from `bamboo` to `cedar` so the aligned command tree is served from its own discovery config.

### Added

- **`calendar book get|search` and `calendar acl list`** (cedar discovery overrides) — query a specific calendar (primary via `--id primary`), fuzzy-search calendars by name, and list a calendar's access-control entries. Maps to the calendar MCP `get_calendar` / `search_calendar` / `list_acls` tools.
- **`calendar attendee list|add|delete`** (`internal/helpers/calendar_commands.go`) — manage event participants under the wukong-aligned `attendee` naming (equivalent to the legacy `participant` group; calls `get/add/remove_calendar_participant`).
- **`minutes tag list` and `minutes tag query --tag-id`** — list a user's AI-minutes tags and query minutes by tag (`query_user_tag_list` / `query_minutes_by_tag_id`).
- **`minutes list mine|shared|all`** (`internal/helpers/minutes_commands.go`) — list own / shared / all minutes with renamed output fields.
- **`mail folder create|update|delete`, `mail template create|list|get|update|delete`, `mail contact create|list|update|batch-delete`, and `mail message list`** — full mail folder / message-template / contact CRUD plus folder-scoped message listing.
- **`chat file upload`** (`internal/helpers/chat_file.go`) — upload a local file (init/PUT/commit) or a remote URL to a conversation's file space.
- **`todo task add-attachment`** (`internal/helpers/todo_commands.go`) — attach a local file to a todo (multi-step upload).
- **aitable extensions** (`internal/helpers/aitable_extra.go`) — advanced permission / roles, view sub-commands (lock / duplicate / frozen-cols / row-height / fill-color-rule / card / timebar), section node management, workflow enable/disable, record `upsert` / `share-url` / `history-list` / primary-doc, and field search-options. Helper tools route to the hardcoded `aitable-helper` supplement endpoint.
- **sheet, drive, wiki, doc helper coverage** synced from dws-wukong (`internal/helpers/sheet.go`, `drive.go`, `wiki.go`, `doc.go`).

### Changed

- **Discovery version code `bamboo` → `cedar`** (`internal/market/registry.go`; `discoveryAPIPath = "/cli/discovery/apis/cedar"`) — version codes step by first letter (bamboo → cedar → …); `cedar` carries the dws-wukong alignment. Older binaries keep reading `bamboo`, so the change is isolated to this release line. All test/mock/generator fixtures updated to the cedar path.
- **CLI output envelope aligned with wukong for cross-edition parity** (`internal/app/runner.go`, `internal/compat/registry.go`) — dry-run prints a `DRY-RUN Arguments:` line, successful results carry `success: true`, missing-required-flag wording is unified to `missing required flag(s): --x`, and OutputTransform applies to the response content layer.
- **New flag transforms** (`internal/compat/transform.go`) — `parse_bool` (explicit boolean strings so `--flag false` is honoured) and `attendance_class_check_time` (`HH:mm` → UTC+8 milliseconds for shift check-times).
- **`--calendar-id` accepted on calendar event / participant / room / attachment commands** so calendars other than the primary can be targeted.

### Fixed

- **Client-side validation** for calendar recurrence completeness and attendance schedule / class / group inputs, surfacing input errors before they reach the server.

## [1.0.42] - 2026-06-25

This release rounds out `dws dev connect` — bridge a DingTalk robot to your local AI (Claude Code / Codex / opencode / Qoder / …): a generic `custom` channel for any headless CLI tool, in-chat `/new` / `/clear` session commands aligned to each agent's real session op, and a fix for long opencode turns being cut at 30 seconds.

### Added

- **`dws devapp robot connect` — generic `custom` channel for self-built / unsupported AI tools** (issue #37; `internal/helpers/devapp_connect.go`, `internal/helpers/connect_stream.go`) — a new `--agent-cmd "<command>"` flag (and `custom` channel) lets the bot forward to any headless AI CLI that takes a question as its trailing argument and prints the answer to stdout, so tools that aren't built-in (e.g. 网易有道龙虾 LobsterAI) or self-built agents can be onboarded without code changes. `--agent-cmd` forces the `custom` channel unless `--channel` is set explicitly; detection also falls back to `custom` when `DWS_AGENT_CMD` is present.

### Changed

- **`robot connect` now hints how to match terminal answer quality** (issue #39; `internal/helpers/devapp_connect.go`) — when neither a work dir nor a knowledge source is configured, the connector prints a one-time note that the bot runs in a clean temp dir without local project context, pointing at `--agent-workdir` / `--knowledge-dir` / `--knowledge-source` / `--agent-model`. The robot quickstart gains matching FAQ entries, plus a clarification that step 3 (`robot connect`) produces no approval ticket (issue #19).

- **`robot connect` session commands `/new` vs `/clear` now use each channel's real session op** (PR #20; `internal/helpers/connect_opencode.go`, `internal/helpers/connect_stream.go`) — `/new` (and `/start`, `/reset`) opens a fresh session and leaves the previous one intact (resumable where the agent supports it); `/clear` actively disposes the current session through the agent's real delete primitive — opencode issues `DELETE /session/:id`. Channels whose agent exposes no delete in the mode DWS drives it (Codex app-server, Qoder stream, Claude-family exec) fall back to a reset, so `/clear` behaves like `/new` there. Previously both commands only dropped the local `conversationId → sessionId` mapping, so the two were indistinguishable and opencode sessions were never disposed (they leaked).

### Fixed

- **`robot connect` no longer aborts long opencode turns at 30 seconds** (PR #19; `internal/helpers/connect_opencode.go`) — the shared opencode HTTP client hard-coded a 30s `Timeout` that covered every request, including `POST /session/{id}/message`, so a long agent turn (e.g. a multi-minute research report) was killed mid-flight with `context deadline exceeded (Client.Timeout exceeded while awaiting headers)` even though the per-turn budget (`DWS_AGENT_TIMEOUT_MS`, default 300s) was far larger. The client-level deadline is removed so the per-request ctx governs the round-trip; only the `/global/health` probe keeps a short 10s timeout so startup detection stays snappy.

## [1.0.41] - 2026-06-24

This release makes the installers work from mainland China out of the box (no env var) and keeps the Gitee mirror in sync automatically.

### Added

- **Auto-fallback to the Gitee mirror when GitHub is unreachable** (#492; `scripts/install.sh`, `scripts/install.ps1`, `scripts/install-skills.sh`) — the installers probe GitHub Releases on startup and, when it is unreachable (typical in mainland China), automatically resolve the version and download every asset (binary, `checksums.txt`, `dws-skills.zip`) from the Gitee mirror instead. A plain `curl … | sh` now works in China with no `DWS_GITEE_REPO` needed. Explicit `DWS_GITEE_REPO` still wins, `DWS_NO_FALLBACK=1` forces GitHub, and local source-checkout installs skip the probe.

### Changed

- **CI mirrors repo code to Gitee automatically** (#493; `.github/workflows/mirror-to-gitee.yml`) — the mirror workflow now pushes `main` + tags to the Gitee mirror over HTTPS using `GITEE_TOKEN` (no SSH key), on every push to `main` and every tag, keeping the Gitee `raw/main` install scripts and tags in sync without any manual `git push`. Gated on `GITEE_TOKEN`; skips cleanly when unset.

## [1.0.40] - 2026-06-24

This release adds China-accessible install mirrors so the CLI installs reliably from mainland China, where GitHub raw + Releases are slow or fail.

### Added

- **China mirror via Gitee + npmmirror** (#486; `scripts/install.sh`, `scripts/install.ps1`, `scripts/install-skills.sh`, `scripts/release/sync-to-gitee.sh`, `.github/workflows/release.yml`, `.github/workflows/mirror-to-gitee.yml`) — an opt-in `DWS_GITEE_REPO` env var makes all three installers resolve the latest version and every release asset (binary, `checksums.txt`, `dws-skills.zip`) from the Gitee OpenAPI v5 instead of GitHub; with it unset, installation defaults to GitHub (fully backward compatible). The release pipeline mirrors release attachments to the matching Gitee release after each tag (gated on `GITEE_TOKEN`/`GITEE_REPO`), and a hub-mirror workflow keeps the repo code in sync (gated on `GITEE_PRIVATE_KEY`). README documents three China install channels: Gitee raw script, Gitee release binaries, and the npm package via `registry.npmmirror.com`.
- **Skills embedded in the binary** (#488; `skills_embed.go`, `internal/app/skill_setup.go`, `internal/app/skill_setup_embed.go`) — the `skills/` tree (mono + multi) is embedded into the `dws` binary via `go:embed` and `dws skill setup` defaults to the embedded copy, refreshing the installed skill instead of silently reusing a stale copy probed from the current working directory — so skills install offline with no separate download.

## [1.0.39] - 2026-06-18

This release makes the AI-sent indicator opt-in. 1.0.38 unconditionally tagged every user-identity send/reply with the edition claw identity, so the IM server rendered a "Send from AI" badge under every message — and on the open edition a stale hardcoded value even leaked the Wukong-branded label (「悟空AI发送」) to external users. The badge is now off by default and shown only when the caller explicitly asks for it.

### Added

- **`--ai-tag` opt-in flag for `chat message send` / `chat message reply`** (#477; `internal/helpers/chat.go`) — by default no `clawType` tool argument is attached, so delivered messages carry no "Send from AI" badge. Passing `--ai-tag` attaches `edition.ClawType()` so the IM server renders the badge (open edition `openClaw` → 「通过AI发送」; the wukong overlay sets its own value → 「悟空AI发送」). Covers the text/Markdown, rich-media, and `--user`/`--open-dingtalk-id` direct send paths plus `reply`. Bot (`send-by-bot`) and webhook sends are intentionally untouched — they already render as bot messages. The badge is opt-in so dws does not brand every message a user sends.

### Fixed

- **`dws chat message reply` no longer leaks the Wukong AI label on the open edition** (#475, fixes #474; `internal/helpers/chat.go`, `pkg/edition/edition.go`) — the reply path hardcoded `clawType: "wukong"`, so open-source quoted replies were tagged 「悟空AI发送」 by the IM server, leaking Wukong branding to external users (reported by an external customer integrating via openclaw). The value now derives from the edition via the new `edition.ClawType()` accessor (open → `DefaultOSSClawType` = `openClaw`), and — together with #477 — is only attached when `--ai-tag` is passed. The earlier fix existed on a branch (PR #450) but was never merged to main; #475 cherry-picked it.

## [1.0.38] - 2026-06-16

This release adds client-side agent attribution for usage stats, fixes two commands that silently misbehaved (`dws sheet export` hanging, `dws upgrade --dry-run` actually upgrading), hardens the document write path against server-rejected characters, and makes the long-broken `--no-browser` login flag actually work.

### Added

- **Client-side `agent_code` detection + per-channel agent instance id for usage stats** (#467; `internal/auth/agent_code_detect.go`, `internal/auth/identity.go`, `docs/agent-code.md`) — every MCP request now carries `x-dingtalk-dws-agent-code` (which agent host is driving dws — e.g. `claudecode` / `codex` / `qoder` / `cursor` / `hermes` / `openclaw`, falling back to `custom`), `x-dws-agent-instance-id` (a per-machine×channel id, `dwsa_<base62(sha256(machineId|agent_code))>`), the existing machine-level `x-dws-agent-id`, and `X-Cli-Version`. Detection is a confidence ladder, each signature verified on real hosts / official docs (never guessed; anything unrecognized resolves to `custom`): T0 explicit `DINGTALK_DWS_AGENTCODE`, T1 per-agent env signatures, T2 `VSCODE_BRAND` covering the whole VS Code fork family, T3 the macOS `__CFBundleIdentifier` map, T4 `custom`. `identity.json` migrates v1 → v2 transparently and keeps `x-dws-agent-id` machine-level for continuity. **Trust boundary:** `agent_code` and both ids are client self-reported and forgeable — they are for stats / observability only and must not be used for auth, authorization, rate-limiting, billing, or revocation. Server-side gateway work (header passthrough allowlist + logging the fields into the warehouse) is required before the data lands and is tracked separately.

### Fixed

- **`dws sheet export` no longer hangs for the full ~5-minute poll timeout** (#462; `internal/compat/pipeline.go`) — the pipeline poll loop compared the API status against `pollUntilValue` with case-sensitive `==`, but the API returns `"success"` while the pipeline config declares `"SUCCESS"`, so the match never fired and the loop spun until timeout. Switched to `strings.EqualFold`, aligning with the case-insensitive `normalizeAsyncStatus` helper already used for `doc export` / `aitable export`.
- **`dws upgrade --dry-run` now previews instead of performing a real upgrade** (#416, fixes #364; `internal/app/upgrade.go`) — `newUpgradeCommand` registered no `--dry-run` flag and never read the global persistent one, so `--dry-run` fell through and ran a real, irreversible upgrade (download + binary replace), directly contradicting the flag's documented `预览操作内容，不实际执行` contract. It now resolves the target release and platform asset (so "already latest" / "no build for this platform" is still surfaced), prints the 1–5 steps it *would* perform via the side-effect-free `writeDryRunPlan`, and returns before any backup / download / replace. Covered by `TestWriteDryRunPlan_*` and an updated help test.
- **`dws doc create` / `dws doc update` strip server-rejected characters instead of failing** (#465; `internal/helpers/doc.go`, `internal/helpers/doc_jsonml.go`) — the Markdown write path sent raw content straight through, and the dangerous-Unicode strip only ran on the JSONML branch, so content carrying C0 control characters (anything `< 0x20` except `\t` / `\n`), DEL (`0x7F`), or zero-width / line-separator codepoints (`U+200D`, `U+2028`, `U+2029`) — common in LLM-generated or copy-pasted text — was rejected by the server-side `RejectControlChars` validator and the command failed. `stripDocDangerousUnicode` is renamed to `stripDocInputUnsafe`, extended to match the authoritative `apiclient.rejectDangerousChars` set, and applied on both the Markdown and JSONML node write paths. Tab and newline are preserved. Ported from dws-wukong.
- **`dws auth login --no-browser` is now honored** (#365; `internal/app/auth_command.go`, `internal/auth/device_flow.go`, `internal/auth/oauth_provider.go`) — the flag was already defined (and hidden) but never wired to the login providers, so the browser always opened regardless. The value is now passed into `DeviceFlowProvider.NoBrowser` / `OAuthProvider.NoBrowser` and gates the `openBrowser` call; the flag is also unhidden so headless / remote sessions can discover it.

## [1.0.37] - 2026-06-11

This release realigns the npm channel and hardens PAT batch grants. Background on the npm realignment: 1.0.36 was re-cut on GitHub on 2026-06-11 to fold in the canonical-tree poisoned-cache guard (#454), but the npm registry permanently forbids republishing a version number, so the npm package stayed on the original, unguarded cut. 1.0.37 is therefore the first version where **every** distribution channel — GitHub releases, `dws upgrade`, the install scripts, and npm — ships the same guarded build. If you installed 1.0.36 from npm, upgrade to this version.

### Fixed

- **PAT batch grants carry the agent identity and require explicit confirmation** (#455; `internal/pat/chmod.go`, `internal/auth/channel.go`, `internal/app/runner.go`) — an explicit `--agentCode` flag or the `DINGTALK_DWS_AGENTCODE` env var is now carried into PAT batch plan/grant arguments instead of being dropped, and a missing agentCode is forwarded as absent so the PAT core can apply the server-side default rather than failing. Batch grants now refuse to execute without an explicit `--yes` (dry-run and single-scope grants keep their existing behavior), closing the gap where a multi-scope grant could fire without a deliberate confirmation. Only the canonical env name `DINGTALK_DWS_AGENTCODE` is recognized; draft/reversed spellings from earlier iterations are ignored. Verified against prepub: dry-run, single grant, flag-priority grant, and batch grant all resolve the target agentCode, with the granted rows confirmed server-side. Tests: `internal/pat/chmod_test.go`, `internal/pat/browser_policy_test.go`, `test/unit/pat_host_owned_signal_test.go`.

## [1.0.36] - 2026-06-10

This release closes out the poisoned-discovery-cache lock-out for good, with four layers of defense landing together. The lock-out class (seen again on 2026-06-09 as `chat_permission_grant flag redefined: params`): the dynamic command tree is built from cached discovery data **before** Cobra dispatches any command, so a pflag panic fed by a poisoned cache aborted *every* invocation — including `dws cache refresh` and `dws upgrade`, the very commands that could repair it. Now: (1) any panic during the build is recovered instead of crashing (#447), (2) the four known envelope shapes that made pflag panic are skipped at registration so they never fire (#449), (3) when an unknown panic class does fire, the CLI quarantines the poisoned cache and rebuilds itself from a fresh fetch — and `dws upgrade` clears the discovery caches after every binary swap, so simply getting this version onto a machine is enough to escape, no manual cache surgery (#452), and (4) the same guards now also cover the canonical `dws mcp` tree, which is built even earlier and sat outside all three defenses as originally cut (#454 — this release was re-cut on 2026-06-11 to include it; verified against the preserved real poisoned cache from the 2026-05-25 incident). Also in this release: `dws devdoc` gains RAG-backed Open Platform doc search and a new error-diagnosis command (#434), and `dws doc create` stops producing documents with two identical titles (#448).

**Escaping a locked-out older binary**: a binary ≤1.0.35 bricked by a poisoned cache cannot run `dws upgrade`. Either bypass the cache for one invocation with `DWS_CACHE_DIR=$(mktemp -d) dws upgrade`, or delete `~/.dws/cache/<partition>/tools/` by hand, or reinstall via the install script. Once 1.0.36 is on the machine this never needs doing again.

### Added

- **`dws devdoc` — RAG-backed Open Platform doc search and error diagnosis** (#434; `internal/helpers/devdoc.go`, `internal/transport/client.go`) — `dws devdoc article search` now routes to the upstream `search_open_platform_docs_rag` tool, returning structured RAG/reference payloads (the CLI stays a thin invoker; no extra AI analysis layer). New `dws devdoc error diagnose` (alias `troubleshoot`) routes to `search_open_error_code_rag` for diagnosing DingTalk Open Platform API errors, with `--request-id` (hidden `--trace-id` kept for compatibility), `--error-code`, `--error-message`, `--api`, `--context`, `--query`, `--page`, `--size`. Transport-side: query parameters required by DingTalk MCP gateway URLs are preserved on the wire but their values are redacted from debug logs. Default MCP / skill hosts stay on production `https://mcp.dingtalk.com` (prepub remains runtime-configurable). Skill docs (mono + multi `dingtalk-devdoc`) and `docs/command-index.md` updated alongside.

### Fixed

- **CLI no longer bricks when the dynamic command build panics — degrades to built-in commands** (#447; `internal/app/legacy.go`) — `buildEnvelopeCommandsSafe` wraps the envelope-driven build in a local `recover()`. On panic the CLI logs it, prints a stderr hint, and falls back to the hardcoded helper commands, so `auth` / `cache` / `doctor` / `version` / `upgrade` and the helpers stay alive and `dws cache refresh` can rebuild the poisoned cache. Before this, the only recovery from the pre-1.0.32 lock-out class was manually deleting cache files; the duplicate-flag class itself had been fixed at the builder level, but any *future* panic class in the cache-driven build would have bricked the CLI again. Tests: `TestNewLegacyPublicCommandsPanicFallsBackToHelpers`, `TestNewLegacyPublicCommandsNoPanicKeepsDynamicPath`.
- **Envelope-driven flag registration no longer panics on the four known malformed-envelope shapes** (#449; `internal/compat/registry.go`) — while reproducing the lock-out byte-for-byte, four envelope shapes were found still forwarded to pflag calls that panic, each bricking every invocation: a flag named `params` / `json` colliding with the reserved payload flags (the original `flag redefined: params` — earlier dedup fixes covered the alias list and Detail-schema path but not the primary name); two bindings resolving to the same long flag name across bindings; two flags claiming the same shorthand; and a multi-character shorthand. Two small guards applied at every registration site (`ApplyBindings`, `registerPositionalAliasFlags`): `canRegisterFlag` skips duplicate/reserved long names (the value stays reachable via `--params`), and `safeShorthand` drops an invalid or already-taken shorthand while keeping the long flag. The trailing `--json` / `--params` registration is now idempotent. Defense in depth with #447: the escape hatch should never trigger for these known vectors. Test: `TestBuildDynamicCommandsSurvivesMalformedFlagEnvelope` (5 table-driven vectors).
- **Poisoned discovery cache now self-heals: quarantine + rebuild on panic, and `dws upgrade` clears discovery caches** (#452; `internal/app/legacy.go`, `internal/app/upgrade.go`, `internal/cache/store.go`) — #447's recovery is upgraded from "degrade and ask the user to run `dws cache refresh`" to a two-stage self-heal: on the first build panic the partition's discovery cache is moved aside to `<partition>.quarantined` (kept on disk for inspection; a previous quarantine is replaced so nothing accumulates — new `Store.QuarantinePartition`) and the build retried once against a fresh fetch. If the retry succeeds the user gets the full dynamic command tree with zero manual steps; only a second panic (remote envelope itself still poisoned, or offline) degrades to helper commands with the `cache refresh` hint. Additionally `dws upgrade` purges discovery-derived caches (`market` / `tools` / `detail` across all partitions — new `Store.PurgeDiscoveryData`) after a successful binary swap, leaving the co-located `downloads/` cache untouched, so an upgraded binary always rebuilds its command tree from fresh data instead of inheriting snapshots written by the old version. Tests: `internal/cache/store_quarantine_test.go`, rewritten `internal/app/legacy_panic_fallback_test.go` (self-heal success, double-panic degradation, no-cache no-op, happy path).
- **Canonical `dws mcp` tree no longer escapes the poisoned-cache guards** (#454; `internal/cli/canonical.go`, `internal/app/root.go`) — the canonical tree is assembled from cached catalog data *before* the legacy command build, so a pflag panic there — a tool schema property named after the reserved `--params` flag, exactly what the 2026-05-25 incident cache contained — bypassed #447/#449/#452 entirely and still bricked every invocation, including on this release as originally cut. Two layers, mirroring the existing guards: `applyFlagSpecs` skips reserved (`--json`/`--params`), duplicate, and alias-colliding flag names and sanitizes shorthands (`canRegisterToolFlag` / `safeToolShorthand`; a skipped property stays reachable through the reserved JSON payload flags), and `newMCPCommand` wraps the build in the #452 recover → quarantine → retry-once → degrade-to-stub sequence. Verified against the preserved real poisoned cache: the original cut locks out on `--version` / `cache refresh` / `doctor`; this build self-heals on first run and `cache refresh` clears the poison. Tests: `internal/cli/canonical_flag_guard_test.go` (4 cases), `internal/app/canonical_panic_fallback_test.go` (4 cases mirroring the legacy fallback suite).
- **`dws doc create` no longer produces a document with two identical headings** (#448; `internal/helpers/doc.go`) — the platform renders the document name as the page title, and LLM agents habitually repeat `# <title>` as the markdown body's first line despite the skill docs saying not to, so duplicate-heading documents kept appearing. The `doc create` helper (which wins the envelope merge via `preferLegacyLeaf`) now strips a leading ATX H1 whose text exactly equals `--name` (trimmed, case-insensitive) before forwarding to `create_document`, printing a stderr note so agents learn the convention. Deliberately conservative: only an exact match is removed (`# 背景` stays), ATX closing hashes are handled without over-trimming names ending in `#` (e.g. `C#`), H2+/setext headings are never touched, and a body that is nothing but the duplicate H1 omits the `markdown` param instead of sending an empty string. JSONML bodies are out of scope. Tests: `TestStripLeadingDuplicateTitleHeading` (9 cases) plus three end-to-end cobra tests asserting the exact `markdown` param sent.

## [1.0.35] - 2026-06-08

### Fixed

- **`chat message send` @-mentions not rendered in group / direct chat** (#433, `internal/helpers/chat.go`) — when sending a group message or an openDingTalkId direct message (`send_personal_message`) as the current user, the `content` body was packed with `json.Marshal`, whose default HTML escaping turns the `<` `>` in `<@openDingTalkId>` / `<@all>` into `<` `>`. The DingTalk client renders @-mentions by matching the **literal** `<@...>` token, so after escaping the match fails and the mention shows as plain text — while the API still returns `success`, masking the bug. Fix: add `marshalMessageContent`, which serializes `{title,text}` with `json.Encoder` + `SetEscapeHTML(false)`; both the group and openDingTalkId-direct `send_personal_message` paths now use it, preserving the literal `<@...>`. Added regression test `TestChatMessageSendContentNotHTMLEscaped` asserting the content keeps the literal token and is never HTML-escaped. Verified on a real device: `@someone` and `@all` both render as clickable blue mentions.
- **`chat` skill docs & scripts aligned to direct-chat `list-direct`** (#424) — `chat message list` now supports group chats only (`--user` / `--open-dingtalk-id` removed); reading a direct chat moves to the dedicated `list-direct` command, but the skill docs and scripts still taught `chat message list --user`, which now errors with `unknown flag: --user`, also breaking `chat_history_with_user.py` (listed as the "preferred" way to query direct chats). This update: `skills/{mono,multi/dingtalk-chat}/references/products/chat.md` switches `message list` to group-only and documents the new `list-direct` command, syncing the intent routing / key-distinction / context-passing tables / caveats; `skills/mono/references/best_practices/01-messaging.md` changes query-private-chat from `list --user` to `list-direct` (the multi version was already updated); `chat_history_with_user.py` (mono + multi) now calls `list-direct` and fixes response parsing (unwraps `result.messages`, aligns `createTime/content/sender` fields — it previously crashed on `'str' object has no attribute 'get'`). Direct-chat sending still uses `chat message send --user` (since v1.0.34 the direct-send rpc is folded into the `send` command; there is no separate `send-direct`). Docs/scripts only; no change to CLI binary behavior.
- **`pat chmod` batch authorization did not pass through `agentCode`** (#414, `internal/pat/chmod.go`) — the batch plan / grant paths (`buildBatchPlanArgs` / `batchArgs`) previously carried `agentCode` only in the single-grant `toolArgs`; batch calls omitted it, so a batch authorization with an explicit `agentCode` was processed under the default agent. Fix: the batch plan / grant args now also carry `agentCode`, matching the single-grant path.
- **`pat` JSON output escaped the authorization URL into an unreadable form** (#401, `internal/pat`) — the authorization URL attached to PAT error messages, after default HTML escaping, turned `&` into `&`, breaking the link when copied / recognized on mobile. Fix: the PAT error-enrichment JSON output now uses `SetEscapeHTML(false)` (scoped to PAT JSON only), preserving the readable `&` separators.

## [1.0.34] - 2026-06-03

### Changed

- **Service discovery path now carries a version-coded segment** (`internal/market/registry.go`) — the server-list endpoint moves from `/cli/discovery/apis` to `/cli/discovery/apis/bamboo`. The path is now a single `discoveryAPIPath` constant so future version bumps touch one place. Only the path changes; the MCP base host stays on production `https://mcp.dingtalk.com` and the auth / skill / doctor endpoints are untouched. Discovery via the edition `DiscoveryURL` hook (full-URL `FetchServersFromURL`) is unaffected. Server side must serve the new path.

### Removed

- **`dws aiapp` — AI application product taken offline** — removed the `aiapp` product surface (`create` / `query` / `modify`) from the CLI: deleted `internal/helpers/aiapp.go`, dropped it from the generator coverage targets and `knownRegistryProducts`, removed the `aiapp` skill references (mono `references/products/aiapp.md` + `dingtalk-aiapp` multi skill), and unpublished the `aiapp` server from the service-discovery envelope. Product count drops from 19 to 18.

## [1.0.33] - 2026-06-02

This release merges the multi-contributor `pre-mcp-discovery` feature branch into `main` as a single squash (#391), bringing a large batch of new product surface — full DingTalk **docs** (`doc`), **knowledge base** (`wiki`), **AI app** (`aiapp`), AI-table **forms** + **import/export**, and reworked **mail** / **todo** / **report** command trees — while keeping service discovery pinned to production `https://mcp.dingtalk.com` (the branch's `pre-mcp.dingtalk.com` endpoint change was deliberately excluded; the four host constants in `skill_command.go` / `auth/endpoints.go` / `cli/loader.go` / `market/registry.go` stay on prod). It also folds in the portable auth bundle (`dws auth export` / `import`, #357) and PAT batch authorization (#389).

### Added

- **`dws doc` — full DingTalk document command family** (#387, #362, #388, #390; `internal/helpers/doc.go`, `internal/helpers/doc_jsonml.go`, `internal/helpers/docjsonml/`) — search / list / info / read / create / update / upload / download / copy / move / rename, plus `file`, `folder`, `block`-level editing and `comment` (list / create / reply / create-inline). Authoring supports both DocxXML and a JSONML format with a v2 schema validator (`docjsonml/jsonml-schema-v2.json` + `doc_jsonml_validate_v2.go`). Document export and OA alignment land here.
- **`dws wiki` — knowledge base management** (`internal/helpers/wiki.go`, `internal/helpers/wiki_proxy.go`) — knowledge space `create` / `get` / `list` / `search` and member `add` / `list` / `update`, routed through a wiki proxy server.
- **`dws aiapp` — AI application lifecycle** (`internal/helpers/aiapp.go`) — `create` (with prompt / attachments / skills), `query` by task ID, `modify` by thread ID.
- **`dws aitable` forms + import/export** (`internal/helpers/aitable_form.go`, `internal/helpers/aitable_export_import.go`) — datasheet form management and full record import/export, the latter driven through the async-task helper for large datasets.
- **Reworked `chat` / `report` / `todo` / `contact` / `mail` command trees aligned to the Wukong baseline** (#355; `internal/compat/mail_hooks.go`, `internal/compat/todo_hooks.go`, `internal/helpers/report_readable.go`) — mail and todo gain dedicated compat hooks; `report` gains a human-readable rendering path alongside the raw JSON, plus deprecation shims for the old report shape.
- **`dws auth export` / `dws auth import`** (#357) — portable auth bundle for migrating Linux sandbox credentials. Exports the encrypted keychain (`~/.local/share/dws-cli`, including `auth-token.enc` and `dek`) plus required `~/.dws` config so refresh tokens survive import; copying only `app.json` leaves access tokens expiring after ~2 hours. Supports `-o` / `-i` tar.gz paths and `--base64` for copy/paste between sandboxes. `dws auth status` now shows refresh-token validity in table output.
- **Async-task and paging infrastructure** (`pkg/asynctask/`, `pkg/paging/`) — shared helpers underpinning long-running operations (e.g. aitable import/export, doc export) and cursor/page traversal.

### Changed

- **`envelope` now registers `cli.Aliases` as cobra aliases** (#391) — discovery-generated commands expose their declared aliases natively in the command tree, with accompanying command-structure and JSON-parsing cleanups.
- **Breaking: `dws pat chmod` prints a compact authorization summary by default, and gains batch authorization flows** (#389; `internal/pat/chmod.go`) — scripts that parse the raw MCP JSON from stdout must now pass `--format json` or `--verbose` to keep the machine-readable payload; the default summary keeps grant status, agentCode, grantType, scope counts, and a next-action hint. New batch grant/plan flows (`pat.batch_grant` / `pat.batch_plan`) authorize multiple products in one session, fall back to the legacy single-grant path when the server reports `PAT_BATCH_AUTH_UNSUPPORTED`, use the server's default `agentCode` when none is given, and surface per-tool authorization metadata for grant planning.
- **Skill packs synced to the Wukong-aligned content** across attendance / calendar / minutes / oa / sheet and others (#391).

## [1.0.32] - 2026-05-25

Two user-visible regressions resolved plus two AI-agent discoverability fixes. `dws drive upload` was returning `HTTP 403 SignatureDoesNotMatch` for any file whose MIME detects to a non-empty value — basically every real file — because the helper added a client-side `Content-Type` fallback whenever `drive.get_upload_info` returned an empty headers map. DingTalk drive's OSS presigned PUT URLs are signed against an empty `Content-Type` at signing time, so any client-supplied header makes the signature OSS recomputes diverge from the server-signed one, and the PUT is rejected (#347). On Apple Silicon, `dws upgrade` was aborting at the "解压并验证" step with `signal: killed` because GoReleaser cross-compiles `darwin/arm64` binaries on `ubuntu-latest` with no codesign step, and macOS 11+ `amfid` SIGKILLs unsigned arm64 binaries on first exec (#339) — the release pipeline now ad-hoc signs every darwin tarball, and the upgrade client self-heals if it ever encounters an unsigned binary again. On the AI-agent discoverability side, `dws aitable attachment upload-file` (the one-shot prepare + PUT + commit composite) is no longer hidden from `--help` — agents that only browse the command tree were getting stuck at the prepare-only `attachment upload` step, which returns an upload URL + fileToken but doesn't actually upload. And `dws --help` itself now surfaces the missing-command upgrade hint that the custom `renderRootHelp` had been silently dropping from cobra's `root.Long`.

### Added

- **`dws aitable attachment upload-file` is now visible in `dws aitable attachment --help`** (#347, `internal/helpers/aitable.go`) — the hardcoded one-shot composite (prepare + HTTP PUT + commit, returns `fileToken` directly) was previously marked `Hidden:true` and only reachable by agents that read `skills/references/products/aitable.md`. Agents that only discover commands via `--help` were getting stuck at the sibling envelope-generated `attachment upload` (prepare-only): they'd receive `uploadUrl` + `fileToken`, have no idea how to consume the URL, and either write the URL into the attachment field as if it were a token (wrong shape — the field expects `[{"fileToken":"ft_xxx"}]`) or fall back to "please use the UI" messages, which made `dws` look broken even though the capability was fully implemented. Unhiding mirrors the discoverability pattern `lark-cli base +record-upload-attachment` already follows. `Short` is tightened to explicitly mention the 3 steps it bundles; `Long` calls out the prepare-only sibling and recommends `upload-file` as the default for AI agents. The sibling `attachment upload` (prepare-only) keeps its envelope-generated registration but gets a new `Long` that states it is only step 1 of a 3-step flow, lists what an agent must do after (HTTP PUT to `uploadUrl`, then write `[{"fileToken":"ft_xxx"}]` into the attachment field), and points to `upload-file` as the recommended one-shot alternative. `TestAITableUploadFileCommandIsDiscoverable` in `internal/helpers/aitable_upload_file_test.go` guards against re-introducing `Hidden:true`.
- **`dws --help` root output now surfaces the `dws upgrade` hint when no listed command fits** (#347, `internal/app/root.go` + `internal/app/root_help.go`) — `root.Long` is set to `"提示: 如果遇到能力缺失、命令报错、新功能未注册、或无法完成任务, 请先用 'dws upgrade' 升级到最新版本后再试. 钉钉 OpenAPI 和 dws CLI 持续迭代, 新能力和 bugfix 会先在新版本上线."`. The custom `renderRootHelp` (which replaces cobra's default template to render the services / utilities sections) had been silently dropping `root.Long`; restoring it costs one `Fprintln` after the command list, separated by a blank line. The natural failure mode for both agents and users staring at `dws --help` is to give up or hack around when none of the listed commands fit — but in many cases the right action is simply `dws upgrade`, because new capabilities and bugfixes ship continuously and a missing command is usually a stale-binary issue. `TestRenderRootHelpIncludesLong` in `internal/app/visibility_test.go` uses a sentinel `Long` string and asserts the rendered output contains it verbatim, so any future rewrite of the help renderer that drops `Long` fails this test immediately.

### Fixed

- **`dws drive upload` no longer fails with `HTTP 403 SignatureDoesNotMatch` on any non-empty MIME type** (#347, `internal/helpers/drive.go`) — `httpPutDriveFile` was setting `req.Header["Content-Type"] = fallbackMIME` whenever the prepare_upload response returned an empty headers map. DingTalk drive's OSS presigned URLs sign `StringToSign` against an empty `Content-Type` at signing time, so any client-side header makes the signature OSS recomputes at PUT time differ from the server's presignature, and the upload is rejected with `403 SignatureDoesNotMatch`. This broke every `dws drive upload` for any file whose MIME detects to a non-empty value (`image/png`, `application/pdf`, every common binary) — i.e. essentially every real upload. Fix: drop the `hasContentType` / `fallbackMIME` path entirely, trust the server's headers map as authoritative; empty map means "no client-side headers needed", do not infer. `httpPutDriveFile`'s signature loses the `fallbackMIME` parameter. Manual verification: `curl -X PUT -H "Content-Type:" --data-binary @file <same-presigned-url>` returns `HTTP 200`, proving the only difference was the client-side `Content-Type`. `TestHttpPutDriveFile_NoContentTypeWhenServerHeadersEmpty` guards the empty-map path; `TestHttpPutDriveFile_PassthroughServerHeaders` guards that server-provided `Content-Type` / `x-oss-*` headers are forwarded verbatim. Important: `internal/helpers/aitable.go`'s `upload-file` helper deliberately keeps its `Set("Content-Type", mimeType)` call — its OSS endpoint uses a different signing mode (server includes the client-declared MIME in the signature, verified across 12 file types — all succeed). The two helpers must not be unified without re-validating both endpoints.
- **`dws upgrade` no longer dies with `signal: killed` on Apple Silicon after fetching the new binary** (#339) — GoReleaser cross-compiles `darwin/arm64` binaries on `ubuntu-latest` with no codesign step, and macOS 11+ on Apple Silicon requires at least an ad-hoc signature on every arm64 binary; `amfid` SIGKILLs unsigned arm64 binaries on first exec, which the upgrade client surfaces as `signal: killed` and aborts at the "解压并验证" step. Two layers of fix:
  - **Release-side ad-hoc signing** (`scripts/release/post-goreleaser.sh` + `.github/workflows/release.yml`) — after GoReleaser produces the per-platform tarballs, `post-goreleaser.sh` unpacks each `dws-darwin-*.tar.gz`, applies an ad-hoc signature (`codesign --force --sign -` locally, `rcodesign` in CI), deterministically repacks the tarball, and rewrites the matching line in `checksums.txt` so the checksum stays consistent with the resigned tarball. `release.yml` installs `rcodesign 0.27.0` before GoReleaser runs. Every 1.0.32+ tarball ships signed; the install regression is fixed at the source.
  - **Client-side self-heal in `validateNewBinary`** (`internal/app/upgrade.go`) — when running the freshly-extracted binary returns `signal: killed` on darwin, the validator retries once after running `codesign --force --sign -` on the binary and clearing the `com.apple.quarantine` xattr. This keeps `dws upgrade` working even if a future release ever skips the signing step again, and covers users upgrading from older unsigned binaries. `internal/app/upgrade_test.go` (+80 lines) covers the retry path end-to-end: a stripped binary exits 137 on first exec, `validateNewBinary` recovers via ad-hoc sign + xattr clear, the final binary shows `Signature=adhoc` and runs.

## [1.0.31] - 2026-05-21

Closes the last drive-surface gap with the Wukong edition: `dws drive upload` lands as a single-shot composite (`drive.get_upload_info` → HTTP PUT to OSS → `drive.commit_upload`) so a local file reaches DingTalk drive in one CLI invocation, no manual three-step orchestration. Two more drive commands — `dws drive list-spaces` (list visible drive spaces) and `dws drive delete` (delete a drive file, routed via `serverOverride` to the doc MCP server) — ship via the portal envelope; `dws cache refresh` once to pick them up. Companion skill docs teach the agent to recognise dingpan URLs of the form `alidocs.dingtalk.com/document/edit?dentryKey=…` / `…/document/preview?dentryKey=…` and pass the whole URL through to `--node` instead of trying to extract `dentryKey` by hand (the server interprets `dentryKey` and a bare `nodeId` differently — manual extraction was failing).

### Added

- **`dws drive upload --file <path> [--folder <dentryUuid>] [--space-id <id>] [--file-name <name>] [--mime-type <type>]`** (#335, see `internal/helpers/drive.go`) — composite leaf that runs the full three-step upload internally:
  1. `drive.get_upload_info` — fetch the OSS-signed `resourceUrl` + `uploadId` + per-URL headers.
  2. HTTP `PUT` the file binary to OSS (10-minute timeout, attaches every header returned by step 1).
  3. `drive.commit_upload` — register the new file under the target space / folder.

  `--dry-run` prints the three step invocations as a single JSON payload without making any network calls. `--file -` is rejected on purpose: this is a local-path upload, not stdin streaming. `--folder` only accepts a `dentryUuid`; pure-numeric values are rejected up front (`validateDriveParentID`) so callers don't accidentally pass a chat-link `dentryId` (a different ID namespace) where the drive API expects a `dentryUuid`. Response normalisation handles all the wrapper shapes the upstream returns — `content` / `result` envelopes, `resourceUrls[]` arrays, and the flat `resourceUrl` / `uploadUrl` fallbacks — so the composite produces a stable JSON shape regardless of which path the upstream takes. The helper only registers `upload`; the existing six envelope-generated leaves (`list` / `info` / `download` / `mkdir` / `upload-info` / `commit`) keep flowing through dynamic discovery unchanged. `pickCommands.MergeHardcodedLeaves` guarantees dynamic leaves win on collision, so this helper only fills the upload gap.
- **`dws drive list-spaces` and `dws drive delete` (envelope rollout)** (#335, ships via portal envelope) — `list_spaces` registers as a plain `cliName` alias on the existing drive MCP server; `delete_document` registers with `serverOverride: doc` so the call routes to the doc MCP server (which owns the delete API), surfacing under the drive command tree for ergonomics. **Existing users must run `dws cache refresh` once** to pick up these two new leaves; no binary upgrade is required for them, but they pair naturally with the v1.0.31 client that ships `upload`.
- **`skills/references/url-patterns.md`** (#335) — single authority for dispatching `alidocs.dingtalk.com` URLs across doc / sheet / wiki. Five-way split: `/i/p/<token>` short links → expand via `doc info`; `/i/nodes/<id>` node URLs → probe with `doc info` and route by `contentType` / `extension` / `nodeType`; `/spreadsheetv2/...` → `sheet`; `/document/edit|preview?dentryKey=<key>` (dingpan format) → pass the whole URL to `--node`, do not strip `dentryKey` by hand; `/i/share/...` (read-only share) → use the `read_url` fallback. The "URL precheck" Step 0 in `skills/SKILL.md` now redirects every URL-bearing prompt through this dispatcher before the agent picks a product.

### Changed

- **`skills/references/products/doc.md` — `--node` accepts dingpan URLs end-to-end** (#335) — `dws doc info` / `dws doc read` examples gain two extra rows showing `--node "https://alidocs.dingtalk.com/document/edit?dentryKey=<KEY>"` and `…/preview?dentryKey=<KEY>` as first-class `--node` inputs. The "URL recognition & DOC_ID extraction" table adds the `document/edit|preview?dentryKey=<key>` row, and the extraction rules are split into three explicit clauses so the agent stops manually pulling `dentryKey` out of the URL and feeding it as a bare `nodeId` (which the server rejects). The "nodeId dual-format note" upgrades to "nodeId multi-format note" with four equivalent `--node` input shapes side by side.

## [1.0.30] - 2026-05-19

Aligns the open-source CLI with the IM envelope and schema-pipeline plumbing the Wukong edition has been running in pre-prod, plus three user-visible quality-of-life fixes. The most visible one: chat-bot webhook payloads carrying literal Chinese mentions (`@所有人 周报来了` / `@张三 看一下`) no longer fail with `file not found` — `@` is only treated as the `@<filename>` file-injection prefix when followed by an ASCII path-shaped character. The `chat` command tree is refactored to lean on the service-discovery envelope: thin wrappers (`chat search`, `chat group rename`, `chat group members list/add/remove/add-bot`, `chat bot search`) move out of the hardcoded helper and become envelope-generated dynamic commands; the helper keeps only the chat commands with real business logic (intelligent routing, current-user resolution, response normalization, stdin/@file input). A new `dws chat message reply` joins the existing `send` / `send-by-bot` / `recall-by-bot` / `send-by-webhook` family. Underneath: `transform: invert_bool` lets envelopes flip boolean semantics between CLI surface and MCP body (e.g. `--off` ↔ `mute=true`); the pipeline executor fail-fast on upstream `content.errorCode` instead of polling forever; service-discovery dedup keeps two envelope entries that share an MCP endpoint but declare different `cli.id` as separate descriptors (so the `bot-root` / `bot-message` / `bot-group` trio fronting one MCP server stays as three distinct CLI command roots); and `dws chat` no longer nests as `dws chat chat` when two envelope servers both declare the same top-level command name.

### Added

- **`transform: invert_bool` for envelope flag overrides** (#317, see `internal/compat/transform.go`) — flips a boolean at send time. Strings `true`/`1`/`yes`/`on` → `false`; `false`/`0`/`no`/`off`/`""` → `true`. Used when the CLI surface and the MCP body have opposite semantics — e.g. envelope declares `--off` on the CLI but the MCP parameter is `mute=true` for "muted". The framework flips at send time so the envelope keeps the natural CLI verb without forcing every caller to remember the inverted mapping. Coverage in `internal/compat/transform_test.go`.
- **`dws chat message reply`** (#317, see `internal/helpers/chat.go`) — reply to a chat message. Sits alongside `send` / `send-by-bot` / `recall-by-bot` / `send-by-webhook` under `dws chat message`.

### Changed

- **`chat` command tree refactored to lean on the service-discovery envelope** (#317, commit `6be1247`) — `internal/helpers/chat.go` now only carries the chat commands that need real business logic on top of the raw MCP call: `chat message send` (current-user resolution + symmetric direct/group title validation), `chat message send-by-bot` / `recall-by-bot` / `send-by-webhook` (bot routing + stdin/@file input), and `chat group create` (response normalization). The thin wrappers — `chat search`, `chat group rename`, `chat group members list/add/remove/add-bot`, `chat bot search` — are now produced by the envelope as dynamic commands. Net diff in the helper: `+358 / -71` overall (re-aligning to envelope-owned chat structure), and `chat_test.go` drops 71 lines of test-stubs the dynamic path covers natively. Every previously documented chat command keeps the same flag set and the same MCP tool routing — the surface is just sourced differently.
- **Pipeline executor fail-fast on `content.errorCode`** (#317, see `internal/compat/pipeline.go`) — when an upstream tool returns a non-empty `content.errorCode`, `executePipelineCall` raises a validation error immediately with the upstream `errorMessage` instead of proceeding into the poll/download phase. Pre-execution cobra validation (`MarkFlagRequired`) only checks that a flag was set, not that its value was non-empty — so a `--required-flag ""` reaches the upstream tool and the upstream rejects with `errorCode`. Without the short-circuit the pipeline kept polling for a task ID that would never exist, either spinning to `PollTimeout` or burning through retries with no actionable error. Exit code 2 (validation), same as any other CLI-layer pre-flight rejection.
- **Service-discovery dedup keys now include `cli.id`** (#317, see `internal/market/registry.go`) — `NormalizeServers` used to dedup envelope entries by endpoint alone (and by `displayName` in the second pass), which collapsed envelope entries that intentionally split one MCP endpoint into multiple CLI command trees. The `bot-root` / `bot-message` / `bot-group` trio all front the same `.../server/4717...` MCP endpoint and share the displayName `机器人消息`, but each declares a distinct `cli.id` and a distinct CLI command root; the old dedup kept only the last-write and dropped two of them. The dedup key now appends `#<cli.id>` when present, falling back to endpoint / name when absent so historical envelopes without `cli.id` keep their existing behaviour. Coverage in `internal/market/registry_test.go`.

### Fixed

- **`@<text>` injection no longer eats Chinese mentions like `@所有人` / `@张三`** (#317, see `internal/cli/stdin.go`) — `ReadFileArg` and `ResolveInputSource` used to treat *any* value starting with `@` as the `@<filename>` injection syntax. Chat-bot webhook payloads commonly contain literal mentions, so `dws chat message send-by-bot --text "@所有人 周报"` was failing with `file not found: 所有人 周报` before the message reached the API. The new `looksLikeFilePath` heuristic accepts `@` followed by an ASCII path-prefix character (`A-Z` / `a-z` / `0-9` / `.` / `/` / `~` / `_` / `-`), or `@-` for stdin, and passes the value through unchanged otherwise. `@A 但接下来都是中文@测试` *does* still attempt a file lookup because the rune right after `@` is ASCII — this matches the documented `@<path>` prefix shape. The historical "bare `@` is an error" behaviour is preserved. Coverage in `internal/cli/stdin_test.go::TestReadFileArgChineseAtMention`.
- **`dws chat` no longer nests as `dws chat chat` when two envelope servers contribute the same top-level command** (#317, see `internal/compat/dynamic_commands.go`) — `BuildDynamicCommands` used to overwrite `topLevel[name]` on the second contribution and rely on `attachOrMerge` later, which then attached the *whole* incoming command (named `chat`) under the existing root, producing `dws chat chat <leaf>`. The new `mergeSubcommandsInto` moves the second contribution's *children* under the first root and drops the duplicate wrapper, so e.g. `group-chat` + `im` envelopes that both declare `cli.command: chat` produce a single flat `dws chat` subtree.
- **Multi-server tool-name authority correction in the runtime runner** (#317, see `internal/app/runner.go` + `internal/app/direct_runtime.go`) — when two envelope servers share the same `cli.command`, the per-product endpoint map `endpoints[cmd]` in `registerDynamicServer` is second-writer-wins, and `catalog.FindProduct` may return the wrong server's endpoint for a tool whose real owner is the *other* server. `runtimeRunner.Run` now cross-checks the canonical tool→endpoint map exposed by the new `directRuntimeToolEndpoint`: when the per-tool endpoint exists and differs from the per-product endpoint the catalog returned, the tool-owner endpoint wins. Pairs with the registry dedup change above so the routing matches the dedup result.

## [1.0.29] - 2026-05-17

Three discovery-envelope products land on the open-source surface — `aiapp` (AI applications), `live` (DingTalk live streaming), and `aisearch` (enterprise people search) — closing the gap with the Wukong edition's product list. The `aisearch` envelope ships rich model-tolerance affordances (short flags, flag aliases, subcommand aliases) so AI agents that hallucinate keyword synonyms (`--query` / `--name` / `--q` / `--text` / `--find`) or alias subcommands (`search` / `find` / `query` / `user` / `people` / ...) still route to the canonical `person` tool instead of erroring out. To support that final fragment of agent tolerance, `internal/compat/registry.go` relaxes the envelope-generated leaf command's `Args` validator from `cobra.NoArgs` to `cobra.ArbitraryArgs` — restoring cobra's own default (`legacyArgs` returns nil for leaves) so trailing positional words are silently ignored. Plus the previously-shipped credential-isolation fix.

### Added

- **`dws aiapp` / `dws live` / `dws aisearch` — three new products discovered via envelope** (no public issue; pre-Diamond rollout) — open-source `dws` now exposes:
  - **`dws aiapp`** — AI application lifecycle: `create --prompt <p> [--attachments <json>] [--skills <csv>]` / `query --task-id <id>` / `modify --prompt <p> --thread-id <id> [--skills <csv>]`. Backed by upstream `create_ai_app` / `query_ai_app` / `modify_ai_app` MCP tools.
  - **`dws live stream list`** — list my DingTalk live streams. Backed by upstream `get_my_lives`.
  - **`dws aisearch person`** — enterprise people search by keyword + multi-dimension filter. Dimensions: `all` (default) / `name` / `department` / `position` / `duty` / `supervisor` / `subordinate` / `phone` / `jobNumber` — multiple comma-separated (`--dimension name,department`). Backed by upstream `enterprise_person_search`.
  - The `aisearch` envelope additionally registers `-w` / `-d` short flags (keyword / dimension); hidden flag aliases `--query` / `--name` / `--q` / `--text` / `--find` all routing to `keyword`; and cobra subcommand aliases `search` / `find` / `query` / `user` / `people` / `search-person` / `search-user` / `user-search` / `lookup` / `ask` / `contact` all routing to `person`. This closes the F-class model-tolerance regression cases in `dws-wukong/auto-test/cli_to_mcp/testcases/aisearch/test_90_aisearch_param_regression.py` (50/50 pass for aiapp + live + aisearch on the pre-mcp build).
  - **Users must run `dws cache refresh` once** to pick up the new envelopes; no binary upgrade is required, but pairs naturally with the v1.0.29 client (see Fixed below for the envelope-leaf-Args change).

### Fixed

- **Envelope-generated leaf commands now tolerate trailing positional args** (#306, no public issue) — `NewDirectCommand` in `internal/compat/registry.go` was hard-coding `cobra.NoArgs` for leaves without positional bindings (`totalMax == 0`). This is stricter than cobra's own `legacyArgs` (cobra `args.go:30-32` returns `nil` for any command without subcommands), and surfaced as `unknown command "<word>" for "<leaf>"` whenever an AI agent passed trailing positional words after a leaf — e.g. `dws aisearch person search --keyword "张"` or `dws aisearch person user search --keyword "张"`. Switching the `totalMax == 0` branch (and the initial value) from `cobra.NoArgs` to `cobra.ArbitraryArgs` restores cobra's natural leaf behavior: trailing positional args are silently ignored. Existing positional-binding paths (`MinimumNArgs` / `RangeArgs` / `MaximumNArgs`) are unchanged. Verified against `dws-wukong/auto-test/cli_to_mcp/testcases` — aiapp (9/9) + live (3/3) + aisearch (38/38) = **50/50** pass, vs 48/50 before this patch.

### Security

- **App credential files are partitioned by edition to prevent cross-edition credential leakage** (#300, no public issue; found during internal review) — different `dws` editions sharing the same config directory previously read and wrote the same `app.json`. A sibling edition that pinned its OAuth client ID could persist that ID through the shared post-login path, and the open-source build could later adopt it from the same file. Open-source/empty edition keeps the legacy `app.json` path for compatibility; sibling editions now use `app-<edition>.json`, matching the existing cache partitioning strategy. This prevents new cross-edition app credential writes and reads from colliding. After a sibling edition saves its new partitioned file, it also best-effort removes a legacy `~/.dws/app.json` only when that file's `clientId` matches the sibling edition being saved; a different, unparsable, or otherwise unowned `app.json` is left untouched to avoid deleting open-source credentials. If you previously ran multiple editions in one shared `~/.dws`, remove any confirmed-stale orphan manually with `rm ~/.dws/app.json` after verifying it is not the open-source credential file you still need.

## [1.0.28] - 2026-05-14

A single symmetric follow-up to 1.0.26's #250: `dws chat message send --group <cid>` now refuses an empty `--title` at the CLI layer instead of letting the call fall through to the API and surface a misleading `发群服务窗会话消息失败` error. No other behaviour changes.

### Fixed

- **`dws chat message send` rejects missing `--title` on group messages** (#294, completes #250) — `send_message_as_user`'s schema marks `title` as required (just like `send_direct_message_as_user`), but `buildChatMessageSendInvocation` only had the pre-validation on the direct-message branches. Group sends without a title were falling through to the API and returning the same misleading `发群服务窗会话消息失败` that #250 already fixed for direct messages. The check now covers both branches: missing `--title` on `--group` returns `--title is required for group messages (--group)` with exit code 2; missing on `--user` / `--open-dingtalk-id` keeps the original `--title is required for direct messages (--user / --open-dingtalk-id)`. The `Long` help, `--title` flag description, the first `Example`, and `skills/references/products/chat.md` (including the drive→chat workflow example) are realigned to "title is required for both direct and group messages" — the docs previously contradicted themselves (the prose said 群聊可选 while the flag listing said 必填). `internal/helpers/chat_test.go` adds a `group-without-title` rejection case; the existing `group` / `positional-text` success cases now pass `--title` to stay aligned with the new validation. No API request shape change — the server has always required `title`; the CLI now matches.

## [1.0.27] - 2026-05-14

Two user-visible fixes plus the schema primitive they're built on. `dws doc update` now reads Markdown from a file or stdin, so long / multi-line / table-heavy content no longer gets mangled by shell escaping; `dws sheet find --query` stops returning `unknown flag` on the open-source build, restoring copy-paste from internal wukong docs. Underneath, schema/discovery envelopes get a generic `file_read` transform and a `CLIFlagOverride.MapsTo` field that lets two sibling CLI flags route into the same MCP parameter slot. Also suppresses a noisy WARN on normal stdio-plugin shutdown.

### Added

- **`file_read` transform + `CLIFlagOverride.MapsTo` field** (#291, closes #277 #278 #282 #288) — discovery envelopes can now declare a path-typed CLI flag that performs the "file path → file contents string" conversion client-side before the value reaches the upstream MCP parameter.
  - `transform: "file_read"` (`internal/compat/transform.go`) — reads the file at the flag's value with UTF-8 validation; `-` means stdin. Any IO / encoding failure is surfaced as a validation error (exit 2), distinct from the generic transient-failure path (exit 1).
  - `CLIFlagOverride.MapsTo` (`internal/market/registry.go`) — redirects the flag's final value (post-transform or literal) into a named MCP parameter slot instead of the default `params[propertyName]`. This lets a single MCP parameter (e.g. `markdown`) be fed by two sibling CLI flags — a literal `--content` and a file-reading `--content-file` — paired with the existing tool-level `MutuallyExclusive` / `RequireOneOf` to express "exclusive, at least one".
  - Wired into the `internal/compat/dynamic_commands.go` normalizer via a separate `mapsToRoutes` collection + routing pass; empty `MapsTo` preserves the legacy `params[propertyName] = value` semantics, so every pre-existing dynamic_commands test passes unchanged. Pre-prod end-to-end verified across 6 cases (see PR #291's Validation table).
- **`dws doc update --content-file <path>` (envelope rollout)** — fixes "long Markdown can't reach the doc". The old command only accepted `--content "..."`, so long / multi-line / table-heavy Markdown got mangled by shell escaping and AI agents writing >2KB of content were stuck. The envelope now maps both `--content` (literal) and `--content-file` (`file_read` transform) to the `markdown` parameter, makes them mutually exclusive via cobra's `MarkFlagsMutuallyExclusive`, and requires at least one via `RequireOneOf`. `--content-file -` reads from stdin, so `cat long.md | dws doc update --content-file -` works directly. **Existing users must run `dws cache refresh` once** to pick up the new envelope.
- **`dws sheet find --query` hidden alias (envelope rollout)** — fixes "unknown flag when copy-pasting commands across editions". Users copying `dws sheet find --query "..."` from internal wukong docs onto open-source `dws` got `unknown flag: --query`, because the open-source primary flag is named `--find`. The envelope now registers `--query` as a hidden alias of `--find` via `CLIFlagOverride.Aliases` (the field shipped in 1.0.26) — it doesn't show up in `--help`, but accepts values and writes to the same MCP parameter. `--find` behaviour is unchanged. Also requires `dws cache refresh` once.

### Fixed

- **Noisy `failed to stop stdio client: exit status 1` WARN on normal stdio-plugin shutdown** (#285) — when `Stop()` explicitly `Kill`s the subprocess, the non-zero exit code returned by `cmd.Wait()` is expected behaviour, but it was being propagated as an error and logged to stderr on every CLI exit, polluting agent log parsing. `Stop()` now returns `nil` after Kill + Wait; the error path is reserved for "process exited on its own with non-zero" (e.g. stdin close without an explicit Kill). `internal/transport/stdio.go` + `stdio_integration_test.go` assert "Stop() returns nil after kill".

## [1.0.26] - 2026-05-12

Platform-stability round: Windows PAT-auth browser opener no longer truncates URLs at `&userCode=`, macOS sandbox hosts get an opt-in keychain fallback, and `dws doc download` rejects `axls` nodes before requesting `drive:download` consent. Two new global output formats `-f ndjson` and `-f csv` (matching `larksuite/cli`) land as first-class citizens with real-traffic-verified list detection. The `dws doc comment *` regression tracked in #240 is also resolved — fix is in the market metadata, users just need `dws cache refresh` once.

### Added

- **`-f ndjson` and `-f csv` global output formats** (#259, closes #252) — `ndjson` emits one compact JSON record per line (works straight with `jq -c` / `while read` / log pipelines); `csv` goes through `encoding/csv` (RFC-4180 — quoting, embedded newlines, CJK all handled by stdlib) and reuses the existing `-f table` column resolver (`normalizePayload` / `unwrapPrimaryObject` / `extractRowsFromMap` / `rowsFromSlice` / `formatValue`) so table and csv stay visually aligned. After a 7-product real-traffic sweep (contact / chat / doc / mail / todo / minutes / schema), the `preferredListKeys` whitelist was extended to cover the actual DingTalk envelope shapes — `contact user search` (`result`), `chat search` (`result.value`), `doc search` (`documents`), `mail mailbox list` (`emailAccounts`), `todo task list` (`result.todoCards`) — so these commands now degrade into a proper row stream instead of collapsing to a single-line `key,value` blob. Lives in `internal/output/ndjson.go` + `internal/output/csv.go`; `--format` help in `internal/app/flags.go` now lists `ndjson|csv` alongside `json|table|raw|pretty`.

### Changed

- **Sticky flag splitting is now schema-aware** (#272) — PreParse `StickyHandler` 此前会把任何前缀命中已知 flag 的 `--flagsuffix` 一律切成 `--flag suffix`，于是 `--starttime20260507` 这类拼错被静默改写成 `--start time20260507`，把假值传到下游。新行为按 flag 的 pflag 类型 / JSON Schema `format` / `enum` 校验 suffix 是否像合法 value（共享逻辑见 `pkg/cmdutil/sticky_suffix.go`），不像就保留原 token 让 cobra 报 `unknown flag`。slice/array/object 类型的 flag 永不切分。首 rune 读取使用 `utf8.DecodeRuneInString`，对中文等多字节 value 安全。

### Added

- **`available_flags` field on unknown-flag errors** (#272) — `dws -f json` 的 unknown-flag 错误体里新增 `available_flags`（已排序、过滤掉 hidden 与内部 `json` / `params`），方便 agent 不解析 `--help` 就能恢复。Human-readable 输出会附 `Flags: ...` 行，截断在 200 字节内。

### Fixed

- **`dws chat message send` 单聊缺 `--title` 时前置校验** (#250) — 单聊（`--user` / `--open-dingtalk-id`）的底层工具 `send_direct_message_as_user` 在 API 层强制要求 title，缺失时返回误导性的 `发群服务窗会话消息失败`。CLI 现在在 `buildChatMessageSendInvocation` 里前置校验，直接返回 `--title is required for direct messages (--user / --open-dingtalk-id)`；同时把 `Long` help、`--title` flag 描述、Example 和 `skills/references/products/chat.md` 全部对齐为「单聊必填，群聊可选」。群聊行为不变。
- **PAT auth URLs were truncated on Windows browser open** (#242, fixes #230) — `cmd /c start <url>` on Windows interprets `&` as a command separator, so PAT URLs containing `&userCode=...` were silently chopped before the userCode segment, and the browser landed on a 0-permission DingTalk page. The retry opener now uses `rundll32 url.dll,FileProtocolHandler`, which passes the URL through verbatim. The PAT response also exposes a copy-safe `data.authorizationUrl` (in addition to the service-provided `data.uri`, which is preserved as-is), and human-readable PAT output prints `PAT_AUTHORIZATION_URL=<full-url>` on its own line so OpenClaw-style host wrappers that swallow or reformat stderr can still capture the full link. Legacy DingTalk hash-route shapes (`https://open-dev.dingtalk.com/fe/old#%2FpersonalAuthorization%3FflowId=...%26userCode=...`) are normalised back into the working `/fe/old?hash=...#/personalAuthorization?...&userCode=...` form. Regression tests cover the issue-shaped URLs (encoded hash, fragment, `&userCode`) plus the OpenClaw malformed-hash variant.
- **`dws doc download` triggered `drive:download` PAT consent for unsupported axls nodes** (#268, fixes #190) — added a `get_document_info` preflight before `download_file`, so online-sheet (`axls`) nodes are rejected locally with guidance to use sheet range tools instead. The preflight reads `extension` from deterministic response paths (no recursive payload scan) and routes its own PAT errors back through `handlePatAuthCheck`, preserving device-flow / host-owned PAT behaviour. Costs one extra MCP roundtrip per `doc download` — deliberate, so the unsupported path fails before consent. Lives in `internal/app/doc_download_preflight.go`; coverage in `internal/app/runner_test.go`.
- **macOS sandbox hosts (Codex App etc.) couldn't read/write tokens via Keychain** (#267, fixes #214) — sandboxed macOS environments intercept `security` / Keychain APIs, so every token operation failed. New opt-in `DWS_DISABLE_KEYCHAIN=1` switches macOS to the same file-DEK path Linux uses (DEK at `~/Library/Application Support/dws-cli/dek`, mode `0600`), bypassing the system Keychain. Default behaviour is unchanged — fallback is strictly opt-in because file-DEK is a weaker trust model than Keychain-managed storage (DEK file sits next to ciphertext in the same directory). The Darwin / Linux file-DEK implementation is now shared in `internal/keychain/file_dek.go` (Linux path deduplicated by ~40 lines). Documented in `docs/reference.md` (中英) with the security tradeoff spelt out so users make the choice explicitly.
- **`dws doc comment {list,create,create-inline,reply}` returned `PARAM_ERROR - 未找到指定工具`** (fixes #240, also #234) — the four comment tools used to live on an independent `doc-comment` MCP server. After the Portal merged comment functionality into the `doc` server descriptor, the runtime `tools/list` on the merged `doc` server didn't include them, so every `dws doc comment *` call returned the "tool not found" PARAM_ERROR. The market metadata for the `doc` server now declares `serverOverride: "doc-comment"` on all four comment `toolOverrides`, so the existing CLI routing path sends `dws doc comment *` to the still-running `doc-comment` MCP server (which has the tools). No CLI code change was required, but **existing users must run `dws cache refresh` once** to pick up the updated descriptor — without that, the stale local market cache keeps pointing the call at the merged `doc` server and the error persists. Verified post-refresh: dry-run resolves to `https://mcp-gw.dingtalk.com/server/doc-comment` with tool `list_comments`, real calls return normal business responses (e.g. legitimate cross-org authz errors) instead of `未找到指定工具`.

## [1.0.25] - 2026-05-11

Two generic envelope-schema enhancements that close gaps the `cli_to_mcp` test suite kept surfacing — both product-agnostic, no hardcoded helper commands. Plus missing skill references for the already-registered `sheet` and `wiki` products are now shipped.

### Added

- **`sheet` (在线电子表格) skill reference + product-overview entry** — the `sheet` product registers **34 envelope tools** covering worksheet CRUD (`create` / `new` / `list` / `info` / `copy_sheet` / `update_sheet`), range read/write (`range read` / `range update` / `append`), dimension ops (`add-dimension` / `insert-dimension` / `delete-dimension` / `move-dimension` / `update-dimension`), merge (`merge-cells` / `unmerge-cells`), find/replace (`find` / `replace`), filter views (`filter-view {create, list, update, delete, update-criteria, delete-criteria}`), sheet-level filters (`create_filter` / `get_filter` / `update_filter` / `delete_filter` / `set_filter_criteria` / `clear_filter_criteria` / `sort_filter`), image write (`write-image`), and async export (`submit_export_job` + `query_export_job`). These were live in the envelope but `skills/references/products/sheet.md` had not shipped and `skills/SKILL.md` 产品总览 didn't list `sheet`, so agents had no reference to consult and were skipping it during intent routing. This release adds the doc, registers `sheet` in 产品总览 + 意图判断决策树, extends `description` to include 在线电子表格, adds a Sheet row to `README.md` / `README_zh.md` "Key Services", and notes the v1.0.25 reality on naming (about a third of `sheet` tools still expose snake_case cli_names pending `CLIAliases` (#246) rollout) and on export (no consolidated `dws sheet export` exists in v1.0.25 — `submit_export_job` + `query_export_job` are the atomic primitives; Pipeline (#247) provides the future plumbing).
- **`wiki` (知识库) skill reference + product-overview entry** — the wiki product's 7 envelope tools (`wiki.create_wikiSpace`, `wiki.get_wikiSpace`, `wiki.list_wikiSpaces`, `wiki.search_wikiSpaces`, `wiki.add_member`, `wiki.list_member`, `wiki.update_member`, surfaced as `dws wiki space create / get / list / search` and `dws wiki member add / list / update`) have been registered for a while, but no `skills/references/products/wiki.md` shipped with them, so agents had no per-command reference to consult. This release adds the reference doc, registers `wiki` in `skills/SKILL.md`'s 产品总览 table and 意图判断决策树, mentions 知识库 in the skill `description` frontmatter, adds a Wiki row to `README.md` / `README_zh.md` "Key Services", and removes `wiki` from the "Coming soon" callout (which was now stale).
- **`CLIToolOverride.CLIAliases` envelope field** (#246) — lets a single MCP tool register additional cobra command aliases via envelope JSON (e.g. `range read` also accepts `range get`, `member list` accepts `member ls`). Plumbed through the existing `Route.Aliases → cobra.Command.Aliases` path; sibling conflicts are silently dropped by cobra. Lives in `internal/market/registry.go` + `internal/compat/dynamic_commands.go`.
- **`json_parse_strict` transform** (#246) — strict-JSON variant of `json_parse` that does **not** fall back to YAML. Use when the upstream tool requires a structured array/object and silently coercing a malformed input to a scalar string would mask a real user error (observed: `filter-view --criteria 'NOT_VALID_JSON'` was being accepted and quietly creating an empty-criteria view). In `internal/compat/transform.go`.
- **`CLIToolOverride.Pipeline` + pipeline executor** (#247) — a single CLI command can now orchestrate an ordered sequence of MCP tool calls plus optional HTTP-download sinks, declared entirely in envelope JSON. Motivating use case: the "submit-job → poll-status → download-result" pattern (e.g. sheet export) that previously required per-product hardcoded helpers.
  - `PipelineStep` supports `type:"call"` (with optional `PollUntilField` / `PollUntilValue` / `PollIntervalSec` / `PollTimeoutSec` for polling loops) and `type:"download"` (resolves `DownloadURLField`, HTTP GETs the body, writes to the path from `OutputFlag`, infers filename for directory paths).
  - Template language: `$flag.<name>` resolves a user CLI flag by alias; `$step.<idx>.<dotPath>` walks a prior step's response (works through wrapped MCP envelopes); literals pass through.
  - `CLIFlagOverride.PipelineLocal` marks a flag as CLI-side only so `CollectBindings` skips it (value never reaches MCP params); the pipeline executor still reads it via `extractFlagValuesByAlias`.
  - Download step emits machine-parseable plain-text lines (`jobId: <id>\n`, `downloadUrl: <url>\n`) alongside the standard JSON envelope, so shell pipelines and regex-based tests can extract key values without JSON parsing.

## [1.0.24] - 2026-05-09

Three small but user-visible safety/usability changes: the embedded distribution now refuses to self-upgrade, the `dws auth login` help text finally matches the actual default flow (loopback, not device), and the release workflow gains a manual fallback trigger.

### Changed

- **`dws upgrade` is blocked in embedded distributions** (#248) — when the CLI is shipped as an embedded asset (e.g. inside another product), `dws upgrade` would happily overwrite the host-managed binary. The upgrade entry point now detects the embedded build flag and exits early with a clear message; covered by `internal/app/upgrade_embedded_guard_test.go`.

### Docs

- **`dws auth login` help text reflects the real default** (#238, fixes #226) — the long help previously claimed "OAuth 设备流 (默认)", but the actual default starts a 127.0.0.1 loopback listener and only switches to device flow when `--device` is passed. SSH-into-headless-Linux users following the old text hit a dead end (remote-side 127.0.0.1 is unreachable from the local browser). Help and two `flagErrorWithSuggestions` messages in `root.go` are realigned: each method is named after its real flag (`OAuth Loopback 流 (默认)` / `OAuth 设备流 (--device)` / `直接提供 Token (--token)`), with an explicit `--device` example for SSH/headless. No behaviour change.

### CI

- **`workflow_dispatch` trigger added to release workflow as a fallback** (#261) — GitHub occasionally drops tag-push events; the release job can now be re-run manually against any tag ref without having to delete and re-push the tag.

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
