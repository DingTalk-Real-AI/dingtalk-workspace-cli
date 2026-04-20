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
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var validGrantTypes = map[string]bool{
	"once":      true,
	"session":   true,
	"permanent": true,
}

func newChmodCommand(caller edition.ToolCaller) *cobra.Command {
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
			agentCode, _ := cmd.Flags().GetString("agentCode")
			if agentCode == "" {
				return fmt.Errorf("flag --agentCode is required\n  hint: dws pat chmod <scope>... --agentCode <id>")
			}
			scopes := args
			grantType, _ := cmd.Flags().GetString("grant-type")
			sessionID, _ := cmd.Flags().GetString("session-id")

			if !validGrantTypes[grantType] {
				return fmt.Errorf("invalid --grant-type %q, must be one of: once, session, permanent", grantType)
			}

			if grantType == "session" && sessionID == "" && os.Getenv("DWS_SESSION_ID") == "" {
				return fmt.Errorf("--session-id is required when --grant-type is session\n  hint: dws pat chmod <scope> --agentCode <id> --grant-type session --session-id <id>")
			}

			if caller != nil && caller.DryRun() {
				bold := color.New(color.FgYellow, color.Bold)
				bold.Println("[DRY-RUN] Preview only, not executed:")
				fmt.Printf("%-16s%s\n", "Tool:", "个人授权")
				fmt.Printf("%-16s%s\n", "AgentCode:", agentCode)
				fmt.Printf("%-16s%v\n", "Scope:", scopes)
				fmt.Printf("%-16s%s\n", "GrantType:", grantType)
				if sessionID != "" {
					fmt.Printf("%-16s%s\n", "SessionID:", sessionID)
				}
				return nil
			}

			if caller == nil {
				return fmt.Errorf("internal error: tool runtime not initialized")
			}

			toolArgs := map[string]any{
				"agentCode": agentCode,
				"scope":     scopes,
				"grantType": grantType,
			}
			if sessionID == "" {
				sessionID = os.Getenv("DWS_SESSION_ID")
			}
			if sessionID != "" {
				toolArgs["sessionId"] = sessionID
			}

			ctx := context.Background()
			result, err := caller.CallTool(ctx, "pat", "个人授权", toolArgs)
			if err != nil {
				return fmt.Errorf("pat chmod failed: %w", err)
			}

			return handleToolResult(result)
		},
	}

	chmodCmd.Flags().String("agentCode", "", "Agent 唯一标识（必填）")
	_ = chmodCmd.MarkFlagRequired("agentCode")
	chmodCmd.Flags().String("grant-type", "session", "授权策略: once|session|permanent")
	chmodCmd.Flags().String("session-id", "", "会话标识（session 模式下必填）")

	return chmodCmd
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
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
