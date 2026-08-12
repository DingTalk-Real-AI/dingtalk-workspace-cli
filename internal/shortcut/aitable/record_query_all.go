// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func queryAllRecords(rt *shortcut.RuntimeContext, params map[string]any, maxRecords int) ([]map[string]any, error) {
	all := make([]map[string]any, 0)
	cursor := ""
	seen := map[string]bool{}
	for page := 0; ; page++ {
		request := cloneAnyMap(params)
		request["limit"] = recordBatchSize
		if cursor != "" {
			request["cursor"] = cursor
		}
		data, err := rt.CallMCPData(serverMain, "query_records", request)
		if err != nil {
			return nil, err
		}
		records, found := findRecords(data)
		if !found {
			return nil, fmt.Errorf("query_records page %d is missing the records collection", page+1)
		}
		all = append(all, records...)
		if maxRecords > 0 && len(all) > maxRecords {
			return nil, fmt.Errorf("query matched more than the allowed %d records", maxRecords)
		}
		if !responseHasMore(data) {
			return all, nil
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
