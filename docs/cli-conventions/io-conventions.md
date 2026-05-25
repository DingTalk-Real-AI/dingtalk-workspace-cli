# DWS CLI 出参入参规范

> 基于项目根目录 `出参入参规范.md`（版本 1.0）润色，去掉了 MCP 映射和返回值部分。

---

## 1. 总则：分层命名体系

CLI 体系涉及多个层面，各层有独立的命名规约。**建议**同一层内保持统一风格，不同层之间可以有不同的命名风格。

| 层面 | 命名风格 | 示例 |
|------|---------|------|
| CLI flag | kebab-case | `--base-id`, `--start`, `--group-id` |
| CLI 命令路径 | 小写 + 空格分层 | `dws calendar event list` |
| 全局 flag | kebab-case | `--format`, `--dry-run`, `--verbose`, `--yes` |

### 1.1 命令层级结构

```
dws <product> [resource] <action> [flags] [positional-args]
```

| 层级 | 说明 | 示例 |
|------|------|------|
| product | 产品名，小写单词 | calendar, chat, oa, aitable, todo |
| resource | 资源名（可选） | event, message, approval, record |
| action | 动作，标准动词 | list, get, create, update, delete, search |

### 1.2 推荐动词表（建议优先使用）

下列动词在项目内使用频率最高、语义最清晰，**建议新增命令优先从这张表里选用**，有助于 AI Agent 正确理解命令语义：

| 动词 | 语义 | 典型场景 |
|------|------|---------|
| list | 列举资源 | 有明确的筛选条件，返回列表 |
| get | 获取详情 | 按 ID 获取单个/批量资源详情 |
| create | 新建资源 | 创建新的实例 |
| update | 修改资源 | 修改已有实例的属性 |
| delete | 删除资源 | 移除资源（危险操作，需 `--yes`） |
| search | 搜索资源 | 按关键词模糊搜索，无精确 ID |
| send | 发送 | 消息、通知类操作 |
| approve / reject | 审批 | OA 审批类操作 |

**说明**：
- 本表是**推荐清单，不是白名单**。项目里 `query`、`add`、`read`、`info`、`import` 等动词都有合法的在用场景（对应钉钉 OpenAPI 语义或业务习惯），新增时**不强求**一定替换成推荐动词
- 如果某个新动词能更准确表达业务语义（例如 `import` 表达"从外部批量导入"比 `create` 更清楚），**建议保留语义清晰的原动词**
- Code Review 时对动词的判断仅作**建议性提示**，不作为打回依据

### 1.3 search vs list 区分

| 动词 | 语义 | 适用场景 | 典型 flag |
|------|------|---------|----------|
| search | 模糊搜索 | 按关键词模糊匹配，无精确 ID | `--query`, `--keyword` |
| list | 列举/筛选 | 按精确条件筛选，有时间/状态/ID 过滤 | `--start`, `--end`, `--status` |

- 新增产品搜索关键词 flag 统一用 `--query`
- `--keyword` 仅在后端 API 已用 keyword 时兼容保留

### 1.4 过滤与作用域规范

同一资源仅过滤条件不同时，应统一为 `list + --scope` 而非拆成多个子命令。

```bash
# 推荐
dws calendar event list --scope mine --start ... --end ...
dws oa approval list --scope pending --start ...

# 不推荐（仅过滤条件不同，却拆成独立子命令）
dws calendar event list-mine ...
dws oa approval list-pending ...
```

`--scope` 枚举值：相同语义**建议**使用相同名称（例如统一用 `mine` 而非 `my`），以降低 AI Agent 跨产品调用时的学习成本。

| 值 | 语义 | 适用场景 |
|----|------|---------|
| mine | 仅我的 | 日程、任务、项目 |
| all | 全部 | 管理员视角 |
| pending | 待处理 | 审批、任务 |
| initiated | 我发起的 | 审批 |

### 1.5 camelCase 别名

- Flag 定义时只使用 kebab-case
- camelCase 别名由 `RegisterCamelCaseAliases` 框架自动派生
- 开发者不需要手动注册 camelCase 别名

---

## 2. CLI Flag 入参规范

### 2.1 Flag 命名规则

| 规则 | 正确 | 错误 |
|------|------|------|
| kebab-case | `--base-id`, `--group-id` | `--baseId`, `--base_id` |
| 语义明确 | `--instance-id` | `--id`（有歧义时） |
| ID 类加资源前缀 | `--task-id`, `--report-id` | `--id`（存在多种 ID 时） |
| 单一资源可用 `--id` | `calendar event get --id` | — |

### 2.2 Flag 类型规范

| 类型 | CLI flag 声明 | 示例 |
|------|-------------|------|
| string | `Flags().String(...)` | `--start "2026-03-10T14:00:00+08:00"` |
| int | `Flags().Int(...)` | `--size 20`, `--cursor 0` |
| bool | `Flags().Bool(...)` | `--forward`, `--at-all`, `--yes` |
| string (enum) | `Flags().String(...)` + Help 列举 | `--format json\|table\|raw` |
| stringSlice | `Flags().StringSlice(...)` | `--ids id1,id2,id3` |

### 2.3 全局 Flag 一览

| Flag | 短名 | 类型 | 默认值 | 说明 |
|------|------|------|-------|------|
| `--format` | `-f` | string | table | 输出格式：json / table / raw |
| `--verbose` | `-v` | bool | false | 显示详细日志 |
| `--debug` | — | bool | false | 显示调试日志 |
| `--yes` | `-y` | bool | false | 跳过确认提示（AI Agent 模式） |
| `--dry-run` | — | bool | false | 预览操作内容，不实际执行 |
| `--timeout` | — | int | 30 | HTTP 请求超时时间（秒） |

---

## 3. 时间字段规范

### 3.1 统一标准

所有 CLI 时间 flag 统一接受 **ISO-8601 字符串**。

| 维度 | 标准 |
|------|------|
| CLI 入参格式 | ISO-8601 字符串 |
| CLI 入参 flag 名 | 统一使用 `--start` / `--end` |
| 默认时区 | Asia/Shanghai（UTC+8） |
| **建议** | 优先使用 ISO-8601 字符串，避免毫秒时间戳作为 CLI 入参（可读性更好、AI Agent 更容易理解） |

### 3.2 支持的时间格式

| 格式 | 示例 |
|------|------|
| RFC3339（推荐） | `2026-03-10T14:00:00+08:00` |
| UTC | `2026-03-10T14:00:00Z` |
| 无时区 | `2026-03-10T14:00:00` |
| 空格分隔 | `2026-03-10 14:00:00` |
| 仅日期 | `2026-03-10` |

### 3.3 各产品时间 Flag 对照

| 产品 | 命令 | CLI Flag |
|------|------|---------|
| 日历 | event list | `--start` / `--end` |
| OA | approval list-pending | `--start` / `--end` |
| 日志 | report list | `--start` / `--end` |
| 听记 | minutes list | `--start` / `--end` |
| 考勤 | attendance list | `--start` / `--end` |

---

## 4. 身份 ID 规范

### 4.1 ID 字段命名对照

| 资源 | CLI Flag | 来源命令 |
|------|---------|---------|
| 用户 | `--user` | `contact user search` |
| 部门 | `--dept-id` | `contact dept search` |
| 群会话 | `--group` | `chat search` |
| 审批实例 | `--instance-id` | `oa approval list-*` |
| 日程 | `--id` | `calendar event list` |
| 待办任务 | `--task-id` | `todo task list` |
| 日志 | `--report-id` | `report list` |
| Base | `--base-id` | `aitable base search` |
| Table | `--table-id` | `aitable base get` |
| Field | `--field-id` | `aitable table get` |
| 记录 | `--record-ids` | `aitable record query` |
| 文档节点 | `--node` | `doc search` |
| 云盘文件 | `--dentry-id` | `drive list` |
| 听记 | `--id(s)` | `minutes list` |

### 4.2 ID 暴露策略

| 场景 | 策略 |
|------|------|
| 自操作 | 不暴露 userId，从 session 自动获取 |
| 管理操作 | 暴露 `--user`，查他人数据时传入 |
| 租户 ID | **禁止暴露** corpId / tenantId 不作为 flag |
| 内部 ID | **禁止暴露** unionId / openId 不暴露给终端用户 |

---

## 5. 分页参数规范

### 5.1 两种分页模式

| 模式 | 适用场景 | CLI Flag |
|------|---------|---------|
| 游标分页（优先） | 大数据集、增量拉取 | `--cursor` / `--size` |
| 页码分页 | 总量已知、翻页场景 | `--page` / `--size` |

### 5.2 各产品分页模式对照

| 产品 | 命令 | 分页模式 | CLI Flag |
|------|------|---------|---------|
| OA | list-pending | 页码 | `--page` / `--size` |
| OA | list-initiated | 游标 | `--next-token` / `--max-results` |
| OA | list-forms | 游标 | `--cursor` / `--size` |
| 日志 | report list | 游标 | `--cursor` / `--size` |
| 待办 | task list | 页码 | `--page` / `--size` |
| 聊天 | search | 游标 | `--cursor` |

---

## 6. 错误处理规范

### 6.1 退出码体系

| 退出码 | 含义 | 典型场景 |
|-------|------|---------|
| 0 | 成功 | 命令正常完成 |
| 1 | API/MCP 服务端错误 | MCP 调用失败、网络超时 |
| 2 | 认证错误 | Token 过期、未登录 |
| 3 | 输入校验错误 | 参数缺失、JSON 格式错误 |
| 4 | 权限不足 | PAT 授权拒绝、403 |
| 5 | 内部/未知错误 | 文件锁超时 |

### 6.2 错误码命名

格式：`CATEGORY_SPECIFIC`（大写 snake_case）

| 错误码 | 退出码 | 含义 |
|--------|-------|------|
| `AUTH_NOT_CONFIGURED` | 2 | 未登录 |
| `AUTH_TOKEN_EXPIRED` | 2 | Token 过期 |
| `AUTH_PERMISSION_DENIED` | 4 | 权限不足 |
| `INPUT_MISSING_PARAM` | 3 | 必填参数缺失 |
| `INPUT_INVALID_JSON` | 3 | JSON 格式错误 |
| `MCP_SERVER_ERROR` | 1 | 服务端错误 |
| `MCP_TOOL_ERROR` | 1 | 工具调用失败 |
| `NETWORK_TIMEOUT` | 1 | 网络超时 |
| `RESOURCE_NOT_FOUND` | 1 | 资源不存在 |

---

## 7. Description 与 Help 规范

### 7.1 四要素 Description（建议）

**建议**每个命令的 Description 覆盖以下要素，能显著提升 AI Agent 选择命令的准确率：

| 要素 | 说明 | 示例 |
|------|------|------|
| **用途** | 这个命令做什么 | "查询指定时间范围内的日程列表" |
| **场景** | 什么时候用（≥3 种） | "查看今日安排 / 查某段时间的日程 / 查某个日历的日程" |
| **区分** | 与相似命令的区别 | "查自己的日程用 list-mine，查所有日历用 list" |
| **示例** | 调用示例 | `dws calendar event list --start "2026-03-10T14:00:00+08:00"` |

### 7.2 危险操作标注

危险操作（删除、撤销、覆盖等）**建议**在 Help 中标注 `[危险]`，并默认要求 `--yes` 确认（保护用户数据安全）。

---

## 8. 安全规范

| 规则 | 说明 |
|------|------|
| Token 不暴露 | accessToken / mcpToken 不作为 flag 或返回值 |
| 内部 ID 不暴露 | unionId / openId / corpId 不暴露给终端用户 |
| 内部类名不暴露 | 禁止 Java 类名出现在参数中 |

### 危险操作确认

| 操作类型 | 要求 |
|---------|------|
| 删除 | 必须确认，Help 标注 `[危险]` |
| 撤销 | 必须确认 |
| 覆盖写入 | 必须确认 |
| 批量操作 | ≤30 条/次 |
