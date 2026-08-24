# Live 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、官方 Cobra 树、Live 实现、dingtalk-misc Live Skill 与冻结正式
`internal/cli/param_concepts.json`。未使用固定 Catalog、历史 badcase、用户 Shortcut 或已安装
插件，也没有修改当前工作区正式别名表。

Live 只有 1 个 Agent 可见叶 `live stream list`，调用 `get_my_lives`，业务参数为空，始终查询
当前用户发起的直播列表。Skill、Help、Schema 与实现对这一公开能力一致。参数幻觉风险来自
Agent 把返回字段或其他直播平台经验反向猜成请求参数：分页、状态筛选、直播/主播 ID、时间范围、
创建/开播/结束、观众群与回放控制。

候选因此不新增任何 alias 或 concept，只增加 1 个 `scope_strict` 纯保护 override 和 16 个
guard fixture。生成结果为 0 alias、33 blocked、6 ambiguous；所有保护都不与真实业务、隐藏或
全局 flag 冲突。候选已通过 canonical/global 原生行为、16 组 dispatch 前保护、非目标结构恒等、
`internal/cli`、`internal/pipeline`、generated drift、Schema Catalog 政策与完整
`internal/app`（239.904 秒）。候选可以进入正式 fail-closed 规则落地评审。

## 参数问题

### 1. 无业务参数的列表命令容易被补出分页、搜索和排序参数

真实 leaf 的 `parameters={}`，没有 page、size、limit、cursor、offset、query、keyword、sort 或
order。返回列表并不证明接口支持客户端分页或搜索。把这些名称自动映射到任意全局 flag 会让
Agent 误以为筛选已生效。

候选对所有分页、搜索和排序名做 block，裸 type/time/date ambiguous。没有 canonical 业务目标，
所以不存在符合准入条件的自动 alias。

### 2. 状态、时间、直播 ID 等返回字段不是请求过滤条件

Schema 的选择文案提到“状态或观看量等列表信息”，但这些是返回内容，不是 `--status`、
`--live-id`、`--stream-id`、`--start-time` 等输入。单直播查询、时间范围过滤或按状态筛选在冻结
公开面均不存在。

候选 block status/state、各类 live/stream/room/anchor ID 与时间范围；裸 id/type/time/date
ambiguous。参数字典不能从返回 Schema 推导新输入，也不能自动改用另一个未公开接口。

### 3. “我的直播”身份由当前会话决定，不接受用户或主播选择器

`get_my_lives` 没有参数，身份来自当前登录会话/所选 profile。`--profile` 是真实全局组织/账号
选择，不等于 `--user-id`、`--anchor-id` 或 `--creator-id`。把任意用户 ID 改名为 profile 不仅
值域不同，还会混淆账号切换与业务筛选。

候选保持 `--profile` 原生，block 具体用户/主播 ID，裸 user/owner/id ambiguous。它不查人、
不切账号，也不替用户选择组织。

### 4. 列表查询不能承载创建、控制、观众范围或回放意图

冻结公开面明确不支持创建、开播、结束、群观众配置、录制或回放控制。title、cover-url、
group-id、conversation-id、record、playback、start-live、end-live 等若落到 list，会形成“命令
退出了但动作没有发生”或错把查询当写操作的幻觉。

候选 block 这些写/控制字段，只提供明确失败，不自动跨命令或调用潜在 MCP 能力。要扩展能力
必须先新增真实 Cobra leaf、Contract/Safety、Skill 和测试，再评审参数别名。

## 当前别名表可以实施的方案

1. 为 `live stream list` 增加一个不含 aliases 的 `scope_strict` override。
2. block 分页、搜索、排序、状态、精确 ID、时间范围与创建/控制/回放字段。
3. 对裸 id/type/time/date/user/owner 提示歧义，避免误当返回字段过滤器。
4. 保持 format/fields/jq/profile/timeout/mock/dry-run 等真实全局参数原生。
5. 用 guard fixture 锁定“0 业务参数”事实，未来若新增真实 flag，生成冲突应强制重新评审。

## 当前能力支持不了的事项

- 给无参数接口创造 page/size/cursor 等分页输入；
- 根据返回字段 status、watch count 或时间自动构造服务端过滤；
- 按 live-id/stream-id 查询单个直播；
- 把 user-id/anchor-id 转成 profile 或切换登录身份；
- 查询任意用户的直播，而非当前会话用户；
- 创建、预约、开播、结束或修改直播；
- 设置观众群、封面、录制或回放；
- 从自然语言时间范围换算或过滤返回列表；
- 自动切到未公开的直播管理接口；
- 用别名表声明新的 Runtime/Schema 能力。

## 第一轮改造建议

第一轮只落地纯保护 override，不增加任何自动 alias。它能立即把最常见的“返回字段当输入”和
“把列表当控制命令”在 dispatch 前拦截。若产品后续需要分页、单直播详情或直播控制，应先增加
真实命令和安全声明，再重新分析值域、角色、确认门和参数别名；不能提前在字典中预埋假参数。

## 候选 `param_concepts.json` 改动与审核

- 新增 concept 0、自动 alias 0；
- 新增 1 个 `live stream list` scope_strict override；
- 新增 16 个 fixture，全部为 block/ambiguous，active 为 0；
- `go generate ./internal/cli` 从 569 个命令作用域变为 570 个；
- Live entry 生成 33 blocked、6 ambiguous，fallback 无变化；
- 真实 flag 冲突 0，global/hidden flag 均未被保护规则覆盖；
- 删除 Live 改动后，非目标 concept、override、fixture 与冻结正式表结构恒等；
- 规则没有把返回字段、全局参数、profile 身份或潜在未来接口混为一类；
- 生成规模与 1 个无参数叶相符，没有异常扩散到其他产品或命令。

审核结论：候选是纯 fail-closed 治理，不改变任何成功输入或 payload；若完整应用门禁通过，可
进入正式落地评审。候选位置：`docs/parameter-hallucination/live/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 570 个命令作用域；Live 0 alias、33 blocked、6 ambiguous |
| canonical 原生行为 | 通过 | 无业务参数 dry-run 指向 `get_my_lives`，executed=false |
| 全局参数原生行为 | 通过 | format/fields/dry-run 继续正常，不进入业务别名表 |
| block/ambiguous | 通过 | 16 组代表幻觉全部在 MCP dispatch 前停止 |
| alias/canonical | 不适用 | 审核结论为 0 自动 alias，不制造映射覆盖率 |
| 非目标回归 | 通过 | JSON 结构恒等；generated diff 仅新增 Live entry；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | CLI 83.639 秒；pipeline 0.421 秒 |
| generated drift | 通过 | 参数别名与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 通过 | 239.904 秒；纯 guard fixture 不要求新增 complete-command 模板 |

正式替换不需要新增 complete-command payload 模板，因为候选没有 active alias fixture；完整
应用与仓库政策已全绿，但未来新增任何 Live 真实 flag 时仍须重新审核 guard 冲突。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00；
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`；
- 候选 SHA-256：
  `842d3a7f04073962622ce0ab23c8b5cf22a3f5b771917f9d0765219060d54d1b`；
- 命令实现：`internal/helpers/live.go`；
- Skill：dingtalk-misc `references/live.md`；
- Schema：同一冻结二进制运行时声明组装，`parameters={}`，interface 为 `get_my_lives`；
- 官方树边界：1 个产品、1 个 group、1 个 Agent 可见可执行叶，无 shortcut/hint 兼容叶；
- 行为调用：仅 dry-run/mock；没有发起真实直播查询或任何业务写操作；
- 明确未使用：固定 Catalog、历史 badcase、评测工作簿、用户 Shortcut、已安装插件。

## 可复用分析流程

对“零业务参数”产品先证明空参数是事实而不是 Schema 漏项，再区分真实全局 flag、返回字段、
身份上下文与其他产品能力；只有存在真实 canonical 目标时才考虑 alias，否则用精确 scope_strict
保护锁定能力边界；最后用 canonical dry-run、dispatch 前 guard、完整应用测试和仓库政策确认
纯保护规则不会改变成功路径。
