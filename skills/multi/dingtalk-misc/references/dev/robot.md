# 应用机器人

分为两条互斥路径：已有应用用 `robot config/get/enable/disable`；新建独立智能体机器人用异步 `submit/result`。不要为已有应用误走建号。

## 已有应用配置

```bash
dws dev app robot get --unified-app-id <id> --format json
dws dev app robot config --unified-app-id <id> --name <机器人名> --brief <简介> --mode STREAM --dry-run --format json
dws dev app robot enable --unified-app-id <id> --dry-run --format json
dws dev app robot disable --unified-app-id <id> --dry-run --format json
```

`config` 是 upsert，至少传一个实际要改的字段；国际化字段传 JSON。最初请求只允许 dry-run；展示预检返回的应用/机器人、动作、业务参数和影响并取得用户对该预览的明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`。参数变化就重新预检并确认，确认前不得真实配置、启停或提交。写后只回读一次 `robot get`：`UNCONFIGURED` 未配置，`OFFLINE` 已配置未启用，`ONLINE` 已启用。ONLINE 只代表机器人能力开启，不代表版本已发布或本地连接已建立。

按应用名称审计机器人时先 `dev app list --name` 取得唯一 `unifiedAppId`，随后直接 `dev app robot get`；不存在 `dev app search`，也不要切到 `dws chat bot` 猜命令。

## 字段与恢复路径

| 用户要求 | 写入位置 | 回读核对 |
|---|---|---|
| 应用说明 | `app update --desc` | `app get` 的 `desc` |
| 机器人简介 | `app robot config --brief` | `robot get` 的 `brief` |
| 机器人详细描述 | `app robot config --desc` | `robot get` 的 `desc` |

简介与描述不是别名；同时要求时分别传值。只改简介就只传 ID 和 `--brief`，不重传名称、模式或应用说明。明确禁止自动加权限/关闭证书校验时，不擅自开启对应开关；要求核实的字段未返回时标明无法核实，不能用未传参数证明远端状态。

当前平台停用后可能读回 `UNCONFIGURED`，不能承诺保留配置的临时暂停。停用前保存恢复所需配置，在预检确认中说明影响；要求原样恢复但缺少必要配置时先澄清。停用后回读：仍有可启用配置才走 `enable`；`UNCONFIGURED` 时不反复 enable，在用户授权恢复的范围内用已保存配置重新 `config`，重新预检、展示并确认后执行，回读核对字段和状态。恢复配置不等于发布或建立连接。

## 异步建号

```bash
dws dev app robot submit --name <智能体名> --robot-name <机器人名> --desc <描述> --dry-run --format json
dws dev app robot result --task-id <submit返回的taskId> --format json
```

正式 submit 后按返回的 `intervalSeconds` 轮询同一个 taskId，直到 `SUCCESS`、`APPROVAL_REQUIRED`、失败或过期；不要重复创建新任务。只有结果返回明确 `unifiedAppId`，才能继续 get/enable、版本发布和最终删除。没有 ID 时报告建号终态及阻塞，禁止猜应用。

## 完成门禁

- 要求线上可用：配置/启用后继续 [`version.md`](./version.md)，到 `RELEASE` 或明确审核态才算该阶段闭环。
- 要求本地调试：发布状态与 [`connect.md`](./connect.md) 的本地连接状态分别报告；connect 成功不代表发布完成。仅配置 STREAM 模式、不建连接时不要读取 connect 文档或操作连接器。
- 要求最终删除：先保存机器人配置/状态和版本结果，再按 [`app.md`](./app.md) 最后删除。
- 相同业务错误无状态变化时不改名、不重提任务、不用 help/debug/verbose 循环；如实报告 `errorCode/errorMsg`。

已知路径直接执行；仅 flag 确有疑问时查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
