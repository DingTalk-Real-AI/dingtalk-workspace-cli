// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package profilectx

import "testing"

func TestCrossPlatformCoverageSetAndGet(t *testing.T) {
	t.Cleanup(func() { Set("") })

	Set("  fixture-profile  ")
	if got := Get(); got != "fixture-profile" {
		t.Fatalf("Get() = %q, want fixture-profile", got)
	}

	Set("")
	if got := Get(); got != "" {
		t.Fatalf("Get() after reset = %q, want empty", got)
	}
}
