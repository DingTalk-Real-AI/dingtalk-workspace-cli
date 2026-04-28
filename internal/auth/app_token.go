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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	// appTokenPrefix is the keychain account prefix for app-level tokens.
	appTokenPrefix = "app-token:"

	// tokenExpiryBuffer is the buffer time before actual expiry to consider
	// the token as expired (same as user token: 5 minutes).
	tokenExpiryBuffer = 5 * time.Minute
)

// AppTokenData stores the app-level access token obtained from the unified
// POST /v1.0/oauth2/accessToken endpoint. It works for both new-style
// (api.dingtalk.com) and legacy (oapi.dingtalk.com) APIs — the auth method
// (header vs query param) is chosen by the caller based on the target host.
type AppTokenData struct {
	AccessToken string    `json:"access_token,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`

	// Associated app credentials
	ClientID  string    `json:"client_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTokenValid returns true if the access token has not expired.
func (d *AppTokenData) IsTokenValid() bool {
	if d == nil || d.AccessToken == "" {
		return false
	}
	return time.Now().Before(d.ExpiresAt.Add(-tokenExpiryBuffer))
}

// SaveAppTokenData persists AppTokenData to keychain, keyed by clientID.
func SaveAppTokenData(data *AppTokenData) error {
	if data.ClientID == "" {
		return fmt.Errorf("clientID is required for saving app token data")
	}
	data.UpdatedAt = time.Now()
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal app token data: %w", err)
	}
	defer func() {
		for i := range jsonData {
			jsonData[i] = 0
		}
	}()

	account := appTokenPrefix + data.ClientID
	if err := keychain.Set(keychain.Service, account, string(jsonData)); err != nil {
		return fmt.Errorf("save app token to keychain: %w", err)
	}
	return nil
}

// LoadAppTokenData loads AppTokenData from keychain for the given clientID.
// Returns nil, nil if no data exists.
func LoadAppTokenData(clientID string) (*AppTokenData, error) {
	if clientID == "" {
		return nil, fmt.Errorf("clientID is required for loading app token data")
	}
	account := appTokenPrefix + clientID
	jsonStr, err := keychain.Get(keychain.Service, account)
	if err != nil {
		return nil, nil // Not found is not an error
	}
	if jsonStr == "" {
		return nil, nil
	}

	var data AppTokenData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("parse app token data: %w", err)
	}
	return &data, nil
}

// DeleteAppTokenData removes AppTokenData from keychain for the given clientID.
func DeleteAppTokenData(clientID string) error {
	if clientID == "" {
		return nil
	}
	account := appTokenPrefix + clientID
	return keychain.Remove(keychain.Service, account)
}

// --- Token Fetch Function ---

// FetchAppToken obtains an app-level access token from the unified endpoint:
//
//	POST https://api.dingtalk.com/v1.0/oauth2/accessToken
//	Body: {"appKey":"X","appSecret":"X"}
//	Response: {"accessToken":"xxx","expireIn":7200}
//
// The same token works for both api.dingtalk.com and oapi.dingtalk.com.
func FetchAppToken(ctx context.Context, appKey, appSecret string) (token string, expiresIn int64, err error) {
	body := map[string]string{
		"appKey":    appKey,
		"appSecret": appSecret,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, AppAccessTokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := appTokenHTTPClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("fetching app token: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return "", 0, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("获取 app token 失败 (HTTP %d): %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", 0, fmt.Errorf("parsing app token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", 0, fmt.Errorf("app token 响应缺少 accessToken 字段")
	}
	if result.ExpireIn <= 0 {
		result.ExpireIn = config.DefaultAccessTokenExpiry
	}
	return result.AccessToken, result.ExpireIn, nil
}

// --- AppTokenProvider ---

// AppTokenProvider manages app-level token acquisition, caching and auto-refresh.
type AppTokenProvider struct {
	ConfigDir  string
	AppKey     string
	AppSecret  string
	HTTPClient *http.Client // injectable for testing; nil uses default
}

// GetToken returns a valid app-level access token.
// Tokens are cached in keychain and auto-refreshed when expired (with 5-min buffer).
func (p *AppTokenProvider) GetToken(ctx context.Context) (string, error) {
	if p.AppKey == "" || p.AppSecret == "" {
		return "", fmt.Errorf("缺少应用凭证 (appKey/appSecret)，请通过 --client-id/--client-secret 指定或先执行 dws auth login")
	}

	// Load cached token data.
	data, err := LoadAppTokenData(p.AppKey)
	if err != nil {
		data = nil // Treat load errors as cache miss
	}

	// Fast path: cached token is still valid.
	if data != nil && data.IsTokenValid() {
		return data.AccessToken, nil
	}

	// Slow path: fetch a new token.
	if data == nil {
		data = &AppTokenData{ClientID: p.AppKey}
	}

	now := time.Now()
	token, expiresIn, fetchErr := FetchAppToken(ctx, p.AppKey, p.AppSecret)
	if fetchErr != nil {
		return "", fetchErr
	}
	data.AccessToken = token
	data.ExpiresAt = now.Add(time.Duration(expiresIn) * time.Second)

	// Persist updated token data.
	if saveErr := SaveAppTokenData(data); saveErr != nil {
		// Log but don't fail — token is still usable this time.
		// Write to stderr so we don't corrupt stdout JSON output when piped
		// into jq/grep/etc.
		fmt.Fprintf(os.Stderr, "Warning: 无法缓存 app token: %v\n", saveErr)
	}

	return data.AccessToken, nil
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// appTokenHTTPClient is the default HTTP client for app token operations.
var appTokenHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}
