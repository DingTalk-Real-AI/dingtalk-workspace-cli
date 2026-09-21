# 数字员工生命周期与 DSH 接入

## 服务端设备绑定

connect 在接入 Agent 前调用服务端 bind，返回 ID 后持久保存。使用 `--dry-run --format json` 预览，确认后去掉 dry-run 并加 --yes。

设备 ID 缺省随机生成并在当前配置目录长期保存，也可显式传 `--device-id`；不要复制设备配置到另一台机器。`--local-agent-name` 可选，`--extensions` 传字符串。主管身份由网关注入，不传 identity/userId/orgId。

换机器前先在旧机器执行 `dws dingtalk-tag connect unbind --agent-uuid <agentUuid> --dry-run --format json` 并确认执行，等待旧实例释放且解绑成功。再在新机器执行 `dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --dry-run --format json`，确认后连接并保存新绑定 ID。两步之间员工未绑定；CLI 不提供远程停机或在线判断。

换 Agent 或设备均先解绑，再 connect 并保存新的服务端 ID。unbind 携带保存的 ID，成功后保留历史回执，重复解绑不解除后继绑定。有在途或待恢复任务时服务端拒绝；保留旧 ID，不启动新 Agent。status/list 新增的 serverBindingState 仅是本地回执，不证明服务端当前绑定或在线。

结果未知的 bind 必须先由服务端核对，禁止自动重试；confirmed 回执但本地提交失败时重试原参数命令。已提交的新绑定启动失败使用 restart。只保存 Profile 使用 `manage login`。

## 接入普通本地 Agent

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --agent-workdir <directory> --daemon --alwayson --dry-run --format json
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --agent-workdir <directory> --daemon --alwayson --yes --format json
dws dingtalk-tag connect status --agent-uuid <agentUuid> --format json
dws dingtalk-tag connect stop --agent-uuid <agentUuid> --format json
dws dingtalk-tag connect restart --agent-uuid <agentUuid> --format json
```

连接使用数字员工 Profile 启动 Event Consumer，调用与 dev connect 共用的 Agent 协议，最终以员工身份引用回复文本。后台结果只有在 ready 后才返回运行成功；不支持的 Agent 或缺失的依赖必须明确报错。

支持 `qoder/qoderwork/workbuddy/claudecode/codebuddy/codex/gemini/opencode/custom`；custom 使用 `--agent-cmd`，问题作为最后一个参数、stdout 作为答案。模型、工作目录、会话和权限参数沿用 dev connect，模型推理是否远端执行由 Agent 自身决定。

默认仅主管可用；`--allowed-users` 接收精确 userId 并在员工上下文解析，`--allowed-groups` 接收该上下文的群会话 ID，群消息仍需满足用户白名单。不要向用户索要其它内部人员标识。

DSH 注册后由正在运行的宿主员工级启动；宿主不可用时返回 `restartRequired=true`。不接受普通 Agent 的 `--daemon/--alwayson` 参数。旧 DSH binding 向后兼容，不自动迁移到其他 Adapter。

暂停使用 `connect stop`；解绑使用 `connect unbind --agent-uuid <agentUuid>`（不删除员工、Profile、Token 或审计）；换绑先完成 unbind，再使用 `connect --agent-uuid <agentUuid> --channel qoder`。自然语言要求后台接入时加 `--daemon --alwayson`。两步写操作均先 dry-run，用户确认后执行。旧实例必须停止并确认释放；unknown 或超时不能通过删配置强行绕过。新 binding 已提交后的启动失败用 restart 恢复。

本地状态、会话、去重记录和无正文审计按员工隔离。未知回复结果或进程中断的任务需要核实，不自动重新执行 Agent；后台重启保留订阅重试预算。远端 ack/replay/cursor 尚未提供，不承诺 exactly-once 或断线不丢消息。


## 创建草稿

```bash
dws dingtalk-tag manage create \
  --name "<名称>" --description "<职责>" \
  --main-program-type local_agent \
  --dry-run --format json
```

create 只创建草稿并返回 `agentUuid`。名称和描述必填；部门可不传，由服务端补操作人的主任职部门。`local_agent` 发布不要求平台 Prompt、模型、Skill 或 MCP。创建成功后必须立即保存 `agentUuid`；后续失败只从 detail/save-draft/publish 恢复。

## 修改：按字段更新

```bash
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type draft --format json
dws dingtalk-tag manage save-draft --agent-uuid <agentUuid> --dry-run --format json
```

`save-draft` 只更新显式基础字段，未传字段保持原值；Skill/MCP 的创建、更新、删除统一通过 `capability` 子命令管理。头像统一使用 `--avatar-url`；它既可接收公网 HTTP(S)，也可接收本地路径，本地文件由 CLI 复用 Skill 上传封装并回写 OSS URL。

CLI 的 `--main-program-type` 映射到 MCP 顶层字段 `type`；详情响应也以 `type` 返回 `open_code` 或 `local_agent`。详情查询命令自身的 `--type draft|published` 仅选择配置来源。

`open_code` 必须至少配置一个响应模式；`local_agent` 可省略。创建 `open_code` 或从其它类型切换到 `open_code` 时同时传 `--response-mode`。

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
dws dingtalk-tag manage login --agent-uuid <agentUuid> --dry-run --format json
dws dingtalk-tag manage login --agent-uuid <agentUuid> --format json
```

前置条件：published 详情存在。该命令只获取一次性授权信息、执行受管换票并保存数字员工独立 Profile；它保持主管 Profile 当前激活，不查询 operator、不保存 DSH binding、不调用或重启 DSH。

后续需要接入 DSH 时，单独执行下面的 DSH 模式；它会重新获取一次性授权信息并幂等注册。

## 接入已有数字员工到 DSH

本节只适用于企业把 `local_agent` 数字员工接入本地 Agent/DSH。A2A 或其他需要登录数字员工 DWS 的场景使用 `dws dingtalk-tag manage login --agent-uuid <agentUuid>`，不要使用 `connect`。

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --dry-run --format json
# 用户确认后
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --yes --format json
```

前置条件同上。该模式会保存数字员工独立 Profile、保持主管 Profile 当前激活，并继续解析 operator、保存 DSH binding 和幂等注册 DSH；它不会修改或发布员工，也不会自动重启 DSH。

成功结果在 DWS envelope 的 `data` 中返回身份与运行结果。宿主确认启动时返回实际运行状态和 readiness；仅注册时返回 `restartRequired=true`，不能视为已在线。若 Profile 和绑定已落盘但 DSH 注册失败，使用 `connect restart --agent-uuid <agentUuid>` 幂等补注册并启动，无需换票，也不要重新创建员工。

## 创建并接入的一次请求

1. 一次收集创建和发布所需完整信息。
2. 汇总展示 create/save-draft/publish/connect 四阶段及影响，只确认一次。
3. 创建并记录 `agentUuid`。
4. 补全草稿并发布；任何阶段失败都报告 `agentUuid` 与下一条恢复命令。
5. 发布成功后单独执行 connect；connect 失败不回滚已发布员工。

connect 的未知子命令只提示查看 `connect --help`，不推荐其他生命周期操作。发布详情查询失败时保留实际网络、权限或服务端错误；只有查询成功且发布数据为空时才提示尚未发布。
