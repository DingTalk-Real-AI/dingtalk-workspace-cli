# 白板整页替换与清空

在整页重绘、清空，或删除部分内容需要重建完整终态时读取本页。overwrite 会替换整个单页白板，不是
局部节点更新。

## 执行顺序

1. 对同一稳定目标执行一次 `+query`：内嵌白板使用 `nodeId/partId`，独立白板使用
   `nodeId/pageId` 并保留当前 revision。保留完整当前快照并汇总将被删除的节点。
2. 准备完整终态，保留用户未要求删除的内容。必读 [diff.md](./diff.md) 并执行同一目标的 +diff；展示变化、删除重建影响、warnings 和 blockers 后停止，等待用户明确确认当前差异。失败或有 blocker 不得提交。
3. 确认后通过 +update 携带同一 sourceDigest 一次提交完整终态，独立白板使用 diff 的 target.revision。内容、目标或版本变化须重新 diff 和确认；不要先清空再追加。
4. `+update` 成功且 `verified=true` 表示已在内部对同一稳定目标完成最终
   读回校验，直接依据验证结果、summary 和 receipt 交付，不再追加 `+query`。
5. 仅用户明确要求更新后完整快照时，才对同一目标追加一次 `+query`；若此次读取
   失败，分别报告更新已验证成功、快照获取失败，不把已验证的写入改报失败或重放。

```bash
# 文档内嵌白板示例
dws whiteboard +query --node <DOC_NODE_ID> --part-id <PART_ID> --format json
dws whiteboard +diff --node <DOC_NODE_ID> --part-id <PART_ID> --source @overwrite.json --format json
# 展示差异并等待用户确认后才执行更新
dws whiteboard +update --node <DOC_NODE_ID> --part-id <PART_ID> \
  --source @overwrite.json --expected-source-digest <DIFF_SOURCE_DIGEST> --format json
```

独立白板执行读取、构造、diff、展示确认、写入流程。diff 会读取最新完整页面，不能因已 query 而省略；update 使用 diff 的 target.revision 和 sourceDigest。内嵌白板须披露预览非原子。

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

独立白板 overwrite 的 pageId、expectedRevision 和 requestId 缺一不可；revision
冲突应重新 query 后交由调用方决定是否基于新快照重新构造终态，不能自动覆盖。
内嵌分支仍维持原参数、接口和返回结构。两类目标的失败均不得触发跨类型回退。
