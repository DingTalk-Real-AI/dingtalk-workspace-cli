// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func aitableImportDataResultSpec() *contract.ResultSpec {
	object := AitableNavigationObjectSchema()
	properties := object["properties"].(map[string]any)
	properties["status"] = map[string]any{"type": "string", "description": "导入任务业务状态，与 MCP 调用状态分开"}
	properties["taskId"] = map[string]any{"type": "string", "description": "下游导入任务 ID；继续等待仍使用原 importId"}
	properties["tableIds"] = map[string]any{"type": []string{"array", "null"}, "description": "服务端返回的新增表 ID 候选列表", "items": map[string]any{"type": "string"}}
	object["required"] = []string{"status"}
	schema, _ := json.Marshal(object)
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure},
		DataSchema: aitableResultSchemaWithDryRun("导入任务状态及导航链接", schema)}
}

// executeAitableImportResult distinguishes accepted waiting from completed import;
// it never submits a second task or resolves links by an extra read.
func executeAitableImportResult(cmd *cobra.Command, args map[string]any) error {
	if preview, ok := aitableNavigationDryRunResult(cmd, "import_data", args); ok {
		return output.StoreResult(cmd.Context(), preview)
	}
	data, err := callAitableUnifiedDataContext(cmd.Context(), "import_data", args)
	if err != nil {
		return err
	}
	object, _ := data.(map[string]any)
	switch object["status"] {
	case "success":
		return output.StoreResult(cmd.Context(), output.Success(data))
	case "pending":
		importID := fmt.Sprint(args["importId"])
		next := "dws aitable import data --import-id " + ShellQuoteArg(importID) + " --format json"
		return output.StoreResult(cmd.Context(), output.Pending(data, &output.OperationInfo{ID: importID, State: "pending", NextCommand: next}))
	case "partial_failure":
		started := true
		return output.StoreResult(cmd.Context(), output.Failure(&output.ErrorInfo{
			Type: "api", Subtype: "import_task_partial_failure", Message: "服务端报告导入任务部分失败，请保留原 importId 和回执，核对失败项及目标数据，不要自动重放导入",
			ExecutionStarted: &started, Details: map[string]any{"result": data},
		}))
	case "all_fail", "timeout", "failed", "failure":
		started := true
		return output.StoreResult(cmd.Context(), output.Failure(&output.ErrorInfo{
			Type: "api", Subtype: "import_task_failed", Message: "服务端报告导入任务失败，请核对目标数据后再决定是否重试",
			ExecutionStarted: &started, Details: map[string]any{"result": data},
		}))
	default:
		started := true
		return output.StoreResult(cmd.Context(), output.Failure(&output.ErrorInfo{
			Type: "api", Subtype: "projection_unknown", Message: "导入未返回成功或等待状态，请保留 importId 并核对目标数据",
			ExecutionStarted: &started, Details: map[string]any{"result": data},
		}))
	}
}
