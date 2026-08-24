# DWS Dev CLI 参数幻觉分析

## 1. 结论摘要

本报告以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，只使用该提交的真实 Cobra 命令树与 Help、运行时组装 Schema、Dev Skill、`schema_command_exclusions.go`、命令实现和正式 `internal/cli/param_concepts.json`。没有使用历史 badcase、`dws-eval`、`merged_scan.json`、历史工作簿、固定 Catalog、用户 Shortcut 或插件。

Dev 的主要风险不是单个 flag 拼写，而是同一产品内存在多层、近似但不可互换的标识符和配置角色。应用既可由 `unifiedAppId` 标识，也可能出现 `appKey`；机器人连接使用独立的 `robotClientId/secret`；版本同时有 `version-id` 与创建时的 `version`；机器人提交还有异步 `task-id`。与此同时，应用名、机器人名、Agent 名和删除确认名都带 `name`，普通文本、国际化 JSON、图标媒体、预览媒体及六类 URL/安全端点又大量共享相似词根。模型若统一写成 `--id`、`--client-id`、`--name`、`--description`、`--media-id` 或 `--url`，无法仅凭名称安全决定目标。

量化结果如下：

- 官方 Cobra 树共有 49 个 runnable 路径，全部可见；其中 38 个路径带业务参数，合计出现 140 次 canonical 业务参数，使用 77 个不同 canonical flag 名。
- 运行时 Schema 发布 34 个 Dev 工具；另有 12 个可执行父命令，以及 3 个被 `schema_command_exclusions.go` 精确排除的公开 runnable leaf：`dev app version check-approval`、`dev connect list`、`dev connect restart`。
- 34 个 Schema leaf 合计发布 108 次参数、52 个不同 flag 名；逐 leaf 对账真实 Help，公开业务参数差异为 0。
- 官方树还有 6 次原生 hidden flag，涉及 4 个名称：`keyword`、`member-user-ids`、`daemon-supervise`、`daemon-worker`。
- Dev Skill 的机械扫描报告 9 条“未知命令”，逐项复核后均来自可执行 `dev connect` 父命令或上述 exclusion 命令，真实 Skill 参数漂移为 0。
- 聚合得到 7 类参数问题、81 条“问题—命令”明细，覆盖全部 38 个参数化官方路径。
- 正式基线只为 Dev 生成 1 个命令条目、2 个 alias、2 个 block、0 个 ambiguous；候选草稿扩展到 36 个受 reducer 管理的参数化 leaf、246 个 alias、253 个 block、26 个 ambiguous。
- 候选相对正式文件新增 31 个 Dev concept，扩展 `app_id`、`page_cursor`、`page_number`、`pagination_size`、`search_query`、`user_ids` 6 个既有 concept 的 Dev scope，并新增 20 个 Dev command override；既有 override、morphological rules 和 253 条 fixture 均未变化。
- 候选已通过 22 组 alias/canonical 最终 payload 或本地结果等价、13 组 block/ambiguous、2 组原生隐藏兼容、4 组非 Dev 回归，以及 PreParse、三包测试、generated drift 和 Schema Catalog policy。

第一轮可以安全落地“同实体、同角色、同 cardinality、值可原样传递”的参数名归一，并对 `id/client-id/name/media-id/url` 等无唯一答案的输入进行 block 或 ambiguous。不能通过别名表解决的内容包括：标识符查询与互换、名称查 ID、单值/列表转换、i18n JSON/URL 列表构造、`dev connect` 可执行父命令的 reducer 边界、Schema exclusion 以及 `dev doc search` 后端网关不可用。

## 2. 七类参数问题

### 2.1 应用、凭证、机器人、版本和任务标识符容易被统一写成 `id/client-id`

Dev 至少存在以下相邻标识符：

- `--unified-app-id`：应用统一 ID，出现在 29 个 Schema leaf，并被 local/excluded leaf 继续使用；
- `--app-key`：应用 key，仅用于应用查询和列表过滤；
- `--robot-client-id` / `--robot-client-secret`：机器人连接凭证；
- `--version-id`：已创建版本的 ID；
- `--version`：创建版本时的版本号或版本标签；
- `--task-id`：机器人提交后的异步任务 ID；
- `--app-group-id`：应用列表过滤维度。

正式表的 `app_id` 只覆盖 `dev app get`，因此大部分 `--app-id/--application-id` 仍无法在其他 Dev leaf 中兜底。候选将 `app_id` 扩展到所有受治理的 `--unified-app-id` leaf，但明确排除 `dev connect` 可执行父命令；另外分别建立 appKey、robot client、version ID、version label 与 robot task ID concept。

`--client-id` 不能被无条件改成 `--robot-client-id`：它已经是 root persistent 真实 flag，语义属于 DWS 全局客户端配置。中央规则不会覆盖真实 flag，候选只允许 `bot-client-id/robot-app-client-id` 等角色明确的名称。多 ID leaf 中的 `--id` 返回 ambiguous，不做值域猜测。

### 2.2 应用名、机器人名、Agent 名与删除确认名共享 `name` 词根

应用 create/update/list 中的 `--name` 表示应用名称；robot config 中的 `--name` 表示机器人展示名称；robot submit 同时有 `--name` 与 `--robot-name`；`dev connect` 还有 Agent 运行配置中的命名语义。它们不能只因为真实 flag 叫 `name` 就归为同一个全局 concept。

最敏感的是 `dev app delete --confirm-name`。这个参数是删除确认值，不是普通应用名入口。候选仅接受 `--confirm-app-name/--confirmation-name` 等明确确认型名称，并 block `--name/--app-name`，防止普通文本被自动提升为删除授权。

### 2.3 成员、审批人、权限范围和事件订阅混合单值/列表及不同角色

代表性参数包括：

- 成员列表：`--user-ids`，原生隐藏兼容为 `--member-user-ids`；
- 成员类型：`--member-type`；
- 版本审批人单值：`--approver-user-id`；
- 权限写入列表：`--scope-values`；
- 权限查询过滤：`--scope-value` 与 `--scope-type`；
- 事件订阅列表：`--event-codes`；
- connect 运行范围：`--allowed-users/--allowed-groups/--owner-user-id/--notify-staff-id`。

这些值可能都表现为 ID 或 CSV，但业务角色和 cardinality 不同。候选允许同角色、同单复数的名称变体，禁止把单个 `user-id` 自动扩展成 `user-ids`，也不允许把权限查询过滤 `scope-value` 改成权限写入列表 `scope-values`。

### 2.4 搜索、过滤、分页、排序、状态和本地输出协议名称不统一

应用、事件、权限、版本列表使用 `cursor + page-size`，而 `dev doc search` 使用 `page + size + query`。应用列表又同时出现 `name/robot-name/creator/develop-type/filter-cool-app/sort-type/sort-order`；权限列表同时出现 `scope-type/scope-value/api-status/auth-status/keyword`。

候选在同一协议内复用 `page_cursor`、`page_number`、`pagination_size` 和 `search_query`，并在相反协议上保护。它不会计算 page 与 cursor，也不会翻译 sort/status/type 的枚举值。

`dev connect list/status --json` 是本地 bool 输出开关，不等同于全局 `--format json` 字符串协议。二者不能由单纯 alias 安全互换，保留为当前边界。

### 2.5 描述、简介、国际化 JSON、媒体 ID 和 Agent 配置文本容易被当作通用 `text/payload`

应用与机器人命令同时使用：

- 普通文本：`--desc`、`--brief`；
- 国际化 JSON：`--i18n-name`、`--i18n-brief`、`--i18n-description`；
- 媒体资源：`--icon-media-id`、`--preview-media-id`；
- 能力列表：`--skills`；
- connect 配置：model、memory、workdir、knowledge、card template、audit sheet 等多个独立角色。

候选只对 `description/app-description/robot-description`、`icon-id/preview-id` 等角色明确且值可原样传递的名称进行改写。在 robot submit 同时存在图标和预览媒体时，泛化 `--media-id` 返回 ambiguous；`data/payload/text` 不会被猜成 i18n JSON 或普通描述。

### 2.6 回调、出口、Web 入口、重定向、SSO 和 IP 白名单属于不同安全端点

三组命令暴露了不同端点角色：

- robot config：`--event-callback-url`、`--outgoing-url`；
- webapp config：`--homepage-url`、`--omp-url`、`--pc-homepage-url`；
- security config：`--redirect-urls`、`--sso-urls`、`--ip-whitelist`。

这些参数不能统一成 `url/urls`。候选为每个角色建立独立 concept，并把泛化 `url/urls/callback-url` 设为 ambiguous 或 block。别名层只改参数名，不拆分逗号列表、不做 URL/IP 格式校验，也不推断安全策略。

### 2.7 可执行父命令、Schema exclusion、隐藏兼容 flag 与运行时可用性形成边界

同一提交内存在四个不同层次：

- 49 个官方 runnable Cobra 路径；
- 34 个 Agent Schema 工具；
- 36 个候选中央 reducer 命令条目；
- 最终接口或本地运行是否可用。

`dev connect` 本身是可执行父命令，并直接承载 27 个公开业务 flag，但当前中央 reducer 不治理这个父命令。`dev connect list/restart` 与 `dev app version check-approval` 可执行，却被 Schema 精确排除。`dev doc search` 的命令、Schema 和参数可以全部正确，但 Skill 明确记录当前运行时网关不可用，应使用 `devdoc article search` 等替代路径。

候选没有通过 JSON 改写这些独立边界：只治理 36 个已经进入 reducer 的参数化 leaf；`dev connect` 父命令和 `connect list` 保持 canonical 行为；Schema exclusion 与后端故障分别列为当前无法解决。

## 3. 当前别名表可以直接实施的方案

候选完整文件位于同目录 `param_concepts.json`，主要改动如下：

1. 新增 31 个 Dev 专用 concept，覆盖 appKey、名称、机器人凭证、版本/任务、人员角色、权限范围、事件、文本/i18n、媒体、URL/安全端点等。
2. 扩展 `app_id`、`page_cursor`、`page_number`、`pagination_size`、`search_query`、`user_ids` 6 个既有 concept 的 Dev 精确命令范围。
3. 新增 20 个 command override，用于绑定真实 `name/desc/version` 等宽泛 canonical、限定 scoped alias，并配置 `scope_strict`、block 和 ambiguous。
4. 候选生成 36 个 Dev 命令条目；没有把 `dev connect` 父命令或 `dev connect list` 伪装成已治理对象。

生成影响面由正式基线的 1/2/2/0 扩展为 36/246/253/26（命令/alias/block/ambiguous）。独立审核确认：

- 36 个生成路径全部存在于冻结提交的官方树；
- 246 个 alias 的目标全部是对应 leaf 的真实 canonical flag；
- alias 来源均未与该 leaf 的真实 flag 冲突；
- 253 个 block 和 26 个 ambiguous 均未拦截真实 canonical flag；
- `dev connect` 可执行父命令没有进入生成结果；
- 既有非 Dev override、morphological rules 和 253 条 fixture 完全不变。

规则数量较大主要来自 `unified-app-id` 覆盖面、同一 leaf 内多角色 ID/名称/URL 的保护性展开，以及列表协议的正反向 block。数量本身不是通过依据，完整 generated audit 与最终 payload 等价才是准入依据。

## 4. 当前能力支持不了或不应该自动处理的事项

以下事项不能通过 `param_concepts.json` 解决：

- unifiedAppId、appKey、robot clientId、versionId、taskId 之间的查询或转换；
- 把 root `--client-id` 自动解释成 `--robot-client-id`；
- 应用名、机器人名或 Agent 名到稳定 ID 的解析；
- 单个 userId 与 user-ids/allowed-users 之间的自动扩展或收缩；
- `scope-value` 查询过滤与 `scope-values` 写入列表之间的转换；
- 把普通文本构造成 i18n JSON，或自动拆分 URL/IP 列表；
- 把本地 bool `--json` 与全局字符串 `--format json` 自动互换；
- 让 `dev connect` 可执行父命令的 27 个业务 flag 自动进入中央 reducer；
- 让 3 个 Schema exclusion 命令通过别名表进入 Agent Schema；
- 修复 `dev doc search` 后端网关不可用。

这些边界不阻塞第一轮安全的参数名治理，但必须在 Help/Skill 中持续明确，不能把“PreParse 能改名”写成“最终业务能力已可用”。

## 5. 候选草稿验证结果

候选仅在冻结快照 `/private/tmp/dws-main-param-analysis.HcTfUP` 中临时替换正式输入并重新生成、构建和测试；当前工作区正式 `internal/cli/param_concepts.json` 与 `internal/cli/param_aliases_generated.go` 的 SHA-256 分别保持 `1ba7dc90…6fed` 和 `4e4bbc41…4f36`，均未修改。

验证结果：

- JSON 结构、候选 scope 和 generated 全量规则审核通过；
- `go generate ./internal/cli` 结果稳定，生成 316 个全局命令条目；
- 22 组 alias/canonical 最终 dry-run payload 或隔离 HOME 本地结果完全等价；
- 13 组 block/ambiguous 均在 dispatch 前停止；
- 2 组原生 hidden 兼容 `keyword/member-user-ids` 保持正常；
- 4 组 Calendar、Doc、Drive、Mail 非目标 alias 最终 payload 未变化；
- 3 组生产入口 PreParse 验证通过：`stop --app-id`、`restart --bot-client-id` 和 excluded `version check-approval` 均命中候选；
- `dev connect --app-id` 仍返回 unknown flag，证明可执行父命令没有被候选误标为中央治理成功；
- `internal/cli`（128.774s）、`internal/pipeline`（0.967s）、`internal/app`（240.818s）全部通过；
- `check-generated-drift.sh` 通过，Schema assembly 两次结果一致；
- `check-schema-catalog.sh` 通过，最终仍为 27 个产品、1018 个工具；
- runtime confirmation truth、flag/help/schema homology 和 Catalog assembly determinism 均通过。

## 6. 第一轮改造建议

1. 先评审并落地 31 个 Dev concept、6 个既有 concept 的 Dev scope 和 20 个精确 override，不修改真实 CLI flag。
2. Dev owner 重点审核 `dev app list`、`permission list`、`robot config/submit`、`security config`、`webapp config` 六个多角色 leaf 的 block/ambiguous。
3. 单独评审 `dev connect` 父命令是否应进入中央 reducer；在没有框架级支持前，只声明 canonical 边界。
4. 单独评审 3 个 `schema_command_exclusions.go` 项是否仍需排除，这与参数 alias 治理分开提交。
5. 将本报告的 22/13/2/4 行为集转为长期测试，并保留 `--client-id`、`confirm-name`、`media-id`、`url/urls` 四类强保护回归。

## 7. 可复用到其他产品的流程

1. 冻结线上 main commit，并在隔离副本重建二进制；
2. 合并官方 runnable Cobra 树、runtime Schema、exclusion、可执行父命令和隐藏 flag 形成全量清单；
3. 逐 leaf 对账真实 Help、完整 Schema 和 Skill；
4. 按业务实体、角色、cardinality、值域、数据结构和安全含义聚合参数问题；
5. 只有值可原样传递且目标唯一时配置 alias，否则使用 block/ambiguous 或列为不支持；
6. 基于冻结提交正式 `param_concepts.json` 生成完整候选，不覆盖当前工作区正式文件；
7. 全量审核命令路径、alias 目标、真实 flag 冲突、保护规则和非目标 diff；
8. 在隔离副本验证生成确定性、PreParse、最终 payload、本地结果、保护、原生兼容、非目标回归、包测试和政策门禁；
9. 报告同时交付可落地规则与真实运行边界，不能用 Schema 存在、Help 可解析或 PreParse 命中替代最终行为证据。
