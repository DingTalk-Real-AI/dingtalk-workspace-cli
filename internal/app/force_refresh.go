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

// ForceRefreshAccessToken preserves the original force-refresh entry point.
// New recovery paths should pass the token rejected by the server to
// ForceRefreshRejectedToken so concurrent refreshes can be deduplicated.
func ForceRefreshAccessToken(ctx context.Context, configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	data, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		return "", err
	}
	return ForceRefreshRejectedToken(ctx, configDir, data.AccessToken)
}

// ForceRefreshRejectedToken atomically refreshes rejectedAccessToken, or
// reuses the persisted token if another goroutine/process already rotated it.
func ForceRefreshRejectedToken(ctx context.Context, configDir, rejectedAccessToken string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	disc := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := authpkg.NewOAuthProvider(configDir, disc)
	configureOAuthProviderCompatibility(provider, configDir)
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
