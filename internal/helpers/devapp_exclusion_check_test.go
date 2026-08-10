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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestDevFormerExclusionsHaveFinalContractAndUnifiedRollout(t *testing.T) {
	root := newDevAppTestRoot(&captureRunner{})
	for _, path := range [][]string{
		{"dev", "connect", "list"},
		{"dev", "connect", "restart"},
		{"dev", "app", "version", "check-approval"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd == nil || !cmd.Runnable() {
			t.Fatalf("%v is not a runnable command: cmd=%v err=%v", path, cmd, err)
		}
		if _, ok := contractfinal.RuntimeContractFinal(cmd); !ok {
			t.Fatalf("%s has no ContractFinal", cmd.CommandPath())
		}
		if got := output.CommandRollout(cmd); got != output.RolloutUnifiedActive {
			t.Fatalf("%s rollout=%s, want %s", cmd.CommandPath(), got, output.RolloutUnifiedActive)
		}
	}
}
