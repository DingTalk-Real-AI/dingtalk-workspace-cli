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

package app

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const (
	mcpMetaMarketSearchTool = "search_market_mcps"

	mcpMarketSearchDefaultLimit = 20
	mcpMarketSearchMaxLimit     = 100
)

// mcpMarketSearchFields is the reviewed per-item projection published in the
// Result declaration. Other endpoint fields are dropped so the CLI contract
// does not change when the endpoint adds or removes fields.
var mcpMarketSearchFields = []string{"mcpId", "name", "description", "serverName", "official", "corpName", "charged"}

func newMCPMarketGroup(caller edition.ToolCaller) *cobra.Command {
	// GroupNavigationOnly installs the navigation RunE, so the group declares none.
	group := &cobra.Command{
		Use:               "market",
		Short:             "钉钉 MCP 市场",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	group.AddCommand(newMCPMarketSearchCommand(caller))
	return group
}

func newMCPMarketSearchCommand(caller edition.ToolCaller) *cobra.Command {
	const path = "mcp market search"
	return helpers.NewLeafCommand(helpers.LeafSpec{
		Use:   "search",
		Short: "按名称关键词搜索钉钉 MCP 市场",
		Long: "按名称关键词搜索钉钉 MCP 市场中已上架的 MCP，返回 mcpId、名称、描述、服务名、提供方、是否官方和是否收费。" +
			"只覆盖已上架市场的 MCP，企业内发布但未上架市场的 MCP 不在结果中。\n\n" +
			"拿到 mcpId 后用 dws mcp published tools <mcpId> 查看工具，或用 dws mcp url get <mcpId> 获取连接地址。" +
			"续页时把 meta.pagination.next_token 原样传给 --cursor；某页条数少于 --limit 即视为已到末页。",
		Example: "  dws mcp market search --keyword 机器人\n" +
			"  dws mcp market search --keyword 日历 --limit 50 --format json",
		Tool:          mcpMetaMarketSearchTool,
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []helpers.LeafFlag{
			{Name: "keyword", Usage: "MCP 名称或描述关键词（必填）", Bind: "search", Required: true, MarkRequired: true, Trim: true},
			{Name: "limit", Usage: "单页返回数量，默认 20，最大 100", Kind: helpers.LeafInt, Bind: "pageSize", Default: "20", ArgDefault: "20"},
			{Name: "cursor", Usage: "分页游标；首次不传，续页原样回填 next_token", Bind: "cursor", Trim: true, OmitEmpty: true},
		},
		Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Validate: validateMCPMarketSearch,
		Contract: helpers.LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mcp",
				Name:           "market_search",
				CanonicalPath:  "mcp.market_search",
				CLIPath:        path,
				PrimaryCLIPath: path,
			},
			Description: "按名称关键词搜索钉钉 MCP 市场，返回市场 mcpId 与 MCP 摘要",
			Interface: &contract.InterfaceSpec{
				Mode:         contract.InterfaceModeComposite,
				Availability: contract.InterfaceAvailable,
				Reason:       "Reviewed unpinned remote adapter: the public CLI wrapper calls the helper-only mcp-meta/search_market_mcps endpoint, which is intentionally absent from the public product catalog and pinned MCP metadata; the CLI maps --cursor to the endpoint page number, treats a short page as the last page and projects each item to the reviewed fields.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按名称关键词搜索钉钉 MCP 市场，拿到后续命令所需的市场 mcpId",
				UseWhen:      []string{"想获取新的 MCP，或想知道市场里有哪些 MCP 但不知道 mcpId 时"},
				AvoidWhen: []string{
					"已知 mcpId 时直接使用 dws mcp published tools 或 dws mcp url get",
					"查找企业内发布但未上架市场的 MCP 时不适用；查询自己有开发权限的 MCP 服务使用 dws dev mcp service list",
				},
				Examples: []string{"dws mcp market search --keyword 机器人 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "keyword", Property: "search", Required: mcpMarketSearchBoolPtr(true)},
				{Name: "limit", Property: "pageSize", Required: mcpMarketSearchBoolPtr(false)},
				{Name: "cursor", Property: "page", Required: mcpMarketSearchBoolPtr(false)},
			},
			Result:     mcpMarketSearchResultSpec(),
			DryRun:     &contract.DryRunSpec{PreviewKind: "request"},
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
		},
		ResultCall: func(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
			return callMCPMarketSearch(cmd, caller, tool, args)
		},
	})
}

func validateMCPMarketSearch(cmd *cobra.Command, _ []string) error {
	if limit, err := cmd.Flags().GetInt("limit"); err != nil || limit < 1 || limit > mcpMarketSearchMaxLimit {
		return apperrors.NewValidation(fmt.Sprintf("--limit 必须是 1-%d 的整数", mcpMarketSearchMaxLimit))
	}
	if flag := cmd.Flags().Lookup("cursor"); flag != nil {
		if cursor := strings.TrimSpace(flag.Value.String()); cursor != "" {
			if page, err := strconv.Atoi(cursor); err != nil || page < 1 || page == int(^uint(0)>>1) {
				return apperrors.NewValidation("--cursor 必须原样使用上一页返回的 next_token",
					apperrors.WithHint("首次查询不传 --cursor；续页使用 meta.pagination.next_token"))
			}
		}
	}
	return nil
}

func callMCPMarketSearch(cmd *cobra.Command, caller edition.ToolCaller, tool string, args map[string]any) (output.CommandResult, error) {
	if caller == nil {
		return nil, fmt.Errorf("MCP tool caller is not configured")
	}
	keyword, _ := args["search"].(string)
	keyword = strings.TrimSpace(keyword)
	// --limit is an int flag with a default, validated before ResultCall runs.
	pageSize, _ := args["pageSize"].(int)
	page := 1
	if cursor, ok := args["cursor"].(string); ok && strings.TrimSpace(cursor) != "" {
		page, _ = strconv.Atoi(strings.TrimSpace(cursor)) // validated by validateMCPMarketSearch
	}
	toolArgs := map[string]any{"search": keyword, "page": page, "pageSize": pageSize}

	if caller.DryRun() {
		preview := map[string]any{"server": mcpMetaServerID, "tool": tool, "args": toolArgs}
		return output.Success(preview, output.WithDryRun()), nil
	}

	items, err := callMCPMarketSearchPage(cmd, caller, tool, toolArgs)
	if err != nil {
		return nil, err
	}
	// The endpoint's totalCount can disagree with the returned items, so a
	// short page is the only reliable end-of-results signal.
	exhausted := len(items) < pageSize
	next := ""
	if !exhausted {
		next = strconv.Itoa(page + 1)
	}

	data := map[string]any{"keyword": keyword, "mcps": items}
	meta := &output.Meta{Count: output.NewCount(len(items)), Pagination: &output.Pagination{
		EndpointExhausted: exhausted, NextToken: next,
	}}
	return output.Success(data, output.WithMeta(meta)), nil
}

func callMCPMarketSearchPage(cmd *cobra.Command, caller edition.ToolCaller, tool string, toolArgs map[string]any) ([]any, error) {
	result, err := caller.CallTool(cmd.Context(), mcpMetaServerID, tool, toolArgs)
	if err != nil {
		return nil, fmt.Errorf("搜索 MCP 市场: %w", err)
	}
	payload, err := decodeMCPMetaResult(result)
	if err != nil {
		return nil, err
	}
	body, _ := payload.(map[string]any)
	resultPage, _ := body["result"].(map[string]any)
	raw, ok := resultPage["values"].([]any)
	if !ok {
		return nil, fmt.Errorf("MCP 元服务返回的搜索结果缺少 result.values 列表")
	}
	items := make([]any, 0, len(raw))
	for i, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok || item["mcpId"] == nil {
			return nil, fmt.Errorf("MCP 元服务返回的搜索结果第 %d 项缺少 mcpId", i)
		}
		if _, ok := item["name"].(string); !ok {
			return nil, fmt.Errorf("MCP 元服务返回的搜索结果第 %d 项缺少有效 name", i)
		}
		projected := make(map[string]any, len(mcpMarketSearchFields))
		for _, field := range mcpMarketSearchFields {
			value, present := item[field]
			if !present {
				continue
			}
			valid := false
			switch field {
			case "mcpId":
				number, ok := value.(float64)
				valid = ok && math.Trunc(number) == number
			case "official", "charged":
				_, valid = value.(bool)
			case "serverName":
				_, valid = value.(string)
				valid = valid || value == nil
			default:
				_, valid = value.(string)
			}
			if !valid {
				return nil, fmt.Errorf("MCP 元服务返回的搜索结果第 %d 项字段 %s 类型无效", i, field)
			}
			projected[field] = value
		}
		// serverName is published as nullable; keep the key stable when the
		// endpoint omits null values instead of returning them.
		if _, present := projected["serverName"]; !present {
			projected["serverName"] = nil
		}
		items = append(items, projected)
	}
	return items, nil
}

func mcpMarketSearchResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
		"oneOf":[{
			"type":"object",
			"description":"MCP 市场搜索结果；续页信息位于 meta.pagination",
			"properties":{
				"keyword":{"type":"string","description":"本次搜索关键词"},
				"mcps":{"type":"array","description":"匹配的市场 MCP 列表","items":{
					"type":"object",
					"description":"市场 MCP 摘要",
					"properties":{
						"mcpId":{"type":"integer","description":"市场 MCP 唯一标识，可传给 dws mcp published tools 或 dws mcp url get"},
						"name":{"type":"string","description":"MCP 名称"},
						"description":{"type":"string","description":"MCP 功能描述"},
						"serverName":{"type":["string","null"],"description":"官方 MCP 的网关服务名；多数 MCP 没有该值，返回 null"},
						"official":{"type":"boolean","description":"是否钉钉官方 MCP"},
						"corpName":{"type":"string","description":"提供方企业名称"},
						"charged":{"type":"boolean","description":"是否收费"}
					},
					"required":["mcpId","name"],
					"additionalProperties":false
				}}
			},
			"required":["keyword","mcps"],
			"additionalProperties":false
		},{
			"type":"object",
			"description":"dry-run 请求预览，不访问远端；不代表查询结果",
			"properties":{
				"server":{"type":"string","const":"mcp-meta","description":"将调用的 MCP 元服务"},
				"tool":{"type":"string","const":"search_market_mcps","description":"将调用的市场搜索工具"},
				"args":{
					"type":"object",
					"description":"经过参数校验的远端请求参数",
					"properties":{
						"search":{"type":"string","description":"搜索关键词"},
						"page":{"type":"integer","minimum":1,"description":"从 1 开始的页码"},
						"pageSize":{"type":"integer","minimum":1,"maximum":100,"description":"单页结果数量"}
					},
					"required":["search","page","pageSize"],
					"additionalProperties":false
				}
			},
			"required":["server","tool","args"],
			"additionalProperties":false
		}]
		}`),
	}
}

func mcpMarketSearchBoolPtr(v bool) *bool { return &v }
