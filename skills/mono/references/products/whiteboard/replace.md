# 白板整页替换与清空

仅在用户明确要求整页重绘或清空时读取本页。overwrite 会替换整个单页白板，不是
局部节点更新。

## 执行顺序

1. 对同一 `nodeId/partId` 执行一次 `+query`，保留完整当前快照并汇总将被删除的
   节点。
2. 向用户说明目标、overwrite 影响和新节点数量，按 Runtime gate 取得确认。
3. 一次提交完整终态；不要先清空再追加。
4. `+update` 成功且 `verified=true` 表示已在内部对同一 `nodeId/partId` 完成最终
   读回校验，直接依据验证结果、summary 和 receipt 交付，不再追加 `+query`。
5. 仅用户明确要求更新后完整快照时，才对同一目标追加一次 `+query`；若此次读取
   失败，分别报告更新已验证成功、快照获取失败，不把已验证的写入改报失败或重放。

```bash
# 旧版快照
dws whiteboard +query --node <DOC_NODE_ID> --part-id <PART_ID> --format json
dws whiteboard +update --node <DOC_NODE_ID> --part-id <PART_ID> \
  --source @overwrite.json --format json
```

清空整页的更新文件：

```json
{
  "overwrite": true,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": []
  }
}
```

只有用户明确要求清空时才允许空数组。整页重绘使用相同信封，但 `nodes` 必须包含
完整终态。`+update` 自身的回执缺失、读回失败或不一致不能报告完整成功；保留旧快照、
完整新 Payload、可用 receipt 和差异。超时或 commit-unknown 同样不证明未提交：
停止写入，最多再对同一目标 `+query` 一次只读对账，报告已提交但未验证或提交状态
不明，不自动重放 overwrite、清空或追加。
