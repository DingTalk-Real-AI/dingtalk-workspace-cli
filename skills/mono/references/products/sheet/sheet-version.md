# 历史版本 (version)

## 使用场景

管理钉钉在线电子表格的历史版本快照。当用户说"保存版本/存个快照/看历史版本/版本列表/回滚到某个版本/恢复到之前的表格"时使用。

- 手动保存当前表格为一个版本快照 → `version save`
- 查看表格的历史版本列表 → `version list`（别名 `ls`）。返回不含版本名称；列表项包含 `version`、`type`、`userId`、`createTime`、`updateTime`、`editorList` 等服务端字段
- 把表格回滚到指定历史版本或已确认的精确 revision → `version revert`（危险操作，默认需二次确认）

三个命令统一用 `--node` 指定表格文档（ID 或 URL）。默认从 `version list` 选择稳定的历史版本。用户明确要求恢复到某个精确 revision 时，也可把已从同一工作簿真实查询结果确认的 revision 传给 `--version`，即使它没有出现在版本列表中。禁止猜测版本号或 revision。

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
返回表格的历史版本列表；顶层包含 `versions`、`nextCursor`、`hasMore`，列表项不含 `name`。回滚前先从列表项拿到真实 `version`（版本号）。

### 回滚表格到指定版本
```
Usage:
  dws sheet version revert [flags]
Example:
  dws sheet version revert --node SHEET_ID --version 3

Flags:
      --node string    表格文档 ID 或 URL (必填)
      --version int    目标历史版本或已确认 revision (必填，通常从 version list 获取)
```
把表格回滚到指定历史版本或精确 revision。**危险操作**：会覆盖当前内容，默认要求二次确认。本文不提供带 `--yes` 的可复制示例；执行器只有在当前流程中向用户展示完整目标参数和覆盖风险、并获得明确确认后，才可动态追加全局 `--yes`。

`version list` 只列选定的保存或回滚点，并不包含每一个 revision。未列入版本列表的 revision 只有在服务端仍可恢复时才能回滚成功；过旧或内容不可用时应直接报告失败，禁止改猜相邻 revision 重试。

### 精确 revision 回滚

只有用户明确要求恢复到某个 revision 时才走此流程：

1. 用 `revision-get` 记录回滚前的当前 revision。
2. 目标 revision 必须来自同一工作簿的真实查询结果，或由用户明确提供；不要根据次数、时间或相邻版本推算。
3. 向用户展示工作簿、目标 revision 以及“当前内容将被覆盖”的风险，然后停止并等待明确确认；确认前禁止调用回滚工具，也禁止预先添加全局 `--yes`。
4. 只有用户对当前展示的参数明确确认后，执行器才可对同一条命令动态追加全局 `--yes` 并执行；任一参数发生变化都必须重新确认。
5. 用 `csv-get`、`range read` 或其他对应读取命令回读所需内容。
6. 需要审计回滚事件时，用回滚前后的 revision 查询 `changeset-get`，确认出现 `STATE_RESET`，且 `reset.targetRevision` 等于目标 revision。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `version list` | `version`（版本号） | 作为 `version revert --version` 的入参 |
| `revision-get` / `changeset-get` | 已确认属于同一工作簿的 `revision` | 用户明确要求精确恢复时，作为 `version revert --version` 的入参 |
| `version save` | 版本快照结果 | 确认已生成快照 |
| `version revert` | 回滚结果 | 确认回滚完成，回读表格内容验证 |

## 注意事项

- ★ 回滚 `version revert` 会**覆盖当前表格内容**，属危险操作；确认前禁止调用工具。只有用户对当前完整参数明确确认后，执行器才可动态追加全局 `--yes`；任一参数变化都必须重新确认
- ★ 默认从 `version list` 选择历史版本；只有用户明确要求精确 revision 时，才使用同一工作簿真实查询结果中已确认的 revision
- ★ 未列入版本列表的 revision 不保证仍可恢复；失败时直接说明，不猜测其他 revision，不自动重试
- ★ `version list` 返回的 `version` 可作为 `changeset-get` 的 start/end 锚点，但相邻历史版本之间可能包含多个 revision。需要查看逐次变更时读 [sheet-revision-changeset](./sheet-revision-changeset.md)
- 回滚后应用独立读命令（`csv-get` / `range read`）回读确认，避免"写返回不等于完成"
