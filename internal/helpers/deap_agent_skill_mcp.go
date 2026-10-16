// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

const (
	deapAgentSkillCreateFileTool       = "create_skill_from_file"
	deapAgentSkillUploadCredentialTool = "create_skill_upload_credential"
	deapAgentSkillCreateURLTool        = "create_skill_by_url"
	deapAgentSkillListTool             = "list_skills"
	deapAgentSkillQueryTool            = "query_skill"
	deapAgentSkillQueryIdentity        = "get_skill_detail"
	deapAgentSkillUpdateTool           = "update_skill"
	deapAgentSkillDeleteTool           = "delete_skill"
	deapAgentMCPCreateTool             = "create_mcp"
	deapAgentMCPListTool               = "list_mcps"
	deapAgentMCPQueryTool              = "query_mcp"
	deapAgentMCPQueryIdentity          = "get_mcp_detail"
	deapAgentMCPUpdateTool             = "update_mcp"
	deapAgentMCPDeleteTool             = "delete_mcp"
	deapAgentMCPCheckTool              = "check_mcp"
	deapAgentSkillUploadPath           = "/v1.0/assistant/skills/upload"
	deapAgentSkillUploadProdBase       = "https://api-deap.dingtalk.com"
	deapAgentSkillUploadPreBase        = "https://pre-api-deap.dingtalk.com"

	deapAgentConfigFileMaxSize    = 1024 * 1024
	deapAgentSkillMaxPackageSize  = 50 * 1024 * 1024
	deapAgentSkillMaxExpandedSize = 200 * 1024 * 1024
	deapAgentSkillMaxEntries      = 10000
)

var deapAgentSnapshots = []string{"draft", "published"}

// Keep file/ZIP I/O injectable so read races and decompressor failures can be
// verified without platform-specific permission tricks or huge archives.
var deapAgentReadFile = os.ReadFile
var deapAgentOpenZIPEntry = func(entry *zip.File) (io.ReadCloser, error) { return entry.Open() }

type deapAgentSkillPackage struct {
	path string
	size int64
	file *os.File
}

type deapAgentSkillCreated struct {
	SkillID     string `json:"skillId"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Version     int64  `json:"version,omitempty"`
}

type deapAgentSkillPackageUploader interface {
	Upload(ctx context.Context, agentUUID, fileName string, file io.Reader) (string, error)
}

type deapAgentOpenAPISkillUploader struct {
	baseURL           string
	httpClient        *http.Client
	resolveCredential func(context.Context, string) (string, error)
	validateTarget    func(string) error
}

type deapAgentOpenAPISkillDetail struct {
	SkillID     string `json:"skillId"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type deapAgentOpenAPISkillPayload struct {
	SkillID string                       `json:"skillId"`
	Skill   *deapAgentOpenAPISkillDetail `json:"skill"`
}

type deapAgentOpenAPISkillEnvelope struct {
	SkillID string                        `json:"skillId"`
	Skill   *deapAgentOpenAPISkillDetail  `json:"skill"`
	Success *bool                         `json:"success"`
	Data    *deapAgentOpenAPISkillPayload `json:"data"`
	Result  *deapAgentOpenAPISkillPayload `json:"result"`
	Content *deapAgentOpenAPISkillPayload `json:"content"`
}

type deapAgentOpenAPIUploadPayload struct {
	FileURL string `json:"fileUrl"`
}

type deapAgentOpenAPIUploadEnvelope struct {
	FileURL string                         `json:"fileUrl"`
	Success *bool                          `json:"success"`
	Data    *deapAgentOpenAPIUploadPayload `json:"data"`
	Result  *deapAgentOpenAPIUploadPayload `json:"result"`
	Content *deapAgentOpenAPIUploadPayload `json:"content"`
}

type deapAgentSkillUploadCredentialPayload struct {
	TemporaryAPIKey string `json:"temporaryApiKey"`
	ExpireAt        int64  `json:"expireAt"`
}

type deapAgentSkillUploadCredentialEnvelope struct {
	TemporaryAPIKey string                                 `json:"temporaryApiKey"`
	ExpireAt        int64                                  `json:"expireAt"`
	Success         *bool                                  `json:"success"`
	Data            *deapAgentSkillUploadCredentialPayload `json:"data"`
	Result          *deapAgentSkillUploadCredentialPayload `json:"result"`
	Content         *deapAgentSkillUploadCredentialPayload `json:"content"`
}

func (u deapAgentOpenAPISkillUploader) Upload(ctx context.Context, agentUUID, fileName string, file io.Reader) (string, error) {
	fileURL, err := u.uploadReader(ctx, agentUUID, fileName, deapAgentSkillUploadPath, file)
	if err != nil {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: err}
	}
	return fileURL, nil
}

func (u deapAgentOpenAPISkillUploader) uploadFile(
	ctx context.Context, agentUUID, filePath, uploadPath string,
) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("本地文件不可读")
	}
	defer file.Close()
	return u.uploadReader(ctx, agentUUID, filepath.Base(filePath), uploadPath, file)
}

func (u deapAgentOpenAPISkillUploader) uploadReader(
	ctx context.Context, agentUUID, fileName, uploadPath string, file io.Reader,
) (fileURL string, err error) {
	started := time.Now()
	slog.DebugContext(ctx, "dingtalk_tag.file_upload", "stage", "start", "target_path", uploadPath)
	defer func() {
		slog.DebugContext(ctx, "dingtalk_tag.file_upload", "stage", "complete", "target_path", uploadPath,
			"success", err == nil, "duration_ms", time.Since(started).Milliseconds())
	}()
	baseURL, err := u.uploadBaseURL()
	if err != nil {
		return "", fmt.Errorf("OpenAPI 环境解析失败")
	}
	credential, err := u.temporaryCredential(ctx, agentUUID)
	if err != nil {
		// 保留底层阶段原因与服务端 code/trace 以便定位（凭证解析失败已在
		// temporaryCredential 内脱敏），不再统一吞成无信息的“认证失败”。
		return "", fmt.Errorf("OpenAPI 认证失败: %w", err)
	}
	client := apiclient.NewClient(credential, baseURL)
	if u.httpClient != nil {
		client.HTTPClient = u.httpClient
	}
	if u.validateTarget != nil {
		client.TargetValidator = u.validateTarget
	}
	response, err := client.UploadMultipart(ctx, apiclient.MultipartUploadRequest{
		Path:       uploadPath,
		FieldName:  "file",
		FileName:   filepath.Base(fileName),
		File:       file,
		BearerAuth: true,
	})
	if err != nil {
		return "", fmt.Errorf("OpenAPI 上传请求失败")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("OpenAPI 返回 HTTP %d", response.StatusCode)
	}

	var envelope deapAgentOpenAPIUploadEnvelope
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return "", fmt.Errorf("OpenAPI 上传响应格式非法")
	}
	if envelope.Success != nil && !*envelope.Success {
		return "", fmt.Errorf("OpenAPI 上传失败")
	}
	payload := deapAgentOpenAPIUploadPayload{FileURL: envelope.FileURL}
	for _, candidate := range []*deapAgentOpenAPIUploadPayload{envelope.Data, envelope.Result, envelope.Content} {
		if candidate != nil {
			payload = *candidate
			break
		}
	}
	if strings.TrimSpace(payload.FileURL) == "" {
		return "", fmt.Errorf("OpenAPI 上传响应缺少 fileUrl")
	}
	return payload.FileURL, nil
}

func deapAgentParseSkillCreated(body []byte) (deapAgentSkillCreated, error) {
	var envelope deapAgentOpenAPISkillEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return deapAgentSkillCreated{}, fmt.Errorf("创建响应格式非法")
	}
	payload := deapAgentOpenAPISkillPayload{SkillID: envelope.SkillID, Skill: envelope.Skill}
	for _, candidate := range []*deapAgentOpenAPISkillPayload{envelope.Data, envelope.Result, envelope.Content} {
		if candidate != nil {
			payload = *candidate
			break
		}
	}
	if strings.TrimSpace(payload.SkillID) == "" {
		return deapAgentSkillCreated{}, fmt.Errorf("创建响应缺少 skillId")
	}
	created := deapAgentSkillCreated{SkillID: payload.SkillID}
	if payload.Skill != nil {
		created.Name = payload.Skill.Name
		created.DisplayName = payload.Skill.DisplayName
		created.Description = payload.Skill.Description
	}
	return created, nil
}

func (u deapAgentOpenAPISkillUploader) temporaryCredential(ctx context.Context, agentUUID string) (string, error) {
	if u.resolveCredential != nil {
		credential, resolveErr := u.resolveCredential(ctx, agentUUID)
		if resolveErr != nil {
			// 注入式解析器的错误可能携带凭证材料，绝不外泄，只保留阶段语义。
			return "", fmt.Errorf("OpenAPI 凭证解析失败")
		}
		return credential, nil
	}
	if deps == nil || deps.Caller == nil {
		return "", fmt.Errorf("OpenAPI credential resolver is not configured")
	}
	responseText, err := callMCPToolReturnTextOnServer(ctx, deapAgentServerID,
		deapAgentSkillUploadCredentialTool, map[string]any{"agentUuid": agentUUID})
	if err != nil {
		// 凭证尚未取得，调用错误只含服务端 code/trace、不含密钥；保留以便定位预发/线上抖动。
		return "", fmt.Errorf("获取上传凭证失败: %w", err)
	}
	return deapAgentParseSkillUploadCredential(responseText)
}

func deapAgentParseSkillUploadCredential(responseText string) (string, error) {
	var envelope deapAgentSkillUploadCredentialEnvelope
	if err := json.Unmarshal([]byte(responseText), &envelope); err != nil {
		return "", fmt.Errorf("temporary upload credential response is invalid")
	}
	if envelope.Success != nil && !*envelope.Success {
		return "", fmt.Errorf("temporary upload credential request failed")
	}
	payload := deapAgentSkillUploadCredentialPayload{
		TemporaryAPIKey: envelope.TemporaryAPIKey,
		ExpireAt:        envelope.ExpireAt,
	}
	for _, candidate := range []*deapAgentSkillUploadCredentialPayload{
		envelope.Data, envelope.Result, envelope.Content,
	} {
		if candidate != nil {
			payload = *candidate
			break
		}
	}
	if strings.TrimSpace(payload.TemporaryAPIKey) == "" || payload.ExpireAt <= 0 {
		return "", fmt.Errorf("temporary upload credential response is incomplete")
	}
	return strings.TrimSpace(payload.TemporaryAPIKey), nil
}

func (u deapAgentOpenAPISkillUploader) uploadBaseURL() (string, error) {
	if strings.TrimSpace(u.baseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(u.baseURL), "/"), nil
	}
	parsed, err := url.Parse(config.GetMCPBaseURL())
	if err != nil {
		return "", err
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "mcp.dingtalk.com":
		return deapAgentSkillUploadProdBase, nil
	case host == "pre-mcp.dingtalk.com", strings.HasPrefix(host, "pre-mcp-gw."):
		return deapAgentSkillUploadPreBase, nil
	default:
		return "", fmt.Errorf("unsupported MCP environment")
	}
}

func deapAgentSkillStageFromResponse(body []byte, fallback string) string {
	lower := strings.ToLower(string(body))
	for _, stage := range []string{"upload", "create", "query"} {
		if strings.Contains(lower, stage+" stage") {
			return stage
		}
	}
	return fallback
}

type deapAgentSkillStageError struct {
	Operation string
	Stage     string
	Err       error
}

func (e *deapAgentSkillStageError) Error() string {
	if e == nil {
		return "skill create 阶段失败"
	}
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "create"
	}
	detail := ""
	if e.Err != nil {
		detail = e.Err.Error()
	}
	lower := strings.ToLower(detail)
	for _, marker := range []string{"http://", "https://", "token", "secret", "credential", "password", "fileurl", "uploadurl"} {
		if strings.Contains(lower, marker) {
			detail = "下游错误详情已脱敏"
			break
		}
	}
	if detail == "" {
		return fmt.Sprintf("skill %s %s 阶段失败", operation, e.Stage)
	}
	return fmt.Sprintf("skill %s %s 阶段失败: %s", operation, e.Stage, detail)
}

func (e *deapAgentSkillStageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var deapAgentSkillUploader deapAgentSkillPackageUploader = deapAgentOpenAPISkillUploader{}

func deapAgentValidateSkillPackage(rawPath string) (deapAgentSkillPackage, error) {
	pkg, err := deapAgentOpenSkillPackage(rawPath)
	if err == nil {
		pkg.file.Close()
		pkg.file = nil
	}
	return pkg, err
}

// The caller owns the returned file until upload completes. ZIP validation uses
// ReadAt on this handle, so it cannot validate one inode and upload another.
func deapAgentOpenSkillPackage(rawPath string) (deapAgentSkillPackage, error) {
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(rawPath)), ".zip") {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 必须使用 .zip 扩展名")
	}
	resolved, err := apperrors.SafeInputPath(strings.TrimSpace(rawPath))
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation(fmt.Sprintf("参数 --file 路径不安全: %v", err))
	}
	// Reject special files before a potentially blocking open. The handle is
	// checked again below; path metadata is not used to validate the ZIP bytes.
	if info, err := os.Stat(resolved); err == nil && !info.Mode().IsRegular() {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 必须是普通文件")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 文件不可读")
	}
	pkg, err := deapAgentValidateOpenedSkillPackage(file, resolved)
	if err != nil {
		file.Close()
	}
	return pkg, err
}

func deapAgentValidateOpenedSkillPackage(file *os.File, resolved string) (deapAgentSkillPackage, error) {
	info, err := file.Stat()
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 文件不可读")
	}
	if !info.Mode().IsRegular() {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 必须是普通文件")
	}
	if info.Size() > deapAgentSkillMaxPackageSize {
		return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 不能超过 50 MiB")
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 不是有效 ZIP 文件")
	}
	if len(reader.File) > deapAgentSkillMaxEntries {
		return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 文件条目过多")
	}
	foundSkill := false
	var declaredSize uint64
	var expandedSize int64
	for _, entry := range reader.File {
		name := entry.Name
		cleanName := pathpkg.Clean(strings.ReplaceAll(name, "\\", "/"))
		if strings.Contains(name, "\\") || pathpkg.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, "../") || (len(cleanName) >= 2 && cleanName[1] == ':') {
			return deapAgentSkillPackage{}, apperrors.NewValidation(fmt.Sprintf("Skill ZIP 包含不安全路径 %q", name))
		}
		mode := entry.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !mode.IsRegular()) {
			return deapAgentSkillPackage{}, apperrors.NewValidation(fmt.Sprintf("Skill ZIP 包含不安全文件类型 %q", name))
		}
		if entry.UncompressedSize64 > uint64(deapAgentSkillMaxExpandedSize) || declaredSize > uint64(deapAgentSkillMaxExpandedSize)-entry.UncompressedSize64 {
			return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 解压后内容过大")
		}
		declaredSize += entry.UncompressedSize64
		if !entry.FileInfo().IsDir() && pathpkg.Base(cleanName) == "SKILL.md" {
			foundSkill = true
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		body, openErr := deapAgentOpenZIPEntry(entry)
		if openErr != nil {
			return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 文件内容损坏")
		}
		remaining := deapAgentSkillMaxExpandedSize - expandedSize
		readSize, readErr := io.CopyN(io.Discard, body, remaining+1)
		closeErr := body.Close()
		expandedSize += readSize
		if expandedSize > deapAgentSkillMaxExpandedSize {
			return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 解压后内容过大")
		}
		if readErr != nil && readErr != io.EOF {
			return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 文件内容损坏")
		}
		if closeErr != nil {
			return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 文件内容损坏")
		}
	}
	if !foundSkill {
		return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 中缺少 SKILL.md")
	}
	slog.Debug("dingtalk_tag.skill_package_validated", "file_size", info.Size())
	return deapAgentSkillPackage{path: resolved, size: info.Size(), file: file}, nil
}

func newDeapCapabilityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "capability",
		Short:             "数字员工能力资源管理",
		Long:              "管理目标数字员工的 Skill/MCP 能力资源，统一 create|update|delete|list|query 五个动作。create 内部完成校验并把资源自动挂载到草稿；update 修改资源内容或启停状态并保持现有挂载（MCP 更新前内部先 check_mcp 校验、Skill 替换 ZIP 时 CLI 内部完成上传）；delete 删除资源并清理草稿挂载。以上写操作都只改草稿、不自动发布，发布另行执行 manage publish。list/query 只读，不改变挂载或发布状态。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(newDeapAgentSkillCommand(), newDeapAgentMCPCommand())
	return cmd
}

func newDeapAgentSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "skill",
		Short:             "管理数字员工 Skill",
		Long:              "管理目标数字员工域的 Skill 资源：create 从本地 ZIP 校验并创建、自动挂载到草稿；update 按 skillId 改启停状态（--enabled=true|false）或替换 ZIP（--file，CLI 内部完成上传），保持现有挂载；delete 删除 Skill 并清理草稿挂载；list/query 只读查询 draft/published 快照。写操作只改草稿、不自动发布。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(newDeapAgentSkillCreateCommand(), newDeapAgentSkillUpdateCommand(), newDeapAgentSkillDeleteCommand(), newDeapAgentSkillListCommand(), newDeapAgentSkillQueryCommand())
	return cmd
}

func newDeapAgentMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "mcp",
		Short:             "管理数字员工 MCP",
		Long:              "管理目标数字员工域的 MCP：create 从本地 JSON 校验并创建、把原 mcpId 自动挂载到草稿；update 按 mcpId 改启停状态（--enabled=true|false）或替换配置（--config-file，更新前 CLI 内部先 check_mcp 校验通过才提交），保持现有挂载；delete 删除 MCP 并清理草稿挂载；list/query 只读查询。写操作只改草稿、不自动发布，敏感配置通过本地 JSON 文件传入、不进入命令行。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(newDeapAgentMCPCreateCommand(), newDeapAgentMCPUpdateCommand(), newDeapAgentMCPDeleteCommand(), newDeapAgentMCPListCommand(), newDeapAgentMCPQueryCommand())
	return cmd
}

func newDeapAgentSkillCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "从本地 ZIP 创建 Skill 资源",
		Long: "校验本地 Skill ZIP 后，先通过 OpenAPI multipart 接口上传，再调用 create_skill_by_url 完成 Skill Center create、query 并自动挂载到员工草稿，不自动发布。ZIP 不进入 MCP JSON，临时签名 URL 不落盘、不输出。",
		Tool: deapAgentSkillCreateFileTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（Skill Center V2 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "file", Usage: "本地 Skill ZIP（相对当前目录、最大 50 MiB、必须包含 SKILL.md）", Bind: "file", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
		Validate: func(cmd *cobra.Command, _ []string) error {
			rawPath, _ := cmd.Flags().GetString("file")
			if _, err := deapAgentValidateSkillPackage(rawPath); err != nil {
				return &deapAgentSkillStageError{Stage: "validate", Err: err}
			}
			return nil
		},
		Call: deapAgentCallSkillCreate,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentSkillCreateFileTool, CanonicalPath: "dingtalk-tag.create_skill_from_file", CLIPath: "dingtalk-tag capability skill create", PrimaryCLIPath: "dingtalk-tag capability skill create", Group: "capability.skill"},
			Description: "校验本地 ZIP，依次调用 OpenAPI upload 与 create_skill_by_url，自动挂载员工草稿但不发布，并只输出安全创建结果。",
			DryRun:      deapAgentPlanDryRun,
			Interface:   &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "本地 ZIP 校验后串联 OpenAPI multipart upload 与 create_skill_by_url"},
			Selection:   contract.SelectionSpec{AgentSummary: "从本地 ZIP 创建 Skill 并自动挂载草稿，不自动发布", UseWhen: []string{"已有合法 Skill ZIP，需要为目标数字员工创建、挂载并取得 skillId 时"}, AvoidWhen: []string{"已有 skillId 需要升级 ZIP 时使用 capability skill update"}, Examples: []string{"dws dingtalk-tag capability skill create --agent-uuid <agentUuid> --file ./my-skill.zip --dry-run --format json"}},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid", InterfaceType: "string"},
				{Name: "file", Property: "file", InterfaceType: "binary"},
			},
		},
	})
}

func deapAgentCallSkillCreate(cmd *cobra.Command, _ string, args map[string]any) error {
	agentUUID, _ := args["agentUuid"].(string)
	rawPath, _ := args["file"].(string)
	pkg, err := deapAgentOpenSkillPackage(rawPath)
	if err != nil {
		return &deapAgentSkillStageError{Stage: "validate", Err: err}
	}
	defer pkg.file.Close()
	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dryRun":       true,
			"dry_run":      true,
			"preview_kind": contract.DryRunPreviewPlan,
			"executed":     false,
			"action":       "upload_then_create_skill",
			"agentUuid":    agentUUID,
			"fileName":     filepath.Base(pkg.path),
			"fileSize":     pkg.size,
		})
	}
	fileURL, err := deapAgentSkillUploader.Upload(cmd.Context(), agentUUID, filepath.Base(pkg.path), io.NewSectionReader(pkg.file, 0, pkg.size))
	if err != nil {
		if staged, ok := err.(*deapAgentSkillStageError); ok {
			return staged
		}
		return &deapAgentSkillStageError{Stage: "upload", Err: err}
	}
	responseText, err := callMCPToolReturnTextOnServer(cmd.Context(), deapAgentServerID, deapAgentSkillCreateURLTool, map[string]any{
		"agentUuid": agentUUID,
		"fileUrl":   fileURL,
	})
	if err != nil {
		stage := deapAgentSkillStageFromResponse([]byte(err.Error()), "create")
		return &deapAgentSkillStageError{Stage: stage, Err: err}
	}
	result, err := deapAgentParseSkillCreated([]byte(responseText))
	if err != nil {
		return &deapAgentSkillStageError{Stage: "query", Err: err}
	}
	// deapAgentParseSkillCreated already requires a non-empty skillId.
	return deps.Out.PrintJSON(result)
}

func newDeapAgentSkillUpdateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "update", Short: "更新 Skill 启停状态或替换 ZIP",
		Long: "按 skillId 更新草稿中的 Skill，保持现有挂载关系，不自动发布。--enabled=true|false 切换启用状态（不传则不改）；--file 提供新 ZIP 时，CLI 先本地校验再通过 OpenAPI multipart 上传，签名 URL 不落盘、不输出。--enabled 与 --file 必须二选一且互斥：换包用 --file、切启停用 --enabled，不能同时提供（对应服务端 update_skill 的 fileUrl 不能与 enabled/attributes 并存）。先 --dry-run 检查参数，再加 --yes。",
		Tool: deapAgentSkillUpdateTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（Skill Center V2 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "skill-id", Usage: "Skill ID", Bind: "skillId", Required: true, Trim: true},
			{Name: "enabled", Usage: "启用状态 true|false；不传保持原值（草稿态，不自动发布）；不能与 --file 同时使用", Bind: "enabled", Kind: LeafBool},
			{Name: "file", Usage: "可选本地 Skill ZIP（相对当前目录、最大 50 MiB、必须包含 SKILL.md）；提供时替换 Skill 包；不能与 --enabled 同时使用", Bind: "file", Trim: true, OmitEmpty: true},
		},
		Constraints: []LeafConstraint{{
			Kind: LeafExactlyOne, Flags: []string{"enabled", "file"},
			Description: "--enabled 与 --file 必须二选一且不能同时提供",
		}},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		Validate: func(cmd *cobra.Command, _ []string) error {
			hasEnabled := cmd.Flags().Changed("enabled")
			rawPath, _ := cmd.Flags().GetString("file")
			hasFile := strings.TrimSpace(rawPath) != ""
			if !hasEnabled && !hasFile {
				return apperrors.NewValidation("update 至少需要提供 --enabled 或 --file 之一")
			}
			if hasFile && hasEnabled {
				return apperrors.NewValidation("--file 与 --enabled 互斥：替换 Skill 包请单独使用 --file，切换启停请单独使用 --enabled，不能同时提供")
			}
			if hasFile {
				if _, err := deapAgentValidateSkillPackage(rawPath); err != nil {
					return &deapAgentSkillStageError{Operation: "update", Stage: "validate", Err: err}
				}
			}
			return nil
		},
		Call: deapAgentCallSkillUpdate,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentSkillUpdateTool, CanonicalPath: "dingtalk-tag.update_skill", CLIPath: "dingtalk-tag capability skill update", PrimaryCLIPath: "dingtalk-tag capability skill update", Group: "capability.skill"},
			Description: "按 skillId 更新草稿 Skill 的启停状态或替换 ZIP（CLI 内部上传），保持现有挂载，不自动发布。",
			DryRun:      deapAgentDryRun, Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "可选本地 ZIP 校验上传后调用 update_skill"},
			Selection: contract.SelectionSpec{AgentSummary: "更新草稿 Skill 启停状态或替换 ZIP", UseWhen: []string{"需要在不重建的前提下启用/禁用或升级已有 Skill 时"}, AvoidWhen: []string{"需要新增 Skill 时使用 capability skill create", "只查看现有 Skill 时使用 capability skill list/query"}, Examples: []string{"dws dingtalk-tag capability skill update --agent-uuid <agentUuid> --skill-id <skillId> --enabled=false --dry-run --format json"}},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid", InterfaceType: "string"},
				{Name: "skill-id", Property: "skillId", InterfaceType: "string"},
				{Name: "enabled", Property: "enabled", InterfaceType: "boolean"},
				{Name: "file", Property: "fileUrl", InterfaceType: "binary", Description: "可选本地 ZIP；CLI 上传后以 fileUrl 下发，ZIP 不进入 MCP JSON"},
			},
		},
	})
}

func deapAgentCallSkillUpdate(cmd *cobra.Command, tool string, args map[string]any) error {
	rawPath, _ := args["file"].(string)
	delete(args, "file")
	if strings.TrimSpace(rawPath) == "" {
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	pkg, err := deapAgentOpenSkillPackage(rawPath)
	if err != nil {
		return &deapAgentSkillStageError{Operation: "update", Stage: "validate", Err: err}
	}
	defer pkg.file.Close()
	if deps.Caller.DryRun() {
		// 干跑只预览占位，不上传、不生成签名 URL。
		args["fileUrl"] = map[string]any{"localFile": filepath.Base(pkg.path), "upload": true, "redacted": true}
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	agentUUID, _ := args["agentUuid"].(string)
	fileURL, err := deapAgentSkillUploader.Upload(cmd.Context(), agentUUID, filepath.Base(pkg.path), io.NewSectionReader(pkg.file, 0, pkg.size))
	if err != nil {
		if staged, ok := err.(*deapAgentSkillStageError); ok {
			return &deapAgentSkillStageError{Operation: "update", Stage: staged.Stage, Err: staged.Err}
		}
		return &deapAgentSkillStageError{Operation: "update", Stage: "upload", Err: err}
	}
	args["fileUrl"] = fileURL
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

func newDeapAgentSkillDeleteCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "delete", Short: "删除 Skill 并清理草稿挂载",
		Long: "按 skillId 删除草稿中的 Skill 资源并清理其草稿挂载，不自动发布。这是不可逆写操作，先 --dry-run 检查参数，再加 --yes。",
		Tool: deapAgentSkillDeleteTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（Skill Center V2 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "skill-id", Usage: "Skill ID", Bind: "skillId", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentSkillDeleteTool, CanonicalPath: "dingtalk-tag.delete_skill", CLIPath: "dingtalk-tag capability skill delete", PrimaryCLIPath: "dingtalk-tag capability skill delete", Group: "capability.skill"},
			Description: "按 skillId 删除草稿 Skill 资源并清理草稿挂载，不自动发布。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentSkillDeleteTool),
			Selection: contract.SelectionSpec{AgentSummary: "删除草稿 Skill 并清理挂载", UseWhen: []string{"确认不再需要某个 Skill，需要从资源和草稿挂载中移除时"}, AvoidWhen: []string{"只需临时禁用时用 capability skill update --enabled=false"}, Examples: []string{"dws dingtalk-tag capability skill delete --agent-uuid <agentUuid> --skill-id <skillId> --dry-run --format json"}},
		},
	})
}

func newDeapAgentSkillListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "list", Short: "查询 Skill 资源列表",
		Long: "查询目标数字员工 tenant 下的 Skill 资源列表及非敏感配置。查询无数据会返回健康空列表。",
		Tool: deapAgentSkillListTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（Skill Center V2 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "snapshot", Usage: "配置快照：draft 或 published", Bind: "snapshot", Default: "draft", ArgDefault: "draft", Enum: deapAgentSnapshots},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentSkillListTool, CanonicalPath: "dingtalk-tag.list_skills", CLIPath: "dingtalk-tag capability skill list", PrimaryCLIPath: "dingtalk-tag capability skill list", Group: "capability.skill"},
			Description: "查询独立 Skill 资源列表和非敏感配置。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentSkillListTool),
			Selection: contract.SelectionSpec{AgentSummary: "查询目标数字员工 tenant 下的 Skill 列表", UseWhen: []string{"需要选择或核对目标数字员工的 Skill 时"}, AvoidWhen: []string{"已知 skillId 需要完整详情时使用 capability skill query"}, Examples: []string{"dws dingtalk-tag capability skill list --agent-uuid <agentUuid> --snapshot draft --format json"}},
		},
	})
}

func newDeapAgentSkillQueryCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "query", Short: "查询 Skill 资源详情",
		Long: "按 skillId 查询独立 Skill 资源详情和非敏感配置。查询失败与查询无数据由服务端分别返回。",
		Tool: deapAgentSkillQueryTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（Skill Center V2 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "skill-id", Usage: "Skill ID", Bind: "skillId", Required: true, Trim: true},
			{Name: "snapshot", Usage: "配置快照：draft 或 published", Bind: "snapshot", Default: "draft", ArgDefault: "draft", Enum: deapAgentSnapshots},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentSkillQueryIdentity, CanonicalPath: "dingtalk-tag.get_skill_detail", CLIPath: "dingtalk-tag capability skill query", PrimaryCLIPath: "dingtalk-tag capability skill query", Group: "capability.skill"},
			Description: "按 skillId 查询独立 Skill 资源详情和非敏感配置。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentSkillQueryTool),
			Selection: contract.SelectionSpec{AgentSummary: "查询目标数字员工 tenant 下的一个 Skill", UseWhen: []string{"已知 agentUuid 和 skillId，需要核对解析信息或配置时"}, AvoidWhen: []string{"需要浏览全部 Skill 时使用 capability skill list"}, Examples: []string{"dws dingtalk-tag capability skill query --agent-uuid <agentUuid> --skill-id <skillId> --format json"}},
		},
	})
}

func newDeapAgentMCPCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "创建 MCP 并自动挂载到员工草稿",
		Long: "从本地 JSON 对象文件在目标数字员工资源域创建 MCP。CLI 先内部调用 check_mcp 做只读连通性校验，通过后服务端依次创建、查询，再将原 mcpId 自动挂载到草稿的 MCP 列表并回读确认，保留已有选择，不克隆、不自动发布。文件根节点必须包含 name 和 configString；configString 内 mcpServers.<名称>.type 必须显式填写 streamable-http 或 sse，不能只填 URL。CLI 将配置字段展开到 create_mcp 工具根节点，不包装 config。凭据不会进入 argv。若失败信息含 stage=query_created_mcp 或 stage=mount_draft，保留已创建的 mcpId，先查询资源和 draft 并按 trace 排查挂载，禁止重复 create。同一员工的创建、保存和发布应串行执行。此语义依赖已部署自动挂载实现的 OpenAPI 与保留未传字段的 Studio saveDraft；仅升级 CLI 不会改变旧服务端行为。",
		Tool: deapAgentMCPCreateTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（MCP 资源 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "config-file", Usage: "配置 JSON 对象文件（最大 1 MiB；根节点 name/configString 必填；configString 内 mcpServers.<名称>.type 必填 streamable-http 或 sse；敏感值放 configString/envs）", Bind: "configFile", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
		Call:   deapAgentCallMCPCreateFromFile,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPCreateTool, CanonicalPath: "dingtalk-tag.create_mcp", CLIPath: "dingtalk-tag capability mcp create", PrimaryCLIPath: "dingtalk-tag capability mcp create", Group: "capability.mcp"},
			Description: "通过本地 JSON 文件传入 name/configString；configString 内 mcpServers.<名称>.type 必须显式填写 streamable-http 或 sse。CLI 内部先 check_mcp 校验，再在 agentUuid 员工域创建 MCP 并自动挂载到草稿 MCP 列表；保留已有选择，不克隆、不自动发布。失败时按 stage 和已创建 mcpId 查询资源与草稿、按 trace 排查挂载，禁止重复 create。",
			DryRun:      deapAgentDryRun, Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "先调用 check_mcp 校验，再调用 create_mcp 创建并自动挂载草稿"},
			Selection: contract.SelectionSpec{AgentSummary: "为指定数字员工创建 MCP 并自动挂载草稿，不自动发布", UseWhen: []string{"已知 agentUuid，需要新增 MCP 定义和鉴权配置、取得 mcpId 并加入员工草稿时"}, AvoidWhen: []string{"只需查询现有 MCP 时使用 capability mcp list 或 capability mcp query", "已创建资源但草稿挂载未确认时先查询资源与草稿并按 trace 排查，不要重复 create", "不要把凭据直接拼进命令行"}, Examples: []string{"dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --dry-run --format json"}},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid", InterfaceType: "string"},
				{Name: "config-file", Description: "本地 JSON 文件；configString 内 mcpServers.<名称>.type 必须显式填写 streamable-http 或 sse；字段展开到工具根节点，不对应单个 config 属性"},
			},
		},
	})
}

func newDeapAgentMCPListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "list", Short: "查询 MCP 资源列表",
		Long: "查询目标数字员工 agentUuid 资源域的 MCP 列表和服务端脱敏配置，不是企业公共资源列表。资源存在不代表当前仍被选中或已发布；草稿选择和发布结果需分别查询 manage detail --snapshot draft/published。任何凭据都不得出现在响应中。",
		Tool: deapAgentMCPListTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（MCP 资源 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "keywords", Usage: "名称或描述关键词", Bind: "keywords", Trim: true},
			{Name: "page", Usage: "页码", Bind: "page", Kind: LeafInt, Default: "1", ArgDefault: "1"},
			{Name: "page-size", Usage: "每页数量", Bind: "pageSize", Kind: LeafInt, Default: "20", ArgDefault: "20"},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPListTool, CanonicalPath: "dingtalk-tag.list_mcps", CLIPath: "dingtalk-tag capability mcp list", PrimaryCLIPath: "dingtalk-tag capability mcp list", Group: "capability.mcp"},
			Description: "查询独立 MCP 资源列表和服务端脱敏配置。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentMCPListTool),
			Selection: contract.SelectionSpec{AgentSummary: "查询指定数字员工域的 MCP 资源列表", UseWhen: []string{"需要选择可关联到该数字员工草稿的 MCP 时"}, AvoidWhen: []string{"已知 mcpId 需要单项详情时使用 capability mcp query"}, Examples: []string{"dws dingtalk-tag capability mcp list --agent-uuid <agentUuid> --keywords 文档 --page 1 --page-size 20 --format json"}},
		},
	})
}

func newDeapAgentMCPQueryCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "query", Short: "查询 MCP 资源详情",
		Long: "按 agentUuid 和该员工域的 mcpId 查询 MCP 定义、工具解析结果和服务端脱敏配置，不跨员工或企业资源域回退。资源存在不代表当前仍被选中或已发布；草稿选择和发布结果需分别查询 manage detail --snapshot draft/published。响应不得包含密钥、Token 或临时签名地址。",
		Tool: deapAgentMCPQueryTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（MCP 资源 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "mcp-id", Usage: "MCP ID", Bind: "mcpId", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPQueryIdentity, CanonicalPath: "dingtalk-tag.get_mcp_detail", CLIPath: "dingtalk-tag capability mcp query", PrimaryCLIPath: "dingtalk-tag capability mcp query", Group: "capability.mcp"},
			Description: "按 mcpId 查询独立 MCP 资源定义、工具列表和脱敏配置。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentMCPQueryTool),
			Selection: contract.SelectionSpec{AgentSummary: "查询指定数字员工域的 MCP 脱敏详情", UseWhen: []string{"已知 agentUuid 和 mcpId，需要核对定义或工具解析结果时"}, AvoidWhen: []string{"需要取得明文凭据时不要使用，系统不提供明文回显"}, Examples: []string{"dws dingtalk-tag capability mcp query --agent-uuid <agentUuid> --mcp-id <mcpId> --format json"}},
		},
	})
}

func newDeapAgentMCPUpdateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "update", Short: "更新 MCP 启停状态或替换配置",
		Long: "按 mcpId 更新草稿中的 MCP，保持现有挂载关系，不自动发布。--enabled=true|false 切换启用状态（不传则不改）；--config-file 提供新配置时，CLI 先内部调 check_mcp 只读连通性校验，通过才提交 update_mcp。文件根节点包含 name/configString 等 MCP 配置字段；configString 内 mcpServers.<名称>.type 必须显式填写 streamable-http 或 sse，不能只填 URL。凭据不进入命令行。--enabled 与 --config-file 至少提供一项。先 --dry-run 检查参数，再加 --yes。",
		Tool: deapAgentMCPUpdateTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（MCP 资源 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "mcp-id", Usage: "MCP ID", Bind: "mcpId", Required: true, Trim: true},
			{Name: "enabled", Usage: "启用状态 true|false；不传保持原值（草稿态，不自动发布）", Bind: "enabled", Kind: LeafBool},
			{Name: "config-file", Usage: "可选配置 JSON 对象文件（最大 1 MiB；根节点 name/configString 必填；configString 内 mcpServers.<名称>.type 必填 streamable-http 或 sse；敏感值放 configString/envs）；提供时替换配置", Bind: "configFile", Trim: true, OmitEmpty: true},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		Validate: func(cmd *cobra.Command, _ []string) error {
			hasEnabled := cmd.Flags().Changed("enabled")
			rawPath, _ := cmd.Flags().GetString("config-file")
			if !hasEnabled && strings.TrimSpace(rawPath) == "" {
				return apperrors.NewValidation("update 至少需要提供 --enabled 或 --config-file 之一")
			}
			return nil
		},
		Call: deapAgentCallMCPUpdate,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPUpdateTool, CanonicalPath: "dingtalk-tag.update_mcp", CLIPath: "dingtalk-tag capability mcp update", PrimaryCLIPath: "dingtalk-tag capability mcp update", Group: "capability.mcp"},
			Description: "按 mcpId 更新草稿 MCP 的启停状态或配置；提供新配置时，configString 内 mcpServers.<名称>.type 必须显式填写 streamable-http 或 sse；CLI 内部先 check_mcp 校验通过才提交，保持现有挂载，不自动发布。",
			DryRun:      deapAgentDryRun, Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "提供配置时先调用 check_mcp 校验，再调用 update_mcp；仅改 enabled 时直接更新"},
			Selection: contract.SelectionSpec{AgentSummary: "更新草稿 MCP 启停状态或配置", UseWhen: []string{"需要在不重建的前提下启用/禁用或修改已有 MCP 配置时"}, AvoidWhen: []string{"需要新增 MCP 时使用 capability mcp create", "不要把凭据直接拼进命令行"}, Examples: []string{"dws dingtalk-tag capability mcp update --agent-uuid <agentUuid> --mcp-id <mcpId> --config-file ./mcp.json --dry-run --format json"}},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid", InterfaceType: "string"},
				{Name: "mcp-id", Property: "mcpId", InterfaceType: "string"},
				{Name: "enabled", Property: "enabled", InterfaceType: "boolean"},
				{Name: "config-file", Description: "可选本地 JSON 文件；configString 内 mcpServers.<名称>.type 必须显式填写 streamable-http 或 sse；字段展开到工具根节点，不对应单个 config 属性"},
			},
		},
	})
}

func newDeapAgentMCPDeleteCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "delete", Short: "删除 MCP 并清理草稿挂载",
		Long: "按 mcpId 删除草稿中的 MCP 资源并清理其草稿挂载，不自动发布。这是不可逆写操作，先 --dry-run 检查参数，再加 --yes。",
		Tool: deapAgentMCPDeleteTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "目标数字员工 UUID（MCP 资源 tenant）", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "mcp-id", Usage: "MCP ID", Bind: "mcpId", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPDeleteTool, CanonicalPath: "dingtalk-tag.delete_mcp", CLIPath: "dingtalk-tag capability mcp delete", PrimaryCLIPath: "dingtalk-tag capability mcp delete", Group: "capability.mcp"},
			Description: "按 mcpId 删除草稿 MCP 资源并清理草稿挂载，不自动发布。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentMCPDeleteTool),
			Selection: contract.SelectionSpec{AgentSummary: "删除草稿 MCP 并清理挂载", UseWhen: []string{"确认不再需要某个 MCP，需要从资源和草稿挂载中移除时"}, AvoidWhen: []string{"只需临时禁用时用 capability mcp update --enabled=false"}, Examples: []string{"dws dingtalk-tag capability mcp delete --agent-uuid <agentUuid> --mcp-id <mcpId> --dry-run --format json"}},
		},
	})
}

func deapAgentCallMCPCreateFromFile(cmd *cobra.Command, tool string, args map[string]any) error {
	rawPath, _ := args["configFile"].(string)
	config, err := deapAgentReadJSONObjectFile(rawPath, "config-file")
	if err != nil {
		return err
	}
	if err := deapAgentValidateMCPCreateConfig(config); err != nil {
		return err
	}
	delete(args, "configFile")
	if deps.Caller.DryRun() {
		// Preview only field names with redacted placeholders; never pass secrets
		// to the runner, which also logs dry-run arguments to stderr.
		for key := range config {
			args[key] = "[redacted]"
		}
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	if err := deapAgentCheckMCP(cmd.Context(), stringArgument(args, "agentUuid"), config); err != nil {
		return err
	}
	for key, value := range config {
		args[key] = value
	}
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

func deapAgentValidateMCPCreateConfig(config map[string]any) error {
	for _, key := range []string{"name", "configString"} {
		value, ok := config[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return apperrors.NewValidation("config-file 根节点 " + key + " 必须是非空字符串；不要包装在 config 中")
		}
	}
	for key := range config {
		switch key {
		case "name", "description", "detailIntro", "userQuestionTips", "configType", "configString", "envs", "toolsDisabled":
		default:
			// Do not echo user-provided keys: malformed keys can themselves contain secrets.
			return apperrors.NewValidation("config-file 包含不支持的字段；仅接受 MCP 配置字段，agentUuid 必须由 --agent-uuid 传入")
		}
	}
	return nil
}

func deapAgentCallMCPUpdate(cmd *cobra.Command, tool string, args map[string]any) error {
	rawPath, _ := args["configFile"].(string)
	delete(args, "configFile")
	if strings.TrimSpace(rawPath) == "" {
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	config, err := deapAgentReadJSONObjectFile(rawPath, "config-file")
	if err != nil {
		return err
	}
	if err := deapAgentValidateMCPCreateConfig(config); err != nil {
		return err
	}
	if deps.Caller.DryRun() {
		// Preview only field names with redacted placeholders; never pass secrets
		// to the runner, which also logs dry-run arguments to stderr. check_mcp is
		// skipped in dry-run so no live probe or credential leaves the machine.
		for key := range config {
			args[key] = "[redacted]"
		}
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	if err := deapAgentCheckMCP(cmd.Context(), stringArgument(args, "agentUuid"), config); err != nil {
		return err
	}
	for key, value := range config {
		args[key] = value
	}
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

// deapAgentCheckMCP runs the read-only check_mcp probe before create/update commits.
// It forwards only the connectivity-relevant fields and blocks the write when
// the server reports the config invalid or unreachable.
func deapAgentCheckMCP(ctx context.Context, agentUUID string, config map[string]any) error {
	checkArgs := map[string]any{"agentUuid": agentUUID}
	for _, key := range []string{"name", "configType", "configString", "envs"} {
		if value, ok := config[key]; ok {
			checkArgs[key] = value
		}
	}
	responseText, err := callMCPToolReturnTextOnServer(ctx, deapAgentServerID, deapAgentMCPCheckTool, checkArgs)
	if err != nil {
		return fmt.Errorf("check_mcp 连通性校验失败: %w", err)
	}
	var envelope struct {
		Success *bool `json:"success"`
		IsError *bool `json:"isError"`
	}
	if err := json.Unmarshal([]byte(responseText), &envelope); err != nil {
		return fmt.Errorf("check_mcp 响应格式非法，已阻止写入")
	}
	if envelope.Success == nil || !*envelope.Success || (envelope.IsError != nil && *envelope.IsError) {
		return fmt.Errorf("check_mcp 连通性校验未通过，已阻止写入")
	}
	return nil
}

func deapAgentReadJSONObjectFile(rawPath, flagName string) (map[string]any, error) {
	value, err := deapAgentReadJSONFile(rawPath, flagName)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 必须指向 JSON 对象文件", flagName))
	}
	return object, nil
}

func deapAgentReadJSONFile(rawPath, flagName string) (any, error) {
	path, err := apperrors.SafeInputPath(strings.TrimSpace(rawPath))
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 路径不安全: %v", flagName, err))
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 文件不可读", flagName))
	}
	if !info.Mode().IsRegular() {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 必须是普通文件", flagName))
	}
	if info.Size() > deapAgentConfigFileMaxSize {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 文件不能超过 1 MiB", flagName))
	}
	data, err := deapAgentReadFile(path)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 文件不可读", flagName))
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 必须是合法 JSON", flagName))
	}
	return value, nil
}
