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
func LookupStdioClient(productID string) (*transport.StdioClient, bool) {
	stdioMu.RLock()
	defer stdioMu.RUnlock()
	c, ok := stdioClients[productID]
	return c, ok
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
