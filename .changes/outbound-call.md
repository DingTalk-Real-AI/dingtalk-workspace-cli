# 新增 AI 语音外呼

新增 `dws outbound-call create` 与 `dws outbound-call detail`，支持创建钉钉 AI 语音外呼任务、按 taskId 查询通话状态与听记链接。

- 参照 sport，通过 helpers 手写注册和 edition 补充端点接入完整命令树；入口从根帮助隐藏，通过 Skill 与 Schema 发现。
- create 每次只呼叫一名用户，必填 `--callee`（钉钉 userId）、`--task`（任务说明）、`--opening`（开场白），映射为单元素 calleeInfos；输入上限 1 MiB，不支持批量 JSON、文件或标准输入。
- 先本地校验、再确认、后提交；create 声明 write/high/user_required/non_idempotent，detail 声明 read/low/not_required/idempotent。`--dry-run` 只生成本地预览，不调用远端。
- 使用统一结果输出并声明叶子 Result、参数映射与选择语义。创建回执可识别单一 taskId 时返回 pending，查询指令携带原 profile；受理与查询成功均不代表通话完成。
- 实际执行只支持单 profile。创建请求超时、断连、HTTP 5xx 或响应无法解析等提交结果无法确认的情况返回 submission_unknown，提示先核实任务、禁止自动重试；明确未发送或拒绝的失败保留原分类。
- 外呼参数及性能 argv 脱敏，不记录原始响应或传输快照；外呼失败日志和统计仅使用固定文本，结构化输出保留原错误分类。复用现有命令审计，不新增专用诊断或统计协议。
- mono/multi Skill 增加外呼路由、助手身份、结束条件与礼貌收尾指引，电话 DING 和短信 DING 仍使用 ding。

测试覆盖输入校验、确认、零远端调用预览、最终 Schema、模拟传输、结果不确定性、profile 绑定与日志脱敏，不实际发起外呼。

回滚：移除 outbound-call 注册、补充端点、业务实现及对应声明和文档入口。
