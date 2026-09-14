# Minutes Recipe 通用约束

> 返回入口：[DingTalk Minutes Skill](../../SKILL.md) · [Reference 与脚本索引](../minutes.md) · [兼容 Recipe](../lite-recipes.md)

本页只约束仍使用旧 Recipe 名称的兼容调用方，不复制根 Skill 的完整执行契约。

## 目标与 ID

| 字段 | 真实来源 | 后续用途 |
|---|---|---|
| `taskUuid` | Minutes 搜索/列表/创建的真实结果，或可信听记 URL 解析 | 所有详情、内容、更新、权限、下载与异步产物命令 |
| 成员 UID | 同 profile 下 AI Search/Contact 的唯一人员结果 | `+share/+unshare` 与需要身份绑定的发言人替换 |
| `sessionId` | upload create 的真实结果 | upload complete/cancel 与状态恢复 |
| `taskId` | 异步 create 的真实结果 | 继续轮询或恢复，不能重新 create 代替 |

标题、姓名、手机号、列表顺序和“最新一条”都不能替代稳定 ID。目标零命中、多候选、跨组织或分页未完成时停止并消歧。

## 分页与批量

- 完整列表和逐字稿必须读取到端点穷尽并检查 `complete=true`；缺 token、cursor 不前进/循环、页数上限或某页失败都属于不完整。
- 优先使用发布的批量接口或 Shortcut；没有批量能力时采用有界执行，不把 shell `& wait` 写成通用要求。
- 批量结果逐项记录目标、动作、成功、失败与未知状态。部分成功返回完整 ledger，不只汇报成功项。

## 写入与恢复

- Runtime 要求确认时，远端写调用数在确认前必须为 0；dry-run 也必须为 0。
- 非幂等或状态未知的 create/update 不盲目重试。先按真实 ID 读回，再决定只恢复未完成步骤。
- 写入成功必须有业务回执和必要读回证据；退出码 0、HTTP 200 或 `success=true` 单独都不足以证明完成。

## 跨产品传递

- Minutes 只交付真实听记内容和稳定 ID；写文档、建待办、发消息分别切换 `dingtalk-doc`、`dingtalk-todo`、`dingtalk-chat`。
- 跨产品传递时保留来源听记的 `taskUuid/title/profile`，不得把另一个组织或另一个候选的字段拼接到当前对象。
