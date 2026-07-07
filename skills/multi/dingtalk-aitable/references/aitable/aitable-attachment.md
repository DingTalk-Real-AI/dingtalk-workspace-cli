# attachment — 附件上传

> **STOP — 不要使用钉盘 (drive) 上传！** 钉盘 fileId 无法写入 attachment 字段。必须使用以下流程。

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

# 2. PUT 上传（Content-Type 留空）
curl -X PUT "<uploadUrl>" -H "Content-Type:" --data-binary @report.pdf

# 3. 写入记录
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"recXXX","cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```
