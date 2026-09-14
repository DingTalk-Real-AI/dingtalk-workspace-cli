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

import "testing"

// TestErrorsExitCodeMapConsistentWithExitCode 是 B209 的 errors 侧同源锁定：
// exitcodes.go 的 exitCodeByCategory 映射表必须与 ExitCode() 的 switch 分支
// 逐类别一致（同源不双轨，契约 §4）。任一单边修改即失败，防止未来漂移。
// output 侧同源锁定由 internal/output emitter_phase_c_test.go
// TestExitCodeForEnvelopeSameSourceAsErrorsExitCode 交叉断言（B209）。
func TestErrorsExitCodeMapConsistentWithExitCode(t *testing.T) {
	t.Parallel()

	cats := []Category{
		CategoryAPI,
		CategoryAuth,
		CategoryValidation,
		CategoryDiscovery,
		CategoryInternal,
		CategoryPartial,
	}
	for _, cat := range cats {
		table := exitCodeByCategory[cat]
		viaSwitch := (&Error{Category: cat, Message: "x"}).ExitCode()
		if table != viaSwitch {
			t.Fatalf("exitCodeByCategory[%q]=%d disagrees with ExitCode()=%d", cat, table, viaSwitch)
		}
	}
}

// TestErrorsExitCodeConstantsEqualMap 锁定类别专属常量与映射表值一致。
func TestErrorsExitCodeConstantsEqualMap(t *testing.T) {
	t.Parallel()

	want := map[Category]int{
		CategoryAPI:        ExitCodeAPI,
		CategoryAuth:       ExitCodeAuth,
		CategoryValidation: ExitCodeValidation,
		CategoryDiscovery:  ExitCodeDiscovery,
		CategoryInternal:   ExitCodeInternal,
		CategoryPartial:    ExitCodeInternal,
	}
	for cat, wantCode := range want {
		if got := exitCodeByCategory[cat]; got != wantCode {
			t.Fatalf("exitCodeByCategory[%q]=%d, want %d", cat, got, wantCode)
		}
	}
}
