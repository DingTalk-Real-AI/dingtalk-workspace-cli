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

## 当前本机映射

当前 Codex 中的 DWS skill/办公能力验证场景使用：

| 项目 | 值 |
|---|---|
| 目标组织 | 阿里巴巴 |
| 稳定 profile | `dingd8e1123006514592:04061459256343` |
| 渠道 | EI智能体评测 |
| `DWS_CHANNEL` | `18451e165920b301ade00efae99b2c253e1e900b` |

登录：

```bash
DWS_CHANNEL='18451e165920b301ade00efae99b2c253e1e900b' \
  dws auth login \
  --profile 'dingd8e1123006514592:04061459256343' \
  --format json
```

后续命令：

```bash
DWS_CHANNEL='18451e165920b301ade00efae99b2c253e1e900b' \
  dws <product> <command> \
  --profile 'dingd8e1123006514592:04061459256343' \
  --format json
```

2026-07-22 已用 CLI v1.0.54 验证：不带渠道码时阿里巴巴组织登录被拒；带上述渠道码后 OAuth 登录成功，随后 `minutes list all` 调用成功。

## 组织白名单

当前服务端渠道配置组织白名单：

```text
793652894
515819978
21001
```

该数字组织白名单属于服务端控制面，不等同于本地 `profile` 中的字符串 `corpId`。禁止自行推导两者的映射。

## 已登记渠道

下表来自 `availableChannels` 渠道目录，不代表任一组织实时返回的 `allowedChannels`。执行登录时仍以服务端组织策略为准。

| channelCodeTitle | channelCode | 适用宿主/业务 |
|---|---|---|
| QoderWork | `51d4ceade40174304fc591dbf17448aeebf50328` | 阿里云 qteam 桌面 Agent |
| Oneday | `2a4a658e467998befb7fa333c19ba2b3a3bacfa4` | 自然语言获取、加工并写回文档 |
| ei_shuziren_v1 | `17590d48d77f9b1ab47ec7d37c16d9ff513f96fb` | 集团业务线数字员工 |
| ideaLAB | `d3417efb28cf4b8dc0c074705e74eb6208f54154` | 企业级 Agent 构建与智能应用平台 |
| QoderWake | `a3f5d3f4ab8cb87a2a1c93cbf69c713dab11b16e` | 持久身份、长期记忆的数字员工 |
| AI-SOP | `1e54a489615d81b3a6815d1e99b22ea6358303fd` | 天猫国际运营 AI 助理 |
| AgentX | `394af5a87e1044dbbb52ad037f49e2b74bea8881` | 菜鸟 AI 供应链平台 |
| EI智能体评测 | `18451e165920b301ade00efae99b2c253e1e900b` | DWS 办公能力与 Agent 自动化评测 |
| it-digital-human | `49edc1b7679b8d9f804af345920adbbb9472f2a6` | IT 数字人 |
| Devix | `bc623574bfc80b76b642e35f676d42e1921ee6ef` | AONE AI Native 研发平台 |
| ai-lab-agent | `dbd7cf1fea014e6b1d22bfbd179d00b6523b0e39` | 企业发展 Web Agent |
| Otter | `d7b20e3e1a154102deb2cf4b785c9b86aecbed83` | 淘天数据平台 OtterAgent |
| leto-dws | `ea51a28080a83b7466943c2b87f6d3f256460233` | 章鱼 DWS |
| CRM AI助理 | `cfeeb8e52530f77e1c4698b6d2faf4f2bf38008b` | 阿里云 CIO CRM AI 助理 |
| 法务AI助理桌面端 | `2b11bed6c34473aba7998d44801c1ad5cccb17b9` | 法务桌面 Agent |
| qianwen-aiworks | `b3e3d7e31ca3a5d626943a5cb72acb70b4fc76e3` | 千问 aiworks |
| Nexa | `02dbe0983567bb3bc2977e0a80826cb97ca0a1e9` | 直播业务 AI-Native 组织 |

## 排查顺序

1. 运行 `dws profile list --format json`，解析目标组织的稳定 `profile`。
2. 按真实宿主从登记表选择渠道；当前本机 Codex 规则命中“EI智能体评测”。
3. 使用命令级 `DWS_CHANNEL` 重新执行 `dws auth login --profile ... --format json`。
4. 使用相同 `DWS_CHANNEL` 和 `profile` 执行一个最小只读产品命令验证。
5. 若仍失败，加 `--verbose` 重试一次并按原始服务端错误分类；禁止轮询尝试整张渠道表。
