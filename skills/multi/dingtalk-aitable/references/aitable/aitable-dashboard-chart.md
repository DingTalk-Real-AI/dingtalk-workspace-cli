# dashboard & chart — 专项配置

仅在 Chart 创建/更新缺少 config，或 Dashboard 明确需要完整 config/自动布局时读取。普通 Dashboard 按名称创建、读取、改名或删除走根 Skill 直达，不读取本文。

本文是 Dashboard/Chart 专项配置的终点；命令和参数已覆盖时直接执行，不再读取其他 AITable Reference 或 Help。

## 专项配置最短路径

```bash
# Dashboard 完整 config 缺少结构时，只读取 Dashboard 模板
dws aitable dashboard config-example --format json

# Chart config 缺少结构时，只读取 Chart 模板
dws aitable +chart-widgets-example --format json

# 已有真实 ID 时按需读取详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json
```

## 要点

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用
- `chart create` 以及携带 `--layout` 的 `chart update` 会在写入前自动调用 `get_dashboard` 并强制校验根网格。只有 `schemaVersionTypeVerified=true` 时才使用原始类型判据：JSON number `2` 为 48 列，其他已验证值为 12 列；证据缺失时命令停止且不调用 Chart 写工具
- 任务上下文已确认应用模式时，Chart 写命令显式传 `--is-app-mode=true`，DWS 按只读上下文强制使用 48 列；该上下文作为顶层 isAppMode 传给 MCP 作写前校验，不写入 config。未确认应用模式时省略，不能为了绕过 schemaVersion 证据传 true
- 全局 12/48 列只适用于根布局。存在非根 `parentId` 时使用容器自己的坐标系；拿不到容器真实列数时，创建操作停止，更新操作省略 layout 以保留原值
- `schemaVersion` 与 `isAppMode` 都是只读字段，不能写入 Dashboard config
- 删除 dashboard 会级联删除其全部 chart；确认前必须说明该影响
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断
- Dashboard 普通 CRUD 不在本文重新维护；需要完整配置时才读取 `dashboard config-example`

## Dashboard 专项命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `dashboard config-example` | 查看完整仪表盘配置模板 | 无 | 仅在完整 config 结构缺失时调用一次 |
| `dashboard arrange` | 自动重排图表布局 | `--base-id` `--dashboard-id` | 仅用户明确接受统一按行重排时使用；改变位置、宽高和叠放关系；返回 `{totalColumns, layout, alignedChartCount}` |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout`；根布局按 Dashboard 元信息选择 12/48 列 |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` | 不可逆；由 Runtime 请求确认，Reference 不携带确认绕过参数 |
| `+chart-widgets-example` | 查看所有图表类型的 widgets 模板 | 无 |

## 配置获取流程

已有符合当前 leaf Schema 的合法 config 时直接创建/更新，不读取模板。只有 Chart config 缺少结构时调用一次 `+chart-widgets-example`；旧入口返回全量 JSONC；优先用 `+chart-templates --chart-types LINE,PIE` 正确解析并只投影目标类型。MCP 新筛选入口部署确认后才追加 `--server-filter`。Dashboard 完整 config 缺少结构时只调用一次 `dashboard config-example`，不要同时读取两个模板。

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
