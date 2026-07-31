# Workflow：新人入职群聊接待

目标：完成“查人 → 拉群 → 发欢迎消息 → 创建待办 → 预约会议”的闭环。所有 Agent 调用统一补充 `--format json`。

## 触发语

- “给新人 XXX 办理入职”
- “欢迎 XXX 加入团队，拉他进项目群并发 onboarding”
- “给 XXX 建入职待办和欢迎会”

## 步骤

### 1. 获取新人身份

```bash
dws aisearch person --keyword "<新人姓名>" --dimension name --format json
# 或
dws contact user search --query "<新人姓名>" --format json
```

提取 `openDingTalkId` 和 `userId`。返回多条时让用户确认；返回零条时停止并报告。

### 2. 搜索目标群并添加成员

```bash
dws chat search --query "<项目群名>" --format json
dws chat group members add --id <openConversationId> --users <userId> --format json
```

群不存在时重新核对群名；无权限时停止并报告群管理员。

### 3. 发送欢迎消息

```bash
dws chat message send --group <openConversationId> \
  --title "欢迎 <姓名> 加入" \
  --text "@所有人 欢迎 <姓名> 加入项目组！入职待办和欢迎会已安排。" \
  --at-all \
  --format json
```

### 4. 创建入职待办

```bash
dws todo task create --title "<姓名> 入职待办：完成环境配置" \
  --executors <userId> \
  --priority 40 \
  --due-time "<3 天后 18:00>" \
  --format json
```

### 5. 预约欢迎会议

```bash
dws calendar event create --summary "<姓名> 入职欢迎会" \
  --start-time "<明天 14:00>" \
  --end-time "<明天 15:00>" \
  --attendees <userId> \
  --format json
```

## 验收

完成后汇报：

- 已拉入的群名和 `openConversationId`。
- 欢迎消息发送结果。
- 待办 ID 和截止时间。
- 会议 ID 和时间。

任一步失败时保留已完成结果，并说明失败步骤与是否需要回滚或人工处理；不要把中间步骤成功误报为整个 Workflow 完成。
