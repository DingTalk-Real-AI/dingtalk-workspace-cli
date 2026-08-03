# IM Chat Skill 精简与渐进加载优化方案

## 1. 背景

当前 `im-chat-skill-hint-align` 分支已经增强了 Chat Skill 的 Shortcut 路由、执行骨架、身份边界、查询与资源处理、低频原子回退和错误导航。与 Lark IM Skill 对比后，DWS 在 Agent 路由和执行约束上更直接，但根 Skill 仍存在以下问题：

- 高频执行骨架与核心意图表重复。
- Runtime Shortcut Catalog、leaf Schema 和 leaf Help 的读取规则在多处重复。
- Shortcut 错误处理与 Workflow 错误导航重复。
- 身份、ID 和三种置顶对象的边界分散在不同章节。
- 缺少简短、集中且可复用的核心对象与查询结果语义。
- Frontmatter 能力召回仍可扩展，但不能削弱 DING、邮件和班级群等产品边界。

本方案只调整根文件 `skills/multi/dingtalk-chat/SKILL.md` 的组织和必要语义，不把 API 手册、完整 Shortcut 清单或权限表重新放回根 Skill。

## 2. 修改目标

1. 提高高频 Chat 意图的直接命中率，减少不必要的 Catalog、Schema 和 `--help` 调用。
2. 提前建立渐进加载顺序，避免模型在高频任务中优先进入原子命令树。
3. 补齐身份、核心对象、ID 和查询结果的必要语义，降低错误重试和 ID 混用。
4. 合并重复 SOP，确保新增内容不会增加根 Skill 的总体 token。
5. 保持参数、安全和完整 API 事实由 leaf Schema、Help 和 references 按需提供。

## 3. 设计原则

### 3.1 根 Skill 只保留决策必需信息

根 Skill 应负责：

- Skill 触发范围和跨产品排除边界。
- 渐进加载与能力选择顺序。
- 高频意图到精确 Shortcut 的映射。
- 身份、ID、幂等、分页、部分失败等跨命令不变量。
- 低频能力的导航入口。
- 错误恢复和停止条件。

以下内容继续留在 Schema 或 references：

- 完整 Shortcut Catalog。
- API Resources 全量列表。
- 权限 scope 表。
- 叶级参数全集和接口字段格式。
- 只服务单个命令的实现细节。

### 3.2 渐进加载规则必须早于命令骨架

模型应在看到具体命令前，先知道何时直接执行、何时才加载额外上下文。但完整一级命令树不应提前，以免低频原子命令干扰高频 Shortcut 选择。

### 3.3 同一事实只保留一个权威位置

- 高频 Shortcut 只出现在一张核心意图表中。
- Catalog、Schema 和 Help 的读取顺序只定义一次。
- 错误恢复与 reference 导航只定义一次。
- 身份、对象和 ID 的公共边界集中定义，后续章节只引用，不重复解释。

## 4. 目标章节结构

```text
1. Frontmatter
2. Preconditions
3. 加载与路由顺序
4. 核心对象与 ID
5. 核心意图与执行骨架
6. 统一发送
7. 查询、资源与卡片
8. 低频原子路由
   ├── 一级命令树
   ├── branch references
   └── 低频操作回退表
9. 错误恢复与按需 Reference
10. 跨产品协作
```

## 5. 具体修改

### 5.1 扩展 Frontmatter 产品能力

扩展 `description` 的正向能力召回，覆盖：

- 单聊、群聊、建群、群搜索和群成员管理。
- 消息发送、回复、转发、撤回、查询和聊天记录搜索。
- 图片、文件和消息资源下载。
- 表情回应、收藏、Pin、消息置顶和会话置顶。
- 应用机器人、Webhook 和互动卡片。
- 未读、红点、消息已读状态和会话分类。

同时保留明确排除：

- DING、短信和电话转到 `dingtalk-ding`。
- 邮件转到 `dingtalk-mail`。
- 班级群转到对应的低频产品 Skill。
- 找人本身由 `dingtalk-contact` 或 `dingtalk-aisearch` 负责，Chat 只消费真实人员 ID。

Frontmatter 只描述真实能力和路由边界，不加入参数、SOP 或 token 实现细节。

### 5.2 前移并合并渐进加载规则

将现有“Shortcut 发现”“Shortcut 执行契约”和“渐进加载与一级路由”的加载决策部分合并为紧随 Preconditions 的唯一章节：

```markdown
## 加载与路由顺序

1. 已知高频意图：直接使用“核心意图与执行骨架”，不查 Help。
2. 已有匹配 Shortcut：直接执行；参数、约束或安全不确定时才查 leaf Schema。
3. 仅 Cobra flags 不确定时查 leaf `--help`。
4. 现有路由无法定位低频能力时，才查 Runtime Shortcut Catalog。
5. 没有 Shortcut 时，按需读取对应 branch reference，进入原子命令。
```

同一章节保留以下公共规则：

- 路由优先级为 `exact recipe/runnable script > public Shortcut > atomic command`。
- 不猜测 `cli_path` 或参数名称。
- `confirmation=user_required` 时先确认，再添加 `--yes`。
- 来源冲突时采用更安全的解释并报告契约漂移。
- 命令已确定且参数清楚时直接执行，不为验证已知路径重复发现。

删除其他章节重复出现的 Catalog、Schema、Help 选择说明。

### 5.3 新增“核心对象与 ID”小表

增加不超过 8 行的表格，集中表达：

| 对象 | 核心标识与边界 |
|---|---|
| 人员 | 姓名必须先解析成唯一真实的 `userId` 或 `openDingTalkId`，名称不能作为 ID 传递 |
| 会话 | 使用真实 `openConversationId` / cid；群名只能用于 Shortcut 的目标解析 |
| 消息 | 使用真实 `openMessageId` / msgId，并保持与身份及会话一致 |
| 发送任务 | `openTaskId` 只用于查询发送状态，不能替代消息 ID |
| Thread | thread/topic ID 必须绑定真实会话，不跨会话复用 |
| 身份 | current-user、app-bot 和 Webhook 是不同操作者，不能自动互换 |
| 状态 | 收藏、消息置顶、消息 Pin 和会话置顶作用于不同对象 |

新增后删除后文对这些边界的重复说明。

### 5.4 合并高频骨架与核心意图表

删除独立的“高频直接执行骨架”，将其全部合入唯一的“核心意图与执行骨架”表。表格固定为三列：

| 用户意图 | 精确 Shortcut 骨架 | 必须保留的执行边界 |
|---|---|---|

至少覆盖以下高频场景：

- 姓名发单聊、群名发群消息。
- user、bot、webhook 三种身份发送。
- 建群、改群名、拉人和成员查询。
- 拉取会话消息、查询详情、撤回和发送状态。
- 关键词搜索、组合搜索和查询 @ 我的消息。
- 群邀请链接和群机器人。
- 会话置顶和收藏列表。
- 查和某人的聊天记录。
- 群消息翻页导出。
- 机器人多群广播。

表中直接给出正确参数骨架；命中后照抄参数名，不先调用 `--help`。同一个 Shortcut 不再在其他表中重复列出。

### 5.5 补充统一身份规则

在“统一发送”开头加入统一规则：

> 身份决定真实操作者、可见范围和可用能力；同一目标使用 user、bot 或 webhook 时，结果和权限可能不同，禁止自动切换身份重试。

继续保留：

- 发送前检查身份、目标、正文、标题、@、消息类型和附件路径。
- 重试复用相同 `--idempotency-key`。
- user、bot、webhook 的精确发送模板。
- @ 占位符、新行和文件能力边界。

不加入 Lark 的 access token 类型说明。

### 5.6 补充查询结果与增强失败语义

在“查询、资源与卡片”中增加：

- 发送者名称缺失时保留真实 ID，不猜姓名，也不自动扩大通讯录查询。
- 可选增强字段缺失不代表主查询失败；增强请求失败时保留主结果并写入 per-item ledger。

继续保留：

- `--page-all` 只在确需完整分页时使用。
- 部分失败保留已有结果，禁止把不完整结果声明为完整。
- 资源下载默认关闭，显式请求后才增加请求和本地输出。
- 子消息资源优先使用子 `messageId`。
- 输出路径、覆盖、HTTPS 和重定向安全限制。

### 5.7 拆分“渐进加载”与“一级命令树”

前移的只有加载决策。完整一级命令树及 branch references 改名为“低频原子路由”，保留在高频意图、统一发送和查询规则之后。

这样可以：

- 防止高频任务优先进入 atomic branch。
- 降低不必要的 Schema 和 Help 查询。
- 继续为没有 Shortcut 的能力提供确定导航。

低频原子回退表继续保留收藏、编辑、外部群升级、群昵称、分类、共同群、群公告、群身份、置顶、未读、已读、授权、退群和解散群等差异化入口。

### 5.8 合并错误恢复与 Workflow 导航

将现有“Shortcut 错误处理”和“Workflow 与错误导航”合并为：

```markdown
## 错误恢复与按需 Reference
```

只保留以下规则：

- 路径或参数错误时，按 Catalog、Schema、Help 的既定顺序校正一次。
- 始终从实际输出重新提取下游 ID。
- 复杂消息任务按需读取 `01-messaging.md`。
- Onboarding 按需读取对应 workflow。
- 命令错误按需读取 `chat-error-recovery.md`。
- 权限不足、歧义未消除、无结果或契约冲突时停止并报告。

删除其他位置重复的 `01-messaging.md` 和错误恢复入口。

### 5.9 保留跨产品协作边界

继续保留根 Skill 中不可由 Chat 自己完成的路由：

- 人名解析到 Contact / AISearch。
- DING、短信和电话到 Ding Skill。
- 邮件到 Mail Skill。
- 本地文件与已有 mediaId 的发送差异。

如果某项边界已经在 Frontmatter 或核心对象表中完整表达，正文只保留执行阶段真正需要的补充，不重复整段说明。

## 6. 删除与合并清单

| 当前内容 | 处理方式 |
|---|---|
| “Shortcut 发现（按需）” | 合入前置“加载与路由顺序” |
| “Shortcut 执行契约” | 公共规则合入前置章节 |
| “高频直接执行骨架” | 删除，内容合入核心意图表 |
| “渐进加载与一级路由”中的加载说明 | 前移并去重 |
| 完整一级命令树 | 保留，改放“低频原子路由” |
| “Shortcut 错误处理” | 合入统一错误章节 |
| “Workflow 与错误导航” | 合入统一错误章节 |
| 分散的身份、ID、置顶说明 | 合入核心对象表或统一身份规则 |
| 重复的 `01-messaging.md` 入口 | 只保留一处 |

## 7. 不纳入本次修改

- 不展开 97 个公开 Shortcut。
- 不复制 Lark 的完整 API Resources 和权限 scope 表。
- 不在根 Skill 中维护 leaf 参数全集。
- 不引入与当前 CLI 不一致的新命令或参数。
- 不改变 Schema、Help、Runtime Catalog 和 reference 的事实优先级。
- 不通过增加默认查询、自动通讯录查询或默认资源增强来换取结果丰富度。

## 8. 实施顺序

1. 更新 Frontmatter description，确认能力召回和排除边界。
2. 合并并前移“加载与路由顺序”。
3. 新增“核心对象与 ID”表，删除相应重复边界。
4. 合并高频骨架和核心意图表。
5. 补充统一身份规则。
6. 补充查询结果和增强失败语义。
7. 将一级命令树调整为后置的“低频原子路由”。
8. 合并错误恢复与 reference 导航。
9. 全文检查重复命令、重复 reference 和冲突参数。
10. 运行 Skill 格式及相关策略测试，并用高频场景做静态路由验证。

## 9. 验收标准

### 9.1 内容与结构

- 根 Skill 中只有一份 Catalog、Schema、Help 读取顺序。
- 根 Skill 中只有一张高频意图与 Shortcut 骨架表。
- 根 Skill 中只有一个错误恢复章节。
- `01-messaging.md` 的同类导航不重复。
- 核心对象与 ID 表不超过 8 行数据。
- 一级原子命令树位于高频路由之后。
- Frontmatter 同时覆盖主要产品能力和明确排除边界。

### 9.2 执行行为

- “发给某人”“发到某群”“查 @ 我”“改群名”等高频意图可直接选中已评审 Shortcut，不先查 Help。
- 低频未知意图才触发 Runtime Shortcut Catalog。
- 参数或安全不确定时读取 leaf Schema；只有 Cobra flags 不确定时读取 leaf Help。
- user、bot、webhook 不被自动互换。
- `openTaskId`、消息 ID、会话 ID 不混用。
- 发送者名称或 reaction 等增强缺失时，不把主查询误判为失败。
- 部分失败保留已有结果并明确报告 ledger/completeness。

### 9.3 Token 与维护成本

- 修改后的根 Skill 不超过当前 211 行，并以不丢失必要路由和边界为前提尽量低于 195 行。
- 文件单词数和字符数不高于修改前基线。
- 新增内容通过删除重复 SOP 抵消。
- 不新增完整 Shortcut、API 或权限清单。

## 10. 预期收益

- 减少高频任务中的 `--help` 和重复 Schema 查询。
- 降低因身份切换、ID 混用和发送者名称缺失导致的错误重试。
- 让 Shortcut、Schema、Help、Catalog 和 references 各自保持单一职责。
- 在不增加默认 token 和耗时的前提下，提高 Chat Skill 的选择准确率和执行成功率。
- 降低后续新增 Shortcut 时同时维护多张表和多处规则的漂移风险。
