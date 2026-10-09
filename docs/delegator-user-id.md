# DWS 委托身份透传

## 范围

Managed Agent 宿主可为一次命令注入委托人身份。DWS 校验输入组合，在内置 MCP 网关请求中透传；实际登录 token、profile 和工具 arguments 保持原有语义。参数隐藏不代表可信身份或鉴权通过。

委托身份由根命令的本次执行作用域持有，并在公共 `runtimeRunner` 的请求边界绑定。业务请求只要经过该 runner 且满足下文的网关边界，就会使用本次身份；旧 helper 即使重新创建 `context.Background()`，runner 也会补入身份快照，无需为透传身份逐条修改业务命令。

全局可解析不代表所有传输通道均支持。独立 published MCP 客户端、直接 OpenAPI、文件字节传输、事件长连接等绕过公共 runner 的通道不在本协议范围内。各业务仍需按[业务接入指南](delegator-business-integration.md)核查实际路径，并补充真实命令测试。

数字员工识别、工具白名单、ID 解析、身份一致性及业务联合权限由网关和服务端实现。本 PR 不实现或证明这些服务端能力。

## 参数与 Header

三个参数均为全局 hidden persistent flag，可放在业务命令前后；普通 Help 和 Agent Schema 不展示。

| CLI 参数 | DWS → 网关 Header | 含义 |
| --- | --- | --- |
| `--delegator-user-id` | `delegator-user-id` | 委托人在指定组织内的 User ID |
| `--delegator-corp-id` | `delegator-corp-id` | 上述 User ID 所属组织 |
| `--delegator-open-dingtalk-id` | `delegator-open-dingtalk-id` | 委托人的 Open DingTalk ID，服务端结合可信接收方上下文解析 |

```sh
dws aitable base list --limit 10 --delegator-user-id '<user-id>' --delegator-corp-id '<corp-id>' --format json
dws aitable base list --limit 10 --delegator-open-dingtalk-id '<open-dingtalk-id>' --format json
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

- 在根命令执行入口消费参数，校验后为当前 root 建立不可变的身份快照，并绑定本次执行作用域。参数解析状态随即清理；未提供或无效输入对应空快照。复用命令树的后续调用不能继承前次身份；Help 和解析失败路径也清理参数。
- 公共 runner 保留请求 context 已有的快照（包括显式空快照），仅在快照缺失时从当前执行作用域补入。旧 helper 丢弃 context 时也能补入本次身份；重试沿用首次捕获的快照，业务响应不能改写它。执行完成、校验失败、业务错误或 panic 时清理作用域，已经创建的请求 context 不被后续执行改写。
- 只向 `mcp-gw.dingtalk.com`、`pre-mcp-gw.dingtalk.com`、`mcp-gw.dingtalk.io`、`pre-mcp-gw.dingtalk.io` 四个主机的 HTTPS MCP 业务请求添加委托输入，仅允许默认端口或 443。第三方/插件、自定义非网关端点、stdio、`mcp-meta` 发现请求和使用 scoped token 的登录/换票辅助请求均排除。
- 独立 published MCP 客户端、直接 OpenAPI、文件上传下载的字节请求、事件长连接不因存在全局参数而获得委托身份。文件下载前若有经公共 runner 发出的网关元信息查询，按该查询自身的边界处理。
- 不向进程共享身份 Header 注入委托信息，也不保存进程级“当前委托人”。保留字段由当前 root 的本次快照最终决定，其他 edition/credential/plugin Header 来源不能覆盖或伪造。
- 纯本地 dry-run 保持现有零网络屏障；原本允许的 dry-run 辅助读取，仅在经过公共 runner 且满足传输边界时携带身份，不新增网络访问。
- 跨源重定向清除全部委托 Header；重定向链返回原始源时也不会恢复。同源重定向保留。
- DWS 从不发送 `delegator-uid`。该字段仅由网关依据服务端解析结果向业务系统注入。
- 命令计时记录和结构化 Header 日志对委托字段脱敏；不增加身份原文日志。

支持同一 root 串行复用，以及多个预先构造的 root 串行执行；每次执行都重新绑定自己的快照和 runner，不沿用其他 root 或上一次调用的身份。现有 helpers/auth 仍含进程共享状态，**不承诺同一进程内多 root 并发执行安全**；需要并发 CLI 调用时使用独立进程。不可变身份快照不等于整个命令运行时具备并发安全性。

### 业务调用链的上下文规范

公共 runner 负责委托身份绑定；业务代码仍应从 `cmd.Context()` 派生并传递上下文，以保留取消、超时和其他调用信息。runner 补入身份不会自动恢复 helper 已丢弃的取消信号或 deadline。新增代码应使用带 context 的入口；既有 Background 路径的清理按业务生命周期需要推进，不再是公共 runner 路径透传身份的前提。

本次真实命令回归矩阵包括已有 AI 表格入口，以及 `doc read`、`doc +fetch`、`calendar event get`、`todo task get`、`wiki feed list`。检查应从真实命令经过 runner 到 HTTP 请求，断言 Header 具体值、串行 root 复用及多个预建 root 的身份隔离；测试结果以实际执行报告为准。这些代表性入口不等于所有业务分支均已验证。`extra_headers_count` 仅统计额外 Header 数量，不能代替字段值检查，也不能证明服务端委托授权成功。

## 旧文档参数兼容

`--principal-user-id` 继续使用原文档域 `drive-internal.check_capability` 流程。新参数不作为其别名，也不跳过或替代其校验。

同一次调用不得混用非空 `--principal-user-id` 和任一显式 `--delegator-*` 参数，包括值相同的情况。旧入口缺少组织及 Open DingTalk ID 上下文，CLI 无法证明两套身份等价，因此返回 `conflicting_delegation_protocols`，不执行业务请求。后续若迁移旧入口，需要单独定义兼容契约并验证服务端覆盖。

## 服务端接入依赖

DWS 将有效输入交给网关，网关按约定传给 Portal 的 `delegatorUserId`、`delegatorCorpId`、`delegatorOpenDingtalkId` 字段。Portal 仅在真实数字员工 UID 与实际 toolName 命中白名单且委托身份有效时返回 `delegatorUid`；网关据此向业务系统添加 `delegator-uid`。

按当前协议，未提供、无效、解析失败、身份不一致或未命中白名单时，服务端继续普通调用。因此携带 CLI 参数本身不保证委托权限已经生效。上线验收需补充网关接收、Portal 解析和业务联合权限的真实联调证据。
