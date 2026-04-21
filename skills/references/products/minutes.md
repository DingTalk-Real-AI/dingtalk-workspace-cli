# AI听记 (minutes) 命令参考

## 命令总览

### 查询我创建的听记列表
```
Usage:
  dws minutes list mine [flags]
Example:
  dws minutes list mine
  dws minutes list mine --max 10
  dws minutes list mine --max 10 --next-token <nextToken>
  dws minutes list mine --query "周会"
Flags:
      --max float         查询的听记篇数 (默认 10)
      --next-token string 分页 token (首页留空，后续填写前次返回的 nextToken)
      --query string      关键字筛选 (可选)
      --start string      开始时间 ISO-8601 (可选)
      --end string        结束时间 ISO-8601 (可选)
```

查询我创建的听记列表，支持 `--max` 和 `--next-token` 分页，支持按关键字和时间范围筛选。

### 查询他人共享给我的听记列表
```
Usage:
  dws minutes list shared [flags]
Example:
  dws minutes list shared
  dws minutes list shared --max 20
  dws minutes list shared --max 5 --next-token <nextToken>
Flags:
      --max float         查询的听记篇数 (默认 10)
      --next-token string 分页 token (首页留空，后续填写前次返回的 nextToken)
      --query string      关键字筛选 (可选)
      --start string      开始时间 ISO-8601 (可选)
      --end string        结束时间 ISO-8601 (可选)
```

查询他人共享给我的听记列表，支持 `--max` 和 `--next-token` 分页，支持按关键字和时间范围筛选。

### 查询我有权限访问的所有听记列表
```
Usage:
  dws minutes list all [flags]
Example:
  dws minutes list all
  dws minutes list all --max 20
  dws minutes list all --query "周会" --max 20
  dws minutes list all --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00"
  dws minutes list all --max 10 --next-token <nextToken>
Flags:
      --end string        结束时间 ISO-8601 (可选)
      --query string      关键字筛选 (可选)
      --max float         查询的听记篇数 (默认 10)
      --next-token string 分页 token (首页留空，后续填写前次返回的 nextToken)
      --start string      开始时间 ISO-8601 (可选)
```

查询我有权限访问的所有听记列表（包括我创建的、他人共享给我的等所有有权限的听记）。支持按关键字和时间范围筛选。时间范围和关键字为可选参数，不传则返回所有有权限的听记。支持使用 `--max` 和 `--next-token` 进行分页查询。

### 获取听记基础信息
```
Usage:
  dws minutes get info [flags]
Example:
  dws minutes get info --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

返回字段: 创建人、开始时间、截止时间、听记标题、听记访问链接URL

### 获取听记 AI 摘要
```
Usage:
  dws minutes get summary [flags]
Example:
  dws minutes get summary --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

返回 Markdown 格式摘要，涵盖会议主题、核心结论、关键讨论点等

### 获取听记关键字列表
```
Usage:
  dws minutes get keywords [flags]
Example:
  dws minutes get keywords --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

### 获取听记语音转写原文
```
Usage:
  dws minutes get transcription [flags]
Example:
  dws minutes get transcription --id <taskUuid>
  dws minutes get transcription --id <taskUuid> --direction 1
Flags:
      --direction string   排序方向: 0=正序, 1=倒序 (默认 0)
      --id string          听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
      --next-token string 下一页的token 首次查询可空 后续查询需填写前次请求返回的nextToken
```

每条记录包含: 发言人信息、转写文本、对应时间戳

### 获取听记中提取的待办事项
```
Usage:
  dws minutes get todos [flags]
Example:
  dws minutes get todos --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

每条记录包含: 待办内容、待办唯一ID、参与人信息、待办时间

### 批量查询听记详情
```
Usage:
  dws minutes get batch [flags]
Example:
  dws minutes get batch --ids uuid1,uuid2,uuid3
Flags:
      --ids string   听记 taskUuid 列表，逗号分隔 (必填)
```

返回字段: 听记标题、时长、参与人列表、创建时间、taskUuid、听记状态

### 修改听记标题
```
Usage:
  dws minutes update title [flags]
Example:
  dws minutes update title --id <taskUuid> --title "Q2 复盘会议"
Flags:
      --id string      听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
      --title string   新标题 (必填)
```

### 发起听记（开始录音）
```
Usage:
  dws minutes record start [flags]
Example:
  dws minutes record start
  dws minutes record start --session-id <sessionId>
Flags:
      --session-id string   AI 助理会话 ID (可选)
```

### 暂停听记录音
```
Usage:
  dws minutes record pause [flags]
Example:
  dws minutes record pause --id <taskUuid>
  dws minutes record pause --id <taskUuid> --session-id <sessionId>
Flags:
      --id string           听记 taskUuid (必填)
      --session-id string   AI 助理会话 ID (可选)
```

### 恢复听记录音
```
Usage:
  dws minutes record resume [flags]
Example:
  dws minutes record resume --id <taskUuid>
  dws minutes record resume --id <taskUuid> --session-id <sessionId>
Flags:
      --id string           听记 taskUuid (必填)
      --session-id string   AI 助理会话 ID (可选)
```

### 结束听记录音
```
Usage:
  dws minutes record stop [flags]
Example:
  dws minutes record stop --id <taskUuid>
  dws minutes record stop --id <taskUuid> --session-id <sessionId>
Flags:
      --id string           听记 taskUuid (必填)
      --session-id string   AI 助理会话 ID (可选)
```

## 意图判断

用户说"我的听记/我创建的听记" → `list mine`（可附加 `--query`、`--start`、`--end` 筛选）
用户说"别人给我的听记/共享听记" → `list shared`（可附加 `--query`、`--start`、`--end` 筛选）
用户说"有权限的听记/我能访问的听记/所有听记" → `list all`（可附加 `--query`、`--start`、`--end` 筛选）
用户说"某时间段内的听记/按时间查听记/按关键词查听记" → 根据所属范围选择 `list mine`/`list shared`/`list all`，附加 `--start`、`--end`、`--query` 参数
用户说"听记详情/听记信息" → `get info`
用户说"摘要/总结/会议纪要" → `get summary`
用户说"关键字/关键词" → `get keywords`
用户说"原文/转写/录音文字" → `get transcription`
用户说"会议待办/听记待办" → `get todos`
用户说"改听记标题/重命名听记" → `update title`
用户说"发起听记/开始录音" → `record start`
用户说"暂停听记/暂停录音" → `record pause`
用户说"继续听记/恢复录音" → `record resume`
用户说"结束听记/结束录音" → `record stop`
用户传入听记 URL（如 `https://shanji.dingtalk.com/app/transcribes/xxx`），从 URL 提取 taskUuid，再执行对应的 get/update 操作

## 核心工作流

```bash
# 0. 发起听记（开始录音）
dws minutes record start --format json

# 1. 查看我的听记列表 — 提取 taskUuid
dws minutes list mine --format json
dws minutes list mine --max 10 --next-token <nextToken> --format json
dws minutes list mine --query "周会" --format json

# 1b. 查看共享给我的听记
dws minutes list shared --max 20 --format json
dws minutes list shared --query "日报" --format json

# 1c. 查看我有权限访问的所有听记（支持关键字和时间范围筛选）
dws minutes list all --format json
dws minutes list all --query "周会" --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00" --format json

# 2. 获取 AI 摘要
dws minutes get summary --id <taskUuid> --format json

# 3. 查看完整转写原文
dws minutes get transcription --id <taskUuid> --format json

# 4. 提取待办事项
dws minutes get todos --id <taskUuid> --format json

# 5. 修改标题
dws minutes update title --id <taskUuid> --title "新标题" --format json

# 6. 录音控制（基于 start 返回的 taskUuid）
dws minutes record pause --id <taskUuid> --format json
dws minutes record resume --id <taskUuid> --format json
dws minutes record stop --id <taskUuid> --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list mine` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `list shared` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `list all` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `get batch` | 各听记 `taskUuid` | 进一步查询详情 |

## 注意事项
- `taskUuid` 是听记的唯一标识，所有 get/update 操作均以此为入参
- `record start` 对应 MCP 工具 `execute_listening_note_command` 的 `cmd=create`，通常会返回可继续控制录音的 `taskUuid/uuid`
- `record pause` / `record resume` / `record stop` 对应 `cmd=pause/resume/end`，需要传入 `--id`（映射 MCP 入参 `uuid`）
- 如果用户传入听记 URL（格式: `https://shanji.dingtalk.com/app/transcribes/<taskUuid>`），直接从路径末段提取 taskUuid 作为 `--id` 参数，无需再调用 list 查询
- `list mine`、`list shared`、`list all` 统一走 `list_by_keyword_and_time_range` 链路，通过 `belongingConditionId` 区分（`created` / `shared` / `noLimit`）
- 三个 list 命令均支持 `--max`、`--next-token` 分页及 `--query`、`--start`、`--end` 筛选
- `list mine`、`list shared` 默认每页 20 条，`list all` 默认每页 10 条
- `get summary` 返回 AI 生成的结构化 Markdown 摘要
- `get transcription` 的 `--direction` 控制时间排序: 0=正序(默认), 1=倒序
- `get batch` 支持一次查询多个听记，用逗号分隔 taskUuid

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [minutes_recent_summary.py](../../scripts/minutes_recent_summary.py) | 获取最近听记的 AI 摘要并合并 | `python minutes_recent_summary.py --max 5` |
| [minutes_extract_todos.py](../../scripts/minutes_extract_todos.py) | 从听记中提取待办事项汇总 | `python minutes_extract_todos.py --max 5` |
