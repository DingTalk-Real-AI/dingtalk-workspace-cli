// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package errors

import "testing"

func TestCrossPlatformCoverageBusinessDiagnosticsDoNotMutateDetailsOrRetryPolicy(t *testing.T) {
	original := map[string]any{"existing": "value"}
	err := NewAPI("rejected", WithDetails(original), WithServerDiag(ServerDiagnostics{ErrorStage: "processContent", ErrorCategory: "DOWNSTREAM"})).(*Error)
	if err.FailureStage != "processContent" || err.Details["error_category"] != "DOWNSTREAM" || err.Details["existing"] != "value" || err.Retryable {
		t.Fatalf("lost diagnostics: %#v", err)
	}
	if _, ok := original["error_category"]; ok {
		t.Fatal("mutated caller-owned details")
	}
	err = NewAPI("gateway", WithFailureStage("gateway"), WithServerDiag(ServerDiagnostics{ErrorStage: "processContent"})).(*Error)
	if err.FailureStage != "gateway" || err.Details != nil {
		t.Fatalf("overwrote existing failure stage: %#v", err)
	}
	if (ServerDiagnostics{ErrorCategory: "DOWNSTREAM"}).IsEmpty() || (ServerDiagnostics{ErrorStage: "validation"}).IsEmpty() {
		t.Fatal("business diagnostics incorrectly empty")
	}
}
