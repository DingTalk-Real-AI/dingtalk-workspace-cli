# DWS Chat / IM 参数问题分析与兜底方案

> 分析日期：2026-07-27
> 分析分支：`fix/param-hallucination`
> 分析基线：`0ef1b73`
> 分析对象：Chat / IM 产品的 CLI 参数
> 数据来源：当前 Schema、真实 `--help`、Chat Skill 和 `param_concepts.json`
> 不包含：历史实验、badcase、出现次数和模型通过率

## 1. 结论摘要

本轮共盘点148个当前可执行的 Chat / IM 命令，其中73个是稳定 Schema 命令。结论是：**Chat / IM 当前不存在大面积“Schema 参数不可执行”的基础契约问题，但存在比较明显的产品级参数语义不统一问题。**

稳定命令的 Schema 参数与真实 `--help` 参数一致，说明当前生成和发布链路基本健康。主要风险来自另外三个层面：

1. 同一个业务实体在不同命令中使用不同参数名，例如开放会话 ID 同时使用 `group`、`id`、`chat`、`conversation-id`；
2. 同一个参数名在不同命令中表示不同含义，例如 `group` 既可能是开放会话 ID，也可能是群名称关键词；
3. 用户、消息、机器人等标识符存在单复数、角色和值域差异，表面近似但不能安全互换。

这类问题不会让所有命令都失败，但会显著增加 Agent 根据相邻命令、输出字段或自然语言自行猜测参数名的概率。轻则在参数解析阶段报错，重则把错误值传入一个真实存在但含义不同的参数，形成更隐蔽的错误执行风险。

### 量化结果

| 分析维度 | 当前结果 | 说明 |
|---|---:|---|
| 可执行命令规模 | 148个 | 73个稳定 Schema 命令、32个兼容命令、42个快捷命令、1个可执行父命令 |
| 参数表面规模 | 433组命令与参数组合 | 共出现110种真实参数拼写，Agent 跨命令类推参数名的空间较大 |
| 稳定命令参数规模 | 257组命令与参数组合 | 73个稳定命令共使用85种真实参数拼写 |
| Schema 与真实 Help 一致性 | 73/73个稳定命令一致 | 当前主要矛盾不是 Schema 发布了不存在的参数，而是跨命令语义不统一 |
| 产品级主要问题 | 7类 | 覆盖异名、同名异义、值域、角色、单复数和 Skill 契约差异 |
| 会话标识符问题 | 涉及89个命令 | 是覆盖范围最大的问题，同一个开放会话 ID 使用多种参数名 |
| 用户标识符问题 | 涉及28个命令 | 同时存在 userId/openDingTalkId、单值/列表和角色差异，保护要求最高 |
| 消息标识符问题 | 涉及21个命令 | 普通单值 ID 适合归一，但列表和引用消息必须独立处理 |
| Skill 参数契约问题 | 涉及13个命令 | 包含5个失效参数、3个漏写参数、6个缺少精确说明的稳定命令，以及4个兼容参数说明缺口 |
| 当前中央兜底覆盖 | 14个 Chat 命令路径 | 已证明 alias、bind、block 等能力可用，但目前仍属于点状覆盖 |

以上问题范围存在交叉，例如同一个命令可能同时存在会话标识符问题和用户标识符问题，因此各类涉及命令数不能直接相加为问题命令总数。

聚合后共有7类主要参数问题：

| 参数问题 | 典型现象 | 现有兜底能力结论 | 推荐处理 |
|---|---|---|---|
| 会话标识符命名不统一 | 同一个开放会话 ID 使用 `group`、`id`、`chat`、`conversation-id` 等名称 | 部分可以解决 | 拆出 `open_conversation_id`，只做命令级、值不变的别名；真实兼容参数保持原生 |
| `group` 同名异义 | 有时表示开放会话 ID，有时表示群名称关键词 | 可以按命令解决 | 建立独立 `group_name`，只在两个快捷命令中配置 `group-name -> group` |
| 数字群号与开放会话 ID 混淆 | `group-id` 既容易被理解为数字群号，也容易被理解为开放会话 ID | 不能靠改名自动转换 | 拆分概念并阻止错误映射；需要时先查询再执行 |
| 消息标识符命名、角色和单复数不统一 | `msg-id`、`message-id`、`open-message-id`、`msg-ids`、`ref-msg-id` 并存 | 大部分可以解决 | 单值、列表和引用角色分别建 concept，禁止单复数互转 |
| 用户标识符命名、角色、单复数和值域混杂 | `user/users` 可能表示 userId、openDingTalkId、混合列表或角色型用户 | 只能部分解决 | 安全的单值命令增加别名；列表、混合值域和多目标命令使用 block/ambiguous |
| 机器人标识符值域不同 | `robot-code` 与 `bot-id` 名称相近但不是同一种值 | 不应互相兜底 | 分开维护 `robot_code` 和 `open_bot_id`，禁止跨值域别名 |
| Skill 与真实命令参数不一致 | Skill 存在已失效参数、漏写参数和缺少命令段 | 别名表不负责解决 | 直接修正 Skill，并保留真实 Help/Schema 为参数事实 |

因此，本轮不需要为了统一表面命名去改每个 `skill.go`。当前中央兜底链路适合处理“名称不同但业务含义和值完全相同”的命令级别名；需要查询、改值、改变单复数或选择多个可能目标的情况，必须保护性拦截或继续由命令本地逻辑处理。

完整命令和参数明细见同目录输出的 Excel 工作簿。

## 2. 分析依据

本轮只使用当前产品事实：

1. 当前构建的 `dws chat ... --help`；
2. `dws schema chat --compact -f json` 和 `internal/cli/schema_catalog.json`；
3. `skills/mono/references/products/chat.md`；
4. `internal/cli/schema_command_exclusions.json` 中的兼容命令和快捷命令；
5. `internal/cli/param_concepts.json`，用于判断现有兜底能力；
6. 必要的命令实现，用于确认参数值是否可以原样传递。

没有使用 `dws-eval`、`merged_scan.json`、历史 alias 工作簿或任何实验次数。

## 3. 参数问题与解决方式

### 3.1 会话标识符命名不统一

#### 问题

同一个开放会话 ID 在不同命令中使用了以下真实参数名：

```text
group
id
chat
conversation-id
open-conversation-id
src-conversation-id
dest-conversation-id
source
target
conversation-ids
```

该问题涉及89个命令，是 Chat / IM 中覆盖面最大的参数问题。Agent 在一个命令中学会 `--conversation-id` 后，很容易在另一个只接受 `--group` 或 `--id` 的命令中继续使用它。

#### 需要保留的边界

- `conversation-ids` 是列表，不能和单值概念混用；
- `src-`、`dest-`、`source`、`target` 表示方向角色，不能全部无条件简化；
- reaction 相关命令已经原生接受 `chat/group/id/conversation-id`，中央层不应再次重写这些真实参数。

#### 解决方案

1. 将当前含义过宽的 `group_id` 拆分或收敛为 `open_conversation_id`；
2. 另建 `open_conversation_ids`；
3. 对 `--id`、`--group` 等宽泛真实参数，通过精确命令 `bind` 说明它代表开放会话 ID；
4. 对当前命令不存在、但语义和值完全相同的参数，增加命令级别名；
5. 对真实兼容参数保持原生解析，只在 Skill 中推荐一个主要参数名。

结论：现有兜底功能可以解决其中的“一对一名称差异”，但不能把单值变成列表，也不应消除源/目标角色。

### 3.2 `group` 同名异义

#### 问题

大多数基础命令中的 `--group` 表示开放会话 ID，但以下两个快捷命令中的 `--group` 表示群名称搜索关键词：

```text
chat +group-members --group
chat +send-to-group --group
```

如果把 `group` 全局归入开放会话 ID，快捷命令会被错误治理；如果把它全局理解为群名称，又会破坏大量基础命令。

#### 解决方案

- 建立独立 `group_name` concept；
- 保留 `chat +group-members` 已有的 `group-name -> group` 命令级别名；
- 为 `chat +send-to-group` 增加相同的命令级别名；
- 基础命令不接收群名称别名。

结论：现有兜底功能能够解决，但必须严格限制到精确命令。

### 3.3 数字群号与开放会话 ID 混淆

#### 问题

```text
chat group get-by-group-id --group-id
```

这里的 `group-id` 是数字群号；其他大量 Chat 命令需要的是字符串形式的开放会话 ID。两者不是同一个值，只是可以通过查询建立转换关系。

#### 解决方案

- 将 `numeric_group_id` 与 `open_conversation_id` 完全分开；
- 从开放会话 ID concept 中排除数字群号；
- 当用户提供数字群号但目标命令需要开放会话 ID 时，提示先执行 `get-by-group-id`；
- 不配置全局 `group-id -> group/conversation-id`。

结论：现有兜底功能可以阻止错误改名，但不能完成“查询后替换参数值”。

### 3.4 消息标识符命名、角色和单复数不统一

#### 问题

同一个普通消息 ID 使用：

```text
msg-id
message-id
open-message-id
```

列表使用 `msg-ids`，引用消息使用 `ref-msg-id`。合计涉及21个命令。

#### 解决方案

- 新增 `open_message_id`，只覆盖单个普通消息 ID；
- 新增 `open_message_ids`，只覆盖消息 ID 列表；
- `ref-msg-id` 保持“被引用消息”的角色，可单独建立 `referenced_open_message_id`；
- 在单值命令中 block `message-ids/msg-ids` 等列表拼写；
- 明确排除 `open-task-id`、`topic-id`、`resource-id` 等其他实体。

结论：大部分名称差异可以通过现有 concept 和命令级别名解决，单复数不能自动互转。

### 3.5 用户标识符命名、角色、单复数和值域混杂

#### 问题

用户参数同时存在以下差异：

- `--user` 可能是单个 userId，也可能承载逗号分隔列表；
- `--users` 可能是 userId 列表、openDingTalkId 列表或两者混合；
- `receiver`、`new-owner`、`ref-sender`、`applicant` 等参数保留了业务角色，但名字没有写明标识符类型；
- 同一个命令有时同时存在 `user` 和 `users`，错误的 `user-id` 可能对应多个合理目标。

该问题涉及28个命令，是最需要保护性约束的一类。

#### 可以先做的安全别名

以下命令的 `--user` 明确是单个 userId，可以审核 `user-id -> user`：

```text
chat conversation-info
chat group transfer-owner
chat group-role query-user
chat group-role remove-user
chat group-role set-user
chat message list
chat message list-direct
chat message send
chat +conversation-info
chat +messages-list-direct
```

#### 必须保护的情况

- 同一命令同时存在 `user` 和 `users`：对宽泛来源参数提示歧义；
- 真实目标是列表：不把单数 `user-id` 静默提升为列表；
- `users` 接受混合 userId/openDingTalkId：不归入单一 `user_ids`；
- 角色型参数：只允许同角色、同值域的命令级别名。

结论：现有兜底功能可以处理高置信度单值别名，也可以通过 block/ambiguous 防止错误执行，但不能安全地自动完成单值到列表或不同身份值域之间的转换。

### 3.6 机器人标识符值域不同

Chat 中至少存在：

```text
robot-code   # robotCode
bot-id       # openBotId
```

Skill 还会提到机器人开放用户标识。它们的名称相近，但值不能互换。

解决方案是分别维护 `robot_code` 和 `open_bot_id`，保留各自真实参数；禁止 `robot-id`、`bot-code` 等不明确的跨值域映射。

结论：这里的主要目标是防止错误兜底，不是增加更多自动别名。

### 3.7 Skill 与真实命令参数不一致

#### Skill 中存在、当前命令不存在

`chat message send` 的 Skill 仍列出：

```text
dentry-id
space-id
file-name
file-type
file-size
```

当前真实命令使用 `file-path` 完成本地文件上传和发送。旧参数集合不能通过一对一别名转换成 `file-path`。

#### Skill 漏写真参数

| 命令 | 漏写参数 |
|---|---|
| `chat group create` | `thread`、`type` |
| `chat group list-my-groups` | `exclude-muted` |

#### Skill 缺少精确命令参数段

```text
chat category list
chat group set-admin
chat group-mute
chat group-mute-member
chat message list-direct
chat set-top
```

四个 reaction 命令还存在真实兼容参数 `chat/group/id` 未在 Skill 中说明的问题。这些参数本身可以正常执行，Skill 应继续推荐 `conversation-id`，并在兼容说明中注明其他名称是原生命令支持的兼容参数。

结论：这类问题直接修改 Skill；`param_concepts.json` 不应该承担修复文档错误的责任。

## 4. 第一轮建议改造

### 优先级一：先避免错误执行

1. 将数字群号与开放会话 ID 从当前 `group_id` 边界中拆开；
2. 将群名称与开放会话 ID 分开；
3. 对用户单值/列表、多目标和混合值域配置 block 或 ambiguous；
4. 修正 Skill 中已不存在的参数和漏写参数。

### 优先级二：增加高置信度兼容

1. 新增 `open_message_id` 和 `open_message_ids`；
2. 对10个明确的单值 userId 命令审核 `user-id -> user`；
3. 为 `chat +send-to-group` 增加 `group-name -> group`；
4. 按命令逐步扩展 `open_conversation_id`，只接受值不变的一对一映射。

### 优先级三：收敛公开说明

1. Skill 为每类标识符推荐一个主要参数名；
2. 原生命令已经支持的兼容参数放在兼容说明，不由中央链路重复重写；
3. 建立产品级 Schema、Help、Skill 参数对账检查。

## 5. 当前能力支持不了或不应该做的事项

| 场景 | 原因 | 当前安全处理 |
|---|---|---|
| 数字群号自动变成开放会话 ID | 需要跨命令查询，参数值会变化 | 明确区分两种 ID，引导先查询 |
| 群名自动变成开放会话 ID | 需要搜索并处理零个、一个或多个候选 | 只保留两个快捷命令的本地搜索能力 |
| 单个 userId 自动变成用户列表 | 单复数和作用范围发生变化，同命令可能有多个目标 | block/ambiguous，引导使用真实列表参数 |
| Skill 的5个旧文件参数自动变成 `file-path` | 多个旧字段不能一对一生成本地文件路径和上传流程 | 修改 Skill，使用当前 `file-path` |
| 强制把已经存在的真实兼容参数统一改名 | 会改变 Cobra 的原生解析、参数冲突和脚本兼容行为 | 保持原生，只统一推荐口径 |

这些限制不阻塞第一轮 concept 治理。第一轮可以先完成安全别名、保护性拦截和文档修复。

## 6. 复用到其他产品的流程

后续分析 Calendar、Drive、Contact 等产品时，复用同一流程：

```text
盘点当前命令和真实参数
  → 对账 Schema / Help / Skill
  → 按业务实体归并参数
  → 找出异名、同名异义、单复数、角色和值域问题
  → 判断 concept / 命令级 alias / bind / block / ambiguous
  → 单列当前链路支持不了的查询、改值和多参数转换
  → 通过最终参数组装和原生解析测试验证
```

每个产品最终只需要维护同样的五个汇报页面：汇报总览、参数问题明细、兜底解决方案、当前无法解决、分析依据。

## 7. 相关文件

- 稳定 Schema：`internal/cli/schema_catalog.json`
- 兼容命令范围：`internal/cli/schema_command_exclusions.json`
- Chat Skill：`skills/mono/references/products/chat.md`
- 参数概念源：`internal/cli/param_concepts.json`
- 参数归一生成：`internal/cli/param_aliases.go`
- 运行时查询：`internal/cli/param_aliases_lookup.go`
- 语义别名处理：`internal/pipeline/handlers/semantic_alias.go`
