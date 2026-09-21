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

// NewLoginRetryGuidanceErrorForTest exposes the private retry condition to
// cross-package command-boundary tests. Production code must not call it.
//
// coverage profiles. The app-package test that calls it is partitioned outside
// the CI coverage shards, so the auth-package coverage test covers it here
// instead; without noinline some compiler/platform combinations inline the
// single-statement body and count it under the caller.
//
//go:noinline keeps this constructor's statement attributed to this file in
func NewLoginRetryGuidanceErrorForTest(cause error) error {
	return &profileMigrationLoginRetryError{cause: cause}
}
