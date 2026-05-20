# 家校群 (edu-group) 命令参考

## 命令总览

### student-group (师生群管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `student-group info` | 查询班级师生群信息 | `--dept-id` |
| `student-group exists` | 检查是否已创建师生群 | `--dept-id` |
| `student-group members` | 查询师生群成员列表 | `--dept-id` |
| `student-group is-in` | 判断用户是否在师生群中 | `--dept-id` |
| `student-group conversation` | 查询班级群会话详情 | `--dept-id` |
| `student-group create` | 创建班级师生群 | `--dept-id` |
| `student-group disband` | 解散班级师生群 ⚠️ | `--dept-id` |

### class-group (班级群会话管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `class-group conversation-id` | 获取班级群会话ID | `--dept-id` |
| `class-group conversation` | 获取班级群完整会话信息 | `--dept-id` |
| `class-group exists` | 检查班级群是否存在 | `--dept-id` |
| `class-group list-by-cids` | 根据会话ID列表批量查询 | `--conversation-ids` |

### batch (批量操作)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `batch check-student-group` | 批量检查是否已创建师生群 | `--class-ids` |
| `batch get-class-groups` | 批量获取班级群信息 | `--class-ids` |
| `batch create-student-groups` | 批量创建师生群 | 无 |

## 意图判断

用户说"师生群/学生群" → student-group 子命令
用户说"班级群/群会话" → class-group 子命令
用户说"批量/一键操作" → batch 子命令

关键区分: student-group(师生群) vs class-group(班级群会话管理)

## 核心工作流

1. 检查群是否存在 → `student-group exists --dept-id <deptId>`
2. 如不存在，创建 → `student-group create --dept-id <deptId>`
3. 查看成员 → `student-group members --dept-id <deptId>`
4. 获取会话ID → `class-group conversation-id --dept-id <deptId>`

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `edu-contact school class-list` | `deptId` | 所有 edu-group 命令的 --dept-id |
| `class-group conversation-id` | `conversationId` | edu-app 消息命令的 --cid |
| `batch check-student-group` | 未创建群的班级 | batch create-student-groups |
