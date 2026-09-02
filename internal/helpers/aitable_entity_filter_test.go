// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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

func TestCrossPlatformCoverageAITableViewFilterReadBackPrefersCompleteExternalProjection(t *testing.T) {
	expected := []any{map[string]any{
		"operator": "eq",
		"operands": []any{"fldOwner", map[string]any{"userId": "staff1", "corpId": "ding1"}},
	}}
	view := map[string]any{
		"filter": map[string]any{
			"operator": "and",
			"operands": []any{map[string]any{
				"operator": "eq", "operands": []any{"fldOwner", "12345"},
			}},
		},
		"filterExternal": map[string]any{
			"operator": "and",
			"operands": []any{map[string]any{
				"operator": "eq",
				"operands": []any{"fldOwner", map[string]any{"userId": "staff1", "corpId": "ding1"}},
			}},
		},
		"filterExternalComplete": true,
	}

	matched, unknown, _ := compareAitableViewFilterReadBack(
		view, expected, expected, map[string]string{"fldOwner": "user"})
	if !matched || unknown {
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
		view, expected, expected, map[string]string{"fldOwner": "user"})
	if matched || !unknown {
		t.Fatalf("matched=%v unknown=%v", matched, unknown)
	}
}
