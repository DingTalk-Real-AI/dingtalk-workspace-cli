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

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

const (
	ToolSearchAgentArmDirectSchema  = "direct_schema"
	ToolSearchAgentArmSearchInspect = "search_inspect"
)

// ToolSearchAgentABInput is populated by an Agent runner. Every case/trial
// must have both arms so the comparison remains paired.
type ToolSearchAgentABInput struct {
	Version string                 `json:"version"`
	Catalog CatalogVersionRef      `json:"catalog"`
	Runs    []ToolSearchAgentABRun `json:"runs"`
}

// ToolSearchAgentABRun records end-to-end outcomes rather than retrieval
// proxies. ContextTokens must come from the selected model's tokenizer.
type ToolSearchAgentABRun struct {
	CaseID            string  `json:"case_id"`
	Trial             int     `json:"trial"`
	Arm               string  `json:"arm"`
	TaskCompleted     bool    `json:"task_completed"`
	CorrectToolPlan   bool    `json:"correct_tool_plan"`
	UnsafeAction      bool    `json:"unsafe_action"`
	RecoveryAttempted bool    `json:"recovery_attempted"`
	RecoverySucceeded bool    `json:"recovery_succeeded"`
	ContextTokens     float64 `json:"context_tokens"`
	ToolCalls         float64 `json:"tool_calls"`
	LatencyMS         float64 `json:"latency_ms"`
}

type ToolSearchAgentABArmMetrics struct {
	Runs                 int     `json:"runs"`
	TaskSuccessRate      float64 `json:"task_success_rate"`
	CorrectPlanRate      float64 `json:"correct_plan_rate"`
	UnsafeActionRate     float64 `json:"unsafe_action_rate"`
	RecoverySuccessRate  float64 `json:"recovery_success_rate"`
	RecoveryAttempts     int     `json:"recovery_attempts"`
	AverageContextTokens float64 `json:"average_context_tokens"`
	AverageToolCalls     float64 `json:"average_tool_calls"`
	AverageLatencyMS     float64 `json:"average_latency_ms"`
}

// ToolSearchAgentABDelta is search_inspect minus direct_schema. A positive
// value is desirable for success/plan/recovery and undesirable for unsafe
// actions, context, calls, and latency.
type ToolSearchAgentABDelta struct {
	DirectSchema  float64 `json:"direct_schema"`
	SearchInspect float64 `json:"search_inspect"`
	Delta         float64 `json:"delta"`
	CI95Low       float64 `json:"ci95_low"`
	CI95High      float64 `json:"ci95_high"`
}

type ToolSearchAgentABReport struct {
	Version       string                            `json:"version"`
	Catalog       CatalogVersionRef                 `json:"catalog"`
	Cases         int                               `json:"cases"`
	PairedTrials  int                               `json:"paired_trials"`
	DirectSchema  ToolSearchAgentABArmMetrics       `json:"direct_schema"`
	SearchInspect ToolSearchAgentABArmMetrics       `json:"search_inspect"`
	Deltas        map[string]ToolSearchAgentABDelta `json:"deltas"`
	Method        string                            `json:"method"`
}

// ToolSearchAgentPlanOutput is the strict final response of a non-executing
// Agent planning run. Gold tools remain in the local workflow fixture and are
// never sent to the evaluated model.
type ToolSearchAgentPlanOutput struct {
	Results []ToolSearchAgentPlanResult `json:"results"`
}

type ToolSearchAgentPlanResult struct {
	ID             string   `json:"id"`
	CanonicalPaths []string `json:"canonical_paths"`
}

type ToolSearchAgentPlanMetrics struct {
	Cases              int     `json:"cases"`
	CompleteRate       float64 `json:"complete_rate"`
	ExactMinimalRate   float64 `json:"exact_minimal_rate"`
	RequiredToolRecall float64 `json:"required_tool_recall"`
	PlanPrecision      float64 `json:"plan_precision"`
	RequiredTools      int     `json:"required_tools"`
	PredictedTools     int     `json:"predicted_tools"`
	UnnecessaryTools   int     `json:"unnecessary_tools"`
}

type ToolSearchAgentPlanningABReport struct {
	Version       string                     `json:"version"`
	Model         string                     `json:"model"`
	DirectSchema  ToolSearchAgentPlanMetrics `json:"direct_schema"`
	SearchInspect ToolSearchAgentPlanMetrics `json:"search_inspect"`
	Delta         map[string]float64         `json:"delta_search_minus_direct"`
	Limitations   []string                   `json:"limitations"`
}

// ScoreToolSearchAgentPlanningAB scores an answer-free paired planning smoke
// run against reviewed workflow requirements. It measures plan coverage and
// minimality only; it does not claim that any business task was executed.
func ScoreToolSearchAgentPlanningAB(model string, workflows []ToolSearchWorkflowEvaluationCase, direct, search ToolSearchAgentPlanOutput) (ToolSearchAgentPlanningABReport, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return ToolSearchAgentPlanningABReport{}, fmt.Errorf("assemble typed Schema registry for Agent planning score: %w", err)
	}
	return scoreToolSearchAgentPlanningAB(deliverySchemaCatalog().Index, model, workflows, direct, search)
}

func scoreToolSearchAgentPlanningAB(index SchemaIndex, model string, workflows []ToolSearchWorkflowEvaluationCase, direct, search ToolSearchAgentPlanOutput) (ToolSearchAgentPlanningABReport, error) {
	if strings.TrimSpace(model) == "" {
		return ToolSearchAgentPlanningABReport{}, fmt.Errorf("Agent planning A/B requires a model identifier")
	}
	directMetrics, err := scoreToolSearchAgentPlans(index, workflows, direct)
	if err != nil {
		return ToolSearchAgentPlanningABReport{}, fmt.Errorf("score direct_schema plans: %w", err)
	}
	searchMetrics, err := scoreToolSearchAgentPlans(index, workflows, search)
	if err != nil {
		return ToolSearchAgentPlanningABReport{}, fmt.Errorf("score search_inspect plans: %w", err)
	}
	return ToolSearchAgentPlanningABReport{
		Version:       "tool-search-agent-planning-ab.v1",
		Model:         strings.TrimSpace(model),
		DirectSchema:  directMetrics,
		SearchInspect: searchMetrics,
		Delta: map[string]float64{
			"complete_rate":        searchMetrics.CompleteRate - directMetrics.CompleteRate,
			"exact_minimal_rate":   searchMetrics.ExactMinimalRate - directMetrics.ExactMinimalRate,
			"required_tool_recall": searchMetrics.RequiredToolRecall - directMetrics.RequiredToolRecall,
			"plan_precision":       searchMetrics.PlanPrecision - directMetrics.PlanPrecision,
			"unnecessary_tools":    float64(searchMetrics.UnnecessaryTools - directMetrics.UnnecessaryTools),
		},
		Limitations: []string{
			"one batch and one model trial per arm; no confidence interval",
			"reviewed ten-workflow diagnostic fixture is not an independent release test",
			"planning only: no parameters, business execution, recovery, latency, or tokenizer usage is scored",
			"extra prerequisites can be operationally valid even when absent from the minimal reviewed gold",
		},
	}, nil
}

func scoreToolSearchAgentPlans(index SchemaIndex, workflows []ToolSearchWorkflowEvaluationCase, output ToolSearchAgentPlanOutput) (ToolSearchAgentPlanMetrics, error) {
	byID := make(map[string]ToolSearchAgentPlanResult, len(output.Results))
	for _, result := range output.Results {
		if strings.TrimSpace(result.ID) == "" || len(result.CanonicalPaths) == 0 {
			return ToolSearchAgentPlanMetrics{}, fmt.Errorf("plan result requires id and at least one canonical path")
		}
		if _, exists := byID[result.ID]; exists {
			return ToolSearchAgentPlanMetrics{}, fmt.Errorf("duplicate plan result %q", result.ID)
		}
		seen := make(map[string]bool, len(result.CanonicalPaths))
		for _, canonical := range result.CanonicalPaths {
			if _, ok := index.Resolve(canonical); !ok {
				return ToolSearchAgentPlanMetrics{}, fmt.Errorf("plan result %q contains unknown canonical %q", result.ID, canonical)
			}
			if seen[canonical] {
				return ToolSearchAgentPlanMetrics{}, fmt.Errorf("plan result %q repeats canonical %q", result.ID, canonical)
			}
			seen[canonical] = true
		}
		byID[result.ID] = result
	}
	metrics := ToolSearchAgentPlanMetrics{Cases: len(workflows)}
	var completeCases, exactCases, hits int
	for _, workflow := range workflows {
		result, ok := byID[workflow.ID]
		if !ok {
			return ToolSearchAgentPlanMetrics{}, fmt.Errorf("missing plan result %q", workflow.ID)
		}
		delete(byID, workflow.ID)
		gold := stringSet(workflow.Required)
		caseHits := 0
		for _, canonical := range result.CanonicalPaths {
			if gold[canonical] {
				caseHits++
			}
		}
		hits += caseHits
		metrics.RequiredTools += len(workflow.Required)
		metrics.PredictedTools += len(result.CanonicalPaths)
		if caseHits == len(workflow.Required) {
			completeCases++
		}
		if stringSlicesEqual(result.CanonicalPaths, workflow.Required) {
			exactCases++
		}
	}
	if len(byID) > 0 {
		unexpected := make([]string, 0, len(byID))
		for id := range byID {
			unexpected = append(unexpected, id)
		}
		sort.Strings(unexpected)
		return ToolSearchAgentPlanMetrics{}, fmt.Errorf("unexpected plan results: %s", strings.Join(unexpected, ", "))
	}
	if metrics.Cases > 0 {
		metrics.CompleteRate = float64(completeCases) / float64(metrics.Cases)
		metrics.ExactMinimalRate = float64(exactCases) / float64(metrics.Cases)
	}
	if metrics.RequiredTools > 0 {
		metrics.RequiredToolRecall = float64(hits) / float64(metrics.RequiredTools)
	}
	if metrics.PredictedTools > 0 {
		metrics.PlanPrecision = float64(hits) / float64(metrics.PredictedTools)
	}
	metrics.UnnecessaryTools = metrics.PredictedTools - hits
	return metrics, nil
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type toolSearchAgentPair struct {
	caseID string
	direct ToolSearchAgentABRun
	search ToolSearchAgentABRun
}

// AggregateToolSearchAgentAB validates a paired experiment and computes
// case-cluster bootstrap confidence intervals with a fixed seed. Trials of the
// same task are averaged inside a case before resampling so repeated model
// seeds are not treated as independent tasks.
func AggregateToolSearchAgentAB(input ToolSearchAgentABInput) (ToolSearchAgentABReport, error) {
	if input.Version != "tool-search-agent-ab.v1" {
		return ToolSearchAgentABReport{}, fmt.Errorf("unsupported Agent A/B input version %q", input.Version)
	}
	if strings.TrimSpace(input.Catalog.SourceHash) == "" || strings.TrimSpace(input.Catalog.SurfaceHash) == "" {
		return ToolSearchAgentABReport{}, fmt.Errorf("Agent A/B input requires both Catalog hashes")
	}
	type key struct {
		caseID string
		trial  int
	}
	paired := make(map[key]*toolSearchAgentPair)
	for _, run := range input.Runs {
		if strings.TrimSpace(run.CaseID) == "" || run.Trial < 0 {
			return ToolSearchAgentABReport{}, fmt.Errorf("Agent A/B run requires case_id and non-negative trial")
		}
		if run.ContextTokens < 0 || run.ToolCalls < 0 || run.LatencyMS < 0 {
			return ToolSearchAgentABReport{}, fmt.Errorf("Agent A/B run %s/%d has a negative cost", run.CaseID, run.Trial)
		}
		if run.RecoverySucceeded && !run.RecoveryAttempted {
			return ToolSearchAgentABReport{}, fmt.Errorf("Agent A/B run %s/%d marks recovery success without an attempt", run.CaseID, run.Trial)
		}
		itemKey := key{caseID: run.CaseID, trial: run.Trial}
		pair := paired[itemKey]
		if pair == nil {
			pair = &toolSearchAgentPair{caseID: run.CaseID}
			paired[itemKey] = pair
		}
		switch run.Arm {
		case ToolSearchAgentArmDirectSchema:
			if pair.direct.Arm != "" {
				return ToolSearchAgentABReport{}, fmt.Errorf("duplicate direct_schema run for %s/%d", run.CaseID, run.Trial)
			}
			pair.direct = run
		case ToolSearchAgentArmSearchInspect:
			if pair.search.Arm != "" {
				return ToolSearchAgentABReport{}, fmt.Errorf("duplicate search_inspect run for %s/%d", run.CaseID, run.Trial)
			}
			pair.search = run
		default:
			return ToolSearchAgentABReport{}, fmt.Errorf("unknown Agent A/B arm %q", run.Arm)
		}
	}
	if len(paired) == 0 {
		return ToolSearchAgentABReport{}, fmt.Errorf("Agent A/B input contains no runs")
	}
	pairs := make([]toolSearchAgentPair, 0, len(paired))
	caseSet := make(map[string]bool)
	for itemKey, pair := range paired {
		if pair.direct.Arm == "" || pair.search.Arm == "" {
			return ToolSearchAgentABReport{}, fmt.Errorf("Agent A/B case %s/%d is not paired", itemKey.caseID, itemKey.trial)
		}
		pairs = append(pairs, *pair)
		caseSet[itemKey.caseID] = true
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].caseID == pairs[j].caseID {
			return pairs[i].direct.Trial < pairs[j].direct.Trial
		}
		return pairs[i].caseID < pairs[j].caseID
	})

	directMetrics := aggregateToolSearchAgentArm(pairs, false)
	searchMetrics := aggregateToolSearchAgentArm(pairs, true)
	metrics := []struct {
		name   string
		value  func(ToolSearchAgentABRun) float64
		direct float64
		search float64
	}{
		{"task_success_rate", func(run ToolSearchAgentABRun) float64 { return boolFloat(run.TaskCompleted) }, directMetrics.TaskSuccessRate, searchMetrics.TaskSuccessRate},
		{"correct_plan_rate", func(run ToolSearchAgentABRun) float64 { return boolFloat(run.CorrectToolPlan) }, directMetrics.CorrectPlanRate, searchMetrics.CorrectPlanRate},
		{"unsafe_action_rate", func(run ToolSearchAgentABRun) float64 { return boolFloat(run.UnsafeAction) }, directMetrics.UnsafeActionRate, searchMetrics.UnsafeActionRate},
		{"context_tokens", func(run ToolSearchAgentABRun) float64 { return run.ContextTokens }, directMetrics.AverageContextTokens, searchMetrics.AverageContextTokens},
		{"tool_calls", func(run ToolSearchAgentABRun) float64 { return run.ToolCalls }, directMetrics.AverageToolCalls, searchMetrics.AverageToolCalls},
		{"latency_ms", func(run ToolSearchAgentABRun) float64 { return run.LatencyMS }, directMetrics.AverageLatencyMS, searchMetrics.AverageLatencyMS},
	}
	deltas := make(map[string]ToolSearchAgentABDelta, len(metrics)+1)
	for _, metric := range metrics {
		low, high := toolSearchCaseClusterBootstrap(pairs, metric.value, 10000, 927)
		deltas[metric.name] = ToolSearchAgentABDelta{
			DirectSchema:  metric.direct,
			SearchInspect: metric.search,
			Delta:         metric.search - metric.direct,
			CI95Low:       low,
			CI95High:      high,
		}
	}
	if directMetrics.RecoveryAttempts > 0 && searchMetrics.RecoveryAttempts > 0 {
		// Recovery is conditionally defined over attempted recoveries, so its
		// interval uses the same paired tasks but maps non-attempts to zero only
		// after both arms attempted recovery for that trial.
		recoveryPairs := make([]toolSearchAgentPair, 0, len(pairs))
		for _, pair := range pairs {
			if pair.direct.RecoveryAttempted && pair.search.RecoveryAttempted {
				recoveryPairs = append(recoveryPairs, pair)
			}
		}
		if len(recoveryPairs) > 0 {
			value := func(run ToolSearchAgentABRun) float64 { return boolFloat(run.RecoverySucceeded) }
			low, high := toolSearchCaseClusterBootstrap(recoveryPairs, value, 10000, 928)
			directRecovery := meanAgentRunValue(recoveryPairs, false, value)
			searchRecovery := meanAgentRunValue(recoveryPairs, true, value)
			deltas["recovery_success_rate_paired_attempts"] = ToolSearchAgentABDelta{
				DirectSchema:  directRecovery,
				SearchInspect: searchRecovery,
				Delta:         searchRecovery - directRecovery,
				CI95Low:       low,
				CI95High:      high,
			}
		}
	}
	return ToolSearchAgentABReport{
		Version:       "tool-search-agent-ab-report.v1",
		Catalog:       input.Catalog,
		Cases:         len(caseSet),
		PairedTrials:  len(pairs),
		DirectSchema:  directMetrics,
		SearchInspect: searchMetrics,
		Deltas:        deltas,
		Method:        "paired by case_id/trial; 95% percentile bootstrap clustered by case_id; 10000 resamples; fixed seed",
	}, nil
}

func aggregateToolSearchAgentArm(pairs []toolSearchAgentPair, search bool) ToolSearchAgentABArmMetrics {
	metrics := ToolSearchAgentABArmMetrics{Runs: len(pairs)}
	for _, pair := range pairs {
		run := pair.direct
		if search {
			run = pair.search
		}
		metrics.TaskSuccessRate += boolFloat(run.TaskCompleted)
		metrics.CorrectPlanRate += boolFloat(run.CorrectToolPlan)
		metrics.UnsafeActionRate += boolFloat(run.UnsafeAction)
		metrics.AverageContextTokens += run.ContextTokens
		metrics.AverageToolCalls += run.ToolCalls
		metrics.AverageLatencyMS += run.LatencyMS
		if run.RecoveryAttempted {
			metrics.RecoveryAttempts++
			metrics.RecoverySuccessRate += boolFloat(run.RecoverySucceeded)
		}
	}
	if metrics.Runs > 0 {
		denominator := float64(metrics.Runs)
		metrics.TaskSuccessRate /= denominator
		metrics.CorrectPlanRate /= denominator
		metrics.UnsafeActionRate /= denominator
		metrics.AverageContextTokens /= denominator
		metrics.AverageToolCalls /= denominator
		metrics.AverageLatencyMS /= denominator
	}
	if metrics.RecoveryAttempts > 0 {
		metrics.RecoverySuccessRate /= float64(metrics.RecoveryAttempts)
	}
	return metrics
}

func toolSearchCaseClusterBootstrap(pairs []toolSearchAgentPair, value func(ToolSearchAgentABRun) float64, samples int, seed int64) (float64, float64) {
	byCase := make(map[string][]float64)
	for _, pair := range pairs {
		byCase[pair.caseID] = append(byCase[pair.caseID], value(pair.search)-value(pair.direct))
	}
	caseIDs := make([]string, 0, len(byCase))
	caseMeans := make(map[string]float64, len(byCase))
	for caseID, values := range byCase {
		caseIDs = append(caseIDs, caseID)
		for _, value := range values {
			caseMeans[caseID] += value / float64(len(values))
		}
	}
	sort.Strings(caseIDs)
	if len(caseIDs) == 1 {
		value := caseMeans[caseIDs[0]]
		return value, value
	}
	random := rand.New(rand.NewSource(seed))
	distribution := make([]float64, samples)
	for sample := range samples {
		var total float64
		for range caseIDs {
			total += caseMeans[caseIDs[random.Intn(len(caseIDs))]]
		}
		distribution[sample] = total / float64(len(caseIDs))
	}
	sort.Float64s(distribution)
	return percentile(distribution, 0.025), percentile(distribution, 0.975)
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(quantile * float64(len(sorted)-1))
	return sorted[index]
}

func meanAgentRunValue(pairs []toolSearchAgentPair, search bool, value func(ToolSearchAgentABRun) float64) float64 {
	if len(pairs) == 0 {
		return 0
	}
	var total float64
	for _, pair := range pairs {
		run := pair.direct
		if search {
			run = pair.search
		}
		total += value(run)
	}
	return total / float64(len(pairs))
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
