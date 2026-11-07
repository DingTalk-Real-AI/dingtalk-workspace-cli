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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestAgentSelectionScenariosCoverEveryExecutableSchemaTool(t *testing.T) {
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	fixture, report, err := cli.BuildAgentSelectionEvalFixture(bound)
	if err != nil {
		t.Fatalf("BuildAgentSelectionEvalFixture() error = %v", err)
	}
	if report.Tools != len(bound.Commands) {
		t.Fatalf("selection tools = %d, bound commands = %d", report.Tools, len(bound.Commands))
	}
	if report.PositiveAssertions < report.Tools {
		t.Fatalf("positive selection coverage = %+v, want at least one assertion per tool", report)
	}
	if report.NegativeAssertions < report.Tools {
		t.Fatalf("negative selection coverage = %+v, want at least one assertion per tool", report)
	}
	if report.Tools == 0 {
		t.Fatal("selection contract unexpectedly contains no tools")
	}
	if len(fixture.Cases) != report.PositiveAssertions+report.NegativeAssertions {
		t.Fatalf("selection fixture cases = %d, report = %+v", len(fixture.Cases), report)
	}
	if report.FixtureSHA256 == "" {
		t.Fatal("selection fixture digest is empty")
	}
	t.Logf("validated %d bound tools, %d positive assertions, %d negative assertions (%s)", report.Tools, report.PositiveAssertions, report.NegativeAssertions, report.FixtureSHA256)
}

func TestAgentSelectionRetiredPromoteRejectsCapabilityIntent(t *testing.T) {
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	fixture, _, err := cli.BuildAgentSelectionEvalFixture(bound)
	if err != nil {
		t.Fatalf("BuildAgentSelectionEvalFixture() error = %v", err)
	}

	const canonical = "chat.promote_message_to_thread"
	foundRetirementOnlyPositive := false
	foundCapabilityNegative := false
	for _, testCase := range fixture.Cases {
		if testCase.ExpectedCanonical == canonical {
			if !strings.Contains(testCase.Scenario, "下线") && !strings.Contains(testCase.Scenario, "历史") {
				t.Fatalf("retired promote has active positive selection scenario: %q", testCase.Scenario)
			}
			foundRetirementOnlyPositive = true
		}
		if testCase.ID == canonical+"/avoid_when/0" && strings.Contains(testCase.Scenario, "已有消息升级") {
			if testCase.ForbiddenCanonical != canonical {
				t.Fatalf("retired promote capability case forbidden canonical = %q, want %q", testCase.ForbiddenCanonical, canonical)
			}
			if testCase.ExpectedCanonical != "" {
				t.Fatalf("retired promote capability case unexpectedly selects %q", testCase.ExpectedCanonical)
			}
			foundCapabilityNegative = true
		}
	}
	if !foundRetirementOnlyPositive {
		t.Fatal("retired promote is missing a retirement-only positive selection scenario")
	}
	if !foundCapabilityNegative {
		t.Fatal("retired promote is missing a negative selection scenario for the former capability intent")
	}
}
