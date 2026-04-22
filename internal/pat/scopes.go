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

	"github.com/spf13/cobra"
)

// scopesCmd is `dws pat scopes`. It lists the scopes currently granted to a
// business agent. When --agentCode is omitted the server falls back to its
// default agent.
var scopesCmd = &cobra.Command{
	Use:   "scopes",
	Short: "列出当前已授权的 scope",
	Long: `查询指定 agent 当前已授权的 scope 列表。

flag:
  --agentCode <id>   可选；缺省时服务端使用 default agent

stdout 原样透传服务端返回的 text content。`,
	Args: cobra.NoArgs,
	Example: `  dws pat scopes
  dws pat scopes --agentCode agt-xxxx`,
	RunE: runScopes,
}

func init() {
	scopesCmd.Flags().String("agentCode", "",
		"Agent 唯一标识（可选；亦可通过 env DINGTALK_DWS_AGENTCODE 注入，flag 优先；缺省时服务端使用 default agent）")
}

func runScopes(cmd *cobra.Command, _ []string) error {
	flagVal, _ := cmd.Flags().GetString("agentCode")
	// required=false: for `dws pat scopes` an empty agentCode is an
	// intentional signal to the server ("use the default agent"). But if
	// the host exported DINGTALK_DWS_AGENTCODE we honour it so that a
	// single env per shell covers all pat subcommands consistently.
	// See docs/pat/contract.md §9.
	agentCode, err := resolveAgentCode(flagVal, false)
	if err != nil {
		return err
	}

	toolArgs := map[string]any{}
	if agentCode != "" {
		toolArgs["agentCode"] = agentCode
	}

	if caller != nil && caller.DryRun() {
		fmt.Printf("[DRY-RUN] %s\n", patScopesToolName)
		b, _ := json.MarshalIndent(toolArgs, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	if caller == nil {
		return fmt.Errorf("internal error: tool runtime not initialized")
	}

	ctx := context.Background()
	result, err := caller.CallTool(ctx, "pat", patScopesToolName, toolArgs)
	if err != nil {
		return fmt.Errorf("pat scopes failed: %w", err)
	}
	return emitPassthroughResult(result)
}
