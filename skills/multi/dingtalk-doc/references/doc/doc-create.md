# 创建在线文字文档

本页只处理钉钉在线文字文档（`adoc`）创建。普通文件上传走 `dingtalk-drive`，表格和多维表分别走对应产品。

## 唯一推荐入口

```bash
dws doc +create --name "<文档名>" [--content "短文本"] [--folder <ID> | --workspace <ID>] --format json
dws doc +create --name "<文档名>" --content @body.md --format json
dws doc +create --name "<文档名>" --content @body.json --doc-format jsonml --format json
```

- 统一输入协议：已有或临时文件先暂存到当前工作目录后传 `@相对文件`；单次生成文本可用 `--content -` 从 stdin 读取。
- `@file` 禁止绝对路径和 `..` 逃逸；不要直接引用宿主临时目录。
- `--name` 是文档名称；默认不要再在正文开头重复同名 H1，只有用户明确要求正文一级标题时才保留。
- JSONML 顶层必须是数组；仅在确实需要富结构时加载 JSONML cookbook。

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

## 创作与执行策略

采用 Plan → Execute → Observe → Iterate，将循环放在本地内容与定点修正上，不把长文拆成一串远程创建/追加：

1. **Plan**：明确受众、目的、范围和结构；正文由一个主上下文串行维护，避免按章节并行生成造成重复、矛盾和语气漂移。
2. **Draft**：短内容可直接传；多行、长文或含特殊字符时先形成 cwd 内相对文件。用户未要求富结构时优先 Markdown；只有样式/引用/嵌套结构确有必要时才用 JSONML。
3. **Execute**：只调用一次 `+create`。DWS Runtime 会分片长 Markdown、记录每步并回读验证；Agent 不采用“先骨架、再逐节远程插入”流程，以减少网络次数、顺序错误和 commit-unknown 面。
4. **Observe**：先检查回执中的 `nodeId`、`verified`、`steps` 和失败状态。回执已证明完整时不重复拉全文；需要内容质量验收时，只用 `+fetch --scope section/keyword` 读取待检查部分。
5. **Iterate**：后续修正复用同一 `nodeId`，按 [`doc-update.md`](doc-update.md) 做最小 block/文本修改，禁止重新创建整篇。

交付前检查标题是否重复、段落是否连贯、编号是否统一；只有真实行列数据才使用表格，富组件服务于理解而不是装饰。明确字数要求时应在写入前完成本地统计，不能凭模型估算宣称达标。

## 高级通道

只有 shortcut 未公开所需的底层参数或需要原始响应时，才读取精确 leaf Schema 后使用 `dws doc create`。不要因为熟悉旧参数就默认退回原子命令，也不要使用已删除的 Python 创建脚本。

复杂排版按需读取 [doc-style-guideline.md](style/doc-style-guideline.md)；命令选路仍以本页的 `+create` 为准。
