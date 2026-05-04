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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// supplementOnlyCatalogLoader mimics the post-fix EnvironmentLoader: the
// catalog has the product entry (materialised from SupplementServers) but
// no tool list — the overlay owns the tool tree locally.
type supplementOnlyCatalogLoader struct{}

func (supplementOnlyCatalogLoader) Load(_ context.Context) (ir.Catalog, error) {
	return ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:        "conference",
				ServerKey: "conference",
				Endpoint:  "stdio://conference-catalog",
				Tools:     nil,
			},
		},
	}, nil
}

func resetDynamicServers(t *testing.T) {
	t.Helper()
	orig := snapshotDynamicServers()
	t.Cleanup(func() { restoreDynamicServers(orig) })
}

type dynamicServerSnapshot struct {
	endpoints     map[string]string
	products      map[string]bool
	aliases       map[string]string
	toolEndpoints map[string]string
}

func snapshotDynamicServers() dynamicServerSnapshot {
	dynamicMu.RLock()
	defer dynamicMu.RUnlock()
	return dynamicServerSnapshot{
		endpoints:     cloneStringMap(dynamicEndpoints),
		products:      cloneBoolMap(dynamicProducts),
		aliases:       cloneStringMap(dynamicAliases),
		toolEndpoints: cloneStringMap(dynamicToolEndpoints),
	}
}

func restoreDynamicServers(s dynamicServerSnapshot) {
	dynamicMu.Lock()
	defer dynamicMu.Unlock()
	dynamicEndpoints = s.endpoints
	dynamicProducts = s.products
	dynamicAliases = s.aliases
	dynamicToolEndpoints = s.toolEndpoints
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBoolMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// TestRuntimeRunner_ToolMiss_FallsBackToDirectRuntime pins the runner's
// bridge between the catalog path (where a product entry can come from
// SupplementServers with no tool list) and the direct-runtime path (which
// carries the authoritative per-tool endpoint map). When the catalog knows
// the product but not the tool, the runner should not fail-fast with
// endpoint_not_resolved — it should consult dynamicEndpoints one more time
// and proceed if an endpoint is registered.
//
// This is the narrow recovery path that keeps hardcoded overlay commands
// working under a gray-released envelope: the supplement-materialised
// catalog entry has endpoint+no tools, and SetDynamicServers holds the
// operational endpoint indexed by product / command.
func TestRuntimeRunner_ToolMiss_FallsBackToDirectRuntime(t *testing.T) {
	resetDynamicServers(t)
	SetDynamicServers([]market.ServerDescriptor{
		{
			Key:         "conference",
			DisplayName: "会议",
			Endpoint:    "stdio://conference-fake",
			CLI: market.CLIOverlay{
				ID:      "conference",
				Command: "conference",
			},
			Source: "edition_supplement",
		},
	})

	runner := &runtimeRunner{
		loader:    supplementOnlyCatalogLoader{},
		transport: transport.NewClient(nil),
		fallback:  executor.EchoRunner{},
	}

	// Kind = api_invocation forces the code to skip the Run() opening
	// direct-runtime attempt and go through the catalog path instead, so
	// the tool-miss recovery branch we're testing actually runs.
	inv := executor.Invocation{
		Kind:             "api_invocation",
		CanonicalProduct: "conference",
		Tool:             "create_meeting_reservation",
		CanonicalPath:    "conference.create_meeting_reservation",
		DryRun:           true,
		Params:           map[string]any{},
	}

	result, err := runner.Run(context.Background(), inv)
	if err != nil {
		t.Fatalf("runner.Run returned error, want tool-miss fallback success: %v", err)
	}
	if result.Response == nil {
		t.Fatalf("expected non-nil Response on dry-run")
	}
	if got, _ := result.Response["dry_run"].(bool); !got {
		t.Fatalf("expected dry_run=true in Response, got %v", result.Response)
	}
	if got, _ := result.Response["transport"].(string); got != "stdio" {
		t.Fatalf("expected transport=stdio in Response (proof we hit stdio://conference-fake), got %v", result.Response)
	}
}

// TestRuntimeRunner_ToolMiss_NoDynamicEntry_StillFailsClosed is the inverse
// guard: when both the catalog tool list and dynamicEndpoints have no
// record for the requested tool, the runner must still surface
// endpoint_not_resolved instead of silently producing empty output.
func TestRuntimeRunner_ToolMiss_NoDynamicEntry_StillFailsClosed(t *testing.T) {
	resetDynamicServers(t)
	SetDynamicServers([]market.ServerDescriptor{}) // intentionally empty

	runner := &runtimeRunner{
		loader:    supplementOnlyCatalogLoader{},
		transport: transport.NewClient(nil),
		fallback:  executor.EchoRunner{},
	}

	inv := executor.Invocation{
		Kind:             "api_invocation",
		CanonicalProduct: "conference",
		Tool:             "nonexistent_tool",
		CanonicalPath:    "conference.nonexistent_tool",
		Params:           map[string]any{},
	}

	_, err := runner.Run(context.Background(), inv)
	if err == nil {
		t.Fatalf("expected endpoint_not_resolved error, got nil")
	}
	if !strings.Contains(err.Error(), "endpoint not resolved") {
		t.Fatalf("expected endpoint_not_resolved error, got %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent_tool") {
		t.Fatalf("error should name the missing tool; got %v", err)
	}
}
