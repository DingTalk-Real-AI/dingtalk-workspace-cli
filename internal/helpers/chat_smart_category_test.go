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
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
		"--keywords", "alpha, ,beta,",
		"--members", "open-id-1, ,open-id-2,",
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

func TestChatCategoryCreateSmartRejectsBlankInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "empty name",
			args:    []string{"--name", ""},
			wantErr: "--name must not be blank",
		},
		{
			name:    "blank name",
			args:    []string{"--name", " \t "},
			wantErr: "--name must not be blank",
		},
		{
			name:    "empty keywords",
			args:    []string{"--name", "priority", "--keywords", ""},
			wantErr: "--keywords must contain at least one non-empty value",
		},
		{
			name:    "blank keywords CSV",
			args:    []string{"--name", "priority", "--keywords", " , , "},
			wantErr: "--keywords must contain at least one non-empty value",
		},
		{
			name:    "empty keywords JSON array",
			args:    []string{"--name", "priority", "--keywords", "[]"},
			wantErr: "--keywords must contain at least one non-empty value",
		},
		{
			name:    "empty members",
			args:    []string{"--name", "priority", "--members", ""},
			wantErr: "--members must contain at least one non-empty value",
		},
		{
			name:    "blank members CSV",
			args:    []string{"--name", "priority", "--members", " , , "},
			wantErr: "--members must contain at least one non-empty value",
		},
		{
			name:    "blank members JSON array",
			args:    []string{"--name", "priority", "--members", `["", " "]`},
			wantErr: "--members must contain at least one non-empty value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousDeps := deps
			t.Cleanup(func() { deps = previousDeps })

			caller := &chatSmartCategoryCaller{}
			InitDeps(caller)
			deps.Out.w = io.Discard

			cmd := newChatCommand()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs(append([]string{"category", "create-smart"}, tt.args...))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if got := apperrors.ExitCode(err); got != 3 {
				t.Fatalf("exit code = %d, want validation code 3", got)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool call count = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestChatCategoryCreateSmartAllowsOmittedRulesAndTrimsName(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	caller := &chatSmartCategoryCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard

	cmd := newChatCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"category", "create-smart", "--name", "  priority  "})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("chat category create-smart returned error: %v", err)
	}

	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	want := map[string]any{"categoryName": "priority"}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, want)
	}
}
