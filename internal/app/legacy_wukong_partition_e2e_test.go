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

package app

import (
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// TestEditionPartition_SingleSourceOfTruth is the regression test that
// specifically targets the original bug: internal/app.loadDynamicCommands
// was computing its partition one way (editionPartition() →
// "wukong/default") while internal/cli.EnvironmentLoader was hardcoding
// config.DefaultPartition ("default/default"). This meant runtime endpoint
// resolution and command-tree generation read different cache files, and
// under gray-release the two partitions carried disjoint product lists —
// the historical root cause of `dws conference meeting create` failing
// while `dws todo task list` succeeded on the same host.
//
// Keeping both sides funneled through config.EditionPartition is the
// central invariant the fix enforces. If this test ever regresses, the
// two-partition split almost certainly came back.
func TestEditionPartition_SingleSourceOfTruth(t *testing.T) {
	t.Cleanup(func() { edition.Override(&edition.Hooks{}) })

	cases := []struct {
		name    string
		edition string
		want    string
	}{
		{"open edition falls through to default/default", "", config.DefaultPartition},
		{"explicit open edition remains default", "open", config.DefaultPartition},
		{"wukong overlay uses wukong/default", "wukong", "wukong/default"},
		{"custom edition is namespaced", "internal-lab", "internal-lab/default"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edition.Override(&edition.Hooks{Name: tc.edition})
			legacy := editionPartition()
			shared := config.EditionPartition(edition.Get().Name)

			if legacy != shared {
				t.Fatalf("editionPartition()=%q, config.EditionPartition()=%q — partition split regressed for edition %q", legacy, shared, tc.edition)
			}
			if legacy != tc.want {
				t.Fatalf("editionPartition()=%q, want %q for edition %q", legacy, tc.want, tc.edition)
			}
		})
	}
}

func TestRuntimeCachePartitionUsesProfileIdentity(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(func() {
		authpkg.SetRuntimeProfile("")
		edition.Override(&edition.Hooks{})
	})
	edition.Override(&edition.Hooks{})

	authpkg.SetRuntimeProfile("")
	if got := runtimeCachePartition(); got != editionPartition() {
		t.Fatalf("runtimeCachePartition()=%q, want edition partition %q without profile", got, editionPartition())
	}

	authpkg.SetRuntimeProfile("corp_same:user_a")
	partitionA := runtimeCachePartition()
	authpkg.SetRuntimeProfile("corp_same:user_b")
	partitionB := runtimeCachePartition()

	if partitionA == partitionB {
		t.Fatalf("runtime cache partitions should differ for two profiles: %q", partitionA)
	}
	for _, got := range []string{partitionA, partitionB} {
		if !strings.Contains(got, "/profile/") {
			t.Fatalf("runtime cache partition %q missing profile namespace", got)
		}
	}
}
