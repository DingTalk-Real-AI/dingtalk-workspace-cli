# AI 听记 (minutes) 命令参考

> 本文档与真实 CLI 严格对齐。历史版本里 `list mine/shared/all` 和 `record start/pause/resume/stop` 等写法**均不存在**，已全部修正。

## 命令总览

### list 类（查询听记列表）

| 子命令 | 用途 |
|-------|------|
| `list query` | 统一查询入口，通过 `--__scope__` 区分我创建 / 共享给我 / 全部 |
| `list-by-keyword-range` | 按关键词 + 时间范围查询（顶级，不在 list 下） |
| `list-my-created-minutes` | 简化接口：我创建的听记 |
| `list-shared-minutes` | 简化接口：共享给我的听记 |

### get 类（听记详情）

| 子命令 | 用途 |
|-------|------|
| `get info` | 听记基础信息（标题 / 时长 / 链接） |
| `get summary` | AI 摘要（Markdown） |
| `get keywords` | 关键词列表 |
| `get transcription` | 语音转写原文 |
| `get todos` | 待办事项 |
| `get batch` | 批量查询多个听记详情 |

### update 类（修改听记）

| 子命令 | 用途 |
|-------|------|
| `update title` | 修改标题 |
| `update summary` | 修改摘要内容 |

### 其他顶级命令

| 子命令 | 用途 |
|-------|------|
| `upload create` | 创建上传会话 |
| `upload complete` | 提交上传完成 |
| `upload cancel` | 取消上传 |
| `hot-word add` | 添加个人热词 |
| `mind-graph create` | 生成思维导图 |
| `mind-graph status` | 查询思维导图生成状态 |
| `speaker replace` | 替换发言人 |
| `replace-text` | 替换听记文字 |
| `record` | 录音控制（⚠️ 当前 CLI 未挂子命令，预留） |

---

## list query — 统一查询听记列表

通过 `--__scope__` 区分听记归属：`created`（我创建）/ `shared`（共享给我）/ `noLimit`（全部）。

```
Usage:
  dws minutes list query [flags]
Example:
  dws minutes list query --__scope__ created --max 10
  dws minutes list query --__scope__ shared --max 20 --next-token <nextToken>
  dws minutes list query --__scope__ noLimit --query "周会" --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00"
Flags:
      --__scope__ string    归属范围：created / shared / noLimit (必填)
      --query string        关键字 keyword
      --start string        开始时间 createTimeStart (ISO-8601 可选)
      --end string          结束时间 createTimeEnd (ISO-8601 可选)
      --max string          单次最大返回条数 maxResults
      --next-token string   分页游标 nextToken
```

> 约定：`--max` 在 CLI 是 string 类型，传数字字符串如 `"10"`。

---

## list-by-keyword-range — 按关键词 + 时间范围查询

```
Usage:
  dws minutes list-by-keyword-range [flags]
Example:
  dws minutes list-by-keyword-range --keyword "周会" --create-time-start "2026-03-01T00:00:00+08:00" --create-time-end "2026-03-20T23:59:59+08:00" --page-size 20
Flags:
      --keyword string                  关键字
      --create-time-start string        起始时间 (ISO-8601)
      --create-time-end string          结束时间 (ISO-8601)
      --belonging-condition-id string   归属条件 ID（如 created/shared/noLimit，可选）
      --biz-type-list string            业务类型列表（可选）
      --offset string                   偏移量
      --page-size string                每页数量
```

---

## list-my-created-minutes — 查询我创建的听记（简化接口）

```
Usage:
  dws minutes list-my-created-minutes [flags]
Example:
  dws minutes list-my-created-minutes --max-results 20
  dws minutes list-my-created-minutes --max-results 20 --next-token <nextToken>
Flags:
      --max-results string   单次最大返回条数
      --next-token string    分页游标
```

---

## list-shared-minutes — 查询共享给我的听记（简化接口）

```
Usage:
  dws minutes list-shared-minutes [flags]
Example:
  dws minutes list-shared-minutes --max-results 20
Flags:
      --max-results string   单次最大返回条数
```

---

## get info — 听记基础信息

返回字段：创建人、开始时间、截止时间、听记标题、访问链接 URL。

```
Usage:
  dws minutes get info [flags]
Example:
  dws minutes get info --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

---

## get summary — AI 摘要

返回 Markdown 格式摘要，涵盖会议主题、核心结论、关键讨论点。

```
Usage:
  dws minutes get summary [flags]
Example:
  dws minutes get summary --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

---

## get keywords — 关键词列表

```
Usage:
  dws minutes get keywords [flags]
Example:
  dws minutes get keywords --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

---

## get transcription — 语音转写原文

每条记录包含：发言人信息、转写文本、对应时间戳。

```
Usage:
  dws minutes get transcription [flags]
Example:
  dws minutes get transcription --id <taskUuid>
  dws minutes get transcription --id <taskUuid> --direction 1
  dws minutes get transcription --id <taskUuid> --next-token <nextToken>
Flags:
      --id string          听记 taskUuid (必填)
      --direction string   排序方向：0=正序(默认) / 1=倒序
      --next-token string  分页游标
```

---

## get todos — 提取待办事项

每条记录包含：待办内容、待办 ID、参与人、待办时间。

```
Usage:
  dws minutes get todos [flags]
Example:
  dws minutes get todos --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

---

## get batch — 批量查询听记详情

```
Usage:
  dws minutes get batch [flags]
Example:
  dws minutes get batch --ids uuid1,uuid2,uuid3
Flags:
      --ids string   听记 taskUuid 列表，逗号分隔 (必填)
```

---

## update title — 修改听记标题

```
Usage:
  dws minutes update title [flags]
Example:
  dws minutes update title --id <taskUuid> --title "Q2 复盘会议"
Flags:
      --id string      听记 taskUuid (必填)
      --title string   新标题 (必填)
```

---

## update summary — 修改听记摘要

```
Usage:
  dws minutes update summary [flags]
Example:
  dws minutes update summary --id <taskUuid> --content "## 核心结论..."
Flags:
      --id string        听记 taskUuid (必填)
      --content string   新的摘要内容 summaryText (必填)
```

---

## upload create — 创建文件上传会话

```
Usage:
  dws minutes upload create [flags]
Example:
  dws minutes upload create --file-name "周会录音.mp3" --file-size 10485760 --title "Q2 周会"
Flags:
      --file-name string              文件名 (必填)
      --file-size string              文件大小（字节） (必填)
      --title string                  听记标题
      --input-language string         输入语言 minutesOption.inputLanguage
      --template-id string            模板 ID minutesOption.templateId
      --enable-message-card string    是否启用消息卡片 minutesOption.enableMessageCard
```

---

## upload complete — 提交上传完成

```
Usage:
  dws minutes upload complete [flags]
Example:
  dws minutes upload complete --session-id <sessionId>
Flags:
      --session-id string   上传会话 ID (必填，由 upload create 返回)
```

---

## upload cancel — 取消上传

```
Usage:
  dws minutes upload cancel [flags]
Example:
  dws minutes upload cancel --session-id <sessionId>
Flags:
      --session-id string   上传会话 ID (必填)
```

---

## hot-word add — 添加个人热词

```
Usage:
  dws minutes hot-word add [flags]
Example:
  dws minutes hot-word add --words "钉钉闪会,DingChat,AI 听记"
Flags:
      --words string   热词列表 hotWordList，逗号分隔 (必填)
```

---

## mind-graph create — 生成思维导图

```
Usage:
  dws minutes mind-graph create [flags]
Example:
  dws minutes mind-graph create --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

---

## mind-graph status — 查询思维导图生成状态

```
Usage:
  dws minutes mind-graph status [flags]
Example:
  dws minutes mind-graph status --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

---

## speaker replace — 替换发言人

```
Usage:
  dws minutes speaker replace [flags]
Example:
  dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "张三" --target-uid <userId>
Flags:
      --id string           听记 taskUuid (必填)
      --from string         当前发言人昵称 speakerNick (必填)
      --to string           目标昵称 targetNickName (必填)
      --target-uid string   目标用户 userId
```

---

## replace-text — 替换听记文字

```
Usage:
  dws minutes replace-text [flags]
Example:
  dws minutes replace-text --id <taskUuid> --search "DingTalk" --replace "钉钉"
Flags:
      --id string        听记 taskUuid (必填)
      --search string    原文 originalText (必填)
      --replace string   替换为 replacedText (必填)
```

---

## record — 录音控制

> ⚠️ 当前 CLI 下 `minutes record` 没有挂任何子命令或 flag（`dws minutes record --help` 看不到任何参数），预留入口。如需录音控制，请确认 CLI 版本或改用上层 MCP 工具。

---

## 意图判断

- 用户说"我的听记/我创建的听记" → `list query --__scope__ created` 或 `list-my-created-minutes`
- 用户说"共享听记/别人给我的听记" → `list query --__scope__ shared` 或 `list-shared-minutes`
- 用户说"所有听记/我能访问的听记" → `list query --__scope__ noLimit`
- 用户说"按时间/按关键词查听记" → `list query` 带 `--query/--start/--end`，或 `list-by-keyword-range`
- 用户说"听记详情/听记信息" → `get info`
- 用户说"摘要/会议总结" → `get summary`
- 用户说"关键字/关键词" → `get keywords`
- 用户说"原文/转写/录音文字" → `get transcription`
- 用户说"会议待办/听记待办" → `get todos`
- 用户说"改听记标题/重命名" → `update title`
- 用户说"改摘要/改总结" → `update summary`
- 用户说"上传录音/上传听记" → `upload create` → （PUT 文件）→ `upload complete`
- 用户说"添加热词" → `hot-word add`
- 用户说"生成思维导图" → `mind-graph create` → `mind-graph status` 轮询
- 用户说"改发言人名字" → `speaker replace`
- 用户说"替换听记里的文字" → `replace-text`
- 用户传入听记 URL（`https://shanji.dingtalk.com/app/transcribes/<taskUuid>`）→ 从末段提取 taskUuid 作为 `--id`

---

## 核心工作流

```bash
# 1. 查我创建的听记 — 提取 taskUuid
dws minutes list query --__scope__ created --max 10 --format json

# 2. 获取 AI 摘要
dws minutes get summary --id <taskUuid> --format json

# 3. 查完整转写
dws minutes get transcription --id <taskUuid> --format json

# 4. 提取待办
dws minutes get todos --id <taskUuid> --format json

# 5. 改标题
dws minutes update title --id <taskUuid> --title "新标题" --format json
```

### 上传本地录音文件（跨 HTTP PUT）

```bash
# Step 1: 创建上传会话 — 拿 sessionId + 上传 URL
dws minutes upload create --file-name "周会.mp3" --file-size <字节数> --title "Q2 周会" --format json

# Step 2: HTTP PUT 上传文件到返回的上传 URL
curl -X PUT -T "周会.mp3" "<上传 URL>"

# Step 3: 提交完成 — 触发听记任务
dws minutes upload complete --session-id <sessionId> --format json
```

### 生成思维导图（异步）

```bash
# Step 1: 触发生成
dws minutes mind-graph create --id <taskUuid> --format json

# Step 2: 轮询状态
dws minutes mind-graph status --id <taskUuid> --format json
```

---

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list query` / `list-my-created-minutes` / `list-shared-minutes` / `list-by-keyword-range` | `taskUuid`、`nextToken` | `get *` / `update *` 的 `--id`；翻页时 `--next-token` |
| `get batch` | 各 `taskUuid` | 进一步查详情 |
| `upload create` | `sessionId`、上传 URL | HTTP PUT；`upload complete --session-id` |
| `mind-graph create` | （异步）| `mind-graph status --id` 轮询 |

---

## 注意事项

- `taskUuid` 是听记唯一标识，所有 `get` / `update` / `mind-graph` / `speaker` / `replace-text` 操作均以此入参
- 历史文档里的 `list mine/shared/all` 和 `record start/pause/resume/stop` **不存在于真实 CLI**，请使用本文档的正确路径
- `list query` 的 `--__scope__` flag 带双下划线前后缀，是 CLI 自动生成的 stub 占位符，属实际参数
- `--max` / `--max-results` / `--page-size` / `--direction` 等数值型 flag 在真实 CLI 均为 `string` 类型（自动生成 stub），需传字符串形式
- 时间字段统一 ISO-8601（如 `2026-03-10T00:00:00+08:00`）
- 如果用户传入听记 URL（`https://shanji.dingtalk.com/app/transcribes/<taskUuid>`），直接从路径末段提取 taskUuid，**无需再调用 list 查询**
- `get transcription --direction` 控制时间排序：0=正序(默认) / 1=倒序
- `record` 顶层命令当前无子命令也无 flag，属未完成接口；如历史文档提到的 `record start/pause/resume/stop` 不可用
