// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestToolSearchExactGuardReturnsInspectReference(t *testing.T) {
	engine := newToolSearchTestEngine(t)
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
	base := newToolSearchTestEngine(t)
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
	engine, err := NewToolSearchEngine(registry, config)
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
	engine := newToolSearchTestEngine(t)
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
	engine := newToolSearchTestEngine(t)
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
	engine := newToolSearchTestEngine(t)
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
	engine := newToolSearchTestEngine(t)
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

func TestToolSearchSubqueriesRoundRobinAndAcceptsDeepCandidateLimit(t *testing.T) {
	engine := newToolSearchTestEngine(t)
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
	if response.Truncated {
		t.Fatalf("internal subquery budget leaked into final response: %#v", response)
	}
}

func TestToolSearchActionRerankDoesNotChangeNoSignalOrdering(t *testing.T) {
	engine := newToolSearchTestEngine(t)
	raw := newToolSearchBM25Retriever(engine.documents, engine.config.FieldWeights, engine.config.BM25K1, engine.config.BM25B, false)
	action := newToolSearchActionRetriever(raw, engine.documents, false)
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
	engine := newToolSearchTestEngine(t)
	raw := newToolSearchBM25Retriever(engine.documents, engine.config.FieldWeights, engine.config.BM25K1, engine.config.BM25B, false)
	action := newToolSearchActionRetriever(raw, engine.documents, false)
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
	engine := newToolSearchTestEngine(t)
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
	engine := newToolSearchTestEngine(t)
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
	// The refusal must keep the same candidates wire shape as every other
	// strategy: a nil slice would encode as null instead of [].
	if response.Candidates == nil {
		t.Fatal("decomposed_exact_filtered returned nil candidates; want an empty slice")
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(payload), `"candidates":[]`) {
		t.Fatalf("response JSON omits an empty candidates array: %s", payload)
	}
}

// TestDeliveryToolSearchEngineMemoInvalidatedOnCatalogReset pins the memo to
// one Catalog generation. The engine caches the Catalog source/surface hashes
// it indexed, so a Catalog delivery reset (RegisterSchemaSourceRoot and the
// ForTest resetters) must force a rebuild instead of serving stale hashes.
func TestDeliveryToolSearchEngineMemoInvalidatedOnCatalogReset(t *testing.T) {
	t.Cleanup(func() {
		resetDeliveryToolSearchEngineStateForTest()
		RestorePackageCLISchemaDeliveryForTest()
	})
	resetDeliveryToolSearchEngineStateForTest()
	first, err := NewDeliveryToolSearchEngine()
	if err != nil {
		t.Fatalf("NewDeliveryToolSearchEngine() error = %v", err)
	}
	second, err := NewDeliveryToolSearchEngine()
	if err != nil {
		t.Fatalf("NewDeliveryToolSearchEngine() second call error = %v", err)
	}
	if first != second {
		t.Fatal("memoized delivery engine rebuilt without a Catalog reset")
	}
	loaded := deliverySchemaCatalog()
	if got := first.catalogVersion(); got.SourceHash != loaded.Snapshot.SourceHash || got.SurfaceHash != loaded.Snapshot.SurfaceHash {
		t.Fatalf("engine catalog version = %#v, want the delivered Catalog hashes", got)
	}

	resetDeliverySchemaCatalogStateForTest()
	if deliveryToolSearchEngine != nil {
		t.Fatal("Catalog delivery reset left the memoized engine installed")
	}
	third, err := NewDeliveryToolSearchEngine()
	if err != nil {
		t.Fatalf("NewDeliveryToolSearchEngine() after reset error = %v", err)
	}
	if third == first {
		t.Fatal("engine was reused across a Catalog delivery reset")
	}
}

func TestDefaultToolSearchConfigExcludesUseWhen(t *testing.T) {
	config := DefaultToolSearchConfig()
	if config.LexicalAlgorithm != ToolSearchLexicalBM25 {
		t.Fatalf("default lexical algorithm = %q, want independently conservative control %q", config.LexicalAlgorithm, ToolSearchLexicalBM25)
	}
	if config.IncludeUseWhen {
		t.Fatal("DefaultToolSearchConfig() enables answer-bearing use_when projection")
	}
	tool := toolSearchTestTool("chat", "send", "chat send", "发送消息", "write")
	fields := toolSearchDocumentFields(tool, config.IncludeUseWhen)
	if fields[toolSearchUseWhen] != "" {
		t.Fatalf("default use_when field = %q", fields[toolSearchUseWhen])
	}
}

func TestToolSearchExplainAttachesBreakdownOnlyWhenEnabled(t *testing.T) {
	query := ToolSearchRequest{Query: "给群里发送消息"}
	plain := newToolSearchTestEngine(t)
	plainResponse, err := plain.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(plainResponse.Candidates) == 0 || plainResponse.Candidates[0].CanonicalPath != "chat.send" {
		t.Fatalf("plain candidates = %#v", plainResponse.Candidates)
	}
	plainPayload, err := json.Marshal(plainResponse)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plainPayload), "score_breakdown") {
		t.Fatalf("explain leaked into default response: %s", plainPayload)
	}
	for _, candidate := range plainResponse.Candidates {
		if candidate.ScoreBreakdown != nil {
			t.Fatalf("default candidate carried breakdown: %#v", candidate)
		}
	}

	explainConfig := DefaultToolSearchConfig()
	explainConfig.Explain = true
	explained := newToolSearchTestEngineWithConfig(t, explainConfig)
	explainedResponse, err := explained.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("Search(explain) error = %v", err)
	}
	if len(explainedResponse.Candidates) != len(plainResponse.Candidates) {
		t.Fatalf("explain changed candidate count: %d vs %d", len(explainedResponse.Candidates), len(plainResponse.Candidates))
	}
	for index := range explainedResponse.Candidates {
		plainPath := plainResponse.Candidates[index].CanonicalPath
		candidate := explainedResponse.Candidates[index]
		if candidate.CanonicalPath != plainPath {
			t.Fatalf("explain changed ordering at %d: %s vs %s", index, plainPath, candidate.CanonicalPath)
		}
		if candidate.ScoreBreakdown == nil {
			t.Fatalf("explained candidate missing breakdown: %#v", candidate)
		}
		if candidate.ScoreBreakdown.Score <= 0 || len(candidate.ScoreBreakdown.FieldScores) == 0 {
			t.Fatalf("breakdown incomplete: %#v", candidate.ScoreBreakdown)
		}
		if candidate.ScoreBreakdown.Multiplier != nil || candidate.ScoreBreakdown.QueryClass != nil {
			t.Fatalf("ensemble breakdown should not carry action fields: %#v", candidate.ScoreBreakdown)
		}
	}
}

func TestToolSearchActionExplainIncludesMultiplierAndQueryClass(t *testing.T) {
	config := DefaultToolSearchConfig()
	config.LexicalAlgorithm = ToolSearchLexicalBM25Action
	config.Explain = true
	engine := newToolSearchTestEngineWithConfig(t, config)
	response, err := engine.Search(context.Background(), ToolSearchRequest{Query: "给群里发送消息"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Candidates) == 0 || response.Candidates[0].CanonicalPath != "chat.send" {
		t.Fatalf("action candidates = %#v", response.Candidates)
	}
	for _, candidate := range response.Candidates {
		if candidate.ScoreBreakdown == nil {
			t.Fatalf("action candidate missing breakdown: %#v", candidate)
		}
		if candidate.ScoreBreakdown.Multiplier == nil || *candidate.ScoreBreakdown.Multiplier <= 0 {
			t.Fatalf("action breakdown missing multiplier: %#v", candidate.ScoreBreakdown)
		}
		if candidate.ScoreBreakdown.QueryClass == nil || len(candidate.ScoreBreakdown.QueryClass.Actions) == 0 {
			t.Fatalf("action breakdown missing query class: %#v", candidate.ScoreBreakdown)
		}
	}
}

func TestToolSearchAvoidWhenPenaltyMatchesEchoedPhraseOnly(t *testing.T) {
	tool := toolSearchTestTool("chat", "send", "chat send", "发送群聊消息", "write")
	tool.Selection.AvoidWhen = []string{
		"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令",
		"只搜索群文件时使用 drive search",
	}
	if reason := toolSearchAvoidWhenPenaltyReason("只搜索群文件时使用 drive search", tool); reason == "" {
		t.Fatal("echoed avoid_when phrase was not penalized")
	}
	if reason := toolSearchAvoidWhenPenaltyReason("给群里发送消息", tool); reason != "" {
		t.Fatalf("unrelated query was penalized: %q", reason)
	}
	if reason := toolSearchAvoidWhenPenaltyReason("需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令", tool); reason != "" {
		t.Fatal("shortcut boilerplate should be filtered out")
	}
	if reason := toolSearchAvoidWhenPenaltyReason("搜索群文件", tool); reason != "" {
		t.Fatalf("partial phrase overlap must not trigger the penalty: %q", reason)
	}
}

func TestToolSearchAvoidWhenPenaltyVisibleInExplainAndDemotesScore(t *testing.T) {
	tools := []ToolSpec{
		toolSearchTestTool("chat", "send", "chat send", "发送群聊消息", "write"),
		toolSearchTestTool("chat", "read_status", "chat read-status", "查询群消息已读状态", "read"),
	}
	tools[0].Selection.AvoidWhen = []string{"仅查询状态时使用读取命令"}
	registry := SchemaRegistry{
		Kind:     "schema",
		Level:    "catalog",
		Products: []ProductSpec{{ID: "chat", Tools: tools}},
	}
	build := func(explain bool) *ToolSearchEngine {
		config := DefaultToolSearchConfig()
		config.CatalogSourceHash = "source-test"
		config.CatalogSurfaceHash = "surface-test"
		config.Explain = explain
		engine, err := NewToolSearchEngine(registry, config)
		if err != nil {
			t.Fatalf("NewToolSearchEngine(explain=%v): %v", explain, err)
		}
		return engine
	}
	// The query echoes chat.send's avoid_when phrase verbatim, so the
	// demotion layer must fire on the ensemble default path.
	query := ToolSearchRequest{Query: "发送群聊消息，仅查询状态时使用读取命令"}
	explained, err := build(true).Search(context.Background(), query)
	if err != nil {
		t.Fatalf("Search(explain) error = %v", err)
	}
	var send *ToolReference
	for index := range explained.Candidates {
		if explained.Candidates[index].CanonicalPath == "chat.send" {
			send = &explained.Candidates[index]
		}
	}
	if send == nil {
		t.Fatalf("chat.send dropped out entirely: %#v", explained.Candidates)
	}
	if send.ScoreBreakdown == nil || send.ScoreBreakdown.AvoidWhenPenalty == "" {
		t.Fatalf("avoid_when penalty not visible in explain: %#v", send)
	}
	plain, err := build(false).Search(context.Background(), query)
	if err != nil {
		t.Fatalf("Search(plain) error = %v", err)
	}
	for index := range plain.Candidates {
		if plain.Candidates[index].CanonicalPath == "chat.send" && index >= 0 && send.Rank > 0 {
			// Demotion must not reorder above the plain run's rank.
			if send.Rank < index+1 {
				t.Fatalf("penalized rank %d improved over plain rank %d", send.Rank, index+1)
			}
		}
	}
}

func TestResolveToolSearchConfigFromEnvOverridesOnlyWhenSet(t *testing.T) {
	config := ResolveToolSearchConfigFromEnv(DefaultToolSearchConfig())
	if config.LexicalAlgorithm != ToolSearchLexicalBM25 || config.BM25K1 != defaultToolSearchBM25K1 || config.Explain {
		t.Fatalf("no-env config changed: %#v", config)
	}

	t.Setenv("DWS_TOOL_SEARCH_ALGORITHM", ToolSearchLexicalBM25Action)
	t.Setenv("DWS_TOOL_SEARCH_K1", "1.2")
	t.Setenv("DWS_TOOL_SEARCH_WEIGHT_SUMMARY", "4.5")
	t.Setenv("DWS_TOOL_SEARCH_EXPLAIN", "1")
	config = ResolveToolSearchConfigFromEnv(DefaultToolSearchConfig())
	if config.LexicalAlgorithm != ToolSearchLexicalBM25Action {
		t.Fatalf("algorithm override = %q", config.LexicalAlgorithm)
	}
	if config.BM25K1 != 1.2 {
		t.Fatalf("k1 override = %v", config.BM25K1)
	}
	if config.FieldWeights.Summary != 4.5 {
		t.Fatalf("summary weight override = %v", config.FieldWeights.Summary)
	}
	if !config.Explain {
		t.Fatal("explain override not applied")
	}
	if config.BM25B != defaultToolSearchBM25B {
		t.Fatalf("unset env should keep default b, got %v", config.BM25B)
	}

	t.Setenv("DWS_TOOL_SEARCH_ALGORITHM", "not-a-real-algorithm")
	if got := ResolveToolSearchConfigFromEnv(DefaultToolSearchConfig()).LexicalAlgorithm; got != ToolSearchLexicalBM25 {
		t.Fatalf("invalid algorithm env should be ignored, got %q", got)
	}
}

func TestToolSearchRejectsNonFiniteAndNegativeRankingConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolSearchConfig)
	}{
		{name: "nan k1", mutate: func(config *ToolSearchConfig) { config.BM25K1 = math.NaN() }},
		{name: "negative k1", mutate: func(config *ToolSearchConfig) { config.BM25K1 = -1 }},
		{name: "infinite b", mutate: func(config *ToolSearchConfig) { config.BM25B = math.Inf(1) }},
		{name: "negative b", mutate: func(config *ToolSearchConfig) { config.BM25B = -0.1 }},
		{name: "b above one", mutate: func(config *ToolSearchConfig) { config.BM25B = 1.01 }},
		{name: "negative identity weight", mutate: func(config *ToolSearchConfig) { config.FieldWeights.Identity = -1 }},
		{name: "nan summary weight", mutate: func(config *ToolSearchConfig) { config.FieldWeights.Summary = math.NaN() }},
	}
	registry := newToolSearchTestRegistry(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultToolSearchConfig()
			test.mutate(&config)
			if _, err := NewToolSearchEngine(registry, config); err == nil {
				t.Fatalf("NewToolSearchEngine() accepted invalid config: %#v", config)
			}
		})
	}
	t.Setenv("DWS_TOOL_SEARCH_K1", "NaN")
	if _, err := NewToolSearchEngine(registry, ResolveToolSearchConfigFromEnv(DefaultToolSearchConfig())); err == nil {
		t.Fatal("NewToolSearchEngine() accepted NaN from DWS_TOOL_SEARCH_K1")
	}
}

func TestToolSearchLexicalRetrieverCanSelectGoTFIDF(t *testing.T) {
	engine := newToolSearchTestEngine(t)
	engine.lexical = newToolSearchTFIDFRetriever(engine.documents, engine.config.FieldWeights, false)
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
	engine := newToolSearchTestEngine(t)
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
	engine, err := NewToolSearchEngine(registry, config)
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
	if !response.Truncated {
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
		Hint: toolSearchAbstainHint,
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

func TestToolSearchQuantizedOrderingBreaksUlpTiesCanonically(t *testing.T) {
	// Scores differing below the quantization grid must fall back to the
	// canonical tie-break: Go's architecture-dependent FMA fusion can produce
	// exactly such ulp-level differences for the same expression across
	// arm64/amd64, and the deterministic wire contract must not depend on them.
	hits := []LexicalHit{
		{CanonicalPath: "zzz.uldp_tie", Score: 1.0000000004},
		{CanonicalPath: "aaa.uldp_tie", Score: 1.0000000001},
		{CanonicalPath: "mmm.clearly_higher", Score: 2},
	}
	got := truncateAndSortLexicalHits(hits, 3)
	if got[0].CanonicalPath != "mmm.clearly_higher" {
		t.Fatalf("first = %s, want mmm.clearly_higher", got[0].CanonicalPath)
	}
	if got[1].CanonicalPath != "aaa.uldp_tie" || got[2].CanonicalPath != "zzz.uldp_tie" {
		t.Fatalf("ulp-level tie must order canonically, got %s then %s", got[1].CanonicalPath, got[2].CanonicalPath)
	}
}

func TestToolSearchIsDeterministicAcrossProcesses(t *testing.T) {
	const helperEnv = "DWS_TOOL_SEARCH_DETERMINISM_CHILD"
	if os.Getenv(helperEnv) == "1" {
		engine := newToolSearchTestEngine(t)
		tfidfEngine := newToolSearchTestEngine(t)
		tfidfEngine.lexical = newToolSearchTFIDFRetriever(tfidfEngine.documents, tfidfEngine.config.FieldWeights, false)
		tests := []struct {
			name    string
			engine  *ToolSearchEngine
			request ToolSearchRequest
		}{
			{name: "pure_chinese", engine: engine, request: ToolSearchRequest{Query: "给群里发文件并确认消息已读", Limit: 20}},
			{name: "mixed_identifiers", engine: engine, request: ToolSearchRequest{Query: "给群里发文件并确认消息已读 baseId openConversationId status upload", Limit: 20}},
			{name: "exact", engine: engine, request: ToolSearchRequest{Query: "chat.read_status", Limit: 20}},
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
			if len(outputs) != 4 || outputs[2].Response.Strategy != "exact_guard" || outputs[3].Response.Strategy != ToolSearchLexicalTFIDF {
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
	engine := newToolSearchTestEngine(b)
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
		_ = newToolSearchTestEngine(b)
	}
}

type toolSearchTestingT interface {
	Helper()
	Fatalf(string, ...any)
}

func newToolSearchTestEngine(t toolSearchTestingT) *ToolSearchEngine {
	t.Helper()
	return newToolSearchTestEngineWithConfig(t, DefaultToolSearchConfig())
}

func newToolSearchTestEngineWithConfig(t toolSearchTestingT, config ToolSearchConfig) *ToolSearchEngine {
	t.Helper()
	config.CatalogSourceHash = "source-test"
	config.CatalogSurfaceHash = "surface-test"
	engine, err := NewToolSearchEngine(newToolSearchTestRegistry(t), config)
	if err != nil {
		t.Fatalf("NewToolSearchEngine() error = %v", err)
	}
	return engine
}

func newToolSearchTestRegistry(t toolSearchTestingT) SchemaRegistry {
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
	return SchemaRegistry{
		Kind:  "schema",
		Level: "catalog",
		Products: []ProductSpec{
			{ID: "chat", Tools: tools[:2]},
			{ID: "doc", Tools: tools[2:3]},
			{ID: "drive", Tools: tools[3:]},
		},
	}
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

func TestToolSearchValidatesEffectVocabularyCaseInsensitively(t *testing.T) {
	engine := newToolSearchTestEngine(t)
	if _, err := engine.Search(context.Background(), ToolSearchRequest{
		Query: "查询群消息已读状态", Effects: []string{"READ"},
	}); err != nil {
		t.Fatalf("upper-case effect should normalize: %v", err)
	}
	for _, effect := range []string{"rw", "删除", "readx"} {
		if _, err := engine.Search(context.Background(), ToolSearchRequest{
			Query: "查询群消息已读状态", Effects: []string{effect},
		}); err == nil {
			t.Fatalf("effect %q accepted", effect)
		} else {
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "invalid_effect" {
				t.Fatalf("effect %q error = %v, want invalid_effect", effect, err)
			}
		}
	}
}

func TestToolSearchRejectsDangerousFilterUnicodeBeforeVocabulary(t *testing.T) {
	// Control/Bidi characters must be refused before the effect vocabulary and
	// Catalog product checks run, otherwise the rejected value is echoed back
	// through an invalid_effect / unknown_product message that can carry
	// invisible characters into a terminal or log line.
	engine := newToolSearchTestEngine(t)
	for name, request := range map[string]ToolSearchRequest{
		"effect_control_char":  {Query: "查询群消息已读状态", Effects: []string{"read\u0007"}},
		"effect_bidi_override": {Query: "查询群消息已读状态", Effects: []string{"read\u202e"}},
		"product_bidi_override": {
			Query: "查询群消息已读状态", ProductIDs: []string{"chat\u202e"},
		},
		"exclude_control_char": {
			Query: "查询群消息已读状态", ExcludeCanonicalPaths: []string{"chat.send\u0000"},
		},
	} {
		_, err := engine.Search(context.Background(), request)
		if err == nil {
			t.Fatalf("%s: dangerous filter unicode accepted", name)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "dangerous_filter_unicode" {
			t.Fatalf("%s: error = %v, want dangerous_filter_unicode", name, err)
		}
	}
}

func TestToolSearchRejectsUnknownProduct(t *testing.T) {
	engine := newToolSearchTestEngine(t)
	if _, err := engine.Search(context.Background(), ToolSearchRequest{
		Query: "查询群消息已读状态", ProductIDs: []string{"chat"},
	}); err != nil {
		t.Fatalf("known product rejected: %v", err)
	}
	_, err := engine.Search(context.Background(), ToolSearchRequest{
		Query: "查询群消息已读状态", ProductIDs: []string{"chta"},
	})
	if err == nil {
		t.Fatal("unknown product accepted")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "unknown_product" {
		t.Fatalf("unknown product error = %v, want unknown_product", err)
	}
}

func TestToolSearchAbstainCarriesConvergenceHint(t *testing.T) {
	engine := newToolSearchTestEngine(t)
	response, err := engine.Search(context.Background(), ToolSearchRequest{
		Query: "量子涨落检测器校准",
	})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if !response.Abstained || len(response.Candidates) != 0 {
		t.Fatalf("expected abstained empty response, got abstained=%v candidates=%d", response.Abstained, len(response.Candidates))
	}
	if response.Hint == "" {
		t.Fatal("abstained lexical response must carry a convergence hint")
	}
	if response.ExactFiltered != nil {
		t.Fatalf("natural-language query must not produce exact_filtered, got %v", response.ExactFiltered)
	}
	// exact_filtered keeps its typed refusal and must not inherit the hint.
	filtered, err := engine.Search(context.Background(), ToolSearchRequest{
		Query: "chat send", ProductIDs: []string{"doc"},
	})
	if err != nil {
		t.Fatalf("filtered search error: %v", err)
	}
	if filtered.ExactFiltered == nil {
		t.Fatal("expected exact_filtered refusal")
	}
	if filtered.Hint != "" {
		t.Fatalf("exact_filtered must keep its own reason, got extra hint %q", filtered.Hint)
	}
}

func TestToolSearchWeakMatchFlagsFieldOnlyHits(t *testing.T) {
	weak := ToolSearchResponse{Candidates: []ToolReference{
		{CanonicalPath: "a", MatchedFields: []string{"parameters"}},
		{CanonicalPath: "b", MatchedFields: []string{"description", "parameters"}},
	}}
	finalized, err := finalizeToolSearchResponse(weak)
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.WeakMatch || finalized.Hint != toolSearchWeakMatchHint {
		t.Fatalf("expected weak_match hint, got weak=%v hint=%q", finalized.WeakMatch, finalized.Hint)
	}
	strong := ToolSearchResponse{Candidates: []ToolReference{
		{CanonicalPath: "a", MatchedFields: []string{"parameters"}},
		{CanonicalPath: "b", MatchedFields: []string{"summary"}},
	}}
	finalized, err = finalizeToolSearchResponse(strong)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.WeakMatch || finalized.Hint != "" {
		t.Fatalf("summary hit must clear weak flag, got weak=%v hint=%q", finalized.WeakMatch, finalized.Hint)
	}
}
