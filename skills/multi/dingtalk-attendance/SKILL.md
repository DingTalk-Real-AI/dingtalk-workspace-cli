---
name: dingtalk-attendance
description: 钉钉考勤。Use when 用户说 考勤/打卡记录/查打卡/查班次/考勤汇总/考勤规则/出勤情况/请假加班补卡/排班/考勤组/假期余额。命令前缀：dws attendance。支持查询与写操作（班次、排班、考勤组、个人规则设置、假期规则/余额等），写操作需二次确认后再加 --yes。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉考勤 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[attendance.md](references/attendance.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查我 / 某人 某天的打卡" | `dws attendance record get --user <userId> --date <YYYY-MM-DD>` |
| "查考勤组 / 考勤规则 / 打卡范围" | `dws attendance rules --date <YYYY-MM-DD>` |
| "查班次 / 排班 / 谁今天上什么班" | `dws attendance shift list --users <userId1,userId2> --start <YYYY-MM-DD> --end <YYYY-MM-DD>` |
| "考勤统计 / 周月汇总 / 出勤天数" | `dws attendance summary --user <userId> --date <YYYY-MM-DD> --stats-type week\|month` |
| "请假 / 加班 / 补卡 / 出差 记录或提交链接" | `dws attendance approve list` / `dws attendance approve templates --type <类型>` |
| "班次 / 补卡规则 / 加班规则 / 考勤组 查询与修改" | `class` / `adjustment` / `overtime` / `group` 子命令 |
| "排班 / 假期规则 / 假期余额 / 签到 / 报表" | `schedule` / `vacation` / `checkin` / `report` 子命令 |

> 完整命令集（含写操作与参数）见 [references/attendance.md](references/attendance.md)。

## 评测高频硬约束

- 当前 dws 已注册全部考勤子命令组（`record` / `check` / `approve` / `shift` / `schedule` / `class` / `adjustment` / `overtime` / `group` / `summary` / `rules` / `selfsetting` / `globalsetting` / `vacation` / `checkin` / `report` / `boss-check`），查询与写操作大多可直接调用后端。**不要再以"开源版只读/不支持写操作"为由拒答。** 创建班次、导入排班、加人入考勤组、保存个人规则设置、设置假期余额等写操作可执行，但必须先展示参数摘要并二次确认，再追加 `--yes`（或 `--user-say-yes`）执行；不要在未确认时直接写、也不要伪装成功。个别命令受权限/数据影响返回空或权限错误（如 `report` 系列仅管理员），如实说明即可。
- 查询迟到/缺勤名单时，空打卡结果不等于"没人迟到"。必须结合排班、`NotSigned`、`Absenteeism`、无记录人员分别说明；数据缺失要标为"无记录/无法判断"，不要归为正常。
- 做部门 Top N 排名时，用户要求前 N 名就必须输出 N 个部门；无打卡记录或无可计算数据的部门按 0 或"无数据"保留在排名中，不能只输出有数据的少数部门。
- `summary` 必须同时传 `--user`、`--date`、`--stats-type`（week/month），缺一返回 C0002。
- `shift list` 时间跨度不超过 7 天，最多 50 人；`record get` 一次只查一个用户一天。
- 所有 dws 命令带 `--format json`，时间/日期按命令要求分别使用 `YYYY-MM-DD` 或 `yyyy-MM-dd HH:mm:ss`。

## 跨产品协作

- 拿到 userId 前先用 `dingtalk-aisearch` 解析人名
