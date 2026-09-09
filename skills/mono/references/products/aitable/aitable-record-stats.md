# record stats / group-stats — 服务端聚合统计

统计任务优先使用服务端聚合，不要先用 `record query --all` 下载全表再计算。

## 命令选择

| 需求 | 命令 | 底层接口 |
|------|------|----------|
| 总数、求和、平均值、最大/最小值、中位数、完整率等标量统计 | `record stats` | `query_records_stats` |
| 按字段分组统计 | `record group-stats` + `--group` | `query_stats` |
| 满足条件的唯一门店/客户/商品数量 | `record group-stats` + `distinct`，不传 `--group` | `query_stats` |

## 不分组统计

```bash
dws aitable record stats \
  --base-id <BASE_ID> \
  --table-id <TABLE_ID> \
  --stats '[{"fieldId":"<FIELD_ID>","statsType":"COUNT"}]' \
  --format json
```

- `statsType` 必须大写。
- `--stats` 单次最多 20 项，同一 `fieldId` 不得重复；同字段多个指标拆成多次调用。
- 支持基础类型 `COUNT`、`COUNT_COLUMN`、`SUM`、`AVG`、`MAX`、`MIN`，以及运行时支持的 `MEDIAN`、`STANDARD_DEVIATION`、`RANGE`、`DISTINCT`、`DISTINCT_RATIO`、完整率、勾选率和日期统计类型。
- 统计全部匹配记录时省略 `--limit`；传入 limit 会改变统计范围。
- 可选参数：`--filters`、`--sort`、`--keyword`、`--search-field-ids`、`--data-version`。

## 分组或去重统计

```bash
dws aitable record group-stats \
  --base-id <BASE_ID> \
  --table-id <TABLE_ID> \
  --group '[{"fieldId":"<GROUP_FIELD_ID>","direction":"ASC","fieldConfig":null,"arraySplitMode":true}]' \
  --stats '[{"fieldId":"<VALUE_FIELD_ID>","statsType":"avg"}]' \
  --limit 1000 \
  --format json
```

- `statsType` 必须小写；基础类型为 `sum`、`avg`、`count`、`max`、`min`，后端还可能支持 `median`、`distinct`、`distinct_ratio` 等高级类型。
- `--group` 和 `--sort` 都是 JSON 数组编码后的字符串，CLI 会原样映射到 MCP 的 `group` / `sortDsl`。
- 分组结果最多 1000 行。不要依赖服务端 limit 选择 Top N；应在基数不超过 1000 时取完整分组结果后排序。
- 条件唯一实体计数不传 `--group`，对实体字段使用 `distinct`。

## 过滤条件

```bash
--filters '{"operator":"and","operands":[{"operator":"gt","operands":["fldAmount",0]}]}'
```

- 根节点必须是 `and` / `or`。
- `lt`、`gt`、`lte`、`gte` 的值必须是 JSON 数字，不能写成数字字符串。
- 单选/多选字段建议使用 `field get` 返回的 option ID。
- 所有 Base、table、field 和 option ID 都必须从当前目标 Base 的实时元数据取得，不能复用示例或历史 ID。

## 与 record query / psql 的正反边界

| 用户请求 | 应选 | 不应选 / 原因 |
|---|---|---|
| “本月订单总金额”“平均客单金额”“最大单笔金额” | `record stats` | 单表直接标量聚合；禁止 `record query --all` 后本地计算。 |
| “各状态分别多少条”“满足条件的唯一门店数” | `record group-stats` | 单表直接分组或去重；不需要 SQL。 |
| “金额最高的 10 条记录” | `record query --sort ... --limit 10` | 排序对象是原始记录，不是聚合结果。 |
| “销售额最高的 10 个门店并排名” | `psql` | 需要先分组聚合，再对汇总结果排序或排名；不能依赖 group-stats 的 limit 产生 Top N。 |
| “各门店销售额占总额比例”“关联订单表与门店表统计城市销售额” | `psql` | 涉及聚合后派生或同 Base 多表 JOIN。 |

## 降级边界

原生统计不能直接表达时，先判断是否可由 `psql` 完成。只有用户明确要求记录明细或少量校验样本，才使用 `record query`；聚合接口明确失败不构成直接全量拉取的理由。需要先逐行运算再聚合的指标优先由 `psql` 在服务端计算；若 PSQL 也无法表达，停止并说明具体限制，不得遍历全表后在 Agent、脚本或电子表格中二次计算。
