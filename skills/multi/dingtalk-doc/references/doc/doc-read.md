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
  dws doc read --node <DOC_ID> --content-format jsonml --scope outline
Flags:
      --node string     文档 ID 或 URL (必填)
      --content-format string   输出格式: 默认为 markdown，可选 jsonml（返回完整 JSONML 结构）
      --output string   输出到本地文件路径（仅 --content-format jsonml 时生效）
      --scope string    JSONML 节点范围: outline / range / section / tags
      --tags string     自定义 JSONML tag 列表，逗号分隔；仅 --scope tags 使用且必填
      --max-depth int   筛选遍历最大深度，0 表示不限
      --start-block-id string   range / section 起始块 UUID
      --end-block-id string     range 结束块 UUID；空或 "-1" 表示到文档末尾
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

## scope/tags 节点筛选（返回 JSONML fragment）

只读取文档大纲、一个块或一段区间时，用 `--scope` 避免拉取整篇。筛选仅适用于 JSONML，必须同时传 `--content-format jsonml`。

- `--scope outline`：返回全部 `h1` 到 `h6` 标题。
- `--scope tags --tags h1,table`：返回指定 tag；`--tags` 只能与 `scope=tags` 一起使用。
- `--scope range --start-block-id <UUID> [--end-block-id <UUID>]`：返回顶层闭区间；结束 ID 为空或 `-1` 时读到文末。
- `--scope section --start-block-id <UUID>`：返回该块及其完整子树。
- `--max-depth` 可限制筛选遍历深度，`0` 表示不限制。

块 UUID 可从 [`doc-block.md`](./doc-block.md) 的块列表或完整 JSONML 节点的 `uuid` 属性取得。

筛选结果是查询用的虚拟 fragment 容器：

```json
["fragment", {"source": "outline"}, ["h1", {"uuid": "h1a"}, "一级标题"]]
```

消费时必须剥掉 `"fragment"` 和属性对象，只取后面的 children。不得把 fragment 容器整体写回文档。

```bash
dws doc read --node <DOC_ID> --content-format jsonml --scope outline
dws doc read --node <DOC_ID> --content-format jsonml --scope range --start-block-id <UUID_A> --end-block-id <UUID_B>
dws doc read --node <DOC_ID> --content-format jsonml --scope section --start-block-id <UUID>
dws doc read --node <DOC_ID> --content-format jsonml --scope tags --tags table,img
```

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| Markdown 正文 | 用户可读输出 / 二次处理 |
| JSONML `jsonml`（完整 body，无 scope） | [`doc-update.md`](./doc-update.md) 的 `--content-file` + `--content-format jsonml` |
| JSONML fragment（scope 筛选） | 剥掉 fragment 外层后消费 children；不得整体写回 |
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
