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

package cli

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMCPIdentityHeadersExcludeMCPOnlyMetadata(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_AGENT_VER", "1.2.3")
	t.Setenv("DWS_AGENT_EXT", `{"ua":"test-agent/1.2.3"}`)

	headers := MCPIdentityHeaders()
	if headers == nil {
		t.Fatal("MCPIdentityHeaders() returned nil")
	}
	for key := range headers {
		if strings.EqualFold(key, "x-dws-agent-ver") || strings.EqualFold(key, "x-dws-agent-ext") {
			t.Fatalf("shared identity export leaked MCP-only Header %q", key)
		}
	}
}
