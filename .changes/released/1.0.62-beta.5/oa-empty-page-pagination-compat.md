---
category: Fixed
---

- **OA 空页分页兼容** — 待审批、已处理和已发起审批列表兼容成功响应中 `values:[]` 省略 `hasMore` 的终页编码，避免空列表误报 `missing_pagination`；保留显式分页值及业务状态、数组结构和其他接口的严格校验。
