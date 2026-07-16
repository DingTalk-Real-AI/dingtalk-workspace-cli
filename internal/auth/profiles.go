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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

var (
	profilesAcquireDualLock  = AcquireDualLock
	profilesReadFile         = os.ReadFile
	profilesRename           = os.Rename
	profilesMkdirAll         = os.MkdirAll
	profilesMarshalIndent    = json.MarshalIndent
	profilesWriteFile        = os.WriteFile
	profilesRemove           = os.Remove
	profilesLoad             = LoadProfiles
	profilesSave             = SaveProfiles
	profilesEnsureMigration  = ensureProfilesMigrationLocked
	profilesSyncLegacyMirror = syncLegacyTokenMirrorLocked
	profilesTokenExists      = TokenDataExistsKeychain
	profilesLoadLegacy       = LoadTokenDataKeychain
	profilesSaveCorp         = SaveTokenDataKeychainForCorpID
	profilesTokenExistsCorp  = TokenDataExistsKeychainForCorpID
	profilesLoadCorp         = LoadTokenDataKeychainForCorpID
	profilesSaveLegacy       = SaveTokenDataKeychain
	profilesWriteMarker      = WriteTokenMarker
	profilesDeleteLegacy     = DeleteTokenDataKeychain
	profilesDeleteMarker     = DeleteTokenMarker
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

const profilesJSONFile = "profiles.json"

const (
	ProfileStatusActive  = "active"
	ProfileStatusExpired = "expired"
	ProfileStatusRevoked = "revoked"
)

// ProfilesConfig stores non-sensitive profile metadata. Token material stays in keychain.
type ProfilesConfig struct {
	Version         int       `json:"version"`
	PrimaryProfile  string    `json:"primaryProfile,omitempty"`
	CurrentProfile  string    `json:"currentProfile,omitempty"`
	PreviousProfile string    `json:"previousProfile,omitempty"`
	Profiles        []Profile `json:"profiles,omitempty"`
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
	if len(cfg.Profiles) > 0 {
		return nil
	}
	if !profilesTokenExists() {
		return nil
	}
	data, err := profilesLoadLegacy()
	if err != nil || data == nil || strings.TrimSpace(data.CorpID) == "" {
		return nil
	}
	if err := profilesSaveCorp(data.CorpID, data); err != nil {
		return err
	}
	return upsertProfileFromToken(configDir, cfg, data, false)
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
	normalizeProfilesConfig(cfg)
	now := time.Now().Format(time.RFC3339)
	idx := profileIndexByCorpID(cfg, corpID)
	if idx < 0 {
		profile := Profile{
			Name:         chooseProfileName(cfg, data),
			CorpID:       corpID,
			CorpName:     strings.TrimSpace(data.CorpName),
			UserID:       strings.TrimSpace(data.UserID),
			UserName:     strings.TrimSpace(data.UserName),
			ClientID:     strings.TrimSpace(data.ClientID),
			Status:       ProfileStatusActive,
			ExpiresAt:    timeOrRFC3339(data.ExpiresAt),
			RefreshExpAt: timeOrRFC3339(data.RefreshExpAt),
			LastLoginAt:  now,
			LastUsedAt:   now,
			UpdatedAt:    now,
		}
		cfg.Profiles = append(cfg.Profiles, profile)
	} else {
		p := &cfg.Profiles[idx]
		if shouldRefreshProfileName(p, data) {
			p.Name = chooseProfileName(cfg, data)
		}
		if v := strings.TrimSpace(data.CorpName); v != "" {
			p.CorpName = v
		}
		if v := strings.TrimSpace(data.UserID); v != "" {
			p.UserID = v
		}
		if v := strings.TrimSpace(data.UserName); v != "" {
			p.UserName = v
		}
		if v := strings.TrimSpace(data.ClientID); v != "" {
			p.ClientID = v
		}
		p.Status = ProfileStatusActive
		p.ExpiresAt = timeOrRFC3339(data.ExpiresAt)
		p.RefreshExpAt = timeOrRFC3339(data.RefreshExpAt)
		p.LastLoginAt = now
		p.LastUsedAt = now
		p.UpdatedAt = now
	}
	if cfg.PrimaryProfile == "" {
		cfg.PrimaryProfile = corpID
	}
	if makeCurrent && cfg.CurrentProfile != corpID {
		if cfg.CurrentProfile != "" {
			cfg.PreviousProfile = cfg.CurrentProfile
		}
		cfg.CurrentProfile = corpID
	}
	if cfg.CurrentProfile == "" {
		cfg.CurrentProfile = corpID
	}
	return profilesSave(configDir, cfg)
}

// ResolveProfile returns a profile selected by name/corpId or by current/primary fallback.
func ResolveProfile(configDir, selector string) (*Profile, error) {
	if err := profilesEnsureMigration(configDir); err != nil {
		return nil, err
	}
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, err
	}
	selector = strings.TrimSpace(selector)
	if selector != "" {
		p := findProfile(cfg, selector)
		if p == nil {
			return nil, fmt.Errorf("profile %q not found", selector)
		}
		return p, nil
	}
	if p := findProfile(cfg, cfg.CurrentProfile); p != nil {
		return p, nil
	}
	if p := findProfile(cfg, cfg.PrimaryProfile); p != nil {
		return p, nil
	}
	return nil, nil
}

func resolveProfileForLoad(configDir, selector string) (*Profile, error) {
	if err := profilesEnsureMigration(configDir); err != nil {
		return nil, err
	}
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return nil, err
	}
	selector = strings.TrimSpace(selector)
	if selector != "" {
		p := findProfile(cfg, selector)
		if p == nil {
			return nil, fmt.Errorf("profile %q not found", selector)
		}
		return p, nil
	}
	for _, candidate := range []string{cfg.CurrentProfile, cfg.PrimaryProfile} {
		if p := findProfile(cfg, candidate); p != nil && profilesTokenExistsCorp(p.CorpID) {
			return p, nil
		}
	}
	if p := findProfile(cfg, cfg.CurrentProfile); p != nil {
		return p, nil
	}
	if p := findProfile(cfg, cfg.PrimaryProfile); p != nil {
		return p, nil
	}
	return nil, nil
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
	p := findProfile(cfg, selector)
	if p == nil {
		return nil, fmt.Errorf("profile %q not found", strings.TrimSpace(selector))
	}
	if cfg.CurrentProfile != p.CorpID {
		if cfg.CurrentProfile != "" {
			cfg.PreviousProfile = cfg.CurrentProfile
		}
		cfg.CurrentProfile = p.CorpID
	}
	touchProfile(cfg, p.CorpID)
	if err := profilesSave(configDir, cfg); err != nil {
		return nil, err
	}
	if err := profilesSyncLegacyMirror(configDir); err != nil {
		return nil, err
	}
	return findProfile(cfg, p.CorpID), nil
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
	prev := strings.TrimSpace(cfg.PreviousProfile)
	if prev == "" {
		return nil, fmt.Errorf("previous profile is empty")
	}
	p := findProfile(cfg, prev)
	if p == nil {
		return nil, fmt.Errorf("previous profile %q not found", prev)
	}
	cfg.PreviousProfile, cfg.CurrentProfile = cfg.CurrentProfile, p.CorpID
	touchProfile(cfg, p.CorpID)
	if err := profilesSave(configDir, cfg); err != nil {
		return nil, err
	}
	if err := profilesSyncLegacyMirror(configDir); err != nil {
		return nil, err
	}
	return findProfile(cfg, p.CorpID), nil
}

// RemoveProfile removes a profile from metadata and returns the removed profile.
func RemoveProfile(configDir, selector string) (*Profile, error) {
	var result *Profile
	err := withProfilesLock(configDir, func() error {
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
	p := findProfile(cfg, selector)
	if p == nil {
		return nil, fmt.Errorf("profile %q not found", strings.TrimSpace(selector))
	}
	removed := *p
	kept := cfg.Profiles[:0]
	for _, profile := range cfg.Profiles {
		if profile.CorpID != removed.CorpID {
			kept = append(kept, profile)
		}
	}
	cfg.Profiles = kept
	if cfg.PrimaryProfile == removed.CorpID {
		cfg.PrimaryProfile = firstProfileCorpID(cfg)
	}
	if cfg.CurrentProfile == removed.CorpID {
		cfg.CurrentProfile = cfg.PrimaryProfile
		if cfg.CurrentProfile == "" {
			cfg.CurrentProfile = firstProfileCorpID(cfg)
		}
	}
	if cfg.PreviousProfile == removed.CorpID {
		cfg.PreviousProfile = ""
	}
	if len(cfg.Profiles) == 0 {
		cfg.PrimaryProfile = ""
		cfg.CurrentProfile = ""
		cfg.PreviousProfile = ""
	}
	if err := profilesSave(configDir, cfg); err != nil {
		return nil, err
	}
	return &removed, nil
}

// MarkProfileStatus updates a profile status if it exists.
func MarkProfileStatus(configDir, corpID, status string) error {
	if strings.TrimSpace(corpID) == "" {
		return nil
	}
	return withProfilesLock(configDir, func() error {
		return markProfileStatusLocked(configDir, corpID, status)
	})
}

func markProfileStatusLocked(configDir, corpID, status string) error {
	cfg, err := profilesLoad(configDir)
	if err != nil {
		return err
	}
	p := findProfile(cfg, corpID)
	if p == nil {
		return nil
	}
	p.Status = strings.TrimSpace(status)
	p.UpdatedAt = time.Now().Format(time.RFC3339)
	return profilesSave(configDir, cfg)
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
	hadReadError := false
	for _, candidate := range []string{cfg.CurrentProfile, cfg.PrimaryProfile} {
		p := findProfile(cfg, candidate)
		if p == nil {
			continue
		}
		data, loadErr := profilesLoadCorp(p.CorpID)
		if loadErr != nil {
			// Transient keychain read failure: do NOT touch the existing mirror.
			hadReadError = true
			continue
		}
		if data != nil {
			if err := profilesSaveLegacy(data); err != nil {
				return err
			}
			return profilesWriteMarker(configDir)
		}
	}
	if hadReadError {
		// Keep the existing legacy mirror untouched rather than wiping a host
		// app's login state just because keychain was momentarily unavailable.
		return nil
	}
	// All candidate profiles confirmed absent (no token): clear the mirror.
	_ = profilesDeleteLegacy()
	_ = profilesDeleteMarker(configDir)
	return nil
}

func normalizeProfilesConfig(cfg *ProfilesConfig) {
	if cfg == nil {
		return
	}
	cfg.Version = 1
	seen := make(map[string]bool, len(cfg.Profiles))
	profiles := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		p.CorpID = strings.TrimSpace(p.CorpID)
		if p.CorpID == "" || seen[p.CorpID] {
			continue
		}
		seen[p.CorpID] = true
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			p.Name = p.CorpID
		}
		if corpName := strings.TrimSpace(p.CorpName); p.Name == p.CorpID && corpName != "" && !profileNameTakenByOtherCorp(cfg, corpName, p.CorpID) {
			p.Name = corpName
		}
		if p.Status == "" {
			p.Status = ProfileStatusActive
		}
		profiles = append(profiles, p)
	}
	cfg.Profiles = profiles
	if cfg.PrimaryProfile != "" && findProfile(cfg, cfg.PrimaryProfile) == nil {
		cfg.PrimaryProfile = ""
	}
	if cfg.CurrentProfile != "" && findProfile(cfg, cfg.CurrentProfile) == nil {
		cfg.CurrentProfile = ""
	}
	if cfg.PreviousProfile != "" && findProfile(cfg, cfg.PreviousProfile) == nil {
		cfg.PreviousProfile = ""
	}
	if cfg.PrimaryProfile == "" {
		cfg.PrimaryProfile = firstProfileCorpID(cfg)
	}
	if cfg.CurrentProfile == "" {
		cfg.CurrentProfile = cfg.PrimaryProfile
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
	if !profileNameTakenByOtherCorp(cfg, base, data.CorpID) {
		return base
	}
	suffix := shortCorpID(data.CorpID)
	name := base + "-" + suffix
	if !profileNameTakenByOtherCorp(cfg, name, data.CorpID) {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%s-%d", base, suffix, i)
		if !profileNameTakenByOtherCorp(cfg, candidate, data.CorpID) {
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

func profileNameTakenByOtherCorp(cfg *ProfilesConfig, name, corpID string) bool {
	name = strings.TrimSpace(name)
	corpID = strings.TrimSpace(corpID)
	for _, p := range cfg.Profiles {
		if p.CorpID != corpID && p.Name == name {
			return true
		}
	}
	return false
}

func findProfile(cfg *ProfilesConfig, selector string) *Profile {
	if cfg == nil {
		return nil
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil
	}
	var corpNameMatch *Profile
	for i := range cfg.Profiles {
		if cfg.Profiles[i].CorpID == selector || cfg.Profiles[i].Name == selector {
			return &cfg.Profiles[i]
		}
		if strings.TrimSpace(cfg.Profiles[i].CorpName) == selector {
			if corpNameMatch != nil {
				return nil
			}
			corpNameMatch = &cfg.Profiles[i]
		}
	}
	return corpNameMatch
}

func profileIndexByCorpID(cfg *ProfilesConfig, corpID string) int {
	if cfg == nil {
		return -1
	}
	for i := range cfg.Profiles {
		if cfg.Profiles[i].CorpID == corpID {
			return i
		}
	}
	return -1
}

func firstProfileCorpID(cfg *ProfilesConfig) string {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return ""
	}
	return cfg.Profiles[0].CorpID
}

func touchProfile(cfg *ProfilesConfig, corpID string) {
	if p := findProfile(cfg, corpID); p != nil {
		now := time.Now().Format(time.RFC3339)
		p.LastUsedAt = now
		p.UpdatedAt = now
	}
}

func timeOrRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func shortCorpID(corpID string) string {
	corpID = strings.TrimSpace(corpID)
	if len(corpID) <= 8 {
		return corpID
	}
	return corpID[len(corpID)-8:]
}
