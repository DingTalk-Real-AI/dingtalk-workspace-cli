# 听记核心工作流与上下文

## 核心工作流

```bash
# 0. 发起听记（开始录音）
dws minutes record start --format json

# 1. 查看我的听记列表 — 提取 taskUuid
dws minutes list mine --format json
dws minutes list mine --limit 10 --cursor <nextToken> --format json
dws minutes list mine --query "周会" --format json

# 1b. 查看共享给我的听记
dws minutes list shared --limit 20 --format json
dws minutes list shared --query "日报" --format json

# 1c. 查看我有权限访问的所有听记（支持关键字和时间范围筛选）
dws minutes list all --format json
dws minutes list all --query "周会" --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00" --format json

# 2. 获取 AI 摘要
dws minutes get summary --id <taskUuid> --format json

# 3. 查看完整转写原文（拉完后默认按时间线返回，AI 必须主动追问"是否按发言人聚类"）
dws minutes get transcription --id <taskUuid> --format json
# 3a. 用户确认聚类 → AI 在本地按 speakerNick 分组并提取核心要点（无需新调用 dws）
# 3b. 用户提供"某某人讲了 XX" → AI 模糊匹配关键词后，引导确认替换发言人
dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总" --format json

# 4. 提取待办事项
dws minutes get todos --id <taskUuid> --format json

# 4b. 获取音频/视频地址（用于下载或播放原始媒体文件）
dws minutes get audio --id <taskUuid> --format json

# 5. 修改标题
dws minutes update title --id <taskUuid> --title "新标题" --format json

# 6. 更新纪要内容
dws minutes update summary --id <taskUuid> --content "新的纪要内容" --format json

# 7. 录音控制（基于 start 返回的 taskUuid）
dws minutes record pause --id <taskUuid> --format json
dws minutes record resume --id <taskUuid> --format json
dws minutes record stop --id <taskUuid> --format json

# 8. 思维导图
dws minutes mind-graph create --id <taskUuid> --format json
dws minutes mind-graph status --id <taskUuid> --format json

# 9. 替换发言人
dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四" --format json

# 9b. 发言人段落总结（异步：create 触发 → 延迟 5s → 轮询 get，最多 20 次）
dws minutes speaker summary create --ids <taskUuid1,taskUuid2> --format json
# 等待至少 5 秒...
dws minutes speaker summary get --ids <taskUuid1,taskUuid2> --format json
# 若返回为空，继续等待 5s 后重试，最多 20 次

# 10. 添加个人热词
dws minutes hot-word add --words "OKR,钉钉,Copilot" --format json

# 11. 查找替换听记文字
dws minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字" --format json

# 12. 文件上传转听记（三步流程）
# 12a. 创建上传会话，获取预签名 URL 和 sessionId
dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --format json
# 12b. 用 HTTP PUT 上传文件到预签名 URL（不带 HEADER）
curl -X PUT "<presignedUrl>" -T "/path/to/meeting.mp4"
# 12c. 通知服务端上传完成，创建听记
dws minutes upload complete --session-id <sessionId> --format json
# 12d.（可选）取消上传会话
dws minutes upload cancel --session-id <sessionId> --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list mine` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --cursor |
| `list shared` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --cursor |
| `list all` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --cursor |
| `get batch` | 各听记 `taskUuid` | 进一步查询详情 |
| `get audio` | 音频/视频 OSS 地址 | 用 HTTP GET 下载录音文件 / 在浏览器播放 |
| `record start` | `taskUuid`/`uuid` | record pause/resume/stop 的 --id |
| `upload create` | `sessionId`、`presignedUrl` | HTTP PUT 上传文件；upload complete/cancel 的 --session-id |
| `mind-graph create` | 任务状态 | mind-graph status 轮询 |
