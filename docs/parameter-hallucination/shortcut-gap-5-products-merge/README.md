# Shortcut 缺口五产品参数别名表草稿

本目录保存 Agoal、DevApp、Whiteboard、AITable、Chat 五个产品的合并参数别名表草稿。

- 冻结基线：`origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`
- 来源分支：`codex/param-hallucination-shortcut-gaps@5427b049893bf92b26144a95f01ac89e89a1261a`（隔离 worktree）
- 覆盖：42 条公开 Shortcut
- Concept 变更：18 个，其中仅 `aitable_field_ids` 和 `aitable_datasource_config_json` 为新建实体
- 新增 command override：40 条
- 新增 PreParse fixture：154 条
- 关键收紧：`chat +chat-list-mine` 不再复用分页大小；AITable datasource 配置 concept 仅保留明确 sourceConfig 拼写；DevApp `skill-names` 保持歧义
- Whiteboard：因官方别名生成源树未覆盖 runnable leaf，本轮不写入任何 Whiteboard 候选变更
- 合并验证：Chat/AITable 已纳入确认门检查，四个有候选规则的产品均补最终 transport payload 代表用例；DevApp `skill-list` 的完整模板显式携带 canonical `--skills`
- 正式落地：已写入 `internal/cli/param_concepts.json` 并重新生成；随后语义重放到 `origin/main@e9de6856d1ec989c9eb8063f2f131db48879179a`，保留 main 的 `chat thread forward` / `chat thread list-replies` 迁移

完整草稿：[`param_concepts.json`](./param_concepts.json)。同目录 [`param_concepts.schema.json`](./param_concepts.schema.json) 是冻结基线 Schema 的字节一致副本，可按 JSON 中的 `$schema` 相对路径独立校验。

## 修订同步

- DevApp：`skill-list` 保留为 ID 列表的值不变别名，`skill-names` 保持歧义；新增两条 fixture，并补齐 canonical payload 模板。
- AITable：宽泛的 `datasource-config` / `data-source-config` 不再自动归一到 `source-config`，在存在自动同步配置角色时保持歧义。
- Chat：`chat +chat-list-mine` 的 `limit` 是总量上限，不再复用分页大小 concept；仅 `max-results` / `max-result` 作为角色完整别名，分页式命名被阻断或保持歧义。
- 单产品 Markdown、XLSX 与完整候选表已按以上口径同步，不再以旧单产品候选作为合并来源。

## 边界

该文件保留基于冻结基线合并五份独立候选后的评审快照；生产配置已在本分支合入 `internal/cli/param_concepts.json`，并在最新 main 上重新生成和验证。
