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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var (
	tokenJSONMarshalIndent       = json.MarshalIndent
	tokenJSONMarshal             = json.Marshal
	tokenMkdirAll                = os.MkdirAll
	tokenReadFile                = os.ReadFile
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
	tokenSaveProfiles           = SaveProfiles
	tokenWriteMarker            = WriteTokenMarker
	tokenWriteManualMarker      = WriteManualTokenMarker
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
	UpdatedAt   string `json:"updated_at"`
	ManualToken bool   `json:"manual_token,omitempty"`
	// Revision changes on every credential publication. Runtime token caches
	// use it as a cheap cross-process invalidation signal without reading the
	// platform keychain on every request.
	Revision string `json:"revision,omitempty"`
}

// WriteTokenMarker writes a token.json marker containing only an updated_at
// timestamp. The host application uses this file's presence and mtime to
// decide whether it needs to trigger a new auth exchange.
func WriteTokenMarker(configDir string) error {
	return writeTokenMarker(configDir, false)
}

// WriteManualTokenMarker marks the legacy global keychain slot as an explicit
// `auth login --token` credential. The additive field keeps older hosts, which
// only inspect token.json presence and mtime, fully compatible.
func WriteManualTokenMarker(configDir string) error {
	return writeTokenMarker(configDir, true)
}

func writeTokenMarker(configDir string, manual bool) error {
	marker := TokenMarker{
		UpdatedAt:   time.Now().Format(time.RFC3339),
		ManualToken: manual,
		Revision:    uuid.NewString(),
	}
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

// ReadTokenMarkerRevision returns the current credential publication revision.
// Existing markers without a revision remain readable, but callers must avoid
// caching them because they cannot prove that the credential is unchanged.
func ReadTokenMarkerRevision(configDir string) (revision string, present bool, err error) {
	data, err := tokenReadFile(filepath.Join(configDir, tokenJSONFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read token marker: %w", err)
	}
	var marker TokenMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		// The marker is only a cache-coherency hint. A malformed historical or
		// externally modified marker must disable caching, not make an otherwise
		// valid credential unusable.
		return "", true, nil
	}
	return strings.TrimSpace(marker.Revision), true, nil
}

func manualTokenMarkerActive(configDir string) (bool, error) {
	data, err := tokenReadFile(filepath.Join(configDir, tokenJSONFile))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var marker TokenMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		// Historical hosts only require the marker's presence. A malformed old
		// marker must not make profile authentication unusable.
		return false, nil
	}
	return marker.ManualToken, nil
}

// DeleteTokenMarker removes the token.json marker file.
func DeleteTokenMarker(configDir string) error {
	if err := tokenRemove(filepath.Join(configDir, tokenJSONFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SaveTokenData persists TokenData under the auth dual lock. When an edition
// hook (SaveToken) is registered, the locked write delegates to that hook;
// otherwise it falls back to the default keychain-based storage.
func SaveTokenData(configDir string, data *TokenData) error {
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
		cfg, err := tokenLoadProfiles(configDir)
		if err != nil {
			return err
		}
		if err := ensureProfilesWritable(cfg); err != nil {
			return err
		}
		runtimeSelector := strings.TrimSpace(RuntimeProfile())
		makeCurrent := runtimeSelector == ""
		exactSelector := profileSelector(corpID, userID)
		mirrorOrg := makeCurrent ||
			exactProfileSelectorForCorp(cfg, corpID, cfg.OrgCurrentProfiles[corpID]) == exactSelector
		existingIdentity := profileIndexByIdentity(cfg, corpID, userID) >= 0
		upgradesLegacyProfile := !existingIdentity && userID != "" && legacyProfileIndexByCorpID(cfg, corpID) >= 0
		logging.AuthDebug(
			"auth.token.persist.plan",
			"corp_id", corpID,
			"user_id", userID,
			"user_name", strings.TrimSpace(data.UserName),
			"identity_selector", exactSelector,
			"existing_identity", existingIdentity,
			"upgrades_legacy_profile", upgradesLegacyProfile,
			"profiles_before", len(cfg.Profiles),
			"runtime_profile", runtimeSelector,
			"write_identity_slot", userID != "",
			"write_org_mirror", mirrorOrg,
			"write_global_mirror", makeCurrent,
		)
		snapshot, err := snapshotTokenPersistence(configDir, cfg, corpID, userID, mirrorOrg)
		if err != nil {
			return err
		}
		preserveManualDefault := !makeCurrent &&
			snapshot.marker.known &&
			snapshot.marker.exists &&
			snapshot.marker.manual
		rollback := func(operationErr error) error {
			if rollbackErr := restoreTokenPersistence(configDir, snapshot); rollbackErr != nil {
				return errors.Join(operationErr, fmt.Errorf("rollback token persistence: %w", rollbackErr))
			}
			return operationErr
		}
		if userID != "" {
			if err := tokenSaveKeychainForIdentity(corpID, userID, data); err != nil {
				return rollback(err)
			}
		} else {
			for _, profile := range cfg.Profiles {
				if strings.TrimSpace(profile.CorpID) == corpID && strings.TrimSpace(profile.UserID) != "" {
					return fmt.Errorf("cannot store profile for corpId %q without userId because account identities already exist", corpID)
				}
			}
		}
		if err := tokenUpsertProfile(configDir, data, makeCurrent); err != nil {
			return rollback(err)
		}
		if mirrorOrg {
			if err := tokenSaveKeychainForCorpID(corpID, data); err != nil {
				return rollback(err)
			}
		}
		if makeCurrent {
			if err := tokenSaveKeychain(data); err != nil {
				return rollback(err)
			}
		} else if !preserveManualDefault {
			if err := tokenSyncLegacyMirror(configDir); err != nil {
				return rollback(err)
			}
		}
		if preserveManualDefault {
			if err := tokenWriteManualMarker(configDir); err != nil {
				return rollback(err)
			}
		} else if err := tokenWriteMarker(configDir); err != nil {
			return rollback(err)
		}
		logging.AuthDebug(
			"auth.token.persist.done",
			"corp_id", corpID,
			"user_id", userID,
			"user_name", strings.TrimSpace(data.UserName),
			"identity_selector", exactSelector,
			"write_identity_slot", userID != "",
			"write_org_mirror", mirrorOrg,
			"write_global_mirror", makeCurrent,
		)
		return nil
	}
	legacySnapshot, err := snapshotTokenSlot(tokenLoadKeychain)
	if err != nil {
		return err
	}
	markerSnapshot, err := snapshotTokenMarker(configDir)
	if err != nil {
		return err
	}
	if err := tokenSaveKeychain(data); err != nil {
		return err
	}
	if err := tokenWriteManualMarker(configDir); err != nil {
		var rollbackErr error
		if restoreErr := restoreTokenSlot(
			legacySnapshot,
			tokenSaveKeychain,
			tokenDeleteKeychain,
		); restoreErr != nil {
			rollbackErr = errors.Join(rollbackErr, restoreErr)
		}
		if restoreErr := restoreTokenMarker(configDir, markerSnapshot); restoreErr != nil {
			rollbackErr = errors.Join(rollbackErr, restoreErr)
		}
		if rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback manual token persistence: %w", rollbackErr))
		}
		return err
	}
	return nil
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
	if strings.TrimSpace(profile) == "" {
		manual, err := manualTokenMarkerActive(configDir)
		if err != nil {
			return nil, err
		}
		if manual {
			data, loadErr := tokenLoadKeychain()
			if loadErr == nil && data != nil && strings.TrimSpace(data.CorpID) == "" {
				return data, nil
			}
			if loadErr != nil && !errors.Is(loadErr, ErrTokenDataNotFound) {
				return nil, loadErr
			}
		}
	}
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

// DeleteTokenData removes token data. Edition hooks and the default keychain
// path are both serialized with refresh through the auth dual lock.
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
		return withProfilesLock(configDir, func() error {
			return h.DeleteToken(configDir)
		})
	}
	return withProfilesLock(configDir, func() error {
		return deleteTokenDataForProfileLocked(configDir, profile)
	})
}

func deleteTokenDataForProfileLocked(configDir, profile string) error {
	if strings.TrimSpace(profile) == "" {
		manual, err := manualTokenMarkerActive(configDir)
		if err != nil {
			return err
		}
		if manual {
			return deleteManualTokenDataLocked(configDir)
		}
	}
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
		originalCfg := cloneProfilesConfig(cfg)
		identitySnapshots, err := snapshotDeletionIdentities(cfg, removed, exact)
		if err != nil {
			return err
		}
		orgSnapshot := snapshotTokenSlotForDeletion(func() (*TokenData, error) {
			return tokenLoadKeychainForCorpID(removed.CorpID)
		})
		legacySnapshot := snapshotTokenSlotForDeletion(tokenLoadKeychain)
		markerSnapshot := snapshotTokenMarkerForDeletion(configDir)

		// Clean the deprecated secure-store copy before changing the profile
		// transaction. A cleanup failure therefore leaves all current metadata
		// and keychain slots untouched.
		if err := tokenDeleteSecure(configDir); err != nil {
			return err
		}

		removeSelector := removed.CorpID
		orgCurrent := false
		if exact {
			removeSelector = ProfileSelector(removed)
			orgCurrent = exactProfileSelectorForCorp(
				cfg,
				removed.CorpID,
				cfg.OrgCurrentProfiles[removed.CorpID],
			) == ProfileSelector(removed)
		}
		if _, err := tokenRemoveProfile(configDir, removeSelector); err != nil {
			return err
		}
		rollback := func(operationErr error) error {
			if rollbackErr := restoreProfileDeletion(
				configDir,
				originalCfg,
				identitySnapshots,
				removed.CorpID,
				orgSnapshot,
				legacySnapshot,
				markerSnapshot,
			); rollbackErr != nil {
				return errors.Join(operationErr, fmt.Errorf("rollback profile deletion: %w", rollbackErr))
			}
			return operationErr
		}

		if !exact || orgCurrent {
			updated, loadErr := tokenLoadProfiles(configDir)
			if loadErr != nil {
				return rollback(loadErr)
			}
			replacementSelector := updated.OrgCurrentProfiles[removed.CorpID]
			if exact && replacementSelector != "" {
				replacement, _, resolveErr := tokenResolveSelection(configDir, updated, replacementSelector)
				if resolveErr != nil {
					return rollback(resolveErr)
				}
				if err := tokenSyncOrganizationMirror(*replacement); err != nil {
					return rollback(err)
				}
			} else if err := tokenDeleteKeychainForCorpID(removed.CorpID); err != nil {
				return rollback(err)
			}
		}
		preserveManualDefault := markerSnapshot.known &&
			markerSnapshot.exists &&
			markerSnapshot.manual
		if !preserveManualDefault {
			if err := tokenSyncLegacyMirror(configDir); err != nil {
				return rollback(err)
			}
		}
		for _, snapshot := range identitySnapshots {
			if err := tokenDeleteKeychainIdentity(snapshot.profile.CorpID, snapshot.profile.UserID); err != nil {
				return rollback(err)
			}
		}
		return nil
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

func deleteManualTokenDataLocked(configDir string) error {
	legacySnapshot := snapshotTokenSlotForDeletion(tokenLoadKeychain)
	if err := tokenDeleteSecure(configDir); err != nil {
		return err
	}
	if err := tokenDeleteKeychain(); err != nil {
		return err
	}
	if err := tokenDeleteMarker(configDir); err != nil {
		if legacySnapshot.known {
			if rollbackErr := restoreTokenSlot(
				legacySnapshot,
				tokenSaveKeychain,
				tokenDeleteKeychain,
			); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("rollback manual token deletion: %w", rollbackErr))
			}
		}
		return err
	}
	return nil
}

type deletionIdentitySnapshot struct {
	profile Profile
	token   *TokenData
}

type tokenSlotSnapshot struct {
	token  *TokenData
	known  bool
	exists bool
}

type tokenMarkerSnapshot struct {
	known  bool
	exists bool
	manual bool
}

type tokenPersistenceSnapshot struct {
	profiles *ProfilesConfig
	corpID   string
	userID   string
	identity tokenSlotSnapshot
	org      tokenSlotSnapshot
	legacy   tokenSlotSnapshot
	marker   tokenMarkerSnapshot
}

func cloneProfilesConfig(cfg *ProfilesConfig) *ProfilesConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.Profiles = append([]Profile(nil), cfg.Profiles...)
	for i := range cloned.Profiles {
		cloned.Profiles[i].AuthorizedDomains = append(
			[]string(nil),
			cloned.Profiles[i].AuthorizedDomains...,
		)
	}
	if cfg.OrgCurrentProfiles != nil {
		cloned.OrgCurrentProfiles = make(map[string]string, len(cfg.OrgCurrentProfiles))
		for corpID, selector := range cfg.OrgCurrentProfiles {
			cloned.OrgCurrentProfiles[corpID] = selector
		}
	}
	return &cloned
}

func snapshotDeletionIdentities(cfg *ProfilesConfig, removed Profile, exact bool) ([]deletionIdentitySnapshot, error) {
	var profiles []Profile
	if exact {
		profiles = []Profile{removed}
	} else {
		for _, candidate := range cfg.Profiles {
			if strings.TrimSpace(candidate.CorpID) == strings.TrimSpace(removed.CorpID) {
				profiles = append(profiles, candidate)
			}
		}
	}
	snapshots := make([]deletionIdentitySnapshot, 0, len(profiles))
	for _, candidate := range profiles {
		if strings.TrimSpace(candidate.UserID) == "" {
			continue
		}
		data, err := tokenLoadKeychainIdentity(candidate.CorpID, candidate.UserID)
		if err != nil {
			if errors.Is(err, ErrTokenDataNotFound) {
				snapshots = append(snapshots, deletionIdentitySnapshot{profile: candidate})
				continue
			}
			// A damaged target slot must remain removable. It cannot be restored
			// during rollback, but every readable slot in the same transaction
			// still is.
			snapshots = append(snapshots, deletionIdentitySnapshot{profile: candidate})
			continue
		}
		snapshots = append(snapshots, deletionIdentitySnapshot{profile: candidate, token: data})
	}
	return snapshots, nil
}

func snapshotTokenSlot(load func() (*TokenData, error)) (tokenSlotSnapshot, error) {
	data, err := load()
	if err != nil {
		if errors.Is(err, ErrTokenDataNotFound) {
			return tokenSlotSnapshot{known: true}, nil
		}
		return tokenSlotSnapshot{}, err
	}
	return tokenSlotSnapshot{token: data, known: true, exists: data != nil}, nil
}

func snapshotTokenSlotForDeletion(load func() (*TokenData, error)) tokenSlotSnapshot {
	snapshot, err := snapshotTokenSlot(load)
	if err != nil {
		return tokenSlotSnapshot{}
	}
	return snapshot
}

func snapshotTokenMarker(configDir string) (tokenMarkerSnapshot, error) {
	data, err := tokenReadFile(filepath.Join(configDir, tokenJSONFile))
	if err != nil {
		if os.IsNotExist(err) {
			return tokenMarkerSnapshot{known: true}, nil
		}
		return tokenMarkerSnapshot{}, err
	}
	var marker TokenMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return tokenMarkerSnapshot{known: true, exists: true}, nil
	}
	return tokenMarkerSnapshot{known: true, exists: true, manual: marker.ManualToken}, nil
}

func snapshotTokenMarkerForDeletion(configDir string) tokenMarkerSnapshot {
	snapshot, err := snapshotTokenMarker(configDir)
	if err != nil {
		return tokenMarkerSnapshot{}
	}
	return snapshot
}

func restoreProfileDeletion(
	configDir string,
	cfg *ProfilesConfig,
	identities []deletionIdentitySnapshot,
	corpID string,
	org tokenSlotSnapshot,
	legacy tokenSlotSnapshot,
	marker tokenMarkerSnapshot,
) error {
	var rollbackErr error
	for _, snapshot := range identities {
		if snapshot.token == nil {
			continue
		}
		if err := tokenSaveKeychainForIdentity(
			snapshot.profile.CorpID,
			snapshot.profile.UserID,
			snapshot.token,
		); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if err := tokenSaveProfiles(configDir, cloneProfilesConfig(cfg)); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if org.known {
		if org.exists {
			if err := tokenSaveKeychainForCorpID(corpID, org.token); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		} else if err := tokenDeleteKeychainForCorpID(corpID); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if legacy.known {
		if legacy.exists {
			if err := tokenSaveKeychain(legacy.token); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		} else if err := tokenDeleteKeychain(); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if marker.known {
		if err := restoreTokenMarker(configDir, marker); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func snapshotTokenPersistence(
	configDir string,
	cfg *ProfilesConfig,
	corpID, userID string,
	includeOrganization bool,
) (tokenPersistenceSnapshot, error) {
	snapshot := tokenPersistenceSnapshot{
		profiles: cloneProfilesConfig(cfg),
		corpID:   corpID,
		userID:   userID,
	}
	var err error
	if strings.TrimSpace(userID) != "" {
		snapshot.identity, err = snapshotTokenSlot(func() (*TokenData, error) {
			return tokenLoadKeychainIdentity(corpID, userID)
		})
		if err != nil {
			return tokenPersistenceSnapshot{}, err
		}
	}
	if includeOrganization {
		snapshot.org, err = snapshotTokenSlot(func() (*TokenData, error) {
			return tokenLoadKeychainForCorpID(corpID)
		})
		if err != nil {
			return tokenPersistenceSnapshot{}, err
		}
	}
	snapshot.legacy, err = snapshotTokenSlot(tokenLoadKeychain)
	if err != nil {
		return tokenPersistenceSnapshot{}, err
	}
	snapshot.marker, err = snapshotTokenMarker(configDir)
	if err != nil {
		return tokenPersistenceSnapshot{}, err
	}
	return snapshot, nil
}

func restoreTokenPersistence(configDir string, snapshot tokenPersistenceSnapshot) error {
	var rollbackErr error
	if err := tokenSaveProfiles(configDir, cloneProfilesConfig(snapshot.profiles)); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if strings.TrimSpace(snapshot.userID) != "" && snapshot.identity.known {
		if err := restoreTokenSlot(
			snapshot.identity,
			func(data *TokenData) error {
				return tokenSaveKeychainForIdentity(snapshot.corpID, snapshot.userID, data)
			},
			func() error {
				return tokenDeleteKeychainIdentity(snapshot.corpID, snapshot.userID)
			},
		); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if snapshot.org.known {
		if err := restoreTokenSlot(
			snapshot.org,
			func(data *TokenData) error {
				return tokenSaveKeychainForCorpID(snapshot.corpID, data)
			},
			func() error {
				return tokenDeleteKeychainForCorpID(snapshot.corpID)
			},
		); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if snapshot.legacy.known {
		if err := restoreTokenSlot(snapshot.legacy, tokenSaveKeychain, tokenDeleteKeychain); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if snapshot.marker.known {
		if err := restoreTokenMarker(configDir, snapshot.marker); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func restoreTokenSlot(
	snapshot tokenSlotSnapshot,
	save func(*TokenData) error,
	remove func() error,
) error {
	if snapshot.exists {
		return save(snapshot.token)
	}
	return remove()
}

func restoreTokenMarker(configDir string, marker tokenMarkerSnapshot) error {
	switch {
	case !marker.exists:
		return tokenDeleteMarker(configDir)
	case marker.manual:
		return tokenWriteManualMarker(configDir)
	default:
		return tokenWriteMarker(configDir)
	}
}

// DeleteAllTokenData removes all profile-scoped and legacy token data.
func DeleteAllTokenData(configDir string) error {
	if h := edition.Get(); h.DeleteToken != nil {
		return withProfilesLock(configDir, func() error {
			return h.DeleteToken(configDir)
		})
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
