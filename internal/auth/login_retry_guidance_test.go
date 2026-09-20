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

package auth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestCrossPlatformCoverageLoginRetryGuidanceRejectsUnrelatedErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "nil"},
		{name: "plain", err: errors.New("login failed")},
		{name: "missing DEK alone", err: keychain.ErrDEKMissing},
		{name: "ciphertext mismatch alone", err: keychain.ErrCiphertextKeyMismatch},
		{name: "wrapped unrelated", err: fmt.Errorf("save token: %w", keychain.ErrDEKMissing)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if guidance, ok := LoginRetryGuidance(tc.err); ok || guidance != "" {
				t.Fatalf("LoginRetryGuidance(%v) = %q, %v; want empty, false", tc.err, guidance, ok)
			}
		})
	}
}

func TestCrossPlatformCoverageLoginRetryGuidanceFindsNestedRetryCondition(t *testing.T) {
	cause := fmt.Errorf("snapshot: %w", keychain.ErrDEKMissing)
	retryErr := &profileMigrationLoginRetryError{cause: cause}
	nested := fmt.Errorf("provider persistence: %w", fmt.Errorf("save token: %w", retryErr))

	guidance, ok := LoginRetryGuidance(nested)
	if !ok || guidance != profileLoginRetryGuidance {
		t.Fatalf("LoginRetryGuidance() = %q, %v; want %q, true", guidance, ok, profileLoginRetryGuidance)
	}
	if !errors.Is(nested, keychain.ErrDEKMissing) {
		t.Fatalf("nested retry error lost original cause: %v", nested)
	}
}
