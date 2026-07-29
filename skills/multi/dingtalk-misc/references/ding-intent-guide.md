# ding 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "DING消息/查DING/DING历史" | 查询 DING 消息列表 | `ding message list` | `chat message list` | ding 是独立顶层命令；ding message list 查 DING 消息；chat message list 查普通聊天消息 |
| "DING接收状态/谁收到了DING" | DING 接收状态 | `ding message receiver-status` | `chat message read-status` | ding 是独立顶层命令；receiver-status 查 DING 接收；chat message read-status 查普通消息已读 |
| "发DING/DING通知" | 发送 DING 消息 | `ding message send` | `chat message send` | DING 是钉钉的强提醒（应用内/短信/电话），独立顶层命令；普通群消息用 chat |
| "撤回DING" | 撤回 DING 消息 | `ding message recall` | `chat message recall` | DING 撤回独立命令；chat recall 是撤回普通聊天消息 |
| "以我的名义发DING/个人发DING/用户身份DING" | 以用户身份发 DING | `ding message send-personal` | `ding message send` | send-personal 以用户身份发送，无需 robot-code；send 以机器人身份发送 |
| "以我的名义撤回DING/个人撤回DING" | 以用户身份撤回 DING | `ding message recall-personal` | `ding message recall` | recall-personal 以用户身份撤回；recall 以机器人身份撤回 |
| "消息转DING/把这条消息DING给某人/转发为DING" | 消息转 DING | `ding message send-by-message` | `ding message send-personal` | send-by-message 是将已有消息转为 DING，需指定原消息；send-personal 是直接发新 DING |
