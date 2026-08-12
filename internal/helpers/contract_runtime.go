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

package helpers

import (
	"sync"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// contractRuntime holds DeclareLeafMetadata execution hooks keyed by command
// pointer so Sheet outer guards (and tests) can run Validate before their own
// confirmation without relying on PreRunE (direct RunE calls skip PreRunE).
type contractRuntime struct {
	validate func(*cobra.Command, []string) error
	safety   contract.SafetySpec // zero when Confirmation is not user_required
	confirm  bool
}

var contractRuntimeByCmd sync.Map // *cobra.Command → *contractRuntime

func storeContractRuntime(cmd *cobra.Command, rt *contractRuntime) {
	if cmd == nil || rt == nil {
		return
	}
	contractRuntimeByCmd.Store(cmd, rt)
}

func loadContractRuntime(cmd *cobra.Command) (*contractRuntime, bool) {
	if cmd == nil {
		return nil, false
	}
	v, ok := contractRuntimeByCmd.Load(cmd)
	if !ok {
		return nil, false
	}
	rt, ok := v.(*contractRuntime)
	return rt, ok
}

// ContractValidate returns the DeclareLeafMetadata Validate hook when present.
// Used by outer Sheet guards so Validate and confirmation stay on the RunE
// layer for proxy/direct RunE call sites.
func ContractValidate(cmd *cobra.Command) func(*cobra.Command, []string) error {
	rt, ok := loadContractRuntime(cmd)
	if !ok || rt == nil {
		return nil
	}
	return rt.validate
}
