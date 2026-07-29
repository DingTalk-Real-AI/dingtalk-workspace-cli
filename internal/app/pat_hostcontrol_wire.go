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
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// init wires the PAT classifier's hostControl injection hook. This
// guarantees cleanPATJSON emits data.hostControl in host-owned mode
// regardless of whether the PAT error was surfaced via the active retry
// path or the passive classifier path.
//
// Decision rule:
//   - Host-owned is triggered iff DINGTALK_DWS_AGENTCODE is non-empty.
//   - When triggered, `clawType` in the emitted hostControl block MUST be the
//     exact value the CLI actually injects on the wire. Each edition supplies
//     its existing default and an optional valid DWS_AGENT_PRODUCT overrides
//     it. Invalid input falls back here for library compatibility; root command
//     execution rejects it before network access.
//   - When DINGTALK_DWS_AGENTCODE is empty the provider returns "" so
//     HostControlBlock yields nil and no hostControl block is emitted.
func init() {
	apperrors.SetHostControlProvider(hostControlProviderFromEnv)
	apperrors.SetPATOpenBrowserProvider(func() bool {
		return pat.EffectiveOpenBrowser(defaultConfigDir())
	})
}

func hostControlProviderFromEnv() string {
	if !authpkg.HostOwnsPATFlow() {
		return ""
	}
	return effectiveClawType()
}

// effectiveClawType resolves the literal value injected into outbound
// `claw-type` headers without invoking credential hooks from PAT error
// serialization. MergeHeaders implementations that set claw-type must satisfy
// the edition contract that this value is independent of the base map.
func effectiveClawType() string {
	headers := make(map[string]string)
	if h := edition.Get(); h != nil {
		if h.MergeHeaders != nil {
			headers = h.MergeHeaders(headers)
		}
	}
	return resolveEffectiveAgentProduct(headers)
}
