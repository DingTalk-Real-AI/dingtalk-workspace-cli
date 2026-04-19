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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// resolveAccessTokenFromDir loads OAuth then legacy token from configDir, applying
// the same host compatibility hooks as MCP. It mirrors the former body of
// getCachedRuntimeToken (excluding process-level cache and timing).
func resolveAccessTokenFromDir(ctx context.Context, configDir string) (string, error) {
	disc := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := authpkg.NewOAuthProvider(configDir, disc)
	configureOAuthProviderCompatibility(provider, configDir)
	token, tokenErr := provider.GetAccessToken(ctx)
	if tokenErr == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), nil
	}
	if tokenErr != nil && errors.Is(tokenErr, authpkg.ErrTokenDecryption) {
		return "", tokenErr
	}
	manager := authpkg.NewManager(configDir, nil)
	configureLegacyAuthManagerCompatibility(manager)
	if leg, _, err := manager.GetToken(); err == nil && strings.TrimSpace(leg) != "" {
		return strings.TrimSpace(leg), nil
	}
	return "", nil
}

// ResolveAuxiliaryAccessToken resolves a bearer token for HTTP clients that should
// align with MCP tool calls. Non-empty explicitToken wins. When configDir matches
// the active edition config directory, the same process-cached path as MCP is used.
// Otherwise tokens are loaded from configDir with host compatibility hooks applied.
func ResolveAuxiliaryAccessToken(ctx context.Context, configDir, explicitToken string) (string, error) {
	if t := strings.TrimSpace(explicitToken); t != "" {
		return t, nil
	}
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	if filepath.Clean(configDir) == filepath.Clean(defaultConfigDir()) {
		if tok := resolveRuntimeAuthToken(ctx, ""); tok != "" {
			return tok, nil
		}
		return "", noCredentialsError()
	}
	tok, err := resolveAccessTokenFromDir(ctx, configDir)
	if err != nil {
		return "", err
	}
	if tok != "" {
		return tok, nil
	}
	return "", noCredentialsError()
}

func noCredentialsError() error {
	if edition.Get().IsEmbedded {
		return fmt.Errorf("认证信息已失效，请重新认证")
	}
	return fmt.Errorf("no credentials found, run: dws auth login")
}
