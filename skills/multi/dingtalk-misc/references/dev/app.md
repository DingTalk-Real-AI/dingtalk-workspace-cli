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

## 状态与结果

- `app list` 用于定位，不能用其空的 `appStatus` 判断状态；状态以 `app get` 为准。
- `app get` 若带 `appSecret`，只内部使用并脱敏，不写入回答。
- `disable/enable` 先看返回的 `disabled/enabled`，需要最终状态时再看 `app get.appStatus`。
- 创建结果可能没有版本状态，更新结果可能带 `versionStatus`；两者都不等于已上线。需要上线时继续走 [`version.md`](./version.md)，不要仅凭某次写结果推断已发布。
- 多应用命中时列出候选并停止写操作；`ServiceResult.success=false` 原样报告 `errorCode/errorMsg`。

## 多步骤顺序

若一句话同时要求“创建、配置/发布、最后删除”，依赖顺序固定为：创建 → 配置 → 版本生效或明确阻塞 → 收集用户要求的状态/详情 → 删除。中文位置不改变依赖关系，不要因“办完后删除”出现在中间就提前删除或停下来反问。

已知上述路径时直接执行；仅 flag 确有疑问时查一次该 leaf compact Schema，Schema 不可用才查一次精确 leaf help。
