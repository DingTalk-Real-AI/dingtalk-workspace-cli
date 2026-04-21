---
name: wukong-doc-business-review-report
description: 当用户需要季度汇报、年度汇报、KPI复盘、经营复盘或管理层业务汇报，并期望输出为钉钉文档时使用。Distinct from wukong-doc-weekly-work-report（后者侧重个人或团队周报）。Do NOT use for 日常进度更新、个人周报、主题资讯汇总。
---

# wukong-doc-business-review-report

用于生成季度、半年度或年度经营汇报。主文件只保留触发、工具选择和最短工作流；完整模板、示例和检查项见 `references/`。

## 严格禁止 (NEVER DO)

- 不要把对话文本当最终交付，最终产物必须是钉钉云文档。
- 不要在缺少业务口径、指标口径或时间范围时直接生成成品。
- 不要编造 KPI、目标值、同比环比或会议结论。
- 不要混用不同统计口径或不同截止日期的数据。
- 不要只有问题没有对策，或只有对策没有负责人和时间点。
- 不要跳过来源标注，所有关键数字都要能追溯。

## Tool 总览

| 工具 | 用途 | 安全等级 | 必填参数 | 示例 |
|---|---|---|---|---|
| `dws aitable base search` | 查找 KPI 看板 | `L1` | `query` | 搜索“经营分析” |
| `dws aitable record query` | 拉取指标数据 | `L1` | `base-id`, `table-id` | 查询 GMV/DAU |
| `dws doc search/read` | 获取项目文档和历史汇报 | `L1` | `keyword` 或 `node` | 搜索“Q1 业务复盘” |
| `dws calendar event list` | 获取关键里程碑会议 | `L1` | `start`, `end` | 拉取季度会议 |
| `dws minutes` | 提取会议摘要和行动项 | `L1` | `id` | 获取复盘纪要 |
| `dws doc create/update` | 生成最终钉钉文档 | `L2` | `name` 或 `node` | 创建汇报文档 |

## 意图判断决策树

用户提到“季度汇报/年度汇报/经营复盘/KPI复盘”：
- 有明确业务线和时间范围 → 进入汇报生成
- 缺时间范围 → 先补季度或年度窗口
- 缺指标口径 → 先补核心 KPI 列表

用户提到“给管理层汇报/给 CEO 看”：
- 默认输出结论先行版本，摘要放最前

用户提到“复盘问题/找原因/下阶段怎么做”：
- 在结果后追加问题归因和对策规划

## 易混淆场景

| 如果用户要… | 应使用 |
|---|---|
| 个人或团队一周工作总结 | `wukong-doc-weekly-work-report` |
| 某主题在时间窗内的动态汇总 | `wukong-doc-topic-trend-report` |
| 行业、赛道或竞争格局研究 | `wukong-doc-industry-research` |

## 核心工作流

1. 确认最小输入：
   - 业务口径
   - 时间范围
   - 核心 KPI
2. 收集事实：
   - AI 表格取指标
   - 文档取项目进展
   - 日历和纪要取关键事件
3. 形成结构：
   - 摘要
   - 指标达成
   - 业务进展
   - 问题与风险
   - 对策与下阶段计划
4. 交付钉钉文档：

```text
-> dws aitable base search --query "国际电商业务 KPI 看板"
<- 返回 baseId

-> dws aitable record query --base-id BASE_ID --table-id TABLE_ID --query "GMV"
<- 返回目标值、实际值、同比环比

-> dws doc search --query "国际电商业务 Q1 项目进展"
<- 返回项目文档列表

-> dws doc create --name "国际电商业务 2025 Q1 经营汇报"
<- 返回文档节点
```

## 上下文传递规则

| 操作 | 从返回中提取 | 用于 |
|---|---|---|
| KPI 查询 | `指标名`, `目标值`, `实际值`, `截止日期` | 指标达成分析 |
| 文档读取 | `里程碑`, `项目状态`, `风险点` | 业务进展章节 |
| 会议纪要 | `决策`, `行动项`, `负责人` | 问题与对策章节 |
| 文档创建 | `node` 或 `url` | 最终文档更新 |

## MCP 调用约束

- 单次调用优先只做一个动作，避免多任务混杂。
- 参数尽量不超过 5 个，嵌套不超过 1 层。
- 先读后写，先确认文档节点再更新内容。

## 错误处理

- 找不到指标表时，先缩小业务关键词，再搜索看板或月报。
- 数据来源冲突时，只保留口径明确的一组，并说明冲突。
- 缺项目进展时，可先交付“指标版复盘”，但必须明确缺失项。
- 用户只给“做个复盘”时，先补三项：业务线、时间范围、核心指标。

## 质量标准

- 关键数据必须包含来源和截止日期。
- 问题、根因、对策必须一一对应。
- 对策必须有负责人和时间点。
- 最终交付必须是钉钉云文档。

## Checklist

- 是否明确业务口径、时间范围、核心 KPI
- 是否全部关键数据可追溯
- 是否时间口径一致
- 是否问题与对策成对出现
- 是否已创建并更新钉钉文档

## 按需查阅

- 汇报模板：`references/report-template.md`
- 数据采集：`references/data-collection.md`
- 问题归因：`references/root-cause-analysis.md`
- 成稿示例：`references/sample-output.md`
- 交付检查：`references/checklist.md`
