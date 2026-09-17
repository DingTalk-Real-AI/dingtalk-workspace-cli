## Summary

为白板功能 PR #1384 预先登记精确的 Schema 兼容性例外，允许 `whiteboard.create_with_content` 的 `confirmation` 从 `not_required` 调整为 `user_required`，支持渲染预览后经用户明确确认再创建白板的流程。

本 PR 仅授权这一单向兼容性变更，实际命令声明与运行时确认逻辑由功能 PR 落地。反向降低确认要求、其他命令借用例外，以及同时修改未授权的 `risk` 字段，仍会被拒绝。

## Risk tier

- [x] High-risk：修改 Schema 兼容性策略。

## Verification

测试代码提交：`c5a76891b791c2727859a70fbc1306e27b8935a4`。后续提交仅补充测试证据。

- Agent 示例契约：1,744 个示例通过校验，164 个 dry-run 完成；13 个 reviewed manual 示例维持 contract-only。
- 确定性选择场景：覆盖 1,435 个工具、1,491 个正向断言、1,795 个负向断言。
- `schema-compat` 与 `schemacompat`：66 个顶层测试通过，包含白板确认例外边界和基线策略集成测试。
- `gofmt` 与 `git diff --check` 通过。
- 本次运行定向测试，未运行完整 Go 测试套件。
- Release fragment / generated drift / command surface：N/A，本 PR 不改变用户命令、生成输入或运行时行为。

### Agent 测试报告

本地 Agent 示例契约、dry-run 与确定性选择场景校验；未运行线上 `/eval` 或真实模型语义评测。

![Agent 测试报告](https://raw.githubusercontent.com/zxwang6/dingtalk-workspace-cli/fix/whiteboard_confirmation/.github/pr-evidence/whiteboard-confirmation/agent-report.png)

### 指令 CI 集成测试

本地 Schema 兼容性单元测试与策略集成测试。图片根据真实日志生成，不是 GitHub Actions 页面截图。

![指令 CI 集成测试](https://raw.githubusercontent.com/zxwang6/dingtalk-workspace-cli/fix/whiteboard_confirmation/.github/pr-evidence/whiteboard-confirmation/command-ci.png)

完整日志和执行命令见 [测试证据目录](https://github.com/zxwang6/dingtalk-workspace-cli/tree/fix/whiteboard_confirmation/.github/pr-evidence/whiteboard-confirmation)。

## Notes

关联：#1384。此 PR 需先合入 main，供后续功能 PR 使用基线拥有的兼容性授权。
