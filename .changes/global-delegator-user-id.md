---
category: Added
---

- 新增全局隐藏参数 `--delegator-user-id`、`--delegator-corp-id` 和 `--delegator-open-dingtalk-id`，按调用隔离并向内置 MCP 网关透传委托身份。无效输入整组省略；旧文档委托入口保持兼容，新旧协议禁止混用。实际委托授权依赖网关和业务服务接入。
