## 最小 DWS 执行契约

- 只用 `dws`；结构化读取加 `--format json`，按真实返回判断。
- 已知命令直调；参数/约束/安全不明查 leaf 窄 Schema。Schema 不可用才读已知 leaf Help 一次；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 不查 Help：优先错误中的明确 suggestion，其次已加载 Skill/reference 中的明确兼容入口；均无则报漂移并停，禁全 Catalog。低频 reference 不默认 Help，禁 root/parent/product Help。发现后必须执行或说明阻塞。
- 不猜命令/flag/字段/ID/账号/业务事实；ID 来自真实返回。目标零命中/多候选/类型不明先消歧；仅可选时间/展示范围用契约默认，缺必需信息即停。
- 解析/读/写同一 profile，ID 不跨组织。多账号只用唯一 `isOrgCurrent=true`；否则用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出/记录 token、refresh token、appSecret、webhook token；已注入认证时不索要。
- 写须符合明确意图；确认以最终 Runtime gate/Schema 为准，确认后才加 `--yes`。
- 写后验证结果，不凭退出码宣称成功。退出须最终答复，区分完成、部分、阻塞、待确认、失败；保留已有数据及 `complete/hasMore/stopReason/failures`。
- 时间戳按会话时区展示，必要时保留原值。
- 认证/权限/profile/confirmation/未知错误只读 `dingtalk-shared` 对应 reference，禁连续猜替代命令。
