// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	ToolSearchLexicalBM25       = "fielded_bm25_ensemble"
	ToolSearchLexicalBM25Action = "fielded_bm25_action_v1"
	ToolSearchLexicalTFIDF      = "fielded_tfidf_cosine"
)

// ToolSearchLexicalRequest contains a prefiltered Catalog domain. Hard
// filtering occurs before scoring, so a retriever cannot spend its Top-K on
// candidates that DWS is required to reject.
type ToolSearchLexicalRequest struct {
	Query                  string
	CandidateLimit         int
	EligibleCanonicalPaths []string
}

// LexicalHit is an internal ordering result, not a confidence probability.
// Explain is populated only when the engine was built with Explain=true.
type LexicalHit struct {
	CanonicalPath string
	Score         float64
	MatchedFields []string
	Explain       *ToolSearchScoreBreakdown
}

// LexicalRetriever is the zero-model local recall boundary. Implementations
// must return a deterministic score-desc/canonical-asc order and omit zero
// score documents.
type LexicalRetriever interface {
	Name() string
	Retrieve(context.Context, ToolSearchLexicalRequest) ([]LexicalHit, error)
}

func newToolSearchLexicalRetriever(documents map[string]toolSearchDocument, config ToolSearchConfig) (LexicalRetriever, error) {
	var retriever LexicalRetriever
	switch config.LexicalAlgorithm {
	case ToolSearchLexicalBM25Action:
		retriever = newToolSearchActionRetriever(
			newToolSearchBM25Retriever(documents, config.FieldWeights, config.BM25K1, config.BM25B, config.Explain),
			documents,
			config.Explain,
		)
	case ToolSearchLexicalBM25:
		retriever = newToolSearchBM25Retriever(documents, config.FieldWeights, config.BM25K1, config.BM25B, config.Explain)
	case ToolSearchLexicalTFIDF:
		retriever = newToolSearchTFIDFRetriever(documents, config.FieldWeights, config.Explain)
	default:
		return nil, fmt.Errorf("unknown tool search lexical algorithm %q", config.LexicalAlgorithm)
	}
	return newToolSearchAvoidWhenRetriever(retriever, documents, config.Explain), nil
}

type toolSearchActionRetriever struct {
	base      LexicalRetriever
	documents map[string]toolSearchDocument
	explain   bool
}

var toolSearchASCIIWordPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9._-]*`)

func newToolSearchActionRetriever(base LexicalRetriever, documents map[string]toolSearchDocument, explain bool) *toolSearchActionRetriever {
	return &toolSearchActionRetriever{base: base, documents: documents, explain: explain}
}

func (*toolSearchActionRetriever) Name() string { return ToolSearchLexicalBM25Action }

func (r *toolSearchActionRetriever) Retrieve(ctx context.Context, request ToolSearchLexicalRequest) ([]LexicalHit, error) {
	deep := request
	deep.CandidateLimit = len(request.EligibleCanonicalPaths)
	hits, err := r.base.Retrieve(ctx, deep)
	if err != nil {
		return nil, err
	}
	query := classifyToolSearchQuery(request.Query)
	if !query.hasSignal() || toolSearchHasUnclassifiedASCII(request.Query) {
		return truncateAndSortLexicalHits(hits, request.CandidateLimit), nil
	}
	for index := range hits {
		multiplier := toolSearchStructuredMultiplier(query, r.documents[hits[index].CanonicalPath].tool)
		hits[index].Score *= multiplier
		if r.explain && hits[index].Explain != nil {
			hits[index].Explain.Multiplier = &multiplier
			hits[index].Explain.Score = hits[index].Score
			hits[index].Explain.QueryClass = newToolSearchExplainQueryClass(query)
		}
	}
	return truncateAndSortLexicalHits(hits, request.CandidateLimit), nil
}

// toolSearchHasUnclassifiedASCII reports whether the query carries a technical
// identifier (camelCase or separator-joined tokens such as openConversationId,
// task_id, file-path) that the compact vocabularies cannot classify. Plain
// English words no longer disable structured reranking: they match (or fail
// to match) the vocabularies exactly like Chinese phrases do, so mixed and
// English queries keep the effect/product/action/entity routing instead of
// silently degrading to raw BM25.
func toolSearchHasUnclassifiedASCII(query string) bool {
	for _, token := range toolSearchASCIIWordPattern.FindAllString(query, -1) {
		if len(token) < 4 {
			continue
		}
		switch strings.ToLower(token) {
		case "id", "ids", "url", "uri":
			continue
		}
		if strings.ContainsAny(token, "._-") {
			return true
		}
		// Only an internal uppercase transition (openConversationId) marks a
		// technical identifier; a capitalized plain word ("Send") does not.
		if internal := token[1:]; strings.ToLower(internal) != internal {
			return true
		}
	}
	return false
}

type toolSearchQueryClass struct {
	actions  map[string]bool
	products map[string]bool
	entities map[string]bool
	effect   string
}

func (q toolSearchQueryClass) hasSignal() bool {
	return len(q.actions) > 0 || len(q.products) > 0 || len(q.entities) > 0 || q.effect != ""
}

type toolSearchTermClass struct {
	name   string
	query  []string
	tool   []string
	effect string
}

var toolSearchActionClasses = []toolSearchTermClass{
	{name: "approve", query: []string{"同意", "批准", "审批通过", "approve"}, tool: []string{"approve"}},
	{name: "reject", query: []string{"拒绝", "驳回", "reject"}, tool: []string{"reject"}},
	{name: "upload", query: []string{"上传", "upload"}, tool: []string{"upload"}},
	{name: "download", query: []string{"下载", "download"}, tool: []string{"download"}},
	{name: "send", query: []string{"发送", "发给", "发消息", "通知", "投递", "send", "notify"}, tool: []string{"send", "notify"}},
	{name: "search", query: []string{"搜索", "查找", "检索", "定位", "search", "find", "locate"}, tool: []string{"search", "find", "resolve"}},
	{name: "list", query: []string{"列出", "列表", "浏览", "遍历", "list", "browse"}, tool: []string{"list", "all"}},
	{name: "read", query: []string{"读取", "查看", "获取", "详情", "正文", "read", "fetch"}, tool: []string{"read", "get", "detail", "info", "query"}},
	{name: "create", query: []string{"创建", "新建", "create"}, tool: []string{"create", "new"}},
	{name: "add", query: []string{"添加", "追加", "增加", "授权", "授予", "add", "invite"}, tool: []string{"add", "grant", "invite"}},
	{name: "update", query: []string{"更新", "修改", "编辑", "重命名", "调整", "update", "edit", "rename"}, tool: []string{"update", "edit", "rename", "set"}},
	{name: "delete", query: []string{"删除", "移除", "清除", "delete", "remove"}, tool: []string{"delete", "remove", "clear"}},
	{name: "complete", query: []string{"完成", "提交入库", "complete", "commit"}, tool: []string{"complete", "commit", "finish"}},
	{name: "enable", query: []string{"启用", "开启", "enable"}, tool: []string{"enable"}},
	{name: "disable", query: []string{"禁用", "停用", "关闭", "disable"}, tool: []string{"disable"}},
}

var toolSearchProductClasses = []toolSearchTermClass{
	{name: "chat", query: []string{"群聊", "群消息", "会话", "单聊", "chat"}, tool: []string{"chat"}},
	{name: "calendar", query: []string{"日程", "会议日历", "会议室", "calendar"}, tool: []string{"calendar"}},
	{name: "drive", query: []string{"钉盘", "云盘", "drive"}, tool: []string{"drive"}},
	{name: "wiki", query: []string{"知识库", "wiki"}, tool: []string{"wiki"}},
	{name: "todo", query: []string{"待办", "todo"}, tool: []string{"todo"}},
	{name: "minutes", query: []string{"听记", "会议纪要", "minutes"}, tool: []string{"minutes"}},
	{name: "oa", query: []string{"审批", "oa", "approval"}, tool: []string{"oa"}},
	{name: "mail", query: []string{"邮件", "邮箱", "草稿", "mail", "email"}, tool: []string{"mail"}},
	{name: "sheet", query: []string{"电子表格", "工作表", "单元格", "sheet", "spreadsheet"}, tool: []string{"sheet"}},
	{name: "doc", query: []string{"在线文字文档", "文档正文", "adoc", "doc", "document"}, tool: []string{"doc"}},
}

var toolSearchEntityClasses = []toolSearchTermClass{
	{name: "task_id", query: []string{"任务 id", "任务id", "task id", "taskid"}, tool: []string{"任务 id", "任务id", "task id", "taskid"}},
	{name: "task", query: []string{"任务", "task id", "taskid", "task"}, tool: []string{"任务", "task"}},
	{name: "permission", query: []string{"权限", "授权", "协作者", "permission"}, tool: []string{"权限", "授权", "协作者", "permission"}},
	{name: "content", query: []string{"正文", "文档内容", "content"}, tool: []string{"正文", "内容", "content"}},
	{name: "status", query: []string{"状态", "已读", "status"}, tool: []string{"状态", "已读", "status"}},
	{name: "draft", query: []string{"草稿", "draft"}, tool: []string{"草稿", "draft"}},
	{name: "event", query: []string{"日程", "event"}, tool: []string{"日程", "event"}},
	{name: "reminder", query: []string{"提醒", "reminder"}, tool: []string{"提醒", "reminder"}},
	{name: "summary", query: []string{"摘要", "summary"}, tool: []string{"摘要", "summary"}},
	{name: "sheet", query: []string{"工作表", "worksheet"}, tool: []string{"工作表", "sheet"}},
	{name: "range", query: []string{"单元格范围", "区域", "range"}, tool: []string{"范围", "区域", "range"}},
	{name: "group", query: []string{"群聊", "群名", "group"}, tool: []string{"群聊", "群", "group"}},
}

func classifyToolSearchQuery(query string) toolSearchQueryClass {
	normalized := strings.ToLower(strings.TrimSpace(query))
	result := toolSearchQueryClass{actions: map[string]bool{}, products: map[string]bool{}, entities: map[string]bool{}}
	for _, class := range toolSearchActionClasses {
		if containsAnyToolSearchPhrase(normalized, class.query) {
			result.actions[class.name] = true
		}
	}
	for _, class := range toolSearchProductClasses {
		if containsAnyToolSearchPhrase(normalized, class.query) {
			result.products[class.name] = true
		}
	}
	for _, class := range toolSearchEntityClasses {
		if class.name == "task" && result.entities["task_id"] {
			continue
		}
		if containsAnyToolSearchPhrase(normalized, class.query) {
			result.entities[class.name] = true
		}
	}
	// “查询” is intentionally only a broad read-effect signal. Treating it as
	// get/list/search would impose a false sibling preference.
	if strings.Contains(normalized, "查询") || result.actions["read"] || result.actions["search"] || result.actions["list"] {
		result.effect = "read"
	}
	return result
}

func toolSearchStructuredMultiplier(query toolSearchQueryClass, tool ToolSpec) float64 {
	multiplier := 1.0
	if query.effect != "" {
		if tool.Safety.Effect == query.effect {
			multiplier += 0.10
		} else {
			multiplier -= 0.50
		}
	}
	if len(query.products) > 0 {
		if query.products[tool.Identity.ProductID] {
			multiplier += 0.50
		} else {
			multiplier -= 0.30
		}
	}
	toolText := strings.ToLower(strings.Join([]string{
		tool.Identity.Name,
		tool.Identity.CLIName,
		tool.Identity.CanonicalPath,
		tool.Identity.PrimaryCLIPath,
		tool.Title,
		tool.Selection.AgentSummary,
	}, " "))
	if len(query.actions) > 0 {
		matched := false
		for _, class := range toolSearchActionClasses {
			if query.actions[class.name] && containsAnyToolSearchWord(toolText, class.tool) {
				matched = true
				break
			}
		}
		if matched {
			multiplier += 0.60
		} else {
			// An explicit action is a stronger constraint than generic lexical
			// overlap. Penalize tools that do not advertise that action even
			// when their sibling verb is not in the compact action vocabulary.
			multiplier -= 0.65
		}
	}
	if len(query.entities) > 0 {
		matched := false
		for _, class := range toolSearchEntityClasses {
			if query.entities[class.name] && containsAnyToolSearchPhrase(toolText, class.tool) {
				matched = true
				break
			}
		}
		if matched {
			multiplier += 0.35
		} else {
			multiplier -= 0.10
		}
	}
	if multiplier < 0.20 {
		return 0.20
	}
	return multiplier
}

func containsAnyToolSearchPhrase(text string, values []string) bool {
	for _, value := range values {
		if toolSearchPhraseContains(text, value) {
			return true
		}
	}
	return false
}

// toolSearchPhraseContains matches ASCII single-token values on word
// boundaries so short English values such as "oa" cannot fire inside
// unrelated words ("download" contains "oa"). Mixed-script or multi-word
// values keep substring semantics, which is what Chinese phrases and values
// like "task id" rely on.
func toolSearchPhraseContains(text, value string) bool {
	if value != "" && isAllToolSearchASCII(value) && !strings.ContainsAny(value, " \t") {
		return containsToolSearchASCIIWord(text, value)
	}
	return strings.Contains(text, value)
}

func isAllToolSearchASCII(value string) bool {
	for _, character := range value {
		if character >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func containsToolSearchASCIIWord(text, word string) bool {
	word = strings.ToLower(word)
	for _, token := range toolSearchASCIIWordPattern.FindAllString(strings.ToLower(text), -1) {
		if token == word {
			return true
		}
	}
	return false
}

func containsAnyToolSearchWord(text string, values []string) bool {
	normalized := strings.NewReplacer(".", " ", "_", " ", "-", " ", "/", " ").Replace(text)
	words := strings.Fields(normalized)
	for _, value := range values {
		for _, word := range words {
			if word == value {
				return true
			}
		}
	}
	return false
}

func newToolSearchExplainQueryClass(query toolSearchQueryClass) *ToolSearchExplainQueryClass {
	return &ToolSearchExplainQueryClass{
		Actions:  sortedToolSearchClassKeys(query.actions),
		Products: sortedToolSearchClassKeys(query.products),
		Entities: sortedToolSearchClassKeys(query.entities),
		Effect:   query.effect,
	}
}

func sortedToolSearchClassKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// toolSearchAvoidWhenPenalty is applied when a candidate's avoid_when prose
// matches the query intent. It is a soft demotion, not an eligibility filter:
// avoid_when is natural-language guidance (often "prefer X for that intent"),
// so a lexical hit can be wrong and the tool must remain reachable.
const toolSearchAvoidWhenPenalty = 0.20

// toolSearchAvoidWhenShortcutNoise filters the boilerplate avoid_when that
// every shortcut carries; it carries no intent signal.
const toolSearchAvoidWhenShortcutNoise = "需要该 Shortcut 未公开的底层参数"

// toolSearchAvoidWhenPenaltyReason returns the matched avoid_when phrase, or
// "" when the tool should not be demoted for this query. The only match shape
// is the query containing the avoidance prose itself (the Forbidden@5
// evaluation proxy). Intent-overlap matching was tried and rejected: a tool's
// own avoid_when describes the same sibling scenario space as its use_when,
// so "shares an action/entity with the phrase" demotes the gold answer.
func toolSearchAvoidWhenPenaltyReason(query string, tool ToolSpec) string {
	if len(tool.Selection.AvoidWhen) == 0 {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return ""
	}
	for _, phrase := range tool.Selection.AvoidWhen {
		phrase = strings.ToLower(strings.TrimSpace(phrase))
		if phrase == "" || strings.Contains(phrase, toolSearchAvoidWhenShortcutNoise) {
			continue
		}
		if strings.Contains(normalized, phrase) {
			return phrase
		}
	}
	return ""
}

// toolSearchAvoidWhenRetriever demotes candidates whose avoid_when prose
// matches the query intent. It wraps every lexical algorithm, including the
// production ensemble default, so the Forbidden exposure guard is not tied to
// action_v1.
type toolSearchAvoidWhenRetriever struct {
	base      LexicalRetriever
	documents map[string]toolSearchDocument
	explain   bool
}

func newToolSearchAvoidWhenRetriever(base LexicalRetriever, documents map[string]toolSearchDocument, explain bool) *toolSearchAvoidWhenRetriever {
	return &toolSearchAvoidWhenRetriever{base: base, documents: documents, explain: explain}
}

func (r *toolSearchAvoidWhenRetriever) Name() string { return r.base.Name() }

func (r *toolSearchAvoidWhenRetriever) Retrieve(ctx context.Context, request ToolSearchLexicalRequest) ([]LexicalHit, error) {
	hits, err := r.base.Retrieve(ctx, request)
	if err != nil {
		return nil, err
	}
	penalized := false
	for index := range hits {
		reason := toolSearchAvoidWhenPenaltyReason(request.Query, r.documents[hits[index].CanonicalPath].tool)
		if reason == "" {
			continue
		}
		hits[index].Score *= toolSearchAvoidWhenPenalty
		if r.explain && hits[index].Explain != nil {
			hits[index].Explain.Score = hits[index].Score
			hits[index].Explain.AvoidWhenPenalty = reason
		}
		penalized = true
	}
	if !penalized {
		return hits, nil
	}
	return truncateAndSortLexicalHits(hits, request.CandidateLimit), nil
}

type toolSearchBM25Retriever struct {
	fieldIndex map[toolSearchField]toolSearchBM25Index
	weights    ToolSearchFieldWeights
	explain    bool
}

func newToolSearchBM25Retriever(documents map[string]toolSearchDocument, weights ToolSearchFieldWeights, k1, b float64, explain bool) *toolSearchBM25Retriever {
	fieldTerms := toolSearchFieldTerms(documents)
	fieldIndex := make(map[toolSearchField]toolSearchBM25Index, len(toolSearchFieldOrder))
	for _, field := range toolSearchFieldOrder {
		fieldIndex[field] = newToolSearchBM25Index(fieldTerms[field], k1, b)
	}
	return &toolSearchBM25Retriever{fieldIndex: fieldIndex, weights: weights, explain: explain}
}

func (*toolSearchBM25Retriever) Name() string { return ToolSearchLexicalBM25 }

func (r *toolSearchBM25Retriever) Retrieve(ctx context.Context, request ToolSearchLexicalRequest) ([]LexicalHit, error) {
	queryTerms := orderedToolSearchQueryTerms(request.Query)
	hits := make([]LexicalHit, 0, len(request.EligibleCanonicalPaths))
	for _, canonical := range request.EligibleCanonicalPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contributions := make(map[toolSearchField]float64)
		score := 0.0
		for _, field := range toolSearchFieldOrder {
			weight := toolSearchFieldWeight(r.weights, field)
			if weight <= 0 {
				continue
			}
			value := r.fieldIndex[field].score(canonical, queryTerms) * weight
			if value > 0 {
				contributions[field] = value
				score += value
			}
		}
		if score > 0 {
			hit := LexicalHit{CanonicalPath: canonical, Score: score, MatchedFields: matchedToolSearchFields(contributions)}
			if r.explain {
				hit.Explain = newToolSearchBreakdown(score, contributions)
			}
			hits = append(hits, hit)
		}
	}
	return truncateAndSortLexicalHits(hits, request.CandidateLimit), nil
}

type toolSearchTFIDFDocument struct {
	weights map[string]float64
	norm    float64
}

type toolSearchTFIDFIndex struct {
	documents map[string]toolSearchTFIDFDocument
	idf       map[string]float64
}

type toolSearchTFIDFRetriever struct {
	fieldIndex map[toolSearchField]toolSearchTFIDFIndex
	weights    ToolSearchFieldWeights
	explain    bool
}

func newToolSearchTFIDFRetriever(documents map[string]toolSearchDocument, weights ToolSearchFieldWeights, explain bool) *toolSearchTFIDFRetriever {
	fieldTerms := toolSearchFieldTerms(documents)
	fieldIndex := make(map[toolSearchField]toolSearchTFIDFIndex, len(toolSearchFieldOrder))
	for _, field := range toolSearchFieldOrder {
		fieldIndex[field] = newToolSearchTFIDFIndex(fieldTerms[field])
	}
	return &toolSearchTFIDFRetriever{fieldIndex: fieldIndex, weights: weights, explain: explain}
}

func (*toolSearchTFIDFRetriever) Name() string { return ToolSearchLexicalTFIDF }

func (r *toolSearchTFIDFRetriever) Retrieve(ctx context.Context, request ToolSearchLexicalRequest) ([]LexicalHit, error) {
	queryFrequency := toolSearchTermFrequency(tokenizeToolSearchText(request.Query))
	hits := make([]LexicalHit, 0, len(request.EligibleCanonicalPaths))
	for _, canonical := range request.EligibleCanonicalPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contributions := make(map[toolSearchField]float64)
		score := 0.0
		for _, field := range toolSearchFieldOrder {
			weight := toolSearchFieldWeight(r.weights, field)
			if weight <= 0 {
				continue
			}
			value := r.fieldIndex[field].score(canonical, queryFrequency) * weight
			if value > 0 {
				contributions[field] = value
				score += value
			}
		}
		if score > 0 {
			hit := LexicalHit{CanonicalPath: canonical, Score: score, MatchedFields: matchedToolSearchFields(contributions)}
			if r.explain {
				hit.Explain = newToolSearchBreakdown(score, contributions)
			}
			hits = append(hits, hit)
		}
	}
	return truncateAndSortLexicalHits(hits, request.CandidateLimit), nil
}

func toolSearchFieldTerms(documents map[string]toolSearchDocument) map[toolSearchField]map[string][]string {
	fieldTerms := make(map[toolSearchField]map[string][]string, len(toolSearchFieldOrder))
	for _, field := range toolSearchFieldOrder {
		fieldTerms[field] = make(map[string][]string, len(documents))
	}
	for canonical, document := range documents {
		for _, field := range toolSearchFieldOrder {
			fieldTerms[field][canonical] = tokenizeToolSearchText(document.fields[field])
		}
	}
	return fieldTerms
}

func orderedToolSearchQueryTerms(query string) []toolSearchQueryTerm {
	frequency := toolSearchTermFrequency(tokenizeToolSearchText(query))
	terms := make([]toolSearchQueryTerm, 0, len(frequency))
	for term, count := range frequency {
		terms = append(terms, toolSearchQueryTerm{term: term, count: count})
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].term < terms[j].term })
	return terms
}

func toolSearchTermFrequency(tokens []string) map[string]int {
	frequency := make(map[string]int, len(tokens))
	for _, token := range tokens {
		frequency[token]++
	}
	return frequency
}

func newToolSearchTFIDFIndex(documents map[string][]string) toolSearchTFIDFIndex {
	index := toolSearchTFIDFIndex{documents: make(map[string]toolSearchTFIDFDocument, len(documents)), idf: map[string]float64{}}
	documentFrequency := map[string]int{}
	frequencies := make(map[string]map[string]int, len(documents))
	for canonical, tokens := range documents {
		frequency := toolSearchTermFrequency(tokens)
		frequencies[canonical] = frequency
		for term := range frequency {
			documentFrequency[term]++
		}
	}
	n := float64(len(documents))
	for term, frequency := range documentFrequency {
		index.idf[term] = math.Log((n+1)/(float64(frequency)+1)) + 1
	}
	for canonical, frequency := range frequencies {
		weights := make(map[string]float64, len(frequency))
		normSquared := 0.0
		terms := make([]string, 0, len(frequency))
		for term := range frequency {
			terms = append(terms, term)
		}
		sort.Strings(terms)
		for _, term := range terms {
			count := frequency[term]
			value := float64(count) * index.idf[term]
			weights[term] = value
			normSquared += value * value
		}
		index.documents[canonical] = toolSearchTFIDFDocument{weights: weights, norm: math.Sqrt(normSquared)}
	}
	return index
}

func (i toolSearchTFIDFIndex) score(canonical string, queryFrequency map[string]int) float64 {
	document, ok := i.documents[canonical]
	if !ok || document.norm == 0 {
		return 0
	}
	terms := make([]string, 0, len(queryFrequency))
	for term := range queryFrequency {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	dot := 0.0
	queryNormSquared := 0.0
	for _, term := range terms {
		idf, ok := i.idf[term]
		if !ok {
			continue
		}
		queryWeight := float64(queryFrequency[term]) * idf
		queryNormSquared += queryWeight * queryWeight
		dot += document.weights[term] * queryWeight
	}
	if queryNormSquared == 0 {
		return 0
	}
	return dot / (document.norm * math.Sqrt(queryNormSquared))
}

func truncateAndSortLexicalHits(hits []LexicalHit, limit int) []LexicalHit {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].CanonicalPath < hits[j].CanonicalPath
	})
	if limit >= 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
