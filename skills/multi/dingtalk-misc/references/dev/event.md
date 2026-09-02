# 应用事件订阅

事件配置走 `dws dev app event`；它不等于个人 IM/OA 实时监听。

## 最短命令路径

```bash
# 按事件名称或事件码搜候选；只有用户要求全部时才翻全量
dws dev app event list --unified-app-id <id> --keyword <关键词> --page-size 50 --format json

dws dev app event subscribe --unified-app-id <id> --event-codes <code1,code2> --dry-run --format json
dws dev app event unsubscribe --unified-app-id <id> --event-codes <code1,code2> --dry-run --format json
```

## 闭环规则

1. 从 `event list` 实际返回的 `data.events[]` 中读取 `eventCode/eventName/subscribed`，名称多匹配时让用户确认；不要按 Schema 的通用列表描述猜成 `items[]`，也不要查 help 或文档猜事件码。
2. 退订前确认目标当前 `subscribed=true`。最初请求只允许 dry-run；展示预检返回的应用 ID、subscribe/unsubscribe 动作、完整事件码及影响并取得用户对该预览的明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`。参数变化就重新预检并确认，确认前不得真实订阅或退订。
3. 每种不同动作（subscribe 与 unsubscribe）各只做一次 dry-run 和至多一次正式执行；一次动作失败不阻止另一项独立动作按相同预算尝试。看正式写结果的 `success/needsPublish/versionRequiredAction`；只有写入明确成功且要求发布，或回读已证明配置改变时，才按 [`version.md`](./version.md) 做版本闭环。
4. 到 `RELEASE` 后只回读目标事件，确认 `subscribed` 与请求一致；进入审批态就如实报告待审批，不重复发布。
5. 全量列表读取 `meta.pagination`：`endpoint_exhausted=false` 时把原样 `next_token` 传给下一次 `--cursor`，到 true 停止；CLI 不支持 `--page/--page-num`。普通关键词查询不要翻无关事件。最终给匹配数和相关事件，不倾倒整个列表。

正式写入若返回 `66117` 或泛化 `BUSINESS_ERROR`，做一次目标事件回读；`subscribed` 未变化就原样报告并停止该动作，不重复正式写入。此类错误本身不证明连接器异常：不得因此读取 connect/security/permission/recipes/openapi、重启连接器、搜索文档或创建/发布版本。只有服务端文本明确指出“长链接未在线”时，才按 [`connect.md`](./connect.md) 查询一次；实例存在且 down 才重启一次，并以一次新状态为证决定是否重试。没有记录则说明需先建立连接，不扫描本机进程或目录。

已知路径直接执行；仅 flag 确有疑问时查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
