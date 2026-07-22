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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

var (
	profilesAcquireDualLock   = AcquireDualLock
	profilesReadFile          = os.ReadFile
	profilesRename            = os.Rename
	profilesMkdirAll          = os.MkdirAll
	profilesMarshalIndent     = json.MarshalIndent
	profilesWriteFile         = os.WriteFile
	profilesRemove            = os.Remove
	profilesLoad              = LoadProfiles
	profilesSave              = SaveProfiles
	profilesEnsureMigration   = ensureProfilesMigrationLocked
	profilesSyncLegacyMirror  = syncLegacyTokenMirrorLocked
	profilesTokenExists       = TokenDataExistsKeychain
	profilesLoadLegacy        = LoadTokenDataKeychain
	profilesSaveCorp          = SaveTokenDataKeychainForCorpID
	profilesSaveIdentity      = SaveTokenDataKeychainForIdentity
	profilesTokenExistsCorp   = TokenDataExistsKeychainForCorpID
	profilesLoadCorp          = LoadTokenDataKeychainForCorpID
	profilesDeleteCorp        = DeleteTokenDataKeychainForCorpID
	profilesLoadIdentity      = LoadTokenDataKeychainForIdentity
	profilesSaveLegacy        = SaveTokenDataKeychain
	profilesWriteMarker       = WriteTokenMarker
	profilesWriteManualMarker = WriteManualTokenMarker
	profilesDeleteLegacy      = DeleteTokenDataKeychain
	profilesDeleteMarker      = DeleteTokenMarker
)

// withProfilesLock runs fn while holding the auth dual-layer lock (process +
// cross-process file lock) so that all read-modify-write cycles on
// profiles.json and the legacy token mirror are serialized.
//
// The lock is NOT reentrant. fn must only call the lock-free *Locked variants;
// calling a public (locking) function from within fn would deadlock. Paths that
// already hold the lock (e.g. OAuthProvider.lockedRefresh and the read path
// reached from it) must likewise call the lock-free variants directly.
func withProfilesLock(configDir string, fn func() error) error {
	lock, err := profilesAcquireDualLock(context.Background(), configDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

const (
	profilesJSONFile                = "profiles.json"
	profilesVersion                 = 2
	unresolvedProfileSelectorPrefix = "@legacy/"
)

const (
	ProfileStatusActive      = "active"
	ProfileStatusExpired     = "expired"
	ProfileStatusRevoked     = "revoked"
	ProfileStatusUnavailable = "unavailable"
)

// ProfilesConfig stores non-sensitive profile metadata. Token material stays in keychain.
type ProfilesConfig struct {
	Version            int               `json:"version"`
	PrimaryProfile     string            `json:"primaryProfile,omitempty"`
	CurrentProfile     string            `json:"currentProfile,omitempty"`
	PreviousProfile    string            `json:"previousProfile,omitempty"`
	OrgCurrentProfiles map[string]string `json:"orgCurrentProfiles,omitempty"`
	Profiles           []Profile         `json:"profiles,omitempty"`
}

// Profile is a logged-in DingTalk organization identity.
type Profile struct {
	Name              string   `json:"name"`
	CorpID            string   `json:"corpId"`
	CorpName          string   `json:"corpName,omitempty"`
	UserID            string   `json:"userId,omitempty"`
	UserName          string   `json:"userName,omitempty"`
	ClientID          string   `json:"clientId,omitempty"`
	Status            string   `json:"status,omitempty"`
	AuthorizedDomains []string `json:"authorizedDomains,omitempty"`
	ExpiresAt         string   `json:"expiresAt,omitempty"`
	RefreshExpAt      string   `json:"refreshExpAt,omitempty"`
	LastLoginAt       string   `json:"lastLoginAt,omitempty"`
	LastUsedAt        string   `json:"lastUsedAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
}

var (
	runtimeProfileMu sync.RWMutex
	runtimeProfile   string
)

// SetRuntimeProfile sets a process-local one-shot profile override.
func SetRuntimeProfile(profile string) {
	runtimeProfileMu.Lock()
	defer runtimeProfileMu.Unlock()
	runtimeProfile = strings.TrimSpace(profile)
}

// RuntimeProfile returns the process-local one-shot profile override.
func RuntimeProfile() string {
	runtimeProfileMu.RLock()
	defer runtimeProfileMu.RUnlock()
	return runtimeProfile
}

// ProfilesPath returns the profile metadata path for a config dir.
func ProfilesPath(configDir string) string {
	return filepath.Join(configDir, profilesJSONFile)
}

// LoadProfiles reads profiles.json. A missing file returns an empty config.
func LoadProfiles(configDir string) (*ProfilesConfig, error) {
	path := ProfilesPath(configDir)
	data, err := profilesReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProfilesConfig{Version: 1}, nil
		}
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	var cfg ProfilesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Corrupt file (e.g. an interrupted concurrent write): quarantine it and
		// rebuild an empty config so the CLI can self-heal (auth reset / re-login)
		// instead of being permanently locked out by an unreadable profiles.json.
		quarantine := path + ".corrupt-" + time.Now().Format("20060102-150405.000")
		_ = profilesRename(path, quarantine)
		return &ProfilesConfig{Version: 1}, nil
	}
	normalizeProfilesConfig(&cfg)
	return &cfg, nil
}

// SaveProfiles writes profiles.json atomically.
func SaveProfiles(configDir string, cfg *ProfilesConfig) error {
	if cfg == nil {
		cfg = &ProfilesConfig{}
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return err
	}
	normalizeProfilesConfig(cfg)
	if err := profilesMkdirAll(configDir, config.DirPerm); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := profilesMarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profiles: %w", err)
	}
	data = append(data, '\n')
	path := ProfilesPath(configDir)
	// Per-write random temp name: a fixed "profiles.json.tmp" lets two
	// concurrent writers interleave into the same temp file and rename a
	// corrupted result into place.
	tmp := path + "." + uuid.New().String() + ".tmp"
	if err := profilesWriteFile(tmp, data, config.FilePerm); err != nil {
		return fmt.Errorf("write profiles tmp: %w", err)
	}
	if err := profilesRename(tmp, path); err != nil {
		_ = profilesRemove(tmp)
		return fmt.Errorf("rename profiles: %w", err)
	}
	return nil
}

// EnsureProfilesMigration initializes profiles.json from the legacy auth-token slot when needed.
// EnsureProfilesMigration migrates a legacy single-slot token into the
// profiles registry. It acquires the lock; call ensureProfilesMigrationLocked
// from contexts that already hold it (refresh / read paths).
func EnsureProfilesMigration(configDir string) error {
	return withProfilesLock(configDir, func() error {
		return ensureProfilesMigrationLocked(configDir)
	})
}

func ensureProfilesMigrationLocked(configDir string) error {
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return err
	}
	if cfg.Version > profilesVersion {
		return nil
	}
	if len(cfg.Profiles) == 0 {
		// Version 2 with no profiles is an intentional logged-out tombstone.
		// Never resurrect a stale legacy mirror after logout/reset.
		if cfg.Version >= profilesVersion {
			return nil
		}
		if !profilesTokenExists() {
			return nil
		}
		data, loadErr := profilesLoadLegacy()
		if loadErr != nil || data == nil || strings.TrimSpace(data.CorpID) == "" {
			return nil
		}
		if err := profilesSaveCorp(data.CorpID, data); err != nil {
			return err
		}
		if strings.TrimSpace(data.UserID) != "" {
			if err := profilesSaveIdentity(data.CorpID, data.UserID, data); err != nil {
				return err
			}
		}
		return upsertProfileFromToken(configDir, cfg, data, false)
	}

	legacySelectionState := cfg.Version < profilesVersion
	changed := false
	if cfg.OrgCurrentProfiles == nil {
		cfg.OrgCurrentProfiles = make(map[string]string)
	}
	orgTokens := make(map[string]*TokenData)
	var legacyToken *TokenData
	var legacyTokenErr error
	legacyTokenLoaded := false
	for i := range cfg.Profiles {
		p := &cfg.Profiles[i]
		corpID := strings.TrimSpace(p.CorpID)
		if corpID == "" {
			continue
		}
		orgToken, loaded := orgTokens[corpID]
		if !loaded {
			token, loadErr := profilesLoadCorp(corpID)
			if legacySelectionState && loadErr != nil && !errors.Is(loadErr, ErrTokenDataNotFound) {
				return loadErr
			}
			if loadErr != nil {
				token = nil
			}
			var v2RepairProfile *Profile
			if !legacySelectionState && errors.Is(loadErr, ErrTokenDataNotFound) {
				v2RepairProfile = uniqueV2GlobalRepairProfile(cfg, corpID)
				if v2RepairProfile != nil && strings.TrimSpace(v2RepairProfile.UserID) != "" {
					_, identityErr := profilesLoadIdentity(corpID, v2RepairProfile.UserID)
					switch {
					case identityErr == nil:
						// The exact identity is already usable. Do not let a stale
						// global compatibility mirror recreate the organization slot.
						v2RepairProfile = nil
					case !errors.Is(identityErr, ErrTokenDataNotFound):
						return identityErr
					}
				}
			}
			if (legacySelectionState || v2RepairProfile != nil) && errors.Is(loadErr, ErrTokenDataNotFound) {
				// v1.0.50/1.0.51 installations can retain the selected
				// organization only in the global compatibility slot. Consult
				// that slot while migrating v1, or while repairing a non-empty
				// v2 registry left half-migrated by an earlier CLI. The v2 path
				// additionally requires one unambiguous profile, a missing exact
				// slot when its userId is known, and a non-conflicting token userId.
				// Persist the untouched organization mirror before identity
				// enrichment below.
				if !legacyTokenLoaded {
					legacyToken, legacyTokenErr = profilesLoadLegacy()
					legacyTokenLoaded = true
				}
				if legacyTokenErr != nil && !errors.Is(legacyTokenErr, ErrTokenDataNotFound) {
					return legacyTokenErr
				}
				legacyMatchesProfile := legacySelectionState ||
					legacyTokenMatchesV2RepairProfile(legacyToken, v2RepairProfile)
				if legacyTokenErr == nil &&
					legacyToken != nil &&
					strings.TrimSpace(legacyToken.CorpID) == corpID &&
					legacyMatchesProfile {
					token = legacyToken
					if err := profilesSaveCorp(corpID, token); err != nil {
						return err
					}
				}
			}
			orgToken = token
			orgTokens[corpID] = orgToken
		}
		if orgToken == nil {
			continue
		}
		// v1.0.52 stored one token per organization. Some of those token blobs
		// predate userId persistence even though profiles.json already recorded
		// the account identity. Version 2 loads exact identities and therefore
		// cannot safely use an organization mirror with no userId. A single
		// profile with a known userId makes that association unambiguous,
		// including when an earlier migration already bumped profiles.json to v2
		// but failed before writing the identity slot. Enrich only the copy saved
		// to that exact slot; never infer an identity for an organization with
		// multiple accounts.
		identityToken := orgToken
		if strings.TrimSpace(orgToken.UserID) == "" &&
			strings.TrimSpace(p.UserID) != "" &&
			len(profilesForCorpID(cfg, corpID)) == 1 {
			enriched := *orgToken
			enriched.UserID = strings.TrimSpace(p.UserID)
			if strings.TrimSpace(enriched.UserName) == "" {
				enriched.UserName = strings.TrimSpace(p.UserName)
			}
			identityToken = &enriched
		}
		if strings.TrimSpace(p.UserID) == "" && strings.TrimSpace(orgToken.UserID) != "" {
			if existing := findExactProfile(cfg, corpID, orgToken.UserID); existing != nil && existing != p {
				p.CorpID = ""
				changed = true
				continue
			}
			p.UserID = strings.TrimSpace(orgToken.UserID)
			if p.UserName == "" {
				p.UserName = strings.TrimSpace(orgToken.UserName)
			}
			changed = true
		}
		if strings.TrimSpace(p.UserID) == "" || strings.TrimSpace(identityToken.UserID) != strings.TrimSpace(p.UserID) {
			continue
		}
		_, identityErr := profilesLoadIdentity(corpID, p.UserID)
		if errors.Is(identityErr, ErrTokenDataNotFound) {
			if err := profilesSaveIdentity(corpID, p.UserID, identityToken); err != nil {
				return err
			}
		} else if identityErr != nil {
			return identityErr
		}
	}
	for _, corpID := range uniqueProfileCorpIDs(cfg) {
		if exactProfileSelectorForCorp(cfg, corpID, cfg.OrgCurrentProfiles[corpID]) != "" {
			continue
		}
		if _, exists := cfg.OrgCurrentProfiles[corpID]; exists {
			delete(cfg.OrgCurrentProfiles, corpID)
			changed = true
		}
		if !legacySelectionState {
			continue
		}

		if orgToken := orgTokens[corpID]; orgToken != nil {
			if p := findExactProfile(cfg, corpID, orgToken.UserID); p != nil {
				cfg.OrgCurrentProfiles[corpID] = ProfileSelector(*p)
				changed = true
				continue
			}
		}
		for _, pointer := range []string{cfg.CurrentProfile, cfg.PreviousProfile, cfg.PrimaryProfile} {
			if exact := exactProfileSelectorForCorp(cfg, corpID, pointer); exact != "" {
				cfg.OrgCurrentProfiles[corpID] = exact
				changed = true
				break
			}
		}
		if cfg.OrgCurrentProfiles[corpID] != "" {
			continue
		}
		profiles := profilesForCorpID(cfg, corpID)
		if len(profiles) == 1 {
			cfg.OrgCurrentProfiles[corpID] = ProfileSelector(*profiles[0])
			changed = true
		}
	}
	if exact := canonicalStoredSelector(cfg, cfg.CurrentProfile); exact != "" && exact != cfg.CurrentProfile {
		cfg.CurrentProfile = exact
		changed = true
	}
	if exact := canonicalStoredSelector(cfg, cfg.PrimaryProfile); exact != "" && exact != cfg.PrimaryProfile {
		cfg.PrimaryProfile = exact
		changed = true
	}
	if legacySelectionState && cfg.CurrentProfile == "" {
		if exact := canonicalStoredSelector(cfg, cfg.PrimaryProfile); exact != "" {
			cfg.CurrentProfile = exact
			changed = true
		} else if len(cfg.Profiles) == 1 {
			cfg.CurrentProfile = ProfileSelector(cfg.Profiles[0])
			changed = true
		}
	}
	if exact := canonicalStoredSelector(cfg, cfg.PreviousProfile); exact != "" && exact != cfg.PreviousProfile {
		cfg.PreviousProfile = exact
		changed = true
	}
	if cfg.PreviousProfile != "" && cfg.PreviousProfile == cfg.CurrentProfile {
		cfg.PreviousProfile = ""
		changed = true
	}
	if cfg.Version < profilesVersion {
		cfg.Version = profilesVersion
		changed = true
	}
	if changed {
		return profilesSave(configDir, cfg)
	}
	return nil
}

// uniqueV2GlobalRepairProfile returns the only profile that may safely be
// recovered from the legacy global token mirror. A v2 registry with multiple
// accounts in one organization is deliberately ineligible, even if one token
// happens to carry a matching userId: the global slot is a mutable compatibility
// mirror and is not authoritative account-selection state. A sole unresolved
// profile is eligible only for organization-slot repair; the token matcher below
// rejects any global token that tries to attach a userId to it.
func uniqueV2GlobalRepairProfile(cfg *ProfilesConfig, corpID string) *Profile {
	profiles := profilesForCorpID(cfg, corpID)
	if len(profiles) != 1 {
		return nil
	}
	return profiles[0]
}

func legacyTokenMatchesV2RepairProfile(data *TokenData, profile *Profile) bool {
	if data == nil || strings.TrimSpace(data.CorpID) != strings.TrimSpace(profile.CorpID) {
		return false
	}
	tokenUserID := strings.TrimSpace(data.UserID)
	return tokenUserID == "" || tokenUserID == strings.TrimSpace(profile.UserID)
}

// UpsertProfileFromToken updates profiles.json after a successful login or refresh.
func UpsertProfileFromToken(configDir string, data *TokenData) error {
	return UpsertProfileFromTokenWithCurrent(configDir, data, true)
}

// UpsertProfileFromTokenWithCurrent updates profiles.json and optionally makes
// the token's corp the persistent current profile.
func UpsertProfileFromTokenWithCurrent(configDir string, data *TokenData, makeCurrent bool) error {
	return withProfilesLock(configDir, func() error {
		return upsertProfileFromTokenWithCurrentLocked(configDir, data, makeCurrent)
	})
}

func upsertProfileFromTokenWithCurrentLocked(configDir string, data *TokenData, makeCurrent bool) error {
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return err
	}
	return upsertProfileFromToken(configDir, cfg, data, makeCurrent)
}

func upsertProfileFromToken(configDir string, cfg *ProfilesConfig, data *TokenData, makeCurrent bool) error {
	if data == nil {
		return nil
	}
	corpID := strings.TrimSpace(data.CorpID)
	if corpID == "" {
		return nil
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return err
	}
	normalizeProfilesConfig(cfg)
	cfg.Version = profilesVersion
	now := time.Now().Format(time.RFC3339)
	userID := strings.TrimSpace(data.UserID)
	var previousCurrent *Profile
	if makeCurrent && strings.TrimSpace(cfg.CurrentProfile) != "" {
		previousCurrent, _, _ = resolveProfileSelection(configDir, cfg, cfg.CurrentProfile)
	}
	idx := profileIndexByIdentity(cfg, corpID, userID)
	if idx < 0 && userID != "" && len(profilesForCorpID(cfg, corpID)) == 1 {
		// Upgrade an organization-scoped v1 profile only when it is the sole
		// account in that organization. If exact identities already coexist
		// with a blank profile, consuming the blank profile here would silently
		// discard that unresolved historical account.
		idx = legacyProfileIndexByCorpID(cfg, corpID)
	}
	if idx < 0 {
		profile := Profile{
			Name:        chooseProfileName(cfg, data),
			CorpID:      corpID,
			CorpName:    strings.TrimSpace(data.CorpName),
			UserID:      userID,
			UserName:    strings.TrimSpace(data.UserName),
			ClientID:    strings.TrimSpace(data.ClientID),
			Status:      ProfileStatusActive,
			LastLoginAt: now,
			LastUsedAt:  now,
			UpdatedAt:   now,
		}
		cfg.Profiles = append(cfg.Profiles, profile)
		idx = len(cfg.Profiles) - 1
	} else {
		p := &cfg.Profiles[idx]
		if userID != "" {
			p.UserID = userID
		}
		if shouldRefreshProfileName(p, data) {
			p.Name = chooseProfileName(cfg, data)
		}
		if v := strings.TrimSpace(data.CorpName); v != "" {
			p.CorpName = v
		}
		if v := strings.TrimSpace(data.UserName); v != "" {
			p.UserName = v
		}
		if v := strings.TrimSpace(data.ClientID); v != "" {
			p.ClientID = v
		}
		p.Status = ProfileStatusActive
		p.LastLoginAt = now
		p.LastUsedAt = now
		p.UpdatedAt = now
	}
	storedProfile := &cfg.Profiles[idx]
	if userID == "" && len(profilesForCorpID(cfg, corpID)) > 1 &&
		(strings.TrimSpace(storedProfile.Name) == corpID || profileNameTakenByOtherIdentity(cfg, storedProfile.Name, corpID, "")) {
		// A blank profile needs a stable name when exact identities coexist in
		// the same organization; the corpId selector denotes the organization
		// as a whole and is therefore not an exact account selector.
		storedProfile.Name = chooseProfileName(cfg, data)
	}
	if makeCurrent {
		newSelector := storedProfileSelector(cfg, storedProfile)
		if previousCurrent != nil && storedProfileSelector(cfg, previousCurrent) != newSelector {
			cfg.PreviousProfile = storedProfileSelector(cfg, previousCurrent)
		}
		cfg.CurrentProfile = newSelector
		if userID == "" {
			delete(cfg.OrgCurrentProfiles, corpID)
		} else {
			setOrgCurrentProfile(cfg, corpID, newSelector)
		}
	}
	if cfg.CurrentProfile == "" {
		cfg.CurrentProfile = storedProfileSelector(cfg, storedProfile)
		if userID == "" {
			delete(cfg.OrgCurrentProfiles, corpID)
		} else {
			setOrgCurrentProfile(cfg, corpID, cfg.CurrentProfile)
		}
	}
	return profilesSave(configDir, cfg)
}

// ProfileSelector returns the exact identity selector for a profile when its
// userId is known, otherwise it returns the historical corpId selector.
func ProfileSelector(profile Profile) string {
	return profileSelector(profile.CorpID, profile.UserID)
}

// storedProfileSelector returns a selector that remains exact inside
// profiles.json. A blank userId has only an organization selector in the
// public compatibility surface; when other accounts share that organization,
// use the profile's unique local name so current/previous pointers do not
// accidentally resolve to an exact account through OrgCurrentProfiles.
func storedProfileSelector(cfg *ProfilesConfig, profile *Profile) string {
	if profile == nil {
		return ""
	}
	if strings.TrimSpace(profile.UserID) != "" {
		return ProfileSelector(*profile)
	}
	if cfg == nil {
		return strings.TrimSpace(profile.CorpID)
	}
	if len(profilesForCorpID(cfg, profile.CorpID)) <= 1 {
		return strings.TrimSpace(profile.CorpID)
	}
	name := strings.TrimSpace(profile.Name)
	if localProfileSelectorIsSafe(cfg, profile, name) {
		return name
	}
	return unresolvedProfileSelector(profile.CorpID)
}

// ProfileSelectionSelector returns the stable selector used for one profile.
// Exact identities use corpId:userId. A historical profile without userId
// keeps the organization selector while it is the only account, and otherwise
// uses either an unambiguous local name or a reserved, reversible selector.
func ProfileSelectionSelector(profile Profile, cfg *ProfilesConfig) string {
	return storedProfileSelector(cfg, &profile)
}

func localProfileSelectorIsSafe(cfg *ProfilesConfig, profile *Profile, name string) bool {
	if cfg == nil || profile == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, ":") || strings.HasPrefix(name, unresolvedProfileSelectorPrefix) {
		return false
	}
	nameMatches := 0
	for i := range cfg.Profiles {
		candidate := &cfg.Profiles[i]
		if strings.TrimSpace(candidate.Name) == name {
			nameMatches++
		}
		// Organization selectors are resolved before ordinary local names.
		// Never persist a local selector that can be captured by that grammar.
		if strings.TrimSpace(candidate.CorpID) == name || strings.TrimSpace(candidate.CorpName) == name {
			return false
		}
	}
	return nameMatches == 1
}

func unresolvedProfileSelector(corpID string) string {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return ""
	}
	return unresolvedProfileSelectorPrefix + base64.RawURLEncoding.EncodeToString([]byte(corpID))
}

func parseUnresolvedProfileSelector(selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if !strings.HasPrefix(selector, unresolvedProfileSelectorPrefix) {
		return "", false
	}
	encoded := strings.TrimPrefix(selector, unresolvedProfileSelectorPrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	corpID := strings.TrimSpace(string(decoded))
	if corpID == "" || unresolvedProfileSelector(corpID) != selector {
		return "", false
	}
	return corpID, true
}

// TokenProfileSelector returns the exact identity selector for token data when
// its userId is known, otherwise it returns the historical corpId selector.
func TokenProfileSelector(data *TokenData) string {
	if data == nil {
		return ""
	}
	return profileSelector(data.CorpID, data.UserID)
}

// StableTokenProfileSelector preserves the exact selector that loaded a token.
// This matters for an unresolved historical account sharing an organization
// with exact identities: reducing its selector to corpId would follow
// OrgCurrentProfiles and could mark or reauthorize a different account.
func StableTokenProfileSelector(configDir string, data *TokenData) string {
	if selector := strings.TrimSpace(RuntimeProfile()); selector != "" {
		return selector
	}
	fallback := TokenProfileSelector(data)
	if data == nil {
		return fallback
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil || cfg == nil || strings.TrimSpace(cfg.CurrentProfile) == "" {
		return fallback
	}
	selector := canonicalStoredSelector(cfg, cfg.CurrentProfile)
	if selector == "" {
		selector = strings.TrimSpace(cfg.CurrentProfile)
	}
	profile, _, err := resolveProfileSelection(configDir, cfg, selector)
	if err != nil || profile == nil ||
		!sameProfileIdentity(profile.CorpID, profile.UserID, data.CorpID, data.UserID) {
		return fallback
	}
	return storedProfileSelector(cfg, profile)
}

func profileSelector(corpID, userID string) string {
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	if corpID == "" {
		return ""
	}
	if userID == "" {
		return corpID
	}
	return corpID + ":" + userID
}

// ParseIdentitySelector splits corpId:userId selectors.
func ParseIdentitySelector(selector string) (corpID, userID string, ok bool) {
	selector = strings.TrimSpace(selector)
	idx := strings.Index(selector, ":")
	if idx <= 0 || idx >= len(selector)-1 {
		return "", "", false
	}
	corpID = strings.TrimSpace(selector[:idx])
	userID = strings.TrimSpace(selector[idx+1:])
	if corpID == "" || userID == "" {
		return "", "", false
	}
	return corpID, userID, true
}

// ResolveProfile returns a profile selected by name/corpId/identity or by
// current fallback.
func ResolveProfile(configDir, selector string) (*Profile, error) {
	p, _, err := ResolveProfileWithScope(configDir, selector)
	return p, err
}

// ResolveProfileWithScope resolves a selector and reports whether it targets
// one identity (compound selector or local profile name) rather than an
// organization as a whole.
func ResolveProfileWithScope(configDir, selector string) (*Profile, bool, error) {
	var (
		result *Profile
		exact  bool
	)
	err := withProfilesLock(configDir, func() error {
		var resolveErr error
		result, exact, resolveErr = resolveProfileWithScopeLocked(configDir, selector)
		return resolveErr
	})
	return result, exact, err
}

func resolveProfileWithScopeLocked(configDir, selector string) (*Profile, bool, error) {
	if err := profilesEnsureMigration(configDir); err != nil {
		return nil, false, err
	}
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, false, err
	}
	selector = strings.TrimSpace(selector)
	if selector != "" {
		return resolveProfileSelection(configDir, cfg, selector)
	}
	if strings.TrimSpace(cfg.CurrentProfile) != "" {
		return resolveProfileSelection(configDir, cfg, cfg.CurrentProfile)
	}
	return nil, false, nil
}

// ResolveProfileDeletionScope resolves selectors for destructive profile
// removal. Organization selectors intentionally resolve to the whole
// organization even when it has multiple accounts and no current account.
func ResolveProfileDeletionScope(configDir, selector string) (*Profile, bool, error) {
	var (
		result *Profile
		exact  bool
	)
	err := withProfilesLock(configDir, func() error {
		if err := profilesEnsureMigration(configDir); err != nil {
			return err
		}
		cfg, err := profilesLoad(configDir)
		if err != nil {
			return err
		}
		if err := ensureProfilesWritable(cfg); err != nil {
			return err
		}
		var resolveErr error
		result, exact, resolveErr = resolveProfileDeletionSelection(cfg, selector)
		return resolveErr
	})
	return result, exact, err
}

func resolveProfileForLoadLocked(configDir, selector string) (*Profile, *ProfilesConfig, error) {
	if err := profilesEnsureMigration(configDir); err != nil {
		return nil, nil, err
	}
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, nil, err
	}
	selector = strings.TrimSpace(selector)
	if selector != "" {
		p, _, resolveErr := resolveProfileSelection(configDir, cfg, selector)
		return p, cfg, resolveErr
	}
	if strings.TrimSpace(cfg.CurrentProfile) != "" {
		p, _, resolveErr := resolveProfileSelection(configDir, cfg, cfg.CurrentProfile)
		return p, cfg, resolveErr
	}
	return nil, cfg, nil
}

// resolveProfileForLoad is retained as a focused test seam around the locked
// resolver. Production callers already hold the auth lock.
func resolveProfileForLoad(configDir, selector string) (*Profile, error) {
	profile, _, err := resolveProfileForLoadLocked(configDir, selector)
	return profile, err
}

// SetCurrentProfile persists the selected current profile.
func SetCurrentProfile(configDir, selector string) (*Profile, error) {
	var result *Profile
	err := withProfilesLock(configDir, func() error {
		p, e := setCurrentProfileLocked(configDir, selector)
		result = p
		return e
	})
	return result, err
}

func setCurrentProfileLocked(configDir, selector string) (*Profile, error) {
	if err := profilesEnsureMigration(configDir); err != nil {
		return nil, err
	}
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, err
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return nil, err
	}
	p, _, err := resolveProfileSelection(configDir, cfg, selector)
	if err != nil {
		return nil, err
	}
	originalCfg := cloneProfilesConfig(cfg)
	syncOrganization := shouldSyncOrganizationMirror(cfg, *p)
	mirrors, err := snapshotProfileSelectionMirrors(configDir, p.CorpID, syncOrganization)
	if err != nil {
		return nil, err
	}
	var previousCurrent *Profile
	if strings.TrimSpace(cfg.CurrentProfile) != "" {
		previousCurrent, _, _ = resolveProfileSelection(configDir, cfg, cfg.CurrentProfile)
	}
	storedSelector := storedProfileSelector(cfg, p)
	if cfg.CurrentProfile != storedSelector {
		if previousCurrent != nil {
			cfg.PreviousProfile = storedProfileSelector(cfg, previousCurrent)
		}
		cfg.CurrentProfile = storedSelector
	}
	if strings.TrimSpace(p.UserID) == "" {
		delete(cfg.OrgCurrentProfiles, strings.TrimSpace(p.CorpID))
	} else {
		setOrgCurrentProfile(cfg, p.CorpID, storedSelector)
	}
	touchProfileUsage(p)
	if err := profilesSave(configDir, cfg); err != nil {
		return nil, err
	}
	if syncOrganization {
		if err := syncOrganizationTokenMirrorForProfile(*p); err != nil {
			return nil, rollbackProfileSelection(configDir, originalCfg, p.CorpID, mirrors, err)
		}
	}
	if err := profilesSyncLegacyMirror(configDir); err != nil {
		return nil, rollbackProfileSelection(configDir, originalCfg, p.CorpID, mirrors, err)
	}
	return findExactProfile(cfg, p.CorpID, p.UserID), nil
}

// UsePreviousProfile toggles currentProfile and previousProfile.
func UsePreviousProfile(configDir string) (*Profile, error) {
	var result *Profile
	err := withProfilesLock(configDir, func() error {
		p, e := usePreviousProfileLocked(configDir)
		result = p
		return e
	})
	return result, err
}

func usePreviousProfileLocked(configDir string) (*Profile, error) {
	if err := profilesEnsureMigration(configDir); err != nil {
		return nil, err
	}
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, err
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return nil, err
	}
	prev := strings.TrimSpace(cfg.PreviousProfile)
	if prev == "" {
		return nil, fmt.Errorf("previous profile is empty")
	}
	p, _, err := resolveProfileSelection(configDir, cfg, prev)
	if err != nil {
		return nil, fmt.Errorf("resolve previous profile %q: %w", prev, err)
	}
	originalCfg := cloneProfilesConfig(cfg)
	syncOrganization := shouldSyncOrganizationMirror(cfg, *p)
	mirrors, err := snapshotProfileSelectionMirrors(configDir, p.CorpID, syncOrganization)
	if err != nil {
		return nil, err
	}
	var current *Profile
	if strings.TrimSpace(cfg.CurrentProfile) != "" {
		current, _, _ = resolveProfileSelection(configDir, cfg, cfg.CurrentProfile)
	}
	cfg.CurrentProfile = storedProfileSelector(cfg, p)
	if current != nil {
		cfg.PreviousProfile = storedProfileSelector(cfg, current)
	} else {
		cfg.PreviousProfile = ""
	}
	if strings.TrimSpace(p.UserID) == "" {
		delete(cfg.OrgCurrentProfiles, strings.TrimSpace(p.CorpID))
	} else {
		setOrgCurrentProfile(cfg, p.CorpID, ProfileSelector(*p))
	}
	touchProfileUsage(p)
	if err := profilesSave(configDir, cfg); err != nil {
		return nil, err
	}
	if syncOrganization {
		if err := syncOrganizationTokenMirrorForProfile(*p); err != nil {
			return nil, rollbackProfileSelection(configDir, originalCfg, p.CorpID, mirrors, err)
		}
	}
	if err := profilesSyncLegacyMirror(configDir); err != nil {
		return nil, rollbackProfileSelection(configDir, originalCfg, p.CorpID, mirrors, err)
	}
	return findExactProfile(cfg, p.CorpID, p.UserID), nil
}

func touchProfileUsage(profile *Profile) {
	if profile == nil {
		return
	}
	now := time.Now().Format(time.RFC3339)
	profile.LastUsedAt = now
	profile.UpdatedAt = now
}

// RemoveProfile removes a profile from metadata and returns the removed profile.
func RemoveProfile(configDir, selector string) (*Profile, error) {
	var result *Profile
	err := withProfilesLock(configDir, func() error {
		if err := ensureProfilesMigrationLocked(configDir); err != nil {
			return err
		}
		p, e := removeProfileLocked(configDir, selector)
		result = p
		return e
	})
	return result, err
}

func removeProfileLocked(configDir, selector string) (*Profile, error) {
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, err
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return nil, err
	}
	p, exact, err := resolveProfileDeletionSelection(cfg, selector)
	if err != nil {
		return nil, err
	}
	removed := *p
	originalCurrentSelector := strings.TrimSpace(cfg.CurrentProfile)
	originalOrganizationCurrent := strings.TrimSpace(cfg.OrgCurrentProfiles[strings.TrimSpace(removed.CorpID)])
	pointers := []*string{&cfg.PrimaryProfile, &cfg.CurrentProfile, &cfg.PreviousProfile}
	pointerMatches := make([]bool, len(pointers))
	for i, pointer := range pointers {
		selected, _, resolveErr := resolveProfileSelection(configDir, cfg, *pointer)
		if resolveErr == nil && selected != nil {
			if exact {
				pointerMatches[i] =
					sameProfileIdentity(selected.CorpID, selected.UserID, removed.CorpID, removed.UserID)
			} else {
				pointerMatches[i] = strings.TrimSpace(selected.CorpID) == strings.TrimSpace(removed.CorpID)
			}
		}
	}
	kept := cfg.Profiles[:0]
	for _, profile := range cfg.Profiles {
		remove := profile.CorpID == removed.CorpID
		if exact {
			remove = sameProfileIdentity(profile.CorpID, profile.UserID, removed.CorpID, removed.UserID)
		}
		if !remove {
			kept = append(kept, profile)
		}
	}
	cfg.Profiles = kept
	replacementSelector := ""
	if exact && len(profilesForCorpID(cfg, removed.CorpID)) > 0 {
		remaining := profilesForCorpID(cfg, removed.CorpID)
		if len(remaining) == 1 {
			replacementSelector = storedProfileSelector(cfg, remaining[0])
		}
	}
	if exact {
		if selectorMatchesIdentity(cfg.OrgCurrentProfiles[removed.CorpID], removed) {
			if replacementSelector == "" {
				delete(cfg.OrgCurrentProfiles, removed.CorpID)
			} else {
				cfg.OrgCurrentProfiles[removed.CorpID] = replacementSelector
			}
		}
	} else {
		delete(cfg.OrgCurrentProfiles, removed.CorpID)
	}
	for i, pointer := range pointers {
		if exact {
			if pointerMatches[i] {
				*pointer = replacementSelector
			}
			continue
		}
		if pointerMatches[i] || selectorTargetsCorp(*pointer, removed.CorpID) {
			*pointer = ""
		}
	}
	if cfg.CurrentProfile == "" {
		if previous := canonicalStoredSelector(cfg, cfg.PreviousProfile); previous != "" {
			cfg.CurrentProfile = previous
			cfg.PreviousProfile = ""
		} else if len(cfg.Profiles) == 1 {
			cfg.CurrentProfile = storedProfileSelector(cfg, &cfg.Profiles[0])
		}
	}
	if cfg.PreviousProfile != "" && cfg.PreviousProfile == cfg.CurrentProfile {
		cfg.PreviousProfile = ""
	}
	currentSelectionChanged := strings.TrimSpace(cfg.CurrentProfile) != originalCurrentSelector
	organizationCurrentChanged := strings.TrimSpace(cfg.OrgCurrentProfiles[strings.TrimSpace(removed.CorpID)]) != originalOrganizationCurrent
	if strings.TrimSpace(cfg.CurrentProfile) != "" {
		if current, _, resolveErr := resolveProfileSelection(configDir, cfg, cfg.CurrentProfile); resolveErr == nil && current != nil {
			if (currentSelectionChanged || organizationCurrentChanged) &&
				strings.TrimSpace(current.CorpID) == strings.TrimSpace(removed.CorpID) &&
				unresolvedProfileForCorp(cfg, removed.CorpID) != nil {
				if strings.TrimSpace(current.UserID) == "" {
					delete(cfg.OrgCurrentProfiles, strings.TrimSpace(current.CorpID))
				} else {
					setOrgCurrentProfile(cfg, current.CorpID, storedProfileSelector(cfg, current))
				}
			}
		}
	}
	if len(cfg.Profiles) == 0 {
		cfg.PrimaryProfile = ""
		cfg.CurrentProfile = ""
		cfg.PreviousProfile = ""
		cfg.OrgCurrentProfiles = nil
	}
	if err := profilesSave(configDir, cfg); err != nil {
		return nil, err
	}
	return &removed, nil
}

func selectorMatchesIdentity(selector string, profile Profile) bool {
	corpID, userID, exact := ParseIdentitySelector(selector)
	return exact && sameProfileIdentity(corpID, userID, profile.CorpID, profile.UserID)
}

func selectorTargetsCorp(selector, corpID string) bool {
	selector = strings.TrimSpace(selector)
	corpID = strings.TrimSpace(corpID)
	if selector == corpID {
		return true
	}
	if selectedCorpID, unresolved := parseUnresolvedProfileSelector(selector); unresolved {
		return selectedCorpID == corpID
	}
	selectedCorpID, _, exact := ParseIdentitySelector(selector)
	return exact && selectedCorpID == corpID
}

// MarkProfileStatus updates a selected profile status if it exists.
func MarkProfileStatus(configDir, selector, status string) error {
	if strings.TrimSpace(selector) == "" {
		return nil
	}
	return withProfilesLock(configDir, func() error {
		return markProfileStatusLocked(configDir, selector, status)
	})
}

func markProfileStatusLocked(configDir, selector, status string) error {
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return err
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return err
	}
	p, _, resolveErr := resolveProfileSelection(configDir, cfg, selector)
	if resolveErr != nil {
		return nil
	}
	p.Status = strings.TrimSpace(status)
	p.UpdatedAt = time.Now().Format(time.RFC3339)
	return profilesSave(configDir, cfg)
}

func ensureProfilesWritable(cfg *ProfilesConfig) error {
	if cfg != nil && cfg.Version > profilesVersion {
		return fmt.Errorf(
			"profiles.json version %d is newer than supported version %d; upgrade dws before changing profiles",
			cfg.Version,
			profilesVersion,
		)
	}
	return nil
}

type profileSelectionMirrorSnapshot struct {
	organization tokenSlotSnapshot
	legacy       tokenSlotSnapshot
	marker       tokenMarkerSnapshot
}

func snapshotProfileSelectionMirrors(configDir, corpID string, includeOrganization bool) (profileSelectionMirrorSnapshot, error) {
	var organization tokenSlotSnapshot
	if includeOrganization {
		var err error
		organization, err = snapshotTokenSlot(func() (*TokenData, error) {
			return profilesLoadCorp(corpID)
		})
		if err != nil {
			return profileSelectionMirrorSnapshot{}, err
		}
	}
	legacy, err := snapshotTokenSlot(profilesLoadLegacy)
	if err != nil {
		return profileSelectionMirrorSnapshot{}, err
	}
	marker, err := snapshotTokenMarker(configDir)
	if err != nil {
		return profileSelectionMirrorSnapshot{}, err
	}
	return profileSelectionMirrorSnapshot{
		organization: organization,
		legacy:       legacy,
		marker:       marker,
	}, nil
}

func rollbackProfileSelection(
	configDir string,
	cfg *ProfilesConfig,
	corpID string,
	mirrors profileSelectionMirrorSnapshot,
	operationErr error,
) error {
	var rollbackErr error
	if err := profilesSave(configDir, cloneProfilesConfig(cfg)); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if mirrors.organization.known {
		if mirrors.organization.exists {
			if err := profilesSaveCorp(corpID, mirrors.organization.token); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		} else if err := profilesDeleteCorp(corpID); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if mirrors.legacy.exists {
		if err := profilesSaveLegacy(mirrors.legacy.token); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	} else if err := profilesDeleteLegacy(); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	switch {
	case !mirrors.marker.exists:
		if err := profilesDeleteMarker(configDir); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	case mirrors.marker.manual:
		if err := profilesWriteManualMarker(configDir); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	default:
		if err := profilesWriteMarker(configDir); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if rollbackErr != nil {
		return errors.Join(operationErr, fmt.Errorf("rollback profile selection: %w", rollbackErr))
	}
	return operationErr
}

// SyncLegacyTokenMirror mirrors the current profile token into legacy auth-token.
func SyncLegacyTokenMirror(configDir string) error {
	return withProfilesLock(configDir, func() error {
		return syncLegacyTokenMirrorLocked(configDir)
	})
}

func syncLegacyTokenMirrorLocked(configDir string) error {
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return err
	}
	current := strings.TrimSpace(cfg.CurrentProfile)
	if current != "" {
		p, _, resolveErr := resolveProfileSelection(configDir, cfg, current)
		if resolveErr != nil {
			return resolveErr
		}
		data, loadErr := loadTokenForProfileIdentity(*p)
		if loadErr != nil {
			if errors.Is(loadErr, ErrTokenDataNotFound) {
				// The current identity and its organization mirror are both
				// confirmed absent. Clear the stale global compatibility mirror.
			} else {
				// Keep the existing legacy mirror untouched rather than wiping a host
				// app's login state just because keychain was momentarily unavailable.
				return nil
			}
		}
		if data != nil {
			if err := profilesSaveLegacy(data); err != nil {
				return err
			}
			return profilesWriteMarker(configDir)
		}
	}
	// No current profile (or its token is confirmed absent): clear the mirror.
	if err := profilesDeleteLegacy(); err != nil {
		return err
	}
	return profilesDeleteMarker(configDir)
}

func syncOrganizationTokenMirrorForProfile(profile Profile) error {
	data, err := loadTokenForProfileIdentity(profile)
	if err != nil {
		return err
	}
	return profilesSaveCorp(profile.CorpID, data)
}

func loadTokenForProfileIdentity(profile Profile) (*TokenData, error) {
	if strings.TrimSpace(profile.UserID) == "" {
		data, err := profilesLoadCorp(profile.CorpID)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, ErrTokenDataNotFound
		}
		if strings.TrimSpace(data.UserID) != "" {
			return nil, fmt.Errorf(
				"organization token mirror for corpId %q belongs to userId %q; cannot use it for unresolved profile %q",
				profile.CorpID,
				data.UserID,
				profile.Name,
			)
		}
		return data, nil
	}
	data, err := profilesLoadIdentity(profile.CorpID, profile.UserID)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, ErrTokenDataNotFound) {
		return nil, err
	}
	orgData, orgErr := profilesLoadCorp(profile.CorpID)
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
	if saveErr := profilesSaveIdentity(profile.CorpID, profile.UserID, orgData); saveErr != nil {
		return nil, saveErr
	}
	return orgData, nil
}

func normalizeProfilesConfig(cfg *ProfilesConfig) {
	if cfg == nil {
		return
	}
	if cfg.Version <= 0 {
		cfg.Version = 1
	}
	seen := make(map[string]bool, len(cfg.Profiles))
	profiles := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		p.CorpID = strings.TrimSpace(p.CorpID)
		p.UserID = strings.TrimSpace(p.UserID)
		identity := profileIdentityKey(p.CorpID, p.UserID)
		if p.CorpID == "" || seen[identity] {
			continue
		}
		seen[identity] = true
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			p.Name = p.CorpID
		}
		if corpName := strings.TrimSpace(p.CorpName); p.Name == p.CorpID && corpName != "" && !profileNameTakenByOtherIdentity(cfg, corpName, p.CorpID, p.UserID) {
			p.Name = corpName
		}
		if p.Status == "" {
			p.Status = ProfileStatusActive
		}
		profiles = append(profiles, p)
	}
	cfg.Profiles = profiles
	if len(cfg.OrgCurrentProfiles) > 0 {
		normalized := make(map[string]string, len(cfg.OrgCurrentProfiles))
		for corpID, selector := range cfg.OrgCurrentProfiles {
			corpID = strings.TrimSpace(corpID)
			if exact := exactProfileSelectorForCorp(cfg, corpID, selector); exact != "" {
				normalized[corpID] = exact
			}
		}
		if len(normalized) == 0 {
			cfg.OrgCurrentProfiles = nil
		} else {
			cfg.OrgCurrentProfiles = normalized
		}
	}
	if cfg.PrimaryProfile != "" && !profileSelectorReferenceExists(cfg, cfg.PrimaryProfile) {
		cfg.PrimaryProfile = ""
	}
	if cfg.CurrentProfile != "" && !profileSelectorReferenceExists(cfg, cfg.CurrentProfile) {
		cfg.CurrentProfile = ""
	}
	if cfg.PreviousProfile != "" && !profileSelectorReferenceExists(cfg, cfg.PreviousProfile) {
		cfg.PreviousProfile = ""
	}
}

func chooseProfileName(cfg *ProfilesConfig, data *TokenData) string {
	base := strings.TrimSpace(data.CorpName)
	if base == "" {
		base = strings.TrimSpace(data.CorpID)
	}
	if base == "" {
		base = "profile"
	}
	corpID := strings.TrimSpace(data.CorpID)
	userID := strings.TrimSpace(data.UserID)
	if !profileNameTakenByOtherIdentity(cfg, base, corpID, userID) {
		return base
	}
	for _, suffix := range []string{
		strings.TrimSpace(data.UserName),
		shortProfileID(userID),
		shortCorpID(corpID),
	} {
		if suffix == "" {
			continue
		}
		name := base + "-" + suffix
		if !profileNameTakenByOtherIdentity(cfg, name, corpID, userID) {
			return name
		}
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !profileNameTakenByOtherIdentity(cfg, candidate, corpID, userID) {
			return candidate
		}
	}
}

func shouldRefreshProfileName(p *Profile, data *TokenData) bool {
	if p == nil || data == nil {
		return false
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return true
	}
	return strings.TrimSpace(data.CorpName) != "" && name == strings.TrimSpace(p.CorpID)
}

func profileNameTakenByOtherIdentity(cfg *ProfilesConfig, name, corpID, userID string) bool {
	name = strings.TrimSpace(name)
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	for _, p := range cfg.Profiles {
		if strings.TrimSpace(p.Name) == name && !sameProfileIdentity(p.CorpID, p.UserID, corpID, userID) {
			return true
		}
	}
	return false
}

// resolveProfileSelection resolves a user-facing selector to one exact identity.
// The bool reports whether the selector targets one identity (compound selector
// or local profile name) rather than an organization as a whole.
func resolveProfileSelection(_ string, cfg *ProfilesConfig, selector string) (*Profile, bool, error) {
	if cfg == nil {
		return nil, false, fmt.Errorf("profile %q not found", strings.TrimSpace(selector))
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, false, fmt.Errorf("profile selector is empty")
	}
	if corpID, unresolved := parseUnresolvedProfileSelector(selector); unresolved {
		if profile := unresolvedProfileForCorp(cfg, corpID); profile != nil {
			return profile, true, nil
		}
		return nil, true, fmt.Errorf("historical profile for organization %q not found", corpID)
	}

	if organization, account, compound := ParseIdentitySelector(selector); compound {
		corpID, err := resolveOrganizationCorpID(cfg, organization)
		if err != nil {
			return nil, true, err
		}
		if corpID == "" {
			return nil, true, fmt.Errorf("organization %q not found", organization)
		}
		if p := findExactProfile(cfg, corpID, account); p != nil {
			return p, true, nil
		}
		var matches []*Profile
		for _, p := range profilesForCorpID(cfg, corpID) {
			if strings.TrimSpace(p.UserName) == account {
				matches = append(matches, p)
			}
		}
		switch len(matches) {
		case 1:
			return matches[0], true, nil
		case 0:
			return nil, true, fmt.Errorf("account %q not found in organization %q", account, organization)
		default:
			return nil, true, fmt.Errorf(
				"account name %q is ambiguous in organization %q; use one of: %s",
				account,
				organization,
				strings.Join(profileSelectorCandidates(matches), ", "),
			)
		}
	}

	if profiles := profilesForCorpID(cfg, selector); len(profiles) > 0 {
		return resolveOrganizationDefault(cfg, selector, selector, profiles)
	}

	orgIDs := make(map[string]struct{})
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpName) != selector {
			continue
		}
		orgIDs[strings.TrimSpace(cfg.Profiles[i].CorpID)] = struct{}{}
	}
	if len(orgIDs) == 1 {
		for corpID := range orgIDs {
			return resolveOrganizationDefault(cfg, corpID, selector, profilesForCorpID(cfg, corpID))
		}
	}
	if len(orgIDs) > 1 {
		candidates := make([]string, 0, len(orgIDs))
		for corpID := range orgIDs {
			candidates = append(candidates, corpID)
		}
		sort.Strings(candidates)
		return nil, false, fmt.Errorf(
			"organization name %q is ambiguous; use one of: %s",
			selector,
			strings.Join(candidates, ", "),
		)
	}

	var nameMatches []*Profile
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].Name) != selector {
			continue
		}
		nameMatches = append(nameMatches, &cfg.Profiles[i])
	}
	switch len(nameMatches) {
	case 1:
		return nameMatches[0], true, nil
	case 0:
		return nil, false, fmt.Errorf("profile %q not found", selector)
	default:
		return nil, true, fmt.Errorf(
			"profile name %q is ambiguous; use one of: %s",
			selector,
			strings.Join(profileSelectorCandidates(nameMatches), ", "),
		)
	}
}

func resolveProfileDeletionSelection(cfg *ProfilesConfig, selector string) (*Profile, bool, error) {
	if cfg == nil {
		return nil, false, fmt.Errorf("profile %q not found", strings.TrimSpace(selector))
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, false, fmt.Errorf("profile selector is empty")
	}
	if corpID, unresolved := parseUnresolvedProfileSelector(selector); unresolved {
		if profile := unresolvedProfileForCorp(cfg, corpID); profile != nil {
			return profile, true, nil
		}
		return nil, true, fmt.Errorf("historical profile for organization %q not found", corpID)
	}
	if _, _, compound := ParseIdentitySelector(selector); compound {
		return resolveProfileSelection("", cfg, selector)
	}
	if profiles := profilesForCorpID(cfg, selector); len(profiles) > 0 {
		return profiles[0], false, nil
	}
	corpID, err := resolveOrganizationCorpID(cfg, selector)
	if err != nil {
		return nil, false, err
	}
	if corpID != "" {
		profiles := profilesForCorpID(cfg, corpID)
		if len(profiles) > 0 {
			return profiles[0], false, nil
		}
	}

	var nameMatches []*Profile
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].Name) == selector {
			nameMatches = append(nameMatches, &cfg.Profiles[i])
		}
	}
	switch len(nameMatches) {
	case 1:
		return nameMatches[0], true, nil
	case 0:
		return nil, false, fmt.Errorf("profile %q not found", selector)
	default:
		return nil, true, fmt.Errorf(
			"profile name %q is ambiguous; use one of: %s",
			selector,
			strings.Join(profileSelectorCandidates(nameMatches), ", "),
		)
	}
}

func resolveOrganizationCorpID(cfg *ProfilesConfig, selector string) (string, error) {
	if cfg == nil {
		return "", nil
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", nil
	}
	if len(profilesForCorpID(cfg, selector)) > 0 {
		return selector, nil
	}
	orgIDs := make(map[string]struct{})
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpName) != selector {
			continue
		}
		orgIDs[strings.TrimSpace(cfg.Profiles[i].CorpID)] = struct{}{}
	}
	if len(orgIDs) == 0 {
		return "", nil
	}
	if len(orgIDs) == 1 {
		for corpID := range orgIDs {
			return corpID, nil
		}
	}
	candidates := make([]string, 0, len(orgIDs))
	for corpID := range orgIDs {
		candidates = append(candidates, corpID)
	}
	sort.Strings(candidates)
	return "", fmt.Errorf(
		"organization name %q is ambiguous; use one of: %s",
		selector,
		strings.Join(candidates, ", "),
	)
}

func resolveOrganizationDefault(cfg *ProfilesConfig, corpID, displaySelector string, profiles []*Profile) (*Profile, bool, error) {
	if len(profiles) == 0 {
		return nil, false, fmt.Errorf("organization %q not found", displaySelector)
	}
	if exact := exactProfileSelectorForCorp(cfg, corpID, cfg.OrgCurrentProfiles[corpID]); exact != "" {
		selectedCorpID, userID, _ := ParseIdentitySelector(exact)
		if p := findExactProfile(cfg, selectedCorpID, userID); p != nil {
			return p, false, nil
		}
	}
	if unresolved := unresolvedProfileForCorp(cfg, corpID); unresolved != nil {
		// With no exact organization-current selection, the organization slot
		// belongs to the sole unresolved historical account. Do not choose an
		// arbitrary exact identity merely because it shares the corpId.
		return unresolved, false, nil
	}
	if len(profiles) == 1 {
		return profiles[0], false, nil
	}
	return nil, false, fmt.Errorf(
		"organization %q has multiple accounts and no current account; use one of: %s",
		displaySelector,
		strings.Join(profileSelectorCandidates(profiles), ", "),
	)
}

func profileSelectorCandidates(profiles []*Profile) []string {
	cfg := &ProfilesConfig{Profiles: make([]Profile, 0, len(profiles))}
	for _, profile := range profiles {
		if profile != nil {
			cfg.Profiles = append(cfg.Profiles, *profile)
		}
	}
	candidates := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if p == nil {
			continue
		}
		candidates = append(candidates, storedProfileSelector(cfg, p))
	}
	sort.Strings(candidates)
	return candidates
}

func exactProfileSelectorForCorp(cfg *ProfilesConfig, corpID, selector string) string {
	selectedCorpID, userID, exact := ParseIdentitySelector(selector)
	if !exact || strings.TrimSpace(selectedCorpID) != strings.TrimSpace(corpID) {
		return ""
	}
	if p := findExactProfile(cfg, selectedCorpID, userID); p != nil {
		return ProfileSelector(*p)
	}
	return ""
}

func canonicalStoredSelector(cfg *ProfilesConfig, selector string) string {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return ""
	}
	if corpID, unresolved := parseUnresolvedProfileSelector(selector); unresolved {
		if profile := unresolvedProfileForCorp(cfg, corpID); profile != nil {
			return storedProfileSelector(cfg, profile)
		}
		return ""
	}
	if corpID, userID, exact := ParseIdentitySelector(selector); exact {
		if p := findExactProfile(cfg, corpID, userID); p != nil {
			return ProfileSelector(*p)
		}
		// A colon-containing legacy local name is recoverable only when it does
		// not name a real exact identity. Exact corpId:userId always wins.
		if profile := unresolvedProfileForLocalName(cfg, selector); profile != nil {
			return storedProfileSelector(cfg, profile)
		}
		return ""
	}
	// Older multi-account writers stored the unresolved profile's local name.
	// Recover that intent before the organization grammar can capture a
	// CorpName-like name, then persist the safe selector.
	if profile := unresolvedProfileForLocalName(cfg, selector); profile != nil {
		return storedProfileSelector(cfg, profile)
	}
	if profiles := profilesForCorpID(cfg, selector); len(profiles) > 0 {
		if exact := exactProfileSelectorForCorp(cfg, selector, cfg.OrgCurrentProfiles[selector]); exact != "" {
			return exact
		}
		if len(profiles) == 1 {
			return storedProfileSelector(cfg, profiles[0])
		}
		return ""
	}
	p, _, err := resolveProfileSelection("", cfg, selector)
	if err != nil || p == nil {
		return ""
	}
	return storedProfileSelector(cfg, p)
}

func setOrgCurrentProfile(cfg *ProfilesConfig, corpID, selector string) {
	if cfg == nil {
		return
	}
	exact := exactProfileSelectorForCorp(cfg, corpID, selector)
	if exact == "" {
		return
	}
	if cfg.OrgCurrentProfiles == nil {
		cfg.OrgCurrentProfiles = make(map[string]string)
	}
	cfg.OrgCurrentProfiles[strings.TrimSpace(corpID)] = exact
}

func uniqueProfileCorpIDs(cfg *ProfilesConfig) []string {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, p := range cfg.Profiles {
		corpID := strings.TrimSpace(p.CorpID)
		if corpID == "" || seen[corpID] {
			continue
		}
		seen[corpID] = true
		result = append(result, corpID)
	}
	return result
}

func profileIndexByIdentity(cfg *ProfilesConfig, corpID, userID string) int {
	if cfg == nil {
		return -1
	}
	for i := range cfg.Profiles {
		if sameProfileIdentity(cfg.Profiles[i].CorpID, cfg.Profiles[i].UserID, corpID, userID) {
			return i
		}
	}
	return -1
}

func legacyProfileIndexByCorpID(cfg *ProfilesConfig, corpID string) int {
	if cfg == nil {
		return -1
	}
	match := -1
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpID) != strings.TrimSpace(corpID) || strings.TrimSpace(cfg.Profiles[i].UserID) != "" {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = i
	}
	return match
}

func findExactProfile(cfg *ProfilesConfig, corpID, userID string) *Profile {
	idx := profileIndexByIdentity(cfg, corpID, userID)
	if idx < 0 {
		return nil
	}
	return &cfg.Profiles[idx]
}

func profilesForCorpID(cfg *ProfilesConfig, corpID string) []*Profile {
	if cfg == nil {
		return nil
	}
	corpID = strings.TrimSpace(corpID)
	result := make([]*Profile, 0)
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpID) == corpID {
			result = append(result, &cfg.Profiles[i])
		}
	}
	return result
}

func unresolvedProfileForCorp(cfg *ProfilesConfig, corpID string) *Profile {
	if cfg == nil {
		return nil
	}
	corpID = strings.TrimSpace(corpID)
	for i := range cfg.Profiles {
		profile := &cfg.Profiles[i]
		if strings.TrimSpace(profile.CorpID) == corpID && strings.TrimSpace(profile.UserID) == "" {
			return profile
		}
	}
	return nil
}

func unresolvedProfileForLocalName(cfg *ProfilesConfig, name string) *Profile {
	if cfg == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	var match *Profile
	for i := range cfg.Profiles {
		profile := &cfg.Profiles[i]
		if strings.TrimSpace(profile.UserID) != "" || strings.TrimSpace(profile.Name) != name ||
			len(profilesForCorpID(cfg, profile.CorpID)) <= 1 {
			continue
		}
		if match != nil {
			return nil
		}
		match = profile
	}
	return match
}

// When a blank profile coexists with exact accounts, the organization slot is
// that unresolved profile's only canonical credential. Exact identities must
// remain in their identity slots and may still become global current without
// overwriting the organization slot.
func shouldSyncOrganizationMirror(cfg *ProfilesConfig, profile Profile) bool {
	return strings.TrimSpace(profile.UserID) == "" || unresolvedProfileForCorp(cfg, profile.CorpID) == nil
}

func profileSelectorReferenceExists(cfg *ProfilesConfig, selector string) bool {
	if cfg == nil {
		return false
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	if corpID, unresolved := parseUnresolvedProfileSelector(selector); unresolved {
		return unresolvedProfileForCorp(cfg, corpID) != nil
	}
	if unresolvedProfileForLocalName(cfg, selector) != nil {
		return true
	}
	if corpID, userID, exact := ParseIdentitySelector(selector); exact {
		if findExactProfile(cfg, corpID, userID) != nil {
			return true
		}
		for i := range cfg.Profiles {
			p := &cfg.Profiles[i]
			orgMatches := strings.TrimSpace(p.CorpID) == corpID || strings.TrimSpace(p.CorpName) == corpID
			accountMatches := strings.TrimSpace(p.UserID) == userID || strings.TrimSpace(p.UserName) == userID
			if orgMatches && accountMatches {
				return true
			}
		}
		return false
	}
	if len(profilesForCorpID(cfg, selector)) > 0 {
		return true
	}
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpName) == selector ||
			strings.TrimSpace(cfg.Profiles[i].Name) == selector {
			return true
		}
	}
	return false
}

func profileIdentityKey(corpID, userID string) string {
	return strings.TrimSpace(corpID) + "\x00" + strings.TrimSpace(userID)
}

func sameProfileIdentity(leftCorpID, leftUserID, rightCorpID, rightUserID string) bool {
	return profileIdentityKey(leftCorpID, leftUserID) == profileIdentityKey(rightCorpID, rightUserID)
}

func shortCorpID(corpID string) string {
	corpID = strings.TrimSpace(corpID)
	if len(corpID) <= 8 {
		return corpID
	}
	return corpID[len(corpID)-8:]
}

func shortProfileID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}
