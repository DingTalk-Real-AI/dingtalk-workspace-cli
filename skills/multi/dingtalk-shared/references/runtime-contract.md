## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行；Help 不参与选路。准备 Help 时，本轮仅查一次精确 leaf：真实 `unknown flag` 后查 leaf Help，`unknown command` 后只查一次 shortcut 清单；禁止试探后缀。
- 参数或安全语义不确定时只查一次精确 leaf Schema：`--fields use_when,avoid_when,parameters,constraints,confirmation`；禁用产品级/`--all`，禁止靠失败探测门禁。
- 本地内容作为命令输入时，已有或临时文件先暂存到 cwd，再用显式文件参数传递；stdin 仅承载内容，不承载交互确认。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 严格按 Runtime `confirmation` 执行：`not_required` 直接执行且不加 `--yes`；`user_required` 才需要确认。用户在当前请求中已明确同一目标、动作和参数时可视为本次确认并加 `--yes`；信息不全时先询问。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
