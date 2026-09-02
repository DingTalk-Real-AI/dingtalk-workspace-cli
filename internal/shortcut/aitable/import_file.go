// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var importHTTPDo = (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}).Do

var importNewRequestWithContext = http.NewRequestWithContext

var ImportFile = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+import-file",
	Product:     serverMain,
	Description: "申请上传、PUT 本地 CSV/XLS/XLSX，并用同一 importId 触发导入；也可只续等已有任务",
	Intent:      "当你要从本地文件完成 AI 表格导入闭环时使用；DWS 不暴露签名 URL，超时保留同一 importId 供只续等。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: aitableCompositeContractWithResult(
		"+import-file",
		"申请上传、PUT 本地 CSV/XLS/XLSX，并用同一 importId 触发导入；也可只续等已有任务",
		"当你要从本地文件完成 AI 表格导入闭环时使用；DWS 不暴露签名 URL，超时保留同一 importId 供只续等。",
		"JSON 记录写入用 record create/upsert；只申请上传地址用 +import-upload；续等时不能改变目标表或映射参数",
		`dws aitable +import-file --base-id B --file ./data.xlsx --table-id T`,
		aitableImportFileResultSpec(),
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID；首次导入模式使用，值不能为空", RequiredWhen: "未提供 --resume-import-id 时"},
		{Name: "file", Type: shortcut.FlagString, Desc: "本地非空普通 CSV/XLS/XLSX 文件；首次导入模式使用，值不能为空", RequiredWhen: "未提供 --resume-import-id 时"},
		{Name: "resume-import-id", Type: shortcut.FlagString, Desc: "只续等已有 importId；与首次导入参数互斥"},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "追加导入的目标 Table ID（可选）"},
		{Name: "timeout", Type: shortcut.FlagInt, Desc: "服务端等待超时（可选，必须大于 0）"},
		{Name: "header-row", Type: shortcut.FlagInt, Desc: "XLS/XLSX 表头行号（可选，必须大于 0；CSV 不支持）"},
		{Name: "src-sheet-name", Type: shortcut.FlagString, Desc: "XLS/XLSX 源 Sheet 名（可选；CSV 不支持）"},
		{Name: "field-mapping", Type: shortcut.FlagString, Desc: "字段映射 JSON 对象，目标字段名和源列名不能为空（可选）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"resume-import-id", "base-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"resume-import-id", "file"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"resume-import-id", "table-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"resume-import-id", "header-row"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"resume-import-id", "src-sheet-name"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"resume-import-id", "field-mapping"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"base-id", "file"}, Description: "首次导入模式下值不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"timeout", "header-row"}, Description: "显式值必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"field-mapping"}, Description: "目标字段名和源列名不能为空"},
	},
	Tips: []string{
		`dws aitable +import-file --base-id B --file ./data.xlsx --table-id T`,
		`dws aitable +import-file --resume-import-id IMPORT_ID --timeout 120`,
	},
	Validate: validateImportFileFlags,
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeImportFile(rt)
	},
}

func validateImportFileFlags(rt *shortcut.RuntimeContext) error {
	resume := strings.TrimSpace(rt.Str("resume-import-id"))
	if resume == "" && (strings.TrimSpace(rt.Str("base-id")) == "" || strings.TrimSpace(rt.Str("file")) == "") {
		return apperrors.NewValidation("首次导入必须同时提供 --base-id 和 --file；续等使用 --resume-import-id")
	}
	if rt.Changed("timeout") && rt.Int("timeout") <= 0 {
		return apperrors.NewValidation("--timeout 必须大于 0")
	}
	if rt.Changed("header-row") && rt.Int("header-row") <= 0 {
		return apperrors.NewValidation("--header-row 必须大于 0")
	}
	return nil
}

func executeImportFile(rt *shortcut.RuntimeContext) error {
	if importID := strings.TrimSpace(rt.Str("resume-import-id")); importID != "" {
		return resumeImportFile(rt, importID)
	}
	file, info, err := openImportFile(rt.Str("file"))
	if err != nil {
		return err
	}
	defer file.Close()
	if strings.EqualFold(filepath.Ext(info.Name()), ".csv") {
		if rt.Changed("header-row") {
			return apperrors.NewValidation("CSV 表头固定为第一行，不支持 --header-row")
		}
		if rt.Changed("src-sheet-name") {
			return apperrors.NewValidation("CSV 没有 Sheet，不支持 --src-sheet-name")
		}
	}
	// Validate every option that will be sent to import_data before applying
	// prepare/upload side effects. The real importId is added after preparation.
	params, err := importDataParams(rt, "")
	if err != nil {
		return err
	}
	delete(params, "importId")

	baseID := strings.TrimSpace(rt.Str("base-id"))
	result := newCompositeResult("import_file")
	result.Resolved = map[string]any{"mode": "initial", "baseId": baseID, "fileName": info.Name(), "fileSize": info.Size()}
	result.Plan = []compositeStep{
		{Index: 1, Name: "prepare import upload", Tool: "prepare_import_upload", Status: "planned"},
		{Index: 2, Name: "PUT file bytes", Tool: "HTTP PUT", Status: "planned"},
		{Index: 3, Name: "trigger and wait import", Tool: "import_data", Status: "planned"},
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	prepareData, err := rt.CallMCPWriteDataStrict(serverMain, "prepare_import_upload", map[string]any{
		"baseId": baseID, "fileName": info.Name(), "fileSize": info.Size(),
	})
	if err != nil {
		result.Status = "unknown"
		return compositeError(result, err, false)
	}
	uploadURL := findStringByKeys(prepareData, "uploadUrl")
	importID := findStringByKeys(prepareData, "importId")
	if uploadURL == "" || importID == "" {
		result.Status = "unknown"
		return compositeError(result, fmt.Errorf("prepare_import_upload response is missing uploadUrl or importId"), false)
	}
	result.Resolved["importId"] = importID
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "prepare_import_upload", "importId": importID})
	if err := validateImportUploadURL(uploadURL); err != nil {
		result.Status = "partial_success"
		return compositeError(result, err, false)
	}

	request, err := importNewRequestWithContext(rt.Command().Context(), http.MethodPut, uploadURL, file)
	if err != nil {
		result.Status = "partial_success"
		return compositeError(result, fmt.Errorf("cannot construct import HTTP PUT request"), false)
	}
	request.Header.Set("Content-Type", "")
	request.ContentLength = info.Size()
	response, err := importHTTPDo(request)
	if err != nil {
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "inspect upload state; do not start a new import blindly"}
		// The transport error can contain the signed upload URL. Keep it out of
		// user-visible output because the URL is an ephemeral credential.
		return compositeError(result, fmt.Errorf("import HTTP PUT failed before a response was received"), false)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"importId": importID, "httpStatus": response.StatusCode, "nextStep": "inspect upload state; do not start a new import blindly"}
		return compositeError(result, fmt.Errorf("import HTTP PUT returned status %d", response.StatusCode), false)
	}
	if closeErr != nil {
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "inspect upload state; do not start a new import blindly"}
		return compositeError(result, fmt.Errorf("import HTTP PUT response body could not be closed cleanly"), false)
	}
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "HTTP PUT", "importId": importID, "fileName": info.Name(), "size": info.Size()})

	params["importId"] = importID
	importData, importErr := rt.CallMCPWriteDataStrict(serverMain, "import_data", params)
	if importErr != nil {
		result.Status = "unknown"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "check target data, then continue waiting with the same importId"}
		result.NextCommand = aitableRecoveryCommand("dws", "aitable", "+import-file", "--resume-import-id", importID)
		return compositeError(result, importErr, false)
	}
	return finishImportFile(rt, result, "initial", importID, importData, 3)
}

func resumeImportFile(rt *shortcut.RuntimeContext, importID string) error {
	result := newCompositeResult("import_file")
	result.Resolved = map[string]any{"mode": "resume", "importId": importID}
	result.Plan = []compositeStep{{Index: 1, Name: "continue waiting import", Tool: "import_data", Status: "planned"}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	params := map[string]any{"importId": importID}
	if rt.Changed("timeout") {
		params["timeout"] = rt.Int("timeout")
	}
	data, err := rt.CallMCPWriteDataStrict(serverMain, "import_data", params)
	if err != nil {
		result.Status = "unknown"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "check target data before continuing to wait again"}
		result.NextCommand = aitableRecoveryCommand("dws", "aitable", "+import-file", "--resume-import-id", importID)
		return compositeError(result, err, false)
	}
	return finishImportFile(rt, result, "resume", importID, data, 1)
}

func finishImportFile(rt *shortcut.RuntimeContext, result compositeResult, mode, importID string, data map[string]any, completed int) error {
	result.Result = map[string]any{"mode": mode, "importId": importID, "response": sanitizeImportOutput(data, "")}
	switch importFileOutcome(data) {
	case "success":
		result.CompletedCount = completed
		result.Verification = map[string]any{"status": "service_confirmed", "importId": importID}
		return rt.Output(result)
	case "pending":
		result.Status = "pending"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "check target data or continue waiting with the same importId"}
		result.NextCommand = aitableRecoveryCommand("dws", "aitable", "+import-file", "--resume-import-id", importID)
		if output.UsesUnifiedResult(rt.Command()) {
			return output.StoreResult(rt.Command().Context(), output.Pending(result, &output.OperationInfo{
				ID:          importID,
				State:       "pending",
				NextCommand: result.NextCommand,
			}))
		}
		return rt.Output(result)
	case "partial_failure", "failure":
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "inspect failed items and target data before any retry"}
		return compositeError(result, fmt.Errorf("import_data returned a non-success terminal outcome"), false)
	default:
		result.Status = "unknown"
		result.Checkpoint = map[string]any{"importId": importID, "nextStep": "inspect target data; the import response outcome is not trustworthy"}
		return compositeError(result, fmt.Errorf("import_data response has no consistent success/pending/failure outcome"), false)
	}
}

func importFileOutcome(payload map[string]any) string {
	objects := importResponseObjects(payload)
	unified := make([]string, 0)
	for index, object := range objects {
		_, hasOutcome := object["outcome"]
		_, hasOK := object["ok"]
		// A nested ok may be a business field. A root ok or any outcome marks
		// a unified envelope and therefore requires the complete ok/outcome pair.
		if !hasOutcome && !(index == 0 && hasOK) {
			continue
		}
		outcome, outcomeOK := object["outcome"].(string)
		ok, okFound := object["ok"].(bool)
		if !outcomeOK || !okFound {
			return "invalid"
		}
		switch outcome {
		case "success", "pending":
			if !ok {
				return "invalid"
			}
		case "partial_failure", "failure":
			if ok {
				return "invalid"
			}
		default:
			return "invalid"
		}
		if len(unified) > 0 && unified[0] != outcome {
			return "invalid"
		}
		unified = append(unified, outcome)
	}
	if len(unified) > 0 {
		return unified[0]
	}
	for _, wanted := range []string{"failure", "partial_failure", "pending", "success"} {
		for _, object := range objects {
			status, _ := object["status"].(string)
			normalized := map[string]string{"error": "failure", "failed": "failure", "failure": "failure", "partial_failure": "partial_failure", "pending": "pending", "success": "success"}[status]
			if normalized == wanted {
				return wanted
			}
		}
	}
	hasSuccess, hasFailure := false, false
	for _, object := range objects {
		if success, found := object["success"].(bool); found {
			hasSuccess = hasSuccess || success
			hasFailure = hasFailure || !success
		}
	}
	if hasSuccess && hasFailure {
		return "invalid"
	}
	if hasFailure {
		return "failure"
	}
	if hasSuccess {
		return "success"
	}
	return "unknown"
}

func importResponseObjects(payload map[string]any) []map[string]any {
	objects := make([]map[string]any, 0, 3)
	queue := []map[string]any{payload}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		objects = append(objects, current)
		for _, key := range []string{"data", "result"} {
			if nested, ok := current[key].(map[string]any); ok {
				queue = append(queue, nested)
			}
		}
	}
	return objects
}

func importDataParams(rt *shortcut.RuntimeContext, importID string) (map[string]any, error) {
	params := map[string]any{"importId": importID}
	for flag, property := range map[string]string{"table-id": "tableId", "src-sheet-name": "srcSheetName"} {
		if rt.Changed(flag) {
			params[property] = rt.Str(flag)
		}
	}
	for flag, property := range map[string]string{"timeout": "timeout", "header-row": "headerRow"} {
		if rt.Changed(flag) {
			params[property] = rt.Int(flag)
		}
	}
	if rt.Changed("field-mapping") {
		mapping, err := parseJSONObject("field-mapping", rt.Str("field-mapping"))
		if err != nil {
			return nil, err
		}
		for target, source := range mapping {
			text, ok := source.(string)
			if strings.TrimSpace(target) == "" || !ok || strings.TrimSpace(text) == "" {
				return nil, apperrors.NewValidation("--field-mapping 的目标字段名和源列名必须是非空字符串")
			}
		}
		params["fieldMapping"] = mapping
	}
	return params, nil
}

func openImportFile(path string) (*os.File, os.FileInfo, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return nil, nil, apperrors.NewValidation("无法打开 --file: " + err.Error())
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		file.Close()
		return nil, nil, apperrors.NewValidation("--file 必须是非空普通文件")
	}
	switch strings.ToLower(filepath.Ext(info.Name())) {
	case ".csv", ".xls", ".xlsx":
		return file, info, nil
	default:
		file.Close()
		return nil, nil, apperrors.NewValidation("--file 仅支持 CSV、XLS 或 XLSX")
	}
}

func validateImportUploadURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("prepare_import_upload returned an invalid uploadUrl")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("plaintext import uploadUrl is allowed only for loopback test servers")
		}
	}
	return nil
}

func sanitizeImportOutput(value any, key string) any {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	if strings.Contains(normalized, "uploadurl") || strings.Contains(normalized, "authorization") ||
		strings.HasSuffix(normalized, "token") || strings.HasSuffix(normalized, "secret") ||
		strings.HasSuffix(normalized, "signature") || strings.HasSuffix(normalized, "password") ||
		strings.HasSuffix(normalized, "credential") || strings.HasSuffix(normalized, "apikey") {
		return "<redacted>"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, child := range typed {
			out[childKey] = sanitizeImportOutput(child, childKey)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = sanitizeImportOutput(child, key)
		}
		return out
	case string:
		lower := strings.ToLower(typed)
		if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") ||
			strings.Contains(lower, "authorization:") || strings.Contains(lower, "x-oss-signature") ||
			strings.Contains(lower, "x-amz-signature") || strings.Contains(lower, "signature=") {
			return "<redacted>"
		}
	}
	return value
}
