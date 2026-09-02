// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitabletarget

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestEntityTypeAndCandidateValidationBranches(t *testing.T) {
	for _, raw := range []string{"person", " DEPARTMENT ", "GROUP"} {
		if _, err := ParseEntityType(raw); err != nil {
			t.Fatalf("ParseEntityType(%q): %v", raw, err)
		}
	}
	if _, err := ParseEntityType("robot"); err == nil {
		t.Fatal("unknown entity type must fail")
	}

	valid := []struct {
		kind EntityType
		item map[string]any
		key  string
	}{
		{EntityPerson, map[string]any{"name": " Alice ", "person": map[string]any{"userId": "u", "corpId": "c"}}, "person:user:c:u"},
		{EntityPerson, map[string]any{"name": "Alice", "person": map[string]any{"userRef": "ref"}}, "person:ref:ref"},
		{EntityDepartment, map[string]any{"name": "研发", "department": map[string]any{"departmentId": "d"}}, "department:id:d"},
		{EntityDepartment, map[string]any{"name": "研发", "department": map[string]any{"departmentKey": "key"}}, "department:key:key"},
		{EntityGroup, map[string]any{"name": "群", "group": map[string]any{"cid": "cid"}}, "group:cid:cid"},
	}
	for _, test := range valid {
		candidate, err := parseEntityCandidate(test.kind, test.item)
		if err != nil || entityReferenceKey(candidate.Reference) != test.key {
			t.Fatalf("parseEntityCandidate(%s) = %#v, %v", test.kind, candidate, err)
		}
	}

	invalid := []struct {
		kind EntityType
		item map[string]any
	}{
		{EntityPerson, map[string]any{"name": ""}},
		{EntityPerson, map[string]any{"name": "Alice", "person": map[string]any{"userId": "u"}}},
		{EntityPerson, map[string]any{"name": "Alice", "person": map[string]any{"corpId": "c"}}},
		{EntityPerson, map[string]any{"name": "Alice", "person": "bad"}},
		{EntityDepartment, map[string]any{"name": "研发", "department": "bad"}},
		{EntityGroup, map[string]any{"name": "群", "group": map[string]any{}}},
	}
	for _, test := range invalid {
		if _, err := parseEntityCandidate(test.kind, test.item); err == nil {
			t.Fatalf("parseEntityCandidate(%s, %#v) succeeded", test.kind, test.item)
		}
	}
	if key := entityReferenceKey(EntityReference{}); key != "" {
		t.Fatalf("empty reference key = %q", key)
	}
}

func TestSearchEntitiesValidationAndResponseFailures(t *testing.T) {
	for _, query := range []string{"", "   ", strings.Repeat("字", 101)} {
		if _, err := SearchEntities(&resolverReader{}, EntityPerson, query); err == nil {
			t.Fatalf("SearchEntities(%q) succeeded", query)
		}
	}
	if _, err := SearchEntities(&resolverReader{}, EntityType("UNKNOWN"), "x"); err == nil {
		t.Fatal("invalid entity type succeeded")
	}
	if _, err := ResolveEntity(&resolverReader{}, EntityPerson, ""); err == nil {
		t.Fatal("ResolveEntity must propagate search validation errors")
	}
	plain := errors.New("network")
	if _, err := SearchEntities(&resolverReader{steps: []resolverStep{{err: plain}}}, EntityPerson, "x"); !errors.Is(err, plain) {
		t.Fatalf("plain error = %v", err)
	}

	for name, data := range map[string]map[string]any{
		"missing candidates":    {"hasMore": false},
		"wrong candidates type": {"candidates": "bad", "hasMore": false},
		"scalar candidate":      {"candidates": []any{"bad"}, "hasMore": false},
		"malformed person":      {"candidates": []any{map[string]any{"name": "Alice", "person": map[string]any{}}}, "hasMore": false},
		"missing has more":      {"candidates": []any{}},
		"terminal cursor":       {"candidates": []any{}, "hasMore": false, "nextCursor": "stale"},
		"missing cursor":        {"candidates": []any{}, "hasMore": true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := SearchEntities(&resolverReader{steps: []resolverStep{{data: data}}}, EntityPerson, "Alice"); err == nil {
				t.Fatal("malformed response succeeded")
			}
		})
	}
}

func TestSearchEntitiesRejectsCursorLoopsAndPageLimit(t *testing.T) {
	loop := &resolverReader{steps: []resolverStep{
		{data: map[string]any{"candidates": []any{}, "hasMore": true, "nextCursor": "next"}},
		{data: map[string]any{"candidates": []any{}, "hasMore": true, "nextCursor": "next"}},
	}}
	if _, err := SearchEntities(loop, EntityGroup, "群"); err == nil {
		t.Fatal("cursor loop succeeded")
	}

	steps := make([]resolverStep, maxResolutionPages)
	for index := range steps {
		steps[index].data = map[string]any{
			"candidates": []any{}, "hasMore": true, "nextCursor": fmt.Sprintf("cursor-%d", index),
		}
	}
	reader := &resolverReader{steps: steps}
	if _, err := SearchEntities(reader, EntityDepartment, "研发"); err == nil {
		t.Fatal("page limit succeeded")
	}
	if len(reader.calls) != maxResolutionPages {
		t.Fatalf("page limit stopped after %d calls", len(reader.calls))
	}
}

func TestEntityCandidateDeduplicationBranches(t *testing.T) {
	candidates := []EntityCandidate{
		{Name: "old", Reference: EntityReference{DepartmentID: "d"}},
		{Name: "<em>研发</em>", Reference: EntityReference{DepartmentID: "d"}},
		{Name: "ignored", Reference: EntityReference{}},
		{Name: "群", Reference: EntityReference{CID: "cid"}},
	}
	got := dedupeEntityCandidates(candidates, "研发")
	if len(got) != 2 || normalizeEntityDisplayName(got[0].Name) != "研发" || got[1].Reference.CID != "cid" {
		t.Fatalf("dedupeEntityCandidates() = %#v", got)
	}
}
