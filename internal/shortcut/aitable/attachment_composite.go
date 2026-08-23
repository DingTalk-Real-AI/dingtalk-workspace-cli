// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const maxAttachmentFileSize = int64(100 * 1024 * 1024)

func rejectAttachmentUploadRedirect(*http.Request, []*http.Request) error {
	// Upload credentials are issued for one exact endpoint. Do not let a 3xx
	// response redirect local file bytes or the credential to another host or
	// to a URL outside validateAttachmentUploadURL's transport policy.
	return http.ErrUseLastResponse
}

var attachmentHTTPDo = (&http.Client{CheckRedirect: rejectAttachmentUploadRedirect}).Do

var AttachmentPut = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+attachment-put",
	Product:     serverMain,
	Description: "准备凭证、实际 PUT 本地文件、写入 attachment 单元格并读回验证",
	Intent:      "当你要把本地文件真正上传并落到指定记录附件字段时使用；不是只返回 uploadUrl，支持 replace 或在现有项可安全重写时 append。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: aitableCompositeContract(
		"+attachment-put",
		"准备凭证、实际 PUT 本地文件、写入 attachment 单元格并读回验证",
		"当你要把本地文件真正上传并落到指定记录附件字段时使用；不是只返回 uploadUrl，支持 replace 或在现有项可安全重写时 append。",
		"只申请上传凭证用 attachment upload；不要用 drive fileId；现有附件读回缺 fileToken 时无法安全 append，只能 replace 或停止",
		`dws aitable +attachment-put --base-id B --table-id T --record-id R --field-id F --file ./report.pdf --mode replace`,
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-id", Type: shortcut.FlagString, Desc: "Record ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "attachment Field ID", Required: true},
		{Name: "file", Type: shortcut.FlagString, Desc: "本地文件路径", Required: true},
		{Name: "mode", Type: shortcut.FlagString, Default: "append", Desc: "append 保留现有附件；replace 整体替换", Enum: []string{"append", "replace"}},
		{Name: "mime-type", Type: shortcut.FlagString, Desc: "覆盖自动推断的 MIME type（可选）"},
	},
	Tips: []string{`dws aitable +attachment-put --base-id B --table-id T --record-id R --field-id F --file ./report.pdf --mode replace`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeAttachmentPut(rt)
	},
}

var AttachmentRemove = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+attachment-remove",
	Product:     serverMain,
	Description: "从 attachment 字段清空全部或按文件名移除，写前确保剩余项具有可重写 fileToken，并读回验证",
	Intent:      "当你要清空附件字段，或按精确文件名移除且安全保留其他附件时使用；读回若不给剩余项 fileToken 会在写前停止并说明服务边界。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: aitableCompositeContract(
		"+attachment-remove",
		"从 attachment 字段清空全部或按文件名移除，写前确保剩余项具有可重写 fileToken，并读回验证",
		"当你要清空附件字段，或按精确文件名移除且安全保留其他附件时使用；读回若不给剩余项 fileToken 会在写前停止并说明服务边界。",
		"读回仅有下载 URL 而没有剩余附件 fileToken 时，DWS 无法在覆盖式写接口上安全对齐逐项删除；此时只能 --clear-all 或停止",
		`dws aitable +attachment-remove --base-id B --table-id T --record-id R --field-id F --clear-all`,
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-id", Type: shortcut.FlagString, Desc: "Record ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "attachment Field ID", Required: true},
		{Name: "remove-name", Type: shortcut.FlagString, Desc: "移除精确文件名的所有匹配项；与 --clear-all 二选一"},
		{Name: "clear-all", Type: shortcut.FlagBool, Desc: "清空该字段全部附件；与 --remove-name 二选一"},
	},
	Tips: []string{`dws aitable +attachment-remove --base-id B --table-id T --record-id R --field-id F --clear-all`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeAttachmentRemove(rt)
	},
}

func executeAttachmentPut(rt *shortcut.RuntimeContext) error {
	file, info, mimeType, err := openAttachmentFile(rt.Str("file"), rt.Str("mime-type"))
	if err != nil {
		return err
	}
	defer file.Close()
	baseID, tableID, recordIDValue, fieldID := rt.Str("base-id"), rt.Str("table-id"), rt.Str("record-id"), rt.Str("field-id")
	record, existing, err := readAttachmentCell(rt, baseID, tableID, recordIDValue, fieldID)
	if err != nil {
		return err
	}
	_ = record
	desired := make([]any, 0, len(existing)+1)
	if rt.Str("mode") == "append" {
		for index, item := range existing {
			token := attachmentToken(item)
			if token == "" {
				return apperrors.NewValidation(fmt.Sprintf("现有附件[%d]读回缺少 fileToken，覆盖式写接口无法安全 append；请用 --mode replace 或保留原数据后人工处理", index),
					apperrors.WithReason("attachment_tokens_unavailable"), apperrors.WithExecutionStarted(false))
			}
			desired = append(desired, map[string]any{"fileToken": token})
		}
	}
	result := newCompositeResult("attachment_put")
	result.Resolved = map[string]any{"baseId": baseID, "tableId": tableID, "recordId": recordIDValue, "fieldId": fieldID, "fileName": info.Name(), "size": info.Size(), "mode": rt.Str("mode")}
	result.Plan = []compositeStep{
		{Index: 1, Name: "prepare attachment upload", Tool: "prepare_attachment_upload", Status: "planned"},
		{Index: 2, Name: "PUT file bytes", Tool: "HTTP PUT", Status: "planned"},
		{Index: 3, Name: "write and verify attachment cell", Tool: "update_records", Status: "planned"},
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	prepareData, err := rt.CallMCPWriteDataStrict(serverMain, "prepare_attachment_upload", map[string]any{"baseId": baseID, "fileName": info.Name(), "size": info.Size(), "mimeType": mimeType})
	if err != nil {
		result.Status = "unknown"
		return compositeError(result, err, false)
	}
	uploadURL := findStringByKeys(prepareData, "uploadUrl")
	fileToken := findStringByKeys(prepareData, "fileToken")
	if uploadURL == "" || fileToken == "" {
		result.Status = "unknown"
		return compositeError(result, fmt.Errorf("prepare_attachment_upload response is missing uploadUrl or fileToken"), false)
	}
	if err := validateAttachmentUploadURL(uploadURL); err != nil {
		result.Status = "unknown"
		return compositeError(result, err, false)
	}
	// validateAttachmentUploadURL has already rejected every URL shape for
	// which NewRequestWithContext can fail; PUT is a fixed valid method.
	request, _ := http.NewRequestWithContext(rt.Command().Context(), http.MethodPut, uploadURL, file)
	request.Header.Set("Content-Type", mimeType)
	request.ContentLength = info.Size()
	response, err := attachmentHTTPDo(request)
	if err != nil {
		result.Status = "partial_success"
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "prepare_attachment_upload", "fileToken": fileToken})
		return compositeError(result, fmt.Errorf("attachment HTTP PUT failed: %w", err), false)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 || closeErr != nil {
		result.Status = "partial_success"
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "prepare_attachment_upload", "fileToken": fileToken, "httpStatus": response.StatusCode})
		return compositeError(result, fmt.Errorf("attachment HTTP PUT returned status %d (close error: %v)", response.StatusCode, closeErr), false)
	}
	desired = append(desired, map[string]any{"fileToken": fileToken})
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "HTTP PUT", "fileToken": fileToken, "fileName": info.Name(), "size": info.Size()})
	_, writeErr := rt.CallMCPWriteDataStrict(serverMain, "update_records", map[string]any{
		"baseId": baseID, "tableId": tableID,
		"records": []any{map[string]any{"recordId": recordIDValue, "cells": map[string]any{fieldID: desired}}},
	})
	_, readBack, verifyErr := readAttachmentCell(rt, baseID, tableID, recordIDValue, fieldID)
	if verifyErr == nil {
		verifyErr = verifyAttachmentPut(readBack, len(desired), fileToken, info.Name(), info.Size())
	}
	if verifyErr != nil {
		result.Status = "partial_success"
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "update_records", "recordId": recordIDValue, "fieldId": fieldID})
		if writeErr != nil {
			result.Warnings = append(result.Warnings, "record write response error: "+writeErr.Error())
		}
		return compositeError(result, verifyErr, false)
	}
	result.CompletedCount = 1
	result.Verification = map[string]any{"status": "verified", "attachmentCount": len(readBack), "fileName": info.Name(), "size": info.Size()}
	result.Result = map[string]any{"fileToken": fileToken, "fileName": info.Name(), "size": info.Size(), "mimeType": mimeType, "attachmentCount": len(readBack)}
	if writeErr != nil {
		result.Status = "recovered"
		result.Warnings = append(result.Warnings, "record write response was an error, but the attachment cell was proven by read-back")
	}
	return rt.Output(result)
}

func executeAttachmentRemove(rt *shortcut.RuntimeContext) error {
	removeName := strings.TrimSpace(rt.Str("remove-name"))
	clearAll := rt.Bool("clear-all")
	if (removeName == "") == !clearAll {
		return apperrors.NewValidation("必须且只能提供 --remove-name 或 --clear-all")
	}
	baseID, tableID, recordIDValue, fieldID := rt.Str("base-id"), rt.Str("table-id"), rt.Str("record-id"), rt.Str("field-id")
	_, existing, err := readAttachmentCell(rt, baseID, tableID, recordIDValue, fieldID)
	if err != nil {
		return err
	}
	desired := make([]any, 0, len(existing))
	removed := 0
	if clearAll {
		removed = len(existing)
	} else {
		for index, item := range existing {
			if attachmentName(item) == removeName {
				removed++
				continue
			}
			token := attachmentToken(item)
			if token == "" {
				return apperrors.NewValidation(fmt.Sprintf("剩余附件[%d]读回缺少 fileToken，覆盖式写接口无法安全保留后删除；服务端当前不支持按附件增量删除", index),
					apperrors.WithReason("attachment_tokens_unavailable"), apperrors.WithExecutionStarted(false))
			}
			desired = append(desired, map[string]any{"fileToken": token})
		}
	}
	result := newCompositeResult("attachment_remove")
	result.RequestedCount = removed
	result.Resolved = map[string]any{"baseId": baseID, "tableId": tableID, "recordId": recordIDValue, "fieldId": fieldID, "removeName": removeName, "clearAll": clearAll}
	if removed == 0 {
		result.Status = "unchanged"
		result.Executed = false
		result.Verification = map[string]any{"status": "verified", "attachmentCount": len(existing), "removedCount": 0}
		return rt.Output(result)
	}
	result.Plan = []compositeStep{{Index: 1, Name: "replace attachment cell without selected items", Tool: "update_records", Status: "planned", Count: removed}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	_, writeErr := rt.CallMCPWriteDataStrict(serverMain, "update_records", map[string]any{
		"baseId": baseID, "tableId": tableID,
		"records": []any{map[string]any{"recordId": recordIDValue, "cells": map[string]any{fieldID: desired}}},
	})
	_, readBack, verifyErr := readAttachmentCell(rt, baseID, tableID, recordIDValue, fieldID)
	if verifyErr == nil && len(readBack) != len(desired) {
		verifyErr = fmt.Errorf("attachment read-back count is %d, want %d", len(readBack), len(desired))
	}
	if verifyErr == nil && removeName != "" {
		for _, item := range readBack {
			if attachmentName(item) == removeName {
				verifyErr = fmt.Errorf("attachment %q is still present after removal", removeName)
				break
			}
		}
	}
	if verifyErr != nil {
		result.Status = "unknown"
		if writeErr != nil {
			result.Warnings = append(result.Warnings, "record write response error: "+writeErr.Error())
		}
		return compositeError(result, verifyErr, true)
	}
	result.CompletedCount = removed
	result.Verification = map[string]any{"status": "verified", "removedCount": removed, "attachmentCount": len(readBack)}
	result.Result = map[string]any{"removedCount": removed, "attachmentCount": len(readBack)}
	if writeErr != nil {
		result.Status = "recovered"
		result.Warnings = append(result.Warnings, "record write response was an error, but removal was proven by read-back")
	}
	return rt.Output(result)
}

func openAttachmentFile(path, overrideMIME string) (*os.File, os.FileInfo, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, "", apperrors.NewValidation("无法打开 --file: " + err.Error())
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxAttachmentFileSize {
		file.Close()
		return nil, nil, "", apperrors.NewValidation(fmt.Sprintf("--file 必须是 1..%d 字节的普通文件", maxAttachmentFileSize))
	}
	mimeType := strings.TrimSpace(overrideMIME)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(info.Name())))
	}
	if mimeType == "" {
		header := make([]byte, 512)
		read, _ := file.Read(header)
		mimeType = http.DetectContentType(header[:read])
		_, _ = file.Seek(0, io.SeekStart)
	}
	return file, info, mimeType, nil
}

func validateAttachmentUploadURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("prepare_attachment_upload returned an invalid uploadUrl")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("plaintext attachment uploadUrl is allowed only for loopback test servers")
		}
	}
	return nil
}

func readAttachmentCell(rt *shortcut.RuntimeContext, baseID, tableID, recordIDValue, fieldID string) (map[string]any, []map[string]any, error) {
	records, err := queryRecordsByIDs(rt, baseID, tableID, []string{recordIDValue})
	if err != nil {
		return nil, nil, err
	}
	if len(records) != 1 || recordID(records[0]) != recordIDValue {
		return nil, nil, fmt.Errorf("attachment read-back returned %d exact records for %s, want 1", len(records), recordIDValue)
	}
	cells, ok := records[0]["cells"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("record %s is missing cells", recordIDValue)
	}
	raw, exists := cells[fieldID]
	if !exists || raw == nil {
		return records[0], []map[string]any{}, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, nil, fmt.Errorf("attachment field %s is not an array", fieldID)
	}
	out := make([]map[string]any, 0, len(list))
	for index, item := range list {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("attachment field %s item %d is not an object", fieldID, index)
		}
		out = append(out, object)
	}
	return records[0], out, nil
}

func attachmentToken(item map[string]any) string {
	return stringValue(item, "fileToken", "file_token")
}

func attachmentName(item map[string]any) string {
	return stringValue(item, "fileName", "filename", "name")
}

func verifyAttachmentPut(actual []map[string]any, expectedCount int, token, name string, size int64) error {
	if len(actual) != expectedCount {
		return fmt.Errorf("attachment read-back count is %d, want %d", len(actual), expectedCount)
	}
	for _, item := range actual {
		if attachmentToken(item) == token {
			return nil
		}
		itemSize, sizeOK := numericInt64(item["size"])
		if attachmentName(item) == name && sizeOK && itemSize == size {
			return nil
		}
	}
	return fmt.Errorf("attachment read-back does not identify uploaded token or file %s/%d", name, size)
}

func numericInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	}
	return 0, false
}
