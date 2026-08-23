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

// Package testseam turns the package-var injection-seam swap into a mechanism
// instead of a convention. Tests that stub a package-level function var
// (e.g. pipelineBuildEffectiveRegistry) must use Swap so the previous value
// is structurally restored via t.Cleanup; a forgotten restore then cannot
// leak state into sibling tests.
package testseam

import "testing"

// Swap replaces *ptr with next for the duration of the test and restores the
// previous value when the test ends. It is the required form for package-var
// seam swaps:
//
//	testseam.Swap(t, &somePackageFn, func(args) (ret, error) { ... })
//
// Prefer it over the manual `prev := fn; t.Cleanup(...); fn = stub` trio:
// Swap cannot forget the restore. Like the manual pattern it replaces, Swap
// mutates process-global state and is therefore NOT safe for t.Parallel tests
// — sequential tests only.
func Swap[T any](t *testing.T, ptr *T, next T) {
	t.Helper()
	prev := *ptr
	*ptr = next
	t.Cleanup(func() { *ptr = prev })
}

// Protect snapshots *ptr and restores it at test end without replacing it.
// Use it when the code under test mutates the seam itself (e.g. os.Args) and
// no stub value is available up front. Same sequential-only caveat as Swap.
func Protect[T any](t *testing.T, ptr *T) {
	t.Helper()
	prev := *ptr
	t.Cleanup(func() { *ptr = prev })
}
