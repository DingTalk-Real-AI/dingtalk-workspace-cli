# 应用事件定义配置

企业自建应用「产生什么事件」走 `dws dev event`。三条命令面不同层，不要混用：

| 命令面 | 管的事 |
|---|---|
| `dws dev event ...` | 应用产生的个人事件定义：草稿、校验、启用、下线（本文件） |
| `dws dev app event ...` | 应用接收哪些钉钉回调事件的订阅，见 [`event.md`](./event.md) |
| `dws event ...` | 个人与应用事件的发现、已发布 Schema 查询与实时消费，见 [应用事件参考](../../../dingtalk-event/references/event-app.md) |

## 最短命令路径

```bash
# 定位现有定义，拿 configRevision 与 status
dws dev event list --unified-app-id <id> --status DRAFT --page-size 20 --format json
dws dev event get --unified-app-id <id> --event-key <key> --format json

# 一份 JSON 文件定义一个事件；创建后 eventKey 不可改
dws dev event create --unified-app-id <id> --file ./definition.json --dry-run --format json

# 改草稿必须带从 get 读回的 configRevision
dws dev event update --unified-app-id <id> --event-key <key> --file ./definition.json --expected-revision <n> --dry-run --format json

dws dev event validate --unified-app-id <id> --event-key <key> --format json
dws dev event debug --unified-app-id <id> --event-key <key> --expected-revision <n> --timeout 120s --dry-run --format json
dws dev event publish  --unified-app-id <id> --event-key <key> --expected-revision <n> --dry-run --format json
dws dev event offline  --unified-app-id <id> --event-key <key> --expected-revision <n> --dry-run --format json
```

## 状态、快照与版本

| 状态 | Agent 应如何解释 |
|---|---|
| DRAFT | 新建草稿，无已发布定义；可受控 debug，产品允许对 experienceUids 体验人投递；全量开放须 publish，软开实现仍待办 |
| ONLINE | 已发布快照正式生效；update 只改草稿，不改变已发布快照；草稿可面向体验人软开，全量生效仍需 debug/publish |
| OFFLINE | 已下线；仍保留草稿、快照与订阅关系，不释放 eventKey。重新启用须 validate、debug、publish |

`get` 缺省返回 draft/published 两个视图；可用 `--view draft` 或 `--view published` 定位单一视图，结合 `hasDraftChanges`、`scopeReady` 与真实 status 判断，不把草稿当线上契约。

`configRevision` 是并发控制修订号，update/publish/offline 后取返回的新值；debug 不递增它，但草稿更新会清除旧调试通过标记。业务 Schema 版本 `payloadSchemaVersion` 与平台协议 `schemaVersion`（如 `v1`）是另一组概念，不用修订号替代业务版本，也不把两种版本写入业务 payloadSchema。消费 flatten 顶层使用 `payload_schema_version`；历史查询参数/信封使用 `payloadSchemaVersion`。应用事件用 `dws event schema <eventKey> --payload-schema-version <ver>` 查询指定已发布业务 Schema；省略则取当前生效版本。

publish 没有服务端人工审核环节，但写操作仍按本文件 dry-run/用户确认规则执行。必须针对当前修订完成 debug：`appAuthenticated`、`eventAccepted`、`subscriptionMatched`、`consumerReceived` 全部 true 且 `debugPassed=true`。没有阶段事实返回 `debug_not_ready`，阶段缺失/未通过返回 `debug_not_passed`；两者均不得当作通过或据此 publish。调试只验证临时订阅，不证明正式开放态。

### DRAFT 调试与体验投递

产品规则明确允许两类对象走 `dws dev event debug`：`status=DRAFT` 的事件，以及 ONLINE 事件上尚未 publish 的草稿业务版本。按当前真实 configRevision 冻结草稿，通过受控 debugId 关联调试会话和临时订阅，在该会话内校验真实上报与接收；不能用普通正式上报或正式 consume 代替草稿调试。debugId 属于受控会话机制，不代表 CLI 已暴露同名 flag，不自行编造参数。

debug 成功只证明该修订满足发布前的调试前置：不会使草稿自动 ONLINE，不会替换 ONLINE 事件的发布快照，也不豁免正式入口门禁。Agent 不得因草稿未 publish 或正式上报被拒就跳过 debug；正式入口的 `EVENT_NOT_AVAILABLE` / `460003` 不能当成受控调试失败证据。应独立读取本次 debug 的阶段事实与结果。

**debug 能力缺口**：当前分支有 CLI 命令与结果判定契约，但本 issue 尚无完整受控 debug 跑通证据；实现/环境待办是受控 debugId、临时订阅、冻结草稿及真实四阶段回执的端到端验证或补齐。服务端桩/依赖未就绪、未回报阶段事实时标记 `debug_not_ready`，不能假报已通，也不能改写为「DRAFT 禁止调试」。有阶段事实但未全部通过按 `debug_not_passed` 处理。本任务仅对齐 Skill，不实现上述缺口。

### 草稿体验软开与全量发布分界

尚未 publish 的草稿业务版本应允许投递给已配置的体验人员（experienceUids），包括 DRAFT 事件和 ONLINE 事件上的新草稿版本；全量正式开放仍要求已 publish 的 ONLINE 版本。不能将草稿版本未发布写成所有正式上报/投递一律拒绝。体验软开不自动 ONLINE、不替换全量发布快照，也不等于受控 debug 会话；DRAFT 可调试与体验人可投递是相容的产品规则。

历史版本 Schema 可读取与当前接收范围可投递是两件事，configRevision 不是 payloadSchemaVersion。应分别核对接收人是否属于真实 experienceUids、业务版本所属草稿/发布快照、环境与应用身份；查历史 Schema 用已支持的 `--payload-schema-version`，不自行编造体验人员或其他 CLI flag。

已有验收记录显示：v5 草稿体验投递可能被 `400 event.notAvailable`、`REJECTED / EVENT_NOT_AVAILABLE / 460003` 拒绝且没有对应 MQ send，这是**现状与产品预期不一致**，不是正确门禁。Agent 应报告体验/草稿软开异常，核查是否只走已发布定义匹配、是否遗漏体验人员与草稿路径；不能将自动 publish 或猜版本重试作为修复。全量路径仍核对已发布 ONLINE 版本；这些错误码不能单独证明事件不存在、已下线或请求版本未生效。正式/体验拒绝均不替代 debug 的独立阶段判定。

**实现侧软开仍待办**：补齐或修复草稿版本对体验人的匹配、接纳与下推，并验证接收范围隔离及全量发布边界。本次只改文档，不改 open-callback / 二方包，也不声称软开已可用。消费侧完整判读见 [草稿体验投递、全量开放与错误判读](../../../dingtalk-event/references/event-app.md#草稿体验投递全量开放与错误判读)。

## 事件建模

一个应用事件 = 「哪个业务系统、什么对象、发生了什么动作，并携带哪些业务数据」：

- **eventKey 三段式** `app_user_{业务系统}_{对象}_{动作}`：业务系统取应用/子系统简称（如 `hr`），对象取业务实体（如 `employee`），动作取发生的事情（如 `joined`）。粒度以「消费方拿到 payload 就能独立处理」为准，不要把多个动作合并进一个事件；需要另一动作就再建一个事件。
- **payloadSchema 只建模业务 Payload**：平台信封（eventId、occurredAt、tenant、source、audit、delivery 等）由平台生成，不要写进 Schema。字段从消费方视角命名与定类型（string/number/boolean/object/array），必填字段写入 `required`，可缺省字段不要放进 `required`；后续只做兼容变更（新增可选字段），破坏性变更换新 eventKey。
- **samplePayload 是脱敏示例**：字段名与类型必须和 payloadSchema 一致（`validate` 会逐字段比对），数值给数值、布尔给布尔，不要放真实用户数据。

### payloadSchema 配置说明（对照服务端校验器）

`payloadSchema` 是**业务 Payload 的 JSON Schema**，服务端按受控子集逐项校验（与 `validate`、推送入队前校验同一套规则）。只定义业务字段——信封（eventId、occurredAt、tenant、subject、source、audit）由平台生成，不要建模进 Schema。

**受控关键字（白名单，未知关键字一律拒绝，不做"忽略后通过"）：**

| 关键字 | 语义 | 备注 |
|---|---|---|
| `type` | 类型 | 必读；见下方受控类型 |
| `properties` | 字段集合 | 仅 `type=object` 允许声明 |
| `required` | 必填字段 | 独立生效：声明在 properties 中不自动成为必填；引用未声明字段会被拒（悬空 required） |
| `additionalProperties` | 额外字段 | `true` 允许 / `false` 出现即校验失败 / 缺省允许；只作用于当前对象层，不向嵌套递归 |
| `items` | 数组元素 Schema | 仅 `type=array` 允许声明；必须是对象（不支持元组形式） |
| `enum` | 枚举取值 | 标准语义 |
| `description` | 字段说明 | 建议每个业务字段都写，消费侧依赖它理解语义 |
| `title` | 字段标题 | 可选 |
| `$schema` | 方言声明 | 可选；携带时须与平台方言一致 |

**受控类型（7 种）：** `object`、`array`、`string`、`number`、`integer`、`boolean`、`null`。

**硬性约束：**

- 根类型固定 `object`（`type: "object"`），数组或标量作根会被拒。
- 禁止远程引用：`$ref` 不在受控关键字内，不下载外部 Schema。
- 嵌套深度 ≤ 16 层；JSON 文本 ≤ 64KB；不做静默截断。
- 每个字段建议都写 `description`——消费 Agent 靠它理解业务语义。

**样例对齐（samplePayload）：**

- 字段名、类型、嵌套结构必须与 `payloadSchema` 逐字段一致：`number` 字段给 `12.34` 而不是 `"12.34"`；`required` 中的字段一个不能少（缺失报 `$: 未找到所需属性`）；出现 Schema 未声明的字段报 `未在 properties 中声明`。
- 数值/布尔务必是 JSON 原生类型——通过 MCP 工具对象形态传参时平台会把标量降级为字符串，导致样例比对失败，**payloadSchema/samplePayload 建议以 JSON 文本字符串传入**（CLI `--file` 方式天然保真）。

**兼容性：** 发布后 Schema 只允许兼容变更（新增可选字段、放宽 enum 等，破坏性变更返回 `SCHEMA_INCOMPATIBLE`）；`additionalProperties: false` 时新增字段也需评估存量消费方。

### 定义文件完整样例

```json
{
  "eventKey": "app_user_hr_employee_joined",
  "name": "员工入职",
  "description": "新员工完成入职手续后触发，携带工号与部门",
  "payloadSchema": {
    "type": "object",
    "properties": {
      "employeeId": {"type": "string", "description": "工号"},
      "department": {"type": "string", "description": "入职部门"}
    },
    "required": ["employeeId"]
  },
  "samplePayload": {
    "employeeId": "E1001",
    "department": "技术部"
  }
}
```

## 创建流程

1. 写定义文件（如 `./event.json`，内容为上面的完整样例）。
2. 预检：`dws dev event create --unified-app-id <id> --file ./event.json --dry-run --format json`，确认解析出的字段无误。
3. 创建：同一命令去掉 `--dry-run` 加 `--yes`，得到 `configRevision = 1` 的 DRAFT 草稿。
4. 校验：`dws dev event validate --unified-app-id <id> --event-key <key> --format json`，`issues` 为空才算过。
5. 修正（如需）：改文件后 `dws dev event update --unified-app-id <id> --event-key <key> --file ./event.json --expected-revision 1`，修订号随之递增。
6. 受控调试：`dws dev event debug --unified-app-id <id> --event-key <key> --expected-revision <n> --timeout 120s --dry-run --format json`；按预览确认后执行，仅四阶段与 debugPassed 全通过才继续，草稿未改时 revision 不变。
7. 上线：`dws dev event publish --unified-app-id <id> --event-key <key> --expected-revision <n>`；状态变 ONLINE 后事件才可被订阅与消费。
8. 验证消费：`dws event list --category appEvent` 应出现该事件；业务系统真实推送一条后，`dws event consume <key> --flatten -f ndjson` 能收到 `payload` 与样例同构的数据。
9. 下线：`dws dev event offline --unified-app-id <id> --event-key <key> --expected-revision <n>`。下线不释放 eventKey；要换事件语义请新建。

## 定义文件规则

CLI 只读 `--file`，任何情况下都不改写它。文件顶层只接受 `eventKey`、`name`、`description`、`payloadSchema`、`samplePayload` 五个键；`category` 本轮不进入事件定义输入契约。

1. `eventKey` 必须是文件顶层的非空字符串，形如 `^app_user_[a-z][a-z0-9]*(?:_[a-z0-9]+){2,}$`，不超过 128 字符，格式为 `app_user_{业务系统}_{对象}_{动作}`，例如 `app_user_hr_employee_joined`。它同时用作 MQ Tag，创建后固定，`update` 不能改名。
2. `payloadSchema` 必填，必须是 JSON 对象且 `payloadSchema.type` 固定为 `"object"`；业务字段写在 `payloadSchema.properties` 下。
3. `samplePayload` 可选，必须是脱敏后的 JSON 对象。要省略就删掉这个键——写 `null` 会被本地预检拒绝，因为 `null` 会原样下发并可能清空服务端已保存的样例。
4. Scope、启停状态、版本与修订号由平台按规则赋值。文件里出现 `scope`、`scopes`、`defaultScope`、`status`、`version`、`configRevision`、`expectedConfigRevision`、`debugPassedRevision`、`unifiedAppId`、`published` 中任何一个都会被本地拒绝，删掉再试。
5. Scope 的赋值规则：**scope = 生产应用的 unifiedAppId 本身**（契约统一；历史事件可能为 `dws_app_event_<id>` 旧值）。消费侧把 scope 当元数据透出，不代表已授权。

本地预检只做快速失败。格式、保留名与全局唯一性的权威校验在平台侧，Schema 符合性判断是 `validate` 的职责。

## 消息通道约定

应用事件接入与下游推送使用独立 Topic，名称不因消费服务所在工程而变化：

| 用途 | Topic |
|---|---|
| 一方事件接入 | 平台官方事件接入通道 |
| 三方应用事件接入 | 应用事件接入通道 |
| 下游个人事件推送 | 个人事件推送通道 |

应用事件的 MQ Tag 与 `eventKey` 完全一致，不做点号、前缀或其他转换；一方事件使用 `user_` 前缀，三方应用事件使用 `app_user_` 前缀。

## 闭环规则

1. 除 create 外，update/debug/publish/offline 之前先用 `get` 读回真实的 `configRevision`，原样传给 `--expected-revision`。它必须 ≥ 1：CLI 不用默认修订号代替，缺失或为 0 直接报错。
2. 最初请求只允许 dry-run；展示预检返回的应用 ID、动作、完整定义内容与影响并取得用户对该预览的明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`。参数变化就重新预检并确认，确认前不得真实写入。dry-run 不跳过本地文件预检。
3. 返回 `409` / `CONFIG_REVISION_CONFLICT` 表示草稿已被别处改动。这不是网络抖动，CLI 不会自动换修订号重放：重新 `get` 取最新草稿，对比并合并你的修改后，用返回的真实 `configRevision` 再提交。错误的 `details` 里同时给出 `expectedConfigRevision` 与 `currentConfigRevision`，先对比再动手。
4. `publish` 前的生效校验由服务端内置执行：草稿不合法（如样例与 Schema 不符）时返回 `APP_USER_EVENT_META_VALIDATE_FAILED`，先 `validate` 修复，再针对最新 `configRevision` 完成 debug；草稿改动后不能复用旧调试结论。
5. `eventKey` 全平台唯一且不因下线释放。撞名（`EVENT_KEY_CONFLICT`）时换一个新的 `eventKey` 重新创建，不要覆盖已有定义。
6. 生效后的 Schema 只允许兼容变更（`SCHEMA_INCOMPATIBLE`）；破坏性变化请使用新的 `eventKey`。
7. 列表读取 `meta.pagination`：`endpoint_exhausted=false` 时把原样 `next_token` 传给下一次 `--cursor`，到 true 停止；CLI 不支持 `--page/--page-num`。`list` 缺分页事实会直接失败而不是返回半页结果，此时保留原始响应并停止翻页。

## 权限与身份

只认 `--unified-app-id`，不接受 ClientId 或 AppKey 替代。返回 `APP_MANAGEMENT_FORBIDDEN` 表示当前身份没有该应用的管理权：用 `dws dev app member list` 确认成员角色，或改用有管理权的 `--profile`。

已知路径直接执行；仅 flag 确有疑问时查一次精确 leaf compact Schema（`dws schema --cli-path "dev event create" --compact --format json`），Schema 不可用才查一次 leaf help。
