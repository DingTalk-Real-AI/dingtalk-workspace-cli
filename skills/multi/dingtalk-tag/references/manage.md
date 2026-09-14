# dingtalk-tag manage — 数字员工生命周期管理

命令前缀 `dws dingtalk-tag manage`。全部针对数字员工配置本体，含高影响写与不可逆删除。

## create — 创建草稿态数字员工

```
Usage:
  dws dingtalk-tag manage create --name <名称> --description <职责描述> [flags]
Flags:
  --name             必填，同组织内唯一（≤30 Unicode 码点）
  --description      必填，职责描述（≤300 码点）
  --dept-id          归属部门 ID
  --dept-name        归属部门名称
  --icon             头像地址或 OSS objectPath
  --employee-no      工号（≤64 码点）
  --position-name    岗位名称（≤128 码点，发布前必填）
  --supervisor-uid   直属上级钉钉 uid
  --main-program-type 可选：open_code | local_agent；无特殊要求默认不传
  --response-mode    mention_only | targeted_proactive | mention_only,targeted_proactive（发布前必填）
  --profile-json     档案 JSON 对象；独立档案 flag 覆盖其同名字段
Example:
  dws dingtalk-tag manage create --name "周报助手" --description "汇总并推送团队周报" --dry-run --format json
```

只建草稿，不会上线。`--position-name` 与 `--response-mode` 是发布的前置条件，可在此处给或后续用 `save-draft` 补。

`create` 当前为 `confirmation=not_required`：先用 `--dry-run` 核对，确认参数无误后移除 `--dry-run` 执行即可，不要额外猜测或重复创建。

`--profile-json` 只接收 `employeeNo`、`positionName`、`directSupervisorUid`、`mainProgramType`、`responseMode` 五个字段。`mainProgramType` 仅支持 `open_code`、`local_agent`，`a2a` 暂不支持：没有特殊要求时默认不传，CLI 保持省略，由 OpenAPI 按 `open_code` 处理；用户明确要求时也可以显式传 `open_code`；只有明确接入本地 Agent/DSH 时才传 `local_agent`。`responseMode` 支持单值，也支持用英文逗号分隔的双值组合，CLI 会规范化为 `mention_only,targeted_proactive`。独立档案 flag 会覆盖 `profile-json` 的同名字段，包括独立的 `--main-program-type` 覆盖 `profile-json.mainProgramType`。

## detail / list — 查询

```
Usage:
  dws dingtalk-tag manage detail --agent-uuid <agentUuid> [--type draft|published]
  dws dingtalk-tag manage list [--keyword <关键词>] [--main-program-type open_code|local_agent] [--page 1] [--page-size 20]
Example:
  dws dingtalk-tag manage detail --agent-uuid <agentUuid> --format json
  dws dingtalk-tag manage list --keyword "周报" --main-program-type local_agent --format json
```

`--keyword` 按名称、岗位或工号模糊匹配。`--main-program-type` 仅支持 `open_code`、`local_agent`，不传表示不过滤。`--page` / `--page-size` 均不得小于 1。

`detail` 的 `--type` 默认为 `draft`；需要核对已发布配置时显式传 `--type published`。所有数字员工 ID 统一使用 `agentUuid` / `--agent-uuid`，不要混用其它旧 ID 命名。

## login — 登录数字员工 DWS

```
Usage:
  dws dingtalk-tag manage login --agent-uuid <agentUuid> [--client-id <appId>]
Flags:
  --agent-uuid   必填，数字员工 ID
  --client-id    可选，用于授权的应用 ID；不传时由服务端选择默认应用
```

`login` 用于 A2A 或其他需要登录数字员工 DWS 的场景。企业接入本地 Agent/DSH 应使用 `dws dingtalk-tag connect`，不要使用 `manage login`。该命令会在内部完成以下步骤：

1. 查询已发布详情，取得可信的 `profile.corpId`、`profile.robotUid`、`profile.staffId`。
2. 申请临时 AuthCode，并使用同次响应的 `dwsClientId` 换票。
3. 使用新 Token 在线查询并核验员工身份：授权 `uid` 对应 `robotUid`，`staffId` 对应 Profile `userId`；授权 `orgId` 不作为 `corpId`。
4. 保存或刷新精确 `corpId:userId` Profile，同时保留发起操作的主管 Profile 为当前 Profile。

成功输出只包含保存后的 `dwsProfile` 和使用提示，不包含 AuthCode、Access Token 或 Refresh Token。单次以员工身份执行时，在目标命令的全局参数中传 `--profile <corpId:userId>`；需要切换默认账号时：`dws profile use <corpId:userId>`。不要再手动执行 `dws auth exchange`。

## save-draft — 全量覆写草稿

```
Usage:
  dws dingtalk-tag manage save-draft --agent-uuid <agentUuid> [flags]
Flags:
  --agent-uuid       必填
  --prompt           人设 / System Prompt（≤5000 码点）
  其余基础与档案字段同 create（name / description / icon / dept-* / employee-no /
  position-name / supervisor-uid / main-program-type / response-mode / profile-json）
  --skills-file      Skill 草稿配置 JSON 数组；不传保持原关联，显式 [] 才清空
  --mcps-file        MCP 草稿配置 JSON 数组；不传保持原关联，显式 [] 才清空
```

**基础字段和档案字段是全量覆写，不是增量 patch：未传字段会被清空。Skill/MCP 关联是例外：对应文件不传时保持原配置，只有显式传入空数组文件才清空。**

正确的增量修改姿势：

1. 先 `dws dingtalk-tag manage detail` 查出当前完整配置
2. 把**其它全部仍需保留的基础与档案字段**连同要改的字段一并带上；`mainProgramType` 没有特殊要求时默认不传，也允许显式传 `open_code`，只有明确保持或切换为本地 Agent/DSH 模式时才传 `local_agent`；需要变更 Skill/MCP 时再提供对应文件
3. 整体提交

只传要改的那一个字段会把其它字段全部清空，且这个后果在 `--dry-run` 的参数预览里看不出来（预览只显示你传了什么，不显示"没传的会被清掉"）。

特别注意 `--icon`：不要把 `detail` 返回的临时 `iconUrl` 直接回填持久化——那是带时效的临时地址。

写操作，需用户确认：先 `--dry-run`，确认后加 `--yes`。

关联文件格式示例：

```json
[{"skillId":"<skillId>","enabled":true,"attributes":{}}]
```

```json
[{"mcpId":"<mcpId>","enabled":true,"config":{}}]
```

MCP 凭据只允许通过服务端支持的安全引用传入，不要把明文密钥写进关联文件或提交到代码库。

## publish — 发布

```
Usage:
  dws dingtalk-tag manage publish --agent-uuid <agentUuid> [--allow-join-group]
```

**不携带任何配置**，只发布当前已保存的草稿。名称、头像、描述、组织、岗位、响应模式、人设任一缺失都会返回 `INVALID_PARAM`——先 `detail` 确认齐全再发。

`--allow-join-group` 是可选布尔，控制是否允许加入群聊。

## delete — 删除

```
Usage:
  dws dingtalk-tag manage delete --agent-uuid <agentUuid>
```

**不可逆，且可能有跨系统副作用。** 失败时不要盲目重试，先 `detail` 确认该数字员工是否仍存在——重试可能作用在已被部分删除的状态上。

## 硬约束

- `save-draft` 对基础与档案字段是全量覆写；不先 `detail` 就提交会静默清空未传字段。`skills-file` / `mcps-file` 不传则保持原关联，显式 `[]` 才清空。
- `save-draft` / `publish` / `delete` 必须 `--dry-run` + 用户确认后再 `--yes`。
- 不要传 `--org-id` / `--user-id`：identity 由可信登录态注入，不对 CLI 暴露。

## 跨产品协作

- 查上级或成员的 `uid` / `userId`：切换通讯录 / AI 搜问能力，或用 `dws aisearch person --query "姓名"`。
- 发布后要看执行情况：见 [`run.md`](./run.md)。
