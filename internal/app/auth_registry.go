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

// PluginAuth holds authentication credentials for a plugin-owned
// streamable-http MCP server. Each server is keyed by its canonical
// product ID (CLI.ID) so that different servers can use independent
// tokens without interfering with each other or with the default
// DingTalk OAuth token.
//
// Deprecated: use internal/a2a.PluginAuth (alias of this type).
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

// RegisterPluginAuth stores authentication credentials for a plugin
// server keyed by its canonical product ID. The runner looks up these
// credentials at execution time to inject the correct Bearer token
// instead of the default DingTalk OAuth token.
//
// Deprecated: use internal/a2a.Register.
func RegisterPluginAuth(productID string, auth *PluginAuth) {
	pluginAuthMu.Lock()
	defer pluginAuthMu.Unlock()
	pluginAuthRegistry[productID] = auth
}

// LookupPluginAuth returns the authentication credentials registered
// for the given product ID, or nil if none exists.
//
// Deprecated: use internal/a2a.Lookup.
func LookupPluginAuth(productID string) (*PluginAuth, bool) {
	pluginAuthMu.RLock()
	defer pluginAuthMu.RUnlock()
	auth, ok := pluginAuthRegistry[productID]
	return auth, ok
}
