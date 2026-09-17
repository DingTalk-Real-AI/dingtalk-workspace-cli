// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package skillpaths

import "testing"

func TestCrossPlatformCoverageAgentHomesIsDetachedCopy(t *testing.T) {
	homes := AgentHomes()
	if len(homes) == 0 {
		t.Fatal("empty agent homes")
	}
	homes[0] = "mutated"
	again := AgentHomes()
	if again[0] == "mutated" {
		t.Fatal("AgentHomes returned shared storage")
	}
	if again[0] != ".agents/skills" {
		t.Fatalf("first home = %q", again[0])
	}
}
