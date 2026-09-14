// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

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
	deapAgentMCPCreateTool             = "create_mcp"
	deapAgentMCPListTool               = "list_mcps"
	deapAgentMCPQueryTool              = "query_mcp"
	deapAgentMCPQueryIdentity          = "get_mcp_detail"
	deapAgentSkillUploadPath           = "/v1.0/assistant/skills/upload"
	deapAgentSkillUploadProdBase       = "https://api-deap.dingtalk.com"
	deapAgentSkillUploadPreBase        = "https://pre-api-deap.dingtalk.com"

	deapAgentConfigFileMaxSize    = 1024 * 1024
	deapAgentSkillMaxPackageSize  = 50 * 1024 * 1024
	deapAgentSkillMaxExpandedSize = 200 * 1024 * 1024
	deapAgentSkillMaxEntries      = 10000
)

var deapAgentSnapshots = []string{"draft", "published"}

type deapAgentSkillPackage struct {
	path string
	size int64
}

type deapAgentSkillCreated struct {
	SkillID     string `json:"skillId"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Version     int64  `json:"version,omitempty"`
}

type deapAgentSkillPackageUploader interface {
	Upload(ctx context.Context, agentUUID, filePath string) (string, error)
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

func (u deapAgentOpenAPISkillUploader) Upload(ctx context.Context, agentUUID, filePath string) (string, error) {
	baseURL, err := u.uploadBaseURL()
	if err != nil {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("OpenAPI 环境解析失败")}
	}
	credential, err := u.temporaryCredential(ctx, agentUUID)
	if err != nil {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("OpenAPI 认证失败")}
	}
	file, err := os.Open(filePath)
	if err != nil {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("Skill ZIP 文件不可读")}
	}
	defer file.Close()

	client := apiclient.NewClient(credential, baseURL)
	if u.httpClient != nil {
		client.HTTPClient = u.httpClient
	}
	if u.validateTarget != nil {
		client.TargetValidator = u.validateTarget
	}
	response, err := client.UploadMultipart(ctx, apiclient.MultipartUploadRequest{
		Path:       deapAgentSkillUploadPath,
		FieldName:  "file",
		FileName:   filepath.Base(filePath),
		File:       file,
		BearerAuth: true,
	})
	if err != nil {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("OpenAPI 上传请求失败")}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", &deapAgentSkillStageError{
			Stage: "upload",
			Err:   fmt.Errorf("OpenAPI 返回 HTTP %d", response.StatusCode),
		}
	}

	var envelope deapAgentOpenAPIUploadEnvelope
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("OpenAPI 上传响应格式非法")}
	}
	if envelope.Success != nil && !*envelope.Success {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("OpenAPI 上传失败")}
	}
	payload := deapAgentOpenAPIUploadPayload{FileURL: envelope.FileURL}
	for _, candidate := range []*deapAgentOpenAPIUploadPayload{envelope.Data, envelope.Result, envelope.Content} {
		if candidate != nil {
			payload = *candidate
			break
		}
	}
	if strings.TrimSpace(payload.FileURL) == "" {
		return "", &deapAgentSkillStageError{Stage: "upload", Err: fmt.Errorf("OpenAPI 上传响应缺少 fileUrl")}
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
		return u.resolveCredential(ctx, agentUUID)
	}
	if deps == nil || deps.Caller == nil {
		return "", fmt.Errorf("OpenAPI credential resolver is not configured")
	}
	responseText, err := callMCPToolReturnTextOnServer(ctx, deapAgentServerID,
		deapAgentSkillUploadCredentialTool, map[string]any{"agentUuid": agentUUID})
	if err != nil {
		return "", fmt.Errorf("requesting temporary upload credential")
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
	Stage string
	Err   error
}

func (e *deapAgentSkillStageError) Error() string {
	if e == nil {
		return "skill create 阶段失败"
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
		return fmt.Sprintf("skill create %s 阶段失败", e.Stage)
	}
	return fmt.Sprintf("skill create %s 阶段失败: %s", e.Stage, detail)
}

func (e *deapAgentSkillStageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var deapAgentSkillUploader deapAgentSkillPackageUploader = deapAgentOpenAPISkillUploader{}

func deapAgentValidateSkillPackage(rawPath string) (deapAgentSkillPackage, error) {
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(rawPath)), ".zip") {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 必须使用 .zip 扩展名")
	}
	resolved, err := apperrors.SafeInputPath(strings.TrimSpace(rawPath))
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation(fmt.Sprintf("参数 --file 路径不安全: %v", err))
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 文件不可读")
	}
	if !info.Mode().IsRegular() {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 必须是普通文件")
	}
	if info.Size() > deapAgentSkillMaxPackageSize {
		return deapAgentSkillPackage{}, apperrors.NewValidation("Skill ZIP 不能超过 50 MiB")
	}
	reader, err := zip.OpenReader(resolved)
	if err != nil {
		return deapAgentSkillPackage{}, apperrors.NewValidation("参数 --file 不是有效 ZIP 文件")
	}
	defer reader.Close()
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
		body, openErr := entry.Open()
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
	return deapAgentSkillPackage{path: resolved, size: info.Size()}, nil
}

func newDeapCapabilityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "capability",
		Short:             "数字员工能力资源管理",
		Long:              "创建和查询可配置到数字员工草稿的 Skill/MCP 能力资源。资源创建后不会自动关联数字员工；关联关系由 manage save-draft 的 skills/mcps 配置负责。",
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
		Long:              "创建和查询独立 Skill 资源。Skill 与数字员工的关联由 save-draft skills 配置负责。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(newDeapAgentSkillCreateCommand(), newDeapAgentSkillListCommand(), newDeapAgentSkillQueryCommand())
	return cmd
}

func newDeapAgentMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "mcp",
		Short:             "管理数字员工 MCP",
		Long:              "创建和查询独立 MCP 资源。敏感配置通过本地 JSON 文件传入；与数字员工的关联由 save-draft mcps 配置负责。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(newDeapAgentMCPCreateCommand(), newDeapAgentMCPListCommand(), newDeapAgentMCPQueryCommand())
	return cmd
}

func newDeapAgentSkillCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "从本地 ZIP 创建 Skill 资源",
		Long: "校验本地 Skill ZIP 后，先通过 OpenAPI multipart 接口上传，再调用 create_skill_by_url 完成 Skill Center create 与 query。ZIP 不进入 MCP JSON，临时签名 URL 不落盘、不输出。",
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
			Description: "校验本地 ZIP，依次调用 OpenAPI upload 与 create_skill_by_url，并只输出安全创建结果。",
			DryRun:      deapAgentDryRun,
			Interface:   &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "本地 ZIP 校验后串联 OpenAPI multipart upload 与 create_skill_by_url"},
			Selection:   contract.SelectionSpec{AgentSummary: "从本地 ZIP 创建 Skill 资源", UseWhen: []string{"已有合法 Skill ZIP，需要为目标数字员工创建并取得 skillId 时"}, AvoidWhen: []string{"只有远程 URL 的纯 MCP 场景使用 create_skill_by_url"}, Examples: []string{"dws dingtalk-tag capability skill create --agent-uuid <agentUuid> --file ./my-skill.zip --dry-run --format json"}},
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
	pkg, err := deapAgentValidateSkillPackage(rawPath)
	if err != nil {
		return &deapAgentSkillStageError{Stage: "validate", Err: err}
	}
	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dryRun":    true,
			"action":    "upload_then_create_skill",
			"agentUuid": agentUUID,
			"fileName":  filepath.Base(pkg.path),
			"fileSize":  pkg.size,
		})
	}
	fileURL, err := deapAgentSkillUploader.Upload(cmd.Context(), agentUUID, pkg.path)
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
	if strings.TrimSpace(result.SkillID) == "" {
		return &deapAgentSkillStageError{Stage: "query", Err: fmt.Errorf("创建响应缺少 skillId")}
	}
	return deps.Out.PrintJSON(result)
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
		Use: "create", Short: "创建 MCP 资源",
		Long: "从本地 JSON 对象文件创建独立 MCP 资源。配置字段遵循 McpConfigParam：name、description、detailIntro、userQuestionTips、configType、configString、envs、toolsDisabled。凭据不会进入 argv。",
		Tool: deapAgentMCPCreateTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags:  []LeafFlag{{Name: "config-file", Usage: "McpConfigParam JSON 对象文件（最大 1 MiB；敏感值放 configString/envs）", Bind: "configFile", Required: true, Trim: true}},
		Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
		Call:   deapAgentCallMCPCreateFromFile,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPCreateTool, CanonicalPath: "dingtalk-tag.create_mcp", CLIPath: "dingtalk-tag capability mcp create", PrimaryCLIPath: "dingtalk-tag capability mcp create", Group: "capability.mcp"},
			Description: "通过本地 JSON 文件安全传入定义和凭据，创建独立 MCP 资源。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentMCPCreateTool),
			Selection:  contract.SelectionSpec{AgentSummary: "从本地配置文件创建独立 MCP 资源", UseWhen: []string{"需要注册新的 MCP 定义和鉴权配置并取得 mcpId 时"}, AvoidWhen: []string{"只需查询现有 MCP 时使用 capability mcp list 或 capability mcp query", "不要把凭据直接拼进命令行"}, Examples: []string{"dws dingtalk-tag capability mcp create --config-file ./mcp.json --dry-run --format json"}},
			Parameters: []contract.ParamDecl{{Name: "config-file", Property: "config", InterfaceType: "object"}},
		},
	})
}

func newDeapAgentMCPListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "list", Short: "查询 MCP 资源列表",
		Long: "查询当前企业资源域的 MCP 资源列表和服务端脱敏配置。任何凭据都不得出现在响应中。",
		Tool: deapAgentMCPListTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "keywords", Usage: "名称或描述关键词", Bind: "keywords", Trim: true},
			{Name: "page", Usage: "页码", Bind: "page", Kind: LeafInt, Default: "1", ArgDefault: "1"},
			{Name: "page-size", Usage: "每页数量", Bind: "pageSize", Kind: LeafInt, Default: "20", ArgDefault: "20"},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPListTool, CanonicalPath: "dingtalk-tag.list_mcps", CLIPath: "dingtalk-tag capability mcp list", PrimaryCLIPath: "dingtalk-tag capability mcp list", Group: "capability.mcp"},
			Description: "查询独立 MCP 资源列表和服务端脱敏配置。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentMCPListTool),
			Selection: contract.SelectionSpec{AgentSummary: "查询当前企业的 MCP 资源列表", UseWhen: []string{"需要选择可关联到数字员工草稿的 MCP 时"}, AvoidWhen: []string{"已知 mcpId 需要单项详情时使用 capability mcp query"}, Examples: []string{"dws dingtalk-tag capability mcp list --keywords 文档 --page 1 --page-size 20 --format json"}},
		},
	})
}

func newDeapAgentMCPQueryCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "query", Short: "查询 MCP 资源详情",
		Long: "按 mcpId 查询独立 MCP 资源定义、工具解析结果和服务端脱敏配置。响应不得包含密钥、Token 或临时签名地址。",
		Tool: deapAgentMCPQueryTool, Server: deapAgentServerID, PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "mcp-id", Usage: "MCP ID", Bind: "mcpId", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: deapAgentMCPQueryIdentity, CanonicalPath: "dingtalk-tag.get_mcp_detail", CLIPath: "dingtalk-tag capability mcp query", PrimaryCLIPath: "dingtalk-tag capability mcp query", Group: "capability.mcp"},
			Description: "按 mcpId 查询独立 MCP 资源定义、工具列表和脱敏配置。",
			DryRun:      deapAgentDryRun, Interface: deapAgentMCPInterface(deapAgentMCPQueryTool),
			Selection: contract.SelectionSpec{AgentSummary: "查询一个独立 MCP 资源的脱敏详情", UseWhen: []string{"已知 mcpId，需要核对定义或工具解析结果时"}, AvoidWhen: []string{"需要取得明文凭据时不要使用，系统不提供明文回显"}, Examples: []string{"dws dingtalk-tag capability mcp query --mcp-id <mcpId> --format json"}},
		},
	})
}

func deapAgentCallMCPCreateFromFile(_ *cobra.Command, tool string, args map[string]any) error {
	rawPath, _ := args["configFile"].(string)
	config, err := deapAgentReadJSONObjectFile(rawPath, "config-file")
	if err != nil {
		return err
	}
	delete(args, "configFile")
	if deps.Caller.DryRun() {
		args["config"] = map[string]any{"provided": true, "redacted": true}
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	args["config"] = config
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

func deapAgentCallWithProfileAndDraftFiles(cmd *cobra.Command, tool string, args map[string]any) error {
	summaries := map[string]any{}
	for _, item := range []struct {
		argument string
		flagName string
		property string
	}{
		{"skillsFile", "skills-file", "skills"},
		{"mcpsFile", "mcps-file", "mcps"},
	} {
		rawPath, ok := args[item.argument].(string)
		if !ok || strings.TrimSpace(rawPath) == "" {
			continue
		}
		value, err := deapAgentReadJSONArrayFile(rawPath, item.flagName)
		if err != nil {
			return err
		}
		delete(args, item.argument)
		args[item.property] = value
		summaries[item.property] = map[string]any{"provided": true, "count": len(value), "redacted": true}
	}
	if deps.Caller.DryRun() && len(summaries) > 0 {
		for property, summary := range summaries {
			args[property] = summary
		}
	}
	return deapAgentCallWithProfile(cmd, tool, args)
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

func deapAgentReadJSONArrayFile(rawPath, flagName string) ([]any, error) {
	value, err := deapAgentReadJSONFile(rawPath, flagName)
	if err != nil {
		return nil, err
	}
	array, ok := value.([]any)
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 必须指向 JSON 数组文件", flagName))
	}
	return array, nil
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 文件不可读", flagName))
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("参数 --%s 必须是合法 JSON", flagName))
	}
	return value, nil
}
