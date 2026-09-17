# 应用基础操作

管理应用容器本体：列表、详情、创建、更新、启停和删除。应用状态 `appStatus` 与版本状态 `versionStatus` 是两套状态，不要混用。

## 最短命令路径

```bash
# 按名称定位；唯一命中后保存 unifiedAppId，后续全部复用
dws dev app list --name <应用名> --format json
dws dev app get --unified-app-id <id> --format json

# 已知 AppKey/clientId 时可直接查详情并取得 unifiedAppId
dws dev app get --app-key <appKey> --format json

# 写操作：同一条命令先预检，再正式执行
dws dev app create --name <名称> --desc <描述> --dry-run --format json
dws dev app update --unified-app-id <id> --desc <描述> --dry-run --format json
dws dev app disable --unified-app-id <id> --dry-run --format json
dws dev app enable --unified-app-id <id> --dry-run --format json
dws dev app delete --unified-app-id <id> --confirm-name <应用名> --dry-run --format json
```

最初请求只允许执行 dry-run，不能代替预检后的确认。展示 dry-run 返回的准确应用名称/ID、动作、业务参数和影响，等用户对该预览明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`；定位符或业务参数变化就重新预检并确认，确认前不得发出真实写调用。创建后用返回的 `unifiedAppId`；更新/启停后回读一次 `app get`。删除前还必须核对名称与 ID，删除放在全部依赖步骤之后；成功只按真实返回说明，未回读到消失时不要声称已删除。

应用只有 `list/get/create/update/disable/enable/delete` 及下属能力组；按名称查找使用 `app list --name`，不存在 `dev app search`。列表续页只传返回的 `meta.pagination.next_token` 到 `--cursor`；`endpoint_exhausted=true` 才是到底，禁止使用 `--page/--page-num`。

创建优先携带 `--desc`：用户指定则原样使用，未指定则仅按已表达用途拟定简短说明，预检展示并确认。当前环境有省略说明后返回 `67010` 的实测记录，不能据 CLI 可选标记默认省略，也不能据此断言错误码的通用含义。用户明确要求不填说明/图标时仍省略，不用空格、短横线或猜测值替代。

只改名称时用 `app update --unified-app-id <id> --name <新名称> --dry-run --format json`，不附带保持不变的 `desc`。应用说明与机器人简介不是同一字段，后者用 `robot config --brief` 并由 `robot get` 验证。

新建任务只能使用本次创建成功返回的 ID；失败后不得借用列表中的其它应用或历史同名应用。多应用分别绑定名称、ID 和回执，删除时传各自当前名称到 `--confirm-name`。缺少删除参数时修正并重新预检确认，不遗漏清理，也不越过确认。

dry-run 成功只证明预览成功，不保证服务端接受。业务错误（如 `67010`）含义不明时保留错误码与 Trace ID，不猜定权限或重名原因。按预检实际参数判断请求是否改变：空字符串被省略后与未传字段可能是同一请求，不据此重复正式创建；超时或中断先只读核实是否已创建，结果不明不重复创建。

## 状态与结果

- `app list` 用于定位，不能用其空的 `appStatus` 判断状态；状态以 `app get` 为准。
- `app get` 若带 `appSecret`，只内部使用并脱敏，不写入回答。
- `disable/enable` 先看返回的 `disabled/enabled`，需要最终状态时再看 `app get.appStatus`。
- 创建结果可能没有版本状态，更新结果可能带 `versionStatus`；两者都不等于已上线。需要上线时继续走 [`version.md`](./version.md)，不要仅凭某次写结果推断已发布。
- 多应用命中时列出候选并停止写操作；`ServiceResult.success=false` 原样报告 `errorCode/errorMsg`。

## 多步骤顺序

若一句话同时要求“创建、配置/发布、最后删除”，依赖顺序固定为：创建 → 配置 → 版本生效或明确阻塞 → 收集用户要求的状态/详情 → 删除。中文位置不改变依赖关系，不要因“办完后删除”出现在中间就提前删除或停下来反问。

已知上述路径时直接执行；仅 flag 确有疑问时查一次该 leaf compact Schema，Schema 不可用才查一次精确 leaf help。

确认后的下一轮直接执行已确认且未变化的命令；不要重读本页或重复预检。正式写入和必要回读后，同轮可继续下一项 dry-run，再单独等待其确认。已有创建 ID 不为定位再 list；一份状态回读可同时满足用户的状态查询和写后验证。每轮只简述本次结果及下一次准确预检，不重复整个计划。
