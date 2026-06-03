# 操作审计（Audit）

dws 可以为**每一次命令调用**生成一条结构化审计记录，用于满足**企业合规审计**的通用需求
——任何部署 dws 的企业都可以开启，把员工经 dws 的操作留痕。

设计遵循开源惯例，把「产生事件」和「投递事件」分开：

- **通道 A — 本地审计文件**：始终是源头真相，operator 自己拥有、可随时 `grep`。
- **通道 B — 转发到企业自有 sink**：可选，endpoint 由**部署企业**配置，
  **绝不写死到厂商**。转发前可按脱敏档位降级。

> 审计**默认全关**。不设置 `DWS_AUDIT_ENABLED` 时，dws 不产生任何审计数据，
> 热路径零影响。

## 启用

| 环境变量 | 说明 | 示例 |
|---|---|---|
| `DWS_AUDIT_ENABLED` | 启用本地审计文件 | `true` |
| `DWS_AUDIT_FORWARD_URL` | 转发目标（企业自有 sink，非厂商默认） | `https://audit.internal.example.com/dws` |
| `DWS_AUDIT_FORWARD_TOKEN` | 企业 sink 的 Bearer 鉴权 | `xxxxx` |
| `DWS_AUDIT_FORWARD_REDACT` | 转发脱敏档：`none` / `hashed` / `minimal` | `none` |
| `DWS_AUDIT_REDACT_SALT` | `hashed` 档的加盐值 | `tenant-salt` |
| `DWS_AUDIT_DEVICE_FINGERPRINT` | 采集 `device_id` / `sn_no`（PIPL 个人信息，默认关） | `true` |
| `DWS_AUDIT_NL_INTENT` | 上层 agent 注入的自然语言原文 | `把上周听记导出` |

本地文件路径：`<configDir>/logs/audit.jsonl`（每行一条 JSON）。

## 字段

字段按**可信度**分两类，只有可信字段会被记录：

**① 可信字段（已上）** —— token 验证 / dws 自管 / dws 实测，调用方无法 per-call 伪造：

| 字段 | 含义 | 来源 | 可信原因 |
|---|---|---|---|
| `ts` / `trace_id` | 时间 / 唯一 trace | CLI（`trace_id` == 传输层 execution_id） | dws 实测 |
| `actor` | 用户 id / 姓名 | 登录 token | 网关验签，`user_id` 仅登录流程捕获时有 |
| `org` | 组织 corp_id / 名称 | 登录 token | 网关验签，不可伪造 |
| `client` | `agent_id`（装机 id）/ `source` / `cli_version` | identity.json + 编译版本 | dws 自管/编译注入，非调用方自报 |
| `device` | os / hostname / device_id / sn_no | 本机；`device_id`/`sn_no` 需 opt-in | 读真硬件 |
| `intent` | 自然语言原文 + `provenance` | 仅 agent 层注入 | **标记 `provenance=agent`，明示不可验真** |
| `module` / `command` / `subcommand` | 操作模块 / skill 命令 / 子命令 | CLI 解析实际执行的命令 | dws 实测 |
| `subcommand_desc` | 子命令介绍 | 命令 catalog | 线上 catalog |
| `target` | 操作对象 id / 名称 / 摘要 / 敏感度 | 调用参数 + catalog（`sensitive` → `confidential`） | dws 实测 |
| `flow` | 数据流向 + api + 本地路径 / endpoint / peer ids | 调用参数推断 | dws 实测 |
| `outcome` / `err_class` / `exit_code` | 成败与错误分类 | CLI | dws 实测 |

**② 暂不上字段（可伪造，待网关签名）** —— 见下方 TODO：
`host_agent`（装在哪个 agent，`DINGTALK_AGENT`）、`channel`（渠道，`DWS_CHANNEL`）、`agent_code`（`DINGTALK_DWS_AGENTCODE`）。这三个是调用方自报的环境变量，`export` 即可冒充，**不可信，故先不记录**。

### `flow.direction` 取值

- `local-export`：参数里带本地路径（如 `--output`），数据落到本机磁盘。
- `read`：只读命令（list/get/query/search…），无数据移动。
- `intra-tenant`：数据在租户内对象之间流转，`peer_ids` 收集涉及的人/群/文档 id。
- `external-api`：流向租户外接口（预留）。

## 脱敏档位（仅作用于转发，本地文件始终全量）

| 档位 | 行为 | 适用 |
|---|---|---|
| `none` | 原样转发 | sink 在企业自己信任域内（企业内部审计库） |
| `hashed` | 自然语言、对象名、序列号、peer ids 替换为加盐哈希，可关联不可还原 | 跨信任域但仍需关联 |
| `minimal` | 只留维度（命令×版本×成败×方向），丢弃一切内容/身份 | 纯运维监控 |

## 企业接入示例

数据进企业自己的审计库，全字段、含设备指纹：

```bash
export DWS_AUDIT_ENABLED=true
export DWS_AUDIT_FORWARD_URL="https://audit.internal.example.com/dws"
export DWS_AUDIT_FORWARD_TOKEN="<enterprise-issued-token>"
export DWS_AUDIT_FORWARD_REDACT=none
export DWS_AUDIT_DEVICE_FINGERPRINT=true
# 由上层 agent/skill 在每次调用前注入：
# export DWS_AUDIT_NL_INTENT="<用户这次的自然语言请求>"
```

验证：

```bash
dws minutes export --minute-id m-77 --output ~/Desktop/q2.md --format json
tail -n1 ~/.dws/logs/audit.jsonl | jq .   # 路径随 DWS_CONFIG_DIR / edition 变化
```

## 日志存在哪里 / 能否中心化收集

- **默认：每个用户自己机器上**，`<configDir>/logs/audit.jsonl`，不开转发就不出本机。
- **要中心化收集**：配置 `DWS_AUDIT_FORWARD_URL` 指向一个收集端点，每个用户每次调用就会 POST 一条上去。
  - **企业合规场景**：endpoint 指向**企业自己的审计库**，钉钉/厂商不持有数据（推荐，合规干净）。
  - **平台侧统一收集（钉钉这边收）**：技术上可行——把 endpoint 指向钉钉的审计 ingest 服务即可；
    但这等于厂商集中持有用户操作数据，必须 **opt-in + 明确告知**，否则就是开源 CLI 最忌讳的“偷偷上报”。
    建议拆成两条：**合规全量审计 → 企业自有 sink**；**匿名极简遥测（`minimal` 档）→ 钉钉平台**做运维监控，
    隐私边界才清楚。
- 不管哪种，本地文件始终是源头真相；转发是尽力而为，丢了可从本地文件回补。

### 接收端契约

收集端点（`DWS_AUDIT_FORWARD_URL`）只需实现：

```
POST /
Content-Type: application/json
Authorization: Bearer <token>     # 对应 DWS_AUDIT_FORWARD_TOKEN
X-Dws-Audit-Schema: 2
Body: 一条审计事件 JSON
返回 2xx 即成功
```

任何 HTTP 服务都能接，不需要专用组件。

### 接入阿里云 SLS（生产推荐）

SLS（日志服务）自带写入 / 存储 / 检索 / Dashboard / 留存，是审计落地的标准选型：

1. SLS 控制台建 **Project** + **Logstore**，设留存天数（合规常用 180/365 天），
   给 `trace_id` / `command` / `subcommand` / `outcome` / `corp_id` / `agent_id` 开字段索引。
2. 立一个收 POST 的端点（**函数计算 FC** HTTP 触发器最省运维，或 ECS/K8s），
   校验 Bearer 后把 body 作为一条日志 `PutLogs` 写进 Logstore（整条 JSON 放 `event` 字段，
   另抽 `trace_id`/`command`/`outcome`/`corp_id` 做索引列）。
3. 把 FC 地址作为 `DWS_AUDIT_FORWARD_URL` 下发给各端 dws。

之后“谁 / 何时 / 操作了什么 / 成没成 / 数据流向”直接在 SLS 控制台查询与做看板。

## TODO

- **网关签名的 agent 身份**：`host_agent` / `channel` / `agent_code` 目前是调用方自报的环境变量、可伪造，
  故暂不记录。待网关能回带一个**与 token 绑定的签名 agent 凭证**后再加入审计，确保“装在哪个 agent / 哪个渠道”不可冒充。
- **`actor.user_id` 稳定化**：让登录流程把 `user_id` 落进 token，使其每次都非空（当前仅部分登录流程捕获）。

## 隐私与合规

- `device_id` / `sn_no` 是 PIPL 下的个人信息，**默认不采集**，企业需显式开启并告知用户。
- 自然语言原文只有上层 agent 能提供，审计记录里以 `provenance=agent` 标注，
  表明该字段非 CLI 实测、不可验真。
- 富审计数据是**企业的合规资产**，应进入企业自有 sink；dws 不提供任何厂商默认收集端点。
- `host_agent` / `channel` / `agent_code` 等调用方自报字段在网关签名前**不记录**，避免可伪造数据混入审计。
