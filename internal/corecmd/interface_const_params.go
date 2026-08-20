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

package corecmd

import (
	"sync"

	"github.com/spf13/cobra"
)

var interfaceBoolConstParamsRegistry sync.Map

// attachInterfaceBoolConstParams records framework-owned ConstParams evidence.
// Only a non-empty, entirely boolean declaration is representable in v1;
// empty or mixed declarations remove any evidence for the command.
func attachInterfaceBoolConstParams(cmd *cobra.Command, params map[string]any) {
	if cmd == nil {
		return
	}
	if len(params) == 0 {
		interfaceBoolConstParamsRegistry.Delete(cmd)
		return
	}
	bools := make(map[string]bool, len(params))
	for key, value := range params {
		boolValue, ok := value.(bool)
		if !ok {
			interfaceBoolConstParamsRegistry.Delete(cmd)
			return
		}
		bools[key] = boolValue
	}
	interfaceBoolConstParamsRegistry.Store(cmd, bools)
}

// InterfaceBoolConstParams returns a clone of framework-owned boolean
// ConstParams evidence. Callers cannot mutate the private registry.
func InterfaceBoolConstParams(cmd *cobra.Command) map[string]bool {
	if cmd == nil {
		return nil
	}
	stored, ok := interfaceBoolConstParamsRegistry.Load(cmd)
	if !ok {
		return nil
	}
	params := stored.(map[string]bool)
	clone := make(map[string]bool, len(params))
	for key, value := range params {
		clone[key] = value
	}
	return clone
}
