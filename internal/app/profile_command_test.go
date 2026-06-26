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
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
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
	if resp.PrimaryProfile != "corp_primary" || resp.CurrentProfile != "corp_secondary" || resp.PreviousProfile != "corp_primary" {
		t.Fatalf("profile pointers = primary %q current %q previous %q, want corp_primary/corp_secondary/corp_primary", resp.PrimaryProfile, resp.CurrentProfile, resp.PreviousProfile)
	}
	if len(resp.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2", len(resp.Profiles))
	}
	for _, p := range resp.Profiles {
		if p.CorpName == "" {
			t.Fatalf("profile %s missing corpName in JSON response: %#v", p.CorpID, p)
		}
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
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_primary org")) {
		t.Fatalf("profile use output should include organization name:\n%s", out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_primary" || cfg.PreviousProfile != "corp_secondary" {
		t.Fatalf("profile pointers = current %q previous %q, want corp_primary/corp_secondary", cfg.CurrentProfile, cfg.PreviousProfile)
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
	if !bytes.Contains(out.Bytes(), []byte("组织: corp_secondary org")) {
		t.Fatalf("profile use - output should include organization name:\n%s", out.String())
	}
	cfg, err = authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_secondary" || cfg.PreviousProfile != "corp_primary" {
		t.Fatalf("profile pointers = current %q previous %q, want corp_secondary/corp_primary", cfg.CurrentProfile, cfg.PreviousProfile)
	}
	legacyToken, err = authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if legacyToken.CorpID != "corp_secondary" {
		t.Fatalf("legacy token corp = %q, want corp_secondary", legacyToken.CorpID)
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
	writeProfileListTable(&buf, cfg)
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
}

func TestProfileUseMessageIncludesCorpName(t *testing.T) {
	got := profileUseMessage(&authpkg.Profile{
		Name:     "DingTalk China",
		CorpID:   "ding8196",
		CorpName: "钉钉（中国）信息技术有限公司",
	})
	for _, want := range []string{"DingTalk China", "组织: 钉钉（中国）信息技术有限公司", "ding8196"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("profileUseMessage() missing %q in %q", want, got)
		}
	}
}
