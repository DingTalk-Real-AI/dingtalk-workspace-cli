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
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Verifies that two dws binaries from different editions sharing the same
// configDir (e.g. ~/.dws via DWS_CONFIG_DIR) read and write disjoint
// app.json files. Without partitioning, a sibling edition's post-login
// persistence path could leak its pinned ClientID into the open-source
// build by reading the shared file.

func TestGetAppConfigPath_OpenEditionUsesLegacyName(t *testing.T) {
	prev := edition.Get()
	t.Cleanup(func() { edition.Override(prev) })

	for _, name := range []string{"", "open"} {
		edition.Override(&edition.Hooks{Name: name})
		got := GetAppConfigPath("/tmp/cfg")
		want := filepath.Join("/tmp/cfg", "app.json")
		if got != want {
			t.Fatalf("edition=%q: GetAppConfigPath = %q, want %q", name, got, want)
		}
	}
}

func TestGetAppConfigPath_SiblingEditionUsesSuffixedName(t *testing.T) {
	prev := edition.Get()
	t.Cleanup(func() { edition.Override(prev) })

	cases := []struct {
		editionName string
		wantFile    string
	}{
		{"wukong", "app-wukong.json"},
		{"dev", "app-dev.json"},
		{"embedded", "app-embedded.json"},
	}
	for _, tc := range cases {
		edition.Override(&edition.Hooks{Name: tc.editionName})
		got := GetAppConfigPath("/tmp/cfg")
		want := filepath.Join("/tmp/cfg", tc.wantFile)
		if got != want {
			t.Fatalf("edition=%q: GetAppConfigPath = %q, want %q", tc.editionName, got, want)
		}
	}
}

func TestGetAppConfigPath_OpenAndSiblingAreDisjoint(t *testing.T) {
	// End-to-end invariant: when the same configDir is observed from two
	// different editions, the resulting app.json paths must NOT collide.
	prev := edition.Get()
	t.Cleanup(func() { edition.Override(prev) })

	const cfg = "/tmp/shared-cfg"

	edition.Override(&edition.Hooks{Name: "open"})
	openPath := GetAppConfigPath(cfg)

	edition.Override(&edition.Hooks{Name: "wukong"})
	wukongPath := GetAppConfigPath(cfg)

	if openPath == wukongPath {
		t.Fatalf("open and wukong editions share path %q; cross-edition leakage possible", openPath)
	}
	if filepath.Dir(openPath) != filepath.Dir(wukongPath) {
		t.Fatalf("paths landed in different directories (%q vs %q); partitioning should only differ by filename", filepath.Dir(openPath), filepath.Dir(wukongPath))
	}
}

func TestAppConfigIO_OpenEditionDoesNotReadSiblingCredentials(t *testing.T) {
	prev := edition.Get()
	t.Cleanup(func() {
		edition.Override(prev)
		resetAppConfigCache()
	})

	configDir := t.TempDir()

	edition.Override(&edition.Hooks{Name: "wukong"})
	wukongPath := GetAppConfigPath(configDir)
	if err := os.WriteFile(wukongPath, []byte(`{"clientId":"wukong-cid","createdAt":"2026-05-17T00:00:00+08:00"}`+"\n"), 0600); err != nil {
		t.Fatalf("writing sibling app config: %v", err)
	}

	edition.Override(&edition.Hooks{Name: "open"})
	got, err := LoadAppConfig(configDir)
	if err != nil {
		t.Fatalf("LoadAppConfig(open) error = %v", err)
	}
	if got != nil {
		t.Fatalf("open edition read sibling app config: %#v", got)
	}
}

func TestSaveAppConfig_SiblingEditionRemovesMatchingLegacyAppConfig(t *testing.T) {
	prev := edition.Get()
	t.Cleanup(func() {
		edition.Override(prev)
		resetAppConfigCache()
	})

	configDir := t.TempDir()
	legacyPath := filepath.Join(configDir, appConfigFile)
	legacyJSON := []byte(`{"clientId":"wukong-cid","createdAt":"2026-05-17T00:00:00+08:00"}` + "\n")
	if err := os.WriteFile(legacyPath, legacyJSON, 0600); err != nil {
		t.Fatalf("writing legacy app config: %v", err)
	}

	edition.Override(&edition.Hooks{Name: "wukong"})
	if err := SaveAppConfig(configDir, &AppConfig{ClientID: "wukong-cid"}); err != nil {
		t.Fatalf("SaveAppConfig(wukong) error = %v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("matching legacy app config should be removed, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "app-wukong.json")); err != nil {
		t.Fatalf("sibling app config not written: %v", err)
	}
}

func TestSaveAppConfig_SiblingEditionKeepsDifferentLegacyAppConfig(t *testing.T) {
	prev := edition.Get()
	t.Cleanup(func() {
		edition.Override(prev)
		resetAppConfigCache()
	})

	configDir := t.TempDir()
	legacyPath := filepath.Join(configDir, appConfigFile)
	legacyJSON := []byte(`{"clientId":"open-cid","createdAt":"2026-05-17T00:00:00+08:00"}` + "\n")
	if err := os.WriteFile(legacyPath, legacyJSON, 0600); err != nil {
		t.Fatalf("writing legacy app config: %v", err)
	}

	edition.Override(&edition.Hooks{Name: "wukong"})
	if err := SaveAppConfig(configDir, &AppConfig{ClientID: "wukong-cid"}); err != nil {
		t.Fatalf("SaveAppConfig(wukong) error = %v", err)
	}

	got, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("different legacy app config should be preserved: %v", err)
	}
	if string(got) != string(legacyJSON) {
		t.Fatalf("legacy app config changed: got %q, want %q", got, legacyJSON)
	}
}
