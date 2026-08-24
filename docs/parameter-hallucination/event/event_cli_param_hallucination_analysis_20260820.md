# Event 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、Cobra Help、Event Runtime、dingtalk-event Skill 及正式
`internal/cli/param_concepts.json`。未使用固定 Catalog、历史 badcase、用户 Shortcut
或已安装插件。当前工作区后续分支推进没有改变该冻结基线，也未被本分析改写。

Event 产品有 6 个 Agent 可见且可执行叶：`+listen-im`、`consume`、`list`、`schema`、
`status`、`stop`。正式别名表对 Event 完全没有 concept、override 或 fixture。主要风险
不是简单拼写，而是把四层接口混在一起：高频 IM 意图 facade、底层 EventKey 位置参数、
bus 本地投递过滤、订阅生命周期控制。典型错误包括把 `--event-key` 当 flag、把
`--events`/`--event-types` 当成同一种选择器、把姓名直接传给底层 consume、混用
userId/openDingTalkId/openConversationId、混淆 query/regex/Filter DSL、把运行时长当
订阅 TTL，以及在破坏性 stop 上把 subscribe_id 写成未知 flag。

候选已通过真实生成器、PreParse、5 组 alias/canonical 逐字节输出比较、9 组
block/ambiguous、非目标结构恒等、`internal/cli`、`internal/pipeline`、generated drift
和 Schema Catalog 政策。完整 `internal/app` 的唯一产品相关失败是 6 个 Event 命令均
缺 complete-command E2E 模板；正式状态为“规则与运行链路已验证，补齐 6 个模板后方可
落地”。

## 参数问题

### 1. `+listen-im` 的 kind、events 与 target 是一个受约束的意图编译层

`event +listen-im` 不是底层 EventKey 的另一种拼写。它用：

- `--kind=at-me|sender|group|all-direct|all-group` 选择监听范围；
- `--events=message,reaction,read,recall` 选择高层事件种类；
- `--user`、`--open-dingtalk-id`、`--user-query`、`--chat-id`、`--chat-query` 中最多一个
  选择目标。

`sender` 和 `group` 要求相应目标；`at-me/all-direct/all-group` 不接受目标且只支持
message；`--query` 只适用于纯 message。候选允许 `listen-kind/intent-kind`、
`event-kinds/event-types` 等精确同义写法，并把无角色 `target/id/name` 标为 ambiguous；
raw EventKey、订阅控制、Filter DSL 和输出路由在 facade 上全部 block。

### 2. userId、openDingTalkId、openConversationId 与自然名称不能互换

- `--user` 是一个 userId；
- `--open-dingtalk-id` 是一个 openDingTalkId；
- `+listen-im --chat-id` 与 `consume --group` 都是 openConversationId；
- `--user-query`/`--chat-query` 是 facade 内部唯一解析的自然姓名/群名。

候选扩展既有 `user_id`、`open_conversation_id`，新增单值
`open_dingtalk_id` concept，使同值域拼写在两个监听入口内归一。底层 `consume` 不做
自然名称解析，因此 `user-query/sender-name/chat-query/group-name` 明确 block；复数 ID
也不会自动缩成单值。

### 3. EventKey 是位置参数，`events` 和 `event-types` 都不是替代品

`event consume [event_key...]` 接受一个或多个公开 EventKey 位置参数；
`event schema <event_key>` 也要求位置参数。`consume --event-types` 只是 bus 到本地
consumer 的事件类型投递过滤，省略时按 EventKey 过滤；`+listen-im --events` 则是 facade
的四种高层事件种类。

中央参数表不能把 flag 重写成位置参数。候选因此在 `consume`、`schema` 上 block
`--event-key/--event-keys/--event`，在 facade 上 block raw EventKey；错误会在派发前提示
使用真实位置语法。没有把 `event-types`、`events` 和 EventKey 合并成一个 concept。

### 4. query、filter 与 filter-json 是三种不同过滤层

- `--query` 是逗号分隔的消息正文关键词，只适用于兼容的接收消息 EventKey；
- `--filter` 是客户端事件类型正则，下推到 bus；
- `--filter-json` 是个人事件订阅 Filter DSL JSON。

候选允许 message-query/message-keywords、event-type-regex/filter-regex、
filter-dsl-json/rule-json 的精确角色别名。`filter-file`、`filter-object`、`query-json`、
无角色 `where` 等被 block 或标记 ambiguous。多事件对这些参数还有 Runtime 约束，候选
不尝试按值推断或绕过。

### 5. duration、max-events、TTL 与全局 timeout 的单位和生命周期不同

`--duration` 是 consumer 运行上限，值为 Go duration；`--max-events` 是收到 N 条后退出
的整数；`--ttl` 是服务端订阅 TTL；全局 `--timeout` 是普通请求超时秒数，不控制事件流。

正式表已有 `calendar_duration_minutes`，其 `duration` 表示分钟数。真实生成器拒绝把同一
成员再放入 Event Go duration concept，候选因此使用命令级
`run/listen/runtime-duration → duration`，不污染日历单位。新增 `event_max_events`
concept；`subscription-ttl → ttl` 只在 consume 内精确映射。秒数、TTL 和 duration
之间不转换。

### 6. subscribe_id 在 consume/status 是 flag，在 stop 是位置参数

`consume --subscribe-id` 复用已有订阅，`status --subscribe-id` 过滤一个订阅；
`stop [subscribe_id]` 却要求一个位置参数，并与 `--all` 互斥。stop 是 destructive、
`confirmation=user_required`，必须先 dry-run，再经确认使用 `--yes`。

候选新增 `event_subscription_id` concept，只覆盖 consume/status 的 flag；stop 上的
`--subscribe-id/--subscription-id` 被 block，通用 `--id` 标为 ambiguous，避免破坏性
目标被静默猜测。`--all-subscriptions → --all` 是安全的同布尔含义映射，且 dry-run 输出
与 canonical 逐字节相同。

### 7. 输出/控制参数与 Help 隐藏兼容面容易被误解

`consume --output-dir` 是每事件一个文件的目录；`--route` 是可重复的 regex 路由；
`--flatten` 改业务字段投影；`--compact` 是渲染提示。控制面还包含
`personal-event-base-url`、`stream-source-id`、`stream-ticket-url/mode`，这些 URL、ID
和 mode 角色不能靠泛化 `url/source-id/mode/directory` 猜测。

此外，`event list/status` 的 Cobra 树仍注册并隐藏内部 `--all`、`--all-editions` 等 flag，
但个人事件路径 Runtime 会明确拒绝它们。生成器禁止把真实 flag 配为 block，因此候选不
覆盖该原生 guard；这属于 Help/执行兼容面，应由命令声明清理，而不是别名表改写。

## 当前别名表可以实施的方案

1. 扩展 `user_id`、`open_conversation_id`、`search_query` 到两个精确 Event 监听入口。
2. 新增单值 `open_dingtalk_id`、事件最大条数 `event_max_events`、订阅 ID
   `event_subscription_id` 三个 concept。
3. 为 6 个叶声明意图、EventKey 位置语法、过滤层、生命周期、输出目录、状态与 stop
   目标的 scoped alias、block 和 ambiguous。
4. 保持所有 alias 值原样传递；不解析自然名称、不改 ID 值域、不改单位、不读取过滤
   文件、不把 flag 改成位置参数。
5. 为全部 6 个 Event active 命令补 complete-command payload 模板后，再评审正式替换。

## 当前能力支持不了的事项

- 把 `--event-key`/`--subscription-id` 之类 flag 改写为位置参数；
- 把姓名或群名在底层 `consume` 中自动解析为唯一 ID；
- 自动转换 userId、openDingTalkId 与 openConversationId；
- 把单值 ID 与复数 ID 自动互转；
- 把 facade 的 `message/reaction/read/recall` 转成任意 raw EventKey 组合；
- 把 query、正则和 Filter DSL JSON 互相包装或从文件读取；
- 把秒数、分钟数、Go duration、TTL 或全局 timeout 互换；
- 自动选择 `--output-dir` 与 `--route`，或把文件路径当目录；
- 用别名表消除隐藏内部 flag 或修改 Runtime 的多事件兼容性约束；
- 在没有 complete-command 模板时直接替换正式表。

这些情况应停止并提示精确语法，或先调用 `+listen-im` 的解析层；不得为了继续监听或停止
订阅而猜测目标。

## 第一轮改造建议

第一轮建议落地 typed target、facade kind/events、消息 query、运行边界、订阅 ID、目录/
过滤角色、catalog/status/schema/stop 的低风险别名和保护。落地 PR 必须同步为以下 6 个
命令补 complete-command E2E 模板：`event +listen-im`、`event consume`、`event list`、
`event schema`、`event status`、`event stop`，覆盖 22 个 active fixture。另行清理
`list/status` 隐藏内部 flag 的声明与 Help 同源问题，不把该问题塞进别名表。

## 候选 `param_concepts.json` 改动与审核

候选文件是冻结提交正式表的完整副本，不是增量片段。相对冻结正式文件：

- 修改 3 个既有 concept 的精确 Event 命令范围；
- 新增 3 个 Event 专用 concept；
- 新增 6 个 Event command override；
- 新增 30 个审核 fixture，其中 22 个是 active alias fixture；
- `go generate ./internal/cli` 从 569 个命令作用域变为 575 个；
- 还原 Event 改动后，非目标 concept、override、fixture 与正式表结构完全相同；
- 生成 Go 差异只新增 6 个 Event 条目，command path fallback 无变化；
- 5 组代表 alias/canonical 命令退出码与完整输出逐字节相同；
- 9 组错误输入分别稳定返回 `blocked_flag` 或 `ambiguous_flag`。

候选位置：`docs/parameter-hallucination/event/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与真实生成器 | 通过 | `go generate ./internal/cli`，575 个命令作用域 |
| PreParse 与 alias/canonical 输出 | 通过 | listen、consume、stop-all、list、event schema 五组逐字节一致 |
| block/ambiguous | 通过 | 位置参数、自然目标、ID 角色、stop 目标等 9 组均在业务派发前停止 |
| 原生参数 | 通过 | canonical flags/positionals 保持原生；隐藏内部 flag 继续由 Runtime guard 拒绝 |
| 非目标回归 | 通过 | 非目标 JSON 结构恒等；生成 diff 仅 6 个 Event 条目；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | 隔离冻结副本执行，CLI 106.791 秒 |
| generated drift | 通过 | 双次 alias 与 Schema 装配 hash 一致 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具 |
| 完整 `internal/app` | 未通过 | 238.083 秒；唯一产品相关失败为 complete-command 模板 |
| complete-command payload 门禁 | 未通过 | 200/206 个活跃命令已有模板；Event 缺 6 个命令、22 个 active fixture 模板 |

正式替换前必须补齐 6 个模板并重跑完整 `internal/app` 和政策门禁；未完成前，本候选只
作为完整待审核草稿。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `b3d0a0ddb6fa067222742b96c28130f44a62df85e7b55d699497f15ff0e39d36`。
- 命令实现：`internal/app/event_command.go`、`internal/app/event_personal_command.go`、
  `internal/app/event_listen_im.go`；运行时：`internal/event/`。
- Skill：dingtalk-event 根 Skill，以及 EventKey、OA、生命周期、订阅运维四份 reference。
- Schema 来源：同一冻结二进制运行时声明组装；未使用历史或固定 Schema Catalog。
