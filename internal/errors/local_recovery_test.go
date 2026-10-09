// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package errors

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"testing"
)

func TestCrossPlatformCoverageLocalRecoveryMarker(t *testing.T) {
	cause := stderrors.New("parser failure")
	original := NewValidation("unknown flag", WithCause(cause), WithReason("unknown_flag"), WithHint("use --format json"), WithDetails(map[string]any{"input": "json"}))
	marked := MarkLocalRecovery(MarkUnknownInvocation(original))
	var before, after bytes.Buffer
	if err := PrintJSON(&before, original); err != nil {
		t.Fatal(err)
	}
	if err := PrintJSON(&after, marked); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Bytes(), after.Bytes()) || ExitCode(marked) != 3 || marked.Error() != original.Error() {
		t.Fatalf("marker changed the error contract: original=%s marked=%s", &before, &after)
	}
	var typed *Error
	if !stderrors.As(marked, &typed) || typed.Cause != cause || !stderrors.Is(marked, original) || !stderrors.Is(marked, cause) {
		t.Fatal("marker changed the original error or cause")
	}
	wrapped := fmt.Errorf("outer: %w", marked)
	if !HasLocalRecovery(wrapped) || !IsUnknownInvocationError(wrapped) || MarkLocalRecovery(wrapped) != wrapped || MarkLocalRecovery(nil) != nil {
		t.Fatal("marker did not survive wrappers or preserve nil/idempotency")
	}
	for _, err := range []error{nil, cause, original, MarkUnknownInvocation(original), stderrors.New("Did you mean --format?"), NewValidation("unknown flag", WithHint("Run 'dws --help'"))} {
		if HasLocalRecovery(err) {
			t.Fatalf("unmarked hint inferred as local recovery: %v", err)
		}
	}
	if err := NormalizeValidation(MarkLocalRecovery(cause)); ExitCode(err) != 3 || !HasLocalRecovery(err) {
		t.Fatalf("marker prevented validation normalization: %v", err)
	}
	classified := &stubExitCoder{code: ExitCodeAuth}
	if ExitCode(MarkLocalRecovery(classified)) != ExitCodeAuth {
		t.Fatal("marker changed an existing ExitCoder")
	}
}
