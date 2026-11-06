// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// aitablePluginTypeMinutes is the only plugin datasource type accepted by plugin-create (AI Minutes).
const aitablePluginTypeMinutes = "minutes"

// aitableDatasourcePluginCreateResultSchema describes the executed plugin-create result.
var aitableDatasourcePluginCreateResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"已创建的插件数据源表；仅表示同步流程已保存、发布并已触发首次同步，不代表同步完成",
  "properties":{
    "pluginType":{"type":"string","description":"插件类型，目前恒为 minutes（AI 听记）"},
    "baseId":{"type":"string","description":"数据源表所在的 Base ID"},
    "tableId":{"type":"string","description":"新建的插件数据源表 ID"},
    "flowId":{"type":"string","description":"为该数据源表保存并发布的同步流程 ID"},
    "documentUrl":{"type":"string","description":"Base 文档访问链接"}
  },
  "required":["tableId","flowId"],
  "additionalProperties":true
}`)

// aitableDatasourcePluginCreateResultSpec declares the plugin-create result for the executed and dry-run envelopes.
func aitableDatasourcePluginCreateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: aitableResultSchemaWithDryRun(
			"插件数据源创建结果，或未执行远端调用的 dry-run 请求预览",
			aitableDatasourcePluginCreateResultSchema),
	}
}

// aitableDatasourcePluginCreateData accepts only a receipt that identifies the created table and flow.
func aitableDatasourcePluginCreateData(data any, args map[string]any) (map[string]any, error) {
	created, _ := data.(map[string]any)
	for _, key := range []string{"tableId", "flowId"} {
		if value, _ := created[key].(string); strings.TrimSpace(value) == "" {
			return nil, aitableDatasourcePluginCreateUnknown(args, "返回值缺少 "+key, nil)
		}
	}
	return created, nil
}

// aitableDatasourcePluginCreateCallError keeps definite failures and marks post-dispatch ambiguity as non-retryable.
func aitableDatasourcePluginCreateCallError(args map[string]any, err error) error {
	var cliErr *CLIError
	if errors.As(err, &cliErr) && cliErr.Code == CodeMCPToolError && json.Valid([]byte(strings.TrimSpace(cliErr.Message))) {
		return err
	}
	unknown := aitableCreateCallError("create_plugin_datasource", args, err)
	var typed *apperrors.Error
	if !errors.As(unknown, &typed) || typed.Reason != "create_outcome_unknown" {
		return unknown
	}
	return aitableDatasourcePluginCreateUnknown(args, "", err)
}

// aitableDatasourcePluginCreateUnknown reports that a table may already exist and must be checked before any retry.
func aitableDatasourcePluginCreateUnknown(args map[string]any, detail string, cause error) error {
	message := "插件数据源创建结果不明；可能已创建成功，禁止重复创建"
	if detail != "" {
		message += "（" + detail + "）"
	}
	opts := []apperrors.Option{
		apperrors.WithReason("create_outcome_unknown"),
		apperrors.WithOperation("aitable.create_plugin_datasource"),
		apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false),
		apperrors.WithHint("先用 table get 只读核对该 Base 是否已出现新的插件数据源表，确认不存在后再决定是否重新创建"),
		apperrors.WithDetails(map[string]any{"baseId": args["baseId"], "pluginType": args["pluginType"]}),
	}
	if cause != nil {
		opts = append(opts, apperrors.WithCause(cause))
	}
	return apperrors.NewAPI(message, opts...)
}

func readAitableDatasourceSourceConfig(ctx context.Context, baseID, tableID string) (string, error) {
	raw, err := callAitableReadToolTextContext(ctx, "get_datasource_config", map[string]any{
		"baseId": baseID, "tableId": tableID,
	})
	if err != nil {
		return "", err
	}
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return "", aitableDatasourceConfigReadError("get_datasource_config 返回的不是合法 JSON 对象")
	}
	config, err := aitableprotocol.DatasourceSourceConfig(payload)
	if err != nil {
		return "", aitableDatasourceConfigReadError(err.Error())
	}
	return config, nil
}

func aitableDatasourceConfigReadError(message string) error {
	return apperrors.NewAPI(message+"；未执行数据源更新，请显式提供完整 --source-config",
		apperrors.WithOperation("aitable/get_datasource_config"),
		apperrors.WithReason("datasource_config_unavailable"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false))
}
