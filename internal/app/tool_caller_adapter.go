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

package app

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// toolCallerAdapter bridges executor.Runner to the public edition.ToolCaller
// interface so that private overlays can invoke MCP tools without importing
// internal packages.
type toolCallerAdapter struct {
	runner executor.Runner
	flags  *GlobalFlags
}

func newToolCallerAdapter(runner executor.Runner, flags *GlobalFlags) edition.ToolCaller {
	return &toolCallerAdapter{runner: runner, flags: flags}
}

func (a *toolCallerAdapter) CallTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	inv := executor.NewHelperInvocation("overlay."+productID+"."+toolName, productID, toolName, args)
	result, err := a.runner.Run(ctx, inv)
	if err != nil {
		return nil, err
	}
	return convertResult(result), nil
}

func (a *toolCallerAdapter) Format() string {
	if a.flags != nil {
		return a.flags.Format
	}
	return "json"
}

func (a *toolCallerAdapter) DryRun() bool {
	return a.flags != nil && a.flags.DryRun
}

func convertResult(r executor.Result) *edition.ToolResult {
	resp := r.Response
	if resp == nil {
		// After the fix-wukong-discovery-missing-servers Phase 3 change,
		// runtimeRunner.Run returns an explicit error for catalog misses
		// instead of an empty Response, so this branch should only be
		// reachable for unit tests / unexpected runners. Log a warning so
		// any future regression (silent `{"Content": null}` on the CLI)
		// leaves a trace in the file logger / stderr.
		slog.Warn(
			"tool_caller_adapter: empty runner response — upstream should surface an error instead",
			"product", r.Invocation.CanonicalProduct,
			"tool", r.Invocation.Tool,
			"kind", r.Invocation.Kind,
			"dry_run", r.Invocation.DryRun,
		)
		return &edition.ToolResult{}
	}

	// The runtime runner stores MCP response content under "content".
	contentRaw, ok := resp["content"]
	if !ok {
		// Dry-run or echo mode: serialize the whole response as text.
		data, _ := json.Marshal(resp)
		return &edition.ToolResult{
			Content: []edition.ContentBlock{{Type: "text", Text: string(data)}},
		}
	}

	// Content may be a []any of {type, text} blocks from the MCP response,
	// or a single map for mock mode.
	switch v := contentRaw.(type) {
	case []any:
		blocks := make([]edition.ContentBlock, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				blocks = append(blocks, edition.ContentBlock{
					Type: strVal(m, "type"),
					Text: strVal(m, "text"),
				})
			}
		}
		return &edition.ToolResult{Content: blocks}
	case map[string]any:
		data, _ := json.Marshal(v)
		return &edition.ToolResult{
			Content: []edition.ContentBlock{{Type: "text", Text: string(data)}},
		}
	default:
		data, _ := json.Marshal(contentRaw)
		return &edition.ToolResult{
			Content: []edition.ContentBlock{{Type: "text", Text: string(data)}},
		}
	}
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
