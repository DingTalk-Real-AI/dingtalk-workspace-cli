// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package hrbrain registers fail-closed Shortcut adapters for DingTalk
// Organization Brain reads. Every declaration remains unavailable until the
// lower service supplies a verifiable non-null result and safe live fixtures.
package hrbrain

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const (
	hrbrainBlockerAdapterBusiness = "adapter_business_service"
	hrbrainBlockerTenantFixture   = "tenant_fixture"
)

func hrbrainUnavailableReason(command string) string {
	switch command {
	case "+get-pool", "+list-pool-employees":
		return "classified=" + hrbrainBlockerTenantFixture + "; exact Shortcut and raw atomic calls reach the same remote operation and upstream error, but no safe nonempty pool fixture exists to prove identity and pagination."
	case "+list-pools", "+profile-labels":
		return "classified=" + hrbrainBlockerAdapterBusiness + "; raw atomic calls return success=true with result=null and the exact Shortcut rejects the response, so the business response contract cannot prove a collection or legitimate empty result."
	case "+profile-metadata", "+query-profile", "+profile-career", "+profile-performance", "+search-employees", "+search-employees-structured", "+search-fields":
		return "classified=" + hrbrainBlockerAdapterBusiness + "; exact Shortcut and raw atomic calls reach the same remote server, operation, and upstream error, so no Shortcut-layer defect can be repaired without downstream capability or business-service recovery."
	default:
		panic("missing reviewed HRbrain blocker classification for " + command)
	}
}

type hrbrainPage struct {
	CurrentPage int
	PageSize    int
	TotalCount  int
	HasMore     bool
}

func hrbrainReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func hrbrainObjectResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"value":{"type":"object","description":"严格校验且身份匹配的 HRbrain 业务对象","additionalProperties":true}},"required":["value"],"additionalProperties":false}`,
			description,
		)),
		SensitivePaths: []string{"value.name", "value.mobile", "value.email", "value.workNo", "value.userId"},
	}
}

func hrbrainCollectionResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"当前响应中的有效记录数量"},"items":{"type":"array","description":%q,"items":{"type":"object","description":"严格校验且带稳定身份的 HRbrain 业务记录","additionalProperties":true}}},"required":["count","items"],"additionalProperties":false}`,
			description, description,
		)),
		SensitivePaths: []string{"items.name", "items.mobile", "items.email", "items.workNo", "items.userId"},
	}
}

func hrbrainPageResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"当前页有效记录数量"},"items":{"type":"array","description":%q,"items":{"type":"object","description":"严格校验且带稳定身份的 HRbrain 业务记录","additionalProperties":true}},"currentPage":{"type":"integer","description":"服务端确认的当前页码"},"pageSize":{"type":"integer","description":"服务端确认的分页大小"},"totalCount":{"type":"integer","description":"服务端报告的总记录数"},"complete":{"type":"boolean","description":"服务端证据是否证明到达末页"}},"required":["count","items","currentPage","pageSize","totalCount","complete"],"additionalProperties":false}`,
			description, description,
		)),
		SensitivePaths: []string{"items.name", "items.mobile", "items.email", "items.workNo", "items.userId"},
	}
}

func hrbrainPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind: contract.PaginationKindCursor, CursorParameter: "page",
		MetaPath: contract.PaginationMetaPath, EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath: contract.PaginationNextTokenPath,
	}
}

func hrbrainContract(command, description, intent string, result *contract.ResultSpec, pagination *contract.PaginationSpec, params []contract.ParamDecl, examples ...string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	path := "hrbrain " + command
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "hrbrain", Name: name, CanonicalPath: "hrbrain." + name,
			CLIPath: path, PrimaryCLIPath: path,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceUnavailable, Reason: hrbrainUnavailableReason(command),
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen:    []string{"该 HRbrain Shortcut 当前 unavailable；不要把 null、业务错误或缺少安全 fixture 当作成功，基础通讯录查询改用 contact"},
			Examples:     examples,
		},
		Parameters: params,
		Result:     result,
		Pagination: pagination,
	}
}

func hrbrainBase(command, description, intent string, result *contract.ResultSpec, pagination *contract.PaginationSpec, flags []shortcut.Flag, params []contract.ParamDecl, examples ...string) shortcut.Shortcut {
	return shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       "hrbrain",
		Command:       command,
		Product:       "hrbrain",
		Description:   description,
		Intent:        intent,
		Risk:          shortcut.RiskRead,
		Safety:        hrbrainReadSafety(),
		Contract:      hrbrainContract(command, description, intent, result, pagination, params, examples...),
		Flags:         flags,
		Tips:          examples,
		Hidden:        true,
		Availability:  shortcut.AvailabilityUnavailable,
	}
}

var ListPools = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+list-pools", "查询组织大脑人才池列表", "需要按名称、类型、创建人或标签发现人才池时使用；当前必须保持 unavailable，直到下游返回可验证分页数组。",
		hrbrainPageResult("严格校验的人才池搜索页"), hrbrainPagination(),
		[]shortcut.Flag{
			{Name: "keyword", Type: shortcut.FlagString, Desc: "人才池名称关键词"},
			{Name: "pool-type", Type: shortcut.FlagString, Desc: "人才池类型"},
			{Name: "creator", Type: shortcut.FlagString, Desc: "创建人稳定身份"},
			{Name: "labels", Type: shortcut.FlagStringSlice, Desc: "标签列表"},
			{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码必须大于 0"},
			{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量必须在 1 到 100 之间"},
		},
		[]contract.ParamDecl{
			{Name: "keyword", Property: "keyword"}, {Name: "pool-type", Property: "poolType"},
			{Name: "creator", Property: "creator"}, {Name: "labels", Property: "labels"},
			{Name: "page", Property: "currentPage", InterfaceType: "number"}, {Name: "page-size", Property: "pageSize", InterfaceType: "number"},
		},
		`dws hrbrain +list-pools --page 1 --page-size 20 --format json`,
	)
	declaration.Constraints = hrbrainPageConstraints()
	declaration.Validate = hrbrainValidatePage
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"currentPage": rt.Int("page"), "pageSize": rt.Int("page-size")}
		hrbrainAddOptional(params, rt, "keyword", "keyword")
		hrbrainAddOptional(params, rt, "pool-type", "poolType")
		hrbrainAddOptional(params, rt, "creator", "creator")
		if rt.Changed("labels") {
			params["labels"] = hrbrainCleanValues(rt.StrSlice("labels"))
		}
		return hrbrainCallPage(rt, "list_talent_pools", params, "poolCode")
	}
	return declaration
}()

var GetPool = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+get-pool", "根据 poolCode 查询人才池详情", "已知稳定 poolCode 并需要读取同一人才池详情时使用；没有安全非空 fixture 前保持 unavailable。",
		hrbrainObjectResult("身份匹配的人才池详情"), nil,
		[]shortcut.Flag{{Name: "pool-code", Type: shortcut.FlagString, Required: true, Desc: "人才池稳定 poolCode"}},
		[]contract.ParamDecl{{Name: "pool-code", Property: "poolCode"}},
		`dws hrbrain +get-pool --pool-code <POOL_CODE> --format json`,
	)
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		return hrbrainCallObject(rt, "get_talent_pool_detail", map[string]any{"poolCode": rt.Str("pool-code")}, rt.Str("pool-code"), "poolCode")
	}
	return declaration
}()

var ListPoolEmployees = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+list-pool-employees", "查询指定人才池内的员工列表", "已知稳定 poolCode 并需要分页读取池内员工时使用；缺少安全人才池与员工 fixture 前保持 unavailable。",
		hrbrainPageResult("严格校验的人才池员工页"), hrbrainPagination(),
		[]shortcut.Flag{
			{Name: "pool-code", Type: shortcut.FlagString, Required: true, Desc: "人才池稳定 poolCode"},
			{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码必须大于 0"},
			{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量必须在 1 到 100 之间"},
		},
		[]contract.ParamDecl{
			{Name: "pool-code", Property: "poolCode"}, {Name: "page", Property: "currentPage", InterfaceType: "number"},
			{Name: "page-size", Property: "pageSize", InterfaceType: "number"},
		},
		`dws hrbrain +list-pool-employees --pool-code <POOL_CODE> --page 1 --page-size 20 --format json`,
	)
	declaration.Constraints = hrbrainPageConstraints()
	declaration.Validate = hrbrainValidatePage
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		return hrbrainCallPage(rt, "list_pool_employees", map[string]any{
			"poolCode": rt.Str("pool-code"), "currentPage": rt.Int("page"), "pageSize": rt.Int("page-size"),
		}, "workNo", "staffId", "userId")
	}
	return declaration
}()

var ProfileMetadata = hrbrainCollectionShortcut(
	"+profile-metadata", "查询员工档案元数据结构", "已知工号并需要发现可查询档案模块与字段时使用；下游工具当前不可验证，保持 unavailable。",
	"get_profile_metadata", "work-no", "workNo", "档案元数据", []string{"modelCode", "fieldCode", "code"},
)

var QueryProfile = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+query-profile", "按模块批量查询员工档案数据", "已知工号、模块与字段编码并需要批量读取档案时使用；下游可用性和结果合同未通过 live 验证。",
		hrbrainCollectionResult("严格校验的员工档案模块结果"), nil,
		[]shortcut.Flag{
			{Name: "work-no", Type: shortcut.FlagString, Required: true, Desc: "员工稳定工号"},
			{Name: "data-queries", Type: shortcut.FlagString, Required: true, Desc: "必须是非空 JSON 数组的模块查询条件"},
		},
		[]contract.ParamDecl{{Name: "work-no", Property: "workNo"}, {Name: "data-queries", Property: "dataQueries"}},
		`dws hrbrain +query-profile --work-no <WORK_NO> --data-queries '[{"modelCode":"basic","fields":["name"]}]' --format json`,
	)
	declaration.Constraints = []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"data-queries"}, Description: "必须是非空 JSON 数组的模块查询条件"}}
	declaration.Validate = func(rt *shortcut.RuntimeContext) error {
		_, err := hrbrainJSONArray(rt.Str("data-queries"), "--data-queries")
		return err
	}
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		// Validate already established the exact non-empty object-array shape.
		queries, _ := hrbrainJSONArray(rt.Str("data-queries"), "--data-queries")
		return hrbrainCallCollection(rt, "query_profile_data", map[string]any{"workNo": rt.Str("work-no"), "dataQueries": queries}, "modelCode", "moduleCode")
	}
	return declaration
}()

var ProfileLabels = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+profile-labels", "批量查询员工档案标签", "已知一组稳定工号并需要读取标签时使用；下游当前 success=true 但 result=null，保持 unavailable。",
		hrbrainCollectionResult("严格校验的员工标签结果"), nil,
		[]shortcut.Flag{
			{Name: "staff-ids", Type: shortcut.FlagStringSlice, Required: true, Desc: "一个或多个非空员工工号"},
			{Name: "all-label", Type: shortcut.FlagBool, Desc: "是否返回全部标签"},
		},
		[]contract.ParamDecl{{Name: "staff-ids", Property: "staffIds"}, {Name: "all-label", Property: "allLabel"}},
		`dws hrbrain +profile-labels --staff-ids <WORK_NO> --format json`,
	)
	declaration.Constraints = []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"staff-ids"}, Description: "必须包含一个或多个非空员工工号"}}
	declaration.Validate = func(rt *shortcut.RuntimeContext) error {
		if len(hrbrainCleanValues(rt.StrSlice("staff-ids"))) == 0 {
			return apperrors.NewValidation("--staff-ids 必须包含一个或多个非空员工工号")
		}
		return nil
	}
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"staffIds": hrbrainCleanValues(rt.StrSlice("staff-ids"))}
		if rt.Changed("all-label") {
			params["allLabel"] = rt.Bool("all-label")
		}
		return hrbrainCallCollection(rt, "get_profile_label", params, "workNo", "staffId", "userId")
	}
	return declaration
}()

var ProfileCareer = hrbrainCollectionShortcut(
	"+profile-career", "查询员工公司内职业历程", "已知稳定工号并需要读取岗位或职级变动历史时使用；下游未通过 live 可用性验证。",
	"get_employee_career", "work-no", "workNo", "职业历程", []string{"careerId", "id", "workNo"},
)

var ProfilePerformance = hrbrainCollectionShortcut(
	"+profile-performance", "查询员工历史绩效记录", "已知稳定工号并需要读取历史绩效时使用；下游未通过 live 可用性验证。",
	"get_employee_performance", "work-no", "workNo", "绩效记录", []string{"performanceId", "id", "workNo"},
)

var SearchEmployees = func() shortcut.Shortcut {
	declaration := hrbrainEmployeeSearchBase(
		"+search-employees", "按关键词、部门、职务、职级或人才池搜索员工", "需要按至少一个基础过滤条件搜索员工时使用；当前下游返回业务错误且缺少安全非空 fixture。",
		"search_employees",
	)
	declaration.Constraints = append(hrbrainPageConstraints(), shortcut.Constraint{
		Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"keyword", "dept-name", "position-name", "job-level", "pool-code"},
		Description: "至少提供一个员工搜索过滤条件，禁止无条件枚举组织人员",
	})
	declaration.Validate = hrbrainValidatePage
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"currentPage": rt.Int("page"), "pageSize": rt.Int("page-size")}
		for _, pair := range [][2]string{{"keyword", "keyword"}, {"dept-name", "deptName"}, {"position-name", "positionName"}, {"job-level", "jobLevel"}, {"pool-code", "poolCode"}} {
			hrbrainAddOptional(params, rt, pair[0], pair[1])
		}
		return hrbrainCallPage(rt, "search_employees", params, "workNo", "staffId", "userId")
	}
	return declaration
}()

var SearchEmployeesStructured = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+search-employees-structured", "使用高级条件表达式搜索员工", "已先取得有权限字段，并需要组合过滤表达式搜索员工时使用；下游工具当前不可用。",
		hrbrainPageResult("严格校验的高级员工搜索页"), hrbrainPagination(),
		[]shortcut.Flag{
			{Name: "origin-json", Type: shortcut.FlagString, Required: true, Desc: "必须是非空 JSON 对象的搜索表达式"},
			{Name: "fields", Type: shortcut.FlagString, Required: true, Desc: "必须是非空 JSON 数组的返回字段"},
			{Name: "order-by", Type: shortcut.FlagStringSlice, Desc: "排序字段列表"},
			{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码必须大于 0"},
			{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量必须在 1 到 100 之间"},
		},
		[]contract.ParamDecl{
			{Name: "origin-json", Property: "originJson"}, {Name: "fields", Property: "fields"}, {Name: "order-by", Property: "orderByClauses"},
			{Name: "page", Property: "currentPage", InterfaceType: "number"}, {Name: "page-size", Property: "pageSize", InterfaceType: "number"},
		},
		`dws hrbrain +search-employees-structured --origin-json '{"rules":[],"combinator":"and"}' --fields '[{"label":"name","value":"name"}]' --format json`,
	)
	declaration.Constraints = append(hrbrainPageConstraints(),
		shortcut.Constraint{Kind: shortcut.ConstraintCustom, Flags: []string{"origin-json"}, Description: "必须是非空 JSON 对象的搜索表达式"},
		shortcut.Constraint{Kind: shortcut.ConstraintCustom, Flags: []string{"fields"}, Description: "必须是非空 JSON 数组的返回字段"},
	)
	declaration.Validate = func(rt *shortcut.RuntimeContext) error {
		if err := hrbrainValidatePage(rt); err != nil {
			return err
		}
		if _, err := hrbrainJSONObject(rt.Str("origin-json"), "--origin-json"); err != nil {
			return err
		}
		_, err := hrbrainJSONArray(rt.Str("fields"), "--fields")
		return err
	}
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		// Validate already established both structured input shapes.
		origin, _ := hrbrainJSONObject(rt.Str("origin-json"), "--origin-json")
		fields, _ := hrbrainJSONArray(rt.Str("fields"), "--fields")
		params := map[string]any{
			"originJson": string(mustJSON(origin)), "fields": fields,
			"currentPage": rt.Int("page"), "pageSize": rt.Int("page-size"),
		}
		if rt.Changed("order-by") {
			params["orderByClauses"] = hrbrainCleanValues(rt.StrSlice("order-by"))
		}
		return hrbrainCallPage(rt, "search_employees_structured", params, "workNo", "staffId", "userId")
	}
	return declaration
}()

var SearchFields = func() shortcut.Shortcut {
	declaration := hrbrainBase(
		"+search-fields", "获取当前身份可用的高级人才搜索字段", "构造高级员工搜索表达式前发现字段与操作符时使用；下游工具当前返回业务错误，保持 unavailable。",
		hrbrainCollectionResult("严格校验的高级搜索字段"), nil, nil, nil,
		`dws hrbrain +search-fields --format json`,
	)
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		return hrbrainCallCollection(rt, "get_search_fields", nil, "fieldCode", "code", "value")
	}
	return declaration
}()

func hrbrainCollectionShortcut(command, description, intent, tool, flag, property, resultDescription string, identityKeys []string) shortcut.Shortcut {
	declaration := hrbrainBase(
		command, description, intent, hrbrainCollectionResult("严格校验的"+resultDescription), nil,
		[]shortcut.Flag{{Name: flag, Type: shortcut.FlagString, Required: true, Desc: "员工稳定工号"}},
		[]contract.ParamDecl{{Name: flag, Property: property}},
		fmt.Sprintf("dws hrbrain %s --%s <WORK_NO> --format json", command, flag),
	)
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		return hrbrainCallCollection(rt, tool, map[string]any{property: rt.Str(flag)}, identityKeys...)
	}
	return declaration
}

func hrbrainEmployeeSearchBase(command, description, intent, _ string) shortcut.Shortcut {
	return hrbrainBase(
		command, description, intent, hrbrainPageResult("严格校验的员工搜索页"), hrbrainPagination(),
		[]shortcut.Flag{
			{Name: "keyword", Type: shortcut.FlagString, Desc: "姓名或工号关键词"},
			{Name: "dept-name", Type: shortcut.FlagString, Desc: "部门名称"},
			{Name: "position-name", Type: shortcut.FlagString, Desc: "职务名称"},
			{Name: "job-level", Type: shortcut.FlagString, Desc: "职级"},
			{Name: "pool-code", Type: shortcut.FlagString, Desc: "人才池稳定 poolCode"},
			{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码必须大于 0"},
			{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量必须在 1 到 100 之间"},
		},
		[]contract.ParamDecl{
			{Name: "keyword", Property: "keyword"}, {Name: "dept-name", Property: "deptName"}, {Name: "position-name", Property: "positionName"},
			{Name: "job-level", Property: "jobLevel"}, {Name: "pool-code", Property: "poolCode"},
			{Name: "page", Property: "currentPage", InterfaceType: "number"}, {Name: "page-size", Property: "pageSize", InterfaceType: "number"},
		},
		`dws hrbrain +search-employees --keyword <QUERY> --page 1 --page-size 20 --format json`,
	)
}

func hrbrainPageConstraints() []shortcut.Constraint {
	return []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "页码必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-size"}, Description: "每页数量必须在 1 到 100 之间"},
	}
}

func hrbrainValidatePage(rt *shortcut.RuntimeContext) error {
	if rt.Int("page") < 1 {
		return apperrors.NewValidation("--page 必须大于 0")
	}
	if size := rt.Int("page-size"); size < 1 || size > 100 {
		return apperrors.NewValidation("--page-size 必须在 1 到 100 之间")
	}
	return nil
}

func hrbrainCallObject(rt *shortcut.RuntimeContext, tool string, params map[string]any, expected string, identityKeys ...string) error {
	data, err := rt.CallMCPData("hrbrain", tool, params)
	if err != nil {
		return err
	}
	value, err := responsecheck.RequireSingleObjectResult(data, "hrbrain/"+tool)
	if err != nil {
		return err
	}
	identity := hrbrainIdentity(value, identityKeys...)
	if identity == "" {
		return responsecheck.Error("hrbrain/"+tool, "missing_stable_id", "HRbrain 详情对象缺少稳定身份")
	}
	if expected != "" && identity != strings.TrimSpace(expected) {
		return responsecheck.Error("hrbrain/"+tool, "identity_mismatch", "HRbrain 详情对象身份与请求目标不一致")
	}
	return rt.Output(map[string]any{"value": value})
}

func hrbrainCallCollection(rt *shortcut.RuntimeContext, tool string, params map[string]any, identityKeys ...string) error {
	data, err := rt.CallMCPData("hrbrain", tool, params)
	if err != nil {
		return err
	}
	items, err := hrbrainRequireCollectionResult(data, "hrbrain/"+tool, identityKeys...)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"count": len(items), "items": items})
}

func hrbrainCallPage(rt *shortcut.RuntimeContext, tool string, params map[string]any, identityKeys ...string) error {
	page, size := rt.Int("page"), rt.Int("page-size")
	data, err := rt.CallMCPData("hrbrain", tool, params)
	if err != nil {
		return err
	}
	items, evidence, err := hrbrainProjectPage(data, "hrbrain/"+tool, page, size, identityKeys...)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"count": len(items), "items": items, "currentPage": evidence.CurrentPage,
		"pageSize": evidence.PageSize, "totalCount": evidence.TotalCount, "complete": !evidence.HasMore,
	}
	next := ""
	if evidence.HasMore {
		next = strconv.Itoa(evidence.CurrentPage + 1)
	}
	// The strict page evidence guarantees either terminal+empty-next or
	// continuing+positive next page, the two valid framework states.
	pagination, _ := output.NewPagination(!evidence.HasMore, next)
	return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(&output.Meta{
		Count: output.NewCount(len(items)), Pagination: pagination,
	})))
}

func hrbrainRequireCollectionResult(data map[string]any, operation string, identityKeys ...string) ([]map[string]any, error) {
	result, err := responsecheck.RequireResult(data, operation)
	if err != nil {
		return nil, err
	}
	raw, ok := result.([]any)
	if !ok {
		return nil, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result 应为数组，实际为 %T", result))
	}
	return hrbrainValidateItems(raw, operation, identityKeys...)
}

func hrbrainProjectPage(data map[string]any, operation string, requestedPage, requestedSize int, identityKeys ...string) ([]map[string]any, hrbrainPage, error) {
	result, err := responsecheck.RequireObjectResult(data, operation)
	if err != nil {
		return nil, hrbrainPage{}, err
	}
	raw, ok := result["items"].([]any)
	if !ok {
		if _, present := result["items"]; !present {
			return nil, hrbrainPage{}, responsecheck.Error(operation, "missing_collection", "成功响应缺少 result.items 数组")
		}
		return nil, hrbrainPage{}, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result.items 应为数组，实际为 %T", result["items"]))
	}
	items, err := hrbrainValidateItems(raw, operation, identityKeys...)
	if err != nil {
		return nil, hrbrainPage{}, err
	}
	evidence, err := hrbrainParsePage(result, operation, requestedPage, requestedSize, len(items))
	if err != nil {
		return nil, hrbrainPage{}, err
	}
	return items, evidence, nil
}

func hrbrainValidateItems(raw []any, operation string, identityKeys ...string) ([]map[string]any, error) {
	items := make([]map[string]any, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("结果第 %d 项不是非空对象", index))
		}
		identity := hrbrainIdentity(object, identityKeys...)
		if identity == "" {
			return nil, responsecheck.Error(operation, "missing_item_identity", fmt.Sprintf("结果第 %d 项缺少稳定身份", index))
		}
		if _, duplicate := seen[identity]; duplicate {
			return nil, responsecheck.Error(operation, "duplicate_item_identity", fmt.Sprintf("结果第 %d 项稳定身份重复", index))
		}
		seen[identity] = struct{}{}
		items = append(items, object)
	}
	return items, nil
}

func hrbrainParsePage(result map[string]any, operation string, requestedPage, requestedSize, itemCount int) (hrbrainPage, error) {
	currentPage, ok := hrbrainInteger(result["currentPage"])
	if !ok || currentPage != requestedPage || currentPage < 1 {
		return hrbrainPage{}, responsecheck.Error(operation, "pagination_page_mismatch", "currentPage 缺失、错型或与请求页码不一致")
	}
	pageSize, ok := hrbrainInteger(result["pageSize"])
	if !ok || pageSize != requestedSize || pageSize < 1 || itemCount > pageSize {
		return hrbrainPage{}, responsecheck.Error(operation, "pagination_size_mismatch", "pageSize 缺失、错型、与请求不一致或小于结果条数")
	}
	totalCount, ok := hrbrainInteger(result["totalCount"])
	if !ok || totalCount < 0 || totalCount < itemCount {
		return hrbrainPage{}, responsecheck.Error(operation, "invalid_total_count", "totalCount 缺失、错型、为负数或小于当前页条数")
	}
	hasMore, ok := result["hasMore"].(bool)
	if !ok {
		return hrbrainPage{}, responsecheck.Error(operation, "malformed_pagination", "hasMore 缺失或不是布尔值")
	}
	pageEnd := currentPage * pageSize
	if hasMore && pageEnd >= totalCount || !hasMore && pageEnd < totalCount {
		return hrbrainPage{}, responsecheck.Error(operation, "conflicting_pagination", "hasMore 与 currentPage/pageSize/totalCount 相互矛盾")
	}
	return hrbrainPage{CurrentPage: currentPage, PageSize: pageSize, TotalCount: totalCount, HasMore: hasMore}, nil
}

func hrbrainIdentity(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value := object[key]
		switch typed := value.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				return text
			}
		case float64:
			if typed == math.Trunc(typed) && !math.IsNaN(typed) && !math.IsInf(typed, 0) {
				return strconv.FormatInt(int64(typed), 10)
			}
		}
	}
	return ""
}

func hrbrainInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		if typed != math.Trunc(typed) || math.IsNaN(typed) || math.IsInf(typed, 0) || typed > float64(math.MaxInt) || typed < float64(math.MinInt) {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}

func hrbrainAddOptional(params map[string]any, rt *shortcut.RuntimeContext, flag, property string) {
	if value := strings.TrimSpace(rt.Str(flag)); value != "" {
		params[property] = value
	}
}

func hrbrainCleanValues(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func hrbrainJSONArray(raw, flag string) ([]any, error) {
	var value []any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s 必须是有效 JSON 数组: %v", flag, err))
	}
	if len(value) == 0 {
		return nil, apperrors.NewValidation(flag + " 必须是非空 JSON 数组")
	}
	for index, item := range value {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, apperrors.NewValidation(fmt.Sprintf("%s 第 %d 项必须是非空对象", flag, index))
		}
	}
	return value, nil
}

func hrbrainJSONObject(raw, flag string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s 必须是有效 JSON 对象: %v", flag, err))
	}
	if len(value) == 0 {
		return nil, apperrors.NewValidation(flag + " 必须是非空 JSON 对象")
	}
	return value, nil
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func init() {
	shortcut.Register(
		ListPools, GetPool, ListPoolEmployees,
		ProfileMetadata, QueryProfile, ProfileLabels, ProfileCareer, ProfilePerformance,
		SearchEmployees, SearchEmployeesStructured, SearchFields,
	)
}
