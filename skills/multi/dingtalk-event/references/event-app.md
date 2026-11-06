# 应用事件发现与消费

本参考描述应用事件相关的 Agent 可见 CLI 契约；命令已声明不等于当前环境已部署，也不等于某个应用事件已上线或当前用户已授权。

## 发现 → Schema → consume

```bash
dws event list --category appEvent --format json
dws event list --category appEvent --producer-app-id <producerUnifiedAppId> --format json
dws event list --category appEvent --keyword <完整eventKey> --format json
dws event schema <event_key> --flatten --format json
dws event consume <event_key> --flatten --duration 2m -f ndjson
```

- 官方事件来自内置目录，官方分类可离线查询；应用事件登录后从远端目录读取，目录由 CLI 自动分页。`--keyword` 是 eventKey 全值等值匹配（忽略大小写），不是名称、前缀或模糊搜索。
- 未登录时 stderr 提示应用目录不可用，只返回内置官方项；无筛选浏览遇到远端故障也可能降级。空目录或仅有官方项不能证明没有应用事件。带应用筛选的远端请求失败须保留错误，不当作完整空结果。
- `--include-pending` / `--enabled-only` 作用于内置官方目录，不是应用定义的 DRAFT / ONLINE / OFFLINE 筛选。应用定义状态从 `dws dev event get/list` 读取。
- 读取返回的真实 eventKey、`producer_unified_app_id` 与 `scope`。scope 是元数据，不表示消费授权；定义存在也不表示事件已正式生效。不要把内置目录外任意字符串都当作可用事件。
- 应用事件 `event schema` 登录后读已发布的业务 `payloadSchema`，返回 `jq_root_path=".payload"`。`--flatten` 仅为调用一致性保留，不会把应用业务字段提升到顶层，也不会返回完整投递信封 Schema。
- 应用事件 consume 支持同一入口的单事件/多事件消费，采用 `ruleType=all`、空 filterRule（CLI 不传该字段，不手填 `{}`）。不接受 `--user`、`--open-dingtalk-id`、`--group`、`--role-types`、`--query`、`--filter-json`。沿用同一 profile，等 stderr ready，再逐行读 stdout；ready 只证明订阅就绪，收到业务事件才证明本次消费成功。

## flatten 与版本命名

有业务版本时应用事件输出 6 个顶层字段：`type`、`event_id`、`timestamp`、`subscribe_id`、`payload`、`payload_schema_version`。版本缺失/为空时 `omitempty` 可能省略最后一个字段，仍为 5 个；不要据此推断默认版本。业务 payload 保留其原始字段名，不统一转换为 snake_case。

| 位置与名称 | 含义 |
|---|---|
| flatten 顶层 `payload_schema_version` | 业务 Schema 版本，如字符串 `"4"` |
| 原始信封 / 历史 Schema 查询参数 `payloadSchemaVersion` | 同一个业务版本，使用 camelCase |
| 原始信封 `schemaVersion` | 平台协议版本，如 `"v1"`；flatten 不展示 |
| 配置 `configRevision` | 草稿并发控制修订号，用于 `--expected-revision`，不能当成业务版本 |

一行 flatten NDJSON 示例（ID、时间和 payload 为示意占位）：

```json
{"type":"app_user_hr_employee_joined","event_id":"<event_id>","timestamp":0,"subscribe_id":"<subscribe_id>","payload":{"employeeId":"E1001"},"payload_schema_version":"4"}
```

直接读取 `.payload_schema_version` 与 `.payload.employeeId`，flatten 不再解析 `.data`。原始非 flatten 的字符串 data 信封才需按其格式解码。

历史版本查询使用 camelCase 参数 `payloadSchemaVersion`，取值来自 `.payload_schema_version`。应用事件用 `dws event schema <event_key> --payload-schema-version <ver>` 透传到 portal `GET /dws/event/schema`；省略该 flag 时查询当前生效已发布版本。响应含 `payload_schema_version`（服务端回填的业务版本，入参为空时才是当前生效值）。CLI 不做本地合法性校验；`DRAFT`、超长或从未发布的版本由服务端分别返回 `ILLEGAL_PARAMETER` / `SCHEMA_VERSION_NOT_FOUND` 且不回退最新版。官方内置事件不接受该 flag。事件版本与当前已发布 Schema 不一致时，保留版本值，用匹配历史契约后再解释业务字段。

## DRAFT 调试与体验投递

`status=DRAFT` 的事件，以及 ONLINE 事件上尚未 publish 的草稿业务版本，都允许走 `dws dev event debug`。受控调试用 debugId 关联会话、临时订阅与冻结的当前修订草稿；普通正式上报/consume 不能替代此通道。debugId 是受控机制描述，不自行编造同名 CLI flag。

调试成功只证明该修订满足发布前的调试前置，不使草稿自动 ONLINE、不替换发布快照、不豁免正式入口门禁。Agent 不得因正式上报的 `EVENT_NOT_AVAILABLE` / `event.notAvailable` / `460003` 就跳过 debug，也不得把正式拒绝当成调试失败；debug 结果必须由本次受控会话的阶段事实单独判断。

**debug 能力缺口**：当前分支已声明 CLI/契约，但本 issue 未取得完整端到端 debug 通过证据；受控 debugId、临时订阅、冻结草稿与真实四阶段回执的跑通验证/实现补齐仍待办。服务端桩或依赖未就绪、无阶段事实时标为 `debug_not_ready`，不是「DRAFT 不允许调试」；有阶段事实但未全部通过为 `debug_not_passed`，不能假报已通。配置细节见 [应用事件定义](../../dingtalk-misc/references/dev/event-definition.md#draft-调试与体验投递)。

## 草稿体验投递、全量开放与错误判读

产品分界：面向已配置体验人员（`experienceUids`）时，允许投递尚未 publish 的草稿业务版本，属于受控软开；全量正式开放仍要求已 publish 的 ONLINE 版本。不能把「草稿未发布」概括成所有上报/投递路径一律拒绝。ONLINE 事件上新增的未发布草稿版本同样适用体验人路径，但不会自动成为全量发布快照。体验投递与受控 debug 都允许使用草稿，各自的授权范围与会话机制仍须核对，不能互相替代。

历史 Schema 可查询只表示可读取契约，不证明某个接收人在当前环境已满足体验投递或全量开放条件。configRevision 不是业务版本，`experienceUids` 是产品接收范围描述；当前文档不据此编造 CLI 参数或声称实现已具备软开能力。

已有验收记录显示：草稿 `payloadSchemaVersion=v5` 未 publish、面向体验人员时，可能返回 `400 event.notAvailable`「找不到对应版本的事件定义」，并出现 `REJECTED / EVENT_NOT_AVAILABLE / 460003`。按产品预期，这类入队前拒绝属于**实现异常或缺口**；单独缺少 MQ 成功日志不能证明无入队。

Agent 必须结合投递路径判断 `EVENT_NOT_AVAILABLE` / `event.notAvailable` / `460003`：

1. 保留真实 eventKey、应用、环境、请求 payloadSchemaVersion、接收人范围和关联回执/日志，先区分体验投递、全量正式开放与受控 debug。
2. 在「草稿未发布 + 体验投递」场景，拒绝不应出现，报告「体验/草稿软开路径异常或实现缺口」。核对真实体验人员配置及匹配事实，排查是否仍只读取已发布定义、是否未进入草稿体验路径；这些是排查方向，不是已证实根因。
3. 不把去 publish、改用猜测版本或停止体验流程作为产品修复，不把拒绝当作门禁成功；原样重放也不会证明软开实现正确。保留复现材料，待实现路径补齐后按授权复验，不自动改配置或发布。
4. 对全量正式路径仍核对已发布 ONLINE 版本；有管理权时回读 `dws dev event get` 的 status、draft/published 与对应版本事实。错误码本身不能区分事件不存在、OFFLINE、版本不匹配或路径实现缺口；查询无权/失败/目录为空不能证明不存在。
5. 正式或体验上报的拒绝不能当成 debug 失败，也不能据此跳过 debug。调试结果独立读取本次受控会话阶段事实；无阶段事实为 debug_not_ready。

**实现侧软开仍待办**：草稿业务版本按 experienceUids 匹配体验人并下推的路径需实现补齐或缺陷修复，随后验证体验人能收到、非体验人不能因此获得全量开放、全量仍按已发布版本生效。本次仅修 Skill/docs，不改 open-callback 或二方包，不宣称已跑通。

## 安全边界

- 应用事件只能使用登录后远端目录返回的真实 `eventKey`、`producer_unified_app_id` 和 `scope`；不把任意字符串当作可消费事件，也不把定义存在、目录命中或 `scope` 元数据当作消费授权。
- 目录查询、Schema 查询与 consume 使用同一 `--profile` 和组织身份；零命中、候选不唯一、应用筛选请求失败或目录不完整时停止，不降级为“事件不存在”。
- consume 只允许应用事件规定的 `ruleType=all` 与空 filterRule；不拼接用户、群组、角色或查询过滤条件。`ready` 只证明订阅建立，不证明收到业务事件。
- DRAFT、ONLINE 草稿版本、publish、offline 和 debug 的状态必须按真实 `status`、`configRevision`、发布快照及阶段回执分别判断；Schema 可读不等于事件已上线或当前环境可投递。
- 本参考只描述发现、Schema 和消费契约，不自动执行事件写入、调试、发布、下线或真实业务推送；需要写操作时转到应用事件定义流程，并按其确认和回读规则执行。

## 开放态与验收

| 能力 | 当前分支契约与验收依据 |
|---|---|
| 应用目录、已发布 Schema、订阅与 flatten | CLI 已接入；环境验收分别看远端真实目录项、`.payload` Schema、ready，以及实际 NDJSON 中业务字段/版本 |
| 定义 list/get/create/update/validate/offline | `dws dev event` 已声明；草稿与发布快照分开，管理权由服务端判定 |
| debug / publish | CLI 已声明受控调试与上线契约；仅四个真实阶段全部 true 且 `debugPassed=true` 才能继续。无阶段事实为 `debug_not_ready`，不能宣称已开放可跑通 |
| DRAFT 及 ONLINE 上的未发布草稿版本 | 产品允许受控 debug 与体验人投递；不自动全量开放。debug 跑通与体验软开实现仍待办 |
| OFFLINE、内置 pending/internal 项 | 不作为正式可消费承诺 |
| 指定历史业务版本 Schema 查询 | camelCase 参数约定已知，当前消费 CLI 尚无选择入口，仍待补齐 |

本次文档 review 不执行事件写入、调试、上线或真实业务推送；具体环境的 debug 依赖、publish 生效、scope 授权与端到端投递仍需用真实对象验收，不能从代码声明推导已部署。

配置状态、受控调试与发布快照见 [应用事件定义](../../dingtalk-misc/references/dev/event-definition.md)；接收钉钉回调的应用订阅是另一条 `dws dev app event` 命令面。
