---
name: wukong-doc-business-plan
label: 商业计划书
description: >
  商业计划书生成：强调市场机会、产品与商业模式、竞争、团队、财务与风险，形成可评审BP。
  Use when user mentions "商业计划书", "BP", "融资计划", "创业方案", "帮我写一份BP",
  or asks to "写商业计划", "生成融资方案", "做一份创业计划".
  Do NOT use for general business questions without document generation,
  or simple financial calculations without full BP context.
  Distinct from wukong-doc-company-research (company research only, no BP structure).
  Requires: dws (doc).
---

# SKILL: wukong-doc-business-plan

商业计划书生成：覆盖从市场规模验证（TAM/SAM/SOM）、竞品对比矩阵、商业模式画布到财务预测与融资需求的**全链路**分析，输出结构化的云文档商业计划书。

---

## 1) 输出形式与约束

- **本 skill 被触发即代表用户期望生成云文档**，主 Agent 无需询问，也不得以对话文本替代文档。
- 数据收集完成后，必须通过 `task`（subagent）委托 Writer Agent 创建云文档。
- 任何数据/结论必须来自可追溯来源；不确定则标注"需补充资料"。
- 不可编造事实、链接或来源；财务数据必须标注来源、年份和口径。
- 并列项 ≥ 3 优先用列表/表格；财务数据必须用表格。

### 数据质量红线（MUST）

数据一致性、数据来源、团队章节的完整红线规则见 [references/constraints.md](references/constraints.md)。任一红线不通过则 BP 不可交付。

---

## 2) 意图映射

| 用户说 | 线索 | 执行动作 |
|--------|------|---------|
| "帮我写一份 XX 的商业计划书" | 含项目名 + BP 关键词 | 完整 BP 生成流程（Module A → B → Writer Agent） |
| "帮我做一份 BP，重点分析竞品" | 强调竞品 | 完整流程，A2 竞争格局调研加深，B2 竞品矩阵细化 |
| "融资计划书" / "创业方案" | BP 同义词 | 同完整 BP 流程 |
| "帮我分析一下 XX 公司" | 仅公司调研，无 BP 结构 | **不触发本技能** → 应路由到 `wukong-doc-company-research` |
| "帮我算一下财务模型" | 仅财务计算，无完整 BP | **不触发本技能** → 通用对话或计算工具 |
| "帮我写个项目介绍" | 仅产品介绍，非投资视角 | **不触发本技能** → 通用文档生成 |

---

## 3) 能力清单

| 动作 | 命令 | 必填参数 | 安全等级 | 说明 |
|------|------|----------|---------|------|
| 搜索已有文档 | `dws doc search --query <关键词> --format json` | query | 只读 | 检索内部已有的产品资料、分析文档 |
| 读取文档内容 | `dws doc read --node <DOC_URL> --format json` | node | 只读 | `--node` 必须传完整文档 URL（如 `https://alidocs.dingtalk.com/i/nodes/xxx`） |
| 创建草稿文档 | `dws doc create --name "BP草稿" --format json` | name | 写入 | 返回的 `docUrl` 即为后续 `--node` 参数所需的完整 URL |
| 追加调研素材 | `dws doc update --node <DOC_URL> --markdown <内容> --mode append --format json` | node, markdown | 写入 | `--node` 必须传完整文档 URL |
| 创建最终文档 | `dws doc create --name <标题> --markdown <内容> --format json` | name, markdown | 写入 | 若内容过长，改用先 create 再分块 update append |
| 网络搜索 | `web_search` | query | 只读 | 搜索市场规模、竞品、融资等公开信息 |
| 网页深度阅读 | `fetch` | url, prompt | 只读 | 深入阅读行业报告、招股书等 |

---

## 4) 操作衔接与信息传递

| 步骤 | 先执行 | 获得 | 传给 |
|------|--------|------|------|
| 1. 创建草稿 | `dws doc create` | `docUrl`（完整 URL） | 后续所有 `dws doc update --node` |
| 2. 市场调研 | `web_search` x N | 市场规模、CAGR、用户痛点 | `dws doc update --mode append` 写入草稿 |
| 3. 竞争调研 | `web_search` + `fetch` | 竞品信息、对比数据 | `dws doc update --mode append` 写入草稿 |
| 4. 内部资料 | `dws doc search` → `dws doc read` | 产品/团队/财务信息 | `dws doc update --mode append` 写入草稿 |
| 5. 财务基准 | `web_search` + `fetch` | 行业 benchmark、可比案例 | `dws doc update --mode append` 写入草稿 |
| 6. 结构化分析 | Agent 自行分析 | TAM/SAM/SOM、竞品矩阵、SWOT 等 | `dws doc update --mode append` 写入草稿 |
| 7. Writer Agent | `task` 分发 | 草稿 `docUrl` | Writer Agent 读取草稿 → 创建最终云文档 |

资料收集与分析框架的详细模板见：
- [references/research-templates.md](references/research-templates.md) — Module A 资料收集模板（A1-A4）
- [references/analysis-frameworks.md](references/analysis-frameworks.md) — Module B 结构化分析框架（B1-B5）

---

## 5) 踩坑提醒

| 错误做法 | 正确做法 | 原因 |
|---------|---------|------|
| `dws doc update --node nodeId123` | `dws doc update --node "https://alidocs.dingtalk.com/i/nodes/nodeId123"` | `--node` 必须传完整 URL，裸 nodeId 会报 `invalidRequest.inputArgs.invalid` |
| 一次性传入 3000 字符的 `--markdown` | 分块传入，每块 ≤ 1500 字符 | 超长内容会报 `invalidRequest.inputArgs.invalid` |
| `--markdown` 中用字面量 `\n` | 使用真实换行符 | 字面量 `\n` 导致文档格式错乱 |
| `cat BI 分析 A 轮融资计划.md` | `cat "BI 分析 A 轮融资计划.md"` | 文件名含空格未加引号，shell 拆分为多个参数 |
| `dws doc create --markdown <完整BP>` 一次性创建 | 先 `create` 空文档，再分块 `update --mode append` | 完整 BP 内容过长，一次性传入必然失败 |

---

## 6) 易混淆场景

| 用户说 | 线索 | 应路由到 |
|--------|------|---------|
| "帮我调研一下 XX 公司的背景" | 仅公司调研，无融资/BP 结构需求 | `wukong-doc-company-research` |
| "帮我写个产品白皮书" | 产品文档，非投资视角 | 通用文档生成技能 |
| "帮我做个行业分析报告" | 行业分析，非完整 BP | `wukong-doc-company-research` 或通用分析 |
| "帮我算一下 LTV/CAC" | 单纯财务计算 | 通用对话/计算 |

---

## 7) 核心工作流

### 任务分解（todo_write）

```json
{
  "todos": [
    {"content": "明确项目定位：确认产品、目标市场、融资阶段与受众", "status": "in_progress"},
    {"content": "市场机会与规模验证：TAM/SAM/SOM、增长率、用户画像", "status": "pending"},
    {"content": "竞争格局调研：竞品对比矩阵、市场份额、差异化壁垒", "status": "pending"},
    {"content": "产品/团队/财务信息收集：dws doc search + web_search", "status": "pending"},
    {"content": "结构化分析：商业模式画布 + 财务模型 + SWOT", "status": "pending"},
    {"content": "调用 Writer Agent 生成商业计划书云文档", "status": "pending"}
  ]
}
```

### 资料收集规则

1. **MUST**: 每完成一个调研模块，立即 `dws doc update --mode append` 追加到草稿
2. **MUST**: 遵守 [references/constraints.md](references/constraints.md) 中的全部数据质量红线
3. **SHOULD**: 使用 `dws doc search` 检索内部已有资料
4. **SHOULD**: 使用 `web_search` 多角度覆盖市场、竞品、财务基准
5. **TRAP**: 查不到的信息标注"需补充资料"，不可编造

### Writer Agent 任务分发

主 Agent 完成数据收集和分析后，调用 `task` 分发给 Writer Agent。Writer Agent 执行流程与文档模板见 [references/document-template.md](references/document-template.md)。

---

## 8) 输入与输出

- **必选输入**：项目/公司简介（产品、解决的问题）、目标市场（行业/用户群）
- **可选输入**：融资阶段、关键经营指标、竞争对手、团队背景、融资金额与估值预期
- **输出**：云文档商业计划书（9 大章节 + 附录），可直接用于投资人评审

### 质量标准

- **事实可信**：核心数据有来源，市场规模至少 2 个独立来源交叉验证
- **逻辑严谨**：市场痛点→产品方案→商业模式→竞争壁垒→财务回报完整链路
- **投资视角**：回答"为什么是这个市场""为什么是你""为什么是现在"
- **可操作性**：融资需求明确，资金用途有拆分，里程碑可追踪

---

## 9) 使用示例

### 示例 1: 完整商业计划书

**用户说**: "帮我写一份智能决策平台的商业计划书，我们是做 AI 数据分析的，目标客户是中大型企业，准备 A 轮融资 5000 万"

**AI 做**:
1. `todo_write` 分解 6 步任务
2. `dws doc create --name "BP草稿-智能决策平台"` 创建草稿
3. `web_search` 调研市场规模、竞品、财务基准 → 每步 `dws doc update --mode append`
4. `dws doc search` 检索内部资料 → `dws doc update --mode append`
5. 执行结构化分析（TAM/SAM/SOM、竞品矩阵、SWOT）→ `dws doc update --mode append`
6. `task` 分发 Writer Agent → 读取草稿 → 创建最终云文档

**结果**: 云文档链接，包含 9 大章节的完整商业计划书

### 示例 2: 指定竞品的商业计划书

**用户说**: "帮我做一份 BP，重点分析和竞品 A、竞品 B 的差异化"

**AI 做**: 同示例 1，但在竞争格局调研中重点 `web_search` + `fetch` 深度调研指定竞品，竞品对比矩阵细化维度

**结果**: 竞争分析章节更深入详实的云文档商业计划书

### 示例 3: 最简场景

**用户说**: "帮我写个 BP"

**AI 做**: 先确认必选信息（产品是什么、目标市场），然后执行完整流程

**结果**: 云文档商业计划书

---

## 10) 错误处理

详细的错误处理表见 [references/error-handling.md](references/error-handling.md)，覆盖以下分类：

| 错误分类 | 典型场景 | 处理原则 |
|---------|---------|---------|
| **权限/认证** | `dws doc create` 失败 | 请重新执行上一条命令（最多重试两次） |
| **参数格式** | `--node` 裸 nodeId、`--markdown` 过长、字面量 `\n` | 使用完整 URL、分块写入、真实换行符 |
| **网络异常** | `web_search` 超时、`fetch` 返回 403 | 重试 1-2 次，失败则跳过并标注"需补充资料" |
| **数据不足** | 市场数据来源不足、竞品信息不完整 | 标注"需补充资料"，建议用户提供内部数据 |
| **服务不可用** | `dws` 服务故障 | 提示用户稍后重试，或先以对话形式输出框架 |

---

## 11) 质量检查

完整的质量检查清单与常见失败模式见 [references/checklist.md](references/checklist.md)。
