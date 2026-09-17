# A2UI 展示卡片

使用 `dws chat message send-a2ui-card` 创建卡片，使用 `update-a2ui-card` 更新和完结。`--content` 接受 JSON 字符串数组，每个元素是一条 A2UI 消息。

下面以任务进度卡片为例。布局、组件 ID、数据路径和文案可按需求调整，保持引用一致；组件属性可按需查阅[钉钉公开 Catalog](https://dingtalk.com/card/a2ui/catalogs/public/catalog.json)。

## 创建

用 `createSurface` 指定 Catalog 和初始数据，再用 `updateComponents` 定义组件。替换接收人后发送：

```bash
dws chat message send-a2ui-card \
  --open-dingtalk-id '<接收人openDingTalkId>' \
  --content '["{\"version\":\"v1.0\",\"createSurface\":{\"surfaceId\":\"example-card\",\"catalogId\":\"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json\",\"dataModel\":{\"answer\":{\"displayText\":\"任务已开始。\"},\"status\":\"doing\"}}}","{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"root\",\"component\":\"Column\",\"align\":\"stretch\",\"children\":[\"executionPanel\"]},{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"正在处理任务\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"},{\"id\":\"answer\",\"component\":\"Markdown\",\"content\":{\"path\":\"/answer/displayText\"}}]}}"]' \
  -f json
```

群聊时将 `--open-dingtalk-id` 换成 `--conversation-id '<群openConversationId>'`，选择一个目标。创建状态默认 `PROCESSING`；发送成功后保存返回的 `bizId`，用于后续更新。

允许转发时，在创建命令中加上 `--support-forward`，默认 `false`。

## 更新和完结

更新使用创建时的 `bizId` 和 `surfaceId`。本例修改面板标题和正文：

```bash
dws chat message update-a2ui-card \
  --biz-id '<本次发送返回的bizId>' \
  --flow-status INPUTTING \
  --content '["{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"正在处理第 2 步\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"}]}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/answer/displayText\",\"value\":\"第 2 步正在执行。\"}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/status\",\"value\":\"doing\"}}"]' \
  -f json
```

可按任务进度继续更新。任务完成后写入实际结果，并将 `--flow-status` 设为 `FINISH`：

```bash
dws chat message update-a2ui-card \
  --biz-id '<本次发送返回的bizId>' \
  --flow-status FINISH \
  --content '["{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"处理完成\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"}]}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/answer/displayText\",\"value\":\"任务已完成。\"}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/status\",\"value\":\"finished\"}}"]' \
  -f json
```

`--flow-status` 控制卡片流转状态；示例中的 `dataModel.status` 用于记录业务状态，可按需要维护。

## 组件注解

发送或更新时可传 `--a2ui-annotations '[{"surfaceId":"example-card","componentId":"answer","type":"artifact"}]'`。注解为 JSON 对象数组，与 A2UI 消息通过以下字段对应：

- `surfaceId` 对应消息中的 surface。
- `componentId` 对应该 surface 下 `updateComponents.components[].id`，按需要关联的实际组件填写。
- `type` 表示注解类型，按实际支持的类型和用途选择。

`answer` 和 `artifact` 是本例的取值。更新时也可关联此前已创建的组件。该参数可选，支持 `[]`；省略时创建请求不携带注解字段，更新请求沿用原有的空数组。

发送后核对创建结果和客户端展示，再检查同一 `bizId` 的更新和完结效果。遇到 `a2ui dws catalog validation failed` 时，可对照 Catalog 检查组件属性、JSON 格式、Catalog 地址和 surface 创建顺序。
