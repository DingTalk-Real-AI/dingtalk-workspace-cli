# PR #860 Multi IM 优化：合并后完整代码分析与跨场景复用报告

> PR #860：[DingTalk-Real-AI/dingtalk-workspace-cli#860](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/860)
>
> #860 当前标题：`feat(im): harden Multi IM and publish complete Chat Schema`
>
> #860 当前状态：Open、可合并；核对时共 14 个提交、129 个变更文件，+69,391 / -13,045。
>
> 唯一分析范围：`main@6376f294dab0d0a7bd2f5904077aab89383684d1...codex/multi-im-optimization@33ceab6000d5f18eddbc1c0c97544233f5dd42d6`。
>
> 报告原则：只分析当前 PR #860 的合并后整体代码。提交来源和中间分支不再作为独立分析单元；所有规模、架构、文件、风险和复用结论均以当前 #860 的完整 diff 为准。

## 1. 核心优化思路

### 1.1 一句话结论

PR #860 的本质不是增加一批 IM 命令，而是把 Chat/Event 能力从“分散、可调用”升级为“意图可路由、目标可确定、执行有边界、输出有契约、风险可确认、能力可发现、文档与代码可持续校验”的完整产品链路。

整体优化链路可以概括为：

```text
用户意图
  -> Golden Route 唯一入口
  -> Typed Resolver 确定目标
  -> Identity / Capability Preflight
  -> Bounded Runtime + Typed Confirmation
  -> Versioned Result / Error Ledger
  -> Authored Registry + Selection/Metadata
  -> Generated Schema + Skill/Reference
  -> Policy + Drift + Coverage Gates
```

### 1.2 核心方法总览

| 原有问题 | 核心优化方法 | PR #860 整体落地 | 可复用价值 |
|---|---|---|---|
| 同一意图有多条相似入口 | **Golden Route**：每个意图只推荐一个默认入口 | 统一发送、回复、读取、搜索、建群和监听入口，原始命令只作有理由的 fallback | 通讯录、文档等领域都可先按意图收敛入口 |
| 名称、ID、URL 等目标解析分散 | **Typed Resolver**：业务执行前先得到规范化目标 | 新增 `targetresolver`，全分页检索、精确匹配、歧义和不完整结果均 fail closed | 可扩展为 `DirectoryResolver`、`DocumentTargetResolver` |
| 调用前才发现参数、身份或能力不支持 | **Local Validation + Capability Check** | 建立 user/bot/webhook 能力矩阵，并前置校验时间、页大小、回溯窗口和目标互斥 | 适用于邀请、删除、文档覆盖、权限修改等写操作 |
| 输出依赖原始 API 或展示文本 | **Versioned Contract**：输出稳定的机器契约 | 建立并发布 `im.message-list.v1`，统一消息、引用、资源、reaction 和 continuation | 可定义联系人、文档、云盘的 v1 投影 |
| 分页、导出、下载可能截断或留下半成品 | **Bounded Execution**：限制页数、条数、超时并支持原子提交 | 实现游标停滞检测、完整性账本、安全下载、no-clobber 和跨平台原子替换 | 适用于批量导出、备份、迁移和同步 |
| 高风险命令示例可能绕过确认 | **Typed Confirmation**：Runtime 强制停下，确认后才执行 | 写入/破坏型 Schema metadata 与 Runtime gate 对齐，危险示例不预填 `--yes` | 可用于删除联系人、覆盖文档、权限变更等高风险动作 |
| 能力可运行但 Agent 不可发现 | **Authored Registry + Generated Schema** | Chat Shortcut 达到 97/97；完整 Catalog 为 26 products / 898 tools | 适用于任何具有大量组合命令的产品域 |
| 文档、Schema、Runtime 各自演化 | **Consistency Gates**：把一致性变成 CI 规则 | 增加 Skill Chain、Schema compatibility、interface integrity、generated drift 和 changed-code coverage | 可抽象为所有领域共用的质量框架 |

### 1.3 八个可复用的优化原则

#### 原则一：从“API 优先”转为“意图优先”

先定义用户究竟要完成什么，再为每个意图选择唯一 Golden Route。底层 API、历史命令和兼容别名可以保留，但不应继续出现在默认发现路径中。这样能够显著减少 Agent 在多个相似工具之间随机选择的问题。

#### 原则二：把目标解析变成独立基础设施

所有操作先经过统一 Resolver，将名称、ID、手机号、URL 等输入转换为类型明确的 canonical target。业务命令只消费解析结果，不再各自实现搜索、歧义处理或 ID 猜测。

#### 原则三：把写操作拆成“计划”和“执行”

执行前明确目标、身份、能力、副作用和恢复方式。高风险操作只有在目标唯一且能力满足时才进入执行阶段。这种拆分既降低误操作风险，也让 dry-run、确认和审计更容易实现。

#### 原则四：用版本化域模型隔离上游变化

列表、搜索、读取和写回执都投影到稳定的领域契约。调用方依赖 `messageId`、`canonicalId`、`nextCursor` 等规范字段，而不是供应商原始响应或终端展示文字。

#### 原则五：所有长流程都必须有边界和恢复语义

分页、导出、下载、批量发送不能只保证 happy path；还要定义最大页数、最大条数、游标推进、临时文件、原子替换、checkpoint、部分失败和幂等重试。

#### 原则六：让错误直接驱动下一步动作

错误需要区分未命中、歧义、身份不匹配、能力不足、权限不足、限流和服务端故障，并明确是否可重试以及建议动作。对 Agent 而言，错误是控制流的一部分，而非日志文本。

#### 原则七：把知识一致性纳入工程门禁

Skill 负责路由，Reference 负责细节，Schema 负责机器发现，Runtime 负责真实行为，Policy 负责检查四者闭环。任何新增能力都应同步更新并由测试验证。

#### 原则八：把“正式发布”定义为独立交付阶段

Runtime 中存在一个命令，不等于 Agent 已经能够正确使用它。能力还必须进入 authored Registry，具备 use/avoid hints、风险和确认 metadata，生成 Schema/Agent metadata，并退出临时 exclusion。PR #860 将这一原则具体化为 Chat 97/97、public Shortcut 265/265 和完整 Catalog 898 tools。

### 1.4 标准执行链路

```text
用户自然语言意图
  |
  v
Golden Route：选择唯一推荐入口
  |
  v
Typed Resolver：解析用户、群、会话、消息等目标
  |
  v
Preflight Plan：固化 profile、身份、能力和副作用
  |
  v
Bounded Runtime：在分页、超时、数量和文件边界内执行
  |
  v
Versioned Result / Typed Error Ledger：返回可消费、可恢复结果
  |
  v
Authored Registry + Selection/Metadata：正式发布并指导选择
  |
  v
Generated Schema + Skill/Reference + Policy：持续防止契约漂移
```

### 1.5 复用时应迁移整条链路

不建议只复制某个 shortcut 或 Resolver。若新领域仍缺少稳定输出、错误分类和一致性门禁，局部复用很快会重新产生分散实现。通讯录、文档、云盘、知识库等场景应至少成组迁移以下能力：

```text
意图清单 + Golden Route
  -> 统一 Resolver
  -> Plan / Capability Check
  -> 有界 Runtime
  -> 版本化结果与结构化错误
  -> 分层文档、Schema 与 Policy
```

## 2. 分析范围与阅读导航

### 2.1 变更规模与边界

本报告只使用一个三点对比范围：`6376f294...33ceab60`。它表示 PR #860 当前 head 相对当前 PR base 的完整净变化：

| 指标 | 当前 PR #860 |
|---|---:|
| 状态 | Open / mergeable |
| 提交数 | 14 |
| 变更文件 | 129 |
| 新增 / 删除 | +69,391 / -13,045 |
| 生产 Go 文件 | 38（+4,028 / -737） |
| Go 测试文件 | 30（+4,343 / -88） |
| Skill Markdown | 30（+1,166 / -1,577） |
| 主要生成 Schema 文件 | 6（+57,215 / -9,229） |

#860 变更横跨：

- Runtime：`internal/app`、`internal/shortcut`、`internal/helpers`、`internal/errors`。
- 目标解析：新增 `internal/shortcut/targetresolver`。
- Schema：chat/event 的 catalog、agent metadata、hints、registry。
- Skill：mono、chat、event、shared 四类文档链路。
- Policy：新增 Multi IM Skill Chain 检查并扩展上下文预算检查。
- 测试：单元测试、契约测试、mock MCP smoke test、live audit 辅助脚本。

这种范围说明优化目标不是“让某几个命令能跑”，而是建立可持续扩展的产品契约。

69,391 行新增中，大量内容来自 Schema catalog 和 Agent metadata 等生成物，不能把总行数等同于手写业务逻辑。人工设计变更主要集中在：

- Chat Shortcut 的 authored CommandRegistry、selection hints 和风险 metadata；
- 本地参数校验、确认门禁、cursor 修复、shorthand 和跨平台路径；
- Schema 完整性、目标消歧、分页、原子文件及跨平台 changed-code 覆盖测试；
- Chat、Event、Shared 和 Mono 四层文档链路重构。

### 2.2 文档阅读导航

| 阅读目标 | 建议章节 |
|---|---|
| 快速掌握本次优化的方法论 | 第 1 章“核心优化思路” |
| 理解为什么需要改造 | 第 3 章“优化前的典型问题” |
| 理解 Skill、Runtime、Schema、Policy 如何协同 | 第 4 章“总体架构” |
| 查看具体代码机制 | 第 5 章“核心代码优化分析” |
| 查看文档组织变化 | 第 6～8 章 |
| 查看验证方法与质量门禁 | 第 9 章 |
| 复用到通讯录、文档、云盘和知识库 | 第 10～12 章 |
| 查看当前风险收口和整体结论 | 第 13～14 章 |
| 查看 Schema 发布、Runtime 收口和文件分类 | 第 15～18 章 |
| 查看 Go 生产代码和测试代码的专项总结 | 第 19 章 |
| 查看当前 #860 的整体价值与建议 | 第 20～21 章 |

### 2.3 结论类型说明

为避免把“PR 已实现内容”和“建议复用方案”混在一起，后文按以下方式理解：

- **本次落地**：能够在该 PR 的代码、Schema 或文档变更中找到直接依据。
- **分析判断**：根据多处变更归纳出的设计意图或工程影响。
- **复用建议**：面向通讯录、文档等新领域给出的迁移设计，并非声称已在本 PR 中实现。

## 3. 优化前的典型问题

### 3.1 同一意图存在多条入口

发送消息、查看群消息、回复消息等意图可能分别出现在 shortcut、底层 OpenAPI 命令、脚本和多份教程中。模型或用户容易选中功能相似但约束不同的路径，导致参数漂移、行为差异和排障困难。

### 3.2 目标解析散落在业务命令中

用户可能输入 conversation ID、openConversationId、群名、用户 ID、手机号或自然语言名称。若每条命令各自解析，就会出现：

- 同一个名称在不同命令中命中不同对象；
- 多结果时有的命令报错、有的静默选第一个；
- ID 类型混用；
- profile、tenant 或身份上下文不一致；
- 新增一种目标类型时需要修改大量命令。

### 3.3 列表输出面向人而非机器

原始 API 数据结构大而不稳定，字段含义不统一，分页元信息可能丢失。Agent 需要猜测正文、发送者、消息 ID 和时间字段，后续回复、导出和审计难以可靠衔接。

### 3.4 长流程缺少完整性边界

消息列表、成员列表、导出、下载等操作天然包含分页或文件写入。如果没有最大页数、最大条数、下一页游标、临时文件和原子替换机制，容易发生无限翻页、半文件、重复结果或悄然截断。

### 3.5 文档与 Runtime 独立演化

命令已经变更但文档仍指向旧路径，或者 Skill 声称支持某个字段而 Runtime 没有对应输出。问题往往到真实 Agent 调用时才暴露。

## 4. 总体架构：四层协同

### 4.1 Skill 层：告诉 Agent 应该走哪条路

Skill 的职责不是罗列所有命令，而是完成意图路由、默认值选择、风险提示和按需加载 Reference。它应该短、稳定、偏决策，不应复制完整 API 手册。

### 4.2 Runtime 层：保证行为确定且可测试

Runtime 负责统一解析目标、执行能力校验、分页、结果投影、原子文件操作和结构化错误。即使调用者没有阅读文档，Runtime 也应保持安全边界。

### 4.3 Schema 层：提供机器可发现的真实能力

Schema catalog、agent metadata、selection hints 和 command registry 必须共同描述参数、枚举、返回值和入口选择。Schema 不是生成物附件，而是 Agent 运行时能力的一部分。

### 4.4 Policy 层：防止三层再次漂移

Policy 检查关键意图是否同时存在合法 Runtime 路由、Schema 暴露和文档说明，并控制 Skill 的上下文体积。它把系统设计转化为可持续执行的合并门槛。

## 5. 核心代码优化分析

本章按照“目标识别—路由决策—读取契约—长流程执行—错误与兼容治理”的顺序展开。各项机制之间不是孤立关系：Resolver 提供可靠目标，身份/能力矩阵选择执行路径，版本化投影稳定结果，分页与原子写保证完整性，错误和 profile 契约负责失败恢复与上下文一致性。

| 分析主题 | 主要代码区域 | 解决的问题 |
|---|---|---|
| Target Resolver | `internal/shortcut/targetresolver` | 统一名称、ID 和自然语言目标解析 |
| 身份与能力路由 | `internal/shortcut/chat/*contract.go`、`unified_send.go` | 在执行前选择合法发送身份和路径 |
| 消息投影 | `chat_message.go`、`smart/chat_messages.go` | 将异构响应转换为稳定消息模型 |
| 分页与导出 | conversation/message/group/export 相关实现 | 防止静默截断、无限翻页和半成品文件 |
| 事件监听 | `internal/app/event_listen_im.go` | 将 IM 事件封装为显式生命周期能力 |
| 错误与上下文 | `internal/errors`、failure classifier、`profilectx` | 提供可行动错误并防止跨 profile 漂移 |
| 兼容治理 | 参数别名、Schema metadata、selection hints | 保持旧调用可用，同时引导新调用走 Golden Route |

### 5.1 统一 Target Resolver

新增的 `internal/shortcut/targetresolver/resolver.go` 将目标识别从各业务命令抽离。Resolver 的价值在于把“输入是什么”与“接下来执行什么业务”分开。

统一解析应遵循如下优先级：

1. 显式类型化 ID 优先，避免把合法 ID 当名称搜索。
2. 显式 target kind 优先于启发式判断。
3. 自然语言名称进入搜索，并保留候选集。
4. 唯一命中才自动继续；多结果返回可选择候选。
5. 未命中返回稳定错误码和建议动作。
6. 解析结果携带 profile、tenant、来源和置信信息。

这样做带来三个直接收益：

- chat、send、history、members 等命令共享完全一致的目标语义；
- 新增手机号、邮箱、unionId 等输入方式时只扩展 Resolver；
- 解析阶段可以独立测试，不需要触发真实写操作。

#### 向通讯录迁移

可建立 `DirectoryResolver`：

```text
输入：姓名 / 手机号 / 邮箱 / userId / unionId / 部门名 / deptId
输出：TypedDirectoryTarget
  kind: user | department
  canonicalId
  displayName
  matchedBy
  candidates
  profileContext
```

删除、调岗、批量邀请等高风险动作只接受唯一、类型确定的解析结果。

#### 向文档迁移

可建立 `DocumentTargetResolver`：

```text
输入：URL / docId / workspaceId / spaceId / 标题 / 路径
输出：TypedDocumentTarget
  product: doc | drive | wiki
  resourceType
  canonicalId
  containerId
  matchedBy
  candidates
```

这可消除文档 URL、知识库节点 ID、云盘文件 ID 混用造成的问题。

### 5.2 Unified Send 与身份/能力矩阵

`unified_send.go`、`identity_contract.go` 和 `im_contract.go` 将发送动作从“按 API 名称选命令”提升为“按目标、身份与消息能力选择执行器”。

推荐的决策维度包括：

| 维度 | 示例 |
|---|---|
| 目标 | 单聊、群聊、机器人会话 |
| 身份 | 用户身份、机器人身份、应用身份 |
| 内容 | text、markdown、card、media |
| 能力 | 是否允许主动发送、是否支持回复、是否支持卡片更新 |
| 上下文 | profile、tenant、conversation |

这种能力矩阵比在每条命令内写条件分支更易审计。发生能力不匹配时，应在真正发起网络请求前失败，并返回建议使用的 route。

#### 可复用原则

通讯录中的“读取、邀请、修改、删除”，文档中的“读取、创建、编辑、分享、删除”，都应建立身份与能力矩阵。调用者表达的是业务意图，Runtime 决定具体执行入口。

### 5.3 `im.message-list.v1` 消息投影

消息列表相关代码将上游异构响应投影成稳定的版本化结构。核心思想是：保留原始信息的同时，提供统一的机器字段。

典型规范字段包括：

- `messageId`
- `conversationId`
- `senderId` / `senderName`
- `messageType`
- `text` 或规范化内容
- `createdAt`
- `replyToMessageId`
- `raw`（必要时用于诊断）
- `hasMore` / `nextCursor`

版本号的意义是允许未来增加或调整字段，而不破坏依赖 v1 的 Agent 与脚本。

#### 可复用原则

- 通讯录：定义 `directory.user-list.v1`、`directory.department-list.v1`。
- 文档：定义 `document.search-result.v1`、`document.block-list.v1`。
- 云盘：定义 `drive.file-list.v1`、`drive.transfer-result.v1`。

不要让自动化直接依赖供应商原始响应；应通过投影层建立稳定域模型。

### 5.4 分页完整性与有界执行

PR 统一增强了消息、会话和成员列表的分页处理。一个健壮的列表命令至少需要：

- 明确单页与自动翻页模式；
- 返回 `hasMore` 和 `nextCursor`；
- 支持 `maxPages`、`maxItems` 等硬边界；
- 检测游标不前进或循环；
- 去重并保持稳定顺序；
- 明确结果是完整集合还是截断集合；
- 在中途失败时返回已完成量和失败位置。

分页不是 UI 细节，而是数据完整性契约。尤其在“先查再写”的 Agent 流程中，静默截断会导致错误对象被操作。

#### 当前需关注的问题

本次分析发现 `+conversation-list` 的单页模式存在一个边界风险：代码可能在解析下一页游标前提前退出，最终出现 `hasMore=true` 但 `nextCursor=0`。这不会否定整体设计，但应在合并前补回归测试并修复。

### 5.5 原子导出与资源下载

新增或强化的消息导出、资源下载路径采用“临时文件 -> 完整写入 -> 校验 -> 原子替换”的思路，避免失败时留下看似成功的残缺文件。

推荐流程：

```text
解析目标
  -> 校验输出路径与权限
  -> 创建同目录临时文件
  -> 有界分页拉取/流式下载
  -> 校验数量、大小或摘要
  -> fsync/close
  -> 原子 rename 到最终路径
  -> 返回 typed receipt
```

失败时删除临时文件，保留已有最终文件，并返回失败阶段、已处理数量及是否可重试。

#### 可复用原则

该模式适用于：通讯录批量导出、文档离线备份、附件下载、云盘迁移、知识库归档以及审批记录导出。

### 5.6 `+chat-create` 与 `+messages-reply`

补齐这类语义型 shortcut 的意义在于构建完整 Golden Route：用户不需要先理解底层产品 API，再自己拼接参数。命令命名直接对应业务意图，Schema 则描述必要约束。

设计原则是：

- Shortcut 只暴露稳定、常用的业务参数；
- 复杂底层参数放入高级 Reference；
- 输出统一 envelope；
- 目标解析复用 Resolver；
- 写操作具备 preflight、明确身份和结构化回执。

### 5.7 `event +listen-im`

新增 IM 事件监听入口，将底层事件总线能力收敛为 Agent 易理解的生命周期命令。文档进一步拆出 keys、lifecycle、operations、output 四个主题，说明事件能力被视为长期运行流程，而不只是一次 API 调用。

事件监听需要明确：

- start、status、stop 的生命周期；
- profile 与租户隔离；
- endpoint/key 的稳定标识；
- 前台与后台模式；
- 超时、重连和退出语义；
- 事件 envelope 与消息投影；
- 重复事件、顺序和幂等边界。

#### 可复用原则

通讯录变更事件、文档协作事件、云盘文件事件都可以复用同一监听骨架，只替换事件类型与投影器。

### 5.8 结构化错误与服务端失败分类

`internal/errors` 与 `server_failure_classifier.go` 将错误从一段不可解析文字升级为类型化结果。稳定错误至少应包含：

- `code`
- `message`
- `stage`
- `retryable`
- `details`
- `suggestedAction`
- 必要的 request/trace 标识

关键分类包括：参数错误、目标未命中、目标歧义、身份不匹配、能力不支持、权限不足、限流、服务端失败、分页协议异常和文件提交失败。

错误分类的主要消费者不只是人，还包括 Agent 的下一步决策。因此 `suggestedAction` 应可执行，例如“提供 conversationId”“选择一个候选用户”“切换 bot identity”，而非笼统写“请重试”。

### 5.9 Profile 一致性

新增 `profilectx` 并调整 auth/profile 相关逻辑，目的是保证目标解析、API 调用和事件监听使用相同的身份上下文。跨 profile 漂移是多租户 CLI 中非常隐蔽的风险：解析可能在 A 租户命中对象，写操作却在 B 租户执行。

可复用规则：

1. profile 在入口解析一次并向下传递；
2. Resolver 结果记录 profile/tenant；
3. 执行器不得静默回退到另一个默认 profile；
4. 批处理账本记录每项使用的 profile；
5. 事件监听 key 包含足够的上下文隔离信息。

### 5.10 隐藏兼容别名

PR 保留必要的参数或命令别名，但通过 metadata/schema 控制它们不再成为 Agent 的首选入口。这实现了“兼容旧调用，但不继续传播旧范式”。

正确的兼容策略应区分：

- Runtime compatibility：旧命令仍可运行；
- Discovery visibility：新用户和 Agent 默认看不到旧入口；
- Documentation priority：文档只推荐 Golden Route；
- Deprecation observability：能够统计旧入口使用情况；
- Removal criteria：达到明确阈值后再移除。

### 5.11 Shortcut 部分修改详解

当前 #860 共修改 44 个 `internal/shortcut` 文件，其中 26 个生产 Go 文件、18 个测试文件。它不是简单增加命令数量，而是把 Shortcut 从“调用 MCP 的便捷包装”升级为以下分层执行系统：

```text
Shortcut 声明与 Cobra 编译
  -> 本地参数/约束校验
  -> 自然目标解析与全量预检
  -> 身份/能力路由
  -> MCP 读写隔离与 dry-run
  -> 有界分页或批量执行
  -> 统一领域投影
  -> 完整性/失败 ledger
  -> 原子导出或安全资源落盘
```

#### 5.11.1 Shortcut 框架：声明能力，而不是在命令里重复装配 Cobra

`internal/shortcut/types.go` 和 `internal/shortcut/runner.go` 扩展了通用声明模型：

| 修改 | Runtime 行为 | 价值 |
|---|---|---|
| `Flag.Shorthand` | 所有 flag 类型统一通过 Cobra 的 `BoolP/IntP/StringP/StringSliceP` 注册 | `--output` 可安全提供 `-o`，不需要每个命令单独实现 |
| `Shortcut.Aliases` | 将兼容命令映射到同一个执行实现 | 保留旧调用，但 canonical Schema 和文档仍只推荐主入口 |
| `SinglePositionalAliasFor` | 单个位置参数在校验前归一化为指定 flag；与显式 flag 同时出现时拒绝 | 为搜索类命令提供兼容体验，同时避免形成第二套参数契约 |
| `Constraint` | `at_least_one`、`exactly_one`、`mutually_exclusive` 由 Runner 统一执行并投影到 Runtime Schema | Help、Runtime 与 Agent Schema 共用参数关系 |
| `Changed`、`StrFirst`、`IntFirst` | 能区分“用户显式输入”和“默认值”，并统一处理兼容别名 | 防止默认值错误覆盖旧参数或触发不适用校验 |

Runner 的执行顺序也被固定为：位置参数归一化 → required/enum 校验 → 跨参数约束 → 自定义校验 → 风险确认 → Execute。这样可以保证明显错误和未确认写操作不会到达 MCP。

读写调用进一步分离：`CallMCPData` 只允许符合只读命名契约的工具在 dry-run 中执行；`CallMCPWriteData` 在 dry-run 下直接 fail closed。多步写 Shortcut 必须自己输出 plan，不能借“为了拿返回值”之名在预览阶段执行写入。

#### 5.11.2 Target Resolver：把名称解析变成共享基础设施

`internal/shortcut/targetresolver/resolver.go` 新增了独立解析层，核心类型包括：

- `Status`：`resolved`、`ambiguous`、`not_found`、`incomplete`；
- `IdentityRequirement`：区分下游需要 `userId`、`openDingTalkId` 或任一稳定身份；
- `UserResolution` / `ChatResolution`：返回规范目标、匹配方式和 profile；
- `Reader`：只依赖只读 `CallMCPData`，便于 Smart Shortcut、事件入口和测试共同复用。

群目标解析采用以下 fail-closed 算法：

1. `cid...` 形式直接识别为稳定 `openConversationId`，不做名称搜索；
2. 群名调用 `im/search_groups`，每页 10 条，最多 40 页；
3. 对候选按稳定 ID 去重；精确名称优先，但多个精确同名仍然算歧义；
4. `hasMore=true` 时必须存在且推进 `nextCursor`；
5. 满页但无分页元数据、游标停滞或达到页数上限时返回 `resolution_incomplete`；
6. 只有完整候选集中的唯一目标才允许继续执行。

批量解析 `ResolveUsers/ResolveChats` 会先解析全部目标并收集结构化失败；任一目标未命中或有歧义，就在写操作开始前整体停止。它解决了过去每个 `+dm`、`+send-to-group`、`+group-members` 自己搜索并默认取首项的问题。

需要注意：群名解析已经显式证明跨页候选完整；用户解析目前依赖 `search_contact_by_key_word` 单次响应提供完整候选，后续若通讯录接口引入分页，应把相同的完整性规则扩展到用户 Resolver。

#### 5.11.3 身份与能力契约：先判断能不能做，再选择工具

`internal/shortcut/chat/identity_contract.go` 把 `user`、`bot`、`webhook` 三种身份的能力声明为矩阵，而不是散落在各分支的隐含知识：

| 身份 | 目标 | 内容 | 自然目标 | 幂等 | 批量 ledger |
|---|---|---|---|---|---|
| user | 群、单聊 user/openDingTalkId | text、markdown、image、file，audio/video 按 file 发送 | 群名、姓名 | 支持 | 否 |
| bot | 单群、多群、批量单聊 | text、markdown | 不支持 | 不支持 | 多群支持 |
| webhook | token 所属群 | text、markdown | 不支持 | 不支持 | 否 |

`internal/shortcut/chat/im_contract.go` 同时把“不支持什么”也变成可测试契约，例如 thread write、bot rich media、card action callback 和 Range 续传明确标记为不支持，并给出替代路径。这一点很重要：Agent 不应通过试错来发现能力边界。

#### 5.11.4 `+messages-send`：统一发送入口内部做严格能力路由

`internal/shortcut/chat/unified_send.go` 将 user、bot、webhook 的发送入口合并为一个语义命令，但没有把底层差异抹平：

- user 目标必须在群、群名、userId、姓名、openDingTalkId 中恰好选择一个；
- bot 必须提供 `robot-code`，且群目标与批量单聊目标必须二选一；
- webhook 的目标由 token 决定，拒绝额外群或用户目标；
- `@` 参数按身份和群聊/单聊分别校验；
- `uuid/idempotency-key` 仅允许下层真实支持的 user 路径；
- bot/webhook 在网络调用前拒绝 image/file/audio/video；
- 内容正文、`media-id` 和本地文件互斥，并自动推导 `msg-type`。

user 的文件发送被拆成上传与消息发送两个动作，上传超时为 10 分钟；dry-run 只输出两步 plan。bot 多群发送最多 100 个目标，`groups-file` 必须是工作区内不超过 1 MiB 的普通文件，解析后去重，再通过 `im.batch-write.v1` 返回逐目标成功/失败账本。

`batch_write.go` 保持输入顺序，单项失败后继续其余项，但不自动重试写操作，因为远端报错时投递状态可能未知。代价是当前批量执行为顺序调用，优先可审计性和限流安全，而不是最大吞吐量。

#### 5.11.5 Smart Shortcut 变薄：只组合公共能力

Smart 层的主要变化不是继续增加本地业务逻辑，而是把已有命令改造成公共组件的适配器：

| Shortcut/文件 | 修改后的职责 |
|---|---|
| `smart/dm.go` | 解析自然用户目标后，委托统一 user markdown 发送器 |
| `smart/send_to_group.go` | 解析群名或稳定 ID 后，委托同一发送器 |
| `smart/search_msg.go` | 在搜索前完成群和发送者消歧，再使用公共消息投影 |
| `smart/at_me.go` | 使用公共目标、分页和消息结果语义 |
| `smart/thread_replies.go` | 复用 `ProjectMessageV1`，不再维护线程专属返回结构 |
| `smart/resolve.go` | 旧局部解析辅助函数迁移到 `targetresolver` |

这使 Golden Route 保持语义清晰，同时底层身份、消息字段和失败行为只维护一份。

#### 5.11.6 消息读取：从“返回数组”升级为 `im.message-list.v1`

`internal/shortcut/chatmsg/chatmsg.go` 建立所有列表、搜索、mget、@me 和线程读取共享的投影核心 `ProjectMessageV1`。消息行统一包含可获得的：

```text
messageId / conversationId / threadId
sender / senderId / senderType
messageType / text / createTime / updateTime
reactions / quotedMessage / forwarded / resourceRefs
```

投影层做了几项关键防御：

- 同一语义字段兼容不同下层 key，但不会猜测缺失的 sender type；
- 富文本卡片只提取已识别的正文块，普通文本中的 JSON 片段保持原样；
- 无法解密的密文显示为明确标记，不把 base64 当正文；
- 引用消息做一层投影，避免递归链或循环；
- 合并转发递归使用同一投影；
- 资源从当前、引用和转发子消息中深度提取，并生成可执行下载参数；
- reaction 使用下层已内联的数据，不额外制造网络请求。

Envelope 默认保守地令 `complete=false`。只有拿到可靠分页事实时才允许证明完整；消息接口真正可继续执行的边界是末条消息的 `createTime`，因此 `nextPage` 发布 `time + direction`，而不是盲目把下层 cursor 暴露成 CLI 参数。

#### 5.11.7 有界分页：完整、截断和失败必须可区分

`smart/chat_messages.go` 将消息读取拆为目标编译、单页收集、全量收集、投影、资源下载和导出六个阶段。全量模式具备：

- `page-limit` 默认 50、硬上限 500；
- `max-results` 结果上限；
- 稳定 `messageId` 跨页去重；
- 时间边界停滞检测；
- `hasMore=true` 但空页、缺边界或缺分页元数据时停止；
- `stopReason` 区分 `source_complete`、`single_page`、`page_limit`、`result_limit`、`read_failure` 和 `pagination_error`；
- 中途失败保留已读结果，并用 `partial/failures/failedCount` 显式标记。

`chat_conversation.go` 为会话列表加入同类的 `page-all/page-limit`、稳定 ID 去重和游标停滞检测，同时修正一个关键顺序问题：即使只读取一页，也必须在退出前解析 `nextCursor`，否则会出现 `hasMore=true` 却无法继续的结果。

`smart/group_members.go` 则建立 `im.group-members.v1`：用户和机器人分别进入 bucket；用户成员自动翻页并按 `openDingtalkId` 去重，机器人按下层全量结果投影。一个 bucket 失败时，另一个成功 bucket 仍可返回，但整体必须标记 `partial` 和 failures。

#### 5.11.8 本地校验、导出和下载：错误尽量发生在网络调用前

新增的 `smart/local_chat_validation.go` 集中支持：

- `RFC3339`、`YYYY-MM-DD HH:mm:ss`、`YYYY-MM-DD` 三种时间格式；
- 显式 `limit/count/days` 必须为正数；
- 分页默认值只在值为零时回退；
- 本地验证错误携带稳定 reason 和可执行 action。

消息 JSON 导出与资源下载共享文件安全边界：

1. 只接受工作目录内相对路径；
2. 同时识别 Unix 根路径、Windows 盘符、反斜杠和 portable `..` 逃逸；
3. 解析父目录 symlink，防止最终路径逃出工作区；
4. 默认 no-clobber，覆盖必须显式声明；
5. 先写同目录临时文件，完成写入、sync、close 后再原子发布；
6. 失败时删除临时文件，不留下看似成功的半成品。

资源下载明确采用整文件重试而非 Range 续传。这个限制被写入能力契约，避免文档或 Agent 宣称不存在的断点续传能力。

#### 5.11.9 新增和强化的 Golden Route

- `+chat-create`：先解析所有自然成员和可选群主，整体预检成功后只创建一次；默认群主必须能从当前 profile 中取得并加入成员集合。
- `+chat-update`：群名或稳定 ID 统一解析，只承诺当前真实支持的改名能力。
- `+messages-reply`：可先读取原消息自动补齐引用发送者；dry-run 返回引用上下文和真实 transport plan；成功后补充消息、会话、线程、幂等和 delivery status。
- `+chat-members-list`：统一群目标，区分 users/bots，提供 bucket 完整性。
- `+chat-messages`：统一群聊/单聊自然目标、分页、消息投影、资源和导出。
- `+messages-send`：作为高级统一发送入口；`+dm` 和 `+send-to-group` 继续承担更窄、更易选路的常用入口。

这体现了 Golden Route 的正确含义：不是强迫所有意图只剩一个巨型命令，而是为每类意图保留一个推荐入口，公共执行逻辑在下层复用。

#### 5.11.10 测试重点与整体评价

18 个 Shortcut 测试文件主要固定以下不变量：

- 自然目标零命中、多命中、跨页同名和游标停滞时禁止写入；
- dry-run 中写调用为零，确认后写调用次数精确；
- canonical 参数与隐藏 alias 生成相同 MCP payload；
- 消息身份、正文、引用、转发、reaction、资源和分页投影一致；
- 页数/结果上限、中途失败和部分结果不会被标记为完整；
- Unix/Windows 路径、symlink、no-clobber 和原子发布边界；
- 所有公开 Shortcut 均进入 Schema 或有精确、受评审的 exclusion。

Shortcut 改造的核心价值可以概括为：**语义入口仍然轻量，但目标解析、能力判断、完整性证明和安全落盘已经成为 Runtime 强约束。** 当前仍需持续关注三点：`unified_send.go` 参数面较大、下层仍大量使用 `map[string]any` 做兼容投影、用户自然目标的候选完整性尚未像群目标一样显式分页证明。

## 6. 文档架构优化

本次文档改造的核心是“分层而非堆叠”：

```text
SKILL.md
  负责意图识别、入口选择和最短路径
    |
    +-- intent-guide.md
    |     负责意图到命令的系统映射
    |
    +-- contracts.md / runtime-contract.md
    |     负责稳定输入输出与错误约束
    |
    +-- product/reference docs
          负责参数细节、边界与复杂示例
```

这类结构可以降低主 Skill 的上下文消耗，同时避免同一规则复制到多处后发生漂移。

## 7. 逐文档总结（30 份）

文档变更可以先按职责理解，再查看逐文件细节：

| 文档组 | 数量 | 主要职责 | 优化方向 |
|---|---:|---|---|
| Mono | 4 | 全产品入口和产品索引 | 同步 Golden Route，避免复制领域细节 |
| Chat Skill 与总览 | 6 | IM 意图路由、总览和公共合同 | 收敛入口，建立独立 contracts |
| Chat 卡片 | 4 | 卡片模型与生命周期 | 按 create/update/callback/schema 拆分 |
| Chat 领域子文档 | 6 | bot、会话、群、消息及旧工作流 | 对齐新 Runtime，删除重复路由源 |
| Event | 6 | IM 事件监听 | 按 keys/lifecycle/operations/output 分层 |
| Shared | 4 | 跨产品运行约束 | 统一错误、profile 和 Runtime contract |

下面每份文档均按“改了什么—为什么—如何复用”三个问题进行总结。

### 7.1 Mono 文档

#### 1. `skills/mono/SKILL.md`

- 调整 IM 意图的默认路由和入口说明。
- 将运行时已具备的统一消息、会话和事件能力呈现给单体 Skill。
- 强调优先使用稳定 shortcut，底层命令作为高级或兼容入口。
- 复用价值：任何全产品聚合 Skill 都应只维护意图索引，不复制领域手册。

#### 2. `skills/mono/references/best_practices/01-messaging.md`

- 更新消息发送、回复、读取等最佳实践。
- 将自然目标解析、身份选择和分页边界纳入标准流程。
- 复用价值：最佳实践文档应描述决策原则，不只是展示命令样例。

#### 3. `skills/mono/references/products/chat.md`

- 对齐新的 chat shortcut、统一输出和能力边界。
- 收敛旧脚本路径，避免文档继续引导 Agent 选择已被 Runtime 吸收的脚本。
- 复用价值：产品 Reference 必须与 Schema 的可发现能力同步更新。

#### 4. `skills/mono/references/products/event.md`

- 引入 `event +listen-im` 及其生命周期说明。
- 明确事件监听与普通一次性命令的差异。
- 复用价值：所有长生命周期能力都应提供 start/status/stop 和恢复语义。

### 7.2 Chat Skill 与总览文档

#### 5. `skills/multi/dingtalk-chat/SKILL.md`

- 重构 chat 的核心意图路由，突出 Golden Route。
- 增加统一发送、会话创建、消息回复、列表、导出等入口。
- 将详细合同和高级示例下沉到 Reference，降低主入口复杂度。
- 当前版本为 9,928 bytes，已进入 10,000 bytes 上下文预算，但只剩 72 bytes 余量，应继续把细节下沉到 Reference。
- 复用价值：领域 Skill 应是“路由器”，不是完整 API 百科。

#### 6. `skills/multi/dingtalk-chat/references/01-messaging.md`

- 更新端到端消息操作最佳实践。
- 强调先解析目标、再做能力判断、最后执行写操作。
- 补充分页、回复、文件处理等边界。
- 复用价值：将“解析—计划—执行—回执”固化为跨产品写操作模板。

#### 7. `skills/multi/dingtalk-chat/references/chat.md`

- 大幅重构 chat 总参考，覆盖新的 shortcut 和 Runtime 契约。
- 统一参数命名、输出结构、错误处理和高级入口定位。
- 减少与子文档的重复，将细节按主题拆分。
- 复用价值：领域总览应负责导航和共享概念，具体操作落到子 Reference。

#### 8. `skills/multi/dingtalk-chat/references/intent-guide.md`

- 建立用户意图到 Golden Route 的明确映射。
- 说明目标是人、群、会话或消息时分别如何选择命令。
- 降低“多个命令看起来都能完成任务”造成的路由不确定性。
- 复用价值：通讯录和文档也应维护机器可校验的 intent-route 表。

#### 9. `skills/multi/dingtalk-chat/references/contracts.md`

- 新增集中式契约文档。
- 固定消息列表、写操作回执、批处理账本和错误 envelope 等公共结构。
- 将契约与教程分离，便于版本化和自动检查。
- 复用价值：每个领域都应有唯一的 contract source of truth。

#### 10. `skills/multi/dingtalk-chat/references/chat-emoji-list.md`

- 对齐 emoji 相关命令、目标和消息标识参数。
- 使轻量功能也遵循统一会话/消息身份语义。
- 复用价值：边缘功能不能绕过统一 Resolver 和契约。

### 7.3 Chat 卡片文档

#### 11. `skills/multi/dingtalk-chat/references/card/create.md`

- 新增卡片创建流程说明。
- 区分卡片定义、实例创建和消息投递，避免一步调用承载过多隐式行为。
- 复用价值：复杂对象创建应拆成资源定义、实例化和发布阶段。

#### 12. `skills/multi/dingtalk-chat/references/card/update.md`

- 新增卡片更新操作说明。
- 强调实例标识、可更新字段和生命周期边界。
- 复用价值：文档更新、表格更新等都应区分 create contract 与 update contract。

#### 13. `skills/multi/dingtalk-chat/references/card/callback.md`

- 新增卡片回调与交互事件说明。
- 将回调输入视为结构化事件，而不是普通消息文本。
- 复用价值：交互式文档组件、审批回调也可复用事件 envelope。

#### 14. `skills/multi/dingtalk-chat/references/card/schema.md`

- 新增卡片 Schema 说明。
- 集中描述字段结构和约束，减少 create/update 文档重复。
- 复用价值：把数据模型与操作教程分开，Schema 可单独校验和演进。

### 7.4 Chat 领域子文档

#### 15. `skills/multi/dingtalk-chat/references/chat/chat-bot.md`

- 更新机器人发送、广播及身份能力说明。
- 旧脚本能力被收敛进 Runtime，文档不再要求 Agent 拼装脚本流程。
- 注意：文档部分位置使用 `results`，而 Runtime 实际批处理字段为 `succeeded`，需统一。
- 复用价值：文档字段名必须由合同或测试生成，避免手工漂移。

#### 16. `skills/multi/dingtalk-chat/references/chat/chat-conversation.md`

- 更新会话创建、列表和目标标识说明。
- 补充游标、分页和会话类型边界。
- 复用价值：任何容器类资源都应区分 canonical ID、display name 与 page cursor。

#### 17. `skills/multi/dingtalk-chat/references/chat/chat-group.md`

- 更新群成员、群信息与群目标解析流程。
- 统一群名与群 ID 的解析/歧义处理。
- 复用价值：部门名、空间名、目录名同样需要唯一解析和候选返回。

#### 18. `skills/multi/dingtalk-chat/references/chat/chat-message.md`

- 更新消息读取、回复、导出和资源下载说明。
- 对齐 `im.message-list.v1` 投影及分页元数据。
- 复用价值：把列表 contract 与后续动作需要的 canonical ID 同时输出。

#### 19. `skills/multi/dingtalk-chat/references/chat/chat-workflows.md`（删除）

- 删除与 SKILL、intent guide 和领域子文档大量重叠的工作流集合。
- 旧文档容易形成第二套意图路由源。
- 复用价值：当工作流可由 Golden Route 与合同组合推导时，不应继续维护重复手册。

#### 20. `skills/multi/dingtalk-chat/references/lite-recipes.md`（删除）

- 删除过短且容易过时的 recipe 集合。
- 将典型路径并入主 Skill 或更权威的 Reference。
- 复用价值：减少“看似方便但缺少边界”的示例，避免模型只复制 happy path。

### 7.5 Event 文档

#### 21. `skills/multi/dingtalk-event/SKILL.md`

- 增加 IM 监听的意图入口与生命周期路由。
- 将普通事件命令和长期监听操作区分开。
- 复用价值：事件领域 Skill 应首先判断用户要“查询配置”还是“启动持续进程”。

#### 22. `skills/multi/dingtalk-event/references/event-im.md`

- 大幅更新 IM 事件总参考。
- 串联配置、启动、运行、输出、停止和排错。
- 将细节拆到四个新子文档，自己承担导航职责。
- 复用价值：长流程总文档应提供生命周期地图而非重复所有参数。

#### 23. `skills/multi/dingtalk-event/references/event-im-keys.md`

- 新增 endpoint、listener、profile 等关键标识说明。
- 明确 key 的生成、引用和隔离边界。
- 复用价值：后台任务、同步任务、订阅任务都需要稳定 task key。

#### 24. `skills/multi/dingtalk-event/references/event-im-lifecycle.md`

- 新增启动、运行、状态、停止等生命周期说明。
- 描述前后台模式及退出后的状态语义。
- 复用价值：将长任务统一建模为显式状态机。

#### 25. `skills/multi/dingtalk-event/references/event-im-operations.md`

- 新增常见操作和运维流程说明。
- 将查看状态、诊断、停止等操作从概念说明中拆出。
- 复用价值：后台服务文档应提供可操作 runbook。

#### 26. `skills/multi/dingtalk-event/references/event-im-output.md`

- 新增监听输出格式及字段解释。
- 明确事件 envelope 与 IM 消息投影之间的关系。
- 复用价值：通讯录、文档事件应共享 envelope，仅替换 payload contract。

### 7.6 Shared 文档

#### 27. `skills/multi/dws-shared/SKILL.md`

- 更新多产品共享约束和 Reference 导航。
- 将 Runtime contract 与错误码作为跨领域公共基础设施。
- 复用价值：共享 Skill 只承载真正跨域的稳定规则，领域语义仍留在各自 Skill。

#### 28. `skills/multi/dws-shared/references/error-codes.md`

- 扩展结构化错误类别、可重试性和建议动作。
- 支撑 Target Resolver、服务端失败分类和文件流程。
- 复用价值：错误码应跨产品统一命名，领域细节放入 `details`。

#### 29. `skills/multi/dws-shared/references/global-reference.md`

- 更新全局参数、profile、输出和运行约束。
- 明确公共规则的权威来源，减少 chat/event 文档重复。
- 复用价值：全局 Reference 应稳定且少变，产品差异通过链接下沉。

#### 30. `skills/multi/dws-shared/references/runtime-contract.md`

- 新增 Runtime 公共契约说明。
- 固定 envelope、错误、批处理、分页和 profile 传播等跨产品行为。
- 复用价值：通讯录、文档、云盘接入时应直接复用，而非重新设计返回结构。

## 8. 被删除脚本的意义

PR 删除 mono 与 multi chat 下的以下脚本：

- `bot_broadcast.py`
- `chat_export_messages.py`
- `chat_history_with_user.py`

删除并不意味着能力减少，而是能力已进入可测试、可发现、可治理的 Runtime。脚本适合验证原型，但长期作为 Agent 主路径会带来参数契约不可发现、错误格式不一致、身份上下文丢失和重复分页逻辑等问题。

迁移准则：当一个脚本成为稳定业务能力后，应将核心逻辑提升到 Runtime，生成 Schema，补充契约测试，然后从主 Skill 中撤下脚本入口。

## 9. Policy 与测试体系

### 9.1 Multi IM Skill Chain Policy

新增 `scripts/policy/multi-im-skill-chain`，通过意图测试数据检查关键路径是否在 Skill、Reference、Schema 与 Runtime 之间闭环。其价值是防止以下回归：

- 文档推荐了不存在的命令；
- Runtime 已新增命令但 Schema 未暴露；
- 兼容别名重新成为首选；
- 同一意图再次出现多个 Golden Route；
- 关键参数或输出字段只在某一层存在。

当前 intent manifest 包含 9 个关键意图路由；PR 描述记录 `make policy` 通过。本文对代码和配置进行了静态核对，没有把该记录扩展表述为 GitHub CI 全绿。

### 9.2 Context Budget Policy

Skill 上下文预算检查用于控制入口文档大小。渐进披露只有在主 Skill 足够短时才有效。当前 chat Skill 为 9,928 bytes，刚好回到 10,000 bytes 预算内；架构方向正确，但余量很小，内容仍需继续去重或下沉。

### 9.3 测试分层

本次变更配套测试覆盖：

- Resolver 的唯一命中、多命中、未命中与输入类型；
- unified send 的目标和身份选择；
- 消息投影字段；
- 分页边界与兼容行为；
- 结构化错误和服务端失败分类；
- event 命令注册与生命周期；
- Schema/shortcut contract；
- mock MCP 端到端冒烟路径。

PR 描述记录 `internal/app`、`internal/shortcut/chat` 和 `internal/shortcut/smart` 等重点测试通过。本文分析的是远端 head 的 Git 对象，没有在当前 `main` 工作区切换分支重新运行完整测试；当前 GitHub CI 结论仍应以第 21 章列出的 workflow 状态为准。

## 10. 面向通讯录场景的复用蓝图

通讯录复用的重点不是照搬 IM 命令，而是把“找群/找人后执行动作”的模式转换为“找用户/找部门后执行动作”。核心差异在于通讯录写操作通常权限更高、影响范围更大，因此 Resolver 的唯一性和执行前计划需要更严格。

### 10.1 建议 Golden Routes

```text
contact +resolve
contact +user-get
contact +user-list
contact +department-list
contact +member-list
contact +invite
contact +update
contact +export
event +listen-contact
```

### 10.2 Directory Resolver

集中接受姓名、手机号、邮箱、userId、unionId、部门名称和 deptId。输出唯一 canonical target 或候选列表，不允许高风险写操作在歧义状态下继续。

### 10.3 类型化契约

建议定义：

- `directory.user.v1`
- `directory.user-list.v1`
- `directory.department.v1`
- `directory.member-list.v1`
- `directory.batch-write-result.v1`
- `directory.change-event.v1`

### 10.4 风险分级

- 只读查询：可在唯一命中后自动执行。
- 邀请/修改：必须显示 preflight plan。
- 删除/批量调岗：要求显式确认、严格唯一目标和可审计账本。

### 10.5 批处理账本

每条记录包含 input、resolved target、action、status、error、retryable 和 profile。这样部分失败可安全重试，不需要重跑已成功项目。

## 11. 面向文档、云盘和知识库的复用蓝图

文档类场景与 IM 的共同点是目标输入多样、读取结果需要稳定投影、长流程需要完整性保证；不同点是文档还涉及版本冲突、层级容器、权限继承和内容覆盖。因此在复用 Resolver 与 Runtime contract 的同时，应额外引入 revision/etag 和并发控制。

### 11.1 建议 Golden Routes

```text
doc +resolve
doc +create
doc +read
doc +update
doc +export
doc +share
drive +download
drive +upload
wiki +resolve-node
event +listen-document
```

### 11.2 Document Target Resolver

统一解析 URL、docId、fileId、spaceId、workspaceId、知识库节点 ID、标题和路径。解析输出必须包含 product/resource type，避免将同形 ID 传给错误产品接口。

### 11.3 Plan/Execute 分离

写文档、覆盖文件、调整权限、移动知识库节点前先生成计划：

```json
{
  "target": {"type": "document", "id": "..."},
  "action": "replace-content",
  "identity": "user",
  "expectedRevision": "...",
  "sideEffects": ["overwrite content"],
  "recoverability": "revision-history"
}
```

计划可以独立验证权限、版本冲突和副作用；确认后才执行。

### 11.4 列表与搜索契约

搜索结果必须区分 exact、fuzzy 和 candidate，返回 canonical URL/ID、容器、资源类型、权限摘要以及分页状态。不要只返回标题字符串。

### 11.5 原子输出与并发控制

- 导出使用临时文件和原子替换。
- 更新使用 revision/etag 防止覆盖他人修改。
- 上传返回摘要、大小和最终资源 ID。
- 长迁移返回 checkpoint 与逐项账本。

## 12. 可复用的落地模板

### 12.1 新领域接入步骤

1. 列出用户真实意图，而非先列 API。
2. 为每个意图指定唯一 Golden Route。
3. 设计统一 Target Resolver 和 canonical target。
4. 建立身份/能力矩阵。
5. 定义版本化输入、列表、写回执和错误契约。
6. 为长流程增加 maxItems、maxPages、timeout、checkpoint。
7. 将写操作拆为 plan 和 execute。
8. 生成或同步 Schema metadata/hints/registry。
9. 编写短 Skill、意图指南、合同和分主题 Reference。
10. 增加意图链路、上下文预算、Schema 和契约测试。

### 12.2 推荐目录结构

```text
skills/multi/dingtalk-<domain>/
  SKILL.md
  references/
    intent-guide.md
    contracts.md
    <domain>.md
    <topic-a>.md
    <topic-b>.md

internal/shortcut/
  <domain>/
  <domain>resolver/

scripts/policy/
  <domain>-skill-chain/
```

### 12.3 Golden Route 评审问题

- 一个自然语言意图是否只有一个默认入口？
- shortcut 是否隐藏了供应商 API 的非必要复杂度？
- 兼容入口是否仍可运行但不会被 Agent 优先发现？
- Resolver 是否统一处理 ID、名称、URL 和多候选？
- 写操作是否在执行前完成身份和能力校验？
- 输出是否有稳定版本？
- 列表是否明确完整、截断与下一页状态？
- 错误是否能指导 Agent 下一步动作？
- Skill、Reference、Schema 与 Runtime 是否有自动一致性检查？

## 13. PR #860 当前风险收口状态

| 评审问题 | 当前整体实现 | 判断 |
|---|---|---|
| `+conversation-list` 单页模式丢失 `nextCursor` | 将 cursor 解析移动到单页退出判断之前，并补充失败边界测试 | **已修复** |
| `hasMore=true` 但 cursor 缺失、无效或不前进 | 增加 missing/invalid/stalled cursor 测试 | **已强化** |
| Chat Shortcut 未全部进入 Schema | 从 50/97 提升到 97/97，删除 47 项临时 exclusion | **已解决** |
| 高风险示例直接展示 `--yes` | 文档和命令示例改为先运行或 `--dry-run`，由 Runtime 要求确认后再追加 `--yes` | **已改善** |
| 跨平台输出路径判断和覆盖替换不完整 | 统一处理 Unix/Windows 路径语义，并以 build tags 分离 `os.Rename` 与 `windows.Rename` | **已加固** |
| chat bot 文档 `results` 与 Runtime `succeeded` 可能不一致 | 当前 diff 中未见由单一 typed source 自动生成这两个名称 | **仍需确认** |
| Chat Skill 上下文预算偏紧 | 已通过拆分 Reference 和删除重复文档缩短主 Skill，仍应持续由 budget policy 约束 | **持续治理** |
| 兼容别名缺少退出观测 | 已增强 alias/canonical payload 等价测试，但未建立使用量观测或移除阈值 | **长期项** |
| 真实线上语义 | 单元测试和 mock MCP 能证明本地契约，不能证明真实权限、MCP 响应和服务端可用性 | **需要受控 E2E** |

## 14. PR #860 整体架构结论

PR #860 的核心贡献，是把 IM 能力从“散落的命令和教程”升级为“可解析、可计划、可执行、可验证的意图系统”。其最重要的架构资产包括：

- Resolver：统一对象身份；
- Golden Route：统一意图入口；
- Capability Matrix：统一身份与能力判断；
- Versioned Contract：统一机器输出；
- Bounded Execution：统一分页和长流程边界；
- Typed Error Ledger：统一失败处理与恢复；
- Progressive Documentation：统一知识组织；
- Policy Gates：统一防漂移机制。

这些模式与 IM 本身无强绑定。通讯录的“找人/找部门”、文档的“找资源/改内容”、云盘的“找文件/迁移”、知识库的“找节点/调整结构”都面临同一类问题。复用时应迁移完整链路，而不是只复制某个 shortcut：

```text
意图设计
  -> Resolver
  -> Golden Route
  -> Plan/Capability Check
  -> Bounded Runtime
  -> Versioned Result/Error
  -> Documentation + Schema + Policy
```

只有整条链路同时落地，才能真正降低 Agent 的选择成本、减少误操作，并让新领域能力持续演进而不重新陷入命令、文档和实现相互漂移的问题。

---

## 15. 产品化交付闭环

### 15.1 为什么 Runtime 完成不等于 PR 完成

PR #860 同时处理两类问题：一类是命令行为本身，另一类是 Agent 能否发现、正确选择并安全执行这些行为。只有 Runtime、Registry、Schema、selection hints、风险 metadata、Skill、Policy 和测试同时闭环，才能把“隐藏在代码里的能力”升级为“可交付产品能力”。

大行数主要来自重新生成并提交的 Schema catalog 和 Agent metadata，不能等同于 6 万余行手写业务逻辑。评审时应将 authored source、Runtime、generated artifacts、文档和测试分开看。

### 15.2 五个交付收口方向

1. **能力发布完整化**：剩余 47 个公开 Chat Shortcut 从临时 exclusion 转为正式 Registry 和 Schema 能力，使 Chat 达到 97/97。
2. **安全元数据类型化**：为 47 个命令逐一声明 effect、risk、confirmation、idempotency、interface mode 和 runtime gate。
3. **参数前置校验**：时间、页大小、回溯窗口等明显错误在本地拒绝，不再发起无效远端调用。
4. **运行时边界收口**：修复单页 cursor、补 `-o`、增强跨平台输出路径并扩大目标消歧测试。
5. **交付证据闭环**：重新生成 Schema/Agent metadata，强化生成漂移、契约完整性和 changed-code coverage 门禁。

## 16. Schema 发布与能力治理

### 16.1 从 50/97 到 97/97

PR #860 在 `internal/cli/schema_command_registry/products/chat.json` 中补齐 47 个 canonical shortcut，并从 `schema_command_exclusions.json` 删除整个 `chat-shortcuts-pending-schema-curation` 临时排除组。

这一步的意义不是单纯“数字补齐”，而是完成以下状态转换：

```text
公开可执行、但 Schema 暂未评审
  -> authored Registry 有正式记录
  -> selection hints 能指导 Agent 何时用/何时不用
  -> metadata 声明风险和确认方式
  -> generated catalog/Agent metadata 可发现
  -> completeness test 保证不再退回临时排除
```

关键指标变化：

| 指标 | 改造前 | PR #860 当前 head |
|---|---:|---:|
| Chat Schema Shortcut | 50 | 97 |
| 全部公开 Shortcut Schema 覆盖 | 218 / 265 | 265 / 265 |
| Schema catalog 工具数 | 850 | 898 |
| Agent metadata 覆盖工具数 | 850 | 898 |
| hint tools | 1,852 | 1,948 |
| reviewed Agent examples | — | 1,108（1,084 contract + 24 dry-run） |

### 16.2 新发布的 47 个 Shortcut

为便于理解，可按业务域归为五组：

#### 会话分类（3）

- `+category-add-conversation`
- `+category-list-conversations`
- `+category-remove-conversation`

#### 群管理（13）

- `+chat-add-bot`
- `+chat-audit-join`
- `+chat-get-by-id`
- `+chat-members-get`
- `+chat-mute-member`
- `+chat-quit`
- `+chat-remove-bot`
- `+chat-role-remove`
- `+chat-role-remove-user`
- `+chat-transfer-owner`
- `+chat-update`
- `+chat-update-icon`
- `+chat-update-settings`

#### 会话状态（7）

- `+conversation-clear-messages`
- `+conversation-clear-red-point`
- `+conversation-hide`
- `+conversation-mark-read`
- `+conversation-mark-unread`
- `+conversation-mute`
- `+conversation-set-top`

#### Feed 与收藏（4）

- `+feed-group-query-item`
- `+flag-cancel`
- `+flag-create`
- `+flag-list`

#### 消息与资源（20）

- `+messages-add-emoji`
- `+messages-add-text-emotion`
- `+messages-batch-recall-by-bot`
- `+messages-batch-send-by-bot`
- `+messages-combine-forward`
- `+messages-create-text-emotion`
- `+messages-forward`
- `+messages-forward-topic`
- `+messages-list`
- `+messages-recall`
- `+messages-recall-by-bot`
- `+messages-remove-emoji`
- `+messages-remove-text-emotion`
- `+messages-resource-download`
- `+messages-resource-url`
- `+messages-send-by-bot`
- `+messages-set-pin`
- `+messages-set-top`
- `+messages-unset-pin`
- `+messages-unset-top`

### 16.3 风险与确认元数据

47 个新增 metadata 的分布非常清晰：

| 类型 | 数量 | confirmation | runtime gate |
|---|---:|---|---|
| 只读 / low | 8 | `not_required` | `none` |
| 写操作 / medium | 36 | `user_required` | `typed_yes` |
| 破坏性 / high | 3 | `user_required` | `typed_yes` |

三个 high-risk 命令是：

- `chat.shortcut_chat_remove_bot`
- `chat.shortcut_chat_role_remove`
- `chat.shortcut_conversation_clear_messages`

八个只读命令包括分类列表、按 ID 查群、成员获取、Feed 查询、收藏列表、消息列表、资源 URL 和资源下载。其余 39 个命令全部要求用户确认并由 Runtime 的 `typed_yes` 门禁执行。

这种分类实现了三个层面的一致性：

1. Agent 在选择工具时能看到风险；
2. 示例不会把确认当成默认参数；
3. Runtime 即使被直接调用，也会阻止未经确认的写操作。

### 16.4 Authored Source 与生成物分离

PR #860 的权威编辑源主要是：

- `schema_command_registry/products/chat.json`：命令身份和 canonical path；
- `schema_hints/selection/chat.json`：`use_when`、`avoid_when` 和 source refs；
- `schema_hints/metadata/chat.json`：effect、risk、confirmation 等运行语义。

随后由生成流程更新：

- `schema_catalog/catalog.json`
- `schema_catalog/tools/chat.json`
- `schema_agent_metadata/chat.json`
- `schema_agent_metadata/index.json`
- `schema_agent_metadata_audit.json`
- `runtime-surface-completeness.json`

复用到其他产品时，应优先评审 authored source 的准确性，再验证生成物无漂移。直接手改大型 catalog 会削弱可追溯性。

### 16.5 Selection Hints 的价值

每个新 Shortcut 都有独立的 `use_when`、`avoid_when` 和 `source_refs`。三者分别解决：

- **什么时候用**：把用户意图映射到该工具；
- **什么时候不用**：避免和相近工具误路由；
- **依据是什么**：从生成结果追溯到 Registry 和文档锚点。

这比只提供命令描述更适合 Agent，因为工具选择的难点通常不是“工具做什么”，而是“在几个相似工具中为什么选这个”。

## 17. Runtime 安全与正确性加固

### 17.1 本地参数校验

新增 `internal/shortcut/smart/local_chat_validation.go`，集中实现：

- 默认分页上限归一化；
- RFC3339、`YYYY-MM-DD HH:mm:ss`、`YYYY-MM-DD` 三类时间格式检查；
- 带稳定 reason 和 suggested action 的本地 validation error。

落地命令及边界：

| 命令 | 新增校验 |
|---|---|
| `+chat-messages` | `--time` 格式；`--limit/--size/--page-size > 0` |
| `+thread-replies` | 时间格式；显式 `--limit > 0` |
| `+at-me` | `--days` 为 1～3650；`--limit > 0` |
| `+unread-chats` | 显式 `--count > 0` |

核心原则是“能够在本地确定错误，就不要把它推给远端”。这样能减少无效请求，并让 Schema 约束、CLI 错误和测试保持一致。

### 17.2 破坏性确认门禁

`internal/helpers/chat.go` 为以下底层命令补上真实 Runtime 检查：

- `chat category delete`
- `chat clear-messages`

缺少 `--yes` 时，命令返回 `confirmation_required`，列出确认目标和再次执行方式，并保证没有任何下游工具调用。补充测试同时验证：确认前零写入、确认后仅发生一次预期调用。

此外，群解散和升级外部群等示例不再直接展示 `--yes`；升级示例改用 `--dry-run`。这是一个重要的提示工程原则：

> 文档示例应该展示安全的第一步，而不是展示绕过交互确认的最终一步。

### 17.3 单页会话 cursor 修复

`+conversation-list` 的旧实现曾在非 `--page-all` 模式下先退出循环，再解析 `nextCursor`，可能返回 `hasMore=true` 但游标为零。PR #860 将 cursor 解析提前到退出判断之前。

修复后的行为：

- 单页结果保留可执行的下一页游标；
- `hasMore=true` 但游标缺失、无效或停滞时记录 pagination failure；
- `--page-all` 才继续拉下一页；
- `--page-limit` 保持有界。

这说明分页契约的完整性不仅适用于“拉完全部”，单页模式同样必须提供可靠 continuation。

### 17.4 通用 shorthand 支持

`shortcut.Flag` 新增 `Shorthand` 字段，统一注册逻辑从 Cobra 的 `Bool/Int/StringSlice/String` 切换到对应的 `*P` 版本。`+chat-messages --output` 因而获得 `-o`。

该改动是框架级能力，不只服务于当前命令。后续复用时需增加同一命令内 shorthand 唯一性检查，避免不同 flag 争用同一个短参数。

### 17.5 跨平台资源路径加固

原实现主要依赖当前操作系统的 `filepath.IsAbs/Clean`。在 Unix 上，Windows 盘符路径或反斜杠逃逸未必会按预期识别。PR #860 增加 portable 归一化：

- 将反斜杠转换为 `/` 后再检查；
- 识别 Unix 绝对路径；
- 识别 Windows `C:` 等盘符；
- 统一拒绝 `..` 和 `../` 逃逸；
- 在父目录、相对路径和最终解析阶段复用同一判断。
- 用 `file_replace.go` 与 `file_replace_windows.go` 的 build tags 分离平台实现：非 Windows 使用 `os.Rename`，Windows 使用 `windows.Rename`，保证覆盖写仍是原子替换。

该模式可直接用于文档导出、云盘下载、通讯录 CSV 导出和任何工作区内文件写入能力。

### 17.6 目标消歧与 fail-closed 测试

Resolver 实现与测试共同固定以下关键语义：

- 稳定 ID 永远不按名称搜索；
- 自然目标走 Resolver；
- 同名精确匹配仍保持歧义，不能偷偷选一个；
- 必须翻完候选页后才判断是否唯一；
- cursor 无法推进时 fail closed；
- 结构化错误保留候选对象；
- 外部联系人只要有可用 openDingTalkId 也可参与解析。

最重要的改进是“跨页后再唯一判定”。如果只看第一页就认为唯一，在第二页存在同名对象时会产生高风险误操作。

### 17.7 删除过时 hint 子命令

删除 `dws chat send` 和 `dws chat history` 两个仅用于提示的伪子命令。Golden Route 已通过 Shortcut 和 Schema 正式发布后，继续保留 hint 命令会污染命令树并形成第二套发现路径。

兼容不等于永远保留所有提示层。若入口已经由结构化 discovery 接管，应删除无法执行真实业务、只输出迁移提示的旧节点。

## 18. PR #860 文件分类总结

### 18.1 129 个文件按职责归类

| 文件组 | 数量 | 主要内容 |
|---|---:|---|
| 生产 Go | 38 | App、错误、Resolver、Chat/Smart Runtime、契约、Policy 与生成别名 |
| Go 测试 | 30 | Resolver、分页、确认、文件安全、事件、契约及 coverage |
| Skill Markdown | 30 | Mono、Chat、Event、Shared 四层知识链路 |
| Schema / metadata / JSON | 17 | Registry、selection、risk metadata、catalog、Agent metadata 与 intent manifest |
| Skill 脚本 | 6 | 删除已被 Runtime 替代的重复 Python 路径 |
| 生成/审计脚本 | 6 | Skill 生成、Policy 检查、live audit 及其测试 |
| CI / Makefile | 2 | 接入 Policy、兼容性、覆盖率及平台 race 验证 |

### 18.2 Schema 文件逐项总结

| 文件 | 总结 |
|---|---|
| `schema_command_registry/products/chat.json` | 补齐 canonical Shortcut，成为 Chat 发布权威源 |
| `schema_command_exclusions.json` | 删除 47 项“待 Schema 评审”临时豁免，反向完整性转为硬约束 |
| `schema_hints/selection/chat.json` | 为 Chat 工具补齐 use/avoid/source refs，强化 Agent 路由 |
| `schema_hints/metadata/chat.json` | 为 Chat 工具声明 effect、risk、confirmation、idempotency 和 runtime gate |
| `schema_hints/runtime-surface-completeness.json` | 更新 Runtime surface 覆盖指纹与统计 |
| `schema_catalog/catalog.json` | 重新生成全局 catalog 索引 |
| `schema_catalog/tools/chat.json` | 重新生成 Chat 的完整工具契约和示例 |
| `schema_agent_metadata/chat.json` | 生成 Chat Agent 决策元数据 |
| `schema_agent_metadata/index.json` | 更新全产品 metadata 索引和 898 工具统计 |
| `schema_agent_metadata_audit.json` | 更新 coverage、source/surface hash 和审计结果 |

### 18.3 Runtime 文件逐项总结

| 文件 | 总结 |
|---|---|
| `internal/helpers/chat.go` | 为删除分类、清空消息补确认门禁；安全化危险示例；删除旧 hint 子命令 |
| `shortcut/chat/chat_conversation.go` | 修复单页模式 nextCursor 丢失 |
| `shortcut/chat/resource_download.go` | 统一 Unix/Windows 绝对路径和 `..` 逃逸判断 |
| `shortcut/chat/file_replace.go` | 非 Windows 构建使用 `os.Rename` 作为原子替换边界 |
| `shortcut/chat/file_replace_windows.go` | Windows 构建使用 `windows.Rename` 支持覆盖已有目标 |
| `shortcut/runner.go` | 所有 flag 类型支持 shorthand 注册 |
| `shortcut/types.go` | `Flag` 契约增加 `Shorthand` |
| `smart/local_chat_validation.go` | 新增共享时间、页大小和结构化本地错误辅助函数 |
| `smart/chat_messages.go` | 新增时间/页大小校验及 `-o`，复用默认 page limit |
| `smart/at_me.go` | 限定 days 和 limit，组合资源下载校验 |
| `smart/thread_replies.go` | 限定时间格式和 limit |
| `smart/unread_chats.go` | 限定 count 为正数 |
| `smart/group_members.go` | 复用统一默认 page limit 逻辑 |

### 18.4 Policy 文件总结

`scripts/policy/multi-im-skill-chain/main.go` 将工作目录、进程退出、文件打开和 Registry build/bind 抽成可注入函数。行为本身没有改变，但各种 setup failure、I/O failure 和 binding failure 现在可在单元测试中稳定覆盖。

这是一种值得复用的“测试接缝”设计：对进程边界做最小注入，不为测试重写业务逻辑。

### 18.5 安全示例收口示例

#### 1. `skills/mono/references/best_practices/01-messaging.md`

- 将发送、机器人批量发送和消息转发示例中的默认 `--yes` 去掉。
- 明确 Runtime 会先返回确认要求；用户确认后才以相同参数追加 `--yes`。
- 保留唯一目标解析、同 profile 稳定 ID 和未知投递状态停止重发等规则。
- 复用价值：所有写操作最佳实践都应展示“安全第一步 + 确认后续步骤”，不能把确认令牌写进首个示例。

#### 2. `skills/mono/references/products/chat.md`

- 将“普通群升级为外部群”的正式执行示例改为 `--dry-run`。
- 删除直接带 `--yes` 的示例，保留 extension JSON 的预演方式。
- 复用价值：不可逆转换、权限变更和内容覆盖应优先展示 dry-run/preflight。

### 18.6 测试体系总结

PR #860 的 30 个 Go 测试文件主要覆盖：

- 265/265 public Shortcut Schema 覆盖和 Chat 97/97；
- metadata 风险与 Runtime confirmation truth；
- 时间、页大小、回溯窗口等本地无网络校验；
- natural target、稳定 ID、同名候选和跨页消歧；
- 单页/全量分页、cursor 停滞、边界失败和部分结果保留；
- 资源下载的绝对路径、反斜杠、symlink、原子 no-clobber；
- 确认前零写调用、确认后精确一次调用；
- alias 到 canonical payload 的等价性；
- Multi IM Policy 的配置、I/O 和失败矩阵；
- 跨平台 changed-code coverage。

测试名大量增加 `CrossPlatformCoverage` 前缀，是为了让平台覆盖门禁识别相关测试。其积极意义是证据可追踪；长期应避免让命名约定取代真正的覆盖映射机制。

## 19. Go 文件优化专项总结

### 19.1 总体规模与演进主线

PR #860 的 Go 变更应按“生产实现”和“验证实现”分开理解：

| 类型 | 文件数 | 新增 / 删除 | 主要任务 |
|---|---:|---:|---|
| 生产 Go | 38 | +4,028 / -737 | 建立 Resolver、IM Runtime 契约、事件监听、错误/Profile、平台文件替换和 Policy 基座 |
| Go 测试 | 30 | +4,343 / -88 | 固定正常路径、失败边界、安全不变量、Schema/Runtime 一致性与跨平台覆盖 |

整体演进不是把更多逻辑堆进单个命令，而是逐步形成以下 Go 代码分层：

```text
Cobra / App 层
  负责命令注册、参数装配、profile 与进程生命周期
        |
        v
Shortcut / Smart 层
  负责 Golden Route、业务校验、计划和结果组织
        |
        v
Target Resolver / Contract 层
  负责规范目标、身份能力、版本化结果和错误语义
        |
        v
Helper / Transport 调用层
  负责下游工具调用，不重复业务决策
        |
        v
Policy + Tests
  验证 Runtime、Schema、文档和安全元数据保持一致
```

### 19.2 App、错误与上下文文件

| Go 文件 | 主要优化 | 设计价值 |
|---|---|---|
| `internal/app/event_command.go` | 注册新的 `event +listen-im` 入口 | 将 IM 事件监听纳入正式内建命令树 |
| `internal/app/event_listen_im.go` | 新增监听计划编译、自然目标解析、事件类型映射及 consume 生命周期委托 | 把长期事件订阅从参数拼装提升为可验证的计划执行 |
| `internal/app/runner.go` | 增强多 profile 执行和错误结果组织 | 批量或多身份运行时保留逐 profile 的失败语义 |
| `internal/app/server_failure_classifier.go` | 新增服务端业务失败分类器 | 将后端文本/元数据转换为稳定 reason、retryable 和行动建议 |
| `internal/auth/profiles.go` | 收敛 profile 选择和传播方式 | 减少解析与执行使用不同租户身份的风险 |
| `internal/profilectx/profile.go` | 新增轻量 profile context | 让 Resolver、Shortcut 和调用层共享明确身份上下文 |
| `internal/errors/errors.go` | 扩展 typed error、server diagnostics 和结构化输出字段 | 使错误可被 Agent 解析并驱动后续动作 |
| `internal/cli/param_aliases_generated.go` | 发布新增 IM 参数别名 | 兼容旧参数拼写，同时维持 canonical payload |
| `internal/helpers/aisearch.go` | 补齐搜索目标兼容与参数映射 | 为自然人名解析提供更稳定的下层搜索入口 |
| `internal/helpers/chat.go` | 对齐 Chat 底层命令边界、参数与确认相关行为 | 使 shortcut 与底层 helper 的真实能力一致 |

这一组文件的核心优化是：把 profile、错误和生命周期从“隐式全局状态”变为显式上下文与类型化结果。通讯录、文档等领域若只复用 Resolver 而不复用这些基础设施，仍可能出现跨租户误操作或不可恢复错误。

### 19.3 Chat Runtime 与契约文件

| Go 文件 | 主要优化 | 设计价值 |
|---|---|---|
| `internal/shortcut/chat/identity_contract.go` | 新增用户、机器人、webhook 等身份契约 | 把“谁在执行”从命令分支提升为显式类型 |
| `internal/shortcut/chat/im_contract.go` | 新增 IM 列表、写结果与公共字段契约 | 为消息、会话和批处理提供稳定域模型 |
| `internal/shortcut/chat/unified_send.go` | 重构统一发送的目标解析、身份路由、批量发送和能力拒绝 | 用户表达发送意图，Runtime 决定合法执行器 |
| `internal/shortcut/chat/batch_write.go` | 统一批量写去重、成功/失败账本和未知状态 | 支持部分失败审计与安全重试 |
| `internal/shortcut/chat/chat_conversation.go` | 增强会话列表分页、投影和完整性字段 | 单页与全量模式都返回可执行 continuation |
| `internal/shortcut/chat/chat_group.go` | 对齐群目标、成员和群操作的输入输出 | 群名与稳定 ID 共用同一解析语义 |
| `internal/shortcut/chat/chat_message.go` | 统一消息读取、回复及消息字段投影 | 后续回复、转发、下载使用 canonical message identity |
| `internal/shortcut/chat/message_export.go` | 新增消息 ledger 的临时写入、校验和原子发布 | 失败时不留下伪成功的半成品文件 |
| `internal/shortcut/chat/resource_download.go` | 对齐资源下载与安全输出流程 | 下载与导出共享工作区相对路径和原子提交原则 |
| `internal/shortcut/chat/file_replace.go` | 非 Windows 平台使用 `os.Rename` 提供覆盖替换 | 保持平台默认的原子替换语义 |
| `internal/shortcut/chat/file_replace_windows.go` | Windows 平台使用 `windows.Rename` | 修复 `os.Rename` 无法覆盖既有目标的差异 |
| `internal/shortcut/chat/lark_alignment.go` | 补齐创建群、回复等对齐型 Shortcut | 将常用意图封装成稳定 Golden Route |
| `internal/shortcut/chatmsg/chatmsg.go` | 大幅增强 `im.message-list.v1` 投影、消息身份、引用、资源、reaction 和分页 | 隔离下游消息响应差异，为 Agent 提供稳定机器结构 |

这组文件形成了一条清晰的数据流：

```text
Target + Identity
  -> Unified Route
  -> 下层调用
  -> Message/Conversation Projection
  -> Typed Ledger
  -> Atomic Export / Resource Reference
```

其中 `identity_contract.go`、`im_contract.go` 和 `chatmsg.go` 是最值得跨领域复用的部分：前两者展示如何建立类型化边界，后者展示如何把供应商响应投影成稳定领域模型。

### 19.4 Smart Shortcut 与 Resolver 文件

| Go 文件 | 主要优化 | 设计价值 |
|---|---|---|
| `internal/shortcut/targetresolver/resolver.go` | 新增统一用户/群目标解析、候选去重、精确匹配、跨页搜索和结构化歧义错误 | 消除各命令独立猜测 ID/名称的根因 |
| `internal/shortcut/smart/resolve.go` | 将旧的局部解析逻辑收敛到共享 Resolver | 降低重复实现和规则漂移 |
| `internal/shortcut/smart/chat_messages.go` | 重构自然群/用户目标、typed 分页、去重、结果上限与导出 | 将消息历史变为有界、可证明完整的读取流程 |
| `internal/shortcut/smart/group_members.go` | 统一群解析、用户/机器人分桶、用户分页和稳定 ID 去重 | 对复杂成员列表公开完整性和失败信息 |
| `internal/shortcut/smart/at_me.go` | 对齐群目标解析、消息投影与资源引用 | 特殊消息查询仍复用公共契约 |
| `internal/shortcut/smart/search_msg.go` | 统一群与发送者自然目标解析 | 多条件搜索在远端调用前完成消歧 |
| `internal/shortcut/smart/dm.go` | 将单聊发送收敛到统一身份和目标规则 | 避免单聊维护另一套发送实现 |
| `internal/shortcut/smart/send_to_group.go` | 将群发送收敛到 unified send | 群名、群 ID 和身份选择走同一路由 |
| `internal/shortcut/smart/thread_replies.go` | 对齐线程回复的消息投影和资源字段 | 线程场景不再产生独立返回结构 |
| `internal/shortcut/runner.go` | 增强 shortcut flag、约束、隐藏兼容参数和确认处理 | Runtime metadata 能真实映射到 Cobra 行为 |
| `internal/shortcut/types.go` | 扩展 Shortcut/Flag 类型信息 | 为 Schema 生成和 Runtime 装配提供统一源数据 |

这一层最重要的代码优化是“业务适配器变薄”。Smart 命令不再负责全部搜索、能力选择、分页和投影，而是组合共享 Resolver、contract 和执行器。这种组合方式比复制命令模板更适合通讯录和文档扩展。

### 19.5 Policy Go 文件

#### `scripts/policy/multi-im-skill-chain/main.go`

该文件新增了一套可执行的一致性检查器，读取意图 manifest、有效 CommandRegistry、selection hints 和 Markdown marker，验证：

- 每个关键意图是否有唯一 preferred tool；
- fallback 是否有明确 reason code；
- forbidden default 是否不会重新成为首选；
- source refs 和文档 marker 是否存在；
- typed contract 与 event handoff 参考是否漂移；
- 已退休脚本是否被重新发布；
- confirmation metadata 是否被降级。

它把“评审规则”实现成 Go 程序，能够直接加载项目真实 Registry，而不是靠文本搜索猜测命令是否存在。这种 Policy-as-Code 模式可抽象成通讯录、文档等产品共用的领域 manifest 校验器。

### 19.6 边界收口类生产 Go 文件逐项总结

除新增核心抽象外，PR #860 还对既有 Runtime 的关键边界做了系统收口：

| Go 文件 | 主要优化 | 整体价值 |
|---|---|---|
| `internal/helpers/chat.go` | 为删除会话分类、清空消息增加真实 `--yes` 门禁；安全化示例；删除旧 hint 命令 | 补齐 Runtime confirmation truth 和 Golden Route 收口 |
| `internal/shortcut/chat/chat_conversation.go` | 在单页退出前解析 `nextCursor` | 修复 #860 的 continuation 缺陷 |
| `internal/shortcut/chat/resource_download.go` | 兼容 Windows 盘符、反斜杠和 portable `..` 逃逸检查 | 将 #860 文件安全扩展到跨平台 |
| `internal/shortcut/runner.go` | 所有 flag 类型改用 Cobra `*P` 注册 | 提供通用 shorthand 基础能力 |
| `internal/shortcut/types.go` | `Flag` 新增 `Shorthand` | 让 Runtime、Schema 和 CLI 共享短参数定义 |
| `internal/shortcut/smart/local_chat_validation.go` | 新增时间格式、默认 page limit 和结构化本地错误辅助函数 | 将散落校验收敛为公共层 |
| `internal/shortcut/smart/chat_messages.go` | 增加时间/页大小校验和 `-o`；复用默认分页函数 | 阻止无效远端调用，改善常用导出体验 |
| `internal/shortcut/smart/at_me.go` | 校验 1～3650 天及正数 limit | 为特殊读取命令建立明确资源边界 |
| `internal/shortcut/smart/thread_replies.go` | 校验时间格式和正数 limit | 与消息列表使用相同本地校验语义 |
| `internal/shortcut/smart/unread_chats.go` | 校验显式 count 大于 0 | 避免负数/零值被静默当作默认值 |
| `internal/shortcut/smart/group_members.go` | 复用统一默认 page limit | 去除重复默认值分支 |
| `scripts/policy/multi-im-skill-chain/main.go` | 对 getwd、exit、open、Registry build/bind 增加可注入接缝 | 不改变生产行为的前提下完整覆盖失败分支 |

这组代码体现了三种“小而关键”的优化：

1. **修顺序**：cursor 必须先提取，再决定是否继续翻页。
2. **提公共函数**：时间和分页校验不在多个 Smart 命令中复制。
3. **建测试接缝**：只对进程/I/O 边界注入，不侵入业务逻辑。

### 19.7 Go 测试文件优化总结

#### 核心架构行为测试

首先建立新能力的行为基线：

- `targetresolver/resolver_test.go`：唯一命中、多命中、稳定 ID 和结构化候选；
- `unified_send_resolution_test.go`：身份、自然群目标、批量发送和写入前失败；
- `chatmsg_test.go`：消息身份、正文、引用、reaction、资源和分页投影；
- conversation/message/group 相关测试：typed list、分页和兼容字段；
- event listen 测试：计划编译、事件映射、生命周期和清理回滚；
- failure classifier/errors 测试：JSON、人类输出和服务端诊断；
- Multi IM Policy 测试：意图路由、Reference、retired script 和 contract marker。

#### 边界与交付证据测试

其次将测试从“新增功能验证”扩展到“边界与交付证据”：

- 将 public Shortcut Schema 覆盖固定为 265/265、Chat 固定为 97/97；
- 验证 47 个新增 Schema 项的风险分类与 Runtime gate；
- 验证确认前零调用、确认后精确一次调用；
- 覆盖 cursor 缺失、无效、停滞、page limit 和后续页失败；
- 覆盖跨页同名目标仍保持歧义；
- 覆盖 Unix/Windows 路径、symlink、原子写和 no-clobber；
- 覆盖 shorthand、alias payload 等价和本地参数失败；
- 通过 `CrossPlatformCoverage` 测试集合证明 changed-code coverage。

测试策略可概括为：

```text
行为测试：证明新架构“能正确工作”
交付测试：证明所有公开入口“都被发布、都受约束、所有失败边界都不会越权执行”
```

### 19.8 Go 层面的核心可复用模式

| 模式 | Go 实现方式 | 适用场景 |
|---|---|---|
| Typed Resolver | 独立 package + canonical result + structured candidates | 通讯录用户/部门、文档/空间、云盘文件 |
| Thin Adapter | Smart Shortcut 组合 Resolver、contract 和 executor | 所有自然语言 Golden Route |
| Contract Projection | 独立 projector 将 `map[string]any` 转为稳定 v1 envelope | 列表、搜索、事件、批处理结果 |
| Fail Closed | cursor/目标/身份不完整时停止，不猜测默认值 | 写操作和需要完整集合的读取 |
| Bounded Collector | page limit、max results、seen cursor、stable dedupe | 成员、文档块、文件、审批记录 |
| Atomic Publisher | temp file、close/check、rename、no-clobber | 导出、下载、备份和迁移 |
| Typed Confirmation | metadata 声明 + Runtime 强制 gate + 零调用测试 | 删除、覆盖、转移、权限修改 |
| Injectable Boundary | 将 getwd/open/exit/build 等边界封装为函数变量 | CLI、Policy 和后台任务的失败分支测试 |
| Policy as Code | Go 加载真实 Registry/manifest 并做语义校验 | 跨产品 Schema、Skill 和 Runtime 一致性 |

Go 文件层面的核心结论是：PR #860 同时完成“抽象公共机制”“收紧运行边界”和“补齐验证证据”。复用到其他领域时，应优先复制 package 边界、类型契约和测试不变量，而不是复制具体命令函数。

## 20. 当前 #860 的整体价值与跨场景复用

### 20.1 从“运行时能力”到“可交付产品能力”

PR #860 定义了一个更完整的 Done：

| 完成层级 | 判断标准 | 对应 PR |
|---|---|---|
| 能执行 | Runtime 有真实实现 | #860 |
| 行为稳定 | Resolver、分页、typed result/error 有契约 | #860 |
| 可发现 | Registry 和 Schema 正式发布 | #860 |
| 会选择 | selection hints 有 use/avoid 语义 | #860 |
| 执行安全 | local validation、capability 和 typed confirmation 生效 | #860 |
| 可维护 | 文档分层、生成漂移、Policy 和覆盖门禁闭环 | #860 |

因此，新领域不能把“代码合入”当成交付完成。一个通讯录或文档 Shortcut 只有同时满足上表六层，才真正适合 Agent 使用。

### 20.2 通讯录复用重点

在 `DirectoryResolver` 蓝图之上，应补充 PR #860 展示的发布治理：

- 每个 user/department Shortcut 都进入 authored Registry；
- selection hints 区分“查人”“查关注的人”“查某人最近消息”等易混意图；
- 删除、调岗、批量邀请标记为 write/high 并使用 typed confirmation；
- 手机号、邮箱格式和分页大小在本地校验；
- 联系人导出路径使用 portable workspace-relative 规则；
- completeness test 禁止公开 Shortcut 依赖长期 exclusion。

### 20.3 文档与云盘复用重点

在原有 `DocumentTargetResolver`、revision/etag 和原子导出蓝图之上，应补充：

- read、write、destructive 风险 metadata；
- 覆盖、删除、权限回收必须 typed confirmation；
- 示例默认使用 dry-run 或 plan，不直接带确认参数；
- 文档 URL、路径和目标 revision 在本地尽可能校验；
- 导出/下载同时识别 Unix 与 Windows 路径语义；
- 生成 catalog 必须能追溯到 authored Registry 和 contract Reference。

### 20.4 推荐的跨产品发布清单

1. Runtime 已实现且有单元测试。
2. 目标解析支持稳定 ID 和自然目标，并对歧义 fail closed。
3. 列表返回完整性与 continuation。
4. 写操作声明 effect/risk/idempotency。
5. Runtime gate 与 metadata 的 confirmation 完全一致。
6. Registry 有 canonical path。
7. Selection hints 有 use/avoid/source refs。
8. Schema 和 Agent metadata 已生成且 drift clean。
9. 文档第一示例不绕过确认。
10. public shortcut completeness、policy 和 changed-code coverage 通过。

## 21. 综合验证、剩余风险与最终结论

### 21.1 验证事实需要分层理解

PR #860 当前描述中记录的验证包括：

- `DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/app -count=1`
- `DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/shortcut/chat ./internal/shortcut/smart -count=1`
- `make policy`
- generated drift clean
- Schema compatibility 与 authoritative interface integrity 通过
- 26 products / 898 tools
- 176 个 Runtime confirmation gate
- 1,108 个 reviewed Agent examples

上述是 PR 作者提供的本地验证记录。本报告通过 GitHub 状态另行确认：head `33ceab60...` 的 `AI Behavior` 为 success，但 PR-triggered `CI` workflow run #1802 已完成且结论为 failure。因此不能把 PR 描述中的本地验证等同于“GitHub CI 全绿”；失败原因需结合 Actions job/log 单独排查。

### 21.2 仍需关注的风险

1. **CI 未全绿**：当前 PR 仍为 Open，CI run #1802 显示 failure，应追踪失败 job，确认是否为基础设施、覆盖门禁或真实回归。
2. **大型生成物审查成本**：5 万余行增量主要是生成物，应持续保证 authored source、hash 和 drift check 可重建。
3. **Skill 上下文预算**：当前 Chat Skill 为 9,928 bytes，已回到 10,000 bytes 预算内，但余量仅 72 bytes，应继续把细节下沉到 Reference。
4. **字段文档漂移**：`results` / `succeeded` 等批处理字段仍需合同化校验。
5. **Shorthand 冲突**：框架支持 shorthand 后，应增加每个命令内唯一性和保留字符检查。
6. **确认不是幂等**：39 个写命令的 idempotency 仍为 `unknown`；确认只能降低误触发，不能解决重试重复写入。
7. **命名式覆盖标记**：大量测试依赖 `CrossPlatformCoverage` 前缀，长期可考虑用测试清单或覆盖映射替代命名耦合。

### 21.3 最终结论

PR #860 将“可靠执行 IM 意图”和“完整、安全、可发现地交付能力”合并为同一套工程系统。其核心方法是：

```text
Intent First
  -> Golden Route
  -> Typed Target Resolver
  -> Local Validation + Preflight Capability Check
  -> Typed Confirmation
  -> Bounded Runtime
  -> Versioned Result / Error Ledger
  -> Authored Registry + Selection/Metadata
  -> Generated Schema / Agent Metadata
  -> Policy + Drift + Coverage Gates
```

这条链路可以直接作为通讯录、文档、云盘、知识库等新领域的标准交付模板。最关键的认知是：**Runtime 正确只是起点；只有发现、选择、安全、契约和门禁同时闭环，能力才真正完成产品化。**
