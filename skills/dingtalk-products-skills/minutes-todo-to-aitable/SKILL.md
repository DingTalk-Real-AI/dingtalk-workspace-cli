---
name: minutes-todo-to-aitable
description: >
  从听记中提取待办事项，自动创建AI表格进行结构化管理和追踪。
  Use when user says "听记待办生成表格", "把会议待办整理成表格", "听记待办导出AI表格",
  "帮我把待办事项做成表格", "会议待办汇总表", "整理一下最近的听记待办",
  or passes a minutes URL asking "提取待办到表格".
  Do NOT use for: 单纯查看听记内容、听记摘要生成、待办事项手动创建、普通表格数据录入。
  Distinct from minutes-summary (仅摘要) and todo-batch-create (手动创建待办).
  Requires: dingtalk-minutes, dingtalk-aitable, dingtalk-contact.
metadata:
  author: 钉听记团队
  version: 1.0.0
  category: productivity
  tags: [minutes, todo, aitable, automation, dingtalk]
---

# 听记待办 → AI表格

将会议中产生的待办事项自动提取并生成结构化的 AI 表格，方便追踪、分配和管理。一键完成"会议 → 待办 → 表格"的闭环。

## 核心价值

**会后不漏事**：会议中提到的待办经常被遗忘，本技能自动从听记中提取，生成可追踪的表格，避免"会上说好，会后忘记"。

## 交互风格

**像助理，不像机器人**：不暴露技术细节（不说"API"、"taskUuid"、"baseId"），直接做、自然说。

## 严格禁止 (NEVER DO)

- **禁止暴露技术细节**：不说"调用命令"、"taskUuid"、"baseId"等术语
- **禁止编造待办内容**：所有待办必须来自听记原文，不可自行添加
- **禁止跳过确认**：提取的待办列表必须经用户确认后才能写入表格
- **禁止猜测 taskUuid**：听记 ID 必须从 URL 提取或从 list 返回中获取
- **禁止追加时新增字段**：追加到已有表格时，不得创建新字段，只填充已有字段

## 涉及产品

| 产品 | 用途 | 对应命令 |
|------|------|---------|
| `minutes` | 获取听记列表、提取待办事项 | `dws minutes list mine / get info / get todos` |
| `aitable` | 创建AI表格、写入待办记录 | `dws aitable base create / table get / record create` |
| `contact` | 匹配负责人通讯录信息 | `dws contact user search` |

## 能力清单

| 能力 | 安全等级 | 说明 | 约束 |
|------|---------|------|------|
| 定位听记 | 只读 | 根据 URL 或描述智能匹配听记 | 多候选时必须询问用户确认 |
| 提取待办 | 只读 | 获取听记中的待办事项列表 | - |
| 匹配负责人 | 只读 | 根据人名搜索通讯录获取 userId | 多候选时询问确认，未匹配降级为文本 |
| 分析已有表格 | 只读 | 解析已有表格结构，智能映射字段 | 用户选择追加时执行 |
| 创建表格 | 写入 | 创建待办管理AI表格 | 用户选择新建时执行 |
| 写入记录 | 写入 | 将待办逐条写入表格 | 用户确认后执行 |

## 意图判断

| 用户说 | 线索 | 对应操作 |
|--------|------|---------|
| "听记待办生成表格" / "待办导出表格" | 明确要表格 | 执行完整流程 |
| "把会议待办整理一下" | 需要结构化 | 询问是否生成表格 |
| "最近几个会议的待办" | 批量提取 | 获取多条听记的待办 |
| 传入听记 URL + "待办表格" | URL + 关键词 | 直接处理指定听记 |
| "昨天会议的待办" / "上周讨论的事项" | 时间描述 | 智能匹配 + 确认 |
| "添加到这个表格" + 表格链接 | 已有表格 | 追加模式，分析表格结构 |
| "追加到已有的待办表" | 追加意图 | 询问用户提供表格链接 |

## 易混淆场景

| 用户说 | 线索 | 应路由到 | 原因 |
|--------|------|---------|------|
| "总结一下听记" | 仅需摘要 | minutes get summary | 非待办场景 |
| "帮我创建几个待办" | 手动创建 | todo create | 无听记来源 |
| "把数据导入表格" | 非听记数据 | aitable 产品技能 | 数据来源不同 |
| "查看听记内容" | 仅查看 | minutes get transcription | 无需表格 |

## 输入

**必填**：听记 URL 或自然语言描述（主题/时间）

**可选**：
- 提取范围（最近 N 条听记）
- 表格名称
- 是否合并多条听记待办到同一表格
- 已有表格链接（追加模式）：`https://alidocs.dingtalk.com/i/nodes/{baseId}`

## 执行流程

### Step 1：智能定位听记文件

| 输入方式 | 操作 |
|---------|------|
| 用户传入 URL | 从 URL 提取 `taskUuid`，调用 `dws minutes get info --id <taskUuid> --format json` |
| 自然语言描述 | 调用 `dws minutes list mine --max 20 --format json`，按时间/主题匹配 |
| "最近 N 条" | 调用 `dws minutes list mine --max N --format json` |

**MUST**：多候选时列出候选列表，询问用户确认后再继续。

### Step 2：提取待办事项

```bash
dws minutes get todos --id <taskUuid> --format json
```

**MUST**：如果是多条听记，逐条提取待办，合并去重。

返回内容包含：待办内容、参与人信息、时间戳。

### Step 3：通讯录匹配负责人

如果待办事项中包含人名，调用通讯录搜索获取 userId，用于写入 user 类型的负责人字段：

```bash
dws contact user search --query "张三" --format json
```

**匹配策略**：

| 匹配结果 | 处理方式 |
|---------|---------|
| 精确匹配到 1 人 | 提取 `userId`，写入负责人字段：`[{"userId":"xxx"}]` |
| 匹配到多人 | 列出候选名单（含职位、部门），询问用户确认后再填写 |
| 未匹配到 | 跳过负责人字段，由用户在表格中手动填写 |

**注意**：
- 待办可能包含多个人名（如"张三、李四负责"），需逐个匹配，写入格式：`[{"userId":"uid1"},{"userId":"uid2"}]`
- 通讯录搜索是模糊匹配，同名时通过 `title`（职位）和 `deptName`（部门）辅助用户确认
- 匹配失败不阻塞流程，负责人字段留空由用户手动补充

### Step 4：展示待办并确认

向用户展示提取到的待办列表（如"从「XX会议」中找到 N 条待办"），**MUST**：必须等用户确认后才能继续。

### Step 5：确认目标表格（新建 or 追加）

**MUST**：询问用户选择目标表格：

> "待办已确认，请问是：
> 1. **新建表格** — 创建一个新的待办追踪表
> 2. **追加到已有表格** — 请提供表格链接"

| 用户选择 | 后续流程 |
|---------|---------|
| 新建表格 | → Step 6A |
| 追加到已有表格 | 用户提供链接 → Step 6B |
| 用户直接给了表格链接 | 跳过询问 → Step 6B |

### Step 6A：创建新表格（新建模式）

用户选择新建后，创建待办追踪表格：

```bash
dws aitable base create --name "会议待办追踪-{日期}" --format json
```

从返回中提取 `baseId`，然后 → Step 7。

### Step 6B：解析已有表格（追加模式）

用户提供表格链接后：

**1. 从 URL 提取 baseId**

URL 格式：`https://alidocs.dingtalk.com/i/nodes/{baseId}`

**2. 获取表格结构**

```bash
dws aitable base get --base-id <baseId> --format json
# 从返回中提取 tableId

dws aitable table get --base-id <baseId> --table-id <tableId> --format json
# 返回字段列表：fieldId、fieldName、type
```

**3. 智能映射字段**

分析已有字段，自动匹配待办数据：

| 待办数据 | 匹配规则（按优先级） | 写入方式 |
|---------|---------------------|---------|
| 待办内容 | 名称含"待办/任务/内容/事项"的 text 字段，或主字段 | 直接写入 |
| 负责人 | 名称含"负责人/执行人/owner"的 user 字段 | `[{"userId":"xxx"}]` |
| 来源会议 | 名称含"来源/会议/听记"的 text 字段 | 直接写入 |
| 会议时间 | 名称含"时间/日期"的 date 字段 | ISO 日期 |
| 状态 | 名称含"状态/进度"的 singleSelect 字段 | 选项名 |
| 听记链接 | 名称含"链接/URL"的 url 字段 | `{"text":"","link":""}` |

**4. 展示映射结果并确认**

向用户展示字段映射关系，**MUST**：确认后再写入：

> "已分析表格结构，字段映射如下：
> - 待办内容 → 「任务名称」
> - 负责人 → 「执行人」
> - 来源会议 → （未找到匹配字段，将跳过）
> - ...
> 
> 确认写入？"

**5. 严格约束：只填充，不新增**

| 约束 | 说明 |
|------|------|
| **禁止新增字段** | 不得调用 `field create`，保持表格原有结构 |
| **未匹配字段跳过** | 表格中没有对应字段时，该数据不写入，不报错 |
| **只写匹配到的** | 仅填充能匹配上的字段，其余留空 |

**6. 处理未匹配字段**

| 情况 | 处理 |
|------|------|
| 找到匹配字段 | 正常写入 |
| 未找到匹配字段 | **跳过该数据，不新增字段** |
| 字段类型不匹配 | 尝试转换（如 user→text 降级为人名文本），失败则跳过 |

然后 → Step 7。

### Step 7：写入记录

**新建模式（来自 Step 6A）**：
1. 获取 tableId 和 fieldId：`dws aitable table get --base-id <baseId> --format json`
2. 创建待办追踪表（含 6 个字段：待办内容、负责人、来源会议、会议时间、状态、听记链接）
3. 写入待办记录（**MUST：cells 的 key 必须是 fieldId，不是字段名**）

**追加模式（来自 Step 6B）**：
1. 使用 Step 6B 中已获取的 tableId 和 fieldId
2. 按字段映射关系写入记录
3. 跳过未匹配的字段

详细字段定义和写入格式见 [references/table-schema.md](./references/table-schema.md)。

### Step 8：交付结果

向用户展示完成信息，包含表格链接 `https://alidocs.dingtalk.com/i/nodes/{baseId}`。

## 上下文传递表

| 阶段 | 从返回中提取 | 用于 |
|------|-------------|------|
| Step 1: `minutes list mine` | `taskUuid` | Step 2 的 `--id` 参数 |
| Step 1: 用户传入 URL | 从 URL 末段提取 `taskUuid` | Step 2 的 `--id` 参数 |
| Step 1: `minutes get info` | 听记标题、时间 | Step 7 的记录字段 |
| Step 2: `minutes get todos` | 待办内容、参与人 | Step 3 匹配 + Step 4 展示 + Step 7 写入 |
| Step 3: `contact user search` | `userId`、`name`、`title`、`deptName` | Step 7 负责人字段写入（user 类型：`[{"userId":"xxx"}]`） |
| Step 6A: `aitable base create` | `baseId` | Step 7 的 `--base-id` 参数 |
| Step 6B: 用户传入表格 URL | 从 URL 提取 `baseId` | Step 6B 的 `--base-id` 参数 |
| Step 6B: `aitable base get` | `tableId` | Step 6B 的 `--table-id` 参数 |
| Step 6B: `aitable table get` | `fieldId`、`fieldName`、`type` | Step 7 字段映射 + `record create` |
| Step 7: `aitable table create` | `tableId`, `fieldId` | `record create` 的参数（新建模式） |
| Step 6A/6B | 文档 URL | Step 8 交付给用户 |

## 踩坑提醒

| 错误做法 | 正确做法 | 原因 |
|---------|---------|------|
| 用字段名作为 cells 的 key | 用 fieldId 作为 key | AI表格 API 要求必须用 fieldId |
| 直接创建记录 | 先 table get 获取 fieldId | 没有 fieldId 无法写入 |
| 提取完直接写表格 | 展示待办列表等用户确认 | 用户需要核对和筛选 |
| 编造 taskUuid | 从 URL 或 list 返回中获取 | 虚构 ID 会导致失败 |
| 一次写入超过 100 条 | 分批写入，每批最多 100 条 | API 限制 |
| 人名匹配多人时直接选第一个 | 列出候选让用户确认 | 同名情况常见，需用户判断 |
| 通讯录匹配失败时报错终止 | 跳过负责人字段，用户手动补充 | 匹配失败不应阻塞主流程 |
| user 字段写入字符串 | 写入数组格式 `[{"userId":"xxx"}]` | user 类型必须是数组 |
| 追加时不确认字段映射 | 展示映射关系等用户确认 | 避免数据写错字段 |
| 追加时强制要求所有字段 | 跳过未匹配字段，正常写入匹配的 | 已有表格结构可能不完全相同 |
| 追加时新增缺失字段 | **禁止新增字段**，只填充已有字段 | 保持用户表格原有结构不变 |
| 追加到 user 字段时写人名文本 | 先检查字段类型，user 用 userId | 类型不匹配会写入失败 |

## 使用示例

### 示例 1：最简流程（单条听记 + URL）

**用户说**："https://shanji.dingtalk.com/app/transcribes/abc123 帮我把待办整理成表格"

**AI 回应**：找到 3 条待办，确认后生成表格，返回表格链接。

### 示例 2：完整流程（指定听记）

**用户说**："这个听记的待办帮我整理成表格 https://shanji.dingtalk.com/app/transcribes/abc123"

**AI 回应**：
1. 提取待办："从「Q2 产品规划会」中找到 4 条待办：完成用户调研报告、输出竞品分析文档..."
2. 用户确认后创建表格
3. 返回："表格已创建好，包含 4 条待办。[点击查看](链接)"

### 示例 3：批量提取最近会议待办

**用户说**："帮我把最近三个会议的待办整理到一个表格里"

**AI 回应**：
1. 列出 3 条听记及待办数量
2. 确认后生成汇总表格

### 示例 4：模糊描述匹配

**用户说**："昨天那个会的待办帮我做个表"

**AI 回应**：先确认是哪个会议，再提取待办并生成表格。

### 示例 5：追加到已有表格

**用户说**："把这个会议的待办添加到我的项目表里 https://alidocs.dingtalk.com/i/nodes/abc123"

**AI 回应**：
1. 提取待办："从「周会」中找到 3 条待办"
2. 用户确认待办后
3. 分析表格结构："已分析表格，字段映射如下：待办内容→任务名称，负责人→执行人..."
4. 用户确认映射后写入
5. 返回："已添加 3 条待办到表格。[点击查看](链接)"

### 示例 6：用户选择追加模式

**用户说**："帮我把最近会议的待办整理一下"

**AI 回应**：
1. 提取待办并展示
2. 询问："待办已确认，请问是新建表格还是追加到已有表格？"
3. 用户说"追加到已有的"
4. AI："好的，请提供表格链接"
5. 用户提供链接后，分析结构、确认映射、写入

## 错误处理

| 阶段 | 错误 | 处理 | 是否终止 |
|------|------|------|---------|
| Step 1 | 听记 URL 格式错误 | 提示正确格式：https://shanji.dingtalk.com/app/transcribes/xxx | 是 |
| Step 1 | 未找到匹配的听记 | 询问用户扩大时间范围或提供 URL | 是 |
| Step 2 | 听记中无待办事项 | 告知用户"该听记暂无识别出的待办"，建议查看原文 | 是 |
| Step 3 | 通讯录搜索无结果 | 跳过负责人字段，用户在表格中手动填写 | 否 |
| Step 3 | 通讯录返回多人 | 列出候选（含职位、部门），让用户确认 | 否 |
| Step 6A | 表格创建失败 | 重试 1 次，仍失败则报告错误 | 是 |
| Step 6B | 表格 URL 格式错误 | 提示正确格式：https://alidocs.dingtalk.com/i/nodes/xxx | 是 |
| Step 6B | 表格不存在或无权限 | 提示用户检查链接或权限，建议新建表格 | 是 |
| Step 6B | 表格无匹配字段 | 告知用户"未找到可匹配的字段"，建议新建表格 | 是 |
| Step 7 | 记录写入失败 | 逐条重试失败项，记录成功/失败数量 | 否，报告部分成功 |
| Step 7 | fieldId 获取失败 | 检查 table get 返回，重试 1 次 | 是 |

## 详细参考

- [references/table-schema.md](./references/table-schema.md) — 字段定义和数据格式
- [references/batch-processing.md](./references/batch-processing.md) — 批量处理策略
