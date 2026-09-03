// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitabletarget

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilectx"
)

const entitySearchPageSize = 50

var simpleHighlightTag = regexp.MustCompile(`(?i)</?(?:em|b|strong|mark)(?:\s[^>]*)?>`)

// EntityType is the stable search_entities type accepted by AITable MCP.
type EntityType string

const (
	EntityPerson     EntityType = "PERSON"
	EntityDepartment EntityType = "DEPARTMENT"
	EntityGroup      EntityType = "GROUP"
)

// EntityReference is the external identity that can be passed to AITable filters.
// Only one entity type is populated; MCP may return compatible aliases for it.
type EntityReference struct {
	UserID        string `json:"userId,omitempty"`
	CorpID        string `json:"corpId,omitempty"`
	UserRef       string `json:"userRef,omitempty"`
	DepartmentID  string `json:"departmentId,omitempty"`
	DepartmentKey string `json:"departmentKey,omitempty"`
	CID           string `json:"cid,omitempty"`
}

// EntityCandidate is one non-sensitive search candidate used for disambiguation.
type EntityCandidate struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Reference   EntityReference `json:"reference"`
}

// EntitySearchResult contains a complete, de-duplicated candidate set.
type EntitySearchResult struct {
	EntityType EntityType        `json:"entityType"`
	Keyword    string            `json:"keyword"`
	Candidates []EntityCandidate `json:"candidates"`
	Profile    string            `json:"profile,omitempty"`
}

// EntityResolution is returned only for a unique exact display-name match.
type EntityResolution struct {
	Status     string          `json:"status"`
	EntityType EntityType      `json:"entityType"`
	Query      string          `json:"query"`
	MatchType  string          `json:"matchType"`
	Selected   EntityCandidate `json:"selected"`
	Profile    string          `json:"profile,omitempty"`
}

// ParseEntityType validates the public PERSON/DEPARTMENT/GROUP enum.
func ParseEntityType(raw string) (EntityType, error) {
	typed := EntityType(strings.ToUpper(strings.TrimSpace(raw)))
	switch typed {
	case EntityPerson, EntityDepartment, EntityGroup:
		return typed, nil
	default:
		return "", apperrors.NewValidation("entity-type 必须是 PERSON、DEPARTMENT 或 GROUP",
			apperrors.WithReason("invalid_entity_reference"),
			apperrors.WithExecutionStarted(false))
	}
}

// SearchEntities reads every search_entities page before returning candidates.
// A declared continuation without a usable cursor fails closed, because a
// partial candidate set cannot prove that a display name is unique.
func SearchEntities(reader Reader, entityType EntityType, keyword string) (EntitySearchResult, error) {
	query := strings.TrimSpace(keyword)
	if query == "" || aitableprotocol.UTF16Length(query) > 100 {
		return EntitySearchResult{}, apperrors.NewValidation("实体搜索关键词不能为空且不能超过 100 个字符",
			apperrors.WithReason("invalid_entity_reference"),
			apperrors.WithExecutionStarted(false))
	}
	if _, err := ParseEntityType(string(entityType)); err != nil {
		return EntitySearchResult{}, err
	}

	all := make([]EntityCandidate, 0)
	cursor := ""
	seenCursors := map[string]bool{}
	for page := 1; page <= maxResolutionPages; page++ {
		params := map[string]any{
			"entityType": string(entityType),
			"keyword":    query,
			"limit":      entitySearchPageSize,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		data, err := reader.CallMCPData("aitable", "search_entities", params)
		if err != nil {
			if apperrors.IsMCPToolNotFound(err) {
				return EntitySearchResult{}, entityCapabilityUnavailable(entityType, query, err.Error())
			}
			return EntitySearchResult{}, err
		}
		items, found, listErr := findObjectList(data, "candidates")
		if listErr != nil {
			return EntitySearchResult{}, entityInvalidResponse(entityType, query, listErr.Error())
		}
		if !found {
			return EntitySearchResult{}, entityInvalidResponse(entityType, query,
				"search_entities 响应缺少 candidates 列表")
		}
		for index, item := range items {
			candidate, err := parseEntityCandidate(entityType, item)
			if err != nil {
				return EntitySearchResult{}, entityInvalidResponse(entityType, query,
					fmt.Sprintf("candidate[%d]: %v", index, err))
			}
			all = append(all, candidate)
		}
		next, hasMore, hasMoreKnown := Pagination(data)
		if !hasMoreKnown {
			return EntitySearchResult{}, entityResolutionIncomplete(
				entityType, query, all, "search_entities 响应缺少 hasMore")
		}
		if !hasMore {
			if next != "" {
				return EntitySearchResult{}, entityResolutionIncomplete(
					entityType, query, all, "search_entities 返回 hasMore=false 但仍带有 nextCursor")
			}
			return completeEntitySearchResult(entityType, query, all), nil
		}
		if next == "" {
			return EntitySearchResult{}, entityResolutionIncomplete(
				entityType, query, all, "search_entities 声明有后续页但没有 nextCursor")
		}
		if seenCursors[next] || next == cursor {
			return EntitySearchResult{}, entityResolutionIncomplete(
				entityType, query, all, "search_entities cursor 停滞或成环")
		}
		seenCursors[next] = true
		cursor = next
	}
	return EntitySearchResult{}, entityResolutionIncomplete(
		entityType, query, all, fmt.Sprintf("达到 %d 页安全上限", maxResolutionPages))
}

// ResolveEntity selects only one exact normalized name match. A single fuzzy
// candidate still returns not_found together with candidates, never success.
func ResolveEntity(reader Reader, entityType EntityType, query string) (EntityResolution, error) {
	searched, err := SearchEntities(reader, entityType, query)
	if err != nil {
		return EntityResolution{}, err
	}
	normalizedQuery := normalizeEntityDisplayName(query)
	exact := make([]EntityCandidate, 0)
	for _, candidate := range searched.Candidates {
		if strings.EqualFold(normalizeEntityDisplayName(candidate.Name), normalizedQuery) {
			exact = append(exact, candidate)
		}
	}
	if len(exact) == 0 {
		return EntityResolution{}, entityResolutionError(
			"resolution_not_found", entityType, strings.TrimSpace(query), searched.Candidates)
	}
	if len(exact) > 1 {
		return EntityResolution{}, entityResolutionError(
			"resolution_ambiguous", entityType, strings.TrimSpace(query), exact)
	}
	return EntityResolution{
		Status: "resolved", EntityType: entityType, Query: strings.TrimSpace(query),
		MatchType: "exact", Selected: exact[0], Profile: profilectx.Get(),
	}, nil
}

func parseEntityCandidate(entityType EntityType, item map[string]any) (EntityCandidate, error) {
	name := normalizeEntityDisplayName(firstString(item, "name"))
	if name == "" {
		return EntityCandidate{}, fmt.Errorf("name is required")
	}
	candidate := EntityCandidate{
		Name:        name,
		Description: strings.TrimSpace(firstString(item, "description")),
	}
	switch entityType {
	case EntityPerson:
		person := nestedObject(item, "person")
		candidate.Reference.UserID = firstString(person, "userId")
		candidate.Reference.CorpID = firstString(person, "corpId")
		candidate.Reference.UserRef = firstString(person, "userRef")
		if (candidate.Reference.UserID == "") != (candidate.Reference.CorpID == "") {
			return EntityCandidate{}, fmt.Errorf("person must provide userId and corpId together")
		}
		if candidate.Reference.UserID == "" && candidate.Reference.UserRef == "" {
			return EntityCandidate{}, fmt.Errorf("person stable identity is required")
		}
	case EntityDepartment:
		department := nestedObject(item, "department")
		candidate.Reference.DepartmentID = firstString(department, "departmentId")
		candidate.Reference.DepartmentKey = firstString(department, "departmentKey")
		if candidate.Reference.DepartmentID == "" && candidate.Reference.DepartmentKey == "" {
			return EntityCandidate{}, fmt.Errorf("departmentId or departmentKey is required")
		}
	case EntityGroup:
		group := nestedObject(item, "group")
		candidate.Reference.CID = firstString(group, "cid")
		if candidate.Reference.CID == "" {
			return EntityCandidate{}, fmt.Errorf("group cid is required")
		}
	}
	return candidate, nil
}

func nestedObject(item map[string]any, key string) map[string]any {
	if object, ok := item[key].(map[string]any); ok {
		return object
	}
	return map[string]any{}
}

func completeEntitySearchResult(entityType EntityType, keyword string, candidates []EntityCandidate) EntitySearchResult {
	return EntitySearchResult{
		EntityType: entityType,
		Keyword:    keyword,
		Candidates: dedupeEntityCandidates(candidates, keyword),
		Profile:    profilectx.Get(),
	}
}

func dedupeEntityCandidates(candidates []EntityCandidate, preferredName string) []EntityCandidate {
	preferred := normalizeEntityDisplayName(preferredName)
	seen := map[string]int{}
	out := make([]EntityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := entityReferenceKey(candidate.Reference)
		if key == "" {
			continue
		}
		if index, exists := seen[key]; exists {
			currentExact := strings.EqualFold(normalizeEntityDisplayName(out[index].Name), preferred)
			candidateExact := strings.EqualFold(normalizeEntityDisplayName(candidate.Name), preferred)
			if preferred != "" && candidateExact && !currentExact {
				out[index] = candidate
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, candidate)
	}
	return out
}

func entityReferenceKey(reference EntityReference) string {
	switch {
	case reference.UserID != "" && reference.CorpID != "":
		return "person:user:" + reference.CorpID + ":" + reference.UserID
	case reference.UserRef != "":
		return "person:ref:" + reference.UserRef
	case reference.DepartmentID != "":
		return "department:id:" + reference.DepartmentID
	case reference.DepartmentKey != "":
		return "department:key:" + reference.DepartmentKey
	case reference.CID != "":
		return "group:cid:" + reference.CID
	default:
		return ""
	}
}

func normalizeEntityDisplayName(value string) string {
	return strings.TrimSpace(simpleHighlightTag.ReplaceAllString(value, ""))
}

func entityResolutionError(
	reason string,
	entityType EntityType,
	query string,
	candidates []EntityCandidate,
) error {
	return apperrors.NewValidation("实体名称无法唯一精确解析",
		apperrors.WithReason(reason),
		apperrors.WithOrigin("client"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithDetails(map[string]any{
			"entityType": entityType, "query": query, "candidates": candidates,
		}),
	)
}

func entityResolutionIncomplete(
	entityType EntityType,
	query string,
	candidates []EntityCandidate,
	cause string,
) error {
	return apperrors.NewAPI("实体候选集不完整，不能安全选择",
		apperrors.WithReason("resolution_incomplete"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(true),
		apperrors.WithHint(cause),
		apperrors.WithDetails(map[string]any{
			"entityType": entityType, "query": query,
			"candidates": dedupeEntityCandidates(candidates, query),
		}),
	)
}

func entityInvalidResponse(entityType EntityType, query, cause string) error {
	return apperrors.NewAPI("实体搜索返回未知结构，不能安全选择",
		apperrors.WithReason("resolution_incomplete"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithHint(cause),
		apperrors.WithDetails(map[string]any{"entityType": entityType, "query": query}),
	)
}

func entityCapabilityUnavailable(entityType EntityType, query, cause string) error {
	return apperrors.NewAPI("当前 AITable MCP 未提供 search_entities 能力",
		apperrors.WithReason("capability_unavailable"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithHint(cause),
		apperrors.WithDetails(map[string]any{"entityType": entityType, "query": query}),
	)
}
