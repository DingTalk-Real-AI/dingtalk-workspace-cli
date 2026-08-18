---
name: dingtalk-wiki
description: 钉钉知识库与空间管理。Use when 用户明确说 知识库/wiki/创建、查找或列出知识库/命名的团队知识空间/个人知识库/知识库成员/库内节点创建、列表、搜索、复制、移动、删除或知识库动态。仅说“文档空间/我的文档”不触发：普通存储管理与全局文件搜索走 dingtalk-drive；节点正文读写走 dingtalk-doc。命令前缀：dws wiki。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉知识库 Skill

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行。只有 leaf 参数或安全语义不确定时读取精确 Schema，只有 Cobra flag 不确定时读取精确 leaf Help；不要加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "wiki +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws wiki <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service wiki --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws wiki +resolve-space` | read | 按名称搜索知识空间并解析出唯一 spaceId（只读） |
| `dws wiki +wiki-new-doc` | write | 在指定名称的知识库下新建一个文档节点（自动按空间名解析 workspaceId） |
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 按名称解析唯一知识库 | `+space-list --type <orgWikiSpace\|myWikiSpace> --limit 50 --page-all` 后精确匹配名称 | 先明确组织/个人范围；仅 `autoPageComplete=true` 且全量中恰好一个同名项时取 workspaceId |
| 搜索或列出知识库 | `+space-search --query <关键词>` / `+space-list [--type orgWikiSpace\|myWikiSpace]` | 用户要求全部时加 `--page-all`；个人知识库必须明确语义 |
| 为 Drive 发现钉盘存储空间 | `dws wiki space list --type <orgSpace\|mySpace> --format json` | managed 只读前置；返回 spaceId/rootFolderId 后切回 Drive，不进入 Wiki node/member 路由 |
| 已知 workspace 查看详情 | `dws wiki +space-get --workspace <ID或URL>` | 已知 ID 不重复搜索 |
| 创建或删除知识库 | `+space-create --name <名称>` / `+delete-space --workspace <ID>` | 创建会读回；删除整个空间是高风险操作 |
| 浏览或搜索库内节点 | `+node-list --workspace <ID> [--folder <ID>]` / `+node-search --workspace <ID> --query <词>` | 列目录与关键词搜索分开；全量列表加 `--page-all` |
| 查看节点元数据 | `dws wiki +node-get --node <ID或URL>` | 正文读写随后切 Doc |
| 已知 workspace 创建节点 | `+node-create --workspace <ID> --name <名称> [--type <类型>]` | 支持 adoc/axls/able/appt/adraw/amind/folder；创建后读回 |
| 只有知识库名称时新建空文档 | 先按全量 `+space-list` 唯一解析，再 `+node-create --workspace <ID> --name <标题> --type adoc` | 不用单页 `+wiki-new-doc` 猜空间；正文另走 Doc |
| 复制、移入知识库或移出到我的文档 | `+node-copy` / `+move` / `+move-to-drive` | 使用真实 nodeId/workspaceId/folderId；按 Runtime confirmation |
| 删除库内节点 | `+node-delete --workspace <ID> --node <ID>` | 删除前核对归属并确认 |
| 列出或修改知识库成员 | `+member-list` / `+member-add` / `+member-update` / `+member-remove` | userId 1-30 个；角色必须显式 |
| 查看知识库动态 | `+feed-list --workspace <ID>` | 要全部动态加 `--page-all`，否则只是一页 |

## 当前最短路径

- 已知 workspaceId：直接执行 space/node/member/feed 目标命令，不再 resolve。
- 只有知识库名称：先明确组织/个人范围，用 `+space-list --limit 50 --page-all` 取完该范围后按完整名称唯一匹配；未知范围先消歧，不同时扫描两个范围并猜测。
- `+space-search` 只用于快速浏览候选；当前 `+resolve-space/+wiki-new-doc` 不暴露名称搜索的分页完成证据，不作为权威唯一解析或写入 Golden Route。
- 已知 nodeId/URL：元数据直接 `+node-get`；正文直接切 Doc，不先 list/search。
- 创建节点后返回的 nodeId 直接传给 Doc；不通过同名搜索重新定位。
- move/copy/delete 已含预检或读回时，不由 Agent 重复拼装原子命令。
- 普通“文档空间/我的文档”的文件操作按存储意图走 Drive；仅当缺少 Drive spaceId/rootFolderId、需要发现 `orgSpace/mySpace` 时调用 managed `wiki space list`。`orgSpace` 以返回的 nextToken 续页，`mySpace` 不分页；取 ID 后立即回到 Drive。

## 关键结果语义

- `+space-list/+node-list/+feed-list` 默认单页；全量请求显式加 `--page-all`，并检查 `autoPageComplete/autoPageStopReason/pagesFetched` 与分页元数据。
- `+space-search/+node-search` 缺少业务数组不是零命中；只有显式空数组才可报告空结果。
- 名称解析只有在 scoped `+space-list` 返回 `autoPageComplete=true` 且全量中恰好一个精确同名项时才成立；0 条、多条或分页未完成都停止。
- `+space-search`、`+resolve-space` 的单页结果不能证明全局唯一；不得把首页唯一候选直接用于写入。
- 创建空间/节点和复制节点必须取得新 ID 并读回；移动必须验证 workspace/folder；删除必须有 `success=true`。
- 成员列表服务端没有续页游标且最多 50，不能把上限内结果宣称为全量；成员写只具备终态响应证据，不虚构精确读回。
- `partial_failure`、分页未完成或写入效果未知都不是成功。

## 参数与安全边界

- workspaceId、nodeId、folderId、userId 不互相替代；名称不能当 ID。
- 写操作只按精确 leaf Runtime 判定确认；已明确授权具体空间/节点/成员、动作与影响时，首次正式执行直接带 `--yes`，否则先确认。参数变化重新确认；禁止用缺少 `--yes` 的失败探测。
- `+member-list --limit` 为 1-50；成员写 `--users` 为 1-30 个，角色仅 `MANAGER|EDITOR|DOWNLOADER|READER`。
- `+node-create --type` 决定内容产品；建好后 adoc→Doc、axls→Sheet、able→AITable。
- Profile/组织在空间解析、节点操作和验证期间保持一致。

## 按需加载

Golden Route 参数足够时不读 reference；否则最多读取一个：

| 触发条件 | Reference |
|---|---|
| 文档空间、知识库、Drive/Doc 边界不明 | [intent-guide](references/intent-guide.md) |
| 节点类型、复制、移动、移出或删除细节 | [node-ops](references/wiki-node-ops.md) |
| 成员角色、上限与验证语义 | [members](references/wiki-members.md) |
| 分页、空间、动态及低频错误 | [wiki reference](references/wiki.md) |
| 跨产品创建/写正文短流程 | [lite-recipes](references/lite-recipes.md) |

## 错误最短路径

1. 空响应、缺失集合、零/多候选或分页不完整：停止后续写入并返回证据；`+resolve-space resolved=true` 也不能替代完整空间列表的分页完成证据。
2. workspace/node 归属不一致：停止，不尝试换一个 ID 或 profile。
3. 写响应缺少新 ID 或 `success=true`：效果未知，按名称/ID定向回读，不盲目重放。
4. `unknown flag` 只查当前 leaf Help；`unknown command` 只查一次 Wiki Shortcut 清单。
5. 正文、普通存储或 Base 记录误路由时切回对应产品，不在 Wiki 内试探近似命令。

## 跨产品边界

- 明确知识库容器、成员、库内层级与动态 → Wiki。
- 锁定库内 adoc 节点后的正文读写/导出 → Doc；axls 内容 → Sheet；able 记录/字段 → AITable。
- 普通文件、文件夹、“我的文档/文档空间”的存储搜索、传输和整理 → Drive。
- Drive 存储空间发现可复用 managed `wiki space list --type orgSpace|mySpace`；结果是 spaceId/rootFolderId，不能交给 Wiki node/member。知识库 `orgWikiSpace/myWikiSpace` 仍返回 workspaceId。
- Wiki 节点移入/移出使用 `+move/+move-to-drive`；不要把 workspaceId 当普通 Drive folderId。
