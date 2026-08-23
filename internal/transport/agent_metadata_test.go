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

package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCrossPlatformCoverageAgentMetadataReachesEveryMCPMethod(t *testing.T) {
	t.Parallel()

	const (
		agentVersion = "1.2.3-test+7"
		agentExt     = `{"umt":"test-umt","nested":{"enabled":true}}`
		cliVersion   = "9.8.7-cli"
		userAgent    = "dws-user-agent-test/1.0"
	)

	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request requestEnvelope
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seen[request.Method]++
		if got := r.Header.Get(HeaderAgentVersion); got != agentVersion {
			t.Errorf("%s %s = %q, want %q", request.Method, HeaderAgentVersion, got, agentVersion)
		}
		if got := r.Header.Get(HeaderAgentExt); got != agentExt {
			t.Errorf("%s %s = %q, want %q", request.Method, HeaderAgentExt, got, agentExt)
		}
		if got := r.Header.Get(HeaderVersion); got != cliVersion {
			t.Errorf("%s %s = %q, want %q", request.Method, HeaderVersion, got, cliVersion)
		}
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("%s User-Agent = %q, want preset value %q", request.Method, got, userAgent)
		}

		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"%s","capabilities":{}}}`, supportedProtocolVersions[0])
		case "tools/list":
			_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`)
		case "tools/call":
			_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":3,"result":{"content":{"success":true}}}`)
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.ExtraHeaders = map[string]string{
		HeaderAgentVersion: agentVersion,
		HeaderAgentExt:     agentExt,
		HeaderVersion:      cliVersion,
		"User-Agent":       userAgent,
	}
	ctx := context.Background()
	if _, err := client.Initialize(ctx, server.URL); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := client.ListTools(ctx, server.URL); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if _, err := client.CallTool(ctx, server.URL, "test_tool", map[string]any{"value": "safe"}); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}

	for _, method := range []string{"initialize", "tools/list", "tools/call"} {
		if got := seen[method]; got != 1 {
			t.Errorf("%s request count = %d, want 1", method, got)
		}
	}
}
