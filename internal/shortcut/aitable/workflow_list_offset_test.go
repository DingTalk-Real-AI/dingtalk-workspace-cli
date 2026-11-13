// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageWorkflowListOffsetCompleteness(t *testing.T) {
	dataset := make([]any, 21)
	ids := make([]string, 21)
	var enabled, disabled []string
	for i := range dataset {
		ids[i] = fmt.Sprintf("workflow-%02d", i)
		status := "disabled"
		if i%2 == 0 {
			status = "enabled"
			enabled = append(enabled, ids[i])
		} else {
			disabled = append(disabled, ids[i])
		}
		dataset[i] = map[string]any{"workflowId": ids[i], "status": status}
	}
	for _, tc := range []struct {
		name      string
		args      []string
		wantError string
		wantIDs   []string
		offsets   []int
		complete  bool
		more      bool
		next      int
	}{
		{name: "all rejects skipping one", args: []string{"--all", "--offset", "1"}, wantError: "--all requires --offset 0"},
		{name: "all rejects skipped prefix", args: []string{"--all", "--offset", "20"}, wantError: "--all requires --offset 0"},
		{name: "enabled rejects skipped prefix", args: []string{"--all", "--status", "enabled", "--offset", "20"}, wantError: "--all requires --offset 0"},
		{name: "disabled rejects skipped prefix", args: []string{"--all", "--status", "disabled", "--offset", "20"}, wantError: "--all requires --offset 0"},
		{name: "all rejects offset past end", args: []string{"--all", "--offset", "21"}, wantError: "--all requires --offset 0"},
		{name: "status still requires all", args: []string{"--status", "enabled", "--offset", "20"}, wantError: "--status requires --all"},
		{name: "all defaults to zero", args: []string{"--all"}, wantIDs: ids, offsets: []int{0, 20}, complete: true},
		{name: "all accepts explicit zero", args: []string{"--all", "--offset", "0"}, wantIDs: ids, offsets: []int{0, 20}, complete: true},
		{name: "enabled includes first page", args: []string{"--all", "--status", "enabled", "--offset", "0"}, wantIDs: enabled, offsets: []int{0, 20}, complete: true},
		{name: "disabled includes first page", args: []string{"--all", "--status", "disabled"}, wantIDs: disabled, offsets: []int{0, 20}, complete: true},
		{name: "ordinary tail page", args: []string{"--offset", "20"}, wantIDs: ids[20:], offsets: []int{20}},
		{name: "ordinary middle page", args: []string{"--offset", "1", "--limit", "10"}, wantIDs: ids[1:11], offsets: []int{1}, more: true, next: 11},
		{name: "explicit false stays paged", args: []string{"--all=false", "--offset", "20"}, wantIDs: ids[20:], offsets: []int{20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
				if tool != "list_workflows" || args["baseId"] != "fixture-base" {
					return "", fmt.Errorf("unexpected request: %s %v", tool, args)
				}
				offset, limit := args["offset"].(int), args["limit"].(int)
				if offset > len(dataset) {
					offset = len(dataset)
				}
				end := min(offset+limit, len(dataset))
				return mustJSONText(t, map[string]any{"workflows": dataset[offset:end], "hasMore": end < len(dataset)}), nil
			}}
			out, err := runAITableCompositeCLI(t, caller, "+workflow-list", append([]string{"--base-id", "fixture-base"}, tc.args...)...)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || len(caller.calls) != 0 || out != "" {
					t.Fatalf("want rejection before RPC (%s); err=%v calls=%v output=%s", tc.wantError, err, caller.calls, out)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Count     int `json:"count"`
				Workflows []struct {
					ID string `json:"workflowId"`
				} `json:"workflows"`
				Complete *bool `json:"complete"`
				Scanned  *int  `json:"scannedCount"`
				More     bool  `json:"hasMore"`
				Next     *int  `json:"nextOffset"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatalf("output=%s: %v", out, err)
			}
			var gotIDs []string
			for _, row := range result.Workflows {
				gotIDs = append(gotIDs, row.ID)
			}
			if !reflect.DeepEqual(gotIDs, tc.wantIDs) || result.Count != len(tc.wantIDs) {
				t.Fatalf("incomplete or wrong scope: %s; want IDs=%v", out, tc.wantIDs)
			}
			if tc.complete {
				if result.Complete == nil || !*result.Complete || result.Scanned == nil || *result.Scanned != len(dataset) {
					t.Fatalf("incorrect completeness: %s", out)
				}
			} else if result.Complete != nil || result.Scanned != nil {
				t.Fatalf("page claims whole-set completion: %s", out)
			}
			if result.More != tc.more || (tc.more && (result.Next == nil || *result.Next != tc.next)) || (!tc.more && result.Next != nil) {
				t.Fatalf("incorrect pagination: %s", out)
			}
			var offsets []int
			for _, call := range caller.calls {
				offsets = append(offsets, call.args["offset"].(int))
			}
			if !reflect.DeepEqual(offsets, tc.offsets) {
				t.Fatalf("offsets=%v want=%v", offsets, tc.offsets)
			}
		})
	}
}
