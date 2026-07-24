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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestResolvePersonalEventIdentityUsesCorpUserWhenAvailable(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp-1",
		UserID:       "user-1",
		ClientID:     "client-1",
	})

	identity, err := resolvePersonalEventIdentity(context.Background(), configDir, "pre_open_source")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentity() error = %v", err)
	}
	if identity.LocalSubject != "" {
		t.Fatalf("LocalSubject = %q, want empty when corp/user are available", identity.LocalSubject)
	}
	wantKey := "corp_user\x00corp-1\x00user-1\x00client-1\x00pre_open_source"
	if got := identity.Key(); got != wantKey {
		t.Fatalf("identity key = %q, want %q", got, wantKey)
	}
}

func TestResolvePersonalEventIdentityFallsBackToRefreshTokenSubject(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		ClientID:     "client-1",
	})

	identity, err := resolvePersonalEventIdentity(context.Background(), configDir, "pre_open_source")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentity() error = %v", err)
	}
	wantSubject := personalTokenSubject("refresh", "refresh-1")
	if identity.LocalSubject != wantSubject {
		t.Fatalf("LocalSubject = %q, want %q", identity.LocalSubject, wantSubject)
	}
	if strings.Contains(identity.Key(), "refresh-1") || strings.Contains(identity.Key(), "access-1") {
		t.Fatalf("identity key leaked raw token: %q", identity.Key())
	}

	body, err := json.Marshal(redactedPersonalIdentity(identity, "identity-hash-1"))
	if err != nil {
		t.Fatalf("marshal redacted identity: %v", err)
	}
	if strings.Contains(string(body), wantSubject) || strings.Contains(string(body), "refresh-1") || strings.Contains(string(body), "access-1") {
		t.Fatalf("redacted identity leaked local subject/token: %s", string(body))
	}
	if !strings.Contains(string(body), "unknown") {
		t.Fatalf("redacted identity should mark missing corp/user as unknown: %s", string(body))
	}
}

func TestResolvePersonalEventIdentityFallsBackToAccessTokenSubject(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken: "access-1",
		ExpiresAt:   time.Now().Add(time.Hour),
		ClientID:    "client-1",
	})

	identity, err := resolvePersonalEventIdentity(context.Background(), configDir, "pre_open_source")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentity() error = %v", err)
	}
	wantSubject := personalTokenSubject("access", "access-1")
	if identity.LocalSubject != wantSubject {
		t.Fatalf("LocalSubject = %q, want %q", identity.LocalSubject, wantSubject)
	}

	var out bytes.Buffer
	renderPersonalStatusText(&out, identity, "identity-hash-1", nil, busctl.EntryStatus{
		Entry: busctl.BusEntry{WorkDir: "wd", State: busctl.BusStateNotRunning},
	})
	rendered := out.String()
	if !strings.Contains(rendered, "corp=unknown user=unknown") {
		t.Fatalf("status output = %q, want unknown corp/user", rendered)
	}
	if strings.Contains(rendered, wantSubject) || strings.Contains(rendered, "access-1") {
		t.Fatalf("status output leaked local subject/token: %q", rendered)
	}
}

func TestResolvePersonalEventIdentityDefaultsSourceIDToOpen(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken: "access-1",
		ExpiresAt:   time.Now().Add(time.Hour),
		CorpID:      "corp-1",
		UserID:      "user-1",
		ClientID:    "client-1",
	})

	identity, err := resolvePersonalEventIdentity(context.Background(), configDir, "")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentity() error = %v", err)
	}
	if identity.SourceID != "open" {
		t.Fatalf("SourceID = %q, want open", identity.SourceID)
	}
}

func TestPersonalEventDefaultsUseProductionWithoutMCPConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	prev := edition.Get()
	edition.Override(&edition.Hooks{})
	t.Cleanup(func() { edition.Override(prev) })

	if got := personalEventControlBaseURL("", dir); got != "https://mcp.dingtalk.com/dws" {
		t.Fatalf("personalEventControlBaseURL() = %q, want production control URL", got)
	}
	if got := personalEventStreamTicketURL("", dir); got != "https://mcp.dingtalk.com/stream/connections/ticket" {
		t.Fatalf("personalEventStreamTicketURL() = %q, want production ticket URL", got)
	}
	if got := personalEventStreamSourceID(""); got != "open" {
		t.Fatalf("personalEventStreamSourceID() = %q, want open", got)
	}
	if got := config.GetMCPBaseURL(); got != "https://mcp.dingtalk.com" {
		t.Fatalf("config.GetMCPBaseURL() = %q, want production MCP URL", got)
	}
}

func TestPersonalEventDefaultsRespectExplicitAndMCPConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte("https://custom-mcp.example.com\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}

	if got := personalEventControlBaseURL("", dir); got != "https://custom-mcp.example.com/dws" {
		t.Fatalf("personalEventControlBaseURL() = %q, want configured control URL", got)
	}
	if got := personalEventStreamTicketURL("", dir); got != "https://custom-mcp.example.com/stream/connections/ticket" {
		t.Fatalf("personalEventStreamTicketURL() = %q, want configured ticket URL", got)
	}
	if got := personalEventControlBaseURL(" https://override.example.com/dws/ ", dir); got != "https://override.example.com/dws" {
		t.Fatalf("explicit control URL = %q, want trimmed override", got)
	}
	if got := personalEventStreamTicketURL(" https://override.example.com/ticket/ ", dir); got != "https://override.example.com/ticket" {
		t.Fatalf("explicit ticket URL = %q, want trimmed override", got)
	}
	if got := personalEventStreamSourceID("flag_source"); got != "flag_source" {
		t.Fatalf("explicit sourceID = %q, want flag_source", got)
	}
}

func TestPersonalEventSourceIDPrefersEditionOverride(t *testing.T) {
	prev := edition.Get()
	edition.Override(&edition.Hooks{PersonalEventSourceID: "edition_source"})
	t.Cleanup(func() { edition.Override(prev) })

	if got := personalEventStreamSourceID(""); got != "edition_source" {
		t.Fatalf("personalEventStreamSourceID() = %q, want edition_source", got)
	}
	if got := personalEventStreamSourceID("flag_source"); got != "flag_source" {
		t.Fatalf("explicit sourceID = %q, want flag_source", got)
	}
}

func setupPersonalIdentityToken(t *testing.T, data *authpkg.TokenData) string {
	t.Helper()
	configDir := t.TempDir()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal token data: %v", err)
	}
	prev := edition.Get()
	edition.Override(&edition.Hooks{
		LoadToken: func(dir string) ([]byte, error) {
			if filepath.Clean(dir) != filepath.Clean(configDir) {
				t.Fatalf("LoadToken dir = %q, want %q", dir, configDir)
			}
			return raw, nil
		},
	})
	t.Cleanup(func() { edition.Override(prev) })
	return configDir
}
