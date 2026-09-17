// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageParityFieldCreationFailures(t *testing.T) {
	fields := `[{"fieldName":"New","type":"text"}]`
	fail := upsertByKeyStep{err: fmt.Errorf("offline")}
	for _, tc := range []struct {
		name  string
		args  []string
		steps []upsertByKeyStep
	}{
		{"malformed", []string{"--fields", "{"}, nil}, {"empty", []string{"--fields", "[]"}, nil},
		{"bad-resume", []string{"--fields", fields, "--resume-field-ids", "a,b"}, nil},
		{"read-resume-failed", []string{"--fields", fields, "--resume-field-ids", "a"}, []upsertByKeyStep{fail}},
		{"missing-resume-fields", []string{"--fields", fields, "--resume-field-ids", "a"}, []upsertByKeyStep{{text: `{}`}}},
		{"wrong-resume-id", []string{"--fields", fields, "--resume-field-ids", "a"}, []upsertByKeyStep{{text: `{"fields":[{"fieldId":"b","fieldName":"New","type":"text"}]}`}}},
		{"missing-resume-id", []string{"--fields", fields, "--resume-field-ids", "a"}, []upsertByKeyStep{{text: `{"fields":[]}`}}},
		{"preflight-failed", []string{"--fields", fields}, []upsertByKeyStep{fail}},
		{"preflight-missing", []string{"--fields", fields}, []upsertByKeyStep{{text: `{}`}}},
		{"preflight-bad-id", []string{"--fields", fields}, []upsertByKeyStep{{text: `{"fields":[{"fieldName":"Other"}]}`}}},
		{"duplicate-name", []string{"--fields", fields}, []upsertByKeyStep{{text: `{"fields":[{"fieldId":"a","fieldName":"New","type":"text"}]}`}}},
		{"write-error", []string{"--fields", fields}, []upsertByKeyStep{{text: `{"fields":[]}`}, fail}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &upsertByKeyCaller{steps: tc.steps}
			out, err := runAITableCompositeCLI(t, c, "+field-create", append([]string{"--base-id", "b", "--table-id", "t", "--yes"}, tc.args...)...)
			if err == nil || out != "" || len(c.calls) != len(tc.steps) {
				t.Fatal(out, err, len(c.calls), len(tc.steps))
			}
		})
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", fields, "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
	for _, tc := range []struct {
		name   string
		cancel bool
		resume bool
		oldID  bool
	}{
		{"delayed-read-exhausted", false, false, false}, {"cancelled-read", true, false, false}, {"partial-resume-read-exhausted", false, true, false}, {"returned-old-id", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error {
				if tc.cancel {
					return context.Canceled
				}
				return nil
			})
			creates := 0
			c := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
				if tool == "create_fields" {
					creates++
					return `{"results":[{"fieldName":"New","fieldId":"n","success":true}]}`, nil
				}
				if ids, ok := args["fieldIds"].([]string); ok {
					if len(ids) == 1 && ids[0] == "a" {
						return `{"fields":[{"fieldName":"Old","fieldId":"a","type":"text"}]}`, nil
					}
					if tc.oldID {
						return `{"fields":[{"fieldName":"New","fieldId":"n","type":"text"}]}`, nil
					}
					return `{"fields":[]}`, nil
				}
				if tc.oldID {
					return `{"fields":[{"fieldName":"Other","fieldId":"n","type":"text"}]}`, nil
				}
				return `{"fields":[]}`, nil
			}}
			argv := []string{"--base-id", "b", "--table-id", "t", "--fields", fields, "--yes"}
			if tc.resume {
				argv = []string{"--base-id", "b", "--table-id", "t", "--fields", `[{"fieldName":"Old","type":"text"},{"fieldName":"New","type":"text"}]`, "--resume-field-ids", "a", "--yes"}
			}
			out, err := runAITableCompositeCLI(t, c, "+field-create", argv...)
			if err == nil || out != "" || creates != 1 {
				t.Fatal("replayed unknown write or claimed success", out, err, creates)
			}
		})
	}
}
func TestCrossPlatformCoverageParityFieldReceiptsRejectMalformedItems(t *testing.T) {
	expected := []any{map[string]any{"fieldName": "A"}, map[string]any{"fieldName": "B"}}
	for _, rows := range []any{nil, []any{nil}, []any{map[string]any{"fieldName": "Other", "fieldId": "x", "success": true}}, []any{map[string]any{"fieldName": "A", "success": true}}, []any{map[string]any{"fieldName": "A", "fieldId": "x", "success": true}, map[string]any{"fieldName": "B", "fieldId": "x", "success": true}}} {
		if _, err := parityCreatedFieldIDs(map[string]any{"results": rows}, expected); err == nil {
			t.Fatal(rows)
		}
	}
}
func TestCrossPlatformCoverageParityBatchCreateExactEntry(t *testing.T) {
	for _, raw := range []string{`[]`, `[{"recordId":"r","cells":{"f":"v"}}]`} {
		c := &upsertByKeyCaller{}
		if _, err := runAITableCompositeCLI(t, c, "+record-batch-create", "--base-id", "b", "--table-id", "t", "--records", raw, "--yes"); err == nil || len(c.calls) != 0 {
			t.Fatal(raw, err)
		}
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+record-batch-create", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"f":"v"}}]`, "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, "planned") {
		t.Fatal(out, err)
	}
}
func TestCrossPlatformCoverageParityAIValidationAndFailures(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		steps []upsertByKeyStep
	}{
		{[]string{"--field-ids", "a,a"}, nil}, {[]string{"--field-ids", "a", "--record-ids", "r,r"}, nil},
		{[]string{"--field-ids", "a"}, []upsertByKeyStep{{err: fmt.Errorf("offline")}}},
		{[]string{"--field-ids", "a"}, []upsertByKeyStep{{text: `{}`}}},
		{[]string{"--field-ids", "a"}, []upsertByKeyStep{{text: `{"fields":[{"fieldId":"a"}]}`}, {err: fmt.Errorf("NOT_AI_FIELD")}}},
	} {
		c := &upsertByKeyCaller{steps: tc.steps}
		out, err := runAITableCompositeCLI(t, c, "+field-run-ai", append([]string{"--base-id", "b", "--table-id", "t", "--yes"}, tc.args...)...)
		if err == nil || out != "" {
			t.Fatal(out, err)
		}
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+field-run-ai", "--base-id", "b", "--table-id", "t", "--field-ids", "a", "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
	for _, cfg := range []string{`null`, `{"prompt":[null],"outputType":"text"}`, `{"prompt":[{"type":"text"}],"outputType":"text"}`, `{"prompt":[{"type":"fieldRef"}],"outputType":"text"}`, `{"prompt":[{"type":"unknown"}],"outputType":"text"}`} {
		if _, err := parseBootstrapFields(`[{"fieldName":"AI","type":"text","aiConfig":` + cfg + `}]`); err == nil {
			t.Fatal(cfg)
		}
	}
	if _, err := parseBootstrapFields(`[{"fieldName":"AI","type":"text","aiConfig":{"prompt":[{"type":"fieldRef","fieldId":"f"}],"outputType":"text"}}]`); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageParityFieldResumeRestoresDeclarationOrder(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[{"fieldId":"a","fieldName":"Second","type":"text"},{"fieldId":"z","fieldName":"First","type":"text"}]}`}}}
	out, err := runAITableCompositeCLI(t, c, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", `[{"fieldName":"First","type":"text"},{"fieldName":"Second","type":"text"}]`, "--resume-field-ids", "a,z", "--yes")
	if err != nil {
		t.Fatal(out, err)
	}
	var v map[string]any
	if e := json.Unmarshal([]byte(out), &v); e != nil {
		t.Fatal(e)
	}
	ids := v["resolved"].(map[string]any)["fieldIds"].([]any)
	if len(ids) != 2 || ids[0] != "z" || ids[1] != "a" || len(c.calls) != 1 {
		t.Fatal(out, c.calls)
	}
}

func TestCrossPlatformCoverageParityAIRunRejectsOversizedSelectors(t *testing.T) {
	for _, tc := range []struct{ fields, records int }{{11, 0}, {1, 501}} {
		args := []string{"--base-id", "b", "--table-id", "t", "--field-ids", strings.Join(recordIDFixtures(tc.fields), ","), "--yes"}
		if tc.records > 0 {
			args = append(args, "--record-ids", strings.Join(recordIDFixtures(tc.records), ","))
		}
		c := &upsertByKeyCaller{}
		out, err := runAITableCompositeCLI(t, c, "+field-run-ai", args...)
		if err == nil || out != "" || len(c.calls) != 0 {
			t.Fatal(out, err, len(c.calls))
		}
	}
}
