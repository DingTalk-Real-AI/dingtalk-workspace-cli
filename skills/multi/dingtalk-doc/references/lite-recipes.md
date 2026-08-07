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

将本地文件导入为钉钉在线文档。**一条命令完成上传+格式转换+创建**，无需先读取文件内容。

```bash
dws doc +import --file ./report.docx --folder <FOLDER_ID> --format json
```

1. 确认文件路径（用户提供的本地文件路径）
2. 执行：`dws doc +import --file <文件路径> --folder <文件夹ID> --format json`（也可用 `--workspace <知识库ID>`，可选 `--name`）
3. 从返回中提取 `documentUrl`，告知用户导入完成并提供链接
4. `partial_success/unknown` 时按返回的 `taskId/steps` 恢复，不得重跑整个导入

**`--folder` 参数传值规则**：
- 首选路径：用户提供 alidocs URL 时，直接将完整 URL 传入 `--folder`，无需先调 `drive info`
- 预检路径：若需确认 URL 指向的是文件夹，可先调 `dws drive info --node <URL>`：
  - `nodeType == "folder"` → 使用 `nodeId` 或原始 URL 作为 `--folder` 值
  - `nodeType` 不是 folder → 提示用户：该链接指向的不是文件夹
- 禁止：不得使用 `drive info` 返回的 `folderId` 字段作为 `--folder` 的值（`folderId` 是父文件夹 ID，非当前节点 ID）

格式与文档类型映射：
- `.docx` / `.doc` → 文字文档（DOC）
- `.xlsx` / `.xls` → 电子表格（SHEET）
- `.xmind` / `.mark` → 脑图（MIND）
- `.md` / `.txt` → 文字文档（DOC）

> **禁止先 Read 文件再 `doc create` + `doc update`**。`doc import` 是服务端格式转换，客户端无需解析文件内容。
> 详见 [./doc/doc-import.md](./doc/doc-import.md)。
