// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// query_records is backed by a service pager whose stable typed-envelope
// boundary is 20 records. Asking that layer to aggregate more than one page can
// drop the envelope fields used by the record projector, turning a successful
// read into an empty collection. Keep every remote request on one service page
// and aggregate only after each page has passed the CLI's shape validation.
const (
	recordQueryServicePageSize          = 20
	recordQueryMaxConsecutiveEmptyPages = 3
)

type recordQueryWindow struct {
	Records       []map[string]any
	HasMore       bool
	NextCursor    string
	Pages         int
	TotalCount    any
	HasTotalCount bool
}

func queryRecordWindow(rt *shortcut.RuntimeContext, params map[string]any, limit int) (recordQueryWindow, error) {
	if limit <= 0 {
		return recordQueryWindow{}, fmt.Errorf("record query limit must be positive, got %d", limit)
	}
	request := cloneAnyMap(params)
	cursor, _ := request["cursor"].(string)
	cursor = strings.TrimSpace(cursor)
	seen := map[string]bool{}
	if cursor != "" {
		seen[cursor] = true
	}
	window := recordQueryWindow{Records: make([]map[string]any, 0, limit)}
	consecutiveEmptyPages := 0
	seenRecords := map[string]bool{}

	for len(window.Records) < limit {
		pageSize := minInt(recordQueryServicePageSize, limit-len(window.Records))
		request["limit"] = pageSize
		if cursor == "" {
			delete(request, "cursor")
		} else {
			request["cursor"] = cursor
		}
		data, err := rt.CallMCPData(serverMain, "query_records", request)
		if err != nil {
			return recordQueryWindow{}, err
		}
		if !window.HasTotalCount {
			window.TotalCount, window.HasTotalCount = responseTotalCount(data)
		}
		records, found := findRecords(data)
		if !found {
			if explicitEmptyRecordQuery(data) {
				window.Pages++
				window.HasMore = false
				window.NextCursor = ""
				return window, nil
			}
			return recordQueryWindow{}, fmt.Errorf("query_records page %d is missing the records collection", window.Pages+1)
		}
		for _, record := range records {
			id := recordID(record)
			if id == "" {
				return recordQueryWindow{}, fmt.Errorf("query_records result is missing recordId")
			}
			if seenRecords[id] {
				return recordQueryWindow{}, fmt.Errorf("query_records returned duplicate recordId across pages; completeness cannot be established")
			}
			seenRecords[id] = true
		}
		window.Records = append(window.Records, records...)
		window.Pages++

		if !responseHasMore(data) {
			window.HasMore = false
			window.NextCursor = ""
			return window, nil
		}
		if len(records) == 0 {
			consecutiveEmptyPages++
			if consecutiveEmptyPages >= recordQueryMaxConsecutiveEmptyPages {
				return recordQueryWindow{}, fmt.Errorf(
					"query_records made no progress for %d consecutive pages",
					consecutiveEmptyPages,
				)
			}
		} else {
			consecutiveEmptyPages = 0
		}
		next := responseCursor(data)
		if next == "" {
			return recordQueryWindow{}, fmt.Errorf("query_records page %d reports more data but no next cursor", window.Pages)
		}
		if seen[next] {
			return recordQueryWindow{}, fmt.Errorf("query_records cursor cycle detected at %q", next)
		}
		seen[next] = true
		cursor = next
		window.HasMore = true
		window.NextCursor = next
	}

	return window, nil
}

// explicitEmptyRecordQuery recognizes the service's reviewed zero-match wire
// shape. It is deliberately stricter than "records is absent": only a complete
// success envelope with an empty data object is accepted as an empty set.
func explicitEmptyRecordQuery(data map[string]any) bool {
	if data == nil || data["success"] != true || data["status"] != "success" {
		return false
	}
	payload, ok := data["data"].(map[string]any)
	if !ok || len(payload) != 0 {
		return false
	}
	if rawError, exists := data["error"]; exists && rawError != nil {
		errorObject, ok := rawError.(map[string]any)
		if !ok || len(errorObject) != 0 {
			return false
		}
	}
	return true
}

func executeRecordQuery(rt *shortcut.RuntimeContext, params map[string]any) error {
	if rt.Bool("all") && rt.Changed("cursor") {
		return fmt.Errorf("--all 必须从第一页开始，不能与 --cursor 同用")
	}
	if rt.Changed("output") {
		if !rt.Bool("all") {
			return fmt.Errorf("--output requires --all; incomplete pages are not exported")
		}
		cwd, err := aitableWorkingDirectory()
		if err != nil {
			return err
		}
		if _, _, err = localio.ResolveOutputPath(cwd, rt.Str("output"), "", "records.ndjson"); err != nil {
			return err
		}
	}
	if rt.Changed("max-records") && !rt.Bool("all") {
		return fmt.Errorf("--max-records requires --all")
	}
	if rt.Bool("all") && (rt.Int("max-records") < 1 || rt.Int("max-records") > maxCompositeRecordRun) {
		return fmt.Errorf("--max-records must be in [1,%d]", maxCompositeRecordRun)
	}
	limit := 100
	if rt.Changed("limit") {
		limit = rt.Int("limit")
	}
	if limit < 1 || limit > recordBatchSize {
		return fmt.Errorf("--limit must be in [1,%d], got %d", recordBatchSize, limit)
	}
	requestedIDs, exactIDQuery, err := recordQueryRequestedIDs(params)
	if err != nil {
		return err
	}
	if exactIDQuery {
		if len(requestedIDs) > recordBatchSize {
			return fmt.Errorf("--record-ids accepts at most %d unique IDs, got %d", recordBatchSize, len(requestedIDs))
		}
		params = cloneAnyMap(params)
		params["recordIds"] = requestedIDs
		limit = minInt(limit, len(requestedIDs))
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{
			"dry_run":   true,
			"executed":  false,
			"tool":      "query_records",
			"arguments": params,
		})
	}
	if rt.Changed("view-id") {
		params, err = helpers.AITableQueryWithView(rt.Command().Context(), params, rt.Str("view-id"), false)
		if err != nil {
			return err
		}
	}
	if rt.Bool("all") {
		maxRecords := rt.Int("max-records")
		var records []map[string]any
		if exactIDQuery {
			records, err = queryRecordsByIDParams(rt, params, requestedIDs)
		} else {
			params["limit"] = minInt(recordQueryServicePageSize, limit)
			records, err = queryAllRecords(rt, params, maxRecords)
		}
		if err != nil {
			return err
		}
		if exactIDQuery {
			// queryRecordsByIDParams validates each disjoint ID chunk. Only
			// completeness remains to check after joining those chunks.
			if len(records) != len(requestedIDs) {
				return fmt.Errorf("exact-ID full read is incomplete; one or more requested records were not returned")
			}
			if len(records) > maxRecords {
				return fmt.Errorf("exact-ID result exceeds --max-records")
			}
		}
		return outputRecordQuery(rt, records, map[string]any{"success": true, "status": "success", "data": map[string]any{"records": records, "hasMore": false, "size": len(records)}})
	}
	var window recordQueryWindow
	if exactIDQuery {
		selected := requestedIDs[:limit]
		window.Records, err = queryRecordsByIDParams(rt, params, selected)
		window.Pages = (len(selected) + recordQueryServicePageSize - 1) / recordQueryServicePageSize
	} else {
		window, err = queryRecordWindow(rt, params, limit)
	}
	if err != nil {
		return err
	}
	if exactIDQuery {
		complete, err := validateExactRecordQuery(window.Records, requestedIDs)
		if err != nil {
			return err
		}
		if complete {
			// The service may advertise unrelated continuation after every
			// requested ID is already present. Exact-ID completion is stronger
			// evidence than that residual pager state.
			window.HasMore = false
			window.NextCursor = ""
		}
	}
	records := make([]any, 0, len(window.Records))
	for _, record := range window.Records {
		records = append(records, record)
	}
	data := map[string]any{
		"records": records,
		"hasMore": window.HasMore,
		"page":    window.Pages,
		"size":    len(window.Records),
	}
	if exactIDQuery {
		found := map[string]bool{}
		for _, r := range window.Records {
			found[recordID(r)] = true
		}
		missing := []string{}
		for _, id := range requestedIDs[:limit] {
			if !found[id] {
				missing = append(missing, id)
			}
		}
		data["missingRecordIds"] = missing
		data["unqueriedRecordIds"] = append([]string{}, requestedIDs[limit:]...)
		data["requestedCount"] = len(requestedIDs)
		data["complete"] = len(missing) == 0 && limit == len(requestedIDs)
	}
	if window.NextCursor != "" {
		data["nextCursor"] = window.NextCursor
	}
	if window.HasTotalCount {
		data["totalCount"] = window.TotalCount
	}
	return rt.Output(map[string]any{
		"success": true,
		"status":  "success",
		"data":    data,
	})
}

func responseTotalCount(data map[string]any) (any, bool) {
	for {
		if totalCount, exists := data["totalCount"]; exists {
			return totalCount, true
		}
		nested, ok := data["data"].(map[string]any)
		if !ok {
			return nil, false
		}
		data = nested
	}
}

func recordQueryRequestedIDs(params map[string]any) ([]string, bool, error) {
	raw, exists := params["recordIds"]
	if !exists {
		return nil, false, nil
	}
	values, ok := raw.([]string)
	if !ok {
		return nil, false, fmt.Errorf("recordIds must be a string list, got %T", raw)
	}
	ids, err := parseRecordIDs(values)
	if err != nil {
		return nil, false, err
	}
	return ids, true, nil
}

func validateExactRecordQuery(records []map[string]any, requestedIDs []string) (bool, error) {
	wanted := make(map[string]bool, len(requestedIDs))
	for _, id := range requestedIDs {
		wanted[id] = true
	}
	seen := make(map[string]bool, len(records))
	for index, record := range records {
		id := recordID(record)
		if id == "" {
			return false, fmt.Errorf("query_records exact-ID result %d is missing recordId", index)
		}
		if !wanted[id] {
			return false, fmt.Errorf("query_records exact-ID result contains unexpected recordId %q", id)
		}
		if seen[id] {
			return false, fmt.Errorf("query_records exact-ID result contains duplicate recordId %q", id)
		}
		seen[id] = true
	}
	return len(seen) == len(wanted), nil
}

func queryAllRecords(rt *shortcut.RuntimeContext, params map[string]any, maxRecords int) ([]map[string]any, error) {
	all := make([]map[string]any, 0)
	cursor, _ := params["cursor"].(string)
	seen := map[string]bool{}
	if cursor != "" {
		seen[cursor] = true
	}
	emptyPages := 0
	seenRecordIDs := map[string]bool{}
	for page := 0; ; page++ {
		request := cloneAnyMap(params)
		request["limit"] = recordQueryServicePageSize
		if requested, ok := params["limit"].(int); ok && requested > 0 && requested < recordQueryServicePageSize {
			request["limit"] = requested
		}
		if cursor != "" {
			request["cursor"] = cursor
		}
		data, err := rt.CallMCPData(serverMain, "query_records", request)
		if err != nil {
			return nil, err
		}
		records, found := findRecords(data)
		if !found {
			if explicitEmptyRecordQuery(data) {
				return all, nil
			}
			return nil, fmt.Errorf("query_records page %d is missing the records collection", page+1)
		}
		for _, record := range records {
			id := recordID(record)
			if id == "" {
				return nil, fmt.Errorf("query_records page %d is missing recordId", page+1)
			}
			if seenRecordIDs[id] {
				return nil, fmt.Errorf("query_records page %d repeats a recordId from an earlier page; completeness cannot be established", page+1)
			}
			seenRecordIDs[id] = true
		}
		all = append(all, records...)
		if maxRecords > 0 && len(all) > maxRecords {
			return nil, fmt.Errorf("query matched more than the allowed %d records", maxRecords)
		}
		if !responseHasMore(data) {
			return all, nil
		}
		if len(records) == 0 {
			emptyPages++
			if emptyPages >= recordQueryMaxConsecutiveEmptyPages {
				return nil, fmt.Errorf("query_records made no progress across empty pages")
			}
		} else {
			emptyPages = 0
		}
		next := responseCursor(data)
		if next == "" {
			return nil, fmt.Errorf("query_records page %d reports more data but no next cursor", page+1)
		}
		if seen[next] || next == cursor {
			return nil, fmt.Errorf("query_records cursor cycle detected at %q", next)
		}
		seen[next] = true
		cursor = next
		if page >= 9999 {
			return nil, fmt.Errorf("query_records exceeded the 10000-page safety bound")
		}
	}
}

func responseCursor(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"nextCursor", "next_cursor", "cursor"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, key := range []string{"data", "result", "pagination", "page"} {
		if nested, ok := data[key].(map[string]any); ok {
			if cursor := responseCursor(nested); cursor != "" {
				return cursor
			}
		}
	}
	return ""
}

func cloneAnyMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
