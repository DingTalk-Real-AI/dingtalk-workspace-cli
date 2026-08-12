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

package contractfinal

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

// HasDeclaredOrAnnotatedConfirmation reports whether confirmation semantics are
// covered by typed Contract SafetySpec. The risk/gate annotation branches are
// residual bridges: their production writers are retired, so they only fire
// for hand-built annotations (tests and overlay-bridge probes).
func HasDeclaredOrAnnotatedConfirmation(cmd *cobra.Command) bool {
	if final, ok := RuntimeContractFinal(cmd); ok && final.Safety != nil &&
		strings.TrimSpace(final.Safety.Confirmation) != "" {
		return true
	}
	if _, ok := runtimeannotate.RuntimeContractRisk(cmd); ok {
		return true
	}
	_, ok := runtimeannotate.RuntimeContractGate(cmd)
	return ok
}
