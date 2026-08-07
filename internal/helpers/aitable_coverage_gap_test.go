// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/paging"
)

func TestCrossPlatformCoverageAITablePageParsingRemainingEdges(t *testing.T) {
	for name, raw := range map[string]string{
		"trailing invalid JSON": `{"records":[]} {`,
		"data not object":       `{"data":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRecordQueryPage(raw); err == nil {
				t.Fatalf("parseRecordQueryPage(%q) succeeded", raw)
			}
		})
	}
}

func TestCrossPlatformCoverageAITableOptionalCountNumericTypes(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		want    int
		wantErr bool
	}{
		{name: "bad number", value: json.Number("1.5"), wantErr: true},
		{name: "fractional float", value: float64(1.5), wantErr: true},
		{name: "whole float", value: float64(2), want: 2},
		{name: "int", value: int(3), want: 3},
		{name: "int64", value: int64(4), want: 4},
		{name: "wrong type", value: "5", wantErr: true},
		{name: "negative", value: int64(-1), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOptionalNonNegativeInt(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("value %#v succeeded: %#v", tc.value, got)
				}
				return
			}
			if err != nil || got == nil || *got != tc.want {
				t.Fatalf("value %#v = %#v, %v", tc.value, got, err)
			}
		})
	}
}

func TestCrossPlatformCoverageAITableIncompleteResultMetadata(t *testing.T) {
	total := 9
	err := recordQueryIncompleteError(paging.Result{
		Records: []any{map[string]any{"id": "r"}}, Pages: 1, Attempts: 1,
		HasMore: true, LastCursor: "next", StopReason: paging.StopPageLimit,
		TotalCount: &total,
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "pagination incomplete") {
		t.Fatalf("incomplete result error = %v", err)
	}
}

func TestCrossPlatformCoverageAITableExplicitUnlimitedPaginationE2E(t *testing.T) {
	responses := make([]string, paging.DefaultPageLimit+1)
	for index := range responses {
		next := ""
		if index < len(responses)-1 {
			next = fmt.Sprintf(`,"nextCursor":"cursor-%d"`, index+1)
		}
		responses[index] = fmt.Sprintf(`{"records":[{"recordId":"record-%d"}]%s}`, index, next)
	}
	caller := &aitableTestCaller{responses: responses}
	out := installAitableDeps(t, caller)
	if err := recordQueryFetchAll(map[string]any{}, 0); err != nil {
		t.Fatalf("explicit unlimited pagination failed: %v", err)
	}
	if len(caller.calls) != len(responses) {
		t.Fatalf("explicit unlimited pagination calls = %d, want %d", len(caller.calls), len(responses))
	}
	for _, want := range []string{`"pages": 51`, `"fetchedCount": 51`, `"complete": true`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("explicit unlimited output missing %s: %s", want, out.String())
		}
	}
}
