---
name: dingtalk-shared
description: 钉钉(DingTalk) MultiSkill 的轻量共享入口。Use when 用户泛称 DWS/钉钉操作但未明确产品、请求跨产品编排、需要 URL 类型预检或产品边界消歧。清晰的单产品操作优先使用对应 dingtalk-* 子 skill；本 skill 只提供全局执行契约和按需 reference 导航，不承载产品命令全集。
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
- 已知命令直接执行。只有 leaf 参数或安全语义不确定时读取精确 Schema，只有 Cobra flag 不确定时读取精确 leaf Help；不要加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

产品或跨产品规则在最小契约之上增量加载。用户已明确产品内容意图时，意图优先于 URL 形态；多账号选择与跨组织规则读取 [`../dingtalk-misc/references/profile.md`](../dingtalk-misc/references/profile.md)。本地文件、产品边界和跨产品传递规则只在对应任务中加载，避免把全局手册放入每个单产品请求。

## 渐进加载

只读取当前任务需要的文件，不要一次性加载全部 shared references：

| 当前情况 | 必读内容 |
|---|---|
| 已明确单一产品 | 对应 `../dingtalk-*/SKILL.md`；不读路由 reference |
| 泛称 DWS、需要选择产品 | [routing.md](references/routing.md) |
| 跨产品、多步骤、汇总或报告 | [workflow-routing.md](references/workflow-routing.md) |
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
2. 请求包含多个时序步骤、跨产品数据传递或汇总报告：即使 URL 已识别，也要读取
   `workflow-routing.md`，按行动指南组合需要的产品 skill；当前发布包不包含独立
   scenario skill。
3. 请求是单产品操作但产品不明确：读取 `routing.md`，再显式读取目标产品
   `SKILL.md`。
4. `doc/drive/wiki`、`aitable/sheet`、`calendar/minutes` 等边界仍不清楚：
   只读取 `intent-guide.md` 的对应章节。
5. 仍无法判断时向用户追问，不要猜测产品或命令。

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
5. 明确不支持的能力：说明边界，不通过其他接口绕过。
