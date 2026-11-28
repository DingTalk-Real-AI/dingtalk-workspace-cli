// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build darwin

package runtimecontext

import "testing"

func TestCrossPlatformCoverageDarwinLookupProcessInvalidPID(t *testing.T) {
	if _, ok := darwinLookupProcess(-1); ok {
		t.Fatal("expected invalid pid lookup to fail")
	}
}
