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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RuntimeContext is handed to a Shortcut's Validate and Execute hooks. It wraps
// the cobra command and exposes typed flag accessors plus a single CallMCP entry
// point so shortcut authors never touch cobra/executor plumbing directly.
type RuntimeContext struct {
	cmd      *cobra.Command
	shortcut Shortcut
}

// Command returns the underlying cobra command (escape hatch; prefer the typed
// accessors below).
func (rt *RuntimeContext) Command() *cobra.Command { return rt.cmd }

// Str returns the trimmed string value of a flag, or "" if unset.
func (rt *RuntimeContext) Str(name string) string {
	v, _ := rt.cmd.Flags().GetString(name)
	return strings.TrimSpace(v)
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
	return helpers.CallMCPToolOnServer(rt.shortcut.product(), tool, params)
}

// CallMCPData dispatches a read-only tool call to an explicit MCP product and
// returns the PARSED response as data WITHOUT printing. This is the building
// block for multi-step ("smart") shortcuts: call a tool, read its output, feed
// it into the next call. Errors carry DWS's auth/PAT/business classification.
//
// The product is explicit (not the shortcut's own) because smart shortcuts
// routinely cross services — e.g. resolve a name via `contact` then act via
// `chat`. Reads run even under --dry-run so a preview can still resolve inputs.
// Write tools that need parsed responses must use CallMCPWriteData instead; as
// a backstop, obvious write-like tool names are rejected here under --dry-run.
func (rt *RuntimeContext) CallMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	if rt.DryRun() && looksWriteTool(tool) {
		return nil, dryRunWriteError(product, tool)
	}
	return rt.callMCPData(product, tool, params)
}

// CallMCPWriteData dispatches a write tool call and returns its parsed response.
// Unlike CallMCPData, it refuses to run under --dry-run so smart shortcuts cannot
// accidentally perform writes while rendering a preview.
func (rt *RuntimeContext) CallMCPWriteData(product, tool string, params map[string]any) (map[string]any, error) {
	if rt.DryRun() {
		return nil, dryRunWriteError(product, tool)
	}
	return rt.callMCPData(product, tool, params)
}

func dryRunWriteError(product, tool string) error {
	return apperrors.NewValidation(fmt.Sprintf(
		"--dry-run 下禁止执行写操作 %s/%s；请在 shortcut 中输出 preview 后返回", product, tool))
}

func looksWriteTool(tool string) bool {
	tool = strings.TrimSpace(strings.ToLower(tool))
	for _, prefix := range []string{
		"add_", "append_", "approve_", "archive_", "cancel_", "create_",
		"delete_", "disable_", "enable_", "grant_", "import_", "insert_",
		"invite_", "move_", "publish_", "reject_", "remove_", "replace_",
		"respond", "revoke_", "send_", "set_", "submit_", "update_",
		"upload_", "write_",
	} {
		if strings.HasPrefix(tool, prefix) {
			return true
		}
	}
	return false
}

func (rt *RuntimeContext) callMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	text, err := helpers.CallMCPToolTextOnServer(product, tool, params)
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

// Output prints a (typically reshaped/projected) payload honouring the root
// --format/--jq/--fields flags. Multi-step shortcuts use it to emit a clean,
// composed result instead of the raw MCP response — the output-projection
// output-formatting capability.
func (rt *RuntimeContext) Output(payload any) error {
	return output.WriteCommandPayload(rt.cmd, payload, output.FormatJSON)
}

// mount compiles a Shortcut into a cobra command.
func mount(s Shortcut) *cobra.Command {
	cmd := &cobra.Command{
		Use:    s.Command,
		Short:  s.Description,
		Long:   shortcutLongHelp(s),
		Hidden: s.Hidden,
	}
	if len(s.Tips) > 0 {
		cmd.Example = "  " + strings.Join(s.Tips, "\n  ")
	}
	registerFlags(cmd, s.Flags)

	cmd.RunE = func(c *cobra.Command, _ []string) error {
		rt := &RuntimeContext{cmd: c, shortcut: s}
		if err := validateFlags(rt, s); err != nil {
			return err
		}
		if err := validateConstraints(rt, s); err != nil {
			return err
		}
		if s.Validate != nil {
			if err := s.Validate(rt); err != nil {
				return err
			}
		}
		if !confirmRisk(rt, s) {
			return nil
		}
		if s.Execute == nil {
			return apperrors.NewInternal(fmt.Sprintf("shortcut %s %s 未实现 Execute", s.Service, s.Command))
		}
		return s.Execute(rt)
	}
	return cmd
}

// registerFlags declares each Flag on the command with its type/default/desc.
func registerFlags(cmd *cobra.Command, flags []Flag) {
	for _, f := range flags {
		desc := flagHelp(f)
		switch f.Type {
		case FlagBool:
			cmd.Flags().Bool(f.Name, f.Default == "true", desc)
		case FlagInt:
			cmd.Flags().Int(f.Name, atoiDefault(f.Default), desc)
		case FlagStringSlice:
			cmd.Flags().StringSlice(f.Name, nil, desc)
		default: // FlagString and empty
			cmd.Flags().String(f.Name, f.Default, desc)
		}
		if f.Hidden {
			_ = cmd.Flags().MarkHidden(f.Name)
		}
	}
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

func shortcutLongHelp(s Shortcut) string {
	long := strings.TrimSpace(s.Intent)
	if long == "" {
		long = strings.TrimSpace(s.Description)
	}
	if len(s.Constraints) == 0 {
		return long
	}
	lines := make([]string, 0, len(s.Constraints))
	for _, constraint := range s.Constraints {
		lines = append(lines, "  - "+constraintHelp(constraint))
	}
	return long + "\n\n参数约束：\n" + strings.Join(lines, "\n")
}

func constraintHelp(constraint Constraint) string {
	if strings.TrimSpace(constraint.Description) != "" {
		return constraint.Description
	}
	switch constraint.Kind {
	case ConstraintAtLeastOne:
		return fmt.Sprintf("%s 至少指定一个", dashed(constraint.Flags))
	case ConstraintExactlyOne:
		return fmt.Sprintf("%s 必须且只能指定一个", dashed(constraint.Flags))
	case ConstraintMutuallyExclusive:
		return fmt.Sprintf("%s 互斥，最多指定一个", dashed(constraint.Flags))
	default:
		return fmt.Sprintf("%s 使用未识别的约束类型 %q", dashed(constraint.Flags), constraint.Kind)
	}
}

// validateFlags enforces the declarative Required and Enum constraints.
func validateFlags(rt *RuntimeContext, s Shortcut) error {
	for _, f := range s.Flags {
		if f.Required && !rt.Changed(f.Name) {
			return apperrors.NewValidation(fmt.Sprintf("缺少必填参数 --%s：%s", f.Name, f.Desc))
		}
		if f.Required && rt.Changed(f.Name) {
			switch f.Type {
			case FlagStringSlice:
				if !hasNonEmptyString(rt.StrSlice(f.Name)) {
					return apperrors.NewValidation(fmt.Sprintf("必填参数 --%s 不能为空", f.Name))
				}
			case FlagString, "":
				if rt.Str(f.Name) == "" {
					return apperrors.NewValidation(fmt.Sprintf("必填参数 --%s 不能为空", f.Name))
				}
			}
		}
		if len(f.Enum) > 0 && rt.Changed(f.Name) {
			values := []string{rt.Str(f.Name)}
			if f.Type == FlagStringSlice {
				values = rt.StrSlice(f.Name)
			}
			for _, val := range values {
				val = strings.TrimSpace(val)
				if !contains(f.Enum, val) {
					return apperrors.NewValidation(fmt.Sprintf(
						"参数 --%s 取值 %q 不合法，允许值：%s", f.Name, val, strings.Join(f.Enum, ", ")))
				}
			}
		}
	}
	return nil
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func validateConstraints(rt *RuntimeContext, s Shortcut) error {
	for _, constraint := range s.Constraints {
		if len(constraint.Flags) == 0 {
			return apperrors.NewInternal(fmt.Sprintf(
				"shortcut %s %s 的约束 %q 未声明参数", s.Service, s.Command, constraint.Kind))
		}
		switch constraint.Kind {
		case ConstraintAtLeastOne:
			if err := rt.AtLeastOne(constraint.Flags...); err != nil {
				return err
			}
		case ConstraintExactlyOne:
			if err := rt.ExactlyOne(constraint.Flags...); err != nil {
				return err
			}
		case ConstraintMutuallyExclusive:
			if err := rt.MutuallyExclusive(constraint.Flags...); err != nil {
				return err
			}
		case ConstraintCustom:
			if strings.TrimSpace(constraint.Description) == "" {
				return apperrors.NewInternal(fmt.Sprintf(
					"shortcut %s %s 的 custom 约束缺少描述", s.Service, s.Command))
			}
		default:
			return apperrors.NewInternal(fmt.Sprintf(
				"shortcut %s %s 使用未知约束类型 %q", s.Service, s.Command, constraint.Kind))
		}
	}
	return nil
}

// confirmRisk prompts before a write/high-risk-write shortcut unless --yes or
// --dry-run is set. Read-only shortcuts never prompt. Returns false when the
// user declines.
func confirmRisk(rt *RuntimeContext, s Shortcut) bool {
	if s.risk() == RiskRead || rt.DryRun() || rt.Yes() {
		return true
	}
	fmt.Fprintf(rt.cmd.ErrOrStderr(), "即将执行 %s %s（%s），确认继续？(yes/no): ", s.Service, s.Command, s.risk())
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "yes" || answer == "y"
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
