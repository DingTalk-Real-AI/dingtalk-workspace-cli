---
name: dingtalk-aitable
description: 钉钉 AI 表格（多维表）业务操作与 CLI 契约评审。只评审 aitable 合成 JSON 回执也必须加载本 Skill；“不要执行线上业务”不等于免除本机离线契约核对，不能按通用 JSON 经验直接作答。评审以用户原文指定的核对入口为准；背景写命令不是额外目标，Agent 自拟的 Skill 参数不得扩大范围。用户给出 aitable 命令、form share 合成回执、dry-run 或询问写入恢复方案时也必须加载本 Skill；这是本机 Help/Schema 契约核对，不是仓库源码审查，不从源码搜索开始。先加载正文确定入口与规则，不能仅凭摘要执行。适用于 Base/Table、字段与记录、评论、筛选排序、公式、SQL/PostgreSQL/JOIN、表单分享与 CP 同步、App 页面/Widget、模板、导入导出、仪表盘/图表、附件及审批等数据源同步。不做电子表格单元格操作（走 dingtalk-misc）或文字文档编辑（走 dingtalk-doc）。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 表格 Skill

## 先区分操作、用法与契约评审

用户只要求评审返回值或恢复方案时，不执行业务操作，也不按业务任务扩展到上游写入命令。发现目标以用户原始消息为准，Agent 自行摘要或传入 Skill 的参数不是新增请求，不能将“核对该对账命令”改写成“比较对账与原创建命令”。保留用户正在核对的完整原子路径或 `+` Shortcut 名，不做同义替换。背景里提到的原创建命令不是额外发现目标。

只评审一个命令时，先取得一次该入口的 compact Schema，再依据本次结果回答；不要并行或顺序查询背景命令，也不要追加同义入口、完整 Schema 或 Help。未发布 `result` 时说明该结构未公开，不靠重复查询或示例推断字段。用户明确要求比较多个命令时，才分别查询各自契约。Help/Schema 是本机离线查询，“不要执行线上业务”不禁止它；用户明确禁止任何命令时则不查询，并说明契约未核对。

评审中用户要求下一步只读命令时，回答先给出有当前契约依据、使用原 ID/核对键的完整命令，并说明尚未执行；紧接着给出关键判断与禁止动作，再简要解释依据。不先展开工具过程、Schema 字段表或长篇引用，不把下一步埋在分析末尾；无法确定只读入口或参数时说明缺口，不能猜测。没有索要命令时，先用简短结论回答用户的全部安全问题：是否允许重放写入、哪些结果应保留或丢弃、只读恢复有什么前提，再按需解释；不能把关键禁止动作留到分节解释或末尾总结。明确区分本机离线契约核对、用户提供的回执和仍未验证的线上状态：本机 Schema 只能证明命令契约，不能证明业务已经执行或远端状态已经核实。

仅评审失效游标恢复时，读取 [record-query](references/aitable/aitable-record-query.md) 开头的“只评审失效游标恢复”段，集中说明丢弃旧结果、UNAVAILABLE 等待前提和禁止重放写入这三项边界；不转入该 Reference 后面的实际查询流程，也不从新旧结果差异推导补写许可。

以下意图路由优先于下方通用 Schema 导航。表单分享的用法询问与返回值评审不能共用发现路径：

| 请求意图 | 本次唯一契约查询 |
|---|---|
| 评审返回值、恢复方案、故障或 dry-run 样本 | 把正在核对的完整入口原样放入 `dws schema --cli-path "aitable <原入口>" --compact --format json`；保留 `+`，不改查 Help，也不切换原子/Shortcut |
| 仅问原子 `form share update` 的写法 | 只执行 `dws aitable form share update --help`（禁止改查 Schema） |
| 仅问 Shortcut 的写法 | 只查该 Shortcut 的 compact Schema |

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 钉钉业务操作只通过 `dws` CLI；本 Skill 明确发布的脚本可编排 `dws` 并完成预签名文件上传。结构化读取使用 `--format json`，按真实返回判断结果。
- 已知 leaf 直接执行。只有参数或安全语义不确定时，最多读取一次 `dws schema --cli-path "aitable <leaf>" --compact --format json`；仅当该 compact leaf Schema 与 Cobra 实际不一致时，才读取同一 leaf 的 `dws aitable <leaf> --help`。禁止通过父级 Help、产品 Help 或完整 Catalog 探索命令。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；本轮用户已明确要求执行、目标与影响无歧义的非破坏性写操作时，该明确指令就是本次确认，首次调用直接携带 Runtime 所需的 `--yes`，不先制造 `confirmation_required`。文件导入是非幂等操作，仍按 Golden Route 先触发 Runtime 确认门禁，确认后再追加 `--yes`。删除、停用自动化等破坏性或高风险动作仍须先说明对象、动作与影响并取得独立确认。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- Runtime Schema 是当前能力与安全语义的权威来源；同一 leaf 任务内只读一次并复用。只有 Schema 缺字段、含义不清或与真实执行冲突时才读 leaf Help；版本、profile 变化或出现契约错误时立即作废缓存。
- mutation 返回、HTTP 200 和退出码 0 都只是调用回执，不是最终对象状态。写后必须清除受影响的读取缓存，用返回的真实 ID/唯一键发起独立读回，不复用写前结果。
- 同一 Base 的写入默认串行；只有不同 Base、无数据/ID/顺序依赖且失败可独立回读时，才允许受控并发写。无依赖只读调用默认最多 4 路并发。
- 写调用超时、连接中断或返回结果未知时，禁止原样自动重放；先查真实状态。只读调用可对瞬态错误做有上限重试。
- 结构化结果可能是统一 `ok/outcome`、旧 `status` 或 boolean `success` 信封；不固定假设 `data` 层级。`partial_failure` 可以在 stdout 中并返回退出码 7，必须先解析信封再判断执行结果。
- 文件上传返回的 `uploadUrl`、token、authorization、signature 等是凭据；不输出、不写日志，结构化错误也要递归脱敏。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

## 表单分享用法回答契约

收到仅询问用法的请求后，第一步必须立即实际执行且仅执行下列对应命令；即使用户提到“help/schema”，也按入口选择，不能自行替换：

- `form share update`：`dws aitable form share update --help`
- `+form-share-update`：`dws schema --cli-path "aitable +form-share-update" --compact --format json`

Shortcut 名称开头的 `+` 是命令名不可省略的一部分；不得改写、试探其他拼法或改用 `--help`/`-h`。

发现门禁：即使 Skill 或参考文档已提供完整示例，回答前也必须实际执行一次且仅执行一次目标 leaf 的安全 help/schema 查询；不得仅依据 Skill 或参考文档直接作答。本规则优先于下方 reference 导航：命中时不读取任何 reference，不执行其他命令。
用户仅询问用法时，最终回答必须先给出完整命令；缺少必填 ID 时则给出带明确占位符的完整命令模板，禁止猜测。随后明确说明“未传入的分享配置保持原值”；不得执行目标写操作或声称已经执行。上述只读查询是唯一允许的命令。

查询成功后，最终回答只能包含两行纯文本：不要 Markdown 代码围栏、标题、表格、回读命令或其他内容。第一行放用户所问入口的完整命令；已有的必填值必须原样使用，缺少的值必须保留为 `<BASE_ID>`、`<TABLE_ID>`、`<VIEW_ID>` 等明确占位符。第二行先列出需要替换的占位符（没有则省略替换说明），再给出固定的未执行说明，然后立即结束：

```text
dws aitable form share update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --enabled true
请将 <BASE_ID>、<TABLE_ID>、<VIEW_ID> 替换为真实值；未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。
```

已知 ID 与占位符必须区别处理：用户明确给出的短 ID 也按原值使用，不因其长度或看起来像示例就要求替换。所有 ID 已知时，第二行必须原样为“未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。”；只有命令中确实用了占位符时才添加对应替换说明。Shortcut 的命令行必须保留 `--format json`，固定说明中的 `help/schema` 不因本次只查 schema 而改写。

两种入口都必须完整保留用户指定的配置值，尤其是表单名对应的 `--form-name`；不得因精简为两行而只留下 ID 和 `--enabled`。以下是已知 ID 的 Shortcut 用法回答示例（标题按用户输入替换，不能省略）：

```text
dws aitable +form-share-update --base-id base-123 --table-id table-456 --view-id view-789 --enabled true --form-name "报名表" --format json
未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。
```

## 返回值评审专用查询（不适用于命令用法询问）

作答前必须有本次实际执行的目标 compact Schema 查询结果；仅加载 Skill 或看到合成样本不满足此条件。先执行下表命令，再解释，不能直接根据 Skill 回答。

Help/Schema 是离线契约查询，不调用线上业务，也不读写用户的表单。因此“只分析合成返回，不要执行线上业务命令”仍允许且需要下表的本机查询，不能将其误判为线上读取而跳过；只有用户明确禁止任何命令或本机查询时才不执行，并说明本机契约未核对。

仅评审返回值/故障结果（而非询问写法）时，不套用两行命令模板，也不适用“优先 Shortcut”规则。用户指定的原子/Shortcut 入口必须原样保留，即使它们共用 Result 契约也不能互换；严格按下表查询一次后解释样本，不搜索源码/reference，不执行业务读写。

| 用户指定入口 | 唯一契约查询 |
|---|---|
| `form share get` | `dws schema --cli-path "aitable form share get" --compact --format json` |
| `form share update` | `dws schema --cli-path "aitable form share update" --compact --format json` |
| `+form-share-get` | `dws schema --cli-path "aitable +form-share-get" --compact --format json` |
| `+form-share-update` | `dws schema --cli-path "aitable +form-share-update" --compact --format json` |

不得用搜索源码或 reference 代替本机契约查询。get 只诊断分享配置，不返回 `cpSynced`；不能建议“通过 get 回读 cpSynced 后确认闭环”。未验证的 CP 应继续标为未确认并交由服务端诊断，不能把 get 的成功或 UUID 非空当作恢复证明。

先选择 Schema 的实际结果分支：`data.executed=false` 的 dry-run 预览只含 `tool/arguments/executed`（Shortcut 还含 `dry_run=true`），不需要服务端状态或 `cpSynced`，`ok=true` 只表示预览成功。不要把真实执行成功分支的必填字段套到预览上。字段是否必填只读所选分支的 `required`；是否允许额外字段只读该对象的 `additionalProperties`，不能从 `arguments` 子对象推断父对象。

真实执行结果判断：分享开关依据 `enabled`，保留 UUID 不代表开启；UUID/封面为空就如实报告，不能推测唯一成因或拼装封面 URL。get 的成功不证明 CP 同步。update 仅在必需字段完整且类型正确、`cpSynced=true` 时成功；缺失/false/类型异常均不能确认闭环。统一部分失败无顶层 error：原始响应在 `data.succeeded[0].response`，该阶段仅表示收到回执；失败原因在 `data.failed[0].error`，含 `execution_started=true`。不要自行重放写入或补偿 CP。合成样本若不符合该结构，应指出不匹配，不能把 Schema 中的字段补写成样本已有事实。

解释边界：`status` 未公布枚举含义时保留原始数值，不把 0/1 自行翻译成未发布/已发布；`cpSynced=false` 只支持“CP 终态未确认”，不证明外部用户必定无法访问。UUID/封面空值不能证明此前从未创建，短 ID 不能仅因长度被判为占位符。

即使外层仍为 ok=true 或返回结构不符合契约，也不得为再次校验 CP 而执行或建议重发 form share update / +form-share-update（包括稍后传相同配置）；诊断不能新增写入，只保留回执并交由服务端排查。


实际执行 `form share update` 或 `+form-share-update` 后，成功结果必须同时检查 `shareFormUuid`、`status`、`formCover`、`cpSynced`；只有 `cpSynced=true` 才能向用户确认分享闭环完成。部分失败或 `cpSynced=false` 不得描述为成功。DWS 不自行调用第二个 View 更新命令补偿 CP。`formCover` 在旧服务端发布窗口内可能为空，应如实说明，不能由 DWS 拼装封面 URL。

> 命令参考：[aitable.md](references/aitable.md)；PostgreSQL 只读查询：[aitable-psql.md](references/aitable/aitable-psql.md)；复杂命令按需加载 `references/aitable/*.md`；剧本：[06-data-analytics.md](references/06-data-analytics.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`aitable` 当前有 126 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知 leaf 直接执行。参数只查 `dws schema --cli-path "aitable <leaf>" --compact --jq '{cli_path,parameters,constraints,confirmation}' -f json`；仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help；同一任务只读一个 Reference。

仅当根路由、精确 task reference 和 `references/aitable.md` 的低频原子索引都无法定位能力时，才执行 `dws shortcut list --service aitable --format json` 做最终回退；不要为已知意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route（高频复合任务）

已由当前 AITable 调用返回且类型已确认的 ID 直接使用；名称先唯一解析为稳定 ID。用户直接提供的 `/i/nodes/` URL 或来源未验证的 nodeId 先执行 `dws drive info`；若为 `extension=dlink`，将返回的 `result.fileId` 保存为快捷方式入口 ID 并传给 `dws doc info`，再逐跳读取目标 `linkSourceInfo`，最终确认 `extension=able` 后将目标 `linkSourceInfo.nodeId` 作为 baseId。解析失败、字段缺失、ID 重复或最终类型不是 able 时停止；只有明确移动、改名或删除快捷方式入口本身时才保留最初的 `result.fileId` 并切到 Drive。零命中或多候选时也停止，不默认选第一项。

数据分析，以及 PostgreSQL/SQL/SELECT/JOIN 或跨表关联查询，先读 [aitable-psql.md](references/aitable/aitable-psql.md)。原始记录筛选、排序和 Top N 使用 `record query`；单表直接标量、分组或去重统计使用 `record stats` / `record group-stats`；仅同 Base JOIN、字段间算术、CASE、聚合后派生、汇总结果 Top N 或排名、窗口计算等复杂分析使用 `psql`。选用 `psql` 时固定同一 DWS 入口，先用 `dws aitable psql -d <baseId> -l` 发现逻辑表，再用 `-t <tableId>` 查看列类型，使用返回的 `Name` 执行 `LIMIT 3` 最小查询，成功后才以 `-c <SQL>` 执行正式只读查询；`psql` 输出 PostgreSQL 表格文本，不添加 `--format json`。psql 失败后只能从原始意图重新判定：完全属于单表原始记录时重发 `record query`，完全属于单表直接统计时重发 `record stats` / `record group-stats`；必须丢弃 psql 未完成结果并说明切换原因，复杂分析、结果合并和本地等价计算均禁止降级。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 从已确认的 AITable URL 解析稳定 ID | `dws aitable +url-resolve --url <URL>` | 只解析 URL 中已有的 baseId/tableId/viewId/recordId，不远程解析 dlink；原始 `/i/nodes/` URL 必须先按上文规范化，dlink 目标 nodeId 直接作为 baseId |
| 按名称唯一定位并操作 Base/Table | `dws aitable +resolve-base --name <名称>` → `dws aitable +resolve-table --base <ID> --name <表名>` | 默认精确匹配；只有用户明确接受模糊匹配时才加 `--fuzzy` |
| 搜索 Base 候选或检查是否存在 | `dws aitable +base-search --query <关键词>` | 用户说“搜索/找一下/候选/如果没有就创建”时直接走本入口，不先调用 `+resolve-base`；返回 `hasMore/nextCursor`，仅 `hasMore=true` 时续页；AITable Base 名称不得路由到 `dws aisearch person` |
| 浏览 Base 下的数据表 | `dws aitable +list-tables --base <ID>` | 只返回 tableId/tableName，不加载字段 |
| 新建 Base 与整套表字段 | `dws aitable +base-bootstrap --name <名称> --tables '[{"name":"<表名>","fields":[{"fieldName":"<字段名>","type":"text"}]}]'` | 表对象键必须是 `name`，不是 `tableName`；字段使用 `fieldName/type/config`，可选 `description`；参数已足够时直接执行 |
| 复制 Base | `dws aitable +base-copy --base-id <B_OR_URL> [--target-folder-id <FOLDER_ID_OR_URL>] [--only-struct] [--new-name <名称>]` | `base-id` 可传 Base ID 或标准节点 URL；`target-folder-id` 可传 dentryUuid、标准节点 URL 或 Drive 文件夹 URL，省略时复制到源 Base 所在工作区根目录；参数解析与目标验证由 AITable MCP 负责 |
| 已有 Base 新建一张表与字段 | `dws aitable +table-bootstrap --base-id <ID> --name <表名> --fields '<JSON数组>'` | 字段使用 `fieldName/type/config`，可选 `description`；自动按 15 个字段分片并读回验证 |
| 读取字段目录或完整配置 | `dws aitable field list --base-id <B> --table-id <T>` / `dws aitable +field-get --base-id <B> --table-id <T>` | 只需 fieldId/name/type 用 `field list`；需要 config 用 `+field-get`；不存在 `+field-list` 或 `+list-fields` |
| 按名称解析人员、部门或群组实体 | `dws aitable entity search --entity-type PERSON\|DEPARTMENT\|GROUP --keyword <名称>` | 返回候选和可用于筛选的稳定身份；零命中、重名、模糊命中或分页不完整时停止，不默认选择第一项 |
| 查询原始记录、记录筛选/排序、原始记录 Top N 或字段投影 | `dws aitable +record-query --base-id <ID> --table-id <ID> [--record-ids <IDs>] [--field-ids <IDs>] [--filters <JSON>] [--sort <JSON>] [--query <关键词>]` | 用户要求“只返回/仅查看”指定字段时必须传对应 `--field-ids`，不能只在最终文本删列；单表直接标量、分组或去重统计改走 `record stats` / `record group-stats`，复杂服务端分析改走 psql。已获用户明确许可的非分析完整逐行明细可直接使用原子 `record query --all --page-limit <N>`，不需要也不触发 psql 降级门禁。 |
| 新增单条或批量记录 | `dws aitable record create --base-id <ID> --table-id <ID> --records <JSON>` | 当前无 `+record-create`；写前取字段定义，写后按新 ID 回读 |
| 按原写入 token 核对记录结果 | `dws aitable +record-write-result --base-id <B> --table-id <T> --client-token <原UUID>` | unknown 或 ID 不完整时，停止后续写入，下一步仍按原 Base/Table/token 调用本命令只读对账，不改成全表查询；applied 的 ID 集合不保证整批完整或输入顺序，未返回 ID 的记录仍未核实，不按数量差额或输入位置补写；完整 ID 集合确认后再逐 ID 核对实际值，查询失败不是可以重建的写入终态 |
| 更新已知 recordId | `dws aitable +record-update --base-id <ID> --table-id <ID> --records <JSON>` | 自动分片并读回；只传需修改字段 |
| 查询一条记录的变更历史 | `dws aitable +record-history-list --base-id <ID> --table-id <ID> --record-id <ID>` | 已知 recordId 时直接执行，不探测 Help、Catalog 或全量 Schema |
| 管理一条记录的评论 | 查询用 `dws aitable comment list --base-id <B> --table-id <T> --record-id <R>`；创建、回复、更新和删除按需使用同组 leaf | 先读 [comment](references/aitable/aitable-comment.md)；topicId/commentKey 只复用同一记录真实返回；空评论页仍读取 `meta.pagination`，仅 `meta.pagination.endpoint_exhausted=true` 时停止，否则将 `meta.pagination.next_token` 原样传给下一次 `--cursor`；写入未知状态先 list 对账 |
| 按业务键同步或按条件批改 | 唯一键用 `dws aitable +record-upsert-by-key ...`；有界批改用 `dws aitable +record-bulk-patch ... --max-matches <N>` | upsert 仅允许 0 条创建、1 条更新；批改必须有 query/filters/record-ids 边界。普通 update/upsert 直接执行；只有历史、分享、删除恢复、空行或特殊字段值才读 [record-ops](references/aitable-record-ops.md)；明确 AND/OR、日期或比较操作符只读 [filter-sort](references/aitable/aitable-filter-sort.md) |
| 生成记录分享链接并发送给联系人 | `dws aitable +record-share-links --base <B> --table <T> --record-ids <IDs>` → `dws chat +dm --to <姓名> --text <完整链接文本>` | AITable 只生成链接；用户要求“发送”时还必须完成真实发送，不能停在联系人解析 |
| 创建或复制视图 | 创建用 `dws aitable view create --base-id <B> --table-id <T> --view-type <Grid|FormDesigner|Gantt|Calendar|Kanban|Gallery> [--name <名称>]`；复制用 `dws aitable +view-duplicate --base-id <B> --table-id <T> --view-id <V> [--new-name <名称>]` | 创建和复制直接执行；需要配置时按下方“按需加载”选择一个 View Reference |
| 创建并验证 Dashboard，按需创建 Chart | `dws aitable dashboard create --base-id <B> --name <名称>` → `dws aitable +dashboard-get --base-id <B> --dashboard-id <D>`；需要 Chart 时按下方“按需加载”处理 | 只使用创建返回的真实 dashboardId；失败时不要猜同义命令或更换 dashboardId |
| 管理 AI 表格应用模式 | `dws aitable app get --base-id <B>` → `dws aitable app page list --base-id <B>` → 按需 `app page create/update/move/delete` 或 `app widget create/get/list/update/delete` | 一个 Base 只有一个面向用户的 App；页面 `pageId` 同时是对应 Dashboard ID。Widget 的 `config`/`layout` 是完整对象，更新前先读回；创建操作未知状态时不得自动重放 |
| Base 内创建 Section 并移动节点 | `dws aitable +section-create --base-id <B> --name <名称>` → `dws aitable +section-move-node --base-id <B> --node-id <N> --new-parent-section-id <S>` → `dws aitable +section-list-nodes --base-id <B>` | Table、Dashboard、Section 都是 AITable 的 nsheet 节点；禁止改走 Wiki/Drive 文件夹或移动命令 |
| 将本地 CSV/XLSX/XLS 导入新表 | `dws aitable +import-file --base-id <BASE_ID> --file <FILE_PATH>` | 存储示例不预置 `--yes`；首次调用先触发 Runtime 确认门禁，取得明确确认后由执行方追加。DWS 在进程内完成申请凭证、空 Content-Type PUT 和 `import data`，且不暴露签名 URL；不要猜 `+import-csv` 或给 `import upload` 传 `--file` |
| 接入外部数据源（审批等） | `dws aitable +datasource-list-sources --base-id <ID> --datasource-type OA` → 解析 result 构造 sourceConfig → `dws aitable +datasource-create --base-id <ID> --datasource-type OA --source-config '<JSON>'` | 当前仅支持 OA 审批；processCode/name/iconUrl/url 从 list-sources 原样透传，创建后用 `+datasource-sync-status` 查同步结果 |

### 简单 leaf

除 `aitable psql` 外，结构化命令使用 `--format json` 并从真实返回中提取稳定 ID；`aitable psql` 输出 PostgreSQL 表格文本且不支持 `--format json`，按 `aitable-psql.md` 执行。

意图明确时直接使用；参数不确定才读 leaf Schema：

| 用户意图 | 入口 |
|---|---|
| 查看 / 改名 / 删除 Base | `+base-get` / `+base-update` / `+base-delete` |
| 搜索模板 | `+template-search` |
| 查看 / 跨 Base 复制 / 改名 / 删除 Table | `+table-get` / `+table-copy` / `+table-update` / `+table-delete` |
| 创建 / 更新 / 删除普通字段 | `field create` / `field update` / `field delete` |
| 查看 / 删除 View | `+view-get` / `+view-delete` |
| 查看 / 改名 / 删除 Dashboard | `+dashboard-get` / `+dashboard-update` / `+dashboard-delete` |
| 查看 / 修改应用模式 App | `app get` / `app update` |
| 管理应用页面 | `app page create/get/list/update/move/delete` |
| 管理页面 Widget | `app widget create/get/list/update/delete` |

命令接在 `dws aitable` 后；资源 ID 使用 `--base-id/--table-id/--field-id/--view-id/--dashboard-id/--page-id/--widget-id`，改名使用 `--name`。应用模式所有命令都要求 `--base-id`；`app widget create` 另要求包含 `chartType` 的 `--config` 和包含 `x/y/w/h` 的 `--layout`。`+table-copy` 参数不规则，执行前只读其 leaf Schema。不读操作 Reference、Help 或产品 Catalog。

数据源查看来源用 `+datasource-list-sources`，获取字段用 `+datasource-get-fields`，创建、更新、同步、查状态和查配置用 `+datasource-create` / `+datasource-update` / `+datasource-sync` / `+datasource-sync-status` / `+datasource-get-config`。

## 执行约束

- 已有 ID 直接使用；URL 只解析一次；“唯一定位并操作”用 `+resolve-base` / `+resolve-table`，“搜索候选/存在性检查”直接用 `+base-search`。人员、部门或群组只有显示名称时用 `entity search` 取得稳定身份；这些路径不要串行探测。filters/sort 缺 fieldId 时才读取字段目录。
- Golden Route 已给出准确命令和参数时直接执行；不预读或默认读取通用 `references/aitable.md`。只有操作参数、JSON 结构或恢复语义确实缺失时，才读取下方一个精确操作 Reference。
- Shortcut 已含分片或验证时不重复拆步；已有 Base 新建完整表结构直接用 `+table-bootstrap`。
- 单产品线性任务直接执行，不创建 TodoWrite；只有跨产品或多个独立分支的长任务才建计划，并且只在阶段切换时更新，不在每条 CLI 后刷新状态。
- 用户要求资源名带当前时间戳时只取一次并在 Base、Table、Dashboard 等名称中复用同一值；不要为每个资源分别取时间。
- JSON 已返回所需字段时立即复用；不得为寻找同一字段改用 `--verbose`、`raw`、`pretty` 重复请求。

- 记录 filter/sort 缺 fieldId 时才读取字段目录。
- record filter/sort 与 view filter/sort/group 的协议和 Reference 互斥。普通 record query/create/update/upsert 直达；只有历史、分享、删除恢复、空行或特殊字段值读 `record-ops`。
- 普通字段 type 使用 `text/number/date/singleSelect/currency`；`singleSelect` 的 config 为 `{"options":[{"name":"<选项>"}]}`，人民币 `currency` 为 `{"currencyType":"CNY","formatter":"FLOAT_2"}`。
- 仅任务包含 4 个及以上独立业务步骤或用户明确要求时使用 TodoWrite；不按单条 CLI 拆步，只在阶段切换时更新。
- 多个资源名要求同一时间戳时，只取一次并复用。
- 复用 JSON 已返回字段，不以 `--verbose/raw/pretty` 重复请求。
- 数据源创建前必须先 `+datasource-list-sources` 获取 processCode 等透传字段，不凭记忆构造 sourceConfig。

## 记录稳定约束

- 记录分页报 `INVALID_CURSOR`、`CURSOR_SNAPSHOT_CHANGED` 或 `CURSOR_SNAPSHOT_UNAVAILABLE` 时，必须丢弃全部累计结果与旧 cursor，不存在可续传的新 cursor，也不能保留第一页再去重拼接。前两者核对条件后不传 `--cursor` 从第一页只读重查；UNAVAILABLE 需先等待服务修复。不得重跑含写入的整条命令。仅普通可恢复错误且实际提供有效断点时可续传，详见 [record-query](references/aitable/aitable-record-query.md)。
- 查询、写入、筛选或排序前，先用 `field get` 获取目标字段的 `fieldId`、`type` 和 `config`；`cells` 的 key 必须使用 `fieldId`，不是字段中文名。
- select/multipleSelect 写入传选项名称；过滤时先唯一解析 option ID。对 multipleSelect 或其他数组型字段，第二个 operand 必须是 option ID/稳定 ID 数组，不能传裸字符串。
- 人员、部门、群组和关联记录等条件先解析为稳定的结构化 ID；零命中、多命中或类型不符时停止，不得把展示名称或原值直接透传。
- 已获用户明确许可的非分析完整逐行明细可直接使用 `record query --all --page-limit 0`，不需要也不触发 psql 降级门禁；不得用它替代 psql 复杂分析，或在 Agent context、Python、jq、JavaScript、电子表格等本地工具做计算。自动翻页禁止模型手写循环；手动分页必须透传真实 `data.nextCursor`，且查询条件不变。成功空续页 `records=[]` 且 `nextCursor` 为空是正常末页，不得报错、重试或判定漏查。
- 新增或更新只使用真实返回的 ID 回读；写入效果未知时回读，不重放成功批次。
- 全量查询检查 `hasMore`，批量写检查最终状态；分页未结束或 `partial_success` 都不得声称完整完成。

- 记录 `cells` 使用当前 fieldId，按真实字段类型写值，只读字段不得写入。
- 新增或更新只使用真实返回的 ID 回读；写入效果未知时回读，不重放成功批次。
- 全量查询检查 `hasMore`，批量写检查最终状态；分页未结束或 `partial_success` 都不得声称完整完成。

## 安全边界

- 删除不可逆，按 Runtime confirmation 核对真实目标；`base list` 只是最近访问。字段零/多候选、类型不明时停止；多批写保留已完成批次和续跑位置。
- 评论 create/reply 非幂等，评论写操作未知状态时先 `comment list` 对账，不自动重放；delete 只删除本人评论且不可恢复，关联回复处理不作保证。
- `app get` / `app page list` 在 App 不存在时会初始化默认 App，属于幂等条件写；`app page/widget create` 非幂等且不自动重试。删除 Page 会级联删除全部 Widget，删除 Widget 会同步清理布局，均需独立确认。
- 数据源 `+datasource-create` / `+datasource-update` 会触发真实数据同步；执行前确认目标 Base 和 sourceConfig。`+datasource-sync` 单次最多 5 张表。

## 按需加载（复杂 JSON 与恢复语义）

Golden/次级直达覆盖时不读 Reference；否则按最终专有能力读取一个精确 Reference。读取后直接执行，不再读取其他 AITable Reference。

| 触发条件 | Reference |
|---|---|
| `+record-query`、upsert、bulk patch 的记录 filters/sort/date/AND/OR/比较操作符 | [filter-sort](references/aitable/aitable-filter-sort.md) |
| 记录历史、分享、删除恢复、空行或特殊字段值 | [record-ops](references/aitable-record-ops.md) |
| 记录分页错误、失效游标或快照恢复 | [record-query](references/aitable/aitable-record-query.md) |
| 记录统计、分组聚合或去重率 | [record-stats](references/aitable/aitable-record-stats.md) |
| 记录评论查询、创建、回复、更新或删除 | [comment](references/aitable/aitable-comment.md) |
| 查询记录的主键文档，或为记录创建主键文档 | 首次建表前读取 [primary-doc](references/aitable/aitable-primary-doc.md)；普通 Base/Table/字段/记录创建与导入不读取 |
| AI 字段、关联字段、lookup/filterUp 或其他复杂 config | [field](references/aitable/aitable-field.md) |
| formula 字段或公式语法 | [formula-guide](references/aitable/aitable-formula-guide.md) |
| 导入导出任务恢复 | [export-import](references/aitable/aitable-export-import.md) |
| 视图列顺序、移到/放到/固定在最左、filter、sort、group（不得转读记录 `filter-sort`） | [view-config](references/aitable/aitable-view-config.md) |
| Kanban/Gallery 的 card、Gantt 的 timebar、Grid 的 aggregate | [view-types](references/aitable/aitable-view-types.md) |
| 视图锁定、明确冻结前 N 列、行高或填色 | [view-extras](references/aitable/aitable-view-extras.md) |
| Base 内 Section/节点移动或清理 | [section](references/aitable-section.md) |
| Chart 创建或更新所需 config，或 Dashboard 完整 config/arrange；普通 Dashboard CRUD 走上方直达 | [dashboard-chart](references/aitable/aitable-dashboard-chart.md) |
| 表单创建、题目或分享 | [form](references/aitable/aitable-form.md) |
| 附件上传或移除 | [attachment](references/aitable/aitable-attachment.md) |
| 自动化工作流 | [workflow](references/aitable/aitable-workflow.md) |
| 普通角色或高级权限 | [advperm](references/aitable/aitable-advperm.md) |
| 数据源接入、同步管理、sourceConfig 构造或审批数据同步 | [datasource](references/aitable/aitable-datasource.md) |
| SQL、PostgreSQL、SELECT 或同 Base 多表 JOIN | [psql](references/aitable/aitable-psql.md) |
| 产品边界不明确 | [intent-guide](references/intent-guide.md) |
| 只有上述 Reference 仍无法定位的低频原子能力 | [aitable.md](references/aitable.md) 的对应章节 |

不要预加载这些 Reference。

## 错误最短路径

1. 零/多候选、字段歧义或分页不完整：停止并返回证据；仅有效分页会话可透传真实 `nextCursor`。失效游标先执行上方“记录稳定约束”，不能套用普通断点续传。
2. 类型错误只复核目标字段，不删字段或丢输入；`partial_success` 从 checkpoint 续跑，未知写入先回读。
3. 错误提供 `actions` / `available_flags` 时只按其中的 `next_command` 修正一次；`retryable=false` 或目标 ID 类型不符时停止。
4. 数据源同步 `errorCode=4014` 表示同步运行中重复触发，可稍后重试；非数据源表触发同步前先用 `+base-get` 确认 `sync=true`。

## 跨产品边界

- Excel 式单元格、区域和公式操作 → `dingtalk-misc` 的 Sheet。
- Base 作为整体在普通文件夹间移动或做外层存储重命名 → Drive；Base 结构复制/删除，以及 Base 内 Table、Dashboard、Section 的创建、复制、移动、重命名、删除 → AITable。
- 记录主键文档正文 → 取得真实 nodeId 后切 `dingtalk-doc`。
