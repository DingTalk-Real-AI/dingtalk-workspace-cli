# A2UI 展示卡片

使用 `dws chat message send-a2ui-card` 创建卡片，使用 `update-a2ui-card` 更新和完结。`--content` 接受 JSON 字符串数组，每个元素是一条 A2UI 消息；传 `-` 时从 stdin 读取同样的内容。

## 回复占位与主动发送

回复用户消息，或处理明确需要交互反馈的事件任务时，默认先发 A2UI 占位卡，再按实际进度更新同一卡片，不要求用户额外指定“流式”。简短回复从占位直接更新为答案，不编造阶段。事件发生本身不构成回复授权。

主动通知、定时日报及完整结果交付默认准备好内容后直接发送，不自动占位或流式；仅在用户明确要求持续展示进度时启用。占位和进度使用可信的当前回复目标与 profile，不根据正文猜测 ID，也不跟随最终交付目标转移。例如私聊交办、报告发到群里时，进度留在私聊，群里默认只收完整报告。

接入 Agent 时，由接入方在确认回复范围、完成去重及目标校验后，通过任务启动钩子调用 `send-a2ui-card`，在调用模型前发送“思考中…”占位，并将公开进度和结果通过 `update-a2ui-card` 更新到同一卡片。钩子和进度采集需接入方实际实现，安装 Skill 不会自动注册。宿主负责卡片时，模型只输出公开进度和结果，不重复创建卡片或发送最终消息；没有宿主钩子时可由模型尽早调用命令，但不能保证模型首次响应前已有占位。不展示内部推理、凭据或工具原始输入输出。

用户只要求最终结果时，省略过程详情；明确不要中间消息或卡片时，还需跳过占位。模型自行发卡时按此选择；宿主发卡时须在钩子前确定回复策略，不能靠模型事后取消首卡。

## 流式执行

以下命令由宿主或模型中的同一方负责执行，不在同一任务中重复接管。已有卡片的交互事件先核实创建记录和更新权限，按业务更新原卡，不默认另建占位卡。

1. 开始执行时调用 `send-a2ui-card`，发送“思考中…”初始布局和运行状态。创建默认 `PROCESSING`；保存服务端返回的真实 `bizId` 和创建命令，后续沿用 `update-a2ui-card`，不能使用请求侧 ID 或 `openTaskId` 替代 `bizId`。
2. 每生成可展示的完整段落或阶段结果，调用 `update-a2ui-card`，使用 `INPUTTING`。每批提交完整有效的 JSON 消息，不发送未闭合 JSON 或逐 token 拼接请求。
3. 使用同一 profile、真实 `bizId` 和 `surfaceId`，已有组件保持原 ID。正文用 `updateDataModel` 提交该绑定路径的累计正文，它会替换路径值，不会追加字符；不要重放无关数据或表单默认值。布局变化用 `updateComponents` 提交完整组件定义，新增组件使用新 ID 并同步维护父组件 `children`，更新批次不再发送 `createSurface`。
4. 同一卡片串行提交，上一批明确成功后再发下一批。更新失败保留原卡，不自动重新创建；请求超时或响应丢失导致结果未知时，暂停后续更新并核实该批是否生效，不能盲目重试。
5. 任务成功完成时，提交非空有效的最终更新并设为 `FINISH`；涉及正文路径时提交累计最终值，无需重发未变内容。已知执行失败、取消或超时，且卡片仍可安全更新时，保留已有结果，用状态文案说明未完成及原因，再以 `FINISH` 结束输出；`FINISH` 不代表业务成功。卡片更新结果未知时停止写入，不强行补发终态。

创建失败或 A2UI 能力不可用时，先确认是否投递：明确未创建、回复目标和权限有效且任务允许文本交付时，可改用最终文本；明确要求 A2UI 时报告限制。超时、响应丢失或缺少真实 `bizId` 均按结果未知处理，停止写入并核实，不自动重建或补发文本。不得通过回退绕过身份、权限或目标校验。宿主是否自动回退须以实际接入实现为准。

这里的流式更新通过多次调用命令实现，Skill 本身不执行钩子或补发。Agent 被强制终止后不能保证自动收尾；无法确认更新结果时应报告已完成的阶段与未验证的状态。

主动完整发送 A2UI 时，创建请求直接携带完整内容，不添加占位或过程阶段；创建成功后仍需沿用真实 `bizId`，提交非空有效的最终更新并设为 `FINISH`。这是协议完结，不需要经历 `INPUTTING` 或分批生成内容。

运行中的面板标题可使用钉钉宿主约定 `[AIGenerating]` 和 `textEffect: shimmer`；仅设置 `PROCESSING`/`INPUTTING` 不会自动给组件添加动画。结束时更新完整面板定义，移除生成标记和文字特效，保持组件 ID，不重设展开状态。

下面以任务进度卡片为例。布局、组件 ID、数据路径和文案可按需求调整，保持引用一致；组件属性可按需查阅[钉钉公开 Catalog](https://dingtalk.com/card/a2ui/catalogs/public/catalog.json)。

## 创建

用 `createSurface` 指定 Catalog 和初始数据，再用 `updateComponents` 定义组件。替换接收人后发送：

```bash
dws chat message send-a2ui-card \
  --open-dingtalk-id '<接收人openDingTalkId>' \
  --content '["{\"version\":\"v1.0\",\"createSurface\":{\"surfaceId\":\"example-card\",\"catalogId\":\"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json\",\"dataModel\":{\"answer\":{\"displayText\":\"任务已开始。\"},\"status\":\"doing\"}}}","{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"root\",\"component\":\"Column\",\"align\":\"stretch\",\"children\":[\"executionPanel\"]},{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"[AIGenerating]思考中…\",\"textEffect\":\"shimmer\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"},{\"id\":\"answer\",\"component\":\"Markdown\",\"content\":{\"path\":\"/answer/displayText\"}}]}}"]' \
  -f json
```

群聊时将 `--open-dingtalk-id` 换成 `--conversation-id '<群openConversationId>'`，选择一个目标。创建状态默认 `PROCESSING`；发送成功后保存返回的 `bizId`，用于后续更新。

允许转发时，在创建命令中加上 `--support-forward`，默认 `false`。

## 更新和完结

更新使用创建时的 `bizId` 和 `surfaceId`。本例第一批更新面板标题，写入已生成正文；示例步骤和文案需替换为实际执行结果：

```bash
dws chat message update-a2ui-card \
  --biz-id '<本次发送返回的bizId>' \
  --flow-status INPUTTING \
  --content '["{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"[AIGenerating]正在整理分析结果\",\"textEffect\":\"shimmer\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"}]}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/answer/displayText\",\"value\":\"已完成资料读取。\"}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/status\",\"value\":\"doing\"}}"]' \
  -f json
```

第二批正文增长时，发送包含上一批正文的累计值，保持 `INPUTTING`。本批不修改布局，因此只更新数据：

```bash
dws chat message update-a2ui-card \
  --biz-id '<本次发送返回的bizId>' \
  --flow-status INPUTTING \
  --content '["{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/answer/displayText\",\"value\":\"已完成资料读取。\\n\\n已整理出分析结果。\"}}"]' \
  -f json
```

可按同样方式继续分批更新。任务完成后写入完整实际结果，并将 `--flow-status` 设为 `FINISH`：

```bash
dws chat message update-a2ui-card \
  --biz-id '<本次发送返回的bizId>' \
  --flow-status FINISH \
  --content '["{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"处理完成\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"}]}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/answer/displayText\",\"value\":\"已完成资料读取。\\n\\n已整理出分析结果。\\n\\n任务已完成。\"}}","{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"example-card\",\"path\":\"/status\",\"value\":\"finished\"}}"]' \
  -f json
```

`--flow-status` 控制卡片流转状态；示例中的 `dataModel.status` 用于记录业务状态，可按需要维护。

## 组件注解

发送或更新时可传 `--a2ui-annotations '[{"surfaceId":"example-card","componentId":"answer","type":"artifact"}]'`。注解为 JSON 对象数组，与 A2UI 消息通过以下字段对应：

- `surfaceId` 对应消息中的 surface。
- `componentId` 对应该 surface 下 `updateComponents.components[].id`，按需要关联的实际组件填写。
- `type` 表示注解类型，按实际支持的类型和用途选择。

本例的 `componentId` 为 `answer`，`type` 为 `artifact`，不要把组件 ID 当作注解类型。更新时也可关联此前已创建的组件。该参数可选，支持 `[]`；省略时创建请求不携带注解字段，更新请求沿用原有的空数组。

发送后核对创建结果和客户端展示，再检查同一 `bizId` 的更新和完结效果。遇到 `a2ui dws catalog validation failed` 时，可对照 Catalog 检查组件属性、JSON 格式、Catalog 地址和 surface 创建顺序。
