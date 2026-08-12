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

package testseam

import "testing"

func TestCrossPlatformCoverageSwapSetsAndRestores(t *testing.T) {
	value := 1
	t.Run("child", func(t *testing.T) {
		Swap(t, &value, 42)
		if value != 42 {
			t.Fatalf("Swap did not install next value: %d", value)
		}
		Swap(t, &value, 43)
		if value != 43 {
			t.Fatalf("second Swap did not install next value: %d", value)
		}
	})
	if value != 1 {
		t.Fatalf("Swap did not restore previous value: %d", value)
	}
}

func TestCrossPlatformCoverageSwapFunctionSeam(t *testing.T) {
	called := false
	fn := func() int { return 7 }
	t.Run("child", func(t *testing.T) {
		Swap(t, &fn, func() int { called = true; return 9 })
		if got := fn(); got != 9 || !called {
			t.Fatalf("swapped function not invoked: got=%d called=%v", got, called)
		}
	})
	if got := fn(); got != 7 {
		t.Fatalf("function seam not restored: %d", got)
	}
}

func TestCrossPlatformCoverageProtectRestores(t *testing.T) {
	value := 5
	t.Run("child", func(t *testing.T) {
		Protect(t, &value)
		value = 99 // code under test mutates the seam
	})
	if value != 5 {
		t.Fatalf("Protect did not restore mutated value: %d", value)
	}
}
