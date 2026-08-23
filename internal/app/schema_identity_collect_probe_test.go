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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

// TestCollectedIdentityIsValidSingleSource is the standing gate for the
// retired reviewed registry: command identity collected from live Cobra
// leaves carrying ContractFinal.Identity must be a valid single source for
// EffectiveCommandRegistry assembly. It asserts the collector returns a
// non-empty spec set with no missing primaries, builds an effective registry
// from it, and produces a stable SourceHash across repeated collection walks.
func TestCrossPlatformCoverageCollectedIdentityIsValidSingleSource(t *testing.T) {
	root := NewRootCommand()
	collected, report, err := cli.CollectIdentitySpecs(root)
	if err != nil {
		t.Fatalf("collect identity specs: %v", err)
	}
	if len(collected) == 0 {
		t.Fatal("identity collection returned no command specs")
	}
	if len(report.MissingPrimary) != 0 {
		t.Fatalf("identity collection reported missing primaries: %v", report.MissingPrimary)
	}

	t.Logf("walk leaves=%d withIdentity=%d hiddenPrimaries=%d excluded=%d noIdentity=%d collected=%d",
		report.Leaves, report.WithIdentity, report.HiddenPrimaries, report.Excluded, len(report.NoIdentity), len(collected))
	for _, leaf := range report.NoIdentity {
		t.Logf("NO_IDENTITY_LEAF %s", leaf)
	}

	effective, err := cli.BuildEffectiveFromSpecs(collected)
	if err != nil {
		t.Fatalf("build effective registry from collected specs: %v", err)
	}
	sourceHash := effective.SourceHash()
	t.Logf("collected SourceHash=%s commands=%d", sourceHash, len(effective.Commands))
	if sourceHash == "" {
		t.Fatal("effective registry SourceHash is empty")
	}

	collectedAgain, _, err := cli.CollectIdentitySpecs(root)
	if err != nil {
		t.Fatalf("re-collect identity specs: %v", err)
	}
	effectiveAgain, err := cli.BuildEffectiveFromSpecs(collectedAgain)
	if err != nil {
		t.Fatalf("rebuild effective registry from collected specs: %v", err)
	}
	if got := effectiveAgain.SourceHash(); got != sourceHash {
		t.Fatalf("collected identity SourceHash is not stable across walks: %q vs %q", got, sourceHash)
	}
}
