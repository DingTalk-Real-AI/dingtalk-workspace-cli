// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package devdoc registers strict Shortcut adapters for DingTalk Open Platform
// documentation discovery.
package devdoc

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const devdocCompositeReason = "Unavailable after exact Shortcut plus raw isolation: the downstream interface performs semantic recall and returned nonempty neighbors for runtime-random zero-match candidates, so no guaranteed zero-match query can satisfy the public list/search proof contract."

type devdocPageEvidence struct {
	CurrentPage int
	PageSize    int
	TotalCount  int
	HasMore     bool
}

func devdocSearchResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"严格校验但未满足公开 zero-match 证明的语义文档搜索页","properties":{"count":{"type":"integer","description":"当前页文档数量"},"documents":{"type":"array","description":"当前页开放平台文档","items":{"type":"object","description":"带稳定 URL 的开放平台文档","properties":{"title":{"type":"string","description":"文档标题"},"url":{"type":"string","description":"文档稳定 URL"},"description":{"type":"string","description":"文档摘要"}},"required":["title","url"],"additionalProperties":false}},"currentPage":{"type":"integer","description":"服务端确认的当前页码"},"pageSize":{"type":"integer","description":"服务端确认的分页大小"},"totalCount":{"type":"integer","description":"服务端报告的语义召回总数"},"complete":{"type":"boolean","description":"是否到达语义召回末页；不表示 query zero-match 已证明"},"matchSemantics":{"type":"string","description":"下游搜索匹配语义","enum":["semantic"]},"zeroMatchProven":{"type":"boolean","description":"是否通过保证零匹配 query 证明公开投影；当前固定为 false"}},"required":["count","documents","currentPage","pageSize","totalCount","complete","matchSemantics","zeroMatchProven"],"additionalProperties":false}`),
	}
}

func devdocPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "page",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

var SearchDocs = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "devdoc",
	Command:       "+search-docs",
	Product:       "devdoc",
	Description:   "审计钉钉开放平台语义文档搜索结果",
	Intent:        "需要语义搜索钉钉开放平台文档且明确接受无法证明 query zero-match 时才使用；当前 Shortcut 保持 unavailable，请路由到 raw `devdoc article search`。",
	Risk:          shortcut.RiskRead,
	Hidden:        true,
	Availability:  shortcut.AvailabilityUnavailable,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "devdoc", Name: "shortcut_search_docs", CanonicalPath: "devdoc.shortcut_search_docs",
			CLIPath: "devdoc +search-docs", PrimaryCLIPath: "devdoc +search-docs",
		},
		Description: "审计钉钉开放平台语义文档搜索结果",
		Interface: &contract.InterfaceSpec{
			Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceUnavailable, Reason: devdocCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "审计钉钉开放平台语义文档搜索结果",
			UseWhen:      []string{"需要语义搜索钉钉开放平台文档且明确接受无法证明 query zero-match 时才使用；当前 Shortcut 保持 unavailable，请路由到 raw `devdoc article search`。"},
			AvoidWhen:    []string{"需要精确匹配或保证零命中时不要使用；搜索用户业务文档时使用 drive/wiki/doc"},
			Examples:     []string{`dws devdoc +search-docs --query "OAuth2 接入" --size 10 --format json`},
		},
		Parameters: []contract.ParamDecl{
			{Name: "query", Property: "keyword"},
			{Name: "page", Property: "page", InterfaceType: "number"},
			{Name: "size", Property: "size", InterfaceType: "number"},
		},
		Result:     devdocSearchResult(),
		Pagination: devdocPagination(),
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Required: true, Desc: "搜索关键词去除空白后必须非空"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码必须大于 0"},
		{Name: "size", Type: shortcut.FlagInt, Default: "10", Desc: "每页数量必须在 1 到 50 之间"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"query"}, Description: "搜索关键词去除空白后必须非空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "页码必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "每页数量必须在 1 到 50 之间"},
	},
	Tips: []string{`dws devdoc +search-docs --query "OAuth2 接入" --size 10 --format json`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if strings.TrimSpace(rt.Str("query")) == "" {
			return apperrors.NewValidation("--query 不能为空")
		}
		if rt.Int("page") < 1 {
			return apperrors.NewValidation("--page 必须大于 0")
		}
		if size := rt.Int("size"); size < 1 || size > 50 {
			return apperrors.NewValidation("--size 必须在 1 到 50 之间")
		}
		return nil
	},
	Execute: executeSearchDocs,
}

func executeSearchDocs(rt *shortcut.RuntimeContext) error {
	page, size := rt.Int("page"), rt.Int("size")
	data, err := rt.CallMCPData("devdoc", "search_open_platform_docs", map[string]any{
		"keyword": strings.TrimSpace(rt.Str("query")),
		"page":    page,
		"size":    size,
	})
	if err != nil {
		return err
	}
	documents, evidence, err := devdocProjectSearch(data, page, size)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"count":           len(documents),
		"documents":       documents,
		"currentPage":     evidence.CurrentPage,
		"pageSize":        evidence.PageSize,
		"totalCount":      evidence.TotalCount,
		"complete":        !evidence.HasMore,
		"matchSemantics":  "semantic",
		"zeroMatchProven": false,
	}
	next := ""
	if evidence.HasMore {
		next = strconv.Itoa(evidence.CurrentPage + 1)
	}
	// Strict page evidence guarantees the only two valid framework states:
	// terminal+empty-next or continuing+positive next page.
	pagination, _ := output.NewPagination(!evidence.HasMore, next)
	meta := &output.Meta{Count: output.NewCount(len(documents)), Pagination: pagination}
	return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(meta)))
}

func devdocProjectSearch(data map[string]any, requestedPage, requestedSize int) ([]map[string]any, devdocPageEvidence, error) {
	const operation = "devdoc/search_open_platform_docs"
	items, err := responsecheck.RequireObjectCollection(data, operation, "result.items")
	if err != nil {
		return nil, devdocPageEvidence{}, err
	}
	// RequireObjectCollection already proved that result is an object while
	// resolving result.items, so a second fallible shape check is unreachable.
	result, _ := data["result"].(map[string]any)
	evidence, err := devdocParsePage(result, requestedPage, requestedSize, len(items))
	if err != nil {
		return nil, devdocPageEvidence{}, err
	}
	documents := make([]map[string]any, 0, len(items))
	seenURLs := make(map[string]struct{}, len(items))
	for index, item := range items {
		title, titleOK := devdocNonEmptyString(item["title"])
		url, urlOK := devdocNonEmptyString(item["url"])
		if !titleOK || !urlOK {
			return nil, devdocPageEvidence{}, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("文档结果第 %d 项缺少非空 title 或 url", index))
		}
		if _, duplicate := seenURLs[url]; duplicate {
			return nil, devdocPageEvidence{}, responsecheck.Error(operation, "duplicate_item_identity", fmt.Sprintf("文档结果第 %d 项 URL 重复", index))
		}
		seenURLs[url] = struct{}{}
		document := map[string]any{"title": title, "url": url}
		if description, ok := devdocOptionalString(item["desc"]); ok {
			document["description"] = description
		}
		documents = append(documents, document)
	}
	return documents, evidence, nil
}

func devdocParsePage(result map[string]any, requestedPage, requestedSize, itemCount int) (devdocPageEvidence, error) {
	const operation = "devdoc/search_open_platform_docs"
	currentPage, ok := devdocInteger(result["currentPage"])
	if !ok || currentPage < 1 || currentPage != requestedPage {
		return devdocPageEvidence{}, responsecheck.Error(operation, "pagination_page_mismatch", "响应 currentPage 缺失、错型或与请求页码不一致")
	}
	pageSize, ok := devdocInteger(result["pageSize"])
	if !ok || pageSize < 1 || pageSize != requestedSize {
		return devdocPageEvidence{}, responsecheck.Error(operation, "pagination_size_mismatch", "响应 pageSize 缺失、错型或与请求分页大小不一致")
	}
	totalCount, ok := devdocInteger(result["totalCount"])
	if !ok || totalCount < 0 || totalCount < itemCount {
		return devdocPageEvidence{}, responsecheck.Error(operation, "invalid_total_count", "响应 totalCount 缺失、错型、为负数或小于当前页条数")
	}
	if itemCount > pageSize {
		return devdocPageEvidence{}, responsecheck.Error(operation, "page_size_exceeded", "当前页结果数量超过服务端 pageSize")
	}
	hasMore, ok := result["hasMore"].(bool)
	if !ok {
		return devdocPageEvidence{}, responsecheck.Error(operation, "malformed_pagination", "响应 hasMore 缺失或不是布尔值")
	}
	pageEnd := currentPage * pageSize
	if hasMore && pageEnd >= totalCount {
		return devdocPageEvidence{}, responsecheck.Error(operation, "conflicting_pagination", "hasMore=true 但 currentPage/pageSize/totalCount 已到达末页")
	}
	if !hasMore && pageEnd < totalCount {
		return devdocPageEvidence{}, responsecheck.Error(operation, "conflicting_pagination", "hasMore=false 但 totalCount 表明仍有下一页")
	}
	return devdocPageEvidence{CurrentPage: currentPage, PageSize: pageSize, TotalCount: totalCount, HasMore: hasMore}, nil
}

func devdocInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) || typed > float64(math.MaxInt) {
			return 0, false
		}
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func devdocNonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

func devdocOptionalString(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	return devdocNonEmptyString(value)
}

func init() {
	shortcut.Register(SearchDocs)
}
