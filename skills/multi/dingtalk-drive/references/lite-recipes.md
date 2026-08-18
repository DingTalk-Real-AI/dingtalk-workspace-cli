# Drive 精简流程

只在跨 Drive/Doc/Wiki 的组合任务命中时读取。单一 Drive 操作直接按根 Skill 执行。

## 查找并读取在线文档

1. 未指定空间：`dws drive +search --query "<关键词>" --format json`。
2. 明确知识库：用 Wiki 在指定 workspace 搜索。
3. 唯一确定 nodeId 后切 Doc `+fetch` 读取正文；大文档按返回 continuation 继续。

Drive search 找的是节点；正文全文检索优先用 Doc `+search`。

## 列出目录中的文档

- 普通钉盘目录：`dws drive +list --folder <dentryUuid> --format json`
- 知识库目录：切 Wiki，用 workspace + parent node 定位

处理 nextCursor，并设置最大页数/条目数。不要从根目录无界递归。

## 导入为在线文档

```bash
dws doc +import --file ./report.docx --format json
```

- `.doc/.docx/.md/.txt` 通常导入为在线文字文档；`.xls/.xlsx` 导入为在线表格，最终类型以真实返回为准。
- 目标是“保留原文件”时使用 Drive `+upload`，不是 Doc import。
- 用户给目标文件夹 URL 时，若命令契约不保证 URL 直传，先用 `dws drive info --node <URL>` 验证它是 folder，并复用当前节点 nodeId；不要误用其父 folderId。
- 导入超时或返回 task/checkpoint 时按 Doc 返回的 continuation 恢复，不重复创建。

## 模板保形生成

1. `dws drive +copy --node <源nodeId> --folder <目标folderId>` 保形复制。
2. 用返回的新 nodeId 执行 `dws drive +rename`。
3. 切 Doc，只对副本执行局部正文更新。

不要用“读取 Markdown → 新建文档”替代复制；该路径会丢失在线文档的版式属性。

本剧本只适用于 `+copy` 支持的在线文档节点（如 adoc）；普通钉盘文件创建独立副本应改走经用户授权的 download→upload。若源节点是 `able` 且用户要“只复制结构”，切 AITable，按当前 leaf 执行 `+base-copy --base-id <ID> --target-folder-id <真实ID> --only-struct`。只有根目录意图但没有真实目标文件夹 ID 时停止；禁止发明 `--target-root`、用 Drive 完整复制后逐表删记录或自建测试文件夹。
