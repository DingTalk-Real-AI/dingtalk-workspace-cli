# 文档正文媒体

## Golden Route

```bash
# 列出图片和附件，取得真实 resourceId/blockId
dws doc +media-list --node <DOC_ID> --format json

# 插入工作目录内的本地文件
dws doc +media-insert --node <DOC_ID> --file ./image.png --format json

# 紧跟已知正文块插入；--where 与 --ref-block 必须成对出现
dws doc +media-insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where after --format json

# 下载到工作目录，默认不覆盖
dws doc +media-download --node <DOC_ID> --resource-id <RESOURCE_ID> --output ./downloads/ --format json

# 只为临时查看下载到受控临时目录
dws doc +media-preview --node <DOC_ID> --resource-id <RESOURCE_ID> --format json
```

## 封面与正文媒体边界

- 设置或替换文档封面：`dws doc +cover-set --node <DOC_ID> --image <HTTPS_URL>`，本地图片改用 `--file <相对路径>`。
- 下载当前封面：`dws doc +cover-download --node <DOC_ID> --output ./cover.png`。
- 清除当前封面：`dws doc +cover-clear --node <DOC_ID>`。
- 封面属于文档样式，不是正文图片 block；正文图片始终使用 `+media-insert`。旧 `+resource-update/+resource-download/+resource-delete` 只为已有调用兼容，不用于新任务选路。

## 稳定 ID 与结果

- `resourceId`、`blockId`、`nodeId` 必须来自真实 media/block 返回，不能从标题或本地文件名猜测。
- 插入后使用回执中的 `insertedBlockId/affectedBlockIds` 继续定位；CLI 已验证资源、媒体类型和位置，失败时不得重放上传。`insertedBlockId` 是媒体容器本身，绝不是图片后的空块。若要删除图片后的空块，必须先确认 `position.followingBlockExists=true`，再使用 `position.followingBlockId` 删除独立后继块；值为 `false` 时不得删除媒体容器或重传媒体。
- 下载输出只接受工作目录内相对路径，默认 no-clobber。
- 删除源文件是独立的破坏性本地操作，不属于媒体下载；只有用户明确要求且下载验证成功后才能执行。

## 失败最短路径

- `download_doc_attachment` 失败：保留 `nodeId/resourceId` 和服务端错误，停止。
- 临时链接过期：重新调用 `+media-download` 获取新链接，由 CLI 内部下载。
- 禁止把 `+fetch` 返回的临时 OSS URL 交给 `curl/wget`。
- 禁止安装图片、Office 或 Python 依赖作为隐式降级。
- 不得把纯数字 dentryId 当作 drive folder 重试；需要文件树定位时切换到 `dingtalk-drive` 并使用真实 dentryUuid。

只有 shortcut 缺少必要的底层定位参数时，才读取精确原子 leaf Schema；不要把 `doc media insert/download` 作为默认入口。
