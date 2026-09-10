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
	"sync"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// FlagPrincipalUserID is the persistent flag that enables doc-business
// delegation auth: when set, the first invocation of each doc-business tool
// key per node within a session is gated by a check_capability verification
// on behalf of the principal. Granting the capability to the current
// logged-in identity is an out-of-band action performed by the principal;
// the CLI never calls grant_capability.
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
	// codeDelegationNotSupported 是不含节点标识参数（如搜索/列表/创建类命令）
	// 在 --principal-user-id 已启用时本地直接返回的错误码。此类命令缺少
	// nodeId，无法进行 per-node 委托鉴权，客户端在调用服务端之前即拦截。
	// 外壳 Cause 为 CategoryValidation，退出码 3（输入校验错误）。
	codeDelegationNotSupported = "DELEGATION_AUTH_NOT_SUPPORTED"
)

// docBusinessServers 文档业务域服务器白名单：仅这些 server 上的工具调用会触发
// 委托鉴权拦截，其余 server 直接透传。markdown 子命令（fetch/create/overwrite/
// patch/diff）的数据面调用全部复用 drive/doc 域函数（markdown.go →
// uploadToDrive/uploadToDocSpace 等），工具键形如 drive.get_upload_info、
// doc.commit_uploaded_file，自功能初始提交起即经 drive/doc 条目拦截；全仓
// 无任何以 "markdown" 为 serverID 的调用点，故不设 markdown 条目（设置也
// 永不命中）。markdown.go 的 installDocDelegationAuth 注册仍保留：
// --principal-user-id flag 安装需要它。
var docBusinessServers = map[string]bool{
	"drive": true, "doc": true, "sheet": true,
	"wiki": true, "doc-comment": true,
}

// delegationFlowContinuationTools 是多步业务流程的「续调」工具白名单：这些调用
// 在结构上只携带流程句柄（sessionId/taskId/uploadId 等），不携带节点标识，但其
// 所属操作已在流程首个携带节点的调用处完成 check_capability 委托鉴权——import
// 的 create_import_session 带 targetFolderId/workspaceId，上传的 get_upload_info
// /get_file_upload_info 带 parentId/folderId。因此当这些工具在 nodeId 为空时命中
// 委托装饰器，【有条件】放行——仅当本 session 已有一次「带节点的首步检查通过」
// （sessionAuthorized，见 performDelegationAuth）时才跳过 check，避免把「已授权
// 操作的流程续调」误判为 DELEGATION_AUTH_NOT_SUPPORTED。反之，这些工具名也可
// 被独立命令发起（doc import get / sheet import get 仅带 taskId → query_import_task；
// 手动 drive commit → commit_upload），此时无前置授权，必须落回 NOT_SUPPORTED，
// 杜绝无条件放行造成的委托旁路。该错误码的一期语义仅用于拦截搜索/列表/创建等
// 独立无节点命令，无前置授权的续调工具同样不得放行。dry-run 只触发流程首个
// 调用，故续调缺陷仅在真实执行（非 dry-run）时暴露。
//   - confirm_import       : import 确认导入（doc/sheet import 第 3 步，仅带 sessionId）
//   - query_import_task     : import 轮询/查询任务状态（仅带 taskId）
//   - commit_upload         : drive 上传入库（get_upload_info 之后；节点缺失时的续调）
//   - commit_uploaded_file  : doc 文档空间上传入库（get_file_upload_info 之后）
var delegationFlowContinuationTools = map[string]bool{
	"confirm_import":       true,
	"query_import_task":    true,
	"commit_upload":        true,
	"commit_uploaded_file": true,
}

// extractNodeId 从工具入参中提取资源标识。服务端 nodeId 统一承载节点
// （dentryUuid/URL）与知识库（纯数字 ID/URL），由服务端自动识别类型分流，
// 因此这里只需按优先级取第一个非空 string：
//   - 优先级 1（节点/文件标识）：nodeId → fileId → node_id → overwriteFileId
//     → overwriteNodeId（覆盖上传场景下 step1 入参排他地携带 overwrite 键，
//     缺失时 check 请求不带 nodeId、会被服务端 52600007 误拒）→ parentId
//     （drive.list_files 按文件夹 dentryUuid 导航）→ folderId（doc.list_nodes
//     按知识库文件夹导航）→ targetFolderId（import 目标文件夹；copy/move
//     args 另含 nodeId 指定源节点，故仍取 nodeId 不受影响）
//   - 优先级 2（知识库/空间标识）：workspaceId → spaceId → workspace_id → space_id
//
// 全部缺失时返回 ""（调用方直接返回 DELEGATION_AUTH_NOT_SUPPORTED 本地错误）。
func extractNodeId(args map[string]any) string {
	for _, key := range []string{
		"nodeId", "fileId", "node_id", "overwriteFileId", "overwriteNodeId",
		"parentId", "folderId", "targetFolderId",
		"workspaceId", "spaceId", "workspace_id", "space_id",
	} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// dryRunValidator is a package-internal interface satisfied by delegation auth
// decorators. helpers.go's dry-run branch uses it to trigger check_capability
// validation before rendering the preview — without calling inner.CallTool
// (which would be a no-op via EchoRunner but would pollute test recordings).
type dryRunValidator interface {
	ensureDelegationAuth(ctx context.Context, serverID, toolName string, args map[string]any) error
}

// docDelegationAuthCaller decorates edition.ToolCaller: before the first call
// of each doc-business tool key per node it verifies the delegation via
// check_capability for the principal, then passes the original call through.
// Non-doc-business servers bypass the verification.
type docDelegationAuthCaller struct {
	mu          sync.Mutex
	inner       edition.ToolCaller
	principalID string
	checked     map[string]bool
	// sessionAuthorized 记录本进程/本 session 内是否已有一次「带节点的首步
	// 检查通过（allowed）」。它是无节点「流程续调」有条件豁免的前提：只有证明
	// 同 session 首步已鉴权通过，续调（confirm_import/query_import_task/commit_*）
	// 才放行；独立调用（无前置带节点检查）落回 NOT_SUPPORTED，杜绝委托旁路。
	// 与 checked 共用同一把 mu 保护。
	sessionAuthorized bool
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

// ensureDelegationAuth runs the delegation-auth check once per tool-key+node
// combination for doc-business servers; repeated calls with the same tool key
// and node ID are deduplicated. When nodeId is empty, performDelegationAuth
// returns DELEGATION_AUTH_NOT_SUPPORTED immediately (no remote call). For
// calls with a valid node identifier, the cache key is tool-key + nodeId.
func (d *docDelegationAuthCaller) ensureDelegationAuth(ctx context.Context, serverID, toolName string, args map[string]any) error {
	if !docBusinessServers[serverID] {
		return nil
	}
	toolKey := serverID + "." + toolName
	nodeID := extractNodeId(args)
	cacheKey := toolKey
	if nodeID != "" {
		cacheKey = toolKey + "." + nodeID
	}
	d.mu.Lock()
	if d.checked[cacheKey] {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()
	if err := d.performDelegationAuth(ctx, toolKey, args); err != nil {
		return err
	}
	d.mu.Lock()
	d.checked[cacheKey] = true
	d.mu.Unlock()
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
// 在服务端完成授权，这里仅执行 check 校验。nodeId 为空时直接返回本地
// DELEGATION_AUTH_NOT_SUPPORTED 错误，不透传到服务端。
func (d *docDelegationAuthCaller) performDelegationAuth(ctx context.Context, toolKey string, args map[string]any) error {
	nodeID := extractNodeId(args)
	if nodeID == "" {
		// 流程续调有条件豁免（见 delegationFlowContinuationTools 注释）：续调是
		// 「已授权操作的延续」，必须证明同 session 首步已带节点鉴权通过
		// （sessionAuthorized）才放行；否则（独立命令无前置授权）落回 NOT_SUPPORTED，
		// 杜绝独立 query/commit 旁路委托校验。
		toolName := toolKey
		if parts := strings.SplitN(toolKey, ".", 2); len(parts) == 2 {
			toolName = parts[1]
		}
		if delegationFlowContinuationTools[toolName] {
			d.mu.Lock()
			authorized := d.sessionAuthorized
			d.mu.Unlock()
			if authorized {
				return nil
			}
		}
		msg := fmt.Sprintf("当前命令不支持委托鉴权：缺少节点标识参数（--principal-user-id %s）", d.principalID)
		return &CLIError{
			Code:    codeDelegationNotSupported,
			Message: msg,
			Cause:   apperrors.NewValidation(msg, apperrors.WithReason("delegation_not_supported")),
		}
	}
	checkArgs := map[string]any{
		"userId":     d.principalID,
		"mcpToolKey": toolKey,
		"nodeId":     nodeID,
	}
	// 二期：按 mcpToolKey 识别操作类型，从原始 args 提取操作专属参数填充
	// options，为服务端提供更精确的前置管控信息（创建类型 / 上传文件信息 /
	// 复制移动目标 / 成员越权与跨企业预检）。options 为 nil 时不注入该键，
	// 序列化结果与一期完全一致（向后兼容）。注入点在 dry-run（CallReadTool）
	// 与正常（CallTool）两分支之前，故 checkArgs 被两条路径共用。
	if opts := buildDelegationOptions(toolKey, args, resolveCurrentCorpID(ctx)); opts != nil {
		checkArgs["options"] = opts
	}

	// In dry-run mode, CallTool goes through EchoRunner which returns
	// {"dry_run":true} — parseCheckResult would not find "allowed" and would
	// always deny. check_capability is a pure read-only verification, so we
	// route it through CallReadTool (which bypasses the write barrier and
	// issues a real network request) when available.
	var result *edition.ToolResult
	var err error
	if d.inner.DryRun() {
		if rc, ok := d.inner.(edition.ReadToolCaller); ok {
			result, err = rc.CallReadTool(ctx, capabilityServerID, checkCapTool, checkArgs)
		} else {
			result, err = d.inner.CallTool(ctx, capabilityServerID, checkCapTool, checkArgs)
		}
	} else {
		result, err = d.inner.CallTool(ctx, capabilityServerID, checkCapTool, checkArgs)
	}
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
	if err := parseCheckResult(d.principalID, result); err != nil {
		return err
	}
	// 本次为带节点（nodeID!=""）的首步检查且已通过：标记 session 已鉴权，
	// 供后续无节点「流程续调」有条件放行（见 delegationFlowContinuationTools）。
	d.mu.Lock()
	d.sessionAuthorized = true
	d.mu.Unlock()
	return nil
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
// cobra.OnFinalize 还原）。dry-run 下装饰器同样安装，确保预览与执行行为一致。
//
// 闭包内显式链式调用根命令的 PersistentPreRunE 以避免 cobra 就近匹配语义遮蔽
// 根 hook（--output/--debug/--profile/agent 元数据校验/诊断日志）。
func installDocDelegationAuth(cmd *cobra.Command) {
	// Hidden per upstream flag policy: a non-Schema invocable flag must stay
	// out of help/Schema (see corecmd FlagSpec.Hidden "hide the real flag from
	// help/Schema while keeping it invocable" and calendar participantCmd's
	// hidden persistent aliases); visible group persistent flags would have to
	// be declared in every leaf Schema ParamDecl instead.
	cmd.PersistentFlags().String(FlagPrincipalUserID, "", "委托鉴权：指定委托人用户 ID")
	_ = cmd.PersistentFlags().MarkHidden(FlagPrincipalUserID)
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		// Chain the root command's PersistentPreRunE: cobra only executes the
		// nearest ancestor's PersistentPreRunE, so without explicit chaining
		// the root's --output/--debug/--profile/agent metadata validation and
		// diagnostic logging would be shadowed for every leaf under this
		// product group. Guard: skip when the root IS this command (test
		// scenarios where installDocDelegationAuth is called on the root
		// itself) to avoid infinite recursion.
		if rootCmd := c.Root(); rootCmd != cmd && rootCmd.PersistentPreRunE != nil {
			if err := rootCmd.PersistentPreRunE(c, args); err != nil {
				return err
			}
		}

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

// ---------------------------------------------------------------------------
// Phase 2: check_capability options builders
//
// buildDelegationOptions 及其子构建器按 mcpToolKey 的 toolName 分发，从命令
// 原始 args 提取操作专属字段构造契约声明的六个 *ActionParam / permissionManage
// 子对象。设计要点：
//   - 按 toolName（而非完整 mcpToolKey）分发：wiki 路由到 doc server、markdown
//     复用 drive/doc、sheet 同名工具自动覆盖；未命中的 toolName 返回 nil
//     （等价一期，不注入 options 键）。
//   - 区分强校验字段与真选填字段：createActionParam.name / uploadActionParam
//     .fileName / importActionParam.fileName 为服务端强校验，缺失即不构造对应
//     子对象（返回 nil）；fileSize / 旧格式 permission 的 corpId 为真选填，
//     缺失则省略。
//   - options 只含契约声明的子对象，不引入额外 action 字段（服务端由
//     mcpToolKey 推断操作）。
// ---------------------------------------------------------------------------

// buildDelegationOptions 构造 check_capability 的可选 options 对象。toolKey 形如
// "doc.create_document"；先 SplitN 取 toolName 再按操作类型分发。返回 nil 时
// 调用方不注入 options 键，序列化与一期完全一致。corpID 仅供 permission 旧格式
// （--users）等效转换分支使用（本期同企业委托）。
func buildDelegationOptions(toolKey string, args map[string]any, corpID string) map[string]any {
	toolName := toolKey
	if parts := strings.SplitN(toolKey, ".", 2); len(parts) == 2 {
		toolName = parts[1]
	}
	switch toolName {
	case "create_document", "create_file", "create_folder",
		"create_workspace_sheet", "apply_sheet_template":
		if p := buildCreateActionParam(toolName, args); p != nil {
			return map[string]any{"createActionParam": p}
		}
	case "get_upload_info", "commit_upload":
		if p := buildUploadActionParam(args, "fileName"); p != nil {
			return map[string]any{"uploadActionParam": p}
		}
	case "get_file_upload_info", "commit_uploaded_file":
		if p := buildUploadActionParam(args, "name"); p != nil {
			return map[string]any{"uploadActionParam": p}
		}
	case "create_import_session":
		if p := buildImportActionParam(args); p != nil {
			return map[string]any{"importActionParam": p}
		}
	case "copy_document":
		if target := buildTransferTargetNodeId(args); target != "" {
			return map[string]any{"copyActionParam": map[string]any{"targetNodeId": target}}
		}
	case "move_document":
		if target := buildTransferTargetNodeId(args); target != "" {
			return map[string]any{"moveActionParam": map[string]any{"targetNodeId": target}}
		}
	case "add_permission", "update_permission", "remove_permission":
		if members := buildPermissionTargetMembers(args, corpID); members != nil {
			return map[string]any{"permissionManageParam": map[string]any{"targetMembers": members}}
		}
	case "set_file_publish":
		if p := buildShareScopeSetParam(args); p != nil {
			return p
		}
	}
	return nil
}

// buildCreateActionParam 构造 createActionParam。扩展名重建规则（决策 4）：
//   - create_folder / drive mkdir → createFolder:true（无 name）。
//   - create_document → 恒 adoc，name = args["name"]+".adoc"。
//   - create_file → 依 args["type"]：type=="folder" → createFolder:true 无 name；
//     否则 name = args["name"]+"."+type。
//   - create_workspace_sheet / apply_sheet_template → 恒 axls，name=args["name"]
//     +".axls"；apply_sheet_template 的 name 可选，缺失则返回 nil。
//
// name（除 apply_sheet_template 外）是强校验字段，缺失即返回 nil（不构造子对象）。
func buildCreateActionParam(toolName string, args map[string]any) map[string]any {
	switch toolName {
	case "create_folder":
		return map[string]any{"createFolder": true}
	case "create_document":
		name := strings.TrimSpace(stringArg(args, "name"))
		if name == "" {
			return nil
		}
		return map[string]any{"createFolder": false, "name": name + ".adoc"}
	case "create_file":
		typ := strings.TrimSpace(stringArg(args, "type"))
		if typ == "folder" {
			return map[string]any{"createFolder": true}
		}
		name := strings.TrimSpace(stringArg(args, "name"))
		if name == "" {
			return nil
		}
		if typ == "" {
			// type 为命令必填项，正常不会为空；防御性地不重建扩展名，
			// 直传裸 name（避免拼出 "name." 这类畸形值）。
			return map[string]any{"createFolder": false, "name": name}
		}
		return map[string]any{"createFolder": false, "name": name + "." + typ}
	case "create_workspace_sheet", "apply_sheet_template":
		name := strings.TrimSpace(stringArg(args, "name"))
		if name == "" {
			// apply_sheet_template 的 name 可选，缺失则不构造 createActionParam。
			return nil
		}
		return map[string]any{"createFolder": false, "name": name + ".axls"}
	}
	return nil
}

// buildUploadActionParam 构造 uploadActionParam / importActionParam（同构：
// {fileName, fileSize?}）。nameKey 指定从 args 读取文件名的键（drive 上传与
// import 用 "fileName"，doc 上传用 "name"）；输出键恒为契约字段 "fileName"。
// fileName 是强校验字段，缺失即返回 nil；fileSize 为真选填，为零或缺失则省略。
func buildUploadActionParam(args map[string]any, nameKey string) map[string]any {
	fileName := strings.TrimSpace(stringArg(args, nameKey))
	if fileName == "" {
		return nil
	}
	param := map[string]any{"fileName": fileName}
	if size, ok := numericFileSize(args["fileSize"]); ok {
		param["fileSize"] = size
	}
	return param
}

// buildImportActionParam 构造 importActionParam（{fileName, fileSize?}），与
// buildUploadActionParam 的强校验语义一致（fileName 为空返回 nil，fileSize 选填）。
// 差异点：契约要求 importActionParam.fileName 含扩展名，而 import_flow.go 的
// importSessionArgs 采用分离承载（"fileName" 为基础名 + "suffix" 为扩展名），故此处
// 拼回扩展名——suffix 非空且 fileName 尚不含 "." 时用 fileName+"."+suffix；fileName
// 已含 "." 或 suffix 缺失时保持原值（降级，避免拼出 "imp." 这类畸形名）。
func buildImportActionParam(args map[string]any) map[string]any {
	fileName := strings.TrimSpace(stringArg(args, "fileName"))
	if fileName == "" {
		return nil
	}
	if suffix := strings.TrimSpace(stringArg(args, "suffix")); suffix != "" && !strings.Contains(fileName, ".") {
		fileName = fileName + "." + suffix
	}
	param := map[string]any{"fileName": fileName}
	if size, ok := numericFileSize(args["fileSize"]); ok {
		param["fileSize"] = size
	}
	return param
}

// buildShareScopeSetParam 构造 set_file_publish 的 shareScopeSetParam（v3 契约）。
// drive publish set→published=true→互联网公开(WEB=9)，注入 targetScope:9 前置管控；
// published=false（unset/缩小范围）或缺失→返回 nil（不做范围管控、不注入 options）。
// ORG(1) 无对应 CLI 命令，本期不覆盖。targetScope 用整数字面量（契约 type:number）。
func buildShareScopeSetParam(args map[string]any) map[string]any {
	if published, ok := args["published"].(bool); ok && published {
		return map[string]any{"shareScopeSetParam": map[string]any{"targetScope": 9}}
	}
	return nil
}

// buildTransferTargetNodeId 解析 copy/move 的目标节点：优先 targetFolderId，
// 回退 workspaceId；都无则返回 ""（调用方不构造对应子对象）。
func buildTransferTargetNodeId(args map[string]any) string {
	if v := strings.TrimSpace(stringArg(args, "targetFolderId")); v != "" {
		return v
	}
	return strings.TrimSpace(stringArg(args, "workspaceId"))
}

// buildPermissionTargetMembers 构造 permissionManageParam.targetMembers。
//   - 新格式 args["members"]（[]map[string]any）：逐项 type→memberType、id、
//     corpId（存在才带）、roleId（存在才带）直接映射。
//   - 旧格式 args["userIds"]（[]string）+ 可选 args["roleId"]：本期同企业委托，
//     为每个 userId 构造 {memberType:"USER", id, corpId:<当前登录 corpID>,
//     roleId:<存在才带；remove 无 role 则省略>}。corpID 为空则整体返回 nil
//     （契约允许 targetMembers 空、非阻断，服务端写入精确校验兜底）。
//
// 二者都无 / 断言失败 / 旧格式 corpID 缺失 → nil。
func buildPermissionTargetMembers(args map[string]any, corpID string) []map[string]any {
	// 新格式优先：成员已携带显式 type/id/corpId/roleId。
	if raw, ok := args["members"].([]map[string]any); ok && len(raw) > 0 {
		out := make([]map[string]any, 0, len(raw))
		for _, m := range raw {
			member := map[string]any{}
			if t := stringArg(m, "type"); t != "" {
				member["memberType"] = t
			}
			if id := stringArg(m, "id"); id != "" {
				member["id"] = id
			}
			if c := stringArg(m, "corpId"); c != "" {
				member["corpId"] = c
			}
			if r := stringArg(m, "roleId"); r != "" {
				member["roleId"] = r
			}
			out = append(out, member)
		}
		return out
	}
	// 旧格式 --users：本期同企业委托，用当前登录 corpID 等效转换为 USER 成员。
	if ids, ok := args["userIds"].([]string); ok && len(ids) > 0 {
		if corpID == "" {
			return nil
		}
		roleID := stringArg(args, "roleId")
		out := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			if id == "" {
				continue
			}
			member := map[string]any{
				"memberType": "USER",
				"id":         id,
				"corpId":     corpID,
			}
			if roleID != "" {
				member["roleId"] = roleID
			}
			out = append(out, member)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return nil
}

// resolveCurrentCorpID 读取已注册的 $corpId runtimeDefault 解析器返回当前登录
// 企业 corpId，供 permission 旧格式（--users）等效转换使用。它直接读取本包
// runtimeDefaults（RegisterRuntimeDefault 写入的同一 map），故注册与读取落在
// 同一条链，不依赖 edition.RuntimeDefaults hook（开源版该 hook 为 nil）。
// 未注册 / 解析器返回 (,false) / 空值 → 返回 ""（旧格式回退 nil、不阻断）。
func resolveCurrentCorpID(ctx context.Context) string {
	runtimeDefaultsMu.RLock()
	fn := runtimeDefaults[RuntimeDefaultCorpID]
	runtimeDefaultsMu.RUnlock()
	if fn == nil {
		return ""
	}
	v, ok := fn(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// numericFileSize 归一化 fileSize 入参（args 中可能是 int64/int/float64）为
// int64；零值或非数值返回 (0,false)，供调用方省略选填字段。
func numericFileSize(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, n != 0
	case int:
		return int64(n), n != 0
	case float64:
		return int64(n), n != 0
	}
	return 0, false
}
