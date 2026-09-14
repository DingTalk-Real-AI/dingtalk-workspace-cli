# 数字员工服务端设备绑定

基于本地 Agent Adapter 与 connect 生命周期实现，补齐服务端设备绑定；不新增在线查询、心跳、Presence 或跨机器进程控制。

## 指令

| 场景 | 指令 | 服务端动作 |
|---|---|---|
| 首次接入 | `dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex` | 本地预检和换票后 bind，保存回执后接入 |
| 旧版连接补登记 | `dws dingtalk-tag connect bind --agent-uuid <agentUuid>` | bind；不换票、不启动或停止 Agent |
| 同设备换 Agent | `dws dingtalk-tag connect rebind --agent-uuid <agentUuid> --channel qoder` | 保留设备绑定 ID；复用本地生命周期切换 |
| 更换设备标识 | `dws dingtalk-tag connect rebind --agent-uuid <agentUuid> --channel qoder --device-id <newDeviceId>` | 原子 rebind，保存新的绑定 ID 后接入 |
| 新机器接管 | `dws dingtalk-tag connect rebind --agent-uuid <agentUuid> --runtime-binding-id <oldBindingId> --channel codex` | 校验已发布员工、换票，再原子 rebind；不先 unbind |
| 解绑 | `dws dingtalk-tag connect unbind --agent-uuid <agentUuid>` | 旧实例停止、确认释放后 unbind；保留 Profile 和历史绑定 ID |
| 暂停 / 恢复 | `dws dingtalk-tag connect stop/restart --agent-uuid <agentUuid>` | 不新增服务端绑定，不换票 |

表中写操作均先添加 `--dry-run --format json` 预览，经用户确认后再加 `--yes` 执行。需要普通 Agent 后台常驻时添加 `--daemon --alwayson`；DSH 沿用外部宿主管理，不加这两个参数。

新机器接管前，由操作人先在旧机器执行 `connect stop` 并确认旧进程已释放，再从旧机器的 `connect status --format json` 获取 `data.runtimeBindingId`。新机器不能通过这些 MCP 远程停止旧进程；服务端没有在途任务也不等于旧进程已停止。机器失联时需要服务端/宿主协同处置，不能把换绑当作远程强杀或在线证明。

已有本地记录时，`--runtime-binding-id` 是精确前置条件，必须与记录一致，不能用于覆盖另一个绑定。新机器没有本地记录时，该参数必填。同一设备只改 Agent 类型，不调用要求“新设备”的 rebind，也不更新服务端 `localAgentName/extensions`；传入这两个参数会明确拒绝，避免静默忽略。

## 三类标识

- `deviceId`：稳定设备标识。缺省在当前 DWS 配置目录的 `digital-employee-device/identity.json` 随机生成一次，权限 0600；并发生成受锁保护。不是 PID、主机名或每次启动的随机数。已有绑定继续使用保存的设备 ID。显式 `--device-id` 只作用于该绑定，不改其他员工的默认设备标识。不要把设备标识文件和绑定配置复制到另一台机器。
- `runtimeBindingId`：服务端 bind/rebind 返回的绑定关系 ID。rebind 必须使用旧 ID，并保存返回的新 ID；不是本地运行实例 ID。解绑后保留原 ID 作为历史回执，重复本机解绑不会操作后继绑定。
- `bindingRevision` / `runtimeInstanceId`：既有本地绑定代数与运行实例，职责不变。服务端 ID 不替代现有宿主校验。

`connect status/list` 增加 `deviceId`、`runtimeBindingId`、`serverBindingState`。这些是本地保存的最后回执，不执行服务端绑定查询，更不代表设备在线；别的机器可能已经换绑。本次不改变原有 `runtimeState` 的来源。

`serverBindingState` 为 `bound`、`unbound`、`unregistered`（本地没有服务端回执，并非证明服务端不存在绑定）、`unknown` 或 `commit_pending`。

## MCP 契约

固定使用 `deap-dev`，按平台 Schema 将业务字段直接放在 MCP `tools/call.arguments` 顶层，不增加 DTO 名称包装：

| MCP | 顶层业务字段 | success=true 时的返回值 |
|---|---|---|
| `bind_local_agent` | agentUuid、deviceId；可选 localAgentName、extensions | data 为绑定对象，读取 data.runtimeBindingId 非空字符串 |
| `unbind_local_agent` | agentUuid、runtimeBindingId | data 为布尔值 true |
| `rebind_local_agent` | agentUuid、runtimeBindingId、deviceId；localAgentName 见下文配置差异，可选 extensions | data 为绑定对象，读取 data.runtimeBindingId 新绑定 ID |

`identity` 由网关根据主管登录态注入，CLI 不暴露或传入 userId/orgId/identity。请求使用明确主管 Profile 的进程内 Token，不使用刚换取的员工 Token，不改变当前 Profile。服务端负责权限、旧绑定版本和在途/待恢复任务校验。

`--extensions` 应传 JSON 对象序列化后的字符串，例如 `"{}"`；不自动展开为对象，空字符串、数组或任意非 JSON 文本不符合服务端约定。省略时服务端使用 `"{}"`，rebind 不自动保留旧扩展信息。CLI 将非空值原样传入，格式合法性由服务端校验；不在本地扩展或合并旧元数据。

回执只保存请求摘要和绑定结果，不保存扩展字符串或 Token；本模块不转储服务端原始响应或错误正文。响应必须有唯一 JSON 文本和明确 `success`。bind/rebind 的正式成功业务响应为：

```json
{"success":true,"data":{"runtimeBindingId":"binding-new","runtimeId":"runtime-other","status":"ACTIVE"}}
```

从 `data.runtimeBindingId` 读取绑定 ID；`data` 是对象，不能把整个对象解析为字符串。保留旧版 `data` 字符串和顶层 `runtimeBindingId` 映射兼容；同时提供的 ID 必须一致，否则结果未知。已提供但缺少有效绑定 ID 的 data 对象/字符串不能被顶层字段覆盖。`runtimeId` 不能代替绑定 ID，`status=ACTIVE` 也不代表设备在线。unbind 要求 `success=true` 且 `data=true`，无论成功提示 `message` 是否存在、文案是什么，都不放宽为接受对象或字符串。业务失败读取 `errorCode`/`errorMsg`，不是根据 `message` 或 `data=false` 推断；明确拒绝记录为 rejected。

上述示例只描述业务 JSON。MCP 外层可能使用 text 或 structuredContent，实际工具输出由平台映射决定；不能把业务示例当作真实请求的绑定回执。

### 待平台确认的配置差异

- rebind 的 `localAgentName` 在所提供的平台配置中为必填，但描述及服务端定义为可选。CLI 暂不更改其可选性；新的联调请求应显式传入名称，并由平台统一必填声明。现有 pending 请求不能通过补参数绕过原请求对账。
- unbind 的服务端成功返回 `data=true`，所提供的平台配置却将 data 声明为 object。若原样透传，应将平台出参改为 boolean；客户端不会为适配错误配置而把任意对象当作解绑成功。

## 失败与恢复

服务端调用与本地状态无法组成数据库事务，因此在既有员工 operation 锁内使用写前记录：

```text
pending → 单次 MCP 调用 → confirmed → 本地绑定提交 → consumed
   │                         │
   ├─明确拒绝 → rejected     └─本地提交失败：同参数重放回执，不再次换绑
   └─超时/断连/响应损坏：unknown，不自动重放 bind/rebind
```

- 明确拒绝：保留旧 ID，不启动目标 Agent。本地旧实例可能已停止；排除其他设备绑定、旧 ID 失效、权限不足或在途/待恢复任务后，重试原操作。
- 响应丢失或进程在 pending 后退出：不假定失败，也不擅自解绑回滚。先由服务端核对 agentUuid、旧绑定和目标设备的实际结果，再由维护者恢复对应本地记录；当前三个接口没有查询/对账能力，CLI 不提供跳过核对的 force 开关。
- unbind 的未知结果：仅允许携带同一旧 ID 重试；利用服务端约定的幂等和“不解除后继绑定”语义。
- 服务端 confirmed、本地尚未提交：重试同参数的原命令。回执摘要包含主管 Profile、员工 ID、本地代数、操作和请求字段；参数变化会阻断恢复。
- 本地已提交，但新 Adapter 启动失败：使用 `connect restart`，不能再调用 rebind。回执未完成时禁止 restart 启动新实例。
- 旧版本本地连接无服务端 ID：先 `connect bind` 补登记，再进行 unbind/rebind。旧连接的 stop/restart 不新增绑定副作用。
- `--profile-only`：不生成设备 ID、不调用绑定 MCP、不保存绑定；绑定参数与此模式互斥。

## 验证边界与回滚

自动化测试使用隔离配置、模拟 MCP 和模拟 DSH 控制，不更改真实绑定。上线前仍需联调三个 MCP 的顶层业务字段、返回值、忙拒绝和身份注入，以及跨设备旧 ID 保护。

本变更不自动迁移存量连接，也不合并依赖 PR 或发布版本。回滚代码不会撤销已经发生的服务端绑定。新增字段可能被旧版严格 JSON 解码器拒绝；不要直接降级或删除回执。应先使用当前版本确认解绑并停止实例，由维护者保留备份后处理旧格式兼容。
