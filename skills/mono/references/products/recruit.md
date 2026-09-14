# 钉钉招聘

`dws recruit` 提供招聘职位列表、职位详情和职位创建能力。组织、业务标识和当前
操作人由登录身份及 MCP Connector 注入，不要要求用户提供或猜测身份字段。

## 查询

```bash
dws recruit job list --keyword "Java" --status open --size 20 --format json
dws recruit job get --job-id JOB_ID --format json
```

列表状态支持 `draft`、`open`、`invalid`、`closed`。首页不传 `--cursor`；
`hasMore=true` 时把响应 `nextCursor` 回填。查询详情前必须从真实列表结果取得
`jobId`。

## 创建

创建是非幂等写操作。先用 `--dry-run` 检查；实际执行必须经过用户明确确认，并
交由 CLI 的确认流程处理：

```bash
dws recruit job create --from ./job.json --dry-run --format json
dws recruit job create --from ./job.json --format json
```

JSON 必填字段为 `name`、`description`、`jobNature`、`requiredEdu`、`extData`、
`creatorUserId`。
`jobNature` 当前固定为 `FULL-TIME`；学历枚举为 1小学、2初中、3高中、4中专、
5大专、6本科、7硕士、8博士、9其他。`minSalary`、`maxSalary` 可选；两者同时
提供时最低薪资不得高于最高薪资。

`creatorUserId` 是当前创建人 userId，必须来自同一 profile 下的真实通讯录结果。
`ownerUserIds` 可选；提供时是负责人 userId 的 JSON 字符串数组，也必须使用真实
通讯录结果。禁止猜测任何 userId。`extData.headCount` 范围 1–999，薪资月份范围
12–24；提供地址时必须包含地点名、详细地址和可信的经纬度。

文件只写职位对象，不要添加 MCP 信封字段 `atsAddJobParam`、`corpId`、`bizCode`
或 `opUserId`；这些字段由 CLI 或 Connector 处理。
