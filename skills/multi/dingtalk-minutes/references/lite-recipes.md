# minutes Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #7 听记与会后

> 产品命令完整参考见 [minutes.md](./minutes.md)。full recipe 见 [07-minutes.md](../../dingtalk-minutes/references/07-minutes.md)。

### minutes-query（查询与获取）

> **scope 选择铁律（P2 真实 badcase）**：`list` 后的 scope 决定查询范围，最高频误判是把"我能访问的所有听记"错选成 `mine`：
> - `mine` = **仅我自己创建/发起**的听记（范围最窄）
> - `shared` = **仅他人共享给我**的听记
> - `all` = **我可访问的全部**（= mine ∪ shared，范围最广）
> - **判定口诀**：query 含"访问/权限/可见/能看到/所有/我的"等覆盖范围语义 → 一律走 `all`；**仅当**明确说"我创建的/我发起的/我录的" → 才走 `mine`。**不要因为句子里有"我"字就退化成 `mine`。**
> - 错误：`我能访问的所有听记` → `list mine`（漏掉共享给我的，判定不通过）
> - 正确：`我能访问的所有听记` → `dws minutes list all --format json`

> **选对象铁律（0605 P2 EDD badcase 提炼，命令对了但选错听记 = 整任务失败）**：list/搜索拿到结果后，必须按语义精准锁定目标听记，详见 [minutes.md](./minutes.md)「选对象铁律 S1~S6」。速记：
> - **S1 跨组织汇总**：以 list 返回的 `taskUuid + title + organizationName` 三元组为准逐条照抄，组织与听记不可张冠李戴。
> - **S2 "最近一次某类会议"**：先 `--query "<主题词>"`（如周会）过滤出该类，再在候选里取时间最新；主题匹配优先级高于时间。
> - **S3 比时长最长**：必须读 `durationMicros` 字段做数值比较，禁止凭印象/标题猜，口头结论与操作的 taskUuid 须自洽。
> - **S4 内容为空**：锁定 taskUuid 后所有 get/update 复用同一 id；某字段为空就如实说，**禁止偷偷切换到另一条听记**。
> - **S5 模糊日期匹配不到**：日期可能是"会议主题日期"而非"创建日期"，按标题关键词搜，精确日期没命中就放宽 ±7 天/同主题候选请用户确认，**禁止直接报"找不到"**。搜索回退策略：① 先 `--query "<主题关键词>"` 不带日期搜 → ② 若结果过多则加 `--start/--end` 扩大到 ±7 天 → ③ 列出候选让用户确认。
> - **S6 给标题没给 id**：必须先 `list all --query "<标题关键词>"` 定位 taskUuid 再 update/get，禁止凭记忆直接填 `--id` 跳过定位。

**列表查询**（`list` 后**必须**跟 scope：`mine`/`shared`/`all`，默认补 `all`）：

```bash
# 我可访问的所有听记（默认）
dws minutes list all --format json
# 按关键词服务端搜索（严禁全量拉取后本地 grep）
dws minutes list all --query "周会" --format json
# 按时间范围筛选（ISO-8601 格式）
dws minutes list mine --start "2026-05-01T00:00:00+08:00" --end "2026-05-25T23:59:59+08:00" --format json
# 关键词 + 时间组合
dws minutes list all --query "需求评审" --start "2026-05-25T00:00:00+08:00" --end "2026-05-25T23:59:59+08:00" --format json
# 限制条数
dws minutes list mine --limit 5 --format json
# 共享给我的听记
dws minutes list shared --query "ROI" --format json
```

| 参数 | 说明 |
|------|------|
| `--query "<关键词>"` | 服务端关键词搜索 |
| `--start "<ISO-8601>"` | 开始时间 |
| `--end "<ISO-8601>"` | 结束时间 |
| `--limit <N>` | 每页条数，默认 10（`--max` 为兼容别名） |
| `--cursor "<token>"` | 分页 token，首页留空（`--next-token` 为兼容别名） |

**获取详情**：

- 批量基础信息：`minutes get batch --ids <uuid1,uuid2,...>`
- 单篇摘要：`minutes get summary --id <taskUuid>`
- 转写原文（自动翻页）：`minutes get transcription --id <taskUuid>`（返回 `nextToken` 时用 `--next-token <token>` 继续）
- 关键词：`minutes get keywords --id <taskUuid>`
- 待办事项：`minutes get todos --id <taskUuid>`
- 基础信息：`minutes get info --id <taskUuid>`
- 音频地址：`minutes get audio --id <taskUuid>`

> `--id`/`--uuid`/`--task-uuid` 三者等价。推荐 `--id`。

### minutes-edit（编辑与替换）

- **替换转写文字**：`minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字"`
  - 执行前检查特殊字符（引号/书名号/括号等），若包含先提示用户确认去除
  - 替换成功后追问是否加热词：`minutes hot-word add --words "新文字"`
- **替换发言人**：先统一搜人取得 dingUid → `minutes speaker replace --id <taskUuid> --from "发言人X" --to "姓名" --target-uid <userId>`
  - 查询 dingUid：`aisearch person --keyword "姓名" --dimension name --format json` → 取 `userId`
  - 多个匹配 → 列出候选让用户选；无匹配 → 不带 `--target-uid` 执行
- **修改标题**：`minutes update title --id <taskUuid> --title "新标题"`
- **修改摘要**：`minutes update summary --id <taskUuid> --content "新内容"`
- **热词管理**：`minutes hot-word add --words "词1,词2"` / `minutes hot-word list`
- **思维导图**：`minutes mind-graph create --id <taskUuid>` → `mind-graph status --id <taskUuid>` 轮询至完成

### minutes-tag（标签/分组查询）

- 查询标签列表：`minutes tag list` → 返回用户在听记页面创建的所有标签/分组（含 tagId 和名称）
- 按标签查听记：`minutes tag query --tag-id <tagId> [--limit 20] [--cursor <token>]`
  - tagId 来自 `tag list` 返回值，不可编造
  - 支持分页，`--cursor` 传入上一次返回的 nextToken

**典型链路**：用户说"帮我看看'周会'标签下的听记" →
1. `dws minutes tag list --format json` → 按名称匹配找到 tagId
2. `dws minutes tag query --tag-id <tagId> --format json`

### minutes-permission（权限管理）

- 添加成员：`minutes permission add --ids <uuid1,uuid2> --member-uids <uid1,uid2> --policy 4`
  - 需先通过 `aisearch person --keyword "<姓名>" --dimension name` 获取目标 userId
  - policy：0=不可见 / 1=仅查看 / 2=查看+下载 / 3=查看+下载+编辑 / 4=全部权限
- 移除成员：`minutes permission remove --ids <uuid1,uuid2> --member-uids <uid1,uid2>`

### minutes-upload（音频上传）

```bash
# 创建上传会话
dws minutes upload create --file-name "meeting.mp3" --file-size 61565431 --format json
# 上传完成后确认
dws minutes upload complete --session-id <sid> --format json
# 取消上传
dws minutes upload cancel --session-id <sid> --format json
```

### 最佳实践案例速查（详见 [minutes.md](./minutes.md)）

| 案例 | 场景 | 正确链路 |
|------|------|----------|
| 案例 1 | 听记 URL + 创建思维导图 | 提取 taskUuid → `mind-graph create` → `mind-graph status` 轮询；**禁止**走 app-development 或前端库 |
| 案例 2 | 替换文字后未引导热词 | 检查特殊字符 → `replace-text` → 追问加热词 `hot-word add` |
| 案例 3 | 查听记拉了不必要的转写 | 用户只要列表 → `list` 即可，**不要**自动拉 `get transcription` |
| 案例 4 | 拉完转写只输出时间线原文 | 拉完后追问按发言人聚类 → 引导匹配 → 调用 `speaker replace` 写回 |
| 案例 5 | 查某人说了什么不引导替换 | 推断发言人 → **用户确认** → 结构化总结 → 引导 `speaker replace` |
| 案例 6 | 通讯录+部门+转写三路印证 | Step 3 画像 + Step 4 `aisearch person` 并发 → 置信度 ≥70% → 确认 → 替换 |
| 案例 7 | grep 花名误判未参会 | **禁止**在转写文本里 grep 人名判参会；**必须**调 `aisearch person` |
| 案例 8 | 听记类 query 不走 dws | **禁止**用 session_search/browser_use/activity:search 替代 dws；模糊请求先 `list mine` |
| 案例 9 | 按标签筛选听记 | `tag list` → 按名称匹配 tagId → `tag query --tag-id <tagId>`；**禁止**编造 tagId |

### 听记取数深度约束（0609 点踩 case 提炼）

> 详细说明见 [minutes.md](./minutes.md)。

- **转写原文硬约束**：用户诉求含「聚焦原话/逐字/沟通细节/具体讨论了什么」等词时，**必须先调 `get transcription` 翻页拉全**，禁止仅凭 summary 出稿
- **数据源下钻**：听记维度**必须 `get summary`（或 `get transcription`）读正文**，严禁只取标题列表；scope 用 `all`；空时换窗重试或标注
- **听记链接解析**：聊天消息中遇到听记链接（`flash_minutes_detail`/`SHANJI`）→ 解析 `minutesId` → 调 `minutes get summary/transcription`，禁止把链接降级为关键词
- **忠实性约束**：源数据无某要素（行动项/责任人/数字）时禁止生成；统计字段基于实际取数计数，不得编造
- **多源全覆盖**：用户枚举多数据源时每个来源都必须调对应工具；瞬时错误重试；如实声明缺失来源，禁编无来源数字

### 间接意图识别铁律

query 未提"听记"但任务产出依赖会议讨论内容时（报告/总结/日报/复盘/商业分析/市场感知），听记采集是**必跑前置步骤**：

1. **铁律 A**：任务含"会议/讨论/沟通"信息需求 → `dws minutes list` 必跑
2. **铁律 B**：用户说"文档啥也没有" → 听记优先级更高（唯一结构化数据源）
3. **铁律 C**：多源聚合场景 → 每个被提及的数据源都必须有采集动作，听记侧 0 调用 = 严重失败
