// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// recordAttachmentRecoveryHelp documents this product's read-back boundary.
const recordAttachmentRecoveryHelp = `cells key 支持 fieldId 或当前表内精确匹配的唯一字段名；推荐 fieldId 不等于只允许 fieldId。按键入口的 --key-field-id 仍只接受字段 ID。
写入受理后，含附件形状的批次在任一字段读回失败时都可能返回 ATTACHMENT_WRITE_UNCONFIRMED，不代表一定是附件字段的问题。verification.readback 区分 pending/mismatch/unavailable；mismatchedFieldId 存在时指向实际不一致字段，非附件字段也可能不一致；此诊断不证明原写入未发生。
附件 unconfirmed 回执中，未确认项不计为失败；failedCount 为零时可省略。已知该回执属于零值省略时，省略 failedCount 与显式 failedCount=0 表达同一明确失败计数。计数结论为“明确失败计数为 0”；整体结果为“仍有未确认项，保持原 unknown/unconfirmed 并只读核对”。合并报告为“明确失败计数为 0，仍有未确认项”；计数为零成立，全部成功或全部失败均未据此得到证明。
错误诊断不回显附件值或 fileToken，仅限定错误详情和核验诊断；业务 +record-query 的返回以实际回执和已发布契约为准。不能由诊断脱敏推断业务读回必定没有 fileToken，也不能把诊断当作完整业务响应。
用户或回执给出的 fileToken/resourceId 原样保留，不能凭英文、短名或合成代称的字面形状认定其无效或只是占位符。缺少输入 token 与读回 resourceId 的对应证据，只能说明对应未确认，不证明该 resourceId 无效。
非空附件输入若只给 url（含普通 URL、文档节点默认链接及 isLink=false 文件模式），没有可与读回对应的 fileToken/resourceId，即使 URL 相同或文件可下载，现有记录 Shortcut 的稳定身份校验仍不能确认 verified。已受理并进入读回校验的这类写入预期为 unknown、ATTACHMENT_WRITE_UNCONFIRMED、非零退出、retryable=false；不表示确定写入失败，禁止重放或重新上传来刷新 verified。
上述自动校验结果只属于带读回校验的记录 Shortcut，不外推为原子 record update/upsert 的固定返回值。按回执 nextCommand 使用原 Base/Table 和记录 ID/唯一键只读核对；checkpoint.attachmentReadbackCommand 核对已知记录（含已创建 ID）的附件字段，须与创建 token 对账分别执行，不能只查创建结果就宣布附件确认。创建 token 仅核对创建组，更新另按原 ID 回读。URL 可访问只能补充当前可用性证据，缺少稳定身份对应时仍保持 unconfirmed；原回执与当前值/历史归因分别保留。` + "\n" + aitableprotocol.RecordCurrentValueRecoveryHelp

// These request shapes require attachment-safe recovery even if metadata or
// record reads fail. They do not establish a field type or prove a write.
func recordAttachmentFields(records []map[string]any) []string {
	fields := map[string]bool{}
	for _, record := range records {
		cells, _ := record["cells"].(map[string]any)
		for field, value := range cells {
			items, _ := value.([]any)
			for _, raw := range items {
				item, _ := raw.(map[string]any)
				for _, key := range []string{"fileToken", "resourceId", "url"} {
					if text, ok := item[key].(string); ok && text != "" {
						fields[field] = true
					}
				}
			}
		}
	}
	ids := make([]string, 0, len(fields))
	for field := range fields {
		ids = append(ids, field)
	}
	sort.Strings(ids)
	return ids
}

// Empty lists carry no attachment identity. Use field metadata for their
// recovery semantics; unreadable/ambiguous metadata cannot rule out a clear.
// This runs only after failed verification, never after a definite rejection.
func recordAttachmentRecoveryFields(rt *shortcut.RuntimeContext, baseID, tableID string, records []map[string]any) []string {
	ids := recordAttachmentFields(records)
	empty := map[string]any{}
	for _, record := range records {
		cells, _ := record["cells"].(map[string]any)
		for field, value := range cells {
			if isEmptyJSONList(value) {
				empty[field] = value
			}
		}
	}
	if len(empty) == 0 {
		return ids
	}
	resolver := newRecordFieldTypeResolver(rt, baseID, tableID)
	fieldTypes, err := resolver.resolve()
	canonical := empty
	if err == nil {
		canonical, err = recordVerificationCells(empty, nil, resolver.fields)
	}
	if err != nil {
		ids = append(ids, sortedMapKeys(empty)...)
	} else {
		for fieldID := range canonical {
			if fieldTypes[fieldID] == "" || strings.EqualFold(fieldTypes[fieldID], "attachment") {
				ids = append(ids, fieldID)
			}
		}
	}
	sort.Strings(ids)
	return uniqueStrings(ids)
}

func unconfirmedRecordAttachmentError(batch []map[string]any, writeData map[string]any, fieldIDs []string, readbackErr error) error {
	details := map[string]any{
		"attachmentVerification": "unconfirmed",
		"recordIds":              uniqueStrings(append(recordIDs(batch), createdRecordIDs(writeData)...)),
		"fieldIds":               fieldIDs,
	}
	addRecordReadbackDiagnostics(details, readbackErr)
	return apperrors.NewAPI("Attachment write could not be confirmed by read-back; retain original targets and do not replay the write",
		apperrors.WithReason("ATTACHMENT_WRITE_UNCONFIRMED"), apperrors.WithRetryable(false),
		apperrors.WithExecutionStarted(true), apperrors.WithDetails(details))
}

// Metadata alone (filename, size, MIME type) cannot associate an uploaded token
// with a resolved resource. Require a stable identity on both sides and match
// the complete multiset, allowing response-only metadata and reordering.
func recordAttachmentValueEqual(got, want any) bool {
	if got == nil && isEmptyJSONList(want) {
		return true
	}
	actual, actualOK := got.([]any)
	expected, expectedOK := want.([]any)
	if !actualOK || !expectedOK || len(actual) != len(expected) {
		return false
	}
	used := make([]bool, len(actual))
	for _, raw := range expected {
		item, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		matched := false
		for index, value := range actual {
			candidate, ok := value.(map[string]any)
			if used[index] || !ok {
				continue
			}
			identity, conflict := false, false
			for _, key := range []string{"fileToken", "resourceId"} {
				requested, _ := item[key].(string)
				returned, _ := candidate[key].(string)
				if strings.TrimSpace(requested) != "" {
					identity = identity || requested == returned
					conflict = conflict || requested != returned
				}
			}
			if identity && !conflict {
				used[index], matched = true, true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
