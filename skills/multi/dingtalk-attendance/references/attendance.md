# 考勤 (attendance) 命令参考

开源 `dws attendance` 仅支持以下 4 个只读子命令：`record / rules / shift / summary`。

> 写操作（创建班次、导入排班、修改考勤组成员/配置、保存个人规则、更新假期规则、设置假期余额等）在开源版均**不支持**。用户提到这类需求时，直接告知不支持，不要尝试拼命令。

## 命令总览

### 查询个人考勤详情
```
Usage:
  dws attendance record get [flags]
Example:
  dws attendance record get --user USER_ID --date 2026-03-08
Flags:
      --date string   查询日期, 格式 YYYY-MM-DD (必填)
      --user string   钉钉用户 ID (必填)
```

一次只查一个用户一天的考勤详情。

### 查询考勤组与考勤规则
```
Usage:
  dws attendance rules [flags]
Example:
  dws attendance rules --date 2026-03-14
  dws attendance rules --date "2026-03-14 09:00:00"
Flags:
      --date string   考勤日期, 格式 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss (必填)
```

查询当前用户所属的考勤组、打卡规则、弹性工时设置等。

### 批量查询员工班次
```
Usage:
  dws attendance shift list [flags]
Example:
  dws attendance shift list --users userId1,userId2 --start 2026-03-03 --end 2026-03-07
Flags:
      --end string     结束日期, 格式 YYYY-MM-DD (必填)
      --start string   开始日期, 格式 YYYY-MM-DD (必填)
      --users string   用户 ID 列表, 逗号分隔, 最多 50 人 (必填)
```

批量查询多个员工在指定日期的班次信息。**时间跨度不超过 7 天，最多 50 人**。

### 查询某人的考勤统计摘要
```
Usage:
  dws attendance summary [flags]
Example:
  dws attendance summary --user USER_ID --date "2026-03-12 15:00:00" --stats-type month
  dws attendance summary --user USER_ID --date "2026-03-12 15:00:00" --stats-type week
Flags:
      --date string         工作日期, 格式 yyyy-MM-dd HH:mm:ss (必填)
      --stats-type string   统计类型: week / month (必填, 钉钉服务端业务层强制要求, 不填返回 C0002)
      --user string         钉钉用户 ID (必填)
```

`--user`、`--date`、`--stats-type` 均必填。

## 意图判断

用户说"打卡记录/某天考勤/某人今天打卡了吗" → `record get`
用户说"考勤组/考勤规则/打卡范围/弹性工时" → `rules`
用户说"班次/排班/今天上什么班/这周几点上下班" → `shift list`
用户说"考勤汇总/周统计/月统计/出勤天数/迟到次数" → `summary`

## 核心工作流

```bash
# 查个人某天的考勤详情
dws attendance record get --user <USER_ID> --date 2026-03-08 --format json

# 查考勤组和规则
dws attendance rules --date 2026-03-14 --format json

# 批量查班次（最多 50 人 / 7 天）
dws attendance shift list --users user001,user002 --start 2026-03-03 --end 2026-03-07 --format json

# 查月度考勤汇总
dws attendance summary --user <USER_ID> --date "2026-03-12 15:00:00" --stats-type month --format json

# 查周度考勤汇总
dws attendance summary --user <USER_ID> --date "2026-03-12 15:00:00" --stats-type week --format json
```

## 上下文传递表
| 操作 | 提取 | 用于 |
|------|------|------|
| `contact user get-self` / `contact user search` | `userId` | record get / shift list / summary 的 `--user` / `--users` |
| `rules` | `groupId` | 进一步说明考勤组归属 |

## 注意事项

- `record get` 的 `--date` 格式: `YYYY-MM-DD`，CLI 自动转换为毫秒时间戳
- `rules` 的 `--date` 支持 `YYYY-MM-DD` 或 `yyyy-MM-dd HH:mm:ss` 两种格式
- `shift list` 的 `--start/--end` 使用 `YYYY-MM-DD` 格式，跨度不超过 7 天；`--users` 最多 50 人
- `summary` 的 `--date` 必须是 `yyyy-MM-dd HH:mm:ss` 格式；`--stats-type` 仅接受 `week` 或 `month`
- 所有命令必须显式带 `--format json` 便于解析
- 用户 ID 需先从 `contact user get-self` 或 `aisearch person` 获取，不要凭记忆复用

## 严格约束

- 不要凭历史记忆复用 userId / groupId 等任何 ID，每次必须从当次命令返回值中提取
- 不要猜测命令，先查 `--help` 明确命令
- 查询迟到/缺勤名单时，空打卡结果不等于"没人迟到"，必须结合 `NotSigned`、`Absenteeism`、无记录人员分别说明
- 涉及超过 3 条记录的聚合（求和、分组、计数、排序）时必须落 Python 脚本处理，禁止口算或目测
- 遇到时长字段时，注意区分单位是秒、分钟还是小时
- 写操作（创建班次、导入排班、修改考勤组、设置假期余额等）开源版不支持，遇到这类需求直接告知，不要伪装成功
