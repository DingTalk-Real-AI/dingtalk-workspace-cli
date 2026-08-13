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
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"unicode"
	"unicode/utf8"
)

// ToolSearchWorkflowEvaluationCase is a reviewed diagnostic workflow. It is
// intentionally separate from release qrels: the small in-repository fixture
// measures mechanics and cannot approve a ranking configuration.
type ToolSearchWorkflowEvaluationCase struct {
	ID         string   `json:"id"`
	Query      string   `json:"query"`
	Subqueries []string `json:"subqueries"`
	Required   []string `json:"required"`
}

// ToolSearchRetrievalMetrics reports deterministic ranking metrics. Values are
// fractions in [0,1], not percentages.
type ToolSearchRetrievalMetrics struct {
	Cases                 int     `json:"cases"`
	RecallAt1             float64 `json:"recall_at_1"`
	RecallAt5             float64 `json:"recall_at_5"`
	MeanReciprocalRankAt5 float64 `json:"mean_reciprocal_rank_at_5"`
	ZeroResultRate        float64 `json:"zero_result_rate"`
}

// ToolSearchWorkflowMetrics measures whether every required tool is present in
// the shared Top-5 budget and the average required-tool recall.
type ToolSearchWorkflowMetrics struct {
	Cases             int     `json:"cases"`
	CompleteAt5       float64 `json:"complete_at_5"`
	RequiredRecallAt5 float64 `json:"required_recall_at_5"`
}

// ToolSearchIdentityTrustMetrics exercises the complete delivered identity
// set. Exact filtered checks deliberately exclude each canonical path and
// verify that search abstains instead of falling through to a fuzzy sibling.
type ToolSearchIdentityTrustMetrics struct {
	CanonicalCases        int     `json:"canonical_cases"`
	CanonicalPassRate     float64 `json:"canonical_pass_rate"`
	PrimaryCLICases       int     `json:"primary_cli_cases"`
	PrimaryCLIPassRate    float64 `json:"primary_cli_pass_rate"`
	AliasCases            int     `json:"alias_cases"`
	AliasPassRate         float64 `json:"alias_pass_rate"`
	NFKCCases             int     `json:"nfkc_cases"`
	NFKCPassRate          float64 `json:"nfkc_pass_rate"`
	ExactFilteredCases    int     `json:"exact_filtered_cases"`
	ExactFilteredPassRate float64 `json:"exact_filtered_pass_rate"`
}

// ToolSearchNegativeTrustMetrics is a same-source diagnostic only. A
// forbidden exposure means the tool that owns an avoid_when sentence appears
// in the ranking; it does not prove that a model would call that tool.
type ToolSearchNegativeTrustMetrics struct {
	Cases              int     `json:"cases"`
	ExcludedOverBudget int     `json:"excluded_over_budget"`
	ForbiddenAt1       float64 `json:"forbidden_at_1"`
	ForbiddenAt5       float64 `json:"forbidden_at_5"`
}

// ToolSearchIntegrityMetrics verifies invariants that do not depend on
// relevance labels.
type ToolSearchIntegrityMetrics struct {
	ResponsesEvaluated       int `json:"responses_evaluated"`
	CatalogBindingFailures   int `json:"catalog_binding_failures"`
	UnknownCandidateCount    int `json:"unknown_candidate_count"`
	IneligibleCandidateCount int `json:"ineligible_candidate_count"`
	ResponseBudgetViolations int `json:"response_budget_violations"`
}

// ToolSearchTrustMetrics keeps contract integrity separate from ranking
// relevance. Search rank is never presented as a probability of safe use.
type ToolSearchTrustMetrics struct {
	Identity  ToolSearchIdentityTrustMetrics `json:"identity"`
	Negative  ToolSearchNegativeTrustMetrics `json:"negative"`
	Integrity ToolSearchIntegrityMetrics     `json:"integrity"`
}

// ToolSearchContextComparison compares byte-level JSON envelopes, not model
// tokens. The ideal navigation path assumes an oracle already knows the right
// product; its selection success is therefore deliberately not estimated.
type ToolSearchContextComparison struct {
	ToolCount                         int     `json:"tool_count"`
	FullSchemaAllBytes                int     `json:"full_schema_all_bytes"`
	OverviewBytes                     int     `json:"overview_bytes"`
	AverageProductBytes               float64 `json:"average_product_bytes"`
	AverageInspectBytes               float64 `json:"average_inspect_bytes"`
	AverageIdealSchemaNavigationBytes float64 `json:"average_ideal_schema_navigation_bytes"`
	AverageSearchInspectBytes         float64 `json:"average_search_inspect_bytes"`
	ReductionVsFullSchema             float64 `json:"reduction_vs_full_schema"`
	ReductionVsIdealNavigation        float64 `json:"reduction_vs_ideal_navigation"`
}

// ToolSearchDiagnosticComparison is generated from the exact typed Catalog
// embedded in the binary. It is a diagnostic proxy, not an Agent completion
// benchmark: its intent queries come from reviewed use_when prose and therefore
// are not independently authored qrels.
type ToolSearchDiagnosticComparison struct {
	Version                  string                                `json:"version"`
	Catalog                  CatalogVersionRef                     `json:"catalog"`
	Algorithm                string                                `json:"algorithm"`
	TopK                     int                                   `json:"top_k"`
	IntentProxy              ToolSearchRetrievalMetrics            `json:"intent_proxy"`
	IntentExcludedOverBudget int                                   `json:"intent_excluded_over_budget"`
	IntentLanguageSlices     map[string]ToolSearchRetrievalMetrics `json:"intent_language_slices"`
	WorkflowRaw              ToolSearchWorkflowMetrics             `json:"workflow_raw"`
	WorkflowDecomposed       ToolSearchWorkflowMetrics             `json:"workflow_decomposed"`
	Trust                    ToolSearchTrustMetrics                `json:"trust"`
	Context                  ToolSearchContextComparison           `json:"context"`
	AgentABStatus            string                                `json:"agent_ab_status"`
	Limitations              []string                              `json:"limitations"`
}

// BuildDeliveryToolSearchDiagnosticComparison evaluates the shipped Go
// retriever against the declaration-assembled Catalog without reading a
// generated repository JSON artifact. Results belong under policy-tmp.
func BuildDeliveryToolSearchDiagnosticComparison(ctx context.Context, workflows []ToolSearchWorkflowEvaluationCase) (ToolSearchDiagnosticComparison, error) {
	return BuildDeliveryToolSearchDiagnosticComparisonForAlgorithm(ctx, workflows, DefaultToolSearchConfig().LexicalAlgorithm)
}

// BuildDeliveryToolSearchDiagnosticComparisonForAlgorithm runs one shipped Go
// lexical implementation over exactly the same typed Catalog and fixtures.
// This keeps shadow algorithm comparison out of the production default.
func BuildDeliveryToolSearchDiagnosticComparisonForAlgorithm(ctx context.Context, workflows []ToolSearchWorkflowEvaluationCase, algorithm string) (ToolSearchDiagnosticComparison, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return ToolSearchDiagnosticComparison{}, fmt.Errorf("assemble typed Schema registry for evaluation: %w", err)
	}
	loaded := deliverySchemaCatalog()
	config := DefaultToolSearchConfig()
	config.LexicalAlgorithm = algorithm
	config.CatalogSourceHash = loaded.Snapshot.SourceHash
	config.CatalogSurfaceHash = loaded.Snapshot.SurfaceHash
	engine, err := NewToolSearchEngine(loaded.Registry, config)
	if err != nil {
		return ToolSearchDiagnosticComparison{}, err
	}
	allPayload, err := deliverySchemaAllPayload()
	if err != nil {
		return ToolSearchDiagnosticComparison{}, fmt.Errorf("render full Schema: %w", err)
	}
	overviewPayload, err := deliverySchemaOverviewPayload()
	if err != nil {
		return ToolSearchDiagnosticComparison{}, fmt.Errorf("render Schema overview: %w", err)
	}
	allBytes, err := compactJSONSize(allPayload)
	if err != nil {
		return ToolSearchDiagnosticComparison{}, err
	}
	overviewBytes, err := compactJSONSize(stripSchemaPayloadCompact(overviewPayload))
	if err != nil {
		return ToolSearchDiagnosticComparison{}, err
	}

	type proxyCase struct {
		query   string
		gold    string
		product string
	}
	proxyCases := make([]proxyCase, 0)
	intentExcludedOverBudget := 0
	for _, product := range loaded.Registry.Products {
		for _, tool := range product.Tools {
			for _, query := range tool.Selection.UseWhen {
				if query != "" && toolSearchQueryWithinBudget(query) {
					proxyCases = append(proxyCases, proxyCase{query: query, gold: tool.Identity.CanonicalPath, product: product.ID})
				} else if query != "" {
					intentExcludedOverBudget++
				}
			}
		}
	}
	sort.Slice(proxyCases, func(i, j int) bool {
		if proxyCases[i].gold == proxyCases[j].gold {
			return proxyCases[i].query < proxyCases[j].query
		}
		return proxyCases[i].gold < proxyCases[j].gold
	})

	intentAccumulator := &toolSearchRetrievalAccumulator{}
	languageAccumulators := map[string]*toolSearchRetrievalAccumulator{}
	var totalProductBytes, totalInspectBytes, totalSearchBytes float64
	productSizes := make(map[string]int)
	inspectSizes := make(map[string]int)
	for _, item := range proxyCases {
		if err := ctx.Err(); err != nil {
			return ToolSearchDiagnosticComparison{}, err
		}
		response, searchErr := engine.Search(ctx, ToolSearchRequest{Query: item.query, Limit: 5, CandidateLimit: 20})
		if searchErr != nil {
			return ToolSearchDiagnosticComparison{}, fmt.Errorf("search proxy case %s: %w", item.gold, searchErr)
		}
		rank := 0
		for index, candidate := range response.Candidates {
			if candidate.CanonicalPath != item.gold {
				continue
			}
			rank = index + 1
			break
		}
		intentAccumulator.observe(rank, len(response.Candidates) == 0)
		slice := toolSearchQueryLanguageSlice(item.query)
		if languageAccumulators[slice] == nil {
			languageAccumulators[slice] = &toolSearchRetrievalAccumulator{}
		}
		languageAccumulators[slice].observe(rank, len(response.Candidates) == 0)
		encodedResponse, marshalErr := json.Marshal(response)
		if marshalErr != nil {
			return ToolSearchDiagnosticComparison{}, marshalErr
		}
		totalSearchBytes += float64(len(encodedResponse))

		productBytes, ok := productSizes[item.product]
		if !ok {
			payload, payloadErr := queryDeliverySchemaPayload([]string{item.product})
			if payloadErr != nil {
				return ToolSearchDiagnosticComparison{}, payloadErr
			}
			productBytes, payloadErr = compactJSONSize(stripSchemaPayloadCompact(payload))
			if payloadErr != nil {
				return ToolSearchDiagnosticComparison{}, payloadErr
			}
			productSizes[item.product] = productBytes
		}
		totalProductBytes += float64(productBytes)

		inspectBytes, ok := inspectSizes[item.gold]
		if !ok {
			payload, payloadErr := queryDeliverySchemaPayload([]string{item.gold})
			if payloadErr != nil {
				return ToolSearchDiagnosticComparison{}, payloadErr
			}
			payload = stripSchemaPayloadCompact(payload)
			inspectBytes, payloadErr = compactJSONSize(SchemaInspectV1Response{
				Version:  "schema-inspect.v1",
				Catalog:  CatalogVersionRef{SourceHash: loaded.Snapshot.SourceHash, SurfaceHash: loaded.Snapshot.SurfaceHash},
				ToolSpec: payload,
			})
			if payloadErr != nil {
				return ToolSearchDiagnosticComparison{}, payloadErr
			}
			inspectSizes[item.gold] = inspectBytes
		}
		totalInspectBytes += float64(inspectBytes)
	}

	count := float64(len(proxyCases))
	intent := intentAccumulator.metrics()
	languageSlices := make(map[string]ToolSearchRetrievalMetrics, len(languageAccumulators))
	for slice, accumulator := range languageAccumulators {
		languageSlices[slice] = accumulator.metrics()
	}
	contextComparison := ToolSearchContextComparison{
		ToolCount:          len(loaded.Index.CanonicalPaths()),
		FullSchemaAllBytes: allBytes,
		OverviewBytes:      overviewBytes,
	}
	if count > 0 {
		contextComparison.AverageProductBytes = totalProductBytes / count
		contextComparison.AverageInspectBytes = totalInspectBytes / count
		contextComparison.AverageIdealSchemaNavigationBytes = float64(overviewBytes) + contextComparison.AverageProductBytes + contextComparison.AverageInspectBytes
		contextComparison.AverageSearchInspectBytes = totalSearchBytes/count + contextComparison.AverageInspectBytes
		contextComparison.ReductionVsFullSchema = reduction(float64(allBytes), contextComparison.AverageSearchInspectBytes)
		contextComparison.ReductionVsIdealNavigation = reduction(contextComparison.AverageIdealSchemaNavigationBytes, contextComparison.AverageSearchInspectBytes)
	}

	rawWorkflow, decomposedWorkflow, err := evaluateToolSearchWorkflows(ctx, engine, workflows)
	if err != nil {
		return ToolSearchDiagnosticComparison{}, err
	}
	trust, err := evaluateToolSearchTrust(ctx, engine, loaded.Registry)
	if err != nil {
		return ToolSearchDiagnosticComparison{}, err
	}
	return ToolSearchDiagnosticComparison{
		Version:                  "tool-search-diagnostic-comparison.v1",
		Catalog:                  engine.catalogVersion(),
		Algorithm:                engine.lexical.Name(),
		TopK:                     5,
		IntentProxy:              intent,
		IntentExcludedOverBudget: intentExcludedOverBudget,
		IntentLanguageSlices:     languageSlices,
		WorkflowRaw:              rawWorkflow,
		WorkflowDecomposed:       decomposedWorkflow,
		Trust:                    trust,
		Context:                  contextComparison,
		AgentABStatus:            "not_run_requires_independent_tasks_and_model_runs",
		Limitations: []string{
			"intent_proxy queries are reviewed use_when prose from the same Catalog and are not independent qrels",
			"selection prose that exceeds the production query byte or rune budget is counted and excluded rather than weakening runtime limits",
			"direct Schema navigation bytes assume an oracle selects the correct product and do not estimate model selection success",
			"search plus inspect bytes also inspect the gold leaf even when retrieval misses and are an oracle-assisted capacity upper bound, not observed Agent cost",
			"workflow decomposition uses reviewed fixture subqueries and is an upper-bound diagnostic",
			"negative queries are same-source avoid_when prose without alternative gold and measure candidate exposure rather than Agent calls",
			"byte counts are compact JSON bytes rather than provider-specific tokenizer counts",
			"true Agent task success, unsafe action rate, recovery success, latency, and token cost require paired Agent A/B runs",
		},
	}, nil
}

type toolSearchRetrievalAccumulator struct {
	cases       int
	hitAt1      int
	hitAt5      int
	reciprocal  float64
	zeroResults int
}

func (a *toolSearchRetrievalAccumulator) observe(rank int, zeroResults bool) {
	a.cases++
	if zeroResults {
		a.zeroResults++
	}
	if rank == 1 {
		a.hitAt1++
	}
	if rank > 0 && rank <= 5 {
		a.hitAt5++
		a.reciprocal += 1 / float64(rank)
	}
}

func (a *toolSearchRetrievalAccumulator) metrics() ToolSearchRetrievalMetrics {
	result := ToolSearchRetrievalMetrics{Cases: a.cases}
	if a.cases == 0 {
		return result
	}
	denominator := float64(a.cases)
	result.RecallAt1 = float64(a.hitAt1) / denominator
	result.RecallAt5 = float64(a.hitAt5) / denominator
	result.MeanReciprocalRankAt5 = a.reciprocal / denominator
	result.ZeroResultRate = float64(a.zeroResults) / denominator
	return result
}

func toolSearchQueryLanguageSlice(query string) string {
	hasChinese := false
	hasASCIIWord := false
	for _, character := range query {
		if unicode.Is(unicode.Han, character) {
			hasChinese = true
		}
		if character <= unicode.MaxASCII && (unicode.IsLetter(character) || unicode.IsDigit(character)) {
			hasASCIIWord = true
		}
	}
	if hasChinese && hasASCIIWord {
		return "mixed_chinese_ascii"
	}
	if hasChinese {
		return "chinese_only"
	}
	return "non_chinese"
}

func evaluateToolSearchTrust(ctx context.Context, engine *ToolSearchEngine, registry SchemaRegistry) (ToolSearchTrustMetrics, error) {
	result := ToolSearchTrustMetrics{}
	var canonicalPassed, cliPassed, aliasPassed, nfkcPassed, filteredPassed int
	var forbiddenAt1, forbiddenAt5 int
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			result.Identity.CanonicalCases++
			response, err := engine.Search(ctx, ToolSearchRequest{Query: tool.Identity.CanonicalPath})
			if err != nil {
				return result, fmt.Errorf("audit exact canonical %s: %w", tool.Identity.CanonicalPath, err)
			}
			observeToolSearchIntegrity(&result.Integrity, engine, response)
			if exactSearchMatches(response, tool.Identity.CanonicalPath) {
				canonicalPassed++
			}

			if cliPath := tool.Identity.PrimaryCLIPath; cliPath != "" {
				result.Identity.PrimaryCLICases++
				response, err = engine.Search(ctx, ToolSearchRequest{Query: cliPath})
				if err != nil {
					return result, fmt.Errorf("audit exact CLI %s: %w", cliPath, err)
				}
				observeToolSearchIntegrity(&result.Integrity, engine, response)
				if exactSearchMatches(response, tool.Identity.CanonicalPath) {
					cliPassed++
				}
			}
			for _, alias := range tool.Identity.Aliases {
				result.Identity.AliasCases++
				response, err = engine.Search(ctx, ToolSearchRequest{Query: alias})
				if err != nil {
					return result, fmt.Errorf("audit exact alias %s: %w", alias, err)
				}
				observeToolSearchIntegrity(&result.Integrity, engine, response)
				if exactSearchMatches(response, tool.Identity.CanonicalPath) {
					aliasPassed++
				}
			}
			nfkcQuery := toolSearchFullwidthASCII(tool.Identity.CanonicalPath)
			if nfkcQuery != tool.Identity.CanonicalPath {
				result.Identity.NFKCCases++
				response, err = engine.Search(ctx, ToolSearchRequest{Query: nfkcQuery})
				if err != nil {
					return result, fmt.Errorf("audit NFKC identity %s: %w", tool.Identity.CanonicalPath, err)
				}
				observeToolSearchIntegrity(&result.Integrity, engine, response)
				if exactSearchMatches(response, tool.Identity.CanonicalPath) {
					nfkcPassed++
				}
			}

			result.Identity.ExactFilteredCases++
			response, err = engine.Search(ctx, ToolSearchRequest{
				Query:                 tool.Identity.CanonicalPath,
				ExcludeCanonicalPaths: []string{tool.Identity.CanonicalPath},
			})
			if err != nil {
				return result, fmt.Errorf("audit exact filtered %s: %w", tool.Identity.CanonicalPath, err)
			}
			observeToolSearchIntegrity(&result.Integrity, engine, response)
			if response.Strategy == "exact_filtered" && response.Abstained && len(response.Candidates) == 0 && response.ExactFiltered != nil && response.ExactFiltered.CanonicalPath == tool.Identity.CanonicalPath && response.ExactFiltered.Reason == "excluded" {
				filteredPassed++
			}

			for _, query := range tool.Selection.AvoidWhen {
				if query == "" {
					continue
				}
				if !toolSearchQueryWithinBudget(query) {
					result.Negative.ExcludedOverBudget++
					continue
				}
				result.Negative.Cases++
				response, err = engine.Search(ctx, ToolSearchRequest{Query: query, Limit: 5, CandidateLimit: 20})
				if err != nil {
					return result, fmt.Errorf("audit negative %s: %w", tool.Identity.CanonicalPath, err)
				}
				observeToolSearchIntegrity(&result.Integrity, engine, response)
				for rank, candidate := range response.Candidates {
					if candidate.CanonicalPath != tool.Identity.CanonicalPath {
						continue
					}
					if rank == 0 {
						forbiddenAt1++
					}
					forbiddenAt5++
					break
				}
			}
		}
	}
	if result.Identity.CanonicalCases > 0 {
		result.Identity.CanonicalPassRate = float64(canonicalPassed) / float64(result.Identity.CanonicalCases)
	}
	if result.Identity.PrimaryCLICases > 0 {
		result.Identity.PrimaryCLIPassRate = float64(cliPassed) / float64(result.Identity.PrimaryCLICases)
	}
	if result.Identity.AliasCases > 0 {
		result.Identity.AliasPassRate = float64(aliasPassed) / float64(result.Identity.AliasCases)
	}
	if result.Identity.NFKCCases > 0 {
		result.Identity.NFKCPassRate = float64(nfkcPassed) / float64(result.Identity.NFKCCases)
	}
	if result.Identity.ExactFilteredCases > 0 {
		result.Identity.ExactFilteredPassRate = float64(filteredPassed) / float64(result.Identity.ExactFilteredCases)
	}
	if result.Negative.Cases > 0 {
		result.Negative.ForbiddenAt1 = float64(forbiddenAt1) / float64(result.Negative.Cases)
		result.Negative.ForbiddenAt5 = float64(forbiddenAt5) / float64(result.Negative.Cases)
	}
	return result, nil
}

func toolSearchFullwidthASCII(value string) string {
	runes := []rune(value)
	for index, character := range runes {
		if character >= '!' && character <= '~' {
			runes[index] = character + 0xFEE0
		}
	}
	return string(runes)
}

func toolSearchQueryWithinBudget(query string) bool {
	return len(query) <= maxToolSearchQueryBytes && utf8.RuneCountInString(query) <= maxToolSearchQueryRunes
}

func exactSearchMatches(response ToolSearchResponse, canonical string) bool {
	return response.Strategy == "exact_guard" && len(response.Candidates) == 1 && response.Candidates[0].CanonicalPath == canonical && response.Candidates[0].RequiresInspect
}

func observeToolSearchIntegrity(metrics *ToolSearchIntegrityMetrics, engine *ToolSearchEngine, response ToolSearchResponse) {
	metrics.ResponsesEvaluated++
	if response.Catalog != engine.catalogVersion() {
		metrics.CatalogBindingFailures++
	}
	if encoded, err := json.Marshal(response); err != nil || len(encoded) > maxToolSearchResponseBytes {
		metrics.ResponseBudgetViolations++
	}
	for _, candidate := range response.Candidates {
		document, ok := engine.documents[candidate.CanonicalPath]
		if !ok {
			metrics.UnknownCandidateCount++
			continue
		}
		if !toolSearchEligible(document.tool, nil, nil, nil) {
			metrics.IneligibleCandidateCount++
		}
	}
}

func evaluateToolSearchWorkflows(ctx context.Context, engine *ToolSearchEngine, workflows []ToolSearchWorkflowEvaluationCase) (ToolSearchWorkflowMetrics, ToolSearchWorkflowMetrics, error) {
	raw := ToolSearchWorkflowMetrics{Cases: len(workflows)}
	decomposed := ToolSearchWorkflowMetrics{Cases: len(workflows)}
	if len(workflows) == 0 {
		return raw, decomposed, nil
	}
	for _, workflow := range workflows {
		if workflow.ID == "" || workflow.Query == "" || len(workflow.Required) == 0 {
			return raw, decomposed, fmt.Errorf("workflow evaluation case must have id, query and required tools")
		}
		one, err := engine.Search(ctx, ToolSearchRequest{Query: workflow.Query, Limit: 5, CandidateLimit: 20})
		if err != nil {
			return raw, decomposed, fmt.Errorf("search workflow %s: %w", workflow.ID, err)
		}
		many, err := engine.SearchSubqueries(ctx, workflow.Subqueries, ToolSearchRequest{Query: workflow.Query, Limit: 5, CandidateLimit: 20})
		if err != nil {
			return raw, decomposed, fmt.Errorf("search decomposed workflow %s: %w", workflow.ID, err)
		}
		accumulateWorkflowMetrics(&raw, one.Candidates, workflow.Required)
		accumulateWorkflowMetrics(&decomposed, many.Candidates, workflow.Required)
	}
	denominator := float64(len(workflows))
	raw.CompleteAt5 /= denominator
	raw.RequiredRecallAt5 /= denominator
	decomposed.CompleteAt5 /= denominator
	decomposed.RequiredRecallAt5 /= denominator
	return raw, decomposed, nil
}

func accumulateWorkflowMetrics(metrics *ToolSearchWorkflowMetrics, candidates []ToolReference, required []string) {
	found := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		found[candidate.CanonicalPath] = true
	}
	hits := 0
	for _, canonical := range required {
		if found[canonical] {
			hits++
		}
	}
	metrics.RequiredRecallAt5 += float64(hits) / float64(len(required))
	if hits == len(required) {
		metrics.CompleteAt5++
	}
}

func compactJSONSize(value any) (int, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("marshal compact JSON: %w", err)
	}
	return len(encoded), nil
}

func reduction(baseline, candidate float64) float64 {
	if baseline <= 0 {
		return 0
	}
	return math.Max(-1, math.Min(1, 1-candidate/baseline))
}
