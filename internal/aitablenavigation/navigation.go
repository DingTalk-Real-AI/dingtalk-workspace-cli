// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

// Package aitablenavigation carries object access intent independently of write verification.
package aitablenavigation

import (
	"net/url"
	"strings"
)

// Preserve copies navigation from a mutation receipt only when readback has no replacement.
// It never copies mutation/verification status or invents a link for an old server.
func Preserve(target, receipt map[string]any) {
	for _, key := range []string{"url", "links", "appId"} {
		if _, exists := target[key]; !exists {
			if value, present := receipt[key]; present {
				target[key] = value
			}
		}
	}
}

// Fields builds navigation only for IDs independently verified by the composite command.
// This also covers resume-only runs, where there is no new create_fields receipt.
func Fields(baseID, tableID string, ids []string, definitions []any) map[string]any {
	fields := make([]any, 0, len(ids))
	links := make([]any, 0, len(ids))
	for i, id := range ids {
		if strings.TrimSpace(baseID) == "" || strings.TrimSpace(tableID) == "" || strings.TrimSpace(id) == "" {
			continue
		}
		inner := url.Values{"entrance": {"data"}, "sheetId": {tableID}, "fieldId": {id}}
		accessURL := "https://alidocs.dingtalk.com/i/nodes/" + url.PathEscape(baseID) + "?" + url.Values{"iframeQuery": {inner.Encode()}}.Encode()
		field := map[string]any{"baseId": baseID, "tableId": tableID, "fieldId": id, "url": accessURL}
		link := map[string]any{"objectType": "field", "objectId": id, "relation": "target", "url": accessURL}
		if i < len(definitions) {
			if definition, ok := definitions[i].(map[string]any); ok {
				if name, ok := definition["fieldName"].(string); ok {
					field["fieldName"] = name
					link["name"] = name
				}
			}
		}
		field["links"] = []any{link}
		fields = append(fields, field)
		links = append(links, link)
	}
	if len(fields) > 0 {
		baseURL := "https://alidocs.dingtalk.com/i/nodes/" + url.PathEscape(baseID)
		tableURL := baseURL + "?" + url.Values{"iframeQuery": {url.Values{"entrance": {"data"}, "sheetId": {tableID}}.Encode()}}.Encode()
		links = append(links, map[string]any{"objectType": "table", "objectId": tableID, "relation": "parent", "url": tableURL}, map[string]any{"objectType": "base", "objectId": baseID, "relation": "parent", "url": baseURL})
	}
	return map[string]any{"fields": fields, "links": links}
}
