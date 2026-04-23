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

package pat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// resolveSessionIDFromEnv returns the effective session id from environment
// variables. Resolution order:
//  1. DWS_SESSION_ID (primary; Stable per docs/pat/contract.md §3.1)
//  2. REWIND_SESSION_ID (compatibility alias; reference host implementation
//     compatibility — kept only so hosts that already inject the legacy
//     trace triple keep working without code churn)
//
// When both are set to different non-empty values, DWS_SESSION_ID wins and
// we emit a slog.Warn so operators can spot the inconsistency.
func resolveSessionIDFromEnv() string {
	dws := os.Getenv("DWS_SESSION_ID")
	legacy := os.Getenv("REWIND_SESSION_ID")
	if dws != "" && legacy != "" && dws != legacy {
		slog.Warn("pat: DWS_SESSION_ID and REWIND_SESSION_ID disagree; using DWS_SESSION_ID",
			"dws_session_id", dws,
			"legacy_session_id", legacy,
		)
	}
	if dws != "" {
		return dws
	}
	return legacy
}

// agentCodeEnv is the canonical (and only) environment variable name
// used as a per-shell fallback for the --agentCode flag on `dws pat *`
// commands, per docs/pat/contract.md §2.
//
// Why: agent hosts typically set their business agent code once when
// spawning a long-lived shell / sub-process; requiring `--agentCode` on
// every command in that shell forces the host to rewrite every argv.
// Exposing DINGTALK_DWS_AGENTCODE lets the host export the code once and
// let the CLI resolve it on every pat subcommand. The flag always wins
// when both are set so scripted one-offs remain deterministic.
//
// Namespace note: DWS_AGENTCODE / DINGTALK_AGENTCODE / REWIND_AGENTCODE
// are explicitly NOT consumed. The legacy DWS_AGENTCODE alias was
// hard-removed once the public integration surface landed on
// DINGTALK_DWS_AGENTCODE; hosts must migrate rather than rely on a
// silent fallback.
const agentCodeEnv = "DINGTALK_DWS_AGENTCODE"

// agentCodePattern is the validation regex for any --agentCode value
// resolved from either the flag or the agent-code env var. It matches
// documented agent-code generation schemes (e.g. md5 digests, uuid-like
// ids, short host-assigned slugs) while rejecting shell metacharacters
// and whitespace that would otherwise flow unescaped into an MCP tool
// argument. Kept in sync with docs/pat/contract.md §9.
var agentCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// resolveAgentCodeFromEnv returns the fallback agent code from the
// canonical DINGTALK_DWS_AGENTCODE env var. The second return value
// reports the env name that was consumed (for error attribution); it
// is "" when the env is unset or blank. No legacy aliases are honored.
func resolveAgentCodeFromEnv() (string, string) {
	primary := strings.TrimSpace(os.Getenv(agentCodeEnv))
	if primary != "" {
		return primary, agentCodeEnv
	}
	return "", ""
}

// validateAgentCode rejects agent codes that would be ambiguous or unsafe
// once spliced into a shell / MCP argv. Allowed character set is
// [A-Za-z0-9_-], length 1..64 — see agentCodePattern above.
func validateAgentCode(code string) error {
	if code == "" {
		return fmt.Errorf("--agentCode must not be empty")
	}
	if !agentCodePattern.MatchString(code) {
		return fmt.Errorf(
			"invalid agentCode %q: must match %s (A-Z, a-z, 0-9, _, -; 1..64 chars)",
			code, agentCodePattern.String())
	}
	return nil
}

// resolveAgentCode implements the canonical two-tier lookup for
// --agentCode documented at docs/pat/contract.md §9 and the SSOT
// third-party integration guide §3:
//
//  1. explicit --agentCode flag value (highest priority; wins over env)
//  2. DINGTALK_DWS_AGENTCODE env var (per-shell primary fallback)
//  3. empty ("") when required=false; typed error when required=true.
//
// Any non-empty resolved value is validated via validateAgentCode, so
// callers never have to re-validate.
func resolveAgentCode(flagVal string, required bool) (string, error) {
	code := strings.TrimSpace(flagVal)
	envSource := ""
	if code == "" {
		code, envSource = resolveAgentCodeFromEnv()
	}
	if code == "" {
		if required {
			return "", fmt.Errorf(
				"flag --agentCode is required (or set env %s)\n  hint: dws pat chmod <scope>... --agentCode <id>\n  hint: export %s=<id>",
				agentCodeEnv, agentCodeEnv)
		}
		return "", nil
	}
	if err := validateAgentCode(code); err != nil {
		if envSource != "" {
			return "", fmt.Errorf("%s env: %w", envSource, err)
		}
		return "", err
	}
	return code, nil
}

const (
	// patGrantToolName is the English-first wire name for the PAT grant tool.
	patGrantToolName = "pat.grant"

	// patGrantToolNameLegacyAlias is retained for server builds that still
	// expose only the legacy Chinese display name.
	patGrantToolNameLegacyAlias = "个人授权"

	// patApplyToolName is the English-first wire name used by the
	// orchestrator entry `dws pat apply`.
	patApplyToolName = "pat.apply"

	// patStatusToolName is the English-first wire name used by
	// `dws pat status`.
	patStatusToolName = "pat.status"

	// patScopesToolName is the English-first wire name used by
	// `dws pat scopes`.
	patScopesToolName = "pat.scopes"
)

var validGrantTypes = map[string]bool{
	"once":      true,
	"session":   true,
	"permanent": true,
}

// newChmodCommand builds a fresh `dws pat chmod` cobra.Command wired to
// the supplied ToolCaller. A factory is used (instead of a package-level
// var) so multiple RegisterCommands invocations never share mutable flag /
// RunE state across concurrent tests.
func newChmodCommand(c edition.ToolCaller) *cobra.Command {
	chmodCmd := &cobra.Command{
		Use:   "chmod <scope>...",
		Short: "授予指定权限",
		Long: `授予指定 scope 的操作权限。

scope 格式: <product>.<entity>:<permission>
  例: aitable.record:read  chat.group:write  calendar.event:read

grantType 规则:
  once       一次性，执行一次后自动失效
  session    当前会话有效（默认），需要 --session-id
  permanent  永久有效`,
		Args: cobra.MinimumNArgs(1),
		Example: `  dws pat chmod aitable.record:read --agentCode agt-xxxx --grant-type session --session-id session-xxx
  dws pat chmod chat.message:list --grant-type once --agentCode agt-xxxx
  dws pat chmod aitable.record:read aitable.record:write --agentCode agt-xxxx --grant-type permanent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			flagVal, _ := cmd.Flags().GetString("agentCode")
			agentCode, err := resolveAgentCode(flagVal, true)
			if err != nil {
				return err
			}
			scopes := args
			grantType, _ := cmd.Flags().GetString("grant-type")
			sessionID, _ := cmd.Flags().GetString("session-id")

			if !validGrantTypes[grantType] {
				return fmt.Errorf("invalid --grant-type %q, must be one of: once, session, permanent", grantType)
			}

			if grantType == "session" && sessionID == "" && resolveSessionIDFromEnv() == "" {
				return fmt.Errorf("--session-id is required when --grant-type is session\n  hint: dws pat chmod <scope> --agentCode <id> --grant-type session --session-id <id>")
			}

			if c != nil && c.DryRun() {
				bold := color.New(color.FgYellow, color.Bold)
				bold.Println("[DRY-RUN] Preview only, not executed:")
				fmt.Printf("%-16s%s\n", "Tool:", patGrantToolName)
				fmt.Printf("%-16s%s\n", "AgentCode:", agentCode)
				fmt.Printf("%-16s%v\n", "Scope:", scopes)
				fmt.Printf("%-16s%s\n", "GrantType:", grantType)
				if sessionID != "" {
					fmt.Printf("%-16s%s\n", "SessionID:", sessionID)
				}
				return nil
			}

			if c == nil {
				return fmt.Errorf("internal error: tool runtime not initialized")
			}

			toolArgs := map[string]any{
				"agentCode": agentCode,
				"scopes":    scopes,
				"grantType": grantType,
			}
			legacyToolArgs := map[string]any{
				"agentCode": agentCode,
				"scope":     scopes,
				"grantType": grantType,
			}
			if sessionID == "" {
				sessionID = resolveSessionIDFromEnv()
			}
			if sessionID != "" {
				toolArgs["sessionId"] = sessionID
				legacyToolArgs["sessionId"] = sessionID
			}

			ctx := context.Background()
			result, err := callPATToolWithLegacyFallback(ctx, c, "pat", patGrantToolName, patGrantToolNameLegacyAlias, toolArgs, legacyToolArgs)
			if err != nil {
				return fmt.Errorf("pat chmod failed: %w", err)
			}

			return handleToolResult(result)
		},
	}

	// --agentCode is required, but we deliberately do NOT call
	// MarkFlagRequired here. The agent code may also come from the
	// DINGTALK_DWS_AGENTCODE env var (per docs/pat/contract.md §9 and
	// contract.md §2); cobra's MarkFlagRequired would refuse to run
	// before our resolver has a chance to consume the env.
	chmodCmd.Flags().String("agentCode", "",
		"Agent 唯一标识（必填；亦可通过 env DINGTALK_DWS_AGENTCODE 注入，flag 优先）")
	chmodCmd.Flags().String("grant-type", "session", "授权策略: once|session|permanent")
	chmodCmd.Flags().String("session-id", "", "会话标识（session 模式下必填）")

	return chmodCmd
}

// callPATToolWithLegacyFallback invokes the canonical PAT grant tool first,
// then silently retries the legacy Chinese alias when the server has not
// registered the canonical tool yet. The retry intentionally emits no stderr
// banner because host-owned PAT callers parse stderr as machine JSON.
func callPATToolWithLegacyFallback(ctx context.Context, c edition.ToolCaller, productID, toolName, legacyAlias string, toolArgs, legacyArgs map[string]any) (*edition.ToolResult, error) {
	if c == nil {
		return nil, fmt.Errorf("internal error: tool runtime not initialized")
	}
	result, err := c.CallTool(ctx, productID, toolName, toolArgs)
	if err == nil && !isEmptyToolResult(result) {
		return result, nil
	}
	if legacyAlias == "" {
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	if err != nil && !isToolNotRegisteredError(err) {
		return nil, err
	}
	return c.CallTool(ctx, productID, legacyAlias, legacyArgs)
}

func isEmptyToolResult(result *edition.ToolResult) bool {
	if result == nil || len(result.Content) == 0 {
		return true
	}
	for _, block := range result.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return false
		}
	}
	return true
}

// isToolNotRegisteredError reports whether err looks like a server-side
// tool-not-registered / tool-not-found classification. We match on a few
// conservative substrings rather than a structured error type because the
// upstream runner surfaces the server message as plain text.
func isToolNotRegisteredError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"tool_not_found",
		"mcp_tool_not_found",
		"tool not found",
		"tool not registered",
		"tool not exist",
		"tool does not exist",
		"unknown tool",
		"no such tool",
	}
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// handleToolResult processes a ToolResult and writes output to stdout.
func handleToolResult(result *edition.ToolResult) error {
	if result == nil {
		return fmt.Errorf("empty tool result")
	}
	for _, c := range result.Content {
		if c.Type != "text" || c.Text == "" {
			continue
		}
		if respErr := apperrors.ClassifyMCPResponseText(c.Text); respErr != nil {
			return respErr
		}
		fmt.Println(c.Text)
		return nil
	}
	data, _ := json.Marshal(result)
	return fmt.Errorf("empty PAT authorization result: %s", string(data))
}
