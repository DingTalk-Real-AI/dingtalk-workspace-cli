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

package a2a

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

// PluginAuth holds authentication credentials for a plugin-owned
// streamable-http MCP server. Each server is keyed by its canonical
// product ID so different plugins can carry independent tokens without
// colliding with each other or with the default DingTalk OAuth token.
//
// The alias points at the canonical app.PluginAuth type so that callers
// on either side of the deprecation boundary see the same struct.
type PluginAuth = app.PluginAuth

// Register stores authentication credentials for a plugin server keyed
// by its canonical product ID. The runner looks up these credentials at
// execution time to inject the correct Bearer token instead of the
// default DingTalk OAuth token.
func Register(productID string, auth *PluginAuth) {
	app.RegisterPluginAuth(productID, auth)
}

// Lookup returns the authentication credentials registered for the given
// product ID, or (nil, false) when no entry exists.
func Lookup(productID string) (*PluginAuth, bool) {
	return app.LookupPluginAuth(productID)
}
