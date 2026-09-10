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

- 只用 `dws`；结构化读取加 `--format json`，按真实返回判断。
- 已知命令直调；参数/约束/安全不明查 leaf 窄 Schema。Schema 不可用才读已知 leaf Help 一次；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 不查 Help：优先错误中的明确 suggestion，其次已加载 Skill/reference 中的明确兼容入口；均无则报漂移并停，禁全 Catalog。低频 reference 不默认 Help，禁 root/parent/product Help。发现后必须执行或说明阻塞。
- 不猜命令/flag/字段/ID/账号/业务事实；ID 来自真实返回。目标零命中/多候选/类型不明先消歧；仅可选时间/展示范围用契约默认，缺必需信息即停。
- 解析/读/写同一 profile，ID 不跨组织。多账号只用唯一 `isOrgCurrent=true`；否则用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出/记录 token、refresh token、appSecret、webhook token；已注入认证时不索要。
- 写须符合明确意图；确认以最终 Runtime gate/Schema 为准，确认后才加 `--yes`。
- 写后验证结果，不凭退出码宣称成功。退出须最终答复，区分完成、部分、阻塞、待确认、失败；保留已有数据及 `complete/hasMore/stopReason/failures`。
- 时间戳按会话时区展示，必要时保留原值。
- 认证/权限/profile/confirmation/未知错误只读 `dingtalk-shared` 对应 reference，禁连续猜替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

国际版、海外版或 `.io` 区域登录必须执行 `dws auth login --intl`（无头环境再加 `--device`）；`--intl` 只用于登录，后续业务命令按所选 profile 自动路由。

产品或跨产品规则在最小契约之上增量加载。用户已明确产品内容意图时，意图优先于 URL 形态；多账号选择与跨组织规则读取 [`../dingtalk-misc/references/profile.md`](../dingtalk-misc/references/profile.md)。本地文件、产品边界和跨产品传递规则只在对应任务中加载，避免把全局手册放入每个单产品请求。

## 渐进加载

只读取当前任务需要的文件，不要一次性加载全部 shared references：

| 当前情况 | 必读内容 |
|---|---|
| 已明确单一产品 | 对应 `../dingtalk-*/SKILL.md`；不读路由 reference |
| 泛称 DWS、需要选择产品 | [routing.md](references/routing.md) |
| 跨产品、多步骤、汇总或报告 | [workflow-routing.md](references/workflow-routing.md) |
| 输入含用户直接提供的 alidocs `/i/nodes/` URL 或来源未验证的 nodeId（即使产品意图明确） | [url-patterns.md](references/url-patterns.md)；先规范化 dlink 再进入目标产品 |
| 输入含其他 alidocs、shanji 等钉钉 URL 且类型不明 | [url-patterns.md](references/url-patterns.md) |
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

1. 先识别明确的产品内容意图；明确意图直接选择对应产品。用户直接提供的 alidocs
   `/i/nodes/` URL 或来源未验证的 nodeId 仍须读取 `url-patterns.md`：先执行节点类型
   探测，`extension=dlink` 时将 `drive info` 的 `result.fileId` 保存为入口 ID 并传给
   `dws doc info`，再按 `linkSourceInfo.nodeId` 逐跳解析目标，把最终目标 ID 交给候选
   产品。当前调用链已返回真实类型的稳定 ID 可直接复用。
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
