# 第三方 Agent 接入说明：PAT 自定义授权卡片

## 1. 请求头

第三方业务开发者通过 `DINGTALK_AGENT` 指定自己的业务 Agent 名称。

生效请求头：

```http
claw-type: <business-agent-name 或 default>
```

规则：

- `DINGTALK_AGENT` 为空：等价于 `claw-type: default`
- `DINGTALK_AGENT=default`：等价于 `claw-type: default`
- `claw-type = default`：走默认 DWS 行为
- `claw-type != default`：命中 PAT 时，DWS 返回 JSON，由第三方宿主自己处理

业务方将自己的业务 Agent 名称写入 `DINGTALK_AGENT`。

示例：

```http
POST /mcp HTTP/1.1
Content-Type: application/json
claw-type: QoderWork

{"method":"tools/call","params":{"name":"dws.contacts.search","arguments":{"keyword":"Alice"}}}
```

默认模式示例：

```http
POST /mcp HTTP/1.1
Content-Type: application/json
claw-type: default

{"method":"tools/call","params":{"name":"dws.contacts.search","arguments":{"keyword":"Alice"}}}
```

## 2. 触发条件

命中 PAT 时：

- 进程退出码为 `4`
- `stderr` 返回机器可读 JSON

示例命令：

```bash
dws mcp doc create_document --json '{"title":"季度复盘"}'
```

示例伪代码：

```ts
if (exitCode === 4) {
  const payload = JSON.parse(stderrText)
}
```

## 3. 必须解析的字段

至少解析这些字段：

- `code` 或 `error_code`
- `data.authRequestId`：可选
- `data.flowId`：可选
- `data.requiredScopes`：可选
- `data.grantOptions`：可选
- `data.callbacks`

关键点：

- `flowId` 不是必带字段
- `authRequestId` 不是必带字段
- 没有 `flowId` 时，不要轮询

## 4. JSON 示例

### 4.1 普通 PAT

```json
{
  "success": false,
  "code": "PAT_NO_PERMISSION",
  "data": {
    "requiredScopes": ["contact.user.read"],
    "grantOptions": ["once", "session"],
    "authRequestId": "req-001",
    "flowId": "flow-001",
    "callbacks": [
      {
        "name": "list_super_admins",
        "invoke": {
          "type": "cli",
          "argv": ["dws", "pat", "callback", "list-super-admins"],
          "args": {
            "authRequestId": "req-001"
          }
        }
      },
      {
        "name": "send_apply",
        "invoke": {
          "type": "cli",
          "argv": ["dws", "pat", "callback", "send-apply"],
          "args": {
            "authRequestId": "req-001"
          },
          "required": ["adminStaffId"]
        }
      },
      {
        "name": "poll_flow",
        "invoke": {
          "type": "cli",
          "argv": ["dws", "pat", "callback", "poll-flow"],
          "args": {
            "authRequestId": "req-001",
            "flowId": "flow-001"
          },
          "required": ["flowId"]
        }
      }
    ]
  }
}
```

### 4.2 Scope 授权型 PAT

```json
{
  "success": false,
  "code": "PAT_SCOPE_AUTH_REQUIRED",
  "data": {
    "missingScope": "mail:send",
    "callbacks": [
      {
        "name": "auth_login",
        "invoke": {
          "type": "cli",
          "argv": ["dws", "auth", "login", "--scope", "mail:send"]
        }
      }
    ]
  }
}
```

## 5. 业务方宿主如何处理

### 步骤 1：执行原始命令

```bash
dws mcp doc create_document --json '{"title":"季度复盘"}'
```

### 步骤 2：如果退出码是 `4`，解析 `stderr` JSON

```ts
const payload = JSON.parse(stderrText)
```

### 步骤 3：渲染你自己的授权卡片

示例 UI：

- 标题：`Sales Copilot 需要额外权限`
- 原因：`缺少权限：contact.user.read`
- 按钮：`发起授权申请`

### 步骤 4：调用 callback

查询管理员：

```bash
dws pat callback list-super-admins --auth-request-id req-001
```

发送申请：

```bash
dws pat callback send-apply --admin-staff-id manager123 --auth-request-id req-001
```

轮询流程：

```bash
dws pat callback poll-flow --flow-id flow-001 --auth-request-id req-001
```

处理 scope 授权：

```bash
dws auth login --scope mail:send
```

## 6. 什么时候可以重试

只有满足以下条件，宿主才重试原命令：

- `status = "APPROVED"`
- `retrySuggested = true`

示例返回：

```json
{
  "success": true,
  "code": "PAT_CALLBACK_POLL_FLOW",
  "data": {
    "status": "APPROVED",
    "tokenUpdated": true,
    "retrySuggested": true
  }
}
```

此时可重试原命令：

```bash
dws mcp doc create_document --json '{"title":"季度复盘"}'
```

## 7. 最小接入流程

### 请求

```http
claw-type: sales-copilot
```

### 命中 PAT

```json
{
  "success": false,
  "code": "PAT_NO_PERMISSION",
  "data": {
    "authRequestId": "req-001",
    "flowId": "flow-001",
    "requiredScopes": ["contact.user.read"]
  }
}
```

### 第三方动作

1. 自己渲染卡片
2. 调用：
   - `dws pat callback list-super-admins`
   - `dws pat callback send-apply`
   - `dws pat callback poll-flow`
   - 或 `dws auth login --scope ...`
3. 只有在 `retrySuggested = true` 时重试原命令

## 8. 约束

- 不要把 `DWS_CHANNEL` 当作 PAT 控制位
- 不要假设一定有 `flowId`
- 不要假设一定有 `authRequestId`
- 不要直接调用 DingTalk PAT 接口
- 统一走 `dws` 的 callback contract
