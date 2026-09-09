// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package profilemetadata

import "testing"

func TestCrossPlatformCoverageSelectorAndIndexEdges(t *testing.T) {
	if _, _, ok := ParseIdentitySelector("corp:"); ok {
		t.Fatal("empty user accepted")
	}
	if _, _, ok := ParseIdentitySelector(":user"); ok {
		t.Fatal("empty corp accepted")
	}
	cfg := &ProfilesConfig{Profiles: []Profile{{Name: "n", CorpID: "c1", UserID: "u1"}}}
	if got := ExactProfileSelectorForCorp(cfg, "c1", "c1:missing"); got != "" {
		t.Fatalf("missing exact selector = %q", got)
	}
	if ProfileIndexByIdentity(nil, "c", "u") != -1 {
		t.Fatal("nil cfg index")
	}
	if ProfileSelectorReferenceExists(cfg, "c1:missing") {
		t.Fatal("missing exact identity exists")
	}
	if ProfilesForCorpID(nil, "c1") != nil {
		t.Fatal("nil cfg profiles")
	}
}
