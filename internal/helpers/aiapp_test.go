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

type aiappCommandRunner struct {
	last executor.Invocation
}

func (r *aiappCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

func TestAIAppModifyMatchesWukongPayload(t *testing.T) {
	t.Parallel()

	runner := &aiappCommandRunner{}
	cmd := aiappHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"modify",
		"--prompt", "根据新图片优化首页视觉风格",
		"--thread-id", "THREAD_001",
		"--skills", "s1,s2",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "modify_ai_app" {
		t.Fatalf("tool = %q, want modify_ai_app", runner.last.Tool)
	}
	if got := runner.last.Params["threadId"]; got != "THREAD_001" {
		t.Fatalf("threadId = %#v, want THREAD_001", got)
	}
	if _, ok := runner.last.Params["attachments"]; ok {
		t.Fatalf("attachments should not be sent by modify_ai_app: %#v", runner.last.Params["attachments"])
	}
	skills, ok := runner.last.Params["officialSkillUids"].([]string)
	if !ok || len(skills) != 2 || skills[0] != "s1" || skills[1] != "s2" {
		t.Fatalf("officialSkillUids = %#v, want [s1 s2]", runner.last.Params["officialSkillUids"])
	}
}

func TestAIAppCreateRejectsInvalidAttachmentsJSON(t *testing.T) {
	t.Parallel()

	runner := &aiappCommandRunner{}
	cmd := aiappHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"create", "--prompt", "创建应用", "--attachments", `{"bad":true}`})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want JSON array validation failure")
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}
