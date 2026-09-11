# AI 表格记录评论

本 Reference 仅适用于 `dws aitable comment ...`。它操作的是绑定到一条 AI 表格记录的评论，不是在线电子表格单元格批注，也不修改记录字段。

## 命令

| 意图 | 命令 | 关键参数 |
|---|---|---|
| 分页查询评论与回复 | `comment list` | `--base-id --table-id --record-id [--limit 1..100] [--cursor <nextToken>]` |
| 创建评论话题 | `comment create` | 定位参数 + `--content <文本>` 或 `--rich-content <JSON>` |
| 回复已有评论 | `comment reply` | 定位参数 + `--topic-id --comment-key` + 正文 |
| 完整替换本人评论正文 | `comment update` | 定位参数 + `--topic-id --comment-key` + 正文 |
| 删除本人评论 | `comment delete` | 定位参数 + `--topic-id --comment-key`；确认后再追加 `--yes` |

`--content` 是纯文本便捷入口，CLI 会转换成单个 text 节点。`--rich-content` 是 1～100 个有序节点组成的 JSON 数组：

```json
[
  {"type":"text","text":"请确认截图 "},
  {"type":"mention","userId":"<USER_ID>","corpId":"<CORP_ID>"},
  {"type":"image","url":"/core/api/resources/<RESOURCE_ID>/detail","width":800,"height":600}
]
```

- text 总长度最多 10000 个 UTF-16 字符。
- mention 最多 20 个；`userId` 必须是外部用户 ID，`corpId` 省略时使用当前调用企业。
- image 最多 9 个；只接受当前 Base 已上传图片的 `/core/api/resources/<resourceId>/detail` 路径。命令不负责上传或转存图片，宽高是可选的 1～20000 整数。
- `--content` 与 `--rich-content` 必须且只能提供一个；update 会完整替换正文，仅传纯文本会移除旧的 @和图片。

## 稳定标识与分页

`topicId`、`commentKey` 必须来自同一 `baseId/tableId/recordId` 的 create/list 真实返回。reply 的 CLI 参数仍叫 `--comment-key`，底层会按接口协议发送为 `replyCommentKey`，不要自行猜测或拼接标识。

list 的 `comments=[]` 不代表结束。只有 `hasMore=false` 才能停止；`hasMore=true` 时保持三项定位参数不变，将 `nextToken` 原样作为下一次 `--cursor`。过滤后空页是正常结果。

## 权限与失败处理

- list 要求 Base 预览权限且目标记录可见；写操作要求 Base 编辑权限。
- 当前只支持新评论服务；旧评论服务返回 `COMMENT_LEGACY_UNSUPPORTED` 时停止，不自动迁移或改走其他存储。
- create/reply 非幂等，所有评论写操作都不会自动重试。超时、连接中断或结果未知时先 list 对账，再决定后续动作。
- update 无 CAS，并发修改存在最后写入覆盖窗口。
- delete 不可恢复，只能删除本人评论；关联回复如何处理由评论服务决定，执行前必须独立确认目标和影响。
