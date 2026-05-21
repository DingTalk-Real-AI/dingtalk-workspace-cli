# AI听记（minutes）命令测试报告

> 测试时间：2026-04-13 18:34:13 | 测试框架：pytest 9.0.2 | Python 3.13.3

---

## 一、成功用例（PASSED）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_list_mine` | `dws minutes list mine -f json` |
| 2 | `test_list_mine_has_valid_structure` | `dws minutes list mine -f json` |
| 3 | `test_list_mine_idempotent` | `dws minutes list mine -f json` |
| 4 | `test_list_shared` | `dws minutes list shared -f json` |
| 5 | `test_list_shared_structure` | `dws minutes list shared -f json` |
| 6 | `test_list_shared_idempotent` | `dws minutes list shared -f json` |
| 7 | `test_get_info_invalid` | `dws minutes get info --id INVALID -f json` |
| 8 | `test_get_summary_invalid` | `dws minutes get summary --id INVALID -f json` |
| 9 | `test_keywords_invalid` | `dws minutes get keywords --id INVALID -f json` |
| 10 | `test_transcription_invalid` | `dws minutes get transcription --id INVALID -f json` |
| 11 | `test_todos_invalid` | `dws minutes get todos --id INVALID -f json` |
| 12 | `test_batch_invalid` | `dws minutes get info --id INVALID -f json` |
| 13 | `test_update_title_invalid` | `dws minutes update title --id INVALID --title X -f json` |
| 14 | `test_list_wrong_max_flag` | `dws minutes list shared --max 10 -f json` |
| 15 | `test_get_summary_wrong_task_uuid_flag` | `dws minutes get summary --task-uuid 7632756964323339 -f json` |
| 16 | `test_get_info_wrong_url_flag` | `dws minutes get info --url https://example.com -f json` |

---

## 二、跳过用例（SKIPPED）

| 序号 | 测试用例 | 执行命令 | 跳过原因 |
|:---:|:---|:---|:---|
| 1 | `test_get_info` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 2 | `test_get_info_contains_title` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 3 | `test_get_summary` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 4 | `test_get_summary_structure` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 5 | `test_get_keywords` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 6 | `test_keywords_structure` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 7 | `test_get_transcription` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 8 | `test_transcription_structure` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 9 | `test_get_todos` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 10 | `test_todos_structure` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 11 | `test_batch_single` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 12 | `test_batch_multiple` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 13 | `test_update_title` | `dws minutes get info --id INVALID -f json` | No minutes available |
| 14 | `test_update_chinese_title` | `dws minutes get info --id INVALID -f json` | No minutes available |

---

## 三、统计汇总

| 指标 | 数值 |
|------|------|
| 总测试数 | 22 |
| 通过（PASSED） | 8 |
| 失败（FAILED） | 0 |
| 跳过（SKIPPED） | 14 |
| CLI报错（非预期命令错误） | 16 |
| 通过率 | 33.3% |
| 运行时长 | 5.10s |

---

## 五、新增 record 命令用例（待执行）

> 以下为已补充到测试代码中的新增用例，待下一轮执行后回填通过/失败结果。

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_record_start` | `dws minutes record start -f json` |
| 2 | `test_record_start_with_session_id` | `dws minutes record start --session-id test-session-id -f json` |
| 3 | `test_record_start_idempotent` | `dws minutes record start -f json`（重复调用） |
| 4 | `test_record_pause` | `dws minutes record pause --id <taskUuid> -f json` |
| 5 | `test_record_pause_invalid_id` | `dws minutes record pause --id INVALID -f json` |
| 6 | `test_record_pause_missing_id` | `dws minutes record pause -f json` |
| 7 | `test_record_resume` | `dws minutes record resume --id <taskUuid> -f json` |
| 8 | `test_record_resume_invalid_id` | `dws minutes record resume --id INVALID -f json` |
| 9 | `test_record_resume_missing_id` | `dws minutes record resume -f json` |
| 10 | `test_record_stop` | `dws minutes record stop --id <taskUuid> -f json` |
| 11 | `test_record_stop_invalid_id` | `dws minutes record stop --id INVALID -f json` |
| 12 | `test_record_stop_missing_id` | `dws minutes record stop -f json` |
