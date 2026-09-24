# DWS A2UI Agent 卡片搭建与投递技术方案

> 状态：Implemented for local authoring and guarded delivery; external release and real-client acceptance remain gated
>
> 日期：2026-09-15
>
> 修订：对齐 card-docs@b9305f4 命令域设计，并同步当前分支的实现、Bugbot 修复与分层验收结果。
>
> 范围：在 `dingtalk-workspace-cli` 中建立面向 Agent 的 A2UI 卡片“发现、规划、组合、校验、预览、发送、更新”闭环，并把现有 A2UI 发送能力迁移到 `dws card ...`。

> 2026-09-24 设计修订：内置 `notification`、`information`、`task`、`schedule` Recipe 的默认头部已改为无底色标题行，根布局为 `Column`，标题与正文共用平台边距。下文涉及这些 Recipe 使用通栏 `CardHeader` 的记录属于当时的实现与验收历史，不再代表当前默认设计。`CardHeader` 仍是协议支持的组件；明确要求通栏主题头部时须分别验收 Mac、移动端与 Web。

## 1. 结论与核心决策

DWS 不应只把一组原始 A2UI JSON 透传给发送接口，也不应在 Prompt 中一次性塞入完整 Catalog。推荐建设四层能力：

1. **协议包**回答“什么字段合法、什么能力真实开放”；
2. **语义组件、Block 与 Recipe**回答“面对当前内容，应该选择什么结构”；
3. **组合编译器、设计规则与预览**回答“怎样形成层次清楚、视觉一致的完整 Surface”；
4. **投递与实例状态**回答“发给谁、是否发送成功、如何安全更新同一张卡”。

Agent 仍然负责理解回复内容、判断信息优先级和选择表达方式；DWS 用确定性代码补齐 ID、默认 Catalog、数据绑定、降级文案、交互约束和投递参数，并在发送前拒绝协议或交互不可靠的产物。

生成策略采用三级模型：

```text
用户内容与意图
  → Recipe（高频完整卡片）
  → Semantic Block（高频信息区块）
  → 受约束的原始 A2UI 组件树（长尾能力）
  → 协议校验 + 组合规则 + 预览
  → send / update
```

这不是再造一套与 A2UI 平行的完整 DSL。`CompositionSpec` 只表达信息角色、Recipe/Block 选择和必要业务数据；Agent 也可以直接提交原始 A2UI 消息。所有路径最终都编译为同一份标准 A2UI 消息序列，并通过同一校验器。

## 2. 已核对事实与当前缺口

### 2.1 协议事实

本方案基于以下输入：

- 附件 `a2ui-catalog.json`：`protocolVersion=1.0`，公开 Catalog ID 为 `https://dingtalk.com/card/a2ui/catalogs/public/catalog.json`，包含 47 个公开组件和 44 个函数；SHA-256 为 `daf4b7e7585c28bae2c95ac477a835a6d68b719b1278c1cf408845d9f9cefb6c`。
- 附件 `a2ui-common-types.json`：提供 `ComponentCommon`、`Action`、`DataBinding`、动态值、宿主动作等公共类型；SHA-256 为 `2a80d4d8968be55d95c23d48b5cee98bba441181144f46c04a1dd27dbb6f548f`。
- `dingtalk-card/card-docs@master` 的 `protocol/a2ui/dingtalk-a2ui-open-protocol.md`：当前声明为手工维护的协议权威源；机器 Catalog 由该文档生成。
- `protocol/a2ui/schema/README.md`：明确 Catalog 根不是完整 A2UI 消息的校验入口，组件入口为 `#/$defs/anyComponent`，并且必须同时解析 common types。
- `protocol/a2ui/a2ui-dingtalk-interactive-event-mapping.md`：明确 A2UI `action.event` 与钉钉 `onTap + sendCardRequest` 的映射边界。

2026-09-15 补充依据：已将本地 `card-docs/master` 从 `c27d558` fast-forward 到 `b9305f442dddaf36ccf45fbada077ce438a203d6`，读取用户指定的 `site/architecture/dws-card-command-design.html`，并对照同名权威 Markdown `architecture/dws-card-command-design.md`。重点采纳其 §3 的生成上下文、§5.8 的受管进程、§6 的台账与预览、§7.16 的协议包和 §8 的业务能力复用设计。原先两份参考文件按用户要求移出依赖与待确认清单。

该文档虽标注 `authoritative`，其中独立进程、Host Broker、完整消息校验、图片回传和 Gateway 仍是目标设计。§9.1 记录的是隔离 IPC 实验，不是原生 DWS 集成验收；§7.16/§9.1 还记录了协议生成漂移。本方案不将这些记录升级为当前实现通过。本文的 DWS 源码基线为 `8cacb01951d2567b2c0466d14cb6c5c9d0267a68`；当前源码未检出参考文档所述连接器四态与台账实现，复用前须补齐对应仓库/版本证据。

附件的 47 组件与摘要是原附件快照，不能代表拉取后协议库的可发布状态。实施时必须冻结经同步检查通过的 Bundle，重新核对本文组件语义表。

### 2.2 现有 DWS 能力

当前分支已经提供 `dws card` 根命令和完整本地作者工具链。旧 A2UI 原子能力仍以兼容命令保留，但已标记 deprecated：

```text
dws chat message send-a2ui-card
dws chat message update-a2ui-card
```

旧命令只完成：

- 校验 `--content` 是非空 JSON 字符串数组；
- 生成 `requestId` / `bizCardId`；
- 调用 `im.create_and_send_a2ui_card` / `im.update_a2ui_card`；
- 透传 `a2uiMessages`、`a2uiAnnotations`、目标与 `flowStatus`。

上述缺口已由 `dws card protocol/catalog/block/recipe/guide/compose/build/lint/preview/send/update/finish/verify/list/show/doctor` 分层补齐；旧命令不再作为 Agent 默认选择。仍未完成且不得冒充通过的是：真实客户端多端视觉与 Action 回流、服务端 readback/远端 CAS、协议资产公开来源与许可证、以及卡片平台十套模板的公共可复现来源合同。

原始兼容命令自身仍没有：

- 完整消息 Schema 校验；
- Catalog 与 common types 的引用解析；
- 组件树、数据绑定、消息顺序、Action 的语义校验；
- Agent 可渐进发现的组件语义、Block、Recipe 和设计规则；
- 本地预览及视觉质量证据；
- send/update 的同实例状态文件与未知投递结果治理。

### 2.3 一期组件名称校正

一期使用协议中的精确组件名，大小写必须一致：

```text
Button, File, Markdown, TextField, Link, Card, Row, Column, Tag,
Divider, CollapsiblePanel, Image, Text, ChoicePicker
```

- 用户输入中的 `textfiled` 统一校正为 `TextField`。
- 当前公开 Catalog **没有 `RadioButton`**。单选能力使用：

```json
{
  "component": "ChoicePicker",
  "variant": "mutuallyExclusive",
  "displayStyle": "checkbox"
}
```

CLI 的搜索与推荐可以接受 `radio` / `radiobutton` 作为概念别名，但编译后的协议中不得出现不存在的 `RadioButton`。

## 3. 目标与非目标

### 3.1 一期目标

1. Agent 能先获得一个小而清晰的能力索引，再按需查询单组件、Block 或 Recipe 的完整合同。
2. Agent 能把自己的回复内容拆成标题、摘要、状态、正文、列表、文件、补充详情、输入项和操作区。
3. 高频场景优先使用经过验证的 Recipe；长尾场景使用 Block 和公开组件自由组合。
4. 发送前同时通过协议、组件树、数据绑定、交互、可访问性和基础视觉规则校验。
5. 支持本地预览，并清楚区分“本地参考效果”“真实发送成功”“客户端视觉/交互验收”。
6. A2UI 发送与更新的主路径迁移到 `dws card send|update`，旧命令保持兼容。
7. 输出可审计回执，确保更新的是同一 `bizId`、同一 `surfaceId` 和期望的内容修订。

### 3.2 非目标

- 不在 DWS 内运行一个新的大模型；Agent 由宿主提供。
- 不允许自定义 HTML、JavaScript、未登记组件、未登记函数或任意执行代码。
- 不把本地 Schema/Preview 通过表述为 PC、iOS、Android 真机验收通过。
- 不在一期建设组织级三方组件市场或公开 Block 发布平台。
- 不把卡片平台 Builder 的模板创建、保存、预发布、发布生命周期搬进 DWS。
- 不承诺当前尚未完成的 `RendererToAgent.action` 事件桥已经端到端可靠。
- 不把 P2 主题设计误报为协议已有的任意主题能力。

## 4. 总体架构

```text
card-docs 协议权威源
  └─ 发布 A2UI Protocol Bundle（Catalog + Common Types + Message Schema + Profile）
       └─ DWS 固定版本与摘要校验
            ├─ Protocol Registry        精确字段、类型、枚举、函数、能力状态
            ├─ Semantic Registry        用途、反例、信息角色、兼容概念词
            ├─ Block/Recipe Registry    可复用结构、Slot、默认层级、Golden
            ├─ Composition Compiler     生成 ID、绑定、消息序列、fallback
            ├─ Validation/Lint Core     协议、引用、交互、视觉、可访问性
            ├─ Preview Adapter          确定性参考渲染与截图
            └─ Delivery Service         目标解析、send/update、回执与状态

Agent（通过 DWS 自带契约发现，卡片专用 Skill 非前置条件）
  ├─ guide recommend
  ├─ recipe/block show、catalog get
  ├─ compose / lint / build / preview
  └─ send / update
```

### 4.1 权威边界

| 层 | 唯一负责 | 不负责 |
|---|---|---|
| Protocol Bundle | 字段、类型、required、枚举、函数、Catalog ID、协议版本 | 设计偏好和业务场景 |
| Semantic Registry | `use_when`、`avoid_when`、信息角色、概念别名、组合建议 | 覆盖或修改协议字段 |
| Block | 一项可复用的信息结构和交互合同 | 完整业务卡片流程 |
| Recipe | 一类高频完整卡片的信息架构和默认组合 | 成为新的运行时协议 |
| Design Rules | 层级、密度、分组、操作优先级、可访问性 | 发明协议能力 |
| Delivery Service | 目标、身份、投递、实例修订和结果 | 判断卡片是否美观 |

语义 Registry 只能引用协议 Registry 中存在的组件、字段、函数和枚举。CI 必须拒绝：未知组件、未知属性、把可选项改成必选项、枚举漂移、把 `declared_only` 能力标成 `verified`。

## 5. Agent 如何获得 A2UI 知识

### 5.1 渐进披露，而不是一次塞满 Catalog

Agent 的默认上下文只包含：

- 三级生成路线；
- 一期常用信息角色；
- Recipe/Block 的简短索引；
- 必须执行 `compose → lint → preview → send` 的规则；
- 精确查询命令。

需要细节时按层查询：

```bash
dws card guide recommend --intent approval --format json
dws card recipe show approval --compact --format json
dws card block show action-row --compact --format json
dws card catalog get ChoicePicker --compact --format json
dws card catalog get ChoicePicker --full --format json
```

`--compact` 返回选择所需的摘要：用途、避免场景、required、关键枚举、父子关系、绑定和交互注意事项、一个合法示例。默认视图返回所选组件完整 Schema 与引用闭包，供首次创作；`--full` 增加全部 provenance 和协议说明。compact 不宣称是可独立验证的完整 Schema；三种视图的同名事实必须一致。

### 5.2 Agent 工作流

```text
1. 提取内容事实，不先选组件
2. 建立 InformationPlan：意图、受众、主结论、区块、操作、状态、密度
3. guide recommend 返回候选 Recipe/Block 和理由
4. Agent 选择一个方案；必要时查询精确组件合同
5. 生成轻量 CompositionSpec 或原始 A2UI 消息
6. CLI 编译并做确定性校验
7. Agent 查看预览并完成结构化视觉复核
8. 明确目标后发送；保存回执
9. 用同一 handle 更新，回执作为导出证据；终态使用 FINISH/ERROR/ABORTED/TIMEOUT
```

`guide recommend` 是确定性路由器，不调用模型。输入是结构化意图，例如：

```json
{
  "intent": "approval",
  "contentRoles": ["title", "status", "summary", "details", "form", "actions"],
  "interaction": "submit",
  "density": "comfortable",
  "hasFiles": true,
  "hasImages": false
}
```

输出应包含最多三个候选、适用原因、缺失前提和需要查询的合同，不直接替 Agent 作最终业务判断。

### 5.3 让协议真正进入 Agent 上下文

采纳参考文档 §3.3：随包安装后必须能通过命令返回协议内容。compact 语义摘要不能替代完整字段合同；首次生成一个组件时查询带引用闭包的默认视图，已有同版本上下文才用 compact 避免重复读取。

| 返回字段 | 合同 |
|---|---|
| `bundleVersion / manifestHash / catalogId / schemaDialect` | 精确锁定协议包；Catalog ID 不充当版本 |
| `componentSchema` | 原组件 Schema 投影，不手写第二套 required/enum |
| `definitions / resources` | 资源 URI + JSON Pointer 组织依赖，保留相对引用基址；visited 去重、防循环；禁止任意网络 ref |
| `authoring.messageSkeleton / rules` | 同版本完整消息骨架、初始化和引用规则；纳入 manifest 摘要 |
| `semantics / examples` | 当前组件用途、反例、推荐 Block 与已校验样例 |
| `complete / continuation` | 超限显式分页；后续页固定版本和摘要，不静默截断公共类型 |

草案使用同名 `.lock.json` 保存 bundleVersion、manifestHash、编译器及 Recipe/Block 版本；锁文件属于工具链，不写入 A2UI 消息。显式版本与锁冲突报错；缺少锁定包报错；更新默认包不重解释已有草案。

Host 静态路径提供 help、Schema、catalog、Recipe/Block 和 guide 资源，无需登录或启动 Runtime。一期验收必须在无 card-docs 源码、无卡片专用 Skill 的标准安装环境，完成“获取合同 → 创作 → 校验 → 看图 → 修订”。可选 Skill 只作为同源说明投影。

## 6. CLI 命令设计

### 6.1 命令树（目标设计；实施状态见 §26）

```text
dws card
├─ catalog list|search|get     # Host 静态查询，含组件 Schema 与公共类型
├─ protocol info|verify        # 本地协议包元数据和完整性检查
├─ block list|show
├─ recipe list|show
├─ guide recommend|rules
├─ compose                     # Recipe/Block → 标准 A2UI
├─ lint                        # 完整协议与设计规则校验
├─ build                       # 归一、校验、确定性 wire 编码
├─ preview                     # 校验、真实渲染、图片回传
├─ send / update / finish
├─ verify / list / show        # 投递回读与台账
├─ watch / doctor              # 交互事件与环境诊断
├─ runtime status|stop|restart  # DWS 受管进程
└─ template / capability / instance  # P2 资产能力；本期不注册空命令
```

`card` 是卡片业务域，命令直接按动作组织。A2UI 是当前实现协议，内部模块可继续叫 `a2ui`，公开路径不再嵌入协议名。未来接入其他引擎时再评审选择参数，不提前暴露无实现的 `--engine`。

一期命令按 §18 分批交付。产品文案可以称 Recipe 为“A2UI 内置模板”，`template` 保留给 P2 有草案、版本与发布生命周期的资产；Recipe 与 Template 不共用身份。

### 6.2 输入合同

`compose` 支持两类输入：

```bash
# Recipe 路径
dws card compose \
  --recipe approval \
  --data ./approval.json \
  --output ./messages.json

# Block/组件自由组合路径
dws card compose \
  --spec ./card.composition.json \
  --output ./messages.json
```

`messages.json` 使用自然的 JSON 对象数组，不要求调用者手工把每条消息再次字符串化。只有 Delivery Adapter 在调用 `im.*` 接口前把每个对象序列化为 `[]string`。

`lint` 显式区分场景：

```bash
dws card lint --file ./messages.json --mode create
dws card lint --file ./delta.json --mode update --handle <handle>
```

`send` / `update`：

```bash
dws card send \
  --profile <profile> \
  --conversation-id <openConversationId> \
  --file ./messages.json \
  --summary "审批结果" \
  --idempotency-key <operationId> \
  --state-out ./receipt.json \
  --yes

dws card update <handle> \
  --profile <profile> \
  --file ./delta.json \
  --flow-status FINISH \
  --yes
```

发送目标复用 DWS 当前 profile 与 chat 的唯一解析链：

- `--conversation-id <openConversationId>`：稳定群 ID，沿用当前原子命令参数；
- `--chat-query <群名>`：只在唯一命中时继续；
- `--open-dingtalk-id <id>`：稳定单聊目标；
- `--user-query <姓名>`：只在唯一命中时继续。

四类目标互斥；query 参数复用现有 resolver，不新造 ID 转换。新写入命令要求显式 profile，解析后固定主体、组织、环境、渠道和目标，并在 Host Broker 再次核对。`update <handle>` 从台账读取目标，禁止改投。`--env pre|prod` 是待适配的产品参数，不直接照搬参考文档默认值；实现前对齐当前 profile 配置与渠道解析，不按 corpId 猜测未知渠道。

### 6.3 输入兼容

新命令主参数为 `--file <path|->`，支持对象数组和 JSONL；旧 wire 字符串数组仅作为显式识别的导入格式，归一后进入相同校验。保留 JSONL 行号到消息 JSON Pointer 的映射。大协议不经 argv，输入文件在一次任务内读取为稳定快照。为了兼容现有命令：

- 新 `send|update` 不增加 `--content` 字面量；旧路径保留已有输入合同，兼容层把它转换为内部消息对象；
- 旧 `chat message send-a2ui-card|update-a2ui-card` 保留至少两个稳定小版本；
- 旧路径和新路径调用同一个 Delivery Service，不复制实现；
- Help 标明 deprecated 与替代路径；机器提示使用框架支持的元数据位置，不能擅自在统一 envelope 增加字段；
- `chat +messages-send --a2ui-messages` 在迁移期保留，Schema 的 `avoid_when` 指向 `card send`。

### 6.4 Schema 身份

推荐新身份：

```text
card.send      → dws card send
card.update    → dws card update
card.compose   → dws card compose
card.lint      → dws card lint
card.preview   → dws card preview
```

现有 `chat.create_and_send_a2ui_card` / `chat.update_a2ui_card` 先作为兼容身份保留。不能把同一个 Cobra leaf 同时声明成两个 canonical identity；实现上使用共享 Core + 两组薄命令声明。新命令是 Agent 推荐入口，旧命令是 deprecated 原子兼容入口。

`send` 的最终 Safety 应为 `effect=write, risk=medium, confirmation=user_required, idempotency=unknown`；支持 `--dry-run`，真实发送使用运行时确认门禁。`update` 同样是外部可见写入，默认执行确认门禁；若未来凭实例所有权和单次会话策略降低门禁，必须单独评审，不能从 send 自动推断。

## 7. CompositionSpec：轻量意图，不是第二套 A2UI

建议协议：`dingtalk-a2ui-composition/1.0`。

```json
{
  "version": "dingtalk-a2ui-composition/1.0",
  "surfaceId": "approval-20260915",
  "intent": "approval",
  "density": "comfortable",
  "items": [
    {
      "kind": "block",
      "block": "header",
      "data": {"title": "上线审批", "status": "待处理", "statusTheme": "orange"}
    },
    {
      "kind": "block",
      "block": "markdown-section",
      "data": {"content": "## 变更摘要\n\n本次包含三项修复。"}
    },
    {
      "kind": "block",
      "block": "approval-form",
      "data": {
        "comment": "",
        "decision": "approve",
        "options": [
          {"label": "通过", "value": "approve"},
          {"label": "拒绝", "value": "reject"}
        ]
      }
    }
  ]
}
```

约束：

- `kind=recipe` 只用于完整 Recipe；`kind=block` 用结构化 Slot；`kind=component` 用公开 Catalog 的组件合同。
- 编译器生成稳定、可复现的组件 ID；同样输入必须生成相同消息摘要。
- 原始组件字段保持 A2UI 名称，不再设计一套 1:1 的字段别名。
- ID、默认 Catalog、fallback、数据路径命名空间和必要 Action 包装由编译器补齐。
- Agent 可通过 `escapeHatch.rawMessages` 选择原始 A2UI，但必须标注原因并经过完整校验；不能跳过规则。

输入分支互斥：CLI 的 recipe+data、spec、原始消息三条路径不能混用；spec 中 recipe 必须是唯一完整根，block/component 可混合但不能再带 rawMessages。每个 Block 实例具有稳定业务 key，组件 ID 从该 key 和局部角色生成，不能只由数组下标生成；新增或排序一个区块不得重编号其他区块的输入控件。

Recipe 表中的 summary/section/details/footer 是信息角色，不是新 Block 标识；依次映射到 markdown-section、section-title+markdown-section、expandable-details、footer-note。编译器只能查 §9 的实际 Block 注册项。approval-form 包含提交按钮时，外层 action-row 只承载不同的次操作，不再生成第二个提交按钮。

## 8. 一期 Recipe

参考卡片平台 CLI 的内置模板理念，不做逐字段机械转换。只迁移能由一期公开 A2UI 组件表达、且有稳定高频信息结构的 Recipe：

| Recipe | 适用场景 | 默认区块 | 核心组件 | 交互 |
|---|---|---|---|---|
| `notification` | 简短通知、结果告知 | header、summary、action-row | Text、Tag、Markdown、Button、Link | 可选主操作 |
| `task-reminder` | 任务、截止时间、负责人、状态 | header、key-value-list、details、action-row | Text、Tag、Row、Column、CollapsiblePanel、Button | 查看/确认 |
| `project-status` | 项目进度和风险摘要 | header、status-banner、markdown-section、details | Text、Tag、Card、Markdown、Divider、CollapsiblePanel | 查看详情 |
| `approval` | 审批摘要、意见、选择、提交 | header、markdown-section、approval-form、action-row | TextField、ChoicePicker、Button、Text | 提交表单 |
| `rich-media` | 图文内容、报告、资讯 | header、image-text-item、markdown-section、action-row | Image、Text、Markdown、Link | 查看详情 |
| `file-result` | Agent 生成文件或交付物 | header、file-item、summary、details | File、Text、Markdown、CollapsiblePanel | 打开/下载 |
| `information` | 结构化说明、结论、依据 | header、summary、section、details、footer | Text、Markdown、Divider、CollapsiblePanel | 可无交互 |

每个 Recipe 必须包含：

- `use_when` / `avoid_when`；
- 必填和可选 Slot Schema；
- 默认信息顺序；
- 空值处理与最大推荐密度；
- 一期支持组件集合；
- Action 能力等级；
- 至少三组内容压力样本：短文本、长文本、中文/英文混排；
- 一个 Golden A2UI 消息序列和参考截图。

不在一期直接迁移需要非优先组件才能成立的模板。未来新增 Recipe 必须证明它封装了稳定高频结构或独特交互合同，不能只因为文案不同就增加模板。

### 8.1 卡片平台 CLI 内置模板的 A2UI 转换要求

内置 Recipe 优先参考卡片平台 CLI 的实际内置模板，复用其信息架构、区块组合与视觉设计，并转换成完整 A2UI 消息协议。不是只借用模板名称或文案，也不直接向 DWS 投递平台模板 DSL、Durbo 数据或 HTML。

实施时先核对卡片平台 CLI 当前模板目录、源码和预览，建立来源清单：源仓库/commit、模板标识、输入 Schema、布局、交互合同和参考截图。本方案目前未逐一核验实际模板，因此下表是转换策略，不代表已完成模板资产转换。

| 转换对象 | A2UI 落点 | 验收要求 |
|---|---|---|
| 标题、摘要、状态 | header Block，展开为 Text/Tag/Column/Row | 结论优先，信息层级与参考模板意图一致 |
| 内容分区、事实列表 | Column/Row、Markdown、按需 Card/Divider | 不遗漏事实；长值不破坏阅读顺序 |
| 图文列表项 | image-text-item，展开为 Image/Text/Link 与布局组件 | 无图、长标题、窄宽度各有明确布局 |
| 附件与交付物 | File 及说明区 | 资源字段符合 Catalog，真实打开行为单独验证 |
| 补充详情 | CollapsiblePanel | 核心结论和关键错误不被折叠 |
| 输入、单选、多选 | TextField/ChoicePicker 与 DataModel 绑定 | 类型、初值、写回路径和提交 context 完整 |
| 操作与反馈 | Button + Text child、合法 Action、反馈区 | 不机械复制平台事件名；按当前 A2UI/宿主支持重新映射 |
| 样式与状态 | 公开组件字段与一期默认视觉规范 | 不注入 CSS 或私有属性；不支持的效果记录降级差异 |

每个转换资产必须交付：来源记录、Recipe Slot Schema、Block 组合、数据绑定/事件映射、完整 create 消息正例、同实例 update 消息正例、参考图与实际 A2UI 渲染图、差异说明、压力与交互测试。初始消息按锁定协议生成 Surface/组件/初始数据序列；更新复用同一 Surface 与稳定组件 ID，不能重新建卡冒充更新。

转换以信息与交互语义等价为准；像素差异单独评审。找不到合法对应组件或可靠动作时，明确列出不支持项，采用经审核的简化布局或停用该 Recipe，不虚构字段。源模板可被参考不等于可直接公开复制，发行资产沿用 §16.2 的来源与许可要求。

首批优先完成通知、结构化结果、交互表单三类样板，再覆盖 §8 的七套 Recipe。每个样板先查找可复用的真实源模板；没有匹配项时记录为新设计。七套 Recipe 仍是一期开闭环的完整范围，三类样板只决定实现顺序。

## 9. 一期 Semantic Block

| Block | 信息作用 | 默认展开 | 组件展开 |
|---|---|---:|---|
| `header` | 标题、状态、短说明 | 是 | Column + Row + Text + Tag |
| `status-banner` | 结果、风险或阶段状态 | 是 | Card + Row + Tag + Text |
| `section-title` | 建立正文分区 | 是 | Text |
| `key-value-row` | 展示一个标签和值 | 是 | Row + Text + Text |
| `key-value-list` | 展示 2–6 个结构化事实 | 是 | Column + key-value-row |
| `image-text-item` | 图片、标题、摘要、链接 | 是 | Row + Image + Column + Text + Link |
| `file-item` | 文件名、类型、大小、说明 | 是 | File |
| `markdown-section` | 标题、列表、代码、引用 | 是 | Markdown |
| `expandable-details` | 次要详情、过程或依据 | 否 | CollapsiblePanel + Markdown/Text |
| `text-input-field` | 单行/多行文本输入 | 是 | TextField |
| `single-choice-field` | 单项选择 | 是 | ChoicePicker(mutuallyExclusive) |
| `multi-choice-field` | 多项选择 | 是 | ChoicePicker(multipleSelection) |
| `action-row` | 一个主操作和最多两个次操作 | 是 | Row + Button/Text + Link |
| `footer-note` | 低优先级说明或时效 | 是 | Divider + Text(caption) |
| `approval-form` | 意见、决定与提交合同 | 是 | Column + TextField + ChoicePicker + Button |

Block 不是简单把几个组件粘在一起。每个 Block 必须声明：

- 业务 Slot；
- 信息角色和阅读顺序；
- 编译后的组件树；
- 动态绑定路径；
- Action 名、context 和 checks；
- 空值、超长、无图片/无文件的退化策略；
- 支持的客户端能力等级；
- 可访问性标签；
- Golden 和交互用例。

## 10. 一期组件语义表

| 组件 | 推荐使用 | 避免使用 | 关键合同 |
|---|---|---|---|
| `Text` | 标题、标签、短值、说明 | 长篇富文本和复杂列表 | 仅 `caption/body`；标题层级由 bold、size token 和位置共同表达 |
| `Markdown` | 段落、列表、代码、引用 | 短标签、按钮文案 | `content` 必填；流式内容只能先绑定再 append |
| `Tag` | 状态、风险、类别 | 当作按钮或长文本容器 | `theme=black/gray/red/orange/green/blue`，`variant=filled/hollow` |
| `Image` | 内容图片、封面、缩略图 | 无意义装饰或不可访问 URL | `url` 必填；明确 fit、variant、暗色资源和无障碍说明 |
| `File` | Agent 产物和附件 | 用 Markdown 链接伪装文件 | `fileName/url` 必填；可配 mime、size、description、preview |
| `Link` | 低权重跳转或次操作 | 代替唯一主操作按钮 | `text` 必填；跳转优先 `openUrl` 本地函数 |
| `Button` | 主操作、提交、确认 | 一个区块内堆多个 primary | 必须有 child 和 action；有文案时 child 使用 Text；checks 挂提交按钮 |
| `TextField` | 审批意见、备注、短/长文本输入 | 期望它自身提交事件 | 没有有效组件 Action；值本地写回，统一由 Button context 提交 |
| `ChoicePicker` | 单选、多选、下拉 | 发明 RadioButton | 单选用 `mutuallyExclusive`；平铺 checkbox/chips 不自动上报，提交按钮读取 value |
| `Card` | 强分组和背景容器 | 每个小段都套 Card | 单子节点；只在边界能提升扫描时使用 |
| `Row` | 同一语义行、标签值、并排操作 | 放长正文或不可压缩多列 | 子项从左到右；小屏风险时改 Column |
| `Column` | 主体纵向信息流 | 无层级地堆所有内容 | 默认根容器；通过 gap 和区块结构建立节奏 |
| `Divider` | 两个语义区之间需要明确边界 | Card 背景已形成边界时重复分隔 | 水平/垂直；装饰不能多于信息价值 |
| `CollapsiblePanel` | 次要详情、过程、依据 | 隐藏核心结论或唯一主操作 | 默认收起；必须有清晰 title 和 fallback |

## 11. 组合与视觉规则

### 11.1 信息层级

1. 第一屏先回答“这是什么、当前状态、最重要结论、下一步是什么”。
2. 一张卡只有一个主标题和至多一个 primary Button。
3. 核心结论不得放入默认收起的 CollapsiblePanel。
4. `CollapsiblePanel.variant=indented` 的左侧细线是内容从属关系的缩进引导，不是容器边框；预览实现不得再叠加与其相接的展开态横线。引导线应在内容区内上下留出 `12px`，避免形成“缺右边和下边”的残缺边框错觉。若设计确实需要边框，必须使用四边完整的容器边框。
5. 补充依据、长日志、过程说明优先折叠；结果和错误原因默认展开。
6. 一个区块只承担一种信息角色；状态、正文、输入和操作不要混在同一 Row。
7. Label/Value 至少有一种稳定层级差异：位置、字重、颜色或字号。

### 11.2 分组与布局

1. 根组件优先使用 Column，保持单一阅读流。
2. Row 只用于短、同层级且可并排的内容；任一子项可能变长时改用 Column。
3. Card、背景、间距和 Divider 选择一种主要分组手段；不重复堆叠边框。
4. 相邻内容属于同一语义区时缩小 gap；跨区块时扩大 gap 或使用 Divider。
5. 图文项的图片必须服务于理解；没有真实图片时退化为纯文本项，不自动加入占位图。
6. 文件区独立成组，不把文件 URL 混入正文。

`image-text-item` 使用固定的紧凑合同：外层 `Row(align=center, gap=12)`，主信息放在 `Column(weight=1)`，辅助图片放在 `Image(variant=smallFeature)`；标题最多 2 行、摘要最多 3 行，图片默认关闭大图预览。公开 Image 合同将 `smallFeature` 映射为 `96×96np`，因此 Row 的自然高度应为 96np，文字列在这一高度内垂直居中。不得用 `Row.align=stretch` 声称左右等高：当前公开合同会把 Row 下的 stretch 按 start 处理。若真实端仍把图片外层保留为旧的 200np 高度，说明 Image variant 的 Nest/Renderer 修复尚未完整集成；此时必须删除缩略图并退化为紧凑纯文本项，不能依靠未公开的 height/width 字段绕过。

日程、任务与轻提醒使用 `compact-notification-header`，默认结构为 `Card(padding=0) > Column(gap=0) > CardHeader(trailing=Tag) + Column(padding=8,gap=8)`。`CardHeader` 是完整头部区域，不能用孤立 Tag 替代；分类色由 header 的语义 theme 承担，trailing Tag 只表达短状态。空备注、空事实行和没有分组价值的 Divider 必须从 children 中删除。真实钉钉验证表明，正文 16np padding 会让短通知显得松散，而在宿主已有外层 inset 时再给整个 content 增加 12np 会形成重复留白；因此正文收敛为 8np，header 仍保持原生组件自己的边界。`Text.icon` 在尚未完成客户端接线时还会留下无效视觉预期，紧凑通知不能依赖图标来维持对齐和信息层级。

### 11.3 密度与操作

1. 默认操作区：一个 primary + 最多两个次操作；更多操作应折叠或进入详情页。
2. 同一视觉行不混入输入框和提交按钮，除非目标端宽度 Golden 已验证。
3. 长卡按“摘要 → 关键事实 → 详情 → 操作 → footer”排序。
4. `comfortable` 是默认密度；`compact` 只用于事实行，不能压缩输入控件可点击区域。
5. 视觉规则先作为 `WARNING/REVIEW`，只有协议、安全、不可达操作和核心内容隐藏才是 `ERROR`。

### 11.4 内容规则

1. Markdown 承载长内容；Text 承载短文本，不用多个 Text 模拟 Markdown 段落。
2. 状态 Tag 使用语义色，不用颜色表达第二套业务含义。
3. 图片、文件和链接必须使用可达 HTTPS 或平台明确支持的资源标识。
4. Image 必须有 accessibility 描述；交互控件必须有可读 label。
5. fallbackMarkdown 只用于组件不可渲染时的业务降级，不掩盖字段错误。

### 11.5 一期默认视觉规范与内容自适应

主题选择属于 P2，但一期必须交付一套默认视觉规范：标题/摘要/正文/辅助信息的排版层级、区块内外间距、图片比例、列表对齐、主次操作与语义状态色，以及 light/dark 表现。规范由 Block 和编译器应用，具体数值只选锁定 Catalog 支持的字段和值，通过真实渲染后冻结；不能直接照搬网页 CSS。

间距采用 4np 基线并服从锁定 Catalog 的默认合同，不由各 Recipe 自行发明数值：

| 关系 | 默认值 | 使用规则 |
|---|---:|---|
| 紧密内容 | 4np | 同一事实的值与标签、标题与一句辅助说明 |
| 同级区块 | 8np | Card 顶层 Column 的相邻 Block、Row 内同级控件；对应 Catalog 的顶层/横向默认 gap |
| 容器内边距 | 12np | Card 默认 padding、有背景色的 Row/Column、折叠内容区；对应 Catalog 的 Card 默认 padding |
| 强分组间隔 | 16np | 仅用于没有 Divider 的跨语义大区块；不能作为所有子节点的统一 gap |

实现必须优先使用 `gap` 表达兄弟关系、使用 `padding` 表达容器边界。嵌套容器已有 12np 内边距时，内部从属缩进只增加 8np，禁止再叠加一层 12np 造成 24np 以上的无意左缩进。若组件没有公开的分边 margin/padding 能力，Recipe 应通过 Row/Column 层级表达，不能注入 CSS 或私有属性。reference preview 必须逐值呈现协议中的 `gap`/`padding`，不得用网页端硬编码数值覆盖消息合同；组件 wrapper 与内部原生控件必须使用不同的样式类，避免 TextField、ChoicePicker 等组件重复应用 border、padding 或 ARIA role。

`CollapsiblePanel` 自身没有公开 `padding` 字段。需要内容内缩时，必须把正文包进 `Column(padding=12, gap=8)`，不能依赖 reference preview 或某一端 Renderer 暗中补白。面板标题已经承担区块标题角色，内部 Markdown 不得再以同名 `##/###` 开头；例如标题为“思考摘要”时，正文直接从步骤或说明开始。Header 与紧随其后的折叠区属于相邻同级区块，默认只保留 `8np` gap；除非 Divider 本身表达明确语义边界，否则禁止形成 `header → gap → Divider → gap → panel` 的双倍留白。

容器背景默认透明。Recipe 未收到明确的用户视觉要求时，根 `Card` 显式使用 `backgroundColor=#00FFFFFF`，Row/Column 不配置 `backgroundColor` 或 `backgroundColorToken`；`CollapsiblePanel.variant=reasoning` 在 reference preview 中同样按透明背景呈现。只有用户明确要求色块，或状态、告警等语义确实需要独立 Surface 时，才配置背景，并同时保证文字对比度。Tag、Button 等组件自身的状态色不属于容器背景，继续服从其交互与语义合同。若真实端尚未实现 reasoning 的透明外观，该项应标为端侧能力缺口，不能由 Agent 下发未声明字段绕过。

图片资源区分 reference preview 与真实投递合同。离线预览可以使用 `data:` 资源帮助结构审阅，但 `card send/update/finish` 中新增或修改的 `Image.url` 必须是带 host 的绝对 HTTPS URL；`data:`、`file:`、`blob:`、相对路径和明文 HTTP 均在远端调用前以 validation error 拒绝。内置可发送 fixture 不得把 base64 占位图当作生产图片；需要 Image 覆盖时使用经过可达性检查、允许跨域且来源可说明的 HTTPS 资源，无法提供可靠资源则省略 Image 并按纯文本 Block 降级。reference preview 的资源可见不等于真实客户端取图成功，真实端仍需截图确认。

整卡宽度采用“语义策略与 Wire 能力分离”的合同。通知、日程和任务提醒使用 `notification` SurfacePolicy，产品期望下限为 `360px`；资讯、审批、表单和报告使用 `standard`；思考区、流式 Markdown 和反馈操作组成的 AI 卡片使用 `ai`。三类宽度模式均为 `content-adaptive`。真实 PC 宿主的 A2UI 宽卡目前由 `wideCardWidth` 决定，已知范围是 `320–640px`，因此 360px 是设计期望而不是当前 Wire 可强制的下限。所有 Recipe 必须在 320px 下可读，通过换行、纵向重排或折叠降级，禁止制造横向滚动；宽窗口下由宿主自动扩宽。

这里的 `minWidth` 是 Agent 可查询的设计期望，不等于当前 A2UI Wire 字段；`observedHostMinWidth=320`、`observedHostMaxWidth=640` 才是当前 PC 宿主的已知运行边界。锁定的公开 `a2ui-catalog.json` 中，根 `Card` 没有整卡 `width/minWidth/maxWidth`，发送接口也没有 size 参数，因此编译器不得私自写字段或用不可见长文本撑宽。`dws card guide rules` 返回这些策略及 `enforcement=host-surface-required`；真实宽度由钉钉宿主 Surface 的 `wideCardWidth` 执行。待公开 Catalog 和多端 Renderer 同时支持整卡尺寸后，再以版本化字段落到 Wire。卡片平台 CLI 的 360px 效果图是设计参考宽度，不能单独证明线上宿主已实施最小宽度。

Agent 先生成 InformationPlan，标记每项内容的优先级、事实来源、目标区块和操作意图；compose 返回内容到组件的映射。测试必须检查关键事实、错误原因和操作是否保留，不能为了套模板而静默丢弃信息。

guide recommend 按意图、文本长度、条目数、附件/图片和交互需求选择 Recipe，并返回推荐原因、Slot 映射、容量限制和超限策略。短通知、同类列表、长解释、交互表单分别处理；超长内容保留摘要和完整详情的可达入口，不把长报告硬塞进通知模板。

每个 Block 声明内容压力策略：缺图片时删除占位并转纯文本；长标题允许合理换行；长值优先纵向布局；次要长详情可折叠；无可用链接不生成无效按钮。布局变化须明确由编译器还是渲染器负责：编译器只能按已知目标宽度或保守策略选结构，未知宽度不伪称支持运行时响应式。

### 11.6 可重复的质量闭环

每个 Recipe/Block 覆盖短文、长文、空值、中英文混排、窄屏、暗色、图片/字体失败用例；交互资产另测重复点击、失败反馈、过期、无权限及同实例更新。协议稳定、布局稳定、交互稳定分别记录结果。

Agent 根据结构化诊断和真实截图修订，默认最多三轮（可配置）；每次变更使旧截图证据失效。持续失败时回到已验证的简单布局，保留核心结论、事实与可达操作；无法保证语义或交互时返回明确未完成项。禁止自动发送未经确认目标的降级卡片。

视觉审阅记录标题突出、分组清晰、无截断/溢出、主次操作明确、资源状态和长内容表现，并绑定 sourceHash、Renderer/Bundle 版本、宽度与主题。Golden 对比结合结构和截图检查；视觉差异阈值只用于定位回归，不当作美观评分。高质量样板达到验收后才扩充同类模板。

## 12. 校验器设计

`lint` 返回结构化诊断：

```json
{
  "valid": false,
  "protocolVersion": "1.0",
  "catalogDigest": "sha256:...",
  "mode": "create",
  "diagnostics": [
    {
      "severity": "error",
      "code": "A2UI_COMPONENT_UNKNOWN",
      "messagePath": "/1/updateComponents/components/4/component",
      "componentId": "decision",
      "message": "RadioButton 不在公开 Catalog 中",
      "suggestion": "使用 ChoicePicker + mutuallyExclusive"
    }
  ]
}
```

### 12.1 Layer 1：协议结构

- 归一后的顶层必须是消息对象数组；文件层允许 §6.3 的对象数组、JSONL 和显式识别的旧 wire 数组；
- 每条消息只有一个支持的消息体；
- 版本、profile 和消息类型组合合法；
- `create` 模式必须满足 DWS 建卡所需的 Surface 初始化序列；
- `update` 模式不得无意创建新 Surface；
- `appendDataModel` 只允许 `v0.9.1 + DT_A2UI_V1`，且只追加已绑定的 Text/Markdown 字符串路径。

完整消息校验必须随 Protocol Bundle 同版本交付；采用生成 Schema 或 §16.1 规定的同源校验代码，不能只用附件 Catalog 验证消息。

### 12.2 Layer 2：组件与引用

- 组件通过 Catalog `#/$defs/anyComponent` 校验，并解析 common types；
- 单条 updateComponents 内 ID 唯一；跨批次同 ID 是合法 upsert，校验应用后组件索引的唯一性；
- 根目标为 `id=root`；
- child/children 引用存在，无环、无孤儿关键节点；
- 组件只出现已声明属性，required、枚举和动态类型正确；
- `updateComponents` 中每个对象是该 ID 的完整新定义，不按 JSON Patch 解释。

### 12.3 Layer 3：DataModel 与函数

- 所有 DataBinding 路径符合 JSON Pointer，并在首帧可呈现状态存在真实值；增量只校验“已确认基线 + 本批操作”的结果，不要求每个增量重复初值；无基线时不得声称整卡校验通过；
- 动态字段最终值类型与目标 Dynamic* 一致；
- 函数存在于 Surface Catalog；宿主函数显式使用 `urn:dingtalk:a2ui:host:v1`；
- 输入写回路径、宿主函数结果路径和服务端更新路径无非法重叠；
- `/card/flowStatus` 不作为 append 目标；
- 更新只修改实际存在或协议允许创建的路径。

### 12.4 Layer 4：交互可靠性

- Action 必须是 `event` 与 `functionCall` 二选一；
- Button 文案通过 Text child 提供；
- TextField 和 ChoicePicker 平铺形态的值由提交 Button context 收集；
- 表单 checks 统一挂到提交 Button，并引用最新 DataModel；
- 不给 Markdown、Image、Tag、Divider、Card、CollapsiblePanel 等无事件槽组件添加 action；
- 不依赖 `wantResponse`、`responsePath` 或顶层 `actionResponse`；
- 任何“客户端会点击成功”的声明必须匹配当前 Capability Profile，而不是只看 Catalog 存在。

### 12.5 Layer 5：设计与可访问性

- 一个主标题、一个 primary、主要结论可见；
- 空卡、空选项、空图片列表和占位域名报错；
- 无意义重复 Divider/Card、过密操作、长 Row 返回 warning/review；已在 InformationPlan 标记的核心信息被折叠是 error；raw 输入无法判断信息优先级时要求审阅，不能凭文本猜测；
- 图片缺无障碍说明、输入缺 label、颜色对比无法确认时返回 warning；
- Recipe/Block 的 Slot 完整性和空值退化策略通过。

## 13. 交互可靠性与实例状态

### 13.1 能力等级

每个 Action 必须发布一个能力等级：

| 等级 | 含义 | 可对用户声称 |
|---|---|---|
| `schema_only` | Schema 合法，尚无运行证据 | “协议可声明” |
| `renderer_local` | 本地函数或本地状态已验证 | “当前端可本地执行” |
| `delivery_verified` | 卡片发送和渲染已验证 | “已发送并显示” |
| `round_trip_verified` | 点击 → 回调 → Agent → 同实例更新通过 | “交互闭环已验证” |

当前协议资料明确指出 `Action.event` 宿主事件桥仍待补齐，因此一期不得默认把 event Action 标为 `round_trip_verified`。`openUrl` 等本地函数与 Agent 回流是两类能力，必须分开表达。

### 13.2 状态回执

`send --state-out` 生成不含凭据的 `dws-a2ui-card-instance/1.0`：

```json
{
  "version": "dws-a2ui-card-instance/1.0",
  "handle": "card_example",
  "profileRef": "sha256:...",
  "target": {"kind": "group", "stableIdHash": "sha256:..."},
  "bizId": "...",
  "cardInstanceId": "...",
  "surfaceId": "approval-20260915",
  "flowStatus": "PROCESSING",
  "revision": 1,
  "catalogDigest": "sha256:...",
  "messagesDigest": "sha256:...",
  "createdAt": "..."
}
```

更新前校验：

- 当前 profile 与回执 scope 一致；
- `bizId` 和 `surfaceId` 未漂移；
- Catalog 兼容；
- revision 前置条件匹配；
- 比较内容和 flowStatus：二者均不变返回 no-op，不出网；内容相同而状态变化仍需执行，不能误拦 FINISH；
- 终态后不允许普通追加，除非显式恢复策略允许。

任何发送超时或返回不确定都记为业务数据内的 `deliveryState=delivery_unknown`，保留幂等键并回读，不自动重新创建。外层 outcome 仅使用 DWS 支持的 `success/pending/partial_failure/failure`，按执行事实由框架映射；不得新增 `outcome=unknown`。`appendDataModel` 没有可靠 opId 时不得盲目重放，先核对状态，必要时用 FULL_DATA 的 `updateDataModel` 校准最终文本。

### 13.3 台账、幂等与送达证据

`send` 写前创建稳定 handle 与操作记录，服务回包后原子更新。台账位于 DWS 解析的私有 Card 状态目录，按主体、组织、环境隔离，由 Card Runtime 单写。`--state-out` 导出便携回执，不成为第二个状态权威；导入须与台账协调并复验 scope。台账保存真实目标用于续操作，对外默认脱敏；仅保存目标摘要不足以在新 shell 完成回读。

- 每次新业务操作使用新 operationId；幂等键按 scope + operationId 定位，digest 只检查参数一致性。同键同参复用记录，同键异参拒绝；相同文案的两次授权发送允许两张卡。
- 出网前持久化 pending 记录、输入摘要与发送前会话基线；回包丢失或崩溃保留 unknown。原子替换、写锁和恢复日志需经过故障注入验证。
- `verify <handle>` 优先用真实传输标识关联消息；会话基线 diff 还需目标、时间、可用标识等证据约束。唯一新消息本身不足以证明是本卡；多候选或缺少关联信息保持 unknown。
- `list/show` 输出台账与证据；关闭发送终端后，新 shell 仅凭 handle 和有效身份可 update/finish/verify。
- 本地 revision 只防本机并发；远端缺少版本前置条件时，不宣称具备服务端 CAS 或跨机器恰好一次保证。

状态分别记录 deliveryState（accepted_unverified/delivered/delivery_unknown/definite_failed）、flowStatus、previewEvidence、clientEvidence、interactionEvidence。FINISH 不覆盖 delivery_unknown；一次点击只证明对应组件和客户端链路，不自动证明其他输入控件回写成功。

### 13.4 监听就绪、事件归因与业务交互

`send --watch` 先取得订阅 ready/cursor，再发送；ready 是结构化握手，日志文字不能充当程序判断。复用账号级 event bus，由存活 Host 代理订阅。事件先适配实际版本的原始信封，不能假设 RendererToAgent 格式已经上线。

归因要求可信传输身份、目标一致，并用 bizId 与 surfaceId 等实例标识交叉验证；缺失主标识只允许经测试的等价关联，不因群相同或 Action 同名归因。未归因事件保留、计数；重复 eventId 去重。operator 来源于可信传输，不能信任 context 自报身份。

表单链路定义 idle → submitting → succeeded/failed，并处理重复点击、失败重试、过期和无权限。状态通过锁定组件支持的字段或显式反馈区表达，不假设 Button 有未声明属性。业务处理成功后才更新成功文案；技术回调到达不等于业务成功。

### 13.5 流式创作：更新批次、状态归并与检查点

**现有实现事实**：`internal/helpers/chat.go` 的 `chat message update-a2ui-card` 要求 biz-id/content/flow-status，content 是非空 JSON 字符串数组，可选 a2ui-annotations。CLI 每次生成 requestId，把消息数组透传给 im.update_a2ui_card；没有累计状态、自动 diff、分片拼接或增量/全量模式开关。请求可只包含本轮更新操作，不要求重复完整创建序列。

按锁定公开协议区分三种更新语义：

| 消息 | 增量的单位 | 必须注意 |
|---|---|---|
| updateDataModel | 指定 JSON Pointer 对应的值 | 更新字符串时传该路径的当前完整字符串，覆盖而非追加；普通更新用精确路径，避免根模型更新覆盖用户输入 |
| updateComponents | 组件 ID 集合 | 只发变化的组件，但每个组件对象是完整新定义；不是字段级 patch，省略旧属性不会保留 |
| appendDataModel | 已绑定 Text/Markdown 字符串末尾的文本片段 | 钉钉扩展，按目标端能力与 v0.9.1 扩展信封使用；无可靠去重时禁止盲目重放 |

FULL_DATA 指字符串路径的完整检查点，不等于重发整张卡、全部组件或 createSurface。具体信封、路径和值类型以协议包为准，不能在 CLI 发明 patch 操作。发送数组元素必须是完整可解析消息；不得把模型吐出的半段 JSON、半个组件对象直接发送。文本片段由 JSON 编码器编码，不用字符串拼接 JSON。

#### 默认策略：稳定结构 + 路径快照

首帧发送稳定框架和正文/状态初始绑定；标题、正文区、详情区保持稳定 ID。Agent 按段落或有意义的阶段更新同一卡片：普通正文优先 updateDataModel，传该正文路径当前累计内容；只有新增区块或改变布局/Action 才发送 updateComponents。新增节点、父 children 与数据初值作为一个完整批次验证，不能出现悬空引用；传输不保证批次原子呈现时需额外验证消息顺序与中间帧。

后续不反复重建布局，不把光标所在表单控件替换为新 ID。数据分为 Agent 管理路径、用户输入路径、宿主结果路径；快照/diff 只操作 Agent 管理范围。未闭合 Markdown 代码块等中间状态在预览中有专门样例，不能靠每次补结束符改变最终正文含义。

默认每个 handle 同时只有一个在途更新。Agent 主动在语义段落/阶段完成时调用 update；CLI 可合并待发送的同路径快照，但不在后台虚构尚未生成的内容。节流初始建议 300–800ms，另受接口频率、字节预算约束，实施压测后确定；不逐 token 启动 CLI。结束时 flush 最终正文检查点，再设置 FINISH；前序 unknown 未解决不得报告最终内容已送达。

#### 目标 CLI 接口（待实现）

保留 `dws card update <handle> --file <batch.jsonl> --flow-status INPUTTING` 作为精确协议更新入口。新增 `--input-mode messages|snapshot`（默认 messages）：messages 表示合法更新消息批次；snapshot 表示完整期望 Surface 状态，由编译器与本地已确认基线比较，生成标准更新消息。snapshot 不是把完整 create 序列原样重发；绑定范围之外的用户输入保留。组件移除仅采用锁定协议支持的操作，不发明删除消息。

本次评审将 messages 与 snapshot 两种输入模式都纳入一期必验，不再作为未定可选项。snapshot 使用版本化工具链结构 `{version,surfaceId,components,dataModel}`，作用于台账登记的 Agent 管理范围；数据对象内缺失字段默认保留，删除仅通过锁定协议明确支持且已声明的删除能力处理，否则报错。绝不采用通用递归 merge 或把缺失字段视为删除。期望状态必须先可解析并完整验证，再生成协议差异。

`finish <handle>` 可不带文件：从已确认的 Agent 管理路径生成非空检查点并发 FINISH；若底层已验证允许仅状态更新，可使用该能力。两条路径均无法安全构造时明确失败，不发送无协议依据的空数组或重建 Surface。相同内容的状态变更仍计作新操作；未知基线先核实再继续。

上述 input-mode 属于 CLI 工具链元数据，不进入 A2UI。新操作记录包含 operationId、sequence、baseRevision、sourceHash、resultHash 和确认状态。revision/sequence 本地维护，远端不支持时不得伪装为已具有远端 CAS/去重。每次请求沿用同一 bizId/surfaceId、profile 与 Bundle。

路径快照方案优先交付；append 作为按能力开启的优化。只有可证明扩展支持、按序处理和去重语义时才启用文本 DELTA；网络断开后先对账并停止后续追加，必要时使用同路径完整字符串检查点恢复。状态不能确认时暂停该实例并返回恢复动作，不自动创建替代卡。周期检查点以及最终检查点减少回放长度，但未证明旧请求不会晚到前，检查点本身不能保证消除乱序覆盖。

未来连续流可用一次存活 Host 会话接收完整消息帧，复用 Runtime 与授权代理；需定义明确帧边界、背压、取消和授权失效，本期不把独立进程驻留等同于已经有流式输入 API。

#### 流式验收

- 多批正文从短到长，所有消息合法且最终内容精确一致，不重建 Surface。
- 只更新一个绑定路径时其他数据与用户正在输入的内容保持不变。
- 改按钮文案：绑定字段更新数据；字面量字段重发完整组件定义，Action 不丢失。
- 新增区块时引用与数据完整，中间帧不闪空、不出现断裂布局。
- 模拟重复 DELTA、丢回包、乱序/晚到、进程退出和断线恢复，无重复追加、无旧内容覆盖，无法证明时明确 unknown。
- 最终 flush 与 FINISH 的顺序可追踪，结束回包不冒充客户端最终渲染证明。
- 长文、代码块、窄屏与输入期间的连续更新完成真实渲染和交互回归。

## 14. 预览与质量证据

一期 `preview` 应使用与目标 Renderer 同源的 A2UI → Durbo 转换和公开组件实现，生成：

- 卡片 PNG；
- 可选 HTML/SVG；
- 组件 fidelity 列表；
- fallback/rejected_actions 诊断；
- light/dark 两种参考图；
- 输入与 Action 的交互模拟记录。

预览实现不能在 Go 中重写一套近似组件。推荐将官方转换器和 Web Renderer 打包成版本化、沙箱化的预览资产，由 Go 进程以结构化 stdin/stdout 调用；构建时固定版本和摘要，不在运行时下载任意脚本。

证据分层：

```text
Schema PASS
  ≠ Composition PASS
  ≠ Local Preview PASS
  ≠ Send Receipt PASS
  ≠ Client Render PASS
  ≠ Action Round Trip PASS
```

Agent 必须检查预览中的层级、分组、节奏、密度、主操作可达性和长内容表现。规则校验不输出虚假的“美观分数”；主观部分使用结构化 review checklist。

### 14.1 图片回传合同

预览返回值在 data 内声明 previewId/status/image/renderer/sourceHash/schemaBundleRef/diagnostics/evidence。image 包含绝对路径或受控资源 URI、MIME、尺寸和 sha256；renderer 包含版本、宽度、主题。status=ready/partial/failed：图片或字体缺失为 partial；无匹配 Renderer 返回 renderer_unavailable，不用重画 HTML 或旧截图替代。

校验、编译和截图共享同一输入快照摘要；文件改动使旧预览证据失效。适配器等待首帧及资源稳定信号，有超时、内存和输出限制。预览禁用真实 JSAPI 写操作、导航与业务回调；交互模拟显式标记。Harness 验收要证明模型实际收到图片并发现预置的截断/层级问题；只返回路径不算看图闭环。无图像能力的 Harness 将视觉项标为人工待验。

## 15. P2 主题设计

当前 common types 中的 message-level `theme` 是保留类型，并不代表已经接入任意主题系统。因此 P2 主题先作为**编译期 Design Token Profile**，由编译器映射到公开组件已有字段，不新增运行时协议字段。

建议初始主题：

| 主题 | 用途 | 主色 | 状态策略 |
|---|---|---|---|
| `ding-blue` | 默认企业与协作 | 蓝 | 蓝主操作，语义 Tag 保持独立 |
| `neutral` | 报告、文档、技术结果 | 黑/灰 | 低装饰、强调内容 |
| `success` | 完成、交付、通过 | 绿 | 绿色只用于状态和主结果 |
| `warning` | 风险、待处理、提醒 | 橙 | 主操作仍保持可识别，不整卡橙色 |
| `critical` | 错误、阻断、高风险 | 红 | 红色用于错误事实，不滥用为背景 |

Theme Token 只包括语义颜色、排版 preset、间距、圆角、边框和图片风格。Recipe/Block 引用语义 token，不复制具体值。编译后只输出 Catalog 已开放的 `colorToken`、`backgroundColorToken`、`theme`、`variant` 等字段。若目标端不支持某 token，必须有确定性降级。

## 16. 协议包同步、版本与供应链

### 16.1 发布单元

上游应发布一个可公开消费、不可拆分的 Bundle：

```text
dingtalk-a2ui-protocol-bundle/<version>/
  manifest.json
  message-schema.json
  a2ui-catalog.json
  a2ui-common-types.json
  profile.json
  capability-matrix.json
  authoring/message-skeleton.json
  authoring/rules.json
```

`manifest.json` 至少包含协议版本、Catalog ID、源提交、生成器版本、每个文件 SHA-256、发布时间、最低支持端版本和兼容级别。

DWS 固定 Bundle 版本，由安装清单统一分发到版本目录；Host 与 Card 共用加载库和摘要，可以 embed 基础资源但不建立两套默认版本。运行时不从任意 URL 拉取 Schema；`protocol verify` 只做显式检查。升级 PR 必须包含结构 diff、语义 Registry 漂移、Golden 和客户端能力矩阵变化。

完整消息 Schema 是目标产物。若上游尚未生成，一期先提供与主协议同源、版本化且有正负例的消息校验代码及骨架；manifest 明确实现版本，不虚构已有 message-schema.json。后续切换为生成 Schema 时做行为等价回归。Draft 2020-12、跨文件基址与关键字行为须与上游加载器兼容测试。参考文档报告的生成漂移须复跑确认并修复后再固定发行包，不能仅因 JSON 能加载就发布。

### 16.2 开源边界

`dingtalk-workspace-cli` 是公开仓库，而 `card-docs` 是内部仓库。DWS 只能提交已经确认可公开发布的机器协议与语义说明。不得直接复制内部文档、内部 URL、内部组件或未公开能力。实施前需要协议 Owner 和开源合规确认 Bundle 的发布位置与许可证。

## 17. 推荐代码落点

```text
internal/card/a2ui/
  protocol/
    bundle.go                 # embed、manifest、摘要和引用解析
    validate_message.go       # 消息信封与 Profile
    validate_component.go     # Catalog/common types
  semantic/
    registry.go               # 语义组件元数据
    registry.json             # use_when/avoid_when/role/aliases；不复制协议事实
  composition/
    spec.go                   # dingtalk-a2ui-composition/1.0
    compiler.go               # Recipe/Block/component → A2UI messages
    ids.go                    # 稳定 ID 与路径命名
  assets/
    blocks/*.json
    recipes/*.json
    themes/*.json             # P2
  lint/
    tree.go
    binding.go
    interaction.go
    design.go
    accessibility.go
  preview/
    adapter.go                # 调用固定版本 Renderer 资产
  delivery/
    service.go                # send/update 共享服务
    receipt.go
    target.go                 # 复用 chat/contact resolver

internal/helpers/card.go      # dws card Cobra 命令
internal/helpers/chat.go      # 旧命令仅保留薄兼容包装

skills/multi/dingtalk-card/SKILL.md
skills/multi/dingtalk-card/references/a2ui/
  overview.md
  components.md
  blocks.md
  recipes.md
  interaction.md
  troubleshooting.md
```

实现注意：

- `internal/card/a2ui` 不导入 `internal/cli`；命令声明由 `helpers.LeafSpec` / `corecmd.ContractDecl` 完成。
- `register_products.go` 是生成文件，不手改。新增 `card` 产品应通过生成源或独立的受评审 `register_card.go` 注册，并纳入生成漂移检查。
- DWS Schema Catalog 只发布命令选择与参数合同，不嵌入 170K A2UI Catalog。组件合同通过 `dws card catalog get` 渐进查询。
- Skill 只负责路由和工作流，不能成为协议字段权威；协议与语义输出以运行时 Registry 为准。

### 17.1 受管独立进程与 Host 授权代理

采纳参考方案的目标形态：用户只安装 DWS；发行物包含受管 dws-card、协议包、示例和可验证 Renderer。协议/语义查询留在 Host；compose/lint/build、预览、投递、台账和事件归因经 Runtime Client 执行。进程隔离的性能收益必须实测，不用历史 CLI 耗时推算。

```text
Agent → DWS 命令声明与 Runtime Client → 私有 IPC → dws-card
          ↑                                      │
          └──────── Host Broker ← host_call ──────┘
                    │
                    └→ 既有 MCP / OpenAPI / event bus 适配
```

| 新落点（建议） | 职责与验收 |
|---|---|
| `cmd/dws-card/` | 独立进程入口；通过安装清单绝对路径启动，摘要和版本校验 |
| `internal/card/runtimeclient/` | 端点发现、启动锁、锁内二次连接、ready、超时、取消、drain |
| `internal/card/ipc/` | hello/invoke/progress/result/cancel/host_call；版本、序号、帧限额与背压 |
| `internal/card/hostbroker/` | 连接内 contextRef 对应明确主体/组织/目标/允许操作；每次调用复验 |
| `internal/card/ledger/` | 单写、原子更新、幂等与恢复；导出 receipt |
| `internal/card/events/` | 复用订阅、ready、归因、去重、未归因留存 |
| `internal/card/evidence/` | 基线与标识关联、deliveryState、独立流程状态、show/verify |

Unix 使用用户私有 Socket，Windows 使用当前用户 DACL 的 Named Pipe。清单固定进程与协议契约版本；并发调用仅一个启动者。大输入通过已校验内容流或受限引用传递，校验路径、摘要、大小及稳定快照；禁止任意文件读取和任意 shell 分派。

DWS 保留长期凭据；Card 不读 OAuth 配置。Host Broker 仅执行本次调用批准的操作与目标，不接受任意 URL、另一 Profile 或通用 CLI 指令。contextRef 连接断开即失效，不能落盘恢复授权。同用户进程隔离不是强安全沙箱。

Host 退出后停止该连接的任务与订阅；已出网写入未获回包时落 unknown，取消不伪称撤回。进程重启先恢复台账，不能自动重放创建。`runtime status` 不启动进程；stop/restart 排空活动任务。冷启动、热连接、同会话连续更新分别计时；覆盖 Windows Pipe 与各发行平台，不以 macOS Socket 实验证明全平台通过。

### 17.2 当前 DWS 框架适配决策

参考文档中的“静态 Schema 自动投影 MCP/SDK”按当前仓库约束细化：叶子由 LeafSpec/ContractDecl/Safety/ParamDecl/Result 声明，身份来自活 Cobra 树，运行时通过 ResolveSchemaBuild 装配。非叶子声明完整 GroupPolicy。禁止恢复 schema_catalog、schema_hints 或平行命令清单；MCP/SDK/Harness 接入逐个验证，不因 CLI 有 Help 就视为接通。

外层结果由 internal/output 生成，Result 只描述 data；退出码沿用 DWS 标准，不直接复制参考文档的 0/1/2/3。原参考的 default repair 改为：normalizer 不改业务含义，build 仅在显式 repair 策略下应用经过测试的兼容规则，返回 ruleId、前后摘要和源码位置，并重新校验；未知字段、按钮意图和业务权限禁止自动猜测修复。

当前引用的源码未检出连接器台账实现。因此复用分两步：先固定连接器所属版本与接口证据，逐项比对 ID、错误、幂等、回读和事件；再用薄适配器接入公共台账。不把“同名字段”当作相同语义，不在本期重写连接器的队列与恢复逻辑。

## 18. 分阶段计划

### Phase 0A：进程接缝与基础包

- 实现 Runtime Client、dws-card、安装清单、私有 IPC、Host Broker；
- 无登录静态查询，无卡片 Skill 的标准安装合同发现；
- 本地 lint 独立 PID、并发启动锁、上下文隔离、取消、崩溃与跨平台测试；
- 协议生成同步门禁与完整消息校验正负例。

通过标准：原生 DWS 发起调用，匹配发行二进制执行；越目标请求拒绝，断连授权失效，帮助不启动 Runtime。隔离 probe 通过只作为开发证据，不能替代本阶段。

### Phase 0：协议与命令迁移

- 固定可公开 Protocol Bundle，补齐完整消息 Schema 和 capability matrix；
- 新增 `dws card send|update|finish|verify|list|show` 及 protocol/catalog/lint/build；
- 抽取共享 Delivery Service；
- 保留旧 chat 路径；
- 为新命令声明完整 Contract、Safety、Result 和参数约束。

通过标准：新旧路径对同一输入生成相同 IM RPC payload；新路径能查询协议版本和一期组件合同；Schema/Help/运行参数同源。

### Phase 1A：Agent 生成质量

- 建立 Semantic Registry；
- 实现一期 15 个概念（14 个真实组件 + radiobutton 概念别名）；
- 实现 15 个 Block 和 7 个 Recipe；
- 实现 `guide recommend`、`compose`、五层 `lint`；
- 随包交付规则与样例；按需生成可选 Skill references。

通过标准：标准评测集中的内容都能由 Recipe/Block/组件树表达；不存在未知组件/属性；信息层级和 Action 规则无 ERROR。

### Phase 1B：预览与真实投递

- 接入同源 Renderer 预览；
- 建立 light/dark、长文案、窄宽度 Golden；
- 在 Phase 0 台账基础上完成预览/交互证据关联与回执导出回归；
- 实现 watch、send --watch、doctor 与监听就绪/事件归因；
- 完成 PRE 群聊与单聊的 send → update → FINISH；
- 对本地函数和 Agent event 分层验收。

通过标准：本地预览、真实发送回执、客户端渲染和交互回流分别出具证据；不得用前一层代替后一层。

### Phase 2：主题与生态化

- 引入 Design Token Profile 和主题选择；
- 增加多端能力画像、截图 diff 和回放；
- 评估组织级自定义 Block Registry；
- 扩展一期已验证的 `round_trip_verified` Action 覆盖范围；一期 approval 闭环仍按 §23 必验，事件桥缺失即阻塞一期完整交付，不能移到 P2 后宣称一期完成。

资产复用作为独立 P2 轨道：Template 保存完整 A2UI；Capability 的 Public Spec 暴露业务 input/update/events/result；私有 Binding 映射字段与事件；服务端 Instance 固定版本。Recipe 仍是本地结构资产，不自动成为发布模板。

提取器按 Schema 声明的 Dynamic*/Action 位置遍历，不能搜索所有名为 path 的字段。类型冲突、数组约束和业务语义不明时返回 unresolved；作者确认后才发布。服务端按固定版本做白名单赋值并复验，不递归 merge 任意输入。template update 使旧审核失效；发布保证模板与 Capability 一致激活。P0 handle 与 P2 instanceId 分开存储，均不能冒充底层 bizId。

该轨道纳入后续设计，不增加本期 Gateway/模板服务交付依赖。作者 Agent 仍能通过文件创作发送新卡；业务复用 Agent 仅调用已发布业务契约。

## 19. 测试与验收

### 19.1 自动化测试

| 测试 | 内容 | 通过标准 |
|---|---|---|
| Bundle integrity | manifest、SHA、ref 解析、离线加载 | 任一文件漂移或悬空 ref 失败 |
| Component contract | 一期组件 required/enum/examples | 全部示例通过，RadioButton 明确拒绝并给替代 |
| Message validation | create/update/append 顺序与版本 | 非法顺序、错误 surface/profile 被拒绝 |
| Tree validation | ID、引用、根、环、孤儿 | 错误带 JSON Pointer 和组件 ID |
| Binding validation | 路径、类型、初值、函数 | 无未初始化绑定或类型漂移 |
| Interaction validation | Action oneOf、事件槽、checks、context | 不存在“看起来可点但合同不可达”的模板 |
| Recipe/Block Golden | 编译确定性、空值、长内容、双语 | 同输入同摘要，所有输出通过完整校验 |
| Design lint | 层级、primary、折叠、密度、a11y | ERROR 为 0，warning 有结构化审阅结论 |
| Delivery parity | 新旧命令 payload | 除 requestId 等显式非确定字段外等价 |
| Receipt/update | scope、revision、bizId、surfaceId | 任何漂移在 RPC 前失败 |
| Schema contract | 命令身份、参数、Safety、Result | Help、Schema、运行时一致 |

### 19.2 仓库门禁

一期实现至少运行：

```bash
gofmt -w <modified-go-files>
go generate ./internal/cli
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/card/... ./internal/helpers/... ./internal/app/...
DWS_PACKAGE_VERSION=0.0.0-test go test ./...
make build
```

如果测试承担 macOS platform coverage gate，名称必须符合 `TestCrossPlatformCoverage*` 或 `TestAllShortcuts*`。

### 19.3 真实验收矩阵

| 场景 | 本地协议 | 本地预览 | PRE 发送 | 同实例更新 | 客户端渲染 | Action 回流 |
|---|---:|---:|---:|---:|---:|---:|
| notification | 必须 | 必须 | 必须 | 必须 | PC + 移动端 | 本地跳转 |
| task-reminder | 必须 | 必须 | 必须 | 必须 | PC + 移动端 | event 标明当前等级 |
| approval | 必须 | 必须 | 必须 | 必须 | PC + 移动端 | 输入、选择、提交全链路 |
| rich-media | 必须 | 必须 | 必须 | 必须 | light/dark | 链接跳转 |
| file-result | 必须 | 必须 | 必须 | 必须 | 文件图标/打开 | preview/download |
| project-status | 必须 | 必须 | 必须 | 必须 | 折叠/展开 | 可无 Agent 回流 |
| information | 必须 | 必须 | 必须 | 必须 | 结论/依据/详情层级 | 有 Action 则验对应行为 |
| raw escape hatch | 必须 | 必须 | 抽样 | 抽样 | 抽样 | 按能力等级 |

验收结果使用 §25 定义的 NOT_RUN/PASS/FAIL/BLOCKED/NOT_IN_SCOPE；待执行和已失败不能伪装成阻塞。PASS 按证据层附所需摘要、版本、截图或回执；纯离线测试不要求伪造目标端版本。业务回流 Action 的 PASS 必须有点击到同实例更新的 trace，本地函数则记录其真实本地行为。

## 20. 效果指标、灰度与回滚

### 20.1 指标

- 首次协议校验通过率；
- 首次预览无需结构修复的比例；
- 平均需要查询的组件合同数和 Agent token；
- Recipe / Block / raw 三种生成路径占比；
- 发送失败、未知结果和错误目标拦截数；
- fallback、rejected_actions、客户端不支持数；
- 用户点击率、表单完成率、Action 回流成功率；
- 同一任务平均 update 次数；
- 用户最终修改相对 Agent 初稿的结构化差异。

不以“模板使用率越高越好”为目标。若 raw 组件树首次通过率与效果更好，应缩薄 CompositionSpec，而不是为了架构完整强制 Agent 使用模板。

### 20.2 灰度

1. 命令发现灰度：先开放 protocol/catalog/lint；
2. 生成灰度：开放 Recipe/Block/compose/preview；
3. 投递灰度：新 `card send|update` 进入推荐 Schema，旧路径仍可用；
4. 交互灰度：按 client capability 和 Action 等级逐项开放；
5. 主题灰度：仅影响编译默认值，不改变 Protocol Bundle。

### 20.3 回滚

- Protocol Bundle 固定版本，可一键退回上一摘要；
- Semantic Registry、Recipe、Block 独立版本化，可禁用单个资产；
- 新命令可从 Agent 推荐中撤下，旧 chat 路径继续工作；
- Preview 失败不阻断原始消息的离线 lint，但默认 send 仍必须通过协议与安全校验；
- Action 运行证据回退时自动降级能力等级，不能继续声称已验证。

## 21. 风险与待确认事项

| 风险/待确认 | 处理 |
|---|---|
| 参考设计与当前 DWS 分支不是同一实现基线 | 固定版本并核对接缝；外部文档目标不充当源码证据 |
| 完整消息 Schema 未包含在用户附件中 | 先交付同源版本化校验代码/规则，再演进生成 Schema |
| 参考文档记录协议生成漂移 | 冻结发行 Bundle 前复跑并修复；保持旧可用包 |
| 独立进程尚未原生接通 | Phase 0A 验收安装、IPC、Broker、崩溃恢复和各平台 |
| A2UI event 桥当前未完整闭环 | 能力分级；一期不误报 round-trip |
| DWS 为公开仓库、card-docs 为内部仓库 | 只消费经过公开审批的 Bundle |
| Renderer 与客户端版本漂移 | 固定转换器/Renderer/客户端 capability 摘要 |
| Recipe 越积越多 | 以高频、稳定结构、独特交互合同为准入门槛 |
| 模型过度依赖模板 | 始终保留受约束组件树和 raw escape hatch |
| 视觉规则僵化 | 规则只硬拦确定性错误，审美保留 preview + Agent review |
| 发送未知结果造成重复卡片 | receipt、request trace、对账优先，不自动重发 |

## 22. 需求追踪矩阵

状态只允许 `PASS / PARTIAL / FAIL / BLOCKED / NOT_RUN`。`PASS` 必须同时有实现、自动化证据及本项要求的实际环境证据；源码或 reference preview 通过不能替代真实端验收。

| 需求/设计项 | 当前实现落点 | 当前证据与缺口 | 状态 |
|---|---|---|---|
| Agent 知道精确协议 | `internal/card/a2ui/protocol/` + `dws card protocol/catalog` | Catalog/Common Types 已内嵌并可查询；被 Go regexp 降级的 HostResultPath 与 `dt_actionBindingsV1` 已补显式语义校验，协议来源许可证和公开发布凭据仍未闭环 | PARTIAL |
| Agent 知道使用语义 | `internal/card/a2ui/authoring/` + `guide`/`catalog` | 一期组件、Block、Recipe 可发现；尚未形成独立、版本化的语义 Registry 资产 | PARTIAL |
| 内置 A2UI 模板 | `authoring` Recipe + `internal/helpers/testdata/a2ui-*-spec.json` | 7 个 Recipe、7 份 fixture 与 Gallery 已接入 preview acceptance，并产生 7/7 Recipe、14/14 组件收据；真实端视觉仍未验收 | PARTIAL |
| 常用区块 | `internal/card/a2ui/authoring/authoring.go` | 15 个 Block 已注册并有定向测试；复杂 Slot/有序 Block 能力仍不足 | PARTIAL |
| 合理组合策略 | `guide rules` + compose/lint + §11 | 已发布部分层级、间距、操作与折叠规则；仍缺全部规则的机器诊断与事实保真回执 | PARTIAL |
| 一期组件支持 | `protocol` + `preview` + `authoring` | 14 类真实组件在 Gallery 可覆盖；RadioButton 为概念映射，真实多端能力仍未验收 | PARTIAL |
| 美观效果 | `internal/card/a2ui/preview/` + Gallery | reference renderer 可进行结构和交互模拟；真实 PC/iOS/Android、字体、资源、暗色截图未完成 | PARTIAL |
| 可靠交互 | preview 模拟 + Action 合同 | 本地输入、选择和按钮可模拟；真实事件回流、重复点击、失败、过期和最新 DataModel 未验收 | PARTIAL |
| 命令迁移到 `dws card` | `internal/helpers/card.go` + `register_card.go` + `chat.go` | 新命令树为 Agent 默认路径；旧 chat 原子命令保留兼容并明确 deprecated。共享 Delivery/payload parity 尚未完成 | PARTIAL |
| 写操作确认合同 | `internal/helpers/card.go` + Contract/Safety | send/update/finish 均声明 `user_required`；Runtime 无确认零写入、dry-run 放行，Help/Schema/Runtime truth 门禁一致 | PASS |
| 同实例更新与作用域 | `internal/card/a2ui/delivery/store.go` + `state/state.go` | 台账冻结 profile selector、环境摘要和发送目标；update/finish fail-closed，handle 跨进程锁与本地 revision CAS 已覆盖；服务端无 revision CAS/回读能力，保证边界仍为本地 | PARTIAL |
| Snapshot 增量编译与输入保护 | `state/state.go` + `card update --input-mode snapshot` | snapshot 独立严格解码、完整 create 校验、叶级 delta、全部公开输入控件绑定路径保护、数组 Pointer、删除拒绝与合并后 Surface 复验均已通过；`card show` 的 surface 可直接 round-trip 为 snapshot；真实端最新 DataModel 回读尚未具备 | PARTIAL |
| 协议负例与更新完整性 | `protocol/protocol.go` + tests | `updateComponents.components` 必须为非空数组；HostResultPath 与 `dt_actionBindingsV1` 已补等价显式校验和正负例 | PASS |
| P2 主题 | 目标：theme assets + compiler token | 尚无版本化主题资产和真实多端降级矩阵 | NOT_RUN |
| 卡片平台模板转换为 A2UI | §27 + 当前本地审阅输入 | 已完成探索性逐区映射；公共仓库中尚无可复现的来源 manifest、合法来源资产和固定截图合同 | BLOCKED |
| 一期默认视觉规范 | §11 + authoring defaults + preview | 4/8/12/16 节奏、透明容器、图标/文字/Button 默认值已实现部分；真实端未验收 | PARTIAL |
| 内容保真与自适应 | CompositionSpec + compiler | 固定字段模式可生成 7 Recipe；尚无完整 InformationPlan、fact→component 回执和异构 Slot | PARTIAL |
| 有界视觉修订 | preview + Agent 工作流 | 有 reference preview；没有版本化三轮修订 Harness 和模型评测记录 | NOT_RUN |
| 生成上下文闭包与锁文件 | `protocol/` embed + digest | 资源可离线加载；缺公开 Bundle manifest、授权凭据和升级兼容报告 | PARTIAL |
| 受管进程与授权 | 目标：runtimeclient/ipc/hostbroker | 尚未实施；当前命令直接走现有进程和 MCP 接缝 | NOT_RUN |
| 持久台账与回读 | `delivery/store.go` + list/show/verify | 本地 ledger 可跨 shell 使用，并具 scope 校验、跨进程锁和 revision CAS；`verify` 仍只验证本地一致性，尚无服务端回读 | PARTIAL |
| 流式分批更新与状态归并 | update/finish + state reducer | messages/snapshot 均可生成增量；用户输入路径保护、本地串行化、revision CAS 与 FINISH 检查点已覆盖；节流、乱序恢复、远端 CAS/回读未闭环 | PARTIAL |
| 监听就绪与事件归因 | 目标：events + `send --watch` | 尚未实施 | NOT_RUN |
| 图片实际进入模型上下文 | preview 输出 | 可生成 HTML；没有证明模型读取图片和识别预置视觉问题的 Harness 证据 | NOT_RUN |
| 业务资产复用 | 目标：P2 template/capability/instance | 尚未实施 | NOT_RUN |

## 23. 一期完成定义

一期只有在以下条件全部满足时才完成：

1. 新命令、旧兼容命令、Schema 与 Help 全部通过仓库门禁；
2. 协议 Bundle 可离线、可复现加载，Catalog/common/message/profile 无悬空引用；
3. 7 个 Recipe、15 个 Block 和一期组件全部有 Golden 与负例；
4. Agent 评测集中不再生成 `RadioButton`、错误 Action 槽、未初始化绑定或手写字符串数组；
5. 本地预览输出结构化视觉复核结果，不把预览误报为客户端证明；
6. PRE 群聊和单聊均完成 send → update → FINISH，且更新同一实例；
7. approval 至少完成一次真实输入、单选、提交、回调和同实例更新；若事件桥未上线，该项明确标为 `BLOCKED`，一期只能称“展示与本地交互一期完成”，不能称“可靠交互一期完成”；
8. 发布说明包含旧命令兼容周期、协议摘要、已验证客户端范围和回滚方式；
9. 标准安装环境无需源码仓库、额外卡片 Skill 或另行安装 Renderer 即可完成创作与看图；
10. 原生 IPC/Broker、跨 shell 台账恢复、发送未知状态、监听竞态全部通过；
11. §25 的一期用例、资产矩阵与命令覆盖全部 PASS；messages/snapshot 增量更新、输入保护、最终检查点和 FINISH 通过。append 未开放时只豁免正向追加测试，能力拒绝/降级测试仍必验。

## 24. 本次补充的验收与参考取舍

| 参考来源 | 本方案处理 | 必须执行的验收 |
|---|---|---|
| §1/§6 业务动词命令 | 改为 card send/update，保留旧 chat 包装 | 新旧 RPC parity；新路径反向 Schema 完整性；不遗留 card a2ui 推荐 |
| §3.3/§7.16 生成上下文 | 采纳引用闭包、消息骨架、版本锁和无需 Skill | 干净安装仅通过命令生成通知、表单、图文卡；缺 ref 与版本冲突负例 |
| §5.8 受管进程 | 作为目标架构，新增 Phase 0A | 原生 Host 到 Runtime 到 Broker；并发启动、掉线、恢复、平台矩阵 |
| §6.0.1 图片回传 | 采纳真实 Renderer 和实际图片读取 | 资源缺失 partial、无引擎失败、源变化失效、预览不触发业务动作 |
| §6.4/§7.5 台账与幂等 | 采纳持久 handle 和业务操作键；状态独立 | 两次同内容不同操作成功；同键异参拒绝；丢回包不重发；FINISH 保留 unknown |
| §6.6 事件归因 | 加强身份及实例交叉校验 | 同群两卡同名动作不串流；伪造 context 身份拒绝；ready 前无发送 |
| §8 Template/Capability | P2 独立轨道，补齐作者与业务调用边界 | 业务数据里的 path 不误提取；未审核不发布；Input 注入组件拒绝 |
| §6.1 结果与退出码 | 适配当前 DWS envelope/Result 标准 | compact/full Result 一致；不新增 unknown outcome；测试统一错误映射 |
| §6.2 自动 repair | 采用显式策略、可审计确定性修复 | 默认不改输入；规则版本、摘要可追踪；修复后重新校验 |

本轮交付状态：方案、实现、定向测试、本地 reference preview 与可执行验收矩阵已同批落地；实际状态以 §26、§29 和 §30 为准。新增测试落在对应模块的 `*_test.go`；安装/Harness/真实环境用例仍须进入受控集成验收，避免普通 Go 测试出网。

## 25. 可执行验收合同（整体评审后的交付门禁）

本节把 §22 每项需求落到固定用例；与概述发生歧义时，以本节的范围、输入、断言和证据要求为验收依据。其中 source 层、reference preview、Gallery 与本地交互脚本已实施，实际执行结果见 §30；尚未接入的 Harness、真实 Renderer、安装包和端侧用例仍是交付合同，不能因本文存在就认定 PASS。测试实现须与对应功能同批交付。

### 25.1 运行记录与结果判定

每条 case 保存：caseId、requirementRefs、phase、status、源码 commit 与 dirty diff 摘要、安装包版本/摘要、CLI 和 Runtime 实际路径/版本、Bundle/Renderer/资产摘要、平台/视口、输入 fixture 摘要、执行命令、开始结束时间、断言结果、脱敏证据路径。线上用例另存目标 scope、handle、bizId、surfaceId、request trace；不得保存凭据。

- NOT_RUN：还未执行；FAIL：执行后不满足断言；BLOCKED：必要外部条件缺失，并列恢复动作。
- PASS：该 case 全部断言满足，证据与本次输入/构建一致。缺少证据即不能 PASS。
- NOT_IN_SCOPE：只用于预先明确的 P2 或未启用 append 正向能力；不得动态跳过一期失败项。客户平台未验证则明确限制支持范围，不能默认为通过。
- 人工视觉检查也必须绑定 caseId、实际图像和逐项判断。FAIL 修复后重跑受影响项；不得仅更新 Golden 让 diff 消失。
- 源码测试、标准安装、预览、远端受理、送达、端侧交互分别出结果。上游通过不替代下游通过；IPC 模拟实验不替代原生 DWS。

建议报告落点为 `artifacts/card-acceptance/<run-id>/`，包括 manifest、cases、screenshots、trace、summary；用例与期望值放在受评审 testdata，运行报告不作为新的协议/Schema 权威。业务敏感输入使用合成夹具，真实目标配置从验收环境读取。

### 25.2 功能验收目录

表中“零调用”通过传输 spy/假服务请求日志断言，“最终状态”通过独立参考状态应用器及端侧回读断言；不能用被测 diff 函数计算自己的期望结果。

| ID / 阶段 | 设计与测试落点 | 操作/故障注入 | 通过标准与证据 |
|---|---|---|---|
| AC01 / 0A | 协议包；protocol 测试 | 离线加载；改任一字节、缺 common、外部 ref、循环 ref、锁冲突、缺锁定版本 | 正例成功；反例按稳定 code 拒绝；网络请求为零；默认升级不改旧锁；保存诊断与摘要 |
| AC02 / 0A | 发现；catalog/semantic 测试 | 无登录、无源码、无专用 Skill 查询 list/search/get 三视图并遍历分页 | 安装 manifest 中所有组件可查；14 个一期组件无缺项；radio 别名推荐 ChoicePicker；默认闭包完整；compact/full 同名事实一致；不启动 Runtime |
| AC03 / 0A | Runtime；runtimeclient/ipc 集成 | 20 个并发冷启动请求；版本错、损坏二进制、进程忙、取消、断连、drain | 单安装作用域只启动一个有效实例；任务 ID 不串；版本错不执行；进度/错误不污染结果；无订阅与子进程泄漏；记录 PID/时间线 |
| AC04 / 0A | 授权；hostbroker 测试与真实适配 | 正常请求、换目标/组织、伪造 contextRef、重连旧授权、路径越界、输入校验后变更 | 正常路径成功；越界请求零外部调用；断连旧授权失效；按固定快照执行或拒绝；CLI 与 Runtime 不暴露凭据 |
| AC05 / 0 | 输入归一；normalizer 测试 | 同一数据的对象数组/JSONL/旧 wire；截断 JSON、空数组、深度/体积越限、20KB 文件与 stdin | 正例规范结果相同；负例定位原行和 Pointer；新命令 argv/审计不含正文；限额边界 -1/等于/+1 全覆盖 |
| AC06 / 0 | 五层协议校验；protocol/lint 测试 | 未知消息/组件/字段、类型/枚举错误、重复 ID、环、悬空 child、初值缺失、错误动作槽与函数 | 正例通过；每类负例有稳定 code/phase/source/Pointer，RPC 与渲染调用为零；delta 用基线后状态校验；跨批同 ID upsert 合法 |
| AC07 / 0 | 命令迁移；helpers/app 测试 | 新旧命令用合法等价输入，覆盖全部目标/flowStatus/注解；参数互斥和确认拒绝 | 归一 RPC 等价（仅排除明确随机字段）；确认失败零调用；旧路径可执行且推荐新路径；未知字段拒绝是明确兼容变化，不能假称旧非法输入兼容 |
| AC08 / 0 | 框架；Schema/Help/Result 门禁 | 枚举所有 public leaf、查询所有声明身份/alias、跑例子约束和 dry-run | 双向覆盖；完整 GroupPolicy；Result compact/full 一致；统一 envelope/退出码；dry-run 零网络写、零发送台账操作记录；无退休 Catalog 文件 |
| AC09 / 1A | 模板转换；assets/composition | 对每个迁移模板冻结源版本与图，转换 create/update，再渲染 | 来源/映射/支持差异齐全；信息与操作意图等价；更新同 Surface；无平台 DSL/私有属性；三类样板先通过，最终七 Recipe 全覆盖 |
| AC10 / 1A | Block/Recipe 编译；composition 测试 | 所有 Slot 正例/缺值/越限；重复 key、重排/插入区块；混用输入分支；raw 输入 | 15 Block/7 Recipe 全部展开可校验；同输入同摘要；插入不改其他输入控件 ID；冲突输入拒绝；raw 不绕过校验；无第二个提交按钮 |
| AC11 / 1A | 规划推荐；guide/semantic | 通知、长报告、列表、表单的正反例，事实 ID 映射；无合适 Recipe | 候选至多 3 个且均满足前提，解释匹配与容量；关键事实全保留；不适合时推荐 Block/raw 或明确限制，不硬套模板；不调用模型 |
| AC12 / 1A–1B | 视觉规范；design/preview | 执行 §25.3 资产矩阵，注入截断、核心折叠、双主操作、资源失败 | 可判定错误全拒绝；其余 warning 有审阅；所有样板无内容遮挡/丢失；主次层级与模板规范一致；原图与检查记录齐全 |
| AC13 / 1B | 真实预览；preview/Harness | 无 Renderer、超时、字体/图失败、源改动、图片出口失败、按钮/导航动作 | 无 Renderer 不产成功图；缺资源 partial；新摘要不复用旧图；预览副作用为零；图像内容实际可被 Harness 读取；超时回收资源 |
| AC14 / 0–1B | 发送与回读；delivery/evidence | PRE 群和单聊；目标 0/1/多命中；受理后无消息/多候选；只看 FINISH | 唯一目标才发送；受理仅 accepted_unverified；可信关联才 delivered；不猜 messageId；FINISH 不覆盖 unknown；提供回读与客户端证据 |
| AC15 / 0 | 台账/幂等；ledger 测试 | 同键同参/异参、同内容不同操作；出网前/后各点崩溃、损坏记录、回包丢失 | 同操作不新建；异参冲突；不同操作可各建一次；写前失败零请求；出网后 unknown 保留原键并不重发；损坏显式报告不静默清空 |
| AC16 / 0 | 续操作；receipt/runtime 集成 | 关闭发送 shell；新 shell 仅 handle+身份 update/finish/verify；回执冲突/跨组织/版本错 | 同 bizId/surface；正确读取真实目标；scope/revision 错零调用；receipt 与台账不形成双写；仅改 flowStatus 合法；纯 no-op 零请求 |
| AC17 / 1B | 精确增量；delivery/composition | 连续 100 批累计正文、绑定按钮改文案、字面量按钮改文案、新增区块 | 一次 create，后续无 createSurface；最终字符串逐字一致；未改组件不出现在增量；组件完整定义保留 Action；输入路径与控件 ID 不变 |
| AC18 / 1B | snapshot 差异；composition | 已确认 S0 与期望 S1；新增/重排组件、数据缺字段、非法删除、未知基线 | 独立应用生成 delta 后 Agent 管理状态等于 S1 的明确赋值结果；用户路径不变；零差异零 RPC；不支持删除拒绝；不重发完整 create 序列 |
| AC19 / 1B | 顺序与背压；delivery/runtime | 假时钟/慢服务、重复提交、晚到回包、断线、持续输入、最终 FINISH | 每 handle 最多一个在途；待发送快照合并且最终内容不丢；有界队列；旧回包不推进新 revision；unknown 停止依赖更新；flush 先于 FINISH，取消不声称撤回 |
| AC20 / 1B 条件能力 | append；protocol/delivery | 未支持扩展、无 opId 去重、重复 DELTA、支持配置下顺序追加与完整检查点 | 未支持时拒绝/选路径快照且不发 append；仅有实证能力才开放；重复不导致重复文字或明确停止；检查点恢复前核对晚到风险；不以 requestId 代替去重 |
| AC21 / 1B | 事件；events/Host | ready 延迟/失败、同群两卡同名 Action、重复 eventId、伪造 actor、无归因标识 | ready 前零 send；两卡不串；重复只处理一次；伪造权限拒绝；未归因留存；停止后无订阅泄漏 |
| AC22 / 1B | 真实交互；端侧验收 | TextField 输入、单选/多选、checks 失败、提交、重复点击、业务失败、重试、过期；过程中流式更新 | 提交 context 是最新真实输入；checks 阻止非法提交；业务成功后同卡反馈；失败可理解；输入不被正文更新覆盖；本地函数与 Agent 回流分别取证 |
| AC23 / 1B | Agent 效果；Harness 评测 | 按 §25.4 仅给自然语言任务和工具契约；注入视觉错误 | 初稿/修订/工具调用可追踪；无字段幻觉及事实丢失；看到实际图片；三轮预算内成功或明确降级/未完成；不无授权发送 |
| AC24 / 各阶段 | doctor/诊断/审计 | 分别让 auth、组织、IM、event bus、Bundle、Runtime、Renderer 失败 | 分层报告实际失败项与下一步，unknown 不当作 false/true；doctor 无发送副作用；日志无凭据/完整协议；trace 可关联台账 |
| AC25 / 发布前 | 安装/升级/回滚；发行集成 | 冻结包标准安装，隐藏源码/开发依赖；升级后回滚默认 Bundle/资产，保留旧实例 | 一次 DWS 安装含全部必要组件；静态离线可用；旧锁/实例可续操作；不兼容明确拒绝；被禁用资产不可新选；不修改既有实例版本 |
| AC26 / 发布前 | 覆盖与报告；验收 runner | 漏 case、空 evidence、unknown ID、fixture 改动后复用旧报告、仅跑部分场景 | 聚合报告拒绝判完成；每项一期需求、命令、资产都有对应证据；计数变化需评审；支持范围与实际平台矩阵一致 |
| AC27 / P2 | 主题；compiler/assets | 全部主题的明暗与状态组合、未知 token、目标不支持 token | 无新协议字段；语义状态色一致；降级确定；主题变化不改数据/Action；Golden 与规则评审通过 |
| AC28 / P2 | 资产与 Capability；服务集成 | 类型化提取、业务数据 path、冲突绑定、unresolved、草案修改、发布中断、恶意 Input/Update | 不误提取；未审核不可发布；旧审核失效；版本一致激活；Input 白名单；实例固定版本；业务事件可信归因；真实调用及更新通过 |

### 25.3 组件、区块与视觉样本覆盖

资产清单直接从实际 Registry 取值，与固定一期清单双向比对：14 个组件、15 个 Block、7 个 Recipe。新增项必须补用例，少一项失败；协议包总组件数取冻结 manifest，不把旧附件的 47 写死为所有未来版本的上限。

每个 Recipe 与每个可独立预览 Block 至少运行：默认短内容、长内容、可选字段缺失、中英文混排四组 fixture × 320/480 两个逻辑像素宽度 × light/dark，共 16 个基线组合。必填字段缺失另外作为负例；图片、文件、表单等按实际依赖附资源失败和交互样例。所有组件至少有一个合法展示/交互宿主 fixture 和各 required/类型/枚举代表性负例。

长内容 fixture 固定为标题 80 个汉字、正文 2000 字、120 字连续英文 token、20 个列表项、带代码块的 Markdown；只对相关 Slot 应用。超出资产声明容量时可明确拒绝或按已声明策略分组/折叠，但完整内容必须可达，禁止静默删减。已展开卡片边界内不得有非预期横向溢出、覆盖或截断；滚动/折叠等预期行为在 fixture 中声明。

视觉审阅逐项记录：标题/结论可见、辅助信息层级、区块间距、文字可读、主次操作、资源降级、长内容去向。控件标签可读、焦点和键盘行为按支持端验证；颜色对比按项目批准的可访问性基线测量，无法测量不标 PASS。设计规则数值、截图 diff 容差和忽略区域必须预先版本化；任何 Golden 变更需说明用户可见差异。

端侧最低验收为明确版本的 PC、iOS、Android，七 Recipe 在每个端各完成基线展示与一次同实例更新；approval 的输入/单选/多选/提交及错误反馈三端必验。File 打开、Link 跳转、CollapsiblePanel 展开在其 Recipe 中覆盖。缺少设备即对应项 BLOCKED，并限制交付声明。CLI 安装平台矩阵取发行清单，Windows Pipe 不能用 macOS 的证据替代。

### 25.4 Agent 评测与美观判定

冻结 21 个任务：七 Recipe 各一个短内容、长内容、边界/缺信息任务。期望答案和事实 ID 仅留在评测器，模型只收到任务与正常工具结果；另补三个视觉故障诊断任务（截断、核心结论折叠、错误操作层级）。固定 Harness/模型/版本/采样参数，每项运行三次并保留初稿与修订记录，不从多次输出中只挑成功样本。

确定性安全项要求所有试次通过：无未授权写入、关键事实不丢失、协议错误不进入发送、更新同一实例。正常且输入充分的任务，在最多三轮修订后可交付率至少 95%；不充分任务必须明确缺口或生成标注的草案，不能虚构事实。该比例是拟定发行门槛，不是当前实测成绩。首次通过率、token、查询数与修订次数单独报告，不能拿确定性关键词匹配替代模型效果评测。

视觉质量由实际图片的逐项审阅判定，禁止只用模型自评或数字美观分。三类首批样板的基准图需设计/产品评审者确认；其余资产按同一规范验收。评审者需指出具体截图/组件位置与结果；此人工环节不能通过更新截图基线自动消除。没有人工确认时视觉审批项保持 NOT_RUN/BLOCKED。

### 25.5 实施时必须交付的测试入口

1. **源码层**：对应包的 `*_test.go` 与版本化 testdata，执行 §19.2 命令及反向 Schema 完整性、确认真值、Agent 示例门禁；覆盖新增 cmd/dws-card 的构建。普通测试使用 Fake Broker/Clock，禁止访问真实 IM。
2. **原生进程层**：拟新增 `scripts/card/acceptance.sh --layer runtime --binary <absolute-path> --out <dir>`，实际启动 DWS/Runtime，验证 AC03/04/16/19，隔离端点与台账。
3. **渲染层**：同一入口 `--layer preview` 生成资产矩阵图片、诊断及审阅清单；记录真实 Renderer，禁止依赖开发机全局浏览器代替发行组件。
4. **Agent 层**：`--layer agent` 接目标 Harness 与固定评测集；模型/费用配置显式提供。普通 CI 不调用模型，发行验收不能省略此层。
5. **真实环境层**：`--layer live --config <approved-targets-file>`，配置明确 profile、环境、群/单聊与最大卡片数；先展示计划，按本轮发送授权执行，保存真实回读与端侧证据。不得复用历史对话中的群 ID 作为默认目标。
6. **汇总层**：`--layer report` 验证需求→case→执行→证据全部可追踪，任何一期 NOT_RUN/FAIL/BLOCKED 均不能输出整体 PASS。P2 单独汇总。

当前 `scripts/card/acceptance.sh`、`render-recipe-gallery.sh` 与本地交互检查已实施；未实施的 Agent Harness、真实 Renderer、安装包及端侧 runner 仍不得作为已存在的命令宣传。源码阶段可用测试二进制，但发布前必须用冻结版本的标准安装包重跑受影响集成与真实路径，记录实际 executable，不能沿用源码通过记录冒充包验收。

### 25.6 需求覆盖与阶段退出

| §22 需求组 | 必须关联的 case |
|---|---|
| 精确协议、生成上下文闭包与锁文件、一期组件 | AC01/02/05/06 |
| 使用语义、合理组合、内容保真与自适应 | AC10/11/12/23 |
| 内置模板、平台模板转换、常用区块 | AC09/10/12 |
| 默认视觉规范、美观效果、有界修订、图片进入模型 | AC12/13/23 |
| 命令迁移与机器合同 | AC07/08 |
| 同实例更新、持久台账与回读 | AC14/15/16 |
| 流式分批、snapshot、检查点与输入保护 | AC17/18/19/20/22 |
| 可靠交互、监听与归因 | AC21/22 |
| 受管进程与授权 | AC03/04/24/25 |
| P2 主题、业务资产复用 | AC27/28 |
| 全局发行、回滚、覆盖完整性 | AC24/25/26 |

命令维度还需双向覆盖：catalog/protocol→AC01/02，block/recipe/guide/compose→AC09/10/11，lint/build→AC05/06，preview→AC12/13，send→AC07/14/15，update/finish→AC16–20，verify/list/show→AC14–16，watch→AC21/22，doctor→AC24，runtime→AC03/04。任何新增公开 leaf 无 case 关联则 AC26 失败；P2 命令未实现时不注册占位 leaf。

Phase 0A 先通过 AC01–04；Phase 0 增加 AC05–08、14 的传输部分、15/16；Phase 1A 增加 AC09–12 静态与内容部分；Phase 1B 完成全部一期 case 的渲染、Agent、端侧及流式部分。跨阶段 case 分子用例记录，不能提前把整个 case 标 PASS。AC25/26 是一期发行前必过门禁。

本次整体评审修正：统一 catalog 三视图；明确增量按基线后状态校验；修复 status-only FINISH；补稳定 Block key 和角色映射；将 snapshot 增量编译明确纳入一期；补 information 真实用例；消除可靠交互延至 P2 的范围矛盾；完善状态与证据判定。实际实施状态以 §26 和对应验收报告为准，不能从设计文档推断通过。

## 26. 2026-09-15 首批实施状态

本节是当前工作树的实施快照，不是完成声明；最终状态以 §22、§29 和 §30 为准。本批已内嵌 A2UI Catalog/Common Types，完成纯 Go 协议加载、消息归一化、组件 Schema、根/引用/环/孤儿、初始绑定和部分设计规则校验；实现 14 类一期组件语义、`RadioButton` 概念映射、15 个 Block、7 个 Recipe、CompositionSpec 编译、确定性 wire、Surface reducer、snapshot diff 和 reference preview。公开命令已落到 `dws card`，包含 protocol/catalog/block/recipe/guide/compose/build/lint/preview/send/update/finish/verify/list/show/doctor；写命令要求显式 profile，send/update 通过 IM MCP，本地 handle 台账记录 bizId、surfaceId、目标、revision 和 flowStatus。

上述“已实现”仍不等于完整生产就绪。先前识别的 profile/组织/环境作用域、handle 串行化与本地 CAS、严格 snapshot 解码、输入绑定保护、写操作确认、降级 pattern 语义检查和非空 `updateComponents.components` 均已在源码与自动化测试中关闭。2026-09-17 的最终复核又补齐：真实 `corpId:userId` 绑定、死亡进程锁恢复、全部公开输入控件绑定保护、数组 JSON Pointer、Snapshot 删除拒绝、增量合并后完整 Surface 复验、有效 Recipe 回执，以及 `task/schedule` 的真实 `CardHeader` 编译。仍未关闭的是协议资产公开来源/许可证、卡片平台十套来源合法化、服务端 readback/远端 CAS 和真实 PC/iOS/Android 视觉与 Action 回流，详见 §27–§30。

快速反馈入口为：

```bash
scripts/card/acceptance.sh --layer source --out <dir>
scripts/card/acceptance.sh --layer preview --binary <absolute-dws-path> --out <dir>
```

`preview` 当前明确标识为 `reference_preview` / `structural`，不是钉钉同源 Renderer，也不产出真实客户端视觉 PASS。`verify` 当前只验证 `local_ledger` 一致性，并显式返回 `deliveryVerified=false`；服务端消息回读、客户端渲染、Action 事件回流、受管独立 Runtime/Host Broker、真实 Renderer 图片矩阵、Agent 21 题评测和正式发行安装仍未在本批完成，相关 AC 保持 NOT_RUN 或在缺少外部能力时标为 BLOCKED。旧 `chat message send-a2ui-card/update-a2ui-card` 路径仅作兼容入口并已 deprecated；Agent 默认应使用具备协议校验、同身份台账和合并后 Surface 复验的 `card send/update/finish`。两条路径尚未共享同一 Delivery Service，因此不能把兼容命令仍可发送当作 AC07 已通过。

当前 `source` 层 A2UI 定向测试、`preview` acceptance、7 Recipe Gallery、Schema/generated policy 与 command surface 均已通过；preview 收据明确标为 `reference_preview`，不能替代钉钉客户端证据。仓库全量测试仍存在 A2UI 范围外的 JSONML 编译失败，且 merge-base `8cacb019` 同工具链可复现，因此记录为基线阻断而非本分支回归。

## 27. 卡片平台 CLI 十套内置模板逐项还原审计（2026-09-16）

### 27.1 权威基线与证据边界

公共仓库当前没有一份满足来源、许可证、版本和可复现要求的卡片平台 CLI 十套模板视觉基线。此前人工提供的 HTML 与双栏审阅页只能作为探索性输入：它们没有进入本仓库、没有公共来源或发布收据，也没有固定 Renderer、逻辑宽度、DPR、主题、语言和逐图 hash，因此不能作为合并门禁、Golden 或 PASS 证据。本文只保留从该次审阅中得到的结构映射和能力差距，不保存内部仓库地址、个人工作区路径或临时输出路径。

若后续要把左侧参考纳入可重复验收，必须先提供可公开消费且许可证明确的来源包，并在本仓库增加 reviewed manifest。manifest 至少固定 `sourceName/sourceVersion/sourceArtifactHash/recipeVersion/imageHash/frame/DPR/theme/locale/rendererVersion/license`；双栏生成器只消费 manifest 与仓库内 fixture，输出到调用者指定的 artifact 目录，运行产物不提交，也不在技术文档中记录机器本地路径。

当前仓库内的 `scripts/card/recipe-gallery.html` 是 **DWS 自身 7 个 A2UI Recipe 的索引壳**，不是卡片平台十套模板的权威左侧基线；`scripts/card/render-recipe-gallery.sh` 负责从受评审 CompositionSpec 生成七张 reference preview，并输出 lint/compose/preview 收据。两类资产必须分开命名和验收，不能再用 DWS Gallery 证明跨产品 1:1 还原。

### 27.2 十套模板到 A2UI 的逐区映射

保持“7 个高频语义 Recipe”和“10 个来源模板 fixture”两个层次：来源模板数量不能反向迫使公共 Recipe 一一膨胀；同一 Recipe 可由不同 Slot/Block 组合形成不同来源模板。只有结构长期稳定、选择语义独特或交互合同不同，才新增公共 Recipe。

| 来源模板 | A2UI 组织方式 | 本轮已保留 | 仍需真实 Renderer/能力补齐 |
|---|---|---|---|
| `information` | `Card` + 彩色 masthead + 内容 Column + `Tag` + header `Image` + 摘要 + 主 `Button` | 标题、来源、时间、标签、封面、核心摘要、详情操作及顺序 | 图片裁切、蓝色头部高度、字体度量和按钮高度需同宽截图比对 |
| `schedule` | `CardHeader` + 状态 Tag + 会议事实 Column + 主 `Button` | 完整头部、主题、时间、地点、会议号、人数、详情 | 图标接线完成前采用纯文本事实行；整卡宽度仍需宿主 Surface 落实 360px 下限 |
| `task-reminder` | `CardHeader` + 状态 Tag + 主标题 + 有值事实 + 主操作 + 来源 Link | 完整头部、任务、日期、详情、来源；空备注不占位 | label 固定轨道与链接前缀图标属于后续协议增强；整卡宽度仍需宿主 Surface 落实 360px 下限 |
| `approval` | 标题/标签 + 事实列表 + 创建时间 + 双操作 + 两种禁用终态 | 报销事实、拒绝/同意事件、终态结构 | `Button.disabled` 当前协议只保证拦截、不保证端上变灰；必须以业务状态切换完整组件定义并三端验收 |
| `document-permission-request` | 标题 + surfaced 申请区 + 权限状态 + 原因/风险提示 + 详情/双操作 | 信息层级、风险提示、权限值、操作意图 | 下拉权限选择必须用真实 `ChoicePicker` 绑定；外部协作者安全提示与业务权限服务联调 |
| `reply-comment` | 文档标题 + surfaced 评论区 + Markdown + 双操作 | 评论者、上下文、@ 文案、查看/回复 | 卡片 Markdown 的 `atIdList`、逐链接/粗粒度点击边界及回复事件回流需真实验收 |
| `profile` | header `Image` + avatar `Image` + 身份/任职事实 + 双操作 | 封面、头像、岗位、邮箱、电话、司龄、工号、查看/联系 | 参考中的头像压住封面属于 Stack/负位移能力；一期 Row/Column 无等价重叠布局，当前只能语义等价，不能声称像素等价 |
| `rich-media` | header `Image` + `image-text-item`（居中 Row + 权重文字列 + smallFeature Image）+ 主操作 | 主图、栏目、标题、摘要、缩略图、入口 | 公开 `smallFeature=96×96np`；若真实端外层仍占旧 200np 高度，按 Block 合同退化为纯文本项，不得用私有尺寸字段硬调 |
| `form` | TextField + dropdown/checkbox ChoicePicker + 提交 Button | 文本、单选、多选、绑定路径和统一提交 | 一期优先组件没有 DatePicker/DateTimePicker；用 TextField 只能保留字段意图，不能冒充原生日期交互；多选 DataModel 数组与本地模拟器也要补全 |
| `ai-streaming` | 状态 Text + Markdown 内容 + Divider + 进度/反馈 action Row | done 状态内容、生成进度说明、复制/赞/踩动作槽 | pending/writing/done 必须用同 Surface 稳定 ID 增量更新；`textEffect=shimmer` 仅部分端生效；反馈图标及回流需端上验收 |

### 27.3 本轮暴露的当前实现缺口

1. **来源资产未版本化。** 现有 A2UI 画廊只覆盖 7 个抽象 Recipe，未绑定卡片平台 10 个来源模板的 hash、组件区域和差异清单，导致之前左图取错仍能“通过”。
2. **CompositionSpec 表达力不足。** 当前 `Spec` 以固定 `title/subtitle/body/image/metrics/highlights/file/details/actions/options` 为主，无法表达彩色 masthead、任意事实行、两种图片尺寸、评论/权限 surfaced 区、独立终态、多个异构表单字段和流式反馈栏。Agent 被迫把结构压进 Markdown，视觉层次随即丢失。
3. **Recipe 与 fixture 混淆。** 来源模板应是评测 fixture/转换样板，公共 Recipe 是长期稳定的信息与交互模式。不能因为来源有 10 张就注册 10 个近义 Recipe，也不能用 7 个通用示例替代来源模板逐项验收。
4. **reference renderer 丢公开字段。** 修正前没有投影 Row/Column/Card 的 `backgroundColor`、`borderWidth`、`borderColor`，Image 的 `icon/avatar/smallFeature/mediumFeature/largeFeature/header` 也都画成统一 200px 高，导致合法 A2UI 的本地效果被错误压平。本轮已在通用渲染层补上安全的 box-model inline projection、A2UI `#AARRGGBB` 到 CSS `#RRGGBBAA` 转换和公开 Image.variant 几何；没有按模板 ID 注入 CSS。
5. **图标合同未闭环。** 来源中的日程、地点、会议、人员、文档、评论、电话和反馈图标必须映射到协议公开 `Text.icon`/`Icon` 以及真实图标目录。参考页临时字形只用于暴露位置，不能进入版本化 fixture 或发布 Recipe。
6. **表单控件族不足。** `TextField + ChoicePicker` 能覆盖本期指定组件，但无法一比一表达来源表单的日期、日期时间和复选框单控件。若一期范围不扩展组件，验收结论只能是“字段与提交语义等价”，不是“交互一比一”。
7. **状态与交互视觉不能靠 CSS 推断。** `Button.disabled` 的公开说明明确指出端上可能不改变视觉；终态应由 update 下发明确的按钮/状态组件定义。所有操作还要验证 event name、最新 DataModel context、重复点击、失败、过期和同实例反馈。
8. **缺少可比较截图合同。** 当前右图是 reference renderer HTML，左图是另一 Renderer 的 PNG；未绑定同一逻辑 frame、DPR、字体、图片加载结果和 Renderer 版本，因此只能做逐区结构审阅，不能执行像素阈值或宣称 1:1 PASS。

### 27.4 后续统一优化设计

#### P0：先让“还原”可验证

- 新增 `card template parity` 的只读/本地验收资产层，输入为来源 manifest + 10 个来源 PNG + 对应 A2UI fixture；输出每个视觉区的 `exact / semantic / composed / degraded / blocked` 映射、lint 收据、预览收据和差异报告。
- 来源 manifest 固定 `sourceRepo/sourceCommit/sourceArtifactHash/recipeVersion/imageHash/frame/DPR/theme/locale/rendererVersion`；任一项变化使旧视觉证据失效。
- 为 10 个 fixture 固定区域清单，区域缺失直接失败，不能用整图相似度掩盖：masthead、标题、元数据、主图、摘要、事实行、内容面、输入区、状态、操作区、footer。
- 双栏页必须直接消费 manifest，不再手工选择左图；`data-recipe` 与右侧 fixture 一对一，缺一项或多一项均失败。
- 状态枚举使用 `NOT_RUN/PASS/FAIL/BLOCKED/NOT_COMPARABLE`。协议 lint 通过只证明结构合法；reference preview 通过只证明本地可读；真实 PC/iOS/Android 才能关闭视觉与交互项。

#### P1：提升 Agent 可组合表达力

- 把 CompositionSpec 从“固定长表单”升级为 `InformationPlan + ordered blocks`：每个事实带稳定 `factId`、优先级、来源、目标 Block、是否可折叠和超限策略；编译回执返回 fact→component ID 映射。
- 引入类型化 Slot，而不是开放任意组件 JSON：`masthead`、`factRows`、`media`（header/avatar/feature）、`contentSurface`、`commentContext`、`permissionRequest`、`formFields`、`streamStatus`、`feedbackActions`、`terminalActions`。
- 公共 Recipe 仍保持少而稳定；将 10 个来源模板实现为由 Slot/Block 组合生成的受评审样板。只有 schedule、approval、form、ai-streaming 等确有独特交互合同的模式才拥有专门编译策略。
- 编译器统一 4/8/12/16px 节奏：同组 8、相关组 12、章节 16；gap 归后继关系，容器 padding 只表达边界；根卡片选择 padding 0 时必须由每个一级内容区显式拥有 12/16 inset，避免全幅 masthead/hero 与正文错位。
- 图片 Slot 声明语义 variant、裁切、alt、预览开关和失败降级；公开 variant 无法满足 64px 缩略图或头像叠层时返回可审阅降级，不注入私有宽高/负 margin。
- 表单字段按真实控件能力建模；Date/DateTime 不得静默降级成普通 TextField。若组件仍不在一期支持集，guide 必须返回限制和替代交互，而不是生成外观相似但行为错误的控件。
- 流式卡固定 Surface 与组件 ID：pending 创建稳定骨架，writing 只更新路径快照/必要状态组件，done 更新终态与反馈区；输入控件绑定路径永不被正文批次覆盖。

#### P1：补齐 reference renderer，但不把它做成私有客户端

- 只实现 Catalog 已公开且可确定模拟的视觉字段；每项都有协议夹具和 DOM 断言。禁止按 Recipe/template ID 写 CSS。
- 补 `Text.icon`/`Icon` 的目录驱动预览、ChoicePicker 数组选择与 dropdown/chips 形态、Text 自定义明暗色、Divider 宽度/颜色、Image 加载/crop 证据；未知 token/图标显示明确占位和诊断。2026-09-16 已完成第一批图标、文字颜色/字号与按钮视觉 token；ChoicePicker 形态、Divider 完整投影及未知项诊断仍按本项继续推进。
- reference renderer 输出 manifest，记录协议 bundle、输入摘要、逻辑宽度、主题、资源成功/失败和模拟交互范围。它继续标识 `reference_preview`，不得升级为客户端视觉证据。

#### P2：真实端像素与交互收口

- 使用同一运行数据在 PC/iOS/Android 截图；按区域做几何/像素 diff，图片和字体异步稳定后再取证。只对同 Renderer/version/frame/DPR/theme/locale 的证据启用像素阈值。
- 对所有按钮、链接、选择器、输入和流式反馈执行真实事件回流：最新表单值、幂等、失败恢复、过期、权限、同实例 update 和最终状态回读。
- 视觉差异由设计/产品逐项确认；协议本身无法表达的部分要么扩展公开协议并完成端实现，要么保留 `degraded/blocked`，不能通过修改 Golden 消除。

### 27.5 本轮与后续验收矩阵

| 项目 | 当前结果 | 证据/通过标准 |
|---|---|---|
| 来源模板枚举 | BLOCKED | 公共仓库内缺少许可证明确、版本固定且可复现的十套来源 manifest |
| 左图真实性 | BLOCKED | 未提交公共来源资产、逐图 hash、Renderer/version/frame/DPR/theme/locale 收据 |
| 十套 A2UI 结构合法 | NOT_RUN（可复现门禁） | 探索性运行结果不进入最终 PASS；需将十套 fixture 与来源 manifest 纳入仓库命令后重跑 |
| box model/Image variant 本地投影 | PASS（源码层） | `go test ./internal/card/a2ui/preview -count=1`；新增测试覆盖透明根 inset、颜色/边框、8 位色转换、header/avatar variant |
| 内容/层级/操作意图 | PARTIAL | 已留下逐区人工映射；尚未固化为机器 manifest 和事实→组件断言 |
| 与卡片平台参考图像素 1:1 | NOT_COMPARABLE | 两侧 Renderer/字体/图片稳定条件未绑定；profile 重叠、form 日期控件等存在公开能力差异 |
| 真实钉钉视觉 | NOT_RUN | 需要冻结版本的 PC/iOS/Android 同输入截图 |
| 真实交互与流式更新 | NOT_RUN | 需要 Action 回流、同实例 update、三态流式和最新 DataModel 证据 |

因此，本轮只能确认“结构映射已记录、reference renderer 的部分通用失真根因已有源码修复”；不能确认来源基线已经公共化、十套转换已进入可复现门禁，更不能确认真实客户端像素一比一。后续必须先完成 P0 比较合同，再按 P1 解决协议/Composition 表达缺口，最后以 P2 真实端证据关闭 1:1 与可靠交互验收。

### 27.6 图标、文字与按钮视觉 token 收口（2026-09-16）

本轮遵循“消息协议能力”和“reference renderer 外观默认值”分权，不把本地 CSS 能做到的事情伪装成 Agent 可下发能力：

| 视觉项 | 协议事实 | 本轮决策 | 约束与降级 |
|---|---|---|---|
| 图标 | `Text.icon` 支持注册名称或明暗图片 URL；独立 `Icon.name` 仅允许公共 `IconName` 清单 | 同一套 1.8px 圆角描边 SVG；正文前置图标 16px、caption 14px、标题 18px、独立 Icon 20px，文字间距 6px；图标继承语义文字色 | 不再用 Unicode/Emoji 冒充系统图标；未知注册名称在本地先用中性时钟形占位，真实图形仍由宿主图标目录决定，后续补显式诊断 |
| 文字颜色 | `colorToken` > `customLightColor/customDarkColor` > `color`，具体 token 色值由宿主主题解析 | reference renderer 为一级/二级/三级/禁用、链接/主题、成功/警告/危险 token 提供稳定明暗近似值；完整支持 8 个 `color` 档位和自定义明暗色切换 | 本地色值是可读性近似，不是宿主 token 的权威 RGB；消息 fixture 优先写语义 token，不把十六进制值散落到模板 |
| 字号 | `sizeToken` 优先于 `variant`，宿主负责原生字号/行高 | 本地映射公共说明列出的 10 个原生字号 token；标题/正文/动作/说明形成 22/20/18/16/15/13/12/11px 可辨层级 | 本地 px 只服务快速结构审阅；真实多端字体度量必须截图验收 |
| 按钮圆角 | 公共 `Button` 没有 `cornerRadius`；只有 `default/primary/borderless` 外观提示 | reference renderer 统一使用胶囊圆角 `999px`，44px 最小触控高度，保留 primary/default/borderless 的 hover、focus、active、disabled 状态 | Agent 不得生成不存在的按钮圆角字段；若未来要求业务可选圆角，必须先扩公开协议和端实现 |

排版规则沿用 4/8/12/16px 节奏：图标与同一行文字属于紧密组使用 6px 光学间距；按钮是“小控件”因此可使用胶囊形，Card 与内容 Surface 仍保持 12–16px 圆角，不扩散为全局药丸化。主要文字使用 `common_level1_base_color`，元数据使用 `common_level2_base_color`，弱说明使用 `common_level3_base_color`，交互强调使用 `common_link_color` 或主题主色；颜色不得只为“好看”而脱离成功、警告、危险、链接等语义。

十套来源 fixture 已移除 `◷/⌖/⊞/♙/♟/▣/◌/ⓘ/⌕/↗/▢/♧/♤/✓/×` 等替代字形，改用 `Text.icon`。日程事实、文档/评论上下文、审批动作、个人资料操作、外链和流式反馈均复用同一图标渲染路径；按钮标签使用 `common_action_text_style__font_size`，主按钮标签用白色 token，普通按钮用一级文字 token。

本轮源码验收：

| 验收项 | 结果 | 证据与通过标准 |
|---|---|---|
| 图标尺寸与统一风格 | PASS（reference renderer） | `TestRenderProjectsTextIconColorTypographyAndButtonShape` 断言 SVG 图标、14/16/18/20px 尺寸规则及无 Unicode 图标替代 |
| 文字 token 与明暗色 | PASS（reference renderer） | 同一测试断言 `colorToken`、`sizeToken`、`customLightColor/customDarkColor` 投影；`go test ./internal/card/a2ui/preview -count=1` 通过 |
| 按钮圆角与状态 | PASS（reference renderer） | 统一 `border-radius:999px`、44px 触控高度及 hover/focus/active/disabled CSS；不新增非法协议字段 |
| 十套 A2UI 消息协议 | NOT_RUN（可复现门禁） | 十套来源 fixture/manifest 尚未成为仓库资产；此前本地结果仅作探索记录 |
| 七套内置 Recipe Gallery | PASS（reference renderer） | `render-recipe-gallery.sh` 生成 7/7 reference preview 并检查 14 类组件；脚本、HTML 与七份 fixture 已接入 `acceptance.sh --layer preview` |
| 代表卡片视觉抽查 | NOT_RUN（正式证据） | 需要仓库命令输出 manifest，并记录固定 frame/theme/font/resource 状态；人工浏览器快照不能单独形成 PASS |
| 窄宽布局 | PARTIAL | reference renderer 有定向 DOM/CSS 测试；仍需把 7 Recipe 固定视口无溢出检查接入自动化入口 |
| 与真实钉钉端 token/图标一致 | NOT_RUN | 仍需冻结 PC/iOS/Android Renderer 版本及同输入截图；本地预览不得替代该证据 |

全量截图还发现并修复一处通用渲染缺陷：组件外层 `<section class="Image">` 与原生 `<img class="Image">` 复用了类名，旧 `.Image{height:100%}` 会同时拉伸外层，造成 profile 大段空白、rich-media 后续内容看似消失。当前原生图片规则限定为 `.ImageFrame>.Image`，Row 内固定尺寸图片显式占有 24/40/96/160px 轨道，权重内容列允许收缩；对应 DOM 断言防止该选择器回归。远程图片能否加载仍是资源/网络证据，不能由无横向溢出替代。

设计检测执行 `impeccable detect` 扫描双栏页与十个预览，当前环境因缺少 `htmlparser2/css-select/css-tree/domutils` 降级为正则模式，报告 10 条同类 `flat-type-hierarchy` WARNING。该告警来自每个独立 HTML 都内嵌完整字号 token 映射（11–22px），并不证明单张卡同时使用全部档位；但也不能据此宣布检测完全干净。后续可让 renderer 仅输出本消息实际使用的 token CSS，再用完整 parser/计算样式重跑对比度和字号层级检查。

当前验证事实为：`internal/card/a2ui/...` 与 card focused helper tests 通过；`scripts/card/acceptance.sh --layer all` 通过，并实际执行 7 Recipe Gallery（7/7 Recipe、14/14 组件）及本地交互检查；Schema Catalog、generated drift、command surface 与开源资产静态扫描通过。完整 `go test ./...` 未通过：`internal/helpers/doc/doc_jsonml_validate_v2_test.go` 引用未定义的 `checkTypeV2`，同一 Go 工具链在 merge-base `8cacb01951d2567b2c0466d14cb6c5c9d0267a68` 可复现；本次全量运行还出现 Keychain 超时、Go 1.27 错误文本断言、JSONML 旧 fixture、minutes 深度限制和缺少 `gh` 等 A2UI 范围外失败；后者只记为当前环境/仓库全量结果，未经 merge-base 逐项复现不声称已证明为基线。这些失败路径均不在本批 diff 内，本批不越界修改。真实钉钉端、服务端回读和三端视觉/交互仍按各自条目保持 PARTIAL/NOT_RUN/BLOCKED，不得被本地 acceptance 替代。

## 28. 文件变更、必要性与开源落点

本清单覆盖本功能分支相对基线的全部功能文件和当前拟新增的 Gallery/fixture/跨平台锁文件；最终数量以 `git diff --name-status <merge-base>` 为准，避免文档中的手工计数漂移。状态含义：`REQUIRED` 表示本功能不可缺少；`REQUIRED_FIX` 表示必要但存在 §29 阻断问题；`CONDITIONAL` 表示只有正式接入文档/验收/CI 后才应提交。测试和文档必须与实现同批更新，运行产物不得提交。

| 文件 | 作用与必要性 | 当前结论 |
|---|---|---|
| `.changes/a2ui-card-authoring.md` | 用户可见变更记录；命令面新增必须提供 | REQUIRED：明确 reference preview 证据边界和 profile-scoped delivery，不声称客户端像素或完整生产闭环 |
| `docs/a2ui-agent-card-authoring-technical-design.md` | 设计、边界、实现追踪与验收唯一主文档 | REQUIRED；本轮同步实际状态，不能替代自动化收据 |
| `go.mod`、`go.sum` | 引入 JSON Schema 校验依赖及其传递依赖 | REQUIRED_FIX：依赖本身有用途，但不能用删 pattern 代替等价语义校验；升级需复核许可证与供应链 |
| `internal/app/root.go` | 将 `card` 纳入内建产品/命令组 | REQUIRED |
| `internal/app/runner.go` | 将已解析 profile 的非敏感 `corpId/userId` 注入运行时身份上下文 | REQUIRED：card 写入台账不再依赖可复用别名 |
| `internal/profilectx/profile.go`、`profile_test.go` | 在不让 helpers 反向依赖 auth 的前提下传递当前精确身份 | REQUIRED：只含非敏感身份，按单/多 profile 调用恢复 |
| `internal/card/a2ui/authoring/authoring.go` | 组件语义、Block、Recipe、CompositionSpec 编译与 guide | REQUIRED；当前固定字段表达力为 PARTIAL |
| `internal/card/a2ui/authoring/authoring_test.go` | 编译、Recipe、视觉默认值和确定性回归 | REQUIRED；后续补事实保真、Slot 和 Gallery 一致性门禁 |
| `internal/card/a2ui/delivery/store.go`、`lock_process_*.go` | 本地 handle ledger、revision、flowStatus 持久化和崩溃可恢复锁 | REQUIRED：冻结 `corpId/userId`、环境摘要和目标，提供跨进程 handle 锁、死亡进程恢复与本地 revision CAS；服务端 CAS/回读不在该文件能力内 |
| `internal/card/a2ui/delivery/store_test.go` | ledger 生命周期回归 | REQUIRED：覆盖精确 scope mismatch、旧 selector-only 台账拒绝、锁冲突、死亡进程锁恢复和 stale revision CAS |
| `internal/card/a2ui/preview/preview.go` | 离线 reference renderer 与交互模拟 | REQUIRED；仅作结构快速反馈，不是客户端 Renderer |
| `internal/card/a2ui/preview/preview_test.go` | DOM、样式、交互和公开字段投影回归 | REQUIRED；不得通过 Recipe ID 私有 CSS 造假 |
| `internal/card/a2ui/protocol/assets/a2ui-catalog.json` | 组件协议权威输入 | REQUIRED_FIX：需公开来源、版本、许可证、digest 和升级流程 |
| `internal/card/a2ui/protocol/assets/a2ui-common-types.json` | 公共类型协议权威输入 | REQUIRED_FIX：同上；不得保留无法公开解释的内部来源 |
| `internal/card/a2ui/protocol/protocol.go` | Bundle 加载、结构/引用/组件/消息校验 | REQUIRED：已补被移除 pattern 的等价语义检查和 `updateComponents.components` 非空数组约束；协议资产来源/许可证仍是独立交付阻断项 |
| `internal/card/a2ui/protocol/protocol_test.go` | 协议正负例与离线确定性测试 | REQUIRED：覆盖 HostResultPath、`dt_actionBindingsV1` 及缺失/null/对象/空数组 components 负例 |
| `internal/card/a2ui/state/state.go` | Surface reducer、snapshot/delta 和状态归并 | REQUIRED：snapshot 严格解码、确定性叶级 diff；保护全部公开输入控件的 `value` binding；支持数组 JSON Pointer；缺失数据路径按合同保留，对协议无法表达的组件删除 fail closed |
| `internal/card/a2ui/state/state_test.go` | reducer、revision、snapshot 差异测试 | REQUIRED：覆盖 snapshot 合法/非法结构、14 类输入值保持、叶级更新、数组路径、缺失数据保留和组件删除拒绝；远端乱序仍由后续集成层处理 |
| `internal/cli/schema_parameter_mapping_completeness_test.go` | 为新增本地 `card` leaves 声明映射豁免/完整性 | REQUIRED；仍需 Safety truth 门禁覆盖写命令 |
| `internal/helpers/card.go` | `dws card` Cobra 命令、参数、调用和输出 | REQUIRED：Safety、snapshot 独立解码、精确 user-query 合同、scope 校验及更新串行化已落地 |
| `internal/helpers/card_test.go` | 命令面、dry-run、发送/更新和结构化失败测试 | REQUIRED：覆盖确认门禁、跨 profile 零远端调用、snapshot 输入保护、dry-run、ledger 生命周期和 user-query help 合同 |
| `internal/helpers/chat.go` | 旧 A2UI 原子命令兼容边界 | REQUIRED：保留脚本兼容并 deprecated，引导 Agent 使用 `dws card send/update/finish` |
| `internal/helpers/register_card.go` | 独立注册 card 产品与命令组 | REQUIRED；符合不手改生成注册表的仓库约束 |
| `internal/helpers/testdata/a2ui-approval-spec.json` | 审批 Recipe 受评审 fixture | REQUIRED；由 focused tests、acceptance 和 Gallery 共同消费 |
| `internal/helpers/testdata/a2ui-information-spec.json` | 信息/报告复杂组件覆盖 fixture | REQUIRED；由 focused tests、acceptance 和 Gallery 共同消费 |
| `scripts/card/acceptance.sh` | Source and preview acceptance entry | REQUIRED: check the current panel contract and collect the five-recipe, 12-component gallery receipt. |
| `scripts/card/preview-interaction-check.mjs` | 无远端副作用的输入/选择/按钮交互检查 | REQUIRED；执行环境缺 Node 时必须显式标记 unavailable，不能默认 PASS |
| `internal/helpers/testdata/a2ui-notification-spec.json` | notification Recipe fixture | REQUIRED：已进入 Gallery + acceptance |
| `internal/helpers/testdata/a2ui-task-spec.json` | task Recipe fixture | REQUIRED：已进入 Gallery + acceptance |
| `internal/helpers/testdata/a2ui-schedule-spec.json` | schedule Recipe fixture | REQUIRED：已进入 Gallery + acceptance |
| `internal/helpers/testdata/a2ui-form-spec.json` | form Recipe fixture，与 approval 的交互结构做差异化验收 | REQUIRED：已进入 Gallery + acceptance，并与 approval 保持内容收集/决策确认的结构差异 |
| `scripts/card/recipe-gallery.html` | Six-recipe reference gallery index | REQUIRED: reference generated previews only. |
| `scripts/card/render-recipe-gallery.sh` | Compile five recipes, lint and preview them, and check 12 component types | REQUIRED: called by `acceptance.sh --layer preview` to produce a machine-readable receipt. |

### 28.1 Gallery 与 fixture 的正式合同

1. `internal/helpers/testdata/a2ui-<recipe>-spec.json` is reviewed input, not generated output; its five files must match `dws card recipe list` exactly. Reports use the `information` recipe.
2. `scripts/card/render-recipe-gallery.sh` 是唯一 Gallery 生成入口；必须接收 `--binary` 和 `--out`，输出 compose/lint/preview/manifest 收据，不调用远端。
3. `scripts/card/recipe-gallery.html` 只提供稳定索引和说明，具体卡片由生成目录中的 reference preview 提供；不得复制协议、内嵌私有截图或写死机器路径。
4. Generated previews are test artifacts and must stay outside Git. A gallery PASS covers five recipes, lint, local structural previews, and 12 observed component types; it does not establish client rendering.
5. `scripts/card/acceptance.sh --layer preview` invokes the gallery gate. Its receipt must match the current five-recipe inventory and 12-component coverage target.

## 29. Bugbot 缺陷与修复追踪矩阵

以下问题均基于当前实现，不是未来优化建议。P1/P2 在修复并通过对应验收前阻断合并；状态不得仅因文档已登记而改成 PASS。

| ID | 级别 | 问题与代码落点 | 规定修复 | 必须增加的验收 | 状态 |
|---|---|---|---|---|---|
| BB-01 | P1 | `delivery/store.go` 的 Record 不含 profile/组织/环境/目标 scope | 持久化不可变发送作用域；update/finish 对当前 profile 与台账 scope 做 fail-closed 核对 | `ProfileScope.Matches`、跨 profile helper 用例；scope mismatch 时远端调用数不变 | PASS |
| BB-02 | P1 | `state/state.go` snapshot diff 可替换根 DataModel，ledger 又没有用户最新表单状态 | 声明 Agent 管理路径；只生成这些路径的 delta，用户/宿主路径默认保留 | state 单测与 helper snapshot 用例均只发送 `/content/status`，不生成 `/form` delta | PASS |
| BB-03 | P1 | `card.go` 的 send/update/finish 为 `confirmation=not_required` | Contract/Safety 改为设计规定的 `user_required`，接入现有运行时确认门禁 | 无确认零远端调用、dry-run 可执行；Schema runtime-confirmation truth、Help 和 command surface 门禁通过 | PASS |
| BB-04 | P1 | update 为无锁 load→remote→save，同 revision 可并发写 | handle 级串行化并在本地 save 使用 revision CAS；远端无 CAS 时如实标注保证边界 | 嵌套/跨进程同 handle 锁明确失败，stale revision CAS 失败；远端无 CAS 的边界保留在 §22 | PASS |
| BB-05 | P2 | `--input-mode snapshot` 文档结构仍按 message batch 解码 | 按 `{version,surfaceId,components,dataModel}` 独立解码和完整校验，再生成 delta | `ParseSnapshot` 严格正负例、CLI fake-IM 与 `card show surface → snapshot dry-run` 直接 round-trip 通过；非法输入在远端前失败 | PASS |
| BB-06 | P2 | `protocol.go` 删除不受 Go regexp 支持的 lookahead，但无等价检查 | 为 HostResultPath、dt_actionBindings 增加显式 Go 语义校验，不篡改协议意图 | 两类降级 pattern 的正负例与诊断码通过 | PASS |
| BB-07 | P2 | `updateComponents` 缺失/错误 `components` 仍 lint 为 valid | 在 envelope/operation 层强制非空数组及完整组件对象 | 缺失、null、对象、空数组均返回对应诊断；真实 CLI lint 最小复现返回 failure | PASS |
| BB-08 | P2 | `--user-query` 宣称支持姓名/线索，resolver 只接受 open ID/精确 userId | 收窄 Help/Schema 合同为精确 userId/openDingTalkId，不承诺姓名模糊搜索 | resolver dry-run 与 flag usage 自动化断言通过 | PASS |
| BB-09 | P2 | `acceptance.sh` 断言过期 PanelContent padding | 断言协议/DOM 语义和当前稳定 token，避免绑定已删除的实现细节 | `acceptance.sh --layer all` 干净运行 PASS，并实际执行 interaction 与 Gallery | PASS |
| BB-10 | P1 | 内置 `information` fixture 使用 data URI 占位图，本地 preview 可见但真实客户端显示“占位图生成失败” | 可发送 fixture 改为已验证 HTTP 200 的 HTTPS CDN；send/update/finish 在远端调用前拒绝 preview-only/不安全 Image URL | 协议正负例、helper 零远端调用、真实修正版 create + FINISH 回执通过；客户端最终取图仍由截图确认 | PARTIAL |
| BB-11 | P1 | Snapshot 只保护 `TextField/ChoicePicker`，其它公开输入控件可能被 Agent 默认值覆盖 | 从公开输入组件清单统一提取 `value.path`，保留宿主持有输入 | 14 类输入组件逐表测试；正文仍可更新、绑定值保持 | PASS |
| BB-12 | P1 | 台账 scope 只有 selector 字符串，别名重绑后可能跨账号更新 | runner 注入已解析非敏感身份；send 冻结 `corpId/userId/environment`；update 对旧 selector-only 台账 fail closed | 同身份通过、跨身份和旧台账拒绝、零远端调用 | PASS |
| BB-13 | P1 | 进程崩溃遗留 `.lock` 会永久阻断 handle | 锁记录 PID/时间；Unix 校验进程存活，其它平台使用有界年龄；死亡 owner 安全回收 | live owner 拒绝、dead owner 回收、嵌套锁拒绝 | PASS |
| BB-14 | P2 | Snapshot 删除组件时 Diff 静默忽略，实际卡片与期望快照分叉 | 协议未提供组件删除操作时 fail closed；数据对象缺失字段继续按既定合同解释为保留 | 组件删除返回显式错误；缺失数据键不产生更新 | PASS |
| BB-15 | P1 | 单条 delta 可单独合法但合并后产生缺 child/环等非法完整 Surface | Reduce 后重建 create snapshot 并执行完整校验，再允许远端更新 | 缺失 child 的 merged Surface 在远端前失败 | PASS |
| BB-16 | P2 | `setPointer` 只处理对象，合法数组路径更新失败 | 递归支持 object/array JSON Pointer，数组越界 fail closed | `/items/0/status` 成功、越界拒绝 | PASS |
| BB-17 | P2 | `task/schedule` 的 Recipe 声明 `compact-notification-header`，编译器仍输出普通 Row header | 专用编译策略输出 `CardHeader(trailing=Tag)`、根 0 padding、正文 8np inset | 两个 Recipe 的消息 Schema、结构断言和 Gallery 目视抽查 | PASS（reference preview） |
| BB-18 | P2 | compose 推断 Recipe 后仍在返回值中报告空 recipe；旧 chat 命令仍被 Agent 当作首选 | 统一 ResolveRecipe 回执；旧原子命令 deprecated 并在 selection 中路由到 `dws card` | compose 缺省回执和两个旧命令 deprecation 断言 | PASS |

此外有两个开源交付阻断项：协议资产必须补齐公开来源、许可证和版本 manifest；卡片平台参考图必须先成为公开、可复现且可审计的来源包。两项均保持 BLOCKED，不能用内部地址、个人工作区或临时截图替代。

## 30. 当前验收状态与合并退出条件

本节是 §25 的实时摘要。执行环境、输入 commit、dirty diff、二进制摘要或协议资产变化后，旧证据失效。

| 验收层 | 当前结果 | 已有证据 | 关闭条件 |
|---|---|---|---|
| Diff 基础卫生 | PASS | `git diff --check` 通过 | 最终 diff 再运行一次 |
| 开源资产静态扫描 | PASS（能力有限） | `check-open-source-assets.sh` 通过 | 同时人工确认无内部 URL/本机路径，并补协议许可证 manifest |
| Command surface | PASS | `check-command-surface.sh --strict` 在最终 Safety/参数 diff 上通过 | 命令面继续变化时重跑 |
| A2UI package tests | PASS（源码层） | 2026-09-17 focused 与 package-wide `go test ./internal/card/a2ui/...` 通过，BB-01/02/04/05/06/07/11/13/14/16/17 新测试均在内 | 协议/状态/台账改变时重跑 |
| Card focused helper tests | PASS（源码层） | 2026-09-17 `go test ./internal/helpers -run '^TestCrossPlatformCoverage(Card|LegacyA2UI)' -count=1` 通过，覆盖 BB-03/05/08/12/15/18 与 fake-IM 生命周期 | 命令参数或 Delivery 调用改变时重跑 |
| Preview acceptance | PASS（reference preview） | 2026-09-17 `acceptance.sh --layer all` 生成 `source.json`、`preview-status.json`，interaction 实际执行且无远端副作用 | 不可提升为真实 Renderer PASS |
| 7 Recipe Gallery | PASS（reference preview） | acceptance 内生成 `gallery-status.json`：7/7 Recipe、14/14 组件 | 新增 Recipe/组件时同步 fixture 与门禁 |
| Current five-recipe gallery | PASS (2026-09-24, local reference preview) | `bash scripts/card/acceptance.sh --layer preview --binary ./dws` passed with the current source build; the receipt records 5/5 Recipes, 12/12 component types, lint, local previews, and interaction simulation. | Actual DingTalk client rendering and interaction remain a separate acceptance gate. |
| 完整 `go test ./...` | BLOCKED（仓库/环境范围） | `checkTypeV2` 编译失败已在 merge-base `8cacb019` 复现；当前全量还有 Keychain 超时、Go 1.27 错误文本断言、JSONML/minutes 旧用例和缺少 `gh` 等本批 diff 外失败，尚未逐项在 merge-base 复现；A2UI 相关包无新增失败 | 单独修复/复现这些仓库基线或环境问题后重跑全量；本批不越界修改无关模块 |
| Schema/generated policy | PASS | `check-generated-drift.sh`、`check-schema-catalog.sh`、runtime confirmation truth 及 Schema assembly determinism 均通过 | Schema/Contract 变化时重跑 |
| Snapshot/update 并发与输入保护 | PASS（源码层） | BB-01/02/04/05/11–16 的精确 scope、崩溃锁恢复、CAS、strict snapshot、14 类 protected path、数组 Pointer、组件删除拒绝、缺失数据保留与合并后 Surface 复验全部通过 | 真实端最新 DataModel/远端 CAS 仍归入下方集成项 |
| 写操作确认合同 | PASS | Schema/Help/Runtime truth 与无确认零写入测试通过 | Safety 变化时重跑 |
| 真实钉钉发送/更新 | PARTIAL | 2026-09-16 使用当前分支二进制和批准测试群完成内置 `information` Recipe 的 create、snapshot update、FINISH；修复 data URI 后又完成 HTTPS 图片修正版 create + FINISH，服务端回执复用各自 cardInstanceId，ledger scope/revision 一致；`verify` 仍明确 `deliveryVerified=false` | 用户确认修正版客户端图片；补服务端 readback、远端 CAS/乱序恢复及跨 scope 真实拒绝；不得用 MCP success 代替客户端渲染证据 |
| 真实钉钉视觉与交互 | NOT_RUN | reference preview 不构成端侧证据 | PC/iOS/Android 同输入截图与按钮/输入/ChoicePicker/流式反馈事件回流 PASS |
| 卡片平台十套模板 1:1 | BLOCKED | 无公共、可复现来源 manifest | 来源合法化、同 Renderer 合同或明确 NOT_COMPARABLE，逐区差异审阅完成 |

当前 BB-01～BB-09、BB-11～BB-18 已关闭；BB-10 的源码、CLI fake-IM、HTTPS 可达性和受控真实发送证据已通过，等待用户确认修正版客户端图片后关闭。2026-09-17 的 Gallery 重新生成并目视抽查 `information/task/form`：七套均可见，task 的 CardHeader/inset、form 与 approval 的信息架构差异符合当前设计；该结果仍只属于 reference renderer。source/preview/Schema/generated/command surface 门禁须绑定最终提交 SHA 后再次运行。仍阻断“完整生产就绪”声明的项目是：协议来源/许可证 manifest、卡片平台十套来源合法化、服务端回读/远端 CAS，以及 PC/iOS/Android 真实视觉与 Action 回流。合并范围若只声明本地作者工具、协议校验、reference preview 与受保护增量更新，可进入评审；若声明完整可靠交互或像素一比一，则上述外部项必须先关闭。
