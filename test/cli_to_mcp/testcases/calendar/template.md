# 日历（calendar）命令测试报告

> 测试时间：2026-03-30 11:59:19 | 测试框架：pytest 9.0.2 | Python 3.13.3

---

## 一、成功用例（PASSED）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_list_default` | `dws calendar event list -f json` |
| 2 | `test_list_with_time_range` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 3 | `test_list_empty_range` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 4 | `test_get_invalid_id` | `dws calendar event get --id INVALID_EVENT_ID_99999 -f json` |
| 5 | `test_delete_invalid_id` | `dws calendar event delete --id INVALID_99999 --yes -f json` |
| 6 | `test_list_invalid_event` | `dws calendar participant list --event INVALID_EVENT_99999 -f json` |
| 7 | `test_search_returns_list` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 8 | `test_search_different_time_ranges` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 9 | `test_search_past_time_range` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 10 | `test_add_to_invalid_event` | `dws calendar room add --event INVALID_EVENT_99999 --rooms ROOM_DUMMY -f json` |
| 11 | `test_list_groups_returns_data` | `dws calendar room list-groups -f json` |
| 12 | `test_list_groups_is_list` | `dws calendar room list-groups -f json` |
| 13 | `test_list_groups_idempotent` | `dws calendar room list-groups -f json` |
| 14 | `test_event_create_time_with_space_separator` | `dws calendar event create --title 时间格式测试 --start 2026-03-23 14:00:00 --end 2026-03-23 15:00:00 -f json` |
| 15 | `test_event_create_time_without_timezone` | `dws calendar event create --title 无时区测试 --start 2026-03-23T14:00:00 --end 2026-03-23T15:00:00 -f json` |
| 16 | `test_room_add_invalid_room_should_fail` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |

---

## 二、失败用例（FAILED）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_create_basic` | `dws calendar event get --id INVALID_EVENT_ID_99999 -f json` |
| 2 | `test_create_with_desc` | `dws calendar event delete --id INVALID_99999 --yes -f json` |
| 3 | `test_create_verify_via_get` | `dws calendar participant list --event INVALID_EVENT_99999 -f json` |
| 4 | `test_delete_lifecycle` | `dws contact user get-self -f json` |
| 5 | `test_delete_and_redelete` | `dws calendar room search --start 2026-03-30T13:58:52+08:00 --end 2026-03-30T14:58:52+08:00 -f json` |
| 6 | `test_delete_from_invalid_event` | `dws calendar room search --start 2026-03-30T15:58:52+08:00 --end 2026-03-30T16:58:52+08:00 -f json` |

---

## 三、错误用例（ERROR）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_get_returns_detail` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 2 | `test_get_contains_summary` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 3 | `test_update_title` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 4 | `test_update_time` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 5 | `test_update_only_end_time` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 6 | `test_list_participants` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 7 | `test_list_contains_creator` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 8 | `test_add_participant` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 9 | `test_add_multiple_users` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 10 | `test_remove_then_verify` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 11 | `test_remove_invalid_user` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 12 | `test_add_room_to_event` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 13 | `test_add_invalid_room` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 14 | `test_delete_room_from_event` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |
| 15 | `test_delete_multiple_rooms` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |

---

## 四、跳过用例（SKIPPED）

| 序号 | 测试用例 | 执行命令 | 跳过原因 |
|:---:|:---|:---|:---|
| 1 | `test_add_to_invalid_event` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |  |
| 2 | `test_remove_from_invalid_event` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |  |
| 3 | `test_query_self_busy` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |  |
| 4 | `test_query_multiple_users` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |  |
| 5 | `test_query_past_range` | `dws calendar event create --title CLI_Test_Event_1774843126_842c2b --start 2026-03-30T13:58:46+08:00 --end 2026-03-30T14:58:46+08:00 -f json` |  |

---

## 问题描述

**用例断言失败（测试代码层）**（8 条，涉及命令：dws calendar event create、dws calendar event create、dws calendar event create、dws calendar event create、dws calendar event create）
> Assert...

**fixture/setup失败（前置数据层）**（24 条，涉及命令：dws calendar room delete、[fixture/setup 错误，未执行到 CLI 命令]）
> fixture/setup 阶段失败

---

## 五、统计汇总

| 指标 | 数值 |
|------|------|
| 总测试数 | 36 |
| 通过（PASSED） | 10 |
| 失败（FAILED） | 6 |
| 错误（ERROR） | 15 |
| 跳过（SKIPPED） | 5 |
| CLI报错（非预期命令错误） | 19 |
| 通过率 | 20.0% |
| 运行时长 | 73.45s |
