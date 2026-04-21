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

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// statusCmd is `dws pat status [<authRequestId>]`. It queries the async PAT
// authorization state for the given correlation id. When the positional
// argument is omitted the command reads $DWS_PAT_AUTH_REQUEST_ID; both being
// empty is a hard error.
var statusCmd = &cobra.Command{
	Use:   "status [<authRequestId>]",
	Short: "查询异步 PAT 授权流程状态",
	Long: `查询由 ` + "`dws pat apply`" + ` 或服务端 PAT orchestrator 返回的 authRequestId
当前所处的授权状态。

参数 / 环境变量:
  <authRequestId>            位置参数，优先于环境变量
  $DWS_PAT_AUTH_REQUEST_ID   位置参数缺失时的兜底来源

exit code:
  0   已拿到终态（approved / rejected / expired），stdout 为服务端 JSON
  2   身份层鉴权失败（需要重新 dws auth login）
  4   本次查询本身触发了 PAT 拦截（罕见，遵循契约）
  1/5 业务错 / 内部错`,
	Args: cobra.MaximumNArgs(1),
	Example: `  dws pat status req-001
  DWS_PAT_AUTH_REQUEST_ID=req-001 dws pat status`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	authRequestID := ""
	if len(args) == 1 {
		authRequestID = args[0]
	}
	if authRequestID == "" {
		authRequestID = os.Getenv("DWS_PAT_AUTH_REQUEST_ID")
	}
	if authRequestID == "" {
		return fmt.Errorf("authRequestId is required (positional arg or $DWS_PAT_AUTH_REQUEST_ID)")
	}

	toolArgs := map[string]any{"authRequestId": authRequestID}

	if caller != nil && caller.DryRun() {
		fmt.Printf("[DRY-RUN] %s\n", patStatusToolName)
		b, _ := json.MarshalIndent(toolArgs, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	if caller == nil {
		return fmt.Errorf("internal error: tool runtime not initialized")
	}

	ctx := context.Background()
	result, err := caller.CallTool(ctx, "pat", patStatusToolName, toolArgs)
	if err != nil {
		return fmt.Errorf("pat status failed: %w", err)
	}
	return emitPassthroughResult(result)
}

// emitPassthroughResult writes the first text content block verbatim to
// stdout. Non-text blocks (or a nil result) fall back to an indented JSON
// marshal of the entire ToolResult. apperrors.ClassifyMCPResponseText is
// applied so exit codes still reflect gateway-auth / PAT / business errors.
func emitPassthroughResult(result *edition.ToolResult) error {
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
