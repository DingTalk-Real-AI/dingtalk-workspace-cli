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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestValidatedConversationCategoryTitle(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "trim", raw: "  工作群  ", want: "工作群"},
		{name: "fifteen characters", raw: "123456789012345", want: "123456789012345"},
		{name: "blank", raw: "   ", wantErr: "不能为空"},
		{name: "too long", raw: "1234567890123456", wantErr: "最多 15 个字符"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validatedConversationCategoryTitle(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageCategoryCommandsRejectInvalidTitles(t *testing.T) {
	caller := &categoryTitleCaller{}
	old := deps
	t.Cleanup(func() { deps = old })
	InitDeps(caller)

	for _, argv := range [][]string{
		{"category", "create", "--title", "1234567890123456"},
		{"category", "rename", "--category-id", "42", "--title", "   "},
	} {
		cmd := newChatCommand()
		cmd.SetArgs(argv)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("newChatCommand(%v) unexpectedly accepted an invalid title", argv)
		}
	}
	if caller.calls != 0 {
		t.Fatalf("invalid category titles reached MCP %d times", caller.calls)
	}
}

type categoryTitleCaller struct{ calls int }

func (c *categoryTitleCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{}, nil
}
func (*categoryTitleCaller) Format() string { return "json" }
func (*categoryTitleCaller) DryRun() bool   { return false }
func (*categoryTitleCaller) Fields() string { return "" }
func (*categoryTitleCaller) JQ() string     { return "" }
