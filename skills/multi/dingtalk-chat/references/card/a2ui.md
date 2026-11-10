# A2UI 展示卡片

使用 `dws chat message send-a2ui-card` 创建卡片，使用 `update-a2ui-card` 更新和完结。`--content` 接受 JSON 字符串数组，每个元素是一条 A2UI 消息；传 `-` 时从 stdin 读取同样的内容。

## 卡片摘要与转发

创建时用 `--summary` 传一句简短可读的降级文案，客户端无法渲染卡片时展示；它不是完结后的会话摘要，后者见[组件注解](#组件注解)。不要传序列化消息、模板摘要或实例 ID；省略时为 `互动卡片`，显式空值或纯空白会被拒绝。`send-a2ui-card` 的 `--support-forward` 默认 `true`，创建的卡片允许转发；需要禁止转发时显式传 `--support-forward=false`；转发不授予业务操作权限。两者只能在创建时设置，`update-a2ui-card` 没有对应参数，`dataModel.summary`、`fallbackMarkdown` 也不能代替。统一入口 `chat +messages-send --a2ui-messages` 对应 `--card-summary` 和 `--support-forward`，其转发默认值仍为 `false`。

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
  --summary "任务进度与结果" \
  --content '["{\"version\":\"v1.0\",\"createSurface\":{\"surfaceId\":\"example-card\",\"catalogId\":\"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json\",\"dataModel\":{\"answer\":{\"displayText\":\"任务已开始。\"},\"status\":\"doing\"}}}","{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"root\",\"component\":\"Column\",\"align\":\"stretch\",\"children\":[\"executionPanel\"]},{\"id\":\"executionPanel\",\"component\":\"CollapsiblePanel\",\"title\":\"[AIGenerating]思考中…\",\"textEffect\":\"shimmer\",\"children\":[\"answer\"],\"fallbackMarkdown\":\"任务进度\"},{\"id\":\"answer\",\"component\":\"Markdown\",\"content\":{\"path\":\"/answer/displayText\"}}]}}"]' \
  -f json
```

群聊时将 `--open-dingtalk-id` 换成 `--conversation-id '<群openConversationId>'`，选择一个目标。创建状态默认 `PROCESSING`；发送成功后保存返回的 `bizId`，用于后续更新。

示例默认允许转发；需要禁止转发时在创建命令中加上 `--support-forward=false`。显式 `--support-forward=true` 仍允许转发。

## @ 成员或所有人

群聊卡片的 Markdown 组件支持 @ 提醒。正文中写 `<a atId="openDingTalkId">昵称</a>` @ 成员，写 `<a atId="all">everyone</a>` @ 所有人；同时必须通过发送参数声明：

- 每个成员的 openDingTalkId 用 `--at-open-dingtalk-ids` 声明（可逗号分隔或重复传参），发送时透传为请求的 `atOpenDingTalkIds`；声明总数含重复不超过 100。
- @ 所有人必须加 `--at-all`，发送时透传为 `atAll: true`；保留值 `all` 只出现在正文，不进入 `atOpenDingTalkIds`。线上请求中 CLI 会把正文的 `<a atId="all">` 改写为服务端高亮别名 `<a atId="10">`，本地文件与校验仍使用 `all`。

通知校验只在显式传入 @ 参数时启用：可静态确定的 Markdown 标签与声明不一致时 exit 3。支持字面量及同批 `createSurface.dataModel` / `updateDataModel` 中可解析的绝对 `content.path`；合法绑定允许创建时先通知、后续填入正文，CLI 不执行运行时表达式，也不能预知后续绑定值。行内代码、围栏代码、缩进代码及转义的标签示例不作为 @。

不传 @ 参数时保留既有正文和默认摘要，群聊、单聊均不做新增通知校验或标签改写；标签本身不保证通知。单聊仍拒绝显式 @ 参数。创建且显式 `--at-all` 时，仅改写已确定的 Markdown 字符串值，保留其他 JSON 字段顺序、空白和未改动转义；摘要仍来自原始消息。

服务端按声明发通知、失败即拒：声明的 id 无效或无法解析、声明超限、单聊携带 @ 或别名冲突时，服务端返回 INVALID_PARAM（MENTION_OPEN_ID_INVALID / MENTION_RESOLUTION_FAILED / MENTION_LIMIT_EXCEEDED / MENTIONS_GROUP_ONLY / MENTION_ALIAS_CONFLICT 等）拒绝建卡，命令退出码非 0。服务端只认声明、不扫描正文：正文用了但未声明的 atId 既不高亮也不通知，因此 CLI 在发送前按上段一致性校验拦截。

```bash
dws chat message send-a2ui-card \
  --conversation-id '<群openConversationId>' \
  --at-open-dingtalk-ids '<成员openDingTalkId>' \
  --at-all \
  --content '["{\"version\":\"v1.0\",\"updateComponents\":{\"surfaceId\":\"example-card\",\"components\":[{\"id\":\"answer\",\"component\":\"Markdown\",\"content\":\"<a atId=\\\"<成员openDingTalkId>\\\">张三</a> <a atId=\\\"all\\\">everyone</a> 请查看任务进度。\"}]}}"]' \
  -f json
```

静态匹配使用 `atId` 为首个且唯一属性、值使用双引号的 `<a atId="...">` 写法；绑定路径仅解析同批已提供数据。无 @ 参数时沿用既有透传行为；显式通知时，内层消息 JSON 无效会在发送前拒绝。

`aicard lint --preflight ... --at-open-dingtalk-ids ...` 仅检查成员声明与正文的一致性以及 IM 占位符误用；它没有 `--at-all`，不检查所有人标签配对或 100 个声明上限，不能代替发送入口的完整预检。

`--at-all` 沿用现有卡片发送入口的确认策略，不新增交互确认；发送者须有群聊提醒授权。群设置拒绝 @所有人时，以服务端实际错误为准，不应假定已发送成功。正式发布前必须确认服务端已支持 `atOpenDingTalkIds`/`atAll` 及 fail-closed 契约，并执行真实通知验收；旧服务端可能忽略字段，出现有标签却未通知的结果。

统一发送命令 `dws chat +messages-send --as user --a2ui-messages ...` 支持同样的 `--at-open-dingtalk-ids`/`--at-all` 参数与一致性校验。更新卡片（`update-a2ui-card`）不接受 @ 参数。更新保留输入消息，不重新推断通知对象或改写任意数据模型字符串。

## 流式 @：固定通知栏与绑定正文

推荐在创建时用独立 Markdown 组件展示 @，正文仍按前文绑定 `/answer/displayText`。这样占位、流式和 FINISH 都更新正文，不会重发通知，也不必在无组件上下文的增量数据中猜测 @ 标签。

下面生成可供 `--content -` 使用的消息字符串数组；用实际 openDingTalkId 替换参数，并沿用创建回执的 `bizId`：

```bash
python3 - '<成员openDingTalkId>' <<'PYCODE' > mention-stream.json
import json, sys
member = sys.argv[1]
messages = [
    {"version": "v1.0", "createSurface": {"surfaceId": "mention-stream", "catalogId": "https://dingtalk.com/card/a2ui/catalogs/public/catalog.json", "dataModel": {"answer": {"displayText": "思考中…"}}}},
    {"version": "v1.0", "updateComponents": {"surfaceId": "mention-stream", "components": [
        {"id": "root", "component": "Column", "children": ["mentions", "answer"]},
        {"id": "mentions", "component": "Markdown", "content": f'<a atId="{member}">成员</a> <a atId="all">所有人</a>'},
        {"id": "answer", "component": "Markdown", "content": {"path": "/answer/displayText"}}
    ]}}
]
json.dump([json.dumps(item, ensure_ascii=False) for item in messages], sys.stdout, ensure_ascii=False)
PYCODE
dws chat message send-a2ui-card --conversation-id '<群ID>' --at-open-dingtalk-ids '<成员openDingTalkId>' --at-all --content - < mention-stream.json
```

更新时只提交 `/answer/displayText` 的累计正文，并以非空 FINISH 完结；不要重放 `mentions` 组件。只有 `updateDataModel` 的增量没有组件类型上下文，不能安全推断任意值为 Markdown。确需重发 @ 组件时，保留已确认的创建时 wire 表达；当前服务端的所有人高亮别名为 `10`，更新不产生新的通知。`all` 的更新渲染兼容性须结合实际服务端/客户端验证，不把接口接受当作展示证明。

止血：`DWS_A2UI_MENTIONS_DISABLED=1` 或 `true` 会拒绝新的显式 @ 通知，并停止创建时标签改写。默认关闭止血开关；无通知参数的原路径不变。每次 CLI 调用读取变量，宿主必须向 DWS 子进程传入；它不会中断已运行请求或撤回已发消息。恢复时去掉变量，必要时回滚二进制。

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

可按同样方式继续分批更新。任务完成后写入完整实际结果，把承载结果的 `answer` 注解为产物，并将 `--flow-status` 设为 `FINISH`：

```bash
dws chat message update-a2ui-card \
  --biz-id '<本次发送返回的bizId>' \
  --a2ui-annotations '[{"surfaceId":"example-card","componentId":"answer","type":"artifact"}]' \
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

卡片进入 `FINISH`、`CONFIRMING` 或 `CONFIRMED` 时，服务端从 `type` 为 `artifact` 的组件异步生成会话摘要，因此触发这些状态的更新要携带注解。注解组件中没有可展示内容（`Text`、`Markdown`、`Image`、`ImageList`、`Link`、`File`、`Video`、`AudioPlayer`，或包含它们的容器）时，会话摘要显示服务端默认文案，如“暂无可展示的最终产物”。注解承载主结果的组件，`root` 仅作兜底；不要注解思考过程、凭据或无关输入，也不要为生成摘要而改变业务状态。更新受理不代表摘要已生成，无法回读时如实说明未验证。

发送后核对创建结果和客户端展示，再检查同一 `bizId` 的更新和完结效果。遇到 `a2ui dws catalog validation failed` 时，可对照 Catalog 检查组件属性、JSON 格式、Catalog 地址和 surface 创建顺序。
