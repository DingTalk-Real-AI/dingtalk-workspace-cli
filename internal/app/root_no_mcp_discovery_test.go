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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestRootConstructionRegistersOverlayWithoutCallingMCPEndpoint(t *testing.T) {
	withCleanDynamicRegistry(t)
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CacheDirEnv, t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")

	var registryCalls atomic.Int32
	var mcpCalls atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cli/discovery/apis/cedar" {
			registryCalls.Add(1)
			entry := discoveryServerEntry("startup-probe", "Startup discovery probe", nil, map[string]any{
				"ping": map[string]any{
					"cliName":     "ping",
					"description": "Ping without runtime discovery",
					"flags":       map[string]any{},
				},
			})
			entry["server"].(map[string]any)["remotes"] = []any{map[string]any{
				"type": "streamable-http",
				"url":  srv.URL + "/server/startup-probe",
			}}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"count": 1, "nextCursor": ""},
				"servers":  []any{entry},
			})
			return
		}
		if r.URL.Path == "/server/startup-probe" {
			mcpCalls.Add(1)
			http.Error(w, "MCP endpoint must not be called during root construction", http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	if got := lookupCommand(root, "startup-probe ping"); got == nil {
		t.Fatal("overlay command was not registered from versioned CLI metadata")
	}
	if got := registryCalls.Load(); got != 1 {
		t.Fatalf("registry calls = %d, want 1", got)
	}
	if got := mcpCalls.Load(); got != 0 {
		t.Fatalf("MCP endpoint calls during root construction = %d, want 0", got)
	}
}

func TestRootDoesNotRegisterCanonicalMCPCommand(t *testing.T) {
	withCleanDynamicRegistry(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DWS_CONFIG_DIR", home)
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	previous := edition.Get()
	edition.Override(&edition.Hooks{
		Name: "no-discovery-test",
		StaticServers: func() []edition.ServerInfo {
			return nil
		},
	})
	t.Cleanup(func() { edition.Override(previous) })

	root := NewRootCommand()
	for _, command := range root.Commands() {
		if command.Name() == "mcp" {
			t.Fatal("root must not register the deprecated hidden 'dws mcp' discovery tree")
		}
	}
}
