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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// onboardSeqRunner returns canned responses in call order and records each
// invocation so tests can assert which tool was called with what params, with
// no real network/side effect.
type onboardSeqRunner struct {
	responses []map[string]any
	calls     []executor.Invocation
}

func (r *onboardSeqRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, inv)
	var resp map[string]any
	if i := len(r.calls) - 1; i < len(r.responses) {
		resp = r.responses[i]
	}
	return executor.Result{Response: resp}, nil
}

func TestRunConnectOnboardingExistingUnifiedApp(t *testing.T) {
	runner := &onboardSeqRunner{responses: []map[string]any{
		{"clientId": "cid-1", "clientSecret": "sec-1"}, // credentials get
	}}
	var out bytes.Buffer
	creds, err := runConnectOnboarding(runner, &cobra.Command{}, strings.NewReader("2\nUNIFIED-123\n"), &out)
	if err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	if creds.clientID != "cid-1" || creds.clientSecret != "sec-1" {
		t.Fatalf("creds = %+v, want cid-1/sec-1", creds)
	}
	if len(runner.calls) != 1 || runner.calls[0].Params["unifiedAppId"] != "UNIFIED-123" {
		t.Fatalf("expected one credentials-get with unifiedAppId=UNIFIED-123, got %+v", runner.calls)
	}
}

func TestRunConnectOnboardingExistingRawCreds(t *testing.T) {
	runner := &onboardSeqRunner{}
	var out bytes.Buffer
	// choice 2, blank unified id, then raw clientId/secret.
	creds, err := runConnectOnboarding(runner, &cobra.Command{}, strings.NewReader("2\n\nCID\nCSECRET\n"), &out)
	if err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	if creds.clientID != "CID" || creds.clientSecret != "CSECRET" {
		t.Fatalf("creds = %+v, want CID/CSECRET", creds)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("raw creds path should not call the runner, got %d calls", len(runner.calls))
	}
}

func TestRunConnectOnboardingNewApp(t *testing.T) {
	runner := &onboardSeqRunner{responses: []map[string]any{
		{"taskId": "task-1"},                           // submit
		{"clientId": "cid-2", "clientSecret": "sec-2"}, // result poll #1
	}}
	var out bytes.Buffer
	creds, err := runConnectOnboarding(runner, &cobra.Command{}, strings.NewReader("1\nMy App\nMy Bot\nHandles QA\n"), &out)
	if err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	if creds.clientID != "cid-2" || creds.clientSecret != "sec-2" {
		t.Fatalf("creds = %+v, want cid-2/sec-2", creds)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected submit+result (2 calls), got %d", len(runner.calls))
	}
	if runner.calls[0].Tool != devAppRobotSubmitTool {
		t.Errorf("call 0 tool = %q, want %q", runner.calls[0].Tool, devAppRobotSubmitTool)
	}
	if runner.calls[0].Params["name"] != "My App" || runner.calls[0].Params["robotName"] != "My Bot" {
		t.Errorf("submit params = %+v, want name=My App robotName=My Bot", runner.calls[0].Params)
	}
	if runner.calls[1].Tool != devAppRobotResultTool {
		t.Errorf("call 1 tool = %q, want %q", runner.calls[1].Tool, devAppRobotResultTool)
	}
	if runner.calls[1].Params["taskId"] != "task-1" {
		t.Errorf("result taskId = %v, want task-1", runner.calls[1].Params["taskId"])
	}
}

func TestRunConnectOnboardingInvalidChoice(t *testing.T) {
	runner := &onboardSeqRunner{}
	var out bytes.Buffer
	if _, err := runConnectOnboarding(runner, &cobra.Command{}, strings.NewReader("9\n"), &out); err == nil {
		t.Fatal("expected error for invalid choice")
	}
}

func TestRunConnectOnboardingNewAppMissingField(t *testing.T) {
	runner := &onboardSeqRunner{}
	var out bytes.Buffer
	// new app but blank robot name → validation error, no runner call (no side effect).
	if _, err := runConnectOnboarding(runner, &cobra.Command{}, strings.NewReader("1\nMy App\n\nDesc\n"), &out); err == nil {
		t.Fatal("expected validation error for blank robot name")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("should not submit with missing field, got %d calls", len(runner.calls))
	}
}
