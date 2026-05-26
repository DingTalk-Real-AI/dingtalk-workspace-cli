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
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
	"github.com/spf13/cobra"
)

// PR-D：aitable export/import data 内置同步轮询 + 自动 IO
//
// MSE 已经注册了 export_data / import_data / prepare_import_upload 三个 tool
// （动态发现命令），但默认行为是"提交即返回 taskId/importId，Agent 自己轮询"。
// 这导致 Agent 经常：
//   - 把 taskId 当 downloadUrl 直接 GET
//   - 自己 PUT 文件忘记清空 Content-Type → SignatureDoesNotMatch
//   - 没有渐进式退避、没有超时兜底
//
// 本节 helper 用 preferLegacyLeaf 覆盖动态生成命令，内置：
//   - 提交 → 渐进式退避轮询（复用 pkg/asynctask）→ 完成
//   - --output 时自动 GET downloadUrl 落盘
//   - --file 时（import upload）自动 PUT 文件到 OSS（清空 Content-Type）
//   - 保持向后兼容：传 --task-id / --import-id 时只续等不重新提交
//
// TRANSITIONAL: 等 mse 把 asyncBehavior 标注加入 toolOverrides 后，
// 这套 helper 行为可由动态发现层统一处理，本文件可整体删除。
// 工单：plan/mse-yuyuan-patch.md（待后续补充 asyncBehavior 规范）

// ── aitable export data ─────────────────────────────────────

func newAitableExportDataCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: i18n.T("导出 AI 表格数据（内置轮询 + 可选自动落盘）"),
		Long: i18n.T(`提交导出任务并自动轮询直到完成，可选自动下载文件到本地。

行为：
  - 不传 --task-id：创建新任务，渐进式退避轮询（1s/2s/5s/10s/15s/30s）
  - 传 --task-id：仅续等已有任务，不重复创建
  - 传 --output：成功后自动 GET downloadUrl 落盘
  - 默认 5 分钟超时；超时返回 taskId 让用户用 --task-id <ID> 续等

导出范围（--scope）：
  all    全表 + 全附件
  table  指定 --table-id 的单表
  view   指定 --table-id + --view-id 的单视图

导出格式（--export-format）：
  excel                          仅 xlsx
  attachment                     仅附件 zip
  excel_and_attachment           xlsx + 附件 zip
  excel_with_inline_images       xlsx 内嵌图片`),
		Example: `  # 导出全表（excel），不落盘
  dws aitable export data --base-id BASE --scope all --export-format excel

  # 导出单表并自动落盘
  dws aitable export data --base-id BASE --scope table --table-id TBL --output ./tbl.xlsx

  # 续等已有任务
  dws aitable export data --base-id BASE --task-id <TASK_ID> --output ./out.xlsx`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableExportData(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("scope", "", i18n.T("导出范围: all / table / view (创建任务时必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (--scope table/view 时必填)"))
	cmd.Flags().String("view-id", "", i18n.T("View ID (--scope view 时必填)"))
	cmd.Flags().String("export-format", "", i18n.T("导出格式: excel / attachment / excel_and_attachment / excel_with_inline_images（默认 excel_and_attachment）"))
	cmd.Flags().String("task-id", "", i18n.T("已有任务 ID（用于续等，传入时不重新创建任务）"))
	cmd.Flags().String("output", "", i18n.T("本地落盘路径（可选，提供则自动下载到本地）"))
	cmd.Flags().Int("timeout-sec", 300, i18n.T("整体轮询超时（秒），默认 300"))
	return cmd
}

func runAitableExportData(cmd *cobra.Command, runner executor.Runner) error {
	baseID, _ := cmd.Flags().GetString("base-id")
	if strings.TrimSpace(baseID) == "" {
		return apperrors.NewValidation("--base-id is required")
	}
	taskID, _ := cmd.Flags().GetString("task-id")
	output, _ := cmd.Flags().GetString("output")
	timeoutSec, _ := cmd.Flags().GetInt("timeout-sec")
	scope, _ := cmd.Flags().GetString("scope")
	tableID, _ := cmd.Flags().GetString("table-id")
	viewID, _ := cmd.Flags().GetString("view-id")
	exportFmt, _ := cmd.Flags().GetString("export-format")

	if commandDryRun(cmd) {
		params := map[string]any{"baseId": baseID, "scope": scope, "__output__": output, "__async__": true}
		if taskID != "" {
			params["taskId"] = taskID
		}
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "export_data", params,
		))
	}

	// 提交回调：传 --task-id 时跳过提交，直接复用
	submitFn := func(ctx context.Context) (string, error) {
		if taskID != "" {
			return taskID, nil
		}
		// 创建任务的参数校验
		if strings.TrimSpace(scope) == "" {
			return "", apperrors.NewValidation("--scope is required when creating a new task")
		}
		params := map[string]any{
			"baseId": baseID,
			"scope":  scope,
		}
		if tableID != "" {
			params["tableId"] = tableID
		}
		if viewID != "" {
			params["viewId"] = viewID
		}
		if exportFmt != "" {
			params["exportFormat"] = exportFmt
		}
		fmt.Fprintf(os.Stderr, i18n.T("[1/3] 提交导出任务 (base=%s scope=%s)...\n"), baseID, scope)
		result, err := runner.Run(ctx, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "export_data", params,
		))
		if err != nil {
			return "", err
		}
		// 兼容返回结构：taskId 可能在 data 下或 response 顶层
		data := unwrapAitableResp(result.Response)
		if id, ok := data["taskId"].(string); ok && id != "" {
			return id, nil
		}
		if id, ok := data["jobId"].(string); ok && id != "" {
			return id, nil
		}
		// 若服务端直接返回 downloadUrl（同步完成），合成一个伪 task 让查询命中
		if url, _ := data["downloadUrl"].(string); url != "" {
			return "__sync_done__:" + url, nil
		}
		return "", apperrors.NewValidation("export_data response missing taskId/downloadUrl")
	}

	queryFn := func(ctx context.Context, jobID string) (asynctask.QueryResult, error) {
		// 同步返回的伪 task：直接转 SUCCESS
		if strings.HasPrefix(jobID, "__sync_done__:") {
			return asynctask.QueryResult{
				Status:      asynctask.StatusSuccess,
				DownloadURL: strings.TrimPrefix(jobID, "__sync_done__:"),
			}, nil
		}
		params := map[string]any{
			"baseId": baseID,
			"taskId": jobID,
		}
		result, err := runner.Run(ctx, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "export_data", params,
		))
		if err != nil {
			return asynctask.QueryResult{}, err
		}
		return parseAitableExportQueryResult(result.Response), nil
	}

	res, err := asynctask.Submit(cmd.Context(), submitFn, queryFn, asynctask.Options{
		Timeout: time.Duration(timeoutSec) * time.Second,
		ProgressFn: func(attempt int, status asynctask.Status, elapsed time.Duration) {
			fmt.Fprintf(os.Stderr, i18n.T("[2/3] 轮询导出（第 %d 次，状态=%s，已耗时 %s）\n"),
				attempt, status, elapsed.Round(time.Second))
		},
	})
	if err != nil {
		return err
	}

	out := map[string]any{
		"taskId": res.JobID,
		"status": string(res.Status),
	}
	if res.DownloadURL != "" {
		out["downloadUrl"] = res.DownloadURL
	}
	if res.Message != "" {
		out["message"] = res.Message
	}

	switch res.Status {
	case asynctask.StatusSuccess:
		if output != "" && res.DownloadURL != "" {
			fmt.Fprintf(os.Stderr, i18n.T("[3/3] 下载到本地：%s\n"), output)
			if err := asynctask.Download(cmd.Context(), res.DownloadURL, output); err != nil {
				return fmt.Errorf(i18n.T("download failed: %w"), err)
			}
			out["output"] = output
		}
	case asynctask.StatusFailed:
		return apperrors.NewValidation(fmt.Sprintf("export failed: %s", res.Message))
	case asynctask.StatusTimeout:
		fmt.Fprintf(os.Stderr,
			i18n.T("⚠️ 任务超时，请用 dws aitable export data --base-id %s --task-id %s 续等\n"),
			baseID, res.JobID)
	}

	return writeCommandPayload(cmd, out)
}

// ── aitable import upload ───────────────────────────────────

func newAitableImportUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: i18n.T("申请导入文件上传凭证（可选自动 PUT 文件到 OSS）"),
		Long: i18n.T(`申请导入文件的 OSS 上传地址；传入 --file 时本命令自动完成上传。

行为：
  - 不传 --file：返回 { uploadUrl, importId, headers }，调用方自己 PUT
  - 传 --file：本命令自动 PUT 本地文件到 OSS，输出仅含 importId

⚠️ 重要：自己 PUT 时必须清空 Content-Type（OSS 签名要求），否则会触发
SignatureDoesNotMatch。本 helper 已正确处理。

后续用 importId 调 dws aitable import data 触发导入任务。`),
		Example: `  # 一步完成：上传本地 xlsx 并拿到 importId
  dws aitable import upload --base-id BASE --file ./data.xlsx

  # 两步式：手动 PUT
  dws aitable import upload --base-id BASE --file-name data.xlsx --file-size 12345
  # 然后 Agent 自行 PUT uploadUrl（必须清空 Content-Type）`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableImportUpload(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("file", "", i18n.T("本地文件路径（可选，提供则一步完成上传）"))
	cmd.Flags().String("file-name", "", i18n.T("文件名（不传 --file 时必填）"))
	cmd.Flags().String("file-size", "", i18n.T("文件大小，字节（不传 --file 时必填）"))
	return cmd
}

func runAitableImportUpload(cmd *cobra.Command, runner executor.Runner) error {
	baseID, _ := cmd.Flags().GetString("base-id")
	if strings.TrimSpace(baseID) == "" {
		return apperrors.NewValidation("--base-id is required")
	}
	filePath, _ := cmd.Flags().GetString("file")
	fileName, _ := cmd.Flags().GetString("file-name")
	fileSizeStr, _ := cmd.Flags().GetString("file-size")

	// 模式 1：传 --file，自动检测元数据 + 上传
	var fileSize int64
	if filePath != "" {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return apperrors.NewValidation(i18n.T("无法解析文件路径: ") + err.Error())
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return apperrors.NewValidation(i18n.T("文件不存在: ") + absPath)
		}
		if info.IsDir() {
			return apperrors.NewValidation(i18n.T("不是文件: ") + absPath)
		}
		filePath = absPath
		fileSize = info.Size()
		if fileName == "" {
			fileName = filepath.Base(absPath)
		}
	} else {
		// 模式 2：不传 --file，要求显式给 file-name + file-size
		if strings.TrimSpace(fileName) == "" {
			return apperrors.NewValidation("--file-name is required when --file is not provided")
		}
		if strings.TrimSpace(fileSizeStr) == "" {
			return apperrors.NewValidation("--file-size is required when --file is not provided")
		}
		if _, err := fmt.Sscanf(fileSizeStr, "%d", &fileSize); err != nil || fileSize <= 0 {
			return apperrors.NewValidation("--file-size must be a positive integer")
		}
	}

	params := map[string]any{
		"baseId":   baseID,
		"fileName": fileName,
		"fileSize": fileSize,
	}

	if commandDryRun(cmd) {
		if filePath != "" {
			params["__file__"] = filePath
		}
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "prepare_import_upload", params,
		))
	}

	fmt.Fprintf(os.Stderr, i18n.T("[1/2] 申请上传凭证 (%s, %d 字节)...\n"), fileName, fileSize)
	result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "aitable", "prepare_import_upload", params,
	))
	if err != nil {
		return err
	}

	data := unwrapAitableResp(result.Response)
	uploadURL, _ := data["uploadUrl"].(string)
	importID, _ := data["importId"].(string)
	if uploadURL == "" || importID == "" {
		return apperrors.NewValidation("prepare_import_upload response missing uploadUrl / importId")
	}

	out := map[string]any{
		"importId":  importID,
		"uploadUrl": uploadURL,
	}
	if hdr, ok := data["headers"].(map[string]any); ok {
		out["headers"] = hdr
	}

	// 没传 --file：返回 uploadUrl 让 Agent 自己 PUT
	if filePath == "" {
		return writeCommandPayload(cmd, out)
	}

	// 传了 --file：自动 PUT（清空 Content-Type 避免 SignatureDoesNotMatch）
	fmt.Fprintln(os.Stderr, i18n.T("[2/2] 上传文件到 OSS..."))
	if err := putFileToOSS(cmd.Context(), uploadURL, filePath, fileSize); err != nil {
		return err
	}
	// 上传成功后仅返回 importId，让调用方进入 import data
	final := map[string]any{
		"importId": importID,
		"uploaded": true,
		"hint":     i18n.T("接下来用 dws aitable import data --import-id <ID> 触发导入"),
	}
	return writeCommandPayload(cmd, final)
}

// ── aitable import data ─────────────────────────────────────

func newAitableImportDataCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: i18n.T("触发导入任务并轮询直到完成"),
		Long: i18n.T(`基于 import-id 触发导入任务，内置同步轮询直到 success/failed/超时。

行为：
  - 提交后渐进式退避轮询（1s/2s/5s/10s/15s/30s）
  - 默认 5 分钟超时；超时返回 importId 让用户用 --import-id <ID> 续等

成功后返回新建/更新数据表的 tableId，可直接用于后续 record query / update。`),
		Example: `  dws aitable import data --import-id <ID>
  dws aitable import data --import-id <ID> --table-id <TBL>  # 追加到已有表`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableImportData(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("import-id", "", i18n.T("导入任务 ID，从 import upload 获取 (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("追加到已有表时传入；新建表场景留空"))
	cmd.Flags().String("field-mapping", "", i18n.T("字段映射 JSON（追加模式可用）"))
	cmd.Flags().Int("timeout-sec", 300, i18n.T("整体轮询超时（秒），默认 300"))
	return cmd
}

func runAitableImportData(cmd *cobra.Command, runner executor.Runner) error {
	importID, _ := cmd.Flags().GetString("import-id")
	if strings.TrimSpace(importID) == "" {
		return apperrors.NewValidation("--import-id is required")
	}
	tableID, _ := cmd.Flags().GetString("table-id")
	fieldMapping, _ := cmd.Flags().GetString("field-mapping")
	timeoutSec, _ := cmd.Flags().GetInt("timeout-sec")

	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "import_data",
			map[string]any{"importId": importID, "tableId": tableID, "__async__": true},
		))
	}

	// 单 importId 任务，submit = 触发查询；后续轮询用同样 tool。
	// 关键：建表参数 tableId/fieldMapping 必须在"已成功提交"前重复传——首次 RPC
	// 失败时 asynctask.Resume 会保留 lastErr 重试，建表参数不能在首次调用就消费掉
	// 否则重试永远丢参数（agent #1 P0-3 修复）。
	submitted := false
	queryFn := func(ctx context.Context, jobID string) (asynctask.QueryResult, error) {
		params := map[string]any{"importId": jobID}
		if !submitted {
			// 未确认提交前每次都带建表参数；服务端幂等接受重复提交
			if tableID != "" {
				params["tableId"] = tableID
			}
			if fieldMapping != "" {
				params["fieldMapping"] = fieldMapping
			}
		}
		result, err := runner.Run(ctx, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "import_data", params,
		))
		if err != nil {
			return asynctask.QueryResult{}, err
		}
		// 服务端确认收到（无 err）后切到纯轮询模式
		submitted = true
		return parseAitableImportQueryResult(result.Response), nil
	}

	fmt.Fprintf(os.Stderr, i18n.T("[1/1] 触发导入任务 (import-id=%s)...\n"), importID)
	res, err := asynctask.Resume(cmd.Context(), importID, queryFn, asynctask.Options{
		Timeout: time.Duration(timeoutSec) * time.Second,
		ProgressFn: func(attempt int, status asynctask.Status, elapsed time.Duration) {
			fmt.Fprintf(os.Stderr, i18n.T("  轮询导入（第 %d 次，状态=%s，已耗时 %s）\n"),
				attempt, status, elapsed.Round(time.Second))
		},
	})
	if err != nil {
		return err
	}

	out := map[string]any{
		"importId": res.JobID,
		"status":   string(res.Status),
	}
	if res.Raw != nil {
		if tid, ok := res.Raw["tableId"].(string); ok && tid != "" {
			out["tableId"] = tid
		}
		if bid, ok := res.Raw["baseId"].(string); ok && bid != "" {
			out["baseId"] = bid
		}
		if cnt, ok := res.Raw["recordCount"]; ok {
			out["recordCount"] = cnt
		}
	}
	if res.Message != "" {
		out["message"] = res.Message
	}
	if res.Status == asynctask.StatusFailed {
		return apperrors.NewValidation(fmt.Sprintf("import failed: %s", res.Message))
	}
	if res.Status == asynctask.StatusTimeout {
		fmt.Fprintf(os.Stderr,
			i18n.T("⚠️ 任务超时，请用 dws aitable import data --import-id %s 续等\n"), res.JobID)
	}
	return writeCommandPayload(cmd, out)
}

// ── 导入上传工具 ────────────────────────────────────────────────

// putFileToOSS PUT 本地文件到 aitable import uploadUrl。
//
// Protocol contract: import upload URLs are signed with an empty Content-Type.
// Do not add a client-inferred Content-Type header here; doing so changes the
// string OSS verifies and can trigger SignatureDoesNotMatch.
//
// This helper is intentionally scoped to aitable import upload. Other DingTalk
// OSS-backed flows, such as doc media insert or aitable attachment upload, may
// have different header contracts from their prepare APIs and must use helpers
// that preserve those product-specific rules.
func putFileToOSS(ctx context.Context, uploadURL, filePath string, fileSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf(i18n.T("无法打开文件: %w"), err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, f)
	if err != nil {
		return fmt.Errorf(i18n.T("构建上传请求失败: %w"), err)
	}
	req.ContentLength = fileSize
	// 不设 Content-Type：Go 的 http.Client 不会自动加，确保 OSS 签名匹配

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(i18n.T("上传失败: %w"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf(i18n.T("OSS 上传失败 HTTP %d: %s（提示：Content-Type 必须清空以匹配 OSS 签名）"),
			resp.StatusCode, string(body))
	}
	return nil
}

// unwrapAitableResp 处理两种响应包装层次。
func unwrapAitableResp(resp map[string]any) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	if content, ok := resp["content"].(map[string]any); ok && len(content) > 0 {
		resp = content
	}
	if data, ok := resp["data"].(map[string]any); ok && len(data) > 0 {
		return data
	}
	return resp
}

// parseAitableExportQueryResult 解析 export_data 查询返回。
func parseAitableExportQueryResult(resp map[string]any) asynctask.QueryResult {
	data := unwrapAitableResp(resp)
	statusRaw, _ := data["status"].(string)
	url, _ := data["downloadUrl"].(string)
	msg, _ := data["message"].(string)
	// 兼容：部分上游用 SUCCEED / FAILURE 等变体
	st := normalizeAsyncStatus(statusRaw, url != "")
	return asynctask.QueryResult{
		Status:      st,
		DownloadURL: url,
		Message:     msg,
		Raw:         data,
	}
}

// parseAitableImportQueryResult 解析 import_data 查询返回。
func parseAitableImportQueryResult(resp map[string]any) asynctask.QueryResult {
	data := unwrapAitableResp(resp)
	statusRaw, _ := data["status"].(string)
	msg, _ := data["message"].(string)
	// 导入完成判定：拿到 tableId / recordCount 即视为成功
	hasResult := false
	if _, ok := data["tableId"].(string); ok {
		hasResult = true
	}
	if _, ok := data["recordCount"]; ok {
		hasResult = true
	}
	return asynctask.QueryResult{
		Status:  normalizeAsyncStatus(statusRaw, hasResult),
		Message: msg,
		Raw:     data,
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
