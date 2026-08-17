// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// The product-cluster paired bootstrap uses a fixed sample count and seed so a
// sealed qrels run stays reproducible; both are published in the report so an
// auditor does not have to read them out of the source.
const (
	toolSearchIndependentBootstrapSamples = 10_000
	toolSearchIndependentBootstrapSeed    = 20260813
)

// ToolSearchIndependentQrel is an independently authored graded relevance
// judgment. Relevance uses the frozen 1..3 qrels scale.
type ToolSearchIndependentQrel struct {
	Canonical string `json:"canonical"`
	Relevance int    `json:"relevance"`
}

type ToolSearchIndependentWorkflow struct {
	Subqueries []string `json:"subqueries,omitempty"`
	Required   []string `json:"required"`
}

type ToolSearchIndependentCase struct {
	ID              string                         `json:"id"`
	Query           string                         `json:"query"`
	Language        string                         `json:"language"`
	Qrels           []ToolSearchIndependentQrel    `json:"qrels"`
	Forbidden       []string                       `json:"forbidden,omitempty"`
	AlternativeGold []string                       `json:"alternative_gold,omitempty"`
	Workflow        *ToolSearchIndependentWorkflow `json:"workflow,omitempty"`
	ConfusionFamily []string                       `json:"confusion_family,omitempty"`
}

type ToolSearchIndependentRankingMetrics struct {
	Cases                 int     `json:"cases"`
	RecallAt1             float64 `json:"recall_at_1"`
	RecallAt5             float64 `json:"recall_at_5"`
	MeanReciprocalRankAt5 float64 `json:"mean_reciprocal_rank_at_5"`
	NDCGAt5               float64 `json:"ndcg_at_5"`
	ZeroResultRate        float64 `json:"zero_result_rate"`
}

type ToolSearchIndependentSafetyMetrics struct {
	ForbiddenCases        int     `json:"forbidden_cases"`
	ForbiddenExposureAt5  float64 `json:"forbidden_exposure_at_5"`
	AlternativeRecallAt5  float64 `json:"alternative_recall_at_5"`
	SiblingConfusionCases int     `json:"sibling_confusion_cases"`
	SiblingExposureAt5    float64 `json:"sibling_exposure_at_5"`
}

type ToolSearchIndependentReport struct {
	Version            string                                         `json:"version"`
	Catalog            CatalogVersionRef                              `json:"catalog"`
	Algorithm          string                                         `json:"algorithm"`
	ControlAlgorithm   string                                         `json:"control_algorithm"`
	Overall            ToolSearchIndependentRankingMetrics            `json:"overall"`
	ControlOverall     ToolSearchIndependentRankingMetrics            `json:"control_overall"`
	RecallAt5Delta     float64                                        `json:"recall_at_5_delta"`
	ProductClusterCI95 ToolSearchConfidenceInterval                   `json:"product_cluster_recall_at_5_delta_ci_95"`
	LanguageSlices     map[string]ToolSearchIndependentRankingMetrics `json:"language_slices"`
	Safety             ToolSearchIndependentSafetyMetrics             `json:"safety"`
	Workflow           ToolSearchWorkflowMetrics                      `json:"workflow"`
	// BootstrapSamples/BootstrapSeed are set only by the paired delivery
	// evaluation that computes ProductClusterCI95; they make that interval
	// reproducible without reading the source.
	BootstrapSamples int   `json:"bootstrap_samples,omitempty"`
	BootstrapSeed    int64 `json:"bootstrap_seed,omitempty"`
}

type ToolSearchConfidenceInterval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

type independentRankingAccumulator struct {
	metrics ToolSearchIndependentRankingMetrics
	recall1 float64
	recall5 float64
	mrr5    float64
	ndcg5   float64
	zero    int
}

// BuildDeliveryToolSearchIndependentEvaluation evaluates the action-ranking
// candidate against the shipped fielded BM25 control on sealed external qrels.
// It never tunes parameters or reads Catalog JSON; the caller owns qrels
// sealing and threshold enforcement. The candidate may become the runtime
// default only after the independently frozen default-switch gate passes.
func BuildDeliveryToolSearchIndependentEvaluation(ctx context.Context, cases []ToolSearchIndependentCase) (ToolSearchIndependentReport, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return ToolSearchIndependentReport{}, err
	}
	loaded := deliverySchemaCatalog()
	candidateConfig := DefaultToolSearchConfig()
	candidateConfig.LexicalAlgorithm = ToolSearchLexicalBM25Action
	candidateConfig.CatalogSourceHash = loaded.Snapshot.SourceHash
	candidateConfig.CatalogSurfaceHash = loaded.Snapshot.SurfaceHash
	engine, err := NewToolSearchEngine(loaded.Registry, candidateConfig)
	if err != nil {
		return ToolSearchIndependentReport{}, err
	}
	report, err := EvaluateToolSearchIndependent(ctx, engine, cases)
	if err != nil {
		return ToolSearchIndependentReport{}, err
	}
	controlConfig := DefaultToolSearchConfig()
	controlConfig.CatalogSourceHash = loaded.Snapshot.SourceHash
	controlConfig.CatalogSurfaceHash = loaded.Snapshot.SurfaceHash
	control, err := NewToolSearchEngine(loaded.Registry, controlConfig)
	if err != nil {
		return ToolSearchIndependentReport{}, err
	}
	controlReport, err := EvaluateToolSearchIndependent(ctx, control, cases)
	if err != nil {
		return ToolSearchIndependentReport{}, err
	}
	report.ControlAlgorithm = control.lexical.Name()
	report.ControlOverall = controlReport.Overall
	report.RecallAt5Delta = report.Overall.RecallAt5 - report.ControlOverall.RecallAt5
	report.ProductClusterCI95, err = pairedProductClusterRecallCI(ctx, engine, control, cases, toolSearchIndependentBootstrapSamples, toolSearchIndependentBootstrapSeed)
	if err != nil {
		return ToolSearchIndependentReport{}, err
	}
	report.BootstrapSamples = toolSearchIndependentBootstrapSamples
	report.BootstrapSeed = toolSearchIndependentBootstrapSeed
	return report, nil
}

func EvaluateToolSearchIndependent(ctx context.Context, engine *ToolSearchEngine, cases []ToolSearchIndependentCase) (ToolSearchIndependentReport, error) {
	if engine == nil || len(cases) == 0 {
		return ToolSearchIndependentReport{}, fmt.Errorf("independent evaluation requires an engine and non-empty cases")
	}
	overall := &independentRankingAccumulator{}
	slices := map[string]*independentRankingAccumulator{}
	var forbiddenCases, forbiddenExposed int
	var siblingCases, siblingExposed int
	var alternativeRecall float64
	workflowCases := 0
	workflowComplete := 0
	workflowRequiredRecall := 0.0
	for _, item := range cases {
		if err := ctx.Err(); err != nil {
			return ToolSearchIndependentReport{}, err
		}
		response, searchErr := engine.Search(ctx, ToolSearchRequest{Query: item.Query, Limit: 5, CandidateLimit: 20})
		if searchErr != nil {
			return ToolSearchIndependentReport{}, fmt.Errorf("evaluate %s: %w", item.ID, searchErr)
		}
		overall.observe(response.Candidates, item.Qrels)
		if slices[item.Language] == nil {
			slices[item.Language] = &independentRankingAccumulator{}
		}
		slices[item.Language].observe(response.Candidates, item.Qrels)
		if len(item.Forbidden) > 0 {
			forbiddenCases++
			if candidatesContainAny(response.Candidates, item.Forbidden) {
				forbiddenExposed++
			}
			alternativeRecall += candidateSetRecall(response.Candidates, item.AlternativeGold)
		}
		if len(item.ConfusionFamily) > 0 {
			siblingCases++
			if candidatesContainAny(response.Candidates, item.ConfusionFamily) {
				siblingExposed++
			}
		}
		if item.Workflow != nil {
			workflowCases++
			workflowResponse := response
			if len(item.Workflow.Subqueries) > 0 {
				workflowResponse, searchErr = engine.SearchSubqueries(ctx, item.Workflow.Subqueries, ToolSearchRequest{Query: item.Query, Limit: 5, CandidateLimit: 20})
				if searchErr != nil {
					return ToolSearchIndependentReport{}, fmt.Errorf("evaluate workflow %s: %w", item.ID, searchErr)
				}
			}
			recall := candidateSetRecall(workflowResponse.Candidates, item.Workflow.Required)
			workflowRequiredRecall += recall
			if recall == 1 {
				workflowComplete++
			}
		}
	}
	report := ToolSearchIndependentReport{
		Version: "tool-search-independent-evaluation.v1", Catalog: engine.catalogVersion(), Algorithm: engine.lexical.Name(),
		Overall: overall.result(), LanguageSlices: map[string]ToolSearchIndependentRankingMetrics{},
		Safety:   ToolSearchIndependentSafetyMetrics{ForbiddenCases: forbiddenCases, SiblingConfusionCases: siblingCases},
		Workflow: ToolSearchWorkflowMetrics{Cases: workflowCases},
	}
	for name, accumulator := range slices {
		report.LanguageSlices[name] = accumulator.result()
	}
	if forbiddenCases > 0 {
		report.Safety.ForbiddenExposureAt5 = float64(forbiddenExposed) / float64(forbiddenCases)
		report.Safety.AlternativeRecallAt5 = alternativeRecall / float64(forbiddenCases)
	}
	if siblingCases > 0 {
		report.Safety.SiblingExposureAt5 = float64(siblingExposed) / float64(siblingCases)
	}
	if workflowCases > 0 {
		report.Workflow.CompleteAt5 = float64(workflowComplete) / float64(workflowCases)
		report.Workflow.RequiredRecallAt5 = workflowRequiredRecall / float64(workflowCases)
	}
	return report, nil
}

func pairedProductClusterRecallCI(ctx context.Context, candidate, control *ToolSearchEngine, cases []ToolSearchIndependentCase, iterations int, seed int64) (ToolSearchConfidenceInterval, error) {
	clusters := map[string][]float64{}
	for _, item := range cases {
		candidateResponse, err := candidate.Search(ctx, ToolSearchRequest{Query: item.Query, Limit: 5, CandidateLimit: 20})
		if err != nil {
			return ToolSearchConfidenceInterval{}, err
		}
		controlResponse, err := control.Search(ctx, ToolSearchRequest{Query: item.Query, Limit: 5, CandidateLimit: 20})
		if err != nil {
			return ToolSearchConfidenceInterval{}, err
		}
		product := "unknown"
		if len(item.Qrels) > 0 {
			if split := strings.SplitN(item.Qrels[0].Canonical, ".", 2); len(split) > 0 {
				product = split[0]
			}
		}
		clusters[product] = append(clusters[product], candidateSetRecall(candidateResponse.Candidates, qrelCanonicals(item.Qrels))-candidateSetRecall(controlResponse.Candidates, qrelCanonicals(item.Qrels)))
	}
	clusterNames := make([]string, 0, len(clusters))
	clusterMeans := map[string]float64{}
	for name, values := range clusters {
		clusterNames = append(clusterNames, name)
		for _, value := range values {
			clusterMeans[name] += value
		}
		clusterMeans[name] /= float64(len(values))
	}
	sort.Strings(clusterNames)
	if len(clusterNames) == 0 || iterations <= 0 {
		return ToolSearchConfidenceInterval{}, fmt.Errorf("paired product bootstrap requires cases and iterations")
	}
	random := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic evaluation bootstrap
	values := make([]float64, iterations)
	for iteration := range values {
		for sample := 0; sample < len(clusterNames); sample++ {
			values[iteration] += clusterMeans[clusterNames[random.Intn(len(clusterNames))]]
		}
		values[iteration] /= float64(len(clusterNames))
	}
	sort.Float64s(values)
	return ToolSearchConfidenceInterval{Lower: percentile(values, 0.025), Upper: percentile(values, 0.975)}, nil
}

func qrelCanonicals(qrels []ToolSearchIndependentQrel) []string {
	values := make([]string, 0, len(qrels))
	for _, qrel := range qrels {
		values = append(values, qrel.Canonical)
	}
	return values
}

func (a *independentRankingAccumulator) observe(candidates []ToolReference, qrels []ToolSearchIndependentQrel) {
	a.metrics.Cases++
	if len(candidates) == 0 {
		a.zero++
	}
	relevance := make(map[string]int, len(qrels))
	ideal := make([]int, 0, len(qrels))
	for _, qrel := range qrels {
		relevance[qrel.Canonical] = qrel.Relevance
		ideal = append(ideal, qrel.Relevance)
	}
	found := 0
	first := 0
	dcg := 0.0
	for rank, candidate := range candidates {
		if rank >= 5 {
			break
		}
		rel := relevance[candidate.CanonicalPath]
		if rel == 0 {
			continue
		}
		found++
		if first == 0 {
			first = rank + 1
		}
		dcg += (math.Pow(2, float64(rel)) - 1) / math.Log2(float64(rank+2))
	}
	if len(candidates) > 0 && relevance[candidates[0].CanonicalPath] > 0 {
		a.recall1 += 1 / float64(len(qrels))
	}
	if len(qrels) > 0 {
		a.recall5 += float64(found) / float64(len(qrels))
	}
	if first > 0 {
		a.mrr5 += 1 / float64(first)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ideal)))
	idcg := 0.0
	for rank, rel := range ideal {
		if rank >= 5 {
			break
		}
		idcg += (math.Pow(2, float64(rel)) - 1) / math.Log2(float64(rank+2))
	}
	if idcg > 0 {
		a.ndcg5 += dcg / idcg
	}
}

func (a *independentRankingAccumulator) result() ToolSearchIndependentRankingMetrics {
	result := a.metrics
	if result.Cases == 0 {
		return result
	}
	denominator := float64(result.Cases)
	result.RecallAt1 = a.recall1 / denominator
	result.RecallAt5 = a.recall5 / denominator
	result.MeanReciprocalRankAt5 = a.mrr5 / denominator
	result.NDCGAt5 = a.ndcg5 / denominator
	result.ZeroResultRate = float64(a.zero) / denominator
	return result
}

func candidatesContainAny(candidates []ToolReference, paths []string) bool {
	for _, candidate := range candidates {
		if stringSliceContains(paths, candidate.CanonicalPath) {
			return true
		}
	}
	return false
}

func candidateSetRecall(candidates []ToolReference, required []string) float64 {
	if len(required) == 0 {
		return 0
	}
	found := 0
	for _, canonical := range required {
		for _, candidate := range candidates {
			if candidate.CanonicalPath == canonical {
				found++
				break
			}
		}
	}
	return float64(found) / float64(len(required))
}
