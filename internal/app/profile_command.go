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
	"github.com/charmbracelet/huh"
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
		Use:               "use [name|corpId|-]",
		Short:             "切换当前组织 profile",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileSwitchCommand(cmd, args)
		},
	}
}

var (
	profileSwitchSelector            = selectProfileSwitchProfile
	profileSwitchInteractiveTerminal = isInteractiveTerminal
)

func runProfileSwitchCommand(cmd *cobra.Command, args []string) error {
	configDir := defaultConfigDir()
	selector := ""
	if len(args) > 0 {
		selector = strings.TrimSpace(args[0])
	}
	usedTUI := false
	if selector == "" {
		var err error
		selector, err = profileSwitchSelector(cmd, configDir)
		if err != nil {
			return err
		}
		usedTUI = true
	}
	return switchProfileAndWrite(cmd, configDir, selector, usedTUI)
}

func switchProfileAndWrite(cmd *cobra.Command, configDir, selector string, usedTUI bool) error {
	var (
		profile *authpkg.Profile
		err     error
	)
	if strings.TrimSpace(selector) == "-" {
		profile, err = authpkg.UsePreviousProfile(configDir)
	} else {
		profile, err = authpkg.SetCurrentProfile(configDir, selector)
	}
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	ResetRuntimeTokenCache()
	clearCompatCache()
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if strings.EqualFold(strings.TrimSpace(format), "json") && !(usedTUI && authLoginAllowsInteractiveDefault(cmd, format)) {
		cfg, loadErr := authpkg.LoadProfiles(configDir)
		if loadErr != nil {
			return apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", loadErr))
		}
		return writeProfileUseJSON(cmd.OutOrStdout(), profile, cfg)
	}
	fmt.Fprintln(cmd.OutOrStdout(), profileUseMessage(profile))
	return nil
}

func selectProfileSwitchProfile(cmd *cobra.Command, configDir string) (string, error) {
	if !profileSwitchInteractiveTerminal() {
		return "", apperrors.NewValidation("profile selector required in non-interactive mode; use dws auth switch <name|corpId> or dws profile use <name|corpId>")
	}
	if err := authpkg.EnsureProfilesMigration(configDir); err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to migrate profiles: %v", err))
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", err))
	}
	if cfg == nil || len(cfg.Profiles) == 0 {
		return "", apperrors.NewValidation("未找到已登录 profile，请先运行 dws auth login")
	}
	choice := strings.TrimSpace(cfg.CurrentProfile)
	if choice == "" {
		choice = strings.TrimSpace(cfg.PrimaryProfile)
	}
	if choice == "" {
		choice = cfg.Profiles[0].CorpID
	}
	options := make([]huh.Option[string], 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		options = append(options, huh.NewOption(profileSwitchOptionLabel(p, cfg), p.CorpID))
	}
	height := len(options)
	if height > 12 {
		height = 12
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择要切换的组织").
				Description("↑↓ 选择，Enter 确认\n\nORGANIZATION                 STATUS   USER               CORP_ID").
				Options(options...).
				Height(height).
				Value(&choice),
		),
	).WithTheme(authLoginHuhTheme())
	if err := form.Run(); err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("组织选择中止: %v", err))
	}
	return strings.TrimSpace(choice), nil
}

func profileSwitchOptionLabel(p authpkg.Profile, cfg *authpkg.ProfilesConfig) string {
	status := strings.TrimSpace(p.Status)
	if status == "" {
		status = authpkg.ProfileStatusActive
	}
	statusLabel := ""
	switch status {
	case authpkg.ProfileStatusActive:
		statusLabel = "已登录"
	case authpkg.ProfileStatusExpired:
		statusLabel = "已过期"
	case authpkg.ProfileStatusRevoked:
		statusLabel = "已撤销"
	default:
		statusLabel = status
	}
	user := strings.TrimSpace(p.UserName)
	if user == "" {
		user = strings.TrimSpace(p.UserID)
	}
	if user == "" {
		user = "-"
	}
	marker := ""
	if cfg != nil && p.CorpID == cfg.CurrentProfile {
		marker = "  ← 当前"
	} else if cfg != nil && p.CorpID == cfg.PrimaryProfile {
		marker = "  default"
	}
	org := profileOrgName(p)
	return fmt.Sprintf("%-28s %-8s %-18s %s%s", clipProfileCell(org, 28), statusLabel, clipProfileCell(user, 18), p.CorpID, marker)
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
	fmt.Fprintf(w, "%-3s %-3s %-24s %-28s %-34s %-10s %s\n", "CUR", "PRI", "PROFILE", "ORG_NAME", "CORP_ID", "STATUS", "USER")
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
		fmt.Fprintf(
			w,
			"%-3s %-3s %-24s %-28s %-34s %-10s %s\n",
			current,
			primary,
			clipProfileCell(p.Name, 24),
			clipProfileCell(profileOrgName(p), 28),
			clipProfileCell(p.CorpID, 34),
			status,
			user,
		)
	}
}

func profileUseMessage(profile *authpkg.Profile) string {
	if profile == nil {
		return "[OK] 当前 profile 已切换"
	}
	name := strings.TrimSpace(profile.Name)
	corpID := strings.TrimSpace(profile.CorpID)
	if name == "" {
		name = corpID
	}
	orgName := strings.TrimSpace(profile.CorpName)
	if orgName == "" {
		orgName = name
	}
	return fmt.Sprintf("[OK] 当前 profile: %s | 组织: %s (%s)", name, orgName, corpID)
}

func profileOrgName(p authpkg.Profile) string {
	if v := strings.TrimSpace(p.CorpName); v != "" {
		return v
	}
	if v := strings.TrimSpace(p.Name); v != "" {
		return v
	}
	return strings.TrimSpace(p.CorpID)
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
