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

// AgentSnapshotForTest supplies a parsed SDK object to cross-package tests.
func AgentSnapshotForTest(raw []byte, code int32) AgentSnapshot {
	return parseAgentResult(raw, code, nil)
}

// ResultWithAgentForTest exercises request assembly without loading the SDK.
func ResultWithAgentForTest(token string, raw []byte, code int32) Result {
	result := ReadyResultForTest(token)
	if token == "" {
		result.State = StateUnavailable
	}
	result.agent = parseAgentResult(raw, code, nil)
	return result
}
