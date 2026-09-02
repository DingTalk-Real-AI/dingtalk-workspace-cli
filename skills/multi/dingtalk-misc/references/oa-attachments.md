# OA 审批附件

仅在用户要上传审批附件、获取附件链接或授权下载/预览时读取本文件。先读 [oa.md](oa.md)，确保实例来自用户指定的角色来源。

## 目录

- [选路](#选路)
- [目标与 ID 核对](#目标与-id-核对)
- [获取临时下载链接](#获取临时下载链接)
- [授权下载与预览](#授权下载与预览)
- [上传并用于创建审批](#上传并用于创建审批)
- [错误和成功判定](#错误和成功判定)

## 选路

| 用户目标 | 命令 | 成功含义 |
|---|---|---|
| 获取单个附件临时下载链接 | `dws oa approval attachment download-url` | 返回可用的临时 URL；不会保存文件 |
| 为当前用户开通钉盘文件下载权限 | `dws oa approval attachment authorize-download` | 授权接口明确成功；不会返回下载链接 |
| 在审批场景内授权预览附件 | `dws oa approval attachment authorize-preview` | 预览授权明确成功；不等于下载权限 |
| 上传本地文件作为待提交审批附件 | `dws oa approval attachment upload` | 返回完整附件元数据；不等于审批已创建 |

不要因为相邻能力成功就宣称目标能力成功。`download-url`、`authorize-download` 和 `authorize-preview` 是三种不同结果。

## 目标与 ID 核对

1. 从指定角色来源选中审批实例，不跨 `pending`、`cc`、`executed`、`submitted` 或 admin 来源替代。
2. 调用 `dws oa approval detail --instance-id <id> --format json`，从真实详情取得附件名称、`fileId`、需要时的 `spaceId`。
3. 核对流程名、发起人、状态、角色来源和用户要求的附件名称。
4. 评论附件只有在详情明确表明附件来自评论时才增加 `--with-comment-attachment`。

禁止猜测或复用其他实例的 `fileId` / `spaceId`。

## 获取临时下载链接

```bash
dws oa approval attachment download-url \
  --instance-id <processInstanceId> \
  --file-id <fileId> \
  --format json
```

该命令只返回临时下载链接。链接含签名参数，展示或使用时完整保留 `&` 等参数；不要声称文件已下载到本地。

评论附件增加：

```bash
--with-comment-attachment
```

## 授权下载与预览

### 下载授权

```bash
dws oa approval attachment authorize-download \
  --file-infos '[{"spaceId":27827223951,"fileId":"232271651278"}]' \
  --format json
```

- `fileInfos` 必须包含 1～10 项，最多 10。
- 每项使用详情真实返回的数字 `spaceId` 和字符串 `fileId`。
- 成功后若用户还需要链接，再单独调用 `download-url`。

### 预览授权

```bash
dws oa approval attachment authorize-preview \
  --instance-id <processInstanceId> \
  --file-ids <fileId1>,<fileId2> \
  --format json
```

- `fileIds` 最多 20 项且每项非空。
- 评论附件增加 `--with-comment-attachment`。
- 预览授权成功不能替代下载授权。

## 上传并用于创建审批

```bash
dws oa approval attachment upload --file <本地路径> --format json
```

可选 `--file-name`；`--md5` 不传时由 CLI 自动计算。命令完成初始化、OSS PUT 和提交入库，成功返回 `fileId`、`spaceId`、`fileName`、`fileSize`、`fileType`。

将这些真实字段按 [oa-form-components.md](oa/oa-form-components.md) 的 `DDAttachment` 格式写入创建 payload，再回到 [oa-create.md](oa-create.md) 完成预测、确认和创建。上传成功只表示附件入库，不表示审批实例已创建。

## 错误和成功判定

- 参数错误只按 Help/Schema 的明确提示纠正一次。
- 业务错误只有明确 `retryable=true` 才按服务端节奏重试；更换 `spaceId/fileId` 组合不能重置预算。
- `authorize-download` 返回失败后，不得用成功的 `download-url` 冒充授权成功。
- 没有目标来源证明时停止，不跨角色列表寻找可访问附件。
- 创建后只有详情中的 `DDAttachment` 实际回读到文件名、`fileId` 等附件值，才能说附件已验证；字段为空、`"[]"` 或缺失时只能说实例已创建、附件未验证。
- 最终答复分别说明：目标实例、附件名称、执行的确切能力、成功字段或真实错误，以及是否完成用户要求。
