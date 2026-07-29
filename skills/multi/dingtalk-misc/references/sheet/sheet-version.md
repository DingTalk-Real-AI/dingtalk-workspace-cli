# 历史版本 (version)

## 使用场景

管理钉钉在线电子表格的历史版本快照。当用户说"保存版本/存个快照/看历史版本/版本列表/回滚到某个版本/恢复到之前的表格"时使用。

- 手动保存当前表格为一个版本快照 → `version save`
- 查看表格的历史版本列表（含版本号、名称、创建人、创建时间） → `version list`（别名 `ls`）
- 把表格回滚到指定历史版本 → `version revert`（危险操作，默认需二次确认）

三个命令统一用 `--node` 指定表格文档（ID 或 URL）。回滚用的**版本号 `--version` 从 `version list` 的返回中获取**，禁止臆测。

## 命令详细参考

### 保存表格版本快照
```
Usage:
  dws sheet version save [flags]
Example:
  dws sheet version save --node SHEET_ID

Flags:
      --node string   表格文档 ID 或 URL (必填)
```
手动为当前在线电子表格生成一个历史版本快照，便于后续查看或回滚。

### 查看表格历史版本列表
```
Usage:
  dws sheet version list [flags]
  dws sheet version ls [flags]
Example:
  dws sheet version list --node SHEET_ID
  dws sheet version list --node SHEET_ID --limit 10

Flags:
      --node string     表格文档 ID 或 URL (必填)
      --limit int       返回版本数量上限 (可选)
      --cursor string   分页游标 (可选，游标分页)
```
返回表格的历史版本列表；回滚前先用它拿到目标 `version`（版本号）。

### 回滚表格到指定版本
```
Usage:
  dws sheet version revert [flags]
Example:
  dws sheet version revert --node SHEET_ID --version 3 --yes

Flags:
      --node string    表格文档 ID 或 URL (必填)
      --version int    目标版本号 (必填，从 version list 获取)
```
把表格回滚到指定历史版本。**危险操作**：会覆盖当前内容，默认弹出二次确认；AI Agent 或脚本执行时可加全局 `--yes` 跳过确认。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `version list` | `version`（版本号） | 作为 `version revert --version` 的入参 |
| `version save` | 版本快照结果 | 确认已生成快照 |
| `version revert` | 回滚结果 | 确认回滚完成，回读表格内容验证 |

## 注意事项

- ★ 回滚 `version revert` 会**覆盖当前表格内容**，属危险操作，默认二次确认；确认无误再执行，脚本/Agent 场景加 `--yes`
- ★ `--version` 必须来自 `version list` 的真实返回，禁止臆测版本号
- 底层复用 doc 域的历史版本能力（`save_doc_version` / `list_doc_versions` / `revert_doc_version`），`--node` 传在线电子表格文档即可
- 回滚后应用独立读命令（`csv-get` / `range read`）回读确认，避免"写返回不等于完成"
