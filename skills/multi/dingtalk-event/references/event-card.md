# 互动卡片回调事件

先读事件产品入口 [SKILL.md](../SKILL.md) 的命令规则、生命周期和失败处理。本参考覆盖公开个人事件 `user_card_action_triggered`。

<!-- dws-intent: event.listen.card -->互动卡片业务发生回调时，使用 `dws event consume` 长连接实时监听；不要轮询卡片状态模拟事件。

## 订阅规则

- 使用当前用户 OAuth 身份，非默认组织加全局 `--profile <corpId 或 profile 名>`。
- 注册复用 IM 的 `POST /dws/subscription/user`，取消复用 `POST /dws/subscription/cancel`。
- 事件使用 `ruleType=all`，并与 IM/OA 全量事件一致发送 `filterRule={}`。
- 不接受 `--user`、`--open-dingtalk-id`、`--group`、`--query`、`--filter-json` 或 `--role-types`。

## Golden Route

```bash
dws event list --category card
dws event schema user_card_action_triggered --flatten
dws event consume user_card_action_triggered --flatten -f ndjson
```

临时验证可加 `--max-events 1` 或 `--duration 10m`。等待 `[event] ready event_key=user_card_action_triggered ...` 后再触发卡片交互；干净退出会自动取消本次新建的订阅。复用既有订阅时使用 `--subscribe-id`，外部停止先运行 `dws event stop <subscribe_id> --dry-run`，确认后再加 `--yes`。

## 扁平输出

`--flatten` 保持以下顶层结构稳定，并为当前已评审的真实回调描述 `payload` 字段；各层仍允许服务端增加未知字段：

```json
{
  "type": "user_card_action_triggered",
  "event_id": "evt_xxx",
  "timestamp": 1700000000123,
  "subscribe_id": "subId_xxx",
  "payload": {
    "body": {
      "actionData": {
        "context": {
          "answers": {
            "q0": {"custom": "", "selected": ["o1"]},
            "q1": {"selected": []}
          },
          "createUid": "10001",
          "orgId": "20001",
          "outcome": "answered",
          "questions": [
            {
              "id": "q0",
              "header": "会议标题",
              "prompt": "请选择会议类型",
              "selection": "single",
              "allowCustom": true,
              "options": [
                {"id": "o1", "label": "项目讨论", "description": "项目相关讨论会议"}
              ]
            },
            {
              "id": "q1",
              "header": "参会人",
              "prompt": "请选择参会人",
              "selection": "multiple",
              "allowCustom": false,
              "inputKind": "person",
              "options": []
            }
          ],
          "sourceProjectionVersion": "projection-v1",
          "sourceTurnId": "turn_xxx"
        }
      },
      "bizInfoDTO": {"appKey": "app_xxx", "bizId": "biz_xxx"},
      "context": {"answers": "{...}", "questions": "[...]"},
      "conversationContextDTO": {"cid": "cid_xxx"},
      "extension": {"protocolProfile": "profile_xxx"},
      "operatorDTO": {"operatorUserAgent": "client_xxx", "uid": 10001},
      "spaceId": "space_xxx",
      "spaceType": "im_single",
      "triggerTimestamp": 1700000000100
    },
    "event_time": 1700000000100
  }
}
```

读取互动结果时遵循这些规则：

- 结构化业务上下文优先读取 `payload.body.actionData.context`。通过 `questions[].id` 找到 `answers[question_id]`；`selected` 保存选项 ID，再通过同一问题的 `options[].id` 转换为标签，`custom` 独立保留。
- `selected: []` 是合法的未选择状态。`outcome`、`selection`、`inputKind` 和 `spaceType` 都是开放字符串，不据当前样本推断封闭枚举。
- `createUid`、`orgId` 是字符串；操作者读取整数 `operatorDTO.uid`，不要互相转换或用 `createUid` 替代操作者。
- `bizInfoDTO.bizId`、`sourceTurnId`、`spaceId` 和 `conversationContextDTO.cid` 可用于业务关联，但都按不透明 ID 原样保留，不拆解其格式。
- 顶层 `timestamp`、`payload.event_time`、`body.triggerTimestamp` 都是毫秒时间戳，分别保留，不假设三者始终相等。
- `body.context.answers/questions` 是 JSON 字符串形式的兼容副本，只在结构化字段缺失时回退；`extension` 中的 JSON 字符串和 `operatorUserAgent` 主要用于兼容或诊断，也不应优先于结构化字段。

`payload`、`body`、上下文和扩展对象都会保留服务端推送的未知字段，以兼容后续协议扩展。需要业务字段时以实时 payload 和 `dws event schema user_card_action_triggered --flatten` 为准。不传 `--flatten` 时保持兼容 transport envelope，业务数据仍在字符串 `.data` 内。

该事件与其它无目标个人事件可放入同一个 consume，共享 personal bus；每个 EventKey 仍创建独立订阅。启动中任一订阅失败时，现有多事件生命周期会回滚本次已创建项，并沿用同一重试保护。
