// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var WorkflowDeploy = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+workflow-deploy",
	Product:     serverMain,
	Description: "创建或更新完整 workflow-dsl/v1，强制检查 valid/flowId，并可启用后验证 RUNNING 状态",
	Intent:      "当你要一次发布并验证自动化工作流时使用；workflow-id 为空则创建，提供则更新，--enable 还会检查 list 中最终运行状态。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: aitableCompositeContract(
		"+workflow-deploy",
		"创建或更新完整 workflow-dsl/v1，强制检查 valid/flowId，并可启用后验证 RUNNING 状态",
		"当你要一次发布并验证自动化工作流时使用；workflow-id 为空则创建，提供则更新，--enable 还会检查 list 中最终运行状态。",
		"只查看定义用 workflow get；只启停已有流程用 enable/disable；创建回包不确定时不要盲目重试",
		`dws aitable +workflow-deploy --base-id B --dsl '{"version":"workflow-dsl/v1","name":"提醒","nodes":[]}' --enable`,
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "workflow-id", Type: shortcut.FlagString, Desc: "已有 Workflow ID；为空表示创建"},
		{Name: "dsl", Type: shortcut.FlagString, Desc: "完整 workflow-dsl/v1 JSON 对象", Required: true},
		{Name: "locale", Type: shortcut.FlagString, Default: "zh-CN", Desc: "发布 locale"},
		{Name: "enable", Type: shortcut.FlagBool, Desc: "发布后启用并从 workflow list 验证 RUNNING"},
	},
	Tips: []string{`dws aitable +workflow-deploy --base-id B --dsl '{"version":"workflow-dsl/v1","name":"提醒","nodes":[]}' --enable`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeWorkflowDeploy(rt)
	},
}

func executeWorkflowDeploy(rt *shortcut.RuntimeContext) error {
	dsl, err := parseJSONObject("dsl", rt.Str("dsl"))
	if err != nil {
		return err
	}
	if stringValue(dsl, "version") != "workflow-dsl/v1" {
		return apperrors.NewValidation("--dsl.version 必须是 workflow-dsl/v1")
	}
	baseID, workflowID := rt.Str("base-id"), strings.TrimSpace(rt.Str("workflow-id"))
	action, tool := "create", "create_workflow"
	params := map[string]any{"baseId": baseID, "dsl": dsl, "locale": rt.Str("locale")}
	if workflowID != "" {
		action, tool = "update", "update_workflow"
		params["workflowId"] = workflowID
		preflight, preflightErr := rt.CallMCPData(serverHelper, "get_workflow", map[string]any{"baseId": baseID, "workflowId": workflowID})
		if preflightErr != nil {
			return preflightErr
		}
		if !deepContainsString(preflight, workflowID) {
			return apperrors.NewValidation("workflow get preflight does not identify --workflow-id", apperrors.WithReason("target_not_found"), apperrors.WithExecutionStarted(false))
		}
	}
	result := newCompositeResult("workflow_deploy")
	result.Resolved = map[string]any{"action": action, "baseId": baseID, "workflowId": workflowID, "enable": rt.Bool("enable")}
	result.Plan = []compositeStep{{Index: 1, Name: action + " workflow", Tool: tool, Status: "planned"}}
	if rt.Bool("enable") {
		result.Plan = append(result.Plan, compositeStep{Index: 2, Name: "enable and verify workflow", Tool: "enable_workflow", Status: "planned"})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, tool, params)
	if writeErr != nil {
		result.Status = "unknown"
		result.Checkpoint = map[string]any{"nextStep": "resolve workflow existence and definition before retrying", "workflowId": workflowID}
		return compositeError(result, writeErr, false)
	}
	valid, validFound, responseID, publishErr := workflowPublishResult(writeData)
	if publishErr != nil {
		result.Status = "rejected"
		result.Result = writeData
		return compositeError(result, publishErr, false)
	}
	if !validFound || !valid || responseID == "" || (workflowID != "" && responseID != workflowID) {
		result.Status = "rejected"
		result.Result = writeData
		return compositeError(result, fmt.Errorf("%s response must contain valid=true and the expected flowId", tool), false)
	}
	workflowID = responseID
	result.Resolved["workflowId"] = workflowID
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": tool, "workflowId": workflowID})
	readBack, verifyErr := rt.CallMCPData(serverHelper, "get_workflow", map[string]any{"baseId": baseID, "workflowId": workflowID})
	if verifyErr != nil || !deepContainsString(readBack, workflowID) {
		if verifyErr == nil {
			verifyErr = fmt.Errorf("get_workflow read-back does not identify workflowId %s", workflowID)
		}
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"workflowId": workflowID, "nextStep": "verify published workflow"}
		return compositeError(result, verifyErr, action == "update")
	}
	if rt.Bool("enable") {
		enableData, enableErr := rt.CallMCPWriteDataStrict(serverHelper, "enable_workflow", map[string]any{"baseId": baseID, "workflowId": workflowID})
		if enableErr != nil {
			result.Warnings = append(result.Warnings, "enable response error: "+enableErr.Error())
		}
		workflow, listErr := readWorkflowFromList(rt, baseID, workflowID)
		if listErr != nil || !workflowIsRunning(workflow) {
			if listErr == nil {
				listErr = fmt.Errorf("workflow list does not show %s as RUNNING", workflowID)
			}
			result.Status = "partial_success"
			result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "enable_workflow", "workflowId": workflowID, "response": enableData})
			result.Checkpoint = map[string]any{"workflowId": workflowID, "nextStep": "inspect and enable workflow"}
			return compositeError(result, listErr, action == "update")
		}
		result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 2, Name: "enable and verify workflow", Tool: "enable_workflow", Status: "completed", Result: workflow})
	}
	result.CompletedCount = 1
	result.CompletedSteps = append([]compositeStep{{Index: 1, Name: action + " workflow", Tool: tool, Status: "completed", Result: writeData}}, result.CompletedSteps...)
	result.Verification = map[string]any{"status": "verified", "valid": true, "workflowId": workflowID, "running": rt.Bool("enable")}
	result.Result = map[string]any{"action": action, "workflowId": workflowID, "valid": true}
	return rt.Output(result)
}

// workflowPublishResult reads only the reviewed response envelopes rather than
// recursively accepting an unrelated nested valid/flowId field. Conflicting
// values are protocol errors; an outer "success" marker never overrides
// valid=false.
func workflowPublishResult(data map[string]any) (valid bool, validFound bool, workflowID string, err error) {
	var visit func(map[string]any) error
	visit = func(object map[string]any) error {
		if raw, exists := object["valid"]; exists {
			value, ok := raw.(bool)
			if !ok {
				return fmt.Errorf("workflow publish valid must be boolean, got %T", raw)
			}
			if validFound && valid != value {
				return fmt.Errorf("workflow publish response contains conflicting valid values")
			}
			valid, validFound = value, true
		}
		for _, key := range []string{"flowId", "workflowId"} {
			raw, exists := object[key]
			if !exists {
				continue
			}
			value, ok := raw.(string)
			value = strings.TrimSpace(value)
			if !ok || value == "" {
				return fmt.Errorf("workflow publish %s must be a non-empty string", key)
			}
			if workflowID != "" && workflowID != value {
				return fmt.Errorf("workflow publish response contains conflicting workflow IDs")
			}
			workflowID = value
		}
		for _, envelope := range []string{"data", "result", "response"} {
			if nested, ok := object[envelope].(map[string]any); ok {
				if err := visit(nested); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if data == nil {
		return false, false, "", fmt.Errorf("workflow publish response is empty")
	}
	if err := visit(data); err != nil {
		return false, false, "", err
	}
	return valid, validFound, workflowID, nil
}

func findBoolByKeys(value any, keys ...string) (bool, bool) {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(key)] = true
	}
	var walk func(any) (bool, bool)
	walk = func(current any) (bool, bool) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if keySet[strings.ToLower(key)] {
					if value, ok := child.(bool); ok {
						return value, true
					}
				}
			}
			for _, child := range typed {
				if value, found := walk(child); found {
					return value, true
				}
			}
		case []any:
			for _, child := range typed {
				if value, found := walk(child); found {
					return value, true
				}
			}
		}
		return false, false
	}
	return walk(value)
}

func readWorkflowFromList(rt *shortcut.RuntimeContext, baseID, workflowID string) (map[string]any, error) {
	for offset := 0; offset < 10000; offset += 100 {
		data, err := rt.CallMCPData(serverHelper, "list_workflows", map[string]any{"baseId": baseID, "limit": 100, "offset": offset})
		if err != nil {
			return nil, err
		}
		workflows, found := findNamedObjectList(data, "workflows", "list", "items")
		if !found {
			return nil, fmt.Errorf("list_workflows response is missing the workflow list")
		}
		for _, workflow := range workflows {
			if stringValue(workflow, "flowId", "workflowId", "id") == workflowID {
				return workflow, nil
			}
		}
		if len(workflows) < 100 {
			return nil, fmt.Errorf("workflow %s is not present in list_workflows", workflowID)
		}
	}
	return nil, fmt.Errorf("workflow list exceeded 10000 entries")
}

func workflowIsRunning(workflow map[string]any) bool {
	status := strings.ToUpper(stringValue(workflow, "status", "state"))
	if status == "RUNNING" || status == "ENABLED" || status == "ACTIVE" {
		return true
	}
	enabled, found := findBoolByKeys(workflow, "enabled", "isEnabled")
	return found && enabled
}
