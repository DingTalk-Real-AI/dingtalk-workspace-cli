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
	"net/http"
	"net/url"
	"time"
)

// TokenData holds the OAuth token set persisted to disk.
type TokenData struct {
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token"`
	PersistentCode string    `json:"persistent_code"`
	ExpiresAt      time.Time `json:"expires_at"`
	RefreshExpAt   time.Time `json:"refresh_expires_at"`
	CorpID         string    `json:"corp_id"`
	UserID         string    `json:"user_id,omitempty"`
	UserName       string    `json:"user_name,omitempty"`
	CorpName       string    `json:"corp_name,omitempty"`
	ClientID       string    `json:"client_id,omitempty"` // Associated app client ID for refresh
	UpdatedAt      string    `json:"updated_at,omitempty"`
	Source         string    `json:"source,omitempty"`
}

// IsAccessTokenValid returns true if the access token has not expired.
func (t *TokenData) IsAccessTokenValid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	// Give 5-minute buffer before actual expiry.
	return time.Now().Before(t.ExpiresAt.Add(-5 * time.Minute))
}

// IsRefreshTokenValid returns true if the refresh token has not expired.
func (t *TokenData) IsRefreshTokenValid() bool {
	if t == nil || t.RefreshToken == "" {
		return false
	}
	return time.Now().Before(t.RefreshExpAt)
}

// HasPersistentCode returns true if a persistent code is available.
func (t *TokenData) HasPersistentCode() bool {
	return t != nil && t.PersistentCode != ""
}

// SaveTokenData saves TokenData to the platform keychain.
// Uses the new keychain-based storage with random master key for better security.
func SaveTokenData(configDir string, data *TokenData) error {
	return SaveTokenDataKeychain(data)
}

// LoadTokenData reads TokenData from the platform keychain.
// On first call, it attempts to migrate legacy .data file if present.
func LoadTokenData(configDir string) (*TokenData, error) {
	// Try loading from new keychain first
	if TokenDataExistsKeychain() {
		return LoadTokenDataKeychain()
	}

	// Fallback: try legacy .data file and migrate
	data, err := LoadSecureTokenData(configDir)
	if err != nil {
		return nil, err
	}

	// Migrate to keychain for future use
	if err := SaveTokenDataKeychain(data); err == nil {
		// Successfully migrated, delete legacy file
		_ = DeleteSecureData(configDir)
	}

	return data, nil
}

// DeleteTokenData removes token data from both keychain and legacy storage.
func DeleteTokenData(configDir string) error {
	// Delete from keychain
	keychainErr := DeleteTokenDataKeychain()

	// Also clean up any legacy .data file
	legacyErr := DeleteSecureData(configDir)

	// Return keychain error if any, otherwise legacy error
	if keychainErr != nil {
		return keychainErr
	}
	return legacyErr
}

// RevokeTokenRemote calls the appropriate logout/revoke endpoint to invalidate the access token.
// Uses MCP revoke endpoint when clientID is from MCP, otherwise uses DingTalk logout.
// This should be called before deleting local token data.
// The function is best-effort: errors are returned but callers may choose to ignore them.
func RevokeTokenRemote(ctx context.Context) error {
	// Use MCP revoke endpoint when clientID is from MCP
	if IsClientIDFromMCP() {
		return revokeTokenViaMCP(ctx)
	}
	// Direct mode: use DingTalk logout endpoint
	logoutURL, err := url.Parse(LogoutURL)
	if err != nil {
		return fmt.Errorf("parsing logout URL: %w", err)
	}

	q := logoutURL.Query()
	q.Set("client_id", ClientID())
	q.Set("continue", LogoutContinueURL)
	logoutURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logoutURL.String(), nil)
	if err != nil {
		return fmt.Errorf("creating logout request: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		// Do not follow redirects — we just need to hit the logout endpoint.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling logout endpoint: %w", err)
	}
	defer resp.Body.Close()

	// Accept 200 OK or 302 redirect as success.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return fmt.Errorf("logout endpoint returned status %d", resp.StatusCode)
	}

	return nil
}

// revokeTokenViaMCP revokes token via MCP endpoint.
func revokeTokenViaMCP(ctx context.Context) error {
	revokeURL := GetRevokeTokenURL()
	if revokeURL == "" {
		return nil // No revoke endpoint available
	}

	// Load current token to get accessToken
	tokenData, err := LoadTokenData(getDefaultConfigDir())
	if err != nil || tokenData == nil {
		return nil // No token to revoke
	}

	body := map[string]string{
		"clientId":    ClientID(),
		"accessToken": tokenData.AccessToken,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling revoke request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling revoke endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revoke endpoint returned status %d", resp.StatusCode)
	}

	return nil
}
