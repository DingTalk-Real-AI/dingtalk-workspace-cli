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
	"net/http"
	"os"
	"strings"
)

const (
	// DWSChannelEnv carries the upstream channelCode forwarded as x-dws-channel.
	DWSChannelEnv = "DWS_CHANNEL"
	// AgentCodeEnv is the sole per-spawn environment variable the host injects
	// to declare "this process is driven by a third-party Agent host, render
	// authorization UI yourselves". SSOT §1 + §2 / docs/pat/contract.md §7.
	AgentCodeEnv = "DINGTALK_DWS_AGENTCODE"
)

// CurrentChannelCode returns the raw upstream channel code as configured locally.
//
// Deprecated: use internal/a2a.CurrentChannelCode.
func CurrentChannelCode() string {
	return os.Getenv(DWSChannelEnv)
}

// HostOwnsPATFlow reports whether the current process is running under a
// third-party Agent host that will render the PAT authorization card
// itself. Per SSOT §1 + §2, the sole trigger is AgentCodeEnv
// (DINGTALK_DWS_AGENTCODE) being non-empty. The CLI deliberately does
// not consult any other signal (DINGTALK_AGENT / DWS_CHANNEL / the wire
// claw-type header) for this decision so that server-side routing tags
// and the host-owned UI contract remain independent concerns.
func HostOwnsPATFlow() bool {
	return strings.TrimSpace(os.Getenv(AgentCodeEnv)) != ""
}

// ApplyChannelHeader injects the configured channel code into a request.
//
// Deprecated: use internal/a2a.ApplyChannelHeader.
func ApplyChannelHeader(req *http.Request) {
	if req == nil {
		return
	}
	if ch := CurrentChannelCode(); ch != "" {
		req.Header.Set("x-dws-channel", ch)
	}
}
