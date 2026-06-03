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
	"bytes"
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type devdocCommandRunner struct {
	last executor.Invocation
}

func (r *devdocCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

func TestDevdocArticleSearchAcceptsWukongKeywordAlias(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--keyword", "openConversationId", "--page", "2", "--size", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "search_open_platform_docs" {
		t.Fatalf("tool = %q, want search_open_platform_docs", runner.last.Tool)
	}
	if got := runner.last.Params["keyword"]; got != "openConversationId" {
		t.Fatalf("keyword = %#v, want openConversationId", got)
	}
	if got := runner.last.Params["page"]; got != 2 {
		t.Fatalf("page = %#v, want 2", got)
	}
	if got := runner.last.Params["size"]; got != 5 {
		t.Fatalf("size = %#v, want 5", got)
	}
}

func TestDevdocArticleSearchAcceptsPositionalKeyword(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "MCP"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Params["keyword"]; got != "MCP" {
		t.Fatalf("keyword = %#v, want MCP", got)
	}
}
