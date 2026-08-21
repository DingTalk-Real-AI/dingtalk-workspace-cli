# 钉钉教育

> **Draft** — 教育能力尚未完整迁入开源 DWS Binary，当前文档为接入预占。
> Binary 命令就绪后补充命令 Schema、示例和安全规则。

`dws education` 覆盖钉钉教育组织、班级和学生管理，包括组织信息查询、
班级管理、学生管理和家校消息等场景。

所有命令都应加 `--format json`。组织标识由登录身份及 MCP Connector
注入，不要要求用户提供或猜测 `corpId`。

## Use when

- 用户提到教育组织、学校、班级、学生、家长、老师
- 用户需要查询或管理班级成员、学生花名册
- 用户需要发送家校通知或家校消息
- 用户提到班级圈、成长记录

## Avoid when

- 用户操作通讯录中非教育组织的部门或成员（使用 `dws contact`）
- 用户需要发送普通群聊消息（使用 `dws chat`）
- 用户需要管理待办（使用 `dws todo`）

## 命令总览

> 以下为预期命令面，Binary 实现完成后以 `dws education --help` 为准。

| 命令 | 用途 | 必填参数 | 备注 |
|---|---|---|---|
| `dws education org get` | 查询教育组织信息 | — | 只读 |
| `dws education class list` | 列出班级 | `--org-id` | 只读 |
| `dws education class get` | 查询班级详情 | `--class-id` | 只读 |
| `dws education student list` | 列出学生花名册 | `--class-id` | 只读 |
| `dws education student get` | 查询学生详情 | `--student-id` | 只读 |

## 意图判断

- "查一下 XX 班级" / "班级列表" → `dws education class list`
- "学生花名册" / "XX 班的学生" → `dws education student list`
- "XX 学生的信息" → `dws education student get`
- "教育组织" / "学校信息" → `dws education org get`

## 安全规则

- 所有查询命令加 `--format json`
- 组织、业务标识和当前操作人由登录身份注入，不要猜测 `corpId`、`opUserId`
- 学生个人信息属于敏感数据，输出时避免在日志中打印完整手机号和身份证号
- 家校消息发送为写操作，需要用户明确确认

## 跨产品协作

- 需要发送群消息通知时，使用 `dws chat`（见本包 [chat.md](../../dingtalk-chat/references/chat.md)）
- 需要创建待办任务时，使用 `dws todo`
- 需要查询通讯录中非教育成员时，使用 `dws contact`
