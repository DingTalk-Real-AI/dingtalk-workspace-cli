# minutes Lite Recipe

> 完整命令路由见 [minutes-index.md](./minutes-index.md)，高级会后流程见 [07-minutes.md](./07-minutes.md)。

## minutes-query

- 默认范围：`minutes list all --format json`
- 仅自己创建：`minutes list mine --format json`
- 仅他人共享：`minutes list shared --format json`
- 关键词与时间：在同一条 `list` 命令中使用 `--query`、`--start`、`--end`
- 单篇详情：`get info/summary/keywords/todos --id <taskUuid>`
- 多篇基础信息：`get batch --ids <uuid1,uuid2,...>`
- 原文：`get transcription --id <taskUuid>`，分页 token 非空时继续

对象选择必须使用本次 `list` 返回的 taskUuid，并同时核对标题、组织和时间。多候选时让用户确认。

## minutes-edit

- 修改标题：`minutes update title --id <taskUuid> --title "<新标题>"`
- 修改摘要：`minutes update summary --id <taskUuid> --content "<内容>"`
- 替换文字：`minutes replace-text --id <taskUuid> --search "<旧文字>" --replace "<新文字>"`
- 添加热词：`minutes hot-word add --words "词1,词2"`
- 替换发言人：确认对应关系后执行 `minutes speaker replace`

所有写操作先定位唯一 taskUuid，完成后回读验证。

## minutes-tag

1. `minutes tag list --format json` 获取真实 tagId。
2. 按名称匹配标签。
3. `minutes tag query --tag-id <tagId> --format json`；有分页时继续传 `--cursor`。

## minutes-permission

- 添加成员：`minutes permission add --ids <uuid1,uuid2> --member-uids <uid1,uid2> --policy <0-4>`
- 移除成员：`minutes permission remove --ids <uuid1,uuid2> --member-uids <uid1,uid2>`

成员 ID 先通过通讯录查询，禁止编造。

## minutes-upload

```bash
dws minutes upload create --file-name "meeting.mp3" --file-size <bytes> --format json
dws minutes upload complete --session-id <sessionId> --format json
dws minutes upload cancel --session-id <sessionId> --format json
```

使用 `create` 返回的 sessionId 和上传地址；文件大小单位是字节。

## 数据深度

- 只要列表时不要额外拉取转写。
- 要摘要时读取 `get summary`。
- 要原话、逐字稿或沟通细节时读取 `get transcription` 并翻页至结束。
- 要待办时读取 `get todos`；缺少责任人或期限时先确认。
- 多数据源任务分别读取各来源并如实标注缺失项。
