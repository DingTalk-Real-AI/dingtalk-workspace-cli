// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package docresolver resolves document IDs, URLs, and natural titles to one
// typed document target before a shortcut performs a business operation.
package docresolver

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	searchPageSize  = 30
	searchPageLimit = 40
)

// Reader is the read-only RuntimeContext surface needed by the resolver.
type Reader interface {
	CallMCPData(product, tool string, params map[string]any) (map[string]any, error)
}

// Candidate is a credential-free document search candidate.
type Candidate struct {
	CanonicalID  string `json:"canonicalId"`
	Name         string `json:"name,omitempty"`
	Product      string `json:"product"`
	ResourceType string `json:"resourceType,omitempty"`
	URL          string `json:"canonicalUrl,omitempty"`
	ContainerID  string `json:"containerId,omitempty"`
}

// Resolution is the stable result consumed by document shortcuts.
type Resolution struct {
	ContractVersion string      `json:"contractVersion"`
	Status          string      `json:"status"`
	Query           string      `json:"query,omitempty"`
	MatchedBy       string      `json:"matchedBy"`
	Complete        bool        `json:"complete"`
	Selected        Candidate   `json:"selected"`
	Candidates      []Candidate `json:"candidates"`
}

// Resolve accepts exactly one stable node/URL or natural title. Natural title
// resolution exhausts pagination before selecting a unique candidate.
func Resolve(rt Reader, node, query string) (Resolution, error) {
	node = strings.TrimSpace(node)
	query = strings.TrimSpace(query)
	if (node == "") == (query == "") {
		return Resolution{}, apperrors.NewValidation(
			"文档目标必须且只能提供一个：--node 或 --query",
			apperrors.WithReason("document_target_invalid"),
			apperrors.WithFailureStage("target_resolution"),
			apperrors.WithExecutionStarted(false),
		)
	}
	if node != "" {
		return Resolution{
			ContractVersion: "doc.target.v1",
			Status:          "resolved",
			MatchedBy:       "stable_id_or_url",
			Complete:        true,
			Selected: Candidate{
				CanonicalID: node,
				Product:     "doc",
			},
			Candidates: []Candidate{},
		}, nil
	}

	candidates, err := searchAll(rt, query)
	if err != nil {
		return Resolution{}, err
	}
	selected, matchedBy := selectCandidates(candidates, query)
	if len(selected) == 0 {
		return Resolution{}, resolutionError("not_found", query, candidates, "没有找到匹配的在线文字文档")
	}
	if len(selected) > 1 {
		return Resolution{}, resolutionError("ambiguous", query, selected, "找到多个匹配文档，请使用 nodeId 或 URL 消歧")
	}
	if typ := normalizeType(selected[0].ResourceType); typ != "" && typ != "adoc" && typ != "document" && typ != "doc" {
		return Resolution{}, apperrors.NewValidation(
			fmt.Sprintf("目标 %q 的资源类型为 %s，不是在线文字文档", selected[0].Name, selected[0].ResourceType),
			apperrors.WithReason("document_target_type_mismatch"),
			apperrors.WithFailureStage("target_resolution"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithDetails(map[string]any{"status": "type_mismatch", "target": selected[0]}),
		)
	}
	return Resolution{
		ContractVersion: "doc.target.v1",
		Status:          "resolved",
		Query:           query,
		MatchedBy:       matchedBy,
		Complete:        true,
		Selected:        selected[0],
		Candidates:      []Candidate{},
	}, nil
}

func searchAll(rt Reader, query string) ([]Candidate, error) {
	cursor := ""
	seenCursors := map[string]bool{}
	all := make([]Candidate, 0)
	for page := 1; page <= searchPageLimit; page++ {
		params := map[string]any{"keyword": query, "pageSize": searchPageSize}
		if cursor != "" {
			params["pageToken"] = cursor
		}
		data, err := rt.CallMCPData("doc", "search_documents", params)
		if err != nil {
			return nil, err
		}
		rows := extractRows(data)
		all = append(all, extractCandidates(rows)...)
		hasMore, hasMoreKnown, next := extractPage(data)
		switch {
		case hasMoreKnown && !hasMore:
			return dedupe(all), nil
		case hasMoreKnown && hasMore:
			// next is validated below.
		case next != "":
			// Older responses may only publish the continuation.
		case len(rows) < searchPageSize:
			return dedupe(all), nil
		default:
			return nil, incompleteError(query, all, "搜索返回满页结果但没有结束标记或下一页游标")
		}
		next = strings.TrimSpace(next)
		if next == "" {
			return nil, incompleteError(query, all, "搜索声明仍有更多结果但缺少下一页游标")
		}
		if next == cursor || seenCursors[next] {
			return nil, incompleteError(query, all, fmt.Sprintf("搜索分页游标停滞在 %q", next))
		}
		seenCursors[next] = true
		cursor = next
	}
	return nil, incompleteError(query, all, fmt.Sprintf("搜索超过最大页数 %d", searchPageLimit))
}

func selectCandidates(candidates []Candidate, query string) ([]Candidate, string) {
	exact := make([]Candidate, 0)
	for _, candidate := range candidates {
		if strings.EqualFold(strings.TrimSpace(candidate.Name), query) {
			exact = append(exact, candidate)
		}
	}
	if len(exact) > 0 {
		return exact, "exact_title"
	}
	return candidates, "search_candidate"
}

func extractCandidates(rows []any) []Candidate {
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			continue
		}
		candidate := Candidate{
			CanonicalID:  firstString(item, "nodeId", "node_id", "docId", "doc_id", "id"),
			Name:         firstString(item, "name", "title", "docName", "fileName"),
			Product:      "doc",
			ResourceType: firstString(item, "docType", "doc_type", "extension", "fileType", "type"),
			URL:          firstString(item, "url", "docUrl", "nodeUrl", "webUrl"),
			ContainerID:  firstString(item, "workspaceId", "spaceId", "folderId", "parentId"),
		}
		if candidate.CanonicalID != "" {
			out = append(out, candidate)
		}
	}
	return out
}

func dedupe(in []Candidate) []Candidate {
	seen := make(map[string]bool, len(in))
	out := make([]Candidate, 0, len(in))
	for _, candidate := range in {
		if candidate.CanonicalID == "" || seen[candidate.CanonicalID] {
			continue
		}
		seen[candidate.CanonicalID] = true
		out = append(out, candidate)
	}
	return out
}

func extractRows(data map[string]any) []any {
	for _, key := range []string{"documents", "nodes", "items", "list", "records", "result", "data"} {
		value, ok := data[key]
		if !ok {
			continue
		}
		if rows, ok := value.([]any); ok {
			return rows
		}
		if nested, ok := value.(map[string]any); ok {
			if rows := extractRows(nested); rows != nil {
				return rows
			}
		}
	}
	return []any{}
}

func extractPage(data map[string]any) (bool, bool, string) {
	var hasMore bool
	var hasMoreKnown bool
	var next string
	var walk func(map[string]any)
	walk = func(value map[string]any) {
		if !hasMoreKnown {
			for _, key := range []string{"hasMore", "has_more"} {
				if flag, ok := value[key].(bool); ok {
					hasMore, hasMoreKnown = flag, true
					break
				}
			}
		}
		if next == "" {
			next = firstString(value, "nextPageToken", "nextCursor", "next_page_token")
		}
		for _, key := range []string{"result", "data", "page", "pagination"} {
			if nested, ok := value[key].(map[string]any); ok {
				walk(nested)
			}
		}
	}
	walk(data)
	return hasMore, hasMoreKnown, next
}

func firstString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func normalizeType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, ".")))
}

func resolutionError(status, query string, candidates []Candidate, message string) error {
	return apperrors.NewValidation(
		message,
		apperrors.WithReason("document_target_"+status),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(map[string]any{
			"contractVersion": "doc.target.v1",
			"status":          status,
			"query":           query,
			"complete":        true,
			"candidates":      candidates,
		}),
	)
}

func incompleteError(query string, candidates []Candidate, cause string) error {
	return apperrors.NewAPI(
		"无法证明文档搜索候选完整，已停止目标解析",
		apperrors.WithReason("document_target_incomplete"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(true),
		apperrors.WithActions("稍后以相同条件重试", "使用已知 nodeId 或 URL 直接指定目标"),
		apperrors.WithDetails(map[string]any{
			"contractVersion": "doc.target.v1",
			"status":          "incomplete",
			"query":           query,
			"complete":        false,
			"cause":           cause,
			"candidates":      dedupe(candidates),
		}),
	)
}
