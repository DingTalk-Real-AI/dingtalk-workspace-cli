---
category: Added
---

- 新增全局隐藏参数 `--delegator-user-id`、`--delegator-corp-id` 和 `--delegator-open-dingtalk-id`，按调用隔离并向内置 MCP 网关透传委托身份；本次先完成 AI 表格的 DWS 侧接入与回归验证，其他业务按 `docs/delegator-business-integration.md` 逐项迁移。无效输入整组省略；旧文档委托入口保持兼容，新旧协议禁止混用。实际委托授权依赖网关和业务服务接入。
- 修复 AI 表格旧调用入口丢弃命令上下文的问题，覆盖主服务、辅助服务、记录分页、重试、视图更新和工作流发布，使委托身份与取消信号随请求传递；补充真实 AI 表格命令到 HTTP Header 的回归测试与业务接入指南。
