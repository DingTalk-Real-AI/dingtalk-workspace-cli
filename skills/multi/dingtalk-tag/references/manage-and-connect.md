# 数字员工生命周期与 DSH 接入

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

## 接入已有数字员工

本节只适用于企业把 `local_agent` 数字员工接入本地 Agent/DSH。A2A 或其他需要登录数字员工 DWS 的场景使用 `dws dingtalk-tag manage login --agent-uuid <agentUuid>`，不要使用 `connect`。

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --dry-run --format json
# 用户确认后
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --yes --format json
```

前置条件：draft 的 `mainProgramType` 必须是 `local_agent`，且 published 详情存在。connect 会保存数字员工独立 Profile、保持主管 Profile 当前激活，并幂等注册 DSH；它不会修改或发布员工，也不会自动重启 DSH。

成功结果在 DWS envelope 的 `data` 中返回 `status`、`agentUuid`、`dwsProfile`、`operatorOpenDingTalkId`、`protocolVersion` 和 `restartRequired`。若 Profile 已落盘但 DSH 注册失败，重新执行同一 connect 获取新授权码并幂等重试，不要重新创建员工。

## 创建并接入的一次请求

1. 一次收集创建和发布所需完整信息。
2. 汇总展示 create/save-draft/publish/connect 四阶段及影响，只确认一次。
3. 创建并记录 `agentUuid`。
4. 补全草稿并发布；任何阶段失败都报告 `agentUuid` 与下一条恢复命令。
5. 发布成功后单独执行 connect；connect 失败不回滚已发布员工。
