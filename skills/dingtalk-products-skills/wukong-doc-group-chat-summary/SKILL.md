---
name: wukong-doc-group-chat-summary
label: 群聊内容摘要
requires: [dws]
description: >
  基于钉钉群聊的智能摘要与洞察生成能力。从指定群聊/时间范围的消息中提取关键信息，识别决策、行动项、风险，生成钉钉云文档结构化摘要。
  Use when user mentions "总结群聊", "整理聊天记录", "把群里讨论写成文档", "拉一下群里的决策和待办", "帮我追踪群里关于 XX 的讨论".
  Distinct from dws chat message send (仅发送消息) and dws doc create (手动创建文档).
  Do NOT use for 单聊消息整理 (应使用私聊场景技能)、或会议听记摘要 (应使用 dws listener).
---

# 钉钉群聊摘要技能

从指定群聊/时间范围的消息中提取关键信息，识别决策、行动项、风险，生成钉钉云文档结构化摘要。

## 禁止事项

- 禁止通过 curl、HTTP API 等非 `dws` 方式操作钉钉产品
- 禁止凭空构造 chatId、messageId、docId 等标识符，所有 ID 必须从实际命令返回值中提取
- 禁止 `dws` 命令不加 `--format json` 参数
- 禁止在未获取群聊标识前直接拉取消息
- 禁止在结构化提取未完成前直接创建文档

## 强制要求

- 每条 `dws` 命令均须携带 `--format json` 参数
- 必须先定位目标群聊获取 `openConversationId` 后再拉取消息
- 时间窗口必须明确标注（默认过去 24 小时，UTC+8）
- 所有提取的决策/行动项必须可溯源到原始消息
- 创建文档前必须确认结构化提取已完成所有章节（议题/决策/行动项/风险/链接）
- **必须从 `dws doc create` 返回值中提取 `docUrl`，并在最终回复中以可点击链接形式输出给用户**
- **必须在钉钉文档内容的"统计信息"表格中包含"文档链接"字段，格式为 `[链接地址](链接地址)`**
- 禁止在未成功获取 `docUrl` 的情况下结束任务；若 `doc create` 返回中无 `docUrl`，须调用 `dws doc get --node <nodeId> --format json` 补充获取

## 能力清单

| 动作 | 安全等级 | 说明 |
|------|---------|------|
| 意图识别 | 只读 | 提取群聊标识、时间范围、关注主题 |
| 群聊定位 | 只读 | 通过群名/群 ID 搜索目标群聊 |
| 消息拉取 | 只读 | 按时间范围获取群聊消息列表 |
| 消息预处理 | 只读 | 去噪、分段、去重 |
| 结构化提取 | 只读 | 识别议题/决策/行动项/风险/链接 |
| 文档生成 | 写入 | 使用 dws doc create/update 直接写入钉钉云文档 |

## 涉及工具

| 工具 | 用途 |
|------|------|
| `dws chat` | 查询群聊消息 |
| `dws doc` | 创建云文档 |
| `dws contact` | 查询用户信息 |

## 总体工作流

```
用户请求 → 定位群聊 → 拉取消息 → 预处理 → 结构化提取 
→ dws doc create/update → 钉钉文档
```

### Module A — 消息获取

**目标**: 定位目标群聊并拉取指定时间范围的消息

1. **A1 群聊定位**: 调用 `dws chat search --query "<群名>" --format json`，获取 `openConversationId`
2. **A2 拉取消息**: 调用 `dws chat message list --group <openConversationId> --time "<yyyy-MM-dd HH:mm:ss>" --format json`
3. **A3 时间窗口**: 默认过去 24 小时（UTC+8），支持"今天"/"本周"/"最近 3 天"或自定义起止时间
4. **A4 消息预处理**: 去噪（表情/系统消息/闲聊）、分段（>30 分钟间隔）、去重
5. **A5 整理消息分段**: 按时间段归类消息，准备结构化提取的输入

### Module B — 结构化提取

**目标**: 提取"决策、行动、风险、资源"四类关键信息

1. **B1 核心议题**: 识别 3-7 个主要话题，按热度排序
2. **B2 关键决策**: 识别信号词（"决定"/"确认"/"同意"/"拍板"），提取决策人、时间、原始消息
3. **B3 行动项**: 识别信号词（"@XX 负责"/"跟进"/"我来做"），提取负责人、截止时间
4. **B4 风险问题**: 识别信号词（"问题是"/"风险"/"卡住了"/"阻塞"），评估严重程度
5. **B5 重要链接**: 提取钉钉文档/GitHub/外部链接，补充标题摘要
6. **B6 汇总提取结果**: 按模板整理所有章节，准备写入钉钉文档

### Module C — 文档生成

**目标**: 直接使用 `dws doc` 命令创建钉钉云文档并写入摘要内容

1. **C1 创建文档**: 调用 `dws doc create --name "群聊摘要：{群名}（{时间范围}）" --format json`，从返回值中提取 `nodeId` 和 `docUrl`
2. **C2 写入内容**: 调用 `dws doc update --node <nodeId> --mode overwrite --markdown "<结构化摘要的完整 Markdown 内容>" --format json`
   - **关键**: Markdown 内容必须包含"统计信息"表格，其中"文档链接"字段格式为 `| **文档链接** | [{docUrl}]({docUrl}) |`
3. **C3 返回链接**: 将 `docUrl` 以可点击链接格式输出给用户。若 `doc create` 返回中未包含 `docUrl`，则调用 `dws doc get --node <nodeId> --format json` 补充获取，确保最终输出中必须包含文档链接

## 上下文传递表

| 阶段 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `result.value[].openConversationId` | message list 的 --group 参数 |
| `chat message list` | `result.messages[]` | 预处理和结构化提取的输入 |
| 消息预处理 | 分段摘要 | 结构化提取的输入 |
| 结构化提取 | 议题/决策/行动项/风险 | dws doc update 的 --markdown 参数 |
| `doc create` | `result.nodeId`, `result.docUrl` | `nodeId` 用于 doc update 的 --node 参数；`docUrl` 必须以可点击链接形式输出给用户 |

## 意图映射

| 用户说 | 线索 | 映射功能 |
|--------|------|---------|
| "帮我总结一下 {群名} 群的聊天记录" | 明确群名 + 总结 | 完整流程 |
| "把这个群最近一周的讨论整理成摘要" | 时间范围 + 整理 | 完整流程（自定义时间） |
| "总结今天群里的重要决策和待办事项" | 今天 + 决策/待办 | 完整流程（聚焦决策/行动项） |
| "帮我追踪 {群名} 群里关于 {话题} 的讨论" | 话题追踪 | 部分流程（主题过滤） |
| "查一下 {群名} 群昨天发了什么" | 简单查询 | 部分流程（仅 Module A） |

## 易混淆场景

| 用户说 | 线索 | 应路由到 |
|--------|------|---------|
| "在群里发个通知" | 发送消息 | dws chat message send |
| "把会议纪要发到群里" | 群消息 + 已有文档 | dws chat message send |
| "总结一下刚才的会议听记" | 会议/听记 | dws listener summary |
| "把私聊记录整理成文档" | 私聊/单聊 | 不适用本技能 |

## 使用示例

### 示例 1: 完整群聊摘要
**用户说**: "帮我总结一下 项目攻坚群 的聊天记录，最近 3 天的"  
**执行步骤**:
1. 搜索群聊：`dws chat search --query "项目攻坚群"` → 获取 `openConversationId`
2. 计算时间范围：过去 3 天（UTC+8）
3. 拉取消息：`dws chat message list --group <openConversationId> --time "<3 天前>"`
4. 消息预处理：去噪、分段、去重
5. 结构化提取：识别议题/决策/行动项/风险/链接
6. 创建文档：`dws doc create --name "群聊摘要：项目攻坚群" --format json` → 获取 `nodeId`
7. 写入内容：`dws doc update --node <nodeId> --mode overwrite --markdown "<结构化摘要内容>"`  
**期望输出**:
```
✅ 已生成群聊摘要文档

## 📄 文档信息

**文档标题**: 群聊摘要：项目攻坚群
**统计范围**: 3 月 19 日 09:00 ~ 3 月 22 日 09:00 (UTC+8)
**消息总数**: 342 条 | **参与人数**: 12 人

## 📌 核心内容

- **关键决策**: 3 项（技术方案确认、上线时间调整、人员分工）
- **行动项**: 7 项待办，最近截止：3 月 24 日
- **高风险**: 2 项（依赖未就绪、测试资源紧张）

## 🔗 文档链接

[**点击查看完整摘要**](https://alidocs.dingtalk.com/i/nodes/xxxxxx)

或直接访问：https://alidocs.dingtalk.com/i/nodes/xxxxxx
```

### 示例 2: 最简触发
**用户说**: "总结下这个群的讨论"  
**执行步骤**:
1. **追问群聊标识**: "请问需要总结哪个群？可以提供群名或群链接"
2. **追问时间范围**: "需要总结多长时间范围的讨论？（默认过去 24 小时）"
3. 用户确认后执行完整流程  
**期望输出**: 确认信息后执行

### 示例 3: 主题追踪
**用户说**: "帮我追踪 技术选型群 里关于 微服务架构 的讨论"  
**执行步骤**:
1. 搜索群聊获取 `openConversationId`
2. 拉取消息后过滤关键词（或使用 `dws chat message list --group<openConversationId> --time "<时间>"` 后在应用层过滤）
3. 结构化提取时聚焦该主题相关消息
4. 生成摘要文档
5. 创建文档：`dws doc create --name "主题追踪：微服务架构讨论" --format json` → 获取 `nodeId`
6. 写入内容：`dws doc update --node <nodeId> --mode overwrite --markdown "<结构化摘要内容>"`  
**期望输出**: "✅ 已生成主题追踪文档，聚焦'微服务架构'相关讨论，共 23 条消息，识别决策 1 项、行动项 3 项。

## 🔗 文档链接

[**点击查看完整摘要**](https://alidocs.dingtalk.com/i/nodes/xxx)

或直接访问：https://alidocs.dingtalk.com/i/nodes/xxx"

## 错误处理

| 错误 | 原因 | AI 应该怎么做 |
|------|------|--------------|
| 群聊搜索无结果 | 群名错误或已退出 | 提示用户确认群名，或尝试群 ID/链接 |
| 消息拉取失败 | 无权限或 openConversationId 错误 | 提示用户确认是否在群内，检查权限 |
| 时间范围为空 | 格式解析错误 | 追问用户明确起止时间 |
| 无关键决策/行动项 | 群聊内容为闲聊 | 告知用户该时段无实质讨论，建议调整时间范围 |
| 文档创建失败 | 认证失效或权限不足 | 提示用户重新扫码认证，或检查文档创建权限 |
| 文档更新失败 | nodeId 无效或内容格式错误 | 检查摘要内容格式和 nodeId 是否正确，修正后重试 |

## 详细参考（按需读取）

- [references/workflow-detail.md](./references/workflow-detail.md) — 详细工作流步骤与检查项
- [references/doc-template.md](./references/doc-template.md) — 钉钉云文档摘要模板