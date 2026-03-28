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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
)

func TestMergePayloadsAppliesOverridesAfterJSONInputs(t *testing.T) {
	t.Parallel()

	params, err := MergePayloads(
		`{"title":"base","notify":false,"metadata":{"owner":"alice"}}`,
		`{"template_id":42}`,
		map[string]any{
			"title":  "override",
			"notify": true,
			"tags":   []any{"alpha", "beta"},
		},
	)
	if err != nil {
		t.Fatalf("MergePayloads() error = %v", err)
	}

	if params["title"] != "override" {
		t.Fatalf("MergePayloads() title = %#v, want override", params["title"])
	}
	if params["template_id"] != float64(42) {
		t.Fatalf("MergePayloads() template_id = %#v, want 42", params["template_id"])
	}
	if params["notify"] != true {
		t.Fatalf("MergePayloads() notify = %#v, want true", params["notify"])
	}
	if _, ok := params["metadata"].(map[string]any); !ok {
		t.Fatalf("MergePayloads() expected metadata object to be preserved")
	}
}

func TestMergePayloadsRejectsNonObjectJSON(t *testing.T) {
	t.Parallel()

	if _, err := MergePayloads(`["bad"]`, "", nil); err == nil {
		t.Fatalf("MergePayloads() expected validation error for non-object JSON")
	}
}

func TestNewInvocationCarriesCanonicalRuntimeRoutingData(t *testing.T) {
	t.Parallel()

	invocation := NewInvocation(
		ir.CanonicalProduct{ID: "doc"},
		ir.ToolDescriptor{
			RPCName:       "create_document",
			CanonicalPath: "doc.create_document",
		},
		map[string]any{"title": "spec"},
	)

	if invocation.Kind != "canonical_invocation" {
		t.Fatalf("Kind = %q, want canonical_invocation", invocation.Kind)
	}
	if invocation.Stage != "canonical_cli" {
		t.Fatalf("Stage = %q, want canonical_cli", invocation.Stage)
	}
	if invocation.CanonicalProduct != "doc" {
		t.Fatalf("CanonicalProduct = %q, want doc", invocation.CanonicalProduct)
	}
	if invocation.Tool != "create_document" {
		t.Fatalf("Tool = %q, want create_document", invocation.Tool)
	}
	if invocation.CanonicalPath != "doc.create_document" {
		t.Fatalf("CanonicalPath = %q, want doc.create_document", invocation.CanonicalPath)
	}
	if invocation.LegacyPath != "" {
		t.Fatalf("LegacyPath = %q, want empty for canonical invocation", invocation.LegacyPath)
	}
}

func TestEchoRunnerReturnsInvocation(t *testing.T) {
	t.Parallel()

	runner := EchoRunner{}
	result, err := runner.Run(context.Background(), NewInvocation(
		ir.CanonicalProduct{ID: "doc"},
		ir.ToolDescriptor{RPCName: "create_document", CanonicalPath: "doc.create_document"},
		map[string]any{"title": "spec"},
	))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Invocation.CanonicalPath != "doc.create_document" {
		t.Fatalf("Run() canonical path = %q, want doc.create_document", result.Invocation.CanonicalPath)
	}
}

func TestEchoRunnerDryRunReturnsToolCallRequestPreview(t *testing.T) {
	t.Parallel()

	runner := EchoRunner{}
	invocation := NewInvocation(
		ir.CanonicalProduct{ID: "doc"},
		ir.ToolDescriptor{RPCName: "create_document", CanonicalPath: "doc.create_document"},
		map[string]any{"title": "spec"},
	)
	invocation.DryRun = true

	result, err := runner.Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Response == nil {
		t.Fatal("response = nil, want dry-run preview payload")
	}
	if result.Response["dry_run"] != true {
		t.Fatalf("response.dry_run = %#v, want true", result.Response["dry_run"])
	}
	request, ok := result.Response["request"].(map[string]any)
	if !ok {
		t.Fatalf("response.request = %#v, want object", result.Response["request"])
	}
	if request["method"] != "tools/call" {
		t.Fatalf("response.request.method = %#v, want tools/call", request["method"])
	}
}
