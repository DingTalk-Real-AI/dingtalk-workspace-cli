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
- `data.hostControl`：可选
- `data.missingScope`：可选，常见于 `PAT_SCOPE_AUTH_REQUIRED`

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
    "hostControl": {
      "clawType": "sales-copilot",
      "mode": "host",
      "pollingOwner": "host",
      "retryOwner": "host"
    }
  }
}
```

### 4.2 Scope 授权型 PAT

```json
{
  "success": false,
  "code": "PAT_SCOPE_AUTH_REQUIRED",
  "data": {
    "missingScope": "mail:send"
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

### 步骤 4：宿主自己处理后续动作

开源 CLI 不再提供 `dws pat callback ...` 命令面。
后续动作由宿主或宿主自己的后端完成，例如：

- 查询可审批管理员
- 发起授权申请
- 在有 `flowId` 时轮询授权状态
- 根据 `authRequestId` 绑定宿主侧状态

处理 scope 授权：

```bash
dws auth login --scope mail:send
```

## 6. 什么时候可以重试

只有在宿主确认授权完成、且所需 token 刷新已经结束后，才重试原命令。

此时可重试原命令，例如：

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
2. 宿主自己完成管理员查询、申请提交、状态轮询等后续动作
3. 若是 scope 授权，执行 `dws auth login --scope ...` 或宿主自己的等价登录流程
4. 只有在宿主确认授权完成后才重试原命令

## 8. 约束

- 不要把 `DWS_CHANNEL` 当作 PAT 控制位
- 不要假设一定有 `flowId`
- 不要假设一定有 `authRequestId`
- 不要假设存在稳定的 `dws pat callback ...` 命令面
- 宿主必须自己负责后续授权流程
