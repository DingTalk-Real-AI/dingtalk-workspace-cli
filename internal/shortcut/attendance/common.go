// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package attendance

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const attendanceCompatibilityInterfaceReason = "Historical executable Schema compatibility: this command remains callable, but the reviewed Shortcut catalog keeps it non-public until its live-data or downstream proof gap is closed."

func attendanceIntegerObjectResult(description, identity string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"value":{"type":"object","description":"严格校验后的考勤业务结果","properties":{%q:{"type":"integer","minimum":1,"description":"与请求精确绑定的稳定业务 ID"}},"required":[%q],"additionalProperties":true}},"required":["value"],"additionalProperties":false}`,
			description, identity, identity,
		)),
	}
}

func attendanceApproveTemplateResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"严格校验且绑定请求类型的考勤审批模板","properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中的有效审批模板数量"},"templates":{"type":"array","description":"可发起的考勤审批模板","items":{"type":"object","description":"具备稳定流程身份与提交入口的考勤审批模板","properties":{"processCode":{"type":"string","minLength":1,"description":"模板的稳定唯一流程编码"},"approveType":{"type":"string","enum":["REPAIR_CHECK","LEAVE","OVERTIME","TRAVEL","OUT"],"description":"与请求精确绑定的审批类型"},"submitUrl":{"type":"string","minLength":1,"description":"非空审批提交入口"},"formName":{"type":"string","description":"审批模板展示名称"}},"required":["processCode","approveType","submitUrl"],"additionalProperties":true}}},"required":["count","templates"],"additionalProperties":false}`),
	}
}

func attendanceCollectionResult(collection, description, identity, identityType string) *contract.ResultSpec {
	identityConstraint := `"minLength":1`
	if identityType == "integer" {
		identityConstraint = `"minimum":1`
	}
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中的有效业务记录数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"具备稳定身份的考勤业务记录","properties":{%q:{"type":%q,%s,"description":"记录的稳定业务身份"}},"required":[%q],"additionalProperties":true}}},"required":["count",%q],"additionalProperties":false}`,
			description, collection, description, identity, identityType, identityConstraint, identity, collection,
		)),
	}
}

func attendanceCursorPagination(cursorParameter string) *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       cursorParameter,
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func hardenPublicAttendanceContracts() {
	// Composite Shortcut properties are the stable published workflow inputs.
	// Execute owns the explicit adapters to the differently named MCP fields;
	// redirecting an existing non-empty Schema property requires a versioned
	// migration and cannot be folded into this hardening change.
	CheckResult.Contract.Parameters = []contract.ParamDecl{{Name: "users", Property: "users"}, {Name: "start", Property: "start"}, {Name: "end", Property: "end"}, {Name: "offset", Property: "offset"}, {Name: "limit", Property: "limit"}}
	CheckRecord.Contract.Parameters = []contract.ParamDecl{{Name: "users", Property: "users"}, {Name: "start", Property: "start"}, {Name: "end", Property: "end"}}
	ListApprove.Contract.Parameters = []contract.ParamDecl{{Name: "users", Property: "users"}, {Name: "types", Property: "types"}, {Name: "start", Property: "start"}, {Name: "end", Property: "end"}}
	GetApproveTemplate.Contract.Parameters = []contract.ParamDecl{{Name: "type", Property: "type"}}
	GetSchedule.Contract.Parameters = []contract.ParamDecl{{Name: "users", Property: "users"}, {Name: "start", Property: "start"}, {Name: "end", Property: "end"}}
	SearchClass.Contract.Parameters = []contract.ParamDecl{{Name: "query", Property: "query"}, {Name: "filter-type", Property: "filterType"}, {Name: "page", Property: "page"}, {Name: "limit", Property: "limit"}}
	GetClass.Contract.Parameters = []contract.ParamDecl{{Name: "class-id", Property: "classId"}}
	SearchAdjustmentRule.Contract.Parameters = []contract.ParamDecl{{Name: "query", Property: "query"}, {Name: "page", Property: "page"}, {Name: "limit", Property: "limit"}}
	GetOvertimeRule.Contract.Parameters = []contract.ParamDecl{{Name: "overtime-id", Property: "overtimeId"}}
	SearchOvertimeRule.Contract.Parameters = []contract.ParamDecl{{Name: "query", Property: "query"}, {Name: "page", Property: "page"}, {Name: "limit", Property: "limit"}}
	GetSelfSetting.Contract.Parameters = []contract.ParamDecl{{Name: "setting-scene", Property: "settingScene"}, {Name: "user", Property: "user"}}

	collections := []struct {
		declaration  *shortcut.Shortcut
		collection   string
		description  string
		identity     string
		identityType string
		cursor       string
	}{
		{&CheckResult, "records", "严格校验的员工打卡结果", "id", "integer", "offset"},
		{&CheckRecord, "records", "严格校验的员工打卡流水", "id", "integer", ""},
		{&ListApprove, "approvals", "严格校验的考勤审批记录", "id", "integer", ""},
		{&SearchClass, "classes", "严格校验的班次搜索结果", "classId", "integer", "page"},
		{&SearchAdjustmentRule, "rules", "严格校验的补卡规则搜索结果", "ruleId", "integer", "page"},
		{&SearchOvertimeRule, "rules", "严格校验的加班规则搜索结果", "ruleId", "integer", "page"},
	}
	for _, item := range collections {
		item.declaration.OutputRollout = output.RolloutUnifiedActive
		item.declaration.Contract.Result = attendanceCollectionResult(item.collection, item.description, item.identity, item.identityType)
		if item.cursor != "" {
			item.declaration.Contract.Pagination = attendanceCursorPagination(item.cursor)
		}
	}
	CheckResult.Contract.Result.SensitivePaths = []string{"records.corpId", "records.record", "records.userId"}
	CheckRecord.Contract.Result.SensitivePaths = []string{"records.baseAddress", "records.baseLatitude", "records.baseLongitude", "records.corpId", "records.deviceId", "records.features", "records.userAddress", "records.userId", "records.userLatitude", "records.userLongitude"}
	ListApprove.Contract.Result.SensitivePaths = []string{"approvals.corpId", "approvals.originId", "approvals.subType", "approvals.tagName", "approvals.userId"}
	GetApproveTemplate.OutputRollout = output.RolloutUnifiedActive
	GetApproveTemplate.Contract.Result = attendanceApproveTemplateResult()
	GetApproveTemplate.Contract.Result.SensitivePaths = []string{"templates.formName", "templates.processCode", "templates.submitUrl"}
	SearchClass.Contract.Result.SensitivePaths = []string{"classes.name", "classes.ownerName"}
	SearchAdjustmentRule.Contract.Result.SensitivePaths = []string{"rules.name"}
	SearchOvertimeRule.Contract.Result.SensitivePaths = []string{"rules.name"}
	GetOvertimeRule.OutputRollout = output.RolloutUnifiedActive
	GetOvertimeRule.Contract.Result = attendanceIntegerObjectResult("严格校验的加班规则详情", "id")
	GetOvertimeRule.Contract.Result.SensitivePaths = []string{"value.content", "value.groupIdAndNames", "value.name", "value.owner", "value.ownerList", "value.scopes"}
	markAttendanceCompatibilityOnly(&GetSummary)
	markAttendanceCompatibilityOnly(&ListLeaveTypes)
	markAttendanceCompatibilityOnly(&GetLeaveRecords)
	markAttendanceCompatibilityOnly(&GetCheckinRecord)
	markAttendanceCompatibilityOnly(&GetSchedule)
	markAttendanceUnavailable(&GetClass, "下游 get_class_detail 对搜索得到的真实 classId 返回非空详情，但 shiftVO 不回显 id/classId，Shortcut 无法诚实地把详情精确绑定到请求 ID；需要下游补充稳定 ID 回显。")
	markAttendanceCompatibilityOnly(&GetSelfSetting)
}

func markAttendanceCompatibilityOnly(declaration *shortcut.Shortcut) {
	declaration.OutputRollout = output.RolloutLegacyOnly
	declaration.Contract.Result = nil
	declaration.Contract.Pagination = nil
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceAvailable,
		Reason:       attendanceCompatibilityInterfaceReason,
	}
}

func markAttendanceUnavailable(declaration *shortcut.Shortcut, reason string) {
	declaration.OutputRollout = output.RolloutLegacyOnly
	declaration.Contract.Result = nil
	declaration.Contract.Pagination = nil
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceUnavailable,
		Reason:       reason,
	}
}

func attendanceCallCollection(
	rt *shortcut.RuntimeContext,
	product, tool string,
	params map[string]any,
	collection string,
	complete bool,
	extra map[string]any,
	validate func([]map[string]any) error,
	paths ...string,
) error {
	data, err := rt.CallMCPData(product, tool, params)
	if err != nil {
		return err
	}
	items, err := responsecheck.RequireObjectCollection(data, product+"/"+tool, paths...)
	if err != nil {
		return err
	}
	if validate != nil {
		if err := validate(items); err != nil {
			return err
		}
	}
	return attendanceOutputCollection(rt, collection, items, complete, extra, false, "")
}

func attendanceCallValue(rt *shortcut.RuntimeContext, product, tool string, params map[string]any) error {
	data, err := rt.CallMCPData(product, tool, params)
	if err != nil {
		return err
	}
	value, err := responsecheck.RequireResult(data, product+"/"+tool)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"value": value})
}

func attendanceCallObject(rt *shortcut.RuntimeContext, product, tool string, params map[string]any) error {
	data, err := rt.CallMCPData(product, tool, params)
	if err != nil {
		return err
	}
	value, err := responsecheck.RequireSingleObjectResult(data, product+"/"+tool)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"value": value})
}

func attendanceCollectionPayload(collection string, items []map[string]any, complete bool, extra map[string]any) map[string]any {
	payload := map[string]any{
		"count":    len(items),
		collection: items,
		"complete": complete,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func attendanceOutputCollection(rt *shortcut.RuntimeContext, collection string, items []map[string]any, complete bool, legacyExtra map[string]any, paginated bool, nextToken string) error {
	payload := attendanceCollectionPayload(collection, items, complete, legacyExtra)
	if !output.UsesUnifiedResult(rt.Command()) {
		return rt.Output(payload)
	}
	business := map[string]any{"count": len(items), collection: items}
	meta := &output.Meta{Count: output.NewCount(len(items))}
	if paginated {
		pagination, err := output.NewPagination(complete, nextToken)
		if err != nil {
			return responsecheck.Error("attendance/pagination", "invalid_pagination", err.Error())
		}
		meta.Pagination = pagination
	}
	return output.StoreResult(rt.Command().Context(), output.Success(business, output.WithMeta(meta)))
}

func attendanceValidatePositiveIntegerIDs(items []map[string]any, operation string, path ...string) error {
	seen := make(map[int64]struct{}, len(items))
	for index, item := range items {
		value, ok := attendanceNestedValue(item, path...)
		if !ok {
			return responsecheck.Error(operation, "missing_item_identity", fmt.Sprintf("第 %d 项缺少稳定 ID", index))
		}
		identity, ok := attendancePositiveInteger(value)
		if !ok {
			return responsecheck.Error(operation, "invalid_item_identity", fmt.Sprintf("第 %d 项稳定 ID 必须是大于 0 的整数", index))
		}
		if _, duplicate := seen[identity]; duplicate {
			return responsecheck.Error(operation, "duplicate_item_identity", fmt.Sprintf("第 %d 项稳定 ID 重复", index))
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func attendanceValidateExpectedStrings(items []map[string]any, operation, field, expected string) error {
	expected = strings.TrimSpace(expected)
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		value, ok := item[field].(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			return responsecheck.Error(operation, "invalid_item_identity", fmt.Sprintf("第 %d 项 %s 必须是非空字符串", index, field))
		}
		if expected != "" && value != expected {
			return responsecheck.Error(operation, "item_identity_mismatch", fmt.Sprintf("第 %d 项 %s 与请求不一致", index, field))
		}
		if _, duplicate := seen[value]; duplicate {
			return responsecheck.Error(operation, "duplicate_item_identity", fmt.Sprintf("第 %d 项 %s 重复", index, field))
		}
		seen[value] = struct{}{}
	}
	return nil
}

func attendanceValidateApproveTemplates(items []map[string]any, operation, expectedType string) error {
	expectedType = strings.TrimSpace(expectedType)
	seenProcessCodes := make(map[string]struct{}, len(items))
	for index, item := range items {
		processCodeValue, present := item["processCode"]
		if !present {
			return responsecheck.Error(operation, "missing_item_identity", fmt.Sprintf("第 %d 项缺少稳定 processCode", index))
		}
		processCode, ok := processCodeValue.(string)
		processCode = strings.TrimSpace(processCode)
		if !ok || processCode == "" {
			return responsecheck.Error(operation, "invalid_item_identity", fmt.Sprintf("第 %d 项 processCode 必须是非空字符串", index))
		}
		item["processCode"] = processCode
		if _, duplicate := seenProcessCodes[processCode]; duplicate {
			return responsecheck.Error(operation, "duplicate_item_identity", fmt.Sprintf("第 %d 项 processCode 重复", index))
		}
		seenProcessCodes[processCode] = struct{}{}

		approveType, ok := item["approveType"].(string)
		if !ok || approveType != expectedType {
			return responsecheck.Error(operation, "request_identity_mismatch", fmt.Sprintf("第 %d 项 approveType 与请求不一致", index))
		}
		submitURL, ok := item["submitUrl"].(string)
		if !ok || strings.TrimSpace(submitURL) == "" {
			return responsecheck.Error(operation, "malformed_item", fmt.Sprintf("第 %d 项缺少非空 submitUrl", index))
		}
	}
	return nil
}

func attendanceNestedValue(item map[string]any, path ...string) (any, bool) {
	var current any = item
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func attendancePositiveInteger(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), number > 0
	case int32:
		return int64(number), number > 0
	case int64:
		return number, number > 0
	case float64:
		return int64(number), number > 0 && number == float64(int64(number))
	case json.Number:
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		return parsed, err == nil && parsed > 0
	default:
		return 0, false
	}
}

func attendanceValidateUserIDs(values []string, maximum int) error {
	if len(values) == 0 {
		return fmt.Errorf("用户 ID 列表不能为空")
	}
	if maximum > 0 && len(values) > maximum {
		return fmt.Errorf("用户 ID 数量不能超过 %d", maximum)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("用户 ID 不能为空字符串")
		}
		if value != trimmed {
			return fmt.Errorf("用户 ID 不能包含首尾空白")
		}
		if _, duplicate := seen[trimmed]; duplicate {
			return fmt.Errorf("用户 ID 不能重复")
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

func attendanceValidateUserAndTimeBinding(items []map[string]any, operation string, requestedUsers []string, userField, timeField string, startMillis, endMillis int64) error {
	users := make(map[string]struct{}, len(requestedUsers))
	for _, user := range requestedUsers {
		users[user] = struct{}{}
	}
	for index, item := range items {
		rawUser, present := item[userField]
		if !present {
			return responsecheck.Error(operation, "missing_request_binding", fmt.Sprintf("第 %d 项缺少用户身份回显", index))
		}
		user, ok := rawUser.(string)
		user = strings.TrimSpace(user)
		if !ok || user == "" {
			return responsecheck.Error(operation, "malformed_request_binding", fmt.Sprintf("第 %d 项用户身份回显必须是非空字符串", index))
		}
		if _, requested := users[user]; !requested {
			return responsecheck.Error(operation, "request_identity_mismatch", fmt.Sprintf("第 %d 项用户身份不在请求集合中", index))
		}
		rawTimestamp, present := item[timeField]
		if !present {
			return responsecheck.Error(operation, "missing_request_binding", fmt.Sprintf("第 %d 项缺少时间回显", index))
		}
		timestamp, ok := attendancePositiveInteger(rawTimestamp)
		if !ok {
			return responsecheck.Error(operation, "malformed_request_binding", fmt.Sprintf("第 %d 项时间回显必须是大于 0 的整数毫秒时间戳", index))
		}
		if timestamp < startMillis || timestamp > endMillis {
			return responsecheck.Error(operation, "request_range_mismatch", fmt.Sprintf("第 %d 项时间不在请求范围内", index))
		}
	}
	return nil
}

func attendanceValidateApprovalBinding(items []map[string]any, operation string, requestedUsers []string, requestedTypes map[int]struct{}, startMillis, endMillis int64) error {
	users := make(map[string]struct{}, len(requestedUsers))
	for _, user := range requestedUsers {
		users[user] = struct{}{}
	}
	for index, item := range items {
		user, ok := item["userId"].(string)
		user = strings.TrimSpace(user)
		if !ok || user == "" {
			return responsecheck.Error(operation, "missing_request_binding", fmt.Sprintf("第 %d 项缺少用户身份回显", index))
		}
		if _, requested := users[user]; !requested {
			return responsecheck.Error(operation, "request_identity_mismatch", fmt.Sprintf("第 %d 项用户身份不在请求集合中", index))
		}
		bizType, ok := attendancePositiveInteger(item["bizType"])
		if !ok {
			return responsecheck.Error(operation, "missing_request_binding", fmt.Sprintf("第 %d 项缺少审批类型回显", index))
		}
		if _, requested := requestedTypes[int(bizType)]; !requested {
			return responsecheck.Error(operation, "request_type_mismatch", fmt.Sprintf("第 %d 项审批类型不在请求集合中", index))
		}
		begin, beginOK := attendancePositiveInteger(item["beginTime"])
		end, endOK := attendancePositiveInteger(item["endTime"])
		if !beginOK || !endOK || end < begin || end < startMillis || begin > endMillis {
			return responsecheck.Error(operation, "request_range_mismatch", fmt.Sprintf("第 %d 项审批时间不与请求范围相交", index))
		}
	}
	return nil
}

func attendanceValidateMonthRange(startText, endText string) error {
	start, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startText), time.Local)
	if err != nil {
		return fmt.Errorf("--start 日期格式错误，应为 YYYY-MM-DD: %w", err)
	}
	end, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endText), time.Local)
	if err != nil {
		return fmt.Errorf("--end 日期格式错误，应为 YYYY-MM-DD: %w", err)
	}
	if end.Before(start) {
		return fmt.Errorf("--end 不能早于 --start")
	}
	if end.After(start.AddDate(0, 1, 0)) {
		return fmt.Errorf("--start 到 --end 的跨度不能超过 1 个月")
	}
	return nil
}

func attendanceValidatePageRequest(rt *shortcut.RuntimeContext) error {
	_, _, err := attendancePageInput(rt)
	return err
}

func attendancePageEvidence(data map[string]any, operation string, page, limit, itemCount int) (bool, map[string]any, error) {
	if page < 1 || limit < 1 || itemCount < 0 || itemCount > limit {
		return false, nil, responsecheck.Error(operation, "invalid_pagination_request", "页码、页大小或当前项数无效")
	}
	result, err := responsecheck.RequireObjectResult(data, operation)
	if err != nil {
		return false, nil, err
	}
	extra := map[string]any{"page": page, "limit": limit}
	if raw, present := result["currentPage"]; present {
		currentPage, ok := attendanceInt(raw)
		if !ok {
			return false, nil, responsecheck.Error(operation, "invalid_pagination_evidence", "服务端 currentPage 必须是整数")
		}
		if currentPage != page {
			return false, nil, responsecheck.Error(operation, "pagination_page_mismatch", "服务端 currentPage 与请求页码不一致")
		}
	}
	var evidence []bool
	var totalPageValue, totalCountValue *int
	if raw, present := result["totalPage"]; present {
		totalPage, ok := attendanceInt(raw)
		if !ok {
			return false, nil, responsecheck.Error(operation, "invalid_pagination_evidence", "服务端 totalPage 必须是整数")
		}
		if totalPage < 0 {
			return false, nil, responsecheck.Error(operation, "invalid_pagination_evidence", "服务端 totalPage 不能为负数")
		}
		extra["totalPage"] = totalPage
		totalPageValue = &totalPage
		if totalPage == 0 {
			if page != 1 || itemCount != 0 {
				return false, nil, responsecheck.Error(operation, "pagination_count_mismatch", "totalPage=0 只允许第一页为空")
			}
			evidence = append(evidence, true)
		} else {
			if page > totalPage {
				return false, nil, responsecheck.Error(operation, "pagination_page_out_of_range", "请求页码超过服务端 totalPage")
			}
			evidence = append(evidence, page == totalPage)
		}
	}
	if raw, present := result["totalCount"]; present {
		totalCount, ok := attendanceInt(raw)
		if !ok {
			return false, nil, responsecheck.Error(operation, "invalid_pagination_evidence", "服务端 totalCount 必须是整数")
		}
		if totalCount < 0 {
			return false, nil, responsecheck.Error(operation, "invalid_pagination_evidence", "服务端 totalCount 不能为负数")
		}
		extra["totalCount"] = totalCount
		totalCountValue = &totalCount
		if totalCount > 0 && itemCount == 0 {
			return false, nil, responsecheck.Error(operation, "empty_page_with_nonzero_total", "totalCount>0 但当前集合为空，不能证明前进或终止")
		}
		pageStart := (page - 1) * limit
		if pageStart > totalCount || pageStart+itemCount > totalCount {
			return false, nil, responsecheck.Error(operation, "pagination_count_mismatch", "当前页项数与 totalCount/page/limit 矛盾")
		}
		evidence = append(evidence, page*limit >= totalCount)
	}
	if len(evidence) == 0 {
		return false, nil, responsecheck.Error(operation, "missing_pagination_evidence", "分页响应缺少 totalCount/totalPage，无法证明当前页是否完整")
	}
	complete := evidence[0]
	for _, candidate := range evidence[1:] {
		if candidate != complete {
			return false, nil, responsecheck.Error(operation, "conflicting_pagination_evidence", "服务端 totalCount 与 totalPage 对当前页是否完成给出矛盾证据")
		}
	}
	if totalPageValue != nil && totalCountValue != nil {
		expectedPages := 0
		if *totalCountValue > 0 {
			expectedPages = (*totalCountValue + limit - 1) / limit
		}
		if *totalPageValue != expectedPages {
			return false, nil, responsecheck.Error(operation, "pagination_total_mismatch", "totalPage 与 totalCount/limit 矛盾")
		}
	}
	if !complete && itemCount == 0 {
		return false, nil, responsecheck.Error(operation, "pagination_no_progress", "未完成页没有返回任何稳定项，无法安全前进")
	}
	if !complete && itemCount != limit {
		return false, nil, responsecheck.Error(operation, "pagination_short_page", "未完成页返回项数小于 limit，与继续分页证据矛盾")
	}
	if complete && totalCountValue != nil {
		pageStart := (page - 1) * limit
		if pageStart+itemCount != *totalCountValue {
			return false, nil, responsecheck.Error(operation, "pagination_count_mismatch", "终止页项数未精确覆盖 totalCount")
		}
	}
	if !complete {
		extra["nextPage"] = page + 1
	}
	return complete, extra, nil
}

func attendancePageInput(rt *shortcut.RuntimeContext) (int, int, error) {
	page, limit := rt.Int("page"), rt.Int("limit")
	if page < 1 {
		return 0, 0, fmt.Errorf("--page 必须大于等于 1")
	}
	if limit < 1 || limit > 200 {
		return 0, 0, fmt.Errorf("--limit 必须在 1 到 200 之间")
	}
	return page, limit, nil
}

func attendanceInt(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int32:
		return int(number), true
	case int64:
		return int(number), true
	case float64:
		if number != float64(int(number)) {
			return 0, false
		}
		return int(number), true
	default:
		return 0, false
	}
}
