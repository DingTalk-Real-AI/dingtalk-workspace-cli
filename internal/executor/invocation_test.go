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

package executor

import (
	"context"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageInvocationSkipExecutionDuringDryRun(t *testing.T) {
	tests := []struct {
		name       string
		invocation Invocation
		want       bool
	}{
		{name: "normal invocation", invocation: Invocation{}, want: false},
		{name: "ordinary dry run", invocation: Invocation{DryRun: true}, want: true},
		{
			name: "explicit read-only dry run",
			invocation: Invocation{
				DryRun:                    true,
				AllowReadOnlyDuringDryRun: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.invocation.SkipExecutionDuringDryRun(); got != tt.want {
				t.Fatalf("SkipExecutionDuringDryRun() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageEchoRunnerHonorsReadOnlyDryRunCapability(t *testing.T) {
	params := map[string]any{"dryRun": true}
	ordinary := Invocation{DryRun: true, Tool: "pat.batch_plan", Params: params}
	result, err := (EchoRunner{}).Run(context.Background(), ordinary)
	if err != nil {
		t.Fatalf("EchoRunner.Run(ordinary dry-run) error = %v", err)
	}
	if got := result.Response["dry_run"]; got != true {
		t.Fatalf("ordinary dry-run response = %#v, want dry_run=true", result.Response)
	}

	readOnly := ordinary
	readOnly.AllowReadOnlyDuringDryRun = true
	result, err = (EchoRunner{}).Run(context.Background(), readOnly)
	if err != nil {
		t.Fatalf("EchoRunner.Run(read-only dry-run) error = %v", err)
	}
	if result.Response != nil {
		t.Fatalf("read-only dry-run response = %#v, want execution to remain available", result.Response)
	}
}

func TestCrossPlatformCoverageInvocationBuildersAndEchoRunner(t *testing.T) {
	params := map[string]any{"name": "value"}
	compat := NewCompatibilityInvocation("old path", "doc", "read", params)
	if compat.Kind != "compat_invocation" || compat.CanonicalPath != "doc.read" || !reflect.DeepEqual(compat.Params, params) {
		t.Fatalf("unexpected compatibility invocation: %#v", compat)
	}
	if got := NewCompatibilityInvocation("old", "doc", "read", nil).Params; got == nil {
		t.Fatal("nil compatibility params were not normalized")
	}
	help := NewHelperInvocation("old path", "chat", "send", params)
	if help.Kind != "helper_invocation" || help.Stage != "helper_override" {
		t.Fatalf("unexpected helper invocation: %#v", help)
	}
	if got := NewHelperInvocation("old", "chat", "send", nil).Params; got == nil {
		t.Fatal("nil helper params were not normalized")
	}

	workflow := NewWorkflowInvocation("old", "publish", []Invocation{compat, help})
	steps, ok := workflow.Params["steps"].([]any)
	if !ok || len(steps) != 2 || workflow.CanonicalPath != "workflow.publish" {
		t.Fatalf("unexpected workflow invocation: %#v", workflow)
	}

	runner := EchoRunner{}
	result, err := runner.Run(context.Background(), compat)
	if err != nil || result.Invocation.Tool != "read" || result.Response != nil {
		t.Fatalf("EchoRunner normal result = %#v, %v", result, err)
	}
	compat.DryRun = true
	result, err = runner.Run(context.Background(), compat)
	if err != nil || result.Response["dry_run"] != true {
		t.Fatalf("EchoRunner dry-run result = %#v, %v", result, err)
	}
}

func TestCrossPlatformCoverageMergePayloadsAndToolCallRequest(t *testing.T) {
	merged, err := MergePayloads(`{"a":1,"same":"json"}`, `{"b":2,"same":"params"}`, map[string]any{"c": 3, "same": "override"})
	if err != nil {
		t.Fatalf("MergePayloads() error = %v", err)
	}
	if merged["same"] != "override" || merged["a"] != float64(1) || merged["b"] != float64(2) || merged["c"] != 3 {
		t.Fatalf("MergePayloads() = %#v", merged)
	}
	if empty, err := MergePayloads(" ", "", nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty MergePayloads() = %#v, %v", empty, err)
	}
	for _, input := range []string{`{`, `[]`, `null`} {
		if _, err := MergePayloads(input, "", nil); err == nil {
			t.Errorf("MergePayloads(%q) error = nil", input)
		}
	}

	request := ToolCallRequest("read", map[string]any{"id": "1"})
	if request["method"] != "tools/call" || request["jsonrpc"] != "2.0" {
		t.Fatalf("ToolCallRequest() = %#v", request)
	}
	request = ToolCallRequest("read", nil)
	params := request["params"].(map[string]any)
	if params["arguments"] == nil {
		t.Fatal("ToolCallRequest() left nil arguments")
	}
}
