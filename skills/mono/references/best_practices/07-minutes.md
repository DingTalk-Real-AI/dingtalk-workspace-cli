# 听记与会后

> lite（`list-minutes`、`get-minutes-summary`、`get-minutes-transcription`）见 [SKILL.md](../../SKILL.md)。full recipe 见下表。  
> 日程、订会议室、`schedule-meeting` 见 [03-meeting.md](./03-meeting.md)。产品命令见 [minutes.md](../products/minutes.md)。

### 听记列表参数速查

`list mine` / `list shared` / `list all` 均支持以下筛选参数（**有时间/关键词条件时优先使用服务端过滤；无筛选条件时可全量拉取**）：

| 参数 | 说明 | 示例 |
|------|------|------|
| `--query "<关键词>"` | 服务端关键词搜索 | `--query "周会"` |
| `--start "<ISO-8601>"` | 开始时间（含时区） | `--start "2026-05-01T00:00:00+08:00"` |
| `--end "<ISO-8601>"` | 结束时间（含时区） | `--end "2026-05-25T23:59:59+08:00"` |
| `--limit <N>` | 每页返回条数（默认 10） | `--limit 20`（`--max` 为兼容别名） |
| `--cursor "<token>"` | 分页 token（首页留空） | `--cursor "abc123"`（`--next-token` 为兼容别名） |

**组合筛选示例**：
```bash
# 按时间范围
dws minutes list all --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00" --format json
# 关键词 + 时间范围
dws minutes list mine --query "需求评审" --start "2026-05-25T00:00:00+08:00" --end "2026-05-25T23:59:59+08:00" --format json
# 限制条数
dws minutes list mine --limit 5 --format json
```

| Recipe           | 行动指南（固定路线）                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| meeting-followup | 1. **提取待办优先**：`minutes_extract_todos.py`（开源版未引入；可手动用 `dws minutes get todos --id <taskUuid>` 拼装） → 获取行动项列表 无脚本时按[「多源并行采集」](./_common/conventions.md#多源并行采集公共模式)执行 2. 提取行动项 3. `aisearch person --keyword "<姓名>" --dimension name` → 取 `userId` → `todo task create --title "<行动项>" --executors <userId>`（每条行动项；批量时可写 JSON 调 `python scripts/todo_batch_create.py todos.json`） 4. `chat message send --group <openConversationId> --text "<通知内容>"` 通知已创建 |
| share-minutes    | **拉摘要优先**：`minutes_recent_summary.py`（开源版未引入；可手动用 `dws minutes list` + `dws minutes get summary --id <taskUuid>` 组合） → 获取近期听记摘要 备选手动：1. `minutes list mine` → 取 `taskUuid` 2. 用户选定 3. `minutes get summary --id <taskUuid>` → 取摘要 4. 单聊：`contact user search --query "<姓名>"` → 取 `openDingTalkId` → `chat message send --open-dingtalk-id <openDingTalkId> --text "<摘要内容>"`（推荐）；群聊：`--group <openConversationId> --text "<摘要内容>"` 发送。仅当无法获取 openDingTalkId 时才用 `--user <userId>`（备选） |
| browse-minutes   | 1. `minutes list all --query "<关键词>" --limit <N>` → 取全量 `taskUuid`（翻页直至无更多；无主题筛选用 `minutes list mine`；**有时间范围时加 `--start`/`--end` 服务端过滤**，如 `--start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00"`） 2. **详情/元数据优先** `minutes get batch --ids <uuid1,uuid2,...>`（仅 API 上限时拆多批）；**摘要专项**且无 batch 字段时再用并行 `get summary` 或 `**python scripts/minutes_recent_summary.py --limit <N>`** 3. 汇总展示；**同一用户诉求下避免**「半截 list → 再 list 剩余」「拆 4 条一批 summary 连跑多轮」等无必要拆分 |
| minutes-detail   | 1. 确定 `taskUuid`：用户已提供 → 直接用；未提供 → `minutes list mine --limit 5` 让用户选（**有时间线索时加 `--start`/`--end`**，如「上周五的会」→ `--start "2026-05-23T00:00:00+08:00" --end "2026-05-23T23:59:59+08:00"`） 2. 并行拉取四维信息：`minutes get info --id <taskUuid>` `&` `minutes get summary --id <taskUuid>` `&` `minutes get keywords --id <taskUuid>` `&` `minutes get todos --id <taskUuid>` `& wait` 3. 整合输出：基础信息（标题、时间、参与人）→ AI 摘要 → 关键字 → 行动项/待办 4. **展示发言人列表**：从转写/info 提取所有发言人（含已标注姓名和匿名编号），列出每位发言人的发言次数和时长占比，引导用户：「是否需要查看某位发言人的详细内容总结？请输入姓名或编号」→ 用户选定 → 进入 `minutes-speaker-summarize` recipe 5.（可选）用户要求看原文 → `minutes get transcription --id <taskUuid>` |
| minutes-speaker-summarize | 1. 读取转写 → `minutes get transcription --id <uuid>` 2. 声纹标注检查：已标注 → 跳 Step 6；匿名编号 → 继续 3. 转写原文推断（称呼/自我介绍/上下文指代）→ 高置信度跳 Step 6 4. 并发身份推断：`calendar event list` + `participant list` 取日程参与人（最高优先） & `aisearch person` & `chat message list` & `doc search` `& wait`；未找到同时段日程 → 引导用户提供日程链接或参会人名单；两路以上一致才下结论 5. 置信度判断：>70% 直接输出；≤70% 展示 TOP3 候选让用户选（最多一次） 6. 结构化总结输出（核心观点 + 问题 + Action Item + 立场）→ 追问是否替换发言人标注。详见 [10-minutes-speaker-match.md](./10-minutes-speaker-match.md) |
| speaker-correct | 1. **查同时段日程**：从听记元数据获取录音开始时间 → `calendar event list`（前后 30 分钟内）→ 若匹配到日程则提取参会人名单并展示 2. **展示发言人状态表格**：列出所有发言人及标注状态（已识别 / 未标注 / 未标注，发言较少） 3. **让用户选择识别方式**：听音识人（裁剪代表性音频片段让用户辨认）/ 手动设置（用户直接告知对应关系）/ 智能匹配（结合参会人名单和发言内容推断） 4. **执行替换**：确认对应关系后，通讯录查询获取 dingUid → `dws contact user search --query "<姓名>" --format json` → 取 userId（长整型）→ `dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "<姓名>" --target-uid <userId> --format json`；多个匹配时列出候选让用户选；无匹配时执行不带 `--target-uid` 的替换 5. 替换成功后追问「还有其他发言人需要帮你识别和替换吗？」详见 [11-minutes-speaker-correct.md](./11-minutes-speaker-correct.md) |

