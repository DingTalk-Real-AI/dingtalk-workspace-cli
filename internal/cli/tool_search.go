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
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"golang.org/x/text/unicode/norm"
)

const (
	defaultToolSearchLimit          = 5
	defaultToolSearchCandidateLimit = 20
	maxToolSearchLimit              = 20
	maxToolSearchCandidateLimit     = 100
	maxToolSearchQueryRunes         = 256
	maxToolSearchQueryBytes         = 2 * 1024
	maxToolSearchSubqueries         = 8
	maxToolSearchSubqueriesBytes    = 2 * 1024
	maxToolSearchSummaryRunes       = 256
	maxToolSearchResponseBytes      = 8 * 1024
	maxToolSearchRequestBytes       = 64 * 1024
)

var toolSearchIdentifierPattern = regexp.MustCompile(`[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)+`)

// ToolSearchFieldWeights keeps lexical relevance tuning explicit and
// versionable. The defaults are an evaluated starting point, not a contract:
// release tuning must use a query/qrels set that is independent of the
// reviewed use_when prose included in the production index.
type ToolSearchFieldWeights struct {
	Identity    float64
	Summary     float64
	Description float64
	Parameters  float64
	UseWhen     float64
}

// ToolSearchConfig controls the local, dependency-free retrieval stage.
// IncludeUseWhen is opt-in because the reviewed use_when prose is also used as
// a selection fixture. Enabling it requires an independently authored qrels
// set so evaluation cannot index the answer sentence.
type ToolSearchConfig struct {
	DefaultLimit       int
	DefaultCandidates  int
	LexicalAlgorithm   string
	IncludeUseWhen     bool
	CatalogSourceHash  string
	CatalogSurfaceHash string
	FieldWeights       ToolSearchFieldWeights
}

// DefaultToolSearchConfig returns the zero-external-dependency DWS baseline.
func DefaultToolSearchConfig() ToolSearchConfig {
	return ToolSearchConfig{
		DefaultLimit:      defaultToolSearchLimit,
		DefaultCandidates: defaultToolSearchCandidateLimit,
		LexicalAlgorithm:  ToolSearchLexicalBM25Action,
		IncludeUseWhen:    false,
		FieldWeights: ToolSearchFieldWeights{
			Identity:    8,
			Summary:     5,
			Description: 2,
			Parameters:  2,
			UseWhen:     3,
		},
	}
}

// ToolSearchRequest is one action-sized retrieval request. ProductIDs and
// ExcludeCanonicalPaths are hard filters, not relevance hints.
type ToolSearchRequest struct {
	Query                 string
	Limit                 int
	CandidateLimit        int
	ProductIDs            []string
	Effects               []string
	ExcludeCanonicalPaths []string
}

// ToolReference is intentionally smaller than a ToolSpec. An Agent must
// inspect the selected canonical path before execution instead of guessing
// parameters from search output.
type ToolReference struct {
	CanonicalPath   string   `json:"canonical_path"`
	PrimaryCLIPath  string   `json:"primary_cli_path"`
	ProductID       string   `json:"product_id"`
	Title           string   `json:"title,omitempty"`
	AgentSummary    string   `json:"agent_summary,omitempty"`
	Effect          string   `json:"effect,omitempty"`
	Risk            string   `json:"risk,omitempty"`
	Confirmation    string   `json:"confirmation,omitempty"`
	Idempotency     string   `json:"idempotency,omitempty"`
	Rank            int      `json:"rank"`
	MatchedFields   []string `json:"matched_fields,omitempty"`
	RankSources     []string `json:"rank_sources"`
	TruncatedFields []string `json:"truncated_fields,omitempty"`
	RequiresInspect bool     `json:"requires_inspect"`
	score           float64
}

// CatalogVersionRef binds Search and Inspect to one immutable Catalog
// generation.
type CatalogVersionRef struct {
	SourceHash  string `json:"source_hash"`
	SurfaceHash string `json:"surface_hash"`
}

// ToolSearchExactFiltered records why an exact identity was rejected. A
// filtered exact identity never falls through to fuzzy sibling suggestions.
type ToolSearchExactFiltered struct {
	CanonicalPath string `json:"canonical_path"`
	Reason        string `json:"reason"`
}

// ToolSearchResponse carries a bounded local ranking without exposing a
// complete executable Schema.
type ToolSearchResponse struct {
	Version       string                   `json:"version"`
	Catalog       CatalogVersionRef        `json:"catalog"`
	Query         string                   `json:"query"`
	Subqueries    []string                 `json:"subqueries,omitempty"`
	Strategy      string                   `json:"strategy"`
	Candidates    []ToolReference          `json:"candidates"`
	ExactFiltered *ToolSearchExactFiltered `json:"exact_filtered,omitempty"`
	Abstained     bool                     `json:"abstained,omitempty"`
	Truncated     bool                     `json:"truncated,omitempty"`
}

type toolSearchField string

const (
	toolSearchIdentity    toolSearchField = "identity"
	toolSearchSummary     toolSearchField = "summary"
	toolSearchDescription toolSearchField = "description"
	toolSearchParameters  toolSearchField = "parameters"
	toolSearchUseWhen     toolSearchField = "use_when"
)

var toolSearchFieldOrder = [...]toolSearchField{
	toolSearchIdentity,
	toolSearchSummary,
	toolSearchDescription,
	toolSearchParameters,
	toolSearchUseWhen,
}

type toolSearchDocument struct {
	tool   ToolSpec
	fields map[toolSearchField]string
}

type toolSearchBM25Document struct {
	terms  map[string]int
	length int
}

type toolSearchBM25Index struct {
	documents     map[string]toolSearchBM25Document
	idf           map[string]float64
	averageLength float64
	k1            float64
	b             float64
}

type toolSearchQueryTerm struct {
	term  string
	count int
}

// ToolSearchEngine owns one immutable in-memory index over a typed registry.
// It performs no network calls and has no external-ranking path.
type ToolSearchEngine struct {
	index     SchemaIndex
	config    ToolSearchConfig
	documents map[string]toolSearchDocument
	lexical   LexicalRetriever
}

// NewDeliveryToolSearchEngine indexes the immutable in-memory Catalog
// assembled from live Go declarations by RegisterSchemaSourceRoot →
// ResolveSchemaBuild. There is no committed or runtime-read Catalog JSON.
func NewDeliveryToolSearchEngine() (*ToolSearchEngine, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return nil, fmt.Errorf("assemble typed Schema registry for tool search: %w", err)
	}
	loaded := deliverySchemaCatalog()
	config := DefaultToolSearchConfig()
	config.CatalogSourceHash = loaded.Snapshot.SourceHash
	config.CatalogSurfaceHash = loaded.Snapshot.SurfaceHash
	return NewToolSearchEngine(loaded.Registry, config)
}

// NewToolSearchEngine builds a deterministic local index from the same typed
// registry used by schema overview, inspect and --all projections.
func NewToolSearchEngine(registry SchemaRegistry, config ToolSearchConfig) (*ToolSearchEngine, error) {
	index, err := registry.Index()
	if err != nil {
		return nil, fmt.Errorf("index tool search registry: %w", err)
	}
	config = normalizeToolSearchConfig(config)
	documents := make(map[string]toolSearchDocument, len(index.CanonicalPaths()))
	for _, product := range index.Registry().Products {
		for _, tool := range product.Tools {
			canonical := tool.Identity.CanonicalPath
			fields := toolSearchDocumentFields(tool, config.IncludeUseWhen)
			documents[canonical] = toolSearchDocument{tool: tool, fields: fields}
		}
	}
	lexical, err := newToolSearchLexicalRetriever(documents, config)
	if err != nil {
		return nil, err
	}
	return &ToolSearchEngine{
		index:     index,
		config:    config,
		documents: documents,
		lexical:   lexical,
	}, nil
}

// Search retrieves one action-sized query. Exact canonical or CLI identities
// bypass relevance so natural-language ranking cannot demote them.
func (e *ToolSearchEngine) Search(ctx context.Context, request ToolSearchRequest) (ToolSearchResponse, error) {
	if e == nil {
		return ToolSearchResponse{}, fmt.Errorf("tool search engine is nil")
	}
	if err := ctx.Err(); err != nil {
		return ToolSearchResponse{}, err
	}
	request, err := e.normalizeRequest(request)
	if err != nil {
		return ToolSearchResponse{}, err
	}
	response := ToolSearchResponse{
		Version:    "tool-search.v1",
		Catalog:    e.catalogVersion(),
		Query:      request.Query,
		Strategy:   e.lexical.Name(),
		Candidates: []ToolReference{},
	}
	allowedProducts := stringSet(request.ProductIDs)
	allowedEffects := stringSet(request.Effects)
	excluded := stringSet(request.ExcludeCanonicalPaths)
	if exact, ok := e.index.ResolveQuery(norm.NFKC.String(request.Query)); ok {
		if reason := toolSearchIneligibleReason(exact, allowedProducts, allowedEffects, excluded); reason != "" {
			response.Strategy = "exact_filtered"
			response.Abstained = true
			response.ExactFiltered = &ToolSearchExactFiltered{
				CanonicalPath: exact.Identity.CanonicalPath,
				Reason:        reason,
			}
			return finalizeToolSearchResponse(response)
		}
		response.Strategy = "exact_guard"
		reference := toolReference(exact, 1, []string{"identity"}, []string{"exact"})
		reference.Rank = 1
		response.Candidates = []ToolReference{reference}
		return finalizeToolSearchResponse(response)
	}

	eligibleCanonical := make([]string, 0, len(e.documents))
	for canonical, document := range e.documents {
		if toolSearchEligible(document.tool, allowedProducts, allowedEffects, excluded) {
			eligibleCanonical = append(eligibleCanonical, canonical)
		}
	}
	sort.Strings(eligibleCanonical)
	hits, err := e.lexical.Retrieve(ctx, ToolSearchLexicalRequest{
		Query:                  request.Query,
		CandidateLimit:         request.CandidateLimit,
		EligibleCanonicalPaths: eligibleCanonical,
	})
	if err != nil {
		return ToolSearchResponse{}, fmt.Errorf("retrieve local tool candidates: %w", err)
	}
	references := make([]ToolReference, 0, len(hits))
	for _, item := range hits {
		references = append(references, toolReference(
			e.documents[item.CanonicalPath].tool,
			item.Score,
			item.MatchedFields,
			[]string{e.lexical.Name()},
		))
	}

	if len(references) > request.Limit {
		references = references[:request.Limit]
	}
	setToolReferenceRanks(references)
	response.Candidates = references
	return finalizeToolSearchResponse(response)
}

// SearchSubqueries merges action-sized searches round-robin so one action
// cannot consume the complete Top-K budget of a compound workflow.
func (e *ToolSearchEngine) SearchSubqueries(ctx context.Context, subqueries []string, request ToolSearchRequest) (ToolSearchResponse, error) {
	cleaned := stableUniqueStrings(subqueries)
	if len(cleaned) == 0 {
		return ToolSearchResponse{}, apperrors.NewValidation(
			"tool search requires at least one non-empty subquery",
			apperrors.WithReason("subquery_required"),
		)
	}
	if len(cleaned) > maxToolSearchSubqueries {
		return ToolSearchResponse{}, apperrors.NewValidation(
			fmt.Sprintf("tool search accepts at most %d subqueries", maxToolSearchSubqueries),
			apperrors.WithReason("too_many_subqueries"),
		)
	}
	aggregateBytes := 0
	for _, query := range cleaned {
		aggregateBytes += len(query)
	}
	if aggregateBytes > maxToolSearchSubqueriesBytes {
		return ToolSearchResponse{}, apperrors.NewValidation(
			fmt.Sprintf("tool search subqueries exceed %d aggregate UTF-8 bytes", maxToolSearchSubqueriesBytes),
			apperrors.WithReason("subqueries_too_large"),
		)
	}
	base, err := e.normalizeRequest(request)
	if err != nil && strings.TrimSpace(request.Query) != "" {
		return ToolSearchResponse{}, err
	}
	if strings.TrimSpace(request.Query) == "" {
		base = request
		base.Query = cleaned[0]
		base, err = e.normalizeRequest(base)
		if err != nil {
			return ToolSearchResponse{}, err
		}
	}
	perQuery := make([]ToolSearchResponse, 0, len(cleaned))
	responseQuery := strings.TrimSpace(request.Query)
	if responseQuery == "" {
		responseQuery = strings.Join(cleaned, " + ")
	}
	for _, query := range cleaned {
		item := base
		item.Query = query
		// A subquery only needs enough ranked items to fill the shared final
		// budget. Expanding every arm to Top-20 caused an internal response
		// truncation warning to leak into an otherwise complete Top-5 result.
		item.Limit = base.Limit
		result, searchErr := e.Search(ctx, item)
		if searchErr != nil {
			return ToolSearchResponse{}, searchErr
		}
		if result.ExactFiltered != nil {
			return finalizeToolSearchResponse(ToolSearchResponse{
				Version:       "tool-search.v1",
				Catalog:       e.catalogVersion(),
				Query:         responseQuery,
				Subqueries:    cleaned,
				Strategy:      "decomposed_exact_filtered",
				Abstained:     true,
				ExactFiltered: result.ExactFiltered,
			})
		}
		perQuery = append(perQuery, result)
	}
	merged := make([]ToolReference, 0, base.Limit)
	seen := make(map[string]bool)
	for rank := 0; len(merged) < base.Limit; rank++ {
		added := false
		for _, result := range perQuery {
			if rank >= len(result.Candidates) {
				continue
			}
			added = true
			candidate := result.Candidates[rank]
			if seen[candidate.CanonicalPath] {
				continue
			}
			seen[candidate.CanonicalPath] = true
			merged = append(merged, candidate)
			if len(merged) == base.Limit {
				break
			}
		}
		if !added {
			break
		}
	}
	setToolReferenceRanks(merged)
	response := ToolSearchResponse{
		Version:    "tool-search.v1",
		Catalog:    e.catalogVersion(),
		Query:      responseQuery,
		Subqueries: cleaned,
		Strategy:   "decomposed_round_robin",
		Candidates: merged,
	}
	return finalizeToolSearchResponse(response)
}

func normalizeToolSearchConfig(config ToolSearchConfig) ToolSearchConfig {
	defaults := DefaultToolSearchConfig()
	if config.DefaultLimit <= 0 {
		config.DefaultLimit = defaults.DefaultLimit
	}
	if config.DefaultLimit > maxToolSearchLimit {
		config.DefaultLimit = maxToolSearchLimit
	}
	if config.DefaultCandidates <= 0 {
		config.DefaultCandidates = defaults.DefaultCandidates
	}
	if config.DefaultCandidates > maxToolSearchCandidateLimit {
		config.DefaultCandidates = maxToolSearchCandidateLimit
	}
	if config.DefaultCandidates < config.DefaultLimit {
		config.DefaultCandidates = config.DefaultLimit
	}
	if strings.TrimSpace(config.LexicalAlgorithm) == "" {
		config.LexicalAlgorithm = defaults.LexicalAlgorithm
	}
	if config.FieldWeights == (ToolSearchFieldWeights{}) {
		config.FieldWeights = defaults.FieldWeights
	}
	config.CatalogSourceHash = strings.TrimSpace(config.CatalogSourceHash)
	config.CatalogSurfaceHash = strings.TrimSpace(config.CatalogSurfaceHash)
	return config
}

func (e *ToolSearchEngine) normalizeRequest(request ToolSearchRequest) (ToolSearchRequest, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return ToolSearchRequest{}, apperrors.NewValidation(
			"tool search query is required",
			apperrors.WithReason("query_required"),
		)
	}
	if utf8.RuneCountInString(request.Query) > maxToolSearchQueryRunes || len(request.Query) > maxToolSearchQueryBytes {
		return ToolSearchRequest{}, apperrors.NewValidation(
			fmt.Sprintf("tool search query exceeds %d Unicode scalars or %d UTF-8 bytes", maxToolSearchQueryRunes, maxToolSearchQueryBytes),
			apperrors.WithReason("query_too_long"),
		)
	}
	if err := apperrors.RejectControlChars(request.Query, "tool search query"); err != nil {
		return ToolSearchRequest{}, apperrors.NewValidation(err.Error(), apperrors.WithReason("dangerous_query_unicode"))
	}
	if request.Limit < 0 || request.Limit > maxToolSearchLimit {
		return ToolSearchRequest{}, apperrors.NewValidation(
			fmt.Sprintf("tool search limit must be between 1 and %d", maxToolSearchLimit),
			apperrors.WithReason("invalid_limit"),
		)
	}
	if request.Limit == 0 {
		request.Limit = e.config.DefaultLimit
	}
	if request.CandidateLimit < 0 || request.CandidateLimit > maxToolSearchCandidateLimit {
		return ToolSearchRequest{}, apperrors.NewValidation(
			fmt.Sprintf("tool search candidate limit must be between 1 and %d", maxToolSearchCandidateLimit),
			apperrors.WithReason("invalid_candidate_limit"),
		)
	}
	if request.CandidateLimit == 0 {
		request.CandidateLimit = e.config.DefaultCandidates
	}
	if request.CandidateLimit < request.Limit {
		request.CandidateLimit = request.Limit
	}
	request.ProductIDs = sortedUniqueStrings(request.ProductIDs)
	request.Effects = sortedUniqueStrings(request.Effects)
	request.ExcludeCanonicalPaths = sortedUniqueStrings(request.ExcludeCanonicalPaths)
	for field, values := range map[string][]string{
		"product_ids":       request.ProductIDs,
		"effects":           request.Effects,
		"exclude_canonical": request.ExcludeCanonicalPaths,
	} {
		for _, value := range values {
			if err := apperrors.RejectControlChars(value, field); err != nil {
				return ToolSearchRequest{}, apperrors.NewValidation(err.Error(), apperrors.WithReason("dangerous_filter_unicode"))
			}
		}
	}
	return request, nil
}

func toolSearchDocumentFields(tool ToolSpec, includeUseWhen bool) map[toolSearchField]string {
	identity := []string{
		tool.Identity.CanonicalPath,
		tool.Identity.Path,
		tool.Identity.CLIPath,
		tool.Identity.PrimaryCLIPath,
		tool.Identity.Name,
		tool.Identity.CLIName,
		tool.Identity.Group,
		tool.Identity.ProductID,
	}
	identity = append(identity, tool.Identity.Aliases...)
	parameters := make([]string, 0, len(tool.Parameters)*4)
	for _, parameter := range tool.Parameters {
		parameters = append(parameters,
			parameter.Name,
			parameter.Property,
			parameter.Description,
			parameter.InterfaceDescription,
			parameter.Type,
			parameter.InterfaceType,
		)
		parameters = append(parameters, parameter.Enum...)
	}
	useWhen := ""
	if includeUseWhen {
		useWhen = strings.Join(tool.Selection.UseWhen, " ")
	}
	return map[toolSearchField]string{
		toolSearchIdentity:    strings.Join(identity, " "),
		toolSearchSummary:     strings.Join([]string{tool.Selection.AgentSummary, tool.Title, tool.Display}, " "),
		toolSearchDescription: tool.Description,
		toolSearchParameters:  strings.Join(parameters, " "),
		toolSearchUseWhen:     useWhen,
	}
}

func newToolSearchBM25Index(documents map[string][]string) toolSearchBM25Index {
	index := toolSearchBM25Index{
		documents: make(map[string]toolSearchBM25Document, len(documents)),
		idf:       make(map[string]float64),
		k1:        0.9,
		b:         0.4,
	}
	documentFrequency := make(map[string]int)
	var totalLength int
	for canonical, tokens := range documents {
		terms := make(map[string]int)
		for _, token := range tokens {
			terms[token]++
		}
		index.documents[canonical] = toolSearchBM25Document{terms: terms, length: len(tokens)}
		totalLength += len(tokens)
		for term := range terms {
			documentFrequency[term]++
		}
	}
	if len(documents) > 0 {
		index.averageLength = float64(totalLength) / float64(len(documents))
	}
	if index.averageLength <= 0 {
		index.averageLength = 1
	}
	count := float64(len(documents))
	for term, frequency := range documentFrequency {
		index.idf[term] = math.Log(1 + (count-float64(frequency)+0.5)/(float64(frequency)+0.5))
	}
	return index
}

func (i toolSearchBM25Index) score(canonical string, queryTerms []toolSearchQueryTerm) float64 {
	document, ok := i.documents[canonical]
	if !ok {
		return 0
	}
	length := float64(document.length)
	if length < 1 {
		length = 1
	}
	score := 0.0
	// queryTerms is sorted once per request. A Go map iteration here would make
	// floating-point accumulation order process-dependent and could move nearly
	// tied candidates across the Top-K boundary.
	for _, queryTerm := range queryTerms {
		frequency := float64(document.terms[queryTerm.term])
		if frequency == 0 {
			continue
		}
		denominator := frequency + i.k1*(1-i.b+i.b*length/i.averageLength)
		score += i.idf[queryTerm.term] * (frequency * (i.k1 + 1) / denominator) * math.Log1p(float64(queryTerm.count))
	}
	return score
}

func toolSearchFieldWeight(weights ToolSearchFieldWeights, field toolSearchField) float64 {
	switch field {
	case toolSearchIdentity:
		return weights.Identity
	case toolSearchSummary:
		return weights.Summary
	case toolSearchDescription:
		return weights.Description
	case toolSearchParameters:
		return weights.Parameters
	case toolSearchUseWhen:
		return weights.UseWhen
	default:
		return 0
	}
}

func matchedToolSearchFields(contributions map[toolSearchField]float64) []string {
	type contribution struct {
		field toolSearchField
		score float64
	}
	values := make([]contribution, 0, len(contributions))
	for field, score := range contributions {
		values = append(values, contribution{field: field, score: score})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].score != values[j].score {
			return values[i].score > values[j].score
		}
		return values[i].field < values[j].field
	})
	fields := make([]string, 0, len(values))
	for _, value := range values {
		fields = append(fields, string(value.field))
	}
	return fields
}

func toolReference(tool ToolSpec, score float64, matchedFields, sources []string) ToolReference {
	title := truncateToolSearchRunes(tool.Title, maxToolSearchSummaryRunes)
	agentSummary := truncateToolSearchRunes(tool.Selection.AgentSummary, maxToolSearchSummaryRunes)
	truncatedFields := make([]string, 0, 2)
	if title != tool.Title {
		truncatedFields = append(truncatedFields, "title")
	}
	if agentSummary != tool.Selection.AgentSummary {
		truncatedFields = append(truncatedFields, "agent_summary")
	}
	return ToolReference{
		CanonicalPath:   tool.Identity.CanonicalPath,
		PrimaryCLIPath:  tool.Identity.PrimaryCLIPath,
		ProductID:       tool.Identity.ProductID,
		Title:           title,
		AgentSummary:    agentSummary,
		Effect:          tool.Safety.Effect,
		Risk:            tool.Safety.Risk,
		Confirmation:    tool.Safety.Confirmation,
		Idempotency:     tool.Safety.Idempotency,
		MatchedFields:   append([]string(nil), matchedFields...),
		RankSources:     append([]string(nil), sources...),
		TruncatedFields: truncatedFields,
		RequiresInspect: true,
		score:           score,
	}
}

func cloneToolReferences(values []ToolReference) []ToolReference {
	out := make([]ToolReference, len(values))
	for index, value := range values {
		out[index] = value
		out[index].MatchedFields = append([]string(nil), value.MatchedFields...)
		out[index].RankSources = append([]string(nil), value.RankSources...)
		out[index].TruncatedFields = append([]string(nil), value.TruncatedFields...)
	}
	return out
}

func (e *ToolSearchEngine) catalogVersion() CatalogVersionRef {
	return CatalogVersionRef{
		SourceHash:  e.config.CatalogSourceHash,
		SurfaceHash: e.config.CatalogSurfaceHash,
	}
}

func finalizeToolSearchResponse(response ToolSearchResponse) (ToolSearchResponse, error) {
	response.Abstained = response.Abstained || len(response.Candidates) == 0
	for {
		payload, err := json.Marshal(response)
		if err != nil {
			return ToolSearchResponse{}, fmt.Errorf("encode tool search response: %w", err)
		}
		// schema search emits this compact JSON with exactly one trailing newline.
		if len(payload)+1 <= maxToolSearchResponseBytes {
			return response, nil
		}
		if len(response.Candidates) == 0 {
			return ToolSearchResponse{}, fmt.Errorf("tool search response exceeds %d bytes without candidates", maxToolSearchResponseBytes)
		}
		response.Candidates = response.Candidates[:len(response.Candidates)-1]
		response.Truncated = true
		response.Abstained = len(response.Candidates) == 0
		setToolReferenceRanks(response.Candidates)
	}
}

func truncateToolSearchRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func toolSearchEligible(tool ToolSpec, allowedProducts, allowedEffects, excluded map[string]bool) bool {
	return toolSearchIneligibleReason(tool, allowedProducts, allowedEffects, excluded) == ""
}

func toolSearchIneligibleReason(tool ToolSpec, allowedProducts, allowedEffects, excluded map[string]bool) string {
	if excluded[tool.Identity.CanonicalPath] {
		return "excluded"
	}
	if len(allowedProducts) > 0 && !allowedProducts[tool.Identity.ProductID] {
		return "product_mismatch"
	}
	if len(allowedEffects) > 0 && !allowedEffects[tool.Safety.Effect] {
		return "effect_mismatch"
	}
	if !tool.Interface.AgentExecutable() {
		return "unavailable"
	}
	return ""
}

func setToolReferenceRanks(values []ToolReference) {
	for index := range values {
		values[index].Rank = index + 1
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = true
		}
	}
	return set
}

func tokenizeToolSearchText(text string) []string {
	normalized := norm.NFKC.String(text)
	tokens := make([]string, 0)
	for _, identifier := range toolSearchIdentifierPattern.FindAllString(normalized, -1) {
		tokens = append(tokens, strings.ToLower(identifier))
	}
	normalized = splitToolSearchCamelCase(normalized)
	var ascii []rune
	var chinese []rune
	flushASCII := func() {
		if len(ascii) > 0 {
			tokens = append(tokens, strings.ToLower(string(ascii)))
			ascii = ascii[:0]
		}
	}
	flushChinese := func() {
		if len(chinese) == 1 {
			tokens = append(tokens, string(chinese))
		} else {
			for index := 0; index+1 < len(chinese); index++ {
				tokens = append(tokens, string(chinese[index:index+2]))
			}
		}
		chinese = chinese[:0]
	}
	for _, value := range normalized {
		switch {
		case isToolSearchCJK(value):
			flushASCII()
			chinese = append(chinese, value)
		case unicode.IsLetter(value) || unicode.IsDigit(value):
			flushChinese()
			ascii = append(ascii, unicode.ToLower(value))
		default:
			flushASCII()
			flushChinese()
		}
	}
	flushASCII()
	flushChinese()
	return tokens
}

func splitToolSearchCamelCase(value string) string {
	runes := []rune(value)
	var builder strings.Builder
	for index, current := range runes {
		if index > 0 && (unicode.IsLower(runes[index-1]) || unicode.IsDigit(runes[index-1])) && unicode.IsUpper(current) {
			builder.WriteRune(' ')
		}
		builder.WriteRune(current)
	}
	return builder.String()
}

func isToolSearchCJK(value rune) bool {
	return value >= '\u3400' && value <= '\u9fff'
}
