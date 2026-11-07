# dashboard & chart — 仪表盘与图表

## 按需处理

已有合法配置直接使用；缺少图表结构时仅读取一次 `+chart-templates --chart-types <大写类型>`，缺少 Dashboard 完整配置时才读取 `dashboard config-example`。
已有真实 ID 时直接读取对应详情，不重复探索分组导航。

## 要点

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用
- `dashboard share get` 可能返回 `404`（`retryable:true`）——从未分享、甚至刚开启分享后立即查都可能 404；`data` 为 `{}`。按可重试错误处理，不要误判为参数拼错。它有时也会返回 `success + data`（如关闭分享之后），行为不稳定，别用它当"是否已分享"的唯一判据。
- `chart share get` 稳定返回 `success + data`（含 `enabled` 等），可用于分享状态判断；从未分享时 `enabled=false`，不会 404。
- ⚠️ **`dashboard share update` 开启 ORG 分享后返回的 `shareType` 是 `"[1]"`（未映射回 `ORG`，服务端已知问题）**；`chart share update --share-type ORG` 则正确返回 `shareType="ORG"`。判断 dashboard 是否 ORG 分享时对 `"[1]"` 做兼容。
- `chart share update` / `dashboard share update` 的 `--enabled` 是字符串 flag：`--enabled false`（空格）和 `--enabled=false` 都能正确关闭分享。

## dashboard 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `dashboard get` | 获取仪表盘详情（含 charts 列表） | `--base-id` `--dashboard-id` | — |
| `dashboard create` | 创建仪表盘 | `--base-id` + (`--config` 或 `--name`) | `--name` 简化版创建空看板；`--config` 传完整 JSON |
| `dashboard update` | 更新仪表盘 | `--base-id` `--dashboard-id` + (`--config` 或 `--name`) | `--name` 仅改名；`--config` 更新完整配置 |
| `dashboard arrange` | 自动重排仪表盘图表布局 | `--base-id` `--dashboard-id` | 仅用户明确接受按行重排时使用，改变位置与宽高 |
| `dashboard delete` | 删除仪表盘 | `--base-id` `--dashboard-id` `--yes` | — |
| `dashboard config-example` | 查看仪表盘配置模板 | 无 | 创建前先调此命令了解 config 结构 |
| `dashboard share get` | 获取仪表盘分享配置 | `--base-id` `--dashboard-id` | 可能 404，见上方要点 |
| `dashboard share update` | 更新仪表盘分享配置 | `--base-id` `--dashboard-id` `--enabled` | `--enabled true` 开启，`--share-type` 用 `ORG`（ORG 回显 `shareType="[1]"`）；部分组织禁用了 `PUBLIC` 公开分享；若报 `Illegal argument`，改用 `ORG`（组织内分享）。`--enabled false` 关闭 |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout` |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` `--yes` |
| `chart widgets-example` | 查看图表 widgets 配置模板 | 无 |
| `chart share get` | 获取图表分享配置 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart share update` | 更新图表分享配置（`--enabled true/false`，开启配 `--share-type`，用 `ORG`；部分组织禁用了 `PUBLIC` 公开分享；报 `Illegal argument` 时改用 `ORG`） | `--base-id` `--dashboard-id` `--chart-id` `--enabled` |

> `chart create` 的 `--layout` 是**必填**；它和携带 layout 的 `chart update` 都会在写前自动调用 `get_dashboard` 并强制校验根布局。已确认应用模式时传只读上下文参数 `--is-app-mode=true`，使用 48 列；否则只有 `schemaVersionTypeVerified=true` 时才按原始类型判断，JSON number `2` 为 48 列，其他已验证值为 12 列。证据缺失、布局越界或使用非根容器 `parentId` 时停止且不调用 Chart 写工具；`parentId` 省略、为空或 `root-responsive-layout` 均属于根布局。`schemaVersion` 与 `isAppMode` 不能写入 Dashboard/Chart config。`chart update` 的 `--config` 也**必填**——即便只想改 layout，也要带完整 config，否则服务端拒绝。

## 配置获取流程

已有合法配置直接创建；缺少结构时使用 `+chart-templates` 解析 JSONC 并投影所需类型，按真实 tableId/fieldId 绑定配置。

## 保留设计、批量和接续

- 已有明确布局、大小卡混排或纵向叠放时，按声明的 `layout` 创建；创建后不默认追加 `arrange`。等高铺满只属于明确选择的按行重排策略。
- 同一 Dashboard 的 1～20 张图表使用 `+charts-create --charts <JSON数组>` 集中编排，写前只读确认能力后优先服务端批建，缺能力才串行写；`--server-batch=false` 强制串行。逐图独立读取完整配置及布局，摘要和受理数量不等于 verified。
- `+dashboard-diagnose` 只读检查根几何和已声明 minW/minH；页面标题、数字及图例仍需固定视口验收，不能从无重叠推导可读。
- 新协议部署后，`+dashboard-preview` 返回真实前后布局及 source 摘要；原子 `dashboard arrange --expected-layout-fingerprint <真实摘要>` 在源布局已变化时停止。摘要和提交前检查不等价于原子 CAS。
- 多步构建使用 `+build-plan --plan plan.json --checkpoint task.json`。版本 1 计划固定同一 Base，声明顺序/dependsOn、完整业务参数及逐步回读断言；模板和结构化 `$ref` 由程序展开并绑定真实 ID。单表大结构优先 bootstrap。
- 中断后先 `+build-plan-verify --plan plan.json --checkpoint task.json`；接续写入用 `+build-plan ... --resume`。检查点绑定计划摘要、实际账号组织及 Base，已完成步骤重新核验，不重做；未知创建无可靠 ID 时停止并对账，不能按旧值或名称自动重建。
- AI 状态新入口部署后，使用 `+field-wait-ai --base-id <B> --table-id <T> --tasks <本轮taskId/fieldId/recordIds数组>`。指定记录可按本轮行任务终态结束等待；现有兼容接口不提供写回版本，因此结果可读性仍为未确认，标准输出保持 pending。固定等待、运行数量为 0、旧单元格非空均不能证明本轮完成。

### 设计诊断、等待恢复和离线统计

- `+dashboard-diagnose` 可传 `--expected-layout` 完整根设计及 `--regions` 指定区域；设计差异、区域并集覆盖和几何分别判断，不证明页面可读。子容器不套用根坐标。
- `+field-wait-ai --checkpoint <私有任务文件>` 保存全部原任务和逐行证据；恢复携带相同 `--tasks`、`--checkpoint` 和 `--resume`，只读续查，不重触发 AI。`+build-plan` 的固定记录 AI 步骤也持久保留等待证据；本轮结果未核实可读时保持 pending，阻断后续写入。
- `+task-report --checkpoint <文件>` 离线汇总统计，支持 `--baseline` 及显式客户端模型指标。缺失指标保持未知；不同输入/身份/模型/环境、未核验结果或不完整统计不产生提速结论。
- 新 DWS 在 Chart 布局写前核验后传递明确的应用模式布尔值。旧请求省略时 MCP 兼容只检查 48 列绝对上界，不能把它当作具体 12/48 列网格已验证。

默认 `+charts-create` / `+chart-templates` 在写前通过只读能力发现优先选择服务端批建 / 按类型模板；明确缺能力时使用旧串行 / 全量模板。发现失败先排查，不发送写入；可用 `--server-batch=false` / `--server-filter=false` 强制旧路径。写入开始后任何超时或未知结果都不切换路径重放。
