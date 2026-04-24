# AI听记 (minutes) 命令参考

AI 听记：列表 / 详情 / 摘要 / 待办 / 文字稿 / 思维导图 / 发言人 / 热词 / 文件上传。

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

### 覆盖听记 AI 摘要
```
Usage:
  dws minutes update summary [flags]
Example:
  dws minutes update summary --id <taskUuid> --content "修订后的摘要 Markdown"
Flags:
      --id string        听记 taskUuid (必填)
      --content string   新的摘要正文 (必填)，会整体覆盖原摘要
```

将 AI 生成的摘要替换成定制版本，适合人工修订或按业务口径重写后回写。

### 全文替换听记文本
```
Usage:
  dws minutes replace-text [flags]
Example:
  dws minutes replace-text --id <taskUuid> --search "小钉" --replace "DingTalk"
Flags:
      --id string        听记 taskUuid (必填)
      --search string    需要替换的原文 (必填)
      --replace string   替换后的文本 (必填)
```

在转写段落与摘要中一次性替换所有命中文本，适合纠正系统性的识别错误（如专有名词）。

### 替换发言人标签
```
Usage:
  dws minutes speaker replace [flags]
Example:
  dws minutes speaker replace --id <taskUuid> --from "说话人1" --to "张三"
  dws minutes speaker replace --id <taskUuid> --from "说话人1" --to "张三" --target-uid <userId>
Flags:
      --id string          听记 taskUuid (必填)
      --from string        原发言人昵称，如 "说话人1" (必填)
      --to string          新发言人昵称 (必填)
      --target-uid string  绑定到指定钉钉 userId (可选)
```

修正自动分离的发言人标签，可顺带把该说话人对应到真实用户。

### 添加个人热词
```
Usage:
  dws minutes hot-word add [flags]
Example:
  dws minutes hot-word add --words "钉钉,悟空,DWS"
Flags:
      --words string   热词列表，逗号分隔 (必填)
```

把专有名词/业务术语写入个人热词库，后续听记的语音识别会优先匹配，用来兜底高频误识别。

### 生成思维导图
```
Usage:
  dws minutes mind-graph create [flags]
Example:
  dws minutes mind-graph create --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

异步任务：提交后返回 jobId，需要用 `mind-graph status` 轮询结果。

### 查询思维导图状态
```
Usage:
  dws minutes mind-graph status [flags]
Example:
  dws minutes mind-graph status --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

轮询 `mind-graph create` 的生成任务，就绪后返回思维导图内容。

### 创建文件上传会话
```
Usage:
  dws minutes upload create [flags]
Example:
  dws minutes upload create --file-name "会议录音.m4a" --file-size 10485760 --title "Q2 复盘"
Flags:
      --file-name string            本地文件名 (必填)
      --file-size string            文件大小，单位字节 (必填)
      --title string                听记标题 (可选)
      --template-id string          摘要模板 ID (可选)
      --input-language string       语言，如 zh_CN / en_US (可选)
      --enable-message-card string  是否推送消息卡片: true/false (可选)
```

三步上传的第 1 步：返回 sessionId 和预签名 URL，由调用方把本地音视频文件 HTTP PUT 到该 URL。

### 完成上传并生成听记
```
Usage:
  dws minutes upload complete [flags]
Example:
  dws minutes upload complete --session-id <sessionId>
Flags:
      --session-id string   upload create 返回的 sessionId (必填)
```

三步上传的第 3 步：通知服务端文件已上传完毕，触发转写与 AI 处理，返回新的 taskUuid。

### 取消上传
```
Usage:
  dws minutes upload cancel [flags]
Example:
  dws minutes upload cancel --session-id <sessionId>
Flags:
      --session-id string   upload create 返回的 sessionId (必填)
```

上传中途放弃或上游出错时释放会话，避免残留。

## 意图判断

用户说"我的听记/我创建的听记" → `list mine`（可附加 `--query`、`--start`、`--end` 筛选）
用户说"别人给我的听记/共享听记" → `list shared`（可附加 `--query`、`--start`、`--end` 筛选）
用户说"有权限的听记/我能访问的听记/所有听记" → `list all`（可附加 `--query`、`--start`、`--end` 筛选）
用户说"某时间段内的听记/按时间查听记/按关键词查听记" → 根据所属范围选择 `list mine`/`list shared`/`list all`，附加 `--start`、`--end`、`--query` 参数
用户说"听记详情/听记信息" → `get info`
用户说"批量看听记/一次查多篇" → `get batch`（`--ids` 传逗号分隔 taskUuid）
用户说"摘要/总结/会议纪要" → `get summary`
用户说"关键字/关键词" → `get keywords`
用户说"原文/转写/录音文字" → `get transcription`
用户说"会议待办/听记待办" → `get todos`
用户说"改听记标题/重命名听记" → `update title`
用户说"改摘要/覆盖摘要/重写总结" → `update summary`
用户说"全文替换/批量改转写里的错字/专有名词搞错了" → `replace-text`
用户说"改发言人/把说话人1改成张三/认领发言人" → `speaker replace`
用户说"加热词/让后续识别更准/把某个专业术语教给系统" → `hot-word add`
用户说"生成思维导图/导图/脑图" → `mind-graph create`（异步），再 `mind-graph status` 轮询
用户说"上传录音生成听记/把本地音视频变听记" → `upload create` → 本地 PUT → `upload complete`；放弃则 `upload cancel`
用户传入听记 URL（如 `https://shanji.dingtalk.com/app/transcribes/xxx`），从 URL 提取 taskUuid，再执行对应的 get/update 操作

## 核心工作流

```bash
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

# 5. 批量补齐多个 taskUuid 的详情
dws minutes get batch --ids <taskUuid1>,<taskUuid2>,<taskUuid3> --format json

# 6. 修改标题 / 覆盖摘要
dws minutes update title --id <taskUuid> --title "新标题" --format json
dws minutes update summary --id <taskUuid> --content "修订后的摘要 Markdown" --format json

# 7. 修正转写错字 / 发言人标签 / 加热词
dws minutes replace-text --id <taskUuid> --search "小钉" --replace "DingTalk" --format json
dws minutes speaker replace --id <taskUuid> --from "说话人1" --to "张三" --format json
dws minutes hot-word add --words "钉钉,悟空,DWS" --format json

# 8. 异步生成思维导图（create 拿 jobId，status 轮询）
dws minutes mind-graph create --id <taskUuid> --format json
dws minutes mind-graph status --id <taskUuid> --format json

# 9. 从本地音视频上传生成听记（三步走）
dws minutes upload create --file-name "会议录音.m4a" --file-size 10485760 --title "Q2 复盘" --format json
# → 拿到 sessionId 和预签名 URL，调用方自行 HTTP PUT 把文件推上去
dws minutes upload complete --session-id <sessionId> --format json
# 放弃上传
dws minutes upload cancel --session-id <sessionId> --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list mine` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `list shared` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `list all` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `get batch` | 各听记 `taskUuid` | 进一步查询详情 |
| `upload create` | `sessionId`、预签名 URL | 本地 PUT 上传 + `upload complete` / `upload cancel` 的 --session-id |
| `upload complete` | 新听记 `taskUuid` | 后续 get/update/mind-graph 的 --id |
| `mind-graph create` | 异步 jobId（随 taskUuid 关联） | `mind-graph status` 的 --id 查进度与结果 |

## 注意事项
- `taskUuid` 是听记的唯一标识，所有 get/update/mind-graph/speaker/replace-text 操作均以此为入参
- 如果用户传入听记 URL（格式: `https://shanji.dingtalk.com/app/transcribes/<taskUuid>`），直接从路径末段提取 taskUuid 作为 `--id` 参数，无需再调用 list 查询
- `list mine`、`list shared`、`list all` 统一走 `list_by_keyword_and_time_range` 链路，通过 `belongingConditionId` 区分（`created` / `shared` / `noLimit`）；三者均支持 `--max`、`--next-token` 分页及 `--query`、`--start`、`--end` 筛选
- `list mine`、`list shared` 默认每页 20 条，`list all` 默认每页 10 条
- `get info` 是单条查询；`get batch` 是批量补齐 ID 列表（`--ids` 逗号分隔），用在"有一串 taskUuid，想一次拿标题/时长/参与人"的场景
- `get summary` 返回 AI 生成的结构化 Markdown 摘要；`update summary` 会整体覆盖原摘要，不做增量合并
- `get transcription` 的 `--direction` 控制时间排序: 0=正序(默认), 1=倒序
- `replace-text` 是系统性纠错：命中即替换，整篇转写与摘要一起改，慎用模糊文本
- `speaker replace` 只改"标签"，不重跑说话人分离；`--target-uid` 可顺带把该说话人绑定到真实 userId
- `hot-word add` 作用于**个人**热词库，只对本人后续的听记识别生效
- `mind-graph create` 是异步：调用后立即返回，需要用 `mind-graph status --id <taskUuid>` 轮询到 ready 才能拿到图
- 上传流程严格三步：`upload create`（拿 sessionId + 预签名 URL）→ 客户端自己 HTTP PUT 文件 → `upload complete`（触发转写）；中途放弃必须用 `upload cancel` 释放会话，不要只丢 sessionId

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [minutes_recent_summary.py](../../scripts/minutes_recent_summary.py) | 获取最近听记的 AI 摘要并合并 | `python minutes_recent_summary.py --max 5` |
| [minutes_extract_todos.py](../../scripts/minutes_extract_todos.py) | 从听记中提取待办事项汇总 | `python minutes_extract_todos.py --max 5` |
