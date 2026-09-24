# DWS 委托身份透传

## 范围

Managed Agent 宿主可为一次命令注入委托人身份。DWS 校验输入组合，在内置 MCP 网关请求中透传；实际登录 token、profile 和工具 arguments 保持原有语义。参数隐藏不代表可信身份或鉴权通过。

数字员工识别、工具白名单、ID 解析、身份一致性及业务联合权限由网关和服务端实现。本 PR 不实现或证明这些服务端能力。

## 参数与 Header

三个参数均为全局 hidden persistent flag，可放在业务命令前后；普通 Help 和 Agent Schema 不展示。

| CLI 参数 | DWS → 网关 Header | 含义 |
| --- | --- | --- |
| `--delegator-user-id` | `delegator-user-id` | 委托人在指定组织内的 User ID |
| `--delegator-corp-id` | `delegator-corp-id` | 上述 User ID 所属组织 |
| `--delegator-open-dingtalk-id` | `delegator-open-dingtalk-id` | 委托人的 Open DingTalk ID，服务端结合可信接收方上下文解析 |

```sh
dws doc +fetch --node '<node-id>' --delegator-user-id '<user-id>' --delegator-corp-id '<corp-id>'
dws doc +fetch --node '<node-id>' --delegator-open-dingtalk-id '<open-dingtalk-id>'
```

宿主应从可信任务触发上下文取得身份，不能由模型根据历史消息作者或业务查询结果猜测。User ID 与 corpId 必须是一组；数字员工在 A 组织登录，委托人在 B 组织时应传 B，不从当前 profile 补齐。

## 校验和降级

| 输入 | 行为 |
| --- | --- |
| 未提供 | 普通调用，不添加委托 Header |
| User ID + corpId | 透传两项 |
| 仅 Open DingTalk ID | 透传该项 |
| 三项都有 | 全部透传，由服务端校验是否同一人 |
| User ID/corpId 只有一项，即使同时有 Open DingTalk ID | 整组省略，普通调用 |
| 显式空值、空白、控制字符、无效 UTF-8、重复同一参数但值不同 | 整组省略，普通调用 |
| 重复同一参数且值完全相同 | 接受 |

值按原始 UTF-8 字符串写入 HTTP Header，不进行 JSON、URL 或 Base64 编码，不修剪或改写身份。协议尚未约定业务长度上限，本实现不自行设置 ID 长度限制，仍受 HTTP 栈和网关限制。Header 字段名大小写遵循 HTTP 语义。

本地降级在发起业务请求前完成，不先发委托请求、失败后再以普通身份重放。网关的解析失败、身份不一致或白名单未命中按服务端协议处理；CLI 不据此发起身份降级重试，业务错误按原规则返回。

## 生命周期和透传边界

- 在根命令执行入口消费参数，生成不可变的请求 context；参数解析状态随即清理。复用 Cobra 命令树的后续调用不能继承前次新增委托身份；Help 和解析失败路径也清理参数。
- 同一命令中的 shortcut 多请求、允许的 dry-run 辅助读取和现有重试继承相同 context。业务响应不能改写委托身份。纯本地 dry-run 保持现有零网络屏障。
- 只向 DWS 已识别的四个 HTTPS MCP 网关主机（生产、预发及对应国际域名，默认端口或 443）添加委托输入。第三方/插件、自定义非网关端点、stdio、直接 OpenAPI、换票辅助请求和 `mcp-meta` 发现请求不支持本协议。
- 不向共享身份 Header 注入委托信息。保留字段由本次 context 最终决定，其他 edition/credential/plugin Header 来源不能覆盖或伪造。
- 跨源重定向清除全部委托 Header；重定向链返回原始源时也不会恢复。同源重定向保留。
- DWS 从不发送 `delegator-uid`。该字段仅由网关依据服务端解析结果向业务系统注入。
- 命令计时记录和结构化 Header 日志对委托字段脱敏；不增加身份原文日志。

嵌入调用者应给每个并发 CLI 执行创建独立命令树；Cobra 树自身不支持并发 Execute。请求 context 可由同一执行内的并发子请求共享，禁止调用方在辅助请求中丢弃 context。

### 业务调用链的上下文要求

全局参数可解析不等于每个业务入口已经完成透传。命令必须从 `cmd.Context()` 向 MCP 调用、分页和重试传递上下文；重新创建 `context.Background()` 会丢失委托身份。AI 表格的主服务、辅助服务、记录分页、视图更新和工作流发布已按此要求修复。其他产品仍有无上下文的旧调用入口，需要逐项迁移，当前不能宣称所有命令均已覆盖。

回归测试从真实 AI 表格命令入口经过运行器到 HTTP 请求，检查三个委托 Header 的具体值，并验证同一命令树下一次不传参数时不会继承旧身份。`extra_headers_count` 仅统计额外 Header 数量，不能代替字段值检查，也不能证明服务端委托授权成功。

## 旧文档参数兼容

`--principal-user-id` 继续使用原文档域 `drive-internal.check_capability` 流程。新参数不作为其别名，也不跳过或替代其校验。

同一次调用不得混用非空 `--principal-user-id` 和任一显式 `--delegator-*` 参数，包括值相同的情况。旧入口缺少组织及 Open DingTalk ID 上下文，CLI 无法证明两套身份等价，因此返回 `conflicting_delegation_protocols`，不执行业务请求。后续若迁移旧入口，需要单独定义兼容契约并验证服务端覆盖。

## 服务端接入依赖

DWS 将有效输入交给网关，网关按约定传给 Portal 的 `delegatorUserId`、`delegatorCorpId`、`delegatorOpenDingtalkId` 字段。Portal 仅在真实数字员工 UID 与实际 toolName 命中白名单且委托身份有效时返回 `delegatorUid`；网关据此向业务系统添加 `delegator-uid`。

按当前协议，未提供、无效、解析失败、身份不一致或未命中白名单时，服务端继续普通调用。因此携带 CLI 参数本身不保证委托权限已经生效。上线验收需补充网关接收、Portal 解析和业务联合权限的真实联调证据。
