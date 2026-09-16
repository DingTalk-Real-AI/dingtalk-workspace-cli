// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageParityFormFailureBoundaries(t *testing.T) {
	form := upsertByKeyStep{text: `{"forms":[{"viewId":"v","name":"N"}]}`}
	fields := upsertByKeyStep{text: `{"fields":[{"fieldId":"f","fieldName":"F","type":"text"}]}`}
	ack := upsertByKeyStep{text: `{"success":true}`}
	fail := upsertByKeyStep{err: fmt.Errorf("offline")}
	for _, tc := range []struct {
		name, cmd string
		args      []string
		steps     []upsertByKeyStep
	}{
		{"bad-value", "+form-submit", []string{"--view-id", "v", "--value", "{"}, nil},
		{"empty-value", "+form-submit", []string{"--view-id", "v", "--value", "{}"}, nil},
		{"empty-name", "+form-create", []string{"--name", " "}, nil},
		{"directory-unavailable", "+form-get", []string{"--view-id", "v"}, []upsertByKeyStep{fail}},
		{"directory-missing", "+form-get", []string{"--view-id", "v"}, []upsertByKeyStep{{text: `{}`}}},
		{"directory-duplicate", "+form-get", []string{"--view-id", "v"}, []upsertByKeyStep{{text: `{"forms":[{"viewId":"v"},{"viewId":"v"}]}`}}},
		{"field-unavailable", "+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}, []upsertByKeyStep{form, fail}},
		{"field-missing", "+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}, []upsertByKeyStep{form, {text: `{}`}}},
		{"field-absent", "+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}, []upsertByKeyStep{form, {text: `{"fields":[]}`}}},
		{"create-failed", "+form-create", []string{"--name", "N"}, []upsertByKeyStep{fail}},
		{"create-no-id", "+form-create", []string{"--name", "N"}, []upsertByKeyStep{ack}},
		{"created-name-wrong", "+form-create", []string{"--name", "Wrong"}, []upsertByKeyStep{{text: `{"viewId":"v"}`}, form}},
		{"post-write-directory-unavailable", "+form-create", []string{"--name", "N"}, []upsertByKeyStep{{text: `{"viewId":"v"}`}, fail}},
		{"visible-unavailable", "+form-get", []string{"--view-id", "v"}, []upsertByKeyStep{form, form, fail}},
		{"visible-missing", "+form-get", []string{"--view-id", "v"}, []upsertByKeyStep{form, form, {text: `{}`}}},
		{"submit-no-id", "+form-submit", []string{"--view-id", "v", "--value", `{"f":"value"}`}, []upsertByKeyStep{form, ack}},
		{"submit-read-failed", "+form-submit", []string{"--view-id", "v", "--value", `{"f":"value"}`}, []upsertByKeyStep{form, {text: `{"rowId":"r"}`}, fail}},
		{"submit-row-absent", "+form-submit", []string{"--view-id", "v", "--value", `{"f":"value"}`}, []upsertByKeyStep{form, {text: `{"rowId":"r"}`}, {text: `{"records":[],"hasMore":false}`}}},
		{"submit-wrong-value", "+form-submit", []string{"--view-id", "v", "--value", `{"f":"value"}`}, []upsertByKeyStep{form, {text: `{"rowId":"r"}`}, {text: `{"records":[{"recordId":"r","cells":{"f":"other"}}],"hasMore":false}`}}},
		{"hidden-question-no-id", "+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}, []upsertByKeyStep{form, fields, ack, form, {text: `{"fields":[{}]}`}}},
		{"question-still-visible", "+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}, []upsertByKeyStep{form, fields, ack, form, fields}},
		{"field-was-deleted", "+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}, []upsertByKeyStep{form, fields, ack, form, {text: `{"fields":[]}`}, {text: `{"fields":[]}`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &upsertByKeyCaller{steps: tc.steps}
			out, err := runAITableCompositeCLI(t, c, tc.cmd, append([]string{"--base-id", "b", "--table-id", "t", "--yes"}, tc.args...)...)
			if err == nil || out != "" {
				t.Fatal("false success", out, err)
			}
			if len(c.calls) != len(tc.steps) {
				t.Fatal("wrong dispatch boundary", len(c.calls), len(tc.steps), err)
			}
		})
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+form-create", "--base-id", "b", "--table-id", "t", "--name", "N", "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
}
