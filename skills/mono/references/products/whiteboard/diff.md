# 白板写入前 Diff 预览

Agent 对已有白板追加、修改、删除或清空内容时必读本页，不能因用户未要求预览而跳过。
`+diff` 是 CLI-only 只读 Shortcut：它读取一个完整页面，在本地比较 proposed
OpenNodes source，不调用任何白板写 Tool。

## 必经闭环

准备 source 后执行 +diff，展示实际变化和全部风险提示，然后停止等待用户明确确认
当前差异。确认后才执行 +update 并添加 --yes。最初的更新请求、render、dry-run
及写后读回不能替代此确认；不得换原子 update 绕过。目标、revision、source 或模式
变化须重新 diff 和确认。diff 失败或有 blocker 时停止，不提交。

下面的 diff 与 update 示例分属确认前后两个阶段，不得连续自动执行。独立白板
使用 diff 的 target.revision 和 sourceDigest；内嵌白板只绑定 sourceDigest，
并披露非原子限制。CLI 参数仍保留脚本兼容性，这里要求的是 Agent 工作流。

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
