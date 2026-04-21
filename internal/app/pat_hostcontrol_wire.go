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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// init wires the PAT classifier's hostControl injection hook. This
// guarantees cleanPATJSON emits data.hostControl in host-owned mode
// regardless of whether the PAT error was surfaced via the active retry
// path or the passive classifier path (see docs/pat/contract.md §5.2).
//
// Decision rule (contract.md §7):
//   - Host-owned is triggered iff DINGTALK_DWS_AGENTCODE is non-empty.
//   - When triggered, `clawType` in the emitted hostControl block MUST
//     be the exact value the CLI actually injects on the wire into the
//     `claw-type` HTTP header. The open-source build pins that to
//     edition.DefaultOSSClawType ("openClaw") unconditionally — there
//     is no per-spawn env override.
//   - When DINGTALK_DWS_AGENTCODE is empty the provider returns "" so
//     HostControlBlock yields nil and no hostControl block is emitted.
func init() {
	apperrors.SetHostControlProvider(hostControlProviderFromEnv)
}

func hostControlProviderFromEnv() string {
	if !authpkg.HostOwnsPATFlow() {
		return ""
	}
	return effectiveClawType()
}

// effectiveClawType returns the literal value that MergeHeaders will
// inject into outbound `claw-type` headers. Going through the edition
// hook (instead of a hard-coded constant) keeps this site correct for
// downstream editions that override MergeHeaders.
func effectiveClawType() string {
	if h := edition.Get(); h != nil && h.MergeHeaders != nil {
		if v, ok := h.MergeHeaders(map[string]string{})["claw-type"]; ok && v != "" {
			return v
		}
	}
	return edition.DefaultOSSClawType
}
