# Dev 跨域任务闭环

创建或清理应用，同时配置机器人、网页、成员等，也属于跨域。本页覆盖常用路径；读完即可执行，不再为相同命令读取 app/robot/webapp 等专题。仅缺少当前步骤必要参数或遇到未覆盖状态时补读对应段落。各应用分别保存名称、创建 ID 和回执。

## 通用顺序

```text
定位/创建应用
  → 读取用户要求的初始状态或名单
  → 配置成员/权限/安全/网页/机器人/事件
  → 仅当用户明确要求或成功写结果要求发布时创建版本
  → 检查审批并发布（若需要）
  → 回读版本与用户要求的各项结果
  → 删除/停用等清理（若用户要求）
```

句子里“办完后删除”即使出现在配置或发布之前，也按依赖关系把删除移到最后；这不是冲突，不要因此停止反问。只有两个最终状态确实无法同时成立且不能用“操作前快照 + 最终清理”满足时才澄清。

## 常用原子路径（首轮直接执行）

| 域 | 已知 leaf 与关键定位参数 |
|---|---|
| 应用 | `app create --name <name> --desc <用途说明>`；定位 `app list --name <name>`；详情 `app get --unified-app-id <id>`；改名 `app update --unified-app-id <id> --name <新名>`；删除 `app delete --unified-app-id <id> --confirm-name <当前名>` |
| 成员 | `app member list --unified-app-id <id>`；`app member add/remove --unified-app-id <id> --user-ids <真实ID,真实ID> --member-type DEVELOPER`（开发成员） |
| 权限 | `app permission list/add/remove --unified-app-id <id>` |
| 网页 | `app webapp get --unified-app-id <id>`；`app webapp config --unified-app-id <id>` 加用户指定的 `--homepage-url` / `--pc-homepage-url` / `--omp-url`，不猜 `--h5-page-type` |
| 机器人 | `app robot get --unified-app-id <id>`；`app robot config --unified-app-id <id> --name <名称> --brief <简介> --mode STREAM`（只传本次要求字段，明确 STREAM 时不可省略 mode）；启停 `app robot enable/disable --unified-app-id <id>` |
| 事件 | `app event list --unified-app-id <id> --keyword <词> --page-size 50`；从 `data.events[]` 取真实 eventCode；`app event subscribe/unsubscribe --unified-app-id <id> --event-codes <code1,code2>` |
| 版本 | `app version create --unified-app-id <id> --desc <说明>`；`app version check-approval/publish/status --unified-app-id <id> --version-id <返回的versionId>`；普通无需审批且 publishable 才 publish，需选审批人时读取 version.md 对应规则，不猜人选 |
| 安全/凭证 | 安全配置按需读 security.md（无 get）；凭证按需读 credentials.md，不在回答展示密钥 |
| 本机连接器 | `connect list/status/restart`；list/status 另带 `--json`，不扫描系统进程 |

所有命令补 `dws dev` 前缀和 `--format json`；写操作先 `--dry-run`。最初请求不等于预检后的确认，只有展示准确对象、动作、业务参数和影响并取得用户对该预览的明确确认后，才可把同一命令仅由 `--dry-run` 换成 `--yes`。已知路径不要用 help 探路。

## 参数直达与结果复用

- 基础创建 `app create --name <名称> --desc <用途说明>`。说明优先用用户原文；未指定时只依据已表达用途拟定简短说明，随预检展示并确认，不杜撰业务事实。当前环境曾出现省略说明时服务端 `67010`，不能因 CLI 标注可选就推荐默认省略；这不是该错误码的通用释义。用户明确要求不填时仍省略，不擅自填占位值，失败如实报告。最小参数原则主要用于更新：只改名传 `app update --unified-app-id <id> --name <新名>`，不重传未修改字段。
- 删除使用 `app delete --unified-app-id <本任务返回ID> --confirm-name <当前名称>`，先预检并确认。漏参数属于本地可修正错误，补齐后重新预检确认，不把它当无状态变化的业务错误直接放弃。
- 每个新建应用分别保存名称→ID→创建回执；创建失败没有可供后续写入的目标。禁止使用其它应用或历史同名应用“演示”剩余步骤；任务要求新建就不能用 list 命中替代创建。多个应用逐个完成各自确认，不能用一项确认创建另一项。
- 成员增删用 `--user-ids <真实ID,真实ID> --member-type DEVELOPER`（仅当用户要求开发成员）；同角色同动作可一次传齐，写后核对实际名单，不以 userId 猜身份。
- 网页配置按需选 `--homepage-url`、`--pc-homepage-url`、`--omp-url`；不猜 `--h5-page-type`。
- 机器人简介是 `robot config --brief`，详细描述是 `--desc`，应用说明才是 `app update --desc`。只传要修改的字段，回读相同对象和字段。停用可能变成 `UNCONFIGURED`，涉及停启恢复时按需读一次 [robot.md](./robot.md)，先保存配置并说明影响，不盲目 enable。
- 只查开发资料按 [devdoc.md](../devdoc.md) 的相关性与搜索预算；不预读其它应用专题。这里未涵盖的必要参数按需读一个对应专题，不为满足阅读次数限制猜参数。

## 常见闭环

- **网页应用**：`app create/list` → `webapp config` → 写结果要求发布时才 `version create` → `check-approval` → `publish` → `status` → `webapp get`。
- **权限治理**：按关键词 `permission list` → `add/remove` → 成功结果要求发布时才走版本闭环 → 回读目标权限。未指定“多余权限”时不拉全量猜目标，只暂停 remove，其它安全步骤继续完成。
- **机器人**：`app create/list` → `robot config/enable` → 版本闭环；用户另要本地调试时再 `connect`。建联和线上发布是两项独立状态。
- **事件订阅**：按关键词一次 `event list` → subscribe/unsubscribe 各至多一次正式执行 → 只有写成功且要求发布才走版本闭环 → 一次目标回读。
- **安全 + 网页**：先 `app get` 保存应用初始状态，并用 `webapp get` 保存网页配置。安全配置没有读取命令；需要保留旧安全项但没有上游可信的完整旧值时，说明整组覆盖风险并暂停 `security config`，继续不依赖它的网页步骤。用户提供完整目标列表或明确接受覆盖后，才执行 `security config` → `webapp config` → 一次版本闭环 → 回读可读取的应用/网页/版本结果。
- **成员临时变更**：解析唯一 userId → add → `member list` 保存名单 → remove → 回读；若还要删除应用，删除最后做。

明确不发布/不建连接时，任何通用闭环都不能覆盖此限制：保存配置结果并说明未发布；服务端要求发布时报告阻塞，不擅自继续版本链。“维护完成后恢复”等外部条件尚未满足时等待，不把写操作确认当成条件满足。

## 轮次与失败预算

- 每轮推进到下一个真正的依赖停点：收到某条预检的明确确认后，参数未变就直接执行对应正式命令，不重复预检、不再次询问“是否继续”。读取必要结果后，可在本轮继续下一步骤的 dry-run，再展示该次对象、参数、影响并询问确认；下一项正式写入仍须单独确认。外部维护/审批条件不能由确认代替。
- 用户要求的状态快照、名单或配置回读也是验证证据，同一状态不额外再查一次。创建回执已给出 ID 时直接复用，不再 list/get 来寻找 ID。不要用 shell 管道裁剪命令回执；每次工具调用直接执行命令，原始回执留在轨迹。
- 确认摘要用“动作、对象名称/ID、完整业务参数、影响、是否确认”即可；不反复附上完整计划、已完成历史、原始 JSON 或整段 reference。最后一次统一汇总所有交付项。

- 每个写步骤：一次 dry-run，展示预检后等待明确确认；确认前不发出非 dry-run 写调用。正式命令的目标和全部业务参数必须与用户确认的预检完全一致，只允许把 `--dry-run` 换成 `--yes`；任何变化都重新 dry-run、展示并确认。一次必要回读即可。
- 相同业务错误不做无状态变化的重试；只在新的查询结果实际改变参数/状态后重试一次。发布错误按 [`version.md`](./version.md) 止损。
- 某个破坏性选择缺失时，暂停该动作但继续所有不依赖它的步骤；最终分开写“已完成 / 等待选择 / 失败阻塞”，不要只留下一个反问。
- 大列表只保留总数、用户相关项和分页完整性；空列表写“暂无”。最终逐项核对交付清单，避免结果被截断。
- 清理对象只能使用本任务创建并返回的 ID；删除失败不得声称已清理。
- 续轮只保留交付项、真实 ID、已完成回执、已确认预检和下一步依赖的简短状态；不用重新加载已读 reference、重新定位已知 ID 或重复未变的有效预检。不能把一个操作的确认复用到其它动作或目标。
- 预检摘要保留准确对象、完整业务参数与影响，不重复整段 JSON；不得用管道丢弃预检输出后仅打印“已准备”。相互独立的只读查询可并行，依赖 ID 的写操作和确认链仍按顺序执行。
- 中断先核实上一写操作是否生效，不能重新创建已成功对象。区分已完成、待确认/外部条件、失败、结果未知及待清理；只有原始交付项均有证据才总结全部完成。清理仍须满足原请求、依赖关系及预检后的确认，不能因失败自行删对象。
