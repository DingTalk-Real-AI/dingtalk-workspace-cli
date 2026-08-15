---
name: dingtalk-shared
description: 钉钉(DingTalk) MultiSkill 的轻量共享与跨产品调度入口。Use when 用户泛称 DWS/钉钉操作但未明确产品、在同一请求中查日程+待办+听记等多产品汇总、请求跨产品编排、需要 URL 类型预检或产品边界消歧。多产品任务先使用本 Skill 的紧凑调度路径，不要预加载各产品 Skill；清晰的单产品操作仍使用对应 dingtalk-* 子 Skill。
metadata:
  cli_version: ">=0.2.14"
  category: shared
  requires:
    bins:
      - dws
---

# DWS 共享执行契约

本文件只在泛称 DWS、跨产品流程、URL 预检或意图不清时作为入口。明确单产品请求直接使用对应 `dingtalk-*` skill；已经内嵌最小执行契约的产品根 Skill 不需要先完整读取本文件。

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行。Skill/reference 无法定位时才用 `dws schema search --query "<意图>" --limit 5`；选中后携带双 hash Inspect canonical，再按 `primary_cli_path` 执行。参数/安全语义或 Cobra flag 不确定时才补读精确 Schema/Help；不加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

## 多轮写入与追加语义

用户在某一轮明确要求创建、插入或追加内容时，按本轮指令执行：
- **"追加/再插入"类指令即使目标内容疑似已存在也要执行本轮写入**（可提示已存在，但不得以"已存在/已重复"为由拒绝执行明确指令）；
- 不要提前执行后续轮次可能要求的内容；每轮只做本轮明确要求的写入；
- 回读用于验证写入结果，不用于推翻本轮的明确写入指令。


产品或跨产品规则在最小契约之上增量加载。用户已明确产品内容意图时，意图优先于 URL 形态；多账号选择与跨组织规则读取 [`../dingtalk-misc/references/profile.md`](../dingtalk-misc/references/profile.md)。本地文件、产品边界和跨产品传递规则只在对应任务中加载，避免把全局手册放入每个单产品请求。

## 渐进加载

只读取当前任务需要的文件，不要一次性加载全部 shared references：

| 当前情况 | 必读内容 |
|---|---|
| 已明确单一产品 | 对应 `../dingtalk-*/SKILL.md`；不读路由 reference |
| 泛称 DWS、需要选择产品 | [routing.md](references/routing.md) |
| 今日日程 + 未完成待办 + 最近听记的全部或任意组合 | 直接使用下文“常见多产品只读汇总快路径”；不读 reference |
| 其他跨产品、多步骤、汇总或报告 | [workflow-routing.md](references/workflow-routing.md) |
| 输入含 alidocs、shanji 等钉钉 URL 且类型不明 | [url-patterns.md](references/url-patterns.md) |
| 产品边界仍然难以判断 | [intent-guide.md](references/intent-guide.md) 的相关章节 |
| 认证、全局 flag 或输出格式问题 | [global-reference.md](references/global-reference.md) |
| 命令已经返回错误 | [error-codes.md](references/error-codes.md)；只查错误对应章节 |
| `confirmation_required` / 写操作确认 | [confirmation.md](references/confirmation.md) |
| 命令发现、Schema / `--compact` / `--all` | [schema-usage.md](references/schema-usage.md) |
| 怀疑能力不支持 | [capability-limits.md](references/capability-limits.md) |
| 批量/多源采集 | [conventions.md](references/recipes/conventions.md) |
| 固定短流程 | [lite-catalog.md](references/recipes/lite-catalog.md) 对应章节 |

产品命令、脚本和字段细节位于对应产品 skill，不在 `dingtalk-shared` 重复维护。

## 本 skill 作为入口时的路由顺序

1. 先识别明确的产品内容意图；明确意图直接进入对应产品。仅当输入包含钉钉 URL
   且类型不明确或意图与链接类型可能冲突时，读取 `url-patterns.md` 识别节点类型。
2. 请求包含多个时序步骤、跨产品数据传递或汇总报告：先匹配下文紧凑只读快路径。命中则直接执行且不读 reference；未命中才读取 `workflow-routing.md`，按行动指南组合需要的产品 skill。
3. 请求是单产品操作但产品不明确：读取 `routing.md`，再显式读取目标产品
   `SKILL.md`。
4. `doc/drive/wiki`、`aitable/sheet`、`calendar/minutes` 等边界仍不清楚：
   只读取 `intent-guide.md` 的对应章节。
5. 仍无法判断时向用户追问，不要猜测产品或命令。

## 常见多产品只读汇总快路径

当用户在同一请求中要求“今日日程 + 当前未完成待办 + 最近一条听记摘要”的全部或任意组合，直接执行：

```bash
python scripts/cross_product_read_summary.py --date YYYY-MM-DD --timezone <IANA_TIMEZONE> [--include calendar,todos,minutes]
```

- 该脚本只调用真实 `dws` 只读命令，并发采集后只返回汇总所需字段。
- 用户只要其中部分类别时，必须用 `--include` 只读取被请求的产品；三类都要时可省略。
- 命中时不读取 calendar/todo/minutes 产品 Skill 或行动指南，不运行 `--help` / Schema Search，不重复执行同一 list。
- 一类数据为空或失败时，按脚本 JSON 的分类结果如实说明，不用其他产品数据替代。
- 只有用户要求了该脚本未覆盖的字段、时间范围或写操作，才增量加载对应产品 Skill。

## 跨 skill 执行

- 正文中的相对 `Read` 链接是运行时依赖；`metadata.requires.skills` 不会自动加载。
- 选择目标产品后，以目标 skill 的命令、参数和风险规则为准。
- 多步骤流程按顺序传递真实返回值；可以并行的只读采集按对应 workflow/reference
  执行，写操作默认串行并逐步验证。
- 产品 skill 已内联的清晰操作直接执行；仅在遇到该 skill 未覆盖的参数或边界时读取
  更深层 reference。

## 错误最短路径

1. `unknown command` / `unknown flag`：运行对应层级 `--help`，按公开 flag 修正后最多重试一次；命令选择不确定时读 [schema-usage.md](references/schema-usage.md)。
2. `reason=confirmation_required`：按 [confirmation.md](references/confirmation.md) 处理，不要当普通校验错误放弃或静默加 `--yes`。
3. 认证或权限错误：读取 `global-reference.md` 与 `error-codes.md` 对应章节。
4. 其他错误：优先读取 JSON 错误中的 `retryable`、`retry_after_seconds`、
   `next_retry_at`、`hint` 和 `actions`。只有明确 `retryable=true` 时才按服务端节奏重试；
   缺少重试语义时用 `--verbose` 获取诊断并停止，不连续尝试替代命令。
5. 能力可能不存在（如群投票、订阅、导出报表等 Catalog 未覆盖的意图）：`dws schema search` 一次
   确认候选；返回 `abstained=true`、`weak_match=true` 的 `hint`，或候选语义与意图不符时，最多用
   `dws schema <product> --compact` 复核一次该产品能力边界，然后如实结论"当前 CLI 不支持该能力"。
   禁止逐个候选验证、逐产品枚举或改用无关接口绕过。
6. 明确不支持的能力：说明边界，不通过其他接口绕过。
