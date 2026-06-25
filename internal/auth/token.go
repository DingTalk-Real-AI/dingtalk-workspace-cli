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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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

const tokenJSONFile = "token.json"

// TokenMarker is a lightweight file the host application reads to detect
// whether the CLI has a valid token without accessing the keychain.
type TokenMarker struct {
	UpdatedAt string `json:"updated_at"`
}

// WriteTokenMarker writes a token.json marker containing only an updated_at
// timestamp. The host application uses this file's presence and mtime to
// decide whether it needs to trigger a new auth exchange.
func WriteTokenMarker(configDir string) error {
	marker := TokenMarker{UpdatedAt: time.Now().Format(time.RFC3339)}
	data, _ := json.MarshalIndent(marker, "", "  ")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(configDir, tokenJSONFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(configDir, tokenJSONFile))
}

// DeleteTokenMarker removes the token.json marker file.
func DeleteTokenMarker(configDir string) error {
	if err := os.Remove(filepath.Join(configDir, tokenJSONFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SaveTokenData persists TokenData. When an edition hook (SaveToken) is
// registered, it delegates entirely to the hook; otherwise it falls back
// to the default keychain-based storage.
func SaveTokenData(configDir string, data *TokenData) error {
	if h := edition.Get(); h.SaveToken != nil {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling token data for hook: %w", err)
		}
		return h.SaveToken(configDir, jsonData)
	}
	if data != nil && strings.TrimSpace(data.CorpID) != "" {
		if err := SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
			return err
		}
		makeCurrent := strings.TrimSpace(RuntimeProfile()) == ""
		if err := UpsertProfileFromTokenWithCurrent(configDir, data, makeCurrent); err != nil {
			return err
		}
		if makeCurrent {
			if err := SaveTokenDataKeychain(data); err != nil {
				return err
			}
		} else if err := SyncLegacyTokenMirror(configDir); err != nil {
			return err
		}
		return WriteTokenMarker(configDir)
	}
	if err := SaveTokenDataKeychain(data); err != nil {
		return err
	}
	return WriteTokenMarker(configDir)
}

// LoadTokenData reads TokenData. When an edition hook (LoadToken) is
// registered, it delegates entirely to the hook; otherwise it falls back
// to keychain with legacy .data migration.
func LoadTokenData(configDir string) (*TokenData, error) {
	return LoadTokenDataForProfile(configDir, RuntimeProfile())
}

// LoadTokenDataForProfile reads TokenData for a profile selector without mutating
// currentProfile. Empty selector follows the default resolution chain.
func LoadTokenDataForProfile(configDir, profile string) (*TokenData, error) {
	if h := edition.Get(); h.LoadToken != nil {
		if strings.TrimSpace(profile) != "" {
			return nil, fmt.Errorf("profile selection is not supported by the current auth backend")
		}
		jsonData, err := h.LoadToken(configDir)
		if err != nil {
			return nil, err
		}
		var td TokenData
		if err := json.Unmarshal(jsonData, &td); err != nil {
			return nil, fmt.Errorf("parsing token data from hook: %w", err)
		}
		return &td, nil
	}

	// Default: keychain with legacy .data migration
	selected, err := resolveProfileForLoad(configDir, profile)
	if err != nil {
		return nil, err
	}
	if selected != nil {
		data, err := LoadTokenDataKeychainForCorpID(selected.CorpID)
		if err == nil {
			return data, nil
		}
		if strings.TrimSpace(profile) != "" {
			return nil, err
		}
	}
	if TokenDataExistsKeychain() {
		return LoadTokenDataKeychain()
	}
	data, err := LoadSecureTokenData(configDir)
	if err != nil {
		return nil, err
	}
	if err := SaveTokenData(configDir, data); err == nil {
		_ = DeleteSecureData(configDir)
	}
	return data, nil
}

// DeleteTokenData removes token data. When an edition hook (DeleteToken) is
// registered, it delegates entirely to the hook; otherwise it falls back
// to keychain + legacy cleanup.
func DeleteTokenData(configDir string) error {
	return DeleteTokenDataForProfile(configDir, RuntimeProfile())
}

// DeleteTokenDataForProfile removes one profile's token data. Empty selector
// removes the current/default profile, falling back to legacy single-slot auth.
func DeleteTokenDataForProfile(configDir, profile string) error {
	if h := edition.Get(); h.DeleteToken != nil {
		if strings.TrimSpace(profile) != "" {
			return fmt.Errorf("profile selection is not supported by the current auth backend")
		}
		return h.DeleteToken(configDir)
	}
	selected, err := resolveProfileForLoad(configDir, profile)
	if err != nil {
		return err
	}
	if selected != nil {
		keychainErr := DeleteTokenDataKeychainForCorpID(selected.CorpID)
		_, removeErr := RemoveProfile(configDir, selected.CorpID)
		legacyErr := SyncLegacyTokenMirror(configDir)
		secureErr := DeleteSecureData(configDir)
		if keychainErr != nil {
			return keychainErr
		}
		if removeErr != nil {
			return removeErr
		}
		if legacyErr != nil {
			return legacyErr
		}
		return secureErr
	}

	keychainErr := DeleteTokenDataKeychain()
	legacyErr := DeleteSecureData(configDir)
	markerErr := DeleteTokenMarker(configDir)
	if keychainErr != nil {
		return keychainErr
	}
	if legacyErr != nil {
		return legacyErr
	}
	return markerErr
}

// DeleteAllTokenData removes all profile-scoped and legacy token data.
func DeleteAllTokenData(configDir string) error {
	if h := edition.Get(); h.DeleteToken != nil {
		return h.DeleteToken(configDir)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, profile := range cfg.Profiles {
		if err := DeleteTokenDataKeychainForCorpID(profile.CorpID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := os.Remove(ProfilesPath(configDir)); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	if err := DeleteTokenDataKeychain(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := DeleteSecureData(configDir); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := DeleteTokenMarker(configDir); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
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
