// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const (
	recruitServerID      = "recruit"
	recruitCreateJobTool = "create_job"
	recruitGetJobTool    = "get_job_detail"
	recruitListJobsTool  = "list_jobs"
)

var recruitDryRun = &contract.DryRunSpec{
	PreviewKind: contract.DryRunPreviewRequest,
	RemoteReads: false,
}

var (
	recruitListResult = &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"当前页招聘职位查询结果；续页信息位于 meta.pagination","properties":{"jobs":{"type":"array","description":"当前页职位记录","items":{"type":"object","description":"招聘职位摘要","properties":{"jobId":{"type":"string","description":"职位 ID"},"name":{"type":"string","description":"职位名称"},"status":{"type":"number","description":"职位状态枚举值"}},"additionalProperties":true}}},"additionalProperties":true}`),
	}
	recruitJobDetailResult = &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"招聘职位详情","properties":{"jobId":{"type":"string","description":"职位 ID"},"name":{"type":"string","description":"职位名称"},"description":{"type":"string","description":"职位描述"},"status":{"type":"number","description":"职位状态枚举值"}},"additionalProperties":true}`),
	}
	recruitCreateJobResult = &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"招聘职位创建结果","properties":{"jobId":{"type":"string","description":"新创建的职位 ID"}},"required":["jobId"],"additionalProperties":true}`),
	}
)

func recruitCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"}
}

func newRecruitCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "recruit",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询和创建钉钉招聘职位",
			UseWhen:      []string{"需要查询招聘职位列表、查看职位详情或创建职位时"},
			AvoidWhen:    []string{"查询企业员工与组织架构使用 contact；查询人才池、职业历程或绩效使用 hrbrain"},
		},
	})

	root := newGroupCommand(&cobra.Command{
		Use:   "recruit",
		Short: "钉钉招聘",
		Long:  "查询和创建钉钉招聘中的职位信息。",
		RunE:  groupRunE,
	})
	job := newGroupCommand(&cobra.Command{
		Use:   "job",
		Short: "招聘职位管理",
		RunE:  groupRunE,
	})
	job.AddCommand(
		newRecruitJobListCommand(),
		newRecruitJobGetCommand(),
		newRecruitJobCreateCommand(),
	)
	root.AddCommand(job)
	return root
}

func newRecruitJobListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "list",
		Short:         "查询招聘职位列表",
		Long:          "按职位 ID、关键词、状态、创建人、职位性质等条件分页查询招聘职位。",
		Example:       "  dws recruit job list --keyword Java --status open --size 20 --format json\n  dws recruit job list --job-ids JOB_ID_1,JOB_ID_2 --format json",
		Server:        recruitServerID,
		Tool:          recruitListJobsTool,
		Safety:        recruitSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "job-ids", Usage: "职位 ID，多个值用逗号分隔", Kind: LeafStringSlice, Bind: "jobIds"},
			{Name: "required-edu", Usage: "学历要求枚举值", Kind: LeafInt, Bind: "requiredEdu"},
			{Name: "status", Usage: "职位状态：draft/open/invalid/closed，多个值用逗号分隔", Bind: "statusList", Trim: true, OmitEmpty: true, Transform: transformRecruitStatuses},
			{Name: "job-nature", Usage: "职位性质", Bind: "jobNature", Trim: true, OmitEmpty: true},
			{Name: "campus", Usage: "是否为校园招聘", Kind: LeafBool, Bind: "campus"},
			{Name: "start-modified-time", Usage: "修改时间范围起点", Bind: "startModifiedTime", Trim: true, OmitEmpty: true},
			{Name: "end-modified-time", Usage: "修改时间范围终点", Bind: "endModifiedTime", Trim: true, OmitEmpty: true},
			{Name: "creator-user-ids", Usage: "创建人 userId，多个值用逗号分隔", Kind: LeafStringSlice, Bind: "creatorUserIds"},
			{Name: "keyword", Usage: "职位搜索关键词", Bind: "keyword", Trim: true, OmitEmpty: true},
			{Name: "category", Usage: "职位分类", Bind: "category", Trim: true, OmitEmpty: true},
			{Name: "cursor", Usage: "分页游标；首次查询不传，翻页时原样回填返回的 nextCursor", Bind: "cursor", Trim: true, OmitEmpty: true, Transform: transformRecruitCursor},
			{Name: "size", Usage: "分页大小，默认 20", Kind: LeafInt, Bind: "size", Default: "20", ArgDefault: "20"},
		},
		Validate:   validateRecruitList,
		ResultCall: recruitResultCall,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "recruit", Name: recruitListJobsTool, CanonicalPath: "recruit.list_jobs", CLIPath: "recruit job list", PrimaryCLIPath: "recruit job list"},
			Description: "按条件分页查询招聘职位",
			DryRun:      recruitDryRun,
			Result:      recruitListResult,
			Pagination:  recruitCursorPagination(),
			Interface:   recruitMCPInterface(recruitListJobsTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "按关键词、状态、创建人等条件分页查询招聘职位",
				UseWhen:      []string{"需要查找职位、筛选在招职位或取得 jobId 时"},
				AvoidWhen:    []string{"已经持有明确 jobId 且需要完整详情时使用 recruit job get"},
				Examples:     []string{`dws recruit job list --keyword "Java" --status open --size 20 --format json`},
			},
			Parameters: recruitListParamDecls(),
		},
	})
}

func recruitListToolArgs(args map[string]any) map[string]any {
	params := map[string]any{"param": map[string]any{}, "size": args["size"]}
	query := params["param"].(map[string]any)
	for key, value := range args {
		switch key {
		case "cursor":
			params[key] = value
		case "size":
		default:
			query[key] = value
		}
	}
	return params
}

func recruitResultCall(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	toolArgs := args
	if tool == recruitListJobsTool {
		toolArgs = recruitListToolArgs(args)
	}
	if deps.Caller.DryRun() {
		return output.Success(map[string]any{
			"tool": tool, "arguments": toolArgs, "executed": false,
		}, output.WithDryRun()), nil
	}
	data, err := callRecruitMCPToolData(cmd.Context(), tool, toolArgs)
	if err != nil {
		return nil, err
	}
	clean, err := recruitBusinessResultData(data, tool)
	if err != nil {
		return recruitResponseFailure(err), nil
	}
	switch tool {
	case recruitListJobsTool:
		listData, meta, err := recruitListResultData(clean)
		if err != nil {
			return recruitInvalidResponse(err), nil
		}
		return output.Success(listData, output.WithMeta(meta)), nil
	case recruitGetJobTool, recruitCreateJobTool:
		if err := validateRecruitJobResult(clean, tool, args); err != nil {
			return recruitInvalidResponse(err), nil
		}
		return output.Success(clean), nil
	default:
		return recruitInvalidResponse(fmt.Errorf("不支持校验 %s 的业务结果", tool)), nil
	}
}

func validateRecruitJobResult(data map[string]any, tool string, args map[string]any) error {
	jobID, ok := data["jobId"].(string)
	if !ok || strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("%s 返回值缺少非空字符串字段 jobId", tool)
	}
	if tool != recruitGetJobTool {
		return nil
	}
	requestedJobID, ok := args["jobId"].(string)
	if !ok || strings.TrimSpace(requestedJobID) == "" {
		return fmt.Errorf("%s 请求缺少非空字符串字段 jobId", tool)
	}
	if strings.TrimSpace(jobID) != strings.TrimSpace(requestedJobID) {
		return fmt.Errorf("%s 返回的 jobId %q 与请求的 jobId %q 不一致", tool, jobID, requestedJobID)
	}
	return nil
}

func callRecruitMCPToolData(ctx context.Context, tool string, args map[string]any) (any, error) {
	text, err := callMCPToolReturnTextOnServer(ctx, recruitServerID, tool, args)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, nil
	}
	var data any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", tool, err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("存在多个 JSON 值")
		}
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", tool, err))
	}
	return data, nil
}

func recruitResponseFailure(err error) output.CommandResult {
	var businessFailure *recruitBusinessFailure
	if errors.As(err, &businessFailure) {
		return output.Failure(&output.ErrorInfo{
			Type: "api", Message: businessFailure.Error(),
		})
	}
	return recruitInvalidResponse(err)
}

type recruitBusinessFailure struct {
	message string
}

func (e *recruitBusinessFailure) Error() string {
	return e.message
}

func recruitBusinessResultData(data any, tool string) (map[string]any, error) {
	object, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 返回值必须是 JSON 对象", tool)
	}
	successValue, hasSuccess := object["success"]
	resultValue, hasResult := object["result"]
	if !hasSuccess && !hasResult {
		return object, nil
	}
	if !hasSuccess || !hasResult {
		return nil, fmt.Errorf("%s 返回的 Connector 信封必须同时包含 success 和 result", tool)
	}
	success, ok := successValue.(bool)
	if !ok {
		return nil, fmt.Errorf("%s 返回的 Connector 信封字段 success 必须是布尔值", tool)
	}
	if !success {
		message, _ := object["message"].(string)
		if strings.TrimSpace(message) == "" {
			message = "Connector 返回 success=false"
		}
		return nil, &recruitBusinessFailure{message: fmt.Sprintf("%s 调用失败: %s", tool, message)}
	}
	result, ok := resultValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 返回值的 result 必须是 JSON 对象", tool)
	}
	return result, nil
}

func recruitInvalidResponse(err error) output.CommandResult {
	return output.Failure(&output.ErrorInfo{
		Type: "api", Subtype: "invalid_response", Message: err.Error(),
		Hint: "保留原始响应并停止后续操作；不要依据不完整结果继续处理。",
	})
}

func recruitListResultData(data any) (any, *output.Meta, error) {
	object, ok := data.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s 返回值必须是 JSON 对象", recruitListJobsTool)
	}
	// The Connector envelope has already been removed by recruitResultCall.
	// Only normalize the list business payload here so fields named success or
	// result inside that payload cannot be mistaken for another envelope.
	hasMore, ok := object["hasMore"].(bool)
	if !ok {
		return nil, nil, fmt.Errorf("list_jobs 返回值缺少布尔字段 hasMore")
	}
	nextToken := ""
	if hasMore {
		if raw, exists := object["nextCursor"]; exists && raw != nil {
			var err error
			nextToken, err = normalizeRecruitNextCursor(raw)
			if err != nil {
				return nil, nil, err
			}
		}
		if nextToken == "" {
			return nil, nil, fmt.Errorf("list_jobs 返回 hasMore=true 但缺少 nextCursor")
		}
	}
	clean := make(map[string]any, len(object))
	for key, value := range object {
		if key != "hasMore" && key != "nextCursor" {
			clean[key] = value
		}
	}
	if jobs, exists := clean["list"]; exists {
		if _, alreadyNormalized := clean["jobs"]; !alreadyNormalized {
			clean["jobs"] = jobs
		}
		delete(clean, "list")
	}
	pagination := &output.Pagination{EndpointExhausted: !hasMore, NextToken: nextToken, Pages: 1}
	meta := &output.Meta{Pagination: pagination}
	if jobs, exists := clean["jobs"].([]any); exists {
		meta.Count = output.NewCount(len(jobs))
		pagination.Items = len(jobs)
	}
	return clean, meta, nil
}

func normalizeRecruitNextCursor(raw any) (string, error) {
	var value string
	switch cursor := raw.(type) {
	case string:
		value = strings.TrimSpace(cursor)
	case json.Number:
		value = string(cursor)
	case float64:
		value = strconv.FormatFloat(cursor, 'f', -1, 64)
	default:
		return "", fmt.Errorf("list_jobs 的 nextCursor 必须是字符串或数字")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return "", fmt.Errorf("list_jobs 的 nextCursor 必须是可回填的非负十进制 int64 游标")
	}
	return strconv.FormatInt(parsed, 10), nil
}

func newRecruitJobGetCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "get",
		Short:         "查询招聘职位详情",
		Long:          "根据职位 ID 查询招聘职位详情。",
		Example:       "  dws recruit job get --job-id JOB_ID --format json",
		Server:        recruitServerID,
		Tool:          recruitGetJobTool,
		Safety:        recruitSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    recruitResultCall,
		Flags: []LeafFlag{{
			Name: "job-id", Usage: "职位 ID（必填）", Bind: "jobId", Trim: true,
			Required: true, RequiredHint: "--job-id 为必填",
		}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "recruit", Name: recruitGetJobTool, CanonicalPath: "recruit.get_job_detail", CLIPath: "recruit job get", PrimaryCLIPath: "recruit job get"},
			Description: "根据职位 ID 查询招聘职位详情",
			DryRun:      recruitDryRun,
			Result:      recruitJobDetailResult,
			Interface:   recruitMCPInterface(recruitGetJobTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定招聘职位的完整信息",
				UseWhen:      []string{"已经持有明确 jobId，需要查看职位详情时"},
				AvoidWhen:    []string{"不知道 jobId 时先使用 recruit job list 查询"},
				Examples:     []string{"dws recruit job get --job-id <JOB_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{{Name: "job-id", Property: "jobId", Required: boolPtr(true), InterfaceType: "string"}},
		},
	})
}

func newRecruitJobCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "create",
		Short:         "创建招聘职位",
		Long:          "从 JSON 文件读取职位信息并创建招聘职位。该操作会写入远端招聘系统，执行前必须确认。",
		Example:       "  dws recruit job create --from ./job.json --dry-run --format json\n  dws recruit job create --from ./job.json --format json",
		Server:        recruitServerID,
		Tool:          recruitCreateJobTool,
		Safety:        recruitSafetyCreate(),
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    recruitResultCall,
		Flags: []LeafFlag{{
			Name: "from", Usage: "职位 JSON 文件路径（必填）", Bind: "atsAddJobParam", Trim: true,
			Required: true, RequiredHint: "--from 为必填", Transform: loadRecruitJobFile,
		}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "recruit", Name: recruitCreateJobTool, CanonicalPath: "recruit.create_job", CLIPath: "recruit job create", PrimaryCLIPath: "recruit job create"},
			Description: "从 JSON 文件创建招聘职位",
			DryRun:      recruitDryRun,
			Result:      recruitCreateJobResult,
			Interface:   recruitMCPInterface(recruitCreateJobTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "使用结构化 JSON 创建招聘职位",
				UseWhen:      []string{"用户明确要求新建职位，并已准备或同意生成职位 JSON 时；文件必须包含 name、description、jobNature、requiredEdu、extData、creatorUserId；jobNature 固定为 FULL-TIME；creatorUserId 必须使用真实创建人 userId；ownerUserIds 可选"},
				AvoidWhen:    []string{"仅查询职位时使用 recruit job list 或 recruit job get"},
				Examples:     []string{"dws recruit job create --from ./job.json --dry-run --format json"},
			},
			Parameters: []contract.ParamDecl{{Name: "from", Property: "atsAddJobParam", Required: boolPtr(true), InterfaceType: "object", Description: "职位 JSON 文件；CLI 校验后原样作为 atsAddJobParam 对象发送；creatorUserId 为必填的创建人 userId，ownerUserIds 为可选的负责人 userId 字符串数组"}},
		},
	})
}

func recruitListParamDecls() []contract.ParamDecl {
	return []contract.ParamDecl{
		{Name: "job-ids", Property: "param.jobIds", InterfaceType: "array"},
		{Name: "required-edu", Property: "param.requiredEdu", InterfaceType: "number"},
		{Name: "status", Property: "param.statusList", InterfaceType: "array", Enum: []string{"draft", "open", "invalid", "closed"}},
		{Name: "job-nature", Property: "param.jobNature", InterfaceType: "string"},
		{Name: "campus", Property: "param.campus", InterfaceType: "boolean"},
		{Name: "start-modified-time", Property: "param.startModifiedTime", InterfaceType: "string"},
		{Name: "end-modified-time", Property: "param.endModifiedTime", InterfaceType: "string"},
		{Name: "creator-user-ids", Property: "param.creatorUserIds", InterfaceType: "array"},
		{Name: "keyword", Property: "param.keyword", InterfaceType: "string"},
		{Name: "category", Property: "param.category", InterfaceType: "string"},
		{Name: "cursor", Property: "cursor", InterfaceType: "number"},
		{Name: "size", Property: "size", InterfaceType: "number"},
	}
}

func recruitMCPInterface(tool string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeMCP,
		Availability: contract.InterfaceAvailable,
		Ref:          &contract.InterfaceRefSpec{ProductID: recruitServerID, RPCName: tool},
	}
}

func recruitSafetyRead() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func recruitSafetyCreate() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}
}

func transformRecruitStatuses(raw string) (any, error) {
	values := strings.Split(raw, ",")
	statuses := make([]int, 0, len(values))
	seen := make(map[int]bool, len(values))
	lookup := map[string]int{"draft": 0, "open": 1, "invalid": 2, "closed": 3}
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		status, ok := lookup[name]
		if !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("--status 不支持 %q，可选 draft/open/invalid/closed", value))
		}
		if !seen[status] {
			statuses = append(statuses, status)
			seen[status] = true
		}
	}
	return statuses, nil
}

func transformRecruitCursor(raw string) (any, error) {
	value := strings.TrimSpace(raw)
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor < 0 {
		return nil, apperrors.NewValidation("--cursor 必须是大于或等于 0 的整数")
	}
	return cursor, nil
}

func validateRecruitList(cmd *cobra.Command, _ []string) error {
	size, _ := cmd.Flags().GetInt("size")
	if cmd.Flags().Changed("size") && (size < 1 || size > 100) {
		return apperrors.NewValidation("--size 必须在 1 到 100 之间")
	}
	requiredEdu, _ := cmd.Flags().GetInt("required-edu")
	if cmd.Flags().Changed("required-edu") && (requiredEdu < 1 || requiredEdu > 9) {
		return apperrors.NewValidation("--required-edu 必须在 1 到 9 之间")
	}
	return nil
}

func loadRecruitJobFile(path string) (any, error) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("读取职位 JSON 失败: %w", err)
	}
	var job map[string]any
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("职位文件不是有效的 JSON 对象: %v", err))
	}
	if job == nil {
		return nil, apperrors.NewValidation("职位 JSON 顶层必须是对象")
	}
	if err := validateRecruitJob(job); err != nil {
		return nil, err
	}
	return job, nil
}

func validateRecruitJob(job map[string]any) error {
	for _, name := range []string{"name", "description", "jobNature", "requiredEdu", "extData", "creatorUserId"} {
		value, ok := job[name]
		if !ok || value == nil || (isString(value) && strings.TrimSpace(value.(string)) == "") {
			return apperrors.NewValidation(fmt.Sprintf("职位 JSON 缺少必填字段 %s", name))
		}
	}
	for _, name := range []string{"name", "description", "jobNature"} {
		if !isString(job[name]) {
			return apperrors.NewValidation(fmt.Sprintf("职位 JSON 字段 %s 必须是字符串", name))
		}
	}
	if job["jobNature"].(string) != "FULL-TIME" {
		return apperrors.NewValidation("职位 JSON 字段 jobNature 当前仅支持 FULL-TIME")
	}
	requiredEdu, ok := job["requiredEdu"].(float64)
	if !ok {
		return apperrors.NewValidation("职位 JSON 字段 requiredEdu 必须是数字")
	}
	if requiredEdu < 1 || requiredEdu > 9 || requiredEdu != float64(int(requiredEdu)) {
		return apperrors.NewValidation("职位 JSON 字段 requiredEdu 必须是 1 到 9 的整数")
	}

	minSalary, hasMinSalary, err := optionalRecruitNumber(job, "minSalary")
	if err != nil {
		return err
	}
	maxSalary, hasMaxSalary, err := optionalRecruitNumber(job, "maxSalary")
	if err != nil {
		return err
	}
	if hasMinSalary && hasMaxSalary && minSalary > maxSalary {
		return apperrors.NewValidation("职位 JSON 中 minSalary 不能大于 maxSalary")
	}

	for _, name := range []string{"province", "city", "district", "category"} {
		if value, exists := job[name]; exists && value != nil && !isString(value) {
			return apperrors.NewValidation(fmt.Sprintf("职位 JSON 字段 %s 必须是字符串", name))
		}
	}
	if value, exists := job["campus"]; exists && value != nil {
		if _, ok := value.(bool); !ok {
			return apperrors.NewValidation("职位 JSON 字段 campus 必须是布尔值")
		}
	}
	if err := validateRecruitIdentityFields(job); err != nil {
		return err
	}
	if err := validateRecruitAddress(job); err != nil {
		return err
	}
	extData, ok := job["extData"].(map[string]any)
	if !ok {
		return apperrors.NewValidation("职位 JSON 字段 extData 必须是对象")
	}
	if err := validateRecruitExtData(extData); err != nil {
		return err
	}
	return nil
}

func optionalRecruitNumber(values map[string]any, name string) (float64, bool, error) {
	value, exists := values[name]
	if !exists || value == nil {
		return 0, false, nil
	}
	number, ok := value.(float64)
	if !ok {
		return 0, false, apperrors.NewValidation(fmt.Sprintf("职位 JSON 字段 %s 必须是数字", name))
	}
	return number, true, nil
}

func validateRecruitIdentityFields(job map[string]any) error {
	if _, ok := job["creatorUserId"].(string); !ok {
		return apperrors.NewValidation("职位 JSON 字段 creatorUserId 必须是非空字符串")
	}
	if value, exists := job["ownerUserIds"]; exists && value != nil {
		ownerUserIDs, ok := value.([]any)
		if !ok {
			return apperrors.NewValidation("职位 JSON 字段 ownerUserIds 必须是字符串数组")
		}
		for _, owner := range ownerUserIDs {
			ownerUserID, ok := owner.(string)
			if !ok || strings.TrimSpace(ownerUserID) == "" {
				return apperrors.NewValidation("职位 JSON 字段 ownerUserIds 只能包含非空字符串")
			}
		}
	}
	return nil
}

func validateRecruitAddress(job map[string]any) error {
	value, exists := job["address"]
	if !exists || value == nil {
		return nil
	}
	address, ok := value.(map[string]any)
	if !ok {
		return apperrors.NewValidation("职位 JSON 字段 address 必须是对象")
	}
	for _, name := range []string{"name", "detail", "longitude", "latitude"} {
		field, exists := address[name]
		text, ok := field.(string)
		if !exists || !ok || strings.TrimSpace(text) == "" {
			return apperrors.NewValidation(fmt.Sprintf("职位 JSON 字段 address.%s 必须是非空字符串", name))
		}
	}
	return nil
}

func validateRecruitExtData(extData map[string]any) error {
	if value, exists := extData["headCount"]; exists && value != nil {
		headCount, ok := value.(float64)
		if !ok || headCount < 1 || headCount > 999 || headCount != float64(int(headCount)) {
			return apperrors.NewValidation("职位 JSON 字段 extData.headCount 必须是 1 到 999 的整数")
		}
	}
	if value, exists := extData["source"]; exists && value != nil && !isString(value) {
		return apperrors.NewValidation("职位 JSON 字段 extData.source 必须是字符串")
	}
	if value, exists := extData["fullTimeExtData"]; exists && value != nil {
		fullTime, ok := value.(map[string]any)
		if !ok {
			return apperrors.NewValidation("职位 JSON 字段 extData.fullTimeExtData 必须是对象")
		}
		if err := validateRecruitFullTimeExtData(fullTime); err != nil {
			return err
		}
	}
	if value, exists := extData["tags"]; exists && value != nil {
		tags, ok := value.([]any)
		if !ok {
			return apperrors.NewValidation("职位 JSON 字段 extData.tags 必须是数组")
		}
		for _, value := range tags {
			tag, ok := value.(map[string]any)
			if !ok {
				return apperrors.NewValidation("职位 JSON 字段 extData.tags 只能包含对象")
			}
			name, ok := tag["name"].(string)
			if !ok || strings.TrimSpace(name) == "" {
				return apperrors.NewValidation("职位 JSON 字段 extData.tags[].name 必须是非空字符串")
			}
		}
	}
	return nil
}

func validateRecruitFullTimeExtData(fullTime map[string]any) error {
	salaryMonth, hasSalaryMonth, err := optionalRecruitNumber(fullTime, "salaryMonth")
	if err != nil {
		return err
	}
	if hasSalaryMonth && (salaryMonth < 12 || salaryMonth > 24 || salaryMonth != float64(int(salaryMonth))) {
		return apperrors.NewValidation("职位 JSON 字段 extData.fullTimeExtData.salaryMonth 必须是 12 到 24 的整数")
	}
	minExperience, hasMinExperience, err := optionalRecruitNumber(fullTime, "minJobExperience")
	if err != nil {
		return err
	}
	maxExperience, hasMaxExperience, err := optionalRecruitNumber(fullTime, "maxJobExperience")
	if err != nil {
		return err
	}
	if hasMinExperience && minExperience < 0 {
		return apperrors.NewValidation("职位 JSON 字段 extData.fullTimeExtData.minJobExperience 不能小于 0")
	}
	if hasMaxExperience && maxExperience < 0 {
		return apperrors.NewValidation("职位 JSON 字段 extData.fullTimeExtData.maxJobExperience 不能小于 0")
	}
	if hasMinExperience && hasMaxExperience && minExperience > maxExperience {
		return apperrors.NewValidation("职位 JSON 中 minJobExperience 不能大于 maxJobExperience")
	}
	return nil
}

func isString(value any) bool {
	_, ok := value.(string)
	return ok
}
