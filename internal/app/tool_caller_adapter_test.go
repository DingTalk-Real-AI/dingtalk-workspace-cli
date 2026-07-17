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
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

func TestToolCallerTokenOverrideClearsUnpersistedRuntimeProfile(t *testing.T) {
	authpkg.SetRuntimeProfile("corp_not_persisted")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	flags := &GlobalFlags{}
	runner := runtimeProfileCaptureRunner{flags: flags}
	caller := &toolCallerAdapter{runner: runner, flags: flags}
	result, err := caller.CallToolWithToken(
		context.Background(),
		"temporary-access-token",
		"contact",
		"get_current_user_profile",
		nil,
	)
	if err != nil {
		t.Fatalf("CallToolWithToken() error = %v", err)
	}
	if got := result.Content[0].Text; got != `{"profile":"","token":"temporary-access-token"}` {
		t.Fatalf("CallToolWithToken() result = %s", got)
	}
	if authpkg.RuntimeProfile() != "corp_not_persisted" {
		t.Fatalf("runtime profile = %q, want restored selector", authpkg.RuntimeProfile())
	}
	if flags.Token != "" {
		t.Fatalf("token override leaked after call: %q", flags.Token)
	}
}

type runtimeProfileCaptureRunner struct {
	flags *GlobalFlags
}

func (r runtimeProfileCaptureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{
			"content": []any{map[string]any{
				"type": "text",
				"text": `{"profile":"` + authpkg.RuntimeProfile() + `","token":"` + r.flags.Token + `"}`,
			}},
		},
	}, nil
}
