# Whiteboard 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交的 Cobra Help、
运行时 Schema、白板实现、dingtalk-misc Whiteboard Skill、OpenNodes V1 协议与冻结正式
`internal/cli/param_concepts.json`。未使用固定 Catalog、历史 badcase、用户 Shortcut 或
已安装插件，也没有修改当前工作区正式别名表。

Whiteboard 有 2 个 Agent 可见叶：`whiteboard query` 与 `whiteboard update`。两者都要求一个
文档 `nodeId/URL` 和一个内嵌白板 `partId`；update 另要求本地 OpenNodes V1 信封文件。
最危险的参数幻觉是把 `partId`、卡片 `blockId/uuid`、query 返回的页面 `id`、OpenNodes
请求内临时节点 `id` 混成同一个 `--id`，或把 `overwrite/nodes/pages` 猜成 CLI flag。

候选的规则本身已完成条件验证：在隔离副本临时加入 whiteboard fresh 生成树可见性前置后，
生成器、PreParse、2 组 alias/canonical dry-run 逐字节比较、12 组代表 block/ambiguous、
非目标结构恒等、包测试、generated drift 与 Schema Catalog 政策均通过。完整
`internal/app` 运行 308.410 秒后仅 complete-command 覆盖测试失败，缺 query/update 两个
模板及 8 条 active fixture。

但是冻结提交原样执行 `go generate ./internal/cli` 会先失败：fresh declaration-only 命令树
把 whiteboard 顶层隐藏，生成器报告 2 个 override 与 5 个 concept scope 均不匹配可运行
Cobra 命令；普通 CLI 运行时因先注入静态服务又能执行这两个叶。这是基线的生成树/运行时
可见性漂移。因此正式状态为“候选规则条件通过，但必须先修 whiteboard fresh 生成树可见性，
再补 2 个 complete-command 模板，之后方可落地”。

## 参数问题

### 1. 文档 nodeId、白板 partId 与卡片 blockId 是三个值域

`--node` 接受承载白板的文档 nodeId、URL 或同值域 token；`--part-id` 接受白板资源 ID，
也就是 `doc whiteboard insert` 返回的 `whiteboardId`。insert 同时返回的 `blockId` 只用于
文档块删除或回查，JSONML 的 `uuid` 也是块身份，都不能当作 partId。

候选复用正式 `doc_node_id` 并仅追加两个 whiteboard 命令范围；新增
`whiteboard_part_id`，允许 `whiteboard-id/whiteboard-part-id/board-part-id → part-id`。
`block-id/card-id/uuid` block，裸 `id/part/board` ambiguous。别名表不读取 JSONML，也不在
多个白板卡片中自动取第一个。

### 2. query 的页面 id 不是 CLI page-id

OpenNodes query 返回单页 `pages[0].id`，但 DWS 当前只支持单页白板，命令没有
`--page-id`。返回页面 id 是读取结果的一部分，不是下一次 query/update 的定位参数。

候选在两个命令上都 block `page-id`，对裸 `page` ambiguous；不会把 page id 映射到
part-id，也不会新增多页能力。

### 3. update 的 source 是本地完整信封文件，不是内容或 query 输出

`--source` 是一个可读 UTF-8 JSON 文件路径，文件必须包含
`overwrite + source.schemaVersion + source.catalogVersion + source.nodes`。query 返回的是
`schemaVersion + catalogVersion + pages` 的完整读投影，不能直接回写；query-only 字段、
真实节点 ID 和只读节点也不能原样变成 update source。

候选新增 `whiteboard_source_file`，只接受角色完整的
`source-file/request-file/open-nodes-file/whiteboard-json-file → source`。泛化
`file/file-path/path/input` ambiguous；`content/payload/json/body/stdin/query-result` block。
它只改 flag 名，不读取、生成或转换文件内容。

### 4. append/overwrite、nodes 和临时节点 id 属于文件内部

append 是 `overwrite=false` 或省略；overwrite 是文件顶层 `overwrite=true`。`nodes` 是
`source` 内数组。两者都不是 CLI flag。append 至少一个节点；overwrite 允许空数组并可
清空整页。节点 `id` 只是一次请求内的临时引用，不能 patch 既有真实节点。

候选 block `append/overwrite/mode/nodes/node-json`，避免 Agent 猜出不存在的 flags；裸
`id` 保持 ambiguous。模式、版本、节点类型、枚举和引用关系继续由本地外层校验与白板服务
完整校验。

### 5. 输出过滤 flag 真实存在，但 whiteboard 明确拒绝

全局 `--jq`、`--fields` 是真实 Cobra flags，因此中央生成器禁止把它们设为 alias 或 guard；
whiteboard 在 RunE 中显式返回“不支持”。直接验证 `query --jq .` 退出码为 3，发生在远端
调用前。

候选保持这两个真实 flag 原生，不在 `param_concepts` 重复声明。此差异应由 Help/Schema
能力声明继续治理，不能通过别名表隐藏。

### 6. `--yes` 是安全门，不是可归一业务参数

update 的 append 与 overwrite 都是远端写入，`confirmation=user_required`；overwrite 可能
删除全部自有节点，空数组可清空页面。`--yes` 是真实确认 flag，不能由 `confirm/force`
自动补出，也不能在示例或候选 alias 中制造。

候选完全不改 `--yes`。验证只使用 `--dry-run`；`overwrite=true + nodes=[]` 的本地 dry-run
可通过，但真实执行前仍必须先 query、展示影响并取得用户确认。

### 7. fresh 生成树与普通运行时的可见性不一致

冻结提交的 `register_whiteboard.go` 注册了公开命令，普通 `make build` 后
`dws whiteboard --help` 可见 query/update；但 fresh `NewSchemaSourceRootCommand` 在未注入静态
服务时调用 `hideNonDirectRuntimeCommands`，`staticCommands` 仅含 dev/markdown，导致
`walkRunnableParamCommands` 跳过隐藏的 whiteboard 父树。

原基线生成因此对候选报 7 个 missing-scope 错误。隔离验证仅临时把 `whiteboard` 加入
`staticCommands`，不修改正式候选表以外的交付，也不把该补丁带回当前工作区。正式 PR 必须
先用仓库认可的声明可见性修复消除该漂移，并补语义回归测试。

## 当前别名表可以实施的方案

1. 将 `whiteboard query/update` 精确追加到正式 `doc_node_id` 的命令范围。
2. 新增 `whiteboard_part_id` 与 `whiteboard_source_file` 两个专用 concept。
3. 为 query/update 建立两个 `scope_strict` override；对块、页面、模式、文件载体和泛化
   `id/file/page/part` 做 block/ambiguous。
4. 保持真实 `--yes/--jq/--fields` 原生；不改值、不解析 source、不新增命令能力。
5. 先修 fresh 生成树可见性，再补两个 complete-command 最终 payload 模板。

## 当前能力支持不了的事项

- 从文档 URL 自动读取 JSONML、定位 `cardType=hetu` 并选择 partId；
- 在多个白板候选中自动取第一个，或把 blockId/uuid 转成 partId；
- 把 query 的 `pages/nodes` 自动投影成合法 update 信封；
- 把真实节点 ID 改写为局部 patch，或把 pageId 变成 DWS 命令参数；
- 从 `--content/--json/--nodes` 合成本地 source 文件；
- 把 `--overwrite/--append` CLI flag 写入 JSON 文件内部；
- 校正 schemaVersion、catalogVersion、geometry、icon catalogId 或节点枚举值；
- 上传 SVG、查询资源并自动填 Vector resource；
- 根据 overwrite 风险自动添加 `--yes` 或替用户确认；
- 用别名表修复 fresh 生成树可见性或新增 complete-command 模板。

## 第一轮改造建议

第一轮可采用候选的 2 个新 concept、1 个既有 concept 精确扩围和 2 个作用域保护，但落地
顺序必须是：先修 declaration-only fresh 命令树可见性并加入回归；再为 query/update 注册
complete-command E2E 模板，覆盖 8 条 active fixture；最后重跑生成、完整 app 与政策门禁。
OpenNodes 内容转换、SVG/Vector 编排、多页或真实节点 patch 不进入本轮。

## 候选 `param_concepts.json` 改动与审核

- 新增 2 个 Whiteboard 专用 concept；既有 `doc_node_id` 仅追加 2 个精确命令范围；
- 新增 2 个 command override；新增 20 个 fixture，其中 8 个 active alias、12 个 guard；
- 条件生成从 569 个命令作用域变为 571 个，生成 24 个 alias、61 个 blocked、19 个
  ambiguous token；fallback 无变化；
- 非目标 concept、override、fixture 结构恒等；未发现 guard 与真实 flag 冲突；
- 2 组 query/update alias/canonical stdout/stderr 逐字节相同；
- 12 组直接保护检查稳定返回 `blocked_flag` 或 `ambiguous_flag`；
- 原基线生成失败是 whiteboard 父树可见性漂移，不是通过扩大 scope 或删除 guard 绕过；
- 候选仍是基于冻结正式表的完整待审核草稿，不包含临时 Go 前置补丁。

候选位置：`docs/parameter-hallucination/whiteboard/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| 冻结提交原样生成 | 未通过 | 7 个 missing-scope：2 个 override、5 个 concept scope；fresh 生成树跳过 whiteboard |
| 条件 JSON 解析与生成器 | 通过 | 隔离副本临时可见性前置；569→571 个作用域 |
| PreParse 与 alias/canonical | 通过 | query/update 两组 dry-run 最终输出逐字节一致 |
| block/ambiguous | 通过 | 12 组代表错误在派发前停止 |
| OpenNodes 外层行为 | 通过 | query `--jq` 退出 3；query 结果作 source 退出 3；overwrite 清空 dry-run 退出 0 |
| 非目标回归 | 通过 | JSON 结构恒等；生成 diff 仅 2 个 whiteboard entry；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 条件通过 | CLI 80.634 秒 |
| generated drift | 条件通过 | 双次生成确定；571 个作用域 |
| Schema Catalog 政策 | 条件通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 未通过 | 308.410 秒；仅 complete-command 覆盖测试失败 |
| complete-command payload 门禁 | 未通过 | 200/202 个活跃命令有模板；缺 2 个命令、8 条 active fixture；387 active cases |

正式替换前必须同时完成 fresh 生成树可见性修复、2 个模板和全量复验。未完成前，本候选
不得直接替换正式表。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `3a45256af6fe3773a7a944463a5027003a2d25dab8b7e8613f02dcf5435dfba8`。
- 命令实现：`internal/helpers/whiteboard.go`、`internal/helpers/register_whiteboard.go`；
  关联创建命令在 `internal/helpers/doc_whiteboard.go`。
- Skill：dingtalk-misc Whiteboard reference、OpenNodes V1 overview/query/update/errors、Recipes。
- Schema 来源：同一冻结提交运行时声明组装；未使用历史或固定 Catalog。
