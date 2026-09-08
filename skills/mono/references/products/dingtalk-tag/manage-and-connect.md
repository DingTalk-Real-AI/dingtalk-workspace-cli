# 数字员工生命周期与 DSH 接入

## 接入普通本地 Agent

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --agent-workdir <directory> --daemon --alwayson --dry-run --format json
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --agent-workdir <directory> --daemon --alwayson --yes --format json
dws dingtalk-tag connection status --agent-uuid <agentUuid> --format json
dws dingtalk-tag connection stop --agent-uuid <agentUuid> --format json
dws dingtalk-tag connection restart --agent-uuid <agentUuid> --format json
```

连接使用数字员工 Profile 启动 Event Consumer，调用与 dev connect 共用的 Agent 协议，最终以员工身份引用回复文本。后台结果只有在 ready 后才返回运行成功；不支持的 Agent 或缺失的依赖必须明确报错。

支持 `qoder/qoderwork/workbuddy/claudecode/codebuddy/codex/gemini/opencode/custom`；custom 使用 `--agent-cmd`，问题作为最后一个参数、stdout 作为答案。模型、工作目录、会话和权限参数沿用 dev connect，模型推理是否远端执行由 Agent 自身决定。

默认仅主管可用；`--allowed-users` 接收精确 userId 并在员工上下文解析，`--allowed-groups` 接收该上下文的群会话 ID，群消息仍需满足用户白名单。不要把机器人 staffId 直接当作员工事件开放 ID。

DSH 仍只注册配置，返回 `restartRequired=true`；不接受普通 Agent 的 `--daemon/--alwayson` 参数。旧 DSH binding 向后兼容，不自动迁移到其他 Adapter。

本地状态、会话、去重记录和无正文审计按员工隔离。未知回复结果或进程中断的任务需要核实，不自动重新执行 Agent；后台重启保留订阅重试预算。远端 ack/replay/cursor 尚未提供，不承诺 exactly-once 或断线不丢消息。


## 创建草稿

```bash
dws dingtalk-tag manage create \
  --name "<名称>" --description "<职责>" \
  --main-program-type local_agent \
  --position-name "<岗位>" --response-mode mention_only \
  --dry-run --format json
```

create 只创建草稿并返回 `agentUuid`。发布还需要头像、部门、岗位、响应模式和 Prompt 等服务端要求的完整配置。创建成功后必须立即保存 `agentUuid`；后续失败只从 detail/save-draft/publish 恢复。

## 修改：先读后全量保存

```bash
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type draft --format json
dws dingtalk-tag manage save-draft --agent-uuid <agentUuid> <完整保留字段和修改字段> --dry-run --format json
```

`save-draft` 不是 patch。必须回填所有仍需保留的字段，包括 `digitalTagEmployeeProfile.mainProgramType`、`skills` 和 `mcps`。不要把临时签名的 `iconUrl` 当长期 `icon` 回填。

## 发布、查询和删除

```bash
dws dingtalk-tag manage publish --agent-uuid <agentUuid> --dry-run --format json
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type published --format json
dws dingtalk-tag manage list --keyword "<关键词>" --format json
dws dingtalk-tag manage delete --agent-uuid <agentUuid> --dry-run --format json
```

当前版本没有独立下线命令。用户要求下线时应明确说明该限制，不得把不可逆的 `delete` 当作下线，也不要猜测未公开的 DEAP 工具。删除不可逆，先确认目标和影响。

## 只保存已有数字员工的本地 Profile

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --profile-only --dry-run --format json
# 用户确认后
dws dingtalk-tag connect --agent-uuid <agentUuid> --profile-only --yes --format json
```

前置条件：draft 的 `mainProgramType` 必须是 `local_agent`，且 published 详情存在。该模式只获取一次性授权信息、执行受管换票并保存数字员工独立 Profile；它保持主管 Profile 当前激活，不查询 operator、不保存 DSH binding、不调用或重启 DSH。成功结果的 `status` 为 `profile_saved`、`profileOnly` 为 `true`、`restartRequired` 为 `false`。

`--profile-only` 与 `--channel` 不能同时使用。后续需要接入 DSH 时，单独执行下面的 DSH 模式；它会重新获取一次性授权信息并幂等注册。

## 接入已有数字员工到 DSH

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --dry-run --format json
# 用户确认后
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --yes --format json
```

前置条件同上。该模式会保存数字员工独立 Profile、保持主管 Profile 当前激活，并继续解析 operator、保存 DSH binding 和幂等注册 DSH；它不会修改或发布员工，也不会自动重启 DSH。

成功结果在 DWS envelope 的 `data` 中返回 `status`、`agentUuid`、`dwsProfile`、`operatorOpenDingTalkId`、`protocolVersion` 和 `restartRequired`。若 Profile 已落盘但 DSH 注册失败，重新执行同一 connect 获取新授权码并幂等重试，不要重新创建员工。

## 创建并接入的一次请求

1. 一次收集创建和发布所需完整信息。
2. 汇总展示 create/save-draft/publish/connect 四阶段及影响，只确认一次。
3. 创建并记录 `agentUuid`。
4. 补全草稿并发布；任何阶段失败都报告 `agentUuid` 与下一条恢复命令。
5. 发布成功后单独执行 connect；connect 失败不回滚已发布员工。
