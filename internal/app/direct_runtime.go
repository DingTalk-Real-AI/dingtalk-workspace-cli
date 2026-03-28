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
	"os"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

var (
	dynamicMu        sync.RWMutex
	dynamicEndpoints map[string]string
	dynamicProducts  map[string]bool
)

// SetDynamicServers injects server data discovered from servers.json.
// All product endpoints are resolved dynamically from this data.
func SetDynamicServers(servers []market.ServerDescriptor) {
	dynamicMu.Lock()
	defer dynamicMu.Unlock()

	endpoints := make(map[string]string)
	products := make(map[string]bool)
	for _, server := range servers {
		if !server.HasCLIMeta || server.CLI.Skip {
			continue
		}
		id := strings.TrimSpace(server.CLI.ID)
		endpoint := strings.TrimSpace(server.Endpoint)
		if id != "" && endpoint != "" {
			endpoints[id] = endpoint
			products[id] = true
		}
		cmd := strings.TrimSpace(server.CLI.Command)
		if cmd != "" && cmd != id && endpoint != "" {
			endpoints[cmd] = endpoint
			products[cmd] = true
		}
		for _, alias := range server.CLI.Aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" && endpoint != "" {
				endpoints[alias] = endpoint
				products[alias] = true
			}
		}
	}
	dynamicEndpoints = endpoints
	dynamicProducts = products
}

func shouldUseDirectRuntime(invocation executor.Invocation) bool {
	if strings.TrimSpace(os.Getenv(cli.CatalogFixtureEnv)) != "" {
		return false
	}
	switch invocation.Kind {
	case "canonical_invocation", "helper_invocation":
		return true
	default:
		return false
	}
}

func directRuntimeEndpoint(productID string) (string, bool) {
	candidate := strings.TrimSpace(productID)
	if candidate == "" {
		return "", false
	}
	dynamicMu.RLock()
	de := dynamicEndpoints
	dynamicMu.RUnlock()
	if override, ok := productEndpointOverride(candidate); ok {
		return override, true
	}
	if de != nil {
		if endpoint, ok := de[candidate]; ok {
			return endpoint, true
		}
	}
	return "", false
}

// DirectRuntimeProductIDs returns the set of product IDs that have direct
// runtime endpoints configured, sourced from dynamic server discovery.
func DirectRuntimeProductIDs() map[string]bool {
	dynamicMu.RLock()
	dp := dynamicProducts
	dynamicMu.RUnlock()
	ids := make(map[string]bool, len(dp))
	for key := range dp {
		ids[key] = true
	}
	return ids
}
