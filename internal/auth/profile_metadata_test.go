// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package auth

import "testing"

func TestCrossPlatformCoverageResolveProfileMetadataUsesSelectorGrammarAndReturnsCopy(t *testing.T) {
	cfg := &ProfilesConfig{
		Version: 3,
		Profiles: []Profile{
			{Name: "Historical", CorpID: "corp-1", CorpName: "Example Org"},
			{Name: "Exact", CorpID: "corp-1", CorpName: "Example Org", UserID: "user-1", UserName: "Example User", ClientID: "client-1"},
		},
		OrgCurrentProfiles: map[string]string{"corp-1": "corp-1:user-1"},
	}

	for _, selector := range []string{"corp-1", "Example Org", "corp-1:Example User", "Example Org:Example User"} {
		profile, err := ResolveProfileMetadata(cfg, selector)
		if err != nil {
			t.Fatalf("ResolveProfileMetadata(%q) error = %v", selector, err)
		}
		if profile == nil || profile.UserID != "user-1" || profile.ClientID != "client-1" {
			t.Fatalf("ResolveProfileMetadata(%q) = %#v", selector, profile)
		}
	}

	profile, err := ResolveProfileMetadata(cfg, "corp-1:user-1")
	if err != nil {
		t.Fatalf("ResolveProfileMetadata(exact) error = %v", err)
	}
	profile.Name = "mutated copy"
	if cfg.Profiles[1].Name != "Exact" {
		t.Fatalf("ResolveProfileMetadata returned registry-owned pointer")
	}
	if _, err := ResolveProfileMetadata(cfg, "missing"); err == nil {
		t.Fatal("ResolveProfileMetadata(missing) unexpectedly succeeded")
	}
}
