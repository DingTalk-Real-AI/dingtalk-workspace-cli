# IM Shortcut 命令名与参数幻觉合并治理方案

日期：2026-08-05
输入：51 个增量 Shortcut 静态审计 + `dws_multi-im-optimization` 全量实验 badcase 审计
本文件只定义实施与测试顺序，不在本轮修改运行时配置。

## 总原则

治理分成两层，顺序固定：

1. **命令层**：解析完整 path，处理真实命令/原生 alias/安全 rewrite/ambiguous/unknown。
2. **参数层**：只有命令 leaf 已唯一确定后，才运行原生 flag、`param_concepts`、block/ambiguous 和 Cobra 校验。

命令 fallback 不同时修改 flag；参数 fallback 不创建命令。这样才能避免“不存在的 Shortcut 带一个非法 flag，却先返回 unknown flag”的错误类型倒置。

## 第一阶段：先修错误优先级和命令名兜底

### 保持现有规则

- `+group-search → +chat-search` rewrite；
- `+search-group` 当前原生 alias；
- `+send-text`、`+send-single` 等写操作 ambiguous；
- 当前主分支真实 `chat group search` leaf，不再加 fallback。

### 建议新增 rewrite

| 来源 | 目标 | 准入理由 |
|---|---|---|
| `chat +conversation-detail` | `chat +conversation-info` | 单个会话详情、只读、唯一目标、能力与安全等级一致 |
| `chat +bot-list` | `chat +chat-bots` | 指定群机器人列表、只读、唯一目标；参数保留给目标校验 |

### 建议新增 ambiguous

| 来源 | 候选 |
|---|---|
| `chat +conversation-category-list` | `chat +category-list`、`chat +category-list-conversations` |
| `chat +conversation-group-list` | `chat +category-list-conversations`、`chat +conversation-list` |
| `chat +list-my-groups` | `chat +my-groups`、`chat +chat-list-mine`、`chat +chat-list` |

`chat +help` 不进入业务 fallback，直接提示 `dws chat --help`。

### 第一阶段测试

- 回放全部 16 条 Shortcut 命令名调用；
- 对 rewrite 断言只改 argv path、flag/value 顺序不变；
- 对 ambiguous 断言没有执行任何候选；
- 对不存在 path + 非法 flag 断言命令层结果优先；
- 对当前原生命令/alias 断言不触发 fallback。

## 第二阶段：扩展安全参数 concept

### 2.1 会话 ID

- 扩展 `open_conversation_id.commands` 到单一稳定 CID 角色的 51 子集；
- 对真实 `--id` 使用命令级 `bind`；
- 对 `+chat-members-list`、`+chat-update` 使用 scoped aliases，不把 name-or-ID 的 `group` 当全局稳定 CID；
- 对 `+chat-get-by-id` block 所有 CID 拼法；
- forward 类 src/dest 双角色对 generic CID 返回 ambiguous。

### 2.2 消息 ID

- 扩展 `open_message_id/open_message_ids` 到单角色 emoji、资源、Pin/Top 等命令；
- `+messages-reply` 只在命令内将 `msg-id/open-message-id` 归到引用消息；
- forward/topic/combine 保留 source 和单复数；
- bot recall 的 `keys` block 所有消息 ID 拼法。

### 2.3 用户与机器人

- mixed userId/openDingTalkId 参数只做命令级绑定；
- applicant/inviter、成员/群主、发送者/接收者不建立跨角色全局别名；
- `robot_code` 与 `open_bot_id` 分别扩围，并双向 block。

### 第二阶段测试

- 对每条新 alias 验证同实体、同角色、同基数、同值域、值原样传递；
- 对写命令验证归一化后目标、消息角色、确认门禁和下层 argv 不变；
- 对 block/ambiguous 验证不会进入 Runtime/MCP；
- 对原生隐藏兼容参数验证直接走 Cobra，不经中央别名。

## 第三阶段：只增加严格等价的分页/时间项

- `+flag-list limit → size`；
- `+messages-list start → time`；
- `+messages-list` 的 `before/before-time/end/page-all/count` 进入 block/unsupported；
- `+conversation-set-top` 只处理单/列表 CID 的明确拼法，`top/set-top` 不映射到反向语义 `off`；
- `+chat-list` 保持原生 `page-size/limit` 与 `page-token/cursor`。

这一步必须在前两阶段稳定后再做，避免分页同义词吞掉真实能力差异。

## 第四阶段：Schema/Skill 选路与评测

- 保持 51 个 leaf 全部发布到 Runtime Schema；
- 不把 51 条完整参数复制到根 Skill；
- 对高风险写命令、数字 groupId/CID、机器人 keys 和 source/dest 角色补 intent-guide 提示；
- 为 43 个没有在 Skill 精确点名的命令增加按需 leaf 发现/shortcut list 选路 fixture；
- 评测单独统计命令名幻觉、真实参数名幻觉、路径误报和参数值歧义，不能把所有 unknown flag 合并。

## 预计收益

- 直接消除当前未覆盖且唯一等价的两个 Shortcut 命令名；
- 对三类不唯一 list/category 名称从“误执行风险”降为可解释消歧；
- 覆盖 37 条与 51 子集直接相关的实验参数调用中的严格等价部分；
- 阻止数字 groupId/CID、messageId/processQueryKey、robotCode/openBotId 和单复数等高风险误映射；
- 让 138 条历史 `unknown flag` 路径误报回到正确的命令层错误。

## 不应承诺的效果

别名表不能解决群名查询、ID 值转换、单复数值变换、一对多参数生成、自动翻页、角色猜测或业务接口失败。此类 case 只能通过 resolver 型 Shortcut、真实命令能力、Skill 选路、明确消歧或后端修复处理。

## 完成门禁

实施后至少运行：

```text
go generate ./internal/cli
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
DWS_PACKAGE_VERSION=0.0.0-test go test ./...
```

并追加三组聚焦测试：全部命令名 badcase、全部 51 子集参数 badcase、全部 path-before-flag 错误优先级 badcase。由于实验版本没有 `+chat-list`，还要单独补该命令的当前合成测试。
