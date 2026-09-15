// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAITableBaseListRetainsEmptyPageContinuation(t *testing.T) {
	for _, command := range []string{"+base-list", "+base-search"} {
		for _, tc := range []struct {
			raw, cursor string
			fail        bool
		}{
			{`{"data":{"bases":[],"nextCursor":"next"}}`, "", false},
			{`{"bases":[],"hasMore":true}`, "", true},
			{`{"bases":[],"hasMore":true,"nextCursor":"next"}`, "next", true},
			{`{"bases":[],"hasMore":false,"nextCursor":"next"}`, "next", false},
		} {
			c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.raw}}}
			args := []string{}
			if command == "+base-search" {
				args = append(args, "--query", "name")
			}
			if tc.cursor != "" {
				args = append(args, "--cursor", tc.cursor)
			}
			out, err := runAITableCompositeCLI(t, c, command, args...)
			if (err != nil) != tc.fail || len(c.calls) != 1 {
				t.Fatalf("command=%s out=%s err=%v calls=%d", command, out, err, len(c.calls))
			}
			if tc.fail {
				if out != "" {
					t.Fatal("invalid page emitted success", out)
				}
				continue
			}
			if tc.cursor == "" && (!strings.Contains(out, `"nextCursor": "next"`) || !strings.Contains(out, `"hasMore": true`)) {
				t.Fatal(out)
			}
		}
	}
}

func TestCrossPlatformCoverageAITableFieldDescriptionPreservesEmpty(t *testing.T) {
	for _, value := range []string{"meaning", " spaced ", ""} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true}`}}}
		_, err := runAITableCompositeCLI(t, c, "+field-update", "--base-id", "b", "--table-id", "t", "--field-id", "f", "--description", value, "--yes")
		if err != nil || len(c.calls) != 1 || c.calls[0].args["description"] != value {
			t.Fatal(err, c.calls)
		}
	}
}

func TestCrossPlatformCoverageAITableParityAliasesReachSameRequest(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[{"fieldId":"f1"},{"fieldId":"f2"}]}`}}}
	_, err := runAITableCompositeCLI(t, c, "+field-list", "--base-id", "b", "--table-id", "t", "--field-ids", "f1,f2")
	if err != nil || len(c.calls) != 1 || c.calls[0].args["baseId"] != "b" {
		t.Fatal(err, c.calls)
	}
	ids := c.calls[0].args["fieldIds"].([]string)
	if len(ids) != 2 || ids[0] != "f1" || ids[1] != "f2" {
		t.Fatal(ids)
	}
	c = &upsertByKeyCaller{}
	_, err = runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--page-size", "2", "--limit", "1")
	if err == nil || len(c.calls) != 0 {
		t.Fatal("conflicting alias reached network", err, c.calls)
	}
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"bases":[]}`}}}
	_, err = runAITableCompositeCLI(t, c, "+title-resolve", "--keyword", "missing")
	if err != nil || c.calls[0].args["query"] != "missing" {
		t.Fatal(err, c.calls)
	}
}
