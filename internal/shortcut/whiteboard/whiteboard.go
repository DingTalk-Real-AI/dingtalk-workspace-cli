// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package whiteboard declares strict Shortcut adapters for DingTalk document
// whiteboards. The document card remains owned by doc; this package only reads
// and updates the OpenNodes content of an already identified whiteboard part.
package whiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
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
	serverWhiteboard = "whiteboard"
	toolQuery        = "read_whiteboard_content"
	toolUpdate       = "update_whiteboard"
)

var whiteboardMarshalNodes = json.Marshal

type updateFile struct {
	Overwrite bool        `json:"overwrite"`
	Source    *openSource `json:"source"`
}

type openSource struct {
	SchemaVersion  string          `json:"schemaVersion"`
	CatalogVersion string          `json:"catalogVersion"`
	Nodes          json.RawMessage `json:"nodes"`
}

type parsedUpdate struct {
	Overwrite bool
	NodesJSON string
	Nodes     []map[string]any
}

func whiteboardReadSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func whiteboardWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func queryResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"严格校验且绑定到同一文档与白板 part 的 OpenNodes 快照",
			"properties":{
				"nodeId":{"type":"string","description":"承载白板的稳定文档身份"},
				"partId":{"type":"string","description":"文档内白板的稳定 part 身份"},
				"source":{"type":"object","description":"显式 OpenNodes V1 快照，包含 pages 以及每页的 nodes 数组","additionalProperties":true},
				"summary":{"type":"object","description":"服务端完整性计数、字节数与摘要证据","additionalProperties":true},
				"message":{"type":"string","description":"可选服务说明"}
			},
			"required":["nodeId","partId","source","summary"],
			"additionalProperties":false
		}`),
		SensitivePaths: []string{"nodeId", "partId", "source.pages.nodes.id"},
	}
}

func updateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"白板更新回执与同一稳定目标的精确读回验证",
			"properties":{
				"nodeId":{"type":"string","description":"承载白板的稳定文档身份"},
				"partId":{"type":"string","description":"文档内白板的稳定 part 身份"},
				"mode":{"type":"string","description":"实际执行的 append 或 overwrite 模式"},
				"verified":{"type":"boolean","description":"远端更新成功并完成独立读回时为 true；dry-run 请求预览为 false"},
				"verifiedNodeCount":{"type":"integer","description":"按请求稳定节点身份读回验证的节点数"},
				"source":{"type":"object","description":"更新后的严格 OpenNodes V1 快照","additionalProperties":true},
				"summary":{"type":"object","description":"更新后快照的完整性计数与摘要证据","additionalProperties":true},
				"receipt":{"type":"object","description":"显式 success=true 的下游写回执；最终成功仍以独立读回为准","additionalProperties":true}
			},
			"required":["nodeId","partId","mode","verified","verifiedNodeCount","source","summary","receipt"],
			"additionalProperties":false
		}`),
		SensitivePaths: []string{"nodeId", "partId", "source.pages.nodes.id", "receipt.resultJson.createdNodeIds", "receipt.resultJson.idMap"},
	}
}

func whiteboardContract(command, name, description, interfaceReason string, result *contract.ResultSpec, dryRun *contract.DryRunSpec, params []contract.ParamDecl, useWhen, avoidWhen, example string) corecmd.ContractDecl {
	path := "whiteboard " + command
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "whiteboard", Name: name, CanonicalPath: "whiteboard." + name,
			CLIPath: path, PrimaryCLIPath: path,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       interfaceReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    []string{avoidWhen},
			Examples:     []string{example},
		},
		Parameters: params,
		Result:     result,
		DryRun:     dryRun,
	}
}

// Query reads one already identified DingTalk document whiteboard.
var Query = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "whiteboard",
	Command:       "+query",
	Product:       serverWhiteboard,
	Description:   "严格读取已有文档白板的 OpenNodes 快照",
	Intent:        "已知文档 nodeId 与白板 partId，需要读取节点、页面和服务端完整性摘要，并拒绝未知或畸形结果时",
	Risk:          shortcut.RiskRead,
	Safety:        whiteboardReadSafety(),
	Contract: whiteboardContract(
		"+query", "shortcut_query", "严格读取已有文档白板的 OpenNodes 快照",
		"Reviewed composite adapter validates the whiteboard success envelope, every explicit pages[].nodes collection, stable page/node identities, and summary completeness before unified output.",
		queryResultSpec(), nil,
		[]contract.ParamDecl{
			{Name: "node", Property: "nodeId"},
			{Name: "part-id", Property: "partId"},
		},
		"已知文档 nodeId 与白板 partId，需要读取节点、页面和服务端完整性摘要，并拒绝未知或畸形结果时",
		"创建白板卡片路由到 doc whiteboard insert；Lark 风格 preview/SVG/source 导出当前不可由本命令替代",
		"dws whiteboard +query --node <DOC_ID> --part-id <WHITEBOARD_PART_ID>",
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "承载白板的钉钉文档 ID 或 URL；--node 去除空白后不能为空", Required: true},
		{Name: "part-id", Type: shortcut.FlagString, Desc: "文档内白板 part ID；--part-id 去除空白后不能为空", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node"}, Description: "--node 去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"part-id"}, Description: "--part-id 去除空白后不能为空"},
	},
	Tips: []string{"dws whiteboard +query --node <DOC_ID> --part-id <WHITEBOARD_PART_ID>"},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateWhiteboardTarget(rt)
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData(serverWhiteboard, toolQuery, whiteboardTarget(rt))
		if err != nil {
			return err
		}
		projected, err := projectWhiteboardQuery(data, rt.Str("node"), rt.Str("part-id"))
		if err != nil {
			return err
		}
		return rt.Output(projected)
	},
}

// Update appends or overwrites verified OpenNodes on one stable whiteboard.
var Update = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "whiteboard",
	Command:       "+update",
	Product:       serverWhiteboard,
	Description:   "确认后更新白板并按同一稳定目标精确读回",
	Intent:        "已有合规 OpenNodes V1 内容，用户确认后要 append 或 overwrite，并要求按请求节点身份验证同一白板读回时",
	Risk:          shortcut.RiskHighWrite,
	Safety:        whiteboardWriteSafety(),
	Contract: whiteboardContract(
		"+update", "shortcut_update", "确认后更新白板并按同一稳定目标精确读回",
		"Reviewed composite adapter validates OpenNodes locally, requires confirmation, verifies the terminal receipt and request-to-real ID mapping, then reads the same target back exactly.",
		updateResultSpec(), &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
		[]contract.ParamDecl{
			{Name: "node", Property: "nodeId"},
			{Name: "part-id", Property: "partId"},
			{Name: "source", Property: "source"},
		},
		"已有合规 OpenNodes V1 内容，用户确认后要 append 或 overwrite，并要求按请求节点身份验证同一白板读回时",
		"只读使用 whiteboard +query；创建卡片使用 doc whiteboard insert；Mermaid、PlantUML、SVG 和真实节点局部更新当前不可用",
		`dws whiteboard +update --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --source '{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"sample-shape","type":"shape","x":40,"y":40,"width":120,"height":80,"geometry":"dml:roundRect"}]}}'`,
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "承载白板的钉钉文档 ID 或 URL；--node 去除空白后不能为空", Required: true},
		{Name: "part-id", Type: shortcut.FlagString, Desc: "文档内白板 part ID；--part-id 去除空白后不能为空", Required: true},
		{Name: "source", Type: shortcut.FlagString, Desc: "OpenNodes V1 JSON，不能为空；支持字面量、@相对文件或 - 从 stdin 读取", Required: true, Input: []string{"file", "stdin"}},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node"}, Description: "--node 去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"part-id"}, Description: "--part-id 去除空白后不能为空"},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"source"},
			Description: "--source 不能为空且必须是单一 OpenNodes V1 对象；source.nodes 是含稳定唯一 id 和非空 type 的显式数组，append 模式至少一个节点",
		},
	},
	Tips: []string{"dws whiteboard +update --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --source @whiteboard.json"},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := validateWhiteboardTarget(rt); err != nil {
			return err
		}
		_, err := parseWhiteboardSource(rt.Str("source"))
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		parsed, err := parseWhiteboardSource(rt.Str("source"))
		if err != nil {
			return err
		}
		mode := "append"
		if parsed.Overwrite {
			mode = "overwrite"
		}
		target := whiteboardTarget(rt)
		request := map[string]any{
			"nodeId": target["nodeId"], "partId": target["partId"],
			"mode": mode, "nodes": parsed.NodesJSON,
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"nodeId": target["nodeId"], "partId": target["partId"], "mode": mode,
				"verified": false, "verifiedNodeCount": 0,
				"source":  map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "nodes": parsed.Nodes, "pages": []any{}},
				"summary": map[string]any{"preview": true}, "receipt": map[string]any{"dryRun": true, "executed": false},
			})
		}

		receiptData, err := rt.CallMCPWriteDataStrict(serverWhiteboard, toolUpdate, request)
		if err != nil {
			return err
		}
		receipt, err := requireWhiteboardUpdateReceipt(receiptData, target, mode, parsed)
		if err != nil {
			return err
		}
		readback, err := rt.CallMCPData(serverWhiteboard, toolQuery, target)
		if err != nil {
			return err
		}
		projected, err := projectWhiteboardQuery(readback, rt.Str("node"), rt.Str("part-id"))
		if err != nil {
			return err
		}
		if err := verifyWhiteboardUpdate(parsed, projected, receipt.IDMap); err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"nodeId": target["nodeId"], "partId": target["partId"], "mode": mode,
			"verified": true, "verifiedNodeCount": len(parsed.Nodes),
			"source": projected["source"], "summary": projected["summary"], "receipt": receipt.Envelope,
		})
	},
}

func validateWhiteboardTarget(rt *shortcut.RuntimeContext) error {
	if strings.TrimSpace(rt.Str("node")) == "" {
		return apperrors.NewValidation("--node 去除空白后不能为空")
	}
	if strings.TrimSpace(rt.Str("part-id")) == "" {
		return apperrors.NewValidation("--part-id 去除空白后不能为空")
	}
	return nil
}

func whiteboardTarget(rt *shortcut.RuntimeContext) map[string]any {
	return map[string]any{"nodeId": rt.Str("node"), "partId": rt.Str("part-id")}
}

func parseWhiteboardSource(raw string) (*parsedUpdate, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, apperrors.NewValidation("--source 必须是非空 OpenNodes V1 JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var input updateFile
	if err := decoder.Decode(&input); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--source 不是合法 OpenNodes V1 JSON: %v", err))
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--source 不是单一 JSON 对象: %v", err))
	}
	if input.Source == nil {
		return nil, apperrors.NewValidation("--source 缺少 source 对象")
	}
	if input.Source.SchemaVersion != "1.0" {
		return nil, apperrors.NewValidation(`--source source.schemaVersion 必须为 "1.0"`)
	}
	if input.Source.CatalogVersion != "dml-v1" {
		return nil, apperrors.NewValidation(`--source source.catalogVersion 必须为 "dml-v1"`)
	}
	nodes, err := decodeNodeArray(input.Source.Nodes, "--source source.nodes")
	if err != nil {
		return nil, err
	}
	if !input.Overwrite && len(nodes) == 0 {
		return nil, apperrors.NewValidation("append 模式至少需要一个 source.nodes 节点")
	}
	encoded, err := whiteboardMarshalNodes(nodes)
	if err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("编码 OpenNodes 请求失败: %v", err))
	}
	return &parsedUpdate{Overwrite: input.Overwrite, NodesJSON: string(encoded), Nodes: nodes}, nil
}

func decodeNodeArray(raw json.RawMessage, field string) ([]map[string]any, error) {
	if len(raw) == 0 {
		return nil, apperrors.NewValidation(field + " 必须是显式数组")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var values []any
	if err := decoder.Decode(&values); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s 必须是数组: %v", field, err))
	}
	if values == nil {
		return nil, apperrors.NewValidation(field + " 不能为 null")
	}
	nodes := make([]map[string]any, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		node, ok := value.(map[string]any)
		if !ok || len(node) == 0 {
			return nil, apperrors.NewValidation(fmt.Sprintf("%s[%d] 必须是非空对象", field, index))
		}
		id, ok := nonEmptyString(node["id"])
		if !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("%s[%d].id 必须是非空稳定身份", field, index))
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, apperrors.NewValidation(fmt.Sprintf("%s 含重复节点 id %q", field, id))
		}
		seen[id] = struct{}{}
		if _, ok := nonEmptyString(node["type"]); !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("%s[%d].type 必须是非空字符串", field, index))
		}
		nodes[index] = node
	}
	return nodes, nil
}

func projectWhiteboardQuery(data map[string]any, nodeID, partID string) (map[string]any, error) {
	envelope, err := requireWhiteboardSuccess(data, toolQuery)
	if err != nil {
		return nil, err
	}
	value, present := envelope["resultJson"]
	if !present || value == nil {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_result_json", "成功响应缺少非空 resultJson")
	}
	result, err := decodeResultJSON(value)
	if err != nil {
		return nil, err
	}
	if version, ok := nonEmptyString(result["schemaVersion"]); !ok || version != "1.0" {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "invalid_schema_version", `resultJson.schemaVersion 必须为 "1.0"`)
	}
	if version, ok := nonEmptyString(result["catalogVersion"]); !ok || version != "dml-v1" {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "invalid_catalog_version", `resultJson.catalogVersion 必须为 "dml-v1"`)
	}
	pagesValue, present := result["pages"]
	if !present {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_pages", "resultJson 缺少显式 pages 数组")
	}
	pages, nodes, err := validateWhiteboardPages(pagesValue)
	if err != nil {
		return nil, err
	}
	summaryValue, present := envelope["resultSummary"]
	if !present {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_result_summary", "成功响应缺少 resultSummary 完整性证据")
	}
	summary, ok := summaryValue.(map[string]any)
	if !ok || len(summary) == 0 {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_result_summary", "resultSummary 必须是非空对象")
	}
	if err := validateSummary(summary, len(nodes), len(pages)); err != nil {
		return nil, err
	}
	result["pages"] = mapsToAny(pages)
	out := map[string]any{
		"nodeId": strings.TrimSpace(nodeID), "partId": strings.TrimSpace(partID),
		"source": result, "summary": summary,
	}
	if message, present := envelope["message"]; present {
		text, ok := message.(string)
		if !ok {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_message", "响应 message 必须是字符串")
		}
		out["message"] = text
	}
	return out, nil
}

func requireWhiteboardSuccess(data map[string]any, tool string) (map[string]any, error) {
	envelope, err := responsecheck.RequireSuccess(data, serverWhiteboard+"/"+tool)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"error", "errorMessage", "errorMsg"} {
		if text, ok := envelope[key].(string); ok && strings.TrimSpace(text) != "" {
			return nil, responsecheck.Error(serverWhiteboard+"/"+tool, "conflicting_error", "响应同时声明 success=true 与非空 "+key)
		}
	}
	return envelope, nil
}

type verifiedUpdateReceipt struct {
	Envelope map[string]any
	Result   map[string]any
	IDMap    map[string]string
}

func requireWhiteboardUpdateReceipt(data, target map[string]any, mode string, expected *parsedUpdate) (*verifiedUpdateReceipt, error) {
	receipt, err := requireWhiteboardSuccess(data, toolUpdate)
	if err != nil {
		return nil, err
	}
	for _, field := range []string{"nodeId", "partId"} {
		value, ok := nonEmptyString(receipt[field])
		wanted, _ := target[field].(string)
		if !ok || value != strings.TrimSpace(wanted) {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "receipt_target_mismatch", "写回执的 "+field+" 与请求稳定目标不一致")
		}
	}
	value, present := receipt["resultJson"]
	if !present || value == nil {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "missing_terminal_receipt", "成功写响应缺少非空 resultJson 终态回执；远端效果未知")
	}
	result, err := decodeResultJSONForTool(value, toolUpdate)
	if err != nil {
		return nil, err
	}
	receiptMode, ok := nonEmptyString(result["mode"])
	if !ok || receiptMode != mode {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "receipt_mode_mismatch", "写回执 mode 与请求不一致")
	}
	message, ok := nonEmptyString(result["message"])
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "missing_terminal_receipt", "成功写响应缺少非空 resultJson.message 终态回执；远端效果未知")
	}
	result["message"] = message
	created, err := nonEmptyStringArray(result["createdNodeIds"], "resultJson.createdNodeIds")
	if err != nil {
		return nil, err
	}
	if len(created) != len(expected.Nodes) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "receipt_count_mismatch", "createdNodeIds 数量与请求节点数不一致")
	}
	idMapValue, ok := result["idMap"].(map[string]any)
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "malformed_id_map", "resultJson.idMap 必须是显式对象")
	}
	idMap := make(map[string]string, len(idMapValue))
	for key, raw := range idMapValue {
		requestID := strings.TrimSpace(key)
		realID, valid := nonEmptyString(raw)
		if requestID == "" || !valid {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "malformed_id_map", "resultJson.idMap 含空请求或真实节点身份")
		}
		idMap[requestID] = realID
	}
	if len(idMap) != len(expected.Nodes) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "receipt_count_mismatch", "idMap 数量与请求节点数不一致")
	}
	for index, node := range expected.Nodes {
		requestID, _ := nonEmptyString(node["id"])
		if idMap[requestID] != created[index] {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "receipt_identity_mismatch", "idMap 没有按请求顺序精确映射 createdNodeIds")
		}
	}
	deleted, ok := nonNegativeInt(result["deletedNodeCount"])
	if !ok || (!expected.Overwrite && deleted != 0) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "malformed_deleted_count", "deletedNodeCount 必须是非负整数，append 时必须为 0")
	}
	receipt["resultJson"] = result
	return &verifiedUpdateReceipt{Envelope: receipt, Result: result, IDMap: idMap}, nil
}

func decodeResultJSON(value any) (map[string]any, error) {
	return decodeResultJSONForTool(value, toolQuery)
}

func decodeResultJSONForTool(value any, tool string) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return nil, responsecheck.Error(serverWhiteboard+"/"+tool, "empty_result_json", "resultJson 对象为空")
		}
		return typed, nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, responsecheck.Error(serverWhiteboard+"/"+tool, "empty_result_json", "resultJson 字符串为空")
		}
		decoder := json.NewDecoder(strings.NewReader(typed))
		decoder.UseNumber()
		var result map[string]any
		if err := decoder.Decode(&result); err != nil {
			return nil, responsecheck.Error(serverWhiteboard+"/"+tool, "invalid_result_json", fmt.Sprintf("resultJson 不是合法 JSON 对象: %v", err))
		}
		if err := requireJSONEOF(decoder); err != nil || len(result) == 0 {
			return nil, responsecheck.Error(serverWhiteboard+"/"+tool, "invalid_result_json", "resultJson 必须是单一非空 JSON 对象")
		}
		return result, nil
	default:
		return nil, responsecheck.Error(serverWhiteboard+"/"+tool, "malformed_result_json", fmt.Sprintf("resultJson 应为对象或 JSON 字符串，实际为 %T", value))
	}
}

func validateWhiteboardPages(value any) ([]map[string]any, []map[string]any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_collection", fmt.Sprintf("resultJson.pages 必须是数组，实际为 %T", value))
	}
	if len(raw) == 0 {
		return nil, nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_page", "单页白板必须显式返回一个 page")
	}
	pages := make([]map[string]any, len(raw))
	nodes := make([]map[string]any, 0)
	pageIDs := make(map[string]struct{}, len(raw))
	nodeIDs := make(map[string]struct{})
	for pageIndex, item := range raw {
		page, ok := item.(map[string]any)
		if !ok || len(page) == 0 {
			return nil, nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_item", fmt.Sprintf("resultJson.pages[%d] 必须是非空对象", pageIndex))
		}
		pageID, ok := nonEmptyString(page["id"])
		if !ok {
			return nil, nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_page_identity", fmt.Sprintf("resultJson.pages[%d].id 必须是非空稳定身份", pageIndex))
		}
		if _, duplicate := pageIDs[pageID]; duplicate {
			return nil, nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "duplicate_page_identity", "resultJson.pages 含重复 page id")
		}
		pageIDs[pageID] = struct{}{}
		nodesValue, present := page["nodes"]
		if !present {
			return nil, nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_nodes", fmt.Sprintf("resultJson.pages[%d] 缺少显式 nodes 数组", pageIndex))
		}
		pageNodes, err := whiteboardNodeArray(nodesValue, fmt.Sprintf("resultJson.pages[%d].nodes", pageIndex), nodeIDs)
		if err != nil {
			return nil, nil, err
		}
		page["id"] = pageID
		page["nodes"] = mapsToAny(pageNodes)
		pages[pageIndex] = page
		nodes = append(nodes, pageNodes...)
	}
	return pages, nodes, nil
}

func whiteboardNodeArray(value any, field string, seen map[string]struct{}) ([]map[string]any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_collection", fmt.Sprintf("%s 必须是数组，实际为 %T", field, value))
	}
	items := make([]map[string]any, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_item", fmt.Sprintf("%s[%d] 必须是非空对象", field, index))
		}
		id, ok := nonEmptyString(object["id"])
		if !ok {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_node_identity", fmt.Sprintf("%s[%d].id 必须是非空稳定身份", field, index))
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "duplicate_node_identity", "resultJson.pages 跨页含重复节点 id")
		}
		seen[id] = struct{}{}
		if _, ok := nonEmptyString(object["type"]); !ok {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_node_type", fmt.Sprintf("%s[%d].type 必须是非空字符串", field, index))
		}
		object["id"] = id
		items[index] = object
	}
	return items, nil
}

func nonEmptyStringArray(value any, field string) ([]string, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "malformed_collection", field+" 必须是显式数组")
	}
	result := make([]string, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, item := range raw {
		id, ok := nonEmptyString(item)
		if !ok {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "malformed_item", fmt.Sprintf("%s[%d] 必须是非空身份", field, index))
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "duplicate_node_identity", field+" 含重复节点身份")
		}
		seen[id] = struct{}{}
		result[index] = id
	}
	return result, nil
}

func validateSummary(summary map[string]any, nodeCount, pageCount int) error {
	for key, expected := range map[string]int{"nodeCount": nodeCount, "pageCount": pageCount} {
		value, ok := nonNegativeInt(summary[key])
		if !ok || value != expected {
			return responsecheck.Error(serverWhiteboard+"/"+toolQuery, "summary_count_mismatch", fmt.Sprintf("resultSummary.%s 必须等于显式数组长度 %d", key, expected))
		}
	}
	readOnly, ok := nonNegativeInt(summary["readOnlyNodeCount"])
	if !ok || readOnly > nodeCount {
		return responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_read_only_count", "resultSummary.readOnlyNodeCount 必须是不超过 nodeCount 的非负整数")
	}
	unknown, ok := nonNegativeInt(summary["unknownNodeCount"])
	if !ok || unknown > nodeCount {
		return responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_unknown_count", "resultSummary.unknownNodeCount 必须是不超过 nodeCount 的非负整数")
	}
	if _, ok := nonNegativeInt(summary["resultBytes"]); !ok {
		return responsecheck.Error(serverWhiteboard+"/"+toolQuery, "malformed_result_bytes", "resultSummary.resultBytes 必须是非负整数")
	}
	if _, ok := nonEmptyString(summary["resultSha256"]); !ok {
		return responsecheck.Error(serverWhiteboard+"/"+toolQuery, "missing_result_digest", "resultSummary.resultSha256 必须是非空字符串")
	}
	return nil
}

func verifyWhiteboardUpdate(expected *parsedUpdate, projected map[string]any, idMap map[string]string) error {
	source, ok := projected["source"].(map[string]any)
	if !ok {
		return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "missing_readback_source", "更新后读回缺少 OpenNodes source")
	}
	_, readback, err := validateWhiteboardPages(source["pages"])
	if err != nil {
		return err
	}
	byID := make(map[string]map[string]any, len(readback))
	for _, node := range readback {
		id, _ := nonEmptyString(node["id"])
		byID[id] = node
	}
	if expected.Overwrite && whiteboardPageOwnedNodeCount(readback) != len(expected.Nodes) {
		return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "overwrite_count_mismatch", "overwrite 后读回节点数量与请求不一致")
	}
	for _, requested := range expected.Nodes {
		requestID, _ := nonEmptyString(requested["id"])
		realID := idMap[requestID]
		actual := byID[realID]
		if actual == nil {
			return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_identity_mismatch", fmt.Sprintf("更新后未按回执真实身份读回请求节点 %q", requestID))
		}
		requestedType, _ := nonEmptyString(requested["type"])
		actualType, _ := nonEmptyString(actual["type"])
		if requestedType != actualType {
			return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_type_mismatch", fmt.Sprintf("节点 %q 的 type 读回不一致", requestID))
		}
		critical := make(map[string]any, len(requested)-1)
		for key, value := range requested {
			if key != "id" {
				critical[key] = normalizeRequestedReadback(value, idMap, key)
			}
		}
		if err := requireRequestedValue(critical, actual, "node "+requestID); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRequestedReadback(value any, idMap map[string]string, field string) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		requestScope, isRequestRef := typed["scope"].(string)
		requestID, hasRequestID := nonEmptyString(typed["id"])
		for key, child := range typed {
			normalized[key] = normalizeRequestedReadback(child, idMap, key)
		}
		if isRequestRef && requestScope == "request" && hasRequestID {
			if realID := idMap[requestID]; realID != "" {
				normalized["scope"] = "document"
				normalized["id"] = realID
			}
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for index, child := range typed {
			normalized[index] = normalizeRequestedReadback(child, idMap, field)
		}
		return normalized
	case string:
		if field == "parentId" {
			if realID := idMap[typed]; realID != "" {
				return realID
			}
		}
	}
	return value
}

func whiteboardPageOwnedNodeCount(nodes []map[string]any) int {
	count := 0
	for _, node := range nodes {
		if source, ok := nonEmptyString(node["source"]); ok && source == "master" {
			continue
		}
		count++
	}
	return count
}

func requireRequestedValue(expected, actual any, path string) error {
	switch wanted := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_mismatch", fmt.Sprintf("%s 读回类型不一致", path))
		}
		for key, value := range wanted {
			readback, present := got[key]
			if !present {
				return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_missing", fmt.Sprintf("%s.%s 未读回", path, key))
			}
			if err := requireRequestedValue(value, readback, path+"."+key); err != nil {
				return err
			}
		}
		return nil
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(wanted) {
			return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_mismatch", fmt.Sprintf("%s 数组读回不一致", path))
		}
		for index := range wanted {
			if err := requireRequestedValue(wanted[index], got[index], fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
		return nil
	}
	if expectedNumber, expectedOK := numericValue(expected); expectedOK {
		actualNumber, actualOK := numericValue(actual)
		if !actualOK || expectedNumber.Cmp(actualNumber) != 0 {
			return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_mismatch", fmt.Sprintf("%s 数值读回不一致", path))
		}
		return nil
	}
	if expected != actual {
		return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_mismatch", fmt.Sprintf("%s 读回值不一致", path))
	}
	return nil
}

func numericValue(value any) (*big.Rat, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, ok := new(big.Rat).SetString(typed.String())
		return parsed, ok
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(typed), true
	case int:
		return new(big.Rat).SetInt64(int64(typed)), true
	default:
		return nil, false
	}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("multiple JSON values are not allowed")
	} else if !errorsIsEOF(err) {
		return err
	}
	return nil
}

func errorsIsEOF(err error) bool { return err == io.EOF }

func nonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

func nonNegativeInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case float64:
		if typed < 0 || typed != math.Trunc(typed) || math.IsNaN(typed) || math.IsInf(typed, 0) || typed > float64(math.MaxInt) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		if err != nil || parsed < 0 || parsed > int64(math.MaxInt) {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func mapsToAny(values []map[string]any) []any {
	out := make([]any, len(values))
	for index := range values {
		out[index] = values[index]
	}
	return out
}

func init() {
	shortcut.Register(Query, Update)
}
