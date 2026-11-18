# 核心 HTTP 出口代理

本版为统一网络策略的首批交付，解决资源下载固定直连（#1625）和 SafeChat key client 忽略代理（#1574）。

## 覆盖范围

| 链路 | 本版行为 |
| --- | --- |
| OAuth/设备流、token 刷新/撤销、App Token、OIDC、认证发现 | 使用本次调用的网络策略 |
| MCP JSON-RPC、Raw OpenAPI（包括分页、multipart） | 统一路由；保留超时、目标验证和重试规则 |
| localio 下载/上传、聊天资源完整/分段下载 | 默认采用环境代理，也可显式覆盖；保留凭据和文件发布边界 |
| SafeChat 取密钥 HTTP 请求 | 注入相同策略；SDK 独立使用时也采用标准 HTTP(S) 环境代理 |
| 白板导出、Stream/WebSocket、升级/安装器、其他独立 HTTP 客户端、外部 Agent | 尚未接入，继续遵循各自已有网络行为 |

全局参数不表示所有流量都已接入统一策略。本版不提供整个进程禁止直连的防火墙保证；需要这种保证时应继续实施沙箱/网络层出口限制。后续接入白板需适配其公网目标校验，接入事件需同时覆盖 HTTP 取票、WebSocket、重连和 SDK。

## 模式与优先级

`--proxy` > `DWS_PROXY` > 当前 profile 网络配置 > `auto`。

- `auto`：按目标协议采用 HTTP_PROXY / HTTPS_PROXY（或小写形式），没有对应配置则直连。各组取非空大写值优先，HTTP_PROXY 不兜底 HTTPS_PROXY。
- 完整代理 URL：对已覆盖的 HTTP/HTTPS 链路使用该代理，仍执行绕过规则。接受本机或远程 `http://`、`https://`、`socks5://`、`socks5h://`。SOCKS 沿用 Go HTTP Transport 的代理侧目标解析语义。
- `direct`：已覆盖链路强制直连，不采用代理或绕过列表。

DWS_PROXY 显式空值报错，恢复自动请 unset 或设置 auto。配置、连接、代理认证或 TLS 错误均不会自动切换成直连。

绕过列表独立取值：`--no-proxy` > `DWS_NO_PROXY` > 非空 NO_PROXY/no_proxy > 已保存配置 > 空。高优先级整体替换，参数或 DWS_NO_PROXY 显式空值清空用户列表。localhost 和字面量回环目标始终直连；代理地址本身允许是回环地址。

NO_PROXY 支持域名、IP、CIDR、可选端口和 `*`。`corp.example` 匹配自身及子域，`.corp.example` 只匹配子域。CIDR 对 URL 的 IP 字面量生效，**不会解析域名后再匹配**；企业内网域名需要单列。格式错误明确拒绝。

本版不读取系统代理/PAC 或 ALL_PROXY；解释输出的 all_proxy_ignored 会提示 ALL_PROXY 被忽略。

```bash
# 临时指定出口
dws --proxy http://127.0.0.1:7890 chat message download-media <原有参数>

# 环境变量方式，可在登录前使用
export HTTPS_PROXY=http://127.0.0.1:7890
export NO_PROXY=localhost,127.0.0.1,corp.example

# 恢复自动 / 强制已覆盖链路直连
dws --proxy auto <原命令>
dws --proxy direct <原命令>
```

## 持久配置

```bash
dws config network set --proxy http://127.0.0.1:7890 \
  --no-proxy 'localhost,127.0.0.1,::1,corp.example'
dws config network show --effective
dws config network reset
```

配置位于当前 DWS 配置目录的 network.json，遵循 DWS_CONFIG_DIR 和 edition 的目录选择。已登录时保存当前 profile 的覆盖；通过 --profile corpId:userId 选择另一已存在的 profile。未登录且未指定 profile 时保存默认值。profile 按字段继承默认配置，显式空绕过列表能覆盖默认列表。多 profile 的 MCP 执行逐身份解析配置及连接池，修改配置一次只接受一个 profile。

网络配置不读取 token、启动登录或访问业务数据。写入使用现有目录锁和原子替换，set/reset --dry-run 不写文件。reset 清除当前作用域，环境变量仍然有效。文件版本不兼容或格式损坏时明确报错。

带 URL userinfo 的持久保存会被拒绝。代理认证通过受控环境变量 DWS_PROXY 提供，配置查询和解释移除全部 userinfo。避免将密码或签名资源 URL 复制到工单、日志或 shell 命令历史。

企业 TLS 代理可通过 DWS_CA_BUNDLE 或 `config network set --ca-bundle /absolute/path/ca.pem` 追加系统根信任。DWS_CA_BUNDLE 优先，路径必须绝对且文件包含 PEM 证书；失败不关闭校验。`--ca-bundle ''` 清除已保存值。代理和目标仍须通过 TLS 主机名与证书验证。

## 离线诊断

```bash
dws config network explain --url https://storage.corp.example/resource
dws config network show --effective
```

explain 只计算路由，不联网、不解析 DNS。输出模式、来源、目标 origin、代理/直连、绕过原因、脱敏代理地址及额外 CA 是否启用；不输出 URL 路径、查询或 userinfo。show 不带 --effective 时解释已保存配置，带该参数时解释本次 flags/environment 覆盖后的配置。本版没有网络 probe；选择了代理不证明其可达或认证成功。

## 下载与迁移

目标策略和出口选择分开：localio 仍要求 HTTPS、禁止 URL userinfo，允许服务下发的专有部署私网目标及自定义端口，不额外强制本地解析公网地址。上传保留受信目标、443 端口和禁止跳转约束。重定向验证、跨 origin 凭据清理、大小限制和原子落盘保留。

历史 localio 固定直连，本版默认 auto：无代理配置保持直连，已有环境代理的下载/上传开始采用它。企业内网直连域名请加入 NO_PROXY，不默认绕过全部私网，因为有些内网只能经代理访问。

命令开始时快照配置，后续文件/环境变动不改变当前请求和重试的出口。命令结束关闭策略拥有的空闲连接，新命令重新解析。调用方注入的自定义 RoundTripper 仍由其拥有，不改写全局 DefaultTransport。

## Verification

本地验收使用纯本地 origin/CONNECT 代理和 fixture token，不使用真实账号或线上文件：

- 修改前禁止直连时代理 CONNECT 次数为 0，回归测试失败。
- 修改后经 CONNECT 完成 TLS 下载及原子发布；NO_PROXY 命中时直连；代理关闭不回退直连；额外 CA 验证成功，缺少信任失败。
- 完整 Cobra 组装的 dws api 请求经本地 CONNECT，保留原目标 Host 和 fixture token，直连尝试为 0。
- 优先级、空值、大小写、profile 隔离、快照、持久化、dry-run 和脱敏有回归测试。
- SafeChat 嵌套模块单独验证环境代理与 host transport 注入，不能用根模块 go test ./... 代替。

真实钉钉登录/刷新、真实加密消息及线上文件、未接入链路和跨平台原生运行不由上述本地用例证明。PR 描述列出实际执行命令、结果及未验证项。
