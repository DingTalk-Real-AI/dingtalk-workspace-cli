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

// ForceRefreshAccessToken forces a single refresh_token exchange and returns
// the new access_token. It is intended for callers that have observed a
// server-side rejection (HTTP 401 or business code such as
// TOKEN_VERIFIED_FAILED) on what locally appeared to be a still-valid token.
//
// Steps:
//  1. MarkAccessTokenStale rewrites ExpiresAt to a past instant so
//     OAuthProvider.GetAccessToken's fast-path will miss.
//  2. NewOAuthProvider + GetAccessToken triggers lockedRefresh, which uses the
//     existing dual-layer lock (process + file) to serialize concurrent
//     refresh attempts across goroutines and processes.
//  3. ResetRuntimeTokenCache clears the per-process sync.Once cache so the
//     next resolveAuthToken call re-reads from disk.
//
// Existing OAuthProvider.GetAccessToken behaviour is unchanged; this helper
// is the only entry point that orchestrates "force refresh" semantics.
func ForceRefreshAccessToken(ctx context.Context, configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	if err := authpkg.MarkAccessTokenStale(configDir); err != nil {
		return "", fmt.Errorf("mark access token stale: %w", err)
	}
	disc := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := authpkg.NewOAuthProvider(configDir, disc)
	configureOAuthProviderCompatibility(provider, configDir)
	tok, err := provider.GetAccessToken(ctx)
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
