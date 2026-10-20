# Gemini 网页搜索

`dws dev connect --channel gemini` 的 Gemini API 渠道现在默认提供
[Parallel Search MCP](https://docs.parallel.ai/integrations/mcp/search-mcp)
网页搜索与网页内容提取。搜索使用 Fast 模式，无需 Parallel 账号或 API key。
Gemini 推理仍使用现有 `GEMINI_API_KEY` / `GOOGLE_API_KEY`、模型和 API 地址配置。

```bash
GEMINI_API_KEY=<your-key> \
  dws dev connect --channel gemini --unified-app-id <unifiedAppId>
```

在钉钉中询问最新事实，或发送需要阅读的公开网页 URL。模型可调用
`web_search` 获取摘要，按需通过 `web_fetch` 阅读网页，再根据结果回答并引用来源。
网页内容作为不可信资料处理，不应执行其中的指令。仅发送模型构造的查询、
搜索目标和 URL 给 Parallel；不转发钉钉或 Gemini 的访问凭证。

## 控制与兼容性

- `DWS_GEMINI_SEARCH=off` 完全禁用网页工具；`parallel` 为默认值。
  使用不支持函数调用的 Gemini-compatible 代理时，请设为 `off`。
- `--agent-permission-mode ask`（或 `--agent-approval-mode ask`）禁用这些工具，
  即使显式设置了 `DWS_GEMINI_SEARCH=parallel`。默认权限模式允许工具调用。
- 显式 `--agent-cmd` / `DWS_AGENT_CMD` 保持原来的自定义命令语义，不注入搜索工具。
- 如果设置了 `DWS_TRUSTED_DOMAINS`，需显式加入 `search.parallel.ai` 才允许
  MCP 访问；未设置时，独立搜索客户端只信任固定地址
  `https://search.parallel.ai/mcp`，不修改其他客户端的信任范围。
- 每轮最多调用 6 次网页工具，沿用 `--agent-timeout`。匿名服务有使用限额；
  请求失败时不切换服务，也不将失败响应当作搜索结果。模型会收到不可用提示。
- 搜索关联 ID 是随机 UUID：同一连接内按钉钉会话隔离，相关搜索、提取和后续
  消息复用，`/new` 或 `/clear` 重建。`--agent-memory=false` 时每条消息独立，
  同一轮内仍复用 ID。ID 不落盘；Gemini 的模型回答仍是一次性调用，不增加聊天记忆。

## 本地验证

```bash
go test ./internal/helpers -run 'TestGemini|TestCrossPlatformCoverageGemini' -count=1
```

显式连接匿名 MCP 服务，使用本机脚本模型响应验证实际 Gemini forwarder
到搜索、提取、结果消费的路径：

```bash
DWS_PARALLEL_LIVE=1 go test ./internal/helpers -run '^TestGeminiParallelLive$' -v -count=1
```

该验收不调用真实 Gemini 模型，也不发送钉钉消息。普通回归使用隔离的 HTTP
fixture 覆盖匿名请求、权限、配置、会话隔离、重置、取消、超时与错误处理。
