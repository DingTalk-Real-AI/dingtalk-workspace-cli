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

// oauth_refresh.go holds the refresh_token grant path: exchanging a stored
// refresh token for a new TokenData (both direct and MCP-proxied), and
// persisting the refreshed result. The file-lock wrapper lockedRefresh
// lives with OAuthProvider in oauth_provider.go because it is bound to the
// provider's lifecycle.
package auth

import (
	"context"
	"fmt"
)

func (p *OAuthProvider) refreshWithRefreshToken(ctx context.Context, data *TokenData) (*TokenData, error) {
	// Use stored Source to determine refresh path (not current runtime state)
	// This ensures refresh works even if environment variables changed since login
	if data.Source == "mcp" {
		return p.refreshViaMCP(ctx, data)
	}

	// Direct mode: use stored clientId and load saved clientSecret
	clientID := data.ClientID
	if clientID == "" {
		// Fallback for legacy tokens without stored clientId
		clientID = ClientID()
	}
	clientSecret := LoadClientSecret(clientID)
	if clientSecret == "" {
		// Fallback: try current environment
		clientSecret = ClientSecret()
	}

	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("无法刷新 token: 缺少 clientId 或 clientSecret，请重新登录")
	}

	body := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": data.RefreshToken,
		"grantType":    "refresh_token",
	}
	resp, err := p.postJSON(ctx, UserAccessTokenURL, body)
	if err != nil {
		return nil, err
	}
	updated, err := p.parseTokenResponse(resp)
	if err != nil {
		return nil, err
	}
	// Preserve original credentials info
	updated.ClientID = data.ClientID
	updated.Source = data.Source
	updated.PersistentCode = data.PersistentCode
	updated.CorpID = data.CorpID
	updated.UserID = data.UserID
	updated.UserName = data.UserName
	updated.CorpName = data.CorpName

	if err := SaveTokenData(p.configDir, updated); err != nil {
		return nil, fmt.Errorf("保存刷新后的 token 失败（旧 refresh_token 已失效，请重新登录）: %w", err)
	}
	return updated, nil
}

// refreshViaMCP refreshes token via MCP proxy.
func (p *OAuthProvider) refreshViaMCP(ctx context.Context, data *TokenData) (*TokenData, error) {
	// Use stored clientId from token data
	clientID := data.ClientID
	if clientID == "" {
		// Fallback for legacy tokens
		clientID = ClientID()
	}

	if clientID == "" {
		return nil, fmt.Errorf("无法刷新 token: 缺少 clientId，请重新登录")
	}

	url := GetMCPBaseURL() + MCPRefreshTokenPath
	body := map[string]string{
		"clientId":     clientID,
		"refreshToken": data.RefreshToken,
		"grantType":    "refresh_token",
	}
	resp, err := p.postJSON(ctx, url, body)
	if err != nil {
		return nil, err
	}
	updated, err := p.parseMCPTokenResponse(resp)
	if err != nil {
		return nil, err
	}
	// Preserve original credentials info
	updated.ClientID = data.ClientID
	updated.Source = data.Source
	updated.PersistentCode = data.PersistentCode
	updated.CorpID = data.CorpID
	updated.UserID = data.UserID
	updated.UserName = data.UserName
	updated.CorpName = data.CorpName

	if err := SaveTokenData(p.configDir, updated); err != nil {
		return nil, fmt.Errorf("保存刷新后的 token 失败（旧 refresh_token 已失效，请重新登录）: %w", err)
	}
	return updated, nil
}
