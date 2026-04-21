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

package a2a

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

// IdentityHeaders returns the same HTTP header map that the CLI injects
// into MCP tool calls (agent identity, env trace headers, edition
// MergeHeaders). Transports that bypass the MCP runner (for example the
// A2A gateway client) MUST forward these headers so requests are
// attributable to the right host / agent.
//
// The returned map is safe for the caller to mutate; a fresh map is
// produced on each call.
func IdentityHeaders() map[string]string {
	return app.MCPIdentityHeaders()
}
