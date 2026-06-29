# 考勤 (attendance) 命令参考

> **【命令合法性 — 必读】** 当前 dws 只提供以下 4 个考勤命令组，其余命令（check / checkin / class / group settings / adjustment / overtime / schedule / approve / selfsetting / globalsetting / report / vacation / boss-check 等）均**不存在**，调用会返回 `unknown command`：
>
> | 命令 | 用途 |
> |------|------|
> | `attendance rules` | 查询考勤组与考勤规则（我属于哪个考勤组、打卡范围、弹性工时） |
> | `attendance record get` | 查询某个人某天的考勤打卡详情 |
> | `attendance shift list` | 批量查询多名员工在某日期范围内的班次 |
> | `attendance summary` | 查询某个人的考勤统计摘要（周/月） |

> **【日期计算规则】** 所有含日期参数的命令均适用，禁止随意猜测日期范围，必须按下表精确计算：
>
> | 用户表达 | 起始（含）| 结束（含）| 说明 |
> |---------|------------|------------|------|
> | 本周 / 这周 | 本周**周一** | 本周**周日** | 周一为一周第一天 |
> | 上周 | 上周**周一** | 上周**周日** | 往前推一周 |
> | 本月 / 这个月 | 本月 **1 日** | 本月**最后一天** | 必须计算当月实际天数 |
> | 上月 | 上月 **1 日** | 上月**最后一天** | 往前推一个月 |
> | 今天 / 昨天 | 当天 | 当天 | start == end |
>
> 计算日期必须参考当前系统时间，不得硬编码或模糊估算；用户给定具体日期范围时直接采用。

## 命令总览

### 查询考勤组与考勤规则
```
Usage:
  dws attendance rules [flags]
Example:
  dws attendance rules --date 2026-03-14
  dws attendance rules --date "2026-03-14 09:00:00"
Flags:
      --date string   考勤日期，格式 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss (必填)
Notes:
  - 用于回答：我属于哪个考勤组、打卡范围是什么、弹性工时怎么算
  - 认证信息（corpId、optUserId）由系统自动注入，无需手动传入
```

### 查询个人考勤详情
```
Usage:
  dws attendance record get [flags]
Example:
  dws attendance record get --user USER_ID --date 2026-03-08
Flags:
      --date string   查询日期，格式 YYYY-MM-DD (必填)
      --user string   钉钉用户 ID (必填)
```

### 批量查询员工班次信息
```
Usage:
  dws attendance shift list [flags]
Example:
  dws attendance shift list --users userId1,userId2 --start 2026-03-03 --end 2026-03-07
Flags:
      --users string   员工 ID 列表，逗号分隔，最多 50 人 (必填)
      --start string   起始日期，格式 YYYY-MM-DD (必填)
      --end string     结束日期，格式 YYYY-MM-DD (必填)
Notes:
  - 单次查询最多 7 天、最多 50 人
```

### 查询某个人的考勤统计摘要
```
Usage:
  dws attendance summary [flags]
Example:
  dws attendance summary --user USER_ID --date "2026-03-12 15:00:00" --stats-type month
  dws attendance summary --user USER_ID --date "2026-03-12 15:00:00" --stats-type week
Flags:
      --user string         钉钉用户 ID (必填)
      --date string         工作日期，格式 yyyy-MM-dd HH:mm:ss (必填)
      --stats-type string   统计类型：week（周统计）或 month（月统计）(必填)
Notes:
  - --stats-type 必填，不填会返回 C0002 统计类型错误（钉钉服务端业务层强制要求）
```

## 意图判断

用户说"我属于哪个考勤组/打卡范围/弹性工时/考勤规则" → `attendance rules --date <日期>`
用户说"某人某天的打卡记录/打卡详情/几点上下班" → `attendance record get --user <userId> --date <日期>`
用户说"某些人的班次/排了什么班/某段时间的班次" → `attendance shift list --users <ids> --start <开始> --end <结束>`
用户说"某人的考勤统计/本周/本月出勤/迟到早退汇总" → `attendance summary --user <userId> --date <日期> --stats-type week|month`

关键区分：
- `record get` = 某天的逐次打卡明细（单人单日）
- `summary` = 一段时间的统计汇总（单人，周/月）
- `shift list` = 排班（谁哪天上什么班，可批量多人）
- `rules` = 考勤组与规则配置（不查打卡数据）

## 核心工作流

```bash
# 查看考勤组和规则
dws attendance rules --date 2026-03-14 --format json

# 查询某人某天的打卡详情（先用 contact 拿 userId）
dws contact user search --query "张三" --format json
dws attendance record get --user <userId> --date 2026-03-08 --format json

# 批量查询多人班次（最多 7 天、50 人）
dws attendance shift list --users userId1,userId2 --start 2026-03-03 --end 2026-03-07 --format json

# 查看某人本月考勤统计摘要
dws attendance summary --user <userId> --date "2026-03-12 15:00:00" --stats-type month --format json
```

## 上下文传递表

| 操作 | 提取 | 用于 |
|------|------|------|
| `contact user search/get-self` | `userId` | record get / summary 的 --user、shift list 的 --users |

## 注意事项

- `--user` / `--users` 需要 userId，可先用 `contact user search`、`contact user get-self` 或 `aisearch person` 获取
- `shift list` 单次最多 7 天、50 人，跨度更大需分批
- `summary` 的 `--stats-type` 必填（week / month），缺省会被钉钉服务端拒绝
- 认证信息（corpId、optUserId）由系统自动注入，无需手动传入
