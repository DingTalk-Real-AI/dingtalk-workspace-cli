# DWS Chat / IM 参数问题分析与兜底方案（第二轮全量审计）

> 分析日期：2026-07-27
> 分析分支：`fix/param-hallucination`
> 分析基线：`0ef1b73`
> 分析对象：DWS Chat / IM 产品的 CLI 参数
> 对应明细：`im_cli_param_hallucination_analysis_20260727_v2.xlsx`

## 1. 结论摘要

本轮从当前 DWS 的真实命令树出发，对 Chat / IM 产品进行了第二轮全参数审计。分析覆盖 148 个目标命令、110 种可见参数名，并进一步检查了 211 条 Cobra 可执行路径。

结论是：Chat / IM 的参数问题不只集中在会话、用户、消息等业务标识符上，还包括分页、搜索词、时间窗口、文件路径、单复数、参数拼写格式以及同名参数值域冲突。第一轮识别了 7 类主要问题，本轮补充 8 类，合计形成 15 类产品级参数问题。

这些问题需要分成三种方式处理：

1. **值不变、目标唯一的一对一改名**：适合通过 `param_concepts.json` 的 concept 或命令级 alias 兜底；
2. **可能选错目标、改变单复数或丢失信息的输入**：应使用 block/ambiguous 保护性拦截；
3. **需要查数据、转换参数值、补齐多个参数或理解值域的场景**：当前别名体系不能处理，应由命令编排、值规范化或 Schema 参数约束解决。

因此，不能用“是否成功改成了一个真实参数名”作为唯一判断标准。参数名归一成功只说明 Cobra 可以继续解析，不代表业务语义、参数基数和值域一定正确。

### 1.1 量化结果

| 分析项 | 结果 | 说明 |
|---|---:|---|
| 目标命令 | 148 个 | 73 个稳定 Schema 命令、32 个兼容命令、42 个快捷命令、1 个可执行父命令 |
| Cobra 可执行路径 | 211 条 | 157 条非隐藏路径、54 条隐藏路径 |
| 可见参数名 | 110 种 | 表明相邻命令间可供 Agent 类推的参数拼写较多 |
| 原问题明细涉及命令 | 101 个 | 另有 47 个目标命令未出现在原问题明细中 |
| 有业务参数但未进入原明细 | 44 个 | 是第二轮差集审计的重点，不代表这 44 个命令全部存在问题 |
| 主要参数问题 | 15 类 | 原 7 类，加上本轮补充的 8 类 |
| 新增命令级问题明细 | 103 条 | 一行表示某个问题在一个具体命令上的表现 |
| 当前能力外场景 | 9 类 | 不能靠一对一参数名别名安全完成 |
| 补充代表性 CLI 验证 | 14 条 | 11 条失败或被保护性拦截，3 条被原有格式/拼写链路接住 |

上述“涉及命令数”存在交叉。同一个命令可能同时存在会话标识符、分页参数和时间参数问题，因此不能把各类命令数直接相加作为问题命令总数。

## 2. 分析范围与依据

本轮只分析当前产品事实，数据来源为：

1. 当前构建产生的 `dws chat ... --help`；
2. 当前 Cobra 命令树和真实 flag 定义；
3. `dws schema chat --compact -f json` 与 `internal/cli/schema_catalog.json`；
4. `skills/mono/references/products/chat.md`；
5. `internal/cli/schema_command_exclusions.json` 中记录的兼容命令和快捷命令；
6. `internal/cli/param_concepts.json`，用于判断现有参数兜底能力；
7. 必要的命令实现，用于确认参数能否在不改变值的情况下直接映射。

本轮没有把历史 `dws-eval` 实验、`merged_scan.json`、历史 badcase 或出现次数作为问题来源。补充验证用例是从当前 Cobra、Schema 和 Help 的参数差异中生成的，并且只验证预解析与 Cobra 参数解析链路，没有调用真实 RPC。

## 3. 十五类主要参数问题

| 参数问题 | 涉及命令数 | 现有兜底能力 | 处理方向 |
|---|---:|---|---|
| 会话标识符命名不统一 | 89 | 部分可以 | 拆分单值、列表和源/目标角色，按命令配置 alias/bind |
| `group` 同名异义 | 2 | 可以，但必须限定命令 | 群名与开放会话 ID 分开建 concept |
| 数字群号与开放会话 ID 混淆 | 1 | 只能拦截 | 禁止跨值域改名，需要时先查询转换 |
| 消息标识符命名、角色和单复数不统一 | 21 | 大部分可以 | 单值、列表、引用消息分别维护 |
| 用户标识符命名、角色、单复数和值域混杂 | 31 | 部分可以 | 高置信度命令级 alias，其余 block/ambiguous |
| 机器人标识符值域不同 | 4 | 不应互相兜底 | `robotCode` 与 `openBotId` 分开维护 |
| Skill 与真实命令参数不一致 | 13 | 别名表不负责 | 直接修正 Skill 文档契约 |
| 分页数量参数和游标类型不统一 | 35 | 部分可以 | 数量参数按命令映射，禁止 page/cursor 互转 |
| 搜索词参数命名不统一 | 4 | 可以 | 只在 bot find/search 精确路径配置 alias |
| 时间窗口参数名和值格式不统一 | 9 | 只能部分处理 | 一对一可映射，一对多及格式转换必须拦截 |
| 分组名称使用 `name`/`title` 不统一 | 5 | 可以 | 只在 category 命令中配置命令级 alias |
| 本地文件路径使用 `file`/`file-path` 不统一 | 2 | 可以 | 两个精确命令间做值不变映射 |
| 业务 ID 单复数与参数类型不统一 | 13 | 可保护，不应全局转换 | 拆分单值/列表 concept，默认拦截误用 |
| 参数拼写格式不统一 | 2 | 正式 CLI 已支持 | 继续使用原有格式归一并补回归测试 |
| 通用参数同名异义和值域冲突 | 6 | 别名不应处理 | 通过 Schema 类型、枚举和值域约束治理 |

## 4. 第一轮七类问题的复核结论

### 4.1 会话标识符

同一个开放会话 ID 在不同命令中使用 `group`、`id`、`chat`、`conversation-id`、`open-conversation-id` 等名称，是当前覆盖面最大的问题。

可以治理的部分是：输入值保持不变，而且在当前命令中只有一个明确目标参数。不能合并的部分包括：

- `conversation-ids` 等列表参数；
- `src-conversation-id`、`dest-conversation-id`、`source`、`target` 等方向角色；
- 数字群号 `group-id`；
- Cobra 已经原生接受的 `chat/group/id/conversation-id` 兼容参数。

### 4.2 群名、数字群号与开放会话 ID

这三者必须明确分开：

- 群名是搜索条件，需要搜索和候选消歧；
- 数字群号是 `chat group get-by-group-id` 使用的数值标识；
- 开放会话 ID 是多数群聊和消息命令直接使用的字符串标识。

别名表只能处理参数名，不能把一个群名或数字群号自动转换成开放会话 ID。

### 4.3 消息标识符

普通消息 ID、消息 ID 列表和被引用消息 ID 应分别建模。`msg-id`、`message-id`、`open-message-id` 可以在语义明确的单值命令中归一，但不能把 `msg-ids` 自动缩成单值，也不能把普通消息 ID 无条件映射为 `ref-msg-id`。

### 4.4 用户标识符

第二轮补充发现了 `sender-user-id`、`sender-open-dingtalk-id`、`at-users`、`at-user-ids` 等发送者和 @ 用户列表参数，使该类问题的涉及命令数由 28 增至 31。

需要同时保护以下边界：

- userId 与 openDingTalkId 不能跨值域改名；
- 单个用户与用户列表不能默认互转；
- 发送者、接收者、新群主、申请人、邀请人和 @ 用户不能只因都是“用户”就合并；
- 同一命令同时存在多个用户目标时，宽泛的 `user-id` 必须提示歧义。

### 4.5 机器人标识符

`robot-code` 表示 robotCode，`bot-id` 表示 openBotId。名称接近但值不通用，应分别维护 `robot_code` 和 `open_bot_id`，并阻止 `robot-id`、`bot-code` 等不明确输入跨值域归一。

### 4.6 Skill 参数契约

这类问题不是参数别名问题，应直接修改文档：

- `chat message send` 仍描述了 5 个当前命令不存在的旧文件参数；
- 2 个命令合计漏写 3 个真实参数；
- 6 个稳定命令缺少精确 Usage/Flags 段；
- 4 个 reaction 命令未说明 Cobra 原生支持的兼容会话参数。

## 5. 第二轮新增八类问题

### 5.1 分页数量和游标

35 个命令使用了 `limit`、`size`、`count`、`page` 或 `cursor`。其中：

- `limit`、`size`、`count` 在部分命令中都表示返回数量，可以逐命令审核别名；
- `page` 是页码，`cursor` 是服务端返回的翻页令牌，不能互相改名；
- `cursor` 在不同命令中存在 string、int、int64 三种类型，即使参数名相同，也不能认为取值可以跨命令复用。

### 5.2 搜索词

`chat bot find` 使用 `--query`，`chat bot search` 使用 `--name`，对应快捷命令也存在同样差异。可以在这 4 条精确路径中配置单向别名，但不能把 `name` 全局归一为 `query`，否则会影响群名、分组名和角色名等真实参数。

### 5.3 时间窗口

9 个消息查询命令混用单个 `--time` 与 `--start/--end`，并同时存在普通日期时间和 ISO-8601 格式。

单个 `start` 在业务确认后可能映射到 `time`；反方向不能把一个 `time` 无条件扩展成 `start/end`，因为结束时间和时间范围无法唯一推导，时间格式及时区也不是参数名别名能够处理的内容。

### 5.4 分组名称

普通分组创建和重命名使用 `--title`，智能分组创建使用 `--name`。两者业务语义相同、值不需要转换，可以限定在 5 个 category 主路径和快捷路径中配置命令级别名。

### 5.5 本地文件路径

`chat media upload` 使用 `--file`，`chat message send` 使用 `--file-path`。这两个参数都表示本地文件路径，可以进行精确命令的一对一映射，但不能借此恢复 Skill 中已失效的 `file-name`、`file-size`、`file-type` 等旧参数。

### 5.6 业务 ID 单复数

`category-id/category-ids`、`role-id/role-ids` 分别代表单值和列表。当前旧拼写纠错可能把单数形式自动改为复数并继续执行，但“可以解析”不等于“业务上允许扩大为批量操作”。应拆分单值/列表 concept，默认拦截，只有在精确命令确认单元素列表完全等价后才允许映射。

### 5.7 camelCase 参数

`chat chmod` 和 `chat data-auth cross-org` 暴露了 `agentCode`、`permParam`。正式 `dws` CLI 入口会经过 `RunPreParse`，已经能够把常见 kebab-case 输入归一为真实参数，不需要在 `param_concepts.json` 中重复维护。

需要记录的边界是：如果内部测试或代码绕过正式入口，直接对子 Cobra command 调用 `ParseFlags`，这一层格式归一不会生效。

### 5.8 `status`/`type` 同名异义

这两个参数在不同命令中可能表示审批动作、0/1 开关、群类型、媒体类型或资源类型。参数名虽然相同，但值域完全不同。

该问题不能靠别名表解决，也不应该建立跨命令 concept。正确方向是在 Schema 中提供真实类型、枚举和值域说明，并在执行前校验输入。

## 6. 现有兜底体系的适用边界

### 6.1 适合直接进入别名表

满足以下条件时，可以配置 concept 或命令级 alias：

1. 来源参数在当前命令中不是一个真实 flag；
2. 目标参数唯一，没有多个合理候选；
3. 输入值无需转换；
4. 参数的业务实体、角色、值域和单复数保持一致；
5. 替换后不会改变批量范围、时间范围或安全语义。

典型场景包括 bot 搜索词、category 的 `name/title`、两个本地文件路径参数，以及经过逐命令确认的分页数量参数。

### 6.2 应该 block 或 ambiguous

以下情况不应静默改名：

- 一个来源参数可能对应多个真实目标；
- 单值和列表混用；
- userId、openDingTalkId、robotCode、openBotId 等值域混用；
- 一个参数需要扩展成多个参数；
- 替换会丢失结束时间、用户角色或源/目标方向；
- 旧拼写纠错虽然能命中真实 flag，但业务语义尚未审核。

### 6.3 当前链路不能完成的九类场景

| 场景 | 当前安全处理 |
|---|---|
| 数字群号自动转换成开放会话 ID | 区分两类 ID，提示先查询再执行 |
| 群名自动转换成开放会话 ID | 保留快捷命令本地搜索；基础命令不接受群名别名 |
| 单个 userId 自动提升为用户列表 | block/ambiguous，要求明确真实目标参数 |
| Skill 的 5 个旧文件参数转换成 `file-path` | 直接修改 Skill，不做多参数猜测 |
| 强制重写已经存在的真实兼容参数 | 保留 Cobra 原生行为，只统一推荐口径 |
| 页码与游标自动互转 | 保持真实分页方式，错误输入进行保护性拦截 |
| 单个 `time` 自动扩展为 `start/end` 并转换格式 | 提示补全时间范围和正确格式 |
| 通用单数 ID 自动提升为列表 | 拆分 concept，默认 block，逐命令审核 |
| `status/type` 的值域自动翻译 | 通过 Schema 约束和执行前校验解决 |

这些场景“当前无法解决”不等于问题不处理。对于存在信息损失或误执行风险的输入，保护性拦截本身就是当前正确的处理结果。

## 7. 代表性 CLI 验证

本轮从当前参数差异中选择了 14 条代表性变体输入进行解析链路验证。

| 输入示例 | 真实参数 | 当前结果 | 判断 |
|---|---|---|---|
| `chat bot search --query robot` | `--name` | 未知参数 | 可增加命令级 alias |
| `chat bot find --name robot` | `--query` | 未知参数 | 可增加命令级 alias |
| `chat message list-favorites --limit 20` | `--size` | 未知参数 | 可逐命令审核数量别名 |
| `chat message list-all --time ...` | `--start/--end` | 未知参数 | 一对多，不应直接别名 |
| `chat message list --start ...` | `--time` | 未知参数 | 可评估精确命令的 `start -> time` |
| `chat message list-by-sender --time ...` | `--start/--end` | 保护性拦截 | 阻止信息不完整的映射 |
| `chat message list-by-sender --user-id ...` | `--sender-user-id` | 未知参数 | 发送者角色尚未纳入映射 |
| `chat category create-smart --title ...` | `--name` | 未知参数 | 可增加命令级 alias |
| `chat category create --name ...` | `--title` | 未知参数 | 可增加命令级 alias |
| `chat media upload --file-path ...` | `--file` | 未知参数 | 可增加命令级 alias |
| `chat message send-by-webhook --at-user-ids ...` | `--at-users` | 未知参数 | 可做值域明确的列表 alias |
| `chat category add-conv --category-id ...` | `--category-ids` | 旧拼写纠错通过 | 解析成功，但基数安全性仍需审核 |
| `chat group-role set-user --role-id ...` | `--role-ids` | 旧拼写纠错通过 | 解析成功，但不能推广成全局规则 |
| `chat data-auth cross-org --agent-code ...` | `--agentCode` | 格式归一通过 | 正式 CLI 入口已经支持 |

14 条用例中，11 条当前失败或被保护性拦截，3 条被原有拼写纠错或格式归一接住。这说明当前链路已经具备可复用能力，但仍需要把“偶然解析成功”与“业务语义已经审核通过”区分开。

## 8. 后续落地顺序

### 第一优先级：防止错误执行

1. 拆分数字群号、开放会话 ID 和群名称；
2. 拆分用户、消息、分类和群角色参数的单值/列表与业务角色；
3. 对时间一对多、page/cursor、跨值域和多目标输入增加 block/ambiguous；
4. 修正 Skill 中失效、漏写或缺少的参数说明。

### 第二优先级：补充高置信度兼容

1. bot 搜索词 `name/query`；
2. category 分组名称 `name/title`；
3. `file/file-path`；
4. 发送者和 @ 用户列表的精确命令级别名；
5. 逐命令审核 `limit/size/count`，不建立无边界的全局分页别名。

### 第三优先级：补强参数契约

1. 在 Schema 中完善 cursor 类型、时间格式、枚举和值域；
2. 为单值/列表和互斥参数提供显式约束；
3. 建立 Schema、Help、Skill 和 `param_concepts.json` 的产品级对账测试；
4. 将本轮 14 条代表性输入扩展为稳定回归测试集。

## 9. 复用到其他产品的分析流程

后续分析 Calendar、Drive、Contact 等产品时，可复用以下流程：

```text
盘点真实命令和可见参数
  → 对账 Schema / Help / Skill
  → 按业务实体、角色、值域和单复数归类
  → 找出异名、同名异义、格式和类型问题
  → 判断 alias / bind / block / ambiguous
  → 单列需要查值、改值、一对多和多参数转换的能力边界
  → 生成命令级问题明细与解决方案
  → 通过正式 CLI 预解析和 Cobra 参数解析验证
```

每个产品的交付物保持一致：汇报总览、参数问题明细、兜底解决方案、当前无法解决、分析依据，以及对应的文字分析报告。

## 10. 相关文件

- 详细分析工作簿：`docs/parameter-hallucination/im/im_cli_param_hallucination_analysis_20260727_v2.xlsx`
- 当前正式参数概念表：`internal/cli/param_concepts.json`
- IM 分析版参数概念表：`docs/parameter-hallucination/im/param_concepts.json`
- 稳定 Schema：`internal/cli/schema_catalog.json`
- 兼容命令范围：`internal/cli/schema_command_exclusions.json`
- Chat Skill：`skills/mono/references/products/chat.md`
- 参数归一生成：`internal/cli/param_aliases.go`
- 运行时查询：`internal/cli/param_aliases_lookup.go`
- 语义别名处理：`internal/pipeline/handlers/semantic_alias.go`
