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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
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
	tokenSaveKeychainForIdentity = SaveTokenDataKeychainForIdentity
	tokenSaveKeychain            = SaveTokenDataKeychain
	tokenLoadKeychainForCorpID   = LoadTokenDataKeychainForCorpID
	tokenLoadKeychainIdentity    = LoadTokenDataKeychainForIdentity
	tokenLoadKeychain            = LoadTokenDataKeychain
	tokenKeychainExists          = TokenDataExistsKeychain
	tokenDeleteKeychainForCorpID = DeleteTokenDataKeychainForCorpID
	tokenDeleteKeychainIdentity  = DeleteTokenDataKeychainForIdentity
	tokenDeleteKeychain          = DeleteTokenDataKeychain
	tokenRemoveAuthTokenEntries  = keychain.RemoveAuthTokenEntries
	tokenLoadSecure              = LoadSecureTokenData
	tokenDeleteSecure            = DeleteSecureData
	tokenResolveProfile          = func(configDir, selector string) (*Profile, error) {
		profile, _, err := resolveProfileForLoadLocked(configDir, selector)
		return profile, err
	}
	tokenResolveDeletion        = resolveProfileDeletionSelection
	tokenResolveSelection       = resolveProfileSelection
	tokenUpsertProfile          = upsertProfileFromTokenWithCurrentLocked
	tokenRemoveProfile          = removeProfileLocked
	tokenSyncLegacyMirror       = syncLegacyTokenMirrorLocked
	tokenSyncOrganizationMirror = syncOrganizationTokenMirrorForProfile
	tokenLoadProfiles           = LoadProfiles
	tokenWriteMarker            = WriteTokenMarker
	tokenDeleteMarker           = DeleteTokenMarker
	tokenParseURL               = url.Parse
	tokenNewRequest             = http.NewRequestWithContext
	tokenDefaultConfigDir       = getDefaultConfigDir
	tokenLoadData               = LoadTokenData
	tokenRevokeURL              = GetRevokeTokenURL
	tokenMCPBaseURL             = GetMCPBaseURL
	tokenLogoutURL              = LogoutURL
	tokenLogoutContinueURL      = LogoutContinueURL
	tokenLogoutHTTPClient       = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tokenRevokeHTTPClient       = &http.Client{Timeout: 10 * time.Second}
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
		corpID := strings.TrimSpace(data.CorpID)
		userID := strings.TrimSpace(data.UserID)
		if userID != "" {
			if err := tokenSaveKeychainForIdentity(corpID, userID, data); err != nil {
				return err
			}
		} else {
			cfg, err := tokenLoadProfiles(configDir)
			if err != nil {
				return err
			}
			for _, profile := range cfg.Profiles {
				if strings.TrimSpace(profile.CorpID) == corpID && strings.TrimSpace(profile.UserID) != "" {
					return fmt.Errorf("cannot store profile for corpId %q without userId because account identities already exist", corpID)
				}
			}
		}
		runtimeSelector := strings.TrimSpace(RuntimeProfile())
		makeCurrent := runtimeSelector == ""
		if err := tokenUpsertProfile(configDir, data, makeCurrent); err != nil {
			return err
		}
		cfg, err := tokenLoadProfiles(configDir)
		if err != nil {
			return err
		}
		exactSelector := profileSelector(corpID, userID)
		mirrorOrg := makeCurrent ||
			exactProfileSelectorForCorp(cfg, corpID, cfg.OrgCurrentProfiles[corpID]) == exactSelector
		if mirrorOrg {
			if err := tokenSaveKeychainForCorpID(corpID, data); err != nil {
				return err
			}
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

	var result *TokenData
	err := withProfilesLock(configDir, func() error {
		var loadErr error
		result, loadErr = loadTokenDataForProfileLocked(configDir, profile)
		return loadErr
	})
	return result, err
}

func loadTokenDataForProfileLocked(configDir, profile string) (*TokenData, error) {
	// Default: keychain with legacy .data migration
	selected, err := tokenResolveProfile(configDir, profile)
	if err != nil {
		return nil, err
	}
	if selected != nil {
		data, err := tokenLoadProfileIdentity(*selected)
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
			strings.TrimSpace(legacy.CorpID) == strings.TrimSpace(selected.CorpID) &&
			(strings.TrimSpace(selected.UserID) == "" || strings.TrimSpace(legacy.UserID) == strings.TrimSpace(selected.UserID)) {
			return legacy, nil
		} else if lerr != nil && !errors.Is(lerr, ErrTokenDataNotFound) {
			return nil, lerr
		}
		return nil, err
	}
	cfg, err := tokenLoadProfiles(configDir)
	if err != nil {
		return nil, err
	}
	if cfg != nil && cfg.Version >= profilesVersion {
		return nil, ErrTokenDataNotFound
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

func tokenLoadProfileIdentity(profile Profile) (*TokenData, error) {
	if strings.TrimSpace(profile.UserID) == "" {
		return tokenLoadKeychainForCorpID(profile.CorpID)
	}
	data, err := tokenLoadKeychainIdentity(profile.CorpID, profile.UserID)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, ErrTokenDataNotFound) {
		return nil, err
	}
	orgData, orgErr := tokenLoadKeychainForCorpID(profile.CorpID)
	if orgErr != nil {
		if errors.Is(orgErr, ErrTokenDataNotFound) {
			return nil, err
		}
		return nil, orgErr
	}
	if strings.TrimSpace(orgData.UserID) == "" {
		return nil, fmt.Errorf("organization token mirror for corpId %q has no userId; cannot use it for profile %q", profile.CorpID, ProfileSelector(profile))
	}
	if strings.TrimSpace(orgData.UserID) != strings.TrimSpace(profile.UserID) {
		return nil, err
	}
	if saveErr := tokenSaveKeychainForIdentity(profile.CorpID, profile.UserID, orgData); saveErr != nil {
		return nil, saveErr
	}
	return orgData, nil
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
	if err := profilesEnsureMigration(configDir); err != nil {
		return err
	}
	cfg, err := tokenLoadProfiles(configDir)
	if err != nil {
		return err
	}
	effectiveSelector := strings.TrimSpace(profile)
	if effectiveSelector == "" {
		effectiveSelector = strings.TrimSpace(cfg.CurrentProfile)
	}
	var selected *Profile
	exact := false
	if effectiveSelector != "" {
		selected, exact, err = tokenResolveDeletion(cfg, effectiveSelector)
		if err != nil {
			return err
		}
	}
	if selected != nil {
		removed := *selected
		if exact {
			orgCurrent := exactProfileSelectorForCorp(
				cfg,
				removed.CorpID,
				cfg.OrgCurrentProfiles[removed.CorpID],
			) == ProfileSelector(removed)
			if strings.TrimSpace(removed.UserID) != "" {
				if err := tokenDeleteKeychainIdentity(removed.CorpID, removed.UserID); err != nil {
					return err
				}
			}
			if _, err := tokenRemoveProfile(configDir, ProfileSelector(removed)); err != nil {
				return err
			}
			if orgCurrent {
				if err := tokenDeleteKeychainForCorpID(removed.CorpID); err != nil {
					return err
				}
				updated, loadErr := tokenLoadProfiles(configDir)
				if loadErr != nil {
					return loadErr
				}
				if replacementSelector := updated.OrgCurrentProfiles[removed.CorpID]; replacementSelector != "" {
					replacement, _, resolveErr := tokenResolveSelection(configDir, updated, replacementSelector)
					if resolveErr != nil {
						return resolveErr
					}
					if err := tokenSyncOrganizationMirror(*replacement); err != nil {
						return err
					}
				}
			}
		} else {
			for _, candidate := range cfg.Profiles {
				if strings.TrimSpace(candidate.CorpID) != strings.TrimSpace(removed.CorpID) || strings.TrimSpace(candidate.UserID) == "" {
					continue
				}
				if err := tokenDeleteKeychainIdentity(candidate.CorpID, candidate.UserID); err != nil {
					return err
				}
			}
			if err := tokenDeleteKeychainForCorpID(removed.CorpID); err != nil {
				return err
			}
			if _, err := tokenRemoveProfile(configDir, removed.CorpID); err != nil {
				return err
			}
		}
		if err := tokenSyncLegacyMirror(configDir); err != nil {
			return err
		}
		return tokenDeleteSecure(configDir)
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
		// Sweep the complete auth-token namespace so orphan identity slots that
		// are not present in profiles.json cannot survive reset/logout --all.
		if e := tokenRemoveAuthTokenEntries(keychain.Service); e != nil {
			firstErr = e
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
		if e := tokenDeleteSecure(configDir); e != nil && firstErr == nil {
			firstErr = e
		}
		if e := tokenDeleteMarker(configDir); e != nil && firstErr == nil {
			firstErr = e
		}
		if firstErr != nil {
			// Preserve an explicit v2 empty registry so any stale mirror that
			// could not be removed is never imported on a later read.
			if e := profilesSave(configDir, &ProfilesConfig{Version: profilesVersion}); e != nil {
				return fmt.Errorf("%v; save logged-out profile tombstone: %w", firstErr, e)
			}
		}
		return firstErr
	})
}

// RevokeTokenRemote calls the appropriate logout/revoke endpoint to invalidate the access token.
// Uses MCP revoke endpoint when clientID is from MCP, otherwise uses DingTalk logout.
// This should be called before deleting local token data.
// The function is best-effort: errors are returned but callers may choose to ignore them.
func RevokeTokenRemote(ctx context.Context) error {
	tokenData, err := tokenLoadData(tokenDefaultConfigDir())
	if err != nil || tokenData == nil {
		return nil
	}
	// Historical token records may not have Source. Preserve the legacy
	// process-wide MCP decision only for those records.
	if strings.TrimSpace(tokenData.Source) == "" && IsClientIDFromMCP() {
		copy := *tokenData
		copy.Source = "mcp"
		tokenData = &copy
	}
	return RevokeTokenRemoteForData(ctx, tokenData)
}

// RevokeTokenRemoteForData revokes the supplied account token using the
// credential source and client ID persisted with that exact identity.
func RevokeTokenRemoteForData(ctx context.Context, tokenData *TokenData) error {
	if tokenData == nil {
		return nil
	}
	clientID := strings.TrimSpace(tokenData.ClientID)
	if clientID == "" {
		clientID = ClientID()
	}
	if strings.EqualFold(strings.TrimSpace(tokenData.Source), "mcp") {
		return revokeTokenViaMCP(ctx, tokenData, clientID)
	}

	// Direct mode: use DingTalk logout endpoint.
	logoutURL, err := tokenParseURL(tokenLogoutURL)
	if err != nil {
		return fmt.Errorf("parsing logout URL: %w", err)
	}

	q := logoutURL.Query()
	q.Set("client_id", clientID)
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
func revokeTokenViaMCP(ctx context.Context, tokenData *TokenData, clientID string) error {
	revokeURL := tokenMCPBaseURL() + MCPRevokeTokenPath
	body := map[string]string{
		"clientId":    clientID,
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
