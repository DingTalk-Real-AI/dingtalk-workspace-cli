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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

func TestShouldUseDirectRuntimeAcceptsCanonicalInvocation(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	if got := shouldUseDirectRuntime(executor.Invocation{Kind: "canonical_invocation"}); !got {
		t.Fatalf("shouldUseDirectRuntime(canonical_invocation) = %t, want true", got)
	}
}

func TestDirectRuntimeProductIDsReflectDiscoveredDynamicServers(t *testing.T) {
	SetDynamicServers([]market.ServerDescriptor{
		{
			Endpoint:   "https://example.com/doc",
			HasCLIMeta: true,
			CLI: market.CLIOverlay{
				ID:      "doc",
				Command: "documents",
			},
		},
		{
			Endpoint:   "https://example.com/skip",
			HasCLIMeta: true,
			CLI: market.CLIOverlay{
				ID:   "skip-me",
				Skip: true,
			},
		},
	})
	t.Cleanup(func() { SetDynamicServers(nil) })

	ids := DirectRuntimeProductIDs()
	if !ids["doc"] {
		t.Fatalf("DirectRuntimeProductIDs()[doc] = false, want true")
	}
	if !ids["documents"] {
		t.Fatalf("DirectRuntimeProductIDs()[documents] = false, want true")
	}
	if ids["skip-me"] {
		t.Fatalf("DirectRuntimeProductIDs()[skip-me] = true, want false")
	}
}

func TestRuntimeRunnerResolvesDynamicEndpointForCanonicalInvocation(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "*")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":3,"result":{"content":{"ok":true},"isError":false}}`))
	}))
	defer srv.Close()

	SetDynamicServers([]market.ServerDescriptor{
		{
			Endpoint:   srv.URL,
			HasCLIMeta: true,
			CLI: market.CLIOverlay{
				ID:      "doc",
				Command: "documents",
			},
		},
	})
	t.Cleanup(func() { SetDynamicServers(nil) })

	runner := &runtimeRunner{
		loader:    cli.StaticLoader{Catalog: ir.Catalog{}},
		transport: transport.NewClient(nil),
	}

	result, err := runner.Run(context.Background(), executor.Invocation{
		Kind:             "canonical_invocation",
		CanonicalProduct: "doc",
		Tool:             "create_document",
		CanonicalPath:    "doc.create_document",
		Params: map[string]any{
			"title": "Quarterly",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := result.Response["endpoint"]; got != transport.RedactURL(srv.URL) {
		t.Fatalf("result.Response[endpoint] = %#v, want %q", got, transport.RedactURL(srv.URL))
	}
}
