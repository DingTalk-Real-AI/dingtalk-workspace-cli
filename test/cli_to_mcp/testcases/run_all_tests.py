#!/usr/bin/env python3
"""
DWS CLI 全产品测试运行器
批量运行所有产品的 pytest 测试，生成 last_run.log 和汇总报告

报告生成规范（必读）：
1. 请务必从日志或测试代码中提取出每个失败/错误用例的执行命令 (dws ...) ，
   不允许在报告中显示 [未能提取命令]。
2. 命令提取优先级：
   ① 日志 'E     cmd:    dws ...' 行（命令本身报错时有）
   ② PRODUCT_DETAIL_CASES 配置的 cmd 字段
   ③ 从测试源码文件解析 dws.run/run_ok/run_raw 参数（断言失败时日志无 cmd）
3. 问题描述只说问题是什么，不给任何修复建议。
4. 跳过用例必须展示执行命令并说明跳过原因，汇总报告和产品详细报告均需展示。
5. 总数必须包含 skipped，通过率分母排除 skipped。
"""

import json
import os
import re
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Tuple

# 复用与 pytest 用例一致的 dws 路径解析逻辑，避免“终端 OK / pytest 不 OK”。
from test_utils import resolve_dws_bin

# 产品列表（按优先级排序）
WUKONG_PRODUCTS = [
    # P0 - 核心产品（开源 dws v1.0.30 已暴露）
    "aitable",
    "contact",
    "calendar",
    "todo",
    "doc",
    "wiki",
    "chat",
    "minutes",
    # P1 - 重要产品
    "attendance",
    "mail",
    "oa",
    "report",
    # 其他开源产品
    "aiapp",
    "aisearch",
    "devdoc",
    "ding",
    "drive",
    "live",
    "sheet",
]

OPEN_PRODUCTS = [
    # P0 - 核心产品
    "aitable",      # P0
    "contact",      # P0
    "calendar",     # P0
    "todo",         # P0
    "chat",         # P0
    "oa",           # P0
    "attendance",   # P0
    "ding",         # P0
    "report",       # P0
    "workbench",    # P2
    "devdoc",       # P0
    "drive",        # P2 — 钉盘已通过 envelope (list_spaces / delete_document) 进入开源能力清单
]

PRODUCT_PROFILES = {
    "wukong": WUKONG_PRODUCTS,
    "open": OPEN_PRODUCTS,
}

# 产品中文名映射
PRODUCT_NAMES = {
    "aitable": "AI 多维表",
    "contact": "通讯录",
    "calendar": "日历",
    "todo": "待办",
    "doc": "文档",
    "wiki": "知识库",
    "tb": "项目协作",
    "attendance": "考勤",
    "mail": "邮件",
    "workbench": "工作台",
    "bot": "机器人",
    "ding": "DING 消息",
    "notify": "通知",
    "aiapp": "AI 应用",
    "aisearch": "AI 搜索",
    "chat": "群聊",
    "conference": "会议",
    "contract": "合同",
    "devdoc": "开发文档",
    "drive": "网盘",
    "live": "直播",
    "minutes": "AI听记",
    "oa": "OA 审批",
    "recruit": "招聘",
    "report": "报表",
    "headhunter": "一人猎头",
    "sheet": "钉钉表格",
    "finance": "智能财务",
    "edu-app": "家校应用",
    "unified-toolkit": "统一工具箱",
}

# 产品核心问题（固化在报告中，不随测试结果变化；key=产品名，value=核心问题描述）
PRODUCT_ISSUES = {
    "aitable": "搜索索引延迟导致查询无结果；分页测试依赖特定数据量",
    "contact": "手机号搜索接口权限不足，返回空对象；部门搜索无匹配结果",
    "calendar": "event list 无时间范围易超时；create 未解析 eventId；participant/room delete 在 -f json 下仍走交互确认导致非 JSON；部分断言与返回结构不一致",
    "todo": "teardown 调用的 todo task delete 工具未在 MCP Server 注册",
    "tb": "conftest fixture 字段路径错误（应为 data[\"result\"][\"result\"][\"id\"]），导致 43 个 ERROR 级联失败",
    "attendance": "summary / group / shift query 三类工具未在 MCP Server 注册；历史日期查询被服务端拒绝",
    "mail": "mail message search 工具未在 MCP Server 注册（MCP 有灰度控制）",
    "workbench": "workbench app list / get 命令 MCP 工具名无效（invalid resource name）",
    "bot": "robotCode dingdiwdtolfjiih8lfw 在当前组织不存在；webhook 无效 token 断言逻辑错误",
    "ding": "robotCode 不属于当前测试组织，所有发送类接口失败",
    "minutes": "minutes list mine 缺少 maxResult 默认值；--page-size 参数不存在",
    "aisearch": "搜索请求偶发超时，属服务端响应慢，非 CLI 缺陷",
    "recruit": "按姓名搜索面试者服务端返回操作失败，疑似权限或模块未开通",
}

# 产品专项备注（固化在报告中，不随测试结果变化）
# 备注规则：
# - mail: 邮箱功能目前 新增 钉钉组织校验与账号权限限制暂不支持阿里巴巴邮箱，待评估阿里钉定制改造后可能恢复，现阶段请先用小号
PRODUCT_NOTES = {
    "mail": "邮箱功能目前 新增 钉钉组织校验与账号权限限制暂不支持阿里巴巴邮箱，待评估阿里钉定制改造后可能恢复，现阶段请先用小号",
    "contract": "智能合同：推荐 `dws dingtalk contract`；联调 draft/review 可选用例依赖 TEST_CONTRACT_DRAFT_* / TEST_CONTRACT_REVIEW_REQUEST_JSON 等环境变量（见 contract/conftest 与各 test 文件 docstring）",
}

# 各产品详细用例清单（固化，用于生成产品级详细报告，格式参考考勤.md）
# 每条记录：{"name": 用例展示名, "cmd": 执行命令, "status": "pass"/"fail"/"error", "error_type": 错误类型, "stderr": 错误信息, "analysis": 问题描述（只描述问题，不给修复建议）}
PRODUCT_DETAIL_CASES = {
    "aitable": {
        "title": "AI 多维表（aitable）命令测试报告",
        "passed": [
            {"name": "test_list_returns_bases", "cmd": "dws aitable base list -f json"},
            {"name": "test_list_with_limit", "cmd": "dws aitable base list --limit 5 -f json"},
            {"name": "test_search_returns_structure", "cmd": "dws aitable base search --query 测试 -f json"},
            {"name": "test_search_no_match", "cmd": "dws aitable base search --query __no_match__ -f json"},
            {"name": "test_get_returns_structure", "cmd": "dws aitable base get --base-id <id> -f json"},
            {"name": "test_update_name_effective", "cmd": "dws aitable base update --base-id <id> --name <new_name> -f json"},
            {"name": "test_update_with_desc", "cmd": "dws aitable base update --base-id <id> --desc <desc> -f json"},
            {"name": "test_create_and_delete_lifecycle", "cmd": "dws aitable base create --name <name> -f json && dws aitable base delete --base-id <id> -f json"},
            {"name": "test_create_with_template", "cmd": "dws aitable base create --name <name> --template-id <id> -f json"},
            {"name": "所有 table/field/record/template 测试（27条）", "cmd": "pytest aitable/test_02_table.py aitable/test_03_field.py aitable/test_04_record.py aitable/test_05_template.py"},
        ],
        "failed": [
            {
                "name": "test_list_pagination — 分页后第2页无数据",
                "testcase": "TestBaseList::test_list_pagination",
                "cmd": "dws aitable base list --limit 1 --offset 1 -f json",
                "error_type": "断言失败",
                "stderr": "assert 0 >= 1 (第2页返回空列表)",
                "analysis": "测试账号下 base 数量不足2个，分页后第2页为空列表。"
            },
            {
                "name": "test_search_existing_base — 搜索索引延迟导致找不到刚创建的 base",
                "testcase": "TestBaseSearch::test_search_existing_base",
                "cmd": "dws aitable base search --query <刚创建的baseName> -f json",
                "error_type": "断言失败",
                "stderr": "Base 4lgGw3P8vwynN4DNcQqb4p6ZW5daZ90D not found in search results after 10s. Got: []",
                "analysis": "aitable 搜索依赖全文索引，创建后索引同步存在延迟（>10s），实际搜索结果为空列表。"
            },
        ],
        "duration": "55.62s"
    },
    "contact": {
        "title": "通讯录（contact）命令测试报告",
        "passed": [
            {"name": "test_get_self_returns_profile", "cmd": "dws contact user get-self -f json"},
            {"name": "test_get_self_contains_userId", "cmd": "dws contact user get-self -f json"},
            {"name": "test_get_self_contains_orgUserName", "cmd": "dws contact user get-self -f json"},
            {"name": "test_search_returns_userId_list", "cmd": "dws contact user search --keyword 张 -f json"},
            {"name": "test_search_no_match_returns_empty", "cmd": "dws contact user search --keyword __no_match__ -f json"},
            {"name": "test_search_chinese_name", "cmd": "dws contact user search --keyword wukong01 -f json"},
            {"name": "test_search_invalid_format_returns_error", "cmd": "dws contact user search-mobile --mobile INVALID -f json"},
            {"name": "test_get_by_id", "cmd": "dws contact user get --ids 035665695811868955452 -f json"},
            {"name": "test_get_returns_orgEmployeeModel", "cmd": "dws contact user get --ids 035665695811868955452 -f json"},
            {"name": "test_get_invalid_id_returns_empty_model", "cmd": "dws contact user get --ids INVALID_999 -f json"},
            {"name": "test_search_result_has_fields", "cmd": "dws contact dept search --keyword 技术 -f json"},
            {"name": "test_search_no_match (dept)", "cmd": "dws contact dept search --keyword __xyz__ -f json"},
            {"name": "test_list_root_returns_success / test_list_root_contains_depts", "cmd": "dws contact dept list-children --dept-id 1 -f json"},
            {"name": "test_list_invalid_dept", "cmd": "dws contact dept list-children --dept-id 99999999 -f json"},
            {"name": "test_list_returns_deptUserList / test_list_members_structure", "cmd": "dws contact dept list-members --dept-id 1 -f json"},
            {"name": "test_list_invalid_dept_members", "cmd": "dws contact dept list-members --dept-id 99999999 -f json"},
        ],
        "failed": [
            {
                "name": "test_search_self_mobile — 手机号搜索返回 userId 不匹配",
                "testcase": "TestUserSearchMobile::test_search_self_mobile",
                "cmd": "dws contact user search-mobile --mobile 17681800166 -f json",
                "error_type": "断言失败",
                "stderr": "AssertionError: assert '035665695811868955452' == '035665695811868955452' （返回的 userId 与当前登录用户不一致）",
                "analysis": "手机号 17681800166 对应的用户 ID (035665695811868955452) 与测试登录账号 ID (035665695811868955452) 不一致，测试中硬编码的手机号不属于当前测试账号。"
            },
            {
                "name": "test_search_returns_deptList — 搜索‘技术’无匹配部门",
                "testcase": "TestDeptSearch::test_search_returns_deptList",
                "cmd": "dws contact dept search --keyword 技术 -f json",
                "error_type": "测试数据依赖",
                "stderr": "搜索‘技术’应有结果 assert 0 > 0",
                "analysis": "当前测试组织中没有名称包含‘技术’的部门，服务端返回空列表，断言 count > 0 失败。"
            },
        ],
        "duration": "7.32s"
    },
    "calendar": {
        "title": "日历（calendar）命令测试报告",
        "skipped": [
            {
                "name": "TestRoomAdd::test_add_room_to_event",
                "cmd": "dws calendar room add --event-id <id> --room-id <id> -f json",
                "reason": "会议室搜索返回非 JSON（服务端超出100条上限），无法获取有效 roomId，会议室添加用例被跳过"
            },
        ],
        "passed": [
            {"name": "test_list_with_time_range", "cmd": "dws calendar event list --start <start> --end <end> -f json"},
            {"name": "test_list_empty_range", "cmd": "dws calendar event list --start <far_future> --end <far_future> -f json"},
            {"name": "test_get_invalid_id", "cmd": "dws calendar event get --event-id INVALID -f json"},
            {"name": "test_delete_invalid_id", "cmd": "dws calendar event delete --event-id INVALID -f json"},
            {"name": "test_list_invalid_event (participant)", "cmd": "dws calendar participant list --event-id INVALID -f json"},
            {"name": "test_add_to_invalid_event (participant)", "cmd": "dws calendar participant add --event-id INVALID -f json"},
            {"name": "test_remove_from_invalid_event", "cmd": "dws calendar participant remove --event-id INVALID -f json"},
            {"name": "test_search_returns_list (room)", "cmd": "dws calendar room search --start <start> --end <end> -f json"},
            {"name": "test_add_to_invalid_event (room)", "cmd": "dws calendar room add --event-id INVALID -f json"},
            {"name": "test_delete_from_invalid_event", "cmd": "dws calendar room delete --event-id INVALID -f json"},
            {"name": "test_list_groups_returns_data / test_list_groups_is_list", "cmd": "dws calendar room list-groups -f json"},
            {"name": "test_query_self_busy / test_query_multiple_users / test_query_past_range", "cmd": "dws calendar busy query --users <id> --start <start> --end <end> -f json"},
        ],
        "failed": [
            {
                "name": "test_list_default — event list 超时（60s）",
                "testcase": "TestEventList::test_list_default",
                "cmd": "dws calendar event list -f json",
                "error_type": "命令超时",
                "stderr": "subprocess.TimeoutExpired: Command timed out after 60 seconds",
                "analysis": "不带时间范围的 event list 触发全量拉取，命令执行超过 60s 超时阈值，进程被强制终止。"
            },
            {
                "name": "test_create_basic / test_create_with_desc / test_create_verify_via_get / test_delete_lifecycle / test_delete_and_redelete — AttributeError",
                "testcase": "TestEventCreate::test_create_basic / TestEventCreate::test_create_with_desc / TestEventCreate::test_create_verify_via_get / TestEventDelete::test_delete_lifecycle / TestEventDelete::test_delete_and_redelete",
                "cmd": "dws calendar event create --summary <title> --start <start> --end <end> -f json",
                "error_type": "测试代码 Bug",
                "stderr": "AttributeError: 'str' object has no attribute 'get'",
                "analysis": "测试代码将 dws.run() 返回的 dict 套了 str() 转为字符串，再调用 str.get()，str 类型无 get 方法，报 AttributeError。"
            },
            {
                "name": "test_search_different_time_ranges — NameError",
                "testcase": "TestRoomSearch::test_search_different_time_ranges",
                "cmd": "dws calendar room search --start <start1> --end <end1> -f json",
                "error_type": "测试代码 Bug",
                "stderr": "NameError: name 'data' is not defined",
                "analysis": "测试代码中 assert 语句使用了未赋值变量 data（实际已改名为 data1/data2），导致 NameError。"
            },
            {
                "name": "test_search_past_time_range — 不允许查询过去时间段",
                "testcase": "TestRoomSearch::test_search_past_time_range",
                "cmd": "dws calendar room search --start 2025-01-01T09:00:00+08:00 --end 2025-01-01T10:00:00+08:00 -f json",
                "error_type": "业务逻辑限制",
                "stderr": "Error: [400002] filterStartTime can not less current time",
                "analysis": "会议室搜索接口不支持查询历史时段，测试用例传入过去时间（2025-01-01），服务端返回 400002 错误：filterStartTime can not less current time。"
            },
            {
                "name": "test_list_groups_idempotent — 返回结构缺少 data 字段",
                "testcase": "TestRoomListGroups::test_list_groups_idempotent",
                "cmd": "dws calendar room list-groups -f json",
                "error_type": "接口返回结构变更",
                "stderr": "KeyError: 'data'",
                "analysis": "测试断言 result['data'] 时报 KeyError，接口实际返回结构中无顶层 data 字段。"
            },
        ],
        "errors": [
            {
                "name": "TestEventGet / TestEventUpdate / TestParticipant* / TestRoomAdd / TestRoomDelete (共15个 ERROR)",
                "testcase": "conftest.py::event_id (fixture) — 级联 ERROR，影响 TestEventGet / TestEventUpdate / TestParticipant* / TestRoomAdd / TestRoomDelete 共15个用例",
                "error_type": "fixture 字段路径错误（级联 ERROR）",
                "stderr": "AssertionError: Event create must return eventId, got: {...} — 实际 id 位于 data['result']['id']",
                "analysis": "calendar/conftest.py 中 test_event_id fixture 尝试从 data['eventId'] 或 data['id'] 取值，但实际返回结构为 data['result']['id']，event_id 取值为 None，所有依赖该 fixture 的用例全部 ERROR。"
            },
        ],
        "duration": "73.45s"
    },
    "todo": {
        "title": "待办（todo）命令测试报告",
        "passed": [
            {"name": "test_create_returns_taskId", "cmd": "dws todo task create --subject <title> -f json"},
            {"name": "test_create_with_priority", "cmd": "dws todo task create --subject <title> --priority high -f json"},
            {"name": "test_create_with_due", "cmd": "dws todo task create --subject <title> --due <ISO-8601+08> -f json"},
            {"name": "test_list_returns_todoCards / test_list_todoCards_have_fields", "cmd": "dws todo task list -f json"},
            {"name": "test_list_with_status_filter", "cmd": "dws todo task list --status todo -f json"},
            {"name": "test_get_invalid_id", "cmd": "dws todo task get --task-id INVALID -f json"},
            {"name": "test_update_title / test_update_priority / test_update_invalid_id", "cmd": "dws todo task update --task-id <id> --subject <new> -f json"},
            {"name": "test_mark_done_and_undone", "cmd": "dws todo task done --task-id <id> -f json"},
            {"name": "test_done_invalid_task", "cmd": "dws todo task done --task-id INVALID -f json"},
        ],
        "failed": [],
        "errors": [
            {
                "name": "test_done_invalid_task (teardown ERROR)",
                "testcase": "TestTaskDone::test_done_invalid_task (teardown)",
                "error_type": "MCP 工具未注册",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具  (cmd: dws todo task delete --task-id 51036551179 -f json)",
                "analysis": "teardown 阶段调用 dws todo task delete 清理测试数据，但该工具在 MCP Server 中未注册，teardown 失败报 ERROR。"
            },
        ],
        "duration": "5.78s"
    },
    "tb": {
        "title": "项目协作（tb）命令测试报告",
        "skipped": [
            {
                "name": "TestWorktimeUpdate::test_update_worktime",
                "cmd": "dws tb worktime update --worktime-id <id> -f json",
                "reason": "需要已存在的 workHourId，测试环境中无有效工时记录，更新工时用例被跳过"
            },
        ],
        "passed": [
            {"name": "test_list_default", "cmd": "dws tb project list -f json"},
            {"name": "test_list_no_match", "cmd": "dws tb project list --name __no_match__ -f json"},
            {"name": "test_list_mine_returns_data / test_list_mine_idempotent", "cmd": "dws tb project list-mine -f json"},
            {"name": "test_create_basic / test_create_chinese_name / test_create_long_name", "cmd": "dws tb project create --name <name> -f json"},
            {"name": "test_update_invalid_id", "cmd": "dws tb project update --id INVALID --name <n> -f json"},
            {"name": "test_list_members_invalid", "cmd": "dws tb project list-members --project-id INVALID -f json"},
            {"name": "test_add_to_invalid_project", "cmd": "dws tb project add-member --project-id INVALID --user-id <id> -f json"},
            {"name": "test_list_task_types_invalid", "cmd": "dws tb project list-task-types --project-id INVALID -f json"},
            {"name": "test_list_workflow_invalid", "cmd": "dws tb project list-workflow --project-id INVALID -f json"},
            {"name": "test_list_priorities / test_list_priorities_non_empty", "cmd": "dws tb project list-priorities -f json"},
            {"name": "TaskSearch / TaskGet invalid / TaskUpdateRemark invalid / TaskAssign invalid / TaskUpdateDue invalid 等边界用例（共17条）", "cmd": "pytest tb/ -k invalid"},
        ],
        "failed": [
            {
                "name": "test_list_priorities_idempotent — 返回结构缺少 data 字段",
                "testcase": "TestProjectListPriorities::test_list_priorities_idempotent",
                "cmd": "dws tb project list-priorities -f json（执行两次，比较结果）",
                "error_type": "接口返回结构变更",
                "stderr": "KeyError: 'data'",
                "analysis": "测试断言 result['data'] 时报 KeyError，接口实际返回结构为 result['result']['result']，无顶层 data 字段。"
            },
        ],
        "errors": [
            {
                "name": "43 个 ERROR（项目/任务/工时相关用例全部级联失败）",
                "testcase": "tb/conftest.py::project_id (fixture) — 级联 ERROR，影响项目/任务/工时相关43个用例",
                "error_type": "fixture 字段路径错误（级联 ERROR）",
                "stderr": "AssertionError: Project create must return id, got: {...} — 实际 id 位于 data['result']['result']['id']",
                "analysis": "tb/conftest.py 中 test_project_id fixture 尝试从 data['id'] 或 data['result']['id'] 取项目 ID，但实际返回结构嵌套更深（data['result']['result']['id']），pid 取值为 None，所有需要项目 ID 的用例全部 ERROR。"
            },
        ],
        "duration": "15.91s"
    },
    "attendance": {
        "title": "考勤（attendance）命令测试报告",
        "passed": [
            {"name": "dws attendance record get --user 035665695811868955452（今日）", "cmd": "dws attendance record get --user 035665695811868955452 -f json"},
            {"name": "dws attendance record get --user ... --date 2026-03-12（近期）", "cmd": "dws attendance record get --user 035665695811868955452 --date 2026-03-12 -f json"},
            {"name": "dws attendance shift list --users ... --from 2026-03-03 --to 2026-03-09", "cmd": "dws attendance shift list --users 035665695811868955452 --from 2026-03-03 --to 2026-03-09 -f json"},
            {"name": "dws attendance shift list 多用户 --from 2026-03-03 --to 2026-03-07", "cmd": "dws attendance shift list --users 035665695811868955452,... --from 2026-03-03 --to 2026-03-07 -f json"},
        ],
        "failed": [
            {
                "name": "test_get_record_past_date — 查询历史考勤记录失败",
                "testcase": "TestAttendanceRecord::test_get_record_past_date",
                "cmd": "dws attendance record get --user 035665695811868955452 --date 2020-01-01 -f json",
                "error_type": "业务逻辑限制",
                "stderr": "Error: 操作失败: 操作失败。发生错误，建议稍后重试",
                "analysis": "服务端不支持查询过于久远的历史日期（2020-01-01），返回业务错误而非 JSON，导致解析失败。"
            },
            {
                "name": "test_list_shifts_past_range — 时间跨度查询历史数据失败",
                "testcase": "TestAttendanceShiftList::test_list_shifts_past_range",
                "cmd": "dws attendance shift list --users 035665695811868955452 --from 2020-01-01 --to 2020-01-07 -f json",
                "error_type": "业务逻辑限制",
                "stderr": "Error: 时间跨度不能超过7天",
                "analysis": "测试使用历史日期范围（2020-01-01 ~ 2020-01-07），服务端对历史数据查询有额外限制，拒绝请求。"
            },
            {
                "name": "test_query_shift_basic / test_query_shift_multiple_users / test_query_shift_past_range — shift query 工具名无效",
                "testcase": "TestAttendanceShiftQuery::test_query_shift_basic / TestAttendanceShiftQuery::test_query_shift_multiple_users / TestAttendanceShiftQuery::test_query_shift_past_range",
                "cmd": "dws attendance shift query --users ... --from ... --to ... -f json",
                "error_type": "MCP 工具名无效",
                "stderr": "Error: [MCP_TOOL_ERROR] invalid tool name: invalid resource name",
                "analysis": "attendance shift query 对应的 MCP 工具名在服务端未正确注册或命名格式不规范，MCP 路由无法找到对应工具。"
            },
            {
                "name": "test_summary_default / test_summary_with_params / test_summary_idempotent — summary 工具未注册",
                "testcase": "TestAttendanceSummary::test_summary_default / TestAttendanceSummary::test_summary_with_params / TestAttendanceSummary::test_summary_idempotent",
                "cmd": "dws attendance summary -f json",
                "error_type": "MCP 工具未注册",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具",
                "analysis": "attendance summary 工具在 MCP Server 工具列表中不存在，可能该功能尚未上线或工具 ID 已变更。"
            },
            {
                "name": "test_group_default / test_group_with_params / test_group_idempotent — group 工具未注册",
                "testcase": "TestAttendanceGroup::test_group_default / TestAttendanceGroup::test_group_with_params / TestAttendanceGroup::test_group_idempotent",
                "cmd": "dws attendance group -f json",
                "error_type": "MCP 工具未注册",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具",
                "analysis": "attendance group 工具在 MCP Server 中不存在，同 summary 问题。"
            },
        ],
        "duration": "3.25s"
    },
    "mail": {
        "title": "邮件（mail）命令测试报告",
        "note": "邮箱功能目前 新增 钉钉组织校验与账号权限限制暂不支持阿里巴巴邮箱，待评估阿里钉定制改造后可能恢复，现阶段请先用小号",
        "skipped": [
            {
                "name": "TestMailMessageGet::test_get_first_email",
                "cmd": "dws mail message get --email <email> --message-id <id> -f json",
                "reason": "mail message search 工具受 MCP 灰度控制，无法获取邮件 ID，跳过依赖该 ID 的 get 测试"
            },
        ],
        "passed": [
            {"name": "test_list_returns_data / test_list_idempotent / test_list_non_empty", "cmd": "dws mail mailbox list -f json"},
            {"name": "test_get_invalid_id", "cmd": "dws mail message get --message-id INVALID -f json"},
            {"name": "test_get_invalid_email", "cmd": "dws mail message get --email invalid@test.com --message-id INVALID -f json"},
            {"name": "test_send_to_self", "cmd": "dws mail message send --from <email> --to <email> --subject <s> --body <b> -f json"},
            {"name": "test_send_with_cc", "cmd": "dws mail message send --from <email> --to <email> --cc <email> --subject <s> --body <b> -f json"},
            {"name": "test_send_invalid_from", "cmd": "dws mail message send --from invalid --to <email> --subject <s> --body <b> -f json"},
        ],
        "failed": [
            {
                "name": "test_search_inbox — mail message search 工具未注册",
                "testcase": "TestMailMessageSearch::test_search_inbox",
                "cmd": "dws mail message search --email 1i2-hgmfj0xkbl@dingtalk.com --query folderId:2 --size 5 -f json",
                "error_type": "MCP 工具未注册（灰度控制）",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具",
                "analysis": "mail message search 工具受 MCP 灰度控制，当前测试账号所在组织未开放该工具。"
            },
            {
                "name": "test_search_by_subject — 搜索主题关键词失败",
                "testcase": "TestMailMessageSearch::test_search_by_subject",
                "cmd": "dws mail message search --email 1i2-hgmfj0xkbl@dingtalk.com --query subject:\"测试\" --size 5 -f json",
                "error_type": "MCP 工具未注册（灰度控制）",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具",
                "analysis": "同 test_search_inbox，mail message search 受灰度控制。"
            },
            {
                "name": "test_search_with_date — 按日期搜索失败",
                "testcase": "TestMailMessageSearch::test_search_with_date",
                "cmd": "dws mail message search --email 1i2-hgmfj0xkbl@dingtalk.com --query date>2026-01-01T00:00:00Z --size 5 -f json",
                "error_type": "MCP 工具未注册（灰度控制）",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具",
                "analysis": "同 test_search_inbox，mail message search 受灰度控制。"
            },
            {
                "name": "test_get_first_email — 依赖 search 的 get 测试失败",
                "testcase": "TestMailMessageGet::test_get_first_email",
                "cmd": "dws mail message search --email 1i2-hgmfj0xkbl@dingtalk.com --query folderId:2 --size 1 -f json",
                "error_type": "MCP 工具未注册（灰度控制）",
                "stderr": "Error: [MCP_TOOL_ERROR] Tool invocation failed: Tool metadata API error: PARAM_ERROR - 未找到指定工具",
                "analysis": "test_get_first_email 依赖 mail message search 获取邮件 ID，search 工具不可用导致级联失败。"
            },
        ],
        "duration": "4.32s"
    },
    "workbench": {
        "title": "工作台（workbench）命令测试报告",
        "passed": [],
        "failed": [
            {
                "name": "test_list_returns_data / test_list_idempotent / test_list_non_empty — app list 工具名无效",
                "testcase": "TestWorkbenchAppList::test_list_returns_data / TestWorkbenchAppList::test_list_idempotent / TestWorkbenchAppList::test_list_non_empty",
                "cmd": "dws workbench app list -f json",
                "error_type": "MCP 工具名无效",
                "stderr": "Error: [MCP_TOOL_ERROR] invalid tool name: invalid resource name",
                "analysis": "workbench app list 对应的 MCP 工具名在服务端未正确注册，命名格式不符合规范（invalid resource name）。"
            },
            {
                "name": "test_get_invalid_id — app get 工具名无效",
                "testcase": "TestWorkbenchAppGet::test_get_invalid_id",
                "cmd": "dws workbench app get --ids INVALID_99999 -f json",
                "error_type": "MCP 工具名无效",
                "stderr": "Error: [MCP_TOOL_ERROR] invalid tool name: invalid resource name",
                "analysis": "workbench app get 同样受 MCP 工具名问题影响。"
            },
        ],
        "errors": [
            {
                "name": "test_get_by_id / test_get_multiple_ids (2个 ERROR)",
                "testcase": "TestWorkbenchAppGet::test_get_by_id / TestWorkbenchAppGet::test_get_multiple_ids",
                "error_type": "fixture 依赖失败（级联 ERROR）",
                "stderr": "Failed: dws returned non-JSON: cmd: dws workbench app list -f json  stderr: Error: [MCP_TOOL_ERROR] invalid tool name: invalid resource name",
                "analysis": "TestWorkbenchAppGet 的 fixture 通过 app list 获取 app_id，list 工具不可用导致 fixture setup 失败，2个用例 ERROR。"
            },
        ],
        "duration": "0.13s"
    },
    "bot": {
        "title": "机器人（bot）命令测试报告",
        "passed": [
            {"name": "test_send_invalid_chat (group)", "cmd": "dws bot group send --chat-id INVALID --text <t> -f json"},
            {"name": "test_recall_invalid_chat (group)", "cmd": "dws bot group recall --chat-id INVALID --key <k> -f json"},
            {"name": "test_send_invalid_robot (direct)", "cmd": "dws bot direct send --robot-code INVALID --users <id> --title <t> --text <m> -f json"},
            {"name": "test_recall_invalid_robot (direct)", "cmd": "dws bot direct recall --robot-code INVALID --keys <k> -f json"},
        ],
        "skipped": [
            {
                "name": "TestBotGroupSend::test_send_markdown / test_send_plain_text",
                "cmd": "dws bot group send --chat-id <chatId> --text <t> -f json",
                "reason": "需要有效 chatId，测试账号无可用群组 chatId，群发送用例被跳过"
            },
            {
                "name": "TestBotGroupRecall::test_recall_sent_message / test_recall_invalid_key",
                "cmd": "dws bot group recall --chat-id <chatId> --key <k> -f json",
                "reason": "send_markdown 因无 chatId 被跳过，未返回 processQueryKey，群撤回用例无前置数据，被跳过"
            },
            {
                "name": "TestBotWebhookSend::test_send_basic / test_send_at_all",
                "cmd": "dws bot webhook send --token <token> --title <t> --text <m> -f json",
                "reason": "需要有效 webhook token，测试环境未配置有效 token，webhook 发送用例被跳过"
            },
            {
                "name": "TestBotDirectRecall::test_recall_sent",
                "cmd": "dws bot webhook recall --token <token> --keys <k> -f json",
                "reason": "send_basic 因无 token 被跳过，未返回 processQueryKey，webhook 撤回用例无前置数据，被跳过"
            },
        ],
        "failed": [
            {
                "name": "test_send_to_self — robotCode 不在当前组织",
                "testcase": "TestBotDirectSend::test_send_to_self",
                "cmd": "dws bot direct send --robot-code dingdiwdtolfjiih8lfw --users 035665695811868955452 --title CLI单聊测试 --text 消息 -f json",
                "error_type": "配置错误",
                "stderr": "Error: [RESOURCE_NOT_FOUND] 请求的资源不存在",
                "analysis": "robotCode dingdiwdtolfjiih8lfw 不属于当前测试组织，服务端返回资源不存在。需更换为当前组织的有效 robotCode。"
            },
            {
                "name": "test_send_multiple_users / test_recall_sent — 同 robotCode 问题",
                "testcase": "TestBotDirectSend::test_send_multiple_users / TestBotDirectRecall::test_recall_sent",
                "cmd": "dws bot direct send --robot-code dingdiwdtolfjiih8lfw ... -f json",
                "error_type": "配置错误",
                "stderr": "Error: [RESOURCE_NOT_FOUND] 请求的资源不存在",
                "analysis": "同 test_send_to_self，robotCode 配置错误。"
            },
            {
                "name": "test_recall_invalid_key — recall 无效 key 时也因 robotCode 失败",
                "testcase": "TestBotDirectRecall::test_recall_invalid_key",
                "cmd": "dws bot direct recall --robot-code dingdiwdtolfjiih8lfw --keys INVALID_99999 -f json",
                "error_type": "配置错误",
                "stderr": "Error: [RESOURCE_NOT_FOUND] 请求的资源不存在",
                "analysis": "robotCode 无效，即使是测无效 key 的边界用例也因此失败。"
            },
            {
                "name": "test_send_invalid_token — webhook 断言逻辑错误",
                "testcase": "TestBotWebhookSend::test_send_invalid_token",
                "cmd": "dws bot webhook send --token INVALID_TOKEN --title X --text X -f json",
                "error_type": "测试代码 Bug",
                "stderr": "assert (0 != 0 or 'error' in '{\"errcode\": \"300005\", \"errmsg\": \"token is not exist\"}')",
                "analysis": "CLI 正确返回了错误信息（errcode=300005），但测试断言要求 returncode!=0 或 stderr 含 'error'。CLI 实际返回码为 0，错误信息在 stdout 而非 stderr，断言逻辑有误。"
            },
        ],
        "duration": "3.41s"
    },
    "ding": {
        "title": "DING 消息（ding）命令测试报告",
        "skipped": [
            {
                "name": "TestDingMessageRecall::test_recall_sent",
                "cmd": "dws ding message recall --open-msg-task-id <id> -f json",
                "reason": "发送步骤因 robotCode 无效而失败，未能获取 openDingId，撤回用例无前置数据，被跳过"
            },
        ],
        "passed": [
            {"name": "test_send_invalid_robot", "cmd": "dws ding message send --robot-code INVALID --to <id> --content <c> -f json"},
            {"name": "test_recall_invalid_id", "cmd": "dws ding message recall --open-msg-task-id INVALID -f json"},
            {"name": "test_recall_missing_robot", "cmd": "dws ding message recall --open-msg-task-id <id> -f json（无 robotCode）"},
        ],
        "failed": [
            {
                "name": "test_send_app_ding — robotCode 不在当前组织",
                "testcase": "TestDingMessageSend::test_send_app_ding",
                "cmd": "dws ding message send --robot-code dingdiwdtolfjiih8lfw --to 035665695811868955452 --content CLI自动化DING -f json",
                "error_type": "配置错误",
                "stderr": "Error: robotCode is in not valid or not in the org",
                "analysis": "robotCode dingdiwdtolfjiih8lfw 不属于当前测试组织，所有发送类接口均失败。需更换为当前组织的有效 robotCode。"
            },
            {
                "name": "test_send_with_type — 指定 type=app 也失败",
                "testcase": "TestDingMessageSend::test_send_with_type",
                "cmd": "dws ding message send --robot-code dingdiwdtolfjiih8lfw --type app --to 035665695811868955452 --content 类型测试 -f json",
                "error_type": "配置错误",
                "stderr": "Error: robotCode is in not valid or not in the org",
                "analysis": "同 test_send_app_ding，robotCode 配置错误。"
            },
            {
                "name": "test_recall_sent — 因发送失败无法撤回",
                "testcase": "TestDingMessageRecall::test_recall_sent",
                "cmd": "dws ding message send --robot-code dingdiwdtolfjiih8lfw --to ... --content 待撤回 -f json",
                "error_type": "配置错误（级联失败）",
                "stderr": "Error: robotCode is in not valid or not in the org",
                "analysis": "recall 用例先发送再撤回，发送步骤因 robotCode 问题失败，导致 recall 测试级联失败。"
            },
        ],
        "duration": "2.03s"
    },
    "minutes": {
        "title": "AI听记（minutes）命令测试报告",
        "skipped": [
            {
                "name": "TestMinutesGetInfo::test_get_info / test_get_info_contains_title",
                "cmd": "dws minutes get-info --minutes-id <id> -f json",
                "reason": "list shared 返回空列表，无可用 minutes ID，依赖 minutes_id fixture 的全部用例被跳过"
            },
            {
                "name": "TestMinutesGetSummary::test_get_summary / test_get_summary_structure",
                "cmd": "dws minutes get-summary --minutes-id <id> -f json",
                "reason": "同上，minutes_id fixture 无法获取有效 ID"
            },
            {
                "name": "TestMinutesGetKeywords / TestMinutesGetTranscription / TestMinutesGetTodos / TestMinutesGetBatch / TestMinutesUpdateTitle（共10条）",
                "cmd": "dws minutes get-keywords / get-transcription / get-todos / batch-get / update-title --minutes-id <id> -f json",
                "reason": "同上，minutes_id fixture 因 list shared 无数据跳过，所有详情类用例级联跳过"
            },
        ],
        "passed": [
            {"name": "test_list_shared", "cmd": "dws minutes list shared -f json"},
            {"name": "test_list_shared_idempotent", "cmd": "dws minutes list shared -f json（执行两次）"},
            {"name": "test_get_info_invalid", "cmd": "dws minutes get-info --minutes-id INVALID -f json"},
            {"name": "test_get_summary_invalid", "cmd": "dws minutes get-summary --minutes-id INVALID -f json"},
            {"name": "test_keywords_invalid", "cmd": "dws minutes get-keywords --minutes-id INVALID -f json"},
            {"name": "test_transcription_invalid", "cmd": "dws minutes get-transcription --minutes-id INVALID -f json"},
            {"name": "test_todos_invalid", "cmd": "dws minutes get-todos --minutes-id INVALID -f json"},
            {"name": "test_batch_invalid", "cmd": "dws minutes batch-get --minutes-ids INVALID -f json"},
            {"name": "test_update_title_invalid", "cmd": "dws minutes update-title --minutes-id INVALID --title <t> -f json"},
        ],
        "failed": [
            {
                "name": "test_list_mine — maxResult 参数缺默认值",
                "testcase": "TestMinutesListMine::test_list_mine",
                "cmd": "dws minutes list mine -f json",
                "error_type": "CLI 参数缺少默认值",
                "stderr": "Error: maxResult is null",
                "analysis": "dws minutes list mine 未传 maxResult 参数时，CLI 未设置默认值，导致服务端报错。需在 CLI 中为 maxResult 添加默认值（如 20）。"
            },
            {
                "name": "test_list_mine_with_page — --page-size 参数不存在",
                "testcase": "TestMinutesListMine::test_list_mine_with_page",
                "cmd": "dws minutes list mine --page-size 5 -f json",
                "error_type": "参数名错误",
                "stderr": "Error: unknown flag: --page-size",
                "analysis": "CLI 中不存在 --page-size 参数，测试代码使用了错误的参数名。"
            },
            {
                "name": "test_list_mine_idempotent — 同 maxResult 问题",
                "testcase": "TestMinutesListMine::test_list_mine_idempotent",
                "cmd": "dws minutes list mine -f json（执行两次）",
                "error_type": "CLI 参数缺少默认值",
                "stderr": "Error: maxResult is null",
                "analysis": "同 test_list_mine。"
            },
            {
                "name": "test_list_shared_with_page — --page-size 参数不存在",
                "testcase": "TestMinutesListShared::test_list_shared_with_page",
                "cmd": "dws minutes list shared --page-size 5 -f json",
                "error_type": "参数名错误",
                "stderr": "Error: unknown flag: --page-size",
                "analysis": "同 test_list_mine_with_page。"
            },
        ],
        "errors": [
            {
                "name": "14 个 ERROR（get-info / get-summary / get-keywords / get-transcription / get-todos / batch-get / update-title 全部级联失败）",
                "testcase": "minutes/conftest.py::minutes_id (fixture) — 级联 ERROR，影响 get-info / get-summary / get-keywords / get-transcription / get-todos / batch-get / update-title 共14个用例",
                "error_type": "fixture 依赖失败（级联 ERROR）",
                "stderr": "Failed: dws returned non-JSON: cmd: dws minutes list mine -f json  stderr: Error: maxResult is null",
                "analysis": "conftest fixture minutes_id 通过 dws minutes list mine 获取 minutes_id，该命令因 maxResult 缺默认值而失败，导致所有依赖该 fixture 的用例全部 ERROR。修复 maxResult 默认值问题后，这 14 个 ERROR 有望消除。"
            },
        ],
        "duration": "5.10s"
    },
    "aisearch": {
        "title": "AI 搜索（aisearch）命令测试报告",
        "passed": [
            {"name": "test_search_user_query", "cmd": "dws aisearch search --question 当前用户的个人信息 -f json"},
            {"name": "test_search_no_match", "cmd": "dws aisearch search --question __极不可能匹配的内容__ -f json"},
        ],
        "failed": [
            {
                "name": "test_search_basic — 搜索请求超时",
                "testcase": "TestAisearchSearch::test_search_basic",
                "cmd": "dws aisearch search --question 上周的项目会议纪要 --keywords 项目,会议,纪要 -f json",
                "error_type": "服务端超时",
                "stderr": "Error: 请求超时: 请求超时。服务响应较慢，请稍后重试",
                "analysis": "AI 搜索服务在特定关键词组合下响应较慢，超过 CLI 请求超时阈值，命令进程被强制终止。"
            },
        ],
        "duration": "17.22s"
    },
    "recruit": {
        "title": "招聘（recruit）命令测试报告",
        "passed": [
            {"name": "test_search_no_match", "cmd": "dws recruit interview search --name __不存在的候选人__ --size 10 -f json"},
            {"name": "test_search_with_cursor", "cmd": "dws recruit interview search --name 张 --cursor <cursor> --size 5 -f json"},
        ],
        "failed": [
            {
                "name": "test_search_by_name — 服务端返回操作失败",
                "testcase": "TestRecruitInterviewSearch::test_search_by_name",
                "cmd": "dws recruit interview search --name 张三 --size 10 -f json",
                "error_type": "权限/模块未开通",
                "stderr": "Error: 操作失败: 操作失败。发生错误，建议稍后重试",
                "analysis": "按姓名搜索面试者时服务端返回操作失败，疑似当前测试账号所在组织未开通招聘模块，或缺少接口调用权限。"
            },
        ],
        "duration": "2.15s"
    },
    "finance": {
        "title": "智能财务（finance）命令测试报告",
        "passed": [
            {"name": "test_form_data_with_valid_business_id", "cmd": "dws finance process form-data --business-id 202604241459000239220 -f json"},
            {"name": "test_form_data_with_invalid_business_id", "cmd": "dws finance process form-data --business-id INVALID_BIZ_99999 -f json"},
            {"name": "test_form_data_missing_required_flag", "cmd": "dws finance process form-data -f json"},
            {"name": "test_list_with_required_params", "cmd": "dws finance process list --form-name 付款单 --start-time <start> --end-time <end> -f json"},
            {"name": "test_list_with_pagination", "cmd": "dws finance process list --form-name 付款单 --start-time <start> --end-time <end> --page-no 1 --page-size 5 -f json"},
            {"name": "test_list_missing_required_flags", "cmd": "dws finance process list --form-name 付款审批 -f json"},
            {"name": "test_list_application_default", "cmd": "dws finance invoice list-application --page-no 1 --page-size 10 -f json"},
            {"name": "test_list_application_with_time_range", "cmd": "dws finance invoice list-application --page-no 1 --page-size 10 --start-time <start> --end-time <end> -f json"},
            {"name": "test_list_application_large_page_size", "cmd": "dws finance invoice list-application --page-no 1 --page-size 50 -f json"},
            {"name": "test_add_record_with_valid_params", "cmd": "dws finance invoice add-record --business-id 202604241459000239220 --invoice-pdf-url <url> -f json"},
            {"name": "test_add_record_with_invalid_business_id", "cmd": "dws finance invoice add-record --business-id INVALID_BIZ_99999 --invoice-pdf-url <url> -f json"},
            {"name": "test_add_record_missing_required_flags", "cmd": "dws finance invoice add-record --business-id TEST_BIZ_001 -f json"},
        ],
        "failed": [],
        "duration": "6.48s"
    },
}

# 优先级映射
PRIORITY_MAP = {
    "aitable": "P0",
    "contact": "P1", "calendar": "P1", "todo": "P1", "doc": "P1",
    "tb": "P1", "attendance": "P1", "mail": "P1",
    "workbench": "P2", "bot": "P2", "ding": "P2", "notify": "P2",
    "aiapp": "P2", "aisearch": "P2", "chat": "P2", "conference": "P2",
    "contract": "P2", "devdoc": "P2", "live": "P2", "minutes": "P2",
    "oa": "P2", "recruit": "P2", "headhunter": "P2", "finance": "P2"
}


class TestRunner:
    """测试运行器"""
    
    def __init__(
        self,
        base_dir: str,
        products: List[str] = None,
        edition: str = "wukong",
        skip_open_unimplemented_param_cases: bool = False,
        force_products: str = "",
    ):
        self.base_dir = Path(base_dir)
        self.edition = edition
        self.products = products if products is not None else list(WUKONG_PRODUCTS)
        self.results = {}
        self.timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        self.skip_open_unimplemented_param_cases = skip_open_unimplemented_param_cases
        self.force_products = force_products
        # 统一确定本轮使用的 dws 二进制，并传递给 pytest 子进程
        self.dws_bin = self._prepare_dws_bin()
        
    def _prepare_dws_bin(self) -> str:
        """
        解析并确保本轮测试使用用户期望的 dws 二进制。

        规则：
        - 若用户显式设置了环境变量 DWS_BIN：严格使用，不做构建。
        - 否则优先使用仓库内 `dingtalk-cli_b/dws`（与你本地源码一致的构建产物）。
        - 最终回退为 PATH 中的 dws（由 resolve_dws_bin 处理）。
        """
        override = os.environ.get("DWS_BIN", "").strip()
        if override:
            return override

        repo_root = self.base_dir.parent.parent.parent  # .../cli
        local_dws = (repo_root / "dingtalk-cli_b" / "dws").resolve()
        # base_dir = .../test/cli_to_mcp/testcases
        # 仓库根 = .../cli
        # local_dws = .../cli/dingtalk-cli_b/dws

        if local_dws.is_file() and os.access(local_dws, os.X_OK):
            return str(local_dws)

        # 兜底：与 pytest conftest 保持一致的解析逻辑（PATH/which）
        return resolve_dws_bin(__file__)

    @staticmethod
    def _loads_json_safe(text: str):
        try:
            return json.loads((text or "").strip())
        except Exception:
            return None

    @staticmethod
    def _payload_is_success(data) -> bool:
        """仅 success=true 或常见正常结果结构视为成功。"""
        if isinstance(data, list):
            return True
        if not isinstance(data, dict):
            return False
        if data.get("success") is True or data.get("status") == "success":
            return True
        if isinstance(data.get("result"), (dict, list)):
            return True
        if isinstance(data.get("data"), (dict, list)):
            return True
        return False

    @staticmethod
    def _payload_is_structured_error(data) -> bool:
        """结构化错误 JSON：含 error/code/message 或显式 success=false。"""
        if not isinstance(data, dict):
            return False
        if data.get("success") is False or data.get("status") == "error":
            return True
        if "error" in data or "code" in data or "message" in data:
            return True
        return False

    def _raw_entry_is_cli_failure(self, entry: Dict) -> bool:
        """
        统一失败口径：
        1) returncode != 0 -> 失败
        2) stdout/stderr 为结构化错误 JSON -> 失败
        3) stdout/stderr 包含明确错误文本 -> 失败
        4) 只有成功 JSON 才算通过
        """
        if entry.get("returncode", 0) != 0:
            return True

        stdout = (entry.get("stdout", "") or "").strip()
        stderr = (entry.get("stderr", "") or "").strip()
        combined = f"{stdout}\n{stderr}"
        combined_lower = combined.lower()

        # 强规则：只要能强匹配到 `"success": true`（忽略空格/换行），就视为成功
        # 说明：run_raw 记录可能被截断导致 JSON 解析失败，此处避免误判。
        if re.search(r'"success"\s*:\s*true', combined_lower):
            return False

        error_markers = [
            "unknown flag",
            "auth_permission_denied",
            "\"error\":",
            "permission denied",
            "mcp_tool_error",
        ]
        if any(m in combined_lower for m in error_markers):
            return True

        stdout_json = self._loads_json_safe(stdout)
        stderr_json = self._loads_json_safe(stderr)
        if self._payload_is_structured_error(stdout_json) or self._payload_is_structured_error(stderr_json):
            return True

        # 至少一侧是成功 JSON，才算通过；否则按失败处理（非 JSON/未知输出）
        if self._payload_is_success(stdout_json) or self._payload_is_success(stderr_json):
            return False
        return True

    def run_product_tests(self, product: str) -> Dict:
        """运行单个产品的测试"""
        product_dir = self.base_dir / product
        
        if not product_dir.exists():
            print(f"  ⚠️  {product}: 目录不存在，跳过")
            return {"status": "skipped", "reason": "directory_not_found"}
        
        # 检查是否有测试文件
        test_files = list(product_dir.glob("test_*.py"))
        if not test_files:
            print(f"  ⚠️  {product}: 无测试文件，跳过")
            return {"status": "skipped", "reason": "no_test_files"}
        
        log_file = product_dir / "last_run.log"
        raw_results_global = self.base_dir / ".raw_cmd_results.json"
        
        print(f"  🔄 运行 {product} 测试...")
        
        try:
            # 清理上一轮 run_raw 产生的全局日志
            if raw_results_global.exists():
                raw_results_global.unlink()

            # 从 testcases 根目录运行 pytest，指定产品目录
            # 这样可以使用根目录的 conftest.py
            result = subprocess.run(
                ["python3", "-m", "pytest", f"{product}/", "-v", "--tb=short", "-rs"],
                cwd=self.base_dir,
                capture_output=True,
                text=True,
                env={
                    **os.environ,
                    "DWS_BIN": self.dws_bin,
                    "SKIP_OPEN_UNIMPLEMENTED_PARAM_CASES": (
                        "1" if self.skip_open_unimplemented_param_cases else "0"
                    ),
                    "DWS_FORCE_PRODUCTS": self.force_products,
                },
                timeout=900  # 15分钟超时
            )

            # 清理产品目录里历史 raw 文件（仅保留 last_run.log 作为分析来源）
            stale_product_raw = product_dir / ".raw_cmd_results.json"
            if stale_product_raw.exists():
                stale_product_raw.unlink()
            
            error_entries = []
            error_cmd_set = set()

            # 保存日志
            with open(log_file, 'w', encoding='utf-8') as f:
                f.write(result.stdout)
                if result.stderr:
                    f.write("\n\n=== STDERR ===\n")
                    f.write(result.stderr)

                # 追加 CLI 真实执行报错（来自 run_raw 临时记录）
                if raw_results_global.exists():
                    try:
                        with open(raw_results_global, "r", encoding="utf-8") as rf:
                            raw_entries = json.load(rf)
                        error_entries = [
                            e for e in raw_entries
                            if self._raw_entry_is_cli_failure(e)
                        ]
                        # 对 error_entries 按命令去重
                        seen_error_cmds_in_entries = set()
                        unique_error_entries = []
                        for e in error_entries:
                            cmd_norm = self._normalize_cmd_from_dws(e.get("cmd", ""))
                            if cmd_norm and cmd_norm not in seen_error_cmds_in_entries:
                                seen_error_cmds_in_entries.add(cmd_norm)
                                error_cmd_set.add(cmd_norm)
                                unique_error_entries.append(e)
                        error_entries = unique_error_entries
                        if error_entries:
                            f.write("\n\n=== CLI 真实报错（run_raw 捕获，兼容 pytest 失败格式） ===\n")
                            for e in error_entries:
                                cmd = self._normalize_cmd_from_dws(e.get("cmd", ""))
                                stdout = (e.get("stdout", "") or "").strip()
                                stderr = (e.get("stderr", "") or "").strip()
                                # 兼容你现有解析器：沿用 pytest 非 JSON 失败块样式
                                f.write("E   Failed: dws returned non-JSON:\n")
                                f.write(f"E     cmd:    {cmd}\n")
                                f.write(f"E     stdout: {stdout}\n")
                                f.write(f"E     stderr: {stderr}\n\n")
                    except Exception:
                        pass
                    finally:
                        # 临时文件仅用于本轮拼接日志，拼接后删除，避免双来源
                        try:
                            raw_results_global.unlink()
                        except Exception:
                            pass
            
            # 解析结果
            stats = self._parse_pytest_output(result.stdout, product)
            stats["returncode"] = result.returncode
            stats["log_file"] = str(log_file)
            stats["cli_error_count"] = len(error_entries)

            # 强化统计口径：只有“成功 JSON”才算通过。
            # 若某些 pytest PASSED 用例实际执行命令落在 error_cmd_set（结构化错误/明确错误）中，
            # 这些用例不应计入 passed，而应计入 CLI报错。
            try:
                log_text = log_file.read_text(encoding="utf-8")
            except Exception:
                log_text = ""
            passed_tests = re.findall(
                r'^(\S+test_[\w/]+\.py::[\w:]+)\s+PASSED\b',
                log_text,
                re.MULTILINE
            )
            cli_error_from_passed = 0
            for test_id in passed_tests:
                cmd = self._extract_cmd_from_test_file(test_id)
                cmd = self._normalize_cmd_from_dws(cmd)
                if cmd and cmd in error_cmd_set:
                    cli_error_from_passed += 1
            if cli_error_from_passed > 0:
                stats["passed"] = max(0, stats.get("passed", 0) - cli_error_from_passed)
                stats["cli_error_count"] = stats.get("cli_error_count", 0) + cli_error_from_passed
            
            total = stats.get('passed', 0) + stats.get('failed', 0) + stats.get('error', 0)
            cli_error_count = stats.get("cli_error_count", 0)
            if (
                result.returncode == 0
                and stats.get('failed', 0) == 0
                and stats.get('error', 0) == 0
                and cli_error_count == 0
            ):
                print(f"  ✅ {product}: 通过 {stats.get('passed', 0)}/{total} 个")
            else:
                failed = stats.get('failed', 0)
                errors = stats.get('error', 0)
                passed = stats.get('passed', 0)
                print(f"  ❌ {product}: 通过 {passed}, 失败 {failed}, 错误 {errors}, CLI报错 {cli_error_count}")
            
            return stats
            
        except subprocess.TimeoutExpired:
            print(f"  ⏱️  {product}: 超时")
            with open(log_file, 'w', encoding='utf-8') as f:
                f.write("TIMEOUT: Test execution exceeded 5 minutes")
            return {"status": "timeout", "passed": 0, "failed": 0, "error": 0}
            
        except Exception as e:
            print(f"  💥 {product}: 异常 - {e}")
            return {"status": "exception", "error": str(e), "passed": 0, "failed": 0}
    
    def _parse_pytest_output(self, output: str, product: str = "") -> Dict:
        """解析 pytest 输出"""
        stats = {
            "passed": 0,
            "failed": 0,
            "skipped": 0,
            "xfailed": 0,
            "xpassed": 0,
            "error": 0,
            "total": 0
        }
        
        # 查找汇总行，支持多种格式:
        # "15 failed in 1.49s" (全部失败)
        # "1 failed, 38 passed, 1 xfailed in 71.06s" (混合)
        # "38 passed in 0.50s" (全部通过)
        
        # 首先尝试匹配完整的汇总行
        summary_patterns = [
            # 完整格式: "1 failed, 38 passed, 1 xfailed in 71.06s" 或 "4 failed, 2 errors in 0.11s"
            r'(?:(\d+)\s+failed)?[,\s]*(?:(\d+)\s+passed)?[,\s]*(?:(\d+)\s+skipped)?[,\s]*(?:(\d+)\s+xfailed)?[,\s]*(?:(\d+)\s+xpassed)?[,\s]*(?:(\d+)\s+errors?)?\s+in\s+[\d.]+s',
        ]
        
        matched = False
        for pattern in summary_patterns:
            match = re.search(pattern, output, re.IGNORECASE)
            if match:
                # 提取各组数字
                groups = match.groups()
                if groups[0]:  # failed
                    stats["failed"] = int(groups[0])
                if groups[1]:  # passed
                    stats["passed"] = int(groups[1])
                if groups[2]:  # skipped
                    stats["skipped"] = int(groups[2])
                if groups[3]:  # xfailed
                    stats["xfailed"] = int(groups[3])
                if groups[4]:  # xpassed
                    stats["xpassed"] = int(groups[4])
                if groups[5]:  # error
                    stats["error"] = int(groups[5])
                matched = True
                break
        
        # 如果没匹配到完整格式，尝试单独匹配每种状态
        if not matched:
            passed_match = re.search(r'(\d+)\s+passed', output, re.IGNORECASE)
            failed_match = re.search(r'(\d+)\s+failed', output, re.IGNORECASE)
            skipped_match = re.search(r'(\d+)\s+skipped', output, re.IGNORECASE)
            xfailed_match = re.search(r'(\d+)\s+xfailed', output, re.IGNORECASE)
            xpassed_match = re.search(r'(\d+)\s+xpassed', output, re.IGNORECASE)
            error_match = re.search(r'(\d+)\s+error', output, re.IGNORECASE)
            
            if passed_match:
                stats["passed"] = int(passed_match.group(1))
            if failed_match:
                stats["failed"] = int(failed_match.group(1))
            if skipped_match:
                stats["skipped"] = int(skipped_match.group(1))
            if xfailed_match:
                stats["xfailed"] = int(xfailed_match.group(1))
            if xpassed_match:
                stats["xpassed"] = int(xpassed_match.group(1))
            if error_match:
                stats["error"] = int(error_match.group(1))
        
        # 计算总数
        stats["total"] = stats["passed"] + stats["failed"] + stats["skipped"] + stats["xfailed"] + stats["xpassed"] + stats["error"]
        
        # 提取失败详情
        if "FAILED" in output or "ERROR" in output:
            failures = self._extract_failures(output, product)
            stats["failures"] = failures
        
        return stats
    
    def _extract_failures(self, output: str, product: str = "") -> List[Dict]:
        """
        提取失败详情，务必从以下来源提取执行命令（按优先级）：
          1. 日志中的 'E     cmd:    dws ...' 行（命令本身报错时有）
          2. PRODUCT_DETAIL_CASES 中配置的 cmd 字段（静态配置）
          3. 测试源码文件中解析 dws.run(...) 的参数（断言失败时日志无 cmd）
        请务必从日志或测试代码中提取出执行命令，不允许显示 [未能提取命令]。
        """
        failures = []
        created_base_name = ""
        if product == "aitable":
            m_base = re.search(r"\[SETUP\]\s+Created test Base:\s+\S+\s+\(([^)]+)\)", output)
            if m_base:
                created_base_name = m_base.group(1).strip()

        # ① 从 log 中提取所有 cmd 行（优先真实运行日志）
        # - 旧格式：E     cmd:    dws ...
        # - 新格式：DWS_CMD: dws ...
        cmd_lines = re.findall(r'E\s+cmd:\s+(dws\s+\S.*?)\s*$', output, re.MULTILINE)
        if not cmd_lines:
            cmd_lines = re.findall(r'^\s*DWS_CMD:\s+(.*)$', output, re.MULTILINE)

        # 匹配失败测试名，同时按顺序关联 cmd
        failed_tests = re.findall(r'(\S+test_[\w/]+\.py::[\w:]+)\s+FAILED', output)
        error_tests = re.findall(r'(\S+test_[\w/]+\.py::[\w:]+)\s+ERROR', output)

        # 从 short summary 中提取每个失败用例的摘要原因，辅助分层判断
        failed_reason_map: Dict[str, str] = {}
        for test in failed_tests:
            m_reason = re.search(
                rf'^FAILED\s+{re.escape(test)}\s+-\s+(.+)$',
                output,
                re.MULTILINE
            )
            if m_reason:
                failed_reason_map[test] = m_reason.group(1).strip()

        cmd_iter = iter(cmd_lines)
        for test in failed_tests:
            cmd = next(cmd_iter, "")
            # ② 日志无 cmd，查 PRODUCT_DETAIL_CASES
            if not cmd:
                cmd = self._lookup_cmd_from_detail_cases(product, test)
            # ③ 还没找到，从测试源码文件解析 dws.run() 参数
            if not cmd:
                cmd = self._extract_cmd_from_test_file(test)
            cmd = self._normalize_cmd_from_dws(cmd)
            if created_base_name and "<刚创建的baseName>" in cmd:
                cmd = cmd.replace("<刚创建的baseName>", created_base_name)
            # 只接受“真实命令”：出现占位变量（如 <name>）时视为无效，避免误导报告。
            if cmd and re.search(r"<[^>]+>", cmd):
                cmd = ""
            reason = failed_reason_map.get(test, "")
            failures.append({
                "test": test,
                "type": "failed",
                "cmd": cmd,
                "layer": self._classify_failure_layer("failed", reason),
                "reason": reason,
            })
        for test in error_tests:
            cmd = next(cmd_iter, "")
            # ② 日志无 cmd，查 PRODUCT_DETAIL_CASES
            if not cmd:
                cmd = self._lookup_cmd_from_detail_cases(product, test)
            # ③ 还没找到，从测试源码文件解析 dws.run() 参数
            if not cmd:
                cmd = self._extract_cmd_from_test_file(test)
            cmd = self._normalize_cmd_from_dws(cmd)
            if created_base_name and "<刚创建的baseName>" in cmd:
                cmd = cmd.replace("<刚创建的baseName>", created_base_name)
            # 只接受“真实命令”：出现占位变量（如 <id>）时视为无效，避免误导报告。
            if cmd and re.search(r"<[^>]+>", cmd):
                cmd = ""
            # ERROR 类型确实没有用户命令（如 fixture setup 崩溃），才标记
            if not cmd:
                cmd = "[fixture/setup 错误，未执行到 CLI 命令]"
            failures.append({
                "test": test,
                "type": "error",
                "cmd": cmd,
                "layer": self._classify_failure_layer("error", ""),
                "reason": "fixture/setup 阶段失败",
            })

        return failures

    def _classify_failure_layer(self, failure_type: str, reason: str) -> str:
        """将失败归类到统一判定层：CLI 层 / 测试代码层 / 前置数据层。"""
        if failure_type == "error":
            return "fixture/setup失败（前置数据层）"
        r = (reason or "").lower()
        cli_markers = [
            "dws returned non-json",
            "expected success",
            "command returned error",
            "mcp_tool_error",
            "timeoutexpired",
            "auth_permission_denied",
            "unknown flag",
        ]
        if any(m in r for m in cli_markers):
            return "命令执行失败（CLI 层）"
        return "用例断言失败（测试代码层）"

    def _normalize_cmd_from_dws(self, cmd: str) -> str:
        """将命令规范为从 dws 开始，去掉本机绝对路径前缀和 shell 引号。"""
        if not cmd:
            return ""
        cmd = cmd.strip()
        # 兼容日志里出现的绝对路径形式：/path/to/dws xxx
        m = re.search(r'(?:^|[\s\'"`])(?:[^\s\'"`]*/)?(dws\b.*)$', cmd)
        if m:
            cmd = m.group(1).strip()
        # 去掉 shlex.quote 产生的单/双引号，保证比较一致
        cmd = re.sub(r"""(?<!\w)['"]([^'"]+)['"](?!\w)""", r'\1', cmd)
        return cmd

    def _extract_cmd_near_test_line(self, log_text: str, test_id: str) -> str:
        """
        从 last_run.log 中按“就近原则”提取某个测试用例对应的真实执行命令。

        规则：
        - 优先向上回溯查找 `DWS_CMD: ...`（由 conftest.DWSRunner 统一打印）
        - 若未找到，再回溯 `E     cmd:    dws ...`（非 JSON 失败块）
        - 仅在同一个 test 结果行附近回溯有限行数，避免串行用例串台
        """
        if not log_text or not test_id:
            return ""
        lines = log_text.splitlines()
        hit_idxs: List[int] = []
        for i, line in enumerate(lines):
            if test_id in line and (
                " PASSED" in line
                or " FAILED" in line
                or " ERROR" in line
                or " SKIPPED" in line
            ):
                hit_idxs.append(i)
        if not hit_idxs:
            return ""

        idx = hit_idxs[0]

        # 先向下扫描：失败详情块里的 `E     cmd:` / captured stdout 的 `DWS_CMD:` 往往在测试行之后
        end = min(len(lines) - 1, idx + 140)
        for j in range(idx, end + 1):
            l = lines[j].strip()
            if l.startswith("E     cmd:"):
                cmd = l.split("E     cmd:", 1)[1].strip()
                return self._normalize_cmd_from_dws(cmd)
            if l.startswith("DWS_CMD:"):
                cmd = l.split("DWS_CMD:", 1)[1].strip()
                return self._normalize_cmd_from_dws(cmd)

        # 再向上扫描：对 PASSED/部分 SKIPPED 用例，DWS_CMD 可能出现在测试行之前
        start = max(0, idx - 80)
        for j in range(idx, start, -1):
            l = lines[j].strip()
            if l.startswith("DWS_CMD:"):
                cmd = l.split("DWS_CMD:", 1)[1].strip()
                return self._normalize_cmd_from_dws(cmd)
            if l.startswith("E     cmd:"):
                cmd = l.split("E     cmd:", 1)[1].strip()
                return self._normalize_cmd_from_dws(cmd)
        return ""

    def _extract_cmd_from_test_file(self, test_id: str) -> str:
        """
        从测试源码文件中解析指定 test 方法里的 dws.run/run_ok/run_raw 调用，
        还原为 dws <args...> 命令字符串。
        适用于断言失败类用例——日志只有 AssertionError，无 cmd 行。
        务必提取出来，不允许返回空字符串。
        """
        # test_id 格式: product/test_xxx.py::ClassName::test_method
        parts = test_id.split("::")
        if len(parts) < 2:
            return ""
        file_part = parts[0]  # e.g. contact/test_01_user.py
        method_name = parts[-1]  # e.g. test_search_self_mobile

        test_file = self.base_dir / file_part
        if not test_file.exists():
            return ""

        try:
            src = test_file.read_text(encoding="utf-8")
        except Exception:
            return ""

        # 找到方法定义，提取方法体
        method_pattern = re.compile(
            r'def\s+' + re.escape(method_name) + r'\s*\([^)]*\).*?(?=\n    def |\nclass |\Z)',
            re.DOTALL
        )
        m = method_pattern.search(src)
        if not m:
            return ""
        method_body = m.group(0)

        # 从方法体中提取第一个 dws.run/run_ok/run_raw 调用的参数
        # 匹配: dws.run("a", "b", "c", ...) 或 dws.run_ok(...) / dws.run_raw(...)
        run_pattern = re.compile(
            r'dws\.run(?:_ok|_raw)?\s*\(([^)]+)\)',
            re.DOTALL
        )
        rm = run_pattern.search(method_body)
        if not rm:
            return ""

        args_str = rm.group(1)
        # 提取所有 token（仅接受字符串字面量）
        # 说明：为了避免报告里出现 <var> 这类占位符误导读者，
        # 这里不再把变量名替换成占位变量；一旦参数依赖变量，直接返回空串，
        # 由上层回退到日志提示“未捕获真实命令”。
        # 按逗号/换行分隔，提取每个 token
        raw_tokens = re.split(r'[,\n]+', args_str)
        processed = []
        has_non_literal = False
        for tok in raw_tokens:
            tok = tok.strip()
            # 字符串字面量
            m_str = re.match(r'^["\'](.+)["\']$', tok)
            if m_str:
                processed.append(m_str.group(1))
            # 变量名/表达式一律视为“非真实值”
            elif tok and tok not in ('self', 'dws'):
                has_non_literal = True
        if has_non_literal:
            return ""
        if not processed:
            return ""

        # 将选项名（-- 开头）和其对应值拼在一起，建立命令行
        # 格式: ["calendar", "event", "create", "--title", title, ...]
        # 产品 + 子命令在前，选项在后
        # 过滤掉 -f 后面的 json 字符串（稍后统一加）
        cleaned = [t for t in processed if t not in ('json',)]
        cmd = "dws " + " ".join(cleaned)
        # 附加 -f json（测试几乎都用这个格式）
        if "-f" not in cmd and "json" not in cmd:
            cmd += " -f json"
        return cmd

    def _lookup_cmd_from_detail_cases(self, product: str, test_name: str) -> str:
        """从 PRODUCT_DETAIL_CASES 中查找对应测试的命令"""
        detail = PRODUCT_DETAIL_CASES.get(product, {})
        test_short = test_name.split("::")[-1]  # 获取测试方法名

        # 在 failed 和 errors 列表中查找
        for item in detail.get("failed", []) + detail.get("errors", []):
            testcase = item.get("testcase", "")
            if test_short in testcase or testcase.endswith(test_short):
                return item.get("cmd", "")
        return ""
    
    def run_all(self) -> Dict:
        """运行所有产品测试"""
        print("=" * 80)
        print("DWS CLI 全产品测试运行器")
        print(f"开始时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        print("=" * 80)
        print()
        
        for product in self.products:
            self.results[product] = self.run_product_tests(product)
        
        return self.results
    
    @staticmethod
    def _cn_num(n: int) -> str:
        """整数转中文序号（1→一, 2→二, …, 10→十）"""
        cn = ["零", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"]
        return cn[n] if n < len(cn) else str(n)

    def _calc_pass_rate(self, result: Dict) -> float:
        """计算通过率（排除 xfail/skip/CLI报错，仅 passed/(passed+failed+error)）"""
        passed = result.get("passed", 0)
        failed = result.get("failed", 0)
        error = result.get("error", 0)
        total = passed + failed + error
        return round(passed / total * 100, 1) if total > 0 else -1.0  # -1 表示无可执行用例

    def _pass_rate_emoji(self, rate: float) -> str:
        """通过率 → 状态色块（100% PASS，<0 表示全 skip 显示 SKIP）"""
        if rate < 0:
            return "⏭ SKIP"
        elif rate >= 100.0:
            return "✅ PASS"
        else:
            return "❌ FAIL"

    def _extract_skip_reason_from_conftest(self, product: str) -> str:
        """从产品 conftest.py 中提取 _SKIP_REASON 常量作为跳过备注。"""
        conftest = self.base_dir / product / "conftest.py"
        if not conftest.exists():
            return ""
        try:
            text = conftest.read_text(encoding="utf-8")
            m = re.search(r'_SKIP_REASON\s*=\s*["\'](.+?)["\']', text)
            return m.group(1) if m else ""
        except Exception:
            return ""

    def _materialize_cmd_template(self, cmd: str, stderr: str) -> str:
        """
        将报告模板中的占位命令尽量替换成本次运行真实值。

        当前支持：
        - <刚创建的baseName>：从断言日志 `(...baseName...)` 中提取
        """
        if not cmd:
            return cmd
        if "<刚创建的baseName>" in cmd:
            # 示例:
            # Base xxx (CLI_Test_Base_1774250563_96c854) not found in search results ...
            m = re.search(r"\((CLI_Test_Base_[^)]+)\)", stderr or "")
            if m:
                return cmd.replace("<刚创建的baseName>", m.group(1))
        return cmd

    def _cmd_family(self, cmd: str) -> str:
        """
        提取命令族标识：`dws <product> <module> <action>`（最多前 4 段）。
        用于判断运行时命令是否可安全覆盖模板命令。
        """
        if not cmd:
            return ""
        parts = cmd.strip().split()
        if len(parts) >= 4:
            return " ".join(parts[:4])
        return " ".join(parts)

    def _extract_cli_errors_from_log(self, log_text: str) -> List[Dict[str, str]]:
        """
        从 last_run.log 中提取 CLI 真实报错块（run_raw 拼接，或 pytest 失败块同格式）。
        返回: [{"cmd": "...", "stderr": "...", "stdout": "..."}]
        """
        if not log_text:
            return []
        # 使用 [\s\S]*? 匹配多行内容，并通过 lookahead 断言定位边界
        matches = re.findall(
            r'^E\s+Failed:\s+dws returned non-JSON:\s*$\n'
            r'^E\s+cmd:\s+(.*?)\s*$\n'
            r'^E\s+stdout:\s*([\s\S]*?)\s*\n'
            r'^E\s+stderr:\s*([\s\S]*?)(?=^E\s+Failed:|$)',
            log_text,
            re.MULTILINE
        )
        items: List[Dict[str, str]] = []
        seen: set[str] = set()
        for cmd_raw, stdout_raw, stderr_raw in matches:
            cmd = self._normalize_cmd_from_dws(cmd_raw)
            if not cmd or cmd in seen:
                continue
            seen.add(cmd)
            items.append(
                {
                    "cmd": cmd,
                    "stdout": (stdout_raw or "").strip(),
                    "stderr": (stderr_raw or "").strip(),
                }
            )
        return items

    def _extract_skip_reasons_from_log(self, log_text: str) -> Dict[str, str]:
        """
        从 pytest 日志提取 skipped 原因，返回 test_id -> reason。
        兼容两种来源：
        1) 进度行：xxx::test_xxx SKIPPED (reason)
        2) short summary：SKIPPED [n] path.py:line: reason
        """
        reasons: Dict[str, str] = {}
        if not log_text:
            return reasons

        # 1) 进度行中携带的 reason（优先）
        inline_matches = re.findall(
            r'^(\S+test_[\w/]+\.py::[\w:]+)\s+SKIPPED(?:\s*\(([^)]+)\))?',
            log_text,
            re.MULTILINE,
        )
        for test_id, reason in inline_matches:
            txt = (reason or "").strip()
            if txt:
                reasons[test_id] = txt

        # 2) short test summary info 中的 SKIPPED 原因
        # 典型：SKIPPED [1] contact/test_x.py:12: xxx reason
        # 或（pytest_collection_modifyitems 无行号）：SKIPPED [27] tb/test_01_project.py: reason
        summary_matches = re.findall(
            r'^SKIPPED\s+\[\d+\]\s+(\S+test_[\w/]+\.py)(?::\d+)?:\s*(.+)$',
            log_text,
            re.MULTILINE,
        )
        summary_by_file: Dict[str, str] = {}
        for file_path, reason in summary_matches:
            file_only = os.path.basename(file_path)
            summary_by_file.setdefault(file_only, (reason or "").strip())

        if summary_by_file:
            for test_id, _ in inline_matches:
                if reasons.get(test_id):
                    continue
                file_only = os.path.basename(test_id.split("::")[0])
                if file_only in summary_by_file:
                    reasons[test_id] = summary_by_file[file_only]

        return reasons

    @staticmethod
    def _infer_skip_reason_from_rules(test_id: str = "", test_name: str = "", cmd: str = "") -> str:
        """
        当 pytest 未输出 skip reason 时，按约定规则兜底推断：
        - 参数别名/参数黏连 -> 开源版CLI尚未实现
        - 黑名单命令 -> 开源版CLI业务能力暂不支持
        """
        alias_reason = "开源版CLI尚未实现"
        biz_reason = "开源版CLI业务能力暂不支持"

        id_or_name = f"{(test_id or '').lower()} {(test_name or '').lower()}"
        if re.search(r"wrong_.*_flag", id_or_name) or "sticky" in id_or_name:
            return alias_reason

        cmd_l = (cmd or "").lower()
        blacklist_cmd_signatures = (
            ("chat", "message", "send"),
            ("chat", "message", "list-topic-replies"),
            ("chat", "message", "list"),
            ("contact", "dept", "list-children"),
        )
        if any(all(tok in cmd_l for tok in sig) for sig in blacklist_cmd_signatures):
            return biz_reason

        return "未提供（pytest 未输出 skip reason）"

    def generate_report(self) -> str:
        """生成 Markdown 报告（悟空评测风格）"""
        now = datetime.now()
        report_lines = []

        # ── 标题 ──────────────────────────────────────────────────────────────
        report_lines.append(f"# DWS CLI MCP 技能评测报告 — {now.strftime('%Y-%m-%d')}")
        report_lines.append("")
        report_lines.append(f"**生成时间:** {now.strftime('%Y-%m-%d %H:%M:%S')}  ")
        report_lines.append(f"**测试目录:** `{self.base_dir}`")
        report_lines.append(f"**DWS_BIN:** `{self.dws_bin}`")
        report_lines.append("")

        # ── 1. 评测概览 ────────────────────────────────────────────────────────
        total_passed = total_failed = total_error = total_skipped = total_cli_errors = 0
        category_stats = {"all_pass": [], "partial": [], "all_skip": [], "exception": [], "missing": []}
        product_rates: Dict[str, float] = {}
        executed_products: List[str] = []

        for product in self.products:
            result = self.results.get(product, {})
            if result.get("status") == "skipped" and result.get("reason") == "directory_not_found":
                category_stats["missing"].append(product)
                continue
            executed_products.append(product)
            p = result.get("passed", 0)
            f = result.get("failed", 0)
            e = result.get("error", 0)
            s = result.get("skipped", 0)
            c = result.get("cli_error_count", 0)
            total_passed += p
            total_failed += f
            total_error += e
            total_skipped += s
            total_cli_errors += c
            # 通过率 = passed / (passed + failed + error)，排除 skip 和 CLI 报错
            score_total = p + f + e
            rate = round(p / score_total * 100, 1) if score_total > 0 else -1.0  # -1 表示全 skip
            product_rates[product] = rate
            if result.get("status") in ["timeout", "exception"]:
                category_stats["exception"].append(product)
            elif rate < 0:
                # 全部跳过，无可执行用例
                category_stats["all_skip"].append(product)
            elif rate >= 100.0:
                category_stats["all_pass"].append(product)
            else:
                category_stats["partial"].append(product)

        total_cases = total_passed + total_failed + total_error + total_skipped  # 用例数包含 skipped
        # 综合通过率排除 skip 和 CLI 报错
        overall_score_total = total_passed + total_failed + total_error
        overall_rate = round(total_passed / overall_score_total * 100, 1) if overall_score_total > 0 else 0.0

        report_lines.append("## 1. 评测概览")
        report_lines.append("")
        report_lines.append(
            f"本轮共执行用例 **{total_cases} 个**（通过 **{total_passed}** / 失败 **{total_failed}** / 错误 **{total_error}** / 跳过 **{total_skipped}**）。综合通过率 **{overall_rate}%**。"
        )
        report_lines.append(
            f"另有 **{total_cli_errors} 个 CLI 命令执行错误**（真实用户场景补充，辅助定位用例错误根因，不一定是业务逻辑缺陷）。"
        )
        report_lines.append("")
        report_lines.append(
            f"- ✅ **全部通过 ({len(category_stats['all_pass'])} 个)**: "
            + (', '.join(PRODUCT_NAMES.get(p, p) for p in category_stats['all_pass']) or '无')
        )
        report_lines.append(
            f"- ⚠️ **部分通过 ({len(category_stats['partial'])} 个)**: "
            + (', '.join(PRODUCT_NAMES.get(p, p) for p in category_stats['partial']) or '无')
        )
        report_lines.append(
            f"- 📌 **本次运行模块 ({len(executed_products)} 个)**: "
            + (', '.join(PRODUCT_NAMES.get(p, p) for p in executed_products) or '无')
        )
        if category_stats['all_skip']:
            report_lines.append(
                f"- ⏭ **全部跳过 ({len(category_stats['all_skip'])} 个)**: "
                + ', '.join(PRODUCT_NAMES.get(p, p) for p in category_stats['all_skip'])
            )
        if category_stats['exception']:
            report_lines.append(
                f"- ❌ **运行异常 ({len(category_stats['exception'])} 个)**: "
                + ', '.join(PRODUCT_NAMES.get(p, p) for p in category_stats['exception'])
            )
        if category_stats['missing']:
            report_lines.append(
                f"- ➖ **目录缺失 ({len(category_stats['missing'])} 个)**: "
                + ', '.join(category_stats['missing'])
            )
        report_lines.append("")

        # ── 2. 评分明细 ────────────────────────────────────────────────────────
        report_lines.append("## 2. 评分明细")
        report_lines.append("")
        report_lines.append("| 产品 | 优先级 | 用例数 | 通过 | 失败 | 错误 | 跳过 | CLI报错 | 通过率 | 结论 | 备注 |")
        report_lines.append("|------|--------|--------|------|------|------|------|--------|-----------|------|------|")

        for product in self.products:
            result = self.results.get(product, {})
            if result.get("status") == "skipped" and result.get("reason") == "directory_not_found":
                continue
            priority = PRIORITY_MAP.get(product, "P2")
            name = PRODUCT_NAMES.get(product, product)
            note = PRODUCT_NOTES.get(product, "")
            p = result.get("passed", 0)
            f = result.get("failed", 0)
            e = result.get("error", 0)
            s = result.get("skipped", 0)
            c = result.get("cli_error_count", 0)
            total = p + f + e + s  # 用例数包含 skipped
            rate = product_rates.get(product, -1.0)
            verdict = self._pass_rate_emoji(rate)
            rate_str = f"**{rate}%**" if rate >= 0 else "**—**"
            # 全部跳过时，从 conftest.py 提取跳过原因作为备注
            if rate < 0 and not note:
                note = self._extract_skip_reason_from_conftest(product)
            report_lines.append(
                f"| {name} | {priority} | {total} | {p} | {f} | {e} | {s} | {c} | {rate_str} | {verdict} | {note} |"
            )

        report_lines.append("")
        report_lines.append("> 通过率 = 通过 / (通过 + 失败 + 错误)，排除跳过和 CLI 报错。100% ✅ PASS；< 100% ❌ FAIL；全部跳过 ⏭ SKIP")
        report_lines.append("")

        # ── 3. 核心关注 ────────────────────────────────────────────────────────
        partial_with_issues = [
            p for p in category_stats["partial"] + category_stats["exception"]
            if p in PRODUCT_ISSUES
        ]
        if partial_with_issues:
            report_lines.append("## 3. 核心关注")
            report_lines.append("")
            for product in partial_with_issues:
                name = PRODUCT_NAMES.get(product, product)
                rate = product_rates.get(product, 0.0)
                issue = PRODUCT_ISSUES.get(product, "详见 last_run.log")
                note = PRODUCT_NOTES.get(product, "")
                note_str = f"（{note}）" if note else ""
                # 如果 PRODUCT_ISSUES 中已包含备注信息，不再重复附加
                issue = PRODUCT_ISSUES.get(product, "详见 last_run.log")
                if note and note in issue:
                    note_str = ""
                report_lines.append(f"- **{name}**（通过率 {rate}%）：{issue}{note_str}")
            report_lines.append("")

        # ── 4. 详细结果 ────────────────────────────────────────────────────────
        report_lines.append("## 4. 详细结果")
        report_lines.append("")

        for product in self.products:
            result = self.results.get(product, {})
            if result.get("status") == "skipped" and result.get("reason") == "directory_not_found":
                continue

            name = PRODUCT_NAMES.get(product, product)
            p = result.get("passed", 0)
            f = result.get("failed", 0)
            e = result.get("error", 0)
            s = result.get("skipped", 0)
            c = result.get("cli_error_count", 0)
            x = result.get("xfailed", 0)
            rate = product_rates.get(product, -1.0)
            verdict = self._pass_rate_emoji(rate)

            report_lines.append(f"### {name}（{product}）")
            report_lines.append("")
            if result.get("status") == "skipped":
                reason = result.get("reason", "unknown")
                report_lines.append(f"本模块本次未执行（原因：`{reason}`）。")
                report_lines.append("")
                continue
            rate_str = f"**{rate}%**" if rate >= 0 else "**—**"
            report_lines.append(
                f"通过 **{p}** / 失败 **{f}** / 错误 **{e}** / 跳过 {s} / CLI报错 {c} / xfail {x}　"
                f"→ 通过率 {rate_str} {verdict}"
            )

            product_note = PRODUCT_NOTES.get(product, "")
            if product_note:
                report_lines.append("")
                report_lines.append(f"> ⚠️ **备注**: {product_note}")

            failures = result.get("failures", [])
            if failures:
                report_lines.append("")
                report_lines.append("**失败用例（关联命令，前5条）:**")
                report_lines.append("")
                report_lines.append("| 序号 | 测试用例 | 关联命令 | 判定层 | 状态 |")
                report_lines.append("|:---:|:---|:---|:---|:---:|")
                for idx, item in enumerate(failures[:5], 1):
                    cmd = item.get("cmd", "")
                    if not cmd:
                        cmd_display = f"[未捕获真实命令，请查看 {product}/last_run.log]"
                    elif cmd.startswith("[") and cmd.endswith("]"):
                        cmd_display = cmd
                    else:
                        cmd_display = f"`{cmd}`"
                    test_name = item.get("test", "").split("::")[-1] or item.get("test", "")
                    layer = item.get("layer", "")
                    status = "❌ FAILED" if item["type"] == "failed" else "⚠️ ERROR"
                    report_lines.append(f"| {idx} | `{test_name}` | {cmd_display} | {layer} | {status} |")

            # CLI 报错项（从本次 last_run.log 解析，展示前5条）
            cli_error_items: List[Dict[str, str]] = []
            if c and c > 0:
                log_path_for_cli = self.base_dir / product / "last_run.log"
                if log_path_for_cli.exists():
                    try:
                        log_text_for_cli = log_path_for_cli.read_text(encoding="utf-8")
                    except Exception:
                        log_text_for_cli = ""
                    cli_error_items = self._extract_cli_errors_from_log(log_text_for_cli)

            if cli_error_items:
                report_lines.append("")
                report_lines.append("**CLI报错（关联命令，前5条）:**")
                report_lines.append("")
                report_lines.append("| 序号 | 测试用例 | 关联命令 | 判定层 | 状态 |")
                report_lines.append("|:---:|:---|:---|:---|:---:|")
                for idx, item in enumerate(cli_error_items[:5], 1):
                    cmd = item.get("cmd", "")
                    cmd_display = f"`{cmd}`" if cmd else f"[未捕获真实命令，请查看 {product}/last_run.log]"
                    report_lines.append(
                        f"| {idx} | `{('CLI_真实执行#' + str(idx))}` | {cmd_display} | 命令执行失败（CLI 层） | 💥 CLI_ERROR |"
                    )

            # 跳过项（从本次 last_run.log 解析）
            log_path = self.base_dir / product / "last_run.log"
            skipped_items: List[Dict[str, str]] = []
            if log_path.exists():
                try:
                    log_text = log_path.read_text(encoding="utf-8")
                except Exception:
                    log_text = ""

                # aitable 可能需要替换 <刚创建的baseName>（若命令解析到了该占位符）
                created_base_name = ""
                if product == "aitable" and log_text:
                    m_base = re.search(r"\[SETUP\]\s+Created test Base:\s+\S+\s+\(([^)]+)\)", log_text)
                    if m_base:
                        created_base_name = m_base.group(1).strip()

                skipped_matches = re.findall(
                    r'^(\S+test_[\w/]+\.py::[\w:]+)\s+SKIPPED(?:\s*\(([^)]+)\))?',
                    log_text,
                    re.MULTILINE
                )
                skip_reason_map = self._extract_skip_reasons_from_log(log_text)

                for test_full, reason in skipped_matches:
                    cmd = self._extract_cmd_from_test_file(test_full)
                    cmd = self._normalize_cmd_from_dws(cmd)
                    if created_base_name and cmd and "<刚创建的baseName>" in cmd:
                        cmd = cmd.replace("<刚创建的baseName>", created_base_name)
                    if cmd and re.search(r"<[^>]+>", cmd):
                        cmd = ""
                    if cmd:
                        test_name = test_full.split("::")[-1] if "::" in test_full else test_full
                        final_reason = (
                            (reason or "").strip()
                            or skip_reason_map.get(test_full, "").strip()
                            or self._infer_skip_reason_from_rules(
                                test_id=test_full, test_name=test_name, cmd=cmd
                            )
                        )
                        skipped_items.append({
                            "cmd": cmd,
                            "reason": final_reason,
                        })

            if skipped_items:
                report_lines.append("")
                report_lines.append("**跳过项:**")
                report_lines.append("")
                report_lines.append("| 序号 | 执行命令 | 跳过原因 |")
                report_lines.append("|:---:|:---|:---|")
                for idx, item in enumerate(skipped_items, 1):
                    cmd_display = f"`{item.get('cmd','')}`"
                    reason = item.get("reason", "")
                    report_lines.append(f"| {idx} | {cmd_display} | {reason} |")

            report_lines.append("")

        # ── 5. 日志文件 ────────────────────────────────────────────────────────
        report_lines.append("## 5. 日志文件")
        report_lines.append("")
        report_lines.append("```")
        for product in self.products:
            log_path = self.base_dir / product / "last_run.log"
            if log_path.exists():
                report_lines.append(f"{product}/last_run.log")
        report_lines.append("```")
        report_lines.append("")

        return "\n".join(report_lines)
    
    def generate_product_reports(self):
        """为每个有失败的产品生成详细错误报告（参考考勤.md格式），写入对应目录"""
        now_str = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        framework = "pytest 9.0.2 | Python 3.13.3"

        # 子报告必须覆盖所有类型（PASSED/FAILED/ERROR/SKIPPED/CLI报错），
        # 以本次实际运行结果（self.results）为准；标题等静态信息仅做展示。
        for product in self.products:
            product_dir = self.base_dir / product
            if not product_dir.exists():
                continue
            detail = PRODUCT_DETAIL_CASES.get(product, {})
            log_text = ""
            created_base_name = ""
            log_path = product_dir / "last_run.log"
            if log_path.exists():
                try:
                    log_text = log_path.read_text(encoding="utf-8")
                except Exception:
                    log_text = ""
            if product == "aitable" and log_text:
                m_base = re.search(r"\[SETUP\]\s+Created test Base:\s+\S+\s+\(([^)]+)\)", log_text)
                if m_base:
                    created_base_name = m_base.group(1).strip()

            # 必须基于本次 last_run.log 解析真实的 PASSED/FAILED/ERROR/SKIPPED 状态。
            title = detail.get("title") or f"{PRODUCT_NAMES.get(product, product)}（{product}）命令测试报告"
            note = detail.get("note", "")
            duration = detail.get("duration", "N/A")

            # pytest 输出中测试名可能被换行截断，导致正则无法匹配完整名称。
            # 因此采用两步策略：
            # 1. 从 FAILURES/ERRORS 部分标题行提取完整的方法名
            # 2. 从 summary 行提取文件路径
            # 3. 组合成完整的测试 ID
            
            def _extract_failed_error_tests(log_text: str, status: str) -> List[str]:
                """从 FAILURES 或 ERRORS 部分提取完整的测试 ID。"""
                # 提取标题行中的方法名，格式: _____________ TestClass.test_method _____________
                # 使用宽松匹配允许换行，然后过滤只保留测试方法名
                section_pattern = rf'=+ {status} =+(.*?)(?===+ |$)'
                section_match = re.search(section_pattern, log_text, re.DOTALL)
                methods = []
                if section_match:
                    all_methods = re.findall(r'_+[\s]*([\w.]+)[\s]*_+', section_match.group(1))
                    # 过滤只保留测试方法名：格式为 TestClass.test_method
                    # 即：包含 .test_ 且类名以 Test 开头
                    methods = [m for m in all_methods if '.test_' in m and m.startswith('Test')]
                
                # 从 summary 行提取文件路径，格式: FAILED chat/test_xxx.py::TestClass::...
                # status 是 FAILURES/ERRORS，summary 行前缀是 FAILED/ERROR
                status_prefix = 'FAILED' if status == 'FAILURES' else 'ERROR'
                files = re.findall(
                    rf'^{status_prefix}\s+(\S+test_[\w/]+\.py)(?::|\s)',
                    log_text,
                    re.MULTILINE
                )
                
                # 组合成完整的测试 ID
                result = []
                for f, m in zip(files, methods):
                    # m 格式是 TestClass.test_method，转换成 TestClass::test_method
                    parts = m.split('.')
                    if len(parts) == 2:
                        test_id = f'{f}::{parts[0]}::{parts[1]}'
                        result.append(test_id)
                return result
            
            failed_tests = _extract_failed_error_tests(log_text, 'FAILURES')
            error_tests = _extract_failed_error_tests(log_text, 'ERRORS')
            
            # PASSED 和 SKIPPED 通常没有 summary 部分，需要从测试进度行提取
            # 注意：这些行的测试名可能被换行截断，但我们可以尝试匹配
            passed_tests = re.findall(
                r'^(\S+test_[\w/]+\.py::[\w:]+)\s+PASSED\b',
                log_text,
                re.MULTILINE
            )
            skipped_matches = re.findall(
                r'^(\S+test_[\w/]+\.py::[\w:]+)\s+SKIPPED(?:\s*\(([^)]+)\))?',
                log_text,
                re.MULTILINE
            )
            skip_reason_map = self._extract_skip_reasons_from_log(log_text)

            # 仅从 last_run.log 解析 CLI 真实报错（单一来源）
            raw_error_cmds: Dict[str, str] = {}  # cmd -> stderr
            # 兼容 pytest 失败块样式（使用 [\s\S]*? 匹配多行 JSON）：
            # E   Failed: dws returned non-JSON:
            # E     cmd:    dws ...
            # E     stdout: {...多行 JSON...}
            # E     stderr: Error: ...
            legacy_err_matches = re.findall(
                r'^E\s+Failed:\s+dws returned non-JSON:\s*$\n'
                r'^E\s+cmd:\s+(.*?)\s*$\n'
                r'^E\s+stdout:\s*([\s\S]*?)\s*\n'
                r'^E\s+stderr:\s*([\s\S]*?)(?=^E\s+Failed:|$)',
                log_text,
                re.MULTILINE
            )
            for cmd_raw, _stdout_raw, stderr_raw in legacy_err_matches:
                cmd_norm = self._normalize_cmd_from_dws(cmd_raw)
                stderr = (stderr_raw or "").strip()
                if cmd_norm and ("error" in stderr.lower() or stderr):
                    raw_error_cmds[cmd_norm] = stderr

            # 失败详情块中 `E     cmd:` 按出现顺序与 FAILED 用例顺序一致，可用于精确配对
            ordered_e_cmds: List[str] = []
            for cmd_raw in re.findall(r'^E\s+cmd:\s+(.*)$', log_text, re.MULTILINE):
                cmd_norm = self._normalize_cmd_from_dws(cmd_raw)
                if cmd_norm:
                    ordered_e_cmds.append(cmd_norm)

            # passed 命令来自 last_run.log 中的 PASSED 用例；同一条命令可能对应多个用例，这里去重但保留第一条用例名用于备注
            passed_cmd_items: List[Dict[str, str]] = []
            cli_error_items: List[Dict[str, str]] = []
            seen_passed_cmds: Dict[str, str] = {}
            seen_error_cmds: Dict[str, str] = {}
            passed_case_items: List[Dict[str, str]] = []
            
            # 先建立 test_id -> cmd 的映射（从源码解析）
            test_to_cmd: Dict[str, str] = {}
            skipped_test_ids = [t[0] for t in skipped_matches]  # 提取 skipped 的 test_id
            for test_id in passed_tests + failed_tests + error_tests + skipped_test_ids:
                test_name = test_id.split("::")[-1]
                cmd = self._extract_cmd_from_test_file(test_id) or self._extract_cmd_near_test_line(log_text, test_id)
                cmd = self._normalize_cmd_from_dws(cmd)
                if created_base_name and cmd and "<刚创建的baseName>" in cmd:
                    cmd = cmd.replace("<刚创建的baseName>", created_base_name)
                if cmd and re.search(r"<[^>]+>", cmd):
                    cmd = ""
                test_to_cmd[test_id] = cmd
            
            # 建立 cmd -> test_name 的反向映射
            cmd_to_tests: Dict[str, List[str]] = {}
            for test_id, cmd in test_to_cmd.items():
                if cmd:
                    if cmd not in cmd_to_tests:
                        cmd_to_tests[cmd] = []
                    cmd_to_tests[cmd].append(test_id.split("::")[-1])
            
            # 直接从 raw_error_cmds 提取所有 CLI 报错（不依赖 passed_tests）
            for cmd, stderr in raw_error_cmds.items():
                if cmd not in seen_error_cmds:
                    seen_error_cmds[cmd] = ""
                    # 尝试关联到对应的测试用例
                    related_tests = cmd_to_tests.get(cmd, [])
                    test_name = related_tests[0] if related_tests else ""
                    cli_error_items.append({
                        "cmd": cmd,
                        "test": test_name,
                        "stderr": stderr,
                    })
            
            # 处理 passed_tests
            for test_id in passed_tests:
                test_name = test_id.split("::")[-1]
                cmd = test_to_cmd.get(test_id, "")
                passed_case_items.append({"test": test_name, "cmd": cmd})
                if not cmd:
                    continue
                # 如果该命令不在 CLI 报错中，才计入 passed_cmd_items
                if cmd not in raw_error_cmds and cmd not in seen_passed_cmds:
                    seen_passed_cmds[cmd] = test_name
                    passed_cmd_items.append({"cmd": cmd, "test": test_name})

            failed_items: List[Dict[str, str]] = []
            for idx_fail, test_id in enumerate(failed_tests):
                test_name = test_id.split("::")[-1]
                # FAILED 用例优先用失败块里的 E cmd（不会串台），其次再用 test_to_cmd 映射
                cmd = ordered_e_cmds[idx_fail] if idx_fail < len(ordered_e_cmds) else ""
                if not cmd:
                    cmd = test_to_cmd.get(test_id, "")
                failed_items.append({"test": test_name, "cmd": cmd})

            error_items: List[Dict[str, str]] = []
            for test_id in error_tests:
                test_name = test_id.split("::")[-1]
                cmd = test_to_cmd.get(test_id, "")
                error_items.append({"test": test_name, "cmd": cmd})

            skipped_items: List[Dict[str, str]] = []
            for test_id, reason in skipped_matches:
                # SKIPPED 用 test_to_cmd 映射
                cmd = test_to_cmd.get(test_id, "")
                test_name = test_id.split("::")[-1] if "::" in test_id else test_id
                final_reason = (
                    (reason or "").strip()
                    or skip_reason_map.get(test_id, "").strip()
                    or self._infer_skip_reason_from_rules(
                        test_id=test_id, test_name=test_name, cmd=cmd
                    )
                )
                skipped_items.append({
                    "test": test_name,
                    "cmd": cmd,
                    "reason": final_reason,
                })

            runtime_failures = self.results.get(product, {}).get("failures", [])

            lines = []
            lines.append(f"# {title}")
            lines.append("")
            lines.append(f"> 测试时间：{now_str} | 测试框架：{framework}")
            if note:
                lines.append(f"")
                lines.append(f"> ⚠️ **备注**: {note}")
            lines.append("")
            lines.append("---")
            lines.append("")

            section_num = 0

            # 成功的命令
            section_num += 1
            sec_label = self._cn_num(section_num)
            lines.append(f"## {sec_label}、成功用例（PASSED）")
            lines.append("")
            if passed_case_items:
                lines.append("| 序号 | 测试用例 | 关联命令 |")
                lines.append("|:---:|:---|:---|")
                for i, item in enumerate(passed_case_items, 1):
                    cmd = item.get("cmd", "")
                    cmd_display = f"`{cmd}`" if cmd else ""
                    lines.append(f"| {i} | `{item.get('test','')}` | {cmd_display} |")
            else:
                lines.append("_本产品本次无 PASSED 用例_")
            lines.append("")
            lines.append("---")
            lines.append("")

            # CLI 报错命令（本次失败）
            if cli_error_items:
                section_num += 1
                sec_label = self._cn_num(section_num)
                lines.append(f"## {sec_label}、CLI 报错命令（失败）")
                lines.append("")
                lines.append("> 以下命令返回了明确错误（结构化错误 JSON / 错误文本），按口径计为失败。")
                lines.append("")
                lines.append("| 序号 | 命令 | CLI 返回 | 关联用例 |")
                lines.append("|------|------|----------|----------|")
                for i, item in enumerate(cli_error_items, 1):
                    cmd = item.get("cmd", "")
                    stderr = item.get("stderr", "").replace("|", "\\|").replace("\n", " ")
                    if len(stderr) > 80:
                        stderr = stderr[:80] + "..."
                    test_name = item.get("test", "")
                    lines.append(f"| {i} | `{cmd}` | `{stderr}` | `{test_name}` |")
                lines.append("")
                lines.append("---")
                lines.append("")

            # 失败用例（FAILED）
            if failed_items:
                section_num += 1
                sec_label = self._cn_num(section_num)
                lines.append(f"## {sec_label}、失败用例（FAILED）")
                lines.append("")
                lines.append("| 序号 | 测试用例 | 关联命令 |")
                lines.append("|:---:|:---|:---|")
                for i, item in enumerate(failed_items, 1):
                    cmd = item.get("cmd", "")
                    cmd_display = f"`{cmd}`" if cmd else ""
                    lines.append(f"| {i} | `{item.get('test','')}` | {cmd_display} |")
                lines.append("")
                lines.append("---")
                lines.append("")

            # 错误用例（ERROR）
            if error_items:
                section_num += 1
                sec_label = self._cn_num(section_num)
                lines.append(f"## {sec_label}、错误用例（ERROR）")
                lines.append("")
                lines.append("| 序号 | 测试用例 | 关联命令 |")
                lines.append("|:---:|:---|:---|")
                for i, item in enumerate(error_items, 1):
                    cmd = item.get("cmd", "")
                    cmd_display = f"`{cmd}`" if cmd else ""
                    lines.append(f"| {i} | `{item.get('test','')}` | {cmd_display} |")
                lines.append("")
                lines.append("---")
                lines.append("")

            # 跳过的命令
            if skipped_items:
                section_num += 1
                sec_label = self._cn_num(section_num)
                lines.append(f"## {sec_label}、跳过用例（SKIPPED）")
                lines.append("")
                lines.append(f"| 序号 | 测试用例 | 执行命令 | 跳过原因 |")
                lines.append("|:---:|:---|:---|:---|")
                for i, item in enumerate(skipped_items, 1):
                    cmd = item.get('cmd', '')
                    reason = item.get('reason', '')
                    cmd_display = f"`{cmd}`" if cmd else ""
                    lines.append(f"| {i} | `{item.get('test','')}` | {cmd_display} | {reason} |")
                lines.append("")
                lines.append("---")
                lines.append("")
            
            # 失败/错误分析（按判定层分组，来自 self.results 的解析）
            if runtime_failures:
                lines.append("## 问题描述")
                lines.append("")
                from collections import defaultdict
                groups = defaultdict(list)
                for item in runtime_failures:
                    key = item.get("layer", "未知")
                    groups[key].append(item)
                for layer, items in groups.items():
                    cmd_names = []
                    seen = set()
                    for it in items:
                        c = it.get("cmd", "")
                        if not c or c in seen:
                            continue
                        seen.add(c)
                        parts = c.split()
                        cmd_names.append(" ".join(parts[:4]) if len(parts) >= 4 else c)
                    cmds_str = "、".join(cmd_names[:5]) or "N/A"
                    analysis = items[0].get("reason", "")
                    count = len(items)
                    lines.append(f"**{layer}**（{count} 条，涉及命令：{cmds_str}）")
                    if analysis:
                        lines.append(f"> {analysis}")
                    lines.append("")
                lines.append("---")
                lines.append("")
            
            # 统计汇总
            result = self.results.get(product, {})
            p = result.get("passed", 0)
            f_cnt = result.get("failed", 0)
            e_cnt = result.get("error", 0)
            s_cnt = result.get("skipped", 0)
            c_cnt = result.get("cli_error_count", 0)
            total = p + f_cnt + e_cnt + s_cnt
            score_total = p + f_cnt + e_cnt + c_cnt
            rate = round(p / score_total * 100, 1) if score_total > 0 else 0.0
            
            section_num += 1
            sec_label = self._cn_num(section_num)
            lines.append(f"## {sec_label}、统计汇总")
            lines.append("")
            lines.append("| 指标 | 数值 |")
            lines.append("|------|------|") 
            lines.append(f"| 总测试数 | {total} |")
            lines.append(f"| 通过（PASSED） | {p} |")
            lines.append(f"| 失败（FAILED） | {f_cnt} |")
            if e_cnt > 0:
                lines.append(f"| 错误（ERROR） | {e_cnt} |")
            if s_cnt > 0:
                lines.append(f"| 跳过（SKIPPED） | {s_cnt} |")
            if c_cnt > 0:
                lines.append(f"| CLI报错（非预期命令错误） | {c_cnt} |")
            lines.append(f"| 通过率 | {rate}% |")
            lines.append(f"| 运行时长 | {duration} |")
            lines.append("")

            # 输出文件
            output_file = product_dir / f"{PRODUCT_NAMES.get(product, product)}.md"
            with open(output_file, 'w', encoding='utf-8') as fh:
                fh.write("\n".join(lines))
            print(f"  📄 {product} 报告: {output_file}")

    def save_report(self, output_file: str = None):
        """保存报告"""
        if output_file is None:
            output_file = self.base_dir / f"test_report_{self.timestamp}.md"
        
        report = self.generate_report()
        with open(output_file, 'w', encoding='utf-8') as f:
            f.write(report)
        
        print(f"\n📄 报告已保存: {output_file}")
        return output_file
    
    def print_summary(self):
        """打印汇总"""
        print("\n" + "=" * 80)
        print("测试汇总")
        print("=" * 80)
        
        total_passed = 0
        total_failed = 0
        total_error = 0
        total_skipped = 0
        total_cli_errors = 0
        
        for product, result in self.results.items():
            if result.get("status") != "skipped":
                total_passed += result.get("passed", 0)
                total_failed += result.get("failed", 0)
                total_error += result.get("error", 0)
                total_skipped += result.get("skipped", 0)
                total_cli_errors += result.get("cli_error_count", 0)
        
        print(
            f"总计: {total_passed} 通过, {total_failed} 失败, "
            f"{total_error} 错误, {total_skipped} 跳过, {total_cli_errors} CLI报错"
        )
        print("=" * 80)


def check_dws_installed() -> bool:
    """检查 dws 是否已安装"""
    try:
        dws_bin = resolve_dws_bin(__file__)
        if dws_bin != "dws":
            return Path(dws_bin).exists()
        result = subprocess.run(["which", "dws"], capture_output=True, timeout=5)
        return result.returncode == 0
    except:
        return False


def check_dws_authenticated(dws_bin: str) -> bool:
    """检查 dws 是否已 OAuth 登录（与 `dws auth status -f json` 一致）。"""
    try:
        result = subprocess.run(
            [dws_bin, "auth", "status", "-f", "json"],
            capture_output=True,
            text=True,
            timeout=60,
        )
        if result.returncode != 0:
            return False
        data = json.loads(result.stdout)
        return bool(data.get("authenticated"))
    except Exception:
        return False


def main():
    """主函数"""
    import argparse
    
    parser = argparse.ArgumentParser(description="DWS CLI 全产品测试运行器")
    parser.add_argument(
        "--edition",
        choices=["wukong", "open"],
        default="wukong",
        help="产品清单版本：wukong(悟空版) 或 open(开源版)",
    )
    parser.add_argument("--products", help="指定产品，逗号分隔，默认全部")
    parser.add_argument("--output", help="报告输出路径")
    parser.add_argument(
        "--skip-auth-check",
        action="store_true",
        help="跳过 dws 登录状态检查（仅用于排障，常规评测勿用）",
    )
    parser.add_argument(
        "--skip-open-unimplemented-param-cases",
        action="store_true",
        help="跳过开源版未实现能力相关用例（参数别名/黏连 + 黑名单业务命令）",
    )
    parser.add_argument(
        "--force-products",
        default=os.environ.get("DWS_FORCE_PRODUCTS", ""),
        help="强制运行默认跳过的产品，逗号分隔（如 ding,bot）。覆盖产品 conftest 中的全局 skip。"
             " 也可通过环境变量 DWS_FORCE_PRODUCTS 设置。",
    )
    args = parser.parse_args()
    
    base_dir = Path(__file__).parent
    
    # 检查 dws 安装
    dws_installed = check_dws_installed()
    
    print("=" * 80)
    print("DWS CLI 全产品测试运行器")
    print(f"开始时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 80)
    print()
    
    dws_bin = resolve_dws_bin(__file__)

    if dws_installed:
        print("✅ dws CLI 已安装")
        print()
    else:
        print("⚠️  警告: dws CLI 未安装")
        print()

    if dws_installed and not args.skip_auth_check:
        if not check_dws_authenticated(dws_bin):
            print("❌ 未检测到有效 OAuth 登录（`dws auth status` 未返回已登录）。")
            print()
            print("请先执行：")
            print(
                "  dws auth login --client-id dingi8foprfi3jynjjlu "
                "--client-secret rYz_expXcJ0mj8G17FF_3rAp-Y6Pxo-il3yihlP4ThneMlbr6wgRIBujuFXr28DZ"
            )
            print("浏览器授权时请选组织「钉钉iPaaS」。")
            print()
            print("说明见 auto-test/cli_to_mcp/README.md「OAuth 登录」；排障可加 --skip-auth-check。")
            sys.exit(1)

    profile_products = list(PRODUCT_PROFILES[args.edition])
    runner = TestRunner(
        base_dir,
        products=profile_products,
        edition=args.edition,
        skip_open_unimplemented_param_cases=args.skip_open_unimplemented_param_cases,
        force_products=args.force_products,
    )
    print(f"🧭 本轮使用 dws: {runner.dws_bin}")
    print(f"🧩 产品版本: {args.edition} ({len(profile_products)} 个产品)")
    print()
    
    # 如果指定了产品
    if args.products:
        products = [p.strip() for p in args.products.split(",")]
        unknown = [p for p in products if p not in runner.products]
        if unknown:
            print(f"❌ 指定产品不在 {args.edition} 版本清单中: {', '.join(unknown)}")
            sys.exit(1)
        # Scope runner to only the specified products so reports don't
        # overwrite other products' .md files with stale/empty data.
        runner.products = products
        for product in products:
            runner.results[product] = runner.run_product_tests(product)
    else:
        # 运行全部
        runner.run_all()
    
    # 打印汇总
    runner.print_summary()
    
    # 生成各产品详细报告
    print("\n📝 生成各产品详细报告...")
    runner.generate_product_reports()

    # 保存报告
    output_path = args.output
    if output_path is None:
        report_dir = base_dir.parent / ("wukong_report" if args.edition == "wukong" else "open_report")
        report_dir.mkdir(parents=True, exist_ok=True)
        output_path = report_dir / f"test_report_{runner.timestamp}.md"
    runner.save_report(output_path)


if __name__ == "__main__":
    main()
