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

// Registry for schema v3 runtimeDefault resolvers (e.g. $currentUserId,
// $unionId, $corpId). Separate from command RunE logic; the CLI core reads
// this map via edition.Hooks.RuntimeDefaults to inject values into
// envelope-declared flag defaults at invocation time. See
// _docs/discovery-schema-v3.md §2.3.

package helpers

import (
	"fmt"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var (
	runtimeDefaultsMu sync.RWMutex
	runtimeDefaults   = make(map[string]edition.RuntimeDefaultFn, 4)
)

// RegisterRuntimeDefault records a runtime default resolver. Panics on empty
// id, nil fn, or duplicate registration so conflicts surface at init.
func RegisterRuntimeDefault(id string, fn edition.RuntimeDefaultFn) {
	if id == "" {
		panic("products: RegisterRuntimeDefault called with empty id")
	}
	if fn == nil {
		panic(fmt.Sprintf("products: RegisterRuntimeDefault(%q) called with nil resolver", id))
	}
	runtimeDefaultsMu.Lock()
	defer runtimeDefaultsMu.Unlock()
	if _, exists := runtimeDefaults[id]; exists {
		panic(fmt.Sprintf("products: RegisterRuntimeDefault(%q) duplicate registration", id))
	}
	runtimeDefaults[id] = fn
}

// RuntimeDefaultsSnapshot returns a copy of the resolver map for the
// edition.Hooks.RuntimeDefaults hook. Always returns a new map so callers
// can't mutate the registry after boot.
func RuntimeDefaultsSnapshot() map[string]edition.RuntimeDefaultFn {
	runtimeDefaultsMu.RLock()
	defer runtimeDefaultsMu.RUnlock()
	out := make(map[string]edition.RuntimeDefaultFn, len(runtimeDefaults))
	for id, fn := range runtimeDefaults {
		out[id] = fn
	}
	return out
}
