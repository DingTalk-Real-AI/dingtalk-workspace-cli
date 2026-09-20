---
name: dingtalk-aisearch
description: AI搜问：人员语义搜索、跨源主题检索与行为回溯。Use when 语义找人，或目标未知时按主题或行为发现内容。原生最近列表走所属产品；完整手机号精确反查走 dingtalk-contact。前缀：dws aisearch。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 搜问 Skill

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小执行契约

- 只用 `dws` 操作钉钉，每条命令带 `--format json`，只按结构化返回下结论。
- `person/enterprise/behavior` 直接调用，不预读 shared、Reference、Schema、Help 或下游 Skill。
- 不猜命令、字段、ID、profile 或事实；多候选不取首项，不混用不同 `domain` 的 ID。
- 空结果结束本次搜索；失败、不完整或候选结果不能表述成“对象不存在”、全量或唯一。
- 来源不确定或用户列出多个来源时，第一条业务命令必须是一次合并 `--types` 的 `aisearch enterprise`，不能先逐产品搜索。
<!-- DWS_RUNTIME_CONTRACT_END -->

## 选路

| 意图 | 入口 |
|---|---|
| 姓名、工号、部门、职位、职责、上下级或手机号线索找人 | `dws aisearch person` |
| 按主题跨文档、消息、邮件、待办、日程、听记等找内容 | `dws aisearch enterprise` |
| 以我为端点的发送/接收，或我创建、编辑、分享过什么 | `dws aisearch behavior` |
| 仅 IM 且需逐条消息谓词过滤 | `dws chat +search-msg` |
| 无主题地列最近访问/编辑文档 | `dws drive +recent` |
| 枚举部门完整成员 | `dingtalk-contact` |
| 完整手机号精确反查 | `dws contact user search-mobile --mobile "<手机号>" --format json` |
| 已知稳定 ID 后读取/修改 | 对应产品 Skill |

顺序：资源范围 → 答案形态 → 原生谓词。

## 1. 人员搜索

维度：姓名=`name`，部门=`department`，职位=`position`，职责/负责人=`duty`，上级=`supervisor`，下属=`subordinate`，工号=`jobNumber`，手机号线索=`phone`；确实无法判断才用 `all`。

```bash
dws aisearch person --query "<用户原始目标>" --dimension <维度> --format json
```

- 保留完整目标；独立条件分别查。正确维度为空就结束，不换 `all`、缩词或扩同音词。
- 用 `searchEvidence.returnedCandidateCount` 报告本次候选数；无分页完成证据只称“本次返回”。
- 姓名/花名包含查询只交付 `_searchEvidence.exactAliasContainsQuery=true`；`exactAliasContainsCount=0` 即无精确包含项，语义近似不能替代。
- `resolvedRelation.status=resolved` 时直接使用 `target/supervisor/relation` 回答直属上级，不再搜索，也不把上级的上级当直属上级。
- 产品归属与负责人用一次 `dimension=duty`，保留产品名和“负责”；仅职责证据明确者可称负责人，多候选就列候选及部门，共同上级不等于负责人。
- 姓名与花名可由同一候选的 `aliases` 关联；不同域 ID 不可直接比较。需要更多人员详情时才用真实 `userId` 切 Contact。

## 2. 跨源内容搜索

时间词只进 `--time-range`，类型词只进 `--types`，其余主题进 `--queries`。

```bash
dws aisearch enterprise --queries "<主题>" --types <类型CSV> [--time-range "<原时间词>"] --format json
```

类型：文档/普通文件=`document`，消息=`im`，邮件=`mail`，日程=`calendar`，待办=`todo`，听记=`minute`，日志=`report`，AI表格=`notable`，知识内容=`baike`；用户列出的来源合并到一次调用。

- `requestConstraints` 回显实际条件，`sources` 给出各来源数量；`no_returned_items` 仅表示本次未命中，`coverage.status=unknown` 不能证明完整。
- `_searchEvidence.queryMatch=text_evidence_present` 才有文本主题证据；`no_text_evidence` 仅是语义候选，标为未核实。
- 只交付 `result` 保留项并说明 `delivery.omitted`；`doNotRetryOrExpand=true` 时停止，不改词、逐产品补搜或读 Help。
- 唯一精确标题可能附带 `resolvedDetail`。其状态为 `resolved` 或 `resolved_from_search_payload` 且字段足够时直接回答；`failed` 才报告补充读取失败。
- 日程空命中时接口可按原时间调用日历，`meta.nativeFallback=true` 仍属本次 AISearch 证据，无需 Agent 再查日历。
- 精确标题未命中不拿近似标题、最近项或首项替代。“唯一才读取”须有指定来源覆盖、分页结束和唯一精确匹配；正文 `complete=true` 不代表搜索完整。
- 用户只要候选摘要或链接时不读原文；确需正文且同次结果缺字段，才按真实稳定 ID 读取对应产品。

## 3. 行为回溯

```bash
dws aisearch behavior --queries "<主题>" --types <类型CSV> --behavior-type <all|send|receive|create|edit|share> [--time-range "<原时间词>"] [--direction "我->某人|某人->我|我<->某人"] [--chat-scope "<完整群名>"] --format json
```

- 每组动作、方向、时间调用一次并合并类型。方向按当前用户视角：“我发给某人”=`send`；“某人发给我”=`receive`。`chat-scope` 仅用于 IM，方向保留原姓名。
- 成功空数组就交付“已查询，本次 0 条”，按来源列 0；不再用 `enterprise` 拼行为证据，不删主题、扩时间、缩群名或用 recent 代替。
- `timeEvidence.withinRequestedRange=false` 排除；为 `null` 则标时间未核实。`messageEvidence.perMessageVerified=false` 时不能把混合会话 snippet 全归给目标人。
- “某人发过 Word 文件”或“某人今天发给我的消息”只调用一次 behavior：`types=im`、`receive`、人物方向及原时间词；不切 Chat、猜 sender 或扩大年份。
- `resolvedMessageEvidence.status=resolved` 时直接使用 `messages`，即使为空也不再调用 Chat；`identity.match=returned_alias` 只证明展示名匹配返回别名。
- 发送者未知的“谁给我发过某主题文件”用 `direction="某人->我"`，按 `identity.match=any_returned_sender` 的 `messages.sender` 汇总，不先猜人或拆两次搜索。

## 4. 核验与交付

- **时间**：保留原时间词；“本周”不等于近七天。确认数值时间单位。创建行为用创建时间，不能用修改时间或会议开始时间代替。
- **身份**：核对发送者、收件关系和创建者；群内可见不等于发给我。优先用 `stableRefs/identityRefs`，仅同 `domain` 的 ID 可直接比较。
- **主题/类型**：须有文本、附件或对象类型证据；群名、普通链接和搜索命中本身不能证明主题或文件属性。
- **唯一/全量**：检查来源和分页终态，按稳定 ID 去重；只问“有没有”时，有效正向命中即可。
- **摘要**：不补写未返回字段，不把建议写成事实，不把 snippet 称正文；分别报告命中、空结果、失败和未核实项。

## 5. 同条件原生核验与多跳

路径：**AISearch 定位 → 确认目标 → 优先使用同次 `resolvedDetail` → 缺必需字段才按真实 ID 调用一个对应产品。**

- 仅在需要详情、完整成员或证据核验，且已有稳定 ID 或原生可执行的相同精确标题/部门/范围时转用对应产品；保持主题、身份、方向、时间和权限，不扩大扫库。
- 跨来源第一步固定为 enterprise。无稳定 ID 时不能用原生重搜替代；目标不明、身份矛盾、失败或无权限时说明限制并停止。
- 人员用 `userId`，文档用 `nodeId`，听记用 `taskUuid`，待办用真实 `taskId`；Ding uid 不能当 Contact userId，snippet 不能当正文。
- 用户的“唯一才读取”等前置条件仍须满足。ID 域或下一跳不明确时才读 [多跳短流程](references/lite-recipes.md)。

## 6. 禁止本地兜底

钉钉对象只用 DWS 返回作业务证据；未命中时不得用 `find`、`rg`、`mdfind` 或评测 fixture 回答。会议议题负责人用一次 `person --dimension duty` 保留会议与职责原词；只有内容能提供职责原文时才改用一次 `enterprise --types calendar,im,document`，二者不都跑。

## 错误与成本

1. `retryable=true` 时原调用最多重试一次；否则停止，不换身份或绕权限。
2. 成功但为空或无精确目标：不重复、改词或扩时间。
3. `unknown flag` 才查看一次该命令 Help；API、权限和空结果不靠 Help 解决。
4. 独立来源可继续；依赖失败结果的后续停止。保留来源、链接、ID、范围、数量、完整性和错误。

## 按需 Reference

常用路径不读 Reference。低频枚举或兼容参数不明才读 [完整命令参考](references/aisearch.md)；产品边界不明读 [局部意图消歧](references/intent-guide.md)；稳定 ID 域或下一跳不明读 [多跳短流程](references/lite-recipes.md)。
