# 应用基础操作

> 概念锚点：操作的是「应用」容器本体（见 SKILL.md 概念地图）；启停/删除改的是应用 appStatus，不是版本 versionStatus。

应用列表查询、详情、创建、修改、生命周期启停和删除。参数查 `dws schema dev.app.<method>`（list / get / create / update / delete / disable / enable）。

## 应用定位

所有单应用命令统一只用 `--unified-app-id`（全树主键）定位。`--app-key`/`--name` 只在 `dev app list` 里作列表过滤，不能定位单应用；旧的 `--agent-id`/`--app-id`/`--custom-key` 已移除。拿到 appKey/agentId 时，先用 `dev app list` 查出 unifiedAppId 再操作。

## 应用状态 appStatus

列表/详情统一用 `appStatus`，按生命周期判断，不要和版本 `versionStatus` 混。

| appStatus | 枚举 | 含义 | 下一步 |
|-----------|------|------|--------|
| `0` | `IN_ACTIVE` | 已停用，应用不可用 | 需恢复走 `enable` |
| `1` | `ACTIVE` | 已激活，应用可用 | 可继续配权限/网页/机器人/版本 |
| `2` | `WAIT_ACTIVE` | 待激活 | 先回读 `get/list` 确认，别按已生效处理 |
| `3` | `EXPIRED` | 已过期 | 停止写操作，提示到开发者后台或管理员处理 |

`create/update` 返回的 `versionStatus` 是版本状态（见 version.md），不等同应用启停状态。

## 要点

- `get` 主要用于定位核验；若偶尔返回 `clientSecret/appSecret`，脱敏处理，不复制到回答；主动读凭证走 `credentials get`。
- `disable/enable` 后必须回读 `get`/`list`：`appStatus=0` 才算停用完成，`appStatus=1` 才算启用完成；接口只返回成功未带状态时，以回读为准。
- `delete` 前必须展示应用摘要；删除是异步，成功后延迟从列表消失。

## 错误处理

| 情况 | 处理 |
|------|------|
| 多应用命中 | 展示候选，停止写操作 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
