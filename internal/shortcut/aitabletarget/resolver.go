// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

// Package aitabletarget provides deterministic AI Table URL and name
// resolution shared by read and write shortcuts. It never guesses among
// multiple candidates and treats an incomplete/unknown response as an error.
package aitabletarget

import (
	"fmt"
	"net/url"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	maxResolutionPages = 50
	aliDocsHost        = "alidocs.dingtalk.com"
)

// Reader is the read-only transport required for name resolution.
type Reader interface {
	CallMCPData(product, tool string, params map[string]any) (map[string]any, error)
}

// Target is the normalized identity carried by an AI Table URL.
type Target struct {
	SchemaVersion string `json:"schemaVersion"`
	Source        string `json:"source"`
	Kind          string `json:"kind"`
	BaseID        string `json:"baseId"`
	TableID       string `json:"tableId,omitempty"`
	ViewID        string `json:"viewId,omitempty"`
	RecordID      string `json:"recordId,omitempty"`
}

// Candidate is a stable ID/name pair returned by name resolution.
type Candidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Resolution is returned only after exactly one candidate is selected.
type Resolution struct {
	Status     string    `json:"status"`
	EntityType string    `json:"entityType"`
	Query      string    `json:"query"`
	MatchType  string    `json:"matchType"`
	Selected   Candidate `json:"selected"`
}

// ParseURL parses the documented DingTalk AI Table URL forms:
//
//	https://alidocs.dingtalk.com/i/nodes/{baseId}
//	https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}%26viewId%3D{viewId}
func ParseURL(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return Target{}, invalidURL(raw, fmt.Sprintf("URL 解析失败: %v", err))
	}
	if !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Hostname(), aliDocsHost) || parsed.User != nil {
		return Target{}, invalidURL(raw, "仅接受 https://alidocs.dingtalk.com 的无凭据 URL")
	}

	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 3 || segments[0] != "i" || segments[1] != "nodes" {
		return Target{}, invalidURL(raw, "路径必须是 /i/nodes/{baseId}")
	}
	baseID, err := url.PathUnescape(segments[2])
	if err != nil || !validID(baseID) {
		return Target{}, invalidURL(raw, "URL 路径中的 baseId 无效")
	}

	values := parsed.Query()
	nested, err := parseIframeQuery(values.Get("iframeQuery"))
	if err != nil {
		return Target{}, invalidURL(raw, err.Error())
	}
	for key, nestedValues := range nested {
		for _, value := range nestedValues {
			values.Add(key, value)
		}
	}

	tableID, err := uniqueQueryID(values, "sheetId", "tableId")
	if err != nil {
		return Target{}, invalidURL(raw, err.Error())
	}
	viewID, err := uniqueQueryID(values, "viewId")
	if err != nil {
		return Target{}, invalidURL(raw, err.Error())
	}
	recordID, err := uniqueQueryID(values, "recordId")
	if err != nil {
		return Target{}, invalidURL(raw, err.Error())
	}
	if viewID != "" && tableID == "" {
		return Target{}, invalidURL(raw, "viewId 缺少所属 sheetId/tableId")
	}
	if recordID != "" && tableID == "" {
		return Target{}, invalidURL(raw, "recordId 缺少所属 sheetId/tableId")
	}

	kind := "base"
	if tableID != "" {
		kind = "table"
	}
	if viewID != "" {
		kind = "view"
	}
	if recordID != "" {
		kind = "record"
	}
	return Target{
		SchemaVersion: "aitable.target.v1",
		Source:        "url",
		Kind:          kind,
		BaseID:        strings.TrimSpace(baseID),
		TableID:       tableID,
		ViewID:        viewID,
		RecordID:      recordID,
	}, nil
}

func parseIframeQuery(raw string) (url.Values, error) {
	if strings.TrimSpace(raw) == "" {
		return url.Values{}, nil
	}
	value := raw
	for attempt := 0; attempt < 2 && !strings.Contains(value, "="); attempt++ {
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return nil, fmt.Errorf("iframeQuery 解码失败: %w", err)
		}
		if decoded == value {
			break
		}
		value = decoded
	}
	values, err := url.ParseQuery(value)
	if err != nil {
		return nil, fmt.Errorf("iframeQuery 不是合法查询串: %w", err)
	}
	return values, nil
}

func uniqueQueryID(values url.Values, keys ...string) (string, error) {
	unique := map[string]struct{}{}
	for _, key := range keys {
		for _, raw := range values[key] {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if !validID(value) {
				return "", fmt.Errorf("%s 包含无效 ID", key)
			}
			unique[value] = struct{}{}
		}
	}
	if len(unique) > 1 {
		return "", fmt.Errorf("%s 出现互相冲突的多个值", strings.Join(keys, "/"))
	}
	for value := range unique {
		return value, nil
	}
	return "", nil
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "/?#") {
		return false
	}
	for _, r := range value {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// ResolveBaseName resolves one Base by exact name. A substring match is used
// only when allowFuzzy is true. Zero/multiple candidates and incomplete pages
// are structured non-zero errors.
func ResolveBaseName(reader Reader, name string, allowFuzzy bool) (Resolution, error) {
	query := strings.TrimSpace(name)
	if query == "" {
		return Resolution{}, apperrors.NewValidation("Base 名称不能为空")
	}
	all := make([]Candidate, 0)
	cursor := ""
	seen := map[string]bool{}
	for page := 1; page <= maxResolutionPages; page++ {
		params := map[string]any{"query": query}
		if cursor != "" {
			params["cursor"] = cursor
		}
		data, err := reader.CallMCPData("aitable", "search_bases", params)
		if err != nil {
			return Resolution{}, err
		}
		items, found, listErr := findObjectList(data, "bases", "items", "list", "result", "records")
		if listErr != nil {
			return Resolution{}, invalidResponse("base", query, listErr.Error())
		}
		if !found {
			return Resolution{}, invalidResponse("base", query, "search_bases 响应缺少候选列表")
		}
		for index, item := range items {
			id := firstString(item, "baseId", "base_id", "id")
			candidateName := firstString(item, "baseName", "name", "title")
			if id == "" || candidateName == "" {
				return Resolution{}, invalidResponse("base", query,
					fmt.Sprintf("search_bases 候选 %d 缺少 baseId 或名称", index))
			}
			all = append(all, Candidate{ID: id, Name: candidateName})
		}
		next, hasMore, hasMoreKnown := pagination(data)
		if next == "" {
			if hasMoreKnown && hasMore {
				return Resolution{}, incomplete("base", query, all, "search_bases 声明有后续页但没有 cursor")
			}
			return selectCandidate("base", query, dedupe(all), allowFuzzy)
		}
		if seen[next] || next == cursor {
			return Resolution{}, incomplete("base", query, all, "search_bases cursor 停滞或成环")
		}
		seen[next] = true
		cursor = next
	}
	return Resolution{}, incomplete("base", query, all, fmt.Sprintf("达到 %d 页安全上限", maxResolutionPages))
}

// ResolveTableName resolves one table within a known Base using get_tables.
func ResolveTableName(reader Reader, baseID, name string, allowFuzzy bool) (Resolution, error) {
	baseID = strings.TrimSpace(baseID)
	query := strings.TrimSpace(name)
	if !validID(baseID) || query == "" {
		return Resolution{}, apperrors.NewValidation("解析数据表名称需要合法 baseId 和非空名称")
	}
	data, err := reader.CallMCPData("aitable", "get_tables", map[string]any{"baseId": baseID})
	if err != nil {
		return Resolution{}, err
	}
	items, found, listErr := findObjectList(data, "tables", "items", "list", "result", "records")
	if listErr != nil {
		return Resolution{}, invalidResponse("table", query, listErr.Error())
	}
	if !found {
		return Resolution{}, invalidResponse("table", query, "get_tables 响应缺少 tables 列表")
	}
	candidates := make([]Candidate, 0, len(items))
	for index, item := range items {
		id := firstString(item, "tableId", "table_id", "id")
		candidateName := firstString(item, "tableName", "name", "title")
		if id == "" || candidateName == "" {
			return Resolution{}, invalidResponse("table", query,
				fmt.Sprintf("get_tables 候选 %d 缺少 tableId 或名称", index))
		}
		candidates = append(candidates, Candidate{ID: id, Name: candidateName})
	}
	return selectCandidate("table", query, dedupe(candidates), allowFuzzy)
}

func selectCandidate(entityType, query string, candidates []Candidate, allowFuzzy bool) (Resolution, error) {
	exact := make([]Candidate, 0)
	for _, candidate := range candidates {
		if strings.EqualFold(strings.TrimSpace(candidate.Name), query) {
			exact = append(exact, candidate)
		}
	}
	selected := exact
	matchType := "exact"
	if len(selected) == 0 && allowFuzzy {
		matchType = "fuzzy"
		needle := strings.ToLower(query)
		for _, candidate := range candidates {
			if strings.Contains(strings.ToLower(candidate.Name), needle) {
				selected = append(selected, candidate)
			}
		}
	}
	if len(selected) == 0 {
		return Resolution{}, resolutionError("not_found", entityType, query, candidates)
	}
	if len(selected) > 1 {
		return Resolution{}, resolutionError("ambiguous", entityType, query, selected)
	}
	return Resolution{
		Status: "resolved", EntityType: entityType, Query: query,
		MatchType: matchType, Selected: selected[0],
	}, nil
}

func findObjectList(data map[string]any, keys ...string) ([]map[string]any, bool, error) {
	if data == nil {
		return nil, false, nil
	}
	invalid := make([]string, 0)
	for _, key := range keys {
		value, exists := data[key]
		if !exists {
			continue
		}
		if list, ok := value.([]any); ok {
			out := make([]map[string]any, 0, len(list))
			for index, row := range list {
				object, ok := row.(map[string]any)
				if !ok {
					return nil, false, fmt.Errorf("候选列表 %s[%d] 必须是对象，got %T", key, index, row)
				}
				out = append(out, object)
			}
			return out, true, nil
		}
		if object, ok := value.(map[string]any); ok {
			if list, found, err := findObjectList(object, keys...); found || err != nil {
				return list, found, err
			}
			continue
		}
		invalid = append(invalid, fmt.Sprintf("%s=%T", key, value))
	}
	for _, envelope := range []string{"data", "result", "response"} {
		if object, ok := data[envelope].(map[string]any); ok {
			if list, found, err := findObjectList(object, keys...); found || err != nil {
				return list, found, err
			}
		}
	}
	if len(invalid) > 0 {
		return nil, false, fmt.Errorf("候选列表字段类型无效: %v", invalid)
	}
	return nil, false, nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pagination(data map[string]any) (cursor string, hasMore bool, hasMoreKnown bool) {
	if data == nil {
		return "", false, false
	}
	for _, key := range []string{"nextCursor", "cursor"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			cursor = strings.TrimSpace(value)
			break
		}
	}
	if value, ok := data["hasMore"].(bool); ok {
		hasMore, hasMoreKnown = value, true
	}
	for _, key := range []string{"data", "result", "page", "pagination", "meta"} {
		if nested, ok := data[key].(map[string]any); ok {
			next, more, known := pagination(nested)
			if cursor == "" {
				cursor = next
			}
			if !hasMoreKnown && known {
				hasMore, hasMoreKnown = more, true
			}
		}
	}
	return cursor, hasMore, hasMoreKnown
}

func dedupe(values []Candidate) []Candidate {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(values))
	for _, value := range values {
		if value.ID == "" || seen[value.ID] {
			continue
		}
		seen[value.ID] = true
		out = append(out, value)
	}
	return out
}

func invalidURL(raw, reason string) error {
	return apperrors.NewValidation("无法解析 AI 表格 URL",
		apperrors.WithReason("invalid_aitable_url"),
		apperrors.WithOrigin("client"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithHint(reason),
		apperrors.WithDetails(map[string]any{"url": raw}),
	)
}

func resolutionError(status, entityType, query string, candidates []Candidate) error {
	return apperrors.NewValidation(fmt.Sprintf("%s target %s: %q", entityType, status, query),
		apperrors.WithReason("target_"+status),
		apperrors.WithOrigin("client"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithDetails(map[string]any{
			"status": status, "entityType": entityType, "query": query,
			"candidates": candidates,
		}),
	)
}

func incomplete(entityType, query string, candidates []Candidate, cause string) error {
	return apperrors.NewAPI("目标解析候选集不完整，不能安全选择",
		apperrors.WithReason("target_incomplete"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(true),
		apperrors.WithHint(cause),
		apperrors.WithDetails(map[string]any{
			"status": "incomplete", "entityType": entityType, "query": query,
			"candidates": dedupe(candidates),
		}),
	)
}

func invalidResponse(entityType, query, cause string) error {
	return apperrors.NewAPI("目标解析接口返回未知结构，不能把它当作未找到",
		apperrors.WithReason("target_invalid_response"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithHint(cause),
		apperrors.WithDetails(map[string]any{"entityType": entityType, "query": query}),
	)
}
