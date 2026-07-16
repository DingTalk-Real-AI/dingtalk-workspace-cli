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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var (
	tokenJSONMarshalIndent       = json.MarshalIndent
	tokenJSONMarshal             = json.Marshal
	tokenMkdirAll                = os.MkdirAll
	tokenWriteFile               = os.WriteFile
	tokenRename                  = os.Rename
	tokenRemove                  = os.Remove
	tokenGlob                    = filepath.Glob
	tokenSaveKeychainForCorpID   = SaveTokenDataKeychainForCorpID
	tokenSaveKeychain            = SaveTokenDataKeychain
	tokenLoadKeychainForCorpID   = LoadTokenDataKeychainForCorpID
	tokenLoadKeychain            = LoadTokenDataKeychain
	tokenKeychainExists          = TokenDataExistsKeychain
	tokenDeleteKeychainForCorpID = DeleteTokenDataKeychainForCorpID
	tokenDeleteKeychain          = DeleteTokenDataKeychain
	tokenLoadSecure              = LoadSecureTokenData
	tokenDeleteSecure            = DeleteSecureData
	tokenResolveProfile          = resolveProfileForLoad
	tokenUpsertProfile           = upsertProfileFromTokenWithCurrentLocked
	tokenRemoveProfile           = removeProfileLocked
	tokenSyncLegacyMirror        = syncLegacyTokenMirrorLocked
	tokenLoadProfiles            = LoadProfiles
	tokenWriteMarker             = WriteTokenMarker
	tokenDeleteMarker            = DeleteTokenMarker
	tokenParseURL                = url.Parse
	tokenNewRequest              = http.NewRequestWithContext
	tokenDefaultConfigDir        = getDefaultConfigDir
	tokenLoadData                = LoadTokenData
	tokenRevokeURL               = GetRevokeTokenURL
	tokenLogoutURL               = LogoutURL
	tokenLogoutContinueURL       = LogoutContinueURL
	tokenLogoutHTTPClient        = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tokenRevokeHTTPClient        = &http.Client{Timeout: 10 * time.Second}
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
	data, _ := tokenJSONMarshalIndent(marker, "", "  ")
	if err := tokenMkdirAll(configDir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(configDir, tokenJSONFile+"."+uuid.New().String()+".tmp")
	if err := tokenWriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return tokenRename(tmp, filepath.Join(configDir, tokenJSONFile))
}

// DeleteTokenMarker removes the token.json marker file.
func DeleteTokenMarker(configDir string) error {
	if err := tokenRemove(filepath.Join(configDir, tokenJSONFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SaveTokenData persists TokenData. When an edition hook (SaveToken) is
// registered, it delegates entirely to the hook; otherwise it falls back
// to the default keychain-based storage.
func SaveTokenData(configDir string, data *TokenData) error {
	if h := edition.Get(); h.SaveToken != nil {
		return saveTokenViaHook(h, configDir, data)
	}
	return withProfilesLock(configDir, func() error {
		return saveTokenDataLocked(configDir, data)
	})
}

// saveTokenDataLocked performs the keychain + profiles.json + legacy mirror
// writes assuming the auth dual-layer lock is already held. Callers that
// already hold the lock (OAuthProvider refresh path, the legacy secure->keychain
// migration in LoadTokenDataForProfile) must use this instead of SaveTokenData
// to avoid deadlocking on the non-reentrant lock.
func saveTokenDataLocked(configDir string, data *TokenData) error {
	if h := edition.Get(); h.SaveToken != nil {
		return saveTokenViaHook(h, configDir, data)
	}
	if data != nil && strings.TrimSpace(data.CorpID) != "" {
		if err := tokenSaveKeychainForCorpID(data.CorpID, data); err != nil {
			return err
		}
		makeCurrent := strings.TrimSpace(RuntimeProfile()) == ""
		if err := tokenUpsertProfile(configDir, data, makeCurrent); err != nil {
			return err
		}
		if makeCurrent {
			if err := tokenSaveKeychain(data); err != nil {
				return err
			}
		} else if err := tokenSyncLegacyMirror(configDir); err != nil {
			return err
		}
		return tokenWriteMarker(configDir)
	}
	if err := tokenSaveKeychain(data); err != nil {
		return err
	}
	return tokenWriteMarker(configDir)
}

func saveTokenViaHook(h *edition.Hooks, configDir string, data *TokenData) error {
	jsonData, err := tokenJSONMarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling token data for hook: %w", err)
	}
	return h.SaveToken(configDir, jsonData)
}

// LoadTokenData reads TokenData. When an edition hook (LoadToken) is
// registered, it delegates entirely to the hook; otherwise it falls back
// to keychain with legacy .data migration.
func LoadTokenData(configDir string) (*TokenData, error) {
	return LoadTokenDataForProfile(configDir, RuntimeProfile())
}

// LoadValidAccessTokenReadOnly resolves an already-valid access token without
// refreshing credentials, migrating legacy storage, repairing profiles.json,
// or mutating the selected profile. It is intended for strict preview paths
// whose authentication step must itself remain side-effect free.
func LoadValidAccessTokenReadOnly(configDir, profile string) (string, error) {
	data, err := LoadTokenDataForProfileReadOnly(configDir, profile)
	if err != nil {
		return "", err
	}
	return validAccessTokenReadOnly(data)
}

// LoadTokenDataForProfileReadOnly loads token data for audit/preview callers
// without refreshing credentials, migrating storage, quarantining malformed
// profiles, or changing the current profile.
func LoadTokenDataForProfileReadOnly(configDir, profile string) (*TokenData, error) {
	profile = strings.TrimSpace(profile)
	if h := edition.Get(); h.LoadToken != nil {
		if profile != "" {
			return nil, fmt.Errorf("严格 dry-run 的当前鉴权后端不支持 --profile；请使用 --token 或先切换默认 profile")
		}
		jsonData, err := h.LoadToken(configDir)
		if err != nil {
			return nil, fmt.Errorf("严格 dry-run 无法只读加载登录凭证: %w", err)
		}
		var data TokenData
		if err := json.Unmarshal(jsonData, &data); err != nil {
			return nil, fmt.Errorf("严格 dry-run 无法解析登录凭证: %w", err)
		}
		return &data, nil
	}

	profiles, err := loadProfilesReadOnly(configDir)
	if err != nil {
		return nil, err
	}
	return loadTokenDataFromKeychainReadOnly(profiles, profile)
}

func validAccessTokenReadOnly(data *TokenData) (string, error) {
	if data == nil || strings.TrimSpace(data.AccessToken) == "" {
		return "", fmt.Errorf("未找到登录凭证；严格 dry-run 不会自动登录，请先执行 dws auth login")
	}
	if !data.IsAccessTokenValid() {
		return "", fmt.Errorf("登录凭证已过期；严格 dry-run 不会自动刷新，请先执行 dws auth login")
	}
	return strings.TrimSpace(data.AccessToken), nil
}

// loadProfilesReadOnly deliberately avoids LoadProfiles: malformed profile
// metadata is reported in place rather than quarantined/renamed.
func loadProfilesReadOnly(configDir string) (*ProfilesConfig, error) {
	profiles := &ProfilesConfig{Version: 1}
	raw, err := os.ReadFile(ProfilesPath(configDir))
	if err != nil {
		if os.IsNotExist(err) {
			return profiles, nil
		}
		return nil, fmt.Errorf("严格 dry-run 无法只读加载 profiles: %w", err)
	}
	if err := json.Unmarshal(raw, profiles); err != nil {
		return nil, fmt.Errorf("严格 dry-run 无法解析 profiles（不会自动修复）: %w", err)
	}
	return profiles, nil
}

func loadTokenDataFromKeychainReadOnly(profiles *ProfilesConfig, selector string) (*TokenData, error) {
	if selector != "" {
		selected := findProfile(profiles, selector)
		if selected == nil {
			return nil, fmt.Errorf("profile %q not found", selector)
		}
		data, err := LoadTokenDataKeychainForCorpID(selected.CorpID)
		if err != nil {
			return nil, readOnlyTokenLoadError(err)
		}
		return data, nil
	}

	var fallbackProfile *Profile
	seen := map[string]bool{}
	for _, candidate := range []string{profiles.CurrentProfile, profiles.PrimaryProfile} {
		selected := findProfile(profiles, candidate)
		if selected == nil || seen[selected.CorpID] {
			continue
		}
		seen[selected.CorpID] = true
		if fallbackProfile == nil {
			fallbackProfile = selected
		}
		data, err := LoadTokenDataKeychainForCorpID(selected.CorpID)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, ErrTokenDataNotFound) {
			return nil, fmt.Errorf("严格 dry-run 无法只读加载 profile %q 的凭证: %w", selected.CorpID, err)
		}
	}

	legacy, err := LoadTokenDataKeychain()
	if err != nil {
		return nil, readOnlyTokenLoadError(err)
	}
	if fallbackProfile != nil && strings.TrimSpace(legacy.CorpID) != strings.TrimSpace(fallbackProfile.CorpID) {
		return nil, fmt.Errorf("当前 profile 没有登录凭证；严格 dry-run 不会切换到其他组织")
	}
	return legacy, nil
}

func readOnlyTokenLoadError(err error) error {
	if errors.Is(err, ErrTokenDataNotFound) {
		return fmt.Errorf("未找到登录凭证；严格 dry-run 不会迁移旧凭证，请先执行 dws auth login")
	}
	return fmt.Errorf("严格 dry-run 无法只读加载登录凭证: %w", err)
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
	selected, err := tokenResolveProfile(configDir, profile)
	if err != nil {
		return nil, err
	}
	if selected != nil {
		data, err := tokenLoadKeychainForCorpID(selected.CorpID)
		if err == nil {
			return data, nil
		}
		if strings.TrimSpace(profile) != "" || !errors.Is(err, ErrTokenDataNotFound) {
			return nil, err
		}
		// No explicit --profile: `selected` is the resolved current/primary
		// profile. Only fall back to the legacy single slot when it belongs to
		// the SAME org; otherwise surface the error instead of silently acting
		// as a different organization (the legacy mirror may have drifted).
		if legacy, lerr := tokenLoadKeychain(); lerr == nil && legacy != nil &&
			strings.TrimSpace(legacy.CorpID) == strings.TrimSpace(selected.CorpID) {
			return legacy, nil
		} else if lerr != nil && !errors.Is(lerr, ErrTokenDataNotFound) {
			return nil, lerr
		}
		return nil, err
	}
	if tokenKeychainExists() {
		return tokenLoadKeychain()
	}
	data, err := tokenLoadSecure(configDir)
	if err != nil {
		return nil, err
	}
	// One-time legacy secure-store -> keychain migration. This read path may run
	// while the refresh lock is already held, so use the lock-free saver.
	if err := saveTokenDataLocked(configDir, data); err == nil {
		_ = tokenDeleteSecure(configDir)
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
	return withProfilesLock(configDir, func() error {
		return deleteTokenDataForProfileLocked(configDir, profile)
	})
}

func deleteTokenDataForProfileLocked(configDir, profile string) error {
	selected, err := tokenResolveProfile(configDir, profile)
	if err != nil {
		return err
	}
	if selected != nil {
		keychainErr := tokenDeleteKeychainForCorpID(selected.CorpID)
		_, removeErr := tokenRemoveProfile(configDir, selected.CorpID)
		legacyErr := tokenSyncLegacyMirror(configDir)
		secureErr := tokenDeleteSecure(configDir)
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

	keychainErr := tokenDeleteKeychain()
	legacyErr := tokenDeleteSecure(configDir)
	markerErr := tokenDeleteMarker(configDir)
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
	return withProfilesLock(configDir, func() error {
		var firstErr error
		// Best-effort: even if profiles.json is unreadable, still clear every
		// other slot so the user can always self-heal via auth reset / logout.
		if cfg, err := tokenLoadProfiles(configDir); err == nil {
			for _, profile := range cfg.Profiles {
				if e := tokenDeleteKeychainForCorpID(profile.CorpID); e != nil && firstErr == nil {
					firstErr = e
				}
			}
		}
		if e := tokenRemove(ProfilesPath(configDir)); e != nil && !os.IsNotExist(e) && firstErr == nil {
			firstErr = e
		}
		// Sweep any quarantined corrupt-profiles files so they don't accumulate.
		if matches, _ := tokenGlob(ProfilesPath(configDir) + ".corrupt-*"); len(matches) > 0 {
			for _, m := range matches {
				if e := tokenRemove(m); e != nil && !os.IsNotExist(e) && firstErr == nil {
					firstErr = e
				}
			}
		}
		if e := tokenDeleteKeychain(); e != nil && firstErr == nil {
			firstErr = e
		}
		if e := tokenDeleteSecure(configDir); e != nil && firstErr == nil {
			firstErr = e
		}
		if e := tokenDeleteMarker(configDir); e != nil && firstErr == nil {
			firstErr = e
		}
		return firstErr
	})
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
	logoutURL, err := tokenParseURL(tokenLogoutURL)
	if err != nil {
		return fmt.Errorf("parsing logout URL: %w", err)
	}

	q := logoutURL.Query()
	q.Set("client_id", ClientID())
	q.Set("continue", tokenLogoutContinueURL)
	logoutURL.RawQuery = q.Encode()

	req, err := tokenNewRequest(ctx, http.MethodGet, logoutURL.String(), nil)
	if err != nil {
		return fmt.Errorf("creating logout request: %w", err)
	}

	resp, err := tokenLogoutHTTPClient.Do(req)
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
	revokeURL := tokenRevokeURL()
	if revokeURL == "" {
		return nil // No revoke endpoint available
	}

	// Load current token to get accessToken
	tokenData, err := tokenLoadData(tokenDefaultConfigDir())
	if err != nil || tokenData == nil {
		return nil // No token to revoke
	}

	body := map[string]string{
		"clientId":    ClientID(),
		"accessToken": tokenData.AccessToken,
	}
	bodyBytes, err := tokenJSONMarshal(body)
	if err != nil {
		return fmt.Errorf("marshaling revoke request: %w", err)
	}

	req, err := tokenNewRequest(ctx, http.MethodPost, revokeURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tokenRevokeHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling revoke endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revoke endpoint returned status %d", resp.StatusCode)
	}

	return nil
}
