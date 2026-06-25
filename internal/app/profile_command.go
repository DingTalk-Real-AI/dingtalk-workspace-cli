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

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func newProfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "profile",
		Short:             "组织 profile 管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newProfileListCommand(), newProfileUseCommand())
	return cmd
}

func newProfileListCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "list",
		Aliases:           []string{"ls"},
		Short:             "列出已登录组织 profile",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			if err := authpkg.EnsureProfilesMigration(configDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to migrate profiles: %v", err))
			}
			cfg, err := authpkg.LoadProfiles(configDir)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", err))
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				return writeProfileListJSON(cmd.OutOrStdout(), cfg)
			}
			writeProfileListTable(cmd.OutOrStdout(), cfg)
			return nil
		},
	}
}

func newProfileUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "use <name|corpId|->",
		Short:             "切换当前组织 profile",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			var (
				profile *authpkg.Profile
				err     error
			)
			if strings.TrimSpace(args[0]) == "-" {
				profile, err = authpkg.UsePreviousProfile(configDir)
			} else {
				profile, err = authpkg.SetCurrentProfile(configDir, args[0])
			}
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			ResetRuntimeTokenCache()
			clearCompatCache()
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				cfg, loadErr := authpkg.LoadProfiles(configDir)
				if loadErr != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", loadErr))
				}
				return writeProfileUseJSON(cmd.OutOrStdout(), profile, cfg)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[OK] 当前 profile: %s (%s)\n", profile.Name, profile.CorpID)
			return nil
		},
	}
}

type profileListResponse struct {
	Success         bool          `json:"success"`
	PrimaryProfile  string        `json:"primaryProfile,omitempty"`
	CurrentProfile  string        `json:"currentProfile,omitempty"`
	PreviousProfile string        `json:"previousProfile,omitempty"`
	Profiles        []profileView `json:"profiles"`
}

type profileUseResponse struct {
	Success bool        `json:"success"`
	Profile profileView `json:"profile"`
}

type profileView struct {
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
	IsPrimary         bool     `json:"isPrimary"`
	IsCurrent         bool     `json:"isCurrent"`
}

func writeProfileListJSON(w io.Writer, cfg *authpkg.ProfilesConfig) error {
	resp := profileListResponse{
		Success:         true,
		PrimaryProfile:  cfg.PrimaryProfile,
		CurrentProfile:  cfg.CurrentProfile,
		PreviousProfile: cfg.PreviousProfile,
		Profiles:        profileViews(cfg),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func writeProfileUseJSON(w io.Writer, profile *authpkg.Profile, cfg *authpkg.ProfilesConfig) error {
	resp := profileUseResponse{Success: true}
	if profile != nil {
		primaryProfile := ""
		currentProfile := ""
		if cfg != nil {
			primaryProfile = cfg.PrimaryProfile
			currentProfile = cfg.CurrentProfile
		}
		resp.Profile = profileViewFromProfile(*profile, primaryProfile, currentProfile)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func writeProfileListTable(w io.Writer, cfg *authpkg.ProfilesConfig) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		fmt.Fprintln(w, "未找到已登录 profile")
		return
	}
	fmt.Fprintf(w, "%-3s %-3s %-28s %-34s %-10s %s\n", "CUR", "PRI", "NAME", "CORP_ID", "STATUS", "USER")
	for _, p := range cfg.Profiles {
		current := ""
		if p.CorpID == cfg.CurrentProfile {
			current = "*"
		}
		primary := ""
		if p.CorpID == cfg.PrimaryProfile {
			primary = "*"
		}
		user := p.UserName
		if user == "" {
			user = p.UserID
		}
		status := p.Status
		if status == "" {
			status = authpkg.ProfileStatusActive
		}
		fmt.Fprintf(w, "%-3s %-3s %-28s %-34s %-10s %s\n", current, primary, clipProfileCell(p.Name, 28), clipProfileCell(p.CorpID, 34), status, user)
	}
}

func profileViews(cfg *authpkg.ProfilesConfig) []profileView {
	if cfg == nil {
		return nil
	}
	views := make([]profileView, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		views = append(views, profileViewFromProfile(p, cfg.PrimaryProfile, cfg.CurrentProfile))
	}
	return views
}

func profileViewFromProfile(p authpkg.Profile, primaryProfile, currentProfile string) profileView {
	return profileView{
		Name:              p.Name,
		CorpID:            p.CorpID,
		CorpName:          p.CorpName,
		UserID:            p.UserID,
		UserName:          p.UserName,
		ClientID:          p.ClientID,
		Status:            p.Status,
		AuthorizedDomains: p.AuthorizedDomains,
		ExpiresAt:         p.ExpiresAt,
		RefreshExpAt:      p.RefreshExpAt,
		LastLoginAt:       p.LastLoginAt,
		LastUsedAt:        p.LastUsedAt,
		IsPrimary:         p.CorpID == primaryProfile,
		IsCurrent:         p.CorpID == currentProfile,
	}
}

func clipProfileCell(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
