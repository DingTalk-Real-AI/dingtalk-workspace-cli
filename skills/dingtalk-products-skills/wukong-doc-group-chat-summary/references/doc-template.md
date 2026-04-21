# 钉钉云文档摘要模板

群聊摘要技能生成钉钉云文档时遵循的文档结构模板。

**使用说明**:
- 本模板用于指导 `dws doc update` 命令的 Markdown 内容组织
- 结构化提取结果应按此模板格式直接写入钉钉文档
- 支持 Callout、表格、列表等多种 Markdown 格式

---

```markdown
# 群聊摘要：{群名}

## 📊 统计信息

| 项目 | 数值 |
|------|------|
| **统计范围** | {start_date} {start_time} ~ {end_date} {end_time} (UTC+8) |
| **消息总数** | {msg_count} 条 |
| **参与人数** | {user_count} 人 |
| **生成时间** | {gen_date} {gen_time} |
| **文档链接** | [{doc_url}]({doc_url}) |

---

## TL;DR

<!-- 3-5 条核心要点，使用 Callout 格式 -->

> **[决策]** {tldr_decision_summary}  
> **[行动]** {action_count} 项待办，最近截止：{nearest_due_date}  
> **[风险]** {tldr_risk_summary}

---

## 核心议题

### 1. {议题名称}

- **讨论摘要**: {topic_summary}
- **结论/共识**: {topic_conclusion}
- **参与人**: {participant_list}

---

### 2. {议题名称}

- **讨论摘要**: {topic_summary}
- **结论/共识**: {topic_conclusion}
- **参与人**: {participant_list}

---

## 关键决策

| 决策内容 | 决策人 | 时间 | 备注 |
|---------|-------|------|------|
| {decision_content} | {decision_maker} | {decision_time} | {decision_note} |
| {decision_content} | {decision_maker} | {decision_time} | {decision_note} |

---

## 行动项

| 任务 | 负责人 | 截止时间 | 状态 |
|-----|-------|---------|------|
| {task_description} | {owner} | {due_date} | {status} |
| {task_description} | {owner} | {due_date} | {status} |

> **提醒**: 请各负责人按时完成，下次同步时汇报进度。

---

## 风险与问题

<!-- 使用 Callout 突出高风险 -->

> **[高风险]** {risk_description}  
> - **影响**: {risk_impact}  
> - **状态**: {risk_status}  
> - **建议**: {risk_suggestion}

### 其他问题

- **{issue_name}**: {issue_description} —— **状态**: {issue_status}

---

## 重要链接

- [{resource_name}]({resource_url}) - {sharer} 分享于 {share_time}
- [{resource_name}]({resource_url}) - {sharer} 分享于 {share_time}

---

## 待跟进

- [ ] {followup_item_1}
- [ ] {followup_item_2}
- [ ] 下次同步：{next_sync_date}

---

**说明**: 本摘要由 AI 自动生成，所有信息源自群聊记录。如有疑问请核对原始消息。
```

---

## 格式规范

### Callout 使用场景

1. **TL;DR 部分**: 3-5 条核心要点
2. **高风险项**: 突出显示需重点关注的问题
3. **关键决策**: 可选，用于特别重要的决策

### 表格使用场景

1. **核心议题**: 仅当议题≥3 个时使用表格
2. **关键决策**: 必须使用表格
3. **行动项**: 必须使用表格
4. **重要链接**: 可选，链接≥3 个时使用

### 列表使用场景

1. **讨论摘要**: 并列项≥3 时使用无序列表
2. **其他问题**: 无序列表
3. **待跟进**: 复选框列表

### 可读性规则

- 单段 >150 字必须拆分（列表/小节/Callout）
- 连续两段无列表且信息密度高 → 插入列表或 Callout
- 行动项必须有明确的负责人和截止时间
