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
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestToolCallerTokenOverrideDoesNotMutateRuntimeState(t *testing.T) {
	authpkg.SetRuntimeProfile("corp_not_persisted")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	flags := &GlobalFlags{Token: "supervisor-token"}
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
	if got := result.Content[0].Text; got != `{"profile":"corp_not_persisted","globalToken":"supervisor-token","scopedToken":"temporary-access-token"}` {
		t.Fatalf("CallToolWithToken() result = %s", got)
	}
	if authpkg.RuntimeProfile() != "corp_not_persisted" {
		t.Fatalf("runtime profile = %q, want restored selector", authpkg.RuntimeProfile())
	}
	if flags.Token != "supervisor-token" {
		t.Fatalf("token override leaked after call: %q", flags.Token)
	}
}

func TestRuntimeRunnerRequestScopedTokenBypassesStoredProfile(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DINGTALK_CONTACT_MCP_URL", "https://contact.example.test")
	authpkg.SetRuntimeProfile("profile-that-must-not-be-resolved")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	flags := &GlobalFlags{Token: "supervisor-token"}
	baseTransport := transport.NewClient(nil)
	runner := &runtimeRunner{
		transport:   baseTransport,
		globalFlags: flags,
		auditSink:   audit.NopSink{},
	}
	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
	var calledToken string
	testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, endpoint, tool string, _ map[string]any) (transport.ToolCallResult, error) {
		calledToken = client.AuthToken
		if endpoint != "https://contact.example.test" || tool != "get_current_user_profile" {
			t.Fatalf("request route = %s %s", endpoint, tool)
		}
		return transport.ToolCallResult{Content: map[string]any{"result": []any{}}}, nil
	})

	result, err := runner.RunWithToken(context.Background(), executor.NewHelperInvocation(
		"overlay.contact.get_current_user_profile",
		"contact",
		"get_current_user_profile",
		nil,
	), "temporary-access-token")
	if err != nil {
		t.Fatalf("RunWithToken() error = %v", err)
	}
	if !result.Invocation.Implemented || calledToken != "temporary-access-token" {
		t.Fatalf("scoped call result=%#v token=%q", result, calledToken)
	}
	if authpkg.RuntimeProfile() != "profile-that-must-not-be-resolved" || flags.Token != "supervisor-token" {
		t.Fatalf("runtime state changed: profile=%q token=%q", authpkg.RuntimeProfile(), flags.Token)
	}
	if baseTransport.ExecutionId != "" || len(baseTransport.ExtraHeaders) != 0 {
		t.Fatalf("base transport received request state: %#v", baseTransport)
	}
}

type employeeBindingRoundTripper func(*http.Request) (*http.Response, error)

func (f employeeBindingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCrossPlatformCoverageEmployeeBindingTransportDoesNotRetry(t *testing.T) {
	for _, tool := range []string{"bind_local_agent", "unbind_local_agent", "rebind_local_agent", "get_digital_employee_detail"} {
		t.Run(tool, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Setenv("DINGTALK_DEAP_DEV_MCP_URL", "https://binding.example.test")
			attempts := 0
			client := transport.NewClient(&http.Client{Transport: employeeBindingRoundTripper(func(r *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: 500, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: r}, nil
			})})
			client.MaxRetries = 2
			client.RetryDelay = time.Nanosecond
			client.RetryMaxDelay = time.Nanosecond
			runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{}}
			testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
				return nil
			})
			_, err := runner.RunWithToken(context.Background(), executor.NewHelperInvocation("overlay.deap-dev."+tool, "deap-dev", tool, map[string]any{"agentUuid": "test-employee"}), "supervisor-test-token")
			want := 1
			// All tools/call requests are fail-closed: a read-looking tool name
			// is not an independently reviewed replay policy.
			if err == nil || attempts != want || client.MaxRetries != 2 {
				t.Fatalf("retry contract: attempts=%d want=%d base=%d err=%v", attempts, want, client.MaxRetries, err)
			}
		})
	}
}

type runtimeProfileCaptureRunner struct {
	flags *GlobalFlags
}

func (r runtimeProfileCaptureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	return executor.Result{}, fmt.Errorf("unexpected ordinary Run for %s", invocation.Tool)
}

func (r runtimeProfileCaptureRunner) RunWithToken(_ context.Context, invocation executor.Invocation, token string) (executor.Result, error) {
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{
			"content": []any{map[string]any{
				"type": "text",
				"text": `{"profile":"` + authpkg.RuntimeProfile() + `","globalToken":"` + r.flags.Token + `","scopedToken":"` + token + `"}`,
			}},
		},
	}, nil
}
