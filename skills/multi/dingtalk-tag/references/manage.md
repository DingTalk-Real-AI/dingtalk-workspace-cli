# dingtalk-tag manage — 数字员工生命周期管理

命令前缀 `dws dingtalk-tag manage`。全部针对数字员工配置本体，含高影响写与不可逆删除。

## create — 创建草稿态数字员工

```
Usage:
  dws dingtalk-tag manage create --name <名称> --description <职责描述> [flags]
Flags:
  --name             必填，同组织内唯一（≤30 Unicode 码点）
  --description      必填，职责描述（≤300 码点）
  --dept-id          归属部门 ID；可选，省略时服务端补操作人主任职部门
  --avatar-url       公网 HTTP(S) 头像地址，或本地图片路径（≤10 MiB）
  --supervisor-user-id 直属上级 userId
  --main-program-type 可选：open_code | local_agent；无特殊要求默认不传
  --response-mode    mention_only | targeted_proactive | mention_only,targeted_proactive；local_agent 可省略
Example:
  dws dingtalk-tag manage create --name "周报助手" --description "汇总并推送团队周报" --avatar-url ./avatar.png --dry-run --format json
```

只建草稿，不会上线。不传 `--dept-id` 时，OpenAPI 查询操作人主任职部门并补齐；CLI 不接收部门名称。`--avatar-url` 传 HTTP(S) 时直接使用，传本地 jpg/jpeg/png/gif/webp 时复用 Skill 本地文件上传封装，组合执行“先创建草稿 → 上传头像 → 回写草稿”；后两步失败时保留已创建的 `agentUuid`，禁止重复 create。用户只感知 `avatarUrl`，不需要手动调用上传接口。

`create` 当前为 `confirmation=not_required`：先用 `--dry-run` 核对，确认参数无误后移除 `--dry-run` 执行即可，不要额外猜测或重复创建。

MCP 的主程序类型字段为 `type`，仅支持 `open_code`、`local_agent`，`a2a` 暂不支持；CLI 对应参数仍是 `--main-program-type`。没有特殊要求时默认按 `open_code` 处理，因此创建时必须至少传一个 `--response-mode`；`local_agent` 可省略。详情命令的 `--type draft|published` 表示配置来源，不是主程序类型。主管参数和返回都使用字段名 `supervisorUserId`，字段值为当前组织内的 `userId`，不对用户暴露 uid/robotUid 概念。工号由平台管理，本命令不提供 `employee-no`。

## detail / list — 查询

```
Usage:
  dws dingtalk-tag manage detail --agent-uuid <agentUuid> [--type draft|published]
  dws dingtalk-tag manage list [--keyword <关键词>] [--main-program-type open_code|local_agent] [--page 1] [--page-size 20]
Example:
  dws dingtalk-tag manage detail --agent-uuid <agentUuid> --format json
  dws dingtalk-tag manage list --keyword "周报" --main-program-type local_agent --format json
```

`--keyword` 按名称或职责等可见基础信息模糊匹配，不对外提供工号搜索语义。`--main-program-type` 会映射到 MCP 的 `type`，仅支持 `open_code`、`local_agent`，不传表示不过滤。`--page` / `--page-size` 均不得小于 1。

`detail` 的 `--type` 默认为 `draft`；需要核对已发布配置时显式传 `--type published`。数字员工详情只有 `draft` / `published` 两种配置来源，不额外返回 `snapshot`；Skill/MCP 资源才有独立 snapshot。人员标识统一为 `userId`，直属上级的输入与输出字段统一为 `supervisorUserId`，数字员工 ID 统一为 `agentUuid`。

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
  supervisor-user-id / main-program-type / response-mode
```

`save-draft` 只按字段更新数字员工基础草稿：未传字段保持原值。Skill/MCP 的创建、更新、删除统一使用 `dws dingtalk-tag capability skill|mcp ...`，本命令不接受完整 Skill/MCP 数组。成功响应与 `detail --type draft` 结构一致，用于立即确认保存结果。

响应模式规则按合并后的草稿判断：`open_code` 必须至少有一个合法 `responseMode`，`local_agent` 可以没有。显式切换为 `open_code` 时 CLI 要求同时传 `--response-mode`；没有修改类型或响应模式时不要求用户重复回填，OpenAPI 会结合当前草稿做最终校验。

`--avatar-url` 可传可公开访问的 HTTP(S) 地址，也可传本地图片路径。传本地文件时 CLI 复用 Skill 上传封装，自动取得临时上传凭证、完成 multipart 上传，再将 OSS URL 作为 `avatarUrl` 保存；不输出临时凭证，用户无需手工编排上传步骤。

写操作，需用户确认：先 `--dry-run`，确认后加 `--yes`。MCP 敏感配置只通过 capability MCP 的本地 `--config-file` 传入，不要拼进命令行或提交到代码库。

## publish — 发布

```
Usage:
  dws dingtalk-tag manage publish --agent-uuid <agentUuid> [--allow-join-group]
```

**不携带任何配置**，只发布当前已保存的完整草稿。`local_agent` 只要求名称和职责描述，不因平台模型、Prompt、Skill 或 MCP 未配置而阻断。`open_code` 仍按平台运行所需配置校验。

`--allow-join-group` 是可选布尔，控制是否允许加入群聊。

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
