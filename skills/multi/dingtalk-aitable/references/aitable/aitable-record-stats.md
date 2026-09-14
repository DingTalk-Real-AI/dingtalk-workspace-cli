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
  --stats '[{"fieldId":"<VALUE_FIELD_ID>","statsType":"AVG"}]' \
  --limit 1000 \
  --format json
```

- `statsType` 与 `record stats` 统一使用大写枚举，支持 `SUM`、`AVG`、`COUNT`、`MEDIAN`、`DISTINCT`、`DISTINCT_RATIO` 等类型。
- `--group` 和 `--sort` 都是 JSON 数组编码后的字符串；`--sort` 的 fieldId 必须同时出现在 `--group` 中。
- 分组结果最多 1000 行；省略 `--limit` 时使用服务端默认上限 1000，服务端在聚合和排序完成后应用限制。
- 条件唯一实体计数不传 `--group`，对实体字段使用 `DISTINCT`。

## 过滤条件

```bash
--filters '{"operator":"and","operands":[{"operator":"gt","operands":["fldAmount",0]}]}'
```

- 根节点必须是 `and` / `or`。
- `lt`、`gt`、`lte`、`gte` 的值必须是 JSON 数字，不能写成数字字符串。
- 单选/多选字段建议使用 `field get` 返回的 option ID。
- 所有 Base、table、field 和 option ID 都必须从当前目标 Base 的实时元数据取得，不能复用示例或历史 ID。

## 降级边界

只有用户要求记录明细、少量校验样本、精确分位数输入，或聚合接口明确失败时，才使用 `record query`。需要先逐行运算再聚合的指标必须依赖表内已有且可直接聚合的公式字段；没有该字段时停止并请用户先在 AI 表格页面创建，不能遍历全表本地二次计算。
