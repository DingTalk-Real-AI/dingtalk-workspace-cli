# dingtalk-tag manage — 数字员工生命周期管理

命令前缀 `dws dingtalk-tag manage`。全部针对数字员工配置本体，含高影响写与不可逆删除。

## create — 创建草稿态数字员工

```
Usage:
  dws dingtalk-tag manage create --name <名称> --description <职责描述> --type <open_code|local_agent> [flags]
Flags:
  --name             必填，同组织内唯一（≤30 Unicode 码点）
  --description      必填，职责描述（≤300 码点）
  --dept-id          归属部门 ID；可选，省略时服务端补操作人主任职部门
  --avatar-url       公网 HTTP(S) 头像地址，或本地图片路径（≤10 MiB）
  --supervisor-user-id 直属上级 userId
  --type             必填：open_code | local_agent；必须显式填写且不能为空
  --response-mode    mention_only | targeted_proactive | mention_only,targeted_proactive；未提供或空值时默认 mention_only
Example:
  dws dingtalk-tag manage create --name "周报助手" --description "汇总并推送团队周报" --type open_code --avatar-url ./avatar.png --dry-run --format json
```

只建草稿，不会上线。不传 `--dept-id` 时，OpenAPI 查询操作人主任职部门并补齐；CLI 不接收部门名称。`--avatar-url` 传 HTTP(S) 时直接使用，传本地 jpg/jpeg/png/gif/webp 时复用 Skill 本地文件上传封装，组合执行“先创建草稿 → 上传头像 → 回写草稿”；后两步失败时保留已创建的 `agentUuid`，禁止重复 create。用户只感知 `avatarUrl`，不需要手动调用上传接口。

`create` 当前为 `confirmation=not_required`：先用 `--dry-run` 核对，确认参数无误后移除 `--dry-run` 执行即可，不要额外猜测或重复创建。

`create` / `save-draft` 的 MCP 主程序类型字段为 `digitalTagEmployeeProfile.type`，仅支持 `open_code`、`local_agent`，`a2a` 暂不支持；CLI 对应参数为 `--type`。创建时必须显式传 `--type open_code|local_agent`，缺失或空值由 DWS 本地拦截，不自动选择类型；未提供 `--response-mode` 或值为空时，CLI 默认发送 `mention_only`（包括 `local_agent`）。更新时未传主程序类型或响应模式则不更新对应字段。注意：`create` / `list` / `save-draft` 的 `--type` 表示主程序类型，而 `detail` 的 `--type` 是 `--snapshot draft|published`（配置来源）的兼容别名，分属不同子命令，不要混淆。主管参数和返回都使用字段名 `supervisorUserId`，字段值为当前组织内的 `userId`，不对用户暴露 uid/robotUid 概念。工号由平台管理，本命令不提供 `employee-no`。

## detail / list — 查询

```
Usage:
  dws dingtalk-tag manage detail --agent-uuid <agentUuid> [--snapshot draft|published]
  dws dingtalk-tag manage list [--keyword <关键词>] [--type open_code|local_agent] [--page 1] [--page-size 20]
Example:
  dws dingtalk-tag manage detail --agent-uuid <agentUuid> --format json
  dws dingtalk-tag manage list --keyword "周报" --type local_agent --format json
```

`--keyword` 按名称或职责等可见基础信息模糊匹配，不对外提供工号搜索语义。`--type` 会映射到 MCP 的 `type`，仅支持 `open_code`、`local_agent`，不传表示不过滤。`--page` / `--page-size` 均不得小于 1。

`detail` 的 `--snapshot` 默认为 `draft`；需要核对已发布配置时显式传 `--snapshot published`。该参数映射到 MCP 的 `snapshot` 字段；旧 `--type` 参数作为兼容别名，也发送 `snapshot`。响应中的 `snapshot` 明确本次配置来源（draft/published），与生命周期 `status` 独立；未成功发布的员工查询 published 返回 `NOT_FOUND`，不回退到 dev 对象。曾发布后下架的员工仍可读取保留的 published 配置，`status=offline` 不代表本地 Agent 正在运行。Skill/MCP 资源也使用 snapshot。人员标识统一为 `userId`，直属上级的输入与输出字段统一为 `supervisorUserId`，数字员工 ID 统一为 `agentUuid`。

## login — 登录数字员工 DWS

```
Usage:
  dws dingtalk-tag manage login --agent-uuid <agentUuid> [--client-id <appId>]
Flags:
  --agent-uuid   必填，数字员工 ID
  --client-id    可选，用于授权的应用 ID；不传时由服务端选择默认应用
```

`login` 用于 A2A、仅保存 Profile 或其他需要登录数字员工 DWS 的场景。企业真正接入本地 Agent/DSH 时使用 `dws dingtalk-tag connect`。该命令会在内部完成以下步骤：

1. 查询已发布详情，内部取得登录所需的组织与员工身份；对用户只展示 `corpId` 和 `userId`。
2. 申请临时 AuthCode，并使用同次响应的 `dwsClientId` 换票。
3. 使用新 Token 在线核验数字员工身份；内部标识换算不作为用户需要填写的参数。
4. 保存或刷新精确 `corpId:userId` Profile，同时保留发起操作的主管 Profile 为当前 Profile。

成功输出只包含保存后的 `dwsProfile` 和使用提示，不包含 AuthCode、Access Token 或 Refresh Token。单次以员工身份执行时，在目标命令的全局参数中传 `--profile <corpId:userId>`；需要切换默认账号时：`dws profile use <corpId:userId>`。不要再手动执行 `dws auth exchange`。

## save-draft — 更新草稿

```
Usage:
  dws dingtalk-tag manage save-draft --agent-uuid <agentUuid> [flags]
Flags:
  --agent-uuid       必填
  --prompt           人设 / System Prompt（≤5000 码点）
  其余可更新字段：name / description / avatar-url / dept-id /
  supervisor-user-id / type / response-mode
```

`save-draft` 只按字段更新数字员工基础草稿：未传字段保持原值；只传 `agent-uuid` 是空更新，服务端只回读草稿，不修改配置。显式空字符串或纯空白（包括 name、dept-id、prompt）会报参数错误，不表示清空；字段可缺省与允许空字符串是两种不同语义。Skill/MCP 的创建、更新、删除统一使用 `dws dingtalk-tag capability skill|mcp ...`，本命令不接受完整 Skill/MCP 数组。成功响应与 `detail --snapshot draft` 结构一致，用于立即确认保存结果。

更新时未传 `--response-mode` 就不发送该字段，保留草稿原值；不会补写创建时的默认值。显式传入时按合法响应模式更新，服务端结合当前草稿校验。

`--avatar-url` 可传可公开访问的 HTTP(S) 地址，也可传本地图片路径。传本地文件时 CLI 复用 Skill 上传封装，自动取得临时上传凭证、完成 multipart 上传，再将 OSS URL 作为 `avatarUrl` 保存；不输出临时凭证，用户无需手工编排上传步骤。

写操作，需用户确认：先 `--dry-run`，确认后加 `--yes`。MCP 敏感配置只通过 capability MCP 的本地 `--config-file` 传入，不要拼进命令行或提交到代码库。

## publish — 发布

```
Usage:
  dws dingtalk-tag manage publish --agent-uuid <agentUuid>
```

发布当前已保存的完整草稿。`local_agent` 无需用户配置平台人设；DWS 在发布前读取草稿，仅在人设缺失时自动保存默认人设，已有非空人设不会被覆盖。`open_code` 不自动补人设。预览只列出条件补齐与发布计划，不访问远端。若默认人设保存成功而发布失败，默认值保留在草稿，查询草稿后再重试。其他发布所需配置由服务端校验。创建默认发送 `mention_only`；历史草稿缺少响应模式时，先通过 `save-draft --response-mode mention_only` 补齐。

## delete — 删除

```
Usage:
  dws dingtalk-tag manage delete --agent-uuid <agentUuid>
```

**不可逆，且可能有跨系统副作用。** 失败时不要盲目重试，先 `detail` 确认该数字员工是否仍存在——重试可能作用在已被部分删除的状态上。

## 硬约束

- `save-draft` 只更新显式基础字段，不负责 Skill/MCP 挂载或资源生命周期。
- `save-draft` / `publish` / `delete` 必须 `--dry-run` + 用户确认后再 `--yes`。
- 不要传 `--org-id` / `--user-id`：identity 由可信登录态注入，不对 CLI 暴露。

## 跨产品协作

- 查上级或成员的 `userId`：切换通讯录 / AI 搜问能力，或用 `dws aisearch person --query "姓名"`；不要要求用户提供 uid。
- 发布后要看执行情况：见 [`run.md`](./run.md)。
