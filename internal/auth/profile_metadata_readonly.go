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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
)

type ProfileMetadata = profilemetadata.ProfileMetadata

// ResolveProfileMetadataReadOnly reads only non-sensitive profiles.json metadata.
// Keep selection and normalization pure so startup checks do not initialize auth.
func ResolveProfileMetadataReadOnly(configDir, selector string) (*ProfileMetadata, error) {
	cfg, err := loadProfileMetadataReadOnly(configDir)
	if err != nil || cfg == nil {
		return nil, err
	}
	profile, err := resolveProfileFromSnapshot(cfg, selector)
	if err != nil || profile == nil {
		return nil, err
	}
	return &ProfileMetadata{
		UserID:   profile.UserID,
		UserName: profile.UserName,
		CorpID:   profile.CorpID,
	}, nil
}

// loadProfileMetadataReadOnly never quarantines or repairs the persisted
// registry. Callers get a private snapshot, not a multi-file transaction.
func loadProfileMetadataReadOnly(configDir string) (*ProfilesConfig, error) {
	data, err := profilesReadFile(ProfilesPath(configDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profile metadata: %w", err)
	}

	var cfg ProfilesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse profile metadata: %w", err)
	}
	if cfg.Version > profilesMaxVersion {
		return nil, fmt.Errorf("profile metadata version %d is newer than supported version %d", cfg.Version, profilesMaxVersion)
	}
	normalizeProfilesConfig(&cfg)
	return &cfg, nil
}

func resolveProfileFromSnapshot(cfg *ProfilesConfig, selector string) (*Profile, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = strings.TrimSpace(cfg.CurrentProfile)
		if selector == "" {
			return nil, nil
		}
	}
	profile, _, err := resolveProfileSelection("", cfg, selector)
	return profile, err
}
