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

import "sync"

// PluginAuth marks ownership of a plugin-owned streamable-http MCP server and
// holds its optional authentication credentials. Every accepted HTTP plugin,
// including an anonymous one, has a non-nil record keyed by canonical product
// ID (CLI.ID) so execution never falls back to built-in DingTalk OAuth.
type PluginAuth struct {
	// Token is the Bearer token extracted from the plugin's
	// "Authorization" header (e.g. a third-party API key).
	Token string

	// ExtraHeaders contains any additional custom HTTP headers
	// declared by the plugin (excluding Authorization).
	ExtraHeaders map[string]string

	// TrustedDomains lists the hostnames that the token is allowed
	// to be sent to. Typically derived from the server endpoint.
	TrustedDomains []string
}

var (
	pluginAuthMu       sync.RWMutex
	pluginAuthRegistry = make(map[string]*PluginAuth)
)

// RegisterPluginAuth stores ownership and optional authentication credentials
// for a plugin server keyed by its canonical product ID.
func RegisterPluginAuth(productID string, auth *PluginAuth) {
	pluginAuthMu.Lock()
	defer pluginAuthMu.Unlock()
	pluginAuthRegistry[productID] = auth
}

// ClearPluginAuth removes the ownership and credential record for a plugin
// product.
func ClearPluginAuth(productID string) {
	pluginAuthMu.Lock()
	defer pluginAuthMu.Unlock()
	delete(pluginAuthRegistry, productID)
}

// LookupPluginAuth returns plugin ownership and optional authentication
// credentials for the product ID. The bool denotes ownership, not whether a
// Bearer token is present.
func LookupPluginAuth(productID string) (*PluginAuth, bool) {
	pluginAuthMu.RLock()
	defer pluginAuthMu.RUnlock()
	auth, ok := pluginAuthRegistry[productID]
	return auth, ok
}
