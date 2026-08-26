package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// computeFileMD5 is the function used to calculate file MD5. Tests can override via direct assignment.
var computeFileMD5 = fileMD5Hex

func decodeOARequest(raw string) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.UseNumber()
	var request map[string]any
	if err := dec.Decode(&request); err != nil || request == nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("JSON 请求不能为 null")
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("JSON 请求包含多余内容")
	}
	return request, nil
}

func oaFormValues(raw string) ([]map[string]string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	result := make([]map[string]string, 0, len(values))
	for name, value := range values {
		result = append(result, map[string]string{"name": name, "value": value})
	}
	return result, nil
}

func parseOAAttachmentFileInfos(raw string) ([]map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var items []map[string]any
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("--file-infos JSON 解析失败: %w", err)
	}
	if err := rejectTrailingOAAttachmentJSON(decoder); err != nil {
		return nil, err
	}
	if len(items) < 1 || len(items) > 10 {
		return nil, fmt.Errorf("--file-infos 必须包含 1 至 10 个文件")
	}

	infos := make([]map[string]any, 0, len(items))
	for index, item := range items {
		for name := range item {
			if name != "spaceId" && name != "fileId" {
				return nil, fmt.Errorf("--file-infos 第 %d 项包含未知字段 %q", index+1, name)
			}
		}

		spaceValue, ok := item["spaceId"]
		if !ok {
			return nil, fmt.Errorf("--file-infos 第 %d 项缺少 spaceId", index+1)
		}
		spaceID, ok := spaceValue.(json.Number)
		if !ok {
			return nil, fmt.Errorf("--file-infos 第 %d 项 spaceId 必须是数字", index+1)
		}

		fileValue, ok := item["fileId"]
		if !ok {
			return nil, fmt.Errorf("--file-infos 第 %d 项缺少 fileId", index+1)
		}
		fileID, ok := fileValue.(string)
		if !ok {
			return nil, fmt.Errorf("--file-infos 第 %d 项 fileId 必须是字符串", index+1)
		}
		fileID = strings.TrimSpace(fileID)
		if fileID == "" {
			return nil, fmt.Errorf("--file-infos 第 %d 项 fileId 不能为空", index+1)
		}

		infos = append(infos, map[string]any{"spaceId": spaceID, "fileId": fileID})
	}
	return infos, nil
}

func rejectTrailingOAAttachmentJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("--file-infos JSON 包含多余内容")
		}
		return fmt.Errorf("--file-infos JSON 解析失败: %w", err)
	}
	return nil
}

func validateOAAttachmentFileInfos(cmd *cobra.Command, _ []string) error {
	raw, _ := cmd.Flags().GetString("file-infos")
	_, err := parseOAAttachmentFileInfos(raw)
	return err
}

func validateOAPreviewFileIDs(cmd *cobra.Command, _ []string) error {
	fileIDs, _ := cmd.Flags().GetStringSlice("file-ids")
	if len(fileIDs) > 20 {
		return fmt.Errorf("--file-ids 最多包含 20 个附件 ID")
	}
	for index, fileID := range fileIDs {
		if strings.TrimSpace(fileID) == "" {
			return fmt.Errorf("--file-ids 第 %d 项不能为空", index+1)
		}
	}
	return nil
}

func callOAAttachmentResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	return callOAAttachmentResultCtx(cmd.Context(), tool, args)
}

// callOAAttachmentResultCtx 复用统一结果投影逻辑，但允许调用方传入自定义 context
// （例如 upload 命令的 10 分钟超时）：调用 oa/<tool>，提取 result 并包装为 output.Success。
func callOAAttachmentResultCtx(ctx context.Context, tool string, args map[string]any) (output.CommandResult, error) {
	data, err := CallMCPToolDataOnServer(ctx, "oa", tool, args)
	if err != nil {
		return nil, err
	}
	response, ok := data.(map[string]any)
	if !ok {
		return nil, apperrors.NewInternal(fmt.Sprintf("oa/%s 返回值不是 JSON 对象", tool))
	}
	result, ok := response["result"]
	if !ok {
		return nil, apperrors.NewInternal(fmt.Sprintf("oa/%s 返回值缺少 result", tool))
	}
	return output.Success(result), nil
}

// validateOAAttachmentCommitResult 校验 commit_attachment_upload_info 的原始 result
// 包含构造 DDAttachment 所需的全部必需字段，并将 spaceId/fileSize 归一化为声明的 integer
// 类型（int64），确保输出始终符合 ResultSpec schema。当字段缺失或类型错误时返回 Validation
// 错误，避免 Agent 拿到空 fileId 后组装无效的审批表单。
func validateOAAttachmentCommitResult(result any) (map[string]any, error) {
	resultMap, ok := result.(map[string]any)
	if !ok || resultMap == nil {
		return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值 result 不是有效的 JSON 对象")
	}

	// spaceId — 接受 string 或 number，归一化为 int64
	switch v := resultMap["spaceId"].(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值缺少必需字段 spaceId")
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值字段 spaceId 不是有效整数")
		}
		resultMap["spaceId"] = parsed
	case float64:
		// numeric spaceId — keep as-is (already matches JSON number)
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值字段 spaceId 不是有效整数")
		}
		resultMap["spaceId"] = parsed
	default:
		return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值缺少必需字段 spaceId")
	}

	// fileName — must be non-empty string
	if s, ok := resultMap["fileName"].(string); !ok || strings.TrimSpace(s) == "" {
		return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值缺少必需字段 fileName")
	}

	// fileSize — must be > 0 (number)，归一化 json.Number → int64
	switch v := resultMap["fileSize"].(type) {
	case float64:
		if v <= 0 {
			return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值字段 fileSize 必须大于 0")
		}
	case json.Number:
		parsed, err := v.Int64()
		if err != nil || parsed <= 0 {
			return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值字段 fileSize 必须大于 0")
		}
		resultMap["fileSize"] = parsed
	default:
		return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值缺少必需字段 fileSize")
	}

	// fileId — must be non-empty string
	if s, ok := resultMap["fileId"].(string); !ok || strings.TrimSpace(s) == "" {
		return nil, apperrors.NewValidation("oa/commit_attachment_upload_info 返回值缺少必需字段 fileId")
	}

	return resultMap, nil
}

// parseOAAttachmentUploadInfo 解析 oa/init_attachment_upload_info 的返回，提取首个上传地址、
// 签名请求头与 uploadKey。OA 的返回结构为 result.resourceUrls([]string)、result.headers(object)、
// result.uploadKey(string)，与钉盘 doc 的 resourceUrl（单数）不同，因此单独实现。
func parseOAAttachmentUploadInfo(text string) (resourceURL string, headers map[string]string, uploadKey string, err error) {
	var data map[string]any
	if err = json.Unmarshal([]byte(text), &data); err != nil {
		return "", nil, "", apperrors.NewInternal(fmt.Sprintf("解析 init_attachment_upload_info 返回失败: %v", err))
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return "", nil, "", apperrors.NewInternal("oa/init_attachment_upload_info 返回值缺少 result")
	}

	uploadKey, _ = result["uploadKey"].(string)
	if strings.TrimSpace(uploadKey) == "" {
		return "", nil, "", apperrors.NewValidation("oa/init_attachment_upload_info 返回值缺少 uploadKey")
	}

	rawURLs, ok := result["resourceUrls"].([]any)
	if !ok || len(rawURLs) == 0 {
		return "", nil, "", apperrors.NewValidation("oa/init_attachment_upload_info 返回值缺少 resourceUrls")
	}
	resourceURL, _ = rawURLs[0].(string)
	if strings.TrimSpace(resourceURL) == "" {
		return "", nil, "", apperrors.NewValidation("oa/init_attachment_upload_info 首个 resourceUrls 为空")
	}

	rawHeaders, ok := result["headers"].(map[string]any)
	if !ok {
		return "", nil, "", apperrors.NewValidation("oa/init_attachment_upload_info 返回值缺少 headers")
	}
	headers = make(map[string]string)
	for key, value := range rawHeaders {
		if str, ok := value.(string); ok {
			headers[key] = str
		}
	}
	if strings.TrimSpace(headers["Authorization"]) == "" || strings.TrimSpace(headers["x-oss-date"]) == "" {
		return "", nil, "", apperrors.NewValidation("oa/init_attachment_upload_info headers 缺少必需的签名字段 Authorization 或 x-oss-date")
	}
	return resourceURL, headers, uploadKey, nil
}

// runOAAttachmentUpload 端到端上传审批附件：init 获取 OSS 上传凭证 → HTTP PUT 上传文件字节
// → commit 提交入库，一条命令完成三步。PUT 复用包级 httpPutFile（它会删除 Content-Type 并
// 写入签名请求头，避免钉钉 OSS SignatureDoesNotMatch），不要在此重复实现。
func runOAAttachmentUpload(cmd *cobra.Command, _ []string) error {
	filePath := strings.TrimSpace(mustGetFlag(cmd, "file"))
	if filePath == "" {
		return apperrors.NewValidation("--file 不能为空")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return apperrors.NewValidation(fmt.Sprintf("无法读取文件 %s: %v", filePath, err))
	}
	if info.IsDir() {
		return apperrors.NewValidation(fmt.Sprintf("%s 是目录，不是文件", filePath))
	}
	fileSize := info.Size()

	fileName := strings.TrimSpace(mustGetFlag(cmd, "file-name"))
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	md5Hex := strings.TrimSpace(mustGetFlag(cmd, "md5"))
	if md5Hex == "" {
		md5Hex, err = computeFileMD5(filePath)
		if err != nil {
			return apperrors.NewInternal(fmt.Sprintf("计算文件 MD5 失败: %v", err))
		}
	}

	// --dry-run：本地只读工作（校验、Stat、大小、文件名、MD5）已完成，
	// 在任何远程调用（init/PUT/commit）之前 early return，输出 plan 预览。
	// 本命令是 RolloutUnifiedActive，必须经 output.StoreResult 存入统一结果，
	// 不能直接 PrintJSON（否则框架报 "returned without a CommandResult"）。
	// 不伪造 uploadKey/resourceURL/fileId/spaceId——它们只能由远程调用返回。
	if deps.Caller.DryRun() {
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{
			"dry_run":      true,
			"executed":     false,
			"preview_kind": "plan",
			"operation":    "attachment_upload",
			"source":       "oa",
			"file":         filePath,
			"file_name":    fileName,
			"file_size":    fileSize,
			"md5":          md5Hex,
			"steps": []map[string]any{
				{
					"tool":   "oa/init_attachment_upload_info",
					"args":   map[string]any{"fileName": fileName, "fileSize": fileSize, "md5": md5Hex},
					"status": "planned",
				},
				{
					"tool":   "HTTP PUT",
					"args":   map[string]any{"file": filePath, "fileSize": fileSize},
					"status": "planned",
				},
				{
					"tool":     "oa/commit_attachment_upload_info",
					"args":     map[string]any{"fileName": fileName, "fileSize": fileSize},
					"requires": []string{"uploadKey from oa/init_attachment_upload_info"},
					"status":   "planned",
				},
			},
		}, output.WithDryRun()))
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
	defer cancel()

	// Step 1: 初始化上传，拿到 OSS 上传地址、签名头与 uploadKey。
	initText, err := callMCPToolReturnTextOnServer(ctx, "oa", "init_attachment_upload_info", map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"md5":      md5Hex,
	})
	if err != nil {
		return err
	}
	resourceURL, headers, uploadKey, err := parseOAAttachmentUploadInfo(initText)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT 文件字节到 OSS（复用 httpPutFile，勿重复实现）。
	if err := httpPutFile(ctx, resourceURL, headers, filePath, fileSize); err != nil {
		return err
	}

	// Step 3: 提交上传信息完成入库，校验必需字段后以统一输出渲染 commit 结果。
	commitData, err := CallMCPToolDataOnServer(ctx, "oa", "commit_attachment_upload_info", map[string]any{
		"fileName":  fileName,
		"uploadKey": uploadKey,
		"fileSize":  float64(fileSize),
	})
	if err != nil {
		return err
	}
	commitResp, ok := commitData.(map[string]any)
	if !ok {
		return apperrors.NewInternal("oa/commit_attachment_upload_info 返回值不是 JSON 对象")
	}
	commitResult, _ := commitResp["result"]
	normalized, err := validateOAAttachmentCommitResult(commitResult)
	if err != nil {
		return err
	}
	return output.StoreResult(cmd.Context(), output.Success(normalized))
}

func newOAAttachmentCommand() *cobra.Command {
	attachmentCmd := newGroupCommand(&cobra.Command{
		Use:   "attachment",
		Short: "审批附件授权、上传、下载与链接管理",
		RunE:  groupRunE,
	})

	downloadURLCmd := NewLeafCommand(LeafSpec{
		Use:           "download-url",
		Short:         "获取审批附件下载链接",
		Example:       "  dws oa approval attachment download-url --instance-id <processInstanceId> --file-id <fileId>",
		Server:        "oa",
		Tool:          "get_attachment_download_url",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callOAAttachmentResult,
		Flags: []LeafFlag{
			{Name: "instance-id", Usage: "审批实例 ID (必填)", Bind: "processInstanceId", Trim: true, Required: true, MarkRequired: true},
			{Name: "file-id", Usage: "审批附件文件 ID (必填)", Bind: "fileId", Trim: true, Required: true, MarkRequired: true},
			{Name: "with-comment-attachment", Usage: "是否包含评论中的附件", Kind: LeafBool, Bind: "withCommentAttachment"},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_attachment_download_url",
				CanonicalPath:  "oa.get_attachment_download_url",
				CLIPath:        "oa approval attachment download-url",
				PrimaryCLIPath: "oa approval attachment download-url",
			},
			Description: "获取审批附件下载授权并生成临时下载链接",
			Result: &contract.ResultSpec{
				Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema:     json.RawMessage(`{"type":"object","description":"审批附件临时下载信息","properties":{"spaceId":{"type":"integer","description":"审批附件所在钉盘空间 ID"},"agentId":{"type":"integer","description":"审批应用 Agent ID"},"downloadUri":{"type":"string","description":"带临时授权签名的附件下载链接"},"class":{"type":"string","description":"服务端响应类型标识"},"fileId":{"type":"string","description":"审批附件文件 ID"}},"required":["spaceId","agentId","downloadUri","fileId"],"additionalProperties":true}`),
				SensitivePaths: []string{"downloadUri"},
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_attachment_download_url"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取审批实例中指定附件的临时下载链接",
				UseWhen:      []string{"已从审批详情获得 processInstanceId 和 fileId，需要获取附件下载链接时"},
				AvoidWhen: []string{
					"只需查看审批表单和附件元数据时使用 dws oa approval detail",
					"需要将附件真正保存到本地时不要误认为本命令会下载文件；它只返回链接",
				},
				Examples: []string{"dws oa approval attachment download-url --instance-id <processInstanceId> --file-id <fileId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId", InterfaceType: "string"},
				{Name: "file-id", Property: "fileId", InterfaceType: "string"},
				{Name: "with-comment-attachment", Property: "withCommentAttachment", InterfaceType: "boolean"},
			},
		},
	})

	authorizeDownloadCmd := NewLeafCommand(LeafSpec{
		Use:           "authorize-download",
		Short:         "授权当前用户下载审批钉盘文件",
		Long:          "批量授权当前用户下载指定的审批钉盘文件。",
		Example:       `  dws oa approval attachment authorize-download --file-infos '[{"spaceId":27827223951,"fileId":"232271651278"}]'`,
		Server:        "oa",
		Tool:          "auth_download_file",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callOAAttachmentResult,
		Flags: []LeafFlag{
			{
				Name: "file-infos", Usage: "审批钉盘文件信息 JSON 数组 (必填)",
				Bind: "fileInfos", Trim: true, Required: true, MarkRequired: true,
				Transform: func(raw string) (any, error) {
					return parseOAAttachmentFileInfos(raw)
				},
			},
		},
		Constraints: []LeafConstraint{{
			Kind: corecmd.Custom, Flags: []string{"file-infos"},
			Description: "文件信息列表必须包含 1 至 10 项",
		}},
		Validate: validateOAAttachmentFileInfos,
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "auth_download_file",
				CanonicalPath:  "oa.auth_download_file",
				CLIPath:        "oa approval attachment authorize-download",
				PrimaryCLIPath: "oa approval attachment authorize-download",
			},
			Description: "批量授权当前用户下载指定的审批钉盘文件",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"boolean","description":"是否成功为当前用户授予审批钉盘文件下载权限"}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "auth_download_file"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "使用审批钉盘 spaceId/fileId 批量授权当前用户下载文件",
				UseWhen:      []string{"已有一个或多个审批钉盘文件的 spaceId 和 fileId，需要为当前用户取得下载权限时"},
				AvoidWhen: []string{
					"需要生成单个审批附件下载链接时使用 attachment download-url",
					"需要授权在审批单内预览附件时使用 attachment authorize-preview",
				},
				Examples: []string{`dws oa approval attachment authorize-download --file-infos '[{"spaceId":27827223951,"fileId":"232271651278"}]'`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file-infos", Property: "fileInfos", InterfaceType: "array"},
			},
		},
	})

	authorizePreviewCmd := NewLeafCommand(LeafSpec{
		Use:           "authorize-preview",
		Short:         "授权当前用户预览审批附件",
		Long:          "批量授权当前用户预览审批单中的附件。",
		Example:       "  dws oa approval attachment authorize-preview --instance-id <processInstanceId> --file-ids <fileId1>,<fileId2>",
		Server:        "oa",
		Tool:          "auth_preview_attachment",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callOAAttachmentResult,
		Flags: []LeafFlag{
			{Name: "instance-id", Usage: "审批实例 ID (必填)", Bind: "processInstanceId", Trim: true, Required: true, MarkRequired: true},
			{Name: "file-ids", Usage: "附件 ID 列表，多个用逗号分隔 (必填)", Kind: LeafStringSlice, Bind: "fileIdList", Required: true, MarkRequired: true},
			{Name: "with-comment-attachment", Usage: "是否包含评论中的附件", Kind: LeafBool, Bind: "withCommentAttachment"},
		},
		Constraints: []LeafConstraint{{
			Kind: corecmd.Custom, Flags: []string{"file-ids"},
			Description: "附件 ID 列表最多包含 20 项且每项不能为空",
		}},
		Validate: validateOAPreviewFileIDs,
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "auth_preview_attachment",
				CanonicalPath:  "oa.auth_preview_attachment",
				CLIPath:        "oa approval attachment authorize-preview",
				PrimaryCLIPath: "oa approval attachment authorize-preview",
			},
			Description: "批量授权当前用户预览审批单中的附件",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"审批附件预览授权信息","properties":{"spaceId":{"type":"integer","description":"审批附件所在钉盘空间 ID"},"agentId":{"type":"integer","description":"审批应用 Agent ID"},"class":{"type":"string","description":"服务端响应类型标识"}},"required":["spaceId","agentId"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "auth_preview_attachment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按审批实例和附件 ID 列表授权当前用户预览附件",
				UseWhen:      []string{"已有 processInstanceId 和附件 fileId 列表，需要在审批场景中批量取得预览权限时"},
				AvoidWhen: []string{
					"需要下载权限而不是预览权限时使用 attachment authorize-download",
					"需要直接获得单个附件临时下载链接时使用 attachment download-url",
				},
				Examples: []string{"dws oa approval attachment authorize-preview --instance-id <processInstanceId> --file-ids <fileId1>,<fileId2>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId", InterfaceType: "string"},
				{Name: "file-ids", Property: "fileIdList", InterfaceType: "array"},
				{Name: "with-comment-attachment", Property: "withCommentAttachment", InterfaceType: "boolean"},
			},
		},
	})

	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "上传本地文件为审批附件（初始化+PUT+提交，一步完成）",
		Long: `上传本地文件为审批附件（三步自动完成）。

流程:
  1. 初始化上传信息，获取 OSS 上传地址与凭证 (oa/init_attachment_upload_info)
  2. HTTP PUT 上传文件二进制到 OSS
  3. 提交上传信息完成入库 (oa/commit_attachment_upload_info)

--file-name 不传时默认用文件名；--md5 不传时自动计算。`,
		Example: `  dws oa approval attachment upload --file ./合同.pdf
  dws oa approval attachment upload --file ./report.xlsx --file-name Q1报表.xlsx
  dws oa approval attachment upload --file ./data.bin --md5 d41d8cd98f00b204e9800998ecf8427e`,
		RunE: runOAAttachmentUpload,
	}
	DeclareLeafMetadata(uploadCmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "attachment_upload",
				CanonicalPath:  "oa.attachment_upload",
				CLIPath:        "oa approval attachment upload",
				PrimaryCLIPath: "oa approval attachment upload",
			},
			Description: "上传本地文件为审批附件，一条命令完成初始化、HTTP PUT 与提交入库",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"审批附件上传提交结果","properties":{"spaceId":{"type":"integer","description":"审批附件所在钉盘空间 ID"},"fileName":{"type":"string","description":"文件名"},"fileSize":{"type":"integer","description":"文件字节数"},"class":{"type":"string","description":"服务端响应类型标识"},"fileType":{"type":"string","description":"文件类型"},"fileId":{"type":"string","description":"文件 ID"}},"required":["spaceId","fileName","fileSize","fileId"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令包含多个 RPC（init/commit）与本地 HTTP PUT 步骤，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "上传本地文件为审批附件，自动完成初始化+PUT+提交三步",
				UseWhen:      []string{"用户要把本地文件作为审批附件上传（首选一条命令自动完成凭证+PUT+提交）时"},
				AvoidWhen: []string{
					"仅需为已有附件授权下载或预览时使用 attachment authorize-download / authorize-preview",
					"仅需获取已有附件临时下载链接时使用 attachment download-url",
				},
				Examples: []string{"dws oa approval attachment upload --file ./合同.pdf --format json"},
			},
		},
	})
	uploadCmd.Flags().String("file", "", "本地文件路径 (必填)")
	uploadCmd.Flags().String("file-name", "", "完整文件名，例如 合同.pdf (默认使用文件名)")
	uploadCmd.Flags().String("md5", "", "文件原始字节内容的 MD5，32位十六进制字符串 (可选，不传则自动计算)")
	_ = uploadCmd.MarkFlagRequired("file")

	attachmentCmd.AddCommand(downloadURLCmd, authorizeDownloadCmd, authorizePreviewCmd, uploadCmd)
	return attachmentCmd
}

// oaAdminQueryMaxPageSize is the pageSize upper bound of
// get_process_instances_by_admin.
const oaAdminQueryMaxPageSize = float64(20)

// validateOARequestProcessCode checks the processCode field of a decoded
// --request payload: the tool requires it, and the backend answers a bad
// processCode with success:true and an empty list, so reject it client-side.
func validateOARequestProcessCode(request map[string]any) error {
	v, ok := request["processCode"]
	if !ok {
		return fmt.Errorf("--request 缺少必填字段 processCode")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return fmt.Errorf("--request processCode 必须为非空字符串")
	}
	return nil
}

// validateOARequestPageSize checks the pageSize field of a decoded
// --request payload (json.Number values from decodeOARequest).
func validateOARequestPageSize(request map[string]any) error {
	v, ok := request["pageSize"]
	if !ok {
		return nil
	}
	n, ok := v.(json.Number)
	if !ok {
		return fmt.Errorf("pageSize 必须为数字")
	}
	f, err := n.Float64()
	if err != nil || f < 1 || f > oaAdminQueryMaxPageSize {
		return fmt.Errorf("pageSize 必须在 1-%d 之间，got: %s", int(oaAdminQueryMaxPageSize), n.String())
	}
	return nil
}

// oaAdminTimeLayout is the startTime/endTime wire format of
// get_process_instances_by_admin since the 2026-08 MCP contract update
// (both fields changed from epoch millis to strings).
const oaAdminTimeLayout = "2006-01-02 15:04:05"

// oaAdminTimeLayoutHint is the user-facing spelling of the layout used in
// error messages.
const oaAdminTimeLayoutHint = "yyyy-MM-dd HH:mm:ss"

var oaAdminTimeZone = time.FixedZone("Asia/Shanghai", 8*3600)

// formatOAAdminQueryTime renders a millisecond timestamp into the
// yyyy-MM-dd HH:mm:ss string required by get_process_instances_by_admin.
func formatOAAdminQueryTime(ms int64) string {
	return time.UnixMilli(ms).In(oaAdminTimeZone).Format(oaAdminTimeLayout)
}

// validateOARequestTimeRange checks startTime/endTime of a decoded
// --request payload: startTime is required, both must be
// yyyy-MM-dd HH:mm:ss strings, and endTime must be strictly after
// startTime (matching simple mode's ValidateTimeRange).
func validateOARequestTimeRange(request map[string]any) error {
	startRaw, ok := request["startTime"]
	if !ok {
		return fmt.Errorf("--request 缺少必填字段 startTime")
	}
	startStr, ok := startRaw.(string)
	if !ok {
		return fmt.Errorf("startTime 必须为 %s 格式字符串", oaAdminTimeLayoutHint)
	}
	start, err := time.ParseInLocation(oaAdminTimeLayout, startStr, oaAdminTimeZone)
	if err != nil {
		return fmt.Errorf("startTime 必须为 %s 格式，got: %s", oaAdminTimeLayoutHint, startStr)
	}
	endRaw, ok := request["endTime"]
	if !ok {
		return nil
	}
	endStr, ok := endRaw.(string)
	if !ok {
		return fmt.Errorf("endTime 必须为 %s 格式字符串", oaAdminTimeLayoutHint)
	}
	end, err := time.ParseInLocation(oaAdminTimeLayout, endStr, oaAdminTimeZone)
	if err != nil {
		return fmt.Errorf("endTime 必须为 %s 格式，got: %s", oaAdminTimeLayoutHint, endStr)
	}
	if !end.After(start) {
		return fmt.Errorf("--request endTime 必须晚于 startTime")
	}
	return nil
}

// ──────────────────────────────────────────────────────────
// dws oa — OA 审批
// MCP tools（tools/list）: list_pending_approvals, get_processInstance_detail,
// approve_processInstance, reject_processInstance, revoke_processInstance,
// get_processInstance_records, list_initiated_instances, list_pending_tasks,
// list_user_visible_process, append_task, search_form, oa_ding_user, revert_task,
// get_inst_revert_activities, get_process_schema, forecast_process,
// start_process_instance, get_process_instances_by_admin,
// get_attachment_download_url, auth_download_file,
// auth_preview_attachment, init_attachment_upload_info, commit_attachment_upload_info
// ──────────────────────────────────────────────────────────

func newOaCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/oa.json
	// products.oa). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "oa",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询和处理 OA 审批实例、任务、记录、抄送、评论与附件授权",
			UseWhen: []string{
				"查看待审、已办、已发起或抄送审批，并执行同意、拒绝、撤销、转交等审批动作时",
				"获取审批附件下载链接，或为当前用户授权下载、预览审批附件时",
			},
			AvoidWhen: []string{
				"不要用于普通待办任务或工作日志；需要实时监听未来的审批任务/实例事件时使用 event consume",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "oa",
		Short: "OA 审批 / 同意 / 拒绝 / 撤销",
		Long:  `管理钉钉 OA 审批：待办查询、审批详情、同意、拒绝、撤销、操作记录、已发起列表、表单列表与附件授权。`,
		RunE:  groupRunE,
	})

	approvalCmd := newGroupCommand(&cobra.Command{Use: "approval", Short: "审批管理", RunE: groupRunE})

	approvalListPendingCmd := &cobra.Command{
		Use:     "list-pending",
		Short:   "查询待我处理的审批",
		Example: `  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endMs, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			argsMap := map[string]any{
				"starTime": float64(startMs),
				"endTime":  float64(endMs),
			}
			if v, _ := cmd.Flags().GetString("page"); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					argsMap["pageNum"] = n
				}
			}
			if v := flagOrFallback(cmd, "limit", "size"); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					argsMap["pageSize"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("list_pending_approvals", argsMap)
		},
	}
	DeclareLeafMetadata(approvalListPendingCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "list_pending_approvals",
				CanonicalPath:  "oa.list_pending_approvals",
				CLIPath:        "oa approval list-pending",
				PrimaryCLIPath: "oa approval list-pending",
			},
			Description: "查询当前用户待处理的审批单列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "list_pending_approvals"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户待处理的审批单列表",
				UseWhen:      []string{"需要查看待我处理的审批单并提取 processInstanceId / 跳转链接时"},
				AvoidWhen: []string{
					"已知实例只要 taskId 时改用 dws oa approval tasks",
					"不要用本命令执行同意/拒绝",
				},
				Examples: []string{
					"dws oa approval list-pending --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-10T23:59:59+08:00\"",
					"dws oa approval list-pending --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-10T23:59:59+08:00\" --query 关键词",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "end", Property: "endTime"},
				{Name: "limit", Property: "pageSize"},
				{Name: "page", Property: "pageNum"},
				{Name: "start", Property: "starTime"},
			},
		},
	})

	approvalDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "获取审批实例详情",
		Example: `  dws oa approval detail --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("get_processInstance_detail", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}
	DeclareLeafMetadata(approvalDetailCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_processInstance_detail",
				CanonicalPath:  "oa.get_processInstance_detail",
				CLIPath:        "oa approval detail",
				PrimaryCLIPath: "oa approval detail",
			},
			Description: "获取指定审批实例的详情信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_processInstance_detail"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定审批实例的详情信息",
				UseWhen:      []string{"已知 processInstanceId，需要查看表单内容与当前状态详情时"},
				AvoidWhen: []string{
					"需要操作历史而非表单详情时改用 dws oa approval records",
					"该命令不会同意/拒绝/撤销",
				},
				Examples: []string{
					"dws oa approval detail --instance-id <processInstanceId>",
					"dws oa approval detail --instance-id <processInstanceId> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	approvalApproveCmd := &cobra.Command{
		Use:   "approve",
		Short: "同意审批",
		Example: `  dws oa approval approve --instance-id <id> --task-id <taskId>  # 查询 instanceId: dws oa approval list-pending; taskId 来自 dws oa approval tasks
  dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id"); err != nil {
				return err
			}
			taskIdNum, _ := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			argsMap := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            taskIdNum,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("approve_processInstance", argsMap)
		},
	}
	DeclareLeafMetadata(approvalApproveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "approve_processInstance",
				CanonicalPath:  "oa.approve_processInstance",
				CLIPath:        "oa approval approve",
				PrimaryCLIPath: "oa approval approve",
			},
			Description: "同意审批",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "approve_processInstance"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "同意审批",
				UseWhen:      []string{"已知 processInstanceId 与待办 taskId，用户明确要求同意该审批任务时"},
				AvoidWhen: []string{
					"实例或 taskId 未确认时不要同意；先用 list-pending / tasks 取 ID",
					"要拒绝时改用 dws oa approval reject；要撤销自己发起的单时改用 revoke",
				},
				Examples: []string{
					"dws oa approval approve --instance-id <id> --task-id <taskId>",
					"dws oa approval approve --instance-id <id> --task-id <taskId> --remark \"同意\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	approvalRejectCmd := &cobra.Command{
		Use:     "reject",
		Short:   "拒绝审批",
		Example: `  dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不同意"  # 查询 instanceId: dws oa approval list-pending; taskId 来自 dws oa approval tasks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id"); err != nil {
				return err
			}
			taskIdNum, _ := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			argsMap := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            taskIdNum,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("reject_processInstance", argsMap)
		},
	}
	DeclareLeafMetadata(approvalRejectCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "reject_processInstance",
				CanonicalPath:  "oa.reject_processInstance",
				CLIPath:        "oa approval reject",
				PrimaryCLIPath: "oa approval reject",
			},
			Description: "拒绝审批",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "reject_processInstance"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "拒绝审批",
				UseWhen:      []string{"已知 processInstanceId 与 taskId，用户明确要求拒绝/驳回该审批任务时"},
				AvoidWhen: []string{
					"要同意时改用 dws oa approval approve",
					"实例、taskId 或拒绝原因未确认时不要拒绝",
				},
				Examples: []string{
					"dws oa approval reject --instance-id <id> --task-id <taskId> --remark \"不同意\"",
					"dws oa approval reject --instance-id <id> --task-id <taskId> --remark \"不符合要求\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	approvalRevokeCmd := &cobra.Command{
		Use:   "revoke",
		Short: "撤销已发起的审批",
		Example: `  dws oa approval revoke --instance-id <id> --yes  # 查询 instanceId: dws oa approval list-pending
  dws oa approval revoke --instance-id <id> --remark "误发起" --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			instanceId := mustGetFlag(cmd, "instance-id")
			argsMap := map[string]any{
				"processInstanceId": instanceId,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("revoke_processInstance", argsMap)
		},
	}
	DeclareLeafMetadata(approvalRevokeCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "revoke_processInstance",
				CanonicalPath:  "oa.revoke_processInstance",
				CLIPath:        "oa approval revoke",
				PrimaryCLIPath: "oa approval revoke",
			},
			Description: "撤销当前用户已发起的审批实例",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "revoke_processInstance"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "撤销当前用户已发起的审批实例",
				UseWhen:      []string{"用户明确要求撤销自己已发起的审批实例，且 processInstanceId 已确认时"},
				AvoidWhen: []string{
					"不是自己发起的单或 instanceId 未确认时不要撤销",
					"要拒绝别人的待办时改用 reject，不要用 revoke",
				},
				Examples: []string{"dws oa approval revoke --instance-id <instance-id>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	approvalRecordsCmd := &cobra.Command{
		Use:     "records",
		Short:   "获取审批操作记录",
		Example: `  dws oa approval records --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("get_processInstance_records", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}
	DeclareLeafMetadata(approvalRecordsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_processInstance_records",
				CanonicalPath:  "oa.get_processInstance_records",
				CLIPath:        "oa approval records",
				PrimaryCLIPath: "oa approval records",
			},
			Description: "获取审批操作记录",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_processInstance_records"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取审批操作记录",
				UseWhen:      []string{"已知 processInstanceId，需要查看谁做了什么审批操作及结果时"},
				AvoidWhen: []string{
					"需要当前表单详情时改用 dws oa approval detail",
					"该命令只读历史，不处理审批",
				},
				Examples: []string{
					"dws oa approval records --instance-id <processInstanceId>",
					"dws oa approval records --instance-id <processInstanceId> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	approvalListInitiatedCmd := &cobra.Command{
		Use:     "list-initiated",
		Short:   "查询审批模板下已发起的审批记录",
		Example: `  dws oa approval list-initiated --process-code <code> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --limit 20  # processCode 来自管理后台配置`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "process-code"); err != nil {
				return err
			}
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endMs, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			nextToken, _ := strconv.ParseFloat(flagOrFallback(cmd, "cursor", "next-token"), 64)
			maxResults, _ := strconv.ParseFloat(flagOrFallback(cmd, "limit", "max-results"), 64)
			return callMCPTool("list_initiated_instances", map[string]any{
				"processCode": mustGetFlag(cmd, "process-code"),
				"startTime":   float64(startMs),
				"endTime":     float64(endMs),
				"nextToken":   nextToken,
				"maxResults":  maxResults,
			})
		},
	}
	DeclareLeafMetadata(approvalListInitiatedCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "list_initiated_instances",
				CanonicalPath:  "oa.list_initiated_instances",
				CLIPath:        "oa approval list-initiated",
				PrimaryCLIPath: "oa approval list-initiated",
			},
			Description: "查询当前用户已发起的审批实例列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "list_initiated_instances"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户已发起的审批实例列表",
				UseWhen:      []string{"已知 processCode，需要按起止时间查询自己发起的审批实例基础信息时"},
				AvoidWhen: []string{
					"不知道 processCode 时先用 dws oa approval list-forms",
					"要撤销实例时先确认 instanceId 再改用 revoke",
				},
				Examples: []string{
					"dws oa approval list-initiated --process-code <code> --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-10T23:59:59+08:00\" --cursor 0 --limit 20",
					"dws oa approval list-initiated --process-code <code> --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-10T23:59:59+08:00\" --cursor 0 --limit 20 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "end", Property: "endTime"},
				{Name: "limit", Property: "maxResults"},
				{Name: "start", Property: "startTime"},
			},
		},
	})

	approvalTasksCmd := &cobra.Command{
		Use:     "tasks",
		Short:   "查询待我审批的任务 ID",
		Example: `  dws oa approval tasks --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("list_pending_tasks", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}
	DeclareLeafMetadata(approvalTasksCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "list_pending_tasks",
				CanonicalPath:  "oa.list_pending_tasks",
				CLIPath:        "oa approval tasks",
				PrimaryCLIPath: "oa approval tasks",
			},
			Description: "查询待我审批的任务Id",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "list_pending_tasks"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询待我审批的任务Id",
				UseWhen:      []string{"已知 processInstanceId，需要取得当前用户待办 taskId 以便同意或拒绝时"},
				AvoidWhen: []string{
					"还不知道实例列表时先用 dws oa approval list-pending",
					"本命令只取 taskId，不执行审批",
				},
				Examples: []string{
					"dws oa approval tasks --instance-id <processInstanceId>",
					"dws oa approval tasks --instance-id <processInstanceId> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	approvalListFormsCmd := &cobra.Command{
		Use:     "list-forms",
		Short:   "获取当前用户可见的审批表单列表",
		Example: `  dws oa approval list-forms --cursor 0 --limit 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cursor, _ := strconv.ParseFloat(mustGetFlag(cmd, "cursor"), 64)
			pageSize, _ := strconv.ParseFloat(flagOrFallback(cmd, "limit", "size"), 64)
			return callMCPTool("list_user_visible_process", map[string]any{
				"cursor":   cursor,
				"pageSize": pageSize,
			})
		},
	}
	DeclareLeafMetadata(approvalListFormsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "list_user_visible_process",
				CanonicalPath:  "oa.list_user_visible_process",
				CLIPath:        "oa approval list-forms",
				PrimaryCLIPath: "oa approval list-forms",
			},
			Description: "获取当前用户可见的审批表单列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "list_user_visible_process"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前用户可见的审批表单列表",
				UseWhen:      []string{"需要列出可见审批模板并取得 processCode 时"},
				AvoidWhen:    []string{"需要实例、任务或操作记录时不要使用；该命令只列可见模板"},
				Examples: []string{
					"dws oa approval list-forms --cursor 0 --limit 100",
					"dws oa approval list-forms --cursor 0 --limit 100 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "pageSize"},
			},
		},
	})

	// 模糊搜索表单（按 processCode 或 name 关键字）
	approvalSearchFormsCmd := &cobra.Command{
		Use:   "search-forms",
		Short: "按关键字模糊搜索当前用户可见的审批表单",
		Example: `  dws oa approval search-forms --query AI
  dws oa approval search-forms --query 报销`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "query"); err != nil {
				return err
			}
			return callMCPTool("search_form", map[string]any{
				"query": mustGetFlag(cmd, "query"),
			})
		},
	}

	// 获取审批任务的被催办人 userId
	// 仅返回 userId；发 DING 的 robotCode 由 $DINGTALK_DING_ROBOT_CODE / --robot-code 提供，content 由 agent 撰写。
	approvalDingInfoCmd := &cobra.Command{
		Use:   "ding-info",
		Short: "获取审批任务的被催办人 userId（需与 ding message send 串联使用）",
		Example: `  dws oa approval ding-info --task-id <taskId>
  # 返回的 userId 作为 --users 传入 dws ding message send：
  # dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId逗号拼接> --content "请尽快审批"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			return callMCPTool("oa_ding_user", map[string]any{
				"taskId": mustGetFlag(cmd, "task-id"),
			})
		},
	}

	// 已经审批过的
	approvalExecutedListCmd := &cobra.Command{
		Use:     "list-executed",
		Short:   "获取当前用户已经处理过的审批单列表",
		Example: `  dws oa approval list-executed  --limit 20 --page 1 --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			pageNumber, _ := strconv.ParseFloat(mustGetFlag(cmd, "page"), 64)
			argsMap := map[string]any{
				"pageNumber": pageNumber,
				"pageSize":   pageSize,
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("get_done_tasks", argsMap)
		},
	}
	DeclareLeafMetadata(approvalExecutedListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_done_tasks",
				CanonicalPath:  "oa.get_done_tasks",
				CLIPath:        "oa approval list-executed",
				PrimaryCLIPath: "oa approval list-executed",
			},
			Description: "获取员工已处理任务列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_done_tasks"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取员工已处理任务列表",
				UseWhen:      []string{"需要查看当前用户已经处理过的审批单列表时"},
				AvoidWhen: []string{
					"要看待处理单时改用 dws oa approval list-pending",
					"要看自己发起或抄送时改用 list-submitted / list-cc",
				},
				Examples: []string{
					"dws oa approval list-executed --limit <pageSize> --page <pageNumber> --query 关键词",
					"dws oa approval list-executed --limit <pageSize> --page <pageNumber> --query 关键词 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "pageSize"},
				{Name: "page", Property: "pageNumber"},
			},
		},
	})
	// 已发起
	approvalSubmittedListCmd := &cobra.Command{
		Use:     "list-submitted",
		Short:   "获取当前用户已发起的审批单列表",
		Example: `  dws oa approval list-submitted --limit 20 --page 1 --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			pageNumber, _ := strconv.ParseFloat(mustGetFlag(cmd, "page"), 64)
			argsMap := map[string]any{
				"pageNumber": pageNumber,
				"pageSize":   pageSize,
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("get_submitted_instances", argsMap)
		},
	}
	DeclareLeafMetadata(approvalSubmittedListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_submitted_instances",
				CanonicalPath:  "oa.get_submitted_instances",
				CLIPath:        "oa approval list-submitted",
				PrimaryCLIPath: "oa approval list-submitted",
			},
			Description: "获取已提交实例列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_submitted_instances"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取已提交实例列表",
				UseWhen:      []string{"需要查看当前用户已提交/发起相关的审批单列表时"},
				AvoidWhen: []string{
					"要按 processCode 与时间窗列已发起实例时也可对照 list-initiated",
					"要处理待办时改用 list-pending / tasks",
				},
				Examples: []string{
					"dws oa approval list-submitted --limit <pageSize> --page <pageNumber> --query 关键词",
					"dws oa approval list-submitted --limit <pageSize> --page <pageNumber> --query 关键词 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "pageSize"},
				{Name: "page", Property: "pageNumber"},
			},
		},
	})
	// 抄送
	approvalCcListCmd := &cobra.Command{
		Use:     "list-cc",
		Short:   "获取抄送当前用户的审批单列表",
		Example: `  dws oa approval list-cc --limit 20 --page 1 --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			pageNumber, _ := strconv.ParseFloat(mustGetFlag(cmd, "page"), 64)
			argsMap := map[string]any{
				"pageNumber": pageNumber,
				"pageSize":   pageSize,
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("get_noticed_instances", argsMap)
		},
	}
	DeclareLeafMetadata(approvalCcListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_noticed_instances",
				CanonicalPath:  "oa.get_noticed_instances",
				CLIPath:        "oa approval list-cc",
				PrimaryCLIPath: "oa approval list-cc",
			},
			Description: "获取抄送用户的列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_noticed_instances"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取抄送用户的列表",
				UseWhen:      []string{"需要查看抄送给当前用户的审批单列表时"},
				AvoidWhen: []string{
					"要看待处理或自己发起的审批时不要使用",
					"该命令只列抄送实例，不执行审批动作",
				},
				Examples: []string{
					"dws oa approval list-cc --limit <pageSize> --page <pageNumber> --query 关键词",
					"dws oa approval list-cc --limit <pageSize> --page <pageNumber> --query 关键词 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "pageSize"},
				{Name: "page", Property: "pageNumber"},
			},
		},
	})

	// 转交任务
	approvalTransferCmd := &cobra.Command{
		Use:   "redirect-task",
		Short: "转交审批任务给其他人",
		Example: `  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId>
  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "请帮忙处理"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "to-actioner-id"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"taskId":       mustGetFlag(cmd, "task-id"),
				"toActionerId": mustGetFlag(cmd, "to-actioner-id"),
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("redirect_task", argsMap)
		},
	}
	DeclareLeafMetadata(approvalTransferCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "redirect_task",
				CanonicalPath:  "oa.redirect_task",
				CLIPath:        "oa approval redirect-task",
				PrimaryCLIPath: "oa approval redirect-task",
			},
			Description: "转交审批任务",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "redirect_task"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "转交审批任务",
				UseWhen:      []string{"已知 taskId 与接收人 toActionerId，用户明确要求转交审批任务时"},
				AvoidWhen: []string{
					"任务、接收人或转交意图未确认时不要转交",
					"要自己处理时改用 approve/reject",
				},
				Examples: []string{
					"dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId>",
					"dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark \"请帮忙处理\"",
				},
			},
		},
	})

	// 评论审批实例
	approvalCommentCmd := &cobra.Command{
		Use:     "oa-comments",
		Short:   "对审批实例添加评论",
		Example: `  dws oa approval oa-comments --instance-id <processInstanceId> --content "同意，请尽快处理"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			commentText := flagOrFallback(cmd, "content", "text")
			if commentText == "" {
				return fmt.Errorf("--content is required")
			}
			return callMCPTool("dingflow_comments", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"text":              commentText,
			})
		},
	}
	DeclareLeafMetadata(approvalCommentCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "dingflow_comments",
				CanonicalPath:  "oa.dingflow_comments",
				CLIPath:        "oa approval oa-comments",
				PrimaryCLIPath: "oa approval oa-comments",
			},
			Description: "用户添加审批评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "dingflow_comments"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "用户添加审批评论",
				UseWhen:      []string{"已知 processInstanceId，需要为审批实例添加评论文本时"},
				AvoidWhen: []string{
					"只需同意/拒绝/转交时改用对应写命令",
					"评论内容或实例未确认时不要添加",
				},
				Examples: []string{
					"dws oa approval oa-comments --instance-id <processInstanceId> --content \"同意，请尽快处理\"",
					"dws oa approval oa-comments --instance-id <processInstanceId> --content \"同意，请尽快处理\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content", Property: "text"},
				{Name: "instance-id", Property: "processInstanceId"},
			},
		},
	})

	// 审批抄送
	approvalCcCmd := &cobra.Command{
		Use:   "oa-cc-noticer",
		Short: "对审批实例进行抄送",
		Example: `  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "68674200835816"
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "userId1,userId2" --operator-id "123123"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			userListStr := flagOrFallback(cmd, "users", "user-list")
			if userListStr == "" {
				return fmt.Errorf("--users is required")
			}
			userList := strings.Split(userListStr, ",")
			argsMap := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"userList":          userList,
			}
			return callMCPTool("oa_cc_noticer", argsMap)
		},
	}
	DeclareLeafMetadata(approvalCcCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "oa_cc_noticer",
				CanonicalPath:  "oa.oa_cc_noticer",
				CLIPath:        "oa approval oa-cc-noticer",
				PrimaryCLIPath: "oa approval oa-cc-noticer",
			},
			Description: "抄送审批人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "oa_cc_noticer"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "抄送审批人",
				UseWhen:      []string{"已知 processInstanceId，需要为审批实例追加抄送人时"},
				AvoidWhen: []string{
					"只需查看抄送列表时改用 dws oa approval list-cc",
					"实例与抄送人未确认时不要添加",
				},
				Examples: []string{
					"dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users \"68674200835816\"",
					"dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users \"userId1,userId2\" --operator-id \"123123\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "instance-id", Property: "processInstanceId"},
				{Name: "users", Property: "userList"},
			},
		},
	})

	// 加签任务
	approvalAppendTaskCmd := &cobra.Command{
		Use:   "append-task",
		Short: "对审批任务进行加签",
		Example: `  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type before --appender-user-ids "userId1,userId2" --activate-type ALL --agree-all true
  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type Parallel --appender-user-ids "userId1" --activate-type ONE_BY_ONE --agree-all false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id", "type", "appender-user-ids", "activate-type", "agree-all"); err != nil {
				return err
			}
			typeVal := mustGetFlag(cmd, "type")
			if typeVal != "before" && typeVal != "after" && typeVal != "Parallel" {
				return fmt.Errorf("--type must be one of: before, after, Parallel, got: %s", typeVal)
			}
			activateTypeVal := mustGetFlag(cmd, "activate-type")
			if activateTypeVal != "ALL" && activateTypeVal != "ONE_BY_ONE" {
				return fmt.Errorf("--activate-type must be one of: ALL, ONE_BY_ONE, got: %s", activateTypeVal)
			}
			appenderUserIdsStr := mustGetFlag(cmd, "appender-user-ids")
			appenderUserIds := strings.Split(appenderUserIdsStr, ",")
			agreeAll, err := strconv.ParseBool(mustGetFlag(cmd, "agree-all"))
			if err != nil {
				return fmt.Errorf("--agree-all must be 'true' or 'false', got: %s", mustGetFlag(cmd, "agree-all"))
			}
			return callMCPTool("append_task", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            mustGetFlag(cmd, "task-id"),
				"type":              typeVal,
				"appenderUserIds":   appenderUserIds,
				"activateType":      activateTypeVal,
				"agreeAll":          agreeAll,
			})
		},
	}

	// 获取任务可回退的节点信息
	approvalRevertActivitiesCmd := &cobra.Command{
		Use:   "revert-activities",
		Short: "获取审批任务可回退的节点信息（退回前必须调用，获取可回退节点列表）",
		Example: `  dws oa approval revert-activities --task-id <taskId>
  # 返回可回退节点列表，从中选择 targetActivityId 和 action`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			taskIdNum, err := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			if err != nil {
				return fmt.Errorf("--task-id must be a number, got: %s", mustGetFlag(cmd, "task-id"))
			}
			return callMCPTool("get_inst_revert_activities", map[string]any{
				"taskId": taskIdNum,
			})
		},
	}

	// 退回任务（退回到审批人或发起人）
	approvalRevertTaskCmd := &cobra.Command{
		Use:   "revert-task",
		Short: "退回审批任务到指定节点（审批人或发起人）",
		Example: `  # 退回到发起人（targetActivityId 固定传 sid-startevent）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id sid-startevent --action REVERT_FOR_RESUBMIT --remark "补充说明后重提"
  # 退回到某个审批节点（targetActivityId 从审批流程节点信息中获取 activityId）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id <activityId> --action REVERT_FOR_APPROVAL --remark "重新审批"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id", "target-activity-id", "action"); err != nil {
				return err
			}
			action := mustGetFlag(cmd, "action")
			if action != "REVERT_FOR_APPROVAL" && action != "REVERT_FOR_RESUBMIT" {
				return fmt.Errorf("--action must be one of: REVERT_FOR_APPROVAL, REVERT_FOR_RESUBMIT, got: %s", action)
			}
			targetActivityId := mustGetFlag(cmd, "target-activity-id")
			// 退回发起人时，targetActivityId 固定为 sid-startevent
			if action == "REVERT_FOR_RESUBMIT" && targetActivityId != "sid-startevent" {
				return fmt.Errorf("--action=REVERT_FOR_RESUBMIT 时 --target-activity-id 必须为 sid-startevent，got: %s", targetActivityId)
			}
			taskIdNum, err := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			if err != nil {
				return fmt.Errorf("--task-id must be a number, got: %s", mustGetFlag(cmd, "task-id"))
			}
			inner := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            taskIdNum,
				"targetActivityId":  targetActivityId,
				"revertAction":      action,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				inner["remark"] = v
			}
			return callMCPTool("revert_task", map[string]any{
				"RevertTaskRequest": inner,
			})
		},
	}

	approvalFormSchemaCmd := &cobra.Command{
		Use: "form-schema", Short: "查询审批模板的表单 Schema",
		Example: "dws oa approval form-schema --process-code <processCode>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "process-code"); err != nil {
				return err
			}
			return callMCPTool("get_process_schema", map[string]any{"processCode": mustGetFlag(cmd, "process-code")})
		},
	}
	DeclareLeafMetadata(approvalFormSchemaCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_process_schema",
				CanonicalPath:  "oa.get_process_schema",
				CLIPath:        "oa approval form-schema",
				PrimaryCLIPath: "oa approval form-schema",
			},
			Description: "查询审批模板的表单 Schema",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_process_schema"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询审批模板的表单 Schema",
				UseWhen:      []string{"已从 list-forms 或 search-forms 获得 processCode，需要读取字段、选项和必填规则后再填写审批时"},
				AvoidWhen:    []string{"只需列出可用模板时使用 list-forms；不要把返回的 Schema 当作可直接提交的实例请求"},
				Examples:     []string{"dws oa approval form-schema --process-code <processCode>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "process-code", Property: "processCode"},
			},
		},
	})
	approvalForecastCmd := &cobra.Command{
		Use: "forecast-process", Short: "根据表单值预测审批流程与自选节点",
		Example: "dws oa approval forecast-process --process-code <processCode> --dept-id -1 --form-values '{\"金额\":\"100\"}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			if raw, _ := cmd.Flags().GetString("request"); raw != "" {
				request, err := decodeOARequest(raw)
				if err != nil {
					return fmt.Errorf("--request JSON 解析失败: %w", err)
				}
				return callMCPTool("forecast_process", map[string]any{"ProcessForecastPopRequest": request})
			}
			if err := validateRequiredFlags(cmd, "process-code", "dept-id", "form-values"); err != nil {
				return err
			}
			deptID, err := strconv.ParseInt(mustGetFlag(cmd, "dept-id"), 10, 64)
			if err != nil {
				return fmt.Errorf("--dept-id 必须为整数: %w", err)
			}
			values, err := oaFormValues(mustGetFlag(cmd, "form-values"))
			if err != nil {
				return fmt.Errorf("--form-values JSON 解析失败: %w", err)
			}
			return callMCPTool("forecast_process", map[string]any{"ProcessForecastPopRequest": map[string]any{"processCode": mustGetFlag(cmd, "process-code"), "deptId": deptID, "formComponentValues": [][]map[string]string{values}}})
		},
	}
	DeclareLeafMetadata(approvalForecastCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "forecast_process",
				CanonicalPath:  "oa.forecast_process",
				CLIPath:        "oa approval forecast-process",
				PrimaryCLIPath: "oa approval forecast-process",
			},
			Description: "根据表单值预测审批流程与自选节点",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "forecast_process"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "预测审批流程与自选审批节点",
				UseWhen:      []string{"已知道 processCode 且已根据表单 Schema 组装字段，需要在发起前确认审批路径或自选节点时"},
				AvoidWhen:    []string{"需要真正创建审批单时改用 create-instance；未获得字段定义时先用 form-schema"},
				Examples: []string{
					"dws oa approval forecast-process --process-code <processCode> --dept-id -1 --form-values '{\"金额\":\"100\"}'",
					"dws oa approval forecast-process --request '{\"processCode\":\"PROC-xxx\",\"deptId\":-1,\"formComponentValues\":[[{\"name\":\"金额\",\"value\":\"100\"}]]}'",
				},
			},
			// Simple-mode flags are mapping exclusions (encoded inside ProcessForecastPopRequest).
			Parameters: []contract.ParamDecl{
				{Name: "request", Property: "ProcessForecastPopRequest", InterfaceType: "object"},
			},
		},
	})
	// 以管理员身份查询审批实例列表
	listByAdminSimpleFlags := []string{"process-code", "start", "end", "cursor", "limit", "user-ids", "statuses"}
	approvalListByAdminCmd := &cobra.Command{
		Use: "list-by-admin", Short: "以管理员身份查询审批模板的实例列表",
		Example: `  dws oa approval list-by-admin --process-code <code> --start "2026-03-10T00:00:00+08:00" --cursor 0 --limit 20
  dws oa approval list-by-admin --request '{"processCode":"PROC-xxx","startTime":"2026-03-10 00:00:00","cursor":0,"pageSize":20,"statuses":["RUNNING"]}'`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Cobra 的 flag group 校验（英文报错）在 PreRunE 之后执行，
			// 这里先校验同一组约束，保证用户看到的是中文错误。
			request, _ := cmd.Flags().GetString("request")
			processCode, _ := cmd.Flags().GetString("process-code")
			if request == "" && processCode == "" {
				return fmt.Errorf("--request、--process-code 至少指定一个")
			}
			if request != "" {
				for _, name := range listByAdminSimpleFlags {
					if cmd.Flags().Changed(name) {
						return fmt.Errorf("--request 与 --%s 不能同时指定", name)
					}
				}
			}
			if processCode != "" && !cmd.Flags().Changed("start") {
				return fmt.Errorf("--process-code、--start 必须同时指定（缺少 --start）")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if raw, _ := cmd.Flags().GetString("request"); raw != "" {
				request, err := decodeOARequest(raw)
				if err != nil {
					return fmt.Errorf("--request JSON 解析失败: %w", err)
				}
				if err := validateOARequestProcessCode(request); err != nil {
					return err
				}
				if err := validateOARequestPageSize(request); err != nil {
					return err
				}
				if err := validateOARequestTimeRange(request); err != nil {
					return err
				}
				return callMCPTool("get_process_instances_by_admin", map[string]any{"ProcessInstanceListQueryRequest": request})
			}
			if err := validateRequiredFlags(cmd, "process-code", "start"); err != nil {
				return err
			}
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			cursor, err := strconv.ParseFloat(mustGetFlag(cmd, "cursor"), 64)
			if err != nil {
				return fmt.Errorf("--cursor 必须为数字: %w", err)
			}
			pageSize, err := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			if err != nil {
				return fmt.Errorf("--limit 必须为数字: %w", err)
			}
			if pageSize < 1 || pageSize > oaAdminQueryMaxPageSize {
				return fmt.Errorf("--limit 必须在 1-%d 之间，got: %s", int(oaAdminQueryMaxPageSize), mustGetFlag(cmd, "limit"))
			}
			request := map[string]any{
				"processCode": mustGetFlag(cmd, "process-code"),
				"startTime":   formatOAAdminQueryTime(startMs),
				"cursor":      cursor,
				"pageSize":    pageSize,
			}
			if v, _ := cmd.Flags().GetString("end"); v != "" {
				endMs, err := parseISOTimeToMillis("end", v)
				if err != nil {
					return err
				}
				if err := validateTimeRange(startMs, endMs); err != nil {
					return err
				}
				request["endTime"] = formatOAAdminQueryTime(endMs)
			}
			if v, _ := cmd.Flags().GetString("user-ids"); v != "" {
				request["userIds"] = strings.Split(v, ",")
			}
			if v, _ := cmd.Flags().GetString("statuses"); v != "" {
				request["statuses"] = strings.Split(v, ",")
			}
			return callMCPTool("get_process_instances_by_admin", map[string]any{"ProcessInstanceListQueryRequest": request})
		},
	}
	DeclareLeafMetadata(approvalListByAdminCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "get_process_instances_by_admin",
				CanonicalPath:  "oa.get_process_instances_by_admin",
				CLIPath:        "oa approval list-by-admin",
				PrimaryCLIPath: "oa approval list-by-admin",
			},
			Description: "以管理员身份获取审批单列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "get_process_instances_by_admin"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "以管理员身份按模板、时间范围、状态与用户查询企业内审批实例列表",
				UseWhen:      []string{"具备 OA 审批管理员权限，需要跨用户统计或检索某模板下的审批实例时"},
				AvoidWhen: []string{
					"只查自己的待办/已办/已发起/抄送时改用 list-pending / list-executed / list-initiated / list-cc",
					"无 OA 管理员权限时不要使用，该命令查不到数据",
				},
				Examples: []string{
					"dws oa approval list-by-admin --process-code <code> --start \"2026-03-10T00:00:00+08:00\" --cursor 0 --limit 20",
					"dws oa approval list-by-admin --process-code <code> --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-10T23:59:59+08:00\" --statuses RUNNING,COMPLETED --user-ids \"userId1,userId2\"",
				},
			},
			// Simple-mode flags are mapping exclusions (encoded inside ProcessInstanceListQueryRequest).
			Parameters: []contract.ParamDecl{
				{Name: "request", Property: "ProcessInstanceListQueryRequest", InterfaceType: "object"},
			},
		},
	})
	approvalCreateCmd := &cobra.Command{
		Use: "create-instance", Short: "发起审批实例（需要 --yes 确认）",
		Example: "dws oa approval create-instance --process-code <processCode> --form-values '{\"事由\":\"测试\"}' --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !commandDryRun(cmd) {
				yes, _ := cmd.Flags().GetBool("yes")
				if !yes {
					return fmt.Errorf("发起审批实例会创建真实业务数据；请先核对参数，然后添加 --yes 确认执行")
				}
			}
			var request map[string]any
			if raw, _ := cmd.Flags().GetString("request"); raw != "" {
				var err error
				request, err = decodeOARequest(raw)
				if err != nil {
					return fmt.Errorf("--request JSON 解析失败: %w", err)
				}
			} else {
				if err := validateRequiredFlags(cmd, "process-code", "form-values"); err != nil {
					return err
				}
				values, err := oaFormValues(mustGetFlag(cmd, "form-values"))
				if err != nil {
					return fmt.Errorf("--form-values JSON 解析失败: %w", err)
				}
				request = map[string]any{"processCode": mustGetFlag(cmd, "process-code"), "formComponentValues": values}
				if dept, _ := cmd.Flags().GetString("dept-id"); dept != "" {
					value, err := strconv.ParseInt(dept, 10, 64)
					if err != nil {
						return fmt.Errorf("--dept-id 必须为整数: %w", err)
					}
					request["deptId"] = value
				}
				if userID, _ := cmd.Flags().GetString("originator-user-id"); userID != "" {
					request["originatorUserId"] = userID
				}
				if rawApprovers, _ := cmd.Flags().GetString("approvers"); rawApprovers != "" {
					action, _ := cmd.Flags().GetString("approvers-action-type")
					if action != "AND" && action != "OR" && action != "NONE" {
						return fmt.Errorf("--approvers-action-type 必须为 AND、OR 或 NONE")
					}
					request["approvers"] = []map[string]any{{"actionType": action, "userIds": strings.Split(rawApprovers, ",")}}
				}
				if rawCC, _ := cmd.Flags().GetString("cc-list"); rawCC != "" {
					position, _ := cmd.Flags().GetString("cc-position")
					if position != "START" && position != "FINISH" && position != "START_FINISH" {
						return fmt.Errorf("--cc-position 必须为 START、FINISH 或 START_FINISH")
					}
					request["ccList"] = strings.Split(rawCC, ",")
					request["ccPosition"] = position
				}
			}
			return callMCPTool("start_process_instance", map[string]any{"ProcessInstanceCreationPopRequest": request})
		},
	}
	DeclareLeafMetadata(approvalCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "oa",
				Name:           "start_process_instance",
				CanonicalPath:  "oa.start_process_instance",
				CLIPath:        "oa approval create-instance",
				PrimaryCLIPath: "oa approval create-instance",
			},
			Description: "发起审批实例（需要 --yes 确认）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "oa", RPCName: "start_process_instance"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "发起新的审批实例",
				UseWhen:      []string{"用户确认要发起审批，且已查询表单 Schema、核对字段及审批路径后使用"},
				AvoidWhen:    []string{"只需预测流程时使用 forecast-process；用户尚未确认或字段未按 Schema 核对时不要发起"},
				Examples: []string{
					"dws oa approval create-instance --process-code <processCode> --form-values '{\"事由\":\"测试\"}'",
					"dws oa approval create-instance --request '{\"processCode\":\"PROC-xxx\",\"deptId\":-1,\"formComponentValues\":[{\"name\":\"事由\",\"value\":\"测试\"}],\"targetSelectActioners\":[{\"actionerKey\":\"manual-node\",\"actionerStaffIds\":[\"user-id\"]}]}'",
				},
			},
			// Simple-mode flags are mapping exclusions (encoded inside ProcessInstanceCreationPopRequest).
			Parameters: []contract.ParamDecl{
				{Name: "request", Property: "ProcessInstanceCreationPopRequest", InterfaceType: "object"},
			},
		},
	})

	approvalListPendingCmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	approvalListPendingCmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	approvalListPendingCmd.Flags().String("page", "", "分页页码 (可选)")
	approvalListPendingCmd.Flags().String("limit", "", "每页大小 (可选)")
	approvalListPendingCmd.Flags().String("size", "", "每页大小 (可选)")
	approvalListPendingCmd.Flags().Lookup("size").Hidden = true
	approvalListPendingCmd.Flags().String("query", "", "关键字搜索（可选）")

	approvalDetailCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalApproveCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalApproveCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalApproveCmd.Flags().String("remark", "", "审批意见 (可选)")
	approvalRejectCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalRejectCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalRejectCmd.Flags().String("remark", "", "审批意见 (可选)")
	approvalRevokeCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalRevokeCmd.Flags().String("remark", "", "撤销说明 (可选)")
	approvalRecordsCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalListInitiatedCmd.Flags().String("process-code", "", "表单 processCode (必填)")
	approvalListInitiatedCmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	approvalListInitiatedCmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	approvalListInitiatedCmd.Flags().String("cursor", "0", "分页游标，首次传 0")
	approvalListInitiatedCmd.Flags().String("next-token", "0", "分页游标，首次传 0")
	approvalListInitiatedCmd.Flags().Lookup("next-token").Hidden = true
	approvalListInitiatedCmd.Flags().String("limit", "20", "每页大小，最大 20")
	approvalListInitiatedCmd.Flags().String("max-results", "20", "每页大小，最大 20")
	approvalListInitiatedCmd.Flags().Lookup("max-results").Hidden = true
	approvalTasksCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalListFormsCmd.Flags().String("cursor", "0", "分页游标（默认 0，翻页传返回的 cursor）")
	approvalListFormsCmd.Flags().String("limit", "100", "每页大小（默认 100，最大 100）")
	approvalListFormsCmd.Flags().String("size", "100", "每页大小（默认 100，最大 100）")
	approvalListFormsCmd.Flags().Lookup("size").Hidden = true
	approvalSearchFormsCmd.Flags().String("query", "", "关键字（匹配 processCode 或表单名称）(必填)")
	approvalDingInfoCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalExecutedListCmd.Flags().String("page", "1", "分页页码（可选）")
	approvalExecutedListCmd.Flags().String("limit", "20", "每页大小（可选）")
	approvalExecutedListCmd.Flags().String("query", "", "关键字搜索（可选）")
	approvalSubmittedListCmd.Flags().String("page", "1", "分页页码（可选）")
	approvalSubmittedListCmd.Flags().String("limit", "20", "每页大小（可选）")
	approvalSubmittedListCmd.Flags().String("query", "", "关键字搜索（可选）")
	approvalCcListCmd.Flags().String("page", "1", "分页页码（可选）")
	approvalCcListCmd.Flags().String("limit", "20", "每页大小（可选）")
	approvalCcListCmd.Flags().String("query", "", "关键字搜索（可选）")
	approvalTransferCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalTransferCmd.Flags().String("to-actioner-id", "", "转交目标用户 ID (必填)")
	approvalTransferCmd.Flags().String("remark", "", "转交说明 (可选)")
	approvalCommentCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalCommentCmd.Flags().String("content", "", "评论内容 (必填)")
	approvalCommentCmd.Flags().String("text", "", "评论内容 (必填)")
	approvalCommentCmd.Flags().Lookup("text").Hidden = true
	approvalCcCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalCcCmd.Flags().String("users", "", "抄送用户 ID 列表，多个用逗号分隔 (必填)")
	approvalCcCmd.Flags().String("user-list", "", "抄送用户 ID 列表，多个用逗号分隔 (必填)")
	approvalCcCmd.Flags().Lookup("user-list").Hidden = true
	approvalCcCmd.Flags().String("operator-id", "", "操作人 ID (可选)")

	approvalAppendTaskCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalAppendTaskCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalAppendTaskCmd.Flags().String("type", "", "加签类型：before（前加签），after（后加签），Parallel（并加签）(必填)")
	approvalAppendTaskCmd.Flags().String("appender-user-ids", "", "被加签用户 ID 列表，多个用逗号分隔 (必填)")
	approvalAppendTaskCmd.Flags().String("activate-type", "", "任务激活类型：ALL（或签），ONE_BY_ONE（依次审批）(必填)")
	approvalAppendTaskCmd.Flags().String("agree-all", "", "是否需要全部同意，true 或 false (必填)")

	approvalRevertActivitiesCmd.Flags().String("task-id", "", "审批任务 ID (必填)")

	approvalRevertTaskCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalRevertTaskCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalRevertTaskCmd.Flags().String("target-activity-id", "", "退回到的节点 ID（退回发起人固定传 sid-startevent）(必填)")
	approvalRevertTaskCmd.Flags().String("action", "", "退回方式：REVERT_FOR_APPROVAL（退回到审批人）/ REVERT_FOR_RESUBMIT（退回到发起人）(必填)")
	approvalRevertTaskCmd.Flags().String("remark", "", "退回说明 (可选)")
	approvalFormSchemaCmd.Flags().String("process-code", "", "审批模板 processCode (必填)")
	approvalForecastCmd.Flags().String("process-code", "", "审批模板 processCode（简单模式使用；与 --request 互斥）")
	approvalForecastCmd.Flags().String("dept-id", "", "发起人部门 ID（简单模式使用；与 --request 互斥）")
	approvalForecastCmd.Flags().String("form-values", "", "表单值 JSON（简单模式使用；与 --request 互斥）")
	approvalForecastCmd.Flags().String("request", "", "完整请求 JSON（高级模式；与简单模式参数互斥）")
	approvalForecastCmd.MarkFlagsOneRequired("request", "process-code")
	approvalForecastCmd.MarkFlagsRequiredTogether("process-code", "dept-id", "form-values")
	forecastMutuallyExclusive := make([][]string, 0, 3)
	for _, name := range []string{"process-code", "dept-id", "form-values"} {
		approvalForecastCmd.MarkFlagsMutuallyExclusive("request", name)
		forecastMutuallyExclusive = append(forecastMutuallyExclusive, []string{"request", name})
	}
	cli.AnnotateRuntimeConstraints(approvalForecastCmd, cli.RuntimeSchemaConstraints{
		MutuallyExclusive: forecastMutuallyExclusive,
		RequireOneOf:      [][]string{{"request", "process-code"}},
		RequireTogether:   [][]string{{"process-code", "dept-id", "form-values"}},
	})

	approvalListByAdminCmd.Flags().String("process-code", "", "审批模板 processCode（简单模式使用；与 --request 互斥）")
	approvalListByAdminCmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00)（简单模式使用；与 --request 互斥）")
	approvalListByAdminCmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00)（可选）")
	approvalListByAdminCmd.Flags().String("cursor", "0", "分页游标，首次传 0")
	approvalListByAdminCmd.Flags().String("limit", "20", "每页大小，最大 20")
	approvalListByAdminCmd.Flags().String("user-ids", "", "按发起人 userId 过滤，多个用逗号分隔（可选）")
	approvalListByAdminCmd.Flags().String("statuses", "", "按审批状态过滤，多个用逗号分隔（可选，如 RUNNING、TERMINATED、COMPLETED）")
	approvalListByAdminCmd.Flags().String("request", "", "完整请求 JSON（高级模式；与简单模式参数互斥）")
	approvalListByAdminCmd.MarkFlagsOneRequired("request", "process-code")
	approvalListByAdminCmd.MarkFlagsRequiredTogether("process-code", "start")
	listByAdminMutuallyExclusive := make([][]string, 0, len(listByAdminSimpleFlags))
	for _, name := range listByAdminSimpleFlags {
		approvalListByAdminCmd.MarkFlagsMutuallyExclusive("request", name)
		listByAdminMutuallyExclusive = append(listByAdminMutuallyExclusive, []string{"request", name})
	}
	cli.AnnotateRuntimeConstraints(approvalListByAdminCmd, cli.RuntimeSchemaConstraints{
		MutuallyExclusive: listByAdminMutuallyExclusive,
		RequireOneOf:      [][]string{{"request", "process-code"}},
		RequireTogether:   [][]string{{"process-code", "start"}},
	})

	approvalCreateCmd.Flags().String("process-code", "", "审批模板 processCode（简单模式使用；与 --request 互斥）")
	approvalCreateCmd.Flags().String("dept-id", "-1", "发起人部门 ID")
	approvalCreateCmd.Flags().String("form-values", "", "表单值 JSON（简单模式使用；与 --request 互斥）")
	approvalCreateCmd.Flags().String("request", "", "完整请求 JSON（高级模式；与简单模式参数互斥）")
	approvalCreateCmd.Flags().String("originator-user-id", "", "审批发起人 userId")
	approvalCreateCmd.Flags().String("approvers", "", "审批人 userId 列表，多个用逗号分隔")
	approvalCreateCmd.Flags().String("approvers-action-type", "OR", "审批类型：AND、OR 或 NONE")
	approvalCreateCmd.Flags().String("cc-list", "", "抄送人 userId 列表，多个用逗号分隔")
	approvalCreateCmd.Flags().String("cc-position", "START", "抄送时点：START、FINISH 或 START_FINISH")
	approvalCreateCmd.MarkFlagsOneRequired("request", "process-code")
	approvalCreateCmd.MarkFlagsRequiredTogether("process-code", "form-values")
	createSimpleFlags := []string{"process-code", "dept-id", "form-values", "originator-user-id", "approvers", "approvers-action-type", "cc-list", "cc-position"}
	createMutuallyExclusive := make([][]string, 0, len(createSimpleFlags))
	for _, name := range createSimpleFlags {
		approvalCreateCmd.MarkFlagsMutuallyExclusive("request", name)
		createMutuallyExclusive = append(createMutuallyExclusive, []string{"request", name})
	}
	cli.AnnotateRuntimeConstraints(approvalCreateCmd, cli.RuntimeSchemaConstraints{
		MutuallyExclusive: createMutuallyExclusive,
		RequireOneOf:      [][]string{{"request", "process-code"}},
		RequireTogether:   [][]string{{"process-code", "form-values"}},
	})

	approvalCmd.AddCommand(
		approvalListPendingCmd,
		approvalDetailCmd,
		approvalApproveCmd,
		approvalRejectCmd,
		approvalRevokeCmd,
		approvalRecordsCmd,
		approvalListInitiatedCmd,
		approvalTasksCmd,
		approvalListFormsCmd,
		approvalSearchFormsCmd,
		approvalDingInfoCmd,
		approvalExecutedListCmd,
		approvalSubmittedListCmd,
		approvalCcListCmd,
		approvalTransferCmd,
		approvalCommentCmd,
		approvalCcCmd,
		approvalAppendTaskCmd,
		approvalRevertActivitiesCmd,
		approvalRevertTaskCmd,
		approvalFormSchemaCmd,
		approvalForecastCmd,
		approvalListByAdminCmd,
		approvalCreateCmd,
	)
	approvalCmd.AddCommand(newOAAttachmentCommand())
	root.AddCommand(approvalCmd)

	return root
}
