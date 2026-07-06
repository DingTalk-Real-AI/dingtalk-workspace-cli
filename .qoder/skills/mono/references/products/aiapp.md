# AI应用 (aiapp) 命令参考

## 强制触发规则

以下意图默认必须优先使用 `aiapp`，即使用户没有说“AI应用”：

- 创建一个新应用
- 创建一个新系统
- 生成一个业务工具
- 按需求生成一个管理后台、工作台应用、表单系统、业务原型
- 修改一个已经生成过的应用

典型触发词包括但不限于：

- 应用、系统、平台、工具、后台、页面、原型、工作台应用
- CRM、OA、库存、仓库、审批、销售、报销、进销存、客户管理、数据录入、查询工具
- 计算器、待办工具、表单应用、信息收集应用、报表应用
- “帮我做一个…”
- “帮我创建一个…”
- “按这个需求生成一个…”
- “把这个需求做成应用”

典型例子：

- “帮我创建个简易的计算器应用”
- “做一个仓库管理系统”
- “生成一个客户管理工具”
- “按这个 PRD 创建一个钉钉应用”
- “修改刚才那个应用，把首页改得更简洁”

不要误路由到其他产品：

- 不要因为用户提到“表格/记录/字段”，就优先走 `aitable`，如果用户的真实目标是“生成一个完整应用”
- 不要因为用户提到“文档/需求”，就优先走 `doc`，如果用户的真实目标是“把需求做成应用”
- 不要因为用户提到“任务/项目”，就优先走 `tb`，如果用户的真实目标是“生成一个管理系统”

只有在以下场景不要走 `aiapp`：

- 用户明确要操作一个现成的多维表、文档、项目、日历、机器人消息
- 用户不是要“生成应用”，而是要“修改已有产品对象”

## 命令总览

### 创建 AI 应用
```
Usage:
  dws aiapp create [flags]
Example:
  dws aiapp create --prompt "创建一个天气查询应用"
  dws aiapp create --prompt "翻译应用"  --skills s1,s2
  dws aiapp create --prompt "根据附件里的 Excel 创建一个仓库管理应用" --attachments '[{"name":"warehouse.xlsx","type":"excel","url":"https://tmp/warehouse.xlsx"}]'
Flags:
      --prompt string      创建 AI 应用的 prompt (必填)
      --skills string      技能 ID 列表，逗号分隔
      --attachments string 附件对象数组的 JSON 字符串，用于把 Excel/图片等输入材料一起传入
```

### 查询 AI 应用
```
Usage:
  dws aiapp query [flags]
Example:
  dws aiapp query --task-id <taskId>
Flags:
      --task-id string   AI 应用任务 ID (必填)
```

### 修改 AI 应用
```
Usage:
  dws aiapp modify [flags]
Example:
  dws aiapp modify --prompt "改为翻译应用" --thread-id <threadId>
  dws aiapp modify --prompt "新描述" --thread-id <threadId> --skills s1,s2
Flags:
      --prompt string      新的 prompt (必填)
      --skills string      技能 ID 列表，逗号分隔
      --thread-id string   threadId (必填)
```

## 三个动作的关系

`aiapp` 的三个动作不是彼此独立的，它们描述的是同一个应用任务的生命周期：

1. `create` 用来发起一次新的应用生成任务，返回 `taskId` 和 `threadId`
2. `query` 用来跟踪这次任务是否完成，以及读取生成结果
3. `modify` 用来基于同一个 `threadId` 继续和已生成的应用对话，本质上是对已有应用做增量修改

可以把它理解为：

- `create` = 新建一个应用会话
- `query` = 查看这个会话对应任务的执行状态和结果
- `modify` = 在这个会话里继续追加新的需求，让同一个应用继续演化

关键标识符的关系：

- `taskId` 表示某一次具体的执行任务，主要给 `query` 使用
- `threadId` 表示这个应用的持续会话，主要给 `modify` 使用
- 一次 `create` 会产生一个新的 `taskId`，同时也会产出或确认一个 `threadId`
- 后续每次 `modify` 都是在同一个 `threadId` 上发起新的任务，因此也会产生新的 `taskId`

因此推荐把它们看成一条闭环链路，而不是三个平级命令：

`create -> query -> modify -> query -> modify ...`

## `attachments` 的含义

`attachments` 是 `aiapp` 的输入材料参数，用来把文件和当前任务一起提交给 `aiapp`。

它适合这些场景：

- 根据 Excel 创建应用
- 根据需求文档、表格、截图继续完善应用
- 在创建应用时补充新的结构化材料

可以把它理解为：

- `prompt` 描述“希望系统做什么”
- `attachments` 提供“系统可直接参考的文件材料”

### 正确心智

- 当前 CLI 中，`attachments` 是 `create` 的补充输入，不是独立动作
- 即使提供了附件，主动作仍然是 `aiapp create`
- 当用户说“按这个 Excel 建一个应用”时，优先走 `aiapp`，并把 Excel 作为 `attachments` 传入

### 数据结构

`attachments` 不是单个字符串，而是对象数组。每个对象当前按以下结构传递：

```json
[
  {
    "name": "warehouse.xlsx",
    "type": "excel",
    "url": "https://tmp/warehouse.xlsx",
    "size": 102400
  }
]
```

每个附件对象当前可用字段：

- `name`：文件名
- `type`：文件类型或 MIME 类型
- `url`：可直接访问的临时下载地址
- `size`：文件大小，可选

如果要传多个附件，就在数组中放多个对象。

### Excel 场景

如果用户上传了 Excel，通常应该：

- 在 `prompt` 里明确写“根据附件中的 Excel 创建应用”
- 同时把 Excel 通过 `attachments` 传给 `aiapp`

这类需求都应优先视为 `aiapp`：

- “根据这个 Excel 建一个库存管理应用”
- “把附件里的表格做成一个 CRM 应用”
- “按这个 Excel 生成一个新应用”

### 使用规则

- `create` 可以带图片、Excel 等附件
- 当前 `modify` 不支持 `--attachments` 参数；需要结合新材料修改应用时，把材料内容概括进 `prompt`
- `query` 不使用 `attachments`
- 附件应按对象数组传递，而不是传单个 ID 或随意文本
- 如果用户提供本地文件并要创建应用 → 先上传文件获取 URL，再执行 `aiapp create`
- 每个附件对象通常至少应包含可访问的临时下载地址 `url`
- 如果有附件但没有特别完整的 prompt，也仍然建议补一句明确意图，避免模型误判

### 示例

```bash
# 根据 Excel 创建应用
dws aiapp create --prompt "根据附件里的 Excel 创建一个仓库管理应用" \
  --attachments '[{"name":"warehouse.xlsx","type":"excel","url":"https://tmp/warehouse.xlsx"}]' \
  --format json

# 创建应用时结合图片材料
dws aiapp create --prompt "根据附件图片创建一个首页风格参考应用" \
  --attachments '[{"name":"homepage.png","type":"image/png","url":"https://tmp/homepage.png"}]' \
  --format json
```

## `--skills` 的含义

`--skills` 是 `aiapp` 的附加能力参数，不是一个独立动作。

它的作用不是“先执行这些技能，再单独执行 `aiapp`”，而是：

- 在 `create` 时，把这些官方技能作为本次应用生成任务的附加约束或附加能力一起传入
- 在 `modify` 时，把这些官方技能作为本次修改任务的附加约束或附加能力一起传入

可以把它理解为：

- `prompt` 描述“你想做什么应用”
- `--skills` 描述“这次生成或修改时，要额外挂载哪些官方能力或规范”

因此 `--skills` 必须依附于 `create` 或 `modify` 使用，本身不能单独完成创建应用。

### 正确心智

- `aiapp` 是主动作
- `--skills` 是主动作的附加参数
- 用户是否挂载官方技能，会影响应用生成或修改效果
- 即使用户提到了某个官方技能，最终仍然应该优先调用 `aiapp create` 或 `aiapp modify`

### 常见误区

不要把以下场景理解错：

- 用户说“创建一个应用，并遵循设计规范”
  - 正确做法：调用 `aiapp create ... --skills <design-skill-id>`
  - 不正确做法：把“设计规范技能”当成一个和 `aiapp` 平级、单独执行的动作

- 用户说“继续修改刚才那个应用，并带上设计规范”
  - 正确做法：调用 `aiapp modify ... --thread-id <threadId> --skills <design-skill-id>`
  - 不正确做法：先执行设计规范技能，再单独调用 `modify`

- 用户只是在描述“希望页面更漂亮、更标准”
  - 如果已知对应的官方技能 ID，可以通过 `--skills` 传入
  - 如果没有明确的技能 ID，不要编造，优先把需求写进 `prompt`

### 什么时候传 `--skills`

适合传 `--skills` 的情况：

- 用户明确要求遵循某个已知的官方能力、官方规范、官方模板
- 当前上下文里已经拿到了可用的官方技能 ID/Code
- 需要在创建或修改时显式挂载这些技能

不适合传 `--skills` 的情况：

- 没有明确的技能 ID
- 只是模糊地希望“更专业”“更好看”，但没有对应官方技能可挂
- 技能本身不是 `aiapp` 官方子技能，而是其他产品的独立操作能力

### 使用规则

- `create` 和 `modify` 都可以带 `--skills`
- `query` 不使用 `--skills`
- `--skills` 里的值必须是技能 ID，多个值用逗号分隔
- 不要猜测、编造技能 ID，必须来自已知上下文或系统返回

### 示例

```bash
# 创建应用，并挂载一个钉钉设计规范技能
dws aiapp create --prompt "创建一个仓库管理应用" \
  --skills dingtalk_design_spec --format json

# 修改已有应用，并继续带上官方技能
dws aiapp modify --prompt "增加个库存大盘图表页" \
  --thread-id <threadId> \
  --skills dingtalk_design_spec --format json
```

## 意图判断

用户说以下任何一种，都走 `create`：

- “做个应用 / 创建应用 / 生成应用 / 搭个应用”
- “做个系统 / 平台 / 后台 / 工具 / 页面”
- “帮我做个计算器 / 仓库管理 / CRM / OA / 审批 / 报表 / 信息收集应用”
- “按这段需求生成一个钉钉应用”

用户说以下任何一种，都走 `query`：

- “查看应用状态”
- “这个应用创建好了没”
- “查一下 taskId 对应的进度”
- “继续轮询应用创建进展”

用户说以下任何一种，都走 `modify`：

- “修改应用”
- “更新刚才那个应用”
- “基于这个 thread/task 继续改”
- “把首页改一下 / 把功能补上 / 换个风格”

## 核心工作流

```bash
# 1. 创建 AI 应用 — 提取 taskId 和 threadId
dws aiapp create --prompt "创建一个天气查询应用" --format json

# 2. 查询应用状态
dws aiapp query --task-id <taskId> --format json

# 3. 修改应用
dws aiapp modify --prompt "新描述" --thread-id <threadId> \
  --skills skill1,skill2 --format json
```

补充理解：

- 如果用户还没有现成应用，先走 `create`
- 如果用户刚发起了创建，通常先走 `query` 看是否完成
- 如果用户说“继续改刚才那个应用”，优先找上一次返回里的 `threadId`，走 `modify`
- `modify` 完成后，仍然可以继续用新的 `taskId` 走 `query`
- 不要把 `modify` 理解成“覆盖旧应用”，它更像是在同一个应用会话中追加新需求
- `attachments` 不改变 `create/query/modify` 的主流程，它只是给 `create` 增加文件输入
- 如果附件是 Excel，优先理解为 `create` 场景；`modify` 当前不要使用附件参数
- `--skills` 不改变 `create/query/modify` 的主流程，它只是给 `create` 或 `modify` 增加官方附加能力

推荐决策顺序：

1. 先判断用户是不是要“生成一个新的应用/系统”
2. 如果是，直接用 `aiapp create`
3. 如果用户是在已有应用基础上继续改，用 `aiapp modify`
4. 如果只是查进度或结果，用 `aiapp query`

## 查询与轮询预期

`aiapp` 的创建和修改通常不是秒级任务，而是分钟级任务。

- 简单应用通常需要几分钟
- 复杂应用可能持续十几分钟到几十分钟

因此在使用 `query` 时，不要假设“连续几次没有完成”就代表任务卡死。

### 正确的轮询策略

- 不要每 5-10 秒高频轮询
- 建议以 30 秒为默认轮询间隔
- 如果任务已经明确进入长流程，可进一步放宽到 60 秒
- 除非用户明确要求实时刷新，否则不要自行创建过于频繁的定时查询

### 代理行为约束

当 agent 已经帮用户发起了 `aiapp create` 或 `aiapp modify` 后，不要只查询 1-2 次就把后续进度甩给用户自己处理。

- 只要任务仍处于 `queued` 或 `running`，且用户没有明确要求停止，agent 应继续主动跟踪进度
- 如果当前会话无法持续等待，可以创建定时任务继续查询，但定时任务必须是“以拿到最终结果为目标”的，而不是无条件永久轮询
- 如果返回了 `threadViewUrl`，可以告诉用户“你也可以点这个链接查看实时进度”，但这不能替代 agent 自己继续追踪任务

### 定时任务规则

如果 agent 选择创建定时任务来继续查询 `aiapp` 状态，必须同时设置结束条件：

- 当 `status = succeeded` 且已拿到 `appPreviewUrl` 时，立即停止后续查询，并主动删除或关闭该定时任务
- 当 `status = failed` 时，立即停止后续查询，并主动删除或关闭该定时任务
- 不要在已经拿到目标结果后继续保留定时任务
- 不要创建“只会查询、不会自终止”的定时任务

换句话说，定时任务应当是“查询直到成功或失败，然后自动结束”的一次性追踪任务，而不是长期后台轮询

### 如何理解 `query` 返回

`query` 的返回里，除了总状态外，还可能包含进度信息，例如：

- 当前正在执行哪个步骤
- 已完成多少步骤
- 最近一次活动时间
- 当前步骤列表
- 当前 thread 的访问链接

因此判断任务是否还在推进时，不要只看 `status` 是否还是 `running`，还要结合以下字段一起看：

- `updatedAt` / `lastActivityAt`
- `progress.summary`
- `progress.currentStep`
- `progress.steps`
- `threadViewUrl`

只要这些字段仍在变化，通常就说明任务仍在正常推进。

### `threadViewUrl` 与 `appPreviewUrl` 的区别

`query` 或 `create` 的返回里，可能会有两个不同的链接：

- `threadViewUrl`：当前 thread 的访问链接。用于打开 `aiapp` 站点里的会话/生成进度页面，适合在应用还没生成完成时查看实时状态
- `appPreviewUrl`：最终生成出的应用访问地址。只有应用创建成功后才会出现，适合真正打开应用去使用

不要把这两个链接混用：

- 任务还在 `queued/running` 时，优先使用 `threadViewUrl`
- 任务 `succeeded` 且结果里已有 `appPreviewUrl` 时，再使用 `appPreviewUrl`

### 何时停止轮询

- 当 `status = succeeded` 时，停止轮询，并从结果中提取 `threadId`、应用链接等信息
- 当 `status = failed` 时，停止轮询，并向用户报告失败
- 当任务长时间保持 `running`，但 `lastActivityAt` 和 `progress` 都没有变化时，再考虑提示可能卡住，而不是过早下结论
- 如果此前为查询进度创建了定时任务，在 `succeeded` 或 `failed` 后要主动删除该定时任务，而不是留给用户处理

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `create` | `taskId`, `threadId`, `threadViewUrl` | query 的 --task-id, modify 的 --thread-id，或直接打开进度页 |
| `query` | `threadViewUrl`, `appPreviewUrl`, 应用详情 | 查看进度页，或在成功后打开应用预览并决定是否 modify |

## 注意事项

- `create` 通过自然语言 prompt 描述应用功能，系统自动生成
- 对“应用/系统/工具/后台/原型”类需求，优先使用 `aiapp`，不要改用其他产品命令模拟实现
- 对“根据附件 / 根据 Excel / 根据上传表格生成应用”这类需求，也优先使用 `aiapp`
- `attachments` 需要按对象数组传递，当前结构是 `[{name,type,url,size}]`
- `create` 可使用 Excel、图片等附件；`modify` 当前不支持附件参数
- `--skills` 传入官方技能 UID 列表，逗号分隔，可增强应用能力
- `modify` 需要 `--thread-id` 来标识修改哪个会话线程
- 应用未生成完成前，如果需要让用户查看实时状态，优先使用返回里的 `threadViewUrl`
- 应用生成成功后，如果需要真正打开应用，再使用 `appPreviewUrl`
- 创建后可通过 `query` 轮询 `taskId` 查看创建进度，但轮询间隔默认应为 20-30 秒，不要 10 秒级频繁轮询
- agent 不应只查 1-2 次就放弃；如果任务仍在运行，应继续跟踪，或创建带终止条件的定时任务继续跟踪
- 如果 agent 创建了用于查询进度的定时任务，在任务成功或失败后必须主动删除或结束该定时任务
- 不要直接使用用户本地文件路径作为附件 URL，必须先上传转换为互联网可访问链接

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [aiapp_create_and_poll.py](../../scripts/aiapp_create_and_poll.py) | 一键创建 AI 应用并自动轮询进度直到完成 | `python aiapp_create_and_poll.py --prompt "创建一个天气查询应用"` |
