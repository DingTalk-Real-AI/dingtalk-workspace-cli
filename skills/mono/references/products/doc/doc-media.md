# doc media（附件 / 图片：download / insert）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

> ⚠️ **图片插入硬规则**：
> - 图片来源如果是钉盘/文档空间中的文件，**必须先下载到本地**（`dws doc download --node <图片nodeId> --output /tmp/xxx.png`），再执行 `media insert`
> - **禁止**把钉盘/文档节点 URL（如 `alidocs.dingtalk.com/i/nodes/...`）写进 Markdown `![](...)` 图片语法——这些是页面链接，不是可渲染的图片资源
> - 创建文档时需要图文并茂：先用 `doc create` 写入纯文本骨架，再对每张图片执行 `media insert`，最后用 `doc block list` 验证附件块存在

---

## doc media insert（上传附件并插入文档）

`media insert` 是三步自动完成的流程（获取附件上传凭证 → OSS 上传 → 插入附件块到文档），无需手动分步操作。

> **图片插入也走 `media insert`**：用户说"插图 / 加图片 / 嵌入图片"时使用本命令，不要走 [`doc-block.md`](./doc-block.md) `block insert`。

```
Usage:
  dws doc media insert [flags]
Example:
  dws doc media insert --node <DOC_ID> --file ./report.pdf
  dws doc media insert --node <DOC_ID> --file ./data.bin --name "数据文件.dat" --mime-type application/octet-stream
  dws doc media insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where before
Flags:
      --node string        目标文档的标识，支持传入 URL 或 ID (必填)
      --file string        本地文件路径 (必填)
      --name string        附件显示名称 (默认使用文件名)
      --mime-type string   文件 MIME 类型 (默认根据扩展名推断)
      --index int          插入位置索引
      --where string       相对位置: before / after (配合 --ref-block)
      --ref-block string   参考块 ID (配合 --where)
```

### 关键说明

- `--mime-type` 可选，不指定时根据扩展名自动推断；支持常见文件类型（PDF、Office、图片、视频、压缩包等）。
- 与 [`./doc-file-ops.md`](./doc-file-ops.md) `doc upload` 的区别：`upload` 将文件上传到文档空间/知识库作为**独立文件**；`media insert` 将文件作为**附件块插入到文档正文中**。

---

## doc media download（下载文档附件）

```
Usage:
  dws doc media download [flags]
Example:
  dws doc media download --node <DOC_ID> --resource-id <RESOURCE_ID>
  dws doc media download --node "https://alidocs.dingtalk.com/i/nodes/xxx" --resource-id <RESOURCE_ID>
Flags:
      --node string          目标文档的标识，支持传入 URL 或 ID (必填)
      --resource-id string   附件资源 ID，可通过 dws doc block list 获取 (必填)
```

### 关键说明

- `media download` 用于获取文档正文中附件的**临时下载链接**。
- `--resource-id` 可通过 [`./doc-block.md`](./doc-block.md) `block list` 返回的 attachment 块获取，或从 [`./doc-read.md`](./doc-read.md) 返回的 OSS 链接 `/att/<resourceId>.ext` 中提取。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `media insert` `resourceId` | 附件已插入文档；可通过 [`./doc-block.md`](./doc-block.md) `block list` 查看附件块 |
| `media download` `downloadUrl` | 下载文档中的附件资源（临时链接，会过期） |

## 常用模板

```bash
# 基本用法：插入本地文件到文档
dws doc media insert --node <DOC_ID> --file ./report.pdf

# 指定附件显示名称
dws doc media insert --node <DOC_ID> --file ./data.xlsx --name "Q1数据报表.xlsx"

# 指定 MIME 类型（扩展名无法推断时）
dws doc media insert --node <DOC_ID> --file ./data.bin --name "导出数据.dat" --mime-type application/octet-stream

# 在指定块之前插入附件
dws doc media insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where before

# 完整流程：创建文档 → 写入内容 → 插入附件
dws doc create --name "项目报告" --content "# 项目报告\n\n以下为相关附件：" --content-format markdown
# 提取 nodeId 后:
dws doc media insert --node <DOC_ID> --file ./design.pdf
dws doc media insert --node <DOC_ID> --file ./timeline.xlsx --name "项目时间线.xlsx"

# 下载文档中的附件（resourceId 从 block list 获取）
dws doc media download --node <DOC_ID> --resource-id <RESOURCE_ID>
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令族）
- [`./doc-block.md`](./doc-block.md)（block list 取 attachment 的 resourceId）
- [`./doc-file-ops.md`](./doc-file-ops.md)（独立文件上传：`doc upload`）
- [`./style/doc-style-guideline.md` §4.9 附件与图片](./style/doc-style-guideline.md)（图示与附件使用规范）
