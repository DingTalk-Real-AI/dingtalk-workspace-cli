// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
	"github.com/spf13/cobra"
)

// PR-D：aitable export/import 静态命令覆盖
//
// MSE 已经注册了 export_data / import_data / prepare_import_upload 三个 tool
// （动态发现命令）。本文件用 preferLegacyLeaf 固定命令 surface，避免动态
// 发现层和 Wukong 的稳定命令口径漂移。
//
// TRANSITIONAL: 等 mse 把 asyncBehavior 标注加入 toolOverrides 后，
// 这套 helper 行为可由动态发现层统一处理，本文件可整体删除。
// 工单：plan/mse-yuyuan-patch.md（待后续补充 asyncBehavior 规范）

// ── aitable export data ─────────────────────────────────────

func newAitableExportDataCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: i18n.T("导出数据"),
		Long: i18n.T(`导出 AI 表格数据的统一入口。
不传 --task-id 时，根据 --scope / --format 创建新的导出任务，并同步等待结果；
若在等待窗口内完成，则直接返回 downloadUrl 和 fileName。
传入 --task-id 时，继续等待该任务，不会重新创建。

scope 可选值：all（整个 Base）、table（指定数据表）、view（指定视图）。
format 可选值：excel、attachment、excel_and_attachment、excel_with_inline_images。`),
		Example: `  dws aitable export data --base-id BASE_ID --scope all --format excel
  dws aitable export data --base-id BASE_ID --scope table --table-id TABLE_ID --format excel
  dws aitable export data --base-id BASE_ID --scope view --table-id TABLE_ID --view-id VIEW_ID --format excel
  dws aitable export data --base-id BASE_ID --task-id TASK_ID
  # 查询 baseId: dws aitable base list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableExportData(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("scope", "", i18n.T("导出范围：all（整个 Base）、table（指定数据表）、view（指定视图）"))
	cmd.Flags().String("format", "", i18n.T("导出格式：excel、attachment、excel_and_attachment、excel_with_inline_images"))
	cmd.Flags().String("task-id", "", i18n.T("已有导出任务 ID，传入后继续等待（忽略 scope/format/table-id/view-id）"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID，scope=table 或 scope=view 时必填"))
	cmd.Flags().String("view-id", "", i18n.T("View ID，scope=view 时必填"))
	cmd.Flags().Int("timeout-ms", 0, i18n.T("单次等待超时（毫秒），默认 30000，最大 30000"))
	return cmd
}

func runAitableExportData(cmd *cobra.Command, runner executor.Runner) error {
	baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
	if err != nil {
		return err
	}
	taskID, _ := cmd.Flags().GetString("task-id")
	scope, _ := cmd.Flags().GetString("scope")
	format, _ := cmd.Flags().GetString("format")
	tableID, _ := cmd.Flags().GetString("table-id")
	viewID, _ := cmd.Flags().GetString("view-id")
	timeoutMS, _ := cmd.Flags().GetInt("timeout-ms")
	params := map[string]any{
		"baseId": baseID,
	}
	if taskID != "" {
		params["taskId"] = taskID
	} else {
		if strings.TrimSpace(scope) == "" {
			return apperrors.NewValidation("--scope is required")
		}
		if strings.TrimSpace(format) == "" {
			return apperrors.NewValidation("--format is required")
		}
		params["scope"] = scope
		params["format"] = format
	}
	if tableID != "" {
		params["tableId"] = tableID
	}
	if viewID != "" {
		params["viewId"] = viewID
	}
	if timeoutMS > 0 {
		params["timeoutMs"] = timeoutMS
	}
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "export_data", params,
		))
	}
	return runAitableTool(cmd, runner, "export_data", params)
}

// ── aitable import upload ───────────────────────────────────

func newAitableImportUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: i18n.T("准备导入文件上传"),
		Long: i18n.T(`为导入任务申请 OSS 直传地址。返回 uploadUrl 和 importId。
客户端应通过 HTTP PUT 将原始文件字节流上传至 uploadUrl。
上传完成后将 importId 传入 import data 即可触发导入。

完整流程:
  1. dws aitable import upload --base-id BASE_ID --file-name data.xlsx --file-size 204800
     → 获取 uploadUrl 和 importId
  2. curl -X PUT "<uploadUrl>" --data-binary @data.xlsx
     → 上传文件到 OSS
  3. dws aitable import data --import-id <importId>
     → 触发导入`),
		Example: `  dws aitable import upload --base-id BASE_ID --file-name data.xlsx --file-size 204800
  # 查询 baseId: dws aitable base list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableImportUpload(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("file-name", "", i18n.T("文件名，须带扩展名，如 data.xlsx (必填)"))
	cmd.Flags().Int64("file-size", 0, i18n.T("文件大小（字节数）(必填)"))
	return cmd
}

func runAitableImportUpload(cmd *cobra.Command, runner executor.Runner) error {
	fileName, err := aitableRequiredFlag(cmd, "file-name")
	if err != nil {
		return err
	}
	baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
	if err != nil {
		return err
	}
	fileSize, _ := cmd.Flags().GetInt64("file-size")
	params := map[string]any{
		"baseId":   baseID,
		"fileName": fileName,
	}
	if fileSize > 0 {
		params["fileSize"] = fileSize
	}

	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "prepare_import_upload", params,
		))
	}
	return runAitableTool(cmd, runner, "prepare_import_upload", params)
}

// ── aitable import data ─────────────────────────────────────

func newAitableImportDataCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: i18n.T("导入数据"),
		Long: i18n.T(`将已通过 import upload 上传完成的文件导入 AI 表格。
支持两种模式:
  1. 新建表导入（默认）：不传 --table-id，每个 Sheet 会新建为独立的数据表
  2. 追加导入：传入 --table-id，数据将作为新行追加到该已有表中

工具内部会等待导入完成，大多数情况下一次调用即可拿到最终结果。
若在 timeout 内未完成，再次传入相同 importId 继续等待，无需重新提交任务。

追加导入时的注意事项：
  - 系统按列名自动匹配字段，源文件列名须与目标表字段名一致
  - 若需自定义映射关系，使用 --field-mapping 指定（key=目标表字段名，value=源文件列名）
  - 多 Sheet 文件默认使用第一个 Sheet，可通过 --src-sheet-name 指定`),
		Example: `  # 新建表导入
  dws aitable import data --import-id IMPORT_ID
  # 追加到已有表
  dws aitable import data --import-id IMPORT_ID --table-id TABLE_ID
  # 指定表头行和源 Sheet
  dws aitable import data --import-id IMPORT_ID --table-id TABLE_ID --header-row 2 --src-sheet-name "Sheet1"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableImportData(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("import-id", "", i18n.T("prepare_import_upload 返回的 importId (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("目标数据表 ID。传入后数据将作为新行追加到该表中；不传则默认新建表导入"))
	cmd.Flags().Int("timeout", 0, i18n.T("最长等待时间（秒），默认且推荐使用最大值 30"))
	cmd.Flags().Int("header-row", 0, i18n.T("表头所在行号（从 1 开始），数据从 headerRow 的下一行开始读取。不传则自动识别表头行"))
	cmd.Flags().String("src-sheet-name", "", i18n.T("源文件中的 Sheet 名称。多 Sheet 文件时指定从哪个 Sheet 导入数据。不传则默认使用第一个 Sheet"))
	cmd.Flags().String("field-mapping", "", i18n.T("字段映射关系 JSON 对象。key 为目标表的字段名，value 为源文件中的列名。不传则按列名自动匹配"))
	return cmd
}

func runAitableImportData(cmd *cobra.Command, runner executor.Runner) error {
	importID, err := aitableRequiredFlag(cmd, "import-id")
	if err != nil {
		return err
	}
	tableID, _ := cmd.Flags().GetString("table-id")
	fieldMapping, _ := cmd.Flags().GetString("field-mapping")
	headerRow, _ := cmd.Flags().GetInt("header-row")
	srcSheetName, _ := cmd.Flags().GetString("src-sheet-name")
	timeoutSec, _ := cmd.Flags().GetInt("timeout")

	importParams := map[string]any{
		"importId": importID,
	}
	if tableID != "" {
		importParams["tableId"] = tableID
	}
	if timeoutSec > 0 {
		importParams["timeout"] = timeoutSec
	}
	if headerRow > 0 {
		importParams["headerRow"] = headerRow
	}
	if strings.TrimSpace(srcSheetName) != "" {
		importParams["srcSheetName"] = strings.TrimSpace(srcSheetName)
	}
	if fieldMapping != "" {
		fieldMappingValue, err := parseAitableStringMap(fieldMapping, "field-mapping")
		if err != nil {
			return err
		}
		importParams["fieldMapping"] = fieldMappingValue
	}

	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "import_data", importParams,
		))
	}
	return runAitableTool(cmd, runner, "import_data", importParams)
}

func parseAitableStringMap(raw, flagName string) (map[string]string, error) {
	var parsed map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--%s must be a JSON object with string values", flagName))
	}
	if parsed == nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--%s must be a JSON object with string values", flagName))
	}
	return parsed, nil
}

// unwrapAitableResp 处理 MCP/runtime 常见响应包装层次。
func unwrapAitableResp(resp map[string]any) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	preserved := map[string]any{}
	for depth := 0; depth < 8; depth++ {
		preserveAitableWrapperFields(preserved, resp)
		if content, ok := resp["content"].(map[string]any); ok && len(content) > 0 {
			resp = content
			continue
		}
		if data, ok := resp["data"].(map[string]any); ok && len(data) > 0 {
			resp = data
			continue
		}
		if result, ok := resp["result"].(map[string]any); ok && len(result) > 0 {
			resp = result
			continue
		}
		if raw, ok := resp["result"].(string); ok && strings.TrimSpace(raw) != "" {
			var parsed map[string]any
			if json.Unmarshal([]byte(raw), &parsed) == nil && len(parsed) > 0 {
				resp = parsed
				continue
			}
		}
		break
	}
	out := copyAitableMap(resp)
	for k, v := range preserved {
		if k == "status" || k == "state" {
			if s, ok := v.(string); ok && normalizeAsyncStatus(s, false) == asynctask.StatusFailed {
				out["status"] = s
				continue
			}
		}
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func preserveAitableWrapperFields(dst, layer map[string]any) {
	for k, v := range layer {
		switch k {
		case "content", "data", "result":
			continue
		}
		if _, ok := v.(map[string]any); ok {
			continue
		}
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
	if s := firstAitableString(layer, "status", "state"); normalizeAsyncStatus(s, false) == asynctask.StatusFailed {
		dst["status"] = s
	}
}

func normalizeAitableDownloadURL(raw string) string {
	url := strings.TrimSpace(raw)
	if url == "" || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return "https://" + url
}

func firstAitableString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := values[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func copyAitableMap(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

// parseAitableExportQueryResult 解析 export_data 查询返回。
func parseAitableExportQueryResult(resp map[string]any) asynctask.QueryResult {
	data := unwrapAitableResp(resp)
	statusRaw := firstAitableString(data, "status", "state")
	url := normalizeAitableDownloadURL(firstAitableString(data, "downloadUrl", "downloadURL", "url"))
	msg := firstAitableString(data, "message", "msg", "errorMessage")
	// 兼容：部分上游用 SUCCEED / FAILURE 等变体
	st := normalizeAsyncStatus(statusRaw, url != "")
	if st == asynctask.StatusSuccess && url == "" {
		st = asynctask.StatusProcessing
	}
	return asynctask.QueryResult{
		Status:      st,
		DownloadURL: url,
		Message:     msg,
		Raw:         data,
	}
}

// normalizeAsyncStatus 把各种 status 变体规范化到 asynctask.Status。
// hasResult=true 时即便 status 缺失也判定 SUCCESS（部分上游用"data 已就位"暗示完成）。
func normalizeAsyncStatus(raw string, hasResult bool) asynctask.Status {
	s := strings.ToUpper(strings.TrimSpace(raw))
	switch s {
	case "SUCCESS", "SUCCEED", "SUCCEEDED", "DONE", "FINISHED", "COMPLETE", "COMPLETED":
		return asynctask.StatusSuccess
	case "FAILED", "FAILURE", "ERROR":
		return asynctask.StatusFailed
	case "PROCESSING", "RUNNING", "PENDING", "QUEUED", "IN_PROGRESS":
		return asynctask.StatusProcessing
	case "":
		if hasResult {
			return asynctask.StatusSuccess
		}
		return asynctask.StatusProcessing
	default:
		// 未知状态：保守处理为 processing 让上层继续等
		return asynctask.StatusProcessing
	}
}
