// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package whiteboard declares strict Shortcut adapters for DingTalk document
// whiteboards. The document card remains owned by doc; this package only reads
// and updates the OpenNodes content of an already identified whiteboard part.
package whiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
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
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

const (
	serverWhiteboard     = whiteboardcore.ServerID
	toolQuery            = whiteboardcore.EmbeddedQueryTool
	toolUpdate           = whiteboardcore.EmbeddedUpdateTool
	toolQueryStandalone  = whiteboardcore.StandaloneQueryTool
	toolUpdateStandalone = whiteboardcore.StandaloneUpdateTool
)

var whiteboardMarshalNodes = json.Marshal

var whiteboardCoordinateTolerance = big.NewRat(1, 2)

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
			"description":"严格校验的文档内嵌或独立白板查询结果",
			"properties":{
				"nodeId":{"type":"string","description":"承载文档或独立白板的稳定节点身份"},
				"partId":{"type":"string","description":"文档内白板的稳定 part 身份"},
				"revision":{"type":"integer","minimum":0,"description":"独立白板当前 revision"},
				"view":{"type":"string","enum":["summary","page","all"],"description":"独立白板实际查询视图"},
				"source":{"type":"object","description":"显式 OpenNodes V1 快照，包含 pages 以及每页的 nodes 数组","additionalProperties":true},
				"resultDownloadUrl":{"type":"string","description":"独立白板大结果下载地址，与 source 互斥"},
				"summary":{"type":"object","description":"服务端完整性计数、字节数与摘要证据","additionalProperties":true},
				"message":{"type":"string","description":"可选服务说明"}
			},
			"required":["nodeId","summary"],
			"oneOf":[
				{"required":["partId","source"],"not":{"anyOf":[{"required":["revision"]},{"required":["view"]}]}},
				{"required":["revision","view"],"not":{"required":["partId"]}}
			],
			"additionalProperties":false
		}`),
		SensitivePaths: []string{"nodeId", "partId", "source.pages.nodes.id", "resultDownloadUrl"},
	}
}

func updateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"文档内嵌或独立白板更新的精简回执与同一稳定目标的精确读回证据；成功结果不重复返回完整白板快照",
			"properties":{
				"nodeId":{"type":"string","description":"承载文档或独立白板的稳定节点身份"},
				"partId":{"type":"string","description":"文档内白板的稳定 part 身份"},
				"pageId":{"type":"string","description":"独立白板实际更新页 ID"},
				"mode":{"type":"string","description":"实际执行的 append 或 overwrite 模式"},
				"expectedRevision":{"type":"integer","minimum":0,"description":"独立白板 dry-run 中的预期 revision"},
				"previousRevision":{"type":"integer","minimum":0,"description":"独立白板提交前 revision"},
				"committedRevision":{"type":"integer","minimum":0,"description":"独立白板提交后 revision"},
				"requestId":{"type":"string","description":"独立白板稳定幂等请求 ID"},
				"verified":{"type":"boolean","description":"远端更新成功并完成独立读回时为 true；dry-run 请求预览为 false"},
				"verifiedNodeCount":{"type":"integer","description":"按请求稳定节点身份读回验证的节点数"},
				"source":{"type":"object","description":"仅 dry-run 返回的请求预览；真实更新成功时省略，避免重复完整白板快照","additionalProperties":true},
				"summary":{"type":"object","description":"内部完整读回校验后的节点数、页数、字节数与摘要证据","additionalProperties":true},
				"receipt":{
					"type":"object",
					"description":"真实终态回执或未执行的 dry-run 预览标记，二者互斥",
					"oneOf":[
						{
							"properties":{
								"message":{"type":"string","minLength":1,"description":"服务端终态说明"},
								"createdNodeIds":{"type":"array","description":"按请求节点顺序返回的真实节点身份；清空时为空数组","items":{"type":"string","minLength":1}},
								"idMap":{"type":"object","description":"请求临时节点身份到真实节点身份的映射；旧版稀疏回执省略时由 CLI 按有序 createdNodeIds 安全重建；清空时为空对象","additionalProperties":{"type":"string","minLength":1}},
								"deletedNodeCount":{"type":"integer","minimum":0,"description":"本次 overwrite 删除的页面自有节点数；append 为零"},
								"pageId":{"type":"string","description":"独立白板实际更新页 ID"},
								"requestId":{"type":"string","description":"独立白板稳定幂等请求 ID"},
								"previousRevision":{"type":"integer","minimum":0,"description":"独立白板提交前 revision"},
								"committedRevision":{"type":"integer","minimum":0,"description":"独立白板提交后 revision"},
								"idempotentReplay":{"type":"boolean","description":"独立白板是否命中幂等重放"}
							},
							"required":["message","createdNodeIds","idMap","deletedNodeCount"],
							"additionalProperties":false
						},
						{
							"properties":{
								"dryRun":{"type":"boolean","const":true,"description":"未执行远端写入的请求预览标记"},
								"executed":{"type":"boolean","const":false,"description":"dry-run 未执行远端写入"}
							},
							"required":["dryRun","executed"],
							"additionalProperties":false
						}
					]
				}
			},
			"required":["nodeId","mode","verified","verifiedNodeCount","summary","receipt"],
			"allOf":[
				{"oneOf":[
					{"required":["partId"],"not":{"anyOf":[{"required":["expectedRevision"]},{"required":["previousRevision"]},{"required":["committedRevision"]},{"required":["requestId"]}]}},
					{"not":{"required":["partId"]},"anyOf":[{"required":["expectedRevision","requestId"]},{"required":["pageId","previousRevision","committedRevision","requestId"]}]}
				]}
			],
			"oneOf":[
				{
					"properties":{
						"verified":{"const":true,"description":"实际更新已完成独立读回校验"},
						"receipt":{"required":["message"],"description":"真实终态写回执"}
					},
					"not":{"required":["source"]}
				},
				{
					"properties":{
						"verified":{"const":false,"description":"预览未执行远端更新或读回"},
						"verifiedNodeCount":{"const":0,"description":"预览没有读回验证节点"},
						"receipt":{"required":["dryRun","executed"],"description":"未执行的 dry-run 预览标记"}
					},
					"required":["source"]
				}
			],
			"additionalProperties":false
		}`),
		SensitivePaths: []string{"nodeId", "partId", "requestId", "source.nodes.id", "receipt.createdNodeIds", "receipt.idMap"},
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

// Query reads either an embedded document whiteboard or a standalone board.
var Query = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "whiteboard",
	Command:       "+query",
	Product:       serverWhiteboard,
	Description:   "严格读取文档内嵌或独立白板的 OpenNodes 快照",
	Intent:        "读取独立白板，或已知文档 nodeId 与白板 partId 时读取内嵌白板；未提供 partId 默认独立白板",
	Risk:          shortcut.RiskRead,
	Safety:        whiteboardReadSafety(),
	Contract: whiteboardContract(
		"+query", "shortcut_query", "严格读取文档内嵌或独立白板的 OpenNodes 快照",
		"Reviewed composite adapter deterministically selects the embedded or standalone tool from explicit part-id presence, then validates identities, revision, pages, nodes and summary completeness.",
		queryResultSpec(), nil,
		[]contract.ParamDecl{
			{Name: "node", Property: "nodeId"},
			{Name: "part-id", Property: "partId"},
			{Name: "view", Property: "view", Enum: []string{"summary", "page", "all"}},
			{Name: "page-id", Property: "pageId", RequiredWhen: "独立白板且 view=page 时"},
		},
		"读取独立白板，或已知文档 nodeId 与白板 partId 时读取内嵌白板；未提供 partId 默认独立白板",
		"创建白板卡片路由到 doc whiteboard insert；Lark 风格 preview/SVG/source 导出当前不可由本命令替代",
		"dws whiteboard +query --node <WHITEBOARD_NODE_ID> --view all",
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "承载文档或独立白板的节点 ID/URL；--node 去除空白后不能为空", Required: true},
		{Name: "part-id", Type: shortcut.FlagString, Desc: "文档内白板 part ID；显式提供时选择内嵌分支。显式非空 --part-id 选择内嵌白板并禁止 view/page-id；未提供时默认独立白板，view=page 必须提供 page-id"},
		{Name: "view", Type: shortcut.FlagString, Default: "summary", Enum: []string{"summary", "page", "all"}, Desc: "独立白板查询视图，默认 summary。显式非空 --part-id 选择内嵌白板并禁止 view/page-id；未提供时默认独立白板，view=page 必须提供 page-id"},
		{Name: "page-id", Type: shortcut.FlagString, Desc: "独立白板页面 ID；view=page 时必填。显式非空 --part-id 选择内嵌白板并禁止 view/page-id；未提供时默认独立白板，view=page 必须提供 page-id", RequiredWhen: "独立白板且 view=page 时"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node"}, Description: "--node 去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"part-id", "view", "page-id"}, Description: "显式非空 --part-id 选择内嵌白板并禁止 view/page-id；未提供时默认独立白板，view=page 必须提供 page-id"},
	},
	Tips: []string{
		"dws whiteboard +query --node <DOC_ID> --part-id <WHITEBOARD_PART_ID>",
		"dws whiteboard +query --node <WHITEBOARD_NODE_ID> --view all",
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, err := whiteboardQueryCall(rt)
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		call, err := whiteboardQueryCall(rt)
		if err != nil {
			return err
		}
		data, err := rt.CallMCPData(serverWhiteboard, call.Tool, call.Args)
		if err != nil {
			return err
		}
		var projected map[string]any
		if call.Kind == whiteboardcore.KindEmbedded {
			projected, err = projectWhiteboardQuery(data, rt.Str("node"), rt.Str("part-id"))
		} else {
			projected, err = projectStandaloneWhiteboardQuery(data, call.Args)
		}
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
	Description:   "确认后更新文档内嵌或独立白板并精确读回",
	Intent:        "已有合规 OpenNodes V1 内容，用户确认后更新白板；显式 partId 选择内嵌分支，未提供时默认独立分支并要求 revision/requestId",
	Risk:          shortcut.RiskHighWrite,
	Safety:        whiteboardWriteSafety(),
	Contract: whiteboardContract(
		"+update", "shortcut_update", "确认后更新文档内嵌或独立白板并精确读回",
		"Reviewed composite adapter selects one write tool before execution, validates OpenNodes locally, requires confirmation, verifies the terminal receipt and request-to-real ID mapping, then reads the same target back exactly without cross-type fallback.",
		updateResultSpec(), &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
		[]contract.ParamDecl{
			{Name: "node", Property: "nodeId"},
			{Name: "part-id", Property: "partId"},
			{Name: "source", Property: "source"},
			{Name: "page-id", Property: "pageId", RequiredWhen: "独立白板 overwrite 时"},
			{Name: "expected-revision", Property: "expectedRevision", RequiredWhen: "操作独立白板时"},
			{Name: "request-id", Property: "requestId", RequiredWhen: "操作独立白板时"},
		},
		"已有合规 OpenNodes V1 内容，用户确认后更新白板；显式 partId 选择内嵌分支，未提供时默认独立分支并要求 revision/requestId",
		"只读使用 whiteboard +query；创建卡片使用 doc whiteboard insert；Mermaid、PlantUML、SVG 和真实节点局部更新当前不可用",
		`dws whiteboard +update --node <WHITEBOARD_NODE_ID> --expected-revision 12 --request-id wb-update-001 --source '{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"sample-shape","type":"shape","x":40,"y":40,"width":120,"height":80,"geometry":"dml:roundRect"}]}}'`,
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "承载文档或独立白板的节点 ID/URL；--node 去除空白后不能为空", Required: true},
		{Name: "part-id", Type: shortcut.FlagString, Desc: "文档内白板 part ID；显式提供时选择内嵌分支。显式非空 part-id 选择内嵌分支并禁止 revision/requestId；未提供 part-id 时独立分支要求 expected-revision/request-id，overwrite 还要求 page-id"},
		{Name: "source", Type: shortcut.FlagString, Desc: "OpenNodes V1 JSON，不能为空；支持字面量、@相对文件或 - 从 stdin 读取", Required: true, Input: []string{"file", "stdin"}},
		{Name: "page-id", Type: shortcut.FlagString, Desc: "目标页面 ID；独立白板 overwrite 时必填。显式非空 part-id 选择内嵌分支并禁止 revision/requestId；未提供 part-id 时独立分支要求 expected-revision/request-id，overwrite 还要求 page-id", RequiredWhen: "独立白板 overwrite 时"},
		{Name: "expected-revision", Type: shortcut.FlagInt, Desc: "独立白板最新 revision；必须为非负整数。显式非空 part-id 选择内嵌分支并禁止 revision/requestId；未提供 part-id 时独立分支要求 expected-revision/request-id，overwrite 还要求 page-id", RequiredWhen: "操作独立白板时"},
		{Name: "request-id", Type: shortcut.FlagString, Desc: "独立白板 1-128 字符稳定幂等请求 ID。显式非空 part-id 选择内嵌分支并禁止 revision/requestId；未提供 part-id 时独立分支要求 expected-revision/request-id，overwrite 还要求 page-id", RequiredWhen: "操作独立白板时"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node"}, Description: "--node 去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"part-id", "expected-revision", "request-id", "page-id"}, Description: "显式非空 part-id 选择内嵌分支并禁止 revision/requestId；未提供 part-id 时独立分支要求 expected-revision/request-id，overwrite 还要求 page-id"},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"source"},
			Description: "--source 不能为空且必须是单一 OpenNodes V1 对象；source.nodes 是含稳定唯一 id 和非空 type 的显式数组，append 模式至少一个节点；connector 仅引用同一请求中的可写节点并满足端点与 routing/waypoints 约束",
		},
	},
	Tips: []string{
		"dws whiteboard +update --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --source @whiteboard.json",
		"dws whiteboard +update --node <WHITEBOARD_NODE_ID> --expected-revision 12 --request-id wb-update-001 --source @whiteboard.json",
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		parsed, err := parseWhiteboardSource(rt.Str("source"))
		if err != nil {
			return err
		}
		_, err = whiteboardUpdateCall(rt, parsed)
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
		call, err := whiteboardUpdateCall(rt, parsed)
		if err != nil {
			return err
		}
		target := call.Args
		if rt.DryRun() {
			preview := map[string]any{
				"nodeId": target["nodeId"], "mode": mode,
				"verified": false, "verifiedNodeCount": 0,
				"source":  map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "nodes": parsed.Nodes},
				"summary": map[string]any{"preview": true}, "receipt": map[string]any{"dryRun": true, "executed": false},
			}
			for _, key := range []string{"partId", "pageId", "expectedRevision", "requestId"} {
				if value, ok := target[key]; ok {
					preview[key] = value
				}
			}
			return rt.Output(preview)
		}

		receiptData, err := rt.CallMCPWriteDataStrict(serverWhiteboard, call.Tool, call.Args)
		if err != nil {
			return err
		}
		if call.Kind == whiteboardcore.KindEmbedded {
			receipt, err := requireWhiteboardUpdateReceipt(receiptData, target, mode, parsed)
			if err != nil {
				return err
			}
			readTarget := map[string]any{"nodeId": target["nodeId"], "partId": target["partId"]}
			readback, err := rt.CallMCPData(serverWhiteboard, toolQuery, readTarget)
			if err != nil {
				return whiteboardCommittedVerificationError(err, target, mode, receipt)
			}
			projected, err := projectWhiteboardQuery(readback, rt.Str("node"), rt.Str("part-id"))
			if err != nil {
				return whiteboardCommittedVerificationError(err, target, mode, receipt)
			}
			if err := verifyWhiteboardUpdate(parsed, projected, receipt.IDMap); err != nil {
				return whiteboardCommittedVerificationError(err, target, mode, receipt)
			}
			return rt.Output(projectWhiteboardUpdateSuccess(target, mode, parsed, projected, receipt))
		}

		receipt, err := requireStandaloneWhiteboardUpdateReceipt(receiptData, call.Args, parsed)
		if err != nil {
			return err
		}
		readArgs := map[string]any{"nodeId": target["nodeId"], "view": "page", "pageId": receipt.PageID}
		readback, err := rt.CallMCPData(serverWhiteboard, toolQueryStandalone, readArgs)
		if err != nil {
			return standaloneWhiteboardCommittedVerificationError(err, call.Args, receipt)
		}
		projected, err := projectStandaloneWhiteboardQuery(readback, readArgs)
		if err != nil {
			return standaloneWhiteboardCommittedVerificationError(err, call.Args, receipt)
		}
		if revision, ok := nonNegativeInt(projected["revision"]); !ok || revision != receipt.CommittedRevision {
			return standaloneWhiteboardCommittedVerificationError(
				responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "readback_revision_mismatch", "读回 revision 与 committedRevision 不一致"),
				call.Args, receipt)
		}
		if err := verifyWhiteboardUpdate(parsed, projected, receipt.IDMap); err != nil {
			return standaloneWhiteboardCommittedVerificationError(err, call.Args, receipt)
		}
		return rt.Output(projectStandaloneWhiteboardUpdateSuccess(call.Args, parsed, projected, receipt))
	},
}

func whiteboardQueryCall(rt *shortcut.RuntimeContext) (whiteboardcore.Call, error) {
	return whiteboardcore.BuildQueryCall(whiteboardcore.QueryOptions{
		Target: whiteboardcore.Target{
			NodeID: rt.Str("node"), PartID: rt.Str("part-id"), PartIDChanged: rt.Changed("part-id"),
		},
		View: rt.Str("view"), ViewChanged: rt.Changed("view"),
		PageID: rt.Str("page-id"), PageIDChanged: rt.Changed("page-id"),
	})
}

func whiteboardUpdateCall(rt *shortcut.RuntimeContext, parsed *parsedUpdate) (whiteboardcore.Call, error) {
	mode := "append"
	if parsed.Overwrite {
		mode = "overwrite"
	}
	return whiteboardcore.BuildUpdateCall(whiteboardcore.UpdateOptions{
		Target: whiteboardcore.Target{
			NodeID: rt.Str("node"), PartID: rt.Str("part-id"), PartIDChanged: rt.Changed("part-id"),
		},
		PageID: rt.Str("page-id"), PageIDChanged: rt.Changed("page-id"),
		ExpectedRevision: rt.Int("expected-revision"), ExpectedRevisionChanged: rt.Changed("expected-revision"),
		RequestID: rt.Str("request-id"), RequestIDChanged: rt.Changed("request-id"),
		Mode: mode, NodesJSON: parsed.NodesJSON,
	})
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
	if err := validateWhiteboardConnectors(nodes); err != nil {
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

func validateWhiteboardConnectors(nodes []map[string]any) error {
	byID := make(map[string]map[string]any, len(nodes))
	for _, node := range nodes {
		id, _ := nonEmptyString(node["id"])
		byID[id] = node
	}
	for index, node := range nodes {
		nodeType, _ := nonEmptyString(node["type"])
		if nodeType != "connector" {
			continue
		}
		path := fmt.Sprintf("--source source.nodes[%d] connector", index)
		for _, field := range []string{"x", "y", "width", "height", "angle", "parentId", "absoluteBounds", "resolvedPath"} {
			if _, present := node[field]; present {
				return apperrors.NewValidation(fmt.Sprintf("%s 不能包含 query-only 或服务端推导字段 %q", path, field))
			}
		}

		startRef, err := validateWhiteboardConnectorEndpoint(node["start"], path+".start", byID)
		if err != nil {
			return err
		}
		endRef, err := validateWhiteboardConnectorEndpoint(node["end"], path+".end", byID)
		if err != nil {
			return err
		}
		if startRef != "" && startRef == endRef {
			return apperrors.NewValidation(path + " 的 start/end 不能引用同一个节点")
		}

		routing, ok := nonEmptyString(node["routing"])
		if !ok || !containsString([]string{"straight", "polyline", "curve", "orthogonal"}, routing) {
			return apperrors.NewValidation(path + ".routing 必须是 straight、polyline、curve 或 orthogonal")
		}
		waypointsValue, hasWaypoints := node["waypoints"]
		if routing == "straight" && hasWaypoints {
			return apperrors.NewValidation(path + ".waypoints 在 straight routing 下禁止提供，包括空数组")
		}
		if routing == "polyline" && !hasWaypoints {
			return apperrors.NewValidation(path + ".waypoints 在 polyline routing 下至少需要一个点")
		}
		if hasWaypoints {
			waypoints, ok := waypointsValue.([]any)
			if !ok || (routing == "polyline" && len(waypoints) == 0) {
				return apperrors.NewValidation(path + ".waypoints 必须是符合 routing 约束的显式点数组")
			}
			for waypointIndex, waypoint := range waypoints {
				if err := validateWhiteboardPoint(waypoint, fmt.Sprintf("%s.waypoints[%d]", path, waypointIndex)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateWhiteboardConnectorEndpoint(value any, path string, nodes map[string]map[string]any) (string, error) {
	endpoint, ok := value.(map[string]any)
	if !ok || len(endpoint) == 0 {
		return "", apperrors.NewValidation(path + " 必须是非空端点对象")
	}
	endpointType, ok := nonEmptyString(endpoint["type"])
	if !ok {
		return "", apperrors.NewValidation(path + ".type 必须是 point 或 node")
	}
	if err := validateWhiteboardConnectorMarker(endpoint["marker"], path+".marker"); err != nil {
		return "", err
	}
	switch endpointType {
	case "point":
		if _, present := endpoint["nodeRef"]; present {
			return "", apperrors.NewValidation(path + " 的 point 端点不能包含 nodeRef")
		}
		if _, present := endpoint["anchor"]; present {
			return "", apperrors.NewValidation(path + " 的 point 端点不能包含 anchor")
		}
		if err := validateWhiteboardPoint(endpoint["point"], path+".point"); err != nil {
			return "", err
		}
		return "", nil
	case "node":
		if _, present := endpoint["point"]; present {
			return "", apperrors.NewValidation(path + " 的 node 端点不能包含 point")
		}
		if _, present := endpoint["resolvedPoint"]; present {
			return "", apperrors.NewValidation(path + ".resolvedPoint 是 query-only 字段，不能回写")
		}
		nodeRef, ok := endpoint["nodeRef"].(map[string]any)
		if !ok || len(nodeRef) == 0 {
			return "", apperrors.NewValidation(path + ".nodeRef 必须是非空对象")
		}
		scope, scopeOK := nonEmptyString(nodeRef["scope"])
		requestID, idOK := nonEmptyString(nodeRef["id"])
		if !scopeOK || scope != "request" || !idOK {
			return "", apperrors.NewValidation(path + `.nodeRef 必须使用 {"scope":"request","id":"<同一请求节点ID>"}`)
		}
		target := nodes[requestID]
		if target == nil {
			return "", apperrors.NewValidation(path + ".nodeRef.id 必须引用同一 source.nodes 请求中的节点")
		}
		targetType, _ := nonEmptyString(target["type"])
		if !containsString([]string{"shape", "text", "stickyNote", "frame", "group", "path"}, targetType) {
			return "", apperrors.NewValidation(path + ".nodeRef 只能引用同一请求中的 shape、text、stickyNote、frame、group 或 path")
		}
		if hidden, _ := target["hidden"].(bool); hidden {
			return "", apperrors.NewValidation(path + ".nodeRef 不能引用 hidden 节点")
		}
		if anchorValue, present := endpoint["anchor"]; present {
			if err := validateWhiteboardConnectorAnchor(anchorValue, path+".anchor"); err != nil {
				return "", err
			}
		}
		return requestID, nil
	default:
		return "", apperrors.NewValidation(path + ".type 必须是 point 或 node")
	}
}

func validateWhiteboardConnectorAnchor(value any, path string) error {
	anchor, ok := value.(map[string]any)
	if !ok || len(anchor) == 0 {
		return apperrors.NewValidation(path + " 必须是非空对象")
	}
	if _, present := anchor["position"]; present {
		return apperrors.NewValidation(path + ".position 是 query-only 字段，不能回写")
	}
	mode, ok := nonEmptyString(anchor["mode"])
	if !ok {
		return apperrors.NewValidation(path + ".mode 必须是 auto 或 fixed")
	}
	switch mode {
	case "auto":
		if _, present := anchor["side"]; present {
			return apperrors.NewValidation(path + ".side 在 auto 模式下禁止提供")
		}
		return nil
	case "fixed":
		side, ok := nonEmptyString(anchor["side"])
		if !ok || !containsString([]string{"top", "right", "bottom", "left"}, side) {
			return apperrors.NewValidation(path + ".side 在 fixed 模式下必须是 top、right、bottom 或 left")
		}
		return nil
	default:
		return apperrors.NewValidation(path + ".mode 必须是 auto 或 fixed")
	}
}

func validateWhiteboardConnectorMarker(value any, path string) error {
	if value == nil {
		return nil
	}
	marker, ok := value.(map[string]any)
	if !ok || len(marker) == 0 {
		return apperrors.NewValidation(path + " 必须是非空对象")
	}
	catalogID, ok := nonEmptyString(marker["catalogId"])
	if !ok || !containsString([]string{"none", "arrow.open", "arrow.filled"}, catalogID) {
		return apperrors.NewValidation(path + ".catalogId 必须是 none、arrow.open 或 arrow.filled")
	}
	return nil
}

func validateWhiteboardPoint(value any, path string) error {
	point, ok := value.(map[string]any)
	if !ok || len(point) == 0 {
		return apperrors.NewValidation(path + " 必须是包含有限 x/y 的点对象")
	}
	for _, axis := range []string{"x", "y"} {
		if _, ok := numericValue(point[axis]); !ok {
			return apperrors.NewValidation(path + "." + axis + " 必须是有限数值")
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

func projectStandaloneWhiteboardQuery(data, request map[string]any) (map[string]any, error) {
	envelope, err := requireWhiteboardSuccess(data, toolQueryStandalone)
	if err != nil {
		return nil, err
	}
	wantedNode, _ := request["nodeId"].(string)
	nodeID, ok := nonEmptyString(envelope["nodeId"])
	if !ok || nodeID != strings.TrimSpace(wantedNode) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "query_target_mismatch", "独立白板响应 nodeId 与请求目标不一致")
	}
	revision, ok := nonNegativeInt(envelope["revision"])
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "invalid_revision", "独立白板响应缺少非负整数 revision")
	}
	wantedView, _ := request["view"].(string)
	view, ok := nonEmptyString(envelope["view"])
	if !ok || view != strings.TrimSpace(wantedView) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "query_view_mismatch", "独立白板响应 view 与请求不一致")
	}
	summary, ok := envelope["resultSummary"].(map[string]any)
	if !ok || len(summary) == 0 {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "malformed_result_summary", "独立白板响应 resultSummary 必须是非空对象")
	}
	out := map[string]any{
		"nodeId": nodeID, "revision": revision, "view": view, "summary": summary,
	}
	if value, present := envelope["resultJson"]; present && value != nil {
		result, decodeErr := decodeResultJSONForTool(value, toolQueryStandalone)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if version, valid := nonEmptyString(result["schemaVersion"]); !valid || version != "1.0" {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "invalid_schema_version", `resultJson.schemaVersion 必须为 "1.0"`)
		}
		if version, valid := nonEmptyString(result["catalogVersion"]); !valid || version != "dml-v1" {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "invalid_catalog_version", `resultJson.catalogVersion 必须为 "dml-v1"`)
		}
		pagesValue, present := result["pages"]
		if !present {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "missing_pages", "resultJson 缺少显式 pages 数组")
		}
		pages, nodes, validateErr := validateWhiteboardPages(pagesValue)
		if validateErr != nil {
			return nil, validateErr
		}
		if validateErr := validateSummary(summary, len(nodes), len(pages)); validateErr != nil {
			return nil, validateErr
		}
		result["pages"] = mapsToAny(pages)
		out["source"] = result
	}
	if rawURL, present := envelope["resultDownloadUrl"]; present {
		url, valid := nonEmptyString(rawURL)
		if !valid {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "malformed_download_url", "resultDownloadUrl 必须是非空字符串")
		}
		if _, hasSource := out["source"]; hasSource {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "conflicting_result_payload", "resultJson 与 resultDownloadUrl 必须互斥")
		}
		out["resultDownloadUrl"] = url
	}
	if view != "summary" {
		_, hasSource := out["source"]
		_, hasURL := out["resultDownloadUrl"]
		if !hasSource && !hasURL {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "missing_result_payload", "page/all 查询缺少 resultJson 或 resultDownloadUrl")
		}
	}
	if message, present := envelope["message"]; present {
		text, valid := message.(string)
		if !valid {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolQueryStandalone, "malformed_message", "响应 message 必须是字符串")
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
	Message          string
	CreatedNodeIDs   []string
	IDMap            map[string]string
	DeletedNodeCount int
}

type standaloneUpdateReceipt struct {
	PageID            string
	RequestID         string
	PreviousRevision  int
	CommittedRevision int
	CreatedNodeIDs    []string
	IDMap             map[string]string
	DeletedNodeCount  int
	IdempotentReplay  bool
	Message           string
}

func requireStandaloneWhiteboardUpdateReceipt(data, request map[string]any, expected *parsedUpdate) (*standaloneUpdateReceipt, error) {
	receipt, err := requireWhiteboardSuccess(data, toolUpdateStandalone)
	if err != nil {
		return nil, err
	}
	wantedNode, _ := request["nodeId"].(string)
	nodeID, ok := nonEmptyString(receipt["nodeId"])
	if !ok || nodeID != strings.TrimSpace(wantedNode) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_target_mismatch", "独立白板写回执 nodeId 与请求目标不一致")
	}
	wantedMode, _ := request["mode"].(string)
	mode, ok := nonEmptyString(receipt["mode"])
	if !ok || mode != wantedMode {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_mode_mismatch", "独立白板写回执 mode 与请求不一致")
	}
	pageID, ok := nonEmptyString(receipt["pageId"])
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "missing_page_id", "独立白板写回执缺少实际 pageId")
	}
	if wantedPage, present := request["pageId"].(string); present && strings.TrimSpace(wantedPage) != pageID {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_page_mismatch", "独立白板写回执 pageId 与请求不一致")
	}
	wantedRequestID, ok := nonEmptyString(request["requestId"])
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "invalid_request_id_state", "独立白板写请求缺少有效的 requestId")
	}
	// requestId was originally input-only in the standalone whiteboard Tool
	// contract. Accept an omitted echo for compatibility with those servers,
	// but reject malformed or mismatched echoes when the field is present.
	requestID := wantedRequestID
	if rawRequestID, present := receipt["requestId"]; present {
		echoedRequestID, valid := nonEmptyString(rawRequestID)
		if !valid {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "malformed_receipt_request_id", "独立白板写回执 requestId 必须是非空字符串")
		}
		if echoedRequestID != wantedRequestID {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_request_mismatch", "独立白板写回执 requestId 与请求幂等键不一致")
		}
		requestID = echoedRequestID
	}
	previous, ok := nonNegativeInt(receipt["previousRevision"])
	wantedRevision, wantedOK := request["expectedRevision"].(int)
	if !ok || !wantedOK || previous != wantedRevision {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_revision_mismatch", "previousRevision 与 expectedRevision 不一致")
	}
	committed, ok := nonNegativeInt(receipt["committedRevision"])
	if !ok || committed < previous {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "invalid_committed_revision", "committedRevision 必须是不小于 previousRevision 的整数")
	}
	created, err := nonEmptyStringArray(receipt["createdNodeIds"], "createdNodeIds")
	if err != nil {
		return nil, err
	}
	if len(created) != len(expected.Nodes) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_count_mismatch", "createdNodeIds 数量与请求节点数不一致")
	}
	idMap := make(map[string]string, len(expected.Nodes))
	idMapValue, present := receipt["idMap"]
	if !present || idMapValue == nil {
		// Older standalone Tool versions returned ordered createdNodeIds but
		// omitted idMap. The response contract defines createdNodeIds in request
		// order, so an exact count lets us reconstruct the same mapping without
		// weakening the subsequent readback verification.
		for index, node := range expected.Nodes {
			requestNodeID, _ := nonEmptyString(node["id"])
			idMap[requestNodeID] = created[index]
		}
	} else {
		explicitIDMap, valid := idMapValue.(map[string]any)
		if !valid {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "malformed_id_map", "idMap 必须是对象、null 或省略")
		}
		for key, raw := range explicitIDMap {
			requestID := strings.TrimSpace(key)
			realID, valid := nonEmptyString(raw)
			if requestID == "" || !valid {
				return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "malformed_id_map", "idMap 含空请求或真实节点身份")
			}
			idMap[requestID] = realID
		}
	}
	if len(idMap) != len(expected.Nodes) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_count_mismatch", "idMap 数量与请求节点数不一致")
	}
	for index, node := range expected.Nodes {
		requestID, _ := nonEmptyString(node["id"])
		if idMap[requestID] != created[index] {
			return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "receipt_identity_mismatch", "idMap 没有按请求顺序精确映射 createdNodeIds")
		}
	}
	deleted, ok := nonNegativeInt(receipt["deletedNodeCount"])
	if !ok || (!expected.Overwrite && deleted != 0) {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "malformed_deleted_count", "deletedNodeCount 必须是非负整数，append 时必须为 0")
	}
	replay, ok := receipt["idempotentReplay"].(bool)
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "malformed_idempotent_replay", "idempotentReplay 必须是布尔值")
	}
	message, ok := nonEmptyString(receipt["message"])
	if !ok {
		return nil, responsecheck.Error(serverWhiteboard+"/"+toolUpdateStandalone, "missing_terminal_receipt", "独立白板成功写响应缺少非空 message 终态说明")
	}
	return &standaloneUpdateReceipt{
		PageID: pageID, RequestID: requestID, PreviousRevision: previous, CommittedRevision: committed,
		CreatedNodeIDs: created, IDMap: idMap, DeletedNodeCount: deleted,
		IdempotentReplay: replay, Message: message,
	}, nil
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
	return &verifiedUpdateReceipt{
		Message:          message,
		CreatedNodeIDs:   created,
		IDMap:            idMap,
		DeletedNodeCount: deleted,
	}, nil
}

func projectWhiteboardUpdateSuccess(target map[string]any, mode string, parsed *parsedUpdate, projected map[string]any, receipt *verifiedUpdateReceipt) map[string]any {
	return map[string]any{
		"nodeId":            target["nodeId"],
		"partId":            target["partId"],
		"mode":              mode,
		"verified":          true,
		"verifiedNodeCount": len(parsed.Nodes),
		"summary":           projected["summary"],
		"receipt":           projectWhiteboardUpdateReceipt(receipt),
	}
}

func projectWhiteboardUpdateReceipt(receipt *verifiedUpdateReceipt) map[string]any {
	return map[string]any{
		"message":          receipt.Message,
		"createdNodeIds":   append([]string{}, receipt.CreatedNodeIDs...),
		"idMap":            cloneStringMap(receipt.IDMap),
		"deletedNodeCount": receipt.DeletedNodeCount,
	}
}

func projectStandaloneWhiteboardUpdateSuccess(request map[string]any, parsed *parsedUpdate, projected map[string]any, receipt *standaloneUpdateReceipt) map[string]any {
	return map[string]any{
		"nodeId":            request["nodeId"],
		"pageId":            receipt.PageID,
		"requestId":         receipt.RequestID,
		"mode":              request["mode"],
		"previousRevision":  receipt.PreviousRevision,
		"committedRevision": receipt.CommittedRevision,
		"verified":          true,
		"verifiedNodeCount": len(parsed.Nodes),
		"summary":           projected["summary"],
		"receipt":           projectStandaloneWhiteboardUpdateReceipt(receipt),
	}
}

func projectStandaloneWhiteboardUpdateReceipt(receipt *standaloneUpdateReceipt) map[string]any {
	return map[string]any{
		"message":           receipt.Message,
		"createdNodeIds":    append([]string{}, receipt.CreatedNodeIDs...),
		"idMap":             cloneStringMap(receipt.IDMap),
		"deletedNodeCount":  receipt.DeletedNodeCount,
		"idempotentReplay":  receipt.IdempotentReplay,
		"previousRevision":  receipt.PreviousRevision,
		"committedRevision": receipt.CommittedRevision,
		"pageId":            receipt.PageID,
		"requestId":         receipt.RequestID,
	}
}

func standaloneWhiteboardCommittedVerificationError(cause error, request map[string]any, receipt *standaloneUpdateReceipt) error {
	failure := apperrors.Error{
		Category: apperrors.CategoryAPI, Operation: serverWhiteboard + "/" + toolUpdateStandalone,
		Origin: "mcp", FailureStage: "verification", Reason: "readback_failed",
	}
	var typed *apperrors.Error
	if errors.As(cause, &typed) {
		failure = *typed
	}
	details := make(map[string]any, len(failure.Details)+9)
	for key, value := range failure.Details {
		details[key] = value
	}
	details["nodeId"] = request["nodeId"]
	details["pageId"] = receipt.PageID
	details["mode"] = request["mode"]
	details["requestId"] = request["requestId"]
	details["previousRevision"] = receipt.PreviousRevision
	details["committedRevision"] = receipt.CommittedRevision
	details["commitState"] = "committed"
	details["verified"] = false
	details["receipt"] = projectStandaloneWhiteboardUpdateReceipt(receipt)
	failure.Message = "独立白板写入已有成功回执，但读回校验失败：" + cause.Error()
	failure.Cause = cause
	failure.Details = details
	apperrors.WithExecutionStarted(true)(&failure)
	apperrors.WithRetryable(false)(&failure)
	failure.RetryAfterSeconds = nil
	failure.NextRetryAt = nil
	failure.Hint = "已提交，停止重提并只读对账；保留 committedRevision 和 requestId，不能获取新 revision 后自动重放旧请求。"
	failure.Actions = []string{
		"使用同一 nodeId/pageId 只读查询并按 idMap/createdNodeIds 对账；暂未读到节点不代表未提交",
		"报告已提交但未验证及读回差异；不得自动重发 append、overwrite 或更换 requestId",
	}
	return &failure
}

// A validated terminal receipt proves the write committed, even when the
// subsequent read fails or disagrees. Preserve that evidence without a full
// board snapshot so callers cannot mistake verification failure for no commit.
func whiteboardCommittedVerificationError(cause error, target map[string]any, mode string, receipt *verifiedUpdateReceipt) error {
	failure := apperrors.Error{
		Category: apperrors.CategoryAPI, Operation: serverWhiteboard + "/" + toolUpdate,
		Origin: "mcp", FailureStage: "verification", Reason: "readback_failed",
	}
	var typed *apperrors.Error
	if errors.As(cause, &typed) {
		failure = *typed
	}
	details := make(map[string]any, len(failure.Details)+6)
	for key, value := range failure.Details {
		details[key] = value
	}
	details["nodeId"] = target["nodeId"]
	details["partId"] = target["partId"]
	details["mode"] = mode
	details["commitState"] = "committed"
	details["verified"] = false
	details["receipt"] = projectWhiteboardUpdateReceipt(receipt)
	failure.Message = "白板写入已有成功回执，但读回校验失败：" + cause.Error()
	failure.Cause = cause
	failure.Details = details
	apperrors.WithExecutionStarted(true)(&failure)
	apperrors.WithRetryable(false)(&failure)
	// Read-side retry advice must never become permission to replay the write.
	failure.RetryAfterSeconds = nil
	failure.NextRetryAt = nil
	failure.Hint = "已提交，停止重提并只读对账；append 会创建新节点，不会修正已有节点，改成 frame 再提交也会重复创建。"
	failure.Actions = []string{
		"保留 details 中的 nodeId/partId、receipt 和原始 Payload；如需核实，仅再 query 同一白板一次，按 idMap/createdNodeIds 对账；暂未读到节点不代表未提交",
		"报告已提交但未验证及读回差异；不得自动重发 append、overwrite 或删除节点；布局修复须另行确认范围和授权",
	}
	return &failure
}

func cloneStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
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
		if !actualOK || !whiteboardNumbersEquivalent(path, expectedNumber, actualNumber) {
			if isWhiteboardCoordinatePath(path) {
				return whiteboardCoordinateMismatch(path)
			}
			return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_mismatch", fmt.Sprintf("%s 数值读回不一致", path))
		}
		return nil
	}
	if expected != actual {
		return responsecheck.Error(serverWhiteboard+"/"+toolUpdate, "readback_field_mismatch", fmt.Sprintf("%s 读回值不一致", path))
	}
	return nil
}

func whiteboardNumbersEquivalent(path string, expected, actual *big.Rat) bool {
	if expected.Cmp(actual) == 0 {
		return true
	}
	if !isWhiteboardCoordinatePath(path) {
		return false
	}
	delta := new(big.Rat).Sub(expected, actual)
	delta.Abs(delta)
	return delta.Cmp(whiteboardCoordinateTolerance) <= 0
}

func isWhiteboardCoordinatePath(path string) bool {
	return strings.HasSuffix(path, ".x") || strings.HasSuffix(path, ".y")
}

func whiteboardCoordinateMismatch(path string) error {
	return apperrors.NewAPI(fmt.Sprintf("%s 坐标读回超出 0.5 像素容差", path),
		apperrors.WithOperation(serverWhiteboard+"/"+toolUpdate),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithReason("readback_field_mismatch"),
		apperrors.WithRetryable(false),
		apperrors.WithHint("坐标 mismatch 不代表未提交；停止重提并只读对账，禁止改成 frame 后再次 append。"),
		apperrors.WithActions("保留成功回执、真实节点 ID 和坐标差异；报告已提交但未验证，布局修复须另行确认范围和授权"),
	)
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
