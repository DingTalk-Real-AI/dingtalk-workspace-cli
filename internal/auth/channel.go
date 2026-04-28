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

package auth

import (
	"os"
	"strings"
)

const (
	// AgentCodeEnv is the sole per-spawn environment variable the host injects
	// to declare "this process is driven by a third-party Agent host, render
	// authorization UI yourselves". docs/pat/contract.md §7.
	AgentCodeEnv = "DINGTALK_DWS_AGENTCODE"
)

// HostOwnsPATFlow reports whether the current process is running under a
// third-party Agent host that will render the PAT authorization card
// itself. Per docs/pat/contract.md §7, the sole trigger is AgentCodeEnv
// (DINGTALK_DWS_AGENTCODE) being non-empty. The CLI deliberately does
// not consult any other signal (DINGTALK_AGENT / DWS_CHANNEL / the wire
// claw-type header) for this decision so that server-side routing tags
// and the host-owned UI contract remain independent concerns.
func HostOwnsPATFlow() bool {
	return strings.TrimSpace(os.Getenv(AgentCodeEnv)) != ""
}
