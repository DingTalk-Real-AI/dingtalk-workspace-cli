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
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；每条命令带 `--format json`，只按真实结构化返回下结论。
- `person/enterprise/behavior` 按本页直调，不预读 shared、Reference、Schema、Help 或下游 Skill。
- 不猜命令、字段、ID、profile 或事实；缺失可选时间则省略；多候选不取首项，ID 不混域。
- 空结果结束搜索，同条件核验见第 5 节；失败或不完整不能说“没有”，候选不等于全量。
- 用户说“不确定在哪种来源”“文档或消息”“待办或消息”“资料还是邮件”时，第一条业务命令必须是一次合并 `--types` 的 `aisearch enterprise`。先取得候选和稳定 ID，满足条件后才加载一个原生产品 Skill；不能先逐产品搜索。
<!-- DWS_RUNTIME_CONTRACT_END -->

## Golden Route

| 意图 | 唯一首选入口 | 关键槽位 |
|---|---|---|
| 姓名/工号/部门/职位/职责/上下级/手机号线索找人 | `dws aisearch person` | `--query` + `--dimension` |
| 按主题找文档、消息、邮件、待办、听记等内容 | `dws aisearch enterprise` | `--queries` + `--types` + 可选 `--time-range` |
| 以我为关系端点的发送/接收，或我创建、编辑、分享过什么 | `dws aisearch behavior` | 上述内容槽位 + `--behavior-type` + 可选 `--direction/--chat-scope` |
| <!-- dws-intent: chat.search.filtered -->资源只限 IM，答案是逐条消息并带结构化消息谓词 | `dws chat +search-msg` | 发送者、会话、关键词、@、类型、reaction、时间和完整分页由 Chat 负责 |
| 按时间列最近访问/编辑文档，无主题或行为条件 | `dws drive +recent` | 文档集合排序；其他对象用所属产品 recent/list |
| 枚举部门成员、完整人员名单 | `dingtalk-contact` | 部门定位 → 成员列表 → 按需详情，不把人员搜索候选当全量 |
| 完整手机号精确反查 | `dws contact user search-mobile --mobile "<完整手机号>" --format json` | `--mobile` |
| 已知稳定 ID 后读取/修改原对象 | 对应产品 Skill | 不再用 AISearch 重搜 |

选路顺序：资源范围 → 答案形态 → 原生谓词。

## 1. 人员搜索

维度映射：姓名→`name`，部门→`department`，职位/岗位→`position`，职责/技能/负责人→`duty`，上级→`supervisor`，下属→`subordinate`，工号→`jobNumber`，手机号线索→`phone`；确实无法判断维度时才用 `all`。

```bash
dws aisearch person --query "<用户原始目标>" --dimension <维度> --format json
```

- 独立条件分别查询、汇报；保留完整目标，不截名、改昵称或扩同音词。
- 正确维度返回空结果就结束该组，不换 `all` 或缩词扩搜；同条件核验见第 5 节。仅维度选错时改正一次，空结果不等于路由错误。
- 返回同时包含 `searchEvidence.returnedCandidateCount` 和逐候选 `_searchEvidence.aliases/identityRefs`。答复先明确“已完成查询，本次返回 N 个候选”；N=0 仍是查询完成。姓名与花名属于同一候选的别名，可用于解释后续产品返回的展示名；不同 `domain` 的 ID 仍不能直接比较。
- 用户明确要求姓名或花名“包含”某字符串时，只交付 `_searchEvidence.exactAliasContainsQuery=true` 的候选；`searchEvidence.exactAliasContainsCount=0` 就回答本次没有精确包含项，不把语义近似候选充当包含匹配。
- 查询“某人的直属上级”时，接口可能在同次返回中给出 `resolvedRelation`。`status=resolved` 时以其中 `target/supervisor/relation` 回答，不要把普通候选的“上级的上级”关系当直属上级，也不要再发第二条人员搜索。
- 查询“某产品归属部门及谁负责”时，保留产品名和“负责”职责语义做一次 `dimension=duty` 搜索；候选中只有职责证据明确者才能称负责人，多候选则列候选和共同部门，不把共同上级自动当负责人。
- 保留全部候选的姓名、真实 ID 和人员链接。用户要详情才切 Contact： `dws contact user get --ids <userId> --format json`。
- 同一人的多条件须核对交集；姓名、部门、职位不互相替代。
- 无分页完成证据时，只称“本次返回 N 个候选”。

## 2. 跨源内容搜索

先从原句拆槽：时间词只进 `--time-range`，类型词只进 `--types`，剩余主题只进 `--queries`。类型枚举：`document,im,mail,calendar,todo,minute,report,image,link,notable,baike`。

```bash
dws aisearch enterprise --queries "<主题>" --types <类型CSV> [--time-range "<用户原始时间词>"] --format json
```

- 按用户要求分组调用，组内类型合并为 CSV；不按底层产品细拆或重复搜索。
- 来源词直接映射：知识内容/知识条目=`baike`，资料/文档/普通文件=`document`，群消息=`im`，邮件=`mail`，待办=`todo`，日程=`calendar`，听记=`minute`。用户列出的来源必须全部进入同一次 `--types`；例如“知识内容、文档和群消息都看”使用 `--types baike,document,im`。
- 先看 `searchEvidence.requestConstraints` 与 `sources`：它们分别回显实际条件和各来源返回数量。`no_returned_items` 只表示该来源本次没有候选；`coverage.status=unknown` 时不能声称全量或唯一。
- 每条候选看 `_searchEvidence.queryMatch`：`text_evidence_present` 可作为主题证据；`no_text_evidence` 只是后端语义候选，不能当成已确认相关，也不能据此完成“列出相关对象”的任务。
- `searchEvidence.delivery` 已把明确越界结果和冗长的无文本证据长尾折叠掉。只交付 `result` 中保留的候选，并说明 omitted 计数；`doNotRetryOrExpand=true` 时直接回答，禁止改词重搜、逐产品补搜或为了补齐候选读 Help。
- 精确标题在某来源仅有一个候选时，接口会在同一次命令内按稳定 ID 补充 `resolvedDetail`。`status=resolved` 或 `resolved_from_search_payload` 且字段足够回答时，直接交付并停止；不要再加载下游 Skill、重复搜索或再次读取。`status=failed` 才说明补充读取失败。
- 请求含 `calendar` 且时间词可机器解析时，接口可在 AISearch 无日程命中时按原时间范围调用日历 MCP；这类候选标记 `meta.nativeFallback=true`，仍属于同一条 AISearch 命令的原条件核验，不需要 Agent 再发日历命令。
- 精确标题原样传给 `--queries`，只接受精确匹配；未命中就停止，不拿近似标题、最近项或首项替代。同条件原生核验见第 5 节。
- “唯一才读取”须先证实指定来源覆盖、分页结束且仅一个精确匹配；否则报告唯一性未核实，不读正文。正文 `complete=true` 不代表搜索完整。
- 只要候选摘要或链接就不读原文。需要正文或缺必需证据时，满足读取前置条件后才按真实 ID 切对应产品；核验不能绕过“唯一才读取”。
- 空结果或无精确目标均报告本次未命中，不自行缩词或扩时间。

## 3. 行为回溯

```bash
dws aisearch behavior --queries "<主题>" --types <类型CSV> --behavior-type <all|send|receive|create|edit|share> [--time-range "<时间>"] [--direction "我->某人|某人->我|我<->某人"] [--chat-scope "<完整群名>"] --format json
```

- 每组“动作＋方向＋时间”调用一次，类型用 CSV 合并。`chat-scope` 仅用于 `im`；方向用原姓名，不先查邮箱或 userId。行为方向不代表发送者全部消息。
- 动作按当前用户视角选择，适用于所有内容类型：“我发给某人”＝`behavior-type=send, direction=我->某人`；“某人发给我”＝`behavior-type=receive, direction=某人->我`。不能因原句有“发”就选 `send`。“我在某群发过”另加 `types=im, chat-scope=<完整群名>`。
- 仅 IM 逐条过滤走 Chat，复用已解析的稳定身份。
- 空结果只表示本次未命中；不删除主题、不追加同义词、不缩短群名重搜，不用 recent 列表替代行为证据。需补充查询时按第 5 节保留原条件执行。
- `behavior` 在原条件下成功返回空数组时，任务应直接交付“已完成行为查询，本次 0 条”，并按来源列出 0；禁止再调用 `enterprise` 生成无法证明该行为的内容候选。
- 若 `_searchEvidence.timeEvidence.withinRequestedRange=false`，该候选明确越界并排除；值为 `null` 时只能说时间未核实。IM 的 `messageEvidence.perMessageVerified=false` 表示 snippet 仍是混合会话片段，需逐条确认发送者，不能把整段归给目标人。
- “某人发过一个 Word 文件，帮我找”“某人今天发给我的消息，帮我总结”属于行为线索发现：只调用一次 `aisearch behavior`，分别保留“Word 文件”或空主题、`types=im`、`receive`、人物方向和原时间词。用户没有明确要求全量分页时，返回候选足以交付；不再切 Chat、猜 sender 参数或扩大年份。
- 上述人物定向 IM 查询若返回 `resolvedMessageEvidence.status=resolved`，它是同一 AISearch 命令内部按人员别名、原时间范围读取并过滤的消息证据。直接使用 `messages` 回答；即使为空也不要再次调用 Chat。`identity.match=returned_alias` 只证明发送者展示名命中人员搜索返回别名，答复中保留这一证据边界。
- 用户问“谁给我发过某主题的文件”而发送者未知时，方向写 `某人->我`；接口会返回 `identity.match=any_returned_sender` 的逐条消息证据。按 `messages.sender` 汇总，不先猜姓名，也不拆成 document、im 两次查询。
- 返回已足够回答就停止；缺少必需信息时才查询原对象。

## 4. 结果核验与交付

- **时间**：原时间词传给 `--time-range`，按当前日期、时区确定核验区间；“本周”不等于近七天。数值时间先确认秒/毫秒单位，再用程序换算。已知越界记录排除；只有聚合日期或缺少逐条时间时标为时间未核实。创建行为须有创建时间，不能用修改时间或会议开始时间代替。
- **身份**：核对实际发送者、收件关系和创建者；群内可见不等于发给我。Contact `userId` 与 Ding uid 等不同域 ID 不能直接比较；没有明确映射时，既不能认定同一人，也不能因值不同就排除本人。
- **姓名与展示名**：用户给实名、下游返回花名或客户端展示名时，先用 `aisearch person --dimension name` 取得同一候选的 `aliases`。最终说明“人员搜索返回实名 A、花名 B；消息显示 B”，不要把字符串不同直接判成他人，也不要在没有别名证据时强行视为同一人。
- **ID 域**：优先使用 `_searchEvidence.stableRefs` 和 `identityRefs`。只有 `domain` 相同的 ID 才能直接比较；`aisearch.actor`、`aisearch.creatorId` 与 `contact.userId` 默认不是同一域。
- **主题与类型**：候选须有内容证据支持主题关联；搜索命中、群名相关或流程上“可能有关”都不足以确认。无关联证据时标为相关性未核实，不自行选成“最相关”。文本、标题或普通链接不能充当文件证据。
- **唯一与全量**：检查指定来源和分页终态；缺完整性字段就不声称唯一或全量。按稳定 ID 去重，交付清单与已核实的 ID、数量核对，避免读到却漏列。只问“有没有”时，有效命中即可回答。
- **忠实总结**：不补写未返回事实，不把“建议、待确认”改成已发生；分清搜索片段与正文、接口总数与已读取条数。部分结果伴随错误或未完分页时保留说明，不能据此说“没有其他结果”。

## 5. 按原条件转用对应产品查询

- 需要详情、成员清单或核验时，用返回的真实 ID，或原生产品支持的精确标题、部门等条件查询；只加载对应产品 Skill，按其参数调用。
- 保持主题、身份、方向、时间和权限，不猜 ID、不跨组织、不扩大扫库。无精确条件、失败或无权限即说明限制并停止，不自动申请权限。

## 6. 多跳证据链

**定位候选 → 确认目标 → 优先使用同次返回的 `resolvedDetail` → 缺字段时才提取真实 ID 调用对应产品。**目标不明、身份不符或读取失败时停止依赖该对象的后续操作。

跨来源条件链的第一步固定为 `aisearch enterprise`。即使原生产品支持更强的标题搜索，也必须先检查 AISearch 的 `resolvedDetail`；只有该字段缺失且 `_searchEvidence.stableRefs` 已核实时才进入下一跳。AISearch 没给稳定 ID 时不能用原生重搜替代本跳。

- 下游复用本跳真实 ID；人员用 `userId`，文档用 `nodeId`，听记用 `taskUuid`，待办用实际 `taskId`。Ding uid 不能当 Contact userId，snippet 不能当正文。
- 用户的读取前置条件必须满足；只加载下一跳所需 Skill，读取完整性以该次返回为准。ID 提取细节见 [多跳短流程](references/lite-recipes.md)。

## 7. 禁止本地资料兜底

用户问钉钉里的文档、消息、邮件、待办、日程或负责人时，只使用 DWS 返回作为业务证据。AISearch 未命中或候选未核实，不得改用 `find`、`rg`、`mdfind` 等本地文件搜索来回答，也不得把评测 fixture 当成用户钉钉数据。

“谁负责收集会议议题”这类职责问题，用一次 `person --dimension duty` 保留会议和职责原词；返回多位时列候选，不把普通会议参与者或共同上级当负责人。只有内容检索能提供明确职责原文时，才改用一次 `enterprise --types calendar,im,document`；不得两条命令都跑。

“DWS 归哪个部门管、具体谁负责”直接使用一次 `person --dimension duty --query "DWS 负责"`。只交付返回中有职责证据的人员和其部门；不得补写搜索结果中不存在的 CR 分工、组织角色或团队名单。

## 错误与成本最短路径

1. 失败且 `retryable=true`：原调用最多重试一次；否则停止，不换身份或绕过权限。
2. 成功但为空或无精确目标：不重复、改词或扩时间；必要的同条件核验见第 5 节。
3. `unknown flag`：查看一次该命令 Help 修正。API、权限和空结果不靠 Help 或猜参数解决。
4. 独立来源继续查询，依赖失败结果的步骤停止。保留标题、来源、链接、ID、数量、范围和错误；长 snippet 可外置，完整性字段不能删。

## 按需 Reference

常用路径不读 Reference。

| 仅当 | 读取 |
|---|---|
| 低频枚举、返回字段或兼容参数确实无法由本页判断 | [完整命令参考](references/aisearch.md) |
| 搜索与已知对象读取的产品边界仍不明确 | [局部意图消歧](references/intent-guide.md) |
| 多跳结果已有候选，但其稳定 ID 域或下一跳衔接不明确 | [多跳短流程](references/lite-recipes.md) |
