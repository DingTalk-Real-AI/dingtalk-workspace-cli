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
	"context"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

// ResolveAccessToken returns a non-empty bearer token for HTTP clients that
// should behave like an MCP tool call. A non-empty explicitToken wins. When
// configDir matches the active edition config directory, the same process
// cache path as MCP is used; otherwise tokens are loaded from configDir
// with host compatibility hooks applied.
//
// Hosts SHOULD call this helper instead of reaching into pkg/runtimetoken
// or internal/app directly.
func ResolveAccessToken(ctx context.Context, configDir, explicitToken string) (string, error) {
	return app.ResolveAuxiliaryAccessToken(ctx, configDir, explicitToken)
}
