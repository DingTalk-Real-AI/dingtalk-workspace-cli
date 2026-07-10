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

package helpers

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatSmartCategoryCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type chatSmartCategoryCaller struct {
	calls []chatSmartCategoryCall
}

func (c *chatSmartCategoryCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatSmartCategoryCall{
		productID: productID,
		toolName:  toolName,
		args:      args,
	})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*chatSmartCategoryCaller) Format() string { return "json" }
func (*chatSmartCategoryCaller) DryRun() bool   { return false }
func (*chatSmartCategoryCaller) Fields() string { return "" }
func (*chatSmartCategoryCaller) JQ() string     { return "" }

func TestChatCategoryCreateSmartUsesMCPParameterNames(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	caller := &chatSmartCategoryCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard

	cmd := newChatCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{
		"category", "create-smart",
		"--name", "priority",
		"--keywords", "alpha,beta",
		"--members", "open-id-1,open-id-2",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("chat category create-smart returned error: %v", err)
	}

	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "im" || call.toolName != "create_smart_conv_category" {
		t.Fatalf("tool call = %s/%s, want im/create_smart_conv_category", call.productID, call.toolName)
	}
	want := map[string]any{
		"categoryName":          "priority",
		"groupNameKeywords":     []string{"alpha", "beta"},
		"memberOpenDingTalkIds": []string{"open-id-1", "open-id-2"},
	}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("tool args = %#v, want %#v", call.args, want)
	}
}
