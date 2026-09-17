// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return data
}

func TestCrossPlatformCoverageListExplicitEmptyAndRealItemListE2E(t *testing.T) {
	empty, err := ParseListPage(decode(t, `{"success":true,"result":{"itemList":[],"hasMore":false,"nextToken":""}}`))
	if err != nil || len(empty.Items) != 0 || empty.HasMore {
		t.Fatalf("explicit empty = %#v, %v", empty, err)
	}
	page, err := ParseListPage(decode(t, `{"success":true,"result":{"itemList":[{"uuid":"u1","title":"n","startTime":"2026-08-10T01:00:00Z"}],"hasMore":true,"nextToken":"next"}}`))
	if err != nil || len(page.Items) != 1 || !page.HasMore || page.NextToken != "next" {
		t.Fatalf("real itemList = %#v, %v", page, err)
	}
	rows, err := ProjectList(page)
	if err != nil || len(rows) != 1 || rows[0]["taskUuid"] != "u1" {
		t.Fatalf("projection = %#v, %v", rows, err)
	}
}

func TestCrossPlatformCoverageListUnknownOrMalformedIsNotEmptySuccessE2E(t *testing.T) {
	tests := []string{
		`{}`,
		`{"success":true}`,
		`{"success":false,"errorMsg":"denied"}`,
		`{"success":true,"result":{"itemList":null}}`,
		`{"success":true,"result":{"itemList":{}}}`,
		`{"success":true,"result":{"itemList":["bad"]}}`,
		`{"success":true,"result":{"hasMore":true,"itemList":[]}}`,
	}
	for _, raw := range tests {
		if page, err := ParseListPage(decode(t, raw)); err == nil {
			t.Fatalf("unknown response accepted as %#v: %s", page, raw)
		}
	}
	page, err := ParseListPage(decode(t, `{"success":true,"result":{"itemList":[{"title":"missing uuid"}]}}`))
	if err != nil {
		t.Fatalf("parse list envelope: %v", err)
	}
	if rows, err := ProjectList(page); err == nil || rows != nil {
		t.Fatalf("malformed item projected: %#v, %v", rows, err)
	}
}

func TestCrossPlatformCoverageTranscriptShapeAndPaginationContractE2E(t *testing.T) {
	page, err := ParseTranscriptPage(decode(t, `{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`))
	if err != nil || len(page.Items) != 1 || !page.HasMore || page.NextToken != "n2" {
		t.Fatalf("transcript page = %#v, %v", page, err)
	}
	if key, err := StableItemKey(page.Items[0]); err != nil || key != "id:p1" {
		t.Fatalf("stable key = %q, %v", key, err)
	}
	for _, raw := range []string{
		`{"success":true,"result":{"paragraphList":null}}`,
		`{"success":true,"result":{"paragraphList":[],"hasNext":true}}`,
		`{"success":true,"result":{}}`,
	} {
		if _, err := ParseTranscriptPage(decode(t, raw)); err == nil {
			t.Fatalf("invalid transcript accepted: %s", raw)
		}
	}
}

func TestCrossPlatformCoverageLatestRequiresComparableTimestampE2E(t *testing.T) {
	page, err := ParseListPage(decode(t, `{"result":{"itemList":[{"uuid":"old","startTime":1},{"uuid":"new","startTime":"2"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := LatestTaskUUID(page); err != nil || got != "new" {
		t.Fatalf("latest = %q, %v", got, err)
	}
	page, err = ParseListPage(decode(t, `{"result":{"itemList":[{"uuid":"u"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LatestTaskUUID(page); err == nil || !strings.Contains(err.Error(), "comparable") {
		t.Fatalf("missing time accepted: %v", err)
	}
}

func TestCrossPlatformCoverageArtifactEmptyResultIsNotSuccessE2E(t *testing.T) {
	valid := map[string]string{
		"basic":    `{"success":true,"result":{"taskUuid":"u"}}`,
		"summary":  `{"success":true,"result":{"fullSummary":""}}`,
		"keywords": `{"success":true,"result":{"keywords":[]}}`,
		"todos":    `{"success":true,"result":{"actions":[]}}`,
	}
	for name, raw := range valid {
		if err := ValidateArtifact(name, "u", decode(t, raw)); err != nil {
			t.Fatalf("valid %s rejected: %v", name, err)
		}
	}
	for _, name := range []string{"basic", "summary", "keywords", "todos"} {
		if err := ValidateArtifact(name, "u", decode(t, `{"success":true,"result":{}}`)); err == nil {
			t.Fatalf("empty %s result accepted", name)
		}
	}
}
