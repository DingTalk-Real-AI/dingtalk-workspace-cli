// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package drive

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

type drivePageOptions struct {
	PageAll        bool
	PageSize       int
	MaxPages       int
	MaxItems       int
	Cursor         string
	Server         string
	Tool           string
	OutputKey      string
	PageSizeParam  string
	CursorParam    string
	CollectionKeys []string
	Project        func([]any) []map[string]any
}

type drivePageResult struct {
	Business          map[string]any
	EndpointExhausted bool
	NextToken         string
	Pages             int
	Items             int
}

type drivePaginationErrorCursors struct {
	Request string
	Retry   string
}

func drivePageSize(rt *shortcut.RuntimeContext, flag string, fallback int) int {
	if rt.Changed(flag) {
		return rt.Int(flag)
	}
	return fallback
}

func validateDriveAutoPagination(rt *shortcut.RuntimeContext) error {
	if !rt.Bool("page-all") {
		if rt.Changed("max-pages") || rt.Changed("max-items") {
			return fmt.Errorf("--max-pages/--max-items 仅与 --page-all 一起使用")
		}
		return nil
	}
	if rt.Int("max-pages") <= 0 {
		return fmt.Errorf("--max-pages 必须大于 0")
	}
	if rt.Int("max-items") <= 0 {
		return fmt.Errorf("--max-items 必须大于 0")
	}
	return nil
}

func driveAutoPaginationConstraints() []shortcut.Constraint {
	return []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"page-all", "max-pages", "max-items"},
		Description: "--max-pages/--max-items 仅与 --page-all 一起使用，且必须大于 0",
	}}
}

func collectDrivePages(rt *shortcut.RuntimeContext, base map[string]any, options drivePageOptions) (drivePageResult, error) {
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
	pagesRead := 0

	for page := 1; ; page++ {
		remaining := options.MaxItems - len(items)
		requestPageSize := options.PageSize
		if requestPageSize > remaining {
			requestPageSize = remaining
		}
		params := cloneDriveMap(base)
		params[options.PageSizeParam] = requestPageSize
		if cursor != "" {
			params[options.CursorParam] = cursor
		}
		data, err := rt.CallMCPData(options.Server, options.Tool, params)
		if err != nil {
			return drivePageResult{}, drivePaginationError(options, "page_read_failed", err, page, items, drivePaginationErrorCursors{
				Request: cursor,
				Retry:   cursor,
			})
		}
		pagesRead++
		rawItems, pageState, err := requireDriveCollection(data, options.Server+"/"+options.Tool, options.CollectionKeys...)
		if err != nil {
			return drivePageResult{}, drivePaginationError(options, "invalid_page", err, page, items, drivePaginationErrorCursors{Request: cursor})
		}
		projected := options.Project(rawItems)
		pageItems := make([]map[string]any, 0, len(projected))
		pageSeen := map[string]bool{}
		for _, item := range projected {
			key := drivePageItemKey(item)
			if key != "" && (seenItems[key] || pageSeen[key]) {
				continue
			}
			if key != "" {
				pageSeen[key] = true
			}
			pageItems = append(pageItems, item)
		}
		if len(pageItems) > remaining {
			return drivePageResult{}, drivePaginationError(options, "page_size_exceeded", nil, page, items, drivePaginationErrorCursors{Request: cursor})
		}
		for _, item := range pageItems {
			if key := drivePageItemKey(item); key != "" {
				seenItems[key] = true
			}
			items = append(items, item)
		}

		pageHasMore, hasMoreKnown, next := drivePageState(pageState)
		nextCursor := strings.TrimSpace(next)
		if hasMoreKnown && !pageHasMore && nextCursor != "" {
			return drivePageResult{}, drivePaginationError(options, "inconsistent_state", nil, page, items, drivePaginationErrorCursors{Request: cursor})
		}

		endpointExhausted := false
		switch {
		case hasMoreKnown && !pageHasMore:
			endpointExhausted = true
		case hasMoreKnown && pageHasMore && nextCursor == "":
			return drivePageResult{}, drivePaginationError(options, "missing_next_cursor", nil, page, items, drivePaginationErrorCursors{Request: cursor})
		case nextCursor != "":
		case len(projected) < requestPageSize:
			endpointExhausted = true
		default:
			return drivePageResult{}, drivePaginationError(options, "pagination_unproven", nil, page, items, drivePaginationErrorCursors{Request: cursor})
		}

		if !endpointExhausted && seenCursors[nextCursor] {
			return drivePageResult{}, drivePaginationError(options, "stalled_cursor", nil, page, items, drivePaginationErrorCursors{Request: cursor})
		}

		result := drivePageResult{
			Business:          map[string]any{options.OutputKey: items},
			EndpointExhausted: endpointExhausted,
			NextToken:         nextCursor,
			Pages:             pagesRead,
			Items:             len(items),
		}
		if endpointExhausted || !options.PageAll || len(items) >= options.MaxItems || page == pageLimit {
			return result, nil
		}

		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
}

func outputDrivePageResult(rt *shortcut.RuntimeContext, result drivePageResult) error {
	pagination, err := output.NewPagination(result.EndpointExhausted, result.NextToken)
	if err != nil {
		return fmt.Errorf("drive pagination result is invalid: %w", err)
	}
	pagination.Pages = result.Pages
	pagination.Items = result.Items
	meta := &output.Meta{
		Count:      output.NewCount(result.Items),
		Pagination: pagination,
	}
	// Keep the existing data paths for consumers of the already-unified commands.
	// Both projections derive from the same validated pagination state.
	data := cloneDriveMap(result.Business)
	data["count"] = result.Items
	data["hasMore"] = !result.EndpointExhausted
	if result.NextToken != "" {
		data["nextCursor"] = result.NextToken
	}
	return output.StoreResult(rt.Command().Context(), output.Success(data, output.WithMeta(meta)))
}

func drivePaginationError(options drivePageOptions, reason string, cause error, page int, items []map[string]any, cursors drivePaginationErrorCursors) error {
	message := fmt.Sprintf("%s 分页未完成，已在第 %d 页停止", options.Tool, page)
	if cause != nil {
		message += ": " + cause.Error()
	}
	details := map[string]any{
		"status":          "partial_success",
		"complete":        false,
		"reason":          reason,
		"page":            page,
		"count":           len(items),
		options.OutputKey: items,
	}
	if cursors.Request != "" {
		details["requestCursor"] = cursors.Request
	}
	actions := []string{"缩小查询范围后重新执行"}
	if cursors.Retry != "" {
		details["retryCursor"] = cursors.Retry
		actions = append([]string{"使用 retryCursor 重试当前页"}, actions...)
	}
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation(options.Server+"/"+options.Tool),
		apperrors.WithReason("drive_pagination_"+reason),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(cause != nil),
		apperrors.WithActions(actions...),
		apperrors.WithDetails(details),
		apperrors.WithCause(cause),
	)
}

func cloneDriveMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source)+2)
	for key, value := range source {
		out[key] = value
	}
	return out
}

func drivePageItemKey(item map[string]any) string {
	for _, key := range []string{"nodeId", "dentryId", "id", "docUrl", "url"} {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return key + ":" + strings.TrimSpace(value)
		}
	}
	return ""
}

func drivePageState(data map[string]any) (bool, bool, string) {
	hasMore, known := boolField(data, "hasMore", "has_more")
	next := firstString(data, "nextCursor", "nextToken", "nextPageToken", "next_page_token")
	return hasMore, known, next
}
