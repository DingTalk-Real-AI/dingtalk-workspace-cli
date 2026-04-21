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

// oauth_exchange.go holds the authorization_code grant path: exchanging an
// OAuth callback code for a TokenData (both direct and MCP-proxied), and
// the shared response-parsing helpers used by the refresh path as well.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func (p *OAuthProvider) exchangeCode(ctx context.Context, code string) (*TokenData, error) {
	// Use MCP mode if clientID is from MCP server
	if IsClientIDFromMCP() {
		return p.exchangeCodeViaMCP(ctx, code)
	}
	// Direct mode with client secret
	clientID := ClientID()
	clientSecret := ClientSecret()
	body := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}
	resp, err := p.postJSON(ctx, UserAccessTokenURL, body)
	if err != nil {
		return nil, err
	}
	data, err := p.parseTokenResponse(resp)
	if err != nil {
		return nil, err
	}
	// Snapshot credentials used for this token (for refresh)
	data.ClientID = clientID
	data.Source = resolveCredentialSource()
	// Save clientSecret for future refresh (even if env changes)
	if err := SaveClientSecret(clientID, clientSecret); err != nil {
		// Log warning but don't fail login
		fmt.Fprintf(p.Output, "Warning: failed to save client secret: %v\n", err)
	}
	return data, nil
}

// ExchangeCodeForToken exchanges an authorization code for token data using
// the currently configured client credentials.  This is a convenience wrapper
// around OAuthProvider.exchangeCode for callers outside the auth package.
func ExchangeCodeForToken(ctx context.Context, configDir, code string) (*TokenData, error) {
	p := &OAuthProvider{
		configDir:  configDir,
		clientID:   ClientID(),
		Output:     io.Discard,
		httpClient: oauthHTTPClient,
	}
	return p.exchangeCode(ctx, code)
}

// exchangeCodeViaMCP exchanges auth code for token via MCP proxy.
// This is used when client secret is not available (server-side secret management).
func (p *OAuthProvider) exchangeCodeViaMCP(ctx context.Context, code string) (*TokenData, error) {
	clientID := ClientID()
	url := GetMCPBaseURL() + MCPOAuthTokenPath
	body := map[string]string{
		"clientId":  clientID,
		"authCode":  code,
		"grantType": "authorization_code",
	}
	resp, err := p.postJSON(ctx, url, body)
	if err != nil {
		return nil, err
	}
	data, err := p.parseMCPTokenResponse(resp)
	if err != nil {
		return nil, err
	}
	// Snapshot credentials used for this token (for refresh)
	data.ClientID = clientID
	data.Source = "mcp"
	// MCP mode doesn't need to save clientSecret (server-side managed)
	return data, nil
}

func (p *OAuthProvider) parseTokenResponse(body []byte) (*TokenData, error) {
	var resp struct {
		AccessToken    string `json:"accessToken"`
		RefreshToken   string `json:"refreshToken"`
		PersistentCode string `json:"persistentCode"`
		ExpiresIn      int64  `json:"expiresIn"`
		CorpID         string `json:"corpId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if resp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing accessToken")
	}

	now := time.Now()
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		// 默认 2 小时有效期（钉钉 access_token 标准有效期）
		expiresIn = config.DefaultAccessTokenExpiry
	}
	data := &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second),
		RefreshExpAt: now.Add(config.DefaultRefreshTokenLifetime),
		CorpID:       resp.CorpID,
	}
	if resp.PersistentCode != "" {
		data.PersistentCode = resp.PersistentCode
	}
	return data, nil
}

// parseMCPTokenResponse parses token response from MCP proxy.
// MCP OAuth response format: {"accessToken": "...", "refreshToken": "...", "expiresIn": 7200, "corpId": "..."}
func (p *OAuthProvider) parseMCPTokenResponse(body []byte) (*TokenData, error) {
	var resp struct {
		AccessToken    string `json:"accessToken"`
		RefreshToken   string `json:"refreshToken"`
		PersistentCode string `json:"persistentCode"`
		ExpiresIn      int64  `json:"expiresIn"`
		CorpID         string `json:"corpId"`
		// Error fields (when request fails)
		ErrorCode string `json:"errorCode,omitempty"`
		ErrorMsg  string `json:"errorMsg,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing MCP token response: %w (body: %s)", err, string(body))
	}
	// Check for error response
	if resp.ErrorCode != "" || resp.ErrorMsg != "" {
		return nil, fmt.Errorf("MCP token exchange failed: %s - %s", resp.ErrorCode, resp.ErrorMsg)
	}
	if resp.AccessToken == "" {
		return nil, fmt.Errorf("MCP token response missing accessToken (body: %s)", string(body))
	}

	now := time.Now()
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = config.DefaultAccessTokenExpiry
	}
	data := &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second),
		RefreshExpAt: now.Add(config.DefaultRefreshTokenLifetime),
		CorpID:       resp.CorpID,
	}
	if resp.PersistentCode != "" {
		data.PersistentCode = resp.PersistentCode
	}
	return data, nil
}
