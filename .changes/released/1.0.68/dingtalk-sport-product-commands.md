---
category: Added
---

- 新增 misc 长尾能力 `dws sport`（企业运动健康/钉钉运动），接入选定的 21 个 MCP 工具。命令树位于 `internal/helpers/sport.go`，通过 `register_sport.go` 手写注册进入 `NewPublicCommands`，不修改生成式注册表或 app 装配逻辑；端点登记于 edition supplement servers（server ID `sport`）
- sport 根命令不在 `dws --help` 展示；`dws sport --help`、21 个叶子的直接调用和 Agent Schema 查询仍可用，通过 dingtalk-misc Skill 发现能力
- `sport org status`：查询企业是否开通钉钉运动步数榜（`get_org_step_participate_status`）
- `sport step org-info|org-total`：企业单日步数概况、区间总步数（`get_org_health_step_info`、`get_org_total_step_count`）
- `sport step emp-ranking|dept-ranking|dept-emp-ranking`：员工/部门/部门内员工步数排行，支持日/周/月维度与 cursor 分页（`list_emp_step_ranking`、`list_dept_step_ranking`、`list_emp_ranking_in_dept`）
- `sport step emp-info|emp-infos|dept-infos`：员工与部门多日步数明细（`get_emp_step_ext_info`、`get_emp_step_infos`、`get_dept_step_infos_by_dept_id`）
- `sport competition create|config|rule-detail|instances|current-instance|next-instance|announce`：比赛创建、配置/规则/期次查询、开赛播报；create 与 announce 为无幂等写操作，均有 `user_required` 确认门；不提供更新、取消或删除命令
- `sport stats signup|participants-daily|participants|reached|trend`：比赛报名、每日参与、累计参与、达标与趋势统计，分别映射 `get_signup_count`、`count_daily_participants`、`count_total_participants`、`count_reached_users`、`list_daily_participation_trend`
- 新增 dingtalk-misc Skill 的 sport 产品主文档（`skills/multi/dingtalk-misc/references/sport.md`，与 `skills/mono/references/products/sport.md` 保持 identical），并在两份 SKILL.md 与 coverage 清单登记
- 全部 21 个叶子声明 `dry_run.preview_kind=request`、`remote_reads=false`；本地预览在统一信封顶层标记 `dry_run=true`，返回待调用工具与参数及 `executed=false`，不调用远端工具；真实执行沿用统一 MCP 错误分类
- `get_user_org_list`（用户所属企业列表）不接入 CLI；MCP 平台当前仍保留该工具，平台下线由服务维护方单独处理
