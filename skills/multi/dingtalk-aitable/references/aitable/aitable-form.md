# 表单视图 — 使用 view(FormDesigner)

悟空命令面不暴露独立表单命令组。表单在 AI 表格里按 `viewType=FormDesigner` 的视图处理，所有生成命令都走 `view` 和 `field`。

## 命令路线

| 诉求 | 使用命令 | 说明 |
|------|----------|------|
| 列出表单视图 | `dws aitable view list --base-id <BASE_ID> --table-id <TABLE_ID>` | 从返回视图中过滤 `viewType=FormDesigner` |
| 查看表单视图 | `dws aitable view get --base-id <BASE_ID> --table-id <TABLE_ID> --view-ids <VIEW_ID>` | 按 viewId 获取 |
| 创建表单视图 | `dws aitable view create --base-id <BASE_ID> --table-id <TABLE_ID> --view-type FormDesigner --name "表单名"` | 返回 `viewId` |
| 更新表单视图 | `dws aitable view update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --name "新名称"` | 描述用 `--desc` JSON |
| 删除表单视图 | `dws aitable view delete --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --yes` | 不可逆 |
| 添加题目 | `dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> --fields '[...]'` | 题目本质是字段 |
| 删除题目 | `dws aitable field delete --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --yes` | 不可逆 |

## 创建工作流

```bash
# 1. 创建表单视图
dws aitable view create --base-id BASE_ID --table-id TABLE_ID \
  --view-type FormDesigner --name "员工信息收集" --format json

# 2. 添加题目字段
dws aitable field create --base-id BASE_ID --table-id TABLE_ID \
  --fields '[{"fieldName":"姓名","type":"text"},{"fieldName":"邮箱","type":"text"}]' --format json

# 3. 回读表单视图
dws aitable view get --base-id BASE_ID --table-id TABLE_ID --view-ids VIEW_ID --format json
```

## 注意

- 不要生成隐藏兼容的独立表单命令；它不属于悟空对齐的公开命令面。
- `VIEW_ID` 来自 `view create` 返回。
- `FIELD_ID` 来自 `field create` 或 `field get` 返回。
- 字段必填、字段隐藏、分享开关等表单高级配置没有公开的悟空命令入口；不要用隐藏兼容命令替代。
