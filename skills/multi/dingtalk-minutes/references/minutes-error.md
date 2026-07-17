# 听记错误诊断与恢复

> 命令层级与对象选择见 [minutes-overview.md](./minutes-overview.md)，详细参数见 [minutes-commands.md](./minutes-commands.md)。

## 错误决策表

| 错误现象 | 分类 | 处理 |
|---|---|---|
| `taskUuid is invalid` / errcode 300 | ID 无效，不可重试 | 停止复用该 ID；用 `minutes list all` 重新定位真实 taskUuid |
| `permission denied` / 403 | 权限不足，不可重试 | 告知用户联系创建者共享；不要更换参数绕过 |
| `not found` / 404 / `P_DataNotFound` | 资源不存在，不可重试 | 说明听记可能已删除或过期；用 `list all` 查找候选 |
| stdout 为空且无结构化错误 | 临时服务或鉴权异常，可重试 | 原命令重试一次；仍为空则说明服务暂不可用 |
| 网络超时 / 连接失败 | 瞬时错误，可重试 | 原命令重试一次；仍失败则报告网络问题 |
| `result: null` 或内容尚未生成 | 服务端处理中 | 说明摘要/转写仍在生成，稍后再查 |
| `unknown command` | 命令层级错误 | 查对应 `--help`，修正为 `get <subcommand>`、`list <scope>` 等合法结构 |
| `unknown flag` | 参数名错误 | 查具体子命令 `--help`，只修正参数，不在别名之间盲试 |
| `cannot parse time` | 时间格式错误 | 使用 ISO-8601（推荐带时区）或命令支持的日期格式 |
| upload `business error` | 文件、大小、格式或并发会话问题 | 核对文件名、真实字节数、格式与未结束 session；修正后重试一次 |

## taskUuid 恢复

- 无效 taskUuid 不会因重试变为有效。
- 用户未提供 taskUuid 或 URL 时，使用 `minutes list all --query "<关键词>" --format json`。
- 同时核对标题、组织和时间；多候选时请用户确认。
- 不从历史对话、示例或编码字符串拼接 ID。
- `list all` 无结果时，保留时间范围并放宽关键词；仍无结果则明确说明。

## 命令层级恢复

| 错误写法 | 正确写法 |
|---|---|
| `minutes info --id ...` | `minutes get info --id ...` |
| `minutes get --id ...` | `minutes get info/summary/transcription ...` |
| `minutes list --start ...` | `minutes list all --start ...` |
| `minutes summary --id ...` | `minutes get summary --id ...` |

`list` 后必须带 `mine`、`shared` 或 `all`；默认使用 `all`。

## 转写分页

1. 首次执行 `minutes get transcription --id <taskUuid> --format json`。
2. 返回分页 token 时，将该 token 传入下一次调用。
3. token 为空或不存在时结束。
4. 某个 token 返回空内容且没有新 token 时停止，保留已拉取内容并说明覆盖范围。
5. 用户指定时间段时，确认分页结果覆盖该时间段后再分析。

不要使用 shell 管道截断或过滤转写；用 dws 分页参数并在返回 JSON 中处理。

## 服务端筛选

有关键词或时间条件时，把条件放进同一条列表命令：

```bash
dws minutes list all --query "<关键词>" --start "<ISO>" --end "<ISO>" --format json
```

无结果时按顺序处理：

1. 保留时间范围并放宽关键词。
2. 必要时扩大用户允许的时间窗。
3. 列出近似候选供确认。
4. 仍无结果时明确说明，不生成虚构内容。

## 权限与批量查询

- `get batch` 部分成功时，保留成功数据并列出失败 ID 与原因。
- 批量接口整体不可用时，逐条降级为 `get info`。
- 权限失败不改变目标 taskUuid；不要切到另一篇听记替代。
- 用户需要他人听记时，可先检查 `list shared` 或请创建者共享。

## Upload 恢复

固定流程：

1. `upload create --file-name ... --file-size <bytes>` 获取 `sessionId` 与上传地址。
2. 使用返回的地址上传原文件。
3. `upload complete --session-id <sessionId>` 完成。
4. 需要放弃时执行 `upload cancel --session-id <sessionId>`。

`--file-size` 使用真实字节数。已有活动 session 时先完成或取消，不创建并发会话。

## 跨平台执行

每条 dws 命令独立执行，不依赖 `head`、`grep`、`jq`、shell 后台符号或重定向。筛选、分页和输出格式优先使用 CLI 自身参数。

## 自动化脚本

| 脚本 | 场景 |
|---|---|
| [minutes_recent_summary.py](../scripts/minutes_recent_summary.py) | 获取近期听记摘要并合并 |
| [minutes_extract_todos.py](../scripts/minutes_extract_todos.py) | 提取听记待办 |
