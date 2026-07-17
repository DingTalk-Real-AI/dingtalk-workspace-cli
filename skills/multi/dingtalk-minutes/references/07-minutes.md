# 听记与会后工作流

> 产品命令路由见 [minutes-index.md](./minutes-index.md)，scope、taskUuid 与对象选择规则见 [minutes-overview.md](./minutes-overview.md)。

## 通用前置

1. 用户未提供 taskUuid 或 URL 时，先用 `minutes list all` 定位。
2. 同时核对标题、组织、时间与 taskUuid；多候选时让用户确认。
3. 有关键词或时间条件时使用服务端 `--query`、`--start`、`--end` 过滤。
4. 后续调用复用已确认的 taskUuid；不要因某字段为空切换到其他听记。
5. 所有命令带 `--format json`，并从真实返回中提取下一步参数。

## 工作流索引

| 场景 | 固定路线 |
|---|---|
| 查询与浏览 | `list all` → 选择 taskUuid → 按需 `get info/summary/keywords/todos` |
| 多篇基础信息 | 分别定位 taskUuid → `get batch --ids ...` → 保留成功项并说明失败项 |
| 分享摘要 | `get summary` → 解析接收人/群 → 切到 `dingtalk-chat` 发送 |
| 会议待办 | `get todos` → 解析执行人 userId → 切到 `dingtalk-todo` 创建任务 |
| 原文导出 | `list all --start --end` → 每篇 `get transcription` 翻页拉全 → 输出真实转写 |
| 按标签浏览 | `tag list` → 匹配 tagId → `tag query --tag-id` |
| 发言人识别 | 获取转写与参会人 → 形成候选 → 用户确认 → `speaker replace` |
| 修改标题/摘要 | 先定位唯一 taskUuid → `update title/summary` → 回读验证 |

## 查询与详情

```bash
dws minutes list all --query "<关键词>" --start "<ISO>" --end "<ISO>" --format json
dws minutes get info --id <taskUuid> --format json
dws minutes get summary --id <taskUuid> --format json
dws minutes get keywords --id <taskUuid> --format json
dws minutes get todos --id <taskUuid> --format json
```

只需列表时停在 `list`；用户要正文或结论时继续 `get summary`；用户要逐字稿或原话时继续 `get transcription` 并翻页至结束。

## 批量详情

1. 对每个标题或关键词执行 `list all --query`，记录唯一 taskUuid。
2. 使用 `minutes get batch --ids <uuid1,uuid2,...> --format json`。
3. 部分失败时展示成功结果，并列出失败 ID 与原因。
4. 批量接口整体不可用时，逐条执行 `get info`，不要漏掉后续条目。

## 原文导出

1. 计算用户时间范围，传入 `list all --start --end`。
2. 对范围内每篇听记调用 `get transcription --id <taskUuid>`。
3. 返回分页 token 时继续翻页，直到 token 为空。
4. 交付内容必须包含真实转写段落；可以附标题、时间和组织，但不能只交 summary。
5. 范围内无听记时直接说明无数据，不生成模板内容代替。

## 会议待办

1. `minutes get todos --id <taskUuid>` 获取真实行动项。
2. 对责任人使用 `aisearch person` 或 `contact user search` 获取 userId。
3. 切到 `dingtalk-todo`，逐条或批量创建任务。
4. 原数据未给责任人或截止时间时，先请用户确认，不自行补造。

## 发言人处理

- 发言人匹配见 [10-minutes-speaker-match.md](./10-minutes-speaker-match.md)。
- 发言人纠偏见 [11-minutes-speaker-correct.md](./11-minutes-speaker-correct.md)。
- 未确认身份前只展示候选和依据；确认后再调用 `speaker replace` 写回。
- 判断某人是否参会时，结合参会人列表、通讯录和转写线索，不用文本中是否出现姓名作为唯一依据。

## 空数据与错误

- `list` 为空时，保留同一时间范围，先去掉过窄关键词再查一次；仍为空则明确说明。
- 权限失败时说明目标听记未共享给当前用户，不换 ID 绕过。
- 多步流程某一步失败时停止依赖该结果的后续写操作，并说明具体失败点。
- 多数据源任务分别声明各来源的取数状态；缺失来源不得用推测补齐。

## 忠实性

- 标题、组织、时间、时长、责任人和统计数字均以工具返回为准。
- 源数据没有行动项、责任人或数字时，不生成对应事实。
- 汇总前确认每条结论都能追溯到已读取的 summary、transcription 或 todos。
