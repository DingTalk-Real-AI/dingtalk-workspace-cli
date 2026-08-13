// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

type toolSearchProviderStub struct {
	ranking       []string
	err           error
	request       ToolSearchCandidateRequest
	catalog       CatalogVersionRef
	customCatalog bool
	provider      string
	version       string
}

func (s *toolSearchProviderStub) Retrieve(_ context.Context, request ToolSearchCandidateRequest) (ExternalCandidateRanking, error) {
	s.request = request
	catalog := request.Catalog
	if s.customCatalog {
		catalog = s.catalog
	}
	provider := s.provider
	if provider == "" {
		provider = "test-provider"
	}
	version := s.version
	if version == "" {
		version = "v1"
	}
	return ExternalCandidateRanking{
		Catalog:          catalog,
		Provider:         provider,
		ProviderVersion:  version,
		CanonicalRanking: append([]string(nil), s.ranking...),
	}, s.err
}

type toolSearchProviderFunc func(context.Context, ToolSearchCandidateRequest) (ExternalCandidateRanking, error)

func (f toolSearchProviderFunc) Retrieve(ctx context.Context, request ToolSearchCandidateRequest) (ExternalCandidateRanking, error) {
	return f(ctx, request)
}

func TestToolSearchExactGuardReturnsInspectReference(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "chat send"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if response.Strategy != "exact_guard" || len(response.Candidates) != 1 {
		t.Fatalf("response = %#v, want one exact candidate", response)
	}
	reference := response.Candidates[0]
	if reference.CanonicalPath != "chat.send" || reference.Rank != 1 || !reference.RequiresInspect {
		t.Fatalf("reference = %#v", reference)
	}
	if response.Catalog != (CatalogVersionRef{SourceHash: "source-test", SurfaceHash: "surface-test"}) {
		t.Fatalf("Catalog = %#v", response.Catalog)
	}
	if response.Catalog != (CatalogVersionRef{SourceHash: "source-test", SurfaceHash: "surface-test"}) {
		t.Fatalf("Catalog = %#v", response.Catalog)
	}
}

func TestToolSearchExactGuardNormalizesNFKCAndAliases(t *testing.T) {
	base := newToolSearchTestEngine(t, nil)
	registry := base.index.Registry()
	for productIndex := range registry.Products {
		for toolIndex := range registry.Products[productIndex].Tools {
			tool := &registry.Products[productIndex].Tools[toolIndex]
			if tool.Identity.CanonicalPath == "chat.send" {
				tool.Identity.Aliases = []string{"chat.deliver"}
			}
		}
	}
	config := DefaultToolSearchConfig()
	config.CatalogSourceHash = "source-test"
	config.CatalogSurfaceHash = "surface-test"
	engine, err := NewToolSearchEngine(registry, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"chat.deliver", "ｃｈａｔ．ｓｅｎｄ"} {
		response, searchErr := engine.Search(context.Background(), ToolSearchRequest{Query: query})
		if searchErr != nil {
			t.Fatalf("Search(%q): %v", query, searchErr)
		}
		if response.Strategy != "exact_guard" || len(response.Candidates) != 1 || response.Candidates[0].CanonicalPath != "chat.send" {
			t.Fatalf("Search(%q) = %#v", query, response)
		}
	}
}

func TestToolSearchExactGuardCoversIndexedIdentitySet(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	paths := engine.index.CanonicalPaths()
	if len(paths) == 0 {
		t.Fatal("Tool Search identity set is empty")
	}
	for _, canonical := range paths {
		response, searchErr := engine.Search(context.Background(), ToolSearchRequest{Query: canonical})
		if searchErr != nil {
			t.Fatalf("Search(%q) error = %v", canonical, searchErr)
		}
		if canonical == "drive.download" {
			if response.Strategy != "exact_filtered" || response.ExactFiltered == nil || response.ExactFiltered.CanonicalPath != canonical {
				t.Fatalf("Search(%q) = %#v", canonical, response)
			}
			continue
		}
		if response.Strategy != "exact_guard" || len(response.Candidates) != 1 || response.Candidates[0].CanonicalPath != canonical {
			t.Fatalf("Search(%q) = %#v", canonical, response)
		}
	}
}

func TestToolSearchRanksChineseAndAppliesHardFilters(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	response, err := engine.Search(context.Background(), ToolSearchRequest{
		Query:      "给群里发送消息",
		ProductIDs: []string{"chat"},
		Effects:    []string{"write"},
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Candidates) == 0 || response.Candidates[0].CanonicalPath != "chat.send" {
		t.Fatalf("candidates = %#v, want chat.send first", response.Candidates)
	}
	for _, candidate := range response.Candidates {
		if candidate.ProductID != "chat" || candidate.Effect != "write" {
			t.Fatalf("hard filter leaked candidate %#v", candidate)
		}
	}

	excluded, err := engine.Search(context.Background(), ToolSearchRequest{
		Query:                 "给群里发送消息",
		ExcludeCanonicalPaths: []string{"chat.send"},
	})
	if err != nil {
		t.Fatalf("Search(excluded) error = %v", err)
	}
	for _, candidate := range excluded.Candidates {
		if candidate.CanonicalPath == "chat.send" {
			t.Fatal("excluded candidate was returned")
		}
	}
}

func TestToolSearchExactFilteredDoesNotFallThroughToSibling(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	tests := []struct {
		name    string
		request ToolSearchRequest
		reason  string
	}{
		{name: "excluded", request: ToolSearchRequest{Query: "chat send", ExcludeCanonicalPaths: []string{"chat.send"}}, reason: "excluded"},
		{name: "product", request: ToolSearchRequest{Query: "chat send", ProductIDs: []string{"doc"}}, reason: "product_mismatch"},
		{name: "effect", request: ToolSearchRequest{Query: "chat send", Effects: []string{"read"}}, reason: "effect_mismatch"},
		{name: "unavailable", request: ToolSearchRequest{Query: "drive download"}, reason: "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := engine.Search(context.Background(), test.request)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if response.Strategy != "exact_filtered" || !response.Abstained || len(response.Candidates) != 0 {
				t.Fatalf("response = %#v", response)
			}
			if response.ExactFiltered == nil || response.ExactFiltered.CanonicalPath == "" || response.ExactFiltered.Reason != test.reason {
				t.Fatalf("exact filtered = %#v, want reason %q", response.ExactFiltered, test.reason)
			}
		})
	}
}

func TestToolSearchIndexesParameterDescriptionsAndExcludesUnavailableTools(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	parameterResult, err := engine.Search(context.Background(), ToolSearchRequest{Query: "本地媒体文件路径"})
	if err != nil {
		t.Fatalf("Search(parameter description) error = %v", err)
	}
	if len(parameterResult.Candidates) == 0 || parameterResult.Candidates[0].CanonicalPath != "chat.send" {
		t.Fatalf("parameter candidates = %#v", parameterResult.Candidates)
	}
	if !stringSliceContains(parameterResult.Candidates[0].MatchedFields, "parameters") {
		t.Fatalf("matched fields = %#v", parameterResult.Candidates[0].MatchedFields)
	}

	unavailable, err := engine.Search(context.Background(), ToolSearchRequest{Query: "下载钉盘文件"})
	if err != nil {
		t.Fatalf("Search(unavailable identity) error = %v", err)
	}
	for _, candidate := range unavailable.Candidates {
		if candidate.CanonicalPath == "drive.download" {
			t.Fatalf("unavailable tool leaked: %#v", candidate)
		}
	}
}

func TestToolSearchProviderCanRecoverSparseMissAndRRF(t *testing.T) {
	provider := &toolSearchProviderStub{ranking: []string{"doc.read", "chat.send"}}
	engine := newToolSearchTestEngine(t, provider)
	response, err := engine.Search(context.Background(), ToolSearchRequest{
		Query:          "群聊动作",
		Limit:          3,
		CandidateLimit: 3,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if response.Strategy != ToolSearchLexicalBM25Action+"_provider_rrf" {
		t.Fatalf("strategy = %q", response.Strategy)
	}
	if provider.request.Query != "群聊动作" || provider.request.CandidateLimit != 3 {
		t.Fatalf("provider received %#v", provider.request)
	}
	if provider.request.Catalog != (CatalogVersionRef{SourceHash: "source-test", SurfaceHash: "surface-test"}) {
		t.Fatalf("provider Catalog = %#v", provider.request.Catalog)
	}
	foundProviderOnly := false
	for _, candidate := range response.Candidates {
		if candidate.CanonicalPath == "doc.read" {
			foundProviderOnly = true
			if candidate.sparseScore != 0 || !stringSliceContains(candidate.RankSources, "provider") {
				t.Fatalf("provider-only candidate = %#v", candidate)
			}
		}
	}
	if !foundProviderOnly {
		t.Fatalf("provider-only candidate absent: %#v", response.Candidates)
	}
}

func TestToolSearchProviderFailureAndInvalidOutputFallBack(t *testing.T) {
	request := ToolSearchRequest{Query: "群聊消息"}
	local, err := newToolSearchTestEngine(t, nil).Search(context.Background(), request)
	if err != nil {
		t.Fatalf("local Search() error = %v", err)
	}
	for _, test := range []struct {
		name     string
		provider ToolSearchCandidateProvider
	}{
		{name: "error", provider: &toolSearchProviderStub{err: errors.New("offline")}},
		{name: "unknown", provider: &toolSearchProviderStub{ranking: []string{"unknown.tool"}}},
		{name: "duplicate", provider: &toolSearchProviderStub{ranking: []string{"chat.send", "chat.send"}}},
		{name: "stale", provider: &toolSearchProviderStub{ranking: []string{"chat.send"}, customCatalog: true, catalog: CatalogVersionRef{SourceHash: "old", SurfaceHash: "old"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := newToolSearchTestEngine(t, test.provider)
			response, err := engine.Search(context.Background(), request)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if response.Strategy != ToolSearchLexicalBM25Action+"_fallback" || !response.Degraded || len(response.WarningCodes) != 1 {
				t.Fatalf("response = %#v, want deterministic fallback", response)
			}
			encoded, marshalErr := json.Marshal(response)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(string(encoded), "offline") || strings.Contains(string(encoded), "old") {
				t.Fatalf("response leaked provider detail: %s", encoded)
			}
			if len(response.Candidates) == 0 || response.Candidates[0].CanonicalPath != "chat.send" {
				t.Fatalf("fallback candidates = %#v", response.Candidates)
			}
			if !reflect.DeepEqual(response.Candidates, local.Candidates) {
				t.Fatalf("fallback candidates differ from local-only\nfall: %#v\nlocal:%#v", response.Candidates, local.Candidates)
			}
		})
	}
}

func TestToolSearchProviderTimeoutReturnsLocalRanking(t *testing.T) {
	provider := toolSearchProviderFunc(func(ctx context.Context, _ ToolSearchCandidateRequest) (ExternalCandidateRanking, error) {
		<-ctx.Done()
		return ExternalCandidateRanking{}, ctx.Err()
	})
	engine := newToolSearchTestEngine(t, provider)
	engine.config.ProviderTimeout = time.Millisecond
	response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "群聊消息"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !response.Degraded || response.DegradedReasonCode != toolSearchWarningProviderTimeout {
		t.Fatalf("response = %#v", response)
	}
}

func TestToolSearchProviderPanicReturnsLocalRanking(t *testing.T) {
	provider := toolSearchProviderFunc(func(context.Context, ToolSearchCandidateRequest) (ExternalCandidateRanking, error) {
		panic("provider secret")
	})
	engine := newToolSearchTestEngine(t, provider)
	response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "群聊消息"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !response.Degraded || response.DegradedReasonCode != toolSearchWarningProviderInternal {
		t.Fatalf("response = %#v", response)
	}
}

func TestToolSearchProviderLateSuccessIsNotAccepted(t *testing.T) {
	provider := toolSearchProviderFunc(func(ctx context.Context, request ToolSearchCandidateRequest) (ExternalCandidateRanking, error) {
		<-ctx.Done()
		return ExternalCandidateRanking{
			Catalog: request.Catalog, Provider: "late", ProviderVersion: "v1",
			CanonicalRanking: []string{"doc.read"},
		}, nil
	})
	engine := newToolSearchTestEngine(t, provider)
	engine.config.ProviderTimeout = time.Millisecond
	response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "群聊消息"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !response.Degraded || response.DegradedReasonCode != toolSearchWarningProviderTimeout || response.Strategy == ToolSearchLexicalBM25Action+"_provider_rrf" {
		t.Fatalf("response = %#v", response)
	}
}

func TestToolSearchProviderIgnoringContextIsBulkheaded(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var starts atomic.Int32
	provider := toolSearchProviderFunc(func(context.Context, ToolSearchCandidateRequest) (ExternalCandidateRanking, error) {
		starts.Add(1)
		<-release
		return ExternalCandidateRanking{}, errors.New("released")
	})
	engine := newToolSearchTestEngine(t, provider)
	engine.config.ProviderTimeout = time.Millisecond
	for run := 0; run < 2; run++ {
		response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "群聊消息"})
		if err != nil {
			t.Fatalf("Search(%d) error = %v", run, err)
		}
		if !response.Degraded || response.DegradedReasonCode != toolSearchWarningProviderTimeout {
			t.Fatalf("Search(%d) response = %#v", run, response)
		}
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("provider starts = %d, want one bounded stuck call", got)
	}
}

func TestToolSearchSubqueriesRoundRobinAndAcceptsDeepCandidateLimit(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	response, err := engine.SearchSubqueries(
		context.Background(),
		[]string{"群聊发送消息", "读取在线文档"},
		ToolSearchRequest{Limit: 2, CandidateLimit: 100},
	)
	if err != nil {
		t.Fatalf("SearchSubqueries() error = %v", err)
	}
	if response.Strategy != "decomposed_round_robin" || len(response.Candidates) != 2 {
		t.Fatalf("response = %#v", response)
	}
	if response.Candidates[0].CanonicalPath != "chat.send" || response.Candidates[1].CanonicalPath != "doc.read" {
		t.Fatalf("round-robin candidates = %#v", response.Candidates)
	}
	if response.Candidates[0].Rank != 1 || response.Candidates[1].Rank != 2 {
		t.Fatalf("ranks = %#v", response.Candidates)
	}
	if stringSliceContains(response.WarningCodes, toolSearchWarningResponseBudgetExceeded) || response.Truncated {
		t.Fatalf("internal subquery budget leaked into final response: %#v", response)
	}
}

func TestToolSearchActionRerankDoesNotChangeNoSignalOrdering(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	raw := newToolSearchBM25Retriever(engine.documents, engine.config.FieldWeights)
	action := newToolSearchActionRetriever(raw, engine.documents)
	request := ToolSearchLexicalRequest{
		Query:                  "媒体路径",
		CandidateLimit:         4,
		EligibleCanonicalPaths: engine.index.CanonicalPaths(),
	}
	rawHits, err := raw.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	actionHits, err := action.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rawHits, actionHits) {
		t.Fatalf("no-signal ordering changed\nraw: %#v\naction: %#v", rawHits, actionHits)
	}
}

func TestToolSearchActionRerankDefersUnclassifiedTechnicalASCII(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	raw := newToolSearchBM25Retriever(engine.documents, engine.config.FieldWeights)
	action := newToolSearchActionRetriever(raw, engine.documents)
	request := ToolSearchLexicalRequest{
		Query:                  "读取 openConversationId 对应的群消息",
		CandidateLimit:         4,
		EligibleCanonicalPaths: engine.index.CanonicalPaths(),
	}
	rawHits, err := raw.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	actionHits, err := action.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rawHits, actionHits) {
		t.Fatalf("technical ASCII ordering changed\nraw: %#v\naction: %#v", rawHits, actionHits)
	}
	if toolSearchHasUnclassifiedASCII("查询任务 ID") {
		t.Fatal("generic ID unexpectedly disabled structured reranking")
	}
}

func TestToolSearchRejectsInvalidRequestsAndCanceledContext(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	for _, request := range []ToolSearchRequest{
		{},
		{Query: "query", Limit: maxToolSearchLimit + 1},
		{Query: "query", CandidateLimit: maxToolSearchCandidateLimit + 1},
		{Query: strings.Repeat("查", maxToolSearchQueryRunes+1)},
	} {
		if _, err := engine.Search(context.Background(), request); err == nil {
			t.Fatalf("Search(%#v) succeeded", request)
		} else {
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
				t.Fatalf("Search(%#v) error = %#v, want typed validation", request, err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Search(ctx, ToolSearchRequest{Query: "query"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
	tooManySubqueries := make([]string, maxToolSearchSubqueries+1)
	for index := range tooManySubqueries {
		tooManySubqueries[index] = fmt.Sprintf("query-%d", index)
	}
	if _, err := engine.SearchSubqueries(context.Background(), tooManySubqueries, ToolSearchRequest{Limit: 5}); err == nil {
		t.Fatal("SearchSubqueries() accepted too many subqueries")
	}
	oversizedSubqueries := []string{
		strings.Repeat("甲", maxToolSearchQueryRunes-1) + "一",
		strings.Repeat("乙", maxToolSearchQueryRunes-1) + "二",
		strings.Repeat("丙", maxToolSearchQueryRunes-1) + "三",
	}
	if _, err := engine.SearchSubqueries(context.Background(), oversizedSubqueries, ToolSearchRequest{Limit: 5}); err == nil {
		t.Fatal("SearchSubqueries() accepted an aggregate query echo larger than the response budget")
	}
}

func TestToolSearchSubqueriesFailClosedOnExactFilteredAction(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	response, err := engine.SearchSubqueries(
		context.Background(),
		[]string{"chat.send", "查询群消息已读状态"},
		ToolSearchRequest{Limit: 5, ExcludeCanonicalPaths: []string{"chat.send"}},
	)
	if err != nil {
		t.Fatalf("SearchSubqueries() error = %v", err)
	}
	if response.Strategy != "decomposed_exact_filtered" || !response.Abstained || len(response.Candidates) != 0 {
		t.Fatalf("response = %#v, want fail-closed exact_filtered", response)
	}
	if response.ExactFiltered == nil || response.ExactFiltered.CanonicalPath != "chat.send" {
		t.Fatalf("exact_filtered = %#v", response.ExactFiltered)
	}
}

func TestDefaultToolSearchConfigExcludesUseWhen(t *testing.T) {
	config := DefaultToolSearchConfig()
	if config.IncludeUseWhen {
		t.Fatal("DefaultToolSearchConfig() enables answer-bearing use_when projection")
	}
	tool := toolSearchTestTool("chat", "send", "chat send", "发送消息", "write")
	fields := toolSearchDocumentFields(tool, config.IncludeUseWhen)
	if fields[toolSearchUseWhen] != "" {
		t.Fatalf("default use_when field = %q", fields[toolSearchUseWhen])
	}
}

func TestToolSearchLexicalRetrieverCanSelectGoTFIDF(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	engine.lexical = newToolSearchTFIDFRetriever(engine.documents, engine.config.FieldWeights)
	response, err := engine.Search(context.Background(), ToolSearchRequest{
		Query:      "给群里发送消息",
		ProductIDs: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("TF-IDF Search() error = %v", err)
	}
	if response.Strategy != ToolSearchLexicalTFIDF || len(response.Candidates) == 0 || response.Candidates[0].CanonicalPath != "chat.send" {
		t.Fatalf("TF-IDF response = %#v", response)
	}
	if _, err := newToolSearchLexicalRetriever(engine.documents, ToolSearchConfig{LexicalAlgorithm: "unknown"}); err == nil {
		t.Fatal("unknown lexical algorithm was accepted")
	}
}

func TestToolSearchLexicalRetrieverScoresOnlyEligibleDomain(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	hits, err := engine.lexical.Retrieve(context.Background(), ToolSearchLexicalRequest{
		Query:                  "发送群聊消息",
		CandidateLimit:         5,
		EligibleCanonicalPaths: []string{"doc.read"},
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	for _, hit := range hits {
		if hit.CanonicalPath != "doc.read" {
			t.Fatalf("retriever escaped eligible domain: %#v", hits)
		}
	}
}

func TestToolSearchResponseBudgetDropsWholeReferences(t *testing.T) {
	tools := make([]ToolSpec, 0, 30)
	for index := 0; index < 30; index++ {
		tool := toolSearchTestTool("budget", fmt.Sprintf("tool_%02d", index), fmt.Sprintf("budget tool-%02d", index), "预算检索 "+strings.Repeat("长", 500), "read")
		tools = append(tools, tool)
	}
	registry := SchemaRegistry{
		Kind:     "schema",
		Level:    "catalog",
		Products: []ProductSpec{{ID: "budget", Tools: tools}},
	}
	config := DefaultToolSearchConfig()
	config.CatalogSourceHash = "source-test"
	config.CatalogSurfaceHash = "surface-test"
	engine, err := NewToolSearchEngine(registry, config, nil)
	if err != nil {
		t.Fatalf("NewToolSearchEngine() error = %v", err)
	}
	response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "预算检索", Limit: 20, CandidateLimit: 30})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload)+1 > maxToolSearchResponseBytes {
		t.Fatalf("wire response bytes = %d, max = %d", len(payload)+1, maxToolSearchResponseBytes)
	}
	if !response.Truncated || !stringSliceContains(response.WarningCodes, toolSearchWarningResponseBudgetExceeded) {
		t.Fatalf("response = %#v, want budget truncation", response)
	}
	for _, candidate := range response.Candidates {
		if len([]rune(candidate.AgentSummary)) > maxToolSearchSummaryRunes || len([]rune(candidate.Title)) > maxToolSearchSummaryRunes {
			t.Fatalf("candidate text was not bounded: %#v", candidate)
		}
		if !reflect.DeepEqual(candidate.TruncatedFields, []string{"title", "agent_summary"}) {
			t.Fatalf("candidate truncation markers = %#v", candidate.TruncatedFields)
		}
	}
}

func TestToolSearchResponseBudgetIncludesEncoderNewline(t *testing.T) {
	response := ToolSearchResponse{
		Version: "tool-search.v1", Catalog: CatalogVersionRef{SourceHash: "s", SurfaceHash: "f"},
		Strategy: "test", Candidates: []ToolReference{}, Abstained: true,
	}
	empty, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	response.Query = strings.Repeat("q", maxToolSearchResponseBytes-1-len(empty))
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload)+1 != maxToolSearchResponseBytes {
		t.Fatalf("constructed boundary = %d, want %d", len(payload)+1, maxToolSearchResponseBytes)
	}
	if _, err := finalizeToolSearchResponse(response); err != nil {
		t.Fatalf("exact wire boundary rejected: %v", err)
	}
	response.Query += "q"
	if _, err := finalizeToolSearchResponse(response); err == nil {
		t.Fatal("response exceeding budget by encoder newline was accepted")
	}
}

func TestToolSearchTokenizesIdentifiersCamelCaseAndChineseBigrams(t *testing.T) {
	tokens := tokenizeToolSearchText("chat.send_personal_message 给群聊发送FilePath")
	for _, want := range []string{"chat.send_personal_message", "群聊", "file", "path"} {
		if !stringSliceContains(tokens, want) {
			t.Fatalf("tokens %q missing %q", tokens, want)
		}
	}
}

func TestToolSearchIsDeterministicAcrossProcesses(t *testing.T) {
	const helperEnv = "DWS_TOOL_SEARCH_DETERMINISM_CHILD"
	if os.Getenv(helperEnv) == "1" {
		engine := newToolSearchTestEngine(t, nil)
		provider := &toolSearchProviderStub{ranking: []string{"chat.read_status"}}
		providerEngine := newToolSearchTestEngine(t, provider)
		tfidfEngine := newToolSearchTestEngine(t, nil)
		tfidfEngine.lexical = newToolSearchTFIDFRetriever(tfidfEngine.documents, tfidfEngine.config.FieldWeights)
		tests := []struct {
			name    string
			engine  *ToolSearchEngine
			request ToolSearchRequest
		}{
			{name: "pure_chinese", engine: engine, request: ToolSearchRequest{Query: "给群里发文件并确认消息已读", Limit: 20}},
			{name: "mixed_identifiers", engine: engine, request: ToolSearchRequest{Query: "给群里发文件并确认消息已读 baseId openConversationId status upload", Limit: 20}},
			{name: "exact", engine: engine, request: ToolSearchRequest{Query: "chat.read_status", Limit: 20}},
			{name: "provider_fusion", engine: providerEngine, request: ToolSearchRequest{Query: "无词面补召回", Limit: 20}},
			{name: "tfidf", engine: tfidfEngine, request: ToolSearchRequest{Query: "给群里发文件并确认消息已读", Limit: 20}},
		}
		outputs := make([]struct {
			Name     string             `json:"name"`
			Response ToolSearchResponse `json:"response"`
		}, 0, len(tests))
		for _, test := range tests {
			response, searchErr := test.engine.Search(context.Background(), test.request)
			if searchErr != nil {
				fmt.Fprintln(os.Stderr, searchErr)
				os.Exit(2)
			}
			outputs = append(outputs, struct {
				Name     string             `json:"name"`
				Response ToolSearchResponse `json:"response"`
			}{Name: test.name, Response: response})
		}
		payload, err := json.Marshal(outputs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		_, _ = os.Stdout.Write(payload)
		os.Exit(0)
	}

	var golden []byte
	for run := 0; run < 3; run++ {
		command := exec.Command(os.Args[0], "-test.run=^TestToolSearchIsDeterministicAcrossProcesses$")
		command.Env = append(os.Environ(), helperEnv+"=1")
		output, err := command.Output()
		if err != nil {
			t.Fatalf("determinism child %d failed: %v", run, err)
		}
		if run == 0 {
			golden = output
			var outputs []struct {
				Name     string             `json:"name"`
				Response ToolSearchResponse `json:"response"`
			}
			if err := json.Unmarshal(output, &outputs); err != nil {
				t.Fatalf("decode determinism golden: %v", err)
			}
			if len(outputs) != 5 || outputs[2].Response.Strategy != "exact_guard" || outputs[3].Response.Strategy != ToolSearchLexicalBM25Action+"_provider_rrf" || outputs[4].Response.Strategy != ToolSearchLexicalTFIDF {
				t.Fatalf("golden scenarios = %#v", outputs)
			}
			continue
		}
		if string(output) != string(golden) {
			t.Fatalf("determinism child %d output differs\nfirst: %s\nthis:  %s", run, golden, output)
		}
	}
}

func BenchmarkToolSearchChineseQuery(b *testing.B) {
	engine := newToolSearchTestEngine(b, nil)
	request := ToolSearchRequest{Query: "给群里发文件并确认消息已读", Limit: 5}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, searchErr := engine.Search(context.Background(), request); searchErr != nil {
			b.Fatalf("Search() error = %v", searchErr)
		}
	}
}

func BenchmarkToolSearchBuild(b *testing.B) {
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_ = newToolSearchTestEngine(b, nil)
	}
}

type toolSearchTestingT interface {
	Helper()
	Fatalf(string, ...any)
}

func newToolSearchTestEngine(t toolSearchTestingT, provider ToolSearchCandidateProvider) *ToolSearchEngine {
	t.Helper()
	tools := []ToolSpec{
		toolSearchTestTool("chat", "send", "chat send", "发送群聊消息", "write"),
		toolSearchTestTool("chat", "read_status", "chat read-status", "查询群消息已读状态", "read"),
		toolSearchTestTool("doc", "read", "doc read", "读取在线文档正文", "read"),
		toolSearchTestTool("drive", "download", "drive download", "下载钉盘文件", "read"),
	}
	tools[0].Parameters = []ParameterSpec{{
		Name:                 "file-path",
		Property:             "filePath",
		Description:          "本地媒体文件路径",
		InterfaceDescription: "Path of the local media file",
	}}
	tools[3].Interface.Availability = contract.InterfaceUnavailable
	tools[3].Interface.Reason = "disabled in this test Catalog"
	registry := SchemaRegistry{
		Kind:  "schema",
		Level: "catalog",
		Products: []ProductSpec{
			{ID: "chat", Tools: tools[:2]},
			{ID: "doc", Tools: tools[2:3]},
			{ID: "drive", Tools: tools[3:]},
		},
	}
	config := DefaultToolSearchConfig()
	config.CatalogSourceHash = "source-test"
	config.CatalogSurfaceHash = "surface-test"
	engine, err := NewToolSearchEngine(registry, config, provider)
	if err != nil {
		t.Fatalf("NewToolSearchEngine() error = %v", err)
	}
	return engine
}

func toolSearchTestTool(product, name, cliPath, summary, effect string) ToolSpec {
	return ToolSpec{
		Identity: contract.ToolIdentitySpec{
			ProductID:      product,
			Name:           name,
			CLIName:        strings.ReplaceAll(name, "_", "-"),
			CanonicalPath:  product + "." + name,
			Path:           product + "." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
		Title:       summary,
		Description: summary,
		Safety:      contract.SafetySpec{Effect: effect, Idempotency: "unknown"},
		Interface: contract.InterfaceSpec{
			Mode:         contract.InterfaceModeLocal,
			Availability: contract.InterfaceAvailable,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: summary,
			UseWhen:      []string{summary},
			AvoidWhen:    []string{"不是" + summary + "时"},
		},
	}
}
