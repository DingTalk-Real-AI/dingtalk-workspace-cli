支持业务分步完成第三方登录：先执行 `dws auth exchange-prepare --provider ant` 获取服务端授权参数，再由业务调用 IdP/SDK 取码，最后执行 `dws auth token-exchange --provider ant --code <code> --state <state>` 换取 DWS Token。

支持 `--code-stdin`、可选且必须成对的 `--client-id/--client-secret`，默认返回现有 TokenData；显式 `--save` 后在线核验完整身份并保存，保留已有默认账号。两个命令均支持 `--dry-run` 和不修改全局配置的 `--mcp-url`；保存 Token 时记录其刷新所用的 Portal 地址。PKCE verifier 始终由服务端保存，换票失败不会自动重试一次性 code/state。

命令沿用统一结果输出及现有 auth 管理命令的精确 Schema 排除规则。

macOS 终端按实际文件描述符识别，使用 `select` 等待输入，不依赖 `/dev/tty` 或 `/dev/stdin` 文件名，覆盖 `< /dev/tty` 重定向；以 50ms 周期检查取消，不改变终端模式、不关闭借用句柄。高编号输入使用低编号描述符副本等待，完成后只关闭副本；低编号描述符耗尽时明确报错。

`--code-stdin` 的等待输入和后续换票共享一分钟超时，并响应调用方的取消或更短截止时间。取消、超时错误保留原有身份，不归类为参数错误，也不会继续换票。Unix 标准输入使用可取消的轮询读取，取消后保留原文件描述符；Windows 控制台保留原输入模式，通过原生 API 取消读取，重定向管道则通过关闭来中断。宿主通过 `SetIn` 注入的自定义阻塞输入须支持 `Close` 中断读取（取消时会关闭，正常完成时保留）。无 `Close` 的自定义 Reader 须为非阻塞或自行响应 context。

OIDC 换票响应增加实际 DWS `clientId`（与 IdP 的 `client_id` 不同）。`provider-login` 和 `token-exchange --save` 均保存该应用绑定和原 Portal 地址。默认应用继续由 Portal 托管密钥；显式指定的自定义应用将原始密钥保存到现有安全凭证槽，并与 Token/Profile 写入一起回滚。刷新自定义应用时从该槽读取密钥，发送给原 Portal 的 `/oauth2/refreshToken`，不回退到全局应用、不跟随重定向、不在结果或 app.json 中暴露密钥。未使用 `--save` 的分步换票不写入本地凭证。

需要同步部署 Portal：默认 OIDC 响应返回实际 `clientId`，刷新接口接受可选 `clientSecret`。旧版默认响应缺少应用绑定时，新 CLI 会明确要求更新 Portal；旧的缺失绑定记录需要重新登录。
