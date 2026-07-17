# 意图判断

## 意图判断

用户说"表格/多维表/AI表格":
- 查看/查找/列表 → `base search`（优先）或 `base list`（仅浏览最近访问）
- 详情 → `base get`
- 创建 → `base create`
- 修改 → `base update`
- 删除 → `base delete`

用户说"数据表/子表/table":
- 查看 → `table get`
- 创建 → `table create`
- 重命名 / 改备注 / 改行命名规则 → `table update`（三选一：`--name` / `--description` / `--record-name-key`）
- 用户说"行命名规则/记录别名/卡片显示成 task/project/event 这种" → `table update --record-name-key <枚举键>`，**中文 → 枚举键**对照见 [aitable-record-name-key.md](./aitable-record-name-key.md)
- 删除 → `table delete`

用户说"字段/列/column":
- 查看 → `field get`
- 添加 → `field create`（读 [aitable-field.md](./aitable-field.md)）
- 修改 → `field update`
- 删除 → `field delete`

用户说"记录/行/数据/row":
- 查看/搜索 → `record query`（读 [aitable-record-query.md](./aitable-record-query.md)）
- 找空行 / 没填东西的行 → `record query-empty`（读 [aitable-record-query.md](./aitable-record-query.md)）
- 已知 recordId 反查字段值 → `record get`（按 ID 取专用，等价 `record query --record-ids`）
- 添加/写入 → `record create`（读 [aitable-record-create.md](./aitable-record-create.md)）
- 修改/更新（每条独立 cells） → `record update`（读 [aitable-record-update.md](./aitable-record-update.md)）
- **批量更新同一字段值**（统一标记/统一改值） → `record batch-update --record-ids ... --cells '{...}'`
- 删除 → `record delete`
- **查记录的字段变更历史 / 操作审计** → `record history-list`（读 [aitable-record-history.md](./aitable-record-history.md)）
- **取记录分享链接 / 把这行发给同事** → `record share-url`（读 [aitable-record-share.md](./aitable-record-share.md)）
- **不知道有没有 → 有就改、没有就建** → `record upsert`（读 [aitable-record-upsert.md](./aitable-record-upsert.md)）

用户说"视图/view":
- 列出/查看全部视图 → `view list`（或 `view get` 不传 --view-ids，二者等价）
- 看某个视图详情 → `view get --view-ids <ID>`
- 创建 → `view create`
- 修改（含"调整字段顺序/隐藏字段"） → `view update --config '{"visibleFieldIds":[...]}'`
- 修改某一项配置（filter/sort/group/card/timebar/aggregate 等）→ `view update <attr>`（读 [aitable-view-config.md](./aitable-view-config.md)）
- 锁定 / 冻结列 / 行高 / 数据高亮规则 / 复制视图 → 读 [aitable-view-extras.md](./aitable-view-extras.md)
- 删除 → `view delete`

用户说"锁定视图/解锁视图/lock view" → `view lock` / `view lock --off`，详见 [aitable-view-extras.md](./aitable-view-extras.md)

用户说"冻结列/冻结首列/frozen columns" → `view update frozen-cols --count N`，详见 [aitable-view-extras.md](./aitable-view-extras.md)

用户说"行高/单元格高度/紧凑模式/cell height" → `view update row-height --cell-height N`（合法档位 32/56/88/128），详见 [aitable-view-extras.md](./aitable-view-extras.md)

用户说"数据高亮/条件格式/单元格上色/fill color rule" → `view update fill-color-rule --json '[...]'`，详见 [aitable-view-extras.md](./aitable-view-extras.md)

用户说"复制视图/duplicate view" → `view duplicate --view-id ... [--new-name ...]`，详见 [aitable-view-extras.md](./aitable-view-extras.md)

用户说"筛选/过滤/filter" → 读 [aitable-filter-sort.md](./aitable-filter-sort.md)

用户说"统计/分析/聚合/TOP N/全量" → 读 [aitable-data-analysis-sop.md](./aitable-data-analysis-sop.md)

用户说"公式/formula/计算字段/派生指标" → 读 [aitable-formula-guide.md](./aitable-formula-guide.md)

用户说"查找引用/lookup/filterUp/跨表" → 读 [aitable-formula-guide.md](./aitable-formula-guide.md)（§5.4 跨表引用）

用户说"表单/form/收集表/问卷/催办填写" → 读 [aitable-form.md](./aitable-form.md)

用户说"自动化/工作流/流程/触发/automation/workflow" → 读 [aitable-workflow.md](./aitable-workflow.md)
- 新建并发布流程 → `workflow create --base-id <BASE_ID> --dsl @workflow.json`
- 修改并发布已有流程 → 先 `workflow get` 留底，再 `workflow update --base-id <BASE_ID> --workflow-id <WORKFLOW_ID> --dsl @workflow.json`
- 看 Base 里有哪些流程 / 哪些在跑 → `workflow list`（看 `recordCount` / `runningCount`）
- 看某个流程具体配置（触发条件、动作步骤） → `workflow get`
- 启用流程 → `workflow enable`
- 临时停掉流程（调试 / 数据迁移）→ `workflow disable --yes`
- **删除流程**：当前 CLI 暂不支持，引导用户到 AI 表格 Web 端 → 数据表 → 自动化面板手动完成

用户说"仪表盘/图表/chart" → 读 [aitable-dashboard-chart.md](./aitable-dashboard-chart.md)

用户说"仪表盘排版乱了/图表对不齐/重新排布/自动布局/美化仪表盘" → `dashboard arrange`（读 [aitable-dashboard-chart.md](./aitable-dashboard-chart.md)）

用户说"附件/上传文件" → 读 [aitable-attachment.md](./aitable-attachment.md)

用户说"导入/导出/import/export" → 读 [aitable-export-import.md](./aitable-export-import.md)

用户说"模板" → `template search`

用户说"高级权限/角色/权限控制/谁能看/谁能改" → 读 [aitable-advperm.md](./aitable-advperm.md)
- 开/关高级权限 → `advperm enable` / `advperm disable --yes`
- 看角色配置 → `advperm role-list` 或 `advperm role-get`
- 建角色（可同时指定子角色权限） → `advperm role-create --name ... --sub-roles '[...]'`
- 改角色名 / 改子角色权限（PATCH 语义，未传字段不变） → `advperm role-update --role-id ... [--name ...] [--sub-roles '[...]']`
- 删角色 → `advperm role-delete --yes`
- **角色 ↔ 成员绑定**：当前 CLI 不支持，仍需在 AI 表格 Web 端面板手动完成

命令报错/操作失败 → 读 [aitable-error-recovery.md](./aitable-error-recovery.md)

**关键区分**: base=表格文件, table=数据表, field=列, record=行
