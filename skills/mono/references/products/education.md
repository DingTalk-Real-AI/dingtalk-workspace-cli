# 钉钉教育

> **Draft** — 教育能力尚未完整迁入开源 DWS Binary，当前文档为接入预占。

`dws education` 覆盖钉钉教育组织、班级和学生管理。

## Use when

- 用户提到教育组织、学校、班级、学生、家长、老师
- 用户需要查询或管理班级成员、学生花名册
- 用户需要发送家校通知或家校消息

## Avoid when

- 用户操作非教育组织的通讯录（使用 `dws contact`）
- 用户发送普通群聊消息（使用 `dws chat`）

## 命令总览

> 以下为预期命令面，Binary 实现完成后以 `dws education --help` 为准。

| 命令 | 用途 | 备注 |
|---|---|---|
| `dws education org get` | 查询教育组织信息 | 只读 |
| `dws education class list` | 列出班级 | 只读 |
| `dws education class get` | 查询班级详情 | 只读 |
| `dws education student list` | 列出学生花名册 | 只读 |
| `dws education student get` | 查询学生详情 | 只读 |

## 安全规则

- 学生个人信息属于敏感数据，避免在日志中打印完整手机号和身份证号
- 家校消息发送为写操作，需要用户明确确认
