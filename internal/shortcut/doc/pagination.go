// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

type docPageOptions struct {
	PageAll  bool
	PageSize int
	MaxPages int
	MaxItems int
	Cursor   string
}

func collectDocPages(
	rt *shortcut.RuntimeContext,
	tool, outputKey string,
	base map[string]any,
	project func(map[string]any) []map[string]any,
	options docPageOptions,
) (map[string]any, error) {
	if options.PageSize <= 0 {
		options.PageSize = 30
	}
	if options.MaxPages <= 0 {
		options.MaxPages = 20
	}
	if options.MaxItems <= 0 {
		options.MaxItems = 500
	}
	pageLimit := 1
	if options.PageAll {
		pageLimit = options.MaxPages
	}

	items := make([]map[string]any, 0)
	seenItems := map[string]bool{}
	seenCursors := map[string]bool{}
	cursor := strings.TrimSpace(options.Cursor)
	if cursor != "" {
		seenCursors[cursor] = true
	}
	complete := false
	truncated := false
	stopReason := ""
	nextCursor := ""
	hasMore := false
	pagesRead := 0

	for page := 1; page <= pageLimit; page++ {
		remaining := options.MaxItems - len(items)
		requestPageSize := options.PageSize
		if requestPageSize > remaining {
			requestPageSize = remaining
		}
		params := cloneMap(base)
		params["pageSize"] = requestPageSize
		if cursor != "" {
			params["pageToken"] = cursor
		}
		data, err := rt.CallMCPData(productDoc, tool, params)
		if err != nil {
			return nil, docPaginationError(tool, "page_read_failed", err, page, items, cursor)
		}
		pagesRead++
		projected := project(data)
		pageItems := make([]map[string]any, 0, len(projected))
		pageSeen := map[string]bool{}
		for _, item := range projected {
			key := pageItemKey(item)
			if key != "" && (seenItems[key] || pageSeen[key]) {
				continue
			}
			if key != "" {
				pageSeen[key] = true
			}
			pageItems = append(pageItems, item)
		}
		if len(pageItems) > remaining {
			return nil, docPaginationError(tool, "page_size_exceeded", nil, page, items, cursor)
		}
		for _, item := range pageItems {
			if key := pageItemKey(item); key != "" {
				seenItems[key] = true
			}
			items = append(items, item)
		}
		pageHasMore, hasMoreKnown, next := docPageState(data)
		nextCursor = strings.TrimSpace(next)
		switch {
		case hasMoreKnown:
			hasMore = pageHasMore
			complete = !pageHasMore
		case nextCursor != "":
			hasMore = true
		case len(projected) < requestPageSize:
			complete = true
		default:
			hasMore = true
			stopReason = "pagination_unproven"
		}
		if len(items) >= options.MaxItems && hasMore {
			truncated = true
			stopReason = "max_items"
		}
		if truncated {
			complete = false
			hasMore = true
		}

		if truncated || complete || !options.PageAll {
			break
		}
		if stopReason == "pagination_unproven" {
			return nil, docPaginationError(tool, stopReason, nil, page, items, cursor)
		}
		if nextCursor == "" {
			return nil, docPaginationError(tool, "missing_next_cursor", nil, page, items, cursor)
		}
		if seenCursors[nextCursor] {
			return nil, docPaginationError(tool, "stalled_cursor", nil, page, items, nextCursor)
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
		if page == pageLimit {
			truncated = true
			stopReason = "max_pages"
		}
	}

	result := map[string]any{
		"contractVersion": "doc.list.v1",
		"status":          "success",
		"count":           len(items),
		outputKey:         items,
		"pagesRead":       pagesRead,
		"complete":        complete,
		"truncated":       truncated,
		"hasMore":         hasMore,
		"nextCursor":      nextCursor,
		"stopReason":      stopReason,
		"failures":        []map[string]any{},
	}
	return result, nil
}

func docPaginationError(tool, reason string, cause error, page int, items []map[string]any, cursor string) error {
	message := fmt.Sprintf("%s 分页未完成，已在第 %d 页停止", tool, page)
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation("doc/"+tool),
		apperrors.WithReason("doc_pagination_"+reason),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(cause != nil),
		apperrors.WithActions("保留 nextCursor 后继续读取", "缩小查询范围后重试"),
		apperrors.WithDetails(map[string]any{
			"contractVersion": "doc.list.v1",
			"status":          "partial_success",
			"complete":        false,
			"reason":          reason,
			"page":            page,
			"nextCursor":      cursor,
			"count":           len(items),
			"items":           items,
		}),
		apperrors.WithCause(cause),
	)
}

func cloneMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source)+2)
	for key, value := range source {
		out[key] = value
	}
	return out
}

func pageItemKey(item map[string]any) string {
	for _, key := range []string{"nodeId", "id", "url"} {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return key + ":" + strings.TrimSpace(value)
		}
	}
	return ""
}

func docPageState(data map[string]any) (bool, bool, string) {
	var hasMore bool
	var known bool
	var next string
	var walk func(map[string]any)
	walk = func(value map[string]any) {
		if !known {
			for _, key := range []string{"hasMore", "has_more"} {
				if flag, ok := value[key].(bool); ok {
					hasMore, known = flag, true
					break
				}
			}
		}
		if next == "" {
			if value, ok := docFirst(value, "nextPageToken", "nextCursor", "next_page_token"); ok {
				next, _ = value.(string)
			}
		}
		for _, key := range []string{"result", "data", "page", "pagination"} {
			if nested, ok := value[key].(map[string]any); ok {
				walk(nested)
			}
		}
	}
	walk(data)
	return hasMore, known, next
}
