// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageTableUpdateNavigationOutcomes(t *testing.T) {
	for _, status := range []any{"verified", "pending", "unknown", "failed", "future_state", "", nil, 42, []any{}, map[string]any{}} {
		body := map[string]any{"baseId": "b", "tableId": "t", "verificationStatus": status, "url": "server-url", "links": []any{}, "noOp": true}
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"status": "success", "data": body})}}
		out, err := runAITableCompositeCLI(t, caller, "+table-update", "--base-id", "b", "--table-id", "t", "--name", "Name", "--yes")
		if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "update_table" || caller.calls[0].args["newTableName"] != "Name" {
			t.Fatal(err, out, caller.calls)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err, out)
		}
		switch status {
		case "verified", "pending":
			want := "success"
			if status == "pending" {
				want = "pending"
			}
			if envelope["outcome"] != want || !reflect.DeepEqual(envelope["data"], body) {
				t.Fatal(envelope)
			}
		default:
			if envelope["outcome"] != "failure" || envelope["error"].(map[string]any)["exit_code"] == float64(0) {
				t.Fatal(envelope)
			}
		}
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"status": "success", "data": map[string]any{"tableId": "t"}})}}
	out, err := runAITableCompositeCLI(t, caller, "+table-update", "--base-id", "b", "--table-id", "t", "--name", "Name", "--yes")
	if err != nil || !strings.Contains(out, `"outcome": "failure"`) || len(caller.calls) != 1 {
		t.Fatal(out, err, caller.calls)
	}
	caller = &upsertByKeyCaller{}
	if _, err := runAITableCompositeCLI(t, caller, "+table-update", "--base-id", "b", "--table-id", "t", "--dry-run"); err != nil || len(caller.calls) != 0 {
		t.Fatal(err, caller.calls)
	}
}

func TestCrossPlatformCoverageImportDocsNavigation(t *testing.T) {
	base := "https://docs.dingtalk.com/i/nodes/base"
	table := base + "?iframeQuery=entrance%3ddata%26sheetId%3dtable"
	for _, status := range []string{"success", "pending", "partial_failure"} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"status": "success", "data": map[string]any{
			"status": status, "url": table, "links": []any{map[string]any{"url": base}}, "uploadUrl": table,
		}})}}
		out, err := runAITableCompositeCLI(t, caller, "+import-file", "--resume-import-id", "imp", "--yes")
		var final map[string]any
		if status == "partial_failure" {
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Retryable {
				t.Fatal(out, err)
			}
			final = typed.Details["result"].(compositeResult).Result
		} else {
			if err != nil {
				t.Fatal(err, out)
			}
			final = navigationFinalResult(t, out)
		}
		if final["url"] != table || final["response"].(map[string]any)["data"].(map[string]any)["uploadUrl"] != "<redacted>" || len(caller.calls) != 1 {
			t.Fatal(final, caller.calls)
		}
	}
	for _, raw := range []string{
		"http://docs.dingtalk.com/i/nodes/base", "https://docs.dingtalk.com.evil.example/i/nodes/base",
		"https://user@docs.dingtalk.com/i/nodes/base", table + "&token=secret", base + "?token=secret",
	} {
		if isImportNavigationURL(raw) || sanitizeImportOutput(raw, "url") == raw {
			t.Fatal("unsafe docs URL retained", raw)
		}
	}
}

func TestCrossPlatformCoverageBaseCompositeNavigation(t *testing.T) {
	cases := []struct {
		name       string
		read, want map[string]any
	}{
		{"receipt fallback", nil, map[string]any{"url": "receipt", "links": []any{}}},
		{"read wins", map[string]any{"url": "read", "links": []any{map[string]any{"name": "read"}}}, map[string]any{"url": "read", "links": []any{map[string]any{"name": "read"}}}},
		{"null wins", map[string]any{"url": nil, "links": nil}, map[string]any{"url": nil, "links": nil}},
		{"per key", map[string]any{"url": ""}, map[string]any{"url": "", "links": []any{}}},
	}
	for _, command := range []string{"+base-copy", "+base-bootstrap"} {
		for _, tc := range cases {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				receipt := map[string]any{"baseId": "new", "url": "receipt", "links": []any{}}
				read := map[string]any{"baseId": "new"}
				maps.Copy(read, tc.read)
				steps := []upsertByKeyStep{parityStep(map[string]any{"data": receipt}), parityStep(map[string]any{"data": read})}
				args := []string{"--base-id", "source", "--yes"}
				if command == "+base-bootstrap" {
					args = []string{"--name", "Name", "--tables", `[{"name":"Table","fields":[]}]`, "--yes"}
					steps = append(steps,
						parityStep(map[string]any{"data": map[string]any{"tableId": "t", "url": "table-receipt", "links": []any{}}}),
						parityStep(map[string]any{"tables": []any{map[string]any{"tableId": "t", "url": "table-read"}}}),
						parityStep(map[string]any{"fields": []any{}}),
					)
				}
				caller := &upsertByKeyCaller{steps: steps}
				out, err := runAITableCompositeCLI(t, caller, command, args...)
				if err != nil {
					t.Fatal(err, out)
				}
				final := navigationFinalResult(t, out)
				for key, want := range tc.want {
					if got, present := final[key]; !present || !reflect.DeepEqual(got, want) {
						t.Fatalf("%s = %#v, want %#v: %s", key, got, want, out)
					}
				}
				if command == "+base-bootstrap" {
					table := final["tables"].([]any)[0].(map[string]any)
					if table["url"] != "table-read" || !reflect.DeepEqual(table["links"], []any{}) {
						t.Fatal(table)
					}
				}
				if len(caller.calls) != len(steps) {
					t.Fatal(caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageBaseNavigationSurvivesPartialFailure(t *testing.T) {
	for _, command := range []string{"+base-copy", "+base-bootstrap"} {
		for _, wrongID := range []bool{false, true} {
			t.Run(command+map[bool]string{false: "/read-error", true: "/wrong-identity"}[wrongID], func(t *testing.T) {
				failure := upsertByKeyStep{err: errors.New("read unavailable")}
				if wrongID {
					failure = parityStep(map[string]any{"baseId": "other", "url": "wrong", "nested": map[string]any{"baseId": "new"}})
				}
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
					parityStep(map[string]any{"data": map[string]any{"baseId": "new", "url": "receipt"}}), failure,
				}}
				args := []string{"--base-id", "source", "--yes"}
				if command == "+base-bootstrap" {
					args = []string{"--name", "Name", "--tables", `[{"name":"Table","fields":[]}]`, "--yes"}
				}
				out, err := runAITableCompositeCLI(t, caller, command, args...)
				var typed *apperrors.Error
				if out != "" || !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 2 {
					t.Fatal(out, err, caller.calls)
				}
				result := typed.Details["result"].(compositeResult)
				if result.Status != "partial_success" || result.Result["url"] != "receipt" {
					t.Fatal(result)
				}
			})
		}
	}
	for _, value := range []map[string]any{
		{"baseId": "source", "newBaseId": "new", "url": "wrong"}, {"nested": map[string]any{"baseId": "new", "url": "wrong"}},
	} {
		if baseNavigation(value, "new") != nil {
			t.Fatal("unrelated identity supplied navigation", value)
		}
	}
	if baseNavigation(map[string]any{"newBaseId": "new", "url": "new"}, "new")["url"] != "new" {
		t.Fatal("lost explicit copy identity")
	}
}

func TestCrossPlatformCoverageBaseBootstrapMissingIdentityPreservesReceipt(t *testing.T) {
	receipt := map[string]any{"baseName": "Name", "traceId": "creation-receipt"}
	response := map[string]any{"status": "success", "data": receipt}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		parityStep(response),
	}}
	out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap", "--name", "Name", "--tables", `[{"name":"Table","fields":[]}]`, "--yes")
	var typed *apperrors.Error
	if out != "" || !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 1 {
		t.Fatal(out, err, caller.calls)
	}
	result := typed.Details["result"].(compositeResult)
	if result.Status != "unknown" || !result.Executed || !reflect.DeepEqual(result.Checkpoint["createReceipt"], response) {
		t.Fatalf("lost unverified creation receipt: %#v", result)
	}
	if result.Result != nil || !strings.Contains(result.NextCommand, "+base-search") {
		t.Fatalf("unknown identity published navigation or lost recovery: %#v", result)
	}
}

func TestCrossPlatformCoverageImportNavigationAndCredentialRedaction(t *testing.T) {
	base := "https://alidocs.dingtalk.com/i/nodes/base"
	table := base + "?iframeQuery=entrance%3Ddata%26sheetId%3Dtable"
	for _, status := range []string{"success", "pending"} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"status": "success", "data": map[string]any{
			"status": status, "baseId": "base", "tableId": "table", "url": table,
			"links": []any{map[string]any{"url": base}}, "uploadUrl": "https://secret.example/?token=secret",
		}})}}
		out, err := runAITableCompositeCLI(t, caller, "+import-file", "--resume-import-id", "imp", "--yes")
		if err != nil {
			t.Fatal(err, out)
		}
		final := navigationFinalResult(t, out)
		if final["url"] != table || final["baseId"] != "base" || len(caller.calls) != 1 {
			t.Fatal(final, caller.calls)
		}
		response := final["response"].(map[string]any)["data"].(map[string]any)
		if response["uploadUrl"] != "<redacted>" {
			t.Fatal(response)
		}
	}
	for _, raw := range []string{
		"%", "http://alidocs.dingtalk.com/i/nodes/base", "https://user@alidocs.dingtalk.com/i/nodes/base", base + "#fragment",
		"https://upload.example/i/nodes/base", "https://alidocs.dingtalk.com/other/base", base + "/other", base + "?token=secret",
		base + "?iframeQuery=%zz", base + "?iframeQuery=x&iframeQuery=y", base + "?iframeQuery=entrance%3Ddata%26sheetId%3Dt%26token%3Dsecret",
		base + "?iframeQuery=entrance%3Ddata%26sheetId%3D", base + "?iframeQuery=entrance%3Dapps%26sheetId%3Dt",
	} {
		if isImportNavigationURL(raw) || (strings.Contains(raw, "http") && sanitizeImportOutput(raw, "url") == raw) {
			t.Fatalf("unsafe URL retained: %s", raw)
		}
	}
	if !isImportNavigationURL(base) || !isImportNavigationURL(table) || sanitizeImportOutput(table, "uploadUrl") != "<redacted>" {
		t.Fatal("navigation and upload URLs were conflated")
	}
	for _, status := range []string{"all_fail", "timeout"} {
		if importFileOutcome(map[string]any{"status": "success", "data": map[string]any{"status": status}}) != "failure" {
			t.Fatal("task failure became success", status)
		}
	}
}

func TestCrossPlatformCoverageImportNavigationCannotConfirmUnknownTask(t *testing.T) {
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"unknown task", map[string]any{"status": "unrecognized"}},
		{"missing task state", map[string]any{"taskId": "job"}},
		{"malformed task", map[string]any{"status": []any{"success"}}},
		{"contradictory outcome", map[string]any{"ok": true, "outcome": "success", "status": "pending"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.body["baseId"] = "base"
			tc.body["url"] = "https://alidocs.dingtalk.com/i/nodes/base"
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"status": "success", "data": tc.body})}}
			out, err := runAITableCompositeCLI(t, caller, "+import-file", "--resume-import-id", "original-import", "--yes")
			var typed *apperrors.Error
			if out != "" || !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 1 || caller.calls[0].args["importId"] != "original-import" {
				t.Fatal(out, err, caller.calls)
			}
			result := typed.Details["result"].(compositeResult)
			if result.Status != "unknown" || result.CompletedCount != 0 || result.Verification != nil || result.Checkpoint["importId"] != "original-import" || result.Result["url"] != tc.body["url"] {
				t.Fatal("task was confirmed or lost recovery/navigation", result)
			}
		})
	}
}
