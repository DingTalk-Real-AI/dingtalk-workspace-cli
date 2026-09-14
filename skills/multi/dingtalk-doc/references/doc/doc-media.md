# 文档正文媒体

## Golden Route

```bash
# 列出图片和附件，取得真实 resourceId/blockId
dws doc +media-list --node <DOC_ID> --format json

# 插入工作目录内的本地文件
dws doc +media-insert --node <DOC_ID> --file ./image.png --format json

# 下载到工作目录，默认不覆盖
dws doc +media-download --node <DOC_ID> --resource-id <RESOURCE_ID> --output ./downloads/ --format json

# 只为临时查看下载到受控临时目录
dws doc +media-preview --node <DOC_ID> --resource-id <RESOURCE_ID> --format json
```

## 封面与背景 Shortcut

```bash
dws doc +resource-update --node <DOC_ID> --file ./cover.png --format json
dws doc +resource-download --node <DOC_ID> --output ./cover.png --format json
dws doc +resource-delete --node <DOC_ID> --format json
dws doc +background-update --node <DOC_ID> --color "#E8F2FE" --format json
dws doc +background-delete --node <DOC_ID> --format json
```

封面不是正文媒体 block；背景仅接受 `#RRGGBB` 纯色。设置/清除后用一次 `+inspect --include-style` 验证，禁止为这些已知能力查询 shortcut Catalog。

## 稳定 ID 与结果

- `resourceId`、`blockId`、`nodeId` 必须来自真实 media/block 返回，不能从标题或本地文件名猜测。
- 插入成功回执在 `data.blockId` 返回已回读验证的媒体块 ID；后续定位只复用该 `blockId`。回执不提供相邻空块字段，不得据此猜测或自动删除其他块。插入回执成功后禁止重传媒体。下载必须检查 `localPath`、`sizeBytes > 0`。
- 下载输出只接受工作目录内相对路径，默认 no-clobber。
- 删除源文件是独立的破坏性本地操作，不属于媒体下载；只有用户明确要求且下载验证成功后才能执行。

## 失败最短路径

- `download_doc_attachment` 失败：保留 `nodeId/resourceId` 和服务端错误，停止。
- 临时链接过期：重新调用 `+media-download` 获取新链接，由 CLI 内部下载。
- 禁止把 `+fetch` 返回的临时 OSS URL 交给 `curl/wget`。
- 禁止安装图片、Office 或 Python 依赖作为隐式降级。
- 不得把纯数字 dentryId 当作 drive folder 重试；需要文件树定位时切换到 `dingtalk-drive` 并使用真实 dentryUuid。

只有 shortcut 缺少必要的底层定位参数时，才读取精确原子 leaf Schema；不要把 `doc media insert/download` 作为默认入口。
