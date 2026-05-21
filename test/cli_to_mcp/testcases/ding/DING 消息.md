# DING 消息（ding）命令测试报告

> 测试时间：2026-04-13 18:34:13 | 测试框架：pytest 9.0.2 | Python 3.13.3

---

## 一、成功用例（PASSED）

| 序号 | 测试用例 | 关联命令 |
|:---:|:---|:---|
| 1 | `test_send_invalid_robot` | `dws ding message send --robot-code dingdiwdtolfjiih8lfw --users 035665695811868955452 --content CLI自动化DING 1776072837 -f json` |
| 2 | `test_recall_invalid_id` | `dws ding message send --robot-code dingdiwdtolfjiih8lfw --users 035665695811868955452 --content CLI自动化DING 1776072837 -f json` |
| 3 | `test_recall_missing_robot` | `dws ding message recall --id SOME_ID -f json` |

---

## 问题描述

**用例断言失败（测试代码层）**（3 条，涉及命令：dws contact user get-self、dws ding message send、dws ding message send）

---

## 二、统计汇总

| 指标 | 数值 |
|------|------|
| 总测试数 | 5 |
| 通过（PASSED） | 2 |
| 失败（FAILED） | 3 |
| CLI报错（非预期命令错误） | 4 |
| 通过率 | 22.2% |
| 运行时长 | 2.03s |
