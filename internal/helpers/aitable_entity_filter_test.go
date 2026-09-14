// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

type aitableEntityReaderStep struct {
	data map[string]any
	err  error
}

type aitableEntityReaderStub struct {
	steps []aitableEntityReaderStep
	calls []map[string]any
}

func (r *aitableEntityReaderStub) CallMCPData(_ string, _ string, params map[string]any) (map[string]any, error) {
	copy := map[string]any{}
	for key, value := range params {
		copy[key] = value
	}
	r.calls = append(r.calls, copy)
	index := len(r.calls) - 1
	if index >= len(r.steps) {
		return nil, errors.New("unexpected entity search call")
	}
	return r.steps[index].data, r.steps[index].err
}

func TestCrossPlatformCoverageAITableViewFilterResolvesEntityNamesBeforeWrite(t *testing.T) {
	reader := &aitableEntityReaderStub{steps: []aitableEntityReaderStep{{data: map[string]any{
		"data": map[string]any{
			"candidates": []any{map[string]any{
				"name":        "客户成功部",
				"description": "集团/业务中心/客户成功部",
				"department":  map[string]any{"departmentId": "52528700"},
			}},
			"hasMore": false,
		},
	}}}}
	filter := []any{map[string]any{
		"operator": "and",
		"operands": []any{map[string]any{
			"operator": "eq",
			"operands": []any{"fldDept", map[string]any{"entityName": "客户成功部"}},
		}},
	}}

	got, searched, err := normalizeAitableViewFilterEntities(
		filter, map[string]string{"fldDept": "department"}, reader)
	if err != nil {
		t.Fatalf("normalizeAitableViewFilterEntities() error = %v", err)
	}
	if !searched || len(reader.calls) != 1 || reader.calls[0]["entityType"] != "DEPARTMENT" {
		t.Fatalf("searched=%v calls=%#v", searched, reader.calls)
	}
	root := got[0].(map[string]any)
	leaf := root["operands"].([]any)[0].(map[string]any)
	if value := leaf["operands"].([]any)[1]; !reflect.DeepEqual(value, map[string]any{"departmentId": "52528700"}) {
		t.Fatalf("normalized department = %#v", value)
	}
	// 输入必须保持不变，确保解析失败时不会留下部分改写。
	originalValue := filter[0].(map[string]any)["operands"].([]any)[0].(map[string]any)["operands"].([]any)[1]
	if !reflect.DeepEqual(originalValue, map[string]any{"entityName": "客户成功部"}) {
		t.Fatalf("input mutated = %#v", filter)
	}
}

func TestCrossPlatformCoverageAITableViewFilterRejectsBareEntityScalarWithoutSearch(t *testing.T) {
	reader := &aitableEntityReaderStub{}
	filter := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldGroup", "项目群"},
	}}

	_, _, err := normalizeAitableViewFilterEntities(
		filter, map[string]string{"fldGroup": "group"}, reader)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "invalid_entity_reference" || len(reader.calls) != 0 {
		t.Fatalf("error=%#v calls=%#v", err, reader.calls)
	}
}

func TestCrossPlatformCoverageAITableViewFilterRejectsMixedEntityIdentities(t *testing.T) {
	reader := &aitableEntityReaderStub{}
	filter := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldOwner", map[string]any{
			"userId": "staff1", "corpId": "ding1", "departmentId": "dept1",
		}},
	}}

	_, _, err := normalizeAitableViewFilterEntities(
		filter, map[string]string{"fldOwner": "person"}, reader)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "invalid_entity_reference" || len(reader.calls) != 0 {
		t.Fatalf("error=%#v calls=%#v", err, reader.calls)
	}
}

func TestCrossPlatformCoverageAITableViewFilterStableIdentityBypassesSearchAndDedupes(t *testing.T) {
	reader := &aitableEntityReaderStub{}
	filter := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldOwner", []any{
			map[string]any{"userId": "staff1", "corpId": "ding1"},
			map[string]any{"userId": "staff1", "corpId": "ding1"},
		}},
	}}

	got, searched, err := normalizeAitableViewFilterEntities(
		filter, map[string]string{"fldOwner": "person"}, reader)
	if err != nil || searched || len(reader.calls) != 0 {
		t.Fatalf("got=%#v searched=%v calls=%#v err=%v", got, searched, reader.calls, err)
	}
	values := got[0].(map[string]any)["operands"].([]any)[1].([]any)
	if len(values) != 1 || !reflect.DeepEqual(values[0], map[string]any{"userId": "staff1", "corpId": "ding1"}) {
		t.Fatalf("deduped values = %#v", values)
	}
}

func TestCrossPlatformCoverageAITableViewFilterAcceptsExclusiveOpenConversationID(t *testing.T) {
	reader := &aitableEntityReaderStub{}
	filter := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldGroup", map[string]any{"openConversationId": "open-cid-1"}},
	}}

	got, searched, err := normalizeAitableViewFilterEntities(
		filter, map[string]string{"fldGroup": "group"}, reader)
	if err != nil || searched || len(reader.calls) != 0 {
		t.Fatalf("got=%#v searched=%v calls=%#v err=%v", got, searched, reader.calls, err)
	}
	value := got[0].(map[string]any)["operands"].([]any)[1]
	if !reflect.DeepEqual(value, map[string]any{"openConversationId": "open-cid-1"}) {
		t.Fatalf("group reference = %#v", value)
	}

	filter[0].(map[string]any)["operands"].([]any)[1] = map[string]any{
		"cid": "cid-1", "openConversationId": "open-cid-1",
	}
	_, _, err = normalizeAitableViewFilterEntities(
		filter, map[string]string{"fldGroup": "group"}, reader)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "invalid_entity_reference" ||
		!strings.Contains(typed.Hint, "只能提供一个") {
		t.Fatalf("exclusive group identifiers error = %#v", err)
	}
}

func TestCrossPlatformCoverageAITableViewFilterReadBackUsesNormalizedUpdateResponse(t *testing.T) {
	requested := []any{map[string]any{"operator": "eq", "operands": []any{"fldGroup", map[string]any{"openConversationId": "open-cid"}}}}
	normalized := map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "eq", "operands": []any{"fldGroup", "cid"}}}}
	raw := `{"status":"success","data":{"viewId":"view","filter":{"operator":"and","operands":[{"operator":"eq","operands":["fldGroup","cid"]}]}}}`

	expected := aitableViewFilterReadBackExpectation(raw, "view", requested)
	if !reflect.DeepEqual(expected, normalized) {
		t.Fatalf("normalized expectation = %#v", expected)
	}
	matched, unknown, actual := compareAitableViewFilterReadBack(
		map[string]any{"filter": normalized}, expected, map[string]string{"fldGroup": "group"})
	if !matched || unknown || !reflect.DeepEqual(actual, normalized) {
		t.Fatalf("normalized read-back matched=%v unknown=%v actual=%#v", matched, unknown, actual)
	}

	for _, response := range []string{"{", `{"data":{"viewId":"other","filter":[]}}`, `{"data":{"viewId":"view"}}`} {
		if got := aitableViewFilterReadBackExpectation(response, "view", requested); !reflect.DeepEqual(got, requested) {
			t.Fatalf("fallback expectation for %q = %#v", response, got)
		}
	}
}

func TestCrossPlatformCoverageAITableViewFilterReadBackRejectsMissingOrStalePersistedFilter(t *testing.T) {
	requested := []any{map[string]any{
		"operator": "eq", "operands": []any{"fldText", "NEW"},
	}}
	for name, view := range map[string]map[string]any{
		"missing": {},
		"stale": {
			"filter": []any{map[string]any{
				"operator": "eq", "operands": []any{"fldText", "OLD"},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			matched, unknown, _ := compareAitableViewFilterReadBack(
				view, requested, map[string]string{"fldText": "text"})
			if matched || unknown {
				t.Fatalf("matched=%v unknown=%v", matched, unknown)
			}
		})
	}
}

func TestCrossPlatformCoverageAITableViewFilterReadBackKeepsLegacyOpenConversationIDUnknown(t *testing.T) {
	requested := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldGroup", map[string]any{"openConversationId": "open-cid"}},
	}}
	view := map[string]any{"filter": []any{map[string]any{
		"operator": "eq", "operands": []any{"fldGroup", "internal-cid"},
	}}}

	matched, unknown, _ := compareAitableViewFilterReadBack(
		view, requested, map[string]string{"fldGroup": "group"})
	if matched || !unknown {
		t.Fatalf("matched=%v unknown=%v", matched, unknown)
	}
}

func TestCrossPlatformCoverageAITableViewFilterReadBackKeepsLegacyPersonIdentityUnknown(t *testing.T) {
	expected := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldOwner", map[string]any{"userId": "staff1", "corpId": "ding1"}},
	}}
	view := map[string]any{"filter": map[string]any{
		"operator": "and",
		"operands": []any{map[string]any{
			"operator": "eq", "operands": []any{"fldOwner", "12345"},
		}},
	}}

	matched, unknown, _ := compareAitableViewFilterReadBack(
		view, expected, map[string]string{"fldOwner": "user"})
	if matched || !unknown {
		t.Fatalf("matched=%v unknown=%v", matched, unknown)
	}
}

func TestCrossPlatformCoverageAITableEntityFilterStructuralFailuresAndNoops(t *testing.T) {
	reader := &aitableEntityReaderStub{}
	for name, filter := range map[string][]any{
		"non object root":          {"bad"},
		"missing operands":         {map[string]any{"operator": "eq"}},
		"non object logical child": {map[string]any{"operator": "and", "operands": []any{"bad"}}},
		"invalid logical child":    {map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "eq"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := normalizeAitableViewFilterEntities(filter, nil, reader); err == nil {
				t.Fatal("malformed filter succeeded")
			}
		})
	}
	for name, filter := range map[string][]any{
		"exist":            {map[string]any{"operator": "exist", "operands": []any{"field"}}},
		"short operands":   {map[string]any{"operator": "eq", "operands": []any{"field"}}},
		"non entity field": {map[string]any{"operator": "eq", "operands": []any{"field", "value"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, searched, err := normalizeAitableViewFilterEntities(filter, map[string]string{"field": "text"}, reader); err != nil || searched {
				t.Fatalf("noop filter searched=%v err=%v", searched, err)
			}
		})
	}
	if _, _, err := normalizeAitableViewFilterEntities([]any{map[string]any{
		"operator": "eq", "operands": []any{"field", []any{}},
	}}, map[string]string{"field": "department"}, reader); err == nil {
		t.Fatal("empty entity list succeeded")
	}
	if _, _, err := normalizeAitableViewFilterEntities([]any{map[string]any{
		"operator": "eq", "operands": []any{"field", []any{map[string]any{}}},
	}}, map[string]string{"field": "department"}, reader); err == nil || !strings.Contains(err.Error(), "entity value 0") {
		t.Fatalf("nested entity error = %v", err)
	}
	if _, _, err := normalizeAitableViewFilterEntities([]any{map[string]any{
		"operator": "eq", "operands": []any{"field", map[string]any{"entityName": "研发", "departmentId": "d"}},
	}}, map[string]string{"field": "department"}, reader); err == nil {
		t.Fatal("name plus stable reference succeeded")
	}
}

func TestCrossPlatformCoverageAITableEntityFilterResolutionCache(t *testing.T) {
	reader := &aitableEntityReaderStub{steps: []aitableEntityReaderStep{{data: map[string]any{
		"candidates": []any{map[string]any{"name": "研发", "department": map[string]any{"departmentId": "d"}}},
		"hasMore":    false,
	}}}}
	filter := []any{
		map[string]any{"operator": "eq", "operands": []any{"field", map[string]any{"entityName": "研发"}}},
		map[string]any{"operator": "eq", "operands": []any{"field", map[string]any{"entityName": " 研发 "}}},
	}
	if _, searched, err := normalizeAitableViewFilterEntities(filter, map[string]string{"field": "department"}, reader); err != nil || !searched || len(reader.calls) != 1 {
		t.Fatalf("cache calls=%d searched=%v err=%v", len(reader.calls), searched, err)
	}
}

func TestCrossPlatformCoverageAITableStableEntityReferenceMatrix(t *testing.T) {
	valid := []struct {
		kind aitabletarget.EntityType
		in   map[string]any
		want map[string]any
	}{
		{aitabletarget.EntityPerson, map[string]any{"userRef": " ref "}, map[string]any{"userRef": "ref"}},
		{aitabletarget.EntityPerson, map[string]any{"userId": " u ", "corpId": " c "}, map[string]any{"userId": "u", "corpId": "c"}},
		{aitabletarget.EntityDepartment, map[string]any{"departmentId": " d ", "departmentKey": "d"}, map[string]any{"departmentId": "d"}},
		{aitabletarget.EntityDepartment, map[string]any{"departmentKey": " key "}, map[string]any{"departmentKey": "key"}},
		{aitabletarget.EntityGroup, map[string]any{"cid": " cid "}, map[string]any{"cid": "cid"}},
		{aitabletarget.EntityGroup, map[string]any{"openConversationId": " open "}, map[string]any{"openConversationId": "open"}},
	}
	for _, test := range valid {
		got, err := normalizeAitableStableReference(test.kind, test.in)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("normalizeAitableStableReference(%s, %#v) = %#v, %v", test.kind, test.in, got, err)
		}
	}
	invalid := []struct {
		kind aitabletarget.EntityType
		in   map[string]any
	}{
		{aitabletarget.EntityPerson, map[string]any{"cid": "x"}},
		{aitabletarget.EntityPerson, map[string]any{"userRef": "r", "userId": "u", "corpId": "c"}},
		{aitabletarget.EntityPerson, map[string]any{"userId": "u"}},
		{aitabletarget.EntityDepartment, map[string]any{"userRef": "r"}},
		{aitabletarget.EntityDepartment, map[string]any{"departmentId": "a", "departmentKey": "b"}},
		{aitabletarget.EntityGroup, map[string]any{"departmentId": "d"}},
		{aitabletarget.EntityGroup, map[string]any{"cid": "c", "openConversationId": "o"}},
		{aitabletarget.EntityGroup, map[string]any{}},
	}
	for _, test := range invalid {
		if _, err := normalizeAitableStableReference(test.kind, test.in); err == nil {
			t.Fatalf("invalid reference succeeded: %s %#v", test.kind, test.in)
		}
	}
}

func TestCrossPlatformCoverageAITableEntityProjectionBranches(t *testing.T) {
	for _, test := range []struct {
		field string
		kind  aitabletarget.EntityType
		ok    bool
	}{
		{" user ", aitabletarget.EntityPerson, true},
		{"PERSON", aitabletarget.EntityPerson, true},
		{"department", aitabletarget.EntityDepartment, true},
		{"group", aitabletarget.EntityGroup, true},
		{"text", "", false},
	} {
		kind, ok := aitableEntityTypeForField(test.field)
		if kind != test.kind || ok != test.ok {
			t.Fatalf("aitableEntityTypeForField(%q) = %q, %v", test.field, kind, ok)
		}
	}
	for _, reference := range []aitabletarget.EntityReference{
		{UserID: "u", CorpID: "c"}, {UserRef: "r"}, {DepartmentID: "d"},
		{DepartmentKey: "k"}, {CID: "cid"}, {},
	} {
		_ = aitableReferenceMap(reference)
	}
	if hasAitableStableEntityReference(map[string]any{"userId": 1}) || !hasAitableStableEntityReference(map[string]any{"cid": " c "}) {
		t.Fatal("stable reference detection mismatch")
	}

	for _, test := range []struct {
		value   any
		kind    aitabletarget.EntityType
		unknown bool
	}{
		{[]any{"d", map[string]any{"departmentId": "d2"}}, aitabletarget.EntityDepartment, false},
		{map[string]any{"cid": "c"}, aitabletarget.EntityGroup, false},
		{map[string]any{}, aitabletarget.EntityPerson, true},
		{" d ", aitabletarget.EntityDepartment, false},
		{" c ", aitabletarget.EntityGroup, false},
		{"u", aitabletarget.EntityPerson, true},
		{"x", aitabletarget.EntityType("OTHER"), false},
		{" ", aitabletarget.EntityGroup, true},
		{42, aitabletarget.EntityGroup, true},
	} {
		_, unknown := projectPersistedAitableEntityValue(test.value, test.kind)
		if unknown != test.unknown {
			t.Fatalf("projectPersistedAitableEntityValue(%#v, %s) unknown=%v", test.value, test.kind, unknown)
		}
	}
}

func TestCrossPlatformCoverageAITableEntityFilterReadbackAndProjectionFailures(t *testing.T) {
	malformed := map[string]any{"operator": "and", "operands": []any{"raw", map[string]any{"operator": "eq"}}}
	projected, unknown := projectPersistedAitableEntityFilter(malformed, nil)
	if unknown || projected == nil {
		t.Fatalf("malformed logical projection = %#v, %v", projected, unknown)
	}
	if got, unknown := projectPersistedAitableEntityFilter("raw", nil); unknown || got != "raw" {
		t.Fatalf("non-filter projection = %#v, %v", got, unknown)
	}

	requested := []any{map[string]any{"operator": "eq", "operands": []any{"field", map[string]any{"departmentId": "wanted"}}}}
	view := map[string]any{"filter": map[string]any{"operator": "eq", "operands": []any{"field", "actual"}}}
	matched, unknown, actual := compareAitableViewFilterReadBack(view, requested, map[string]string{"field": "department"})
	if matched || unknown || actual == nil {
		t.Fatalf("readback mismatch matched=%v unknown=%v actual=%#v", matched, unknown, actual)
	}
}

func TestCrossPlatformCoverageAITableNativeEntityReaderAndCommandSuccess(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"candidates":[],"hasMore":false}`}}
	installAitableDeps(t, caller)
	if _, err := (nativeAitableEntityReader{}).CallMCPData("aitable", "search_entities", map[string]any{}); err != nil {
		t.Fatalf("native entity reader: %v", err)
	}

	commandCaller := &aitableCommandCoverageCaller{response: map[string]string{
		"search_entities": `{"candidates":[{"name":"研发","department":{"departmentId":"d"}}],"hasMore":false}`,
	}}
	if err := runAitableCoverageCommand(t, commandCaller, "entity", "search", "--entity-type=DEPARTMENT", "--keyword=研发"); err != nil {
		t.Fatalf("entity search command: %v", err)
	}
}

func TestCrossPlatformCoverageAITableEntityResolutionAndLegacyProjectionBranches(t *testing.T) {
	reader := &aitableEntityReaderStub{steps: []aitableEntityReaderStep{{err: errors.New("search failed")}}}
	if _, _, err := normalizeAitableEntityFilterValue(
		map[string]any{"entityName": "研发"}, aitabletarget.EntityDepartment, reader,
		map[string]aitabletarget.EntityResolution{},
	); err == nil {
		t.Fatal("resolution error was not propagated")
	}

	requested := map[string]any{"operator": "and", "operands": []any{
		map[string]any{"operator": "eq", "operands": []any{"field", map[string]any{"departmentId": "d"}}},
	}}
	view := map[string]any{"filter": map[string]any{"operator": "and", "operands": []any{
		map[string]any{"operator": "eq", "operands": []any{"field", "d"}},
	}}}
	matched, unknown, _ := compareAitableViewFilterReadBack(view, requested, map[string]string{"field": "department"})
	if !matched || unknown {
		t.Fatalf("legacy projection matched=%v unknown=%v", matched, unknown)
	}

	for _, condition := range []map[string]any{
		{"operator": "eq", "operands": []any{"field"}},
		{"operator": "eq", "operands": []any{"field", "value"}},
	} {
		if _, unknown := projectPersistedAitableEntityCondition(condition, map[string]string{"field": "text"}); unknown {
			t.Fatalf("non-entity condition became unknown: %#v", condition)
		}
	}
}
