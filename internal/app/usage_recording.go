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
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/usage"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// recordingToolCaller decorates a ToolCaller to record the SHAPE of every MCP
// tool call into the local usage log (see internal/shortcut/usage). It is the
// single chokepoint through which helper and shortcut commands dispatch, so one
// wrapper captures all real usage. Recording never affects the call result and
// is skipped for dry-run.
type recordingToolCaller struct{ inner edition.ToolCaller }

func newRecordingToolCaller(inner edition.ToolCaller) edition.ToolCaller {
	return recordingToolCaller{inner: inner}
}

func (r recordingToolCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	recordedArgs := cloneToolArgs(args)
	res, err := r.inner.CallTool(ctx, product, tool, args)
	usage.Append(product, tool, recordedArgs, err == nil, r.inner.DryRun())
	return res, err
}

func (r recordingToolCaller) CallToolWithToken(
	ctx context.Context,
	token, product, tool string,
	args map[string]any,
) (*edition.ToolResult, error) {
	inner, ok := r.inner.(tokenOverrideToolCaller)
	if !ok {
		return nil, fmt.Errorf("ToolCaller token override is not configured")
	}
	recordedArgs := cloneToolArgs(args)
	res, err := inner.CallToolWithToken(ctx, token, product, tool, args)
	usage.Append(product, tool, recordedArgs, err == nil, r.inner.DryRun())
	return res, err
}

func (r recordingToolCaller) Format() string { return r.inner.Format() }
func (r recordingToolCaller) DryRun() bool   { return r.inner.DryRun() }
func (r recordingToolCaller) Fields() string { return r.inner.Fields() }
func (r recordingToolCaller) JQ() string     { return r.inner.JQ() }

func cloneToolArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}
