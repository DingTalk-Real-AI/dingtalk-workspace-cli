# 上下文传递表

## 上下文传递表
| 操作 | 提取 | 用于 |
|------|------|------|
| `contact user get-self` | `userId` | summary 的 --user |
| `rules` | `groupId` | schedule import 的 --group-id |
| `schedule import` | `classId` | schedule import 的 schedules 中的 classId |
| `contact user search` | `userId` | schedule import/get 的 userId |

| `contact user get-self` / `contact user search` | `userId` | summary 的 --user, vacation records 的 --user；selfsetting get/save 的 --user（必填） |
| 当前登录上下文 | `corpId`, `opUserId` | selfsetting get/save 自动补齐 MCP 入参, CLI 不需要传 `--corp-id` / `--op-user` |
| `vacation types` | `leaveCode` | vacation balance 的 --leave-code, vacation records 的 --leave-code |
