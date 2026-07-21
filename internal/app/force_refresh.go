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
	"fmt"
	"io"
	"log/slog"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

type accessTokenGetter interface {
	GetAccessToken(context.Context) (string, error)
}

type rejectedAccessTokenRefresher interface {
	ForceRefreshRejectedToken(context.Context, string) (string, error)
}

var (
	loadRefreshTokenData = authpkg.LoadTokenData
	newRefreshProvider   = func(configDir string) rejectedAccessTokenRefresher {
		disc := slog.New(slog.NewTextHandler(io.Discard, nil))
		provider := authpkg.NewOAuthProvider(configDir, disc)
		configureOAuthProviderCompatibility(provider, configDir)
		return provider
	}
)

// ForceRefreshAccessToken forces a single refresh_token exchange and returns
// the new access_token. It is intended for callers that have observed a
// server-side rejection (HTTP 401 or business code such as
// TOKEN_VERIFIED_FAILED) on what locally appeared to be a still-valid token.
//
// It snapshots the current access token, then delegates to the OAuth
// provider's dual-locked compare-and-refresh operation. If another caller has
// already rotated the token, that newer token is reused without another
// refresh request.
func ForceRefreshAccessToken(ctx context.Context, configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	data, err := loadRefreshTokenData(configDir)
	if err != nil {
		return "", err
	}
	if data == nil || strings.TrimSpace(data.AccessToken) == "" {
		return "", fmt.Errorf("stored access token is empty")
	}
	return forceRefreshRejectedAccessToken(ctx, configDir, data.AccessToken)
}

func forceRefreshRejectedAccessToken(ctx context.Context, configDir, rejectedAccessToken string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	if strings.TrimSpace(rejectedAccessToken) == "" {
		return "", fmt.Errorf("rejected access token is empty")
	}
	provider := newRefreshProvider(configDir)
	tok, err := provider.ForceRefreshRejectedToken(ctx, rejectedAccessToken)
	if err != nil {
		return "", err
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", fmt.Errorf("force refresh returned empty access token")
	}
	ResetRuntimeTokenCache()
	return tok, nil
}
