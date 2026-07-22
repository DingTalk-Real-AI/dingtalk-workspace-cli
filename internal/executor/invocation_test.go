package executor

import (
	"context"
	"reflect"
	"testing"
)

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
	merged, err = MergePayloads(`{"same":"json"}`, `{"same":"params"}`, nil)
	if err != nil || merged["same"] != "params" {
		t.Fatalf("MergePayloads() precedence = %#v, %v; want --params to win", merged, err)
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
