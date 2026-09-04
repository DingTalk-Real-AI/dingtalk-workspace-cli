// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errors

import (
	"context"
	stderrors "errors"
	"testing"
)

type validationTestExitError struct{}

func (validationTestExitError) Error() string { return "explicit" }
func (validationTestExitError) ExitCode() int { return ExitCodeAuth }

func TestCrossPlatformCoverageNormalizeValidation(t *testing.T) {
	raw := stderrors.New("bad argument")
	err := NormalizeValidation(raw, WithReason("invalid_parameters"))
	var typed *Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("NormalizeValidation() = %T, want *Error", err)
	}
	if typed.Category != CategoryValidation || typed.Reason != "invalid_parameters" || typed.Cause != raw {
		t.Fatalf("NormalizeValidation() = %#v", typed)
	}
	if !stderrors.Is(err, raw) {
		t.Fatal("NormalizeValidation() lost its cause")
	}

	apiErr := NewAPI("upstream failed")
	if got := NormalizeValidation(apiErr); got != apiErr {
		t.Fatalf("typed error changed: got %p want %p", got, apiErr)
	}
	exitErr := validationTestExitError{}
	if got := NormalizeValidation(exitErr); got != exitErr {
		t.Fatalf("ExitCoder changed: got %#v want %#v", got, exitErr)
	}
	for _, preserved := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := NormalizeValidation(preserved); got != preserved {
			t.Fatalf("context error changed: got %v want %v", got, preserved)
		}
	}
	if NormalizeValidation(nil) != nil {
		t.Fatal("NormalizeValidation(nil) must be nil")
	}
}
