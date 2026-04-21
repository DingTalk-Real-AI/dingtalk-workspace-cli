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
	"net/http"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// HostOwnsPATFlow reports whether the current process is running under a
// third-party Agent host (SSOT §1 + §2 / docs/pat/contract.md §7). The
// sole trigger is the DINGTALK_DWS_AGENTCODE environment variable being
// non-empty; DINGTALK_AGENT, DWS_CHANNEL and the wire claw-type header
// do NOT influence this decision.
func HostOwnsPATFlow() bool {
	return auth.HostOwnsPATFlow()
}

// CurrentChannelCode returns the raw upstream channelCode forwarded as the
// x-dws-channel header; sourced from the DWS_CHANNEL environment variable.
func CurrentChannelCode() string {
	return auth.CurrentChannelCode()
}

// ApplyChannelHeader injects the configured upstream channelCode into req
// as the x-dws-channel header. No-op when req is nil or DWS_CHANNEL is
// unset.
func ApplyChannelHeader(req *http.Request) {
	auth.ApplyChannelHeader(req)
}
