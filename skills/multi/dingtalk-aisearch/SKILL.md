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
- 常用 `person/enterprise/behavior` 按本页直接执行，不预读 shared、Reference、Schema、Help 或下游 Skill。
- 不猜命令、字段、ID、profile 或事实；可选时间缺失则省略并说明范围；多候选不取首项，ID 不混域。
- 空结果结束当前搜索，同条件核验见第 5 节；失败或不完整不能说“没有”，候选不等于全量。
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

选路顺序：资源范围 → 答案形态 → 原生谓词。

## 1. 人员搜索

维度映射：姓名→`name`，部门→`department`，职位/岗位→`position`，职责/技能/负责人→`duty`，上级→`supervisor`，下属→`subordinate`，工号→`jobNumber`，手机号线索→`phone`；确实无法判断维度时才用 `all`。

```bash
dws aisearch person --query "<用户原始目标>" --dimension <维度> --format json
```

- 独立条件分别查询、汇报；保留完整目标，不截名、改昵称或扩同音词。
- 正确维度返回空结果就结束该组，不换 `all` 或缩词扩搜；同条件核验见第 5 节。仅维度选错时改正一次，空结果不等于路由错误。
- 保留全部候选的姓名、真实 ID 和人员链接。用户要详情才切 Contact： `dws contact user get --ids <userId> --format json`。
- 同一人的多条件须核对交集；姓名、部门、职位不互相替代。
- 无分页完成证据时，只称“本次返回 N 个候选”。

## 2. 跨源内容搜索

先从原句拆槽：时间词只进 `--time-range`，类型词只进 `--types`，剩余主题只进 `--queries`。类型枚举：`document,im,mail,calendar,todo,minute,report,image,link,notable,baike`。

```bash
dws aisearch enterprise --queries "<主题>" --types <类型CSV> [--time-range "<用户原始时间词>"] --format json
```

- 按用户要求分组调用，组内类型合并为 CSV；不按底层产品细拆或重复搜索。
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
- 返回已足够回答就停止；缺少必需信息时才查询原对象。

## 4. 结果核验与交付

- **时间**：原时间词传给 `--time-range`，按当前日期、时区确定核验区间；“本周”不等于近七天。数值时间先确认秒/毫秒单位，再用程序换算。已知越界记录排除；只有聚合日期或缺少逐条时间时标为时间未核实。创建行为须有创建时间，不能用修改时间或会议开始时间代替。
- **身份**：核对实际发送者、收件关系和创建者；群内可见不等于发给我。Contact `userId` 与 Ding uid 等不同域 ID 不能直接比较；没有明确映射时，既不能认定同一人，也不能因值不同就排除本人。
- **主题与类型**：候选须有内容证据支持主题关联；搜索命中、群名相关或流程上“可能有关”都不足以确认。无关联证据时标为相关性未核实，不自行选成“最相关”。文本、标题或普通链接不能充当文件证据。
- **唯一与全量**：检查指定来源和分页终态；缺完整性字段就不声称唯一或全量。按稳定 ID 去重，交付清单与已核实的 ID、数量核对，避免读到却漏列。只问“有没有”时，有效命中即可回答。
- **忠实总结**：不补写未返回事实，不把“建议、待确认”改成已发生；分清搜索片段与正文、接口总数与已读取条数。部分结果伴随错误或未完分页时保留说明，不能据此说“没有其他结果”。

## 5. 按原条件转用对应产品查询

- 需要详情、成员清单或核验时，用返回的真实 ID，或原生产品支持的精确标题、部门等条件查询；只加载对应产品 Skill，按其参数调用。
- 保持主题、身份、方向、时间和权限，不猜 ID、不跨组织、不扩大扫库。无精确条件、失败或无权限即说明限制并停止，不自动申请权限。

## 6. 多跳证据链

**定位候选 → 确认目标 → 提取真实 ID → 调用对应产品。**目标不明、身份不符或读取失败时停止依赖该对象的后续操作。

- 下游复用本跳真实 ID；人员用 `userId`，文档用 `nodeId`，听记用 `taskUuid`，待办用实际 `taskId`。Ding uid 不能当 Contact userId，snippet 不能当正文。
- 用户的读取前置条件必须满足；只加载下一跳所需 Skill，读取完整性以该次返回为准。ID 提取细节见 [多跳短流程](references/lite-recipes.md)。

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
