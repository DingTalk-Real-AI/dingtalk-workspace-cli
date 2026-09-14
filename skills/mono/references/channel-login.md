# 受控渠道与阿里巴巴组织登录

## 使用场景

在以下任一场景读取本参考：

- 目标组织是阿里巴巴；
- 登录返回 `CHANNEL_REQUIRED`、`channel_not_allowed`、`enterprise_not_authorized` 或“应用暂不受信任”；
- 用户提到渠道码、`DWS_CHANNEL`、渠道白名单或渠道归因；
- 需要判断 `DWS_CHANNEL` 与 `DINGTALK_DWS_AGENTCODE` 的边界。

## 核心契约

- 将 `DWS_CHANNEL` 作为产品/分发渠道 `channelCode`。CLI 在登录权限检查和后续 MCP 请求中把它发送为 `x-dws-channel`。
- 将 `DINGTALK_DWS_AGENTCODE` 作为执行 Agent 身份。两者是独立维度，禁止互相回填或复用。
- 在受控渠道组织中，把 `DWS_CHANNEL` 同时加到 `auth login` 和每一条后续 `dws` 命令。只在单条命令作用域设置，禁止写入 shell profile 或对其他组织全局导出。
- 仅使用与真实宿主/业务场景匹配的已登记渠道。禁止为了通过登录随机尝试其他渠道或伪装成别的产品。
- 把静态 `channelCode` 视为公开路由标识，不视为密钥或可信归因凭证。长期方案必须由服务端校验宿主身份并签发短期、绑定组织和渠道的会话凭证。

## 排查顺序

1. 运行 `dws profile list --format json`，解析目标组织的稳定 `profile`。
2. 确认当前宿主配置的渠道与真实业务场景匹配。
3. 使用命令级 `DWS_CHANNEL` 重新执行 `dws auth login --profile ... --format json`。
4. 使用相同 `DWS_CHANNEL` 和 `profile` 执行一个最小只读产品命令验证。
5. 若仍失败，加 `--verbose` 重试一次并按原始服务端错误分类；禁止轮询尝试其他渠道。
