# aisearch - AI 搜问

> `aisearch` 模块当前有三个规范子命令：`person`（搜人）、`enterprise`（搜企业内部知识内容）和 `behavior`（搜企业内部行为记录）。
>
>  **搜人容错说明**（无需主动使用）：CLI 兼容下列 alias 兜底，模型偶尔写 `search` / `find` / `query` / `contact` / `people` 等也能跑通——但**搜人输出和文档以 `person` 为准**。
>
>  搜人的 keyword flag 同样兼容 `--name` / `--q` / `--query` / `--text` 兜底，规范用法是 `--keyword`。

## 企业人员搜索

通过关键词搜索企业内人员信息，支持按维度筛选。

```
Usage:
  dws aisearch person [flags]
Example:
  dws aisearch person --keyword "张三" --dimension name --format json
  dws aisearch person --keyword "产品部" --dimension department --format json
  dws aisearch person --keyword "五道" --dimension supervisor --format json
  dws aisearch person --keyword "AI搜问" --dimension duty --format json
  dws aisearch person --keyword "李四" --dimension name,department --format json
  dws aisearch person --keyword "13800138000" --dimension phone --format json
  dws aisearch person --keyword "W12345" --dimension jobNumber --format json
Flags:
      --keyword string     搜索关键词 (必填，如人名、技能关键词等)
      --dimension string   查询维度，多个用逗号分隔 (默认 "all")
```

### dimension 可选值

| 值 | 含义 | 触发词 |
|----|------|--------|
| `all` | 全部维度（默认） | — |
| `name` | 姓名 | "叫什么"、"是谁" |
| `department` | 部门 | "部门"、"团队"、"哪个部门" |
| `position` | 职位 | "职位"、"岗位"、"职级" |
| `duty` | 职责/技能 | "负责什么"、"职责"、"技能"、"负责人" |
| `supervisor` | 上级 | "上级"、"领导"、"主管" |
| `subordinate` | 下级 | "下级"、"下属"、"团队成员" |
| `phone` | 手机号 | "手机号是多少"、"电话"、"联系方式" |
| `jobNumber` | 工号 | "工号"、"工号是多少"、"员工编号" |

### keyword 提取规则

仅填入实际的搜索目标（人名、技能关键词等），不包含查询维度词。维度词必须映射到 `--dimension`：

| 用户说 | keyword | dimension |
|--------|---------|-----------|
| "五道的上级是谁" | 五道 | supervisor |
| "张三负责什么" | 张三 | duty |
| "AI搜问的负责人是谁" | AI搜问 | duty |
| "产品部有谁" | 产品部 | department |
| "李四是哪个部门的" | 李四 | department |
| "13800138000是谁" | 13800138000 | phone |
| "工号W12345是谁" | W12345 | jobNumber |

---

## 意图判断

- 用户说"搜人/找人/谁负责/上级是谁/哪个部门的人" → `aisearch person`
- 用户说"搜资料/找方案/查文档/搜企业知识/项目相关内容/工作总结/周报总结" → `aisearch enterprise`
- 用户说"最近/本周/今天 + XX相关消息/文档/邮件有哪些" → `aisearch enterprise`，时间词进 `--time-range`，类型词进 `--types`
- 用户说"我发过/谁发给我/创建过/分享过/收到过/今天我干了什么" → `aisearch behavior`
- 用户说"搜同事/查部门/查通讯录" → `contact`（通讯录）

**关键区分**：`aisearch person`（AI 语义搜人，支持职责/手机号/上级/下级等维度）vs `aisearch enterprise`（按内容找企业内部知识）vs `aisearch behavior`（按动作找发送/创建/分享/编辑/接收记录）vs `contact`（通讯录精确查询：userId/部门成员列表）

### 高优先级抽取规则

- `enterprise` 抽槽顺序固定为：先抽时间词到 `--time-range`，再抽类型词到 `--types`，最后把剩余主题词放进 `--queries`。
- 所有类型词都必须从 `queries` 中剥离，不能写进 `--query/--queries`。例如“最近 OKR 相关邮件”中，`queries=OKR`、`types=mail`、`time-range=最近`。
- 错误示例：不要生成 `--query "搜索问题"`、`--query "OKR 邮件"`、`--query "AI 搜问 日程"`、`--query "项目 待办"` 这类丢失或混入类型词的命令。
- 也不要把完整自然语言原句塞进 `--query`；`enterprise` 不做自然语言解析，必须显式拆出 `--queries`、`--types`、`--time-range`。
- “相关消息/相关文档/相关邮件有哪些”默认是按内容找企业知识，走 `aisearch enterprise`；只有出现“我发过/某人发给我/我收到/我创建/我分享/我编辑”等行为动作时，才走 `aisearch behavior`。
- `queries` 只放主题词，不放“最近/本周/消息/文档/邮件/日程/待办/纪要/图片/链接/有哪些/相关”等时间、类型、语气词。
- 只要用户显式说“最近/本周/今天/昨天/本月/过去一周/Q3”等时间词，就必须填写 `--time-range`。
- 只要用户显式说出任一类型词，就必须填写对应 `--types`；多个类型同时出现时用逗号分隔，如 `--types im,mail`。

### enterprise 类型词映射

| 用户类型词 | types |
|------------|-------|
| 全部、所有、工作总结、日报总结、周报总结、月报总结 | `all` |
| 文档、资料、方案、模板 | `document` |
| 消息、聊天记录、群消息、群里说了什么 | `im` |
| 邮件、邮箱、mail、email | `mail` |
| 日程、会议邀请、会议安排 | `calendar` |
| 待办、任务、TODO | `todo` |
| 会议纪要、纪要、听记、闪记、录音摘要 | `minute` |
| 日志 | `report` |
| 图片、截图 | `image` |
| 链接、URL、网址 | `link` |
| AI 表格、多维表、notable | `notable` |
| 企业百科、百科 | `baike` |

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `aisearch person` | `userId`（用户ID）、`title`（姓名） | 展示搜索结果、后续操作（发消息/建待办等） |

## 重名消歧

> **CAUTION:** 多人同名时禁止默认选第一个 — 须追加 `contact user get --ids userId1,userId2,...` 获取部门/职位后请用户确认。详见 [08-directory.md](../../dingtalk-contact/references/08-directory.md)「多命中」。

---

## 企业内部知识搜索

用于检索企业内部知识内容，例如文档、消息、日程、待办、听记、日志、图片、链接、AI 表格、企业百科、邮件等。它关注内容本身，适合查找某个主题的资料、搜索包含特定关键词的内容、了解项目/产品相关信息、准备汇报材料等场景。

```
Usage:
  dws aisearch enterprise [flags]
Example:
  dws aisearch enterprise --queries "智能化方案" --types document --format json
  dws aisearch enterprise --queries "搜索问题" --types im --time-range "最近" --format json
  dws aisearch enterprise --queries "搜问" --types im --time-range "最近" --format json
  dws aisearch enterprise --queries "OKR" --types mail --time-range "最近" --format json
  dws aisearch enterprise --queries "AI搜问" --types calendar --time-range "本周" --format json
  dws aisearch enterprise --queries "项目" --types todo,minute --time-range "最近" --format json
  dws aisearch enterprise --queries "发版" --types im --time-range "本周" --format json
  dws aisearch enterprise --types all --time-range "本周" --format json
  dws aisearch enterprise --queries "OKR" --types document,im,mail --format json
Flags:
      --queries string      内容关键词，多个用逗号分隔；汇总类场景可留空
      --types string        搜索类型，多个用逗号分隔 (默认 "all")
      --time-range string   时间范围，仅当用户显式给出时间词时填写
```

### enterprise types 可选值

| 值 | 含义 | 触发词 |
|----|------|--------|
| `all` | 全部类型（默认） | 全部、所有、工作总结、日报总结、周报总结、月报总结 |
| `document` | 文档 | 文档、资料、方案、模板 |
| `im` | 消息 | 消息、聊天记录、群消息、群里发了什么 |
| `calendar` | 日程 | 日程、会议邀请、会议安排 |
| `todo` | 待办 | 待办、任务、TODO |
| `minute` | 会议纪要/闪记/听记 | 会议纪要、纪要、听记、闪记、录音摘要 |
| `report` | 日志 | 仅显式出现“日志”时使用 |
| `image` | 图片 | 图片、截图 |
| `link` | 链接 | 链接、URL、网址 |
| `notable` | 多维表 / AI 表格 | AI表格、多维表、notable |
| `baike` | 企业百科 | 企业百科、百科 |
| `mail` | 邮件 | 邮件、邮箱、mail、email |

### enterprise 参数提取规则

- `queries` 只放内容关键词，不放时间、类型词。比如“本周的 OKR 文档”中，`queries=OKR`；“最近 OKR 相关邮件”中，`queries=OKR`，不要写成 `queries=OKR 邮件`。
- 时间信息放到 `--time-range`，仅当用户显式给出“今天/本周/最近/9月/Q3/过去一周”等时间词时填写。
- 类型词放到 `--types`，所有类型词都不能留在 `--query/--queries`；多类型用逗号分隔。文档/资料/方案类型使用底层枚举 `document`（注意不是 `doc`）。`report` 仅在用户显式说“日志”时触发；“周报/日报/月报/工作汇报”不要自动映射为 `report`。
- `mail` 仅在用户显式说“邮件/邮箱/mail/email”时触发；一旦触发，必须进入 `--types mail`，不能留在 `--query/--queries`。
- “工作总结/日报总结/周报总结/月报总结”这类汇总场景，用 `--types all`，`--queries` 可留空。

| 用户说 | queries | types | time-range |
|--------|---------|-------|------------|
| “智能化方案相关文档” | 智能化方案 | document | 空 |
| “最近搜索问题相关的消息都有哪些” | 搜索问题 | im | 最近 |
| “最近搜问相关的消息都有哪些” | 搜问 | im | 最近 |
| “最近 OKR 相关邮件” | OKR | mail | 最近 |
| “本周 AI 搜问相关日程” | AI 搜问 | calendar | 本周 |
| “最近项目相关待办和纪要” | 项目 | todo,minute | 最近 |
| “本周的 OKR 文档” | OKR | document | 本周 |
| “最近发版相关消息” | 发版 | im | 最近 |
| “AI 搜问相关图片和链接” | AI 搜问 | image,link | 空 |
| “OKR 相关 AI 表格和百科” | OKR | notable,baike | 空 |
| “2025-12-06 到 2025-12-19 工作总结” | 空 | all | 2025-12-06 到 2025-12-19 |
| “本周的日志” | 空 | report | 本周 |
| “我收到的邮件” | 空 | mail | 空 |

---

## 企业内部行为记录搜索

用于检索“谁对什么做了什么”的企业内部行为记录，例如发过、创建过、分享过、编辑过、收到过的文档、消息、日程、待办、听记、日志、图片、链接、AI 表格、企业百科、邮件等。它关注行为流向和动作，不是按内容本身找知识。

```
Usage:
  dws aisearch behavior [flags]
Example:
  dws aisearch behavior --queries "智能化方案" --types document --format json
  dws aisearch behavior --types mail --behavior-type send --direction "我->汐峰" --format json
  dws aisearch behavior --types all --behavior-type create --time-range "本周" --format json
  dws aisearch behavior --types im --chat-scope "scrum群" --behavior-type send --time-range "今天" --format json
Flags:
      --queries string          内容关键词，多个用逗号分隔；汇总类场景可留空
      --types string            搜索类型，多个用逗号分隔 (默认 "all")
      --chat-scope string       消息所在会话/群范围，仅 IM 类型且用户明确指定群名时填写
      --behavior-type string    行为类型 (默认 "all")
      --time-range string       时间范围，仅当用户显式给出时间词时填写
      --direction string        交互方向，如 "我->汐峰"、"汐峰->我"、"我<->汐峰"
```

### behavior types 可选值

| 值 | 含义 | 触发词 |
|----|------|--------|
| `all` | 全部类型（默认） | 今天我干了什么、我最近做过什么 |
| `document` | 文档 | 文档、资料、方案、模板 |
| `im` | 消息 | 消息、聊天记录、群消息、群里发了什么 |
| `calendar` | 日程 | 日程、会议邀请、会议安排 |
| `todo` | 待办 | 待办、任务、TODO |
| `minute` | 会议纪要/闪记/听记 | 会议纪要、纪要、听记、闪记、录音摘要 |
| `report` | 日志 | 仅显式出现“日志”时使用 |
| `image` | 图片 | 图片、截图 |
| `link` | 链接 | 链接、URL、网址 |
| `notable` | 多维表 / AI 表格 | AI表格、多维表、notable |
| `baike` | 企业百科 | 企业百科、百科 |
| `mail` | 邮件 | 邮件、邮箱、mail、email |

### behavior-type 可选值

| 值 | 含义 | 示例 |
|----|------|------|
| `all` | 全部行为（默认） | “智能化方案相关内容” |
| `send` | 发送 | “我发给汐峰的消息/邮件” |
| `create` | 创建 | “我创建过哪些文档” |
| `share` | 分享 | “我分享过的资料” |
| `edit` | 编辑 | “我编辑过的文档” |
| `receive` | 接收 | “汐峰发给我的文档”、“我收到的邮件” |

### 参数提取规则

- `queries` 只放内容关键词，不放时间、类型词、行为词。比如“本周我创建的智能化方案文档”中，`queries=智能化方案`。
- `types` 放内容类型，映射规则与 enterprise 相同；所有类型词都不能留在 `--query/--queries`，多类型用逗号分隔。
- 文档/资料/方案类型使用底层枚举 `document`（注意不是 `doc`）。`report` 仅在用户显式说“日志”时触发；“周报/日报/月报/工作汇报”不要自动映射为 `report`。
- `mail` 仅在用户显式说“邮件/邮箱/mail/email”时触发；一旦触发，必须进入 `--types mail`。
- `time-range` 仅当用户显式给出时间词时填写，不要根据语义猜时间。
- `direction` 仅当用户明确指定交互对象时填写，格式为 `发起者->接收者` 或 `我<->某人`；无具体对象时留空。
- “今天我干了什么/我最近做过什么”这类行为汇总场景，用 `--types all`，`--queries` 可留空。

| 用户说 | queries | types | behavior-type | time-range | direction | chat-scope |
|--------|---------|-------|---------------|------------|-----------|------------|
| “我发给汐峰的邮件” | 空 | mail | send | 空 | 我->汐峰 | 空 |
| “我发给汐峰的消息和邮件” | 空 | im,mail | send | 空 | 我->汐峰 | 空 |
| “汐峰发给我的文档” | 空 | document | receive | 空 | 汐峰->我 | 空 |
| “我创建过哪些文档” | 空 | document | create | 空 | 空 | 空 |
| “本周我创建的智能化方案文档” | 智能化方案 | document | create | 本周 | 空 | 空 |
| “我分享过的项目链接和图片” | 项目 | link,image | share | 空 | 空 | 空 |
| “我在 scrum 群里发了什么” | 空 | im | send | 空 | 空 | scrum群 |
| “帮我总结今天干了什么” | 空 | all | all | 今天 | 空 | 空 |

## 行为搜索 vs 知识搜索

- `aisearch behavior`：用户问“我/某人做过什么动作”，有发送、创建、分享、编辑、接收、收到、发给等行为词。
- `aisearch enterprise`：用户问“是什么/怎么做/在哪里/模板/方案/总结”，目标是内容本身或跨类型知识内容汇总。
