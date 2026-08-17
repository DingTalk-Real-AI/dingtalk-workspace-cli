// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// FlagPrincipalUserID is the persistent flag that enables doc-business
// delegation auth: when set, every doc-business tool call is preceded by a
// check_capability verification on behalf of the principal. Granting the
// capability to the current logged-in identity is an out-of-band action
// performed by the principal; the CLI never calls grant_capability.
const (
	FlagPrincipalUserID = "principal-user-id"
	// capabilityServerID is the helper-only drive-internal server hosting the
	// check_capability tool. It is registered as a supplement server without
	// command prefixes, so it is only reachable by explicit server ID.
	capabilityServerID = "drive-internal"
	checkCapTool       = "check_capability"
)

// docBusinessServers 文档业务域服务器白名单：仅这些 server 上的工具调用会触发
// 委托鉴权拦截，其余 server 直接透传。
var docBusinessServers = map[string]bool{
	"drive": true, "doc": true, "sheet": true,
	"wiki": true, "doc-comment": true,
}

// extractNodeId 从工具入参中提取资源标识。服务端 nodeId 统一承载节点
// （dentryUuid/URL）与知识库（纯数字 ID/URL），由服务端自动识别类型分流，
// 因此这里只需按优先级取第一个非空 string：
//   - 优先级 1（节点/文件标识）：nodeId → fileId → node_id
//   - 优先级 2（知识库/空间标识）：workspaceId → spaceId → workspace_id → space_id
//
// 全部缺失时返回 ""（调用方仍会发起鉴权，由服务端返回明确错误）。
func extractNodeId(args map[string]any) string {
	for _, key := range []string{
		"nodeId", "fileId", "node_id",
		"workspaceId", "spaceId", "workspace_id", "space_id",
	} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// docDelegationAuthCaller decorates edition.ToolCaller: before the first call
// of each doc-business toolKey it verifies the delegation via
// check_capability for the principal, then passes the original call through.
// Non-doc-business servers bypass the verification.
type docDelegationAuthCaller struct {
	inner       edition.ToolCaller
	principalID string
	checked     map[string]bool
}

// wrapDocDelegationAuthCaller keeps the optional edition.ReadToolCaller
// capability observable through the decorator (mirrors
// wrapContractConfirmCaller in leaf.go).
func wrapDocDelegationAuthCaller(d *docDelegationAuthCaller, inner edition.ToolCaller) edition.ToolCaller {
	if read, ok := inner.(edition.ReadToolCaller); ok {
		return &docDelegationAuthReadCaller{docDelegationAuthCaller: d, read: read}
	}
	return d
}

// ensureDelegationAuth runs the delegation-auth check once per toolKey for
// doc-business servers; repeated calls of the same toolKey are deduplicated.
func (d *docDelegationAuthCaller) ensureDelegationAuth(ctx context.Context, serverID, toolName string, args map[string]any) error {
	if !docBusinessServers[serverID] {
		return nil
	}
	toolKey := serverID + "." + toolName
	if d.checked[toolKey] {
		return nil
	}
	if err := d.performDelegationAuth(ctx, toolKey, args); err != nil {
		return err
	}
	d.checked[toolKey] = true
	return nil
}

// CallTool intercepts doc-business tool calls with the delegation-auth check
// and then delegates to the inner caller.
func (d *docDelegationAuthCaller) CallTool(ctx context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	if err := d.ensureDelegationAuth(ctx, serverID, toolName, args); err != nil {
		return nil, err
	}
	return d.inner.CallTool(ctx, serverID, toolName, args)
}

// performDelegationAuth executes check_capability on the drive-internal
// capability server via the inner caller (using inner avoids recursing into
// this decorator). grant_capability 是委托人给当前登录身份授权的带外动作，
// CLI 以当前登录身份运行、无法交换成委托人身份，因此不调用 grant；委托人已
// 在服务端完成授权，这里仅执行 check 校验。nodeId 为空时仍发起调用，让服务端
// 返回明确错误（52600007）。
func (d *docDelegationAuthCaller) performDelegationAuth(ctx context.Context, toolKey string, args map[string]any) error {
	nodeID := extractNodeId(args)
	checkArgs := map[string]any{
		"userId":     d.principalID,
		"mcpToolKey": toolKey,
	}
	if nodeID != "" {
		checkArgs["nodeId"] = nodeID
	}
	result, err := d.inner.CallTool(ctx, capabilityServerID, checkCapTool, checkArgs)
	if err != nil {
		return fmt.Errorf("委托鉴权校验失败: %w", err)
	}
	return parseCheckResult(d.principalID, result)
}

// checkCapabilityResponse mirrors the check_capability response payload.
type checkCapabilityResponse struct {
	Allowed       bool   `json:"allowed"`
	DenialReason  string `json:"denialReason"`
	DenialMessage string `json:"denialMessage"`
}

// parseCheckResult 解析 check_capability 响应；allowed=false 时返回携带
// denialMessage（为空时回退 denialReason）的结构化 API 错误。报错文案保持
// 用户视角：只透出委托人 ID 与服务端拒绝原因，不透出 toolKey 等 MCP 内部
// 实现细节（排查信息由 --verbose 输出与审计日志承担）。这里用
// apperrors.NewAPI 而非 CLIError：委托鉴权拒绝是服务端业务性拒绝，应呈现
// category=api/code=1；CLIError 走 PrintJSON 时 category 会兜底成 internal
// 且 Error() 带 [CODE] 技术前缀，与 code=1 自相矛盾。
func parseCheckResult(principalID string, result *edition.ToolResult) error {
	if result == nil {
		return fmt.Errorf("委托鉴权校验返回 nil result")
	}
	text := ""
	for _, c := range result.Content {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			text = c.Text
			break
		}
	}
	if text == "" {
		return fmt.Errorf("委托鉴权校验返回空响应")
	}
	var parsed checkCapabilityResponse
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return fmt.Errorf("解析委托鉴权校验响应失败: %w", err)
	}
	if !parsed.Allowed {
		msg := parsed.DenialMessage
		if strings.TrimSpace(msg) == "" {
			msg = parsed.DenialReason
		}
		return apperrors.NewAPI(
			fmt.Sprintf("委托鉴权未通过（委托人 %s）: %s", principalID, msg),
			apperrors.WithReason("delegation_denied"),
		)
	}
	return nil
}

// Format returns the inner caller's output format.
func (d *docDelegationAuthCaller) Format() string { return d.inner.Format() }

// DryRun returns the inner caller's dry-run state.
func (d *docDelegationAuthCaller) DryRun() bool { return d.inner.DryRun() }

// Fields returns the inner caller's --fields projection.
func (d *docDelegationAuthCaller) Fields() string { return d.inner.Fields() }

// JQ returns the inner caller's --jq filter expression.
func (d *docDelegationAuthCaller) JQ() string { return d.inner.JQ() }

// docDelegationAuthReadCaller preserves the optional ReadToolCaller capability
// while still enforcing delegation auth on the read channel.
type docDelegationAuthReadCaller struct {
	*docDelegationAuthCaller
	read edition.ReadToolCaller
}

// CallReadTool applies the same whitelist interception as CallTool before
// delegating to the inner read channel.
func (d *docDelegationAuthReadCaller) CallReadTool(ctx context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	if err := d.ensureDelegationAuth(ctx, serverID, toolName, args); err != nil {
		return nil, err
	}
	return d.read.CallReadTool(ctx, serverID, toolName, args)
}

// installDocDelegationAuth 在文档业务域根命令上注册 --principal-user-id 持久
// flag，并通过 PersistentPreRunE 在 flag 非空时把 deps.Caller 包装为委托鉴权
// 装饰器（参考 leaf.go 中 contractConfirmCaller 的包装还原模式，执行结束后由
// cobra.OnFinalize 还原）。dry-run 下跳过装饰。
func installDocDelegationAuth(cmd *cobra.Command) {
	// Hidden per upstream flag policy: a non-Schema invocable flag must stay
	// out of help/Schema (see corecmd FlagSpec.Hidden "hide the real flag from
	// help/Schema while keeping it invocable" and calendar participantCmd's
	// hidden persistent aliases); visible group persistent flags would have to
	// be declared in every leaf Schema ParamDecl instead.
	cmd.PersistentFlags().String(FlagPrincipalUserID, "", "委托鉴权：指定委托人用户 ID")
	_ = cmd.PersistentFlags().MarkHidden(FlagPrincipalUserID)
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		principalID, _ := c.Flags().GetString(FlagPrincipalUserID)
		principalID = strings.TrimSpace(principalID)
		if principalID == "" {
			return nil
		}
		if deps == nil || deps.Caller == nil {
			return &CLIError{
				Code:    CodeMCPToolError,
				Message: "MCP caller is not initialized",
			}
		}
		if deps.Caller.DryRun() {
			return nil
		}
		prev := deps.Caller
		d := &docDelegationAuthCaller{
			inner:       prev,
			principalID: principalID,
			checked:     map[string]bool{},
		}
		wrapped := wrapDocDelegationAuthCaller(d, prev)
		deps.Caller = wrapped
		// OnFinalize 闭包在进程内累积；仅当 deps.Caller 仍是本次包装实例时
		// 才还原，避免陈旧闭包覆盖后续安装的 caller。
		cobra.OnFinalize(func() {
			if deps != nil && deps.Caller == wrapped {
				deps.Caller = prev
			}
		})
		return nil
	}
}
