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

package helpers

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Test helpers for package-level deps. Production code must not call these;
// the ForTest suffix is the boundary.

// InitDepsForTest installs caller for the test duration and restores the prior
// deps pointer (including a prior nil) via testseam. Prefer this over
// InitDeps + InitDeps(previousCaller): restoring through InitDeps(nil) leaves a
// non-nil Deps with a nil Caller, which is not the original unset state.
func InitDepsForTest(t *testing.T, caller edition.ToolCaller) {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
}
