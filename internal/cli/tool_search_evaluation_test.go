// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import "testing"

func TestAggregateToolSearchAgentAB(t *testing.T) {
	catalog := CatalogVersionRef{SourceHash: "sha256:source", SurfaceHash: "sha256:surface"}
	input := ToolSearchAgentABInput{
		Version: "tool-search-agent-ab.v1",
		Catalog: catalog,
		Runs: []ToolSearchAgentABRun{
			{CaseID: "a", Trial: 0, Arm: ToolSearchAgentArmDirectSchema, CorrectToolPlan: true, ContextTokens: 100, ToolCalls: 3, LatencyMS: 1000},
			{CaseID: "a", Trial: 0, Arm: ToolSearchAgentArmSearchInspect, TaskCompleted: true, CorrectToolPlan: true, ContextTokens: 40, ToolCalls: 2, LatencyMS: 800},
			{CaseID: "b", Trial: 0, Arm: ToolSearchAgentArmDirectSchema, TaskCompleted: true, CorrectToolPlan: true, UnsafeAction: true, RecoveryAttempted: true, ContextTokens: 120, ToolCalls: 4, LatencyMS: 1200},
			{CaseID: "b", Trial: 0, Arm: ToolSearchAgentArmSearchInspect, TaskCompleted: true, CorrectToolPlan: true, RecoveryAttempted: true, RecoverySucceeded: true, ContextTokens: 50, ToolCalls: 3, LatencyMS: 900},
		},
	}
	report, err := AggregateToolSearchAgentAB(input)
	if err != nil {
		t.Fatalf("AggregateToolSearchAgentAB() error = %v", err)
	}
	if report.Cases != 2 || report.PairedTrials != 2 {
		t.Fatalf("pair counts = cases %d trials %d", report.Cases, report.PairedTrials)
	}
	if report.DirectSchema.TaskSuccessRate != 0.5 || report.SearchInspect.TaskSuccessRate != 1 {
		t.Fatalf("unexpected success rates: direct=%v search=%v", report.DirectSchema.TaskSuccessRate, report.SearchInspect.TaskSuccessRate)
	}
	if delta := report.Deltas["context_tokens"].Delta; delta != -65 {
		t.Fatalf("context delta = %v, want -65", delta)
	}
	if _, ok := report.Deltas["recovery_success_rate_paired_attempts"]; !ok {
		t.Fatal("paired recovery delta missing")
	}
}

func TestAggregateToolSearchAgentABRejectsUnpairedRuns(t *testing.T) {
	_, err := AggregateToolSearchAgentAB(ToolSearchAgentABInput{
		Version: "tool-search-agent-ab.v1",
		Catalog: CatalogVersionRef{SourceHash: "source", SurfaceHash: "surface"},
		Runs: []ToolSearchAgentABRun{
			{CaseID: "a", Arm: ToolSearchAgentArmDirectSchema},
		},
	})
	if err == nil {
		t.Fatal("AggregateToolSearchAgentAB() accepted an unpaired run")
	}
}

func TestScoreToolSearchAgentPlanningAB(t *testing.T) {
	workflows := []ToolSearchWorkflowEvaluationCase{
		{ID: "one", Required: []string{"chat.search_groups", "chat.send_personal_message"}},
		{ID: "two", Required: []string{"drive.upload", "drive.add_permission"}},
	}
	direct := ToolSearchAgentPlanOutput{Results: []ToolSearchAgentPlanResult{
		{ID: "one", CanonicalPaths: []string{"chat.search_groups", "chat.send_personal_message"}},
		{ID: "two", CanonicalPaths: []string{"drive.upload", "drive.add_permission"}},
	}}
	search := ToolSearchAgentPlanOutput{Results: []ToolSearchAgentPlanResult{
		{ID: "one", CanonicalPaths: []string{"chat.list_owned_or_admin_groups", "chat.search_groups", "chat.send_personal_message"}},
		{ID: "two", CanonicalPaths: []string{"drive.upload", "drive.add_permission"}},
	}}
	tools := []ToolSpec{
		toolSearchTestTool("chat", "search_groups", "chat search-groups", "搜索群聊", "read"),
		toolSearchTestTool("chat", "send_personal_message", "chat message send", "发送消息", "write"),
		toolSearchTestTool("drive", "upload", "drive upload", "上传文件", "write"),
		toolSearchTestTool("drive", "add_permission", "drive permission add", "添加权限", "write"),
		toolSearchTestTool("chat", "list_owned_or_admin_groups", "chat list-groups", "列出群聊", "read"),
	}
	index, indexErr := (SchemaRegistry{Kind: "schema", Level: "catalog", Products: []ProductSpec{
		{ID: "chat", Tools: []ToolSpec{tools[0], tools[1], tools[4]}},
		{ID: "drive", Tools: tools[2:4]},
	}}).Index()
	if indexErr != nil {
		t.Fatal(indexErr)
	}
	report, err := scoreToolSearchAgentPlanningAB(index, "test-model", workflows, direct, search)
	if err != nil {
		t.Fatalf("ScoreToolSearchAgentPlanningAB() error = %v", err)
	}
	if report.DirectSchema.ExactMinimalRate != 1 || report.SearchInspect.ExactMinimalRate != 0.5 {
		t.Fatalf("minimal rates = direct %v search %v", report.DirectSchema.ExactMinimalRate, report.SearchInspect.ExactMinimalRate)
	}
	if report.SearchInspect.CompleteRate != 1 || report.SearchInspect.UnnecessaryTools != 1 {
		t.Fatalf("search metrics = %+v", report.SearchInspect)
	}
}
