# Agoal CLI 参数幻觉复核（2026-08-21）

## 结论

Agoal 是本轮全产品复核中新发现的真实漏项。基于最新线上 `main`
`11934eed057267d97e7442ddd420c711ee1802dc`，已形成独立完整候选：62 个 concept、362 个
command override、691 条 fixture。候选仅保存在本目录，未修改正式表。

## 必要兜底

- `obj-template list` 与 `report submit-detail` 复用搜索词、页码、页大小概念；它们是数字分页，明确阻断 cursor/offset。
- `strategy detail/update` 仅在命令内把 `strategy-id` 归一到 API 的 `profile-id`；该 ID 不是账号 profile、userId 或 corpId。
- `contract`、`scorecard`、`user objectives` 只加入角色完整的命令局部别名；`id`、`date`、`payload` 等继续 fail-closed。
- 覆盖式更新的 JSON 字段只允许精确字段名加 `-json`，不把通用 `data/payload/json` 猜成业务字段。

## 未自动解决

`scope-id` 的值域由 `scope-type=DEPT|PERSONAL` 决定，单靠参数名无法把 department-id 或
user-id 安全转换；更新命令仍必须先查详情再覆盖提交。以上属于值语义与工作流，不由 argv alias 代替。

## 验证

独立候选完成 fresh generate、嵌入式 PreParse fixture、alias/canonical、guard 和非目标保持验证；
联合候选新增 13 个有效命令规则，产生 14 个 alias、4 个 block、25 个 ambiguous 行为差异。

分析依据：最新 Cobra/Help、正式 Schema、`dingtalk-misc/references/agoal.md`、真实生成器与 Runtime PreParse。
