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

用户要求把已有本地文件转换为钉钉在线文档时，使用导入流程，无需为上传重复读取全文。独立普通附件使用 `dws drive upload --file <文件路径> --workspace <知识库ID> --format json` 且不加 `--convert`；正文附件交给 Doc 媒体。三者不能互相替代。

```bash
dws doc import --file ./report.docx --format json
```

1. 确认文件路径（用户提供的本地文件路径）
2. 执行：`dws doc import --file <文件路径> --format json`（可选 `--folder <文件夹ID>` / `--workspace <知识库ID>` / `--name "文档名"`）
3. 从完成回执提取 `documentUrl`，沿用其真实节点身份；未确认完成时保留 `taskId` 查询，不按同名搜索重建或宣称完成
4. 超时或中断时 CLI 返回 `taskId`，用 `dws doc import get --task-id <taskId> --format json` 手动查询

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

> 用户明确要求先创建空节点再写正文、编辑现有节点，或先检查本地内容时，按该意图执行；不要用导入替换已有目标或省略指定步骤。导入提交效果未知时只查询原 taskId，不自动重复创建。
> 详见 [./doc/doc-import.md](../../dingtalk-doc/references/doc/doc-import.md)。
