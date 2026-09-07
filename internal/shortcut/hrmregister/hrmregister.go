// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package hrmregister exposes reviewed DWS shortcuts for the employee lifecycle
// MCP tools. It is deliberately a thin adapter: tenant and operator identities
// stay runtime-injected, while DWS owns readable flags, validation, confirmation,
// and the exact argument shape sent to the existing MCP tools.
package hrmregister

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const productHrmregister = "hrmregister"

type valueKind uint8

const (
	valueString valueKind = iota
	valueInt
	valueBool
	valueStrings
	valueJSONArray
	valueJSONString
	valueMillis
)

type parameterSpec struct {
	flag        string
	property    string
	description string
	kind        valueKind
	required    bool
	enum        []string
	container   string
}

type dateRangeSpec struct {
	start string
	end   string
}

type toolSpec struct {
	command     string
	tool        string
	description string
	intent      string
	risk        shortcut.Risk
	parameters  []parameterSpec
	constraints []shortcut.Constraint
	ranges      []dateRangeSpec
	example     string
}

func stringParam(flag, property, description string, required bool) parameterSpec {
	return parameterSpec{flag: flag, property: property, description: description, kind: valueString, required: required}
}

func intParam(flag, property, description string, required bool) parameterSpec {
	return parameterSpec{flag: flag, property: property, description: description, kind: valueInt, required: required}
}

func boolParam(flag, property, description string) parameterSpec {
	return parameterSpec{flag: flag, property: property, description: description, kind: valueBool}
}

func stringsParam(flag, property, description string, required bool) parameterSpec {
	return parameterSpec{flag: flag, property: property, description: description, kind: valueStrings, required: required}
}

func millisParam(flag, property, description string, required bool) parameterSpec {
	return parameterSpec{flag: flag, property: property, description: description, kind: valueMillis, required: required}
}

func hrmregisterSafety(risk shortcut.Risk) contract.SafetySpec {
	switch risk {
	case shortcut.RiskHighWrite:
		return contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"}
	case shortcut.RiskWrite:
		return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"}
	default:
		return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
	}
}

func hrmregisterResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"success":{"type":"boolean","description":"HSF 调用是否成功"},"code":{"type":"string","description":"服务端结果码"},"message":{"type":"string","description":"服务端结果说明"},"result":{"description":"人事 MCP 返回的完整业务结果"}},"additionalProperties":true}`,
			description,
		)),
		SensitivePaths: []string{"result"},
	}
}

func hrmregisterFlag(parameter parameterSpec) shortcut.Flag {
	flag := shortcut.Flag{
		Name:     parameter.flag,
		Desc:     parameter.description,
		Required: parameter.required,
		Enum:     append([]string(nil), parameter.enum...),
	}
	switch parameter.kind {
	case valueInt:
		flag.Type = shortcut.FlagInt
	case valueBool:
		flag.Type = shortcut.FlagBool
	case valueStrings:
		flag.Type = shortcut.FlagStringSlice
	case valueJSONArray, valueJSONString:
		flag.Type = shortcut.FlagString
		flag.Input = []string{"file", "stdin"}
	default:
		flag.Type = shortcut.FlagString
	}
	return flag
}

func hrmregisterContractParameter(parameter parameterSpec) contract.ParamDecl {
	property := parameter.property
	if parameter.container != "" {
		property = parameter.container + "." + property
	}
	decl := contract.ParamDecl{Name: parameter.flag, Property: property}
	if parameter.kind == valueInt || parameter.kind == valueMillis {
		decl.InterfaceType = "number"
	}
	return decl
}

func hrmregisterShortcut(spec toolSpec) shortcut.Shortcut {
	risk := spec.risk
	if risk == "" {
		risk = shortcut.RiskRead
	}
	flags := make([]shortcut.Flag, 0, len(spec.parameters))
	parameters := make([]contract.ParamDecl, 0, len(spec.parameters))
	for _, parameter := range spec.parameters {
		flags = append(flags, hrmregisterFlag(parameter))
		parameters = append(parameters, hrmregisterContractParameter(parameter))
	}
	constraints := append([]shortcut.Constraint(nil), spec.constraints...)
	if hrmregisterNeedsValidation(spec) {
		validatedFlags := make([]string, 0, len(spec.parameters))
		for _, parameter := range spec.parameters {
			validatedFlags = append(validatedFlags, parameter.flag)
		}
		constraints = append(constraints, shortcut.Constraint{
			Kind:        shortcut.ConstraintCustom,
			Flags:       validatedFlags,
			Description: "列表、JSON、分页及日期参数按工具契约校验；日期区间的结束值不得早于开始值",
		})
	}
	example := hrmregisterExample(spec)
	declaration := shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       productHrmregister,
		Command:       spec.command,
		Product:       productHrmregister,
		Description:   spec.description,
		Intent:        spec.intent,
		Risk:          risk,
		Safety:        hrmregisterSafety(risk),
		Contract: corecmd.ContractDecl{
			Identity: contract.ToolIdentitySpec{
				ProductID:      productHrmregister,
				Name:           "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(spec.command, "+"), "-", "_"),
				CanonicalPath:  productHrmregister + ".shortcut_" + strings.ReplaceAll(strings.TrimPrefix(spec.command, "+"), "-", "_"),
				CLIPath:        productHrmregister + " " + spec.command,
				PrimaryCLIPath: productHrmregister + " " + spec.command,
			},
			Description: spec.description,
			Parameters:  parameters,
			Interface: &contract.InterfaceSpec{
				Mode:         contract.InterfaceModeComposite,
				Availability: contract.InterfaceAvailable,
				Reason:       "Reviewed built-in Shortcut adapter: DWS validates CLI inputs, omits runtime-injected tenant/operator identity, preserves the MCP argument contract, and owns confirmation for mutations.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: spec.description,
				UseWhen:      []string{spec.intent},
				AvoidWhen:    []string{"只需普通通讯录搜索、考勤或组织大脑人才池能力时不要使用；写操作未取得用户明确确认时不得执行"},
				Examples:     []string{example},
			},
			Result: hrmregisterResult(spec.description + "的服务端结果"),
		},
		Flags:       flags,
		Constraints: constraints,
		Tips:        []string{example},
		Execute: func(rt *shortcut.RuntimeContext) error {
			params, err := hrmregisterParameters(spec, rt)
			if err != nil {
				return err
			}
			return rt.CallMCP(spec.tool, params)
		},
	}
	if hrmregisterNeedsValidation(spec) {
		declaration.Validate = func(rt *shortcut.RuntimeContext) error {
			return validateHrmregister(spec, rt)
		}
	}
	return declaration
}

func hrmregisterExample(spec toolSpec) string {
	if strings.TrimSpace(spec.example) != "" {
		return spec.example
	}
	parts := []string{"dws", productHrmregister, spec.command}
	included := map[string]bool{}
	appendParameter := func(parameter parameterSpec) {
		if included[parameter.flag] {
			return
		}
		included[parameter.flag] = true
		parts = append(parts, "--"+parameter.flag, hrmregisterExampleValue(parameter))
	}
	for _, parameter := range spec.parameters {
		if parameter.required {
			appendParameter(parameter)
		}
	}
	for _, constraint := range spec.constraints {
		if constraint.Kind != shortcut.ConstraintAtLeastOne && constraint.Kind != shortcut.ConstraintExactlyOne {
			continue
		}
		for _, flag := range constraint.Flags {
			for _, parameter := range spec.parameters {
				if parameter.flag == flag {
					appendParameter(parameter)
					break
				}
			}
			break
		}
	}
	parts = append(parts, "--format", "json")
	return strings.Join(parts, " ")
}

func hrmregisterExampleValue(parameter parameterSpec) string {
	if len(parameter.enum) > 0 {
		return parameter.enum[0]
	}
	switch parameter.kind {
	case valueInt:
		return "1"
	case valueBool:
		return "true"
	case valueStrings:
		return "ID_1"
	case valueJSONArray:
		return `'[{"fieldCode":"FIELD_CODE","value":"VALUE"}]'`
	case valueJSONString:
		return `'{"staffId":"STAFF_ID"}'`
	case valueMillis:
		return "2026-09-15"
	}
	name := strings.ToUpper(strings.ReplaceAll(parameter.flag, "-", "_"))
	if strings.Contains(parameter.flag, "date") {
		return "2026-09-15"
	}
	if strings.Contains(parameter.flag, "mobile") {
		return "13800000000"
	}
	return name
}

func hrmregisterNeedsValidation(spec toolSpec) bool {
	if len(spec.ranges) > 0 {
		return true
	}
	for _, parameter := range spec.parameters {
		switch parameter.kind {
		case valueStrings, valueJSONArray, valueJSONString, valueMillis:
			return true
		case valueInt:
			if parameter.flag == "current" || parameter.flag == "page-size" || parameter.flag == "size" || parameter.flag == "offset" {
				return true
			}
		}
	}
	return false
}

func validateHrmregister(spec toolSpec, rt *shortcut.RuntimeContext) error {
	for _, parameter := range spec.parameters {
		switch parameter.kind {
		case valueStrings:
			values := cleanStrings(rt.StrSlice(parameter.flag))
			if parameter.required && len(values) == 0 {
				return fmt.Errorf("--%s 至少需要一个值", parameter.flag)
			}
		case valueJSONArray:
			if _, err := parseJSONArray(rt.Str(parameter.flag), "--"+parameter.flag); err != nil {
				return err
			}
		case valueJSONString:
			if err := validateJSONObjectString(rt.Str(parameter.flag), "--"+parameter.flag); err != nil {
				return err
			}
		case valueMillis:
			if parameter.required || rt.Changed(parameter.flag) {
				if _, err := parseMillis(rt.Str(parameter.flag)); err != nil {
					return fmt.Errorf("--%s %w", parameter.flag, err)
				}
			}
		case valueInt:
			if !parameter.required && !rt.Changed(parameter.flag) {
				continue
			}
			value := rt.Int(parameter.flag)
			switch parameter.flag {
			case "offset":
				if value < 0 {
					return fmt.Errorf("--offset 不能小于 0")
				}
			case "current", "page-size", "size":
				if value < 1 {
					return fmt.Errorf("--%s 必须大于 0", parameter.flag)
				}
			}
		}
	}
	for _, pair := range spec.ranges {
		start, end := rt.Str(pair.start), rt.Str(pair.end)
		if start == "" || end == "" {
			continue
		}
		startTime, err := parseDate(start)
		if err != nil {
			return fmt.Errorf("--%s %w", pair.start, err)
		}
		endTime, err := parseDate(end)
		if err != nil {
			return fmt.Errorf("--%s %w", pair.end, err)
		}
		if endTime.Before(startTime) {
			return fmt.Errorf("--%s 不能早于 --%s", pair.end, pair.start)
		}
	}
	return nil
}

func hrmregisterParameters(spec toolSpec, rt *shortcut.RuntimeContext) (map[string]any, error) {
	params := map[string]any{}
	for _, parameter := range spec.parameters {
		var (
			value   any
			include bool
			err     error
		)
		switch parameter.kind {
		case valueString:
			value = rt.Str(parameter.flag)
			include = parameter.required || rt.Changed(parameter.flag) || value != ""
		case valueInt:
			value = rt.Int(parameter.flag)
			include = parameter.required || rt.Changed(parameter.flag)
		case valueBool:
			value = rt.Bool(parameter.flag)
			include = parameter.required || rt.Changed(parameter.flag)
		case valueStrings:
			value = cleanStrings(rt.StrSlice(parameter.flag))
			include = parameter.required || len(value.([]string)) > 0
		case valueJSONArray:
			value, err = parseJSONArray(rt.Str(parameter.flag), "--"+parameter.flag)
			include = parameter.required || rt.Changed(parameter.flag)
		case valueJSONString:
			value = rt.Str(parameter.flag)
			include = parameter.required || rt.Changed(parameter.flag)
		case valueMillis:
			if parameter.required || rt.Changed(parameter.flag) {
				value, err = parseMillis(rt.Str(parameter.flag))
				include = true
			}
		}
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		target := params
		if parameter.container != "" {
			nested, ok := params[parameter.container].(map[string]any)
			if !ok {
				nested = map[string]any{}
				params[parameter.container] = nested
			}
			target = nested
		}
		target[parameter.property] = value
	}
	return params, nil
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func parseJSONArray(raw, flag string) ([]any, error) {
	var value []any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("%s 必须是合法 JSON 数组: %w", flag, err)
	}
	if len(value) == 0 {
		return nil, fmt.Errorf("%s 不能为空数组", flag)
	}
	return value, nil
}

func validateJSONObjectString(raw, flag string) error {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fmt.Errorf("%s 必须是合法 JSON 对象: %w", flag, err)
	}
	if len(value) == 0 {
		return fmt.Errorf("%s 不能为空对象", flag)
	}
	return nil
}

func parseDate(raw string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if value, err := time.ParseInLocation(layout, strings.TrimSpace(raw), time.Local); err == nil {
			return value, nil
		}
	}
	return time.Time{}, fmt.Errorf("日期格式应为 YYYY-MM-DD、yyyy-MM-dd HH:mm:ss 或 RFC3339")
}

func parseMillis(raw string) (int64, error) {
	value, err := parseDate(raw)
	if err != nil {
		return 0, err
	}
	return value.UnixMilli(), nil
}

var hrmregisterToolSpecs = []toolSpec{
	{
		command: "+get-roster-value-dbg", tool: helpers.HrmregisterToolGetRosterValueDBG,
		description: "查询员工指定花名册字段的当前值",
		intent:      "安全调试花名册写入前，按员工 ID 与字段 code 读取当前值时使用；只读，不修改档案。",
		parameters: []parameterSpec{
			{flag: "staff-ids", property: "staffIdList", description: "员工 staffId 列表", kind: valueStrings, container: "param"},
			{flag: "field-filters", property: "fieldFilterList", description: "花名册字段 code 列表", kind: valueStrings, container: "param"},
		},
		example: `dws hrmregister +get-roster-value-dbg --staff-ids STAFF_ID --field-filters FIELD_CODE --format json`,
	},
	{
		command: "+get-roster-fields-dbg", tool: helpers.HrmregisterToolGetRosterFieldsDBG,
		description: "查询当前操作人可访问的花名册字段",
		intent:      "更新或调试员工花名册前，发现当前操作人有权限访问的字段 code 时使用。",
	},
	{
		command: "+update-employee-roster", tool: helpers.HrmregisterToolUpdateEmployeeRoster,
		description: "更新指定员工的花名册字段",
		intent:      "已核对目标员工、字段 code、原值和新值，需要按字段分组更新员工档案时使用。",
		risk:        shortcut.RiskWrite,
		parameters: []parameterSpec{
			stringParam("user-id", "userId", "目标员工 userId", true),
			{flag: "groups-json", property: "groups", description: "字段分组和明细 JSON 数组，可用 @文件或 - 从标准输入读取", kind: valueJSONArray, required: true},
		},
		example: `dws hrmregister +update-employee-roster --user-id USER_ID --groups-json @groups.json --format json`,
	},
	{
		command: "+list-contract-legal-entities", tool: helpers.HrmregisterToolListContractLegalEntities,
		description: "查询当前操作人可见的合同签约主体",
		intent:      "筛选或查询员工合同台账前，需要获取可选择的合同签约主体时使用。",
	},
	{
		command: "+list-employee-data-sources", tool: helpers.HrmregisterToolListEmployeeDataSources,
		description: "查询员工绩效或薪资数据源",
		intent:      "查询员工绩效或薪资数据前，需要按来源类型和实体编码发现可用数据源时使用。",
		parameters: []parameterSpec{
			stringParam("source-type", "sourceType", "数据源类型", true),
			stringParam("entity-code", "entityCode", "数据实体编码", true),
		},
		example: `dws hrmregister +list-employee-data-sources --source-type performance --entity-code ENTITY_CODE --format json`,
	},
	{
		command: "+query-employee-salary-data", tool: helpers.HrmregisterToolQueryEmployeeSalaryData,
		description: "查询员工薪资主数据",
		intent:      "已知薪资数据源和实体编码，需要按员工、月份或生效日期范围查询薪资主数据时使用。",
		parameters: []parameterSpec{
			stringParam("source-id", "sourceId", "薪资数据源 ID", true),
			intParam("offset", "offset", "分页偏移量，从 0 开始", false),
			intParam("size", "size", "每页数量，必须大于 0", false),
			stringsParam("user-ids", "userIds", "员工 userId 列表", true),
			stringParam("effect-start-date", "effectStartDate", "生效开始日期，YYYY-MM-DD", false),
			stringParam("effect-end-date", "effectEndDate", "生效结束日期，YYYY-MM-DD", false),
			stringParam("entity-code", "entityCode", "薪资数据实体编码", true),
			stringsParam("salary-months", "salaryMonths", "薪资月份列表，例如 2026-08", false),
		},
		ranges:  []dateRangeSpec{{start: "effect-start-date", end: "effect-end-date"}},
		example: `dws hrmregister +query-employee-salary-data --source-id SOURCE_ID --entity-code ENTITY_CODE --user-ids USER_ID --size 20 --format json`,
	},
	{
		command: "+get-termination-by-id", tool: helpers.HrmregisterToolGetTerminationByID,
		description: "按离职记录 ID 查询离职详情",
		intent:      "已知 terminationId，需要读取同一条离职记录详情时使用。",
		parameters:  []parameterSpec{stringParam("termination-id", "terminationId", "离职记录 ID", true)},
		example:     `dws hrmregister +get-termination-by-id --termination-id TERMINATION_ID --format json`,
	},
	{
		command: "+get-employee-termination-reason", tool: helpers.HrmregisterToolGetEmployeeTerminationReason,
		description: "查询员工离职原因",
		intent:      "已知员工 userId，需要读取其离职原因信息时使用。",
		parameters:  []parameterSpec{stringParam("user-id", "userId", "员工 userId", true)},
	},
	{
		command: "+query-termination-employees", tool: helpers.HrmregisterToolQueryTerminationEmployees,
		description: "分页查询离职员工",
		intent:      "按姓名、最后工作日或部门范围筛选离职员工时使用。",
		parameters: []parameterSpec{
			stringParam("employee-name", "employeeName", "员工姓名关键词", false),
			stringParam("last-work-start-date", "lastWorkStartDate", "最后工作日开始日期，YYYY-MM-DD", true),
			stringParam("last-work-end-date", "lastWorkEndDate", "最后工作日结束日期，YYYY-MM-DD", true),
			boolParam("hide-partner", "hidePartner", "是否隐藏合作伙伴员工"),
			intParam("page-size", "pageSize", "每页数量", false),
			stringsParam("dept-ids", "deptIds", "部门 ID 列表", false),
			intParam("current", "pageNum", "页码，从 1 开始", false),
		},
		ranges: []dateRangeSpec{{start: "last-work-start-date", end: "last-work-end-date"}},
	},
	{
		command: "+query-performance-records", tool: helpers.HrmregisterToolQueryPerformanceRecords,
		description: "查询员工绩效记录",
		intent:      "已知绩效数据源，需要按员工列表分页查询绩效记录时使用。",
		parameters: []parameterSpec{
			intParam("offset", "offset", "分页偏移量，从 0 开始", false),
			intParam("size", "size", "每页数量", false),
			stringsParam("user-ids", "userIds", "员工 userId 列表", true),
			stringParam("source-id", "sourceId", "绩效数据源 ID", true),
		},
	},
	{
		command: "+query-contract-ledgers", tool: helpers.HrmregisterToolQueryContractLedgers,
		description: "分页查询员工合同台账",
		intent:      "按签约主体、合同结束日期、部门、员工状态或合同状态筛选合同台账时使用。",
		parameters: []parameterSpec{
			stringsParam("legal-entity-ids", "legalEntityIds", "合同签约主体 ID 列表", false),
			intParam("current", "current", "页码，从 1 开始", false),
			stringParam("contract-end-date-to", "contractEndEndDate", "合同结束日期范围的结束日期，YYYY-MM-DD", true),
			stringParam("contract-end-date-from", "contractEndStartDate", "合同结束日期范围的开始日期，YYYY-MM-DD", true),
			intParam("page-size", "pageSize", "每页数量", false),
			boolParam("include-sub-dept", "includeSubDept", "是否包含子部门"),
			stringsParam("employee-statuses", "employeeStatuses", "员工状态列表", false),
			stringsParam("contract-types", "contractTypes", "合同类型列表", false),
			stringsParam("dept-ids", "deptIds", "部门 ID 列表", false),
			stringsParam("contract-period-types", "contractPeriodTypes", "合同期限类型列表", false),
			stringsParam("renew-statuses", "renewStatuses", "续签状态列表", false),
		},
		ranges: []dateRangeSpec{{start: "contract-end-date-from", end: "contract-end-date-to"}},
	},
	{
		command: "+count-employee-change-records", tool: helpers.HrmregisterToolCountEmployeeChangeRecords,
		description: "统计员工异动记录数量",
		intent:      "按日期、员工和部门范围统计员工异动记录数量时使用。",
		parameters: []parameterSpec{
			stringParam("start-date", "startDate", "统计开始日期，YYYY-MM-DD", true),
			stringParam("end-date", "endDate", "统计结束日期，YYYY-MM-DD", true),
			stringsParam("user-ids", "userIds", "员工 userId 列表", false),
			stringsParam("dept-ids", "deptIds", "部门 ID 列表", false),
			boolParam("include-sub-dept", "includeSubDept", "是否包含子部门"),
		},
		ranges: []dateRangeSpec{{start: "start-date", end: "end-date"}},
	},
	{
		command: "+query-employee-change-events", tool: helpers.HrmregisterToolQueryEmployeeChangeEvents,
		description: "查询员工异动事件",
		intent:      "按日期、异动类型、员工或部门范围分页查询员工异动事件时使用。",
		parameters: []parameterSpec{
			stringParam("start-date", "startDate", "查询开始日期，YYYY-MM-DD", false),
			stringParam("end-date", "endDate", "查询结束日期，YYYY-MM-DD", false),
			stringParam("change-type", "changeType", "异动类型", false),
			stringsParam("user-ids", "userIds", "员工 userId 列表", false),
			stringsParam("dept-ids", "deptIds", "部门 ID 列表", false),
			boolParam("include-sub-dept", "includeSubDept", "是否包含子部门"),
			intParam("offset", "offset", "分页偏移量，从 0 开始", false),
			intParam("size", "size", "每页数量", false),
		},
		ranges: []dateRangeSpec{{start: "start-date", end: "end-date"}},
	},
	{
		command: "+query-pending-transfers", tool: helpers.HrmregisterToolQueryPendingTransfers,
		description: "查询待生效调岗记录",
		intent:      "按生效日期和部门范围分页查询待生效调岗记录时使用。",
		parameters: []parameterSpec{
			stringParam("effective-start-date", "effectiveStartDate", "生效开始日期，YYYY-MM-DD", false),
			stringParam("effective-end-date", "effectiveEndDate", "生效结束日期，YYYY-MM-DD", false),
			stringsParam("department-ids", "departmentIds", "部门 ID 列表", false),
			intParam("current", "current", "页码，从 1 开始", true),
			intParam("page-size", "pageSize", "每页数量", true),
		},
		ranges: []dateRangeSpec{{start: "effective-start-date", end: "effective-end-date"}},
	},
	{
		command: "+get-transfer-form", tool: helpers.HrmregisterToolGetTransferForm,
		description: "校验并获取员工调岗表单",
		intent:      "创建调岗审批前，按目标部门、职务、职位、职级和生效日期校验并获取表单时使用。",
		parameters: []parameterSpec{
			stringParam("staff-id", "staffId", "目标员工 staffId", true),
			stringsParam("target-department-ids", "targetDepartmentIds", "调岗后完整部门 ID 列表", true),
			intParam("target-main-department-id", "targetMainDepartmentId", "调岗后主部门 ID", true),
			stringParam("target-job-id", "targetJobId", "调岗后职务 ID", false),
			stringParam("target-position-id", "targetPositionId", "调岗后职位 ID", false),
			stringParam("target-rank-id", "targetRankId", "调岗后职级 ID", false),
			stringParam("effective-date", "effectiveDate", "调岗生效日期，YYYY-MM-DD", true),
		},
	},
	{
		command: "+query-current-position", tool: helpers.HrmregisterToolQueryCurrentPosition,
		description: "查询员工当前岗位信息",
		intent:      "创建调岗审批前，需要读取目标员工当前部门、职务、职位和职级时使用。",
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "员工 staffId", true)},
	},
	{
		command: "+cancel-transfer-approval", tool: helpers.HrmregisterToolCancelTransferApproval,
		description: "撤回或删除调岗审批",
		intent:      "已确认调岗审批实例和操作类型，需要撤回或删除该调岗审批时使用。",
		risk:        shortcut.RiskHighWrite,
		parameters: []parameterSpec{
			stringParam("process-instance-id", "processInstanceId", "调岗审批实例 ID", true),
			{flag: "action", property: "action", description: "操作类型：WITHDRAW-撤回，DELETE-删除", kind: valueString, required: true, enum: []string{"WITHDRAW", "DELETE"}},
			stringParam("reason", "reason", "撤回或删除原因，最多 500 字", false),
		},
	},
	{
		command: "+create-transfer-approval", tool: helpers.HrmregisterToolCreateTransferApproval,
		description: "创建员工调岗审批",
		intent:      "已通过调岗表单校验并取得完整审批输入，需要创建调岗审批时使用。",
		risk:        shortcut.RiskWrite,
		parameters: []parameterSpec{
			{flag: "input-json", property: "input", description: "服务端调岗审批输入 JSON 对象，可用 @文件或 - 从标准输入读取", kind: valueJSONString, required: true},
		},
		example: `dws hrmregister +create-transfer-approval --input-json @transfer.json --format json`,
	},
	{
		command: "+revoke-termination-process", tool: helpers.HrmregisterToolRevokeTerminationProcess,
		description: "撤销员工离职流程",
		intent:      "已确认员工、离职记录和撤销原因，并取得确认令牌后撤销离职流程时使用。",
		risk:        shortcut.RiskHighWrite,
		parameters: []parameterSpec{
			stringParam("staff-id", "staffId", "员工 staffId", true),
			stringParam("termination-id", "terminationId", "离职记录 ID", true),
			stringParam("revoke-reason", "revokeReason", "撤销原因", true),
			stringParam("confirmation-token", "confirmationToken", "服务端确认令牌", true),
		},
	},
	{
		command: "+update-termination-information", tool: helpers.HrmregisterToolUpdateTerminationInformation,
		description: "更新员工离职信息",
		intent:      "已确认员工和离职记录，需要修改最后工作日、离职原因、说明或交接人时使用。",
		risk:        shortcut.RiskWrite,
		parameters: []parameterSpec{
			stringParam("staff-id", "staffId", "员工 staffId", true),
			stringParam("termination-id", "terminationId", "离职记录 ID", true),
			stringParam("last-work-date", "lastWorkDate", "新的最后工作日，YYYY-MM-DD", false),
			stringParam("termination-reason-code", "terminationReasonCode", "新的离职原因编码", false),
			stringParam("termination-description", "terminationDescription", "新的离职说明", false),
			stringParam("handover-user-id", "handoverUserId", "新的交接人 userId", false),
			stringParam("confirmation-token", "confirmationToken", "服务端确认令牌", true),
		},
		constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"last-work-date", "termination-reason-code", "termination-description", "handover-user-id"}, Description: "至少修改最后工作日、离职原因、离职说明或交接人中的一项"}},
	},
	{
		command: "+query-pending-terminations", tool: helpers.HrmregisterToolQueryPendingTerminations,
		description: "查询待离职员工记录",
		intent:      "按最后工作日和部门范围分页查询待离职记录时使用。",
		parameters: []parameterSpec{
			stringParam("last-work-start-date", "lastWorkStartDate", "最后工作日开始日期，YYYY-MM-DD", false),
			stringParam("last-work-end-date", "lastWorkEndDate", "最后工作日结束日期，YYYY-MM-DD", false),
			stringsParam("department-ids", "departmentIds", "部门 ID 列表", false),
			intParam("current", "current", "页码，从 1 开始", true),
			intParam("page-size", "pageSize", "每页数量", true),
		},
		ranges: []dateRangeSpec{{start: "last-work-start-date", end: "last-work-end-date"}},
	},
	{
		command: "+query-lifecycle-recipients", tool: helpers.HrmregisterToolQueryLifecycleRecipients,
		description: "查询员工生命周期消息接收人",
		intent:      "需要查询指定员工在某类入转调离业务中的消息接收人时使用。",
		parameters: []parameterSpec{
			stringParam("business-type", "businessType", "生命周期业务类型", true),
			stringParam("employee-user-id", "employeeUserId", "员工 userId", true),
		},
	},
	{
		command: "+query-pending-regularizations", tool: helpers.HrmregisterToolQueryPendingRegularizations,
		description: "查询待转正员工记录",
		intent:      "按转正日期和部门范围分页查询待转正员工时使用。",
		parameters: []parameterSpec{
			stringParam("regularization-start-date", "regularizationStartDate", "转正开始日期，YYYY-MM-DD", false),
			stringParam("regularization-end-date", "regularizationEndDate", "转正结束日期，YYYY-MM-DD", false),
			stringsParam("department-ids", "departmentIds", "部门 ID 列表", false),
			intParam("current", "current", "页码，从 1 开始", true),
			intParam("page-size", "pageSize", "每页数量", true),
		},
		ranges: []dateRangeSpec{{start: "regularization-start-date", end: "regularization-end-date"}},
	},
	{
		command: "+get-regularization-form", tool: helpers.HrmregisterToolGetRegularizationForm,
		description: "获取员工转正表单",
		intent:      "已知员工 staffId，需要查询其转正表单和可操作状态时使用。",
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "员工 staffId", true)},
	},
	{
		command: "+get-employee-material-files", tool: helpers.HrmregisterToolGetEmployeeMaterialFiles,
		description: "查询员工材料附件",
		intent:      "按员工和花名册字段 code 查询其入职或档案材料附件时使用。",
		parameters: []parameterSpec{
			stringParam("staff-id", "staffId", "员工 staffId", true),
			stringsParam("field-codes", "fieldCodes", "材料字段 code 列表", true),
		},
	},
	{
		command: "+query-pre-entry-employees", tool: helpers.HrmregisterToolQueryPreEntryEmployees,
		description: "分页查询待入职员工",
		intent:      "按姓名、手机号或计划入职时间范围分页查询待入职员工时使用。",
		parameters: []parameterSpec{
			stringParam("employee-name", "employeeName", "员工姓名关键词", false),
			stringParam("employee-mobile", "employeeMobile", "员工手机号", false),
			millisParam("pre-entry-start", "preEntryStartTime", "计划入职开始时间，YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss", false),
			millisParam("pre-entry-end", "preEntryEndTime", "计划入职结束时间，YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss", false),
			intParam("current", "current", "页码，从 1 开始", false),
			intParam("page-size", "pageSize", "每页数量", false),
		},
	},
	{
		command: "+confirm-termination", tool: helpers.HrmregisterToolConfirmTermination,
		description: "确认员工离职",
		intent:      "已先查询确认离职表单并确认 canConfirm=true，需要执行不可逆的员工离职确认时使用。",
		risk:        shortcut.RiskHighWrite,
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "员工 staffId", true)},
	},
	{
		command: "+get-confirm-termination-form", tool: helpers.HrmregisterToolGetConfirmTerminationForm,
		description: "获取确认离职表单",
		intent:      "确认员工离职前，查询操作提示、影响范围和 canConfirm 状态时使用。",
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "员工 staffId", true)},
	},
	{
		command: "+query-pre-entry-by-name-mobile", tool: helpers.HrmregisterToolQueryPreEntryByNameMobile,
		description: "按姓名或手机号查询待入职员工",
		intent:      "只知道姓名或手机号，需要精确发现待入职员工时使用。",
		parameters: []parameterSpec{
			stringParam("employee-name", "employeeName", "员工姓名", false),
			stringParam("employee-mobile", "employeeMobile", "员工手机号", false),
		},
		constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"employee-name", "employee-mobile"}, Description: "--employee-name 与 --employee-mobile 至少提供一个"}},
	},
	{
		command: "+confirm-entry", tool: helpers.HrmregisterToolConfirmEntry,
		description: "确认员工入职",
		intent:      "已先查询确认入职表单并确认 canConfirm=true，需要执行员工入职确认时使用。",
		risk:        shortcut.RiskHighWrite,
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "员工 staffId", true)},
	},
	{
		command: "+get-confirm-entry-form", tool: helpers.HrmregisterToolGetConfirmEntryForm,
		description: "获取确认入职表单",
		intent:      "确认员工入职前，查询操作提示、影响范围和 canConfirm 状态时使用。",
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "员工 staffId", true)},
	},
	{
		command: "+invite-perfect-info", tool: helpers.HrmregisterToolInvitePerfectInfo,
		description: "邀请待入职员工完善资料",
		intent:      "已确认目标待入职员工，需要向其发送完善入职资料邀请时使用。",
		risk:        shortcut.RiskWrite,
		parameters:  []parameterSpec{stringParam("staff-id", "staffId", "待入职员工 staffId", true)},
	},
	{
		command: "+add-pre-entry-employee", tool: helpers.HrmregisterToolAddPreEntryEmployee,
		description: "新增待入职员工",
		intent:      "已核对姓名、手机号、计划入职日期及组织信息，需要创建待入职员工记录时使用。",
		risk:        shortcut.RiskWrite,
		parameters: []parameterSpec{
			intParam("main-dept-id", "mainDeptId", "主部门 ID", false),
			stringsParam("dept-ids", "deptIdList", "完整部门 ID 列表", false),
			intParam("employee-type", "employeeType", "员工类型编码", false),
			millisParam("pre-entry-time", "preEntryTime", "计划入职时间，YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss", true),
			stringParam("name", "name", "员工姓名", true),
			stringParam("mobile", "mobile", "员工手机号", true),
			stringParam("position", "position", "职位名称", false),
			stringParam("job-number", "jobNumber", "工号", false),
			stringParam("work-place", "workPlace", "办公地点", false),
			stringParam("email", "email", "邮箱", false),
		},
		example: `dws hrmregister +add-pre-entry-employee --pre-entry-time 2026-09-15 --name 张三 --mobile 13800000000 --format json`,
	},
}

func init() {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: productHrmregister,
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询并办理钉钉智能人事的员工入职、转正、调岗、离职、合同、绩效、薪资和花名册业务",
			UseWhen:      []string{"用户明确处理员工入转调离、合同台账、绩效薪资数据、花名册字段或员工材料时使用"},
			AvoidWhen:    []string{"普通通讯录找人和组织架构查询使用 contact；考勤审批使用 attendance；人才池和组织大脑专项能力使用 hrbrain"},
		},
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("智能人事入转调离指南", "dingtalk-misc", "references/hrmregister.md"),
			},
		},
	})
	items := make([]shortcut.Shortcut, 0, len(hrmregisterToolSpecs))
	for _, spec := range hrmregisterToolSpecs {
		items = append(items, hrmregisterShortcut(spec))
	}
	shortcut.Register(items...)
}
