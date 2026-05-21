# AI 搜索（aisearch）命令测试报告

> 测试时间：2026-04-13 18:34:13 | 测试框架：pytest 9.0.2 | Python 3.13.3

---

## 一、成功用例（PASSED）

_本产品本次无 PASSED 用例_

---

## 二、跳过用例（SKIPPED）

| 序号 | 测试用例 | 执行命令 | 跳过原因 |
|:---:|:---|:---|:---|
| 1 | `test_search_basic` |  | 未提供（pytest 未输出 skip reason） |
| 2 | `test_search_user_query` |  | 未提供（pytest 未输出 skip reason） |
| 3 | `test_search_no_match` | `dws aisearch search --question ZZZNONEXIST99999的文档 --keywords ZZZNONEXIST99999 -f json` | 未提供（pytest 未输出 skip reason） |

---

## 三、统计汇总

| 指标 | 数值 |
|------|------|
| 总测试数 | 3 |
| 通过（PASSED） | 0 |
| 失败（FAILED） | 0 |
| 跳过（SKIPPED） | 3 |
| 通过率 | 0.0% |
| 运行时长 | 17.22s |
