# 日志（Report）

> 本文件是 Report 已知任务的唯一必读 reference，覆盖模板、收件箱、发件箱、详情、统计、提交与验证。不要再预读 `dingtalk-shared`、Report intent/lite/conventions 或父级 Help。

> **审批边界**：请求中出现审批人、抄送人、审批路径或审批单时，立即停止 Report 路径并改读 [oa.md](oa.md)，不得执行任何 `dws report` 命令。Report 的 `--to-user-ids` 表示日志收件人，不是审批人或抄送人；OA 没有同名模板也不能用 Report 替代。

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只用 `dws`；结构化读取加 `--format json`，按真实返回判断。
- 已知命令直调；参数/约束/安全不明查 leaf 窄 Schema。Schema 不可用才读已知 leaf Help 一次；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 不查 Help：优先错误中的明确 suggestion，其次已加载 Skill/reference 中的明确兼容入口；均无则报漂移并停，禁全 Catalog。低频 reference 不默认 Help，禁 root/parent/product Help。发现后必须执行或说明阻塞。
- 不猜命令/flag/字段/ID/账号/业务事实；ID 来自真实返回。目标零命中/多候选/类型不明先消歧；仅可选时间/展示范围用契约默认，缺必需信息即停。
- 解析/读/写同一 profile，ID 不跨组织。多账号只用唯一 `isOrgCurrent=true`；否则用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出/记录 token、refresh token、appSecret、webhook token；已注入认证时不索要。
- 写须符合明确意图；确认以最终 Runtime gate/Schema 为准，确认后才加 `--yes`。
- 写后验证结果，不凭退出码宣称成功。退出须最终答复，区分完成、部分、阻塞、待确认、失败；保留已有数据及 `complete/hasMore/stopReason/failures`。
- 时间戳按会话时区展示，必要时保留原值。
- 认证/权限/profile/confirmation/未知错误只读 `dingtalk-shared` 对应 reference，禁连续猜替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

Report 查询优先使用下方严格 Shortcut；它们会校验响应、稳定 ID、分页游标和时间窗。提交后用返回的 `reportId` 最小读回，失败或部分结果不得包装成成功。

## 产品边界

| 用户目标 | 正确产品 | 不要做 |
|---|---|---|
| 日报、周报、月报、日志模板、我收到/发出的日志 | `dws report` | 不要切到在线文档、邮件、聊天、Wiki、AITable 或全局搜索寻找“可能的日志” |
| 在线文档里的周报模板 | `dws doc` | 不要当成 Report 日志模板 |
| 用户明确要求转发到聊天 | Report 提交后再按用户指定目标协作 | 不要把“提交日志”默认解释成发消息 |

在明确的 Report 时间窗内返回空列表，表示该范围内没有可用结果。应如实报告并停止；除非用户明确要求扩大范围或跨产品搜索，否则不要自行探测其它产品或旧文件。

## Golden Routes

| 意图 | 首选命令 | 关键约束 |
|---|---|---|
| 列出收到的日志 | `dws report +inbox-list --start <ISO> --end <ISO> --cursor 0 --size 20 --format json` | 从返回的 `reports[]` 使用稳定 `reportId`；模板名筛选在当前返回页本地完成 |
| 按发件人列出收到的日志 | 先 `dws aisearch person --query "<姓名>" --dimension name --format json`，再 `dws report +inbox-list --start <ISO> --end <ISO> --sender-user-ids <USER_ID> --cursor 0 --size 20 --format json` | 只过滤当前 profile 的收件箱；人员零命中或多候选时停止并消歧，禁止默认选择第一项或改查他人的发件箱 |
| 列出自己发出的日志 | `dws report +outbox-list --start <ISO> --end <ISO> --template-name <NAME> --cursor 0 --size 20 --format json` | 创建/修改时间窗最多 20 天；模板明确时服务端过滤 |
| 自己最近一篇日志详情 | `dws report +report-latest --format json` | 需要模板关键词时加 `--keyword <TEXT>` |
| 搜索模板 | `dws report +template-search --query <TEXT> --format json` | 返回当前用户模板中的匹配项和稳定 `templateId` |
| 完整列出可用模板 | `dws report template list --format json` | 只投影名称/ID再输出，避免大对象导致截断；“全部”必须有完整性证据 |
| 读取模板字段 | `dws report template get --name <EXACT_NAME> --format json` | 先确认名称唯一，不猜字段 |
| 读取单篇正文 | `dws report entry get --report-id <REPORT_ID> --format json` | ID 必须来自本任务内同 profile 的列表或提交结果 |
| 读取已读统计 | `dws report entry stats --report-id <REPORT_ID> --format json` | 不用标题代替 ID |
| 提交一篇日志 | `dws report entry submit --template-id <TEMPLATE_ID> --contents - --to-user-ids <USER_ID[,USER_ID...]> --format json` | 先读取模板字段并解析至少一个明确收件人；`--to-user-ids` 必填，禁止空值或猜测 |

对已经由本文件定位的命令，不要再执行 `help` 或 `shortcut list`。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。按本 skill/recipe 路由，命中时 Shortcut 优先于原子命令。参数只查 `dws schema --cli-path "report +<shortcut>" --compact --jq '{cli_path,parameters,constraints,confirmation}' -f json`；仅需且已发布 `result` 时查 `--jq '{cli_path,outcomes:.result.outcomes,pagination}'`，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；仅映射、接口或 provenance 审计省略 `--compact`。现有路由和 reference 均无法定位低频能力时，才用 `dws shortcut list --service report --format json` 发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws report +inbox-list` | read | 列出我收到的日志 |
| `dws report +outbox-list` | read | 列出我发出的日志 |
| `dws report +report-latest` | read | 读取我最近提交的一篇日志详情 |
| `dws report +template-search` | read | 按名称搜索可用日志模板 |
<!-- VISIBLE_SHORTCUTS_END -->

## 收件箱：范围、筛选与分页

1. 把“最近 N 天”“今天”等自然语言按 `Asia/Shanghai` 转成带 `+08:00` 的 ISO-8601 起止时间。
2. 按发件人筛选时，先用 `aisearch person` 在同一 profile 下解析稳定 `userId/staffId`，再把唯一 ID 传给 `--sender-user-ids`；零命中、多候选或身份不完整时停止并消歧，禁止选择第一项。
3. 每页 `--size` 最大为 20；默认使用 20。若用户要“全部”，只沿当前响应中真实存在且严格前进的 continuation cursor 翻页，直到响应证明 endpoint exhausted；禁止用 50/100 绕过分页。
4. Shortcut 没有模板 flag。按 `templateName` 在每个已返回页本地筛选，并继续翻真实续页；不要因为第一页没有目标模板就跨产品搜索。
5. “最近一篇/两篇”先在限定范围内收集匹配项，再按真实 `createTime` 排序，最后对选中的 `reportId` 调 `entry get`。不要用列表摘要冒充正文。
6. 返回零条或服务端已穷尽时停止。只有用户明确要求，才扩大时间窗；扩大后仍使用 Report 收件箱。

### 今日/最近收到的日志摘要脚本

用户只需要今天或最近几天的收件箱摘要时，可执行 [`report_received_today.py`](../scripts/report_received_today.py)：`python3 scripts/report_received_today.py --days <N>`。脚本使用 `+inbox-list`，最多扫描 10 页、200 条并受总超时约束；命令失败、响应不完整或达到上限时返回非零状态，不得解释成空结果。脚本不读取每篇正文；用户需要正文时，从摘要中选择明确的 `reportId` 后只调用一次 `entry get`。

## 模板列表与比较

- 用户要查看当前全部模板时，调用一次 `template list`，优先加 `--jq '[.items[] | {name: .report_template_name, templateId: .report_template_id}]'` 仅保留名称和 ID，降低输出体积。
- 如果输出被截断、分页状态未知或工具没有给出完整性证据，不得声称“共 N 个且已全部列出”；应说明已取得的范围并继续取得完整结果。
- 比较两个模板字段时：模板列表只取一次；确认两个精确名称后，两次 `template get` 可并行；最终按字段名、字段类型、必填/选项（若响应提供）比较。
- `template get` 没返回的属性就是未知，不自行推断“必填”“默认值”或提交格式。

## 提交闭环

1. 用 `+template-search` 或一次 `template list` 唯一定位模板；有重名或近似名时先消歧。
2. 用 `template get --name <EXACT_NAME>` 读取字段定义，按返回顺序和字段名构造 `contents`；不要猜键名。
3. 解析用户明确指定的收件人，并在同一 profile 下取得至少一个真实 `userId`；零命中或多候选时先消歧，禁止把姓名、手机号或猜测值直接当成 `userId`。
4. 首选 `--contents -` 从 stdin 传入 JSON。需要文件时，只使用当前工作目录内的相对路径，例如 `--contents-file ./report.json`；不要传工作区外 `/tmp/...` 等绝对路径。
5. 调 `entry submit --to-user-ids <USER_ID[,USER_ID...]>`，记录返回的 `reportId` 和成功状态。该 flag 必填且不能为空：无收件人的请求即使服务端返回成功，日志也对任何人不可见；普通创建不额外加确认 flag。
6. 用返回的 `reportId` 调一次 `entry get` 验证模板、字段和值；若还需证明它出现在发件箱，再用窄时间窗的 `+outbox-list`，不要扫描无关产品。

`contents` 必须是 JSON 数组，每项包含 `key`、`sort`、`content`、`contentType`、`type`，并与模板实际字段一致；编码后的 JSON 上限为 10MB，超出时精简内容或拆成多篇独立日志，不能把一次提交拆成多个片段。内容来自用户提供或可直接推导的事实；缺失业务内容时先向用户确认，不编造日报正文。

## 结果与断言

| 请求 | 最小成功证据 |
|---|---|
| 模板列表 | 工具成功；若声称“全部”，还要有未截断/已穷尽证据；名称逐项真实返回 |
| 日志列表 | `success=true`，范围正确，返回项含稳定 `reportId`，续页状态真实 |
| 详情/统计 | 返回的 `reportId` 与请求一致，目标字段存在 |
| 提交 | 提交成功且返回稳定 ID；读回的模板和关键字段与预期一致 |

最终答复优先给用户要求的名称、发送人、时间、字段、统计或链接，不倾倒原始 JSON。空结果、截断、权限不足、profile 不匹配都应明确说明，不能补写成业务成功。

## 最短错误恢复

- `validation_error`：只修正报错指出的时间、分页或必填参数后重试一次；缺失或空白 `--to-user-ids` 时先取得明确收件人的真实 `userId`，不要填占位值绕过校验。
- `not_found`：检查本任务取得的稳定 ID 和 profile；不要跨产品猜目标。
- `permission_denied` / `auth_required`：停止业务重试，报告所需权限或登录状态。
- 响应结构或 flag 漂移：先读该精确 leaf 的 compact Schema；只有仍显示 Cobra 不匹配时再读同一 leaf Help。
- 不明错误：保留原始错误上下文，不通过扩大搜索范围掩盖失败。
