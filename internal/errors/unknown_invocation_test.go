// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package errors

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"testing"
)

func TestCrossPlatformCoverageUnknownInvocationMarker(t *testing.T) {
	cause := stderrors.New("parser failure")
	original := NewValidation("unknown flag", WithCause(cause), WithReason("unknown_flag"), WithHint("use --help"), WithDetails(map[string]any{"input": "flag"}))
	marked := MarkUnknownInvocation(original)
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
	if !IsUnknownInvocationError(fmt.Errorf("outer: %w", marked)) || MarkUnknownInvocation(marked) != marked || MarkUnknownInvocation(nil) != nil {
		t.Fatal("marker did not survive transparent wrappers or preserve nil/idempotency")
	}
	for _, err := range []error{nil, cause, original, stderrors.New("unknown command"), NewValidation("unknown subcommand", WithReason("unknown_subcommand"))} {
		if IsUnknownInvocationError(err) {
			t.Fatalf("unmarked error inferred as unknown invocation: %v", err)
		}
	}
	if err := NormalizeValidation(MarkUnknownInvocation(cause)); ExitCode(err) != 3 || !IsUnknownInvocationError(err) {
		t.Fatalf("marker prevented native validation normalization: %v", err)
	}
}
