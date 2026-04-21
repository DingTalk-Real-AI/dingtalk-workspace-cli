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
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// applyCmd is the `dws pat apply` orchestrator entry. When a host receives
// exit=4 plus the PAT stderr JSON (see docs/pat/contract.md §2), it can
// invoke this subcommand to actively request the missing scopes in a single
// orchestration round-trip instead of replaying the original business
// command. Exit codes and error classification follow the same rules as
// `dws pat chmod`.
var applyCmd = &cobra.Command{
	Use:   "apply <scope>...",
	Short: "主动申请指定 scope（PAT orchestrator entry）",
	Long: `主动发起一次 PAT scope 申请。

典型使用场景：宿主在收到 exit=4 + PAT stderr JSON 后，直接调用本命令
发起"申请这些 scope"的 orchestration 请求，而不是重跑原业务命令。

scope 格式: <product>.<entity>:<permission>
  例: aitable.record:read  chat.group:write

grantType 规则同 chmod:
  once       一次性
  session    当前会话有效（默认），需要 --session-id
  permanent  永久有效

stdout 输出：JSON 形式的 {"success":true,"authRequestId":"..."}。
authRequestId 用于宿主后续绑定 pat status 查询。`,
	Args: cobra.MinimumNArgs(1),
	Example: `  dws pat apply aitable.record:read --agentCode agt-xxxx --grant-type session --session-id conv-001
  dws pat apply doc.file:create --agentCode agt-xxxx --grant-type once`,
	RunE: runApply,
}

func init() {
	// --agentCode resolves via the canonical two-tier chain
	// (flag > DINGTALK_DWS_AGENTCODE env > error). See resolveAgentCode and
	// docs/pat/contract.md §9. MarkFlagRequired is deliberately NOT used —
	// the env fallback must get a chance.
	applyCmd.Flags().String("agentCode", "",
		"Agent 唯一标识（必填；亦可通过 env DINGTALK_DWS_AGENTCODE 注入，flag 优先）")
	applyCmd.Flags().String("grant-type", "session", "授权策略: once|session|permanent")
	applyCmd.Flags().String("session-id", "", "会话标识（session 模式下必填；兜底顺序 $DWS_SESSION_ID → $REWIND_SESSION_ID）")
}

func runApply(cmd *cobra.Command, args []string) error {
	flagVal, _ := cmd.Flags().GetString("agentCode")
	agentCode, err := resolveAgentCode(flagVal, true)
	if err != nil {
		return err
	}
	grantType, _ := cmd.Flags().GetString("grant-type")
	sessionID, _ := cmd.Flags().GetString("session-id")
	scopes := args

	if !validGrantTypes[grantType] {
		return fmt.Errorf("invalid --grant-type %q, must be one of: once, session, permanent", grantType)
	}
	if grantType == "session" && sessionID == "" && resolveSessionIDFromEnv() == "" {
		return fmt.Errorf("--session-id is required when --grant-type is session\n  hint: dws pat apply <scope>... --grant-type session --session-id <id>")
	}

	toolArgs := map[string]any{
		"scope":     scopes,
		"grantType": grantType,
		"agentCode": agentCode,
	}
	if sessionID == "" {
		sessionID = resolveSessionIDFromEnv()
	}
	if sessionID != "" {
		toolArgs["sessionId"] = sessionID
	}

	if caller != nil && caller.DryRun() {
		fmt.Printf("[DRY-RUN] %s\n", patApplyToolName)
		b, _ := json.MarshalIndent(toolArgs, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	if caller == nil {
		return fmt.Errorf("internal error: tool runtime not initialized")
	}

	ctx := context.Background()
	result, err := callPATToolWithLegacyFallback(ctx, "pat", patApplyToolName, patGrantToolName, toolArgs)
	if err != nil {
		return fmt.Errorf("pat apply failed: %w", err)
	}

	return emitApplyResult(result)
}

// emitApplyResult extracts the authRequestId from the server's first text
// content block and prints a stable `{"success":true,"authRequestId":"..."}`
// JSON line to stdout. When the server did not return an authRequestId, we
// synthesise a client-side UUID v4 so hosts always have a non-empty
// correlation id to bind against.
func emitApplyResult(result *edition.ToolResult) error {
	if result == nil {
		return fmt.Errorf("empty tool result")
	}

	authRequestID := ""
	for _, c := range result.Content {
		if c.Type != "text" || c.Text == "" {
			continue
		}
		if respErr := apperrors.ClassifyMCPResponseText(c.Text); respErr != nil {
			return respErr
		}
		if id := extractAuthRequestID(c.Text); id != "" {
			authRequestID = id
			break
		}
	}
	if authRequestID == "" {
		authRequestID = generateClientAuthRequestID()
	}

	payload := map[string]any{
		"success":       true,
		"authRequestId": authRequestID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal apply response: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

// extractAuthRequestID scans a JSON text blob for data.authRequestId or the
// top-level authRequestId field (some server builds place it at the root).
// Returns "" when no id is present or text is not JSON.
func extractAuthRequestID(text string) string {
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return ""
	}
	if data, ok := body["data"].(map[string]any); ok {
		if id, ok := data["authRequestId"].(string); ok && id != "" {
			return id
		}
	}
	if id, ok := body["authRequestId"].(string); ok && id != "" {
		return id
	}
	return ""
}

// generateClientAuthRequestID returns a RFC-4122 UUID v4 string. It is used
// only as a client-side correlation id when the server omits authRequestId
// in its response payload.
func generateClientAuthRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "cli-auth-req-fallback"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
