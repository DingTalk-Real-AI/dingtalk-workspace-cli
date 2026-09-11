# 数据分析

> 本场景所有 recipe 均为 full。

| Recipe | 行动指南（固定路线） |
|--------|-------------------|
| read-aitable | 1. `aitable base search --query "<表格名>"` → 取 `baseId`/`tableId`<br>2. **按结果类型分流**：<br>　　• 原始记录筛选、排序、Top N、预览或字段投影 → `aitable record query --limit 30 [--filters ...] [--sort ...]`<br>　　• 单表直接标量、分组或去重统计 → `aitable record stats` / `aitable record group-stats`<br>　　• 同 Base JOIN、字段间算术、CASE、聚合后派生、汇总结果 Top N 或排名、窗口计算 → 读 `aitable-psql.md`，固定同一 DWS 入口，按 `psql -l` → `psql -t` → `LIMIT 3` 最小查询 → 正式 SQL 在服务端完成<br>　　• 完整逐行明细或逐条业务操作（不做分析）→ 只有明确授权且满足 psql 降级门禁后，`aitable field get` 取必要 `fieldId`，再服务端过滤的 `aitable record query --all --page-limit 0 --field-ids ...`<br>　　• 完整导出 → `aitable export data`<br>3. 使用 record query 时检查 `hasMore/complete/partial`；使用 psql 时检查返回行数与截断提示；不得把任何原始记录用于本地等价分析<br>4. 总结数据 |
| generate-data-report | 1. 原始记录筛选、排序和 Top N 使用 `record query`；单表直接标量、分组或去重统计使用 `record stats` / `record group-stats`；JOIN、字段间算术、CASE、聚合后派生、汇总结果排名和窗口计算使用 `aitable psql` 在服务端完成；禁止 `record query --all` 后以 Python、jq、JavaScript、电子表格或 Agent context 计算<br>2. 按业务主题规划少量查询：同粒度、同关联链合并；不同事实粒度或不同业务主题拆分；关键复杂结论另用服务端 SQL 复核<br>3. 选用 psql 后失败，先按客户端/宿主、网络/认证/权限或 SQL/函数限制分类修复并重试；不得降级为全量记录读取<br>4. 本地工具仅可格式化、绘图或合并少量服务端汇总结果，不能代替服务端计算；随后 `doc create --name "<报告名>" --content "<分析报告>"` |
| create-aitable-record | **批量导入优先**：`python scripts/import_records.py <baseId> <tableId> data.csv\|data.json [batch_size]`（自动分批创建）<br>单条/少量：1. `aitable base search --query "<表格名>"` → 取 `baseId`/`tableId`<br>2. `aitable field get --base-id <baseId> --table-id <tableId>` → 取 `fieldId` 与类型<br>3. `aitable record create --base-id <baseId> --table-id <tableId> --records '[{"cells":{"<fieldId>":"值"}}]'` |
| update-aitable-record | 1. `aitable base search --query "<表格名>"` → 取 `baseId`/`tableId`<br>2. `aitable record query --base-id <baseId> --table-id <tableId>` → 取 `recordId`，**先展示让用户确认**<br>3. `aitable record update --base-id <baseId> --table-id <tableId> --records '[{"recordId":"<recordId>","cells":{...}}]'` |
| search-aitable-template | 1. `aitable template search --query "<关键词>"` → 取 `templateId`<br>2. 用户选定<br>3. `aitable base create --name "<表格名>" --template-id <templateId>` → 取 `baseId` |
