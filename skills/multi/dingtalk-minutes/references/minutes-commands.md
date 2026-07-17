# 听记命令参考

## 命令总览

### 查询我创建的听记列表
```
Usage:
  dws minutes list mine [flags]
Example:
  dws minutes list mine
  dws minutes list mine --limit 10
  dws minutes list mine --limit 10 --cursor <nextToken>
  dws minutes list mine --query "周会"
Flags:
      --limit float       每页数据条数 (默认 10)
      --cursor string     分页 token (首页留空，后续填写前次返回的 nextToken)
      --query string      关键字筛选 (可选)
      --start string      开始时间 ISO-8601 (可选)
      --end string        结束时间 ISO-8601 (可选)
```

查询我创建的听记列表，支持 `--limit` 和 `--cursor` 分页，支持按关键字和时间范围筛选。

### 查询他人共享给我的听记列表
```
Usage:
  dws minutes list shared [flags]
Example:
  dws minutes list shared
  dws minutes list shared --limit 20
  dws minutes list shared --limit 5 --cursor <nextToken>
Flags:
      --limit float       每页数据条数 (默认 10)
      --cursor string     分页 token (首页留空，后续填写前次返回的 nextToken)
      --query string      关键字筛选 (可选)
      --start string      开始时间 ISO-8601 (可选)
      --end string        结束时间 ISO-8601 (可选)
```

查询他人共享给我的听记列表，支持 `--limit` 和 `--cursor` 分页，支持按关键字和时间范围筛选。

### 查询我有权限访问的所有听记列表
```
Usage:
  dws minutes list all [flags]
Example:
  dws minutes list all
  dws minutes list all --limit 20
  dws minutes list all --query "周会" --limit 20
  dws minutes list all --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00"
  dws minutes list all --limit 10 --cursor <nextToken>
Flags:
      --end string        结束时间 ISO-8601 (可选)
      --query string      关键字筛选 (可选)
      --limit float       每页数据条数 (默认 10)
      --cursor string     分页 token (首页留空，后续填写前次返回的 nextToken)
      --start string      开始时间 ISO-8601 (可选)
```

查询我有权限访问的所有听记列表（包括我创建的、他人共享给我的等所有有权限的听记）。支持按关键字和时间范围筛选。时间范围和关键字为可选参数，不传则返回所有有权限的听记。支持使用 `--limit` 和 `--cursor` 进行分页查询。

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

**发言人信息（按需读取）**：

`get info` 只返回听记元信息，不要求额外读取转写。仅当用户询问发言人列表、某人的发言、身份识别或姓名纠偏时，再读取转写或调用 speaker summary：

1. 从 `speakerName` / `speakerId` 收集请求范围内的不同发言人。
2. 已有真实姓名时直接展示；匿名标识统一展示为「发言人 1」「发言人 2」，不要暴露 `Speaker_X` 等内部格式。
3. 需要识别匿名发言人时，可按用户提供的映射、日程参会人或 [智能匹配流程](10-minutes-speaker-match.md) 生成候选；任何 `speaker replace` 写回都必须先展示映射并取得用户确认。
4. 普通详情、摘要、关键词或待办请求不触发发言人识别流程。

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
  dws minutes get transcription --id <taskUuid> --cursor <nextToken> --format json
Flags:
      --direction string   排序方向: 0=正序, 1=倒序 (默认 0)
      --id string          听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
      --cursor string      分页 token，首次查询留空，后续填写前次返回的 nextToken
```

每条记录包含: 发言人信息、转写文本、对应时间戳

**关键：转写接口每次最多返回 50 段，必须自动翻页才能拿到完整原文！**

> **这是最高频的 silent failure：** `get transcription` 单次调用**最多只返回 50 段**。一篇 30 分钟会议的转写通常有 150-400 段，如果不翻页就只能看到前 1/3 甚至 1/8 的内容。
>
> **分页机制（必须掌握）：**
> - 首次调用：`dws minutes get transcription --id <uuid> --format json`（不传 `--cursor`）
> - 检查返回 JSON 中是否包含 `nextToken` 字段（非空字符串）
> - 如果有 `nextToken`：继续调用并传入 `--cursor <上次返回的nextToken值>`
> - 循环直到返回中**不再包含 `nextToken`**（或 `nextToken` 为空），表示所有段落已拉取完毕
> - **拼合所有页的段落**后，才算拿到了完整转写原文
>
> **自动翻页伪代码（AI 必须内化此逻辑）：**
> ```
> all_paragraphs = []
> next_token = ""           # 首次为空
> loop:
>     if next_token == "":
>         result = dws minutes get transcription --id <uuid> --format json
>     else:
>         result = dws minutes get transcription --id <uuid> --cursor <next_token> --format json
>     all_paragraphs.append(result.paragraphs)
>     next_token = result.nextToken   # 可能为空或不存在
>     if next_token 为空或不存在:
>         break   # 全部拉取完毕
> # 此时 all_paragraphs 才是完整转写原文
> ```
>
> **典型错误（导致“总结整篇听记”只覆盖前 50 段）：**
> - [错误] 只调用一次 `get transcription`，看到返回了内容就以为拿全了 → 实际只有前 50 段
> - [错误] 看到返回 JSON 里有 `nextToken` 字段但不知道这是什么、不知道要传 `--cursor` → 默默丢弃了后续页
> - [错误] 用 `--help` 查参数但在 Windows 下输出被截断，没看到 `--cursor` 参数 → 以为不支持分页
> - [正确] **正确做法：无条件按上述循环逻辑自动翻页，直到 nextToken 为空**

**转写原文拉取策略：**

- 用户要求完整原文、完整逐字稿或基于全文分析时，按 `nextToken` 连续翻页直到 token 为空，再合并所有段落。
- 用户只要求摘要、关键词、待办或基础信息时，不读取转写。
- 用户只要求特定发言人或特定时间段时，优先使用 speaker summary 或只拉取覆盖目标范围所需的数据。
- 不能把第一页当成全文，也不能用未覆盖目标范围的部分转写下完整结论；结果输出按用户目标摘要或提取，避免无差别粘贴全文。
>
> **用户意图为"按发言人分析"时的优化路径：**
> - 如果用户的最终目的是"区分发言人"/"看某人讲了什么"，**不一定需要拉取全部转写原文**
> - 可优先使用 `dws minutes get summary --id <uuid>` 获取结构化摘要（含发言人信息）
> - 或使用 `dws minutes speaker summary get --ids <uuid>` 直接获取按发言人维度的摘要
> - 仅当用户明确要求"逐字稿"/"完整原文"/"每句话"时，才需完整翻页拉取

2. **用户未明确要求查看原文时 → 不要主动拉取转写原文**
   - 示例："查一下和悟空相关的听记"、"帮我看看这个听记的摘要"、"这个会议讲了什么"
   - 这些场景下用户的意图是查列表、看摘要等，**不需要也不应该**把大量转写原文全部拉出来，否则会造成信息过载和不必要的性能开销
   - 如果用户问"这个会议讲了什么"，应优先使用 `get summary` 返回摘要，而非 `get transcription`

**判断原则：只有用户的意图明确指向"原文/转写/逐字稿/录音文字"时，才调用 `get transcription`；其他场景（查列表、看摘要、看待办等）严禁自动附带拉取转写原文。**

**重要 — 转写原文返回后默认按"时间线"组织各段落，AI 必须主动引导发言人聚类与关联（必须严格遵守）：**

`get transcription` 默认返回的段落是按"时间戳正序/倒序"穿插展示的（每条记录包含 `speakerNick + 时间戳 + 文本`），**对人不友好**——同一发言人的内容散落在多个时间点，难以快速看清"某个人主要讲了什么"。因此，AI **在拉取完成（含分页全部完成）后**必须主动启动以下"发言人聚类 → 关键词模糊匹配 → 引导确认 → 调用 `speaker replace`"四阶段工作流：

#### 阶段 1：拉取完成后主动询问"是否按发言人聚类"
拉取（含自动翻页）结束后，AI 必须**主动**追问用户一次（不要默认强制聚类，避免用户只想看时间线原文时被打扰）：

> "已拉取完整转写原文（共 N 段，X 个发言人）。当前默认按时间线返回。是否需要我帮你**按发言人分组聚类**，并提取每位发言人的**核心发言要点**？"

- 用户确认（"好/可以/需要/聚类一下/按发言人分一下"等）→ 进入阶段 2
- 用户拒绝 → 直接展示时间线原文，结束流程，**不再追问**

#### 阶段 2：按发言人聚类 + 提取核心内容
AI 基于已拉取的转写数据，**本地完成**聚类与摘要（无需新调用 dws 命令），输出结构如下：

```
[发言人] 发言人1（共 12 段，约 1820 字）
   核心要点：
   - 介绍 Q3 战略规划与组织调整方向
   - 强调 AI 化转型的三个关键里程碑
   - 提出对供应链效率的具体改进目标

[发言人] 发言人2（共 8 段，约 960 字）
   核心要点：
   - 汇报当前业务的财务数据与利润率
   - 分析竞品在华东市场的最新动作

[发言人] 张三（共 5 段，约 410 字）
   核心要点：
   - 同步研发团队的招聘进度
   - 提出对测试资源的支持诉求
```

**约束：**
- 聚类必须包含**全部发言人**（包括"发言人1/发言人2"占位符与已关联真实姓名的发言人，便于后续替换）
- 每位发言人**最多列出 3-5 条**核心要点，避免冗长
- 输出后必须**主动追问**用户：

> "如果你能告诉我『某某人主要讲了什么』（例如『李总主要讲了战略规划』『王经理主要负责供应链』），我可以根据关键词帮你**自动匹配**对应的发言人，并把『发言人1/发言人2』替换成真实姓名。"

#### 阶段 3：基于用户提供的关键词做模糊匹配
当用户提供形如『某某人主要讲了 XX』的输入时（例如『李总主要讲了战略规划』『拾光负责供应链效率改进』），AI 必须：

1. **提取关键词**：从用户输入中抽取核心实体/主题词（如『战略规划』『供应链效率』『AI 化转型』），可允许多关键词
2. **在阶段 2 已聚类的发言人核心要点中做模糊匹配**：
   - 优先匹配「核心要点」中包含或语义相近的发言人
   - 必要时回看该发言人的原始转写文本进行二次确认
   - 支持**同义词/近义词**容忍（如『战略』~『规划』、『供应链』~『物流』）
3. **匹配结果分级处理：**
   - **唯一高置信匹配**（一个发言人显著命中）→ 进入阶段 4 引导确认
   - **多个候选**（2 个及以上发言人都有部分命中）→ 列出全部候选，让用户选择，例如：
     > "根据关键词『战略规划』，我匹配到 2 个可能的候选：① 发言人1（命中『战略规划/AI 化转型』）② 发言人3（命中『战略方向』）。你说的『李总』更可能是哪一位？"
   - **无匹配** → 如实告知用户，并请其补充更具体的关键词或直接给出"发言人编号 → 真实姓名"的映射，例如：
     > "暂未在转写中匹配到与『战略规划』强相关的发言人。可以再描述得更具体一些，或者直接告诉我『发言人 X 就是李总』，我来帮你替换。"

#### 阶段 4：引导用户确认关联，并调用 `speaker replace` 完成替换
匹配到唯一候选后，AI **不要直接替换**，必须先**显式追问用户确认**：

> "我找到『发言人1』很可能就是你说的『李总』（命中关键词：战略规划、AI 化转型）。是否需要我把这篇听记里的『发言人1』全部替换为『李总』？
>  确认后我会执行：`dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总"`"

- 用户确认 → 立即调用 `dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总" --format json`，并在执行成功后告知用户："已将本篇听记的『发言人1』全部替换为『李总』，纪要与待办中的发言人也已同步更新。"
- 用户希望同时关联通讯录 → 引导用户提供钉钉 UID，调用时附加 `--target-uid <uid>`
- 用户拒绝 → 不替换，可询问是否还有其他发言人需要关联，否则结束流程

**严禁的行为：**
- [禁止] 拉取完转写后直接输出大段时间线原文就结束，不主动引导聚类
- [禁止] 用户提供"某某人讲了 XX"后，AI 自己默认替换而不向用户二次确认
- [禁止] 未执行 `speaker replace` 就把映射视为已写回听记
- [禁止] 模糊匹配置信度不足时仍然给出唯一答案，不让用户参与挑选候选

#### 发言人识别与总结执行链路（用户指定人名查发言时必须遵循）

**触发条件**：用户同时提供了听记来源（URL/时间/关键词）和一个具体人名，目标是获取该人在会议中说了什么。例如"帮我看看这个听记里张三说了什么""李总在今天的会上提了哪些观点"。

> **【重要】** **核心铁律（必须刻在脑子里，违反任何一条都视为严重错误）：**
>
> **铁律 1：严禁在转写文本里 grep "目标人名" 字符串作为存在性判断依据**
> - 花名/真名通常**不会**出现在 TA 自己的发言里，发言里出现的"X"只意味着"`说话人 ≠ X`"或"说话人在叫 X"
> - 在转写中搜不到"灵麦"**完全不能得出"灵麦没参会"的结论**——99% 的发言人都显示为匿名编号"发言人1/2/3"
> - 正确做法是把"目标人名"作为**身份信息**去通讯录查询（Step 4 ①），而不是当作发言内容字符串去 grep
>
> **铁律 2：严禁用 AI 摘要里的"参与人/参会人"字段判断某人是否参会**
> - AI 摘要中的"参与人"字段往往**只截取最显著的 1-2 个名字**（通常是创建人或发言最多的人），**不是完整参会人列表**
> - "AI 摘要参与人只有故愚 → 灵麦没参会" 是典型的错误推理链
> - 如果一定要列出参会人，应使用 `dws minutes get info` / `get batch` 返回的 `participants` 字段，而不是摘要文本里的描述
>
> **铁律 3：通讯录查询是 Step 4 的强制起点，不依赖 Step 3 是否成功**
> - Step 2 一旦返回"未命中真名标注"，**Step 3 与 Step 4 ① 必须并发触发**，不要等 Step 3 失败再补查
> - 通讯录查询 1 个调用就能锁定"目标人物的部门 + 职级 + 上级"——这是身份推断**最低成本、最高收益**的信号源
> - 跳过通讯录直接得出"找不到 X"的结论 = 100% 失败链路
>
> **铁律 4：找不到字面匹配 ≠ 不存在**
> - "发言人识别"功能存在的本质就是**根据角色特征把匿名编号映射到真实人**——"找不到字面匹配"恰恰是这个功能要解决的问题，而不是退出的理由
> - 一旦想说"找不到 X"前，先自问：通讯录查了吗？文档查了吗？聊天记录查了吗？基于角色在转写里做模式匹配了吗？四个全是"没"则禁止给"找不到"的结论

**完整执行链路：**

```
Step 1: 定位听记并读取转写原文
    ↓
Step 2: 声纹标注检查
    ├─ [目标人名已被系统标注] → 直接跳 Step 6
    └─ [仅有匿名编号"发言人1/2/3"] ↓
Step 3: 转写原文内推断（优先，能判断就不走外部查询）
    ├─ [高置信命中] → 直接跳 Step 6
    └─ [无法确定] ↓
Step 4: 多路并发身份推断
    ↓
Step 5: 定向匹配 + 置信度分支
    ├─ 置信度 ≥ 50%  → 展示文本片段请用户确认
    └─ 置信度 < 50%  → 提供候选片段请用户辨认
    ↓
Step 6: 结构化总结输出
    ↓
Step 7: 引导用户替换发言人（调用 speaker replace 写回听记）
```

##### Step 1: 定位听记并读取转写原文

- **有 URL** → 从 URL 提取 taskUuid → `dws minutes get transcription --id <uuid> --format json`（自动翻页拉取全部）
- **有时间/关键词** → `dws minutes list all --query "关键词" --start "..." --end "..." --format json` 筛选 → 获取 taskUuid → 拉取转写
- **什么都没给** → **必须询问用户**提供听记链接、时间或关键词

##### Step 2: 声纹标注检查

检查转写原文中目标人物是否已被系统识别并标注了真实姓名（即 `speakerNick` 直接就是用户提到的人名）：
- **已标注**（某条 `speakerNick` 字面就是"木兰"）→ 直接跳到 Step 6，零确认步骤
- **仅有匿名编号**（发言人1/发言人2/发言人3）→ **必须**继续 Step 3 与 Step 4，**严禁在此处退出**

> **【重要】** **关键认知（违反则案例 7 重现）：**
> - **未命中真名标注 ≠ 目标人物没参会**——绝大多数听记的发言人都是匿名编号，"未命中"是**默认场景**，恰恰是发言人识别功能要解决的问题
> - **不要在转写文本里 grep 目标人名**作为存在性判断——花名/真名通常不会出现在 TA 自己的发言里（参见铁律 1）
> - **不要把 `dws minutes get summary` 摘要文本里写到的"参与人"当作完整参会列表**——AI 摘要里的"参与人"字段只截取最显著的 1-2 个名字，不是完整名册（参见铁律 2）
> - 唯一能下"目标人物没参会"结论的场景是：`get info` / `get batch` 返回的结构化 `participants` 字段里**完整列出参会人且不含目标**，且 `dws contact user search` 也搜不到该人 → 才允许告知用户"该人不在参会列表"

##### Step 3: 转写原文内推断（优先）

**核心原则：先基于转写原文做逻辑推断，能直接判断就不调外部接口。**

充分利用转写文本中的所有可用信息进行综合推断：
- **称呼线索**：其他人称呼"张总/李工/王老师"等
- **自我介绍**："我是 XX 部门的""我负责 XX"
- **上下文指代**：前文提到"张三你来说一下"，紧接着的发言人大概率是张三
- **发言内容特征**：用户说"李总负责战略"，而某发言人大量讨论战略方向
- **发言顺序**：主持人/领导通常先发言或总结性发言

只要能从原文**高置信度**地确定发言人，直接跳 Step 6 输出总结。如果仅凭原文无法确定，继续 Step 4。

##### Step 4: 多路并发身份推断

> **【重要】** **强制规则（违反则案例 7 重现）：**
> - **路径 ① 通讯录查询是必跑项，不依赖 Step 3 是否成功**——Step 2 一旦判定"未命中真名标注"，**Step 3 与 Step 4 ① 必须并发触发**，**严禁等 Step 3 失败再补查**
> - 通讯录查询单次调用即可拿到"部门 + 职级 + 上级 + 真名"——这是身份推断**最低成本、最高收益**的信号源
> - 路径 ②③④ 是**增量信号**，按需触发（如通讯录返回的部门是"产品设计部"这类多角色部门时，再补查 ② 文档）

**触发顺序：**

| 阶段 | 必跑 / 可选 | 触发时机 |
|------|-------------|----------|
| 路径 ① 通讯录组织架构 | **必跑** | Step 2 判定"未命中真名"后立即并发触发，与 Step 3 同时进行 |
| 路径 ② 本人创建的文档 | 可选 | ① 返回的部门是"产品设计部"这类多角色部门，需要更精确的角色信号时 |
| 路径 ③ 近期日程类型 | 可选 | ① ② 都不充分，需要补充职能边界判断时 |
| 路径 ④ 聊天记录 | 可选 | ①②③ 都不充分，需要语言风格/工作内容线索作为最后一道印证时 |

| 路径 | 命令 | 得到什么 |
|------|------|----------|
| ① 通讯录组织架构 | `dws contact user search --query "目标人名"` → 部门/职级/上级/真名 | 职能大类（技术/产品/设计/管理）+ 是否存在该人 |
| ② 本人创建的文档 | `dws doc search --query "目标人名/真名"` 至少获取 3 篇标题 | 角色精确信号（PM写PRD、研发写技术方案、设计师写视觉规范）|
| ③ 近期日程类型 | `dws calendar event list` | 职能边界（参加什么类型的会）|
| ④ 聊天记录 | `dws chat message list` 获取与目标人的近期 IM 消息 | 语言风格/工作内容/职责线索 |

**判定规则**：
- ① 命中（通讯录里搜到该人）→ 至少能拿到"部门 + 职级"信号，置信度起步 ≥ 30%
- ① + ② / ③ / ④ 任一路印证 → 置信度 ≥ 50%
- 两路以上独立信号一致 → 置信度 ≥ 70%
- ① 完全搜不到该人（且通讯录工具本身可用、未报错）→ 才允许告知用户"该人不在通讯录中，请确认花名是否正确"

> **关键约束 1**：部门名 ≠ 角色（"产品设计部"里有 PM、设计师、研究员），必须结合文档产出等信号区分。
>
> **关键约束 2**：① 通讯录查询调用一次就能锁定身份范围，**严禁省略**。常见的失败模式是：在转写文本里反复 grep 目标人名找不到 → 直接放弃 → 告诉用户"找不到"——这是 100% 错误链路（参见案例 7）。

##### Step 5: 定向匹配 + 置信度分支

基于 Step 4 推断的角色，在转写原文中寻找匹配的发言模式：

| 角色 | 典型发言特征 |
|------|----------|
| 产品经理 | 提需求、讲用户场景、定优先级 |
| 研发 | 技术约束、方案评估、排查问题 |
| 管理者 | 发言占比高、最终决策、分配任务 |
| 设计师 | 视觉方案、交互细节、体验讨论 |

**分支 A：置信度 ≥ 50%（文本确认）**

选取最具代表性的连续片段（≥ 2 句完整句子，避免"嗯/对/好"等纯语气词），展示给用户：

> "根据分析，以下发言最可能是 [人名] 的：
> 「[片段内容]」
> 确认是 TA 吗？"

- 用户确认 → Step 6
- 用户否认 → 换下一候选（最多 3 个）
- 3 个全否 → 告知无法仅通过文本确认，建议在听记详情页播放录音辅助辨认

**分支 B：置信度 < 50%（多候选展示）**

当身份推断把握不足时，列出所有候选发言人及其代表性片段，让用户挑选：

> "无法确定哪位是 [人名]。以下是几位候选发言人的代表性内容：
> ① 发言人1：「[片段]」
> ② 发言人3：「[片段]」
> 哪位更像 [人名]？或者你可以在听记详情页播放录音辅助确认。"

- 用户选定 → Step 6
- 用户无法确认 → 告知可在听记详情页点击对应段落播放原始录音来辨认，结束流程

##### Step 6: 结构化总结输出

提取已确认的该发言人的全部发言，结合会议上下文进行综合总结。根据实际内容灵活组织输出结构，例如：

```
[人名] 在本次会议中的发言总结（共 N 段，约 X 字）

核心观点：
- 观点1...
- 观点2...
- 观点3...

关键决策/结论：
- ...

提出的待办/行动项：
- ...
```

##### Step 7: 引导用户替换发言人（必须执行，不可跳过）

总结输出完成后，如果该发言人在转写中仍显示为匿名编号（如"发言人1"），**必须主动引导用户替换**：

> "目前这篇听记中 [人名] 的发言仍显示为『发言人X』。要我帮你把听记里的『发言人X』全部替换为『[人名]』吗？替换后纪要和待办中的发言人也会同步更新。
> 确认后我会执行：`dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "[人名]"`"

- 用户确认 → **先主动调用通讯录模糊查询** `dws contact user search --query "[人名]" --format json` 获取该人员的 userId（长整型 dingUid）：
  - 唯一匹配 → 告知用户匹配结果，确认后执行 `dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "[人名]" --target-uid <userId> --format json`
  - 多个匹配 → 列出候选（姓名+部门+userId）让用户选择后执行
  - 无匹配 → 提示用户未在通讯录中找到此人，执行不带 `--target-uid` 的替换：`dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "[人名]" --format json`
- 用户拒绝 → 不替换，询问是否还有其他发言人需要处理

**追问是否还有其他人需要识别：**

> "还有其他发言人需要我帮你识别和替换吗？比如告诉我『某某人主要讲了什么内容』，我可以帮你匹配。"

**严禁的行为：**
- [禁止] 总结完就结束，不引导用户替换发言人——用户下次看听记时发言人还是"发言人1"，体验极差
- [禁止] AI 自行决定替换而不向用户确认
- [禁止] 未执行 `speaker replace` 就把发言人名称视为已更新

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

**批量查询容错规则：**
> - `get batch` 部分条目权限失败时，**展示成功条目的完整信息 + 列出失败条目的 id 与错误原因**，不要因部分失败就整批放弃改为逐个查询
> - `get batch` 整体报错（如接口级权限/参数问题）时，**降级为逐个 `get info`**，但必须在最终输出中说明"批量接口不可用，已逐个查询"，且所有条目都必须尝试
> - **严禁**：① 部分失败就整批丢弃改逐个查，不展示已成功的数据；② 某一条失败就放弃后续条目；③ 降级后不说明降级原因

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

### 更新纪要内容
```
Usage:
  dws minutes update summary [flags]
Example:
  dws minutes update summary --id <taskUuid> --content "新的纪要内容"
Flags:
      --id string        听记 taskUuid (必填)
      --content string   新的纪要内容 (必填)
```

用传入的摘要文本全量覆盖听记的纪要内容，不触发 AI 重新生成。适用于用户手动编辑或 AI Agent 修改纪要的场景。

**重要 — 修改纪要的完整流程（必须严格执行）：**
当用户要求"精简纪要/优化纪要/修改纪要内容"时，**必须完成以下三步，缺一不可**：
1. **读取**：先调用 `get summary --id <taskUuid>` 获取当前纪要原文
2. **修改**：AI 根据用户要求对纪要内容进行修改（如精简、重新整理、格式优化等），但必须遵守以下约束：
   - **图片必须保留**：原文中的所有 Markdown 图片（如 `![alt](url)`）必须完整保留，不得删除、漏掉、替换为纯文本或打乱语义位置
   - **仅优化文本内容**：可以调整标题层级、段落结构、列表与措辞，但不得破坏图片与对应上下文的关联关系
3. **校验**：写回前必须执行 Markdown 格式检查，确保输出结构合理、可渲染、无明显格式错误（如未闭合代码块、列表层级混乱、标题层级异常等）
4. **写回**：将修改后的完整纪要内容通过 `update summary --id <taskUuid> --content "修改后的完整纪要"` **写回听记**，确保修改持久化

**严禁只读取和修改纪要而不调用 `update summary` 写回**，否则用户看到的仍然是原始纪要，修改不会生效。

### 创建思维导图
```
Usage:
  dws minutes mind-graph create [flags]
Example:
  dws minutes mind-graph create --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

触发创建听记思维导图任务。触发成功后，可通过 `mind-graph status` 轮询任务状态。状态：0=进行中，1=成功，2=失败。

**重要：当用户要求"生成思维导图/创建脑图"时，必须调用此命令（`mind-graph create`），严禁自行生成 HTML 或其他格式的思维导图。** 思维导图由服务端专业引擎生成，AI 不应尝试自己构造思维导图内容。

### 查询思维导图状态
```
Usage:
  dws minutes mind-graph status [flags]
Example:
  dws minutes mind-graph status --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

查询指定听记的思维导图生成状态。返回任务状态：0=进行中，1=成功，2=失败。如果没有返回任务状态，也视为成功。

### 替换发言人
```
Usage:
  dws minutes speaker replace [flags]
Example:
  dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四"
  dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四" --target-uid <uid>
Flags:
      --id string           听记 taskUuid (必填)
      --from string         源发言人昵称 (必填)
      --to string           目标发言人昵称 (必填)
      --target-uid string   目标发言人钉钉 UID (可选)
```

批量替换听记转写中指定发言人，将源发言人（speakerNick）精确匹配的所有段落替换为目标发言人。支持同时替换 nickName 和 subSpeakerNickname 两种匹配方式，并自动更新纪要、待办中的发言人信息。

**重要：**
- 此命令支持替换**任意发言人**，包括已关联通讯录信息的发言人（如"张三"、"李四"等真实姓名），不仅限于"发言人1"之类的占位符
- `--from` 填写当前听记中显示的发言人名称（无论是"发言人1"还是真实姓名），`--to` 填写要替换成的目标名称
- 如果用户希望将发言人关联到通讯录中的具体联系人，可通过 `--target-uid` 传入目标用户的钉钉 UID

### 触发创建发言人段落总结任务
```
Usage:
  dws minutes speaker summary create [flags]
Example:
  dws minutes speaker summary create --ids <uuid1,uuid2>
Flags:
      --ids string          听记 taskUuid 列表，逗号分隔 (必填)
```

触发创建发言人的段落总结任务，将听记中每位发言人的所有发言内容汇总总结。触发后需调用 `speaker summary get` 查询总结结果。

统一使用 canonical 参数 `--ids`。

### 查询发言人段落总结结果
```
Usage:
  dws minutes speaker summary get [flags]
Example:
  dws minutes speaker summary get --ids <uuid1,uuid2>
Flags:
      --ids string          听记 taskUuid 列表，逗号分隔 (必填)
```

查询发言人段落总结任务的结果，返回每位发言人的发言汇总。需先调用 `speaker summary create` 触发任务。

统一使用 canonical 参数 `--ids`。

**speaker summary 异步轮询策略（必须严格遵守）：**

`speaker summary create` 只是触发后端异步任务，**不会立即返回总结结果**。AI 必须按以下策略轮询 `speaker summary get`：

1. 调用 `speaker summary create --ids <uuids>` 触发任务
2. **等待至少 5 秒**后，首次调用 `speaker summary get --ids <uuids>` 查询结果
3. 如果返回结果为空（无内容），**继续等待 5 秒**后重试
4. **最大轮询次数不超过 20 次**（即最长等待约 100 秒）
5. 如果 20 次轮询后仍然为空，视为**任务未完成或无内容**，告知用户："发言人段落总结任务可能仍在处理中，请稍后再试。"

```
伪代码：
speaker summary create --ids <uuids>
wait 5s
for i in 1..20:
    result = speaker summary get --ids <uuids>
    if result 不为空:
        break  # 拿到结果，继续后续流程
    wait 5s
if 仍为空:
    告知用户任务可能仍在处理中
```

**严禁**：
- 调用 `create` 后立即调用 `get`（不等待）——后端异步任务需要时间生成
- 只轮询 1-2 次就放弃——正常任务可能需要 10-30 秒
- 轮询超过 20 次——避免无限等待

**speaker summary 典型应用场景 — 通过发言主题匹配发言人：**

当用户只知道某个人的发言主题或关联内容（如"讲战略规划的那个人是谁"），但不知道对应的发言人编号时，可以通过 speaker summary 获取每位发言人的段落总结，再用关键词匹配确认身份。

**完整链路（发言人段落总结 → 关键词匹配 → 确认 → 替换）：**

1. **确定听记 taskUuid**：从 `list mine/all` 或用户提供的 URL 中获取
2. **触发发言人段落总结**：`dws minutes speaker summary create --ids <taskUuid> --format json`
3. **延迟轮询获取结果**：等待 5s 后调用 `dws minutes speaker summary get --ids <taskUuid> --format json`，最多轮询 20 次
4. **AI 按发言人分组展示总结**：将每位发言人的段落总结以结构化方式呈现给用户
5. **关键词匹配**：从用户描述中抽取关键词（如"战略规划"、"供应链"），在各发言人的段落总结中做模糊匹配
6. **引导用户确认**：
   - 唯一高置信命中 → "『发言人1』的段落总结涉及『战略规划、AI化转型』，很可能就是你说的『李总』，确认替换吗？"
   - 多候选 → 列出所有命中的发言人编号 + 匹配的总结关键词，让用户挑选
   - 无匹配 → 请用户补充更具体的关键词
7. **用户确认后执行替换**：`dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总"`

**自然语言触发示例：**

| Query | AI 处理策略 |
|-------|-----------|
| "帮我看看这个会议里每个人主要说了什么" | speaker summary create → 轮询 get → 按发言人分组展示 |
| "讲战略规划的那个人是谁" | speaker summary create → 轮询 get → 关键词匹配 → 引导确认 |
| "帮我把每个发言人的核心观点总结一下" | speaker summary create → 轮询 get → 结构化输出 |
| "我想知道会上谁讲了供应链相关内容" | speaker summary create → 轮询 get → "供应链"关键词匹配 |
| "那个讲招聘进度的人应该是张三" | speaker summary create → 轮询 get → "招聘进度"匹配 → 确认 → speaker replace |

> **与 `get transcription` 聚类方案的区别**：`speaker summary` 是后端 AI 生成的结构化总结，信息密度更高、匹配更精准；而 `get transcription` 聚类是 AI 在本地按 speakerNick 分组原文再提取要点，适合需要看原文细节的场景。两者可配合使用：先用 speaker summary 快速定位，再用 transcription 看具体原文。

### 添加个人热词
```
Usage:
  dws minutes hot-word add [flags]
Example:
  dws minutes hot-word add --words "钉钉"
  dws minutes hot-word add --words "OKR,钉钉,Copilot"
Flags:
      --words string   要添加的热词，多个用逗号分隔 (必填)
```

添加听记个人热词，用于优化语音识别中专有名词、人名等的识别准确率。支持一次添加多个热词（逗号分隔），每个热词长度不超过 10 个汉字或 5 个英文单词。

### 查询我的热词列表
```
Usage:
  dws minutes hot-word list
Example:
  dws minutes hot-word list
```

查询当前用户配置的所有听记热词列表。无需传入额外参数，系统自动识别当前用户身份。返回用户已添加的全部热词，适用于查看已有热词、去重检查等场景。

### 查找替换听记文字
```
Usage:
  dws minutes replace-text [flags]
Example:
  dws minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字"
Flags:
      --id string        听记 taskUuid (必填)
      --search string    要查找的文字 (必填)
      --replace string   替换为的新文字 (必填)
```

把听记中所有出现的原文字替换为目标文字，包括转写段落和纪要摘要中出现的原文字都会被替换。区分大小写，精确匹配。

**重要 — 执行前必须检查特殊字符并提示用户确认：**
用户输入的原始文本（`--search`）或目标文本（`--replace`）中可能携带特殊字符（如引号 `"` `'` `"` `"`、书名号 `《》`、括号 `【】()（）`、星号 `*`、反斜杠 `\`、换行符、Markdown 格式符号等）。这些特殊字符在转写原文中**通常不存在**，会导致精确匹配失败（替换 0 处）。

AI **必须在执行 `replace-text` 之前**检查用户输入，若发现特殊字符，**先向用户确认**：

> "我注意到你输入的文本中包含特殊字符（如 `…`），转写原文中通常不会包含这些字符，直接匹配可能找不到。建议去掉特殊字符后再替换：
>
> - 原始文本：`XXX` → 建议改为：`YYY`
> - 目标文本：`AAA` → 建议改为：`BBB`
>
> 使用去掉特殊字符的版本执行替换？[是] [否，使用原始输入]"

- 用户确认 → 使用清理后的文本执行替换
- 用户选择原始输入 → 按原样执行，尊重用户意图
- **严禁不提示就自行去掉特殊字符**——用户可能确实需要匹配带特殊字符的文本

**重要 — 执行后必须主动引导用户添加热词（避免长期反复识别错）：**
`replace-text` 仅修正**当前这一篇听记**的文字，**不会影响后续新听记的语音识别结果**。如果用户替换的是一个**长期容易被识别错的专有名词、人名、产品名**（如把"付工"改成"悟空"、把"非书"改成"飞书"），AI **必须在 `replace-text` 成功后主动追问用户**：

> "我已经把这篇听记里的『旧文字』替换为『新文字』。如果这个词以后也容易被识别错，建议把它加到个人热词里，后续新听记就不会再识别错了。要我现在帮你执行 `dws minutes hot-word add --words "新文字"` 吗？"

用户确认后立即调用 `hot-word add`。**严禁只做替换不引导**——这会让用户每次都要手动改一次，体验非常差。

### 创建文件上传会话或者文件转听记或者链接转听记
```
Usage:
  dws minutes upload create [flags]
Example:
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --title "周会录音"
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --input-language "zh" --enable-message-card
Flags:
      --file-name string        文件名（含后缀），如 meeting.mp4 (必填)
      --file-size int           文件大小（字节）(必填，正整数)
      --title string            听记标题，不传时默认使用文件名去掉后缀 (可选)
      --template-id string      纪要生成使用的模板 ID (可选)
      --input-language string   ASR 识别的源语言 (可选)
      --enable-message-card     是否推送闪记卡片消息 (可选，默认 false)
```

创建文件上传会话，获取预签名上传 URL。调用方拿到 URL 后，直接用 HTTP PUT 将文件上传到该 URL。必须与 `upload complete` 配合使用：
1. 调用 `upload create` 获取预签名上传 URL 和 sessionId
2. HTTP PUT 预签名上传 URL 上传文件（不带 HEADER）
3. 调用 `upload complete` 传入 sessionId 完成创建

### 完成文件上传并创建听记
```
Usage:
  dws minutes upload complete [flags]
Example:
  dws minutes upload complete --session-id <sessionId>
Flags:
      --session-id string   上传会话 ID，来自 upload create 返回的 sessionId (必填)
```

文件上传完成后，调用此命令创建听记。必须在 `upload create` 之后、预签名 URL 上传完成后调用。幂等：同一 sessionId 重复调用直接返回已有任务，不会重复创建。

### 取消文件上传会话
```
Usage:
  dws minutes upload cancel [flags]
Example:
  dws minutes upload cancel --session-id <sessionId>
Flags:
      --session-id string   要取消的会话 sessionId (必填)
```

取消 `upload create` 创建的上传会话，释放服务端资源。用于在上传前或上传失败后取消会话。

### 批量添加听记成员并设置权限
```
Usage:
  dws minutes permission add [flags]
Example:
  dws minutes permission add --ids <uuid1,uuid2> --member-uids 123456,789012 --policy 3
  dws minutes permission add --ids <uuid> --member-uids 123456 --policy 2 --cover
  dws minutes permission add --ids <uuid> --member-uids 123456 --policy 3 --sub-resources "OrigContent,Summary"
Flags:
      --ids string            听记 taskUuid 列表，逗号分隔 (必填)
      --member-uids string    成员钉钉 UID 列表，逗号分隔 (必填)
      --policy int            权限类型: 0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看 (必填)
      --cover                 是否覆盖已有权限 (可选，默认 false)
      --sub-resources string  权限子模块，逗号分隔: OrigContent/Summary/Analysis/Note (可选)
```

批量给多个听记增加成员，并设置成员的权限。

**权限类型说明：**

| --policy 值 | 含义 | 说明 |
|------------|------|------|
| 0 | 管理员 | 可管理听记的所有设置和成员权限 |
| 1 | 所有者 | 听记的所有者，拥有最高权限 |
| 2 | 可编辑 | 可编辑听记的纪要、待办等内容 |
| 3 | 可查看/下载 | 可查看和下载听记内容，不可编辑 |
| 4 | 仅查看 | 仅可查看听记内容，不可下载 |

**权限子模块说明（--sub-resources，可选）：**

| 子模块 | 含义 |
|--------|------|
| OrigContent | 原始内容（转写原文） |
| Summary | 纪要（AI 摘要） |
| Analysis | 分析 |
| Note | 笔记 |

不传 `--sub-resources` 时，默认对所有子模块生效。传入后仅对指定的子模块授权。

**典型使用场景：**
- 会议结束后将听记共享给未参会的同事查看
- 给团队成员批量授权某几篇听记的编辑权限
- 限制只共享纪要而不共享原始转写内容

### 批量移除听记成员权限
```
Usage:
  dws minutes permission remove [flags]
Example:
  dws minutes permission remove --ids <uuid1,uuid2> --member-uids 123456,789012
  dws minutes permission remove --ids <uuid> --member-uids 123456
Flags:
      --ids string            听记 taskUuid 列表，逗号分隔 (必填)
      --member-uids string    成员钉钉 UID 列表，逗号分隔 (必填)
```

批量移除多个听记的成员权限。移除后，对应成员将失去对这些听记的访问权限。

**注意事项：**
- 移除权限后，该成员将无法再访问对应的听记内容
- 如果成员是听记的创建者（所有者），无法通过此命令移除其权限
- 建议在移除前先确认成员的当前权限，避免误操作

### 查询我的听记标签/分组列表
```
Usage:
  dws minutes tag list
```

查询当前用户在听记页面手动创建的所有标签或分组列表。无需传入额外参数，系统自动识别当前用户身份。
返回所有标签/分组的列表，每条记录包含 tagId 和标签名称。获取到 tagId 后，可使用 `dws minutes tag query --tag-id <tagId>` 查询该标签下的听记列表。

**注意事项：**
- 标签/分组在听记页面手动创建，此命令仅提供查询能力
- 返回的 tagId 用于后续 `tag query` 命令的 `--tag-id` 参数

### 根据标签ID查询听记列表
```
Usage:
  dws minutes tag query [flags]
Example:
  dws minutes tag query --tag-id <tagId>
  dws minutes tag query --tag-id <tagId> --limit 20
  dws minutes tag query --tag-id <tagId> --limit 10 --cursor <nextToken>
Flags:
      --tag-id string     标签/分组 ID，可通过 tag list 获取 (必填)
      --limit float       每页数据条数 (默认 10)
      --cursor string     分页 token (首页留空)
```

根据用户的标签或分组 ID 查询该标签下的听记列表。支持分页查询。
tagId 可通过 `dws minutes tag list` 获取。

**注意事项：**
- `--tag-id` 必填，值来自 `dws minutes tag list` 返回的 tagId
- 支持分页，使用 `--cursor` 传入上一次返回的 nextToken
- 标签/分组在听记页面手动创建，不支持通过 CLI 创建标签
