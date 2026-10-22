---
category: Added
---

- 新增全局隐藏参数 `--delegator-user-id`、`--delegator-corp-id` 和 `--delegator-open-dingtalk-id`。根命令在本次执行作用域持有校验后的不可变身份快照，公共 runner 统一向四个内置 HTTPS MCP 网关透传；旧 helper 丢弃 context 时也能补入身份，无需按业务逐条改造。无效输入整组省略；旧文档委托入口保持兼容，新旧协议禁止混用。实际委托授权依赖网关和业务服务接入。
- 修复 AI 表格主服务、辅助服务、分页、重试、视图更新和工作流发布的上下文传递，保留取消信号；扩展真实命令到 HTTP Header 的回归矩阵，并覆盖串行 root 复用和多个预建 root 的身份隔离。同进程多 root 并发执行不在支持范围，并发调用使用独立 CLI 进程。
- 更新 DWS 业务接入指南：说明公共 runner 覆盖条件、真实命令测试和绕行通道的接入要求；插件、stdio、发现、scoped 换票、独立 published MCP、直接 OpenAPI、文件字节传输及事件长连接继续排除，跨源重定向清理和 UID 发送边界保持不变。
