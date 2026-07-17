# 听记命令层级与路由

> 本文只保留命令导航和跨命令不变量。具体参数见 [minutes-commands.md](./minutes-commands.md)，错误恢复见 [minutes-error.md](./minutes-error.md)。

## Scope 选择

`dws minutes list` 后必须带 scope：

| Scope | 含义 | 何时使用 |
|---|---|---|
| `mine` | 仅自己创建或发起的听记 | 用户明确说“我创建的 / 我发起的 / 我录的” |
| `shared` | 仅他人共享给自己的听记 | 用户明确限定共享来源 |
| `all` | 当前用户可访问的全部听记（mine + shared） | 默认；用户说“我能看到的 / 可访问的 / 有权限的 / 所有听记” |

不要使用裸 `dws minutes list`。按标题、关键词、参会人或时间定位听记时，默认用 `list all`，避免遗漏共享内容。

## 指令树

```text
dws minutes
├── list <mine|shared|all> [--query] [--start] [--end] [--limit] [--cursor]
├── get
│   ├── info --id <taskUuid>
│   ├── summary --id <taskUuid>
│   ├── transcription --id <taskUuid> [--cursor <token>]
│   ├── keywords --id <taskUuid>
│   ├── todos --id <taskUuid>
│   ├── audio --id <taskUuid>
│   └── batch --ids <uuid1,uuid2>
├── update
│   ├── title --id <taskUuid> --title "..."
│   └── summary --id <taskUuid> --content "..."
├── record
│   ├── start
│   ├── pause --id <taskUuid>
│   ├── resume --id <taskUuid>
│   └── stop --id <taskUuid>
├── speaker
│   ├── replace --id <taskUuid> --from "..." --to "..."
│   └── summary <create|get> --ids <uuid1,uuid2>
├── mind-graph <create|status> --id <taskUuid>
├── hot-word <add|list>
├── replace-text --id <taskUuid> --search "..." --replace "..."
├── upload <create|complete|cancel>
├── permission <add|remove>
└── tag <list|query>
```

## 意图路由

| 用户意图 | 命令 |
|---|---|
| 列出、搜索、按时间找听记 | `minutes list all [--query] [--start] [--end]` |
| 查看会议讲了什么 | `minutes get summary` |
| 查看逐字稿、原话、沟通细节 | `minutes get transcription`，翻页至结束 |
| 查看关键词、待办或基础信息 | `minutes get keywords/todos/info` |
| 批量查看多篇基础信息 | `minutes get batch --ids ...` |
| 修改标题或纪要 | `minutes update title/summary` |
| 生成脑图 | `minutes mind-graph create` 后轮询 `status` |
| 替换文字或发言人 | `minutes replace-text` / `minutes speaker replace` |
| 本地音视频转听记 | `minutes upload create` → 上传 → `complete` |

## taskUuid 与对象选择

1. 用户给 taskUuid 时直接使用；用户给听记 URL 时先按下节提取 taskUuid。
2. 用户只给标题、关键词或时间时，先 `list all` 定位；禁止凭历史上下文或记忆填写 ID。
3. 多候选时同时核对 `taskUuid`、`title`、`organizationName` 和时间。无法唯一确定时列出候选让用户确认。
4. “最近一次某类会议”先按主题过滤，再在同主题候选中按时间取最新；不要只取全列表中时间最新的一条。
5. “最长 / 最短”按返回的 `durationMicros` 数值比较，不按标题或自然语言印象判断。
6. 锁定 taskUuid 后，后续 summary、keywords、transcription、update 必须复用同一个 ID。某字段为空时如实说明，不要悄悄切换到另一篇听记。
7. 标题日期与创建日期可能不同。精确日期未命中时，先保留主题词并放宽时间窗，再列候选确认。
8. 多篇详情优先使用 `get batch`；部分条目失败时保留成功结果并逐条说明失败原因，整体失败时再降级为 `get info`。

## 转写分页与忠实性

- 用户明确要“原文 / 逐字稿 / 原话 / 沟通细节”时，调用 `get transcription` 并持续传入返回的分页 token，直到 token 为空。
- 用户只要摘要、列表、关键词或待办时，不要附带拉取完整转写。
- 用户指定时间区间时，确认已拉取到目标时间段后再分析；未覆盖目标区间时继续翻页或明确说明数据不足。
- 基于听记生成报告或统计时，只使用实际返回的数据。源数据没有责任人、行动项或数字时，不要补造。
- 用户列出多个数据源时，分别读取每个来源；某来源失败时说明缺失，不用其他来源推断精确事实。

## URL → taskUuid

| 输入形式 | 提取规则 |
|---|---|
| `.../app/transcribes/<taskUuid>` | 取 `/transcribes/` 后、查询串前的路径段 |
| `.../meeting/minutes?taskUuid=<taskUuid>` | 取 `taskUuid` 查询参数 |
| 带 `minutesId=<taskUuid>` 的聊天链接 | 取 `minutesId` 查询参数 |
| 纯 taskUuid | 原样使用 |

提取后把纯 taskUuid 传给 `--id`；不要把完整 URL 传给 `--id`，也不要要求用户手工提取。

## 常见错误写法

| 错误 | 正确 |
|---|---|
| `minutes info --id ...` | `minutes get info --id ...` |
| `minutes summary --id ...` | `minutes get summary --id ...` |
| `minutes detail --id ...` | 使用 `get info`、`get summary` 或 `get transcription` |
| `minutes list` | `minutes list all` 或显式 `mine/shared` |
| `minutes get transcription --url <URL>` | 提取 taskUuid 后传 `--id` |
| `minutes upload create --json ...` | 使用 `--file-name` 与 `--file-size` |
| `minutes get transcription ... \| head` | 单独执行 dws 命令并解析 JSON |

## 参数差异

| 语义 | minutes 参数 | 兼容参数 |
|---|---|---|
| 单页条数 | `--limit` | `--max` |
| 分页标识 | `--cursor` | `--next-token` |
| 听记 ID | `--id` | `--uuid` / `--task-uuid` |
| 起止时间 | `--start` / `--end` | 无 |

不要把 calendar 的 `--start-time/--end-time` 或其他产品参数带入 minutes。

## 权限与上传

- 权限错误表示当前用户不能访问该听记，不要通过更换参数或命令绕过。告知用户联系创建者共享权限。
- 批量查询部分无权限时，展示成功条目并列出失败 ID 与原因。
- 上传固定按 `upload create` → 使用返回的上传地址上传文件 → `upload complete`。需要取消时使用创建阶段返回的 `sessionId`。
- `--file-size` 单位是字节，使用真实文件大小，不做估算。
