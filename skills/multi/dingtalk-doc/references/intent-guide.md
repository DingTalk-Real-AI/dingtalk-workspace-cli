# doc 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "搜一下 OAuth2 接入文档" | 搜索开发文档 | `devdoc` | `doc search` | 搜索开放平台技术文档，不是钉钉内部内容 |
| "帮我建一个项目跟踪表" | 创建数据表格 | `aitable` | `doc` / `sheet` | 涉及结构化数据/行列操作，不是富文本文档或电子表格 |
| "帮我写个项目周报" | 创建钉钉文档 | `doc` | `aitable` | 富文本内容创作，不是数据表 |
| "参照这个生成同样的 / 按模板生成 / 复刻 X / 同样的模板 X 月份的" + 已有 alidocs URL | 模板保形生成同形态变体 | `drive copy + drive rename + doc block update` → 见 [模板保形生成](./04-document.md#模板保形生成) | `doc read + doc create`（重写链） | adoc → markdown 是有损投影，read+create 会丢行高/单元格背景色/字号；copy 在 adoc 层保形复制后只在副本上局部修改 |
| "这个 alidocs 表格链接帮我看下"（粘贴原始 URL） | 先 probe 节点类型 | `dws doc info --node` → 按 `extension` 路由 | 直接调 `sheet` | `alidocs/i/nodes/{id}` 可能是文档/axls/able/xlsx 等，禁止凭 URL 猜类型 |
| "帮我记一下明天要做的事" | 创建个人待办 | `todo` | `doc` | 个人待办提醒，非文档内容 |
| "在知识库里创建一个文档" | 创建空文件实体 | `wiki node create --type adoc` | `doc create` | 空间内创建节点归 wiki；doc create 是向已有文档写入内容，不是创建文件节点 |
| "帮我看看收到的日报" | 收到的日志 | `report` | `doc` | 钉钉日志系统（日报/周报），不是文档 |
| "整理一下XX项目的所有讨论" | 跨源主题归档 | #5 generate-topic-report | #4 write-doc | #4 侧重单篇文档创作；按主题跨听记/群消息汇总属于工作汇报 |
| "搜一下智能化方案/最近 OKR 相关邮件/最近发版相关消息" | 搜企业知识内容 | `aisearch enterprise` | `doc search` / `mail search` / `chat message search` | 跨文档、消息、日程、听记、邮件等企业内容语义检索走 enterprise；具体 `queries/types/time-range` 抽槽见 `aisearch.md` |
| "我发给某人的消息/邮件/文档/今天我干了什么" | 搜行为记录 | `aisearch behavior` | `chat` / `mail` / `doc` / `report` | 关注“谁对什么做过什么”，走 behavior；具体 `behavior-type/direction/chat-scope` 抽槽见 `aisearch.md` |
| "把这段文字翻译成英文/translate this" | 通用文本翻译 | `chat text translate` | `doc` / `aisearch` | 纯文本翻译，不是文档编辑或语义搜索 |
| "帮我把这个文档翻译成日文" | 文档内容翻译 | 先 `doc get` 再 `chat text translate` | `chat text translate` 直接传文件 | translate 仅支持纯文本，需先提取文档内容 |
