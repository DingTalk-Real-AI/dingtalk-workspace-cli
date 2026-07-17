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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func TestWriteProfileUseJSONKeepsPrimaryAndCurrentDistinct(t *testing.T) {
	profile := &authpkg.Profile{
		Name:     "B Org",
		CorpID:   "corp_b",
		CorpName: "B Org",
		Status:   authpkg.ProfileStatusActive,
	}
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: "corp_a",
		CurrentProfile: "corp_b",
	}
	var buf bytes.Buffer
	if err := writeProfileUseJSON(&buf, profile, cfg); err != nil {
		t.Fatalf("writeProfileUseJSON() error = %v", err)
	}
	var resp profileUseResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte(`"name"`)) {
		t.Fatalf("profile use JSON should not contain name when corpName is present:\n%s", buf.String())
	}
	if resp.Profile.CorpName != "B Org" {
		t.Fatalf("corpName = %q, want B Org", resp.Profile.CorpName)
	}
	if !resp.Profile.IsCurrent {
		t.Fatalf("isCurrent = false, want true")
	}
	if resp.Profile.IsPrimary {
		t.Fatalf("isPrimary = true, want false")
	}
}

func TestProfileListRootCommandJSONIncludesCorpName(t *testing.T) {
	setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "json", "profile", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile list --format json error = %v\noutput:\n%s", err, out.String())
	}
	var resp profileListResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if !resp.Success {
		t.Fatal("success = false, want true")
	}
	if resp.PrimaryProfile != "" ||
		resp.CurrentProfile != "corp_secondary:user-corp_secondary" ||
		resp.PreviousProfile != "corp_primary:user-corp_primary" {
		t.Fatalf("profile pointers = primary %q current %q previous %q", resp.PrimaryProfile, resp.CurrentProfile, resp.PreviousProfile)
	}
	if len(resp.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2", len(resp.Profiles))
	}
	if bytes.Contains(out.Bytes(), []byte(`"name"`)) {
		t.Fatalf("profile list JSON should not contain name when corpName is present:\n%s", out.String())
	}
	for _, p := range resp.Profiles {
		if p.CorpName == "" {
			t.Fatalf("profile %s missing corpName in JSON response: %#v", p.CorpID, p)
		}
	}
}

func TestProfileListRootCommandJSONIncludesAllAccountsInSameCorp(t *testing.T) {
	first := authLogoutTestToken("corp_same")
	first.UserID = "user_1"
	first.UserName = "账号一"
	second := authLogoutTestToken("corp_same")
	second.AccessToken = "access-second"
	second.RefreshToken = "refresh-second"
	second.UserID = "user_2"
	second.UserName = "账号二"
	setupAuthLogoutProfiles(t, first, second)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "json", "profile", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile list --format json error = %v\noutput:\n%s", err, out.String())
	}
	var resp profileListResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if len(resp.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2: %#v", len(resp.Profiles), resp.Profiles)
	}
	got := make(map[string]profileView, len(resp.Profiles))
	for _, profile := range resp.Profiles {
		got[profile.Profile] = profile
	}
	if _, ok := got["corp_same:user_1"]; !ok {
		t.Fatalf("profiles missing corp_same:user_1: %#v", resp.Profiles)
	}
	current, ok := got["corp_same:user_2"]
	if !ok {
		t.Fatalf("profiles missing corp_same:user_2: %#v", resp.Profiles)
	}
	if !current.IsOrgCurrent || !current.IsCurrent || current.IsPrimary {
		t.Fatalf("last login account markers = %#v, want org-current/current and deprecated primary=false", current)
	}
	if got["corp_same:user_1"].IsOrgCurrent {
		t.Fatalf("older account unexpectedly marked org current: %#v", got["corp_same:user_1"])
	}
}

func TestProfileListUsesRealIdentityTokenState(t *testing.T) {
	token := authLogoutTestToken("corp_real")
	token.ExpiresAt = time.Date(2026, 7, 16, 17, 38, 0, 0, time.Local)
	token.RefreshExpAt = time.Date(2026, 8, 16, 17, 38, 0, 0, time.Local)
	configDir := setupAuthLogoutProfiles(t, token)

	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Profiles[0].Status = authpkg.ProfileStatusActive
	cfg.Profiles[0].ExpiresAt = "2026-07-16T22:29:00+08:00"
	cfg.Profiles[0].RefreshExpAt = "2026-09-16T22:29:00+08:00"
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "json", "profile", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile list error = %v\noutput:\n%s", err, out.String())
	}
	var resp profileListResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if len(resp.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(resp.Profiles))
	}
	got := resp.Profiles[0]
	if got.ExpiresAt != token.ExpiresAt.Format(time.RFC3339) {
		t.Fatalf("expiresAt = %q, want real token %q", got.ExpiresAt, token.ExpiresAt.Format(time.RFC3339))
	}
	if got.RefreshExpAt != token.RefreshExpAt.Format(time.RFC3339) {
		t.Fatalf("refreshExpAt = %q, want real token %q", got.RefreshExpAt, token.RefreshExpAt.Format(time.RFC3339))
	}
	if got.Status != authpkg.ProfileStatusExpired {
		t.Fatalf("status = %q, want expired", got.Status)
	}
}

func TestProfileListDistinguishesMissingAndUnavailableTokenState(t *testing.T) {
	originalLoad := profileLoadTokenData
	t.Cleanup(func() { profileLoadTokenData = originalLoad })
	profile := authpkg.Profile{CorpID: "corp", UserID: "user"}

	profileLoadTokenData = func(string, string) (*authpkg.TokenData, error) {
		return nil, authpkg.ErrTokenDataNotFound
	}
	if state := loadProfileTokenState("cfg", profile); state.Status != authpkg.ProfileStatusRevoked {
		t.Fatalf("missing token status = %q, want revoked", state.Status)
	}

	profileLoadTokenData = func(string, string) (*authpkg.TokenData, error) {
		return nil, errors.New("keychain unavailable")
	}
	if state := loadProfileTokenState("cfg", profile); state.Status != authpkg.ProfileStatusUnavailable {
		t.Fatalf("unavailable token status = %q, want unavailable", state.Status)
	}
}

func TestProfileListCurrentFlagsUseStoredExactSelectors(t *testing.T) {
	first := authLogoutTestToken("corp_same")
	first.UserID = "user_1"
	first.UserName = "账号一"
	second := authLogoutTestToken("corp_same")
	second.AccessToken = "access-second"
	second.RefreshToken = "refresh-second"
	second.UserID = "user_2"
	second.UserName = "账号二"
	configDir := setupAuthLogoutProfiles(t, first, second)

	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.PrimaryProfile = ""
	cfg.CurrentProfile = "corp_same:user_1"
	cfg.OrgCurrentProfiles["corp_same"] = "corp_same:user_1"
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "json", "profile", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile list error = %v\noutput:\n%s", err, out.String())
	}
	var resp profileListResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	got := make(map[string]profileView, len(resp.Profiles))
	for _, profile := range resp.Profiles {
		got[profile.Profile] = profile
	}
	if !got["corp_same:user_1"].IsCurrent || !got["corp_same:user_1"].IsOrgCurrent {
		t.Fatalf("first account flags = %#v, want current and org current", got["corp_same:user_1"])
	}
	if got["corp_same:user_2"].IsCurrent || got["corp_same:user_2"].IsOrgCurrent {
		t.Fatalf("second account flags = %#v, want neither current nor org current", got["corp_same:user_2"])
	}
	if got["corp_same:user_1"].IsPrimary || got["corp_same:user_2"].IsPrimary {
		t.Fatalf("deprecated isPrimary should be false without primaryProfile: %#v", got)
	}
}

func TestProfileListTableOmitsDeprecatedPrimaryColumn(t *testing.T) {
	setupAuthLogoutProfiles(t, authLogoutTestToken("corp_table"))

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"profile", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile list error = %v\noutput:\n%s", err, out.String())
	}
	header := strings.SplitN(out.String(), "\n", 2)[0]
	if strings.Contains(header, "PRI") {
		t.Fatalf("profile list header still contains deprecated PRI column: %q", header)
	}
}

func TestProfileUseRootCommandSwitchesOrganizationAndLegacyMirror(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "profile", "use", "corp_primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile use corp_primary error = %v\noutput:\n%s", err, out.String())
	}
	CloseFileLogger()
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_primary org")) {
		t.Fatalf("profile use output should include organization name:\n%s", out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_primary:user-corp_primary" ||
		cfg.PreviousProfile != "corp_secondary:user-corp_secondary" {
		t.Fatalf("profile pointers = current %q previous %q", cfg.CurrentProfile, cfg.PreviousProfile)
	}
	legacyToken, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if legacyToken.CorpID != "corp_primary" {
		t.Fatalf("legacy token corp = %q, want corp_primary", legacyToken.CorpID)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "profile", "use", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile use - error = %v\noutput:\n%s", err, out.String())
	}
	CloseFileLogger()
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_secondary org")) {
		t.Fatalf("profile use - output should include organization name:\n%s", out.String())
	}
	cfg, err = authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_secondary:user-corp_secondary" ||
		cfg.PreviousProfile != "corp_primary:user-corp_primary" {
		t.Fatalf("profile pointers = current %q previous %q", cfg.CurrentProfile, cfg.PreviousProfile)
	}
	legacyToken, err = authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if legacyToken.CorpID != "corp_secondary" {
		t.Fatalf("legacy token corp = %q, want corp_secondary", legacyToken.CorpID)
	}
}

func TestProfileSwitchRootCommandSwitchesPrimaryOrganizationAndLegacyMirror(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "profile", "switch", "corp_primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile switch corp_primary error = %v\noutput:\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_primary org")) {
		t.Fatalf("profile switch output should include organization name:\n%s", out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_primary:user-corp_primary" ||
		cfg.PreviousProfile != "corp_secondary:user-corp_secondary" {
		t.Fatalf("profile pointers = current %q previous %q", cfg.CurrentProfile, cfg.PreviousProfile)
	}
	legacyToken, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if legacyToken.CorpID != "corp_primary" {
		t.Fatalf("legacy token corp = %q, want corp_primary", legacyToken.CorpID)
	}
}

func TestProfileSwitchRootCommandSupportsCorpIDFlag(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "profile", "switch", "--corpId", "corp_primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile switch --corpId error = %v\noutput:\n%s", err, out.String())
	}
	CloseFileLogger()
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_primary:user-corp_primary" {
		t.Fatalf("currentProfile = %q, want corp_primary:user-corp_primary", cfg.CurrentProfile)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "profile", "use", "--corp", "corp_secondary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile use --corp error = %v\noutput:\n%s", err, out.String())
	}
	CloseFileLogger()
	cfg, err = authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_secondary:user-corp_secondary" {
		t.Fatalf("currentProfile = %q, want corp_secondary:user-corp_secondary", cfg.CurrentProfile)
	}
}

func TestProfileSwitchRootCommandRejectsConflictingSelectors(t *testing.T) {
	setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"profile", "switch", "corp_primary", "--corpId", "corp_secondary"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("profile switch with conflicting selectors succeeded\noutput:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "只能指定一个组织选择器") {
		t.Fatalf("error = %v, want conflicting selector validation", err)
	}
}

func TestProfileSwitchNoArgsUsesTUISelector(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)
	oldSelector := profileSwitchSelector
	t.Cleanup(func() {
		profileSwitchSelector = oldSelector
	})
	called := false
	profileSwitchSelector = func(cmd *cobra.Command, gotConfigDir string) (string, error) {
		called = true
		if gotConfigDir != configDir {
			t.Fatalf("configDir = %q, want %q", gotConfigDir, configDir)
		}
		return "corp_primary", nil
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"profile", "switch"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile switch error = %v\noutput:\n%s", err, out.String())
	}
	if !called {
		t.Fatal("profile switch without args did not invoke TUI selector")
	}
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_primary org")) {
		t.Fatalf("profile switch TUI path should use human output by default:\n%s", out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_primary:user-corp_primary" {
		t.Fatalf("currentProfile = %q, want corp_primary:user-corp_primary", cfg.CurrentProfile)
	}
}

func TestProfileSwitchOptionLabelUsesOnlyOrganizationAndCurrentState(t *testing.T) {
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: "corp_primary",
		CurrentProfile: "corp_secondary",
		Profiles: []authpkg.Profile{
			{
				CorpID:   "corp_primary",
				CorpName: "第一组织",
				UserName: "alice",
				Status:   authpkg.ProfileStatusActive,
			},
			{
				CorpID:   "corp_secondary",
				CorpName: "第二组织",
				UserName: "bob",
				Status:   authpkg.ProfileStatusActive,
			},
		},
	}
	primary := profileSwitchOptionLabel(cfg.Profiles[0], cfg)
	current := profileSwitchOptionLabel(cfg.Profiles[1], cfg)
	for _, label := range []string{primary, current} {
		if strings.Contains(label, "\n") {
			t.Fatalf("profile switch label contains newline: %q", label)
		}
	}
	if !strings.Contains(primary, "第一组织") {
		t.Fatalf("primary option missing organization name: %q", primary)
	}
	if !strings.Contains(current, "当前组织") {
		t.Fatalf("current option missing current marker: %q", current)
	}
	for _, unwanted := range []string{"alice", "bob", "已登录", "主组织", "corp_primary", "corp_secondary"} {
		if strings.Contains(primary, unwanted) || strings.Contains(current, unwanted) {
			t.Fatalf("profile switch option should not contain %q: %q / %q", unwanted, primary, current)
		}
	}
}

func TestProfileSwitchTUIViewUsesFixedOuterTable(t *testing.T) {
	cfg := profileSwitchTestConfig(2)
	model := newProfileSwitchTUIModel(cfg, "corp_00")
	view := model.tableView()
	if lines := strings.Split(view, "\n"); len(lines) != profileSwitchVisibleOptions+4 {
		t.Fatalf("table line count = %d, want %d:\n%s", len(lines), profileSwitchVisibleOptions+4, view)
	}
	for _, want := range []string{"┌", "┬", "┐", "├", "┼", "┤", "└", "┴", "┘", "组织名", "本地状态"} {
		if !strings.Contains(view, want) {
			t.Fatalf("profile switch table missing %q in:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"CORP_ID", "ORGANIZATION", "STATUS"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("profile switch table should not contain %q:\n%s", unwanted, view)
		}
	}
	if got := strings.Count(view, "│"); got != (profileSwitchVisibleOptions+1)*3 {
		t.Fatalf("table vertical separators = %d, want %d\n%s", got, (profileSwitchVisibleOptions+1)*3, view)
	}
	for _, profile := range cfg.Profiles {
		if got := strings.Count(view, profile.CorpID); got != 0 {
			t.Fatalf("profile corpId %s appears %d times, want hidden:\n%s", profile.CorpID, got, view)
		}
	}
}

func TestProfileSwitchTUIPreservesStoredOrderInsteadOfSortingByTime(t *testing.T) {
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: "old",
		CurrentProfile: "old",
		Profiles: []authpkg.Profile{
			{CorpID: "old", CorpName: "旧组织", LastLoginAt: "2026-06-26T10:00:00+08:00"},
			{CorpID: "new", CorpName: "新组织", LastLoginAt: "2026-06-26T12:00:00+08:00"},
			{CorpID: "fallback", CorpName: "兜底组织", UpdatedAt: "2026-06-26T11:00:00+08:00"},
		},
	}
	model := newProfileSwitchTUIModel(cfg, "old")
	gotOrder := []string{model.profiles[0].CorpID, model.profiles[1].CorpID, model.profiles[2].CorpID}
	wantOrder := []string{"old", "new", "fallback"}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("profile order = %v, want %v", gotOrder, wantOrder)
	}
	if got := model.selectedCorpID(); got != "old" {
		t.Fatalf("selectedCorpID = %q, want old", got)
	}
}

func TestProfileSwitchTUIArrowKeysMoveSelectionWithoutDuplicatingRows(t *testing.T) {
	cfg := profileSwitchTestConfig(7)
	model := newProfileSwitchTUIModel(cfg, "corp_00")
	for step := 0; step < 6; step++ {
		view := model.tableView()
		if got := strings.Count(view, "›"); got != 1 {
			t.Fatalf("step %d selected cursor count = %d, want 1:\n%s", step, got, view)
		}
		for _, profile := range cfg.Profiles {
			name := profileOrgName(profile)
			if got := strings.Count(view, name); got > 1 {
				t.Fatalf("step %d profile %s appears %d times, want at most once:\n%s", step, name, got, view)
			}
		}
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = next.(profileSwitchTUIModel)
	}
	if model.selected != 6 || model.offset != 2 {
		t.Fatalf("selection after down keys = selected %d offset %d, want 6/2", model.selected, model.offset)
	}
}

func TestProfileSwitchTableRowsKeepFixedDisplayWidth(t *testing.T) {
	rows := []string{
		profileSwitchTableLine("组织名", "本地状态"),
		profileSwitchTableLine("› 钉钉（中国）信息技术有限公司", "当前组织"),
		profileSwitchTableLine("  ACME", ""),
		profileSwitchTableLine("", ""),
		profileSwitchStyledTableLine("组织名", "本地状态", profileSwitchHeaderStyle()),
		profileSwitchStyledTableLine("› 钉钉（中国）信息技术有限公司", "当前组织", profileSwitchSelectedRowStyle()),
		profileSwitchStyledTableLine("  ACME", "", profileSwitchNormalRowStyle()),
		profileSwitchStyledTableLine("", "", profileSwitchNormalRowStyle()),
	}
	wantWidth := lipgloss.Width(rows[0])
	for i, row := range rows {
		if got := lipgloss.Width(row); got != wantWidth {
			t.Fatalf("row[%d] width = %d, want %d: %q", i, got, wantWidth, row)
		}
		if got := strings.Count(row, "│"); got != 3 {
			t.Fatalf("row[%d] separator count = %d, want 3: %q", i, got, row)
		}
	}
}

func TestProfileSwitchOptionLabelHidesCorpID(t *testing.T) {
	const corpID = "ding8196cd9a2b2405da24f2f5cc6abecb85"
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: corpID,
		CurrentProfile: corpID,
	}
	label := profileSwitchOptionLabel(authpkg.Profile{
		CorpID:   corpID,
		CorpName: "钉钉",
	}, cfg)
	for _, want := range []string{"钉钉", "当前组织"} {
		if !strings.Contains(label, want) {
			t.Fatalf("profile switch label missing %q in %q", want, label)
		}
	}
	for _, unwanted := range []string{"ding8196", "cb85", "主组织"} {
		if strings.Contains(label, unwanted) {
			t.Fatalf("profile switch label should not contain %q in %q", unwanted, label)
		}
	}
}

func profileSwitchTestConfig(count int) *authpkg.ProfilesConfig {
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: "corp_00",
		CurrentProfile: "corp_00",
	}
	for i := 0; i < count; i++ {
		corpID := fmt.Sprintf("corp_%02d", i)
		cfg.Profiles = append(cfg.Profiles, authpkg.Profile{
			CorpID:   corpID,
			CorpName: fmt.Sprintf("组织%02d", i),
			Status:   authpkg.ProfileStatusActive,
		})
	}
	return cfg
}

func TestAuthCommandDoesNotExposeSwitch(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "switch"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("auth switch succeeded, want unknown command error\noutput:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), `unknown command "switch" for "dws auth"`) {
		t.Fatalf("error = %v, want auth switch unknown command", err)
	}
}

func TestProfileUseNoArgsUsesTUISelector(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)
	oldSelector := profileSwitchSelector
	t.Cleanup(func() {
		profileSwitchSelector = oldSelector
	})
	profileSwitchSelector = func(cmd *cobra.Command, gotConfigDir string) (string, error) {
		if gotConfigDir != configDir {
			t.Fatalf("configDir = %q, want %q", gotConfigDir, configDir)
		}
		return "corp_primary", nil
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"profile", "use"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile use error = %v\noutput:\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_primary org")) {
		t.Fatalf("profile use TUI path should use human output by default:\n%s", out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_primary:user-corp_primary" {
		t.Fatalf("currentProfile = %q, want corp_primary:user-corp_primary", cfg.CurrentProfile)
	}
}

func TestProfileSwitchSelectorRequiresInteractiveTerminal(t *testing.T) {
	oldInteractive := profileSwitchInteractiveTerminal
	t.Cleanup(func() {
		profileSwitchInteractiveTerminal = oldInteractive
	})
	profileSwitchInteractiveTerminal = func() bool { return false }

	_, err := selectProfileSwitchProfile(nil, t.TempDir())
	if err == nil {
		t.Fatal("selectProfileSwitchProfile() succeeded, want validation error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("profile selector required")) {
		t.Fatalf("error = %v, want profile selector hint", err)
	}
}

func TestWriteProfileListTableIncludesCorpName(t *testing.T) {
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: "corp_a",
		CurrentProfile: "corp_b",
		Profiles: []authpkg.Profile{
			{
				Name:     "DingTalk China",
				CorpID:   "corp_a",
				CorpName: "钉钉（中国）信息技术有限公司",
				UserName: "alice",
				Status:   authpkg.ProfileStatusActive,
			},
			{
				Name:     "B Org",
				CorpID:   "corp_b",
				CorpName: "B 组织",
				UserID:   "bob-id",
			},
		},
	}
	var buf bytes.Buffer
	writeProfileListTable(&buf, "", cfg)
	out := buf.String()
	for _, want := range []string{
		"ORG_NAME",
		"钉钉（中国）信息技术有限公司",
		"B 组织",
		"corp_a",
		"corp_b",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Fatalf("profile list table missing %q in output:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"PROFILE", "DingTalk China"} {
		if bytes.Contains(buf.Bytes(), []byte(unwanted)) {
			t.Fatalf("profile list table should not contain %q in output:\n%s", unwanted, out)
		}
	}
}

func TestProfileUseMessageIncludesCorpName(t *testing.T) {
	got := profileUseMessage(&authpkg.Profile{
		Name:     "DingTalk China",
		CorpID:   "ding8196",
		CorpName: "钉钉（中国）信息技术有限公司",
	})
	for _, want := range []string{"当前组织: 钉钉（中国）信息技术有限公司", "ding8196"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("profileUseMessage() missing %q in %q", want, got)
		}
	}
	if bytes.Contains([]byte(got), []byte("DingTalk China")) {
		t.Fatalf("profileUseMessage() should not include profile name when corpName is present: %q", got)
	}
}
