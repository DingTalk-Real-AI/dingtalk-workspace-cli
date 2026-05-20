# 家校应用 (edu-app) 命令参考

## 命令总览

### message (消息管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `message summary-list` | 查询消息摘要列表 | `--class-id`, `--cid`, `--target-role`, `--status` |

> `--target-role`: guardian(家长) / student(学生)
> `--status`: 0(未处理) / 1(已处理)

### task (任务管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `task publish-list` | 查询发布的家校任务列表（仅老师） | 无（均可选） |
| `task all-list` | 查询全部家校任务列表（仅老师） | `--biz-id`(班级ID) |
| `task student-list` | 查询学生待办任务列表 | `--students`(JSON数组) |

> `--task-sources` 可选值（逗号分隔）: EDU_HOMEWORK, EDU_CARD, EDU_NOTICE, EDU_SR, EDU_DIPLOMA

### report (成绩单管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `report get` | 获取成绩单列表 | `--ids`(逗号分隔整数) |
| `report by-teacher` | 查询老师创建的成绩单 | 无（均可选） |
| `report by-class` | 查询班级学生成绩明细 | `--report-id`, `--class-id` |
| `report by-student-list` | 查询学生收到的成绩单 | `--class-id`, `--student-id` |
| `report by-student-detail` | 查询学生成绩明细 | `--report-id`, `--student-id`, `--class-id` |

### notice (通知管理)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `notice confirm` | 确认收到通知 | `--notice-id`, `--student-id` |

### circle (班级圈)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `circle posts` | 查询学生班级圈动态 | `--class-id`, `--student-id`, `--target-role` |

> `--target-role`: guardian(家长视角) / student(学生视角)
> 返回动态的文字内容、图片URL列表、发布者姓名、发布时间、评论数、点赞数等。

## 意图判断

用户说"消息/消息摘要" → message summary-list
用户说"作业/任务/打卡/家校任务" → task 子命令
用户说"成绩/成绩单" → report 子命令
用户说"通知/确认通知" → notice confirm
用户说"班级圈/成长记录/学生动态" → circle posts

关键区分: task(家校任务/作业) vs report(成绩单)
关键区分: circle(班级圈动态/成长记录) vs message(AI消息总结)

## 核心工作流

### 老师场景
1. 查看发布的任务 → `task publish-list --need-statistic -f json`
2. 查看某班全部任务 → `task all-list --biz-id <classId>`
3. 查看成绩单 → `report by-teacher --status 1`
4. 查看班级成绩明细 → `report by-class --report-id <id> --class-id <classId>`

### 家长场景
1. 查看孩子待办 → `task student-list --students '[{"userId":"<uid>","bizId":"<classId>"}]'`
2. 确认通知 → `notice confirm --notice-id <id> --student-id <uid>`
3. 查看孩子班级圈动态 → `circle posts --class-id <classId> --student-id <studentId> --target-role guardian`

### 学生场景
1. 查看自己的班级圈动态 → `circle posts --class-id <classId> --student-id <studentId> --target-role student`

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `edu-contact school class-list` | `deptId` | task all-list 的 --biz-id |
| `edu-contact class students` | `userId` | task student-list 的 students.userId |
| `edu-group class-group conversation-id` | `conversationId` | message summary-list 的 --cid |
| `report by-teacher` | `schoolReportId` | report get/by-class/by-student-detail 的 --report-id |
| `edu-contact family children` | `studentUserId`, `classId` | circle posts 的 --student-id, --class-id |
