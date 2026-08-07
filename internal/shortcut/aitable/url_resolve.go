// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

// URLResolve parses a documented DingTalk AI Table URL into stable target IDs.
var URLResolve = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+url-resolve",
	Product:     serverMain,
	Description: "解析 AI 表格 URL 中的 baseId/tableId/viewId/recordId",
	Intent:      "已有钉钉 AI 表格链接、需要提取后续命令使用的稳定 ID 时使用；默认只做严格本地解析，--verify 会调用对应只读接口确认最深层目标真实存在。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_url_resolve",
			CanonicalPath:  "aitable.shortcut_url_resolve",
			CLIPath:        "aitable +url-resolve",
			PrimaryCLIPath: "aitable +url-resolve",
		},
		Description: "解析 AI 表格 URL 中的 baseId/tableId/viewId/recordId。",
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "The command performs strict local URL decoding and can optionally verify the deepest target through a read-only AI Table interface.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "解析 AI 表格 URL 中的 baseId/tableId/viewId/recordId",
			UseWhen:      []string{"已有钉钉 AI 表格链接、需要提取后续命令使用的稳定 ID 时使用；默认只做严格本地解析，--verify 会调用对应只读接口确认最深层目标真实存在。"},
			AvoidWhen:    []string{"只有资源名称而没有 URL 时改用 +resolve-base / +resolve-table；普通文档 URL 用 doc/drive"},
			Examples: []string{
				"dws aitable +url-resolve --url https://alidocs.dingtalk.com/i/nodes/BASE_ID",
				"dws aitable +url-resolve --url 'https://alidocs.dingtalk.com/i/nodes/BASE_ID?iframeQuery=sheetId%3DTABLE_ID%26viewId%3DVIEW_ID' --verify",
			},
		},
		Parameters: []contract.ParamDecl{
			{Name: "url", Description: "钉钉 AI 表格节点 URL"},
			{Name: "verify", Description: "通过只读接口验证最深层目标存在"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "url", Type: shortcut.FlagString, Desc: "AI 表格 URL", Required: true},
		{Name: "verify", Type: shortcut.FlagBool, Default: "false", Desc: "调用只读接口验证最深层目标存在"},
	},
	Tips: []string{
		`dws aitable +url-resolve --url "https://alidocs.dingtalk.com/i/nodes/BASE_ID?iframeQuery=sheetId%3DTABLE_ID"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		target, err := aitabletarget.ParseURL(rt.Str("url"))
		if err != nil {
			return err
		}
		output := map[string]any{
			"status":             "resolved",
			"target":             target,
			"verified":           false,
			"verificationStatus": "not_requested",
		}
		if rt.Bool("verify") {
			verification, err := verifyURLTarget(rt, target)
			if err != nil {
				return err
			}
			output["verified"] = true
			output["verificationStatus"] = "verified"
			output["verification"] = verification
		}
		return rt.Output(output)
	},
}

func verifyURLTarget(rt *shortcut.RuntimeContext, target aitabletarget.Target) (map[string]any, error) {
	var (
		tool       string
		params     map[string]any
		idKeys     []string
		expectedID string
	)
	switch target.Kind {
	case "record":
		tool = "query_records"
		params = map[string]any{"baseId": target.BaseID, "tableId": target.TableID, "recordIds": []string{target.RecordID}}
		idKeys, expectedID = []string{"recordId", "record_id", "id"}, target.RecordID
	case "view":
		tool = "get_views"
		params = map[string]any{"baseId": target.BaseID, "tableId": target.TableID, "viewIds": []string{target.ViewID}}
		idKeys, expectedID = []string{"viewId", "view_id", "id"}, target.ViewID
	case "table":
		tool = "get_tables"
		params = map[string]any{"baseId": target.BaseID, "tableIds": []string{target.TableID}}
		idKeys, expectedID = []string{"tableId", "table_id", "id"}, target.TableID
	default:
		tool = "get_base"
		params = map[string]any{"baseId": target.BaseID}
		idKeys, expectedID = []string{"baseId", "base_id", "id"}, target.BaseID
	}
	data, err := rt.CallMCPData(serverMain, tool, params)
	if err != nil {
		return nil, err
	}
	matched := responseContainsID(data, expectedID, idKeys...)
	if target.Kind == "base" && !matched {
		matched = responseHasAnyKey(data, "baseName", "base_name", "tables", "dashboards")
	}
	if !matched {
		return nil, apperrors.NewAPI(fmt.Sprintf("%s did not prove that %s %q exists", tool, target.Kind, expectedID),
			apperrors.WithOperation("aitable/"+tool),
			apperrors.WithOrigin("mcp"),
			apperrors.WithFailureStage("target_verification"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithRetryable(false),
			apperrors.WithReason("target_verification_failed"),
			apperrors.WithHint("接口返回成功但缺少目标 ID/资源结构，不能把未知响应当作已验证"),
			apperrors.WithDetails(map[string]any{"target": target}),
		)
	}
	return map[string]any{
		"tool":       tool,
		"entityType": target.Kind,
		"id":         expectedID,
		"status":     "verified",
	}, nil
}

func responseContainsID(value any, expected string, keys ...string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			for _, candidateKey := range keys {
				if key == candidateKey {
					if text, ok := child.(string); ok && strings.TrimSpace(text) == expected {
						return true
					}
				}
			}
			if responseContainsID(child, expected, keys...) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if responseContainsID(child, expected, keys...) {
				return true
			}
		}
	}
	return false
}

func responseHasAnyKey(value any, keys ...string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			for _, candidateKey := range keys {
				if key == candidateKey {
					return true
				}
			}
			if responseHasAnyKey(child, keys...) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if responseHasAnyKey(child, keys...) {
				return true
			}
		}
	}
	return false
}
