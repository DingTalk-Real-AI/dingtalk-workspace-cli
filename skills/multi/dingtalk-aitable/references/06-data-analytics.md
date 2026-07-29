# 数据分析

> 本场景所有 recipe 均为 full。

| Recipe | 行动指南（固定路线） |
|--------|-------------------|
| read-aitable | 1. `aitable base search --query "<表格名>"` → 取 `baseId`/`tableId`<br>2. `aitable field get --base-id <baseId> --table-id <tableId>` → 取 `fieldId`<br>3. `aitable record query --base-id <baseId> --table-id <tableId>` → 取记录（分页）<br>　　需要筛选时 `--filters` 格式见 [aitable-filter-sort.md](./aitable/aitable-filter-sort.md)，根节点必须是 `{"operator":"and\|or","operands":[...]}`<br>4. 总结数据 |
| generate-data-report | 1. 同 read-aitable 步骤 1-3<br>2. 按[「多源并行采集」](_common/conventions.md#多源并行采集公共模式)执行 → 补充背景<br>3. `doc create --name "<报告名>" --content "<分析报告>"` |
| create-aitable-record | **批量导入优先**：`python scripts/import_records.py <baseId> <tableId> data.csv\|data.json [batch_size]`（自动分批创建）<br>单条/少量：1. `aitable base search --query "<表格名>"` → 取 `baseId`/`tableId`<br>2. `aitable field get --base-id <baseId> --table-id <tableId>` → 取 `fieldId` 与类型<br>3. `aitable record create --base-id <baseId> --table-id <tableId> --records '[{"cells":{"<fieldId>":"值"}}]'` |
| update-aitable-record | 1. `aitable base search --query "<表格名>"` → 取 `baseId`/`tableId`<br>2. `aitable record query --base-id <baseId> --table-id <tableId>` → 取 `recordId`，**先展示让用户确认**<br>3. `aitable record update --base-id <baseId> --table-id <tableId> --records '[{"recordId":"<recordId>","cells":{...}}]'` |
| search-aitable-template | 1. `aitable template search --query "<关键词>"` → 取 `templateId`<br>2. 用户选定<br>3. `aitable base create --name "<表格名>" --template-id <templateId>` → 取 `baseId` |
