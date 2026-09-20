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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// ErrTokenMigrationRequired signals an inconclusive snapshot requiring repair,
// not a confirmed logout.
var ErrTokenMigrationRequired = errors.New("token storage requires migration")

// ReadTokenDataForProfile observes a best-effort local snapshot without auth
// locks, refresh, migration, quarantine or persistence. Concurrent publication
// may yield an older snapshot or an inconclusive error. Opaque edition hooks
// are rejected because their read-only behavior cannot be guaranteed.
func ReadTokenDataForProfile(configDir, selector string) (*TokenData, error) {
	if edition.Get().LoadToken != nil {
		return nil, fmt.Errorf("read-only inspection is not supported by the current auth backend")
	}
	if strings.TrimSpace(selector) == "" {
		manual, err := manualTokenMarkerActive(configDir)
		if err != nil {
			return nil, err
		}
		if manual {
			data, err := tokenLoadKeychain()
			if err == nil && data != nil && strings.TrimSpace(data.CorpID) == "" {
				return data, nil
			}
			if err != nil && !errors.Is(err, ErrTokenDataNotFound) {
				return nil, err
			}
		}
	}
	cfg, err := loadProfileMetadataReadOnly(configDir)
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.Version < profilesVersion {
		if cfg != nil && len(cfg.Profiles) != 0 {
			return nil, ErrTokenMigrationRequired
		}
		legacy, err := tokenLoadKeychain()
		if err == nil && legacy != nil {
			if strings.TrimSpace(legacy.CorpID) == "" && strings.TrimSpace(selector) == "" {
				return legacy, nil
			}
			return nil, ErrTokenMigrationRequired
		}
		if err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return nil, err
		}
		if _, err := secureStat(filepath.Join(configDir, secureDataFile)); err == nil {
			return nil, ErrTokenMigrationRequired
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		return nil, ErrTokenDataNotFound
	}
	if profileMetadataNeedsMigration(cfg) {
		return nil, ErrTokenMigrationRequired
	}
	selected, err := resolveProfileFromSnapshot(cfg, selector)
	if err != nil {
		// Historical identity selectors may require migration before they can
		// resolve. Report that need without initiating the repair.
		needsMigration, probeErr := profileSelectionNeedsIdentityMigration(cfg, selector)
		if probeErr != nil {
			return nil, probeErr
		}
		if needsMigration {
			return nil, ErrTokenMigrationRequired
		}
		return nil, err
	}
	if selected == nil {
		// A modern empty registry/current profile is a logout boundary. Never
		// resurrect an orphaned global token mirror from it.
		return nil, ErrTokenDataNotFound
	}
	data, err := readProfileIdentitySnapshot(*selected)
	if errors.Is(err, ErrTokenDataNotFound) {
		return probeMissingProfileMigration(cfg, *selected, selector)
	}
	return data, err
}

// readProfileIdentitySnapshot has no compatibility write fallback. Historical
// slots needing repair are handled by the caller's read-only probes.
func readProfileIdentitySnapshot(profile Profile) (*TokenData, error) {
	corpID := strings.TrimSpace(profile.CorpID)
	userID := strings.TrimSpace(profile.UserID)
	var data *TokenData
	var err error
	if userID == "" {
		data, err = tokenLoadKeychainForCorpID(corpID)
	} else {
		data, err = tokenLoadKeychainIdentity(corpID, userID)
	}
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrTokenDataNotFound
	}
	if strings.TrimSpace(data.CorpID) != corpID {
		return nil, fmt.Errorf("token organization does not match selected profile %q", ProfileSelector(profile))
	}
	if strings.TrimSpace(data.UserID) != userID {
		if userID == "" {
			return nil, ErrTokenMigrationRequired
		}
		return nil, fmt.Errorf("token identity does not match selected profile %q", ProfileSelector(profile))
	}
	return data, nil
}

// profileSelectionNeedsIdentityMigration probes only the organization named by
// a failed compound selector. A modern registry may still have a blank userId
// while its organization token already identifies the historical account.
// Test the existing selector grammar against an enriched private snapshot;
// this only identifies snapshots requiring status repair, never a write plan.
func profileSelectionNeedsIdentityMigration(cfg *ProfilesConfig, selector string) (bool, error) {
	organization, _, compound := ParseIdentitySelector(selector)
	if !compound {
		return false, nil
	}
	corpID, err := resolveOrganizationCorpID(cfg, organization)
	if err != nil || corpID == "" || unresolvedProfileForCorp(cfg, corpID) == nil {
		return false, nil
	}
	data, err := profilesLoadCorp(corpID)
	if errors.Is(err, ErrTokenDataNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("probe profile identity migration: %w", err)
	}
	if data == nil || strings.TrimSpace(data.CorpID) != corpID || strings.TrimSpace(data.UserID) == "" {
		return false, nil
	}
	userID := strings.TrimSpace(data.UserID)
	candidate := cloneProfilesConfig(cfg)
	historical := unresolvedProfileForCorp(candidate, corpID)
	if findExactProfile(cfg, corpID, userID) != nil {
		// Migration removes the duplicate historical placeholder, rather than
		// copying token metadata onto the existing exact profile. Removing it
		// can also resolve a username ambiguity for the same identity.
		historical.CorpID = ""
		normalizeProfilesConfig(candidate)
	} else {
		historical.UserID = userID
		if historical.UserName == "" {
			historical.UserName = strings.TrimSpace(data.UserName)
		}
	}
	_, err = resolveProfileFromSnapshot(candidate, selector)
	return err == nil, nil
}

// Some v2 stores still carry legacy local-name pointers (including names with
// a colon). Canonicalizing those pointers and publishing the v3 downgrade
// guard is a real migration, not an ordinary token read.
func profileMetadataNeedsMigration(cfg *ProfilesConfig) bool {
	if cfg == nil || cfg.Version < profilesVersion {
		return true
	}
	for _, selector := range []string{cfg.CurrentProfile, cfg.PrimaryProfile, cfg.PreviousProfile} {
		if canonical := canonicalStoredSelector(cfg, selector); canonical != "" && canonical != selector {
			return true
		}
	}
	return cfg.Version < profilesUnresolvedSelectorVersion && profilesConfigContainsUnresolvedSelector(cfg)
}

// Inspect historical mirrors without writing. Repairable missing slots are
// reported explicitly; snapshot-only callers must never initiate the repair.
func probeMissingProfileMigration(cfg *ProfilesConfig, selected Profile, selector string) (*TokenData, error) {
	if selected.UserID != "" {
		org, err := tokenLoadKeychainForCorpID(selected.CorpID)
		if err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return nil, err
		}
		if err == nil && org != nil {
			if strings.TrimSpace(org.CorpID) != selected.CorpID {
				return nil, fmt.Errorf("organization token mirror does not match selected profile %q", ProfileSelector(selected))
			}
			if strings.TrimSpace(org.UserID) == selected.UserID ||
				(strings.TrimSpace(org.UserID) == "" && len(profilesForCorpID(cfg, selected.CorpID)) == 1) {
				return nil, ErrTokenMigrationRequired
			}
			if strings.TrimSpace(org.UserID) == "" {
				return nil, fmt.Errorf("organization token mirror for corpId %q has no userId; cannot use it for profile %q", selected.CorpID, ProfileSelector(selected))
			}
			// An existing organization mirror for another identity cannot be
			// repaired from the global slot. Only the default read may use the
			// old same-identity global fallback, without modifying any slot.
			if strings.TrimSpace(selector) != "" {
				return nil, ErrTokenDataNotFound
			}
			return readMatchingLegacyToken(selected)
		}
	}
	repair := uniqueV2GlobalRepairProfile(cfg, selected.CorpID)
	if repair == nil && strings.TrimSpace(selector) != "" {
		return nil, ErrTokenDataNotFound
	}
	legacy, err := tokenLoadKeychain()
	if err != nil {
		return nil, err
	}
	if repair != nil && legacyTokenMatchesV2RepairProfile(legacy, repair) {
		return nil, ErrTokenMigrationRequired
	}
	if strings.TrimSpace(selector) == "" && tokenMatchesProfile(legacy, selected) {
		return legacy, nil
	}
	return nil, ErrTokenDataNotFound
}

func readMatchingLegacyToken(selected Profile) (*TokenData, error) {
	legacy, err := tokenLoadKeychain()
	if err != nil {
		return nil, err
	}
	if !tokenMatchesProfile(legacy, selected) {
		return nil, ErrTokenDataNotFound
	}
	return legacy, nil
}

func tokenMatchesProfile(data *TokenData, selected Profile) bool {
	return data != nil && strings.TrimSpace(data.CorpID) == selected.CorpID && strings.TrimSpace(data.UserID) == selected.UserID
}
