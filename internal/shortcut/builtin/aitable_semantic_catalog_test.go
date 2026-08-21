// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageAITableSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_aitable.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "aitable" {
		t.Fatalf("semantic catalog service = %q, want aitable", source.Service)
	}

	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service != "aitable" {
			continue
		}
		if _, duplicate := registered[item.Command]; duplicate {
			t.Fatalf("duplicate registered AI Table Shortcut %s", item.Command)
		}
		registered[item.Command] = item
	}
	if len(registered) != 100 || len(source.Shortcuts) != 100 {
		t.Fatalf("registered/catalog = %d/%d, want 100/100", len(registered), len(source.Shortcuts))
	}

	var missing, stale []string
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		if !record.Reviewed || !item.SemanticReviewed {
			t.Errorf("%s: semantic decision is not reviewed", command)
		}
		if strings.TrimSpace(record.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta {
			t.Errorf("%s: semantic delta was not delivered exactly", command)
		}
		if item.Disposition != record.Disposition {
			t.Errorf("%s: disposition = %q, want %q", command, item.Disposition, record.Disposition)
		}
		if item.Risk != record.Risk {
			t.Errorf("%s: runtime risk = %q, reviewed risk = %q", command, item.Risk, record.Risk)
		}
		if !record.Public || item.Hidden || !shortcut.InPublicCatalog("aitable", command) {
			t.Errorf("%s: reviewed available AI Table Shortcut must be public", command)
		}
		if item.Contract.Empty() {
			t.Errorf("%s: public AI Table Shortcut has empty Contract", command)
			continue
		}
		if strings.TrimSpace(item.Safety.Effect) == "" || strings.TrimSpace(item.Safety.Risk) == "" ||
			strings.TrimSpace(item.Safety.Confirmation) == "" || strings.TrimSpace(item.Safety.Idempotency) == "" {
			t.Errorf("%s: public AI Table Shortcut has incomplete explicit Safety: %#v", command, item.Safety)
		}
		wantCLIPath := "aitable " + command
		if item.Contract.Identity.CLIPath != wantCLIPath || item.Contract.Identity.PrimaryCLIPath != wantCLIPath {
			t.Errorf("%s: identity paths = %q/%q, want %q", command,
				item.Contract.Identity.CLIPath, item.Contract.Identity.PrimaryCLIPath, wantCLIPath)
		}
		if len(item.Contract.Selection.UseWhen) == 0 || len(item.Contract.Selection.AvoidWhen) == 0 {
			t.Errorf("%s: selection contract must contain positive and negative routing", command)
		}
	}
	for command := range source.Shortcuts {
		if _, ok := registered[command]; !ok {
			stale = append(stale, command)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("semantic catalog mismatch: missing=%v stale=%v", missing, stale)
	}
}
