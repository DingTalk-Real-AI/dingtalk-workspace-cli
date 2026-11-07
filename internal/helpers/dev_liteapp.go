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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// 轻应用六个工具与 dev app 同属钉钉开放平台应用服务（同一 MCP server），
// 因此完全复用 devapp 的调用链：devAppCall(runner) → runDevAppTool，
// 经 directRuntimeEndpoint 固定路由，不在命令面暴露任何服务指定参数。
const (
	liteappCreateTool     = "create_lite_app"
	liteappUpdateTool     = "update_lite_app"
	liteappDeleteTool     = "delete_lite_app"
	liteappListTool       = "list_lite_apps"
	liteappDetailTool     = "get_lite_app_detail"
	liteappCredentialTool = "get_lite_app_credentials"
)

func newLiteappGroup(runner executor.Runner) *cobra.Command {
	// dev 子树叶子身份约定：Schema ProductID 固定 "dev"，Name 用远端工具名，
	// CanonicalPath 为 "dev." + 工具名，CLIPath 为完整命令路径（与 dev app/dev mcp 一致）。

	group := &cobra.Command{
		Use:               "liteapp",
		Short:             "轻应用全生命周期（创建即发布）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	group.AddCommand(
		newLiteappCreateCommand(runner),
		newLiteappUpdateCommand(runner),
		newLiteappDeleteCommand(runner),
		newLiteappListCommand(runner),
		newLiteappDetailCommand(runner),
		newLiteappCredentialCommand(runner),
	)
	return group
}

// splitRedirectURIs 把逗号分隔的回调地址声明式装配为 []string：元素 trim、
// 丢弃空串；全空结果由框架按空 Transform 结果跳过该键（不传=不修改）。
func splitRedirectURIs(raw string) (any, error) {
	uris := []string{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			uris = append(uris, trimmed)
		}
	}
	return uris, nil
}

func validateLiteappUpdate(cmd *cobra.Command, _ []string) error {
	for _, name := range []string{"name", "homepage-url", "pc-url", "desc", "icon-media-id"} {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed && strings.TrimSpace(flag.Value.String()) == "" {
			return apperrors.NewValidation(fmt.Sprintf("--%s 不能为空；不修改请勿传空串", name))
		}
	}
	if flag := cmd.Flags().Lookup("redirect-uris"); flag != nil && flag.Changed {
		value, err := cmd.Flags().GetString("redirect-uris")
		if err != nil {
			return apperrors.NewValidation("--redirect-uris 参数无效")
		}
		if strings.TrimSpace(value) == "" {
			return apperrors.NewValidation("--redirect-uris 不能为空；不修改请勿传空串")
		}
		for _, part := range strings.Split(value, ",") {
			if strings.TrimSpace(part) == "" {
				return apperrors.NewValidation("--redirect-uris 不允许包含空地址")
			}
		}
	}
	return nil
}

func validateLiteappList(cmd *cobra.Command, _ []string) error {
	if flag := cmd.Flags().Lookup("size"); flag != nil && flag.Changed {
		size, err := cmd.Flags().GetInt("size")
		if err != nil || size < 1 || size > 50 {
			return apperrors.NewValidation("--size 必须是 1 到 50 之间的整数")
		}
	}
	if flag := cmd.Flags().Lookup("offset"); flag != nil && flag.Changed {
		offset, err := cmd.Flags().GetInt("offset")
		if err != nil || offset < 0 {
			return apperrors.NewValidation("--offset 必须是非负整数")
		}
	}
	return nil
}

func liteappResultSpec(dataSchema string, sensitivePaths ...string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(dataSchema),
		SensitivePaths: sensitivePaths,
	}
}

func newLiteappCreateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "create",
		Short:   "创建轻应用（创建即发布挂工作台，并注册统一应用与 OAuth 凭证）",
		Example: "  dws dev liteapp create --name 周报助手 --homepage-url https://example.com --desc '周报自动化助手' --request-id 6f1c2b3a-uuid --dry-run --format json",
		Safety: contract.SafetySpec{
			// requestId is optional: without it a retry creates a duplicate
			// app, so the unconditional contract must stay non-idempotent.
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Tool:          liteappCreateTool,
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "name", Usage: "应用名称，必填，trim 后非空", Required: true, ValidationMode: corecmd.ValidationShortcut, RequiredError: "--name 不能为空", Trim: true, Bind: "name"},
			{Name: "homepage-url", Usage: "移动端首页地址，必填，http(s) URL", Required: true, ValidationMode: corecmd.ValidationShortcut, RequiredError: "--homepage-url 不能为空", Trim: true, Bind: "homepageUrl"},
			{Name: "pc-url", Usage: "PC 端首页地址，可选，缺省取移动端首页地址", Trim: true, OmitEmpty: true, Bind: "pcUrl"},
			{Name: "desc", Usage: "应用描述，必填，trim 后非空", Required: true, MarkRequired: true, ValidationMode: corecmd.ValidationShortcut, RequiredError: "--desc 不能为空", Trim: true, Bind: "desc"},
			{Name: "icon-media-id", Usage: "图标文件 media_id（logoImg 格式）；不接受 http 地址；不传自动生成默认图标", Trim: true, OmitEmpty: true, Bind: "iconMediaId"},
			{Name: "request-id", Usage: "可选请求 ID；不传不保证幂等，传入后同键同参数重试返回首次结果、参数变化拒绝", Trim: true, OmitEmpty: true, Bind: "requestId"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "create_lite_app", CanonicalPath: "dev.create_lite_app",
				CLIPath: "dev liteapp create", PrimaryCLIPath: "dev liteapp create",
			},
			Description: "经确认后创建钉钉企业内部轻应用，创建即发布挂工作台并注册统一应用与 OAuth 凭证；name、homepage-url、desc 必须提供且 trim 后非空；status 通常为 PUBLISHED，若返回 UNPUBLISHED 表示工作台发布未完成，不要当作已上线。request-id 可选：只有带 request-id 时才提供条件幂等；凭证生成失败时仍可成功，appKey/secret/secretMask 为 null 且带 warning，处置：告知用户稍后在开发者后台重试，并用 dev liteapp credential 补查",
			Parameters:  []contract.ParamDecl{{Name: "desc", Required: boolPtr(true)}},
			DryRun:      devAppDryRun,
			Result:      liteappResultSpec(`{"type":"object","properties":{"appId":{"type":"integer","description":"创建成功的轻应用 ID"},"appName":{"type":"string","description":"应用名称"},"homepageUrl":{"type":"string","description":"移动端首页地址"},"pcUrl":{"type":"string","description":"PC 端首页地址"},"desc":{"type":"string","description":"应用描述"},"iconUrl":{"type":"string","description":"图标地址"},"orgId":{"type":"integer","description":"组织 ID"},"appKey":{"type":["string","null"],"description":"OAuth 应用标识；凭证生成降级时为 null"},"secret":{"type":["string","null"],"description":"OAuth secret 明文；仅限用户明确需要时使用；凭证生成降级时为 null"},"secretMask":{"type":["string","null"],"description":"OAuth secret 掩码；凭证生成降级时为 null"},"unifiedAppId":{"type":["string","null"],"description":"创建时注册的统一应用 ID；统一应用注册未完成时为 null"},"status":{"type":["string","null"],"description":"轻应用发布状态；可能为 PUBLISHED 或未完成发布的 UNPUBLISHED"},"requestId":{"type":["string","null"],"description":"请求 ID；仅传入 request-id 时回显"},"warning":{"type":["string","null"],"description":"成功但存在降级或部分副作用时的警告；无警告时为 null"}}}`, "secret"),
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后创建轻应用；name、homepage-url、desc 必须提供且 trim 后非空；通常返回 PUBLISHED，若 status=UNPUBLISHED 表示尚未完成工作台发布；request-id 可选且只有传入时才有条件幂等；凭证降级时 appKey/secret 为 null 且带 warning",
				UseWhen:      []string{"需要在当前组织创建一个新的钉钉轻应用并直接发布到工作台"},
				AvoidWhen: []string{
					"需要管理已有应用时使用 dev liteapp update/delete 等子命令",
					"用户未确认创建目标名称与首页地址时不要真实执行",
					"status=UNPUBLISHED 时不要向用户宣称应用已上线，应先处理 warning 或到开发者后台核对",
				},
				Examples: []string{"dws dev liteapp create --name 周报助手 --homepage-url https://example.com --desc '周报自动化助手' --request-id 6f1c2b3a-uuid --dry-run --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(liteappCreateTool),
	})
}

func newLiteappUpdateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "update <unifiedAppId>",
		Short: "更新轻应用（仅创建者本人；不传=不修改，空串拒绝）",
		Example: "  dws dev liteapp update uni-5005426001 --desc 新描述\n" +
			"  dws dev liteapp update uni-5005426001 --redirect-uris https://a.example.com/cb,https://b.example.com/cb --yes",
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Tool:          liteappUpdateTool,
		OutputRollout: output.RolloutUnifiedActive,
		Positionals: []LeafPositional{
			liteappAppIDPositional(),
		},
		Flags: []LeafFlag{
			{Name: "name", Usage: "应用名称；不传=不修改，空串拒绝", Trim: true, OmitEmpty: true, Bind: "name"},
			{Name: "homepage-url", Usage: "移动端首页地址；不传=不修改，空串拒绝", Trim: true, OmitEmpty: true, Bind: "homepageUrl"},
			{Name: "pc-url", Usage: "PC 端首页地址；不传=不修改，空串拒绝", Trim: true, OmitEmpty: true, Bind: "pcUrl"},
			{Name: "desc", Usage: "应用描述；不传=不修改，空串拒绝", Trim: true, OmitEmpty: true, Bind: "desc"},
			{Name: "icon-media-id", Usage: "图标文件 media_id；不接受 http 地址；不传=不修改", Trim: true, OmitEmpty: true, Bind: "iconMediaId"},
			{Name: "redirect-uris", Usage: "OAuth 回调地址列表，逗号分隔；不传=不修改，传入=整体覆盖", Trim: true, OmitEmpty: true, Bind: "redirectUris",
				Transform: splitRedirectURIs},
		},
		Constraints: []LeafConstraint{{
			Kind:        LeafAtLeastOne,
			Flags:       []string{"name", "homepage-url", "pc-url", "desc", "icon-media-id", "redirect-uris"},
			Description: "至少提供一个要修改的字段；不修改请勿调用",
		}},
		Validate: validateLiteappUpdate,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "update_lite_app", CanonicalPath: "dev.update_lite_app",
				CLIPath: "dev liteapp update", PrimaryCLIPath: "dev liteapp update",
			},
			Description: "经确认后更新创建者本人的轻应用基础信息与 OAuth 回调地址；返回的 unifiedAppId 可能为 null，缺失时不要继续依赖该字段做后续定位",
			DryRun:      devAppDryRun,
			Result:      liteappResultSpec(`{"type":"object","properties":{"appId":{"type":"integer","description":"轻应用 ID"},"appName":{"type":"string","description":"应用名称"},"desc":{"type":"string","description":"应用描述"},"homepageUrl":{"type":"string","description":"移动端首页地址"},"pcUrl":{"type":"string","description":"PC 端首页地址"},"iconUrl":{"type":"string","description":"图标地址"},"redirectUris":{"type":"array","description":"OAuth 回调地址列表，仅登记后存在"},"unifiedAppId":{"type":["string","null"],"description":"统一应用 ID；服务端未返回时不要继续依赖该字段"},"status":{"type":"string","description":"轻应用状态"},"orgId":{"type":"integer","description":"组织 ID"},"createdAt":{"type":"integer","description":"创建时间戳（毫秒）"}}}`),
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后更新创建者本人的轻应用；返回的 unifiedAppId 可能为 null，缺失时不要继续依赖该字段做后续定位",
				UseWhen:      []string{"需要修改创建者本人的轻应用名称、首页、描述、回调地址等"},
				AvoidWhen:    []string{"目标应用不是当前用户创建时不要调用（服务端仅创建者可更新）"},
				Examples:     []string{"dws dev liteapp update uni-5005426001 --desc 新描述"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(liteappUpdateTool),
	})
}

func newLiteappDeleteCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "delete <unifiedAppId>",
		Short:   "删除轻应用（24 小时软删，仅创建者或组织管理员）",
		Example: "  dws dev liteapp delete uni-5005426001",
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Tool:          liteappDeleteTool,
		OutputRollout: output.RolloutUnifiedActive,
		Positionals: []LeafPositional{
			liteappAppIDPositional(),
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "delete_lite_app", CanonicalPath: "dev.delete_lite_app",
				CLIPath: "dev liteapp delete", PrimaryCLIPath: "dev liteapp delete",
			},
			Description: "经确认后软删创建者本人或组织管理员的轻应用，返回对象中的 timeToDel 为软删截止时间；后端 Long 结果由 CLI 稳定投影，不能把它当作可恢复操作",
			DryRun:      devAppDryRun,
			Result:      liteappResultSpec(`{"type":"object","properties":{"timeToDel":{"type":"integer","description":"本次软删除截止时间戳（毫秒）；后端 ServiceResult<Long> 的 data 由 CLI 稳定投影为此对象"}}}`),
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后软删轻应用（24 小时后物理删除），返回 data.timeToDel 截止时间",
				UseWhen:      []string{"需要删除不再使用的轻应用以释放配额"},
				AvoidWhen:    []string{"应用正在被其他系统使用时不要删除；软删期内不可恢复"},
				Examples:     []string{"dws dev liteapp delete uni-5005426001"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(liteappDeleteTool),
	})
}

func newLiteappListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询当前用户在本组织创建的轻应用列表",
		Example: "  dws dev liteapp list --size 20 --format json",
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Tool:          liteappListTool,
		OutputRollout: output.RolloutUnifiedActive,
		Validate:      validateLiteappList,
		Flags: []LeafFlag{
			{Name: "size", Kind: LeafInt, Usage: "每页数量，默认 20，上限 50", Default: "20", ArgDefault: "20", Bind: "pageSize"},
			{Name: "offset", Kind: LeafInt, Usage: "偏移量，从 0 开始", Bind: "offset"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "list_lite_apps", CanonicalPath: "dev.list_lite_apps",
				CLIPath: "dev liteapp list", PrimaryCLIPath: "dev liteapp list",
			},
			Description: "查询当前用户在本组织创建的轻应用列表（不含 secret 字段）；size 默认 20；列表条目的 unifiedAppId 可能为 null，缺失时不要继续依赖该字段做 update/delete/detail 定位",
			Result:      liteappResultSpec(`{"type":"object","properties":{"total":{"type":"integer","description":"当前用户轻应用总数"},"apps":{"type":"array","description":"轻应用列表条目（按创建时间倒序）","items":{"type":"object","properties":{"appId":{"type":"integer","description":"轻应用 ID"},"appName":{"type":"string","description":"应用名称"},"createdAt":{"type":"integer","description":"创建时间戳（毫秒）"},"homepageUrl":{"type":"string","description":"移动端首页地址"},"pcUrl":{"type":"string","description":"PC 端首页地址"},"orgId":{"type":"integer","description":"组织 ID"},"status":{"type":"string","description":"轻应用状态"},"timeToDel":{"type":"integer","description":"软删截止时间戳（毫秒），仅软删期条目存在"},"unifiedAppId":{"type":["string","null"],"description":"统一应用 ID；服务端未返回时不要继续依赖该字段"}},"additionalProperties":true}}}}`),
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户在本组织创建的轻应用列表；size 默认 20；若条目 unifiedAppId 缺失，不要继续依赖它定位其他操作",
				UseWhen:      []string{"需要查看当前用户创建了哪些轻应用、占用多少配额"},
				AvoidWhen:    []string{"查询单个应用详情时用 dev liteapp detail"},
				Examples:     []string{"dws dev liteapp list --size 20 --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(liteappListTool),
	})
}

func newLiteappDetailCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "detail <unifiedAppId>",
		Short:   "查询轻应用详情（无 secret 明文）",
		Example: "  dws dev liteapp detail uni-5005426001 --format json",
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Tool:          liteappDetailTool,
		OutputRollout: output.RolloutUnifiedActive,
		Positionals: []LeafPositional{
			liteappAppIDPositional(),
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "get_lite_app_detail", CanonicalPath: "dev.get_lite_app_detail",
				CLIPath: "dev liteapp detail", PrimaryCLIPath: "dev liteapp detail",
			},
			Description: "按 unifiedAppId 查询轻应用详情（无 secret 明文）；返回的 unifiedAppId 可能为 null，缺失时不要继续依赖该字段做后续定位",
			Result:      liteappResultSpec(`{"type":"object","properties":{"appId":{"type":"integer","description":"轻应用 ID"},"appName":{"type":"string","description":"应用名称"},"appKey":{"type":"string","description":"OAuth 应用标识"},"secretMask":{"type":"string","description":"OAuth secret 掩码"},"desc":{"type":"string","description":"应用描述"},"homepageUrl":{"type":"string","description":"移动端首页地址"},"pcUrl":{"type":"string","description":"PC 端首页地址"},"iconUrl":{"type":"string","description":"图标地址"},"redirectUris":{"type":"array","description":"OAuth 回调地址列表，仅登记后存在","items":{"type":"string","description":"OAuth 回调地址"}},"unifiedAppId":{"type":["string","null"],"description":"统一应用 ID；服务端未返回时不要继续依赖该字段"},"status":{"type":"string","description":"轻应用状态"},"orgId":{"type":"integer","description":"组织 ID"},"createdAt":{"type":"integer","description":"创建时间戳（毫秒）"}}}`, "secretMask"),
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询单个轻应用的详情与凭证掩码；返回的 unifiedAppId 可能为 null，缺失时不要继续依赖该字段",
				UseWhen:      []string{"需要查看某个轻应用的基础信息、回调地址与凭证掩码"},
				AvoidWhen:    []string{"需要 secret 明文时用 dev liteapp credential"},
				Examples:     []string{"dws dev liteapp detail uni-5005426001 --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(liteappDetailTool),
	})
}

func newLiteappCredentialCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "credential <unifiedAppId>",
		Short:   "查询轻应用凭证（appKey + secret 明文，注意防泄露）",
		Example: "  dws dev liteapp credential uni-5005426001 --format json",
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "high",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Tool:          liteappCredentialTool,
		OutputRollout: output.RolloutUnifiedActive,
		Positionals: []LeafPositional{
			liteappAppIDPositional(),
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "get_lite_app_credentials", CanonicalPath: "dev.get_lite_app_credentials",
				CLIPath: "dev liteapp credential", PrimaryCLIPath: "dev liteapp credential",
			},
			Description: "查询轻应用 OAuth 凭证（appKey 明文 + secret 明文与掩码，可持续获取）",
			Result:      liteappResultSpec(`{"type":"object","properties":{"appKey":{"type":"string","description":"OAuth 应用标识"},"secret":{"type":"string","description":"OAuth secret 明文；注意防泄露"},"secretMask":{"type":"string","description":"OAuth secret 掩码"}}}`, "secret"),
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询轻应用的 OAuth 凭证（含 secret 明文，注意防泄露）",
				UseWhen:      []string{"服务端接入需要 appKey/secret 换取 accessToken 时"},
				AvoidWhen: []string{
					"不要把返回的 secret 写入日志、文档、邮件、群聊或代码仓库",
					"仅需要应用基础信息时用 dev liteapp detail",
				},
				Examples: []string{"dws dev liteapp credential uni-5005426001 --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(liteappCredentialTool),
	})
}

// liteappAppIDPositional 是 update/delete/detail/credential 共用的位置参数声明。
// 定位主键为统一应用 ID（unifiedAppId，与 dev app 全族一致），服务端反查 microAppId。
func liteappAppIDPositional() LeafPositional {
	return LeafPositional{
		Name: "unified_app_id", Usage: "轻应用统一应用 ID（unifiedAppId，create 返回）",
		Kind: LeafString, Required: true, Trim: true, Bind: "unifiedAppId",
	}
}
