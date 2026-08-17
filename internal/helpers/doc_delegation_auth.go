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
	// codeDelegationDenied 是本文件私有的本地错误码，仅用于委托鉴权拒绝的
	// CLIError 外壳。它不进入 errors.go 的公共错误码表：外壳依赖 Cause 携带
	// 结构化 apperrors 错误（CategoryAPI），由渲染侧 errors.As 穿透 Cause 链
	// 恢复 category=api、reason 与退出码 1；缺失 Cause 时未知码会退化为 rc=5。
	codeDelegationDenied = "DELEGATION_AUTH_DENIED"
	// codeDelegationCheckFailed 是 check_capability 调用本身失败（网络异常、
	// 服务端错误等）或响应异常（nil result、空响应、JSON 解析失败）时的本地
	// 错误码，与 codeDelegationDenied 同样使用 CLIError 外壳 + CategoryAPI
	// Cause 的直通形态：裸 fmt.Errorf 会被 WrapErrorWithOperation 模式分类
	// 重包装（底层文本含 "tool" 时透出 MCP_TOOL_ERROR 前缀；解析失败文案命中
	// JSON 模式 → INPUT_INVALID_JSON 退出码 3、其余 → UNCLASSIFIED 退出码 5，
	// 同类故障退出码分裂），外壳则统一保证 category=api 与退出码 1。
	codeDelegationCheckFailed = "DELEGATION_AUTH_CHECK_FAILED"
)

// docBusinessServers 文档业务域服务器白名单：仅这些 server 上的工具调用会触发
// 委托鉴权拦截，其余 server 直接透传。markdown 域工具（原生 Markdown 文件
// 读写）同样以 ProductID "markdown" 发起调用，需一并纳入前置委托鉴权检查；
// 缺失时 markdown.go 注册的装饰器因白名单不命中而成为死旗标。
var docBusinessServers = map[string]bool{
	"drive": true, "doc": true, "sheet": true,
	"wiki": true, "doc-comment": true, "markdown": true,
}

// extractNodeId 从工具入参中提取资源标识。服务端 nodeId 统一承载节点
// （dentryUuid/URL）与知识库（纯数字 ID/URL），由服务端自动识别类型分流，
// 因此这里只需按优先级取第一个非空 string：
//   - 优先级 1（节点/文件标识）：nodeId → fileId → node_id → overwriteFileId
//     → overwriteNodeId（覆盖上传场景下 step1 入参排他地携带 overwrite 键，
//     缺失时 check 请求不带 nodeId、会被服务端 52600007 误拒）
//   - 优先级 2（知识库/空间标识）：workspaceId → spaceId → workspace_id → space_id
//
// 全部缺失时返回 ""（调用方仍会发起鉴权，由服务端返回明确错误）。
func extractNodeId(args map[string]any) string {
	for _, key := range []string{
		"nodeId", "fileId", "node_id", "overwriteFileId", "overwriteNodeId",
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
		msg := fmt.Sprintf("委托鉴权校验失败: %v", err)
		check := apperrors.NewAPI(msg,
			apperrors.WithReason("delegation_check_failed"),
			// WithCause 保留底层错误链（CLIError→apperrors.Error→底层错误），
			// errors.Is/As 仍可命中原始错误；Cause 必须是 *apperrors.Error
			// （CategoryAPI）以保证渲染侧退出码恢复为 1。
			apperrors.WithCause(err),
		)
		return &CLIError{Code: codeDelegationCheckFailed, Message: msg, Cause: check}
	}
	return parseCheckResult(d.principalID, result)
}

// checkCapabilityResponse mirrors the check_capability response payload.
type checkCapabilityResponse struct {
	Allowed       bool   `json:"allowed"`
	DenialReason  string `json:"denialReason"`
	DenialMessage string `json:"denialMessage"`
}

// checkBadResponseError 统一包装 check_capability 响应异常（nil result、空
// 响应、JSON 解析失败）为与 check 调用失败同构的 CLIError 外壳：Code 同为
// codeDelegationCheckFailed，Message 为简短事实文案，Cause 是携带同消息与
// reason=delegation_check_bad_response 的 CategoryAPI 结构化错误，渲染侧经
// errors.As 穿透 Cause 链恢复 category=api 与退出码 1。detail 中已全文嵌入
// 底层错误（仓库惯例：底层错误全文嵌入 Message，不做截断）。
func checkBadResponseError(detail string) error {
	msg := "委托鉴权校验失败: check_capability " + detail
	return &CLIError{
		Code:    codeDelegationCheckFailed,
		Message: msg,
		Cause:   apperrors.NewAPI(msg, apperrors.WithReason("delegation_check_bad_response")),
	}
}

// parseCheckResult 解析 check_capability 响应；allowed=false 时返回携带
// denialMessage（为空时回退 denialReason）的拒绝错误。报错文案保持
// 用户视角：只透出委托人 ID 与服务端拒绝原因，不透出 toolKey 等 MCP 内部
// 实现细节（排查信息由 --verbose 输出与审计日志承担）。所有错误路径一律
// 返回 CLIError 外壳 + 结构化 Cause 而非裸 apperrors/fmt.Errorf：外壳在
// WrapErrorWithOperation 第一分支直通，不会被模式分类二次包装（杜绝
// MCP_TOOL_ERROR 等技术前缀与退出码分裂）；渲染侧经 errors.As 穿透 Cause
// 链恢复 category=api、reason 与退出码 1，故 Cause 必填不可省略。
func parseCheckResult(principalID string, result *edition.ToolResult) error {
	if result == nil {
		return checkBadResponseError("返回空结果")
	}
	text := ""
	for _, c := range result.Content {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			text = c.Text
			break
		}
	}
	if text == "" {
		return checkBadResponseError("返回空响应")
	}
	var parsed checkCapabilityResponse
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return checkBadResponseError(fmt.Sprintf("响应解析失败: %v", err))
	}
	if !parsed.Allowed {
		detail := parsed.DenialMessage
		if strings.TrimSpace(detail) == "" {
			detail = parsed.DenialReason
		}
		msg := fmt.Sprintf("委托鉴权未通过（委托人 %s）: %s", principalID, detail)
		denial := apperrors.NewAPI(msg, apperrors.WithReason("delegation_denied"))
		return &CLIError{Code: codeDelegationDenied, Message: msg, Cause: denial}
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
