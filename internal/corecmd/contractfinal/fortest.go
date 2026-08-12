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

import "github.com/spf13/cobra"

// Cross-package test helpers for this package's command-keyed store.
// Production code must not call these; the ForTest suffix is the boundary.

// ClearRuntimeContractFinalForTest removes a registration (tests only).
func ClearRuntimeContractFinalForTest(cmd *cobra.Command) {
	if cmd != nil {
		contractFinalByCommand.Delete(cmd)
	}
}

// StoreRuntimeContractFinalRawForTest injects a raw map value (tests only).
func StoreRuntimeContractFinalRawForTest(cmd *cobra.Command, raw any) {
	if cmd != nil {
		contractFinalByCommand.Store(cmd, raw)
	}
}
