# AI 应用（aiapp）命令测试报告

> 测试时间：2026-04-13 18:34:13 | 测试框架：pytest 9.0.2 | Python 3.13.3

---

## 一、成功用例（PASSED）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_create_basic` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 2 | `test_create_with_skills` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 3 | `test_create_chinese_prompt` | `dws aiapp create --prompt 请创建一个能够解答数学问题的AI助手，支持四则运算和方程求解 -f json` |
| 4 | `test_query_created_app` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 5 | `test_query_invalid_id` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 6 | `test_query_returns_status` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 7 | `test_modify_prompt` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 8 | `test_modify_with_skills` | `dws aiapp query --task-id INVALID_99999 -f json` |
| 9 | `test_modify_invalid_thread` | `dws aiapp modify --prompt X --thread-id INVALID_99999 -f json` |

---

## 二、统计汇总

| 指标 | 数值 |
|------|------|
| 总测试数 | 7 |
| 通过（PASSED） | 7 |
| 失败（FAILED） | 0 |
| CLI报错（非预期命令错误） | 4 |
| 通过率 | 63.6% |
| 运行时长 | N/A |
