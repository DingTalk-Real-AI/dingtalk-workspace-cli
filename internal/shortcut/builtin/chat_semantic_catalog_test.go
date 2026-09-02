// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

type chatSemanticCatalogFixture struct {
	Service           string                `json:"service"`
	Availability      shortcut.Availability `json:"default_availability"`
	FeaturedShortcuts []string              `json:"featured_shortcuts"`
	Shortcuts         map[string]struct {
		Disposition          shortcut.SemanticDisposition `json:"disposition"`
		SemanticDelta        string                       `json:"semantic_delta"`
		Risk                 shortcut.Risk                `json:"risk"`
		Availability         shortcut.Availability        `json:"availability"`
		Primary              string                       `json:"primary"`
		Public               bool                         `json:"public"`
		CompatibilityVisible bool                         `json:"compatibility_visible"`
		Reviewed             bool                         `json:"reviewed"`
	} `json:"shortcuts"`
}

func TestChatSemanticCatalogExactlyCoversRegisteredShortcuts(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "chat" {
		t.Fatalf("semantic catalog service = %q, want chat", source.Service)
	}

	registered := make(map[string]shortcut.Shortcut)
	helpTierCounts := map[shortcut.HelpTier]int{}
	for _, item := range shortcut.All() {
		if item.Service != "chat" {
			continue
		}
		if _, duplicate := registered[item.Command]; duplicate {
			t.Fatalf("duplicate registered Chat Shortcut %s", item.Command)
		}
		registered[item.Command] = item
		helpTierCounts[item.HelpTier]++
	}
	if got, want := len(registered), 100; got != want {
		t.Fatalf("registered Chat Shortcuts = %d, want %d", got, want)
	}
	if got, want := len(source.Shortcuts), 100; got != want {
		t.Fatalf("reviewed Chat Shortcut records = %d, want %d", got, want)
	}
	if got, want := len(source.FeaturedShortcuts), 26; got != want {
		t.Fatalf("reviewed Chat featured Shortcuts = %d, want %d", got, want)
	}
	for tier, want := range map[shortcut.HelpTier]int{
		shortcut.HelpTierFeatured:      26,
		shortcut.HelpTierCatalog:       67,
		shortcut.HelpTierCompatibility: 5,
		shortcut.HelpTierUnavailable:   2,
	} {
		if got := helpTierCounts[tier]; got != want {
			t.Errorf("Chat help tier %q = %d, want %d", tier, got, want)
		}
	}

	var missing, stale []string
	for command := range registered {
		if _, ok := source.Shortcuts[command]; !ok {
			missing = append(missing, command)
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

	for command, item := range registered {
		record := source.Shortcuts[command]
		if !record.Reviewed || !item.SemanticReviewed {
			t.Errorf("%s: semantic decision is not reviewed", command)
		}
		if strings.TrimSpace(record.SemanticDelta) == "" ||
			item.SemanticDelta != record.SemanticDelta {
			t.Errorf("%s: semantic delta was not delivered exactly", command)
		}
		if item.Disposition != record.Disposition {
			t.Errorf("%s: disposition = %q, want %q", command, item.Disposition, record.Disposition)
		}
		deliveredRisk := item.Risk
		if deliveredRisk == "" {
			deliveredRisk = shortcut.RiskRead
		}
		if deliveredRisk != record.Risk {
			t.Errorf("%s: runtime risk = %q, reviewed risk = %q", command, deliveredRisk, record.Risk)
		}
		reviewedAvailability := record.Availability
		if reviewedAvailability == "" {
			reviewedAvailability = source.Availability
		}
		if item.Availability != reviewedAvailability {
			t.Errorf("%s: availability = %q, want %q", command, item.Availability, reviewedAvailability)
		}
		if got := shortcut.InPublicCatalog("chat", command); got != record.Public {
			t.Errorf("%s: InPublicCatalog = %v, want %v", command, got, record.Public)
		}
		if record.Public != shortcut.InPublicCatalog("chat", command) {
			t.Errorf("%s: delivered public catalog membership differs from record", command)
		}
		if record.CompatibilityVisible {
			if item.Hidden || !item.CompatibilityVisible || record.Public || reviewedAvailability != shortcut.AvailabilityAvailable {
				t.Errorf("%s: compatibility-visible delivery = hidden:%v compatibility:%v public:%v availability:%s",
					command, item.Hidden, item.CompatibilityVisible, record.Public, reviewedAvailability)
			}
		} else if reviewedAvailability == shortcut.AvailabilityAvailable && (!record.Public || item.Hidden) {
			t.Errorf("%s: available reviewed Chat Shortcut must be public or compatibility-visible", command)
		}
		if reviewedAvailability != shortcut.AvailabilityAvailable && (record.Public || !item.Hidden) {
			t.Errorf("%s: %s reviewed Chat Shortcut must be hidden", command, reviewedAvailability)
		}
		if record.Disposition == shortcut.DispositionAliasInternal {
			primary, ok := registered[record.Primary]
			if !ok {
				t.Errorf("%s: primary %q is not registered", command, record.Primary)
			} else if primary.Hidden {
				t.Errorf("%s: primary %q is not public", command, record.Primary)
			}
		}
	}
}
