# wiki Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #4 文档知识

### query-doc

1. 全局搜索：`drive search --query "<关键词>"` → `nodeId`（聚合钉盘+文档空间）
2. 空间内搜索：`wiki node search --workspace <WS_ID> --query "<关键词>"` → `nodeId`
3. `doc read --node <nodeId>`（按需；大文档只抽章节）

### list-folder-docs

`drive list --workspace <WS_ID>` 或 `wiki node list --workspace <WS_ID>`

### import-file

将本地文件导入为钉钉在线文档。**一条命令完成上传+格式转换+创建**，无需先读取文件内容。

```bash
dws doc import --file ./report.docx --format json
```

1. 确认文件路径（用户提供的本地文件路径）
2. 执行：`dws doc import --file <文件路径> --format json`（可选 `--folder <文件夹ID>` / `--workspace <知识库ID>` / `--name "文档名"`）
3. 从返回中提取 `documentUrl`，告知用户导入完成并提供链接
4. 超时或中断时 CLI 返回 `taskId`，用 `dws doc import get --task-id <taskId> --format json` 手动查询

**`--folder` 参数传值规则**：
- 首选路径：用户提供 alidocs URL 时，直接将完整 URL 传入 `--folder`，无需先调 `doc info`
- 预检路径：若需确认 URL 指向的是文件夹，可先调 `dws doc info --node <URL>`：
  - `nodeType == "folder"` → 使用 `nodeId` 或原始 URL 作为 `--folder` 值
  - `nodeType` 不是 folder → 提示用户：该链接指向的不是文件夹
- 禁止：不得使用 `doc info` 返回的 `folderId` 字段作为 `--folder` 的值（`folderId` 是父文件夹 ID，非当前节点 ID）

格式与文档类型映射：
- `.docx` / `.doc` → 文字文档（DOC）
- `.xlsx` / `.xls` → 电子表格（SHEET）
- `.xmind` / `.mark` → 脑图（MIND）
- `.md` / `.txt` → 文字文档（DOC）

> **禁止先 Read 文件再 `doc create` + `doc update`**。`doc import` 是服务端格式转换，客户端无需解析文件内容。
> 详见 [./doc/doc-import.md](../../dingtalk-doc/references/doc/doc-import.md)。
