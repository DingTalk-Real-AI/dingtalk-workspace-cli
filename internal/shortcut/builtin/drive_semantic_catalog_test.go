// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageDriveSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_drive.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "drive" {
		t.Fatalf("semantic catalog service = %q", source.Service)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service != "drive" {
			continue
		}
		if _, duplicate := registered[item.Command]; duplicate {
			t.Fatalf("duplicate registered Drive Shortcut %s", item.Command)
		}
		registered[item.Command] = item
	}
	if len(registered) != 29 || len(source.Shortcuts) != 29 {
		t.Fatalf("registered/catalog = %d/%d, want 29/29", len(registered), len(source.Shortcuts))
	}
	var missing, stale []string
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		if !record.Reviewed || !item.SemanticReviewed {
			t.Errorf("%s: reviewed delivery mismatch", command)
		}
		if got := shortcut.InPublicCatalog("drive", command); got != record.Public || item.Hidden == record.Public {
			t.Errorf("%s: public/hidden mismatch: catalog=%v runtimeHidden=%v", command, record.Public, item.Hidden)
		}
		if strings.TrimSpace(record.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta || item.Disposition != record.Disposition {
			t.Errorf("%s: semantic catalog facts drifted", command)
		}
		if item.Risk != record.Risk {
			t.Errorf("%s: risk = %q, want %q", command, item.Risk, record.Risk)
		}
		if item.Contract.Empty() || strings.TrimSpace(item.Safety.Effect) == "" || strings.TrimSpace(item.Safety.Confirmation) == "" {
			t.Errorf("%s: incomplete explicit contract/safety", command)
		}
		if command != "+find-file" && record.Public {
			if item.OutputRollout != output.RolloutUnifiedActive {
				t.Errorf("%s: output rollout = %q", command, item.OutputRollout)
			}
			if item.Contract.Result == nil {
				t.Errorf("%s: public Drive shortcut lacks Result declaration", command)
			}
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
