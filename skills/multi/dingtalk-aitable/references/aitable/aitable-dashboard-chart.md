# dashboard & chart — 专项配置

新建或重构分析仪表盘、需要选图/指标/布局、图表批建恢复决策或发现数值/页面异常时读取；Chart config 和 Dashboard 完整 config/自动布局也按本文处理。普通 Dashboard 按名称创建、读取、改名或删除仍走根 Skill 直达。

只有分析仪表盘的统计步骤可按需读取 [record-stats](aitable-record-stats.md) 或 [psql](aitable-psql.md)；不全局放开其他 Reference。命令参数明确时直接执行，不重复发现。

## 分析仪表盘执行闭环

1. 明确分析目标、Base/Table、字段、筛选范围、单位和目标视口；tableId/fieldId/viewId 使用当前身份下的真实返回值。
2. 用同范围的服务端统计取得预期值：单表直接统计走 record-stats，JOIN/派生计算等复杂分析走 psql；不下载全表在本地或模型上下文重算。图表公式 `average`、统计 `statsType=AVG`、SQL `AVG(...)` 分别按各自接口使用。
3. 按下方规则取得完整配置，选择图表及尺寸，按已确认的根网格和用户设计填写独立的 `layout`，写入前检查纵向压缩对必需对齐的影响。
4. 用已授权的 chart create/update 或 `+charts-create` 写入，保存逐项 index、真实 chartId、状态和回执；按下方恢复规则处理未完成项。
5. 对原 chartId 独立回读 config/layout，并核对统计与实际页面；数值或布局不符时更新原 chartId，默认最多两轮修正与复验。分别交付配置、数值、页面结论；没有证据的项目标为未验证。

## 获取与使用配置

已有符合当前 leaf Schema 的合法 config 且指标/布局语义明确时直接创建/更新，不重复读模板。构造新建或复杂配置时仅读所需类型，已取得的模板复用；需求增加类型时再补取。以下类型仅为示例，实际只请求任务所需类型：

```bash
dws aitable +chart-templates --chart-types SUMMARY,HISTOGRAM,PIE --format json
```

命令成功后读取 `data.templates.TYPE`；有说明时阅读 `data.documentation.common` 和所需 `data.documentation.byType.TYPE`，说明不作为 config 提交。`documentation` 是可选说明：命令成功且模板合法时，即使说明缺失或有说明诊断，也依据本页、模板和真实上下文继续语义明确的配置，不因缺说明停止，不伪造说明或为补说明重复拉取全量模板。必需的配置、字段聚合、筛选或网格信息无法确认时，停止受影响步骤并说明缺口。

- 新建：复制完整 `data.templates.TYPE` 配置，绑定真实 tableId/fieldId/viewId，修改数据源、指标模式、公式和 `config.name`；保留未修改的颜色、图例、标签、坐标轴及其他展示字段，不重建最小 config。
- 更新：先用 chart get 读取当前完整 config，编辑完整副本后提交；保留未修改字段及显式 `false/none/null/自定义值`，不以新建模板覆盖已有配置。只改 layout 也必须带完整 config；提交标准 JSON，layout 不放进 config。
- 卡片标题用 `config.name`，`config.title` 不是标题别名；创建可省略 name，更新省略时保留原名，不提交空白名称。
- Dashboard 完整 config 缺少结构时才调用一次 `dashboard config-example`，不默认读取两种全量模板。

## 指标选择

SUMMARY 展示单个聚合数字，HISTOGRAM 比较分类，PIE 展示分布占比。记录计数用 `measureType=record-count`；其中冗余 measure 不决定聚合方式。改为字段统计时，同时切换为 `measureType=field` 并设置真实字段及公式，不能只替换 sum。

单表字段统计常用公式为 `count/unique-count/sum/average/max/min`，数值聚合绑定支持该计算的真实字段。

下面仅示意单表字段均值的指标部分，不能当作完整 config 提交；平均值使用 `average`，不用 `avg`、`AVG` 或 sum：

```json
{"measureType":"field","measure":[{"value":"<真实数值fieldId>","externalValue":[{"type":"formula","value":"average"}]}]}
```

多表数据源按对应模板配置，不套用单表指标结构；已有合法公式与展示设置按用户意图保留。排序依据真实字段类型和业务顺序，不按标签字面猜测数值顺序。

## 布局与验收

坐标/尺寸是网格单位。普通模式在原始类型已核准时，JSON number `2` 对应 V2 的 48 列，其他已验证值对应 V1 的 12 列；string `"2"` 不等于 number `2`。仅已确认应用模式时使用 48 列上下文；子容器有独立坐标系，不从现有布局的最大 x 推断列数。

| 类型 | V1 默认 / 最小 w×h | V2 默认 / 最小 w×h |
|---|---|---|
| SUMMARY | 4×4 / 2×4 | 16×12 / 8×8 |
| HISTOGRAM、PIE、BAR、LINE | 4×5 / 3×4 | 16×15 / 12×12 |

其他类型参考对应模板与已核实规格，不猜统一最小值。按标题、分类数量、图例和视口调整尺寸；默认宽度不是固定占比，最小尺寸不保证页面可读。允许大小卡混排和纵向叠放，保留用户设计，不默认 arrange。

前端默认纵向压缩可能使卡片上移，声明 y 或配置回读一致不能证明页面保留空白。写入前检查要求顶对齐的卡片上方是否有不同的阻挡；已知压缩会破坏对齐时先调整布局，不能只提示风险后照写。没有浏览器也要先避开可从布局确定的冲突，页面仍单独标为未验收。

例如左上窄 KPI 与两张等高分析图，可采用独立分栏、让两分析图从 `y=0` 开始；KPI 高度按内容选择，不为对齐强制通栏或与分析图等高。这是可选方案，不覆盖用户明确指定的分区、顺序或坐标；要求与压缩机制冲突时，写前说明限制并确认替代方案，不插入空白业务卡片占位。

`+dashboard-diagnose` 是只读诊断：`geometryVerified` 只证明根几何；设计差异用 `--expected-layout` 比较，类型尺寸仅覆盖返回的 `sizeCheckedChartIds`，未列出或未提供覆盖信息的组件不能判为尺寸通过。`sizeIssues` 仅作提示，不阻断写入、不自动拉伸或重排；空 issues 不证明整体合格。`pageReadableVerified=false` 表示页面未验收，不代表页面失败或通过。

页面验收须固定视口、等待数据加载，检查标题、数值、配色、图例、标签与实际位置。浏览器或权限不可用时报告页面未验收，不以诊断/preview 或退出码代替。异常先核对完整 config、真实数据、筛选、指标和加载状态，再与模板对照；保留显式自定义配色，不凭图形外观推断图表类型或故障原因。

## 写入恢复

| 逐项结果 | 后续动作 |
|---|---|
| failed 且明确写前拒绝 | 仅修正失败项，不重发整批 |
| accepted | 对原 chartId 只读回查，不重建 |
| verified | 仅表示配置回读通过，继续数值和页面验收 |
| unknown、超时或缺可靠 ID | 保留回执及已知 ID，只读对账；查询未找到或数量不足不证明未提交，不能据此补建；禁止切换批建/串行路径重放 |

批量结果逐项判断，顶层 success 或受理数量不代表全部完成；另一项失败不能使已提交项被当作未提交。明确未提交的 avg 参数错误可按回执修正为 average；写入结果未知或复验预算耗尽时停止写入并保留证据。

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
| `+chart-templates` | 读取请求类型配置与同级说明 | `--chart-types` |

## 配置获取流程

配置读取、完整副本编辑和缺说明兼容按上方「获取与使用配置」执行。

## 保留设计、批量和接续

- 已有明确布局、大小卡混排或纵向叠放时，按声明的 `layout` 创建；创建后不默认追加 `arrange`。等高铺满只属于明确选择的按行重排策略。
- 同一 Dashboard 的 1～20 张图表使用 `+charts-create --charts <JSON数组>` 集中编排，写前只读确认能力后优先服务端批建，缺能力才串行写；`--server-batch=false` 强制串行。逐图独立读取完整配置及布局，摘要和受理数量不等于 verified。
- `+dashboard-diagnose` 的尺寸覆盖、设计比较与页面验收边界见上方「布局与验收」。
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
