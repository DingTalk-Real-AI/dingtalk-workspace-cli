---
category: Added
---

- OIDC 登录错误诊断：`auth provider-login <provider>`、`auth exchange-prepare` 和 `auth token-exchange`
保留 Portal 的公开错误码和 trace ID，并将已定义错误码转换为可读说明。
覆盖配置、事务/state、授权码、身份校验、限流、上游调用和 DWS 会话签发错误。

- 从 `error.code`、`error.action`、`trace_id` 解析公开诊断；响应体缺失或无效时，
  使用合法的 `X-Dws-Trace-Id`。后端日志专用 `reason` 不进入客户端输出。
- 人类可读错误显示 `Trace ID` 和 `Server Code`。JSON 复用现有字段：
  传统输出为 `error.server_error_code`，统一输出为 `error.upstream_code`，
  两者均提供 `error.trace_id` 和 `error.details.action/http_status/provider`。
- `stop` 提示检查参数/配置或联系管理员；`restart_login` 提示重新登录或重新
  `exchange-prepare` 取码。保持单次请求，不自动重试旧 code/state。
- 保留现有错误分类、退出码和 token 响应格式。只保留 Portal 已定义的固定公开
  message，不回显任意异常正文、内部 reason 或请求凭证；诊断标识限制长度和字符。

验证包含真实本地 HTTP 错误响应、三条命令的 JSON/人类输出、无效响应的 trace 回退、
配置停止与重新登录提示、取消/超时分类、凭证反射过滤及不重试行为。
