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

// Package runtimetoken resolves API bearer tokens for features that bypass
// the MCP runner (e.g. A2A gateway) but should behave like tool calls.
package runtimetoken

import (
	"context"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

// ResolveAccessToken returns a non-empty bearer token using the same sources
// and caching rules as MCP when configDir matches the active edition directory;
// see app.ResolveAuxiliaryAccessToken.
func ResolveAccessToken(ctx context.Context, configDir, explicitToken string) (string, error) {
	return app.ResolveAuxiliaryAccessToken(ctx, configDir, explicitToken)
}
