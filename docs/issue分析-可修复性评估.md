# GitHub Issue 可修复性评估

> 评估时间：2026-05-04
> 评估范围：30 个 Open Issue
> 评估标准：是否可以在当前 CLI 代码库中修复/实现，不依赖钉钉服务端 API 变更

---

## 总览

| 分类 | 数量 | 说明 |
|------|------|------|
| ✅ 可修复（CLI 层） | 7 | 纯 CLI 代码修改，不依赖 API 变更 |
| ⚠️ 部分可修复 | 4 | CLI 层可做优化/变通，但完整解决需 API 配合 |
| ❌ 不可修复（需 API/平台支持） | 19 | 需要钉钉服务端新增 API 或平台能力 |

---

## ✅ 可修复（纯 CLI 代码修改）

### 1. [#107](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/107) — 多个命令的 flag 在 API 层必填但 --help 未标 (必填)

- **类型**: Bug / Docs
- **问题**: `send-by-bot` 的 `--robot-code`、`--title`，`approval list-initiated` 的 `--process-code` 等 flag 的 help 文本缺少 `(必填)` 标记
- **修复方案**: 修改 `internal/helpers/chat.go` 第 255 行 `--robot-code` 和第 258 行 `--title` 的 Flag 描述，追加 `(必填)`。`--process-code` 属于动态命令，需确认是否可在 catalog 层追加
- **修复难度**: 🟢 低
- **涉及文件**: `internal/helpers/chat.go`

### 2. [#106](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/106) — report create --help 未注明 contents[].key 必须精确匹配模板 field_name

- **类型**: Bug / Docs
- **问题**: `report create` 的 `--contents` help 未说明 key 必须精确等于模板 `field_name`
- **修复方案**: 修改 `internal/helpers/report.go` 中 `newReportCreateCommand` 的 `--contents` flag 描述和 Example，补充使用说明
- **修复难度**: 🟢 低
- **涉及文件**: `internal/helpers/report.go`

### 3. [#194](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/194) — CLI 参数命名不统一，建议增加人类友好的参数别名

- **类型**: UX / Bug
- **问题**: `chat search` 只有 `--query`，用户直觉会用 `--name` 或 `--keyword`
- **修复方案**: 在 `internal/helpers/chat.go` 的 `newChatSearchCommand` 中增加 `--name` 和 `--keyword` 作为隐藏别名（参考 todo.go 中 `FlagOrFallback` 的模式）。类似地为 `--group` 增加 `--group-id` 等别名
- **修复难度**: 🟢 低
- **涉及文件**: `internal/helpers/chat.go`

### 4. [#195](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/195) — chat message list 分页文档不清晰

- **类型**: Bug（实为文档问题，评论区已定位根因）
- **问题**: 用户误将 `nextCursor` 当作 `--time` 参数，实际应使用消息的 `createTime`
- **修复方案**: 更新 `skills/references/products/chat.md` 中 `message list` 的分页说明，明确区分 `nextCursor`（仅用于 `list-all` 的 `--cursor`）和 `createTime`（用于 `list` 的 `--time`）。CLI 层可增加 `--time` 参数校验：检测到纯数字时给出友好提示
- **修复难度**: 🟢 低
- **涉及文件**: `skills/references/products/chat.md`，可选增强动态命令的参数校验逻辑

### 5. [#191](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/191) — chat message 的 createTime 只精确到秒，希望精确到毫秒

- **类型**: Bug / Enhancement
- **问题**: 消息 `createTime` 格式为 `"2026-04-28 11:47:38"`，缺少毫秒和时区
- **修复方案**: 这取决于 API 返回的原始数据。如果 API 返回的是毫秒时间戳，则可在 CLI 的输出格式化层（`internal/output/` 或动态命令的响应处理）将时间戳转为 ISO-8601 格式含毫秒。需先确认 API 原始返回格式
- **修复难度**: 🟡 中（需确认 API 原始返回格式）
- **涉及文件**: 输出格式化相关代码

### 6. [#190](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/190) — doc download 对 axls 表格先触发 PAT 授权，授权后才返回不支持

- **类型**: Bug
- **问题**: 对钉钉在线表格执行 `doc download` 时，先触发 PAT 授权，授权后才提示不支持
- **修复方案**: 在 `doc download` 命令的执行流程中，**先检查文件类型**（通过 nodeId 获取文件 meta 信息，判断 extension 是否为 `axls`），如果是在线表格则直接提示使用 `getRange`，跳过 PAT 授权流程。这需要在动态命令的 pre-request 阶段或 pipeline handler 中增加前置检查
- **修复难度**: 🟡 中
- **涉及文件**: `internal/pipeline/handlers/prerequest.go` 或动态命令逻辑

### 7. [#188](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/188) — dws 安装后没有自动把 skill 复制到 .hermes/skills/ 目录下

- **类型**: Bug
- **问题**: 安装脚本未自动复制 skill 到 `.hermes/skills/` 目录
- **修复方案**: 检查 `scripts/install.sh` 中 `install_skills_to_homes` 函数的目标目录列表，确认是否包含 `.hermes/skills/`。如果缺少则添加。`scripts/install-skills.sh` 也需同步检查
- **修复难度**: 🟢 低
- **涉及文件**: `scripts/install.sh`, `scripts/install-skills.sh`

---

## ⚠️ 部分可修复（CLI 层可优化，完整解决需 API 配合）

### 8. [#189](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/189) — PAT 已授权后仍重复要求授权

- **类型**: Bug
- **问题**: 已完成 permanent 授权后，再次执行仍触发 PAT 授权
- **CLI 层可做的**: 检查 `internal/auth/` 中的 PAT 缓存逻辑，确认是否正确缓存了授权状态。可能是 `oauth_provider.go` 中的 token 刷新逻辑未正确保留 PAT scope 信息。也可能是 #213 已修复的 MCP re-fetch 问题的后遗症
- **需 API 配合的**: 如果是服务端 PAT 状态未正确持久化，则需 API 侧修复
- **修复难度**: 🟡 中
- **涉及文件**: `internal/auth/oauth_provider.go`, `internal/auth/device_flow.go`

### 9. [#214](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/214) — Codex App 沙盒中无法访问密钥链

- **类型**: Bug
- **问题**: Codex App 的沙盒环境无法访问系统密钥链（keychain），导致 DWS 无法使用
- **CLI 层可做的**: 增加 keychain 不可用时的 fallback 策略。当前有两种存储方式：`keychain_store.go`（密钥链）和 `secure_store.go`（文件加密，MAC 地址派生密钥）。可增加环境检测，当 keychain 不可用时自动 fallback 到文件加密存储，或支持环境变量配置凭证
- **需 API/平台配合的**: 如果需要支持无状态的 token 传递（如环境变量），需设计新的认证流程
- **修复难度**: 🟡 中
- **涉及文件**: `internal/auth/keychain_store.go`, `internal/auth/secure_store.go`, `internal/auth/manager.go`

### 10. [#176](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/176) — chat message 获取聊天记录时希望带上 userId 和 messageId

- **类型**: Enhancement
- **问题**: 聊天记录只返回 `sender: "中文名"`，缺少 userId 和 messageId
- **CLI 层可做的**: 如果 API 原始响应中已包含 userId 和 messageId 字段，只需在 CLI 的输出层取消对这些字段的过滤即可。如果 API 未返回则需 API 侧支持
- **修复难度**: 🟢 低（如 API 已返回）/ 🔴 不可修复（如 API 未返回）
- **涉及文件**: 动态命令输出逻辑

### 11. [#208](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/208) — 同一台设备上实现多个账号登录

- **类型**: Feature
- **问题**: 希望在同一设备上隔离登录多个账号，支持不同智能体操作不同账号
- **CLI 层可做的**: 支持 `--profile` 参数或环境变量，将凭证按 profile 隔离存储到不同目录。参考 AWS CLI 的 `--profile` 机制
- **需 API 配合的**: 无需 API 变更，纯 CLI 架构改造
- **修复难度**: 🔴 高（涉及整体凭证管理架构重构）
- **涉及文件**: `internal/auth/` 目录多个文件

---

## ❌ 不可修复（需钉钉服务端 API 或平台能力支持）

### 12. [#212](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/212) — 发送消息时支持 @机器人

- **原因**: 需要钉钉 IM API 支持 @机器人的消息格式和能力

### 13. [#211](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/211) — CLI 支持把消息设置为已读

- **原因**: 需要钉钉 IM API 开放"设置消息已读"的接口

### 14. [#207](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/207) — 群聊内机器人之间相互 @艾特联动

- **原因**: 需要钉钉平台支持机器人间 @ 的事件传递和消息格式

### 15. [#203](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/203) — 直接访问本地的聊天记录

- **原因**: 钉钉聊天记录存储在客户端本地加密数据库中，CLI 无法直接访问；需要钉钉开放本地数据接口

### 16. [#198](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/198) — 支持下载群聊中的图片/文件附件

- **原因**: 需要钉钉 API 开放聊天附件的下载接口（media token 解析和下载链接生成）

### 17. [#196](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/196) — 将 openyida CLI 融合进 DWS 生态

- **原因**: 产品层面的生态整合决策，非 CLI 代码层面可解决

### 18. [#183](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/183) — 钉钉文档支持添加共享成员/协作者

- **原因**: 需要钉钉文档 API 开放添加协作者的接口

### 19. [#172](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/172) — 钉钉文档支持"链接"类型

- **原因**: 需要钉钉文档 API 支持创建链接类型文档节点

### 20. [#151](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/151) — 单聊/机器人发送链接卡片内容

- **原因**: 需要钉钉 IM API 支持发送链接卡片类型的消息（当前仅支持纯文本/Markdown）

### 21. [#149](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/149) — 读取 OA 审批模板的流程设计/表单设计信息

- **原因**: 需要钉钉 OA API 开放审批模板结构的读取接口

### 22. [#136](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/136) — 支持往钉钉文档里插入 image blockType

- **原因**: 需要钉钉文档 API/MCP 开放 `blockType: "image"` 或让 markdown 解析器识别图片语法

### 23. [#110](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/110) — 支持获取最近联系人

- **原因**: 需要钉钉 IM API 开放"最近联系人"列表接口

### 24. [#91](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/91) — 通讯录 userId 支持 schema 调起私聊

- **原因**: 需要钉钉客户端开放 schema 调起能力（`dingtalk://` 协议），非 CLI 代码可控

### 25. [#87](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/87) — 获取工作台自定义应用的数据

- **原因**: 需要钉钉 API 开放自定义应用（如合同审批）的数据读取接口

### 26. [#84](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/84) — 日志读取表格内容丢失

- **原因**: 钉钉日志 API 返回的数据中表格内容被替换为 `[表格]` 占位符，需 API 侧改进返回完整内容

### 27. [#83](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/83) — 支持通过 CLI 发起 OA 审批流程

- **原因**: 需要钉钉 OA API 通过 PAT/OAuth 开放审批发起能力（当前仅企业内部应用有此 API）

### 28. [#79](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/79) — todo 待办增强：查询自己创建的 + 时间范围搜索

- **原因**: 需要钉钉待办 MCP API 开放"按创建者查询"和"时间范围搜索"的能力

### 29. [#73](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/73) — AI 表格读不到公式字段内容

- **原因**: 钉钉 AI 表格 API 返回数据中未包含公式字段的计算结果，需 API 侧修复

### 30. [#68](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/68) — 机器人回复支持表情回复

- **原因**: 需要钉钉 IM API 开放机器人表情回复（emoji reaction）能力

---

## 推荐修复优先级

按 **影响面 × 修复难度** 排序：

| 优先级 | Issue | 修复难度 | 影响面 |
|--------|-------|---------|--------|
| P0 | #107 flag 必填标记 | 🟢 低 | 影响所有下游 MCP wrapper |
| P0 | #106 report create help | 🟢 低 | 影响 agent 调用成功率 |
| P0 | #195 分页文档 | 🟢 低 | P1 级功能阻塞 |
| P1 | #194 参数别名 | 🟢 低 | 提升 UX |
| P1 | #188 skill 安装 | 🟢 低 | 影响新用户体验 |
| P1 | #190 axls 前置检查 | 🟡 中 | 避免无效授权 |
| P2 | #191 时间精度 | 🟡 中 | 提升数据质量 |
| P2 | #189 PAT 重复授权 | 🟡 中 | 影响自动化流程 |
| P2 | #214 沙盒 keychain | 🟡 中 | 影响 Codex 用户 |
| P3 | #176 userId/messageId | 依赖 API | 提升数据完整性 |
| P3 | #208 多账号 | 🔴 高 | 架构改造 |
