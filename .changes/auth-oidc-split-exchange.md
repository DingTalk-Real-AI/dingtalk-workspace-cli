支持业务分步完成第三方登录：先执行 `dws auth exchange-prepare --provider ant` 获取服务端授权参数，再由业务调用 IdP/SDK 取码，最后执行 `dws auth token-exchange --provider ant --code <code> --state <state>` 换取 DWS Token。

支持 `--code-stdin`、可选且必须成对的 `--client-id/--client-secret`，默认返回现有 TokenData；显式 `--save` 后在线核验完整身份并保存，保留已有默认账号。两个命令均支持 `--dry-run`，服务地址使用系统统一的 MCP 配置（`mcp_url` 文件或默认地址），不修改全局配置；保存 Token 时记录其刷新所用的 Portal 地址。PKCE verifier 始终由服务端保存，换票失败不会自动重试一次性 code/state。

命令沿用统一结果输出。作为第三方 IdP 集成入口，两个命令以 Cobra Hidden 隐藏，不出现在 `dws auth --help` 与 shell 补全中，仍可显式执行；不进入 Agent Schema（Hidden 且无 Contract.Identity），也不再占用 reviewed Schema 排除列表。

macOS 终端按实际文件描述符识别，使用 `select` 等待输入，不依赖 `/dev/tty` 或 `/dev/stdin` 文件名，覆盖 `< /dev/tty` 重定向；以 50ms 周期检查取消，不改变终端模式、不关闭借用句柄。高编号输入使用低编号描述符副本等待，完成后只关闭副本；低编号描述符耗尽时明确报错。

`--code-stdin` 的等待输入和后续换票共享一分钟超时，并响应调用方的取消或更短截止时间。取消、超时错误保留原有身份，不归类为参数错误，也不会继续换票。Unix 标准输入使用可取消的轮询读取，取消后保留原文件描述符；Windows 控制台保留原输入模式，通过原生 API 取消读取，重定向管道则通过关闭来中断。宿主通过 `SetIn` 注入的自定义阻塞输入须支持 `Close` 中断读取（取消时会关闭，正常完成时保留）。无 `Close` 的自定义 Reader 须为非阻塞或自行响应 context。

OIDC 换票响应增加实际 DWS `clientId`（与 IdP 的 `client_id` 不同）。`provider-login` 和 `token-exchange --save` 均保存该应用绑定和原 Portal 地址。默认应用继续由 Portal 托管密钥；显式指定的自定义应用将原始密钥保存到现有安全凭证槽，并与 Token/Profile 写入一起回滚。刷新自定义应用时从该槽读取密钥，发送给原 Portal 的 `/oauth2/refreshToken`，不回退到全局应用、不跟随重定向、不在结果或 app.json 中暴露密钥。未使用 `--save` 的分步换票不写入本地凭证。

需要同步部署 Portal：默认 OIDC 响应返回实际 `clientId`，刷新接口接受可选 `clientSecret`。旧版默认响应缺少应用绑定时，新 CLI 会明确要求更新 Portal；旧的缺失绑定记录需要重新登录。

OIDC 登录持久化时记录 `oidc_provider`，刷新成功继续保留；该本地元数据不加入未保存的 token-exchange 返回值。所有 OIDC 分支只通过该字段识别，不再从 Portal 地址或密钥模式推断登录来源。此前未记录该字段的开发测试凭证不做兼容迁移，需重新登录。

OIDC 刷新前校验原应用、Portal 和登录来源绑定，缺失时直接报错，不使用全局应用或默认端点兜底。刷新失败不读取组织镜像或历史全局凭证，即使同用户、同组织也不能切换应用。被拒绝或失效时提示重新执行 provider-login 或原分步登录流程，自定义应用需沿用原 client-id/client-secret；网络、取消、超时、限流和 5xx 保留原凭证，不强制重新登录。普通 OAuth 既有恢复策略保持不变。

已确认的 `invalidParameter.authCode.notFound` 仍保留脱敏后的错误类型和规范化业务码供分类使用；错误识别不再意味着允许 OIDC 凭证恢复。服务端原始错误文本不进入输出、日志或错误链。
