# Dev 能力索引（只在意图不明时读取）

意图清楚时不要读本索引，直接从根 Skill 进入唯一专题 Reference。默认走 `dws dev` 原子命令，不先读 `devapp.md`，也不调用 help 发现 shortcut。

| 用户意图 | 标准命令路径 | 只读此文件 |
|---|---|---|
| 应用创建、列表、详情、状态、更新、停启、能否删除 | `dws dev app ...` | [`app.md`](./app.md) |
| AppKey、AppSecret、clientId、agentId | `dws dev app credentials get` | [`credentials.md`](./credentials.md) |
| 开放平台应用的管理成员、成员角色与角色汇总 | `dws dev app member ...` | [`member.md`](./member.md) |
| 应用权限 | `dws dev app permission ...` | [`permission.md`](./permission.md) |
| IP 白名单、回调或 SSO 地址 | `dws dev app security config` | [`security.md`](./security.md) |
| H5、PC 主页与 OMP 地址 | `dws dev app webapp ...` | [`webapp.md`](./webapp.md) |
| 机器人配置、启停、提交审核 | `dws dev app robot ...` | [`robot.md`](./robot.md) |
| 版本历史、待发布版本、创建、审批、发布与状态 | `dws dev app version ...` | [`version.md`](./version.md) |
| 应用事件订阅 | `dws dev app event ...` | [`event.md`](./event.md) |
| 本机机器人连接器、Stream/长连接实例与健康状态 | `dws dev connect ...` | [`connect.md`](./connect.md) |
| MCP 服务、工具、鉴权、凭证与协作者 | `dws dev mcp ...` | [`mcp.md`](./mcp.md) |
| 同一任务明确跨两个以上 Dev 域 | 按依赖顺序组合原子命令；先不预读专题 | [`recipes.md`](./recipes.md) |

## 共同执行约束

1. 先列交付项，再按依赖顺序做；删除、停用等清理最后执行。
2. 应用名先用一次 `app list --name` 定位，再始终复用同一个 `unifiedAppId`，不要反复查找。
3. 应用定位没有 `dev app search`；连接器没有 `dev app connect`，只能走顶层 `dev connect`。已知命令直接调用；flag 不确定时只查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
4. 写操作的 dry-run、正式执行、回读按专题 Reference 完成；相同业务错误无状态变化时停止，不用 debug/verbose/help 循环重试。
5. Dev 的应用、权限、事件、版本列表使用 `--cursor`，没有 `--page/--page-num`。以 `meta.pagination.endpoint_exhausted=true` 为翻完；为 false 时只把原样 `next_token` 传给下一次 `--cursor`。
6. 最终逐项覆盖用户要求；列表很大时给总数、相关项和分页完整性，空结果明确说明。
