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

package configmeta

import (
	"os"
	"testing"
)

func TestRegisterAndAll(t *testing.T) {
	Reset()
	defer Reset()

	Register(ConfigItem{Name: "ZZZ_LAST", Category: CategoryDebug, Description: "last"})
	Register(ConfigItem{Name: "AAA_FIRST", Category: CategoryCore, Description: "first"})
	Register(ConfigItem{Name: "MMM_MID", Category: CategoryAuth, Description: "mid"})

	all := All()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	// core < auth < debug
	if all[0].Name != "AAA_FIRST" {
		t.Errorf("expected AAA_FIRST first, got %s", all[0].Name)
	}
	if all[1].Name != "MMM_MID" {
		t.Errorf("expected MMM_MID second, got %s", all[1].Name)
	}
	if all[2].Name != "ZZZ_LAST" {
		t.Errorf("expected ZZZ_LAST third, got %s", all[2].Name)
	}
}

func TestRegisterDuplicateIgnored(t *testing.T) {
	Reset()
	defer Reset()

	Register(ConfigItem{Name: "DUP", Category: CategoryCore, Description: "original"})
	Register(ConfigItem{Name: "DUP", Category: CategoryCore, Description: "duplicate"})

	all := All()
	if len(all) != 1 {
		t.Fatalf("expected 1 item, got %d", len(all))
	}
	if all[0].Description != "original" {
		t.Errorf("expected original description, got %q", all[0].Description)
	}
}

func TestByCategory(t *testing.T) {
	Reset()
	defer Reset()

	Register(ConfigItem{Name: "A", Category: CategoryCore})
	Register(ConfigItem{Name: "B", Category: CategoryAuth})
	Register(ConfigItem{Name: "C", Category: CategoryCore})

	core := ByCategory(CategoryCore)
	if len(core) != 2 {
		t.Fatalf("expected 2 core items, got %d", len(core))
	}

	empty := ByCategory(CategoryDebug)
	if len(empty) != 0 {
		t.Fatalf("expected 0 debug items, got %d", len(empty))
	}
}

func TestResolveNonSensitive(t *testing.T) {
	Reset()
	defer Reset()

	Register(ConfigItem{Name: "TEST_VAR_PLAIN", Category: CategoryCore})

	t.Setenv("TEST_VAR_PLAIN", "hello")
	val, ok := Resolve("TEST_VAR_PLAIN")
	if !ok || val != "hello" {
		t.Errorf("expected (hello, true), got (%q, %v)", val, ok)
	}
}

func TestResolveSensitiveMasked(t *testing.T) {
	Reset()
	defer Reset()

	Register(ConfigItem{Name: "TEST_SECRET", Category: CategoryAuth, Sensitive: true})

	t.Setenv("TEST_SECRET", "abcdefgh")
	val, ok := Resolve("TEST_SECRET")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val == "abcdefgh" {
		t.Error("sensitive value should be masked")
	}
	// ab****gh
	if val != "ab****gh" {
		t.Errorf("unexpected masked value: %q", val)
	}
}

func TestResolveUnset(t *testing.T) {
	Reset()
	defer Reset()

	Register(ConfigItem{Name: "UNSET_VAR", Category: CategoryCore})
	os.Unsetenv("UNSET_VAR")

	_, ok := Resolve("UNSET_VAR")
	if ok {
		t.Error("expected ok=false for unset variable")
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"ab", "**"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"abcdefghij", "ab******ij"},
	}
	for _, tc := range tests {
		got := maskValue(tc.in)
		if got != tc.want {
			t.Errorf("maskValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCategories(t *testing.T) {
	cats := Categories()
	if len(cats) != 7 {
		t.Fatalf("expected 7 categories, got %d", len(cats))
	}
	if cats[0] != CategoryCore {
		t.Errorf("expected core first, got %s", cats[0])
	}
	if cats[len(cats)-1] != CategoryExternal {
		t.Errorf("expected external last, got %s", cats[len(cats)-1])
	}
}

func TestReset(t *testing.T) {
	Reset()
	Register(ConfigItem{Name: "X", Category: CategoryCore})
	Reset()
	if len(All()) != 0 {
		t.Error("expected empty registry after Reset")
	}
}
