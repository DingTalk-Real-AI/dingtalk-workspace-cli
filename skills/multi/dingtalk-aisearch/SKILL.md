---
name: dingtalk-aisearch
description: AI搜问：人员语义搜索、跨源内容定位与行为回溯。Use when 按姓名/工号/部门/职责/上下级找人，或在目标对象未知时按主题、语义、来源或行为发现相关内容。搜索结果用于候选定位；命中后需要读取、修改或验证原对象时切换到对象所属产品。完整手机号精确反查走 dingtalk-contact。命令前缀：dws aisearch。
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
- 本页已覆盖 `person`、`enterprise`、`behavior` 的常用参数，直接执行；不要预读 shared、Reference、Schema、Help 或下游产品 Skill。
- 不猜命令、字段、ID、profile 或业务事实；可选时间缺失则省略并说明范围，不阻塞；多候选不取首项，ID 域不混用。
- 合法空结果结束当前搜索；同条件原生核验仅按下文“按原条件转用对应产品查询”规则执行；失败、分页不全或来源未核实不能说“没有”；搜索命中是候选，无完整性证据不得声称完整枚举或精确总数。
<!-- DWS_RUNTIME_CONTRACT_END -->

## Golden Route

| 意图 | 唯一首选入口 | 关键槽位 |
|---|---|---|
| 姓名/工号/部门/职位/职责/上下级/手机号线索找人 | `dws aisearch person` | `--query` + `--dimension` |
| 按主题找文档、消息、邮件、待办、听记等内容 | `dws aisearch enterprise` | `--queries` + `--types` + 可选 `--time-range` |
| 以我为关系端点的发送/接收，或我创建、编辑、分享过什么 | `dws aisearch behavior` | 上述内容槽位 + `--behavior-type` + 可选 `--direction/--chat-scope` |
| <!-- dws-intent: chat.search.filtered -->资源只限 IM，答案是逐条消息并带结构化消息谓词 | `dws chat +search-msg` | 发送者、会话、关键词、@、类型、reaction、时间和完整分页由 Chat 负责 |
| 枚举部门成员、完整人员名单 | `dingtalk-contact` | 部门定位 → 成员列表 → 按需详情，不把人员搜索候选当全量 |
| 完整手机号精确反查 | `dws contact user search-mobile --mobile "<完整手机号>" --format json` | `--mobile` |
| 已知稳定 ID 后读取/修改原对象 | 对应产品 Skill | 不再用 AISearch 重搜 |

选路先判断：①资源范围是跨来源还是仅 IM；②答案形态是发现结果、行为轨迹还是逐条消息；③是否依赖消息原生谓词。跨来源发现走 enterprise，当前用户行为轨迹走 behavior，仅 IM 的消息记录过滤走 Chat。

## 1. 人员搜索

维度映射：姓名→`name`，部门→`department`，职位/岗位→`position`，职责/技能/负责人→`duty`，上级→`supervisor`，下属→`subordinate`，工号→`jobNumber`，手机号线索→`phone`；确实无法判断维度时才用 `all`。

```bash
dws aisearch person --query "<用户原始目标>" --dimension <维度> --format json
```

- 用户给出多个独立条件时，每个条件各调用一次并分别汇报；`--query` 保留完整目标，不截名、不改昵称、不扩同音词。
- 正确维度返回 `success=true,result=[]` 后立即结束该组；不要改用 `all`、半截关键词或其他产品做无约束扩搜；同条件原生核验仅按“按原条件转用对应产品查询”规则执行。只有首选维度本身判断错误时才改正一次，不能把合法空结果当路由错误。
- 从每个候选提取并保留服务端返回的姓名、`userId`、`openDingTalkId` 或人员链接。多候选全部列出；用户要求详情时才用真实 `userId` 切 `dingtalk-contact`，执行 `dws contact user get --ids <userId> --format json`。
- 多个条件共同限定同一人时，必须逐项核对交集；独立命中不等于满足全部条件。姓名、部门、职位字段不能互相替代。
- 用户说“所有候选”但响应没有分页完成证据时，表述为“本次服务返回 N 个候选”，不要虚构全量性。

## 2. 跨源内容搜索

先从原句拆槽：时间词只进 `--time-range`，类型词只进 `--types`，剩余主题只进 `--queries`。类型枚举：`document,im,mail,calendar,todo,minute,report,image,link,notable,baike`。

```bash
dws aisearch enterprise --queries "<主题>" --types <类型CSV> [--time-range "<用户原始时间词>"] --format json
```

- 按用户要求的输出分组调用：同一组里的多个类型合并为 CSV；用户明确要求“分别找/按三类”时各组分开调用。不要再按底层产品拆得更细，也不要逐产品加载 Skill 重复搜索。
- 用户给出《精确标题》时，`--queries` 保留完整标题，只接受标题精确匹配的候选。没有精确命中就停止该搜索，不能拿“最接近”、最近项或列表第一项替代；同条件原生核验按“按原条件转用对应产品查询”规则，不扫描同义词。
- 用户要求“只有一个精确匹配才读取”时，先确认指定来源已覆盖、分页已结束且精确匹配只有一个；缺任一证据就报告“唯一性未核实”，不读取正文。正文的 `complete=true` 不能证明搜索完整。
- 仅要求列出标题、来源、链接或标识时，到搜索结果为止；不要读取原文。用户要求原文或候选缺少任务必需的类型、身份、时间证据时，满足用户的前置条件后，提取真实稳定 ID 并加载一个对应产品 Skill；不能用补充核验为由跳过“唯一才读取”等条件。
- 正确搜索的空数组是该条件下无结果；非空但不含精确目标也是“未找到目标”。不要缩短关键词或扩大时间范围，除非用户要求。

## 3. 行为回溯

```bash
dws aisearch behavior --queries "<主题>" --types <类型CSV> --behavior-type <all|send|receive|create|edit|share> [--time-range "<时间>"] [--direction "我->某人|某人->我|我<->某人"] [--chat-scope "<完整群名>"] --format json
```

- 每个不同的“动作＋方向＋时间”组合调用一次；同一组合的多个类型可用 CSV 合并。`chat-scope` 仅用于 `im`，方向保留用户原文姓名，不先查邮箱或 userId。方向是当前用户参与的关系约束，不是对某个发送者全部消息的集合定义。
- 动作按当前用户视角选择，适用于所有内容类型：“我发给某人”＝`behavior-type=send, direction=我->某人`；“某人发给我”＝`behavior-type=receive, direction=某人->我`。不能因原句有“发”就选 `send`。“我在某群发过”另加 `types=im, chat-scope=<完整群名>`。
- 资源只限 IM、答案要求逐条消息并按发送者、会话、关键词、reaction 或精确时间范围过滤时，改用 `dws chat +search-msg`；已解析出的稳定身份直接作为消息谓词输入。
- 空结果只表示本次未命中；不删除主题、不追加同义词、不缩短群名重搜，不用 recent 列表替代行为证据。需补充查询时按第 5 节保留原条件执行。
- 返回已足够回答就停止；缺少必需信息时才查询原对象。

## 4. 结果核验与交付

- **时间**：原时间词传给 `--time-range`，按当前日期、时区确定核验区间；“本周”不等于近七天。数值时间先确认秒/毫秒单位，再用程序换算。已知越界记录排除；只有聚合日期或缺少逐条时间时标为时间未核实。创建行为须有创建时间，不能用修改时间或会议开始时间代替。
- **身份**：核对实际发送者、收件关系和创建者；群内可见不等于发给我。Contact `userId` 与 Ding uid 等不同域 ID 不能直接比较；没有明确映射时，既不能认定同一人，也不能因值不同就排除本人。
- **主题与类型**：候选须有内容证据支持主题关联；搜索命中、群名相关或流程上“可能有关”都不足以确认。无关联证据时标为相关性未核实，不自行选成“最相关”。文本、标题或普通链接不能充当文件证据。
- **唯一与全量**：检查指定来源和分页终态；缺完整性字段就不声称唯一或全量。按稳定 ID 去重，交付清单与已核实的 ID、数量核对，避免读到却漏列。只问“有没有”时，有效命中即可回答。
- **忠实总结**：不补写未返回事实，不把“建议、待确认”改成已发生；分清搜索片段与正文、接口总数与已读取条数。部分结果伴随错误或未完分页时保留说明，不能据此说“没有其他结果”。

## 5. 按原条件转用对应产品查询

- 需要详情、成员清单或核验时，用返回的真实 ID，或原生产品支持的精确标题、部门等条件查询；只加载对应产品 Skill，按其参数调用。
- 保持原主题、身份、方向、时间及权限范围；不猜 ID、不跨组织复用 ID，不扩大条件扫库。没有明确查询条件、查询失败或无权限时，说明限制并停止，不自动申请权限。

## 6. 多跳证据链

**定位候选 → 确认目标 → 提取真实 ID → 调用对应产品。**目标不明、身份不符或读取失败时停止依赖该对象的后续操作。

| AISearch 证据 | 可传给下游 | 禁止 |
|---|---|---|
| 人员结果 `userId` | `dws contact user get --ids <userId> --format json` | 把人员 URL 的 Ding uid 当 Contact userId |
| 文档结果 `nodeId` | `dws doc +fetch --node <nodeId> --format json` | 用 snippet 冒充正文 |
| 听记结果 `taskUuid` | 详情用 `dws minutes +detail --id <taskUuid> --format json`；逐字稿用 `dws minutes +transcript --id <taskUuid> --format json` | 用最近一条或列表第一项 |
| 待办结果中的真实 `taskId`/深链参数 | `dws todo +get --task-id <taskId> --format json` | 使用历史硬编码 ID |

- 下游读取必须复用定位结果的同一个 ID；读取返回的完整性字段决定能否说“完整”。
- 用户设置了条件分支（如“身份不一致就停止”）时，条件不满足或尚未核实便停止，不继续读取来补交其他内容。
- 只在下一跳确实需要时加载对应 Skill；不要一次加载多个下游 Skill 备用。低频 ID 提取细节才读 [多跳短流程](references/lite-recipes.md)。

## 错误与成本最短路径

1. 调用失败且返回 `retryable=true`：原命令、原参数最多重试一次；仍失败则报告错误并停止该调用。
2. 调用失败且返回 `retryable=false`：停止该调用，不换身份或绕过权限继续尝试。未提供重试标记时，不自行认定可以重试。
3. 调用成功但结果为空或没有精确匹配：说明本次未找到，不重复查询、不改关键词或扩大时间范围；有明确查询条件且需要补充核实时，按第 5 节转用对应产品。
4. 参数报 `unknown flag`：查看一次当前命令 Help，按公开参数修正；API、权限错误和空结果不靠查看 Help 或猜参数解决。
5. 一个来源未命中或失败，不跳过用户要求的其他独立来源；依赖该结果才能执行的后续操作则停止。
6. 保留标题、来源、链接、稳定 ID、数量、范围及错误信息；长 snippet 可外置，不删 `complete/hasMore/nextCursor/stopReason` 等完整性字段。

## 按需 Reference

正常 `person/enterprise/behavior` 不读 Reference。

| 仅当 | 读取 |
|---|---|
| 低频枚举、返回字段或兼容参数确实无法由本页判断 | [完整命令参考](references/aisearch.md) |
| 搜索与已知对象读取的产品边界仍不明确 | [局部意图消歧](references/intent-guide.md) |
| 多跳结果已有候选，但其稳定 ID 域或下一跳衔接不明确 | [多跳短流程](references/lite-recipes.md) |
