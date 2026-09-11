# attachment — 附件上传

> **STOP — 不要使用钉盘 (drive) 上传！** 钉盘 fileId 无法写入 attachment 字段。必须使用以下流程。
>
> 本地文件用 `+attachment-put`（上传并写入已有记录）或以下上传流程；已有可直接下载的在线 URL，可在原子 `record create/update` 的附件字段直接传 `[{"url":"https://..."}]`。URL 转存为 best-effort 异步操作，成功回执只表示已受理，需独立回读并确认附件可用。

## 准备附件上传

```
Usage:
  dws aitable attachment upload [flags]
Example:
  dws aitable attachment upload --base-id <BASE_ID> --file-name report.xlsx --size 204800
  dws aitable attachment upload --base-id <BASE_ID> --file-name photo.png --size 1024 --mime-type image/png
Flags:
      --base-id string     Base ID (必填)
      --file-name string   文件名，必须含扩展名 (必填)
      --size int           文件大小（字节），>0 (必填)
      --mime-type string   MIME type（不传时根据扩展名推断）
```

## 附件上传完整流程（推荐：使用脚本，2 步完成）

```bash
# 步骤 1: 使用脚本一键上传（内部自动完成 prepare + PUT）
python3 scripts/upload_attachment.py <BASE_ID> /path/to/report.pdf
# 输出: { "fileToken": "ft_xxx", "fileName": "report.pdf", "size": 204800 }

# 步骤 2: 在 record create/update 中使用 fileToken 写入
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```

> `uploadUrl` 有时效性（`expiresAt`），脚本会自动在获取后立即上传。

## 手动流程（不使用脚本）

```bash
# 1. 获取上传凭证
dws aitable attachment upload --base-id <BASE_ID> --file-name report.pdf --size 204800 --format json
# → 返回 uploadUrl、fileToken

# 2. PUT 上传（Content-Type 必须是文件的具体 MIME type）
curl -X PUT "<uploadUrl>" -H "Content-Type: application/pdf" --data-binary @report.pdf

# 3. 写入记录
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"recXXX","cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```

## 在线 URL 写入

```bash
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"<RECORD_ID>","cells":{"<FIELD_ID>":[{"url":"https://example.com/report.pdf"}]}}]' --format json
```

附件写入会整体覆盖原列表；传 `[]` 或 `null` 表示清空。同一批 `update_records` 不能混合附件清空与新增/替换，应分开请求。不要将临时下载链接当作持久资源身份，也不要仅凭转存受理回执宣称文件已可下载。

## 移除附件

- 清空字段：`dws aitable +attachment-remove --base-id <B> --table-id <T> --record-id <R> --field-id <F> --clear-all`。
- 按文件名移除：同一命令改用 `--remove-name <文件名>`，从回读结果解析真实 resourceId 后删除并验证；目标缺 resourceId 时仅在剩余项都有 fileToken 的情况下使用替换路径，否则停止。
- 已知 resourceId：`dws aitable attachment remove --base-id <B> --table-id <T> --record-id <R> --field-id <F> --resource-ids <RESOURCE_IDS>`，随后独立回读。

按 Runtime 要求确认删除范围后执行。服务端删除仍有并发覆盖窗口，避免同时修改同一附件字段。
