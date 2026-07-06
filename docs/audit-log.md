# 审计日志（Audit Log）

DWS 自动记录每次 MCP HTTP 调用的审计事件，用于合规追溯和安全审查。默认启用，无需额外配置。

## 功能特性

- **自动记录**：每次命令执行产生一条 JSONL 审计事件
- **按天轮转**：日志文件按日期分割（`audit-YYYYMMDD.jsonl`），默认留存 90 天
- **防篡改**：L1 哈希链（sha256），每条事件链接前一条的 hash，可验证完整性
- **远端转发**：支持 POST 到外部 SIEM 或审计平台
- **三级脱敏**：转发时可按 none/hashed/minimal 脱敏敏感字段
- **CLI 命令**：内置 `dws audit tail/export/verify` 查看和验证

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DWS_AUDIT` | 启用 | 设 `0`/`false`/`off` 关闭审计 |
| `DWS_AUDIT_DIR` | `~/.dws/audit` | 审计日志目录 |
| `DWS_AUDIT_RETENTION_DAYS` | `90` | 日志留存天数 |
| `DWS_AUDIT_FORWARD_URL` | （空） | 远端转发 URL（POST JSON） |
| `DWS_AUDIT_FORWARD_TOKEN` | （空） | 远端转发 Bearer Token |
| `DWS_AUDIT_FORWARD_REDACT` | `none` | 转发脱敏级别：`none`/`hashed`/`minimal` |

## 审计事件格式

每条事件是一行 JSON，字段如下：

```json
{
  "ts": "2026-07-06T10:59:06+08:00",
  "execution_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "agent_id": "agent-xxx",
  "actor": {
    "user_id": "525018",
    "name": "胡奕舟",
    "corp_id": "ding8196cd9a2b2405da24f2f5cc6abecb85",
    "corp_name": "钉钉"
  },
  "product": "calendar",
  "command": "list_events",
  "endpoint": "https://api.dingtalk.com/v1.0/calendar/users/xxx/calendars/primary/events",
  "params_summary": "maxResults=20",
  "result": "success",
  "error_category": "",
  "error_reason": "",
  "duration_ms": 234,
  "cli_version": "1.0.47",
  "os": "darwin",
  "arch": "arm64",
  "prev_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "hash": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
}
```

**字段说明**：

- `ts`：事件时间戳（RFC3339）
- `execution_id`：本次命令执行的唯一 ID
- `agent_id`：Agent 标识（若有）
- `actor`：执行者信息（从登录态获取）
- `product`：调用的产品（如 `calendar`、`chat`、`contact`）
- `command`：调用的命令（如 `list_events`、`send_message`）
- `endpoint`：实际请求的 API 端点（已脱敏）
- `params_summary`：参数摘要（已脱敏）
- `result`：`success` 或 `error`
- `error_category` / `error_reason`：错误分类和原因（成功时为空）
- `duration_ms`：命令执行耗时（毫秒）
- `prev_hash` / `hash`：哈希链字段，用于防篡改验证

## CLI 命令

### 查看最近日志

```bash
# 查看最近 20 条审计事件（默认）
dws audit tail

# 查看最近 50 条
dws audit tail -n 50
```

输出示例：

```
2026-07-06 10:59:06  calendar list_events  user=525018  result=success  234ms
2026-07-06 10:58:42  chat send_message     user=525018  result=success  156ms
2026-07-06 10:57:15  contact search        user=525018  result=success   89ms
```

### 导出日志

```bash
# 导出最近 7 天的 JSONL
dws audit export --since 2026-06-29 --until 2026-07-06 --format jsonl > audit-7d.jsonl

# 导出为 CSV（方便 Excel 打开）
dws audit export --since 2026-07-01 --format csv > audit-july.csv
```

### 验证哈希链

```bash
# 验证当前最新日志文件的哈希链完整性
dws audit verify

# 验证指定文件
dws audit verify --file ~/.dws/audit/audit-20260706.jsonl
```

输出：

```
✓ 哈希链完整：234 条事件全部校验通过
```

或：

```
✗ 哈希链断裂：第 156 条事件的 hash 不匹配
```

## 哈希链防篡改

每条事件的 `hash` 字段由以下公式计算：

```
hash = sha256(prev_hash + event_json_without_hash_fields)
```

- 首条事件的 `prev_hash` 为空字符串
- 后续事件的 `prev_hash` = 前一条的 `hash`
- 任何对历史事件的篡改都会导致后续所有 hash 失效

**验证流程**：

1. 读取日志文件，逐行解析
2. 对每条事件，移除 `prev_hash` 和 `hash` 字段，重新序列化
3. 用前一条的 hash + 当前事件 JSON 计算新 hash
4. 对比计算结果与文件中记录的 hash
5. 全部匹配 = 完整；某条不匹配 = 被篡改

## 远端转发

设置 `DWS_AUDIT_FORWARD_URL` 后，每条审计事件会异步 POST 到指定端点：

```bash
export DWS_AUDIT_FORWARD_URL=https://siem.example.com/ingest/audit
export DWS_AUDIT_FORWARD_TOKEN=your-bearer-token  # 可选
```

**请求格式**：

```http
POST /ingest/audit HTTP/1.1
Host: siem.example.com
Authorization: Bearer your-bearer-token
Content-Type: application/json

{"ts":"2026-07-06T10:59:06+08:00","product":"calendar",...}
```

**超时与重试**：3 秒超时，失败不阻塞命令执行（best-effort），不自动重试。

## 脱敏分级

转发时可通过 `DWS_AUDIT_FORWARD_REDACT` 控制脱敏级别：

| 级别 | 行为 | 适用场景 |
|------|------|----------|
| `none`（默认） | 原样转发，不脱敏 | 内部审计平台 |
| `hashed` | actor.name 哈希化，params_summary 脱敏 | 跨部门共享 |
| `minimal` | 仅保留 ts/product/command/result/duration_ms，移除 actor/endpoint/params | 对外合规报告 |

**示例**：

```bash
# 最小化脱敏（仅保留元数据）
export DWS_AUDIT_FORWARD_REDACT=minimal
export DWS_AUDIT_FORWARD_URL=https://compliance.example.com/audit
```

## 常见问题

### Q: 审计日志占多少磁盘？

典型场景（每天 100 条命令）约 50KB/天，90 天约 4.5MB。日志文件是 JSONL 纯文本，gzip 压缩后约 1/5。

### Q: 关闭审计会影响性能吗？

设置 `DWS_AUDIT=0` 后，审计模块不初始化，零开销。默认启用时，每条事件写入耗时 <1ms（异步磁盘 IO）。

### Q: 哈希链断了怎么办？

可能原因：
1. 手动编辑过日志文件
2. 磁盘损坏
3. 并发写入导致顺序错乱（罕见）

**处理**：
- 备份当前日志
- 用 `dws audit verify` 定位断裂位置
- 从断裂点之后的事件可继续验证（前缀已不可信）

### Q: 如何清理旧日志？

自动清理：`DWS_AUDIT_RETENTION_DAYS=90`（默认），超过 90 天的文件在下次启动时 best-effort 删除。

手动清理：

```bash
# 删除 2026 年 6 月之前的日志
rm ~/.dws/audit/audit-202605*.jsonl
```

## 技术实现

- **核心包**：`internal/audit`
- **集成点**：`internal/app/runner.go` 的 `executeInvocation` 方法（defer 调用 `emitAudit`）
- **身份获取**：从 `auth.LoadTokenData` 读取当前登录用户
- **参数脱敏**：调用 `logging.SanitizeArguments`（与现有日志脱敏逻辑一致）

## 相关文档

- [环境变量参考](./reference.md)
- [架构概览](./architecture.md)
- [自动化与脚本](./automation.md)
