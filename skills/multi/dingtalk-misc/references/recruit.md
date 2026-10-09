# 钉钉招聘

`dws recruit` 查询和创建钉钉招聘职位。当前公开命令只覆盖职位列表、职位详情和
职位创建；候选人、面试、Offer、职位修改、开放或关闭尚未作为稳定命令发布。

所有命令都应加 `--format json`。组织、业务标识和当前操作人由登录身份及 MCP
Connector 注入，不要要求用户提供或猜测 `corpId`、`bizCode`、`opUserId`。

## 查询职位列表

```bash
dws recruit job list --format json
dws recruit job list --keyword "Java" --status open --size 20 --format json
dws recruit job list --job-ids JOB_ID_1,JOB_ID_2 --format json
```

可选筛选包括 `--job-ids`、`--required-edu`、`--status`、`--job-nature`、
`--campus`、`--start-modified-time`、`--end-modified-time`、
`--creator-user-ids`、`--keyword`、`--category`。状态接受：

- `draft`：草稿
- `open`：招聘中
- `invalid`：已失效
- `closed`：已关闭/完成

`--size` 默认 20，范围 1–100。首页不传 `--cursor`；响应 `hasMore=true` 时，
把 `nextCursor` 回填到下一次查询。

## 查询职位详情

先从列表结果取得真实 `jobId`，不得编造：

```bash
dws recruit job get --job-id JOB_ID --format json
```

## 创建职位

创建是非幂等远端写入。先准备只包含职位对象的 UTF-8 JSON 文件，并用
`--dry-run` 检查完整调用；向用户展示职位名称、性质、薪资、创建人和负责人（如有），取得
明确确认后，交由 CLI 的确认流程执行实际创建。不要在存储示例中加入确认绕过参数。

`creatorUserId` 是必填字段，必须使用同一 profile 下当前操作者的真实 userId，不得
猜测或复用其他组织的 userId。`ownerUserIds` 是可选的负责人 userId JSON 字符串数组；
提供负责人时同样必须使用真实通讯录结果。用户说“负责人是我”时先运行
`dws contact user get-self --format json` 取得当前 userId；用户指定其他负责人时按
通讯录 Skill 解析唯一 userId。未指定负责人时允许省略 `ownerUserIds`，不要默认选人。

```bash
dws recruit job create --from ./job.json --dry-run --format json
dws recruit job create --from ./job.json --format json
```

最小 `job.json`：

```json
{
  "name": "Java 开发工程师",
  "description": "负责服务端系统开发",
  "jobNature": "FULL-TIME",
  "requiredEdu": 6,
  "minSalary": 20000,
  "maxSalary": 35000,
  "extData": {
    "headCount": 1,
    "fullTimeExtData": {
      "salaryMonth": 12
    }
  },
  "creatorUserId": "CURRENT_USER_ID",
  "ownerUserIds": ["OWNER_USER_ID"]
}
```

必填字段为 `name`、`description`、`jobNature`、`requiredEdu`、`extData`、
`creatorUserId`；`ownerUserIds` 可选。
`jobNature` 当前固定为 `FULL-TIME`；`requiredEdu` 为 1–9 的整数（1小学、2初中、
3高中、4中专、5大专、6本科、7硕士、8博士、9其他）。`minSalary` 与
`maxSalary` 可选；两者同时提供时最低薪资不得高于最高薪资。

`extData.headCount` 范围 1–999；`extData.fullTimeExtData.salaryMonth` 范围
12–24；最高工作年限不得小于最低工作年限。提供 `address` 时必须同时包含
`name`、`detail`、`longitude`、`latitude`，经纬度只能来自地图选点或可信地址解析，
不得猜测。`source` 可省略，由 Connector 默认补充为 `manual`；不传 `category` 时
Connector 会设置 `checkJobCategory=false`。

可选字段以当前 `dws recruit job create --help` 和 leaf Schema 为准。不要把 MCP
信封字段 `atsAddJobParam`、`corpId`、`bizCode` 或 `opUserId` 写进文件；CLI 与
Connector 会负责包装和身份注入。
