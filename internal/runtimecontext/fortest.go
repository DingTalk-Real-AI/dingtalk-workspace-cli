// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

// ReadyResultForTest constructs a ready result for cross-package tests.
// Production code must obtain Result values from Resolve.
func ReadyResultForTest(value string) Result {
	return Result{
		State:          StateReady,
		PayloadVersion: PayloadVersion,
		Environment:    Environment,
		token:          value,
	}
}
