# 创建在线文字文档

本页只处理钉钉在线文字文档（`adoc`）创建。普通文件上传走 `dingtalk-drive`，表格和多维表分别走对应产品。

## 唯一推荐入口

```bash
dws doc +create --name "<文档名>" [--content "短文本"] [--folder <ID> | --workspace <ID>] --format json
dws doc +create --name "<文档名>" --content @body.md --format json
dws doc +create --name "<文档名>" --content @body.json --doc-format jsonml --format json
```

- 统一输入协议：已有或临时文件先暂存到当前工作目录后传 `@相对文件`；单次生成文本可用 `--content -` 从 stdin 读取。禁止把用户正文直接作为 `printf` 格式字符串。
- `@file` 禁止绝对路径和 `..` 逃逸；不要直接引用宿主临时目录。
- `--name` 是文档名称，不等于正文中的 heading。用户明确要求“正文一级标题”时，正文仍须包含对应 H1。
- 普通纪要、周报、方案和长文本默认 Markdown。**空列表必须使用 JSONML**：用户要求保留无文本列表项/列表占位时，不得写 Markdown 裸 `-`、`*` 或 `1.`。JSONML 顶层必须是单个数组元素；空无序列表项最小正文为 `["root",{},["p",{"uuid":"li-empty","list":{"listId":"list-empty","level":0,"isOrdered":false}},["span",{"data-type":"text"},["span",{"data-type":"leaf"},""]]]]`。颜色、callout、分栏等 Markdown 无法表达的结构同样走 JSONML。

`+create` 负责创建、长 Markdown 分片和最终回读验证。正常成功结果至少包含：

```json
{
  "status": "success",
  "complete": true,
  "data": {
    "nodeId": "...",
    "verified": true
  }
}
```

## 结果处理

- `status=success` 且 `verified=true`：可以报告创建完成，并保留真实 `nodeId`/URL。
- `status=partial_success`：文档或部分分片已经创建；按 `steps` 回读现状，禁止重跑整条创建。
- `status=unknown`：服务端可能已经提交；先定位并读取文档，禁止自动重试。
- 没有真实 `nodeId` 或写回执时，禁止声称“已创建”。

## 高级通道

只有 shortcut 未公开所需的底层参数或需要原始响应时，才读取精确 leaf Schema 后使用 `dws doc create`。不要因为熟悉旧参数就默认退回原子命令，也不要使用已删除的 Python 创建脚本。

明确的富结构规划只读取 [doc-create-workflow.md](style/doc-create-workflow.md) 这一份 reference；“中文内容长、属于周报/方案”本身不触发该 workflow。不要继续连读 style/cookbook/schema，命令选路仍以 `+create` 和精确 leaf Schema 为准。
