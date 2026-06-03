# doc read（读取文档内容）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
> 2. [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) — 仅当使用 `--content-format jsonml` 时必读
>
> **同任务常配合**：[`doc-info.md`](./doc-info.md)（先解析 URL，确认 contentType=ALIDOC、extension=adoc）/ [`doc-update.md`](./doc-update.md)（读后改写）/ [`doc-block.md`](./doc-block.md)（块级精修前先读结构）

## 命令格式

```
Usage:
  dws doc read [flags]
Example:
  dws doc read --node <DOC_ID>
  dws doc read --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
  dws doc read --node <DOC_ID> --content-format jsonml --output ./doc.json
Flags:
      --node string     文档 ID 或 URL (必填)
      --content-format string   输出格式: 默认为 markdown，可选 jsonml（返回完整 JSONML 结构）
      --output string   输出到本地文件路径（仅 --content-format jsonml 时生效）
```

## 关键说明

- 默认返回 **Markdown** 格式的文档内容，仅限有"下载"权限的文档。
- 返回的 Markdown 中，附件以 OSS 临时下载链接形式给出（如 `https://alidocs2.oss-cn-zhangjiakou.aliyuncs.com/res/.../att/<resourceId>.ext?Expires=...`），**链接会过期**。链接过期后从 URL 路径中提取 `<resourceId>`（即 `/att/` 后、扩展名前的 UUID 部分），用 `media download --node <DOC_ID> --resource-id <resourceId>` 重新获取下载链接。
- `--content-format jsonml` 返回完整 JSONML 结构（含 `revision`），用于无损读改写；可直接配合 [`doc-update.md`](./doc-update.md) 的 `--content-format jsonml --content-file` 写回。`revision` 仅在并发敏感场景下需要透传给 update 触发并发检查（详见下方）。

## content-format=jsonml 输出

输出 JSON 对象，包含 `revision`（版本号）和 `jsonml`（JSONML body 数组）：

```json
{
  "revision": 42,
  "jsonml": ["root", {"sectPr": {}}, ["p", {"uuid": "abc"}, "Hello"], ...]
}
```

可直接用于 `doc update --content-format jsonml --content-file` 写回。`revision` 字段在普通改写场景下**不需要**透传——`doc update` 默认直接覆盖。仅在担心多 agent 并发覆盖时，才把 `revision` 通过 `--revision` 透传给 update 触发并发检查（详见下方 §并发安全模式）。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| Markdown 正文 | 用户可读输出 / 二次处理 |
| JSONML `jsonml` 数组 | [`doc-update.md`](./doc-update.md) 的 `--content-file` + `--content-format jsonml` |
| JSONML `revision` | [`doc-update.md`](./doc-update.md) 的 `--revision`（可选；担心被并发覆盖时使用） |
| 附件链接中的 `resourceId` | [`doc-media.md`](./doc-media.md) 的 `--resource-id`（链接过期后续期） |

## 常用模板

```bash
# 默认 Markdown 输出（最常用）
dws doc read --node <DOC_ID> --format json

# alidocs URL 直传
dws doc read --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --format json

# JSONML 完整结构 → 文件（无损改写前置）
dws doc read --node <DOC_ID> --content-format jsonml --output /tmp/doc.json
# 之后修改 /tmp/doc.json 中的 jsonml 数组，再用：
#   dws doc update --node <DOC_ID> --content-file /tmp/doc.json --content-format jsonml --mode overwrite
# 担心被并发覆盖时，再加 --revision <从上面 read 拿到的 revision>
```

## 并发安全模式（担心被并发覆盖时使用）

如果你担心在编辑期间别人也在改这个文档，可以把 read 返回的 `revision` 透传给 update 触发服务端并发检查：

1. `dws doc read --node <DOC_ID> --content-format jsonml --output /tmp/doc.json` — 输出 JSON 中的 `revision` 字段（如 `42`）记下来。
2. 编辑 `/tmp/doc.json` 中的 `jsonml` 字段。
3. `dws doc update --node <DOC_ID> --content-file /tmp/doc.json --content-format jsonml --mode overwrite --revision 42` — 文档若在期间被改过，服务端返回 `VersionConflict`，重做第 1 步即可。

不带 `--revision` 时服务端不做并发检查，直接覆盖；普通单 agent 编辑场景下默认不传即可。

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-info.md`](./doc-info.md)（前置：判断 contentType / extension）
- [`./doc-update.md`](./doc-update.md)（读后改写）
- [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) / [`./format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md)（JSONML 节点结构）
