# Shortcut 参数幻觉缺口复核汇总

分析冻结基线：`origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`。本轮覆盖 42 个此前未进入正式参数兜底范围的公开 Shortcut；修订候选现已合入正式 `internal/cli/param_concepts.json`，并语义重放到 `origin/main@e9de6856d1ec989c9eb8063f2f131db48879179a`。

| 产品 | 命令数 | 变更 concept 数 | 新增 override | 新增 fixture | 报告 | 完整候选 |
|---|---:|---:|---:|---:|---|---|
| Agoal | 5 | 4 | 5 | 17 | [报告](./agoal/agoal_cli_param_hallucination_analysis.md) | [候选表](./agoal/param_concepts.json) |
| DevApp | 6 | 4 | 6 | 24 | [报告](./devapp/devapp_cli_param_hallucination_analysis.md) | [候选表](./devapp/param_concepts.json) |
| Whiteboard | 2 | 0 | 0 | 0 | [报告](./whiteboard/whiteboard_cli_param_hallucination_analysis.md) | [候选表](./whiteboard/param_concepts.json) |
| AITable | 11 | 4 | 11 | 46 | [报告](./aitable/aitable_cli_param_hallucination_analysis.md) | [候选表](./aitable/param_concepts.json) |
| Chat | 18 | 9 | 18 | 67 | [报告](./chat/chat_cli_param_hallucination_analysis.md) | [候选表](./chat/param_concepts.json) |

五产品修订后的完整合并候选、Schema 副本与修订说明见 [`../shortcut-gap-5-products-merge/`](../shortcut-gap-5-products-merge/README.md)。

## 落地状态

- [x] 冻结最新 main 并在隔离 worktree 构建。
- [x] Help/Cobra、运行时 Schema、Skill、正式 concepts 四源复核。
- [x] 每产品独立 Markdown 与完整候选草稿。
- [x] 每产品五页中文 XLSX（已渲染逐页目视并扫描公式错误）。
- [x] 候选逐产品生成、PreParse、alias/canonical、block/ambiguous、非目标别名回归和仓库政策验证。
- [x] 正式表落地、重新生成，并合并最新 main；保留 main 的 `chat thread forward` / `chat thread list-replies` 命令迁移。
