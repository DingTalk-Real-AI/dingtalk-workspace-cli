---
category: Changed
---

- **report entry submit requires recipients** — `dws report entry submit`（及废弃别名 `dws report create`）的 `--to-user-ids` 从可选提升为必填：无接收人的日志提交在服务端仍返回成功，但日志对任何接收人都不可见。openAPI `create_report` 的 `toUserIds` 参数保持可选不动，规则仅在 dws CLI 侧收紧——Cobra required 拦截未传场景，RunE 内对空值/纯分隔符（如 `--to-user-ids ","`）同样 fail-closed 拒绝。修复 [#85724185](https://project.aone.alibaba-inc.com/v2/project/2170318/bug/85724185)。
