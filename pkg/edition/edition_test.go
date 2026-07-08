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

package edition

import "testing"

func TestClawTypeDefaultsToOSSValue(t *testing.T) {
	prev := Get()
	defer Override(prev)

	Override(defaultHooks())
	if got := ClawType(); got != DefaultOSSClawType {
		t.Fatalf("ClawType() = %q, want %q", got, DefaultOSSClawType)
	}
}

func TestClawTypeUsesOverlayValue(t *testing.T) {
	prev := Get()
	defer Override(prev)

	Override(&Hooks{Name: "overlay", ClawTypeValue: "wukong"})
	if got := ClawType(); got != "wukong" {
		t.Fatalf("ClawType() = %q, want overlay value %q", got, "wukong")
	}
}

func TestOpenStaticServersIncludesCoreProducts(t *testing.T) {
	servers := openStaticServers()
	byID := make(map[string]ServerInfo, len(servers))
	for _, server := range servers {
		byID[server.ID] = server
	}

	required := []string{"aitable", "aitable-helper", "calendar", "todo", "doc", "chat", "mail", "oa"}
	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Errorf("openStaticServers() missing required product %q", id)
		}
	}

	helper := byID["aitable-helper"]
	if helper.Endpoint == "" {
		t.Fatal("aitable-helper has empty endpoint")
	}
}

func TestOpenVisibleProductsExcludesCompatibilityOnlyCommands(t *testing.T) {
	visible := openVisibleProducts()
	byID := make(map[string]bool, len(visible))
	for _, id := range visible {
		byID[id] = true
	}
	if byID["conference"] {
		t.Fatal("conference must remain hidden compatibility-only and not appear in VisibleProducts")
	}

	for _, server := range openStaticServers() {
		if server.ID == "conference" {
			t.Fatal("conference must remain compatibility-only and not be added to StaticServers")
		}
	}
}
