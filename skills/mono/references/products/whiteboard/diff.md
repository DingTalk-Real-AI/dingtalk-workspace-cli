# 白板写入前 Diff 预览

仅在准备调用 `dws whiteboard +update`、需要向用户展示影响范围时读取本页。
`+diff` 是 CLI-only 只读 Shortcut：它读取一个完整页面，在本地比较 proposed
OpenNodes source，不调用任何白板写 Tool。

## 推荐闭环

独立白板：

```bash
dws whiteboard +diff --node <WHITEBOARD_NODE_ID> --page-id <PAGE_ID> \
  --source @whiteboard.json --identity-map @identity-map.json --format json

dws whiteboard +update --node <WHITEBOARD_NODE_ID> --page-id <PAGE_ID> \
  --expected-revision <DIFF_TARGET_REVISION> \
  --expected-source-digest <DIFF_SOURCE_DIGEST> \
  --request-id <STABLE_REQUEST_ID> --source @whiteboard.json --format json
```

内嵌白板：

```bash
dws whiteboard +diff --node <DOC_ID> --part-id <PART_ID> \
  --source @whiteboard.json --identity-map @identity-map.json --format json
```

内嵌 diff 固定返回 `embedded_preview_not_atomic` warning：当前没有公开 revision
条件写，必须向用户披露预览至写入间可能漂移。`sourceDigest` 只保证本地 source
未变化，`snapshotDigest` 只用于本次快照审计，都不构成远端原子保证。

## 解释结果

- `executionDiff` 是现有 Tool 的真实语义：append 全部新增并保留旧节点；overwrite
  删除 page-owned 旧节点并重建 proposed 节点，母版只计入 `preservedCount`。
- `logicalDiff.modified` 只有在 `--identity-map` 明确把 proposed 逻辑 ID 绑定到当前
  真实 ID 时才发布。没有映射时返回 `unmatched`，不得猜测修改。
- `logicalDiff.unchangedCount` 和 `executionDiff.preservedCount` 只返回计数。
- `blockerSummary`、`warningSummary` 和全部 summary 计数不受明细预算影响。存在
  blocker 时不得继续 update，即使对应逐节点明细被裁剪。
- `--detail-limit` 默认 100，范围 1..1000，是全局明细预算；business data 另有
  2 MiB 硬上限。`detailsTruncated=true` 时查看 `detailCounts` 和
  `truncationReasons`，不得把未返回明细理解为没有变化。
- 独立 Query 仅返回 `resultDownloadUrl` 时，首版以
  `snapshot_download_required` 失败关闭，不返回部分 diff，也不得继续 update。

Identity map 格式：

```json
{
  "version": 1,
  "target": {"nodeId": "WHITEBOARD_NODE_ID", "pageId": "PAGE_ID", "revision": 12},
  "nodes": {"title": "real-node-101", "body": "real-node-102"}
}
```

内嵌目标改为 `{"nodeId":"DOC_ID","partId":"PART_ID"}`。映射必须是一一对应，
真实 ID 必须仍存在。revision 过期会告警并重新校验身份；目标不一致或真实 ID 已消失
会失败。不要把 Query 真实 ID 直接填进 proposed source 冒充局部 patch。
