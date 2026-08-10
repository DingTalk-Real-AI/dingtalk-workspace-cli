// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package devapp

import (
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestListAppParsePageCollectionAliases(t *testing.T) {
	for _, alias := range []string{"list", "items", "apps", "appList", "result", "data"} {
		t.Run(alias, func(t *testing.T) {
			page, err := listAppParsePage(map[string]any{
				alias:        []any{map[string]any{"name": alias}},
				"hasMore":    false,
				"nextCursor": nil,
			}, "")
			if err != nil {
				t.Fatalf("parse %s alias: %v", alias, err)
			}
			if page.hasMore || page.nextCursor != "" || len(page.apps) != 1 {
				t.Fatalf("page for %s = %#v", alias, page)
			}
		})
	}
}

func TestListAppParsePageNestedEnvelope(t *testing.T) {
	page, err := listAppParsePage(map[string]any{
		"result": map[string]any{
			"apps":       []any{map[string]any{"name": "nested"}},
			"hasMore":    true,
			"nextCursor": "cursor-2",
		},
	}, "cursor-1")
	if err != nil {
		t.Fatalf("parse nested page: %v", err)
	}
	if !page.hasMore || page.nextCursor != "cursor-2" || len(page.apps) != 1 {
		t.Fatalf("nested page = %#v", page)
	}
}

func TestListAppParsePageTerminalCursorVariants(t *testing.T) {
	for _, test := range []struct {
		name        string
		includeNext bool
		nextCursor  any
	}{
		{name: "missing"},
		{name: "null", includeNext: true, nextCursor: nil},
		{name: "empty", includeNext: true, nextCursor: ""},
		{name: "last observed", includeNext: true, nextCursor: "cursor-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := map[string]any{
				"apps":    []any{},
				"hasMore": false,
			}
			if test.includeNext {
				response["nextCursor"] = test.nextCursor
			}
			page, err := listAppParsePage(response, "cursor-1")
			if err != nil {
				t.Fatalf("parse terminal variant: %v", err)
			}
			if page.hasMore || page.nextCursor != "" || len(page.apps) != 0 {
				t.Fatalf("terminal page = %#v", page)
			}
		})
	}
}

func TestListAppParsePageNonTerminalPreservesCursor(t *testing.T) {
	page, err := listAppParsePage(map[string]any{
		"apps":       []any{},
		"hasMore":    true,
		"nextCursor": "  opaque-cursor  ",
	}, "request-cursor")
	if err != nil {
		t.Fatalf("parse non-terminal page: %v", err)
	}
	if !page.hasMore || page.nextCursor != "  opaque-cursor  " {
		t.Fatalf("non-terminal page = %#v", page)
	}
}

func TestListAppParsePageRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name          string
		response      map[string]any
		requestCursor string
	}{
		{name: "nil response", response: nil},
		{name: "missing list", response: map[string]any{"hasMore": false}},
		{name: "missing hasMore", response: map[string]any{"apps": []any{}}},
		{name: "hasMore wrong type", response: map[string]any{"apps": []any{}, "hasMore": "false"}},
		{name: "non-terminal missing cursor", response: map[string]any{"apps": []any{}, "hasMore": true}},
		{name: "non-terminal null cursor", response: map[string]any{"apps": []any{}, "hasMore": true, "nextCursor": nil}},
		{name: "non-terminal empty cursor", response: map[string]any{"apps": []any{}, "hasMore": true, "nextCursor": ""}},
		{name: "non-terminal stalled cursor", response: map[string]any{"apps": []any{}, "hasMore": true, "nextCursor": "cursor-1"}, requestCursor: "cursor-1"},
		{name: "terminal wrong cursor type", response: map[string]any{"apps": []any{}, "hasMore": false, "nextCursor": 1}},
		{name: "wrong list type", response: map[string]any{"apps": "bad", "hasMore": false}},
		{name: "multiple lists", response: map[string]any{"apps": []any{}, "items": []any{}, "hasMore": false}},
		{name: "split metadata", response: map[string]any{
			"apps":   []any{},
			"result": map[string]any{"hasMore": false},
		}},
		{name: "second nested envelope", response: map[string]any{
			"result": map[string]any{
				"data": map[string]any{"apps": []any{}, "hasMore": false},
			},
		}},
		{name: "non-list permissions alias", response: map[string]any{"permissions": []any{}, "hasMore": false}},
		{name: "non-list events alias", response: map[string]any{"events": []any{}, "hasMore": false}},
		{name: "non-list versions alias", response: map[string]any{"versions": []any{}, "hasMore": false}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := listAppParsePage(test.response, test.requestCursor)
			assertListAppPaginationError(t, err)
		})
	}
}

func TestListAppProjectPreservesAliasesOrderAndCount(t *testing.T) {
	raw := []any{
		map[string]any{
			"unified_app_id": "u-1",
			"appName":        "first",
			"clientId":       "key-1",
			"agent_id":       1,
			"appStatus":      "ONLINE",
			"modifyTime":     "t-1",
			"unknown":        "ignored",
		},
		map[string]any{
			"unifiedAppId": "u-2",
			"name":         "second",
		},
	}

	got, err := listAppProject(raw)
	if err != nil {
		t.Fatalf("project valid items: %v", err)
	}
	want := []map[string]any{
		{
			"unifiedAppId": "u-1",
			"name":         "first",
			"appKey":       "key-1",
			"agentId":      1,
			"status":       "ONLINE",
			"gmtModified":  "t-1",
		},
		{"unifiedAppId": "u-2", "name": "second"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection = %#v, want %#v", got, want)
	}
}

func TestListAppProjectRejectsUnprojectableItems(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  []any
	}{
		{name: "non object", raw: []any{"bad"}},
		{name: "empty object", raw: []any{map[string]any{}}},
		{name: "unknown only", raw: []any{map[string]any{"unknown": true}}},
		{name: "mixed invalid", raw: []any{map[string]any{"name": "valid"}, "bad"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			projected, err := listAppProject(test.raw)
			if projected != nil {
				t.Fatalf("partial projection leaked = %#v", projected)
			}
			assertListAppPaginationError(t, err)
		})
	}
}

func TestNonTargetListAliasesRemainCompatible(t *testing.T) {
	permissionItem := map[string]any{"scopeValue": "scope"}
	for _, alias := range []string{"permissionList", "scopes"} {
		got := permissionListProject(map[string]any{alias: []any{permissionItem}})
		if len(got) != 1 || got[0]["scopeValue"] != "scope" {
			t.Fatalf("permission alias %s regressed: %#v", alias, got)
		}
	}

	events := eventListProject(map[string]any{
		"eventList": []any{map[string]any{"eventCode": "event"}},
	})
	if len(events) != 1 || events[0]["eventCode"] != "event" {
		t.Fatalf("eventList alias regressed: %#v", events)
	}

	versions := versionListProject(map[string]any{
		"versionList": []any{map[string]any{"versionId": "version"}},
	})
	if len(versions) != 1 || versions[0]["versionId"] != "version" {
		t.Fatalf("versionList alias regressed: %#v", versions)
	}
}

func assertListAppPaginationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected pagination contract error")
	}
	var structured *apperrors.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if structured.Category != apperrors.CategoryAPI ||
		structured.Reason != "devapp_pagination_contract_invalid" ||
		structured.ExitCode() != 1 {
		t.Fatalf("pagination error = %#v", structured)
	}
}
