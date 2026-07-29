# 听记与会后

> lite（`minutes-query`）见 [lite-recipes.md](./lite-recipes.md)。full recipe 见下表。
> 日程、订会议室、`schedule-meeting` 见 `dingtalk-calendar/references/03-meeting.md`。产品命令见 [minutes.md](./minutes.md)。

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

| Recipe           | 行动指南（固定路线）                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ---------------- |---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| meeting-followup | 1. **提取待办优先**：`python scripts/minutes_extract_todos.py [--id <taskUuid>] [--limit N]` → 获取行动项列表 无脚本时按[「多源并行采集」](_common/conventions.md#多源并行采集公共模式)执行 2. 提取行动项 3. `aisearch person --keyword "<姓名>" --dimension name` → 取 `userId` → `todo task create --title "<行动项>" --executors <userId> --priority <10/20/30/40>`（每条行动项；批量时切到 `dingtalk-todo` 的 batch-create-todo recipe，批量脚本只在该 sub-skill 内可用） 4. `chat message send --group <openConversationId> --text "<通知内容>"` 通知已创建                                                                                                                                                                                         |
| share-minutes    | **拉摘要优先**：`python scripts/minutes_recent_summary.py [--limit N] [--output summary.md]` → 获取近期听记摘要 备选手动：1. `minutes list mine` → 取 `taskUuid` 2. 用户选定 3. `minutes get summary --id <taskUuid>` → 取摘要 4. 单聊：`aisearch person --keyword "<姓名>" --dimension name` → 取 `openDingTalkId` → `chat message send --open-dingtalk-id <openDingTalkId> --text "<摘要内容>"`（推荐）；群聊：`--group <openConversationId> --text "<摘要内容>"` 发送。仅当无法获取 openDingTalkId 时才用 `--user <userId>`（备选）                                                                                                                                                                                |
| browse-minutes   | 1. `minutes list all --query "<关键词>" --limit <N>` → 取全量 `taskUuid`（翻页直至无更多；无主题筛选用 `minutes list mine`；**有时间范围时加 `--start`/`--end` 服务端过滤**，如 `--start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00"`） 2. **详情/元数据优先** `minutes get batch --ids <uuid1,uuid2,...>`（仅 API 上限时拆多批）；**摘要专项**且无 batch 字段时再用并行 `get summary` 或 `**python scripts/minutes_recent_summary.py --limit <N>`** 3. 汇总展示；**同一用户诉求下避免**「半截 list → 再 list 剩余」「拆 4 条一批 summary 连跑多轮」等无必要拆分                                                                                                                                                             |
| minutes-raw-export | 适用"下载/导出最近一周听记原文/逐字稿/转写内容/完整记录"：1. 按用户时间词计算 `--start` / `--end`（最近一周示例：7 天前 00:00:00+08:00 到今天 23:59:59+08:00） 2. `minutes list all --start "<ISO>" --end "<ISO>" --format json` 获取范围内所有可见听记；为空时直接说明无相关数据 3. 对列表中每个 `taskUuid` 逐一 `minutes get transcription --id <taskUuid> --format json`，如返回 `cursor` / `nextToken` / `nextCursor`，继续用 `--cursor <token>` 拉完后续页 4. 交付文件或最终回复必须包含各听记的真实转写段落，可附标题/时间/组织，禁止只交 summary 或让用户再选某一条 |
| minutes-detail   | 1. 确定 `taskUuid`：用户已提供 → 直接用；未提供 → `minutes list mine --limit 5` 让用户选（**有时间线索时加 `--start`/`--end`**，如「上周五的会」→ `--start "2026-05-23T00:00:00+08:00" --end "2026-05-23T23:59:59+08:00"`） 2. 并行拉取四维信息：`minutes get info --id <taskUuid>` `&` `minutes get summary --id <taskUuid>` `&` `minutes get keywords --id <taskUuid>` `&` `minutes get todos --id <taskUuid>` `& wait` 3. 整合输出：基础信息（标题、时间、参与人）→ AI 摘要 → 关键字 → 行动项/待办 4. **展示发言人列表**：从转写/info 提取所有发言人（含已标注姓名和匿名编号），列出每位发言人的发言次数和时长占比，引导用户：「是否需要查看某位发言人的详细内容总结？请输入姓名或编号」→ 用户选定 → 进入 `minutes-speaker-summarize` recipe 5.（可选）用户要求看原文 → `minutes get transcription --id <taskUuid>` |
| minutes-speaker-summarize | 1. 读取转写 → `minutes get transcription --id <uuid>` 2. 声纹标注检查：已标注 → 跳 Step 6；匿名编号 → 继续 3. 转写原文推断（称呼/自我介绍/上下文指代）→ 高置信度跳 Step 6 4. 并发身份推断：`calendar event list` + `participant list` 取日程参与人（最高优先） & `aisearch person` & `chat message list` & `drive search` `& wait`；未找到同时段日程 → 引导用户提供日程链接或参会人名单；两路以上一致才下结论 5. 置信度判断：>70% 直接输出；≤70% 展示 TOP3 候选让用户选（最多一次） 6. 结构化总结输出（核心观点 + 问题 + Action Item + 立场）→ 追问是否替换发言人标注。详见 [10-minutes-speaker-match.md](./10-minutes-speaker-match.md)                                                                                                                                                             |
| browse-by-tag   | 1. `minutes tag list` → 获取用户标签/分组列表（含 tagId 和名称） 2. 按用户指定的标签名称匹配 tagId（若用户直接提供 tagId 则跳过 Step 1） 3. `minutes tag query --tag-id <tagId>` → 返回该标签下的听记列表（支持 `--limit`/`--cursor` 分页） 4. 展示结果，按需调用 `get summary`/`get info` 获取详情                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| speaker-correct | 1. **查同时段日程**：从听记元数据获取录音开始时间 → `calendar event list`（前后 30 分钟内）→ 若匹配到日程则提取参会人名单并展示 2. **展示发言人状态表格**：列出所有发言人及标注状态（已识别 / 未标注 / 未标注，发言较少） 3. **让用户选择识别方式**：听音识人（裁剪代表性音频片段让用户辨认）/ 手动设置（用户直接告知对应关系）/ 智能匹配（结合参会人名单和发言内容推断） 4. **执行替换**：确认对应关系后统一搜人获取 dingUid → `dws aisearch person --keyword "<姓名>" --dimension name --format json` → 取 userId（长整型）→ `dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "<姓名>" --target-uid <userId> --format json`；多个匹配时列出候选让用户选；无匹配时执行不带 `--target-uid` 的替换 5. 替换成功后追问「还有其他发言人需要帮你识别和替换吗？」详见 [11-minutes-speaker-correct.md](./11-minutes-speaker-correct.md)                                            |

---

### 听记场景执行规范（避坑指南）

> 以下规范基于历史高频失败模式总结，**必须严格遵守**。

#### 0. 选对象铁律（命令对了但选错听记 = 整任务失败，0605 P2 EDD badcase 提炼）

> 这是历史任务中**最致命**的一类失败：命令写得完全正确，但 list/搜索拿到结果后**选错了听记对象**，导致后续所有 `get summary` / `get keywords` / `update` 全部作用在错误条目上。详见 [minutes.md](./minutes.md)「选对象铁律 S1~S6」，速记如下：

| 铁律 | 场景 | 正确做法 | 真实 badcase |
|------|------|----------|------------|
| **S1 跨组织汇总** | 「帮我看看我所有听记」涉及多个组织 | 以 list 返回的 `taskUuid + title + organizationName` 三元组**逐条照抄**，组织与听记不可张冠李戴，禁止按标题相似度乱配 | 0028：把「12月文档切片讨论」误识别为「晨会427」 |
| **S2 最近一次某类会议** | 「最近一次周会」 | 先 `--query "周会"` 过滤出**该类会议**，再在候选里按时间取最新；**主题匹配优先级高于时间** | 0026：选成普通的「2026-06-02 录音」而非周会纪要 |
| **S3 比时长最长** | 「最长的那条听记」 | 必须读 `durationMicros`（微秒）字段做数值比较取最大；**口头结论与操作的 taskUuid 必须自洽** | 0030：声称 A 最长却操作了 B |
| **S4 内容为空** | get summary/keywords 返回空 | 锁定 taskUuid 后所有 get/update **复用同一 id**；某字段为空就**如实告知**，禁止偷偷切换到另一条听记 | 0020：第一条空就擅自换另一条展示 |
| **S5 模糊日期匹配不到** | 「4月9日周会纪要」实际创建于 4.14 | 日期可能是"会议主题日期"而非"创建日期"，按标题关键词搜，精确日期没命中就放宽 ±7 天/同主题候选请用户确认，**禁止直接报"找不到"** | 0030：按 4-09 精确匹配导致漏掉 |
| **S6 给标题没给 id** | 「把'X'这条听记标题改成'Y'」 | **必须先** `list all --query "<标题关键词>"` 定位出唯一 taskUuid 再 update/get，禁止凭记忆直接填 `--id` 跳过定位 | 0012：跳过 list 直接用已知 id 改标题 |

> **批量场景补充（0030 P2 badcase 提炼）**：用户一次性要查多条听记详情时，定位出各 taskUuid 后应优先用 `minutes get batch --ids <uuid1,uuid2,...>` 批量查询（见 browse-minutes recipe），而非逐个 `get`。**批量接口容错铁律**：
> - 部分条目权限失败时，**展示成功条目的完整信息 + 列出失败条目的 id 与原因**，不要整批放弃改为逐个查询
> - 若 `get batch` 整体报错（如权限/参数问题），**降级为逐个 `get info`**，但必须在最终输出中说明"批量接口不可用，已逐个查询"
> - 逐个查询时仍需保证**所有条目都尝试过**，不能因某一条失败就放弃后续条目
>
> **批量定位 + 模糊匹配组合策略（0030 实战链路）**：
> ```
> 用户: "帮我批量查一下这几条听记的详情：4月9日周会纪要、4月项目自动化周报、【内部会议】测试报告与多模型适配进展"
>
> Step 1: 逐个关键词搜索定位
>   dws minutes list all --query "周会纪要" --format json        → 若命中则取 taskUuid
>   dws minutes list all --query "项目自动化周报" --format json   → 取 taskUuid
>   dws minutes list all --query "测试报告与多模型适配" --format json → 取 taskUuid
>
> Step 2: 某条搜不到时的回退（S5 铁律）
>   "4月9日周会纪要" 按 --query "周会纪要" 未命中 → 放宽搜索：
>   dws minutes list all --query "周会" --start "2026-04-02T00:00:00+08:00" --end "2026-04-16T23:59:59+08:00" --format json
>   → 列出候选让用户确认，禁止直接报"找不到"
>
> Step 3: 全部定位后批量查询
>   dws minutes get batch --ids <uuid1,uuid2,uuid3> --format json
> ```

#### 1. 空数据场景处理（最高频失败原因）

`minutes list` 返回空列表时的正确行为：

```
正确做法：
1. 调用 dws minutes list（mine/shared/all 均尝试）
2. 若全部返回空 → 明确告知用户「当前没有找到听记记录」
3. 给出替代建议（如：检查是否有进行中的会议、确认听记权限等）

错误做法：
- 沉默放弃，不给用户任何反馈
- 反问用户「你有听记吗？」而不先主动查询
- 在无数据时仍尝试调用 get summary / get transcription（会因缺少 taskUuid 报错）
```

#### 2. 钉钉文档链接读取（严禁误用工具）

当用户提供 `https://alidocs.dingtalk.com/...` 链接时：

```
正确做法：
dws doc read --url "https://alidocs.dingtalk.com/i/nodes/xxx" --format json

严禁使用：
- read_file 打开钉钉 URL（会返回登录页 HTML）
- read_ali_doc（这是 Agent 自身的文档读取工具，非 dws 命令）
- web_fetch / browser_use（无法访问需登录的内部文档）
```

#### 3. 核心链路执行原则

| 场景 | 完整链路 | 无数据时的最低要求 |
|------|---------|------------------|
| 录制转写 | `minutes list` → `get transcription` | 调用了 `minutes list` + 明确告知无数据 |
| 内容读取 | `minutes list` → `get summary` / `get transcription` | 调用了 `minutes list` + 明确告知无数据 |
| 内容总结 | `minutes list` → `get summary`（多条）→ 提炼 | 调用了 `minutes list` + 明确告知无数据 |
| 待办提取 | `minutes list` → `get todos` / `get summary` | 调用了 `minutes list` + 明确告知无数据 |
| 原文文件导出 | `minutes list all --start --end` → 对每条听记 `get transcription` 翻页拉全 → 交付真实原文 | 调用了 `minutes list` + 明确告知无数据；禁止用 summary 替代 |
| 链接解析 | `dws doc read --url <url>` | 调用了 `dws doc` 命令 + 告知错误原因 |
| 聚焦原话/沟通细节 | `minutes list all` → `get transcription`（翻页拉全） | 禁止仅凭 summary 出稿 |
| 聊天消息中听记链接 | 解析 `minutesId` → `minutes get summary/transcription` | 禁止把听记链接降级为关键词 |
| 按标签筛选听记 | `minutes tag list` → 匹配 tagId → `minutes tag query --tag-id <tagId>` | 调用了 `tag list` + 告知无标签或无结果 |

#### 3.5 听记取数深度约束（0609 点踩 case 提炼）

> 以下约束覆盖周报/日报/汇报等需要从听记提取内容的全部场景。详细说明见 [minutes.md](./minutes.md)。

| 约束 | 说明 | 真实 badcase |
|------|------|------------|
| **转写原文硬约束** | 用户诉求含「聚焦原话/逐字/沟通细节」等词时，**必须先调 `get transcription` 翻页拉全**，禁止仅凭 summary 出稿 | 模型以"通话短摘要够用"跳过转写，输出泛化套话（457c3f0） |
| **数据源下钻** | 听记维度**必须 `get summary`（或 `get transcription`）读正文**，严禁只取标题列表；scope 用 `all`；空时换窗重试或标注 | 听记只取标题不调 get summary，周报严重不完整（76f1d42） |
| **听记链接解析** | 聊天消息中遇到听记链接（`flash_minutes_detail`/`SHANJI`）→ 解析 `minutesId` → 调 `minutes get summary/transcription` | 群里日会是听记链接却不读正文，日报缺失（7b8e002） |
| **忠实性约束** | 源数据无某要素（行动项/责任人/数字）时禁止生成；统计字段基于实际取数计数 | 取数成功但篡改成带责任人的 ActionItems、虚报文档数（14e3365） |
| **多源全覆盖** | 用户枚举多数据源时每个来源都必须调对应工具；瞬时错误重试；如实声明缺失来源禁编无来源数字 | 4 类数据源点名，chat/minutes 完全没调，编精确数字（f3197dd） |

#### 4. 通用规范

- **所有 dws 命令必须带 `--format json`**，确保输出可解析
- **参数错误重试不超过 3 次**：同一命令因参数错误失败后，最多调整参数重试 3 次，超过则告知用户并给出替代方案
- **不要要求用户上传音频文件**：听记场景应通过 `minutes list` 获取已有记录，而非要求用户提供文件
- **有时间线索时优先使用 `--start` / `--end` 服务端过滤**：如「昨天的听记」应计算昨日时间范围并传参，而非全量拉取后客户端过滤
