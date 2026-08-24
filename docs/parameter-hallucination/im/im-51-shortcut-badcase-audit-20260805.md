# dws_multi-im-optimization 全量实验 Badcase 审计

审计日期：2026-08-05
数据目录：`/Users/hyz/works/data/dws_multi-im-optimization`
目标：区分真实参数幻觉、Shortcut 命令名幻觉、子命令层级错误、错误类型误报和非幻觉运行时错误。

## 数据口径

- 扫描了目录下全部 480 个 `turn_*.rounds.json`、46 个候选 run 级 report 以及对应 manifest。
- 同一实验存在原始 rounds 时只以 rounds 为准；没有原始轨迹时才读取 `report_*_runN.json`。聚合 report、Markdown、HTML 和 rawEvents 不重复计数。
- 最终得到 6,109 条去重后的 DWS Bash 调用，覆盖 1,482 个实验/模式/Case/Run 组合。
- 其中 632 条有非零退出或明确 unknown/ambiguous 错误标记；其余成功调用不进入 badcase 计数，但用于确认 Case 覆盖。
- 一条 Bash 命令包含 `||` 等多个 DWS 探测时仍按一次工具调用计数；链内额外命令名作为补充证据记录，不虚增主要调用数。

## 版本核验

| 数据组 | 命令面依据 | 可信度 |
|---|---|---|
| `三组对比结果/dws_分支` | 报告明确记录 `b8b55834402c...`，已重建该提交 Cobra 面 | 精确 |
| `20260804_ee943...` 与对应 audit | manifest 的 `ee943d9b3f58...` | 精确 |
| `f050fbde` 系列及 `dws_f050fb` | manifest/目录记录 `f050fbdebca9...` | 精确 |
| `audit_...987c63d...` | manifest 的 `987c63d99cc4...` | 精确 |
| `三组对比结果/dws_main` | 采用实验日期附近 beta.2 main release head `18778704`，并以轨迹真实错误复核 | 近似基线 |

`b8b55834`、`ee943d9b`、`f050fbde` 对 51 个目标命令都只有 50 个，统一缺少 `chat +chat-list`。因此实验不能证明 `+chat-list` 的参数表现。

## 总体结果

| 分类 | 调用数 | 判断 |
|---|---:|---|
| 真实参数名幻觉 | 102 | 命令 leaf 在对应实验版本真实存在，但错误 flag 不存在 |
| Shortcut 命令名幻觉 | 16 | 包含 10 条同时携带非法 flag 的复合调用 |
| 兼容分派节点误当可执行 leaf | 192 | 主要是历史 `chat group search`，带 flag 先报 unknown flag，去掉 flag 才暴露 ambiguous |
| 其他子命令路径幻觉 | 25 | 其中 15 条同时被误报为 unknown flag |
| Schema 查询路径不存在 | 25 | 错误 leaf 的 Schema 探测证据，不是业务执行参数错误 |
| 参数值解析歧义 | 4 | 同名群等 resolution_ambiguous，不是参数名幻觉 |
| 后端业务错误 | 134 | 参数已通过本地解析，失败发生在接口/业务层 |
| 确认门禁 | 20 | 预期安全行为 |
| 其他运行时/探测/非 Chat 错误 | 116 | 排除出本次 Chat 参数与命令名治理 |

上述前五类合计 360 条命令路径或参数幻觉调用，涉及 238 个 Case-Run。

## `unknown flag` 不能直接等于参数幻觉

全量共有 251 条调用输出 `unknown flag`。按“先解析完整命令路径，再校验 flag”的顺序复核后：

- 138 条应首先归为不存在的 Shortcut、错误子命令层级或兼容分派节点；
- 102 条才是真实参数名幻觉；
- 其余 11 条属于非 Chat 产品或探测边界。

典型问题是历史 `dws chat group search --keyword/...`：当时该路径只是无稳定 leaf 参数契约的兼容分派节点，带 flag 先报 `unknown flag`；无 flag 才会返回 `ambiguous command "search"`，Schema 也返回 `unknown runtime schema path "chat group search"`。当前 main 已把它升级为真实 leaf，并接受 `query/keyword/name`，因此治理方式是保留当前子命令判断修复与回归测试，不再为它增加 Shortcut fallback。

## 51 个目标 Shortcut 中捞到的参数 badcase

37 条幻觉调用与 51 子集精确相交，分布如下：

| 命令 | 次数 | 错误 flag | 结论与建议 |
|---|---:|---|---|
| `+chat-update` | 10 | `id/chat-id/conversation-id/open-conversation-id`，且部分还有 `title/new-title` | CID 拼法与 `title/new-title→name` 可命令级处理；generic `id` 需要值域保护 |
| `+flag-list` | 9 | `limit/max/max-results/max-size` | 只确认 `limit→size` 严格等价；`max*` 不自动吞掉 |
| `+chat-members-list` | 4 | `chat-id/id/query` | `chat-id/id→conversation-id` 可；`query` 是当前不支持的成员过滤能力 |
| `+conversation-set-top` | 4 | `open-conversation-id/chat-ids/groups/top` | 单/列表 CID 可 scoped；`groups/top/set-top` 不可一对一改写 |
| `+chat-members-get` | 3 | `conversation-id/group`，并伴随 `open-dingtalk-ids` | `conversation-id→id`、`open-dingtalk-ids→users` 可；群名不能直接当 CID |
| `+chat-get-by-id` | 2 | `group`，值为 CID | 不能兜底到数字 `group-id`，必须 block |
| `+messages-list` | 2 | `count/max-results` | 页大小/总量口径不明；优先路由 `+chat-messages` |
| `+messages-reply` | 2 | `group/msg-id` | `msg-id→ref-msg-id` 可；群名→CID 需要 resolver，不能靠别名 |
| `+messages-recall` | 1 | `message-id` | 当前 main 已作为隐藏原生兼容参数，保持原生即可 |

这里的 37 条只统计请求命令本身属于 51 子集。若一个不存在的命令名可能映射到其中某个命令，但命令名本身有多个合理目标，则放在 Shortcut 幻觉治理，不强行归入参数统计。

## Shortcut 命令名幻觉与兜底建议

| 幻觉命令 | 观察 | 当前状态 | 建议 |
|---|---|---|---|
| `+group-search` | 4 条实际调用，另有同类探测 | 已有 rewrite 到 `+chat-search` | 保持；目标参数仍由真实命令校验 |
| `+search-group` | 2 条 | 当前已是 `+chat-search` 原生命令别名 | 不增加 fallback |
| `+send-text` | 1 条 | 已有 messages-send/dm/send-to-group ambiguous | 保持停止选路，写操作不自动猜 |
| `+send-single` | 1 条 | 已有 dm/messages-send ambiguous | 保持停止选路 |
| `+conversation-detail` | 1 条，意图是单会话详情 | 未覆盖 | 可安全 rewrite 到只读 `+conversation-info` |
| `+bot-list` | 1 条，意图是查看某群机器人 | 未覆盖 | 可安全 rewrite 到只读 `+chat-bots`；错误 flag 继续由目标暴露 |
| `+conversation-category-list` | 1 条，链内还猜了 `+conversation-category` | 未覆盖 | ambiguous：`+category-list` / `+category-list-conversations` |
| `+conversation-group-list` | 1 条 | 未覆盖 | ambiguous：`+category-list-conversations` / `+conversation-list` |
| `+list-my-groups` | 1 条 | 未覆盖 | ambiguous：`+my-groups` / `+chat-list-mine` / 当前 `+chat-list` |
| `+help` | 2 条 | 未覆盖 | 不建业务 fallback；明确提示 `dws chat --help` |
| `+messages-send` | 1 条出现在旧 `987c63d` 版本 | 当前是原生命令 | 版本升级已解决，不增加 fallback |

新增 rewrite 只建议两个：`+conversation-detail → +conversation-info`、`+bot-list → +chat-bots`。三条 list/category 名称没有唯一语义，应记录为 ambiguous，而不是根据单个 Case query 固化成全局 rewrite。

## 对兜底层的要求

1. 先解析完整命令路径；不存在的 path 不得先消费其 flag 并返回 `unknown_flag`。
2. Shortcut rewrite 只解决命令名，不同时偷改参数；目标命令必须继续执行自己的参数校验。
3. 写操作只要发送身份、接收者类型或能力边界不唯一，一律 ambiguous。
4. 当前主分支已真实存在的命令或 alias 不重复进入 fallback 表。
5. 实验中的组合问题保留主次：主分类是命令名/层级，目标映射后的非法 flag 作为次要证据进入参数测试。
6. 历史版本问题与当前问题分开：已经由真实命令面或原生兼容参数解决的，不再追加中央兜底。

## 测试建议

- 对 16 条 Shortcut 幻觉逐条建立 path-only 回放，先断言 rewrite/ambiguous/原生命令/明确拒绝。
- 对 102 条真实参数调用做去重 fixture；同一命令/flag/预期只保留一条结构化断言，原始 Case 引用全部保留。
- 对 138 条误报建立错误优先级测试：不存在 path 必须返回 command/subcommand/fallback 结果，而不是 unknown flag。
- 对 `chat group search` 建历史回归：当前 main 应作为真实 leaf 接受 `query/keyword/name`，Schema 查询也应命中。
- 对 37 条 51 子集 badcase 建 param_concepts/native/block/unsupported 四类断言。
- 对没有实验样本的 `+chat-list` 用当前 Cobra/Schema 合成测试补齐。

完整 632 条异常调用、360 条幻觉明细、102 条真实参数明细和 16 条 Shortcut 命令名明细见 badcase 审计工作簿。
