// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package profilectx

import "testing"

func TestCrossPlatformCoverageSetAndGet(t *testing.T) {
	t.Cleanup(func() { Set(""); SetIdentity(Identity{}); RegisterIdentityResolver(nil) })

	Set("  fixture-profile  ")
	if got := Get(); got != "fixture-profile" {
		t.Fatalf("Get() = %q, want fixture-profile", got)
	}

	Set("")
	if got := Get(); got != "" {
		t.Fatalf("Get() after reset = %q, want empty", got)
	}

	SetIdentity(Identity{CorpID: " corp ", UserID: " user "})
	if got := GetIdentity(); got.CorpID != "corp" || got.UserID != "user" {
		t.Fatalf("GetIdentity() = %+v", got)
	}

	RegisterIdentityResolver(func(selector string) (Identity, error) {
		if selector != "fixture-profile" {
			t.Fatalf("selector = %q, want fixture-profile", selector)
		}
		return Identity{CorpID: " corp-resolved ", UserID: " user-resolved "}, nil
	})
	resolved, err := ResolveIdentity(" fixture-profile ")
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if resolved != (Identity{CorpID: "corp-resolved", UserID: "user-resolved"}) {
		t.Fatalf("ResolveIdentity() = %+v", resolved)
	}
}
