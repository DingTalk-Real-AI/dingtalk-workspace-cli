# 意图判断

## 意图判断

用户说"签到记录/签到数据/签到明细/外勤签到/导出签到" → `checkin records`（查询签到记录）。如果用户意图是**导出签到报表/签到Excel**，则走下方的签到报表导出工作流。
  - **优先级**：只要用户句中含"签到"二字，就走本条，不要走 `check record`。"签到" ≠ "打卡"，签到是外勤场景的独立功能
用户说"打卡记录/出勤/考勤" → `check record`
用户说"指定用户打卡结果/考勤结果/迟到早退/缺卡异常" → `check result`
用户说"指定用户打卡流水/打卡详情/打卡时间地点/打卡记录详情" → `check record`
用户说"审批单/请假记录/加班记录/出差记录/补卡记录" → `approve list`
用户说"查询排班记录/获取排班详情/查看排班/排班表/导出排班/导出排班表/排班导出/XX考勤组的排班" → **必须先 `read_file` 读取 [attendance-schedule.md](./attendance-schedule.md) 后按其中的「排班查询导出工作流」执行**。
  - **优先级**：只要用户句中含"排班"二字且意图是查看/导出，就走本条，不要走 `attendance-report.md`。"导出排班表" ≠ "导出考勤报表"
  - **严禁**绕过 `attendance-schedule.md` 直接调用 `dws attendance schedule get` 命令
  - 脚本自动处理：分批查询（超 20 人自动分批）、userId→姓名转换、classId→班次名称转换、排班表格式 Excel 输出
用户说"排班/导入排班/安排排班/设置排班/安排班次/调班/换班/排休" → **必须先 `read_file` 读取 [attendance-schedule.md](./attendance-schedule.md) 后按其中的「排班导入工作流」执行**。
  - **严禁**绕过 `attendance-schedule.md` 直接调用 `dws attendance schedule import` 命令
  - **严禁**仅凭命令 `--help` 或本文件中的命令参考自行组装排班命令
  - 该文档定义了：考勤组类型校验（必须为 TURN）、班次校验（必须属于该考勤组）、排班回显确认、错误处理等约束，缺一不可
  - 违反约束的后果：排错班次、排错人员、排班数据覆盖无法回退
用户说"班次定义/班次列表/有哪些班次/我负责的班次" → `class search`（返回结果已包含全量属性，无需再调 get）
用户说"班次详情/某个班次的具体信息" → `class search --name "..."`（search 直出，直接返回详情）。`class get` 仅在需要按已知 classId 精确查询时使用
用户说"更新班次/修改班次/班次改名/修改上下班时间" → `class update`
用户说"补卡规则/补卡设置" → `adjustment search`（返回结果已包含全量属性，无需再调 get）
用户说"补卡规则详情/某条补卡规则的具体信息" → `adjustment search --name "..."`（search 直出）。`adjustment get` 仅在需要按已知 adjustmentId 精确查询时使用
用户说"加班规则/加班设置/加班计算" → `overtime search`（返回结果已包含全量属性，无需再调 get）
用户说"加班规则详情/某条加班规则的具体信息" → `overtime search --name "..."`（search 直出）。如需查已删除/被覆盖的历史记录 → `overtime get`
用户说"考勤组列表/有哪些考勤组" → `group search`
用户说"考勤组详情/全量考勤组信息" → `group get`,若返回结果中含成员 userId 列表，则批量调用 `dws contact user get --ids <userId1,userId2>`（或等价通讯录查询），在最终输出中展示员工姓名而非裸 userId
用户说"考勤组成员/打卡地址/打卡wifi/打卡蓝牙" → `group filtered-get`（按需查询，节省成本）,若返回结果中含成员 userId 列表，则批量调用 `dws contact user get --ids <userId1,userId2>`（或等价通讯录查询），在最终输出中展示员工姓名而非裸 userId
用户说"更新考勤组成员/添加考勤人员/删除考勤人员/添加考勤部门/删除考勤部门/加入考勤组/移出考勤组/设置无需考勤/取消无需考勤" → `group update-members`
用户说"修改考勤组/更新考勤组配置/考勤组改名/改变考勤组绑定的班次/修改打卡范围/设置考勤组负责人" → `group update`
用户说"创建考勤组/新建考勤组/添加考勤组" → `group create`
用户说"查询某人的考勤汇总/考勤统计/周统计/月统计" → `summary`
用户说"考勤组/考勤规则/打卡规则" → `rules`
用户说"查询个人规则设置/查看打卡提醒/查看极速打卡/查看缺卡提醒/查看打卡结果通知/查看个人考勤统计通知/查看团队考勤统计通知" → `selfsetting get`
用户说"更新个人规则设置/保存打卡提醒/修改极速打卡/关闭缺卡提醒/开启打卡结果通知/设置个人考勤统计通知/设置团队考勤统计通知" → `selfsetting save`
用户说"考勤字段/考勤列" → `report columns`
用户说"考勤数据/查询考勤报表数据" → `report query-data`（单次查询场景，非导出）
  **导出签到记录/签到报表/签到数据导出/签到明细/外勤签到导出** → **必须先 `read_file` 读取 [attendance-report.md](./attendance-report.md) 后按其中的工作流执行（报表类型为"签到报表"）**。
  - 触发关键词：签到记录导出、签到报表、签到数据、签到明细、外勤签到
  - 数据来源为 `attendance checkin records`（签到接口），非 `report query-data`
  - **严禁**绕过 `attendance-report.md` 直接调用 `python scripts/attendance_report_checkin.py`
  **导出考勤/导出报表/生成考勤报表/出勤汇总导出/考勤明细导出/迟到早退统计导出/全员考勤数据导出/月度考勤报表/考勤表格/考勤 Excel** → **必须先 `read_file` 读取 [attendance-report.md](./attendance-report.md) 后按其中的工作流执行**。
  - **排除**：如果用户说的是"导出**排班**表"/"导出**排班**"/"**排班**导出"，这属于**排班查询导出**，应路由到 [attendance-schedule.md](./attendance-schedule.md)，而非本条。判断标准：句中含"排班"二字 → 走排班；不含"排班"或明确说"考勤报表/考勤数据/出勤统计" → 走报表。
  - **严禁**绕过 `attendance-report.md` 直接调用 `python scripts/attendance_report_*.py` 任何脚本
  - **严禁**仅凭脚本 `--help` 或本文件"自动化脚本"表格里的脚本路径就推断参数自行组装命令
  - 该文档定义了：报表类型默认值、列选择策略（`--column-keywords`）、阶段 1 人员获取流程、错误处理、输出摘要规范，缺一不可
  - 违反约束的后果：报表数据不全、列错位、人员遗漏、用户得到错误结果
  **导出补卡记录/导出请假记录/导出出差记录/导出外出记录/补卡导出/请假导出/出差导出/外出导出/考勤审批记录导出/请假明细/补卡明细/出差明细/外出明细** → **必须先 `read_file` 读取 [attendance-report.md](./attendance-report.md) 后按其中的工作流执行（报表类型为"考勤记录"）**。
  - 触发关键词：补卡记录、请假记录、出差记录、外出记录、审批记录导出、请假明细导出、补卡明细导出
  - 判断标准：句中含"补卡/请假/出差/外出"且含"记录/导出/明细/报表" → 走 attendance-report.md 的"考勤记录"类型
  - **严禁**绕过 `attendance-report.md` 直接调用 `python scripts/attendance_report_record.py`
  - 该文档定义了：考勤记录子类型（leave/trip/out/patch）选择策略、阶段 1 人员获取流程、阶段 3 脚本调用规范
用户说"假期数据/年假/病假/请假记录" → `report query-leave`
用户说"假期/我的假期/假期规则" → `vacation types`
用户说"病假余额/年假余额/事假剩余假期"等查询指定假期规则的余额 → `vacation balance`
用户说"导出假期余额/假期余额列表/所有假期规则余额/假期余额 Excel/年假病假调休余额导出"等全部假期规则余额的查询 → **必须先 `read_file` 读取 [attendance-vacation.md](./attendance-vacation.md) 后按其中工作流执行**
用户说"假期变更/假期记录/请假扣减" → `vacation records`
用户说"更新假期规则/修改假期类型/编辑假期规则" → `vacation update-type --leave-code <LEAVE_CODE>`
用户说"设置假期余额/调整假期额度/更新假期余额" → 先调用 `vacation balance` 获取当前余额，计算修改后的值，再调用 `vacation save-balance`
用户说"增加假期余额/发放年假/给员工加年假" → 先调用 `vacation balance` 获取当前余额，加上要增加的天数，再调用 `vacation save-balance` 设置新总额度
用户说"签到/签到记录" → `checkin records`
