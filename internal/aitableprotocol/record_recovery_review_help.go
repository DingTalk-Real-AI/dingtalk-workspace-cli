// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

// RecordRecoveryReviewHelp describes read-only recovery after an uncertain write.
const RecordRecoveryReviewHelp = `沿原写入回执或 checkpoint 核对原 Base/Table、原记录 ID、clientToken 和恢复文件。保留已知标识，缺失时不要编造；原写入入口与用于查询契约的 Schema 入口分别记录。+record-update 和 +record-upsert 是快捷命令，record update 和 record upsert 是原子命令。原入口未知时，只采用两类入口共有的恢复规则。
按原记录 ID 分别核对创建记录和更新记录。clientToken 只作用于创建分组，不保护更新。+record-write-result 用原 token 核对创建结果，更新分组使用 +record-query 按原 ID 只读核对。已知创建记录 ID 时，即使缺少 token，仍可按 ID 核对当前记录。
缺少原请求的期望 cells 时，只报告当前记录是否存在及可读值，不能仅凭该次读回确认原更新生效。结果仍未知时保持只读核对，禁止重放或重新上传。另行补写前，先核实原请求范围、当前值和需要修正的差异，再按对应写入口的确认要求操作。`

// RecordWriteInputHelp describes the records CLI value, not an RPC envelope.
const RecordWriteInputHelp = `--records 的值是记录数组 [...]；原子 record update/upsert 的 --records-file 文件顶层同样是数组，不给未发布该 flag 的 Shortcut 添加它。更新草稿形状为 [{"recordId":"<原记录ID>","cells":{}}]，实际 cells 按原请求的字段和值填写；upsert 的创建项省略 recordId。不能把 {"records":[...]} 整个对象传给 --records 或写作 records 文件内容；RPC 请求的外层 records 键不属于这个 flag 的值。`

// AtomicRecordAttachmentRecoveryHelp scopes the local verification guarantee.
const AtomicRecordAttachmentRecoveryHelp = `原子 record update/upsert 不执行记录 Shortcut 的本地稳定身份校验；返回状态以本次真实上游回执及该入口已发布契约为准。不能仅因非空附件只传 URL，就预言原子入口必定返回 unknown、ATTACHMENT_WRITE_UNCONFIRMED、非零退出或 retryable=false。URL-only 的这组自动校验结果仅适用于带读回校验的记录 Shortcut；原子入口写后仍需按原 ID 独立只读核对，不凭受理回执宣布附件已确认。` + "\n" + RecordCurrentValueRecoveryHelp

// RecordCurrentValueRecoveryHelp separates business evidence from CLI diagnostics.
const RecordCurrentValueRecoveryHelp = `原写入回执、当前值核验和历史归因分别报告。后续只读回读时，当前值已核验须同时具备字段类型/附件模式契约成立、当前值符合原请求期望、非空附件所需稳定身份对应三项适用证据；这同样适用于后来建立 fileToken/resourceId 对应的非空附件，不只适用于 [] 清空。URL可访问或可下载只补充当前可用性，所需稳定身份对应缺失时，非空附件的当前值仍未确认；[] 清空只依字段类型和空值期望核对，不要求非空身份。该结论不证明首次写入由原请求造成，不把原 unknown/unconfirmed 回执改成 verified；无须重新写入刷新回执。当前为空或非空只描述当前状态；缺少独立的原操作证据时，首次写入是否生效及历史归因保持未确认，不为任一历史经过分配概率。清空的目标值可重复设为 []，值级幂等与并发安全分别判断；这不改变该入口已发布的幂等性契约。未知写入的停写依据是结果未核实及可能覆盖并发新增附件。原范围仍未核实时，下一步是按原 ID 只读核对；读回失败不转为申请新授权后重发，新授权本身不解除本恢复流程的停写义务。
+record-query 提供业务记录值，当前值核验依据实际字段值及相应空值/稳定身份对应证据，不以 attachmentVerification 为必要条件。它未承诺返回该诊断字段，不能要求它必须返回 attachmentVerification=verified；缺少该诊断字段本身不构成核验失败，返回该字段也不能代替上述证据。Shortcut 回执的 attachmentVerification 描述本次自动核验状态；原回执的 verified/unconfirmed 与后续当前值是否已核验分别报告。`
