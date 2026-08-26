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

package shortcut

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var validateShadowResult = output.ValidateResult

// RuntimeContext is handed to a Shortcut's Validate and Execute hooks. It wraps
// the cobra command and exposes typed flag accessors plus a single CallMCP entry
// point so shortcut authors never touch cobra/executor plumbing directly.
type RuntimeContext struct {
	cmd      *cobra.Command
	shortcut Shortcut
}

// AIMessageTagFlag matches `chat message send --ai-tag`: IM send shortcuts
// default to tagging delivered messages as AI-sent, while still allowing users
// to opt out with --ai-tag=false.
func AIMessageTagFlag() Flag {
	return Flag{Name: "ai-tag", Type: FlagBool, Default: "true", Desc: "消息是否带 AI 发送角标（默认 true）"}
}

// Command returns the underlying cobra command (escape hatch; prefer the typed
// accessors below).
func (rt *RuntimeContext) Command() *cobra.Command { return rt.cmd }

// Str returns the trimmed string value of a flag, or "" if unset.
func (rt *RuntimeContext) Str(name string) string {
	v, _ := rt.cmd.Flags().GetString(name)
	return strings.TrimSpace(v)
}

// StrFirst returns the first non-empty string value across a primary flag and
// its aliases.
func (rt *RuntimeContext) StrFirst(names ...string) string {
	for _, name := range names {
		if v := rt.Str(name); v != "" {
			return v
		}
	}
	return ""
}

// Bool returns the bool value of a flag.
func (rt *RuntimeContext) Bool(name string) bool {
	v, _ := rt.cmd.Flags().GetBool(name)
	return v
}

// Int returns the int value of a flag.
func (rt *RuntimeContext) Int(name string) int {
	v, _ := rt.cmd.Flags().GetInt(name)
	return v
}

// IntFirst returns the int value for a primary flag plus aliases. Explicitly set
// aliases are considered before the primary's default, matching native helper
// compatibility flags such as --size for --limit.
func (rt *RuntimeContext) IntFirst(primary string, aliases ...string) int {
	for _, alias := range aliases {
		f := rt.cmd.Flags().Lookup(alias)
		if f != nil && f.Changed {
			return rt.Int(alias)
		}
	}
	return rt.Int(primary)
}

// StrSlice returns the string-slice value of a flag.
func (rt *RuntimeContext) StrSlice(name string) []string {
	v, _ := rt.cmd.Flags().GetStringSlice(name)
	return v
}

// Changed reports whether the user explicitly set the flag on the command line.
func (rt *RuntimeContext) Changed(name string) bool {
	f := rt.cmd.Flags().Lookup(name)
	return f != nil && f.Changed
}

// DryRun reports whether --dry-run is set (inherited from the root command).
func (rt *RuntimeContext) DryRun() bool { return globalBool(rt.cmd, "dry-run") }

// Yes reports whether --yes is set (skip confirmation prompts).
func (rt *RuntimeContext) Yes() bool { return globalBool(rt.cmd, "yes") }

// AddAIMessageTag attaches the clawType parameter expected by IM send APIs when
// the shortcut exposes --ai-tag and the flag is true. This mirrors the native
// `chat message send` behavior.
func (rt *RuntimeContext) AddAIMessageTag(params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	if f := rt.cmd.Flags().Lookup("ai-tag"); f == nil || rt.Bool("ai-tag") {
		params["clawType"] = edition.ClawType()
	}
	return params
}

// CallMCP dispatches a single MCP tool call and prints the result, reusing the
// shared helper path so the shortcut inherits DWS's error classification
// (auth/PAT/business), dry-run preview and --format/--jq/--fields output for
// free. The MCP server id is the shortcut's product (defaults to Service).
//
// Most passthrough shortcuts do all their work in one CallMCP call.
func (rt *RuntimeContext) CallMCP(tool string, params map[string]any) error {
	if params == nil {
		params = map[string]any{}
	}
	if output.UsesUnifiedResult(rt.cmd) {
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"dry_run":   true,
				"executed":  false,
				"tool":      tool,
				"arguments": params,
			})
		}
		data, err := helpers.CallMCPToolDataOnServer(rt.cmd.Context(), rt.shortcut.product(), tool, params)
		if err != nil {
			return err
		}
		return rt.storePayload(tool, data)
	}
	if output.CommandRollout(rt.cmd) == output.RolloutDualValidate {
		preview := any(map[string]any{
			"dry_run":   true,
			"executed":  false,
			"tool":      tool,
			"arguments": params,
		})
		if rt.DryRun() {
			if err := validateShadowResult(rt.resultForPayload(tool, preview)); err != nil {
				return err
			}
			// The legacy caller owns dry-run presentation (including its human
			// preview for non-JSON formats) and does not cross the business-call
			// boundary. Keep using it so dual validation changes no bytes.
			return helpers.CallMCPToolOnServerContext(rt.commandContext(), rt.shortcut.product(), tool, params)
		}
		text, err := helpers.CallMCPToolTextOnServerContext(rt.commandContext(), rt.shortcut.product(), tool, params)
		if err != nil {
			return err
		}
		data := legacyMCPPayload(text)
		if err := validateShadowResult(rt.resultForPayload(tool, data)); err != nil {
			return err
		}
		// dual_validate changes no external bytes: it renders the once-fetched
		// payload through the established legacy projection after validating the
		// shadow unified result.
		return helpers.RenderLegacyMCPText(tool, text)
	}
	return helpers.CallMCPToolOnServerContext(rt.commandContext(), rt.shortcut.product(), tool, params)
}

func legacyMCPPayload(text string) any {
	var data any
	if err := json.Unmarshal([]byte(text), &data); err == nil {
		return data
	}
	return text
}

// CallMCPData dispatches a read-only tool call to an explicit MCP product and
// returns the PARSED response as data WITHOUT printing. This is the building
// block for multi-step ("smart") shortcuts: call a tool, read its output, feed
// it into the next call. Errors carry DWS's auth/PAT/business classification.
//
// The product is explicit (not the shortcut's own) because smart shortcuts
// routinely cross services — e.g. resolve a name via `contact` then act via
// `chat`. Reads run even under --dry-run so a preview can still resolve inputs.
// Write tools that need parsed responses must use CallMCPWriteData instead.
// Under --dry-run this path fails closed unless the tool name belongs to the
// narrow read-only naming contract used by the current MCP registry.
func (rt *RuntimeContext) CallMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	if rt.DryRun() {
		if !looksReadTool(tool) {
			return nil, dryRunWriteError(product, tool)
		}
		return rt.callMCPReadData(product, tool, params)
	}
	return rt.callMCPData(product, tool, params)
}

// CallMCPReadData dispatches a tool whose name is explicitly classified as
// read-only without consulting Cobra state. Callers must invoke it serially
// unless the injected ToolCaller separately documents and enforces concurrent
// safety. Write-shaped tool names fail closed.
func (rt *RuntimeContext) CallMCPReadData(product, tool string, params map[string]any) (map[string]any, error) {
	if !looksReadTool(tool) {
		return nil, apperrors.NewValidation(fmt.Sprintf(
			"并发只读入口拒绝写工具 %s/%s；写操作必须使用 CallMCPWriteData",
			product, tool,
		))
	}
	return rt.callMCPReadData(product, tool, params)
}

// CallMCPWriteData dispatches a write tool call and returns its parsed response.
// Unlike CallMCPData, it refuses to run under --dry-run so smart shortcuts cannot
// accidentally perform writes while rendering a preview. For compatibility with
// existing write shortcuts, an empty text acknowledgement remains an empty map.
func (rt *RuntimeContext) CallMCPWriteData(product, tool string, params map[string]any) (map[string]any, error) {
	if rt.DryRun() {
		return nil, dryRunWriteError(product, tool)
	}
	return rt.callMCPData(product, tool, params)
}

// CallMCPWriteDataStrict dispatches a write tool call whose contract requires a
// non-empty JSON business result. An empty acknowledgement is reported as an
// unknown remote effect so callers can verify it independently before success.
func (rt *RuntimeContext) CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error) {
	if rt.DryRun() {
		return nil, dryRunWriteError(product, tool)
	}
	return rt.callMCPWriteData(product, tool, params)
}

func dryRunWriteError(product, tool string) error {
	return apperrors.NewValidation(fmt.Sprintf(
		"--dry-run 下禁止执行未明确分类为只读的工具 %s/%s；请在 shortcut 中输出 preview 后返回",
		product, tool))
}

func looksReadTool(tool string) bool {
	return helpers.IsReadToolName(tool)
}

func (rt *RuntimeContext) callMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	text, err := helpers.CallMCPToolTextOnServerContext(rt.commandContext(), product, tool, params)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", tool, err))
	}
	return out, nil
}

func (rt *RuntimeContext) callMCPReadData(product, tool string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	text, err := helpers.CallMCPReadToolTextOnServerContext(rt.commandContext(), product, tool, params)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", tool, err))
	}
	return out, nil
}

func (rt *RuntimeContext) callMCPWriteData(product, tool string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	text, err := helpers.CallMCPToolTextOnServerContext(rt.commandContext(), product, tool, params)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, apperrors.NewAPI("MCP write tool returned no business result; the remote effect is unknown",
			apperrors.WithOperation(product+"/"+tool),
			apperrors.WithOrigin("mcp"),
			apperrors.WithFailureStage("response_validation"),
			apperrors.WithRetryable(false),
			apperrors.WithReason("empty_tool_response"),
		)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", tool, err))
	}
	return out, nil
}

func (rt *RuntimeContext) commandContext() context.Context {
	if rt != nil && rt.cmd != nil && rt.cmd.Context() != nil {
		return rt.cmd.Context()
	}
	return context.Background()
}

// Output prints a (typically reshaped/projected) payload honouring the root
// --format/--jq/--fields flags. Multi-step shortcuts use it to emit a clean,
// composed result instead of the raw MCP response — the output-projection
// output-formatting capability.
func (rt *RuntimeContext) Output(payload any) error {
	if output.UsesUnifiedResult(rt.cmd) {
		return output.StoreResult(rt.cmd.Context(), rt.resultForPayload("", payload))
	}
	if output.CommandRollout(rt.cmd) == output.RolloutDualValidate {
		if err := validateShadowResult(rt.resultForPayload("", payload)); err != nil {
			return err
		}
	}
	return output.WriteCommandPayload(rt.cmd, payload, output.FormatJSON)
}

// OutputForTool stores a once-fetched MCP payload while retaining the tool name
// for product-specific outcome normalization. It is intended for composite
// shortcuts that must validate stable identity or collection shape before the
// shared renderer sees the response; callers must not use it to issue a second
// business request.
func (rt *RuntimeContext) OutputForTool(tool string, payload any) error {
	if output.UsesUnifiedResult(rt.cmd) {
		return output.StoreResult(rt.cmd.Context(), rt.resultForPayload(tool, payload))
	}
	if output.CommandRollout(rt.cmd) == output.RolloutDualValidate {
		if err := validateShadowResult(rt.resultForPayload(tool, payload)); err != nil {
			return err
		}
	}
	return output.WriteCommandPayload(rt.cmd, payload, output.FormatJSON)
}

func (rt *RuntimeContext) storePayload(tool string, payload any) error {
	return output.StoreResult(rt.cmd.Context(), rt.resultForPayload(tool, payload))
}

func (rt *RuntimeContext) resultForPayload(tool string, payload any) output.CommandResult {
	if rt.shortcut.product() == "devapp" {
		return helpers.DevAppCommandResultFromPayload(tool, payload, rt.DryRun())
	}
	options := []output.ResultOption{}
	if rt.DryRun() {
		options = append(options, output.WithDryRun())
	}
	result := shortcutCommandResult(payload, options...)
	if tool != "" && !rt.DryRun() && rt.isWriteShortcut() &&
		result.Outcome() == output.OutcomeSuccess && !hasExplicitShortcutSuccess(payload) {
		started := true
		return output.Failure(&output.ErrorInfo{
			Type:             "api",
			Subtype:          "projection_unknown",
			Message:          "write shortcut response has no reviewed terminal success evidence",
			Hint:             "核查目标资源状态后再决定是否重试；为该命令使用专属结果投影表达成功证据。",
			Operation:        rt.shortcut.product() + "/" + tool,
			Origin:           "mcp_gateway",
			Stage:            "response_projection",
			ExecutionStarted: &started,
		})
	}
	return result
}

func (rt *RuntimeContext) isWriteShortcut() bool {
	effect := strings.TrimSpace(rt.shortcut.Safety.Effect)
	if effect == "write" || effect == "destructive" {
		return true
	}
	return rt.shortcut.Risk == RiskWrite || rt.shortcut.Risk == RiskHighWrite
}

func hasExplicitShortcutSuccess(payload any) bool {
	object, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	status := object
	if content, ok := object["content"].(map[string]any); ok {
		status = content
	}
	success, present := status["success"].(bool)
	return present && success
}

func shortcutCommandResult(payload any, options ...output.ResultOption) output.CommandResult {
	if object, ok := payload.(map[string]any); ok {
		status := object
		if content, ok := object["content"].(map[string]any); ok {
			status = content
		}
		if rawSuccess, present := status["success"]; present {
			success, isBool := rawSuccess.(bool)
			if !isBool {
				return output.Failure(&output.ErrorInfo{
					Type:      "api",
					Subtype:   "invalid_success_type",
					Message:   "shortcut response success field must be a JSON boolean",
					Hint:      "写操作先核查目标状态；读取操作保留脱敏响应证据后排查上游。",
					Operation: "shortcut.response_projection",
				}, options...)
			}
			if success {
				return output.Success(payload, options...)
			}
			message := "shortcut operation failed"
			for _, key := range []string{"errorMsg", "errorMessage", "message"} {
				if value, ok := status[key].(string); ok && strings.TrimSpace(value) != "" {
					message = strings.TrimSpace(value)
					break
				}
			}
			return output.Failure(&output.ErrorInfo{Type: "api", Message: message}, options...)
		}
	}
	return output.Success(payload, options...)
}

// mount compiles a Shortcut into a cobra command through the unified command
// path. FromShortcut expands the legacy Risk only when Safety is absent; when
// Safety is explicit the same value drives both ConfirmSafety and ContractFinal.
func mount(s Shortcut) *cobra.Command {
	cmd := corecmd.New(FromShortcut(s))
	// Preserve the historical Shortcut help surface: Tips, rather than Agent
	// selection examples, own cobra's Example block. The Schema declaration still
	// carries its reviewed examples in ContractFinal.
	cmd.Example = shortcutExamples(s.Tips)
	return cmd
}

func flagHelp(f Flag) string {
	parts := make([]string, 0, 2)
	if f.Required && !strings.Contains(f.Desc, "必填") {
		parts = append(parts, "必填")
	}
	if len(f.Enum) > 0 {
		parts = append(parts, "可选值: "+strings.Join(f.Enum, ", "))
	}
	if len(parts) == 0 {
		return f.Desc
	}
	if strings.TrimSpace(f.Desc) == "" {
		return strings.Join(parts, "；")
	}
	return f.Desc + "（" + strings.Join(parts, "；") + "）"
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// globalBool reads a bool flag that may live on the command, inherited flags, or
// the root's persistent flags (e.g. --dry-run/--yes injected at root).
func globalBool(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	sets := []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()}
	if root := cmd.Root(); root != nil {
		sets = append(sets, root.PersistentFlags())
	}
	for _, set := range sets {
		if set == nil {
			continue
		}
		if f := set.Lookup(name); f != nil {
			if v, err := set.GetBool(name); err == nil {
				return v
			}
		}
	}
	return false
}
