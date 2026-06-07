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
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// scriptedRunner returns a scripted Result per call so we can drive the
// submit → poll(query) loop deterministically.
type scriptedRunner struct {
	calls []executor.Invocation
	fn    func(call int, inv executor.Invocation) executor.Result
}

func (r *scriptedRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	idx := len(r.calls)
	r.calls = append(r.calls, inv)
	return r.fn(idx, inv), nil
}

// mcpEnvelope reproduces the real server response shape the executor hands back:
// Response{"content":{"errorCode","errorMsg","success","result":{...}}}. Tests
// must use this (not a flat map) or they would miss the nested-unwrap bug.
func mcpEnvelope(result map[string]any) map[string]any {
	return map[string]any{
		"endpoint": "https://mcp-gw.dingtalk.com/server/op-app",
		"content": map[string]any{
			"errorCode": nil,
			"errorMsg":  nil,
			"success":   true,
			"result":    result,
		},
	}
}

// instantSleep replaces the poll-wait with a no-op so tests run without delay.
func instantSleep(t *testing.T) {
	t.Helper()
	prev := robotCreateSleepFn
	robotCreateSleepFn = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { robotCreateSleepFn = prev })
}

func runBotCreate(t *testing.T, runner executor.Runner, args ...string) (string, error) {
	t.Helper()
	cmd := newConnectBotCreateCommand(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestBotCreatePollsUntilSuccess(t *testing.T) {
	instantSleep(t)

	runner := &scriptedRunner{fn: func(call int, inv executor.Invocation) executor.Result {
		switch call {
		case 0: // submit
			return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{
				"taskId": "T1", "status": "WAITING",
				"interval": float64(1), "expiresIn": float64(60),
			})}
		case 1: // first poll → still waiting
			return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{"status": "WAITING"})}
		default: // second poll → success
			return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{
				"status": "SUCCESS", "agentId": "ag1", "robotCode": "rc1",
				"clientId": "ci1", "clientSecret": "cs1",
			})}
		}
	}}

	out, err := runBotCreate(t, runner, "--app-name", "a", "--robot-name", "b", "--desc", "c")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	// First call must be the async submit, routed to opendev.
	if got := runner.calls[0].Tool; got != robotCreateSubmitTool {
		t.Fatalf("first tool = %q, want %q", got, robotCreateSubmitTool)
	}
	if got := runner.calls[0].CanonicalProduct; got != "opendev" {
		t.Fatalf("product = %q, want opendev", got)
	}
	// Subsequent calls poll query with the submitted taskId.
	if got := runner.calls[1].Tool; got != robotCreateQueryTool {
		t.Fatalf("poll tool = %q, want %q", got, robotCreateQueryTool)
	}
	if got := runner.calls[1].Params["taskId"]; got != "T1" {
		t.Fatalf("poll taskId = %#v, want T1", got)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls = %d (submit + 2 polls expected)", len(runner.calls))
	}
	for _, want := range []string{"SUCCESS", "agentId", "robotCode", "clientId", "clientSecret", "cs1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBotCreateFailSurfacesTaskID(t *testing.T) {
	instantSleep(t)

	runner := &scriptedRunner{fn: func(call int, inv executor.Invocation) executor.Result {
		if call == 0 {
			return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{
				"taskId": "T-FAIL", "interval": float64(1), "expiresIn": float64(60),
			})}
		}
		return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{"status": "FAIL"})}
	}}

	_, err := runBotCreate(t, runner, "--app-name", "a", "--robot-name", "b", "--desc", "c")
	if err == nil {
		t.Fatal("expected error on FAIL, got nil")
	}
	if !strings.Contains(err.Error(), "T-FAIL") || !strings.Contains(err.Error(), "--task-id") {
		t.Fatalf("error should carry taskId + retry hint, got: %v", err)
	}
}

func TestBotCreateDeadlineSurfacesTaskID(t *testing.T) {
	instantSleep(t)

	runner := &scriptedRunner{fn: func(call int, inv executor.Invocation) executor.Result {
		if call == 0 {
			return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{
				"taskId": "T-SLOW", "interval": float64(2), "expiresIn": float64(5),
			})}
		}
		return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{"status": "WAITING"})}
	}}

	_, err := runBotCreate(t, runner, "--app-name", "a", "--robot-name", "b", "--desc", "c")
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
	if !strings.Contains(err.Error(), "T-SLOW") {
		t.Fatalf("deadline error should carry taskId, got: %v", err)
	}
}

func TestBotCreatePassesTaskIDOnRetry(t *testing.T) {
	instantSleep(t)

	runner := &scriptedRunner{fn: func(call int, inv executor.Invocation) executor.Result {
		if call == 0 {
			return executor.Result{Invocation: inv, Response: mcpEnvelope(map[string]any{
				"status": "SUCCESS", "agentId": "ag", "robotCode": "rc",
			})}
		}
		return executor.Result{Invocation: inv}
	}}

	_, err := runBotCreate(t, runner, "--app-name", "a", "--robot-name", "b", "--desc", "c", "--task-id", "PRIOR-1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := runner.calls[0].Params["taskId"]; got != "PRIOR-1" {
		t.Fatalf("submit taskId = %#v, want PRIOR-1", got)
	}
}
