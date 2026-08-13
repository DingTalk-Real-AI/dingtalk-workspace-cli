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
type LexicalHit struct {
	CanonicalPath string
	Score         float64
	MatchedFields []string
}

// LexicalRetriever is the zero-model local recall boundary. Implementations
// must return a deterministic score-desc/canonical-asc order and omit zero
// score documents.
type LexicalRetriever interface {
	Name() string
	Retrieve(context.Context, ToolSearchLexicalRequest) ([]LexicalHit, error)
}

func newToolSearchLexicalRetriever(documents map[string]toolSearchDocument, config ToolSearchConfig) (LexicalRetriever, error) {
	switch config.LexicalAlgorithm {
	case ToolSearchLexicalBM25Action:
		return newToolSearchActionRetriever(newToolSearchBM25Retriever(documents, config.FieldWeights), documents), nil
	case ToolSearchLexicalBM25:
		return newToolSearchBM25Retriever(documents, config.FieldWeights), nil
	case ToolSearchLexicalTFIDF:
		return newToolSearchTFIDFRetriever(documents, config.FieldWeights), nil
	default:
		return nil, fmt.Errorf("unknown tool search lexical algorithm %q", config.LexicalAlgorithm)
	}
}

type toolSearchActionRetriever struct {
	base      LexicalRetriever
	documents map[string]toolSearchDocument
}

var toolSearchASCIIWordPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9._-]*`)

func newToolSearchActionRetriever(base LexicalRetriever, documents map[string]toolSearchDocument) *toolSearchActionRetriever {
	return &toolSearchActionRetriever{base: base, documents: documents}
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
		hits[index].Score *= toolSearchStructuredMultiplier(query, r.documents[hits[index].CanonicalPath].tool)
	}
	return truncateAndSortLexicalHits(hits, request.CandidateLimit), nil
}

func toolSearchHasUnclassifiedASCII(query string) bool {
	for _, token := range toolSearchASCIIWordPattern.FindAllString(strings.ToLower(query), -1) {
		switch token {
		case "id", "ids", "url", "uri":
			continue
		default:
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
	{name: "approve", query: []string{"同意", "批准", "审批通过"}, tool: []string{"approve"}},
	{name: "reject", query: []string{"拒绝", "驳回"}, tool: []string{"reject"}},
	{name: "upload", query: []string{"上传"}, tool: []string{"upload"}},
	{name: "download", query: []string{"下载"}, tool: []string{"download"}},
	{name: "send", query: []string{"发送", "发给", "发消息", "通知", "投递"}, tool: []string{"send", "notify"}},
	{name: "search", query: []string{"搜索", "查找", "检索", "定位"}, tool: []string{"search", "find", "resolve"}},
	{name: "list", query: []string{"列出", "列表", "浏览", "遍历"}, tool: []string{"list", "all"}},
	{name: "read", query: []string{"读取", "查看", "获取", "详情", "正文"}, tool: []string{"read", "get", "detail", "info", "query"}},
	{name: "create", query: []string{"创建", "新建"}, tool: []string{"create", "new"}},
	{name: "add", query: []string{"添加", "追加", "增加", "授权", "授予"}, tool: []string{"add", "grant", "invite"}},
	{name: "update", query: []string{"更新", "修改", "编辑", "重命名", "调整"}, tool: []string{"update", "edit", "rename", "set"}},
	{name: "delete", query: []string{"删除", "移除", "清除"}, tool: []string{"delete", "remove", "clear"}},
	{name: "complete", query: []string{"完成", "提交入库"}, tool: []string{"complete", "commit", "finish"}},
	{name: "enable", query: []string{"启用", "开启"}, tool: []string{"enable"}},
	{name: "disable", query: []string{"禁用", "停用", "关闭"}, tool: []string{"disable"}},
}

var toolSearchProductClasses = []toolSearchTermClass{
	{name: "chat", query: []string{"群聊", "群消息", "会话", "单聊"}, tool: []string{"chat"}},
	{name: "calendar", query: []string{"日程", "会议日历", "会议室"}, tool: []string{"calendar"}},
	{name: "drive", query: []string{"钉盘", "云盘"}, tool: []string{"drive"}},
	{name: "wiki", query: []string{"知识库"}, tool: []string{"wiki"}},
	{name: "todo", query: []string{"待办"}, tool: []string{"todo"}},
	{name: "minutes", query: []string{"听记", "会议纪要"}, tool: []string{"minutes"}},
	{name: "oa", query: []string{"审批"}, tool: []string{"oa"}},
	{name: "mail", query: []string{"邮件", "邮箱", "草稿"}, tool: []string{"mail"}},
	{name: "sheet", query: []string{"电子表格", "工作表", "单元格"}, tool: []string{"sheet"}},
	{name: "doc", query: []string{"在线文字文档", "文档正文", "adoc"}, tool: []string{"doc"}},
}

var toolSearchEntityClasses = []toolSearchTermClass{
	{name: "task_id", query: []string{"任务 id", "任务id", "task id", "taskid"}, tool: []string{"任务 id", "任务id", "task id", "taskid"}},
	{name: "task", query: []string{"任务", "task id", "taskid"}, tool: []string{"任务", "task"}},
	{name: "permission", query: []string{"权限", "授权", "协作者"}, tool: []string{"权限", "授权", "协作者", "permission"}},
	{name: "content", query: []string{"正文", "文档内容"}, tool: []string{"正文", "内容", "content"}},
	{name: "status", query: []string{"状态", "已读"}, tool: []string{"状态", "已读", "status"}},
	{name: "draft", query: []string{"草稿"}, tool: []string{"草稿", "draft"}},
	{name: "event", query: []string{"日程"}, tool: []string{"日程", "event"}},
	{name: "reminder", query: []string{"提醒"}, tool: []string{"提醒", "reminder"}},
	{name: "summary", query: []string{"摘要"}, tool: []string{"摘要", "summary"}},
	{name: "sheet", query: []string{"工作表"}, tool: []string{"工作表", "sheet"}},
	{name: "range", query: []string{"单元格范围", "区域"}, tool: []string{"范围", "区域", "range"}},
	{name: "group", query: []string{"群聊", "群名"}, tool: []string{"群聊", "群", "group"}},
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
		specificTaskID := query.entities["task_id"] && query.products["oa"]
		matched := false
		for _, class := range toolSearchEntityClasses {
			if query.entities[class.name] && containsAnyToolSearchPhrase(toolText, class.tool) {
				matched = true
				break
			}
		}
		if matched && specificTaskID {
			multiplier += 1.00
		} else if matched {
			multiplier += 0.35
		} else if specificTaskID {
			multiplier -= 0.20
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
		if strings.Contains(text, value) {
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

type toolSearchBM25Retriever struct {
	fieldIndex map[toolSearchField]toolSearchBM25Index
	weights    ToolSearchFieldWeights
}

func newToolSearchBM25Retriever(documents map[string]toolSearchDocument, weights ToolSearchFieldWeights) *toolSearchBM25Retriever {
	fieldTerms := toolSearchFieldTerms(documents)
	fieldIndex := make(map[toolSearchField]toolSearchBM25Index, len(toolSearchFieldOrder))
	for _, field := range toolSearchFieldOrder {
		fieldIndex[field] = newToolSearchBM25Index(fieldTerms[field])
	}
	return &toolSearchBM25Retriever{fieldIndex: fieldIndex, weights: weights}
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
			hits = append(hits, LexicalHit{CanonicalPath: canonical, Score: score, MatchedFields: matchedToolSearchFields(contributions)})
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
}

func newToolSearchTFIDFRetriever(documents map[string]toolSearchDocument, weights ToolSearchFieldWeights) *toolSearchTFIDFRetriever {
	fieldTerms := toolSearchFieldTerms(documents)
	fieldIndex := make(map[toolSearchField]toolSearchTFIDFIndex, len(toolSearchFieldOrder))
	for _, field := range toolSearchFieldOrder {
		fieldIndex[field] = newToolSearchTFIDFIndex(fieldTerms[field])
	}
	return &toolSearchTFIDFRetriever{fieldIndex: fieldIndex, weights: weights}
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
			hits = append(hits, LexicalHit{CanonicalPath: canonical, Score: score, MatchedFields: matchedToolSearchFields(contributions)})
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
