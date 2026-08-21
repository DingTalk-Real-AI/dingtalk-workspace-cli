// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageContactSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_contact.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "contact" {
		t.Fatalf("service = %q", source.Service)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "contact" {
			registered[item.Command] = item
		}
	}
	if len(registered) != 16 || len(source.Shortcuts) != 16 {
		t.Fatalf("registered/catalog = %d/%d, want 16/16", len(registered), len(source.Shortcuts))
	}
	wantCompatibilityVisible := map[string]bool{"+list-roles": true}
	public, unavailable := 0, 0
	for command, record := range source.Shortcuts {
		item, ok := registered[command]
		if !ok {
			t.Errorf("catalog contains stale %s", command)
			continue
		}
		availability := record.Availability
		if availability == "" {
			availability = source.Availability
		}
		if !record.Reviewed || !item.SemanticReviewed || item.SemanticDelta != record.SemanticDelta || item.Risk != record.Risk {
			t.Errorf("%s semantic facts drifted", command)
		}
		if item.Contract.Empty() || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Errorf("%s lacks Contract/Safety", command)
		}
		if record.Public {
			public++
			if availability != shortcut.AvailabilityAvailable || item.Hidden || !shortcut.InPublicCatalog("contact", command) || item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Result == nil {
				t.Errorf("%s public availability drift", command)
			}
		} else {
			unavailable++
			if availability != shortcut.AvailabilityUnavailable || shortcut.InPublicCatalog("contact", command) || item.OutputRollout != output.RolloutLegacyOnly || item.Contract.Result != nil {
				t.Errorf("%s unavailable visibility drift", command)
			}
			if record.CompatibilityVisible {
				if item.Hidden || !item.CompatibilityVisible {
					t.Errorf("%s compatibility visibility drift", command)
				}
			} else if !item.Hidden || item.CompatibilityVisible {
				t.Errorf("%s hidden unavailable visibility drift", command)
			}
		}
		if record.CompatibilityVisible != wantCompatibilityVisible[command] {
			t.Errorf("%s compatibility-visible=%v, want %v", command, record.CompatibilityVisible, wantCompatibilityVisible[command])
		}
		if command == "+search-mobile" || command == "+by-mobile" {
			if !strings.Contains(record.SemanticDelta, "专用手机号精确查询接口") {
				t.Errorf("%s must describe the dedicated exact mobile interface", command)
			}
			if strings.Contains(record.SemanticDelta, "关键词能力") || strings.Contains(record.SemanticDelta, "关键词数组") {
				t.Errorf("%s must not describe the retired keyword-search route", command)
			}
		}
		if command == "+list-roster-fields" || command == "+get-roster" {
			if !strings.Contains(record.SemanticDelta, "历史 CLI 继续透传原 MCP 调用与真实错误") {
				t.Errorf("%s must preserve the historical direct CLI execution path", command)
			}
			if strings.Contains(record.SemanticDelta, "0 次远程调用") {
				t.Errorf("%s must not claim a zero-call unavailable runtime", command)
			}
		}
	}
	if public != 13 || unavailable != 3 {
		t.Fatalf("public/unavailable = %d/%d, want 13/3", public, unavailable)
	}
}
