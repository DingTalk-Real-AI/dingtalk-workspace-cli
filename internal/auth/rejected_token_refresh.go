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

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// ForceRefreshRejectedToken refreshes an access token that was rejected by the
// server even though it may still look valid locally. The rejected-token check
// and refresh exchange share the same process + file lock so concurrent callers
// cannot invalidate a token that another caller has already rotated.
func (p *OAuthProvider) ForceRefreshRejectedToken(ctx context.Context, rejectedAccessToken string) (string, error) {
	if p == nil || strings.TrimSpace(p.configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	rejectedAccessToken = strings.TrimSpace(rejectedAccessToken)
	if rejectedAccessToken == "" {
		return "", fmt.Errorf("rejected access token is empty")
	}

	lock, err := AcquireDualLock(ctx, p.configDir)
	if err != nil {
		return "", fmt.Errorf("acquiring dual lock: %w", err)
	}
	defer lock.Release()

	data, err := LoadTokenData(p.configDir)
	if err != nil {
		return "", err
	}
	currentAccessToken := strings.TrimSpace(data.AccessToken)
	if currentAccessToken == "" {
		return "", fmt.Errorf("stored access token is empty")
	}
	if currentAccessToken != rejectedAccessToken {
		if p.logger != nil {
			p.logger.Debug("rejected token already rotated by another goroutine/process")
		}
		return currentAccessToken, nil
	}

	if !data.IsRefreshTokenValid() {
		return "", fmt.Errorf("refresh_token 已过期")
	}
	if err := preflightTokenRefreshPersistence(data); err != nil {
		return "", fmt.Errorf("本地登录态无法安全更新: %w", err)
	}
	refreshed, err := p.refreshWithRefreshToken(ctx, data)
	if err != nil {
		return "", err
	}
	accessToken := strings.TrimSpace(refreshed.AccessToken)
	if accessToken == "" {
		return "", fmt.Errorf("force refresh returned empty access token")
	}
	return accessToken, nil
}

// DeleteTokenDataIfAccessTokenMatches removes only the current runtime
// profile's credential and only while its persisted access token still equals
// expectedAccessToken. The compare and delete share the auth dual lock so a
// concurrently rotated token cannot be removed by stale failure cleanup.
func DeleteTokenDataIfAccessTokenMatches(ctx context.Context, configDir, expectedAccessToken string) (bool, error) {
	expectedAccessToken = strings.TrimSpace(expectedAccessToken)
	if strings.TrimSpace(configDir) == "" {
		return false, fmt.Errorf("config directory is empty")
	}
	if expectedAccessToken == "" {
		return false, fmt.Errorf("expected access token is empty")
	}
	profile := strings.TrimSpace(RuntimeProfile())

	lock, err := AcquireDualLock(ctx, configDir)
	if err != nil {
		return false, fmt.Errorf("acquiring dual lock: %w", err)
	}
	defer lock.Release()

	data, err := LoadTokenDataForProfile(configDir, profile)
	if err != nil {
		return false, err
	}
	if data == nil || strings.TrimSpace(data.AccessToken) != expectedAccessToken {
		return false, nil
	}

	hooks := edition.Get()
	if hooks.DeleteToken != nil {
		if profile != "" {
			return false, fmt.Errorf("profile selection is not supported by the current auth backend")
		}
		if err := hooks.DeleteToken(configDir); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := deleteTokenDataForProfileLocked(configDir, profile); err != nil {
		return false, err
	}
	return true, nil
}
