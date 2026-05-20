# 家校通讯录 (edu-contact) 命令参考

## 命令总览

### school (学校/组织管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `school roles` | 查询用户在组织内的身份 | 无 |
| `school structure` | 查询学校组织架构 | 无 |
| `school periods` | 查询学校学段信息 | 无 |
| `school type` | 查询学校组织类型 | 无 |
| `school stats` | 查询学校统计数据 | `--statistics-type`(可选) |
| `school class-list` | 查询学校所有班级列表 | 无 |

### class (班级管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `class detail` | 查询班级详情 | `--dept-id` |
| `class students` | 查询班级学生信息 | `--dept-id` |
| `class teachers` | 查询班级老师列表 | `--dept-id` |
| `class same-name` | 查询班级内同名学生 | `--dept-id` |
| `class user-role` | 查询用户在班级内的角色 | `--dept-id` |
| `class search-by-name` | 根据姓名查询班级 | `--query-type`, `--name` |
| `class headmaster` | 根据班级名查询班主任 | `--class-name` |
| `class search-by-teacher` | 根据老师姓名查询班级 | `--name` |
| `class update-student` | 更新学生信息 | `--class-id`, `--student-user-id` |
| `class add-student` | 添加学生到班级 | `--dept-id`, `--student-name` |
| `class modify-student-info` | 修改学生信息 | `--dept-id`, `--target-user-id` |
| `class delete-teacher` | 删除班级教师 ⚠️ | `--class-id`, `--teacher-user-id` |
| `class update-info` | 更新班级信息 | `--class-id` |
| `class update-student-number` | 修改学生学号 | `--class-id`, `--student-user-id`, `--student-number` |
| `class add-unofficial-student` | 添加非行政班学生 | `--dept-id`, `--student-staff-ids` |
| `class delete-students` | 批量删除学生 ⚠️ | `--dept-id`, `--student-user-ids` |
| `class update-student-mobile` | 修改学生手机号 | `--dept-id`, `--student-user-id`, `--mobile` |
| `class move-student` | 学生移班 | `--student-user-ids`, `--origin-class-id`, `--target-class-id` |
| `class add-teachers` | 批量添加班级教师 | `--dept-id`, `--teacher-user-ids` |

### family (家庭关系查询)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `family children` | 查询家长的孩子信息 | 无 |
| `family parents` | 查询学生的家长信息 | 无 |

### teacher (教师管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `teacher classes` | 查询老师管理的班级列表 | 无 |
| `teacher update-course` | 更新教师任教科目 | `--teacher-class-infos`(JSON数组) |

## 意图判断

用户说"学校/组织/学段/组织架构" → school 子命令
用户说"班级/学生/教师/班主任" → class 子命令
用户说"家长/孩子/家庭关系" → family 子命令
用户说"任教/科目" → teacher update-course

## 核心工作流

1. 查询角色 → `school roles`
2. 查看班级列表 → `school class-list`（提取 deptId）
3. 查看班级详情 → `class detail --dept-id <deptId>`
4. 查看学生列表 → `class students --dept-id <deptId>`

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `school class-list` | `deptId` | class 子命令的 --dept-id |
| `class students` | `userId` | update-student/delete-students 的 --student-user-id |
| `class teachers` | `userId` | delete-teacher 的 --teacher-user-id |
