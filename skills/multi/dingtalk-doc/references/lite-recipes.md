# doc Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #4 文档知识

### query-doc

1. 用户已提供 URL / `nodeId` 时执行 `dws doc +fetch --node <目标> --format json`。
2. 只有标题时执行 `dws doc +fetch --query "<唯一标题>" --format json`，让 Runtime 解析候选与类型。
3. 零命中、多候选、分页不完整或非 `adoc` 时停止并按返回建议切换产品；禁止无界穷举搜索。

### list-folder-docs

`dws doc +list --workspace <WS_ID> --format json`；知识库层级管理切 `dingtalk-wiki`。

### import-file

将本地文件导入为钉钉在线对象。**一条命令完成上传、转换和创建**，无需先读取文件内容。

```bash
dws doc +import --file ./report.docx --folder <FOLDER_ID> --format json
```

1. 确认文件是工作目录内的相对路径。
2. 执行 `+import`，可选 `--folder`、`--workspace` 和 `--name`。
3. 从返回中提取 `documentUrl`；若 `fallback=upload`、`converted=false`，明确说明结果是原文件对象。
4. `partial_success/unknown` 时按返回的 `taskId/steps` 恢复，不得重跑整个导入。

`--folder` 首选用户提供的 alidocs URL 或真实 `nodeId`；不得使用 `drive info` 返回的父级 `folderId`。

> 禁止先 Read 文件再 `doc create` + `doc update`。详见 [./doc/doc-import.md](./doc/doc-import.md)。
