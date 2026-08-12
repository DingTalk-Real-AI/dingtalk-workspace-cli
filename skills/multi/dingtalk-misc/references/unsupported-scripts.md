# 未产品化 / 半悬空脚本清单

以下脚本位于 `dingtalk-misc/scripts/`，**没有**对应的稳定产品 reference 与路由行。
它们不是当前 Agent 默认能力面的一部分。

## 使用规则

1. **默认不要调用**这些脚本完成用户任务；优先公开 `dws` 命令 / Shortcut。
2. 仅当用户**明确点名**某脚本文件名，或明确要求「跑仓库里的 yida/finance/aiapp 辅助脚本」时才考虑。
3. 调用前用 `--help` / 脚本头注释确认参数；写操作仍遵守 [confirmation.md](../../dingtalk-shared/references/confirmation.md)。
4. 若脚本依赖的 `dws <product>` 子命令在当前二进制不存在，向用户说明能力未暴露，不要改用 HTTP/curl 绕过。

## AI 应用（aiapp）

| 脚本 | 说明 |
|---|---|
| `aiapp_create_and_poll.py` | 创建 AI 应用并轮询；**无** `references/aiapp.md`，mono 产品表历史死链已移除 |

## 宜搭（yida）

| 脚本 | 说明 |
|---|---|
| `yida_form_builder.py` | 表单 schema 构造 |
| `yida_form_fields.py` | 表单字段构造 |
| `yida_form_inspector.py` | 表单检查 |
| `yida_form_update.py` | 表单 schema 更新编排 |
| `yida_custom_page_update.py` | 自定义页 schema 更新编排 |
| `yida_jsx_pipeline.py` | JSX transform / lint 流水线 |
| `yida_page_compiler.py` | 自定义页编译 |
| `yida_page_generate.py` | 自定义页生成 |
| `yida_page_schema.py` | 页面 schema 处理 |
| `yida_page_self_check.py` | 页面自检 |
| `yida_process_flow.py` | 流程辅助 |
| `yida_process_update.py` | 流程保存/发布编排 |
| `yida_report_builder.py` | 报表 schema 构造 |
| `yida_report_charts.py` | 报表图表组件 |
| `yida_report_update.py` | 报表 schema 更新编排 |
| `yida_schema_common.py` | schema 公共工具 |

`routing.md` 可将「宜搭」粗分到 `dingtalk-misc`，但**产品索引表无宜搭正式产品行**；在补齐正式 reference 前，宜搭请求应向用户说明「仅有未产品化脚本，无稳定命令面」。

## 财务辅助（finance）

| 脚本 | 说明 |
|---|---|
| `finance_daily_cashflow.py` | 日现金流辅助 |
| `finance_expense_flow.py` | 费用流辅助 |

无独立 finance CLI 产品面时，不要把这些脚本宣传为正式 CLI 能力。
