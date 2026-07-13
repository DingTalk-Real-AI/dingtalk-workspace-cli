# 透视表管理

## 使用场景

- 用户要按部门、月份等维度汇总数据 → `pivot-table create`
- 用户要查看现有透视表或取得 `pivotTableId` → `pivot-table list`
- 用户要调整行列、聚合方式或显示选项 → `pivot-table update`
- 用户明确要求删除透视表 → `pivot-table delete`（不可逆，必须先确认）

透视表是原生对象，会随源数据更新；不要用一组普通汇总公式冒充透视表。

## 列出与查看

```bash
dws sheet pivot-table list --node <NODE_ID> --sheet-id <SHEET_ID>
dws sheet pivot-table list --node <NODE_ID> --sheet-id <SHEET_ID> --pivot-table-id <PIVOT_TABLE_ID>
```

不传 `--pivot-table-id` 时列出工作表中的全部透视表；传入时返回单个透视表详情。

## 创建

```bash
dws sheet pivot-table create --node <NODE_ID> \
  --source "'Data'!A1:D100" \
  --properties '{
    "rows":[{"field":"部门"}],
    "columns":[{"field":"月份"}],
    "values":[{"field":"销售额","summarize_by":"sum"}],
    "show_row_grand_total":true
  }'
```

可选参数：

- `--target-sheet-id`：将透视表放到指定工作表；不传时由服务端创建目标工作表。
- `--target-position`：目标起始单元格，例如 `B2`。
- `--properties @pivot.json`：从文件读取配置。

`--source` 必须包含工作表前缀并覆盖表头和数据，例如 `"'Data'!A1:D100"`。创建时 `properties.values` 必须是非空数组。

## properties 结构

- `rows` / `columns` / `filters`：字段数组，每项至少包含非空 `field`。
- `values`：聚合字段数组，每项至少包含 `field`；创建时至少一项。
- `summarize_by`：`sum`、`count`、`average`、`max`、`min`、`product`、`count_numbers`、`std_dev`、`std_dev_p`、`var`、`var_p`、`distinct`、`median`。
- `collapse`：字段折叠配置，可为对象或数组。
- 显示选项：`show_row_grand_total`、`show_col_grand_total`、`show_subtotals`、`repeat_row_labels`。

## 更新

```bash
dws sheet pivot-table update --node <NODE_ID> --sheet-id <SHEET_ID> \
  --pivot-table-id <PIVOT_TABLE_ID> \
  --properties '{"show_subtotals":false}'
```

Open 接口支持部分更新，因此更新显示选项时不需要重复传入 `values`。更新行列或聚合字段时，应先用 `list --pivot-table-id` 读取当前配置，再传入需要变更的字段。

## 删除

```bash
dws sheet pivot-table delete --node <NODE_ID> --sheet-id <SHEET_ID> \
  --pivot-table-id <PIVOT_TABLE_ID> --yes
```

删除不可恢复。Agent 必须先展示目标文档、工作表和 `pivotTableId`，获得用户明确同意后才能附加 `--yes`。
