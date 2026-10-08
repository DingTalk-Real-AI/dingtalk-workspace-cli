// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var aitableAPIKeyCreateSchema = json.RawMessage(`{
  "type":"object",
  "description":"创建成功的 API Key",
  "properties":{
    "keyId":{"type":"string","description":"Key 标识，小写规范 UUID"},
    "status":{"type":"string","const":"ACTIVE","description":"有效状态"},
    "createdAt":{"type":"integer","description":"创建时间，毫秒时间戳"},
    "apiKey":{"type":"string","description":"仅创建成功时返回一次的完整凭据"}
  },
  "required":["keyId","status","createdAt","apiKey"],
  "additionalProperties":false
}`)

var aitableAPIKeyListSchema = json.RawMessage(`{
  "type":"object","description":"当前 Base 的有效凭据元数据",
  "properties":{
    "keys":{
      "type":"array","maxItems":1,
      "description":"当前有效 Key 元数据，空数组表示无有效 Key",
      "items":{
        "type":"object","description":"有效凭据的非秘密元数据",
        "properties":{
          "keyId":{"type":"string","description":"Key 标识，小写规范 UUID"},
          "status":{"type":"string","const":"ACTIVE","description":"有效状态"},
          "createdAt":{"type":"integer","description":"创建时间，毫秒时间戳"},
          "createdBy":{
            "type":"object","description":"可选外部创建者身份",
            "properties":{
              "userId":{"type":"string","description":"外部用户 ID"},
              "corpId":{"type":"string","description":"外部组织 ID"}
            },
            "required":["userId","corpId"],
            "additionalProperties":false
          }
        },
        "required":["keyId","status","createdAt"],
        "additionalProperties":false
      }
    }
  },
  "required":["keys"],
  "additionalProperties":false
}`)

var aitableAPIKeyRevokeSchema = json.RawMessage(`{
  "type":"object","description":"指定凭据的撤销结果",
  "properties":{
    "keyId":{"type":"string","description":"撤销的 Key 标识"},
    "revoked":{"type":"boolean","const":true,"description":"撤销完成，重复撤销同样为 true"}
  },
  "required":["keyId","revoked"],
  "additionalProperties":false
}`)

func newAitableAPIKeyCommand() *cobra.Command {
	group := newGroupCommand(&cobra.Command{Use: "api-key", Short: "管理 SQL Sheet API Key", RunE: groupRunE})
	group.AddCommand(newAitableAPIKeyCreateCommand(), newAitableAPIKeyListCommand(), newAitableAPIKeyRevokeCommand())
	return group
}

func newAitableAPIKeyCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "create",
		Short:         "创建 SQL Sheet API Key",
		Long:          "为指定 Base 创建 SQL Sheet API Key，要求管理权限（MANAGER）。完整凭据仅创建时返回一次；已有有效凭据时创建失败。结果未知时先用 api-key list 核对，不自动重试。",
		Example:       "dws aitable api-key create --base-id <BASE_ID> --format json",
		Server:        "aitable",
		Tool:          "create_sql_sheet_api_key",
		Flags:         []LeafFlag{{Name: "base-id", Bind: "baseId", Required: true, Trim: true, Usage: "AI 表格 Base ID"}},
		Safety:        contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "create_sql_sheet_api_key", CanonicalPath: "aitable.create_sql_sheet_api_key", CLIPath: "aitable api-key create", PrimaryCLIPath: "aitable api-key create"},
			Description: "为指定 Base 创建 SQL Sheet API Key，完整凭据仅创建时返回一次。",
			Interface:   aitableMCPInterface("create_sql_sheet_api_key"),
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema:     aitableResultSchemaWithDryRun("创建 SQL Sheet API Key 结果或请求预览", aitableAPIKeyCreateSchema),
				SensitivePaths: []string{"apiKey"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建 SQL Sheet API Key。",
				UseWhen:      []string{"用户需要为指定 Base 创建 SQL Sheet 访问凭据时"},
				AvoidWhen:    []string{"查询有效凭据用 api-key list；替换凭据须先确认并撤销旧 keyId"},
				Examples:     []string{"dws aitable api-key create --base-id <BASE_ID> --format json"},
			},
		},
		ResultCall: callAitableAPIKeyResult,
	})
}

func newAitableAPIKeyListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "list",
		Short:         "查询 SQL Sheet API Key 元数据",
		Long:          "查询指定 Base 的有效 SQL Sheet API Key，要求管理权限（MANAGER）。返回 0 或 1 个凭据的元数据，不返回完整凭据。",
		Example:       "dws aitable api-key list --base-id <BASE_ID> --format json",
		Server:        "aitable",
		Tool:          "list_sql_sheet_api_keys",
		Flags:         []LeafFlag{{Name: "base-id", Bind: "baseId", Required: true, Trim: true, Usage: "AI 表格 Base ID"}},
		Safety:        aitableSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "list_sql_sheet_api_keys", CanonicalPath: "aitable.list_sql_sheet_api_keys", CLIPath: "aitable api-key list", PrimaryCLIPath: "aitable api-key list"},
			Description: "查询指定 Base 的有效 SQL Sheet API Key 元数据。",
			Interface:   aitableMCPInterface("list_sql_sheet_api_keys"),
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: aitableResultSchemaWithDryRun("查询 SQL Sheet API Key 元数据结果或请求预览", aitableAPIKeyListSchema),
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询 SQL Sheet API Key 元数据。",
				UseWhen:      []string{"用户需要查看有效凭据或核对创建状态时"},
				AvoidWhen:    []string{"创建凭据用 api-key create；查询表内记录用 record query"},
				Examples:     []string{"dws aitable api-key list --base-id <BASE_ID> --format json"},
			},
		},
		ResultCall: callAitableAPIKeyResult,
	})
}

func newAitableAPIKeyRevokeCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "revoke",
		Short:   "撤销 SQL Sheet API Key",
		Long:    "撤销指定 Base 的 SQL Sheet API Key，要求管理权限（MANAGER）。keyId 使用同一 Base 创建或查询返回的标识；不存在或已经撤销时同样成功。",
		Example: "dws aitable api-key revoke --base-id <BASE_ID> --key-id <KEY_ID> --format json",
		Server:  "aitable",
		Tool:    "revoke_sql_sheet_api_key",
		Flags: []LeafFlag{{Name: "base-id", Bind: "baseId", Required: true, Trim: true, Usage: "AI 表格 Base ID"},
			{Name: "key-id", Bind: "keyId", Required: true, Trim: true, Usage: "要撤销的 Key 标识，小写规范 UUID"}},
		Constraints:   []LeafConstraint{{Kind: "custom", Flags: []string{"key-id"}, Description: "--key-id 必须为小写规范 UUID"}},
		Safety:        contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "revoke_sql_sheet_api_key", CanonicalPath: "aitable.revoke_sql_sheet_api_key", CLIPath: "aitable api-key revoke", PrimaryCLIPath: "aitable api-key revoke"},
			Description: "撤销指定 Base 的 SQL Sheet API Key，不存在或已撤销时同样成功。",
			Interface:   aitableMCPInterface("revoke_sql_sheet_api_key"),
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: aitableResultSchemaWithDryRun("撤销 SQL Sheet API Key 结果或请求预览", aitableAPIKeyRevokeSchema),
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "撤销 SQL Sheet API Key。",
				UseWhen:      []string{"用户需要撤销指定 Base 的访问凭据时"},
				AvoidWhen:    []string{"查询有效凭据用 api-key list；创建凭据用 api-key create"},
				Examples:     []string{"dws aitable api-key revoke --base-id <BASE_ID> --key-id <KEY_ID> --format json"},
			},
		},
		Validate:   validateAitableAPIKeyRevoke,
		ResultCall: callAitableAPIKeyResult,
	})
}

func validateAitableAPIKeyRevoke(cmd *cobra.Command, _ []string) error {
	if !canonicalAPIKeyID(strings.TrimSpace(mustGetFlag(cmd, "key-id"))) {
		return apperrors.NewValidation("--key-id 必须为小写规范 UUID")
	}
	return nil
}

// canonicalAPIKeyID accepts the lowercase hyphenated form used by the MCP contract.
func canonicalAPIKeyID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.String() == value
}

type apiKeyAuthor struct {
	UserID string `json:"userId"`
	CorpID string `json:"corpId"`
}
type apiKeyMetadata struct {
	KeyID     string        `json:"keyId"`
	Status    string        `json:"status"`
	CreatedAt *int64        `json:"createdAt"`
	CreatedBy *apiKeyAuthor `json:"createdBy,omitempty"`
}
type apiKeyCreated struct {
	KeyID     string `json:"keyId"`
	Status    string `json:"status"`
	CreatedAt *int64 `json:"createdAt"`
	APIKey    string `json:"apiKey"`
}

func (a *apiKeyAuthor) UnmarshalJSON(raw []byte) error {
	type author apiKeyAuthor
	if !decodeAPIKeyData(raw, (*author)(a), "userId", "corpId") {
		return errors.New("invalid API Key author")
	}
	return nil
}

func (m *apiKeyMetadata) UnmarshalJSON(raw []byte) error {
	type metadata apiKeyMetadata
	if !decodeAPIKeyData(raw, (*metadata)(m), "keyId", "status", "createdAt", "createdBy") {
		return errors.New("invalid API Key metadata")
	}
	return nil
}

// decodeAPIKeyData accepts one credential payload with the declared fields.
func decodeAPIKeyData(raw json.RawMessage, target any, fields ...string) bool {
	if jsonutil.RejectDuplicateObjectKeys(raw) != nil || jsonutil.RejectNonCanonicalObjectKeys(raw, fields...) != nil {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target) == nil && d.Decode(new(any)) == io.EOF
}

// callAitableAPIKeyResult validates the credential response and projects the declared output fields.
func callAitableAPIKeyResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if preview, ok := aitableUnifiedDryRunResult(tool, args); ok {
		return preview, nil
	}
	if deps == nil || deps.Caller == nil {
		return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
	}
	result, err := deps.Caller.CallTool(cmd.Context(), "aitable", tool, args)
	if err != nil {
		return nil, aitableprotocol.SQLSheetKeyCallError(tool, err)
	}
	if result == nil || len(result.Content) != 1 || result.Content[0].Type != "text" {
		return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
	}
	var body struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
		Error  map[string]any  `json:"error"`
	}
	raw := []byte(result.Content[0].Text)
	if jsonutil.RejectDuplicateObjectKeys(raw) != nil || jsonutil.RejectNonCanonicalObjectKeys(raw, "status", "data", "error") != nil || json.Unmarshal(raw, &body) != nil {
		return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
	}
	// Reuse the runner's business-error policy, including the gateway's empty object.
	if err := aitableprotocol.SQLSheetKeyResponseError(tool, map[string]any{"status": body.Status, "error": body.Error}, false); err != nil {
		return nil, err
	}
	validMeta := func(id, status string, at *int64) bool {
		return canonicalAPIKeyID(id) && status == "ACTIVE" && at != nil && *at >= 0
	}
	switch tool {
	case "create_sql_sheet_api_key":
		var data apiKeyCreated
		if !decodeAPIKeyData(body.Data, &data, "keyId", "status", "createdAt", "apiKey") || !validMeta(data.KeyID, data.Status, data.CreatedAt) || strings.TrimSpace(data.APIKey) == "" {
			return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
		}
		return output.Success(data), nil
	case "list_sql_sheet_api_keys":
		var data struct {
			Keys []apiKeyMetadata `json:"keys"`
		}
		if !decodeAPIKeyData(body.Data, &data, "keys") || data.Keys == nil || len(data.Keys) > 1 {
			return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
		}
		for _, key := range data.Keys {
			if !validMeta(key.KeyID, key.Status, key.CreatedAt) {
				return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
			}
			if a := key.CreatedBy; a != nil && (strings.TrimSpace(a.UserID) == "" || strings.TrimSpace(a.CorpID) == "") {
				return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
			}
		}
		return output.Success(data), nil
	case "revoke_sql_sheet_api_key":
		var data struct {
			KeyID   string `json:"keyId"`
			Revoked bool   `json:"revoked"`
		}
		if !decodeAPIKeyData(body.Data, &data, "keyId", "revoked") || data.KeyID != args["keyId"] || !data.Revoked {
			return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
		}
		return output.Success(data), nil
	default:
		return nil, aitableprotocol.SQLSheetKeyFailure(tool, "")
	}
}
