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
	"log/slog"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

const stdioEndpointScheme = "stdio://"

var (
	stdioMu      sync.RWMutex
	stdioClients = make(map[string]*transport.StdioClient)
)

// RegisterStdioClient stores a StdioClient keyed by its canonical product ID
// (the CLI.ID used in the server descriptor). The runner looks up this client
// when a stdio:// endpoint is resolved at execution time.
func RegisterStdioClient(productID string, client *transport.StdioClient) {
	stdioMu.Lock()
	defer stdioMu.Unlock()
	stdioClients[productID] = client
}

// LookupStdioClient returns the StdioClient registered for the given product ID.
// The productID can be either the full key (pluginName/serverKey) or just the serverKey.
// This supports backward compatibility with existing CanonicalProduct values.
func LookupStdioClient(productID string) (*transport.StdioClient, bool) {
	stdioMu.RLock()
	defer stdioMu.RUnlock()
	// Try exact match first
	if c, ok := stdioClients[productID]; ok {
		return c, true
	}
	// If not found, try matching by serverKey suffix (for backward compatibility)
	for id, c := range stdioClients {
		if idx := strings.LastIndex(id, "/"); idx >= 0 {
			if id[idx+1:] == productID {
				return c, true
			}
		}
	}
	return nil, false
}

// StdioEndpoint returns a virtual endpoint URL for a stdio-based MCP server.
// Format: stdio://{pluginName}/{serverKey}
func StdioEndpoint(pluginName, serverKey string) string {
	return stdioEndpointScheme + pluginName + "/" + serverKey
}

// IsStdioEndpoint returns true if the endpoint uses the stdio:// scheme.
func IsStdioEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, stdioEndpointScheme)
}

// StopAllStdioClients stops all registered stdio clients.
// This should be called on program exit to terminate child processes.
func StopAllStdioClients() {
	stdioMu.Lock()
	defer stdioMu.Unlock()
	for id, client := range stdioClients {
		if err := client.Stop(); err != nil {
			slog.Warn("failed to stop stdio client", "id", id, "error", err)
		}
	}
	stdioClients = make(map[string]*transport.StdioClient)
}

// StopStdioClient stops a specific stdio client by product ID.
// Returns true if the client was found and stopped, false otherwise.
func StopStdioClient(productID string) bool {
	stdioMu.Lock()
	defer stdioMu.Unlock()
	client, ok := stdioClients[productID]
	if !ok {
		return false
	}
	if err := client.Stop(); err != nil {
		slog.Warn("failed to stop stdio client", "id", productID, "error", err)
	}
	delete(stdioClients, productID)
	return true
}

// StopStdioClientsByPlugin stops all stdio clients belonging to a plugin.
// The productID format is "pluginName/serverKey". This function stops all
// clients whose productID has the given pluginName prefix.
func StopStdioClientsByPlugin(pluginName string) int {
	stdioMu.Lock()
	defer stdioMu.Unlock()
	prefix := pluginName + "/"
	count := 0
	for id, client := range stdioClients {
		if len(id) > len(prefix) && id[:len(prefix)] == prefix {
			if err := client.Stop(); err != nil {
				slog.Warn("failed to stop stdio client", "id", id, "error", err)
			}
			delete(stdioClients, id)
			count++
		}
	}
	return count
}
