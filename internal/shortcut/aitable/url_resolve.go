// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

// URLResolve parses a documented DingTalk AI Table URL into stable target IDs.
var URLResolve = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aitable",
	Command:       "+url-resolve",
	Product:       serverMain,
	Description:   "解析 AI 表格表、视图、记录、字段、表单、App/Page、Application 和流程 URL",
	Intent:        "已有钉钉 AI 表格链接、需要提取后续命令使用的稳定 ID 时使用；默认只做严格本地解析，--verify 会调用对应只读接口确认目标及所属关系；仅 App 链接缺少只读验证入口时明确拒绝。",
	Risk:          shortcut.RiskRead,
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
		Description: "解析 AI 表格表、视图、记录、字段、表单、App/Page、Application 和流程 URL。",
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "The command performs strict local URL decoding and can optionally verify the deepest target through a read-only AI Table interface.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "解析 AI 表格表、视图、记录、字段、表单、App/Page、Application 和流程 URL",
			UseWhen:      []string{"已有钉钉 AI 表格链接、需要提取后续命令使用的稳定 ID 时使用；默认只做严格本地解析，--verify 会调用对应只读接口确认目标及所属关系；仅 App 链接缺少只读验证入口时明确拒绝。"},
			AvoidWhen:    []string{"只有资源名称而没有 URL 时改用 +resolve-base / +resolve-table；普通文档 URL 用 doc/drive"},
			Examples: []string{
				"dws aitable +url-resolve --url https://alidocs.dingtalk.com/i/nodes/BASE_ID",
				"dws aitable +url-resolve --url 'https://alidocs.dingtalk.com/i/nodes/BASE_ID?iframeQuery=sheetId%3DTABLE_ID%26viewId%3DVIEW_ID' --verify",
			},
		},
		Result: urlResolveResultSpec(),
		Parameters: []contract.ParamDecl{
			{Name: "url", Description: "AI 表格节点或独立记录 URL；使用 Web 的对象定位参数"},
			{Name: "verify", Description: "只读验证最深层对象及所属关系；仅 App 链接不调用可能创建 App 的 get_app"},
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

// urlResolveResultSpec publishes local resolution independently from remote existence.
func urlResolveResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": [
        "resolved"
      ],
      "description": "本次解析或验证状态"
    },
    "verified": {
      "type": "boolean",
      "description": "目标存在且已核对请求中父级关系；不证明页面已定位或写后状态已核验"
    },
    "verificationStatus": {
      "type": "string",
      "enum": [
        "not_requested",
        "verified"
      ],
      "description": "not_requested 表示仅解析；verified 表示只读对象验证通过"
    },
    "target": {
      "type": "object",
      "properties": {
        "schemaVersion": {
          "type": "string",
          "enum": [
            "aitable.target.v1"
          ],
          "description": "URL 目标回执版本"
        },
        "source": {
          "type": "string",
          "enum": [
            "url"
          ],
          "description": "目标来源，固定为 url"
        },
        "kind": {
          "type": "string",
          "enum": [
            "base",
            "table",
            "view",
            "record",
            "field",
            "form",
            "app",
            "app_page",
            "application",
            "workflow"
          ],
          "description": "目标对象类型；application 由验证目录进一步区分"
        },
        "baseId": {
          "type": "string",
          "description": "所属 Base 标识"
        },
        "tableId": {
          "type": "string",
          "description": "所属数据表标识，对应 sheetId"
        },
        "viewId": {
          "type": "string",
          "description": "视图或表单标识"
        },
        "recordId": {
          "type": "string",
          "description": "记录标识，对应 rowId 或 record 复合参数"
        },
        "fieldId": {
          "type": "string",
          "description": "字段标识"
        },
        "entrance": {
          "type": "string",
          "description": "Web 顶部入口"
        },
        "appId": {
          "type": "string",
          "description": "应用模式的编码 App 标识"
        },
        "pageId": {
          "type": "string",
          "description": "应用模式页面标识"
        },
        "applicationId": {
          "type": "string",
          "description": "Base 内仪表盘或说明文档的导航节点 ID；不是底层文档 dentryUuid"
        },
        "flowId": {
          "type": "string",
          "description": "自动化流程标识"
        }
      },
      "required": [
        "schemaVersion",
        "source",
        "kind",
        "baseId"
      ],
      "additionalProperties": false,
      "description": "从原始 URL 解析得到的准确对象标识"
    },
    "verification": {
      "type": "object",
      "description": "只读接口及精确命中的对象；Application 经 list_nsheet_nodes 的 nodeId/nodeType 区分 dashboard/document",
      "properties": {
        "tool": {
          "type": "string",
          "description": "实际完成验证的只读工具"
        },
        "entityType": {
          "type": "string",
          "description": "已验证的对象类型"
        },
        "id": {
          "type": "string",
          "description": "精确命中的对象标识"
        },
        "status": {
          "type": "string",
          "enum": [
            "verified"
          ],
          "description": "本次解析或验证状态"
        }
      },
      "required": [
        "tool",
        "entityType",
        "id",
        "status"
      ],
      "additionalProperties": false
    }
  },
  "required": [
    "status",
    "target",
    "verified",
    "verificationStatus"
  ],
  "additionalProperties": false,
  "description": "本地 URL 解析及可选只读对象验证结果"
}`)}
}
