---
name: dingtalk-aisearch
description: AI搜问：人员语义搜索、跨源内容定位与行为回溯。Use when 按姓名/工号/部门/职责/上下级找人；按主题跨文档/消息/邮件/待办/听记/日志/图片/链接/AI表格检索；或询问我/某人发送、收到、创建、编辑、分享过什么。即使请求点名某一内容类型，只要目标是“按主题找记录”而非读取已知对象，也优先本 skill。完整手机号反查走 dingtalk-contact；命中后要读取或修改原对象才切对应产品。命令前缀：dws aisearch。
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
- 不猜命令、字段、ID、profile 或时间。多候选不默认取第一项，不把不同 ID 域互相替代。
- 合法空结果是终态；接口失败、分页不完整或来源未核实不能表述为“没有”。
<!-- DWS_RUNTIME_CONTRACT_END -->

## Golden Route

| 意图 | 唯一首选入口 | 关键槽位 |
|---|---|---|
| 姓名/工号/部门/职位/职责/上下级/手机号线索找人 | `dws aisearch person` | `--query` + `--dimension` |
| 按主题找文档、消息、邮件、待办、听记等内容 | `dws aisearch enterprise` | `--queries` + `--types` + 可选 `--time-range` |
| 我/某人发送、收到、创建、编辑或分享过什么 | `dws aisearch behavior` | 上述内容槽位 + `--behavior-type` + 可选 `--direction/--chat-scope` |
| 完整手机号精确反查 | `dws contact user search-mobile --mobile "<完整手机号>" --format json` | `--mobile` |
| 已知稳定 ID 后读取/修改原对象 | 对应产品 Skill | 不再用 AISearch 重搜 |

只要任务仍是“定位记录”，即使只点名邮件、消息或文档，也由 AISearch 完成；不要提前切 Mail、Chat、Doc、Drive、Wiki、Todo、Minutes 或 Report 重做搜索。

## 1. 人员搜索

维度映射：姓名→`name`，部门→`department`，职位/岗位→`position`，职责/技能/负责人→`duty`，上级→`supervisor`，下属→`subordinate`，工号→`jobNumber`，手机号线索→`phone`；确实无法判断维度时才用 `all`。

```bash
dws aisearch person --query "<用户原始目标>" --dimension <维度> --format json
```

- 用户给出多个独立条件时，每个条件各调用一次并分别汇报；`--query` 保留完整目标，不截名、不改昵称、不扩同音词。
- 正确维度返回 `success=true,result=[]` 后立即结束该组；不要改用 `all`、半截关键词或其他产品扩搜。只有首选维度本身判断错误时才改正一次，不能把合法空结果当路由错误。
- 从每个候选提取并保留服务端返回的姓名、`userId`、`openDingTalkId` 或人员链接。多候选全部列出；用户要求详情时才用真实 `userId` 切 `dingtalk-contact`，执行 `dws contact user get --ids <userId> --format json`。
- 用户说“所有候选”但响应没有分页完成证据时，表述为“本次服务返回 N 个候选”，不要虚构全量性。

## 2. 跨源内容搜索

先从原句拆槽：时间词只进 `--time-range`，类型词只进 `--types`，剩余主题只进 `--queries`。类型枚举：`document,im,mail,calendar,todo,minute,report,image,link,notable,baike`。

```bash
dws aisearch enterprise --queries "<主题>" --types <类型CSV> [--time-range "<用户原始时间词>"] --format json
```

- 按用户要求的输出分组调用：同一组里的多个类型合并为 CSV；用户明确要求“分别找/按三类”时各组分开调用。不要再按底层产品拆得更细，也不要逐产品加载 Skill 重复搜索。
- 用户给出《精确标题》时，`--queries` 保留完整标题，只接受标题精确匹配的候选。没有精确命中就停止，不能拿“最接近”、最近项或列表第一项替代，也不能去 Doc/Drive/Wiki 扫描同义词。
- 仅要求列出标题、来源、链接或标识时，到搜索结果为止；不要读取原文。只有用户明确要求打开/读取且已有唯一正确候选时，才提取真实稳定 ID 并加载一个对应产品 Skill。
- 正确搜索的空数组是该条件下无结果；非空但不含精确目标也是“未找到目标”。不要缩短关键词或扩大时间范围，除非用户要求。

## 3. 行为回溯

```bash
dws aisearch behavior --queries "<主题>" --types <类型CSV> --behavior-type <all|send|receive|create|edit|share> [--time-range "<时间>"] [--direction "我->某人|某人->我|我<->某人"] [--chat-scope "<完整群名>"] --format json
```

- 每个不同的“动作＋方向＋时间”组合调用一次；同一组合的多个类型可用 CSV 合并。`chat-scope` 仅用于 `im`，方向保留用户原文姓名，不先查邮箱或 userId。
- “我在某群发过”＝`types=im, behavior-type=send, chat-scope=<完整群名>`；“我发给某人的邮件”＝`types=mail, behavior-type=send, direction=我->某人`；“某人发给我的文档”＝`types=document, behavior-type=receive, direction=某人->我`。
- `success=true,result=[]` 后按该分类如实汇报并停止；不要改走 Chat/Mail/Doc/Drive recent，也不要缩短群名反复试探。
- 只有命中后还要读取或修改原对象时才切下游 Skill；分类汇总不需要下游调用。

## 4. 多跳证据链

严格逐跳执行：**AISearch 定位唯一目标 → 校验标题/昵称/关系 → 提取本跳真实 ID → 只加载下一跳所需的一个 Skill → 从新证据推导下一查询**。前置目标为空、多候选未消歧、身份不一致或读取失败时立即停止后续依赖步骤。

| AISearch 证据 | 可传给下游 | 禁止 |
|---|---|---|
| 人员结果 `userId` | `dws contact user get --ids <userId> --format json` | 把人员 URL 的 Ding uid 当 Contact userId |
| 文档结果 `nodeId` | `dws doc +fetch --node <nodeId> --format json` | 用 snippet 冒充正文 |
| 听记结果 `taskUuid` | 详情用 `dws minutes +detail --id <taskUuid> --format json`；逐字稿用 `dws minutes +transcript --id <taskUuid> --format json` | 用最近一条或列表第一项 |
| 待办结果中的真实 `taskId`/深链参数 | `dws todo +get --task-id <taskId> --format json` | 使用历史硬编码 ID |

- 下游读取必须复用定位结果的同一个 ID；读取返回的完整性字段决定能否说“完整”。
- 用户设置了条件分支（如“身份不一致就停止”）时，条件不满足便停止，不继续读取来补交其他内容。
- 只在下一跳确实需要时加载对应 Skill；不要一次加载多个下游 Skill 备用。低频 ID 提取细节才读 [多跳短流程](references/lite-recipes.md)。

## 错误与成本最短路径

1. `retryable=true`：原命令、原参数最多重试一次；仍失败则报告该分类接口失败。
2. `retryable=false`、合法空结果或精确目标缺失：立即停止该分支，不换产品、不换近义词。
3. 真实 `unknown flag`：只查看一次当前 leaf Help；已知命令不读 Help，API/权限/空结果错误也不读 Help。
4. 搜索结果只保留完成任务需要的姓名/标题、来源、链接、稳定 ID 和必要状态；最终按用户要求分组，避免复述长 snippet。

## 按需 Reference

正常 `person/enterprise/behavior` 不读 Reference。

| 仅当 | 读取 |
|---|---|
| 低频枚举、返回字段或兼容参数确实无法由本页判断 | [完整命令参考](references/aisearch.md) |
| 搜索与已知对象读取的产品边界仍不明确 | [局部意图消歧](references/intent-guide.md) |
| 多跳结果已有候选，但其稳定 ID 域或下一跳衔接不明确 | [多跳短流程](references/lite-recipes.md) |
