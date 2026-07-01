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
	"errors"
	"os"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
)

func writeEventTestAppConfig(t *testing.T, dir string, cfg authpkg.AppConfig) {
	t.Helper()
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal app config: %v", err)
	}
	if err := os.WriteFile(authpkg.GetAppConfigPath(dir), raw, 0o600); err != nil {
		t.Fatalf("write app config: %v", err)
	}
}

func TestResolveEventCredentials_PortalNormalAllowsMissingClientSecret(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	dir := t.TempDir()

	clientID, clientSecret, err := resolveEventCredentials(dir, eventStreamTicketOptions{
		Mode:     source.PortalTicketModeNormal,
		SourceID: "pre_open_source",
	})
	if err != nil {
		t.Fatalf("resolveEventCredentials: %v", err)
	}
	if clientID != "portal-ticket-normal:pre_open_source" {
		t.Fatalf("clientID = %q, want portal-ticket-normal:pre_open_source", clientID)
	}
	if clientSecret != "" {
		t.Fatalf("clientSecret = %q, want empty", clientSecret)
	}
}

func TestResolveEventCredentials_PortalCustomStillRequiresClientSecret(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	dir := t.TempDir()
	writeEventTestAppConfig(t, dir, authpkg.AppConfig{ClientID: "ding-custom"})

	_, _, err := resolveEventCredentials(dir, eventStreamTicketOptions{
		Mode: source.PortalTicketModeCustom,
	})
	if !errors.Is(err, authpkg.ErrClientSecretEmpty) {
		t.Fatalf("err = %v, want ErrClientSecretEmpty", err)
	}
}
