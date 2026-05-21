# AI 多维表（aitable）命令测试报告

> 测试时间：2026-04-13 18:34:13 | 测试框架：pytest 9.0.2 | Python 3.13.3

---

## 一、成功用例（PASSED）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_list_returns_bases` | `dws aitable base list -f json` |
| 2 | `test_list_with_limit` | `dws aitable base list --limit 1 -f json` |
| 3 | `test_list_pagination` | `dws aitable base list --limit 1 -f json` |
| 4 | `test_search_returns_structure` | `dws aitable base search --query 测试 -f json` |
| 5 | `test_search_no_match` | `dws aitable base search --query ZZZZ_NonExistent_99999 -f json` |
| 6 | `test_get_returns_structure` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 7 | `test_update_status_should_be_success` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 8 | `test_update_name_effective` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 9 | `test_update_with_desc` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 10 | `test_create_and_delete_lifecycle` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 11 | `test_create_with_template` | `dws aitable template search --query 项目 -f json` |
| 12 | `test_get_all_tables` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 13 | `test_get_by_specific_ids` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 14 | `test_create_with_multiple_field_types` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 15 | `test_create_minimal_table` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 16 | `test_rename_table` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 17 | `test_delete_table` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 18 | `test_get_all_fields` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 19 | `test_get_by_field_ids` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 20 | `test_create_text_field` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 21 | `test_create_multiple_types` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 22 | `test_create_currency_field` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 23 | `test_create_progress_field` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 24 | `test_update_field_name` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 25 | `test_delete_field` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 26 | `test_create_single_record` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 27 | `test_create_batch_records` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 28 | `test_create_minimal_record` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 29 | `test_query_all` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 30 | `test_query_by_record_ids` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 31 | `test_query_with_keyword` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 32 | `test_query_with_limit` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 33 | `test_query_with_field_ids` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 34 | `test_update_record` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 35 | `test_delete_records` | `dws aitable base create --name CLI_Test_Base_1776072665_ca1248 -f json` |
| 36 | `test_search_common_keyword` | `dws aitable template search --query 项目管理 -f json` |
| 37 | `test_search_with_limit` | `dws aitable template search --query 项目 --limit 2 -f json` |
| 38 | `test_search_pagination` | `dws aitable template search --query 项目 --limit 1 -f json` |
| 39 | `test_search_no_result` | `dws aitable template search --query ZZZZZ_不存在的模板_99999 -f json` |
| 40 | `test_base_search_wrong_keyword_flag` | `dws aitable base search --keyword 测试 -f json` |
| 41 | `test_record_query_wrong_query_flag` | `dws aitable record query --query 测试 -f json` |
| 42 | `test_base_get_wrong_base_flag` | `dws aitable base get --base INVALID -f json` |
| 43 | `test_base_get_wrong_id_flag` | `dws aitable base get --id INVALID -f json` |
| 44 | `test_field_create_invalid_fields_json` | `dws aitable field create --base-id INVALID --table-id INVALID --fields {bad_json` |

---

## 问题描述

**用例断言失败（测试代码层）**（1 条，涉及命令：dws aitable base create）

---

## 二、统计汇总

| 指标 | 数值 |
|------|------|
| 总测试数 | 45 |
| 通过（PASSED） | 44 |
| 失败（FAILED） | 1 |
| 通过率 | 97.8% |
| 运行时长 | 55.62s |
