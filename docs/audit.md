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

| 字段 | 含义 | 来源 |
|---|---|---|
| `ts` / `trace_id` | 时间 / 唯一 trace | CLI（`trace_id` == 传输层 execution_id） |
| `actor` | 用户 id / 姓名 | 登录 token |
| `org` | 组织 corp_id / 名称 | 登录 token |
| `device` | os / hostname / device_id / sn_no | 本机；`device_id`/`sn_no` 需 opt-in |
| `intent` | 自然语言原文 + `provenance` | 仅 agent 层可注入，标记 `provenance=agent`，CLI 无法验真 |
| `module` / `command` / `subcommand` | 操作模块 / skill 命令 / 子命令 | CLI |
| `subcommand_desc` | 子命令介绍 | 命令 catalog |
| `target` | 操作对象 id / 名称 / 摘要 / 敏感度 | 调用参数 + catalog（`sensitive` → `confidential`） |
| `flow` | 数据流向 + api + 本地路径 / endpoint / peer ids | 调用参数推断 |
| `outcome` / `err_class` / `exit_code` | 成败与错误分类 | CLI |

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

## 隐私与合规

- `device_id` / `sn_no` 是 PIPL 下的个人信息，**默认不采集**，企业需显式开启并告知用户。
- 自然语言原文只有上层 agent 能提供，审计记录里以 `provenance=agent` 标注，
  表明该字段非 CLI 实测、不可验真。
- 富审计数据是**企业的合规资产**，应进入企业自有 sink；dws 不提供任何厂商默认收集端点。
